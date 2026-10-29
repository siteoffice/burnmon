//go:build windows

package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// The map binding builds off the UI thread: go-webview2 runs a binding inside
// its message callback on the UI thread, so a synchronous bdevAgentMap froze
// the window (and every eval queued behind it) for the whole history build.

func TestAgentMapResolveJS(t *testing.T) {
	js, err := agentMapResolveJS("7", agentMapPayload{Source: "history", Hours: 168, Status: "ok"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`window.__bdevAgentMapResolve("7", {`, `"source":"history"`, `"hours":168`, `"status":"ok"`} {
		if !strings.Contains(js, want) {
			t.Errorf("resolve script lacks %q: %s", want, js)
		}
	}
	if strings.Contains(js, `"error"`) {
		t.Errorf("a good answer carries an error field: %s", js)
	}
	js, err = agentMapResolveJS("8", agentMapPayload{Source: "history"}, errors.New("the store is not open"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, `"error":"the store is not open"`) {
		t.Errorf("a failed build does not carry its error: %s", js)
	}
}

func TestAgentMapBindingIsAsync(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := strings.ReplaceAll(string(b), "\r\n", "\n")
	if strings.Contains(src, `w.Bind("bdevAgentMap",`) {
		t.Error("main.go still binds bdevAgentMap, which builds the map on the UI thread")
	}
	i := strings.Index(src, `w.Bind("bdevAgentMapStart", func(key string, req agentMapRequest) {`)
	if i < 0 {
		t.Fatal("main.go has no bdevAgentMapStart binding")
	}
	body := src[i:]
	if e := strings.Index(body, "could not bind bdevAgentMapStart"); e > 0 {
		body = body[:e]
	}
	for _, want := range []string{"go func() {", "agentMapFor(", "agentMapResolveJS(key,", "w.Dispatch("} {
		if !strings.Contains(body, want) {
			t.Errorf("bdevAgentMapStart lacks %q", want)
		}
	}
}

func TestPageWrapsAgentMapStart(t *testing.T) {
	page := idlePage()
	for _, want := range []string{
		"window.__bdevAgentMapResolve = function(key, r){",
		"if(typeof window.bdevAgentMapStart === 'function'){",
		"window.bdevAgentMap = function(req){",
		"window.bdevAgentMapStart(key, req)",
		"if(r && r.error) p.reject(new Error(r.error)); else p.resolve(r);",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
	// mapFetch still calls window.bdevAgentMap, so d38 and d40 can stub it.
	if !strings.Contains(idleFn(t, "mapFetch"), "window.bdevAgentMap({") {
		t.Error("mapFetch no longer calls window.bdevAgentMap")
	}
}
