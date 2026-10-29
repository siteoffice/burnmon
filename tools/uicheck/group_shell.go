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

// shell/d0: moved from check_d0.go.

// d0: the UI review patch's own header (section 1) is present and quiet -
// a plain "PRESSURE nn" text, a headline total, no boxed chips (the old
// #cmd search input and .chip elements are gone) - and the deleted "Today's
// Read" panel (section 8) really is gone from the DOM, not just hidden.
// Replaces this check's own earlier version, which asserted the advisor
// panel (#advisorFindings) populated; that panel and its bdevAdvisorNow
// binding were deleted by this same patch's section 8.
func caseD0(hwnd uintptr, args []string) error {
	time.Sleep(500 * time.Millisecond)

	if exists, err := evalBool(`!!document.getElementById('advisorFindings')`); err != nil {
		return fmt.Errorf("check advisorFindings absent: %w", err)
	} else if exists {
		return fmt.Errorf("#advisorFindings still exists; section 8 was supposed to delete the Today's Read panel")
	}
	if exists, err := evalBool(`!!document.getElementById('cmd')`); err != nil {
		return fmt.Errorf("check #cmd absent: %w", err)
	} else if exists {
		return fmt.Errorf("#cmd (the search input) still exists; section 1 was supposed to remove it")
	}
	if n, err := evalRaw(`document.querySelectorAll('.chip').length`); err != nil {
		return fmt.Errorf("check .chip count: %w", err)
	} else if string(n) != "0" {
		return fmt.Errorf("found %s .chip element(s); section 1 was supposed to remove every boxed chip", n)
	}

	pressureText, err := evalString(`document.getElementById('pressureChip').textContent`)
	if err != nil {
		return fmt.Errorf("read pressure text: %w", err)
	}
	if !strings.HasPrefix(pressureText, "PRESSURE") {
		return fmt.Errorf("pressure text %q does not start with \"PRESSURE\"", pressureText)
	}

	// Give the first poll cycle a moment to land, then require the
	// headline total to hold a real (non-placeholder) value.
	var headline string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		headline, err = evalString(`document.getElementById('hlToday').textContent`)
		if err != nil {
			return fmt.Errorf("read headline total: %w", err)
		}
		if headline != "" && headline != "-" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if headline == "" || headline == "-" {
		return fmt.Errorf("headline total never rendered a real value")
	}
	fmt.Println("uicheck: header OK, pressure =", pressureText, "headline =", headline)

	exportText, err := evalString(`document.getElementById('exportBtn').textContent`)
	if err != nil {
		return fmt.Errorf("read export button text: %w", err)
	}
	if exportText != "EXPORT" {
		return fmt.Errorf("export button text = %q, want \"EXPORT\"", exportText)
	}

	if _, err := screenshot(hwnd, "d0-header"); err != nil {
		return err
	}
	return nil
}

// shell/d1: moved from check_d1.go.

// d1: the header EXPORT button actually writes an export bundle. Clicks
// #exportBtn through the dev eval channel (its own addEventListener path,
// a genuine functional check, same reasoning check_w2.go's own doc comment
// gives for using el.click() over hardware SendInput here), waits for
// #exportStatus to report success, then confirms all three files exist on
// disk with a positive size (the binding's own dataDir\exports\ folder,
// design doc's bdevExport).
func caseD1(hwnd uintptr, args []string) error {

	if _, err := evalRaw(`document.getElementById('exportBtn').click()`); err != nil {
		return fmt.Errorf("click export button: %w", err)
	}

	var status string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		s, err := evalString(`document.getElementById('exportStatus').textContent`)
		if err != nil {
			return fmt.Errorf("read export status: %w", err)
		}
		status = s
		if strings.Contains(status, "exported") || strings.Contains(status, "failed") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("uicheck: export status:", status)
	if !strings.Contains(status, "exported") {
		return fmt.Errorf("export did not report success within 30s, last status: %q", status)
	}

	sd, err := devSettingsDir()
	if err != nil {
		return err
	}
	exportsDir := filepath.Join(sd, "exports")
	entries, err := os.ReadDir(exportsDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", exportsDir, err)
	}
	var newest os.DirEntry
	var newestMod time.Time
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "burnmon-dev_") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestMod) {
			newestMod = info.ModTime()
			newest = e
		}
	}
	if newest == nil {
		return fmt.Errorf("no burnmon-dev_* export folder found under %s", exportsDir)
	}
	dir := filepath.Join(exportsDir, newest.Name())
	for _, f := range []string{"summary.md", "data.json", "daily.csv"} {
		fi, err := os.Stat(filepath.Join(dir, f))
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		if fi.Size() <= 0 {
			return fmt.Errorf("%s is empty", f)
		}
		fmt.Printf("uicheck: %s: %d bytes\n", f, fi.Size())
	}

	if _, err := screenshot(hwnd, "d1-export-done"); err != nil {
		return err
	}
	return nil
}

// shell/d24: moved from check_d24.go.

// d24: light and dark (v0.4.0-alpha.10, public): L toggles the whole page
// between its dark theme (the default) and a light one. Fake mode paints
// five sessions in three vendors, a full chart, system history and process
// groups, then real key presses: L sets data-theme="light" on <html>, the
// page and header backgrounds turn light and the text dark, and every
// session, legend and process-group colour that script draws holds 3:1
// (WCAG, graphics) against the panel it sits on; the second L restores the
// dark theme exactly (body background and session colours equal the first
// read). Screenshots of both. The choice is remembered by the Go side
// (bdevSetUITheme, burnmon-dev-view.json), not by the page, so this only
// checks that the binding exists; the file itself is covered by the Go tests.
const vkL = 0x4C

// d24State is one read of the page's theme. Lum values are WCAG relative
// luminance (0 black, 1 white); MinContrast is the lowest contrast ratio of
// any drawn series colour against the panel background.
type d24State struct {
	Attr        string   `json:"attr"`
	BodyBg      string   `json:"bodyBg"`
	BodyLum     float64  `json:"bodyLum"`
	HeaderLum   float64  `json:"headerLum"`
	TxtLum      float64  `json:"txtLum"`
	PanelLum    float64  `json:"panelLum"`
	Colours     []string `json:"colours"`
	MinContrast float64  `json:"minContrast"`
	Binding     bool     `json:"binding"`
}

const d24ReadJS = `(function(){
  function rgb(s){ var m = /rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(s || ''); return m ? [+m[1], +m[2], +m[3]] : null; }
  function lum(c){
    if(!c) return -1;
    var l = c.map(function(v){ v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); });
    return 0.2126 * l[0] + 0.7152 * l[1] + 0.0722 * l[2];
  }
  var cs = function(el, p){ return getComputedStyle(el)[p]; };
  var bodyBg = cs(document.body, 'backgroundColor');
  var panelLum = lum(rgb(cs(document.querySelector('.panel'), 'backgroundColor')));
  var seen = {}, colours = [], minC = 99;
  function take(el, prop){
    var v = cs(el, prop), c = rgb(v);
    if(!c || seen[v]) return;
    seen[v] = true; colours.push(v);
    var l = lum(c), hi = Math.max(l, panelLum) + 0.05, lo = Math.min(l, panelLum) + 0.05;
    minC = Math.min(minC, hi / lo);
  }
  document.querySelectorAll('#burnBars .bar i, #burnLegend .dot, #sessionCards h4, #histLegend .dot, #vendorStrip .dot, #processGroups .dot, #turnTicker .trow .h').forEach(function(el){
    take(el, el.tagName === 'H4' || el.classList.contains('h') ? 'color' : 'backgroundColor');
  });
  return {
    attr: document.documentElement.getAttribute('data-theme') || '', bodyBg: bodyBg, bodyLum: lum(rgb(bodyBg)),
    headerLum: lum(rgb(cs(document.querySelector('header'), 'backgroundColor'))), txtLum: lum(rgb(cs(document.body, 'color'))),
    panelLum: panelLum, colours: colours, minContrast: colours.length ? minC : -1, binding: typeof window.bdevSetUITheme === 'function'
  };
})()`

const d24FakeJS = `(function(){
  var nowMs = Date.now(), ws = nowMs - 30 * 60000;
  function iso(ms){ return new Date(ms).toISOString(); }
  function s(id, agent, project, n){
    return {session_id: id, vendor: agent, agent: agent, surface: agent, model: '', project: project, start: iso(nowMs - 900000),
      last_turn: iso(nowMs - 5000), turn_count: n, context: 40000 * n, context_window: 200000, tokens: 9000 * n, cost: 0.1 * n, cache_hit_ratio: 0.5};
  }
  var sessions = [s('fake-lt-1', 'claude-code', 'proj-alpha', 4), s('fake-lt-2', 'claude-code', 'proj-beta', 3),
                  s('fake-lt-3', 'cowork', 'host-cwd', 2), s('fake-lt-4', 'codex', 'proj-gamma', 3), s('fake-lt-5', 'copilot-cli', 'proj-delta', 1)];
  var chart = [], i, ids = sessions.map(function(x){ return x.session_id; });
  for(i = 0; i < 30; i++){
    var by = {};
    ids.forEach(function(id, k){ if((i + k) % 3 !== 0) by[id] = {fresh: 800 + 300 * ((i * (k + 2)) % 7), cache_write: 0, cache_read: 4000, output: 300}; });
    chart.push({at: iso(ws + i * 60000), by_session: by, cost: 0});
  }
  var hist = [];
  for(i = 1500; i >= 0; i -= 5){
    var t = nowMs - i * 1000;
    hist.push({Ts: iso(t), CPUPct: 45 + 30 * Math.sin(i / 90), MemUsedMB: 14000 + 1500 * Math.sin(i / 300), MemTotalMB: 32000,
      DiskReadBps: 2e6 * (1 + Math.sin(i / 60)), DiskWriteBps: 1e6, NetDownBps: 5e5 * (1 + Math.cos(i / 80)), NetUpBps: 1e5, GPUPct: 20 + 15 * Math.cos(i / 120)});
  }
  var turns = sessions.map(function(x, k){ return {session_id: x.session_id, turn: x.turn_count, at: iso(nowMs - 20000 * (k + 1)), agent: x.agent,
    fresh: 1000, cache_write: 0, cache_read: 5000, output: 400}; });
  var groups = [{Ts: iso(nowMs), Harness: 'claude', CPUPct: 6, MemMB: 900, IOBps: 4000}, {Ts: iso(nowMs), Harness: 'codex', CPUPct: 3, MemMB: 500, IOBps: 800},
                {Ts: iso(nowMs), Harness: 'wsl', CPUPct: 2, MemMB: 300, IOBps: 100}, {Ts: iso(nowMs), Harness: 'claude-desktop', CPUPct: 1, MemMB: 700, IOBps: 100}];
  var snap = { now: nowMs, hist_gap_ms: 25000,
    sysmon: hist[hist.length - 1], sysmon_history: hist, process_groups_now: groups, process_groups_history: [],
    vendor_strip: {rows: [{agent: 'claude-code', agent_label: 'Claude Code', today: 90000, week: 400000, month: 900000},
                          {agent: 'cowork', agent_label: 'Cowork', today: 20000, week: 100000, month: 300000},
                          {agent: 'codex', agent_label: 'Codex', today: 30000, week: 120000, month: 200000}]},
    activity_heatmap: {rows: []}, todo: {enabled: false}, todo_tasks: {items: []}, headline_today: 140000,
    burn: {generated_at: iso(nowMs), running_window_seconds: 600, sessions: sessions, window_start: iso(ws), bucket_seconds: 60, chart: chart, turns: turns} };
  snap.sysmon = {Ts: iso(nowMs), pressure_score: 30, CPUPct: 40, Cores: [10, 40, 70, 20], MemUsedMB: 14000, MemTotalMB: 32000,
    SwapUsedMB: 0, SwapTotalMB: 1000, NetDownBps: 5e5, NetUpBps: 1e5, Disks: [], Wifi: {OK: true, Connected: false}};
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  return true;
})()`

func d24Read() (d24State, error) {
	var s d24State
	err := evalInto(d24ReadJS, &s)
	return s, err
}

// d24Wait polls until ok holds or 4 s pass, returning the last read.
func d24Wait(ok func(d24State) bool) (d24State, bool, error) {
	var s d24State
	var err error
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if s, err = d24Read(); err != nil {
			return s, false, err
		}
		if ok(s) {
			return s, true, nil
		}
	}
	return s, false, nil
}

// caseD24 starts at 1600x900, dark, with the page focused (applyStart,
// devCaseStart), so the key presses below reach the page.
func caseD24(hwnd uintptr, args []string) error {
	if _, err := evalRaw(d24FakeJS); err != nil {
		return fmt.Errorf("d24: fake paint: %w", err)
	}
	var errs []string
	step := func(name string, vk uint16, ok func(d24State) bool) d24State {
		if vk != 0 {
			if err := pressKey(vk); err != nil {
				errs = append(errs, fmt.Sprintf("%s: press: %v", name, err))
				return d24State{}
			}
		}
		s, good, err := d24Wait(ok)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: eval: %v", name, err))
		} else if !good {
			errs = append(errs, fmt.Sprintf("%s: got %+v", name, s))
		}
		fmt.Printf("uicheck: d24 %s: %+v\n", name, s)
		return s
	}
	start, _, err := d24Wait(func(d24State) bool { return true })
	if err != nil {
		return fmt.Errorf("d24: first read: %w", err)
	}
	isDark := func(s d24State) bool { return s.Attr == "" && s.BodyLum < 0.1 && s.TxtLum > 0.5 }
	if !isDark(start) || len(start.Colours) == 0 {
		errs = append(errs, fmt.Sprintf("dark baseline: got %+v", start))
	}
	if _, err := screenshot(hwnd, "d24-dark-start"); err != nil {
		return err
	}

	step("L turns the page light", vkL, func(s d24State) bool {
		return s.Attr == "light" && s.BodyLum > 0.8 && s.HeaderLum > 0.8 && s.TxtLum < 0.2 && s.PanelLum > 0.8 &&
			len(s.Colours) > 0 && s.MinContrast >= 3 && s.Binding
	})
	time.Sleep(300 * time.Millisecond)
	if _, err := screenshot(hwnd, "d24-light"); err != nil {
		return err
	}

	step("L again restores the dark theme exactly", vkL, func(s d24State) bool {
		return isDark(s) && s.BodyBg == start.BodyBg && strings.Join(s.Colours, "|") == strings.Join(start.Colours, "|")
	})
	time.Sleep(300 * time.Millisecond)
	if _, err := screenshot(hwnd, "d24-dark"); err != nil {
		return err
	}

	if len(errs) > 0 {
		return fmt.Errorf("d24: %d problem(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	fmt.Println("uicheck: d24: L switches dark and light, light colours hold 3:1 on the panel, dark restored exactly")
	return nil
}

// shell/d32: moved from check_d32.go.

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

func caseD32(hwnd uintptr, args []string) error {
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
