//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// d32: BurnMon Dev alone keeps the shared snapshot fresh. Precondition: no
// burnmon.exe process runs, so the only writer can be the burnmon-dev.exe that
// scripts\uicheck.ps1 started. It reads the real %USERPROFILE%\.burnmon
// snapshot.lock and snapshot.json (read only, never written or removed) and
// proves:
//   - snapshot.lock names "burnmon-dev.exe" as its owner within 10 s;
//   - snapshot.json was written less than 10 s ago, and 6 s later written_at
//     has moved forward;
//   - the real bdevSections binding answers reading.status "ok", and the page
//     shows no "not running" text.
//
// Screenshot: d32-dev-only.

func d32Fail(format string, args ...any) error { return fmt.Errorf("d32: "+format, args...) }

// d32ProcessRunning reports whether a process with this exe name runs.
func d32ProcessRunning(exe string) (bool, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return false, err
	}
	for {
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), exe) {
			return true, nil
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			return false, nil
		}
	}
}

type d32Owner struct {
	PID int    `json:"pid"`
	Exe string `json:"exe"`
}

// d32WrittenAt reads written_at from snapshot.json.
func d32WrittenAt(path string) (time.Time, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}
	var s struct {
		WrittenAt string `json:"written_at"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, s.WrittenAt)
}

const d32PageJS = `(function(){
  var t = function(id){ var e = document.getElementById(id); return e ? e.textContent : ''; };
  return { header: t('hdrSnapAge'), notice: t('secNotice'), text: t('secPanels') };
})()`

func init() {
	checks["d32"] = func(hwnd uintptr, args []string) error {
		running, err := d32ProcessRunning("burnmon.exe")
		if err != nil {
			return d32Fail("cannot list processes: %w", err)
		}
		if running {
			return d32Fail("close burnmon.exe first: d32 proves BurnMon Dev alone writes the snapshot")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir := filepath.Join(home, ".burnmon")
		lockPath, snapPath := filepath.Join(dir, "snapshot.lock"), filepath.Join(dir, "snapshot.json")

		// The lock text names burnmon-dev.exe within two ticks.
		var owner d32Owner
		deadline := time.Now().Add(10 * time.Second)
		for {
			owner = d32Owner{}
			if b, err := os.ReadFile(lockPath); err == nil {
				_ = json.Unmarshal(b, &owner)
			}
			if owner.Exe == "burnmon-dev.exe" {
				break
			}
			if time.Now().After(deadline) {
				return d32Fail("snapshot.lock does not name burnmon-dev.exe after 10 s (owner %+v)", owner)
			}
			time.Sleep(500 * time.Millisecond)
		}

		// The file is fresh, and moves forward.
		first, err := d32WrittenAt(snapPath)
		if err != nil {
			return d32Fail("read snapshot.json: %w", err)
		}
		if age := time.Since(first); age >= 10*time.Second || age < -2*time.Second {
			return d32Fail("snapshot.json written_at %s is %s old, want under 10 s", first.Format(time.RFC3339), age.Round(time.Second))
		}
		time.Sleep(6 * time.Second)
		second, err := d32WrittenAt(snapPath)
		if err != nil {
			return d32Fail("read snapshot.json again: %w", err)
		}
		if !second.After(first) {
			return d32Fail("written_at did not move forward in 6 s (%s then %s)", first.Format(time.RFC3339), second.Format(time.RFC3339))
		}

		// The real binding answers ok, and the page does not say "not running".
		if _, err := evalRaw(`(window.__d32 = null, window.bdevSections().then(function(p){ window.__d32 = JSON.stringify(p); }), 1)`); err != nil {
			return err
		}
		var raw string
		for i := 0; i < 20 && raw == ""; i++ {
			time.Sleep(150 * time.Millisecond)
			if raw, err = evalString(`window.__d32 || ''`); err != nil {
				return err
			}
		}
		if raw == "" {
			return d32Fail("bdevSections never answered")
		}
		var st struct {
			Reading struct {
				Status string `json:"status"`
			} `json:"reading"`
		}
		if err := json.Unmarshal([]byte(raw), &st); err != nil {
			return err
		}
		if st.Reading.Status != "ok" {
			return d32Fail("bdevSections reading.status is %q, want ok", st.Reading.Status)
		}
		time.Sleep(1500 * time.Millisecond)
		var pg struct {
			Header string `json:"header"`
			Notice string `json:"notice"`
			Text   string `json:"text"`
		}
		if err := evalInto(d32PageJS, &pg); err != nil {
			return err
		}
		for name, s := range map[string]string{"header": pg.Header, "notice": pg.Notice, "panels": pg.Text} {
			if strings.Contains(s, "not running") {
				return d32Fail("the page %s says %q", name, s)
			}
		}
		if _, err := screenshot(hwnd, "d32-dev-only"); err != nil {
			return err
		}
		return nil
	}
}
