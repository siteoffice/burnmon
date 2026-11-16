package winbind

import (
	"bufio"
	"net"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// escLT is how encoding/json writes "<": a backslash, u, 003c.
const escLT = `\` + "u003c"

// AsyncResolveJS text is part of the page contract: pinned byte for byte, with
// a key (two arguments) and without (one).
func TestAsyncResolveJSText(t *testing.T) {
	got, err := AsyncResolveJS("__bmTurnResolve", "k1", map[string]int{"a": 1})
	if want := `window.__bmTurnResolve && window.__bmTurnResolve("k1", {"a":1})`; err != nil || got != want {
		t.Errorf("keyed: got %q, %v; want %q", got, err, want)
	}
	got, err = AsyncResolveJS("__bdevExportResolve", map[string]int{"a": 1})
	if want := `window.__bdevExportResolve && window.__bdevExportResolve({"a":1})`; err != nil || got != want {
		t.Errorf("no key: got %q, %v; want %q", got, err, want)
	}
	got, err = AsyncResolveJS("r", `k"1`, "x<y")
	if want := `window.r && window.r("k\"1", "x` + escLT + `y")`; err != nil || got != want {
		t.Errorf("escaping: got %q, %v; want %q", got, err, want)
	}
	got, err = AsyncResolveJS("r", "k", nil)
	if want := `window.r && window.r("k", null)`; err != nil || got != want {
		t.Errorf("nil data: got %q, %v; want %q", got, err, want)
	}
	if _, err := AsyncResolveJS("r", "k", make(chan int)); err == nil {
		t.Error("an unmarshalable value must return an error")
	}
}

// fakeWindow runs Dispatch at once and keeps what the page would receive.
type fakeWindow struct {
	Recorder
	mu       sync.Mutex
	bound    map[string]any
	scripts  []string
	html     []string
	onEval   func(script string)
	dispatch atomic.Int64
}

func (f *fakeWindow) Bind(name string, fn interface{}) error {
	if err := f.Recorder.Bind(name, fn); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bound == nil {
		f.bound = map[string]any{}
	}
	f.bound[name] = fn
	return nil
}
func (f *fakeWindow) Dispatch(fn func()) { f.dispatch.Add(1); fn() }
func (f *fakeWindow) Eval(js string) {
	f.mu.Lock()
	f.scripts = append(f.scripts, js)
	cb := f.onEval
	f.mu.Unlock()
	if cb != nil {
		cb(js)
	}
}
func (f *fakeWindow) SetHtml(h string) { f.mu.Lock(); f.html = append(f.html, h); f.mu.Unlock() }

func ask(t *testing.T, addr, line string) string {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
	out, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(out, "\r\n")
}

func TestStartUICheckServerOffWithoutItsEnvironmentVariable(t *testing.T) {
	t.Setenv("WINBIND_TEST_UICHECK", "")
	w := &fakeWindow{}
	if ln := StartUICheckServer(w, UICheckOptions{Env: "WINBIND_TEST_UICHECK", Addr: "127.0.0.1:0", Binding: "tUICheckResult"}); ln != nil {
		ln.Close()
		t.Fatal("started without the environment variable")
	}
	if len(w.Names) != 0 {
		t.Errorf("bound %v without the environment variable", w.Names)
	}
}

func TestStartUICheckServerEvalRoundTripAndScriptText(t *testing.T) {
	t.Setenv("WINBIND_TEST_UICHECK", "1")
	w := &fakeWindow{}
	w.onEval = func(string) {
		w.mu.Lock()
		fn := w.bound["tUICheckResult"].(func(string, string))
		w.mu.Unlock()
		go fn("1", `{"ok":true,"value":2}`)
	}
	ln := StartUICheckServer(w, UICheckOptions{Env: "WINBIND_TEST_UICHECK", Addr: "127.0.0.1:0", Binding: "tUICheckResult"})
	if ln == nil {
		t.Fatal("did not start")
	}
	defer ln.Close()
	if got := ask(t, ln.Addr().String(), `{"script":"1+1"}`); got != `{"ok":true,"value":2}` {
		t.Errorf("response = %q", got)
	}
	want := `(function(){try{var r=(function(){return (1+1)})();window.tUICheckResult("1", JSON.stringify({ok:true,value:(r===undefined?null:r)}))}catch(e){window.tUICheckResult("1", JSON.stringify({ok:false,error:String(e)}))}})();`
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.scripts) != 1 || w.scripts[0] != want {
		t.Errorf("scripts = %q, want [%q]", w.scripts, want)
	}
}

func TestStartUICheckServerBadRequest(t *testing.T) {
	t.Setenv("WINBIND_TEST_UICHECK", "1")
	w := &fakeWindow{}
	ln := StartUICheckServer(w, UICheckOptions{Env: "WINBIND_TEST_UICHECK", Addr: "127.0.0.1:0", Binding: "tUICheckResult"})
	if ln == nil {
		t.Fatal("did not start")
	}
	defer ln.Close()
	if got := ask(t, ln.Addr().String(), `not json`); !strings.HasPrefix(got, `{"ok":false,"error":"bad request: `) {
		t.Errorf("response = %q", got)
	}
}

func TestStartUICheckServerReloadOnlyWithAPage(t *testing.T) {
	t.Setenv("WINBIND_TEST_UICHECK", "1")
	w := &fakeWindow{}
	ln := StartUICheckServer(w, UICheckOptions{Env: "WINBIND_TEST_UICHECK", Addr: "127.0.0.1:0", Binding: "tUICheckResult",
		Page: func() string { return "<html>page</html>" }})
	if ln == nil {
		t.Fatal("did not start")
	}
	defer ln.Close()
	if got := ask(t, ln.Addr().String(), `{"reload":true}`); got != `{"ok":true,"value":null}` {
		t.Errorf("response = %q", got)
	}
	w.mu.Lock()
	if len(w.html) != 1 || w.html[0] != "<html>page</html>" || len(w.scripts) != 0 {
		t.Errorf("html = %q, scripts = %q", w.html, w.scripts)
	}
	w.mu.Unlock()

	// Without a page function the reload flag does nothing: the line is an eval.
	w2 := &fakeWindow{}
	ln2 := StartUICheckServer(w2, UICheckOptions{Env: "WINBIND_TEST_UICHECK", Addr: "127.0.0.1:0", Binding: "tUICheckResult"})
	if ln2 == nil {
		t.Fatal("did not start")
	}
	defer ln2.Close()
	w2.onEval = func(string) {
		w2.mu.Lock()
		fn := w2.bound["tUICheckResult"].(func(string, string))
		w2.mu.Unlock()
		go fn("1", `{"ok":false,"error":"SyntaxError"}`)
	}
	ask(t, ln2.Addr().String(), `{"reload":true}`)
	w2.mu.Lock()
	defer w2.mu.Unlock()
	if len(w2.html) != 0 || len(w2.scripts) != 1 || !regexp.MustCompile(`return \(\)`).MatchString(w2.scripts[0]) {
		t.Errorf("html = %q, scripts = %q", w2.html, w2.scripts)
	}
}

func TestStartHeartbeatDispatchesEverySecond(t *testing.T) {
	w := &fakeWindow{}
	StartHeartbeat(w)
	deadline := time.Now().Add(3500 * time.Millisecond)
	for time.Now().Before(deadline) && w.dispatch.Load() < 2 {
		time.Sleep(50 * time.Millisecond)
	}
	if n := w.dispatch.Load(); n < 2 {
		t.Fatalf("%d dispatches in 3.5 s, want at least 2 (one per second)", n)
	}
}
