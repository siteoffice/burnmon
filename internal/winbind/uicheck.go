package winbind

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// UICheckOptions says how one app runs the dev-only eval server.
type UICheckOptions struct {
	// Env is the environment variable that switches the server on; release
	// builds and the README never set it, so no port opens for a real user.
	Env string
	// Addr is the localhost address to listen on. Each exe has its own (and its
	// own Env and Binding), because both exes can run at once and tools\uicheck
	// must be able to drive either one without them fighting over a listener.
	Addr string
	// Binding is the page-side function the injected script reports through.
	Binding string
	// Page, when set, builds the document the window starts with; a request
	// {"reload":true} loads it again (the window is made with SetHtml, so a
	// plain location.reload would land on about:blank), letting a check prove
	// what survives a real reload. When nil, a reload request is not special.
	Page func() string
}

// StartUICheckServer is dev-only (02_roadmap\2026-09-23_v0.2.3_window_check_patch.md,
// W0): a localhost-only TCP listener that lets tools\uicheck, an external
// process, run JS inside the real running window and get the result back, so a
// session can verify a fix against the actual DOM instead of a proxy check. It
// starts only when o.Env is set, and returns the listener (nil when it did not
// start). Screenshotting and clicking or typing are done by tools\uicheck
// itself against the window's HWND (PrintWindow, SendInput, both pure Win32, no
// cooperation needed); this channel exists only for the one thing an external
// process cannot do on its own: reading and evaluating the page's own DOM and
// JS, which only the process hosting the WebView2 control can do.
func StartUICheckServer(w Window, o UICheckOptions) net.Listener {
	if os.Getenv(o.Env) == "" {
		return nil
	}
	ln, err := net.Listen("tcp", o.Addr)
	if err != nil {
		log.Println("uicheck: could not listen:", err)
		return nil
	}

	srv := &uicheckServer{pending: map[string]chan string{}}

	if err := w.Bind(o.Binding, srv.deliver); err != nil {
		log.Printf("uicheck: could not bind %s: %v", o.Binding, err)
		ln.Close()
		return nil
	}

	log.Printf("uicheck: dev eval server listening on %s", o.Addr)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println("uicheck: accept:", err)
				return
			}
			go srv.handle(conn, w, o)
		}
	}()
	return ln
}

type uicheckServer struct {
	mu      sync.Mutex
	pending map[string]chan string
	nextID  atomic.Uint64
}

// deliver is the page-side binding, called from the page once the script this
// server injected has finished (or thrown).
func (s *uicheckServer) deliver(id, resultJSON string) {
	s.mu.Lock()
	ch := s.pending[id]
	delete(s.pending, id)
	s.mu.Unlock()
	if ch != nil {
		ch <- resultJSON
	}
}

type uicheckRequest struct {
	// Script is a JS expression (not a statement list): its value becomes the
	// eval's own result, JSON-encoded back to the caller. Reach into the page's
	// existing globals and DOM directly, e.g.
	// "document.querySelector('#now').outerHTML".
	Script string `json:"script"`
	Reload bool   `json:"reload,omitempty"`
}

// handle reads exactly one JSON request line, evaluates its Script inside the
// page (via w.Dispatch and w.Eval, the same UI-thread-safe path the async
// bindings use), and writes exactly one JSON response line back:
// {"ok":true,"value":...} or {"ok":false,"error":"..."}.
func (s *uicheckServer) handle(conn net.Conn, w Window, o UICheckOptions) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !scanner.Scan() {
		return
	}
	var req uicheckRequest
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		fmt.Fprintf(conn, "{\"ok\":false,\"error\":%q}\n", "bad request: "+err.Error())
		return
	}

	if req.Reload && o.Page != nil {
		w.Dispatch(func() { w.SetHtml(o.Page()) })
		fmt.Fprintln(conn, `{"ok":true,"value":null}`)
		return
	}

	id := fmt.Sprintf("%d", s.nextID.Add(1))
	ch := make(chan string, 1)
	s.mu.Lock()
	s.pending[id] = ch
	s.mu.Unlock()

	idJSON, err := json.Marshal(id)
	if err != nil {
		fmt.Fprintf(conn, "{\"ok\":false,\"error\":%q}\n", err.Error())
		return
	}
	// r===undefined is common for a script that ran a side effect (a click
	// helper, say) rather than returning a value; JSON.stringify would
	// otherwise produce the bare word undefined, which is not valid JSON.
	script := fmt.Sprintf(
		"(function(){try{var r=(function(){return (%[1]s)})();window.%[2]s(%[3]s, JSON.stringify({ok:true,value:(r===undefined?null:r)}))}catch(e){window.%[2]s(%[3]s, JSON.stringify({ok:false,error:String(e)}))}})();",
		req.Script, o.Binding, idJSON)
	w.Dispatch(func() { w.Eval(script) })

	select {
	case result := <-ch:
		fmt.Fprintln(conn, result)
	case <-time.After(10 * time.Second):
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		fmt.Fprintln(conn, `{"ok":false,"error":"timeout"}`)
	}
}
