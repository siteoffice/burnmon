//go:build !windows

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/store"
)

func newTestBridge(t *testing.T) (*browserBridge, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg, err := pricing.Load("")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := st.UpsertEvents([]schema.Event{{
		Vendor: "anthropic", Agent: "claude-code", Surface: "cli",
		SessionID: "s1", RequestID: "r1", At: now.Add(-time.Minute),
		Model: "claude-sonnet-5", Input: 1000, Output: 200,
	}}); err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: cfg, htmlPath: filepath.Join(dir, "dashboard.html")}
	a.cache.Store = st

	b := &browserBridge{a: a, st: st, token: testToken}
	srv := httptest.NewUnstartedServer(nil)
	b.addr = srv.Listener.Addr().String()
	srv.Config.Handler = b.handler()
	srv.Start()
	t.Cleanup(srv.Close)
	return b, srv
}

const testToken = "test-token-0123456789"

func post(t *testing.T, srv *httptest.Server, name, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/"+name, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(tokenHeader, testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// The page's bound functions must answer over the bridge, or the Now and
// History tabs wait forever (the macOS "infinite loading" bug).
func TestBrowserBridgeAnswersBindings(t *testing.T) {
	_, srv := newTestBridge(t)

	code, body := post(t, srv, "bmLive", "[]")
	if code != http.StatusOK {
		t.Fatalf("bmLive: %d %s", code, body)
	}
	var snap struct {
		Sessions []json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(body), &snap); err != nil {
		t.Fatalf("bmLive body not JSON: %v (%s)", err, body)
	}

	for _, c := range []struct{ name, args string }{
		{"bmVendorStrip", "[]"},
		{"bmForecast", "[]"},
		{"bmSessionInsight", `["s1"]`},
		{"bmTurn", `["s1", 1]`},
		{"bmHistory", `["req-1", {"period":"day","from":"2000-01-01","to":"2100-01-01"}]`},
	} {
		if code, body := post(t, srv, c.name, c.args); code != http.StatusOK {
			t.Errorf("%s: %d %s", c.name, code, body)
		}
	}

	if code, _ := post(t, srv, "bmNoSuch", "[]"); code != http.StatusNotFound {
		t.Errorf("unknown binding: got %d, want 404", code)
	}
	if code, _ := post(t, srv, "bmHistory", "not json"); code != http.StatusBadRequest {
		t.Errorf("bad arguments: got %d, want 400", code)
	}
}

func TestBrowserBridgeInjectsScriptIntoHead(t *testing.T) {
	b, srv := newTestBridge(t)

	// No dashboard yet: the self-reloading warming page.
	resp, err := http.Get(srv.URL + "/?t=" + testToken)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(page), "/api/built") {
		t.Errorf("warming page does not poll /api/built:\n%s", page)
	}

	if err := os.WriteFile(b.a.htmlPath, []byte("<html><head><title>x</title></head><body></body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp, err = http.Get(srv.URL + "/?t=" + testToken)
	if err != nil {
		t.Fatal(err)
	}
	page, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	s := string(page)
	if !strings.HasPrefix(s, "<html><head><script>") {
		t.Errorf("bridge script not injected at the top of <head>:\n%.200s", s)
	}
	if !strings.Contains(s, "window.bmLive") || strings.Contains(s, "__BUILT__") {
		t.Errorf("bridge script missing bmLive or the built stamp placeholder was not filled")
	}
}

func TestBrowserBridgeRefusesForeignRequests(t *testing.T) {
	_, srv := newTestBridge(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	req.Host = "evil.example:80"
	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign Host: got %d, want 403", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/bmLive", strings.NewReader("[]"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set(tokenHeader, testToken)
	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign Origin: got %d, want 403", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/bmLive", strings.NewReader("[]"))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set(tokenHeader, testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("non-JSON POST: got %d, want 415", resp.StatusCode)
	}
}

// Localhost is shared by every user and process on the machine, and the page
// itself carries token, project and session data, so every request needs
// the per-run secret (decision 2026-10-21, PR #4 condition 1).
func TestBrowserBridgeRequiresToken(t *testing.T) {
	b, srv := newTestBridge(t)
	if err := os.WriteFile(b.a.htmlPath, []byte("<html><head></head><body>secret data</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	get := func(path string) (int, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	for _, path := range []string{"/", "/?t=wrong", "/api/built", "/api/built?t=wrong"} {
		code, body := get(path)
		if code != http.StatusForbidden {
			t.Errorf("GET %s: got %d, want 403", path, code)
		}
		if strings.Contains(body, "secret data") {
			t.Errorf("GET %s leaked the dashboard", path)
		}
	}
	if code, _ := get("/?t=" + testToken); code != http.StatusOK {
		t.Errorf("GET / with the token: got %d, want 200", code)
	}

	for _, tok := range []string{"", "wrong"} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/bmLive", strings.NewReader("[]"))
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set(tokenHeader, tok)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("POST bmLive with token %q: got %d, want 403", tok, resp.StatusCode)
		}
	}
}

// The injected script must send the token on every call it makes, the
// warming page's poll included.
func TestBrowserBridgeScriptsCarryToken(t *testing.T) {
	b, srv := newTestBridge(t)
	read := func() string {
		resp, err := http.Get(srv.URL + "/?t=" + testToken)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}
	warm := read()
	if !strings.Contains(warm, testToken) || !strings.Contains(warm, tokenHeader) {
		t.Errorf("warming page poll does not send the token")
	}
	if err := os.WriteFile(b.a.htmlPath, []byte("<html><head></head><body></body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	page := read()
	if !strings.Contains(page, testToken) || !strings.Contains(page, tokenHeader) || strings.Contains(page, "__TOKEN__") {
		t.Errorf("bridge script does not send the token")
	}
}

func TestBridgeTokenFile(t *testing.T) {
	dir := t.TempDir()
	tok1, err := newBridgeToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok1) != 64 {
		t.Errorf("token length %d, want 64 hex characters", len(tok1))
	}
	fi, err := os.Stat(filepath.Join(dir, bridgeTokenFile))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("token file mode %v, want 0600", fi.Mode().Perm())
	}
	if got := readBridgeToken(dir); got != tok1 {
		t.Errorf("readBridgeToken = %q, want %q", got, tok1)
	}
	tok2, err := newBridgeToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tok2 == tok1 {
		t.Error("two starts produced the same token")
	}
}

// A second start that finds the port taken opens the running BurnMon only if
// that port really answers as BurnMon with this token.
func TestProbeBridge(t *testing.T) {
	_, srv := newTestBridge(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	if !probeBridge(addr, testToken) {
		t.Error("probe did not recognise a running bridge")
	}
	if probeBridge(addr, "wrong") {
		t.Error("probe accepted a wrong token")
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello from another program")
	}))
	defer other.Close()
	if probeBridge(strings.TrimPrefix(other.URL, "http://"), testToken) {
		t.Error("probe took another program for BurnMon")
	}
}
