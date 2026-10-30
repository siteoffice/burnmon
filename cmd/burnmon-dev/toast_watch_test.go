//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/toast"
	"burnmon/internal/waits"
)

// TestLoadDevConfigToastEnabled: toasts are on unless the file says
// "toast_enabled": false.
func TestLoadDevConfigToastEnabled(t *testing.T) {
	for _, c := range []struct {
		name string
		body string // empty means no file at all
		want bool
	}{
		{"no file", "", true},
		{"key absent", `{"retention_days": 3}`, true},
		{"on", `{"toast_enabled": true}`, true},
		{"off", `{"toast_enabled": false}`, false},
		{"parse error", `{"toast_enabled": `, true},
	} {
		dir := t.TempDir()
		if c.body != "" {
			dir = writeDevJSON(t, c.body)
		}
		if got := loadDevConfig(dir).ToastEnabled; got != c.want {
			t.Errorf("%s: ToastEnabled = %v, want %v", c.name, got, c.want)
		}
	}
	cfg := loadDevConfig(writeDevJSON(t, `{"toast_enabled": false, "retention_days": 3, "station_theme": "site"}`))
	if cfg.RetentionDays != 3 || cfg.StationTheme != "site" {
		t.Errorf("toast_enabled false changed other keys: %+v", cfg)
	}
}

// TestSaveDevConfigKeyKeepsOtherKeys: the key writer the station theme uses
// writes one key; every other key, known or not, keeps its value.
func TestSaveDevConfigKeyKeepsOtherKeys(t *testing.T) {
	dir := writeDevJSON(t, `{"station_theme": "site", "retention_days": 3, "future": {"a": [1, 2]}}`)
	if err := saveDevConfigKey(dir, "toast_enabled", false); err != nil {
		t.Fatal(err)
	}
	cfg := loadDevConfig(dir)
	if cfg.ToastEnabled || cfg.StationTheme != "site" || cfg.RetentionDays != 3 {
		t.Fatalf("config after save = %+v", cfg)
	}
	b, err := os.ReadFile(devJSONPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 4 {
		t.Fatalf("raw has %d keys, want 4: %s", len(raw), b)
	}
	var fut struct {
		A []int `json:"a"`
	}
	if err := json.Unmarshal(raw["future"], &fut); err != nil || len(fut.A) != 2 || fut.A[0] != 1 || fut.A[1] != 2 {
		t.Fatalf("future key lost: %s (%v)", raw["future"], err)
	}
	if err := saveDevConfigKey(dir, "toast_enabled", true); err != nil {
		t.Fatal(err)
	}
	if !loadDevConfig(dir).ToastEnabled {
		t.Fatal("toast_enabled true did not stick")
	}
	noTempLeft(t, dir)
}

func TestSaveDevConfigKeyRefusesUnparsableFile(t *testing.T) {
	const body = `{"toast_enabled": `
	dir := writeDevJSON(t, body)
	if err := saveDevConfigKey(dir, "toast_enabled", false); err == nil || !strings.Contains(err.Error(), "is not valid JSON") {
		t.Fatalf("err = %v, want the not-valid-JSON refusal", err)
	}
	noTempLeft(t, dir)
	b, _ := os.ReadFile(devJSONPath(dir))
	if string(b) != body {
		t.Fatalf("file changed: %s", b)
	}
}

var toastT0 = time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)

func waitGate(id, title string) waits.Item {
	return waits.Item{Kind: waits.KindGate, ID: id, Title: title, Group: waits.GroupBlocked}
}

func agentItem(id, title string) waits.Item {
	return waits.Item{Kind: waits.KindQuestion, ID: id, Title: title, Group: waits.GroupBlocked, Source: waits.SourceAgentView}
}

func okReading(items ...waits.Item) snapshot.Reading {
	return snapshot.Reading{Status: snapshot.StatusOK, Snapshot: &snapshot.Snapshot{WaitItems: items}}
}

// agentReading is an ok reading whose writer runs the agent view; polled
// false means its first poll has not finished yet.
func agentReading(polled bool, items ...waits.Item) snapshot.Reading {
	r := okReading(items...)
	src := snapshot.Source{Name: waits.SourceAgentView, OK: true}
	if !polled {
		src = snapshot.Source{Name: waits.SourceAgentView, Error: snapshotwriter.AgentViewFirstPoll}
	}
	r.Snapshot.Sources = []snapshot.Source{{Name: "claude_pids", OK: true}, src}
	return r
}

// TestToastTickSkipsReadingsThatAreNotOK: a missing, stale, unreadable or
// unsupported file never steps the gate, before the baseline or after it.
func TestToastTickSkipsReadingsThatAreNotOK(t *testing.T) {
	bad := []snapshot.Reading{
		{Status: snapshot.StatusMissing},
		{Status: snapshot.StatusStale, AgeSeconds: 60},
		{Status: snapshot.StatusUnreadable, Error: "unexpected end of JSON input"},
		{Status: snapshot.StatusUnsupported},
		{Status: snapshot.StatusOK}, // ok without a snapshot: nothing to read
	}
	var g toast.Gate
	now := toastT0
	for _, r := range bad {
		if text, ok := toastTick(&g, r, now, true); ok {
			t.Fatalf("%s reading gave %q", r.Status, text)
		}
		now = now.Add(time.Second)
	}
	three := []waits.Item{waitGate("a", "A"), waitGate("b", "B"), waitGate("c", "C")}
	// Had any of those stepped the gate, an empty list would be the baseline
	// and this first real snapshot would fire "3 things wait on you".
	if text, ok := toastTick(&g, okReading(three...), now, true); ok {
		t.Fatalf("the first real snapshot gave %q, want it taken as the baseline", text)
	}
	// After the baseline: had a bad reading stepped with no items, all three
	// would have left and would fire again on their return.
	for _, r := range bad {
		now = now.Add(time.Second)
		if text, ok := toastTick(&g, r, now, true); ok {
			t.Fatalf("%s reading gave %q", r.Status, text)
		}
	}
	now = now.Add(time.Second)
	if text, ok := toastTick(&g, okReading(three...), now, true); ok {
		t.Fatalf("the same three after bad readings gave %q", text)
	}
	now = now.Add(time.Second)
	if text, ok := toastTick(&g, okReading(append(three, waitGate("d", "D"))...), now, true); !ok || text != "Gate: D" {
		t.Fatalf("a new gate gave %q %v, want Gate: D", text, ok)
	}
}

// TestToastTickOffStillSteps: with toasts off the gate keeps up, so turning
// them on later does not toast what already waited.
func TestToastTickOffStillSteps(t *testing.T) {
	var g toast.Gate
	a, b, c := waitGate("a", "A"), waitGate("b", "B"), waitGate("c", "C")
	if _, ok := toastTick(&g, okReading(a), toastT0, false); ok {
		t.Fatal("baseline while off gave a toast")
	}
	if text, ok := toastTick(&g, okReading(a, b), toastT0.Add(time.Second), false); ok || text != "" {
		t.Fatalf("off: got %q %v, want nothing", text, ok)
	}
	if text, ok := toastTick(&g, okReading(a, b), toastT0.Add(2*time.Second), true); ok {
		t.Fatalf("switched on: b, which entered while off, gave %q", text)
	}
	if text, ok := toastTick(&g, okReading(a, b, c), toastT0.Add(3*time.Second), true); !ok || text != "Gate: C" {
		t.Fatalf("on: a new gate gave %q %v, want Gate: C", text, ok)
	}
}

// TestToastTickWaitsForFirstAgentViewPoll: before the writer's first agent
// view poll its items are missing, and they arrive one write later. Such a
// reading is skipped, so they are no burst: not as the baseline, and not
// when the writer restarts later (another app takes the snapshot lock).
func TestToastTickWaitsForFirstAgentViewPoll(t *testing.T) {
	var g toast.Gate
	gate := waitGate("flow:x", "flow x")
	q1, q2, q3 := agentItem("s1", "burnmon 1a2b3c4d"), agentItem("s2", "znd 5e6f7a8b"), agentItem("s3", "site 0c0d0e0f")
	now := toastT0
	step := func(r snapshot.Reading) (string, bool) {
		now = now.Add(time.Second)
		return toastTick(&g, r, now, true)
	}
	if text, ok := step(agentReading(false, gate)); ok {
		t.Fatalf("reading before the first poll gave %q", text)
	}
	if text, ok := step(agentReading(true, gate, q1, q2)); ok {
		t.Fatalf("the agent view items of the first poll burst: %q", text)
	}
	if text, ok := step(agentReading(false, gate)); ok {
		t.Fatalf("a restarted writer's first write gave %q", text)
	}
	if text, ok := step(agentReading(true, gate, q1, q2)); ok {
		t.Fatalf("the agent view items after a writer restart burst: %q", text)
	}
	if text, ok := step(agentReading(true, gate, q1, q2, q3)); !ok || text != "Question: site 0c0d0e0f" {
		t.Fatalf("a new question gave %q %v", text, ok)
	}
}

func writeToastSnapshot(t *testing.T, path string, at time.Time, items ...waits.Item) {
	t.Helper()
	s := snapshot.Snapshot{SchemaVersion: snapshot.SchemaVersion, WrittenAt: at.UTC().Format(time.RFC3339), TickSeconds: 2, WaitItems: items}
	if err := snapshot.Write(path, s); err != nil {
		t.Fatal(err)
	}
}

// TestToastWatchPoll: the watch reads the file only when its stamp changed,
// takes the first real one as the baseline and shows what enters after it.
func TestToastWatchPoll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	var on atomic.Bool
	on.Store(true)
	var shown []string
	tw := &toastWatch{path: path, on: &on, show: func(s string) { shown = append(shown, s) }}
	now := time.Now()
	tw.poll(now) // no file yet
	writeToastSnapshot(t, path, now, waitGate("a", "A"), waitGate("b", "B"))
	tw.poll(now)
	tw.poll(now) // same stamp: not read again
	if len(shown) != 0 {
		t.Fatalf("baseline showed %q", shown)
	}
	writeToastSnapshot(t, path, now, waitGate("a", "A"), waitGate("b", "B"), waitGate("c", "C"))
	tw.poll(now)
	tw.poll(now)
	if len(shown) != 1 || shown[0] != "Gate: C" {
		t.Fatalf("shown = %q, want one Gate: C", shown)
	}
	on.Store(false)
	writeToastSnapshot(t, path, now, waitGate("a", "A"), waitGate("b", "B"), waitGate("c", "C"), waitGate("d", "D"))
	tw.poll(now)
	if len(shown) != 1 {
		t.Fatalf("toasts off still showed %q", shown[1:])
	}
}

func TestSectionsPayloadCarriesToastEnabled(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if !sectionsPayloadFor(dir, filepath.Join(dir, "none.json"), now).ToastEnabled {
		t.Fatal("fresh dir: toast_enabled false, want the default on")
	}
	if err := saveDevConfigKey(dir, "toast_enabled", false); err != nil {
		t.Fatal(err)
	}
	p := sectionsPayloadFor(dir, filepath.Join(dir, "none.json"), now)
	if p.ToastEnabled {
		t.Fatal("after saving false: toast_enabled still true")
	}
	b, _ := json.Marshal(p)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	if string(m["toast_enabled"]) != "false" {
		t.Fatalf("payload JSON toast_enabled = %s, want false", m["toast_enabled"])
	}
}

// TestPageToastSwitch: the gear dialog has the switch, set from the payload
// (absent counts as on) and saved through bdevSetToast; a failed save puts
// the box back.
func TestPageToastSwitch(t *testing.T) {
	page := idlePage()
	const row = `<label class="setrow"><input type="checkbox" id="secToast"> Show a notification when something waits on you</label>`
	i, j := strings.Index(page, `<div id="secSettings"`), strings.Index(page, `<div id="histAdmin">`)
	if i < 0 || j < i {
		t.Fatal("cannot find the gear dialog")
	}
	if !strings.Contains(page[i:j], row) {
		t.Errorf("the gear dialog lacks %q", row)
	}
	if body := idleFn(t, "secApplyPayload"); !strings.Contains(body, "toast:p.toast_enabled !== false") {
		t.Error("secApplyPayload does not set secState.toast from p.toast_enabled")
	}
	if body := idleFn(t, "secRenderSettings"); !strings.Contains(body, "if(tg) tg.checked = !!secState.toast;") {
		t.Error("secRenderSettings does not tick secToast from secState.toast")
	}
	for _, want := range []string{
		"else if(t.id === 'secToast'){",
		"Promise.resolve(window.bdevSetToast(t.checked))",
		"console.log('bdevSetToast:', e);",
		"t.checked = !!secState.toast;",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page.html lacks %q", want)
		}
	}
}

// TestToastWiring: BurnMon Dev starts the notifier and binds the switch. The
// binding runs on the UI thread, so it only writes the small json file and
// flips the flag the watch goroutine reads.
func TestToastWiring(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := strings.ReplaceAll(string(b), "\r\n", "\n")
	for _, want := range []string{
		`toast.Start("BurnMon Dev", func() { bringToFront(hwnd) })`,
		"startToastWatch(snapPath, &toastOn, n.Show)",
		"if err := w.Bind(\"bdevSetToast\", func(on bool) error {\n\t\terr := saveDevConfigKey(setDir, \"toast_enabled\", on)",
		"toastOn.Store(on)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("main.go lacks %q", want)
		}
	}
}
