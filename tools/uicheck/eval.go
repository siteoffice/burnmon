package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const evalAddr = "127.0.0.1:9333"

// evalAddrDev is burnmon-dev.exe's own dev eval server
// (cmd\burnmon-dev\uicheck_devserver.go, BURNMON_DEV_UICHECK=1), a distinct
// port from burnmon.exe's own 9333 so both can run (and be driven) at
// once, the design doc's own normal-case scenario.
const evalAddrDev = "127.0.0.1:9334"

// currentEvalAddr is which of the two servers evalRaw talks to; main.go
// sets it once at startup based on the check name's own "d" prefix before
// any check runs.
var currentEvalAddr = evalAddr

type evalResponse struct {
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

// evalRaw sends script (a JS expression, not a statement list) to burnmon's
// dev eval server (cmd\burnmon\uicheck_devserver.go, BURNMON_UICHECK=1) and
// returns its JSON-encoded result value.
func evalRaw(script string) (json.RawMessage, error) {
	conn, err := net.DialTimeout("tcp", currentEvalAddr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial %s (is the right exe running with its own UICHECK env var set?): %w", currentEvalAddr, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	req, err := json.Marshal(map[string]string{"script": script})
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return nil, err
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return nil, err
	}
	var resp evalResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("bad response %q: %w", line, err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("script error: %s", resp.Error)
	}
	return resp.Value, nil
}

func evalString(script string) (string, error) {
	raw, err := evalRaw(script)
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("expected string, got %s: %w", raw, err)
	}
	return s, nil
}

func evalBool(script string) (bool, error) {
	raw, err := evalRaw(script)
	if err != nil {
		return false, err
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, fmt.Errorf("expected bool, got %s: %w", raw, err)
	}
	return b, nil
}

func evalInto(script string, v any) error {
	raw, err := evalRaw(script)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

type elementRect struct {
	X, Y, DPR float64
	Found     bool
}

// clickSelector scrolls selector's first match into view, reads its
// on-screen center, and sends a real OS-level click there via win32.go's
// clickAt (SetCursorPos + SendInput), not a JS .click() call.
func clickSelector(hwnd uintptr, selector string) error {
	return clickSelectorAt(hwnd, selector, 0.5, 0.5)
}

// clickSelectorAt is clickSelector but clicks at a fractional point within
// selector's rect (fx,fy in [0,1], 0.5,0.5 is the center) instead of always
// the center: needed for #turn_drawer_overlay, whose center coincides with
// the drawer card centered inside it, so a plain center click would land on
// the card, not the backdrop the overlay-click-to-close behavior is
// testing.
func clickSelectorAt(hwnd uintptr, selector string, fx, fy float64) error {
	script := fmt.Sprintf(`(function(){
  var el = document.querySelector(%s);
  if (!el) return {found:false};
  el.scrollIntoView({block:'center', inline:'center'});
  var r = el.getBoundingClientRect();
  return {found:true, x: r.x + r.width*%v, y: r.y + r.height*%v};
})()`, jsString(selector), fx, fy)
	var rectResult struct {
		Found bool    `json:"found"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
	}
	if err := evalInto(script, &rectResult); err != nil {
		return err
	}
	if !rectResult.Found {
		return fmt.Errorf("no element matches %q", selector)
	}
	// A mouse click at absolute screen coordinates lands on whatever window
	// is physically at that point regardless of foreground/focus state, so
	// no SetForegroundWindow dance is needed here (unlike pressKey, which
	// needs real keyboard focus).
	x, y, err := screenCoords(hwnd, rectResult.X, rectResult.Y)
	if err != nil {
		return err
	}
	return clickAt(x, y)
}

// dragSelector finds selector's centre like clickSelectorAt, then presses there
// and drags the real cursor dy CSS pixels down (negative is up).
func dragSelector(hwnd uintptr, selector string, dy float64) error {
	script := fmt.Sprintf(`(function(){
  var el = document.querySelector(%s);
  if (!el) return {found:false};
  el.scrollIntoView({block:'center', inline:'center'});
  var r = el.getBoundingClientRect();
  return {found:true, x: r.x + r.width*0.5, y: r.y + r.height*0.5};
})()`, jsString(selector))
	var rr struct {
		Found bool    `json:"found"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
	}
	if err := evalInto(script, &rr); err != nil {
		return err
	}
	if !rr.Found {
		return fmt.Errorf("no element matches %q", selector)
	}
	x1, y1, err := screenCoords(hwnd, rr.X, rr.Y)
	if err != nil {
		return err
	}
	x2, y2, err := screenCoords(hwnd, rr.X, rr.Y+dy)
	if err != nil {
		return err
	}
	return dragAt(x1, y1, x2, y2)
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
