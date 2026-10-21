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

	b := &browserBridge{a: a, st: st}
	srv := httptest.NewUnstartedServer(nil)
	b.addr = srv.Listener.Addr().String()
	srv.Config.Handler = b.handler()
	srv.Start()
	t.Cleanup(srv.Close)
	return b, srv
}

func post(t *testing.T, srv *httptest.Server, name, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/"+name, "application/json", strings.NewReader(body))
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
	resp, err := http.Get(srv.URL + "/")
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
	resp, err = http.Get(srv.URL + "/")
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
	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign Origin: got %d, want 403", resp.StatusCode)
	}

	resp, err := http.Post(srv.URL+"/api/bmLive", "text/plain", strings.NewReader("[]"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("non-JSON POST: got %d, want 415", resp.StatusCode)
	}
}
