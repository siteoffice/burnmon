package main

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

	"burnmon/internal/winbind"
)

// startUICheckServer is dev-only (02_roadmap\2026-09-23_v0.2.3_window_check_patch.md,
// W0): a localhost-only TCP listener that lets tools\uicheck, an external
// process, run JS inside the real running window and get the result back,
// so a session can verify a fix against the actual DOM instead of a proxy
// check. Only starts when BURNMON_UICHECK is set; release builds and the
// README never set it, so the "no open port" promise holds for users.
// Screenshotting and clicking/typing are done by tools\uicheck itself
// against the window's HWND (PrintWindow, SendInput, both pure Win32, no
// cooperation needed); this channel exists only for the one thing an
// external process cannot do on its own: reading and evaluating the page's
// own DOM/JS, which only the process hosting the WebView2 control can do.
func startUICheckServer(w winbind.Window) {
	if os.Getenv("BURNMON_UICHECK") == "" {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:9333")
	if err != nil {
		log.Println("uicheck: could not listen:", err)
		return
	}

	srv := &uicheckServer{pending: map[string]chan string{}}

	if err := w.Bind("bmUICheckResult", srv.deliver); err != nil {
		log.Println("uicheck: could not bind bmUICheckResult:", err)
		return
	}

	log.Println("uicheck: dev eval server listening on 127.0.0.1:9333")
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println("uicheck: accept:", err)
				return
			}
			go srv.handle(conn, w)
		}
	}()
}

type uicheckServer struct {
	mu      sync.Mutex
	pending map[string]chan string
	nextID  atomic.Uint64
}

// deliver is bmUICheckResult, called from the page once the script this
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
	// Script is a JS expression (not a statement list): its value becomes
	// the eval's own result, JSON-encoded back to the caller. Reach into
	// the page's existing globals/DOM directly, e.g.
	// "document.querySelector('#now').outerHTML".
	Script string `json:"script"`
}

// handle reads exactly one JSON request line, evaluates its Script inside
// the page (via w.Dispatch/w.Eval, the same UI-thread-safe path bmTurn etc.
// use), and writes exactly one JSON response line back:
// {"ok":true,"value":...} or {"ok":false,"error":"..."}.
func (s *uicheckServer) handle(conn net.Conn, w winbind.Window) {
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
	// r===undefined is common for a script that ran a side effect (a
	// click helper, say) rather than returning a value; JSON.stringify
	// would otherwise produce the bare word undefined, which is not
	// valid JSON.
	script := fmt.Sprintf(
		"(function(){try{var r=(function(){return (%s)})();window.bmUICheckResult(%s, JSON.stringify({ok:true,value:(r===undefined?null:r)}))}catch(e){window.bmUICheckResult(%s, JSON.stringify({ok:false,error:String(e)}))}})();",
		req.Script, idJSON, idJSON)
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
