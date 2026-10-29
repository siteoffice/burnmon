//go:build windows

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

	webview2 "github.com/jchv/go-webview2"
)

// startUICheckServer mirrors cmd\burnmon\uicheck_devserver.go exactly, one
// exe and one port over: BURNMON_DEV_UICHECK (not BURNMON_UICHECK) gates
// it, and it listens on 127.0.0.1:9334 (not 9333), binding
// bdevUICheckResult (not bmUICheckResult). A distinct env var and port,
// rather than reusing burnmon.exe's own, because the design doc's own
// ingest decision has both exes running at once as the normal case, so
// tools\uicheck must be able to drive either one without them fighting
// over the same listener. Dev-only: release builds and the README never
// set BURNMON_DEV_UICHECK, so this never opens a port for a real user.
//
// page builds the document the window starts with; a request {"reload":true}
// loads it again (the window is made with SetHtml, a plain location.reload
// would land on about:blank), so a check can prove what survives a real reload.
func startUICheckServer(w webview2.WebView, page func() string) {
	if os.Getenv("BURNMON_DEV_UICHECK") == "" {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:9334")
	if err != nil {
		log.Println("uicheck: could not listen:", err)
		return
	}

	srv := &uicheckServer{pending: map[string]chan string{}}

	if err := w.Bind("bdevUICheckResult", srv.deliver); err != nil {
		log.Println("uicheck: could not bind bdevUICheckResult:", err)
		return
	}

	log.Println("uicheck: dev eval server listening on 127.0.0.1:9334")
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println("uicheck: accept:", err)
				return
			}
			go srv.handle(conn, w, page)
		}
	}()
}

type uicheckServer struct {
	mu      sync.Mutex
	pending map[string]chan string
	nextID  atomic.Uint64
}

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
	Script string `json:"script"`
	Reload bool   `json:"reload,omitempty"`
}

func (s *uicheckServer) handle(conn net.Conn, w webview2.WebView, page func() string) {
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

	if req.Reload {
		w.Dispatch(func() { w.SetHtml(page()) })
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
	script := fmt.Sprintf(
		"(function(){try{var r=(function(){return (%s)})();window.bdevUICheckResult(%s, JSON.stringify({ok:true,value:(r===undefined?null:r)}))}catch(e){window.bdevUICheckResult(%s, JSON.stringify({ok:false,error:String(e)}))}})();",
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
