//go:build !windows

package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"burnmon/internal/history"
	"burnmon/internal/store"
)

// browserBridge is the macOS/Linux stand-in for the WebView2 window's bound
// functions: a localhost-only HTTP server that serves dashboard.html with a
// small script injected at the top of <head>, defining window.bmLive,
// bmTurn, bmSessionInsight, bmHistory, bmVendorStrip, bmForecast, ccRefresh
// and ccSaveSettings as fetch calls back to /api/<name>. Without it, every
// live section of the page (Now, History, the vendor strip, the forecast,
// session findings, the turn drawer) waits forever for a function that is
// never defined.
//
// It listens on 127.0.0.1 only. Requests are refused unless the Host header
// names this exact listener (blocks DNS rebinding), and POSTs must be JSON
// from this origin (a foreign page cannot send one without a CORS preflight,
// which nothing here answers). Every request, the page included, must carry
// the per-run token: localhost is shared by every user and process on the
// machine, and the page itself holds token, project and session data
// (decision 2026-10-21, PR #4 condition 1).
type browserBridge struct {
	a     *app
	st    *store.Store
	addr  string // host:port the page is served from, e.g. 127.0.0.1:47615
	token string // per-run secret, required on every request

	mu       sync.Mutex
	startErr string // first collection's failure, shown on the warming page
}

// tokenHeader carries the per-run token on the page's API calls; the page
// itself gets it as the t query parameter.
const tokenHeader = "X-BurnMon-Token"

// bridgeTokenFile holds the current run's token in the data folder, readable
// only by the user, so a second start can open the running instance.
const bridgeTokenFile = "bridge-token"

func (b *browserBridge) url() string { return bridgeURL(b.addr, b.token) }

func bridgeURL(addr, token string) string { return "http://" + addr + "/?t=" + token }

// newBridgeToken makes a random 32-byte token for this run and writes it, as
// 64 hex characters, to bridgeTokenFile in dir with mode 0600.
func newBridgeToken(dir string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(raw)
	path := filepath.Join(dir, bridgeTokenFile)
	_ = os.Remove(path) // a fresh file, so the mode below always applies
	if err := os.WriteFile(path, []byte(tok), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}

// readBridgeToken returns the token in dir's bridgeTokenFile, or "" if there
// is none.
func readBridgeToken(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, bridgeTokenFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// probeBridge reports whether addr answers as a running BurnMon bridge that
// accepts token: a 200 JSON reply from /api/built with a "built" field.
// Anything else (another program, an old run, a wrong token) is false.
func probeBridge(addr, token string) bool {
	if token == "" {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/api/built", nil)
	if err != nil {
		return false
	}
	req.Header.Set(tokenHeader, token)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var reply map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return false
	}
	_, ok := reply["built"]
	return ok
}

func (b *browserBridge) tokenOK(r *http.Request) bool {
	got := r.Header.Get(tokenHeader)
	if got == "" {
		got = r.URL.Query().Get("t")
	}
	return b.token != "" && subtle.ConstantTimeCompare([]byte(got), []byte(b.token)) == 1
}

// staleLinkHTML answers a page request without a valid token, most likely a
// bookmark or an open tab from an earlier start.
const staleLinkHTML = `<!doctype html><meta charset="utf-8"><title>BurnMon</title>
<p style="font-family:sans-serif">This BurnMon link is missing its key or is from an earlier start.
Open the link burnmon printed when it started, or start burnmon again.</p>`

func (b *browserBridge) setStartErr(msg string) {
	b.mu.Lock()
	b.startErr = msg
	b.mu.Unlock()
}

func (b *browserBridge) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", b.servePage)
	mux.HandleFunc("/api/built", b.serveBuilt)
	mux.HandleFunc("/api/", b.serveAPI)
	return b.guard(mux)
}

func (b *browserBridge) guard(next http.Handler) http.Handler {
	_, port, _ := net.SplitHostPort(b.addr)
	allowedHosts := map[string]bool{b.addr: true, "localhost:" + port: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHosts[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if !b.tokenOK(r) {
			if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, staleLinkHTML)
				return
			}
			http.Error(w, "forbidden: missing or wrong token", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			if o := r.Header.Get("Origin"); o != "" && !allowedHosts[strings.TrimPrefix(o, "http://")] {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
			if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
				http.Error(w, "expected application/json", http.StatusUnsupportedMediaType)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// builtStamp identifies the dashboard.html on disk by its modification time,
// as a string (nanoseconds overflow a JS number). Empty when none exists yet.
func (b *browserBridge) builtStamp() string {
	fi, err := os.Stat(b.a.htmlPath)
	if err != nil {
		return ""
	}
	return strconv.FormatInt(fi.ModTime().UnixNano(), 10)
}

func (b *browserBridge) servePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html, err := b.readDashboard()
	if err != nil {
		// No snapshot yet (first run): the warming page, which reloads into
		// the dashboard once the first collection has written it.
		poll := b.fillScript(warmingPollScript)
		fmt.Fprint(w, strings.Replace(warmingPageHTML(), "</body>", poll+"</body>", 1))
		return
	}
	shim := strings.Replace(b.fillScript(bridgeScript), "__BUILT__", strconv.Quote(b.builtStamp()), 1)
	if i := strings.Index(html, "<head>"); i >= 0 {
		html = html[:i+len("<head>")] + shim + html[i+len("<head>"):]
	} else {
		html = shim + html
	}
	fmt.Fprint(w, html)
}

// fillScript puts this run's token and its header name into script.
func (b *browserBridge) fillScript(script string) string {
	script = strings.ReplaceAll(script, "__TOKEN__", strconv.Quote(b.token))
	return strings.ReplaceAll(script, "__HEADER__", strconv.Quote(tokenHeader))
}

// readDashboard reads dashboard.html, retrying briefly if it catches the
// file mid-rewrite (rebuild writes it in place).
func (b *browserBridge) readDashboard() (string, error) {
	var html string
	for i := 0; i < 10; i++ {
		raw, err := os.ReadFile(b.a.htmlPath)
		if err != nil {
			return "", err
		}
		html = string(raw)
		if strings.Contains(html, "</html>") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return html, nil
}

func (b *browserBridge) serveBuilt(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	startErr := b.startErr
	b.mu.Unlock()
	writeJSON(w, map[string]string{"built": b.builtStamp(), "error": startErr})
}

func (b *browserBridge) serveAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var args []json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		http.Error(w, "arguments must be a JSON array: "+err.Error(), http.StatusBadRequest)
		return
	}
	arg := func(i int, v any) error {
		if i >= len(args) {
			return fmt.Errorf("missing argument %d", i)
		}
		return json.Unmarshal(args[i], v)
	}

	var result any
	var err error
	switch name := strings.TrimPrefix(r.URL.Path, "/api/"); name {
	case "bmLive":
		buckets := make([]int, len(args))
		for i := range args {
			if err = arg(i, &buckets[i]); err != nil {
				break
			}
		}
		if err == nil {
			result, err = b.a.liveSnapshot(b.st, buckets...)
		}
	case "bmTurn":
		var sid string
		var turn int
		if err = errors.Join(arg(0, &sid), arg(1, &turn)); err == nil {
			result = b.a.turnDetail(b.st, sid, turn)
		}
	case "bmSessionInsight":
		var sid string
		if err = arg(0, &sid); err == nil {
			result = b.a.sessionInsight(b.st, sid)
		}
	case "bmHistory":
		var f history.Filter
		if err = arg(1, &f); err == nil {
			result, err = b.a.historyPayload(b.st, f)
		}
	case "bmVendorStrip":
		result, err = b.a.vendorStrip(b.st)
	case "bmForecast":
		result, err = b.a.forecastPayload(b.st)
	case "ccRefresh":
		_, err = b.a.rebuild(true, nil)
		if err != nil {
			log.Println("refresh failed:", err)
		}
	case "ccSaveSettings":
		var p settingsPayload
		if err = arg(0, &p); err == nil {
			if err = b.a.applySettings(p); err == nil {
				// Saved; a failed rebuild is reported separately so the page
				// can say "saved, but rebuild failed" like the app window.
				rebuildErr := ""
				if _, rerr := b.a.rebuild(true, nil); rerr != nil {
					log.Println("rebuild after settings save failed:", rerr)
					rebuildErr = rerr.Error()
				}
				result = map[string]string{"rebuildError": rebuildErr}
			}
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Println("browser bridge: encode response:", err)
	}
}

// bridgeScript defines the page's bound functions as calls to the bridge.
// bmLive, bmVendorStrip, bmForecast and ccSaveSettings return Promises, as
// go-webview2's own bindings do; bmTurn, bmSessionInsight and bmHistory
// deliver through the page's __bm*Resolve callbacks, matching main.go's
// asyncResolveJS. It also polls /api/built and reloads (keeping the tab,
// via ccReload) whenever a scheduled rebuild rewrites dashboard.html, which
// the app window does through w.Eval.
const bridgeScript = `<script>
(function(){
  var built = __BUILT__;
  var hdr = {}; hdr[__HEADER__] = __TOKEN__;
  function call(name, args){
    var h = {'Content-Type': 'application/json'}; h[__HEADER__] = __TOKEN__;
    return fetch('/api/' + name, {method: 'POST', headers: h, body: JSON.stringify(args || [])})
      .then(function(r){ return r.text().then(function(t){
        if(!r.ok) throw new Error(t || r.statusText);
        return t ? JSON.parse(t) : null;
      }); });
  }
  function reload(){ window.ccReload ? window.ccReload() : location.reload(); }
  function logErr(name){ return function(e){ console.error(name, e); }; }

  window.bmLive = function(){ return call('bmLive', Array.prototype.slice.call(arguments)); };
  window.bmVendorStrip = function(){ return call('bmVendorStrip'); };
  window.bmForecast = function(){ return call('bmForecast'); };
  window.bmTurn = function(sessionID, turn){
    call('bmTurn', [sessionID, turn]).then(function(d){
      window.__bmTurnResolve && window.__bmTurnResolve(sessionID + ':' + turn, d);
    }).catch(logErr('bmTurn'));
  };
  window.bmSessionInsight = function(sessionID){
    call('bmSessionInsight', [sessionID]).then(function(d){
      window.__bmSessionInsightResolve && window.__bmSessionInsightResolve(sessionID, d);
    }).catch(logErr('bmSessionInsight'));
  };
  window.bmHistory = function(reqID, filter){
    call('bmHistory', [reqID, filter]).then(function(d){
      window.__bmHistoryResolve && window.__bmHistoryResolve(reqID, d);
    }).catch(logErr('bmHistory'));
  };
  window.ccRefresh = function(){
    call('ccRefresh').then(reload).catch(function(){ window.ccRefreshFailed && window.ccRefreshFailed(); });
  };
  window.ccSaveSettings = function(p){
    return call('ccSaveSettings', [p]).then(function(r){
      setTimeout(function(){
        if(r && r.rebuildError){ window.ccSettingsRebuildFailed && window.ccSettingsRebuildFailed(); }
        else { reload(); }
      }, 0);
      return r;
    });
  };

  setInterval(function(){
    fetch('/api/built', {headers: hdr}).then(function(r){ return r.json(); }).then(function(b){
      if(b.built && b.built !== built){ built = b.built; reload(); }
    }).catch(function(){});
  }, 10000);
})();
</script>`

// warmingPollScript turns the warming page into a self-reloading one: it
// reloads as soon as the first dashboard.html exists, or shows the first
// collection's failure the same way the app window does (ccWarmFail).
const warmingPollScript = `<script>
var bmHdr = {}; bmHdr[__HEADER__] = __TOKEN__;
setInterval(function(){
  fetch('/api/built', {headers: bmHdr}).then(function(r){ return r.json(); }).then(function(b){
    if(b.built){ location.reload(); }
    else if(b.error && window.ccWarmFail){ ccWarmFail(b.error); }
  }).catch(function(){});
}, 1000);
</script>`
