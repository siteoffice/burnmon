//go:build windows || (darwin && cgo)

// lamp_child.go keeps the lamp running as a child of BurnMon Dev while
// lamp_enabled is true: it starts the own exe with the one argument lamp,
// stops it on exit and when the switch goes off, and starts it again when it
// dies, at most 3 times in 10 minutes. The start goes through os/exec on the
// own exe path only, never through WMI, CIM, PowerShell or cmd.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"burnmon/internal/lamp"
)

const (
	lampRestartMax    = 3
	lampRestartWindow = 10 * time.Minute
)

// lampProc is one running lamp process.
type lampProc interface {
	Stop()
	Done() <-chan struct{}
}

// lampStarter starts the exe with args. The tests swap it for a fake.
type lampStarter func(exe string, args []string) (lampProc, error)

type lampChild struct {
	exe       string
	start     lampStarter
	now       func() time.Time
	logf      func(string, ...any)
	supported bool // the lamp runs on Windows only in this version

	mu       sync.Mutex
	want     bool
	gaveUp   bool // the restart limit was hit; only off and on again clears it
	proc     lampProc
	restarts []time.Time
}

func newLampChild(exe string, start lampStarter, now func() time.Time, logf func(string, ...any)) *lampChild {
	return &lampChild{exe: exe, start: start, now: now, logf: logf, supported: runtime.GOOS == "windows"}
}

// Sync makes the child match the switch: on starts one (once), off stops it.
func (c *lampChild) Sync(on bool) {
	if c == nil || !c.supported {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.want = on
	if !on {
		c.gaveUp = false
		c.restarts = nil
		c.stopLocked()
		return
	}
	if c.proc != nil || c.gaveUp {
		return
	}
	c.startLocked()
}

// Stop ends the child for good, at BurnMon Dev exit.
func (c *lampChild) Stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.want = false
	c.stopLocked()
}

func (c *lampChild) stopLocked() {
	if c.proc != nil {
		p := c.proc
		c.proc = nil
		p.Stop()
	}
}

func (c *lampChild) startLocked() {
	p, err := c.start(c.exe, []string{"lamp"})
	if err != nil {
		c.logf("lamp child: start: %v", err)
		c.gaveUp = true
		return
	}
	c.proc = p
	go func() {
		<-p.Done()
		c.died(p)
	}()
}

// died runs when p ended. A child stopped on purpose is no longer c.proc and
// is ignored. Otherwise it starts again unless 3 restarts already ran in the
// last 10 minutes.
func (c *lampChild) died(p lampProc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.proc != p {
		return
	}
	c.proc = nil
	if !c.want {
		return
	}
	now := c.now()
	kept := c.restarts[:0]
	for _, t := range c.restarts {
		if now.Sub(t) < lampRestartWindow {
			kept = append(kept, t)
		}
	}
	c.restarts = kept
	if len(c.restarts) >= lampRestartMax {
		c.gaveUp = true
		c.logf("lamp child: died %d times in %v, not starting it again", lampRestartMax+1, lampRestartWindow)
		return
	}
	c.restarts = append(c.restarts, now)
	c.startLocked()
}

// execLampProc is a real child process.
type execLampProc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *execLampProc) Stop()                 { _ = p.cmd.Process.Kill() }
func (p *execLampProc) Done() <-chan struct{} { return p.done }

func execLampStart(exe string, args []string) (lampProc, error) {
	cmd := exec.Command(exe, args...)
	// The lamp ends when this process is gone (lampParentGone).
	cmd.Env = lampChildEnv(os.Environ(), os.Getpid())
	// Started hidden, the window first shows through the lamp's own placement,
	// which does not take the focus (inferred from the ShowWindow docs).
	cmd.SysProcAttr = lampChildAttr()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &execLampProc{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// newLampChildForApp is the child BurnMon Dev uses: its own exe, os/exec.
func newLampChildForApp() *lampChild {
	exe, err := os.Executable()
	if err != nil {
		log.Println("lamp child: own exe path:", err)
	}
	return newLampChild(exe, execLampStart, time.Now, log.Printf)
}

// readLampSettings reads the lamp keys from the settings folder's burnmon-dev.json.
func readLampSettings(setDir string) lamp.Settings {
	b, err := os.ReadFile(devConfigReadPath(setDir))
	if err != nil {
		return lamp.DefaultSettings()
	}
	return lamp.ParseSettings(b)
}

// syncLampFromFile starts or stops the child from the saved lamp_enabled.
func syncLampFromFile(setDir string, c *lampChild) {
	c.Sync(readLampSettings(setDir).Enabled)
}

// setLampKey is bdevSetLamp: one of the eleven gear keys with a value of the
// right kind is saved into burnmon-dev.json, then the child follows the file.
// Anything else is refused before a byte is written.
func setLampKey(setDir, key string, value any, c *lampChild) error {
	switch key {
	case "lamp_enabled", "lamp_lines", "lamp_foreman_items":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s takes true or false", key)
		}
	case "lamp_position":
		if s, _ := value.(string); s != "top-center" && s != "top-left" && s != "top-right" {
			return fmt.Errorf("lamp_position takes top-center, top-left or top-right")
		}
	case "lamp_language":
		if s, _ := value.(string); s != "en" && s != "nl" {
			return fmt.Errorf("lamp_language takes en or nl")
		}
	case "lamp_ticker_rows":
		if f, ok := value.(float64); !ok || f < 1 || f > 4 || f != float64(int(f)) {
			return fmt.Errorf("lamp_ticker_rows takes 1, 2, 3 or 4")
		}
	case "lamp_size":
		if s, _ := value.(string); s != "small" && s != "medium" && s != "large" {
			return fmt.Errorf("lamp_size takes small, medium or large")
		}
	case "lamp_theme":
		if s, _ := value.(string); s != "system" && s != "light" && s != "dark" {
			return fmt.Errorf("lamp_theme takes system, light or dark")
		}
	case "lamp_motion":
		if s, _ := value.(string); s != "lively" && s != "calm" && s != "off" {
			return fmt.Errorf("lamp_motion takes lively, calm or off")
		}
	case "lamp_quiet":
		if s, _ := value.(string); s != "hide" && s != "stay" {
			return fmt.Errorf("lamp_quiet takes hide or stay")
		}
	case "lamp_backdrop":
		if s, _ := value.(string); s != "clear" && s != "card" {
			return fmt.Errorf("lamp_backdrop takes clear or card")
		}
	default:
		return fmt.Errorf("%q is not a lamp setting of the gear", key)
	}
	if err := saveDevConfigKey(setDir, key, value); err != nil {
		return err
	}
	syncLampFromFile(setDir, c)
	return nil
}

// lampGear is the lamp object of bdevSections (lampGearPayload).
func lampGear(setDir string) lampGearPayload {
	s := readLampSettings(setDir)
	return lampGearPayload{s.Enabled, s.Position, s.Language, s.Lines, s.ForemanItems,
		s.TickerRows, s.Size, s.Theme, s.Motion, s.Quiet, s.Backdrop}
}
