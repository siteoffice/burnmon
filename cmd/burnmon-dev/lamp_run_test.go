//go:build windows || (darwin && cgo)

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"burnmon/internal/lamp"
	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
	"burnmon/internal/winbind"
)

func TestParseLampArgs(t *testing.T) {
	o, err := parseLampArgs(nil)
	if err != nil || o.demo || o.poses != "" {
		t.Fatalf("no args: %+v %v", o, err)
	}
	o, err = parseLampArgs([]string{"--demo"})
	if err != nil || !o.demo {
		t.Fatalf("--demo: %+v %v", o, err)
	}
	o, err = parseLampArgs([]string{"--poses", "somewhere"})
	if err != nil || o.poses != "somewhere" {
		t.Fatalf("--poses: %+v %v", o, err)
	}
	o, err = parseLampArgs([]string{"--card", "out", "--demo"})
	if err != nil || o.card != "out" || !o.demo || o.cardWindow {
		t.Fatalf("--card --demo: %+v %v", o, err)
	}
	o, err = parseLampArgs([]string{"--card", "out", "--card-window"})
	if err != nil || !o.cardWindow {
		t.Fatalf("--card --card-window: %+v %v", o, err)
	}
	for _, bad := range [][]string{{"--nope"}, {"--poses"}, {"--demo", "--poses", "x"}, {"extra"},
		{"--card"}, {"--card-window"}, {"--card", "x", "--poses", "y"}} {
		if _, err := parseLampArgs(bad); err == nil {
			t.Errorf("parseLampArgs(%v) accepted", bad)
		}
	}
}

func TestRunLampPosesWritesTenFilesAndRefusesMissing(t *testing.T) {
	dir := t.TempDir()
	if code := runLamp([]string{"--poses", dir}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 10 {
		t.Fatalf("%d files, want 10", len(ents))
	}
	if code := runLamp([]string{"--poses", filepath.Join(dir, "missing")}); code == 0 {
		t.Fatal("a missing folder was accepted")
	}
}

func TestLampSingleInstanceNamesDiffer(t *testing.T) {
	if lampMutexName == mutexName || lampDemoMutexName == mutexName || lampMutexName == lampDemoMutexName {
		t.Fatalf("mutex names %q %q %q are not three different names", mutexName, lampMutexName, lampDemoMutexName)
	}
}

// liveFixture writes a fresh snapshot with one question and returns the engine.
func liveFixture(t *testing.T, now time.Time, ageSec int64, settings string) (*lampEngine, string) {
	t.Helper()
	dir := t.TempDir()
	snap := snapshot.Snapshot{
		SchemaVersion: snapshot.SchemaVersion,
		WrittenAt:     now.UTC().Format(time.RFC3339),
		TickSeconds:   2,
		WaitItems: []waits.Item{{
			Kind: waits.KindQuestion, ID: "session:claude:fx1", Title: "Pick one", Group: waits.GroupBlocked,
			AgeSeconds: ageSec,
		}},
		WaitCounts: waits.Counts{Blocked: 1},
	}
	b, _ := json.Marshal(snap)
	snapPath := filepath.Join(dir, "snapshot.json")
	if err := os.WriteFile(snapPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	setDir := filepath.Join(dir, "set")
	if err := os.MkdirAll(setDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if settings != "" {
		if err := os.WriteFile(filepath.Join(setDir, "burnmon-dev.json"), []byte(settings), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return newLampEngine(false, setDir, snapPath), setDir
}

func TestEngineLiveReadsSnapshotSettingsAndTheme(t *testing.T) {
	t.Setenv(settingsDirEnv, "")
	now := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	e, setDir := liveFixture(t, now, 120, `{"lamp_language":"nl"}`)
	if err := os.WriteFile(filepath.Join(setDir, "burnmon-dev-view.json"), []byte(`{"ui_theme":"light"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p := e.payload(now)
	if p.State.Mood != lamp.MoodBlocked || p.State.Demo {
		t.Fatalf("state %+v", p.State)
	}
	if p.Theme != "light" {
		t.Errorf("theme %q", p.Theme)
	}
	if p.Labels["tokens"] != "tokens per minuut" {
		t.Errorf("tokens label %q", p.Labels["tokens"])
	}
	if p.State.Line == "" {
		t.Error("no line")
	}
}

func TestEngineNotFreshIsOff(t *testing.T) {
	t.Setenv(settingsDirEnv, "")
	now := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	e, _ := liveFixture(t, now, 120, "")
	if got := e.payload(now.Add(7 * time.Second)).State.Mood; got != lamp.MoodOff {
		t.Fatalf("mood %q 7 s after a 2 s tick, want off", got)
	}
}

func TestEngineNextLineSurvivesPollsInTheSameFiveMinutes(t *testing.T) {
	t.Setenv(settingsDirEnv, "")
	now := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	e, _ := liveFixture(t, now, 120, "")
	first := e.payload(now).State.Line
	next := e.nextLine()
	if next == "" || next == first {
		t.Fatalf("next line %q after %q", next, first)
	}
	if got := e.payload(now.Add(time.Second)).State.Line; got != next {
		t.Fatalf("the next line %q was replaced by %q on the next poll", next, got)
	}
}

func TestEngineKeepsBurnSamplesPerFreshSnapshot(t *testing.T) {
	t.Setenv(settingsDirEnv, "")
	now := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	e, _ := liveFixture(t, now, 120, "")
	e.payload(now)
	e.payload(now) // same snapshot again: no second sample
	if n := len(e.samples); n != 1 {
		t.Fatalf("%d samples after two reads of one snapshot, want 1", n)
	}
}

func TestEngineDemoWritesNoFileAndFollowsItsOwnClock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv(settingsDirEnv, "")
	e := newLampEngine(true, "", "")
	start := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	e.start = start
	var moods []string
	for sec := 0; sec < lamp.DemoSeconds; sec += 3 {
		p := e.payload(start.Add(time.Duration(sec) * time.Second))
		if !p.State.Demo || p.Theme != "dark" {
			t.Fatalf("second %d: %+v", sec, p)
		}
		moods = append(moods, p.State.Mood)
	}
	if moods[0] != lamp.MoodAsleep || moods[7] != lamp.MoodBored || moods[15] != lamp.MoodAsleep {
		t.Fatalf("moods %v", moods)
	}
	e.nextLine()
	var left []string
	_ = filepath.Walk(home, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			left = append(left, p)
		}
		return nil
	})
	if len(left) != 0 {
		t.Fatalf("the demo wrote files: %v", left)
	}
}

func TestEngineDemoPlaysTheActsOfTheTimeline(t *testing.T) {
	e := newLampEngine(true, "", "")
	start := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	e.start = start
	var order []string
	seq := map[string]int64{}
	holds := map[string]bool{}
	for sec := 0; sec < 2*lamp.DemoSeconds; sec++ {
		p := e.payload(start.Add(time.Duration(sec) * time.Second))
		for _, h := range p.Holds {
			holds[h] = true
		}
		if p.Act == "" {
			continue
		}
		if sec < lamp.DemoSeconds && (len(order) == 0 || order[len(order)-1] != p.Act) {
			order = append(order, p.Act)
			seq[p.Act] = p.ActSeq
		}
		if sec >= lamp.DemoSeconds && p.ActSeq == seq[p.Act] {
			t.Errorf("second %d: act %s has the seq of the first loop", sec, p.Act)
		}
		if len(p.ActLines) == 0 && p.Act != "knock" && p.Act != "alarm" && p.Act != "peek" {
			t.Errorf("second %d: act %s has no line", sec, p.Act)
		}
	}
	want := []string{"wake", "knock", "alarm", "stamp", "slip", "welcome", "coffee", "peek"}
	if strings.Join(order, " ") != strings.Join(want, " ") {
		t.Fatalf("acts in the demo loop %v, want %v", order, want)
	}
	for _, h := range []string{lamp.HoldWall, lamp.HoldSweat, lamp.HoldSleep, lamp.HoldDots} {
		if !holds[h] {
			t.Errorf("the demo payload never holds %s", h)
		}
	}
}

func TestLampUnsupportedSaysTheSentenceAndExits2(t *testing.T) {
	var buf bytes.Buffer
	if code := lampUnsupported(&buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	// The Mac app (the cgo build) has a lamp, so the sentence names the builds
	// that have one and never says "Windows only".
	want := "This build has no lamp window. The lamp runs in the Windows build and in the Mac app (the build with cgo)."
	if got := strings.TrimSpace(buf.String()); got != want {
		t.Fatalf("message %q, want %q", got, want)
	}
	if strings.Contains(buf.String(), "Windows only") {
		t.Fatalf("message %q still says Windows only", buf.String())
	}
}

func listFiles(dir string) []string {
	var out []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func TestCardDemoWritesTwoCardsAndNoLedger(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv(settingsDirEnv, "")
	out := t.TempDir()
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	if code := runCardAt(lampOpts{card: out, demo: true}, now); code != 0 {
		t.Fatalf("exit %d", code)
	}
	files := listFiles(out)
	if len(files) != 2 {
		t.Fatalf("%v", files)
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if !strings.Contains(string(b), ">demo<") {
			t.Errorf("%s has no demo mark", f)
		}
	}
	if left := listFiles(home); len(left) != 0 {
		t.Fatalf("the demo card wrote files outside its folder: %v", left)
	}
}

func TestCardLiveReadsTheLedgerAndRefusesAMissingFolder(t *testing.T) {
	setDir := t.TempDir()
	t.Setenv(settingsDirEnv, setDir)
	ledger := `{"version":1,"days":{"2026-03-04":{"agent_wait_seconds":3600,"longest_wait_seconds":300,"items":[1,2],"lamp_on_seconds":7200}}}`
	if err := os.WriteFile(filepath.Join(setDir, lamp.LedgerFile), []byte(ledger), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(setDir, "burnmon-dev.json"), []byte(`{"lamp_language":"nl"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	if code := runCardAt(lampOpts{card: out}, now); code != 0 {
		t.Fatalf("exit %d", code)
	}
	b, err := os.ReadFile(filepath.Join(out, "glim-week-2026-W10-wide.svg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Mijn agents wachtten op mij.", ">1 h<", ">5 min<", ">2 h<"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the card lacks %q", want)
		}
	}
	if strings.Contains(string(b), "demo") {
		t.Error("a live card says demo")
	}
	if code := runCardAt(lampOpts{card: filepath.Join(out, "missing")}, now); code == 0 {
		t.Fatal("a missing folder was accepted")
	}
}

func TestEngineLiveWritesTheLedgerAtExitAndDemoHasNone(t *testing.T) {
	t.Setenv(settingsDirEnv, "")
	now := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	e, setDir := liveFixture(t, now, 120, "")
	path := filepath.Join(setDir, lamp.LedgerFile)
	e.ledger = lamp.LoadLedger(path)
	e.payload(now)
	e.payload(now.Add(2 * time.Second))
	e.ledger.Close(now.Add(2 * time.Second))
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no ledger: %v", err)
	}
	if strings.Contains(string(b), "session:claude:fx1") || strings.Contains(string(b), "Pick one") {
		t.Fatalf("the ledger holds an id or a title: %s", b)
	}
	if d := lamp.LoadLedger(path).Days()["2026-03-02"]; d.LampOnSeconds != 2 || len(d.Items) != 1 {
		t.Fatalf("ledger day %+v", d)
	}
	if newLampEngine(true, "", "").ledger != nil {
		t.Fatal("the demo engine has a ledger")
	}
}

// The card page calls only the names lampCardBindingNames lists, holds no
// outside resource and no em dash.
func TestCardPageBindingsAndText(t *testing.T) {
	called := winbind.PageCalls(lampCardHTML, "lamp")
	want := append([]string(nil), lampCardBindingNames...)
	sort.Strings(want)
	if strings.Join(called, ",") != strings.Join(want, ",") {
		t.Fatalf("page calls %v, lampCardBindingNames is %v", called, want)
	}
	for _, bad := range []string{string(rune(0x2014)), "http://", "https://", "src=\"//", "<script src", "focus()"} {
		if strings.Contains(lampCardHTML, bad) {
			t.Errorf("the card page holds %q", bad)
		}
	}
	if !strings.Contains(lampCardHTML, "Save PNG") {
		t.Error("the card page has no Save PNG button")
	}
}

// The lamp ends when the BurnMon Dev that started it is gone. A lamp started
// by hand has no variable and stays; so does one with an unreadable value.
func TestLampParentGone(t *testing.T) {
	var asked []int
	alive := func(pid int) bool { asked = append(asked, pid); return pid == 4242 }
	for _, c := range []struct {
		env  string
		want bool
	}{
		{"", false}, {"abc", false}, {"0", false}, {"-5", false}, {"4242", false}, {"4243", true},
	} {
		if got := lampParentGone(c.env, alive); got != c.want {
			t.Errorf("lampParentGone(%q) = %v, want %v", c.env, got, c.want)
		}
	}
	if len(asked) != 2 {
		t.Errorf("alive was asked %v, want only the two good process ids", asked)
	}
}

func TestLampChildEnvAddsTheParentOnceAndKeepsTheRest(t *testing.T) {
	base := []string{"A=1", "BURNMON_LAMP_PARENT=99", "B=2"}
	got := lampChildEnv(base, 4242)
	n, kept := 0, 0
	for _, e := range got {
		switch {
		case strings.HasPrefix(e, "BURNMON_LAMP_PARENT="):
			n++
			if e != "BURNMON_LAMP_PARENT=4242" {
				t.Errorf("parent entry %q", e)
			}
		case e == "A=1" || e == "B=2":
			kept++
		}
	}
	if n != 1 || kept != 2 {
		t.Errorf("env %v: %d parent entries, %d kept", got, n, kept)
	}
	if len(base) != 3 || base[1] != "BURNMON_LAMP_PARENT=99" {
		t.Errorf("the base slice was changed: %v", base)
	}
}

// The lamp's dev eval channel for tools\uicheck (lane scope X5, plan C16)
// starts only with its own environment variable, on its own port beside 9333,
// 9334 and 9335, and only winbind.StartUICheckServer may read the variable.
func TestLampUICheckServerIsDevOnly(t *testing.T) {
	b, err := os.ReadFile("lamp_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	var code []string
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "//") {
			code = append(code, l)
		}
	}
	src := strings.Join(code, "\n")
	for _, want := range []string{`"BURNMON_LAMP_UICHECK"`, `"127.0.0.1:9336"`, `"lampUICheckResult"`, "winbind.StartUICheckServer"} {
		if !strings.Contains(src, want) {
			t.Errorf("lamp_window_windows.go does not contain %s", want)
		}
	}
	for _, other := range []string{"9333", "9334", "9335"} {
		if strings.Contains(src, other) {
			t.Errorf("lamp_window_windows.go uses port %s, which belongs to another exe", other)
		}
	}
	if strings.Count(src, "BURNMON_LAMP_UICHECK") != 1 {
		t.Error("the variable must appear once, in the options of the eval server")
	}
}

// writeLampSnap rewrites the fixture snapshot with n blocked questions, written at at.
func writeLampSnap(t *testing.T, path string, at time.Time, n int) {
	t.Helper()
	var items []waits.Item
	for i := 0; i < n; i++ {
		items = append(items, waits.Item{
			Kind: waits.KindQuestion, ID: "session:claude:fx" + string(rune('a'+i)), Title: "Pick one",
			Group: waits.GroupBlocked, AgeSeconds: 120,
		})
	}
	snap := snapshot.Snapshot{
		SchemaVersion: snapshot.SchemaVersion, WrittenAt: at.UTC().Format(time.RFC3339), TickSeconds: 2,
		WaitItems: items, WaitCounts: waits.CountOf(items),
	}
	b, _ := json.Marshal(snap)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnginePayloadCarriesTheActAndItsSeq(t *testing.T) {
	t.Setenv(settingsDirEnv, "")
	now := time.Date(2026, 3, 2, 12, 3, 10, 0, time.UTC)
	e, _ := liveFixture(t, now, 120, "")
	snapPath := e.snapPath
	old := lampIdle
	lampIdle = func() int { return 0 }
	defer func() { lampIdle = old }()

	if p := e.payload(now); p.Act != "" || p.ActSeq != 0 {
		t.Fatalf("first tick: act %q seq %d, want none (what is already there is not news)", p.Act, p.ActSeq)
	}
	// A second blocked item arrives: alarm.
	at := now.Add(3 * time.Second)
	writeLampSnap(t, snapPath, at, 2)
	p := e.payload(at)
	if p.Act != "alarm" || p.ActSeq != 1 {
		t.Fatalf("blocked 1 to 2: act %q seq %d, want alarm 1", p.Act, p.ActSeq)
	}
	// The act keeps showing, with the same seq, for its 2.6 seconds.
	writeLampSnap(t, snapPath, at.Add(time.Second), 2)
	p = e.payload(at.Add(time.Second))
	if p.Act != "alarm" || p.ActSeq != 1 {
		t.Fatalf("1 s into the act: act %q seq %d, want alarm 1", p.Act, p.ActSeq)
	}
	// After it, no act and the seq stays.
	writeLampSnap(t, snapPath, at.Add(3*time.Second), 2)
	p = e.payload(at.Add(3 * time.Second))
	if p.Act != "" || p.ActSeq != 1 {
		t.Fatalf("after the act: act %q seq %d, want none 1", p.Act, p.ActSeq)
	}
	if p.Look.Rows != 1 || p.Look.Size != 150 || p.Look.Theme != "system" || p.Look.Motion != "lively" || p.Look.Quiet != "hide" || p.Look.Backdrop != "clear" {
		t.Errorf("look %+v, want the defaults", p.Look)
	}
	if p.Quiet {
		t.Error("blocked mood shows the sliver")
	}
}

func TestEngineWelcomeUsesTheIdleSeam(t *testing.T) {
	t.Setenv(settingsDirEnv, "")
	now := time.Date(2026, 3, 2, 12, 3, 10, 0, time.UTC)
	e, _ := liveFixture(t, now, 120, "")
	old := lampIdle
	defer func() { lampIdle = old }()
	idle := 400
	lampIdle = func() int { return idle }
	e.payload(now) // first tick, idle 400
	idle = 1
	at := now.Add(3 * time.Second)
	writeLampSnap(t, e.snapPath, at, 1)
	if p := e.payload(at); p.Act != "welcome" {
		t.Fatalf("idle 400 then 1: act %q, want welcome", p.Act)
	}
}

func TestEngineCalmMotionPlaysNoAct(t *testing.T) {
	t.Setenv(settingsDirEnv, "")
	now := time.Date(2026, 3, 2, 12, 3, 10, 0, time.UTC)
	e, _ := liveFixture(t, now, 120, `{"lamp_motion":"calm"}`)
	old := lampIdle
	lampIdle = func() int { return 0 }
	defer func() { lampIdle = old }()
	e.payload(now)
	at := now.Add(3 * time.Second)
	writeLampSnap(t, e.snapPath, at, 2)
	if p := e.payload(at); p.Act != "" {
		t.Fatalf("calm: act %q", p.Act)
	}
}

// The page shows the line of an act; the engine gives the lines, so the page
// works out none (UI standard 16).
func TestLampActLines(t *testing.T) {
	cases := []struct {
		act   string
		lang  string
		count int
		hour  int
		seq   int64
		want  []string
	}{
		{"slip", "en", 0, 12, 1, []string{"I am not bored. I am testing the rail.", "Whoa.", "You saw nothing."}},
		{"slip", "nl", 0, 12, 1, []string{"Ik verveel me niet. Ik test de rail.", "Ho.", "Je hebt niets gezien."}},
		{"stamp", "en", 0, 12, 1, []string{"Stamped. Back to work."}},
		{"stamp", "en", 0, 12, 2, []string{"Answered. That was all it took."}},
		{"welcome", "en", 1, 12, 1, []string{"Welcome back. One thing waited."}},
		{"welcome", "en", 3, 12, 1, []string{"Welcome back. 3 things waited."}},
		{"coffee", "en", 0, 15, 1, []string{"Three o'clock. Coffee. The agents get none."}},
		{"wake", "nl", 0, 12, 1, []string{"Iemand is iets begonnen."}},
		{"alarm", "en", 2, 12, 1, nil},
		{"", "en", 2, 12, 1, nil},
	}
	for _, c := range cases {
		got := lampActLines(c.act, c.lang, c.count, c.hour, c.seq)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("lampActLines(%q,%q,%d,%d,%d) = %q, want %q", c.act, c.lang, c.count, c.hour, c.seq, got, c.want)
		}
	}
}

// The window size before the page reports one: the ticker is 300 px wide with
// 26 px per row, Glim's box is 120 times the size scale below it. It always
// covers the quiet sliver (120 by 6 times the scale).
func TestLampWindowSize(t *testing.T) {
	cases := []struct {
		rows  int
		size  int
		wantW int
		wantH int
	}{
		{1, 150, 300, 26 + 180},
		{4, 150, 300, 104 + 180},
		{1, 120, 300, 26 + 144},
		{2, 190, 300, 52 + 228},
		{1, 100, 300, 26 + 120},
		{1, 200, 300, 26 + 240},
		{0, 0, 300, 26 + 180}, // bad values are the defaults
		{9, 150, 300, 104 + 180},
	}
	for _, c := range cases {
		w, h := lampWindowSize(lamp.Settings{TickerRows: c.rows, SizePct: c.size})
		if w != c.wantW || h != c.wantH {
			t.Errorf("lampWindowSize(%d,%d) = %dx%d, want %dx%d", c.rows, c.size, w, h, c.wantW, c.wantH)
		}
		if w < 120 || h < 6 {
			t.Errorf("lampWindowSize(%d,%d) = %dx%d does not cover the sliver", c.rows, c.size, w, h)
		}
	}
}

// The page's lampArea JSON (CSS pixels) becomes device rectangles: rounded,
// scaled, an empty or missing box left out, bad JSON gives none.
func TestLampRegionRects(t *testing.T) {
	area := `{"ticker":{"x":10,"y":0,"w":300,"h":26},"glim":{"x":100,"y":26,"w":120.4,"h":180},"extra":null}`
	got := lampRegionRects(area, 1.5)
	want := []lampRect{{15, 0, 450, 39}, {150, 39, 181, 270}}
	if len(got) != len(want) {
		t.Fatalf("rects = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rect %d = %v, want %v", i, got[i], want[i])
		}
	}
	extra := `{"ticker":{"x":0,"y":0,"w":300,"h":26},"glim":null,"extra":{"x":0,"y":0,"w":500,"h":240}}`
	if g := lampRegionRects(extra, 1); len(g) != 2 || g[1] != (lampRect{0, 0, 500, 240}) {
		t.Errorf("extra box: %v", g)
	}
	for _, bad := range []string{"", "{", `{"ticker":{"x":0,"y":0,"w":0,"h":26}}`, `{"ticker":{"x":0,"y":0,"w":-3,"h":26}}`} {
		if g := lampRegionRects(bad, 1); len(g) != 0 {
			t.Errorf("lampRegionRects(%q) = %v, want none", bad, g)
		}
	}
	if g := lampRegionRects(area, 0); len(g) != 2 || g[0].W != 300 {
		t.Errorf("scale 0 must act as 1: %v", g)
	}
}

// The clear backdrop sets the WebView2 default background to fully
// transparent; the card backdrop sets nothing.
func TestLampBackdropEnv(t *testing.T) {
	k, v, ok := lampBackdropEnv("clear")
	if !ok || k != "WEBVIEW2_DEFAULT_BACKGROUND_COLOR" || v != "00000000" {
		t.Errorf("clear = %q %q %v", k, v, ok)
	}
	for _, b := range []string{"card", "", "x"} {
		if _, _, ok := lampBackdropEnv(b); ok {
			t.Errorf("backdrop %q must set no variable", b)
		}
	}
}

// BurnMon Dev must not inherit the transparent background from the lamp.
func TestLampWithoutEnv(t *testing.T) {
	base := []string{"A=1", "WEBVIEW2_DEFAULT_BACKGROUND_COLOR=00000000", "B=2", "WEBVIEW2_DEFAULT_BACKGROUND_COLORX=keep"}
	got := lampWithoutEnv(base, "WEBVIEW2_DEFAULT_BACKGROUND_COLOR")
	if strings.Join(got, "|") != "A=1|B=2|WEBVIEW2_DEFAULT_BACKGROUND_COLORX=keep" {
		t.Errorf("got %v", got)
	}
	if len(base) != 4 || base[1] == "" {
		t.Errorf("base changed: %v", base)
	}
}

// Idle seconds from two 32 bit tick counts, also across the wrap at 49 days.
func TestLampIdleFrom(t *testing.T) {
	cases := []struct {
		now, last uint32
		want      int
	}{
		{10000, 10000, 0},
		{12999, 10000, 2},
		{310000, 10000, 300},
		{500, 4294967000, 0}, // wrapped: 796 ms
		{5000, 4294967000, 5},
		{10000, 20000, 0}, // last after now: never negative
	}
	for _, c := range cases {
		if got := lampIdleFrom(c.now, c.last); got != c.want {
			t.Errorf("lampIdleFrom(%d,%d) = %d, want %d", c.now, c.last, got, c.want)
		}
	}
}

// Fix lane T2, finding 3: the window region holds every box whole. Each edge
// rounds outward (floor on the left and top, ceiling on the right and bottom),
// so a border that falls on a fraction of a device pixel at 125 or 150 percent
// is never cut.
func TestLampRegionRectsHoldEveryBoxWhole(t *testing.T) {
	boxes := []lampBox{
		{X: 10.4, Y: 0, W: 300, H: 26},
		{X: 51.3, Y: 26.2, W: 198.7, H: 180.1},
		{X: 0, Y: 0, W: 301.7, H: 105.5},
	}
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		for _, b := range boxes {
			area, err := json.Marshal(map[string]lampBox{"ticker": b})
			if err != nil {
				t.Fatal(err)
			}
			got := lampRegionRects(string(area), scale)
			if len(got) != 1 {
				t.Fatalf("scale %v box %+v: %d rects", scale, b, len(got))
			}
			r := got[0]
			const eps = 1e-6
			if float64(r.X) > b.X*scale+eps || float64(r.Y) > b.Y*scale+eps ||
				float64(r.X+r.W) < (b.X+b.W)*scale-eps || float64(r.Y+r.H) < (b.Y+b.H)*scale-eps {
				t.Errorf("scale %v: rect %+v does not hold box %+v whole", scale, r, b)
			}
			// Outward rounding adds at most one device pixel on each edge.
			if float64(r.X+r.W) > (b.X+b.W)*scale+1+eps || float64(r.Y+r.H) > (b.Y+b.H)*scale+1+eps {
				t.Errorf("scale %v: rect %+v is more than a pixel too big for box %+v", scale, r, b)
			}
		}
	}
}

var lampTestMons = []lampMonitor{
	{Name: "DISPLAY1", X: -1600, Y: 0, W: 1600, H: 1000, Scale: 1},
	{Name: "DISPLAY2", X: 0, Y: 0, W: 3440, H: 1440, Primary: true, Scale: 1},
	{Name: "DISPLAY3", X: 3440, Y: -300, W: 1440, H: 2560, Scale: 1.25},
}

// The lamp hangs at the top edge of the monitor named in lamp_monitor, at left,
// centre or right. A name that is not connected, or an empty one, is the primary.
func TestLampPlaceOn(t *testing.T) {
	cases := []struct {
		name, pos string
		w         int
		wantX     int
		wantY     int
		wantUsed  string
	}{
		{"DISPLAY2", "top-left", 300, 0, 0, "DISPLAY2"},
		{"DISPLAY2", "top-center", 300, 1570, 0, "DISPLAY2"},
		{"DISPLAY2", "top-right", 300, 3140, 0, "DISPLAY2"},
		{"DISPLAY3", "top-left", 400, 3440, -300, "DISPLAY3"},
		{"DISPLAY3", "top-center", 400, 3960, -300, "DISPLAY3"},
		{"DISPLAY3", "top-right", 400, 4480, -300, "DISPLAY3"},
		{"DISPLAY1", "top-right", 300, -300, 0, "DISPLAY1"},
		{"GONE", "top-center", 300, 1570, 0, "DISPLAY2"},
		{"", "top-left", 300, 0, 0, "DISPLAY2"},
		{"DISPLAY3", "bad", 400, 3960, -300, "DISPLAY3"},
	}
	for _, c := range cases {
		x, y, used := lampPlaceOn(lampTestMons, c.name, c.pos, c.w)
		if x != c.wantX || y != c.wantY || used != c.wantUsed {
			t.Errorf("lampPlaceOn(%q,%q,%d) = %d,%d,%q want %d,%d,%q", c.name, c.pos, c.w, x, y, used, c.wantX, c.wantY, c.wantUsed)
		}
	}
	if x, y, used := lampPlaceOn(nil, "DISPLAY2", "top-left", 300); x != 0 || y != 0 || used != "" {
		t.Errorf("no monitor list: %d,%d,%q", x, y, used)
	}
}

// A dropped lamp snaps to the monitor under the pointer and to the nearest of
// left, centre and right. A pointer between monitors takes the nearest one.
func TestLampSnap(t *testing.T) {
	cases := []struct {
		px, py   int
		w        int
		wantName string
		wantPos  string
	}{
		{100, 500, 300, "DISPLAY2", "top-left"},
		{1720, 500, 300, "DISPLAY2", "top-center"},
		{3300, 500, 300, "DISPLAY2", "top-right"},
		{3500, 100, 400, "DISPLAY3", "top-left"},
		{4160, 100, 400, "DISPLAY3", "top-center"},
		{4800, 100, 400, "DISPLAY3", "top-right"},
		{-800, 300, 300, "DISPLAY1", "top-center"},
		{3440, 2000, 400, "DISPLAY3", "top-left"}, // on the shared edge: the monitor that holds the point
		{-2500, 400, 300, "DISPLAY1", "top-left"}, // left of every monitor: the nearest one
		{9000, 400, 400, "DISPLAY3", "top-right"},
	}
	for _, c := range cases {
		name, pos := lampSnap(lampTestMons, c.px, c.py, c.w)
		if name != c.wantName || pos != c.wantPos {
			t.Errorf("lampSnap(%d,%d,%d) = %q,%q want %q,%q", c.px, c.py, c.w, name, pos, c.wantName, c.wantPos)
		}
	}
	if name, pos := lampSnap(nil, 5, 5, 300); name != "" || pos != "top-center" {
		t.Errorf("no monitor list: %q,%q", name, pos)
	}
}

// While the button is down the window keeps the offset the pointer had inside it.
func TestLampDragPos(t *testing.T) {
	if x, y := lampDragPos(1000, 400, 120, 15); x != 880 || y != 385 {
		t.Errorf("lampDragPos = %d,%d want 880,385", x, y)
	}
	if x, y := lampDragPos(-50, -20, 10, 5); x != -60 || y != -25 {
		t.Errorf("a pointer on another monitor to the left: %d,%d want -60,-25", x, y)
	}
}

// A drop snaps, saves the primary monitor as the empty name, saves both keys,
// tries the second key even when the first fails, and reports every failure.
func TestLampDrop(t *testing.T) {
	type save struct {
		key string
		v   any
	}
	var got []save
	ok := func(key string, v any) error { got = append(got, save{key, v}); return nil }

	name, pos, err := lampDrop(lampTestMons, 3500, 100, 400, ok)
	if err != nil || name != "DISPLAY3" || pos != "top-left" {
		t.Fatalf("drop on DISPLAY3: %q %q %v", name, pos, err)
	}
	if len(got) != 2 || got[0] != (save{"lamp_monitor", "DISPLAY3"}) || got[1] != (save{"lamp_position", "top-left"}) {
		t.Errorf("saves on DISPLAY3: %v", got)
	}

	got = nil
	name, pos, err = lampDrop(lampTestMons, 3300, 500, 300, ok)
	if err != nil || name != "" || pos != "top-right" {
		t.Fatalf("drop on the primary: %q %q %v", name, pos, err)
	}
	if len(got) != 2 || got[0] != (save{"lamp_monitor", ""}) || got[1] != (save{"lamp_position", "top-right"}) {
		t.Errorf("the primary must be saved as the empty name: %v", got)
	}

	got = nil
	failFirst := errors.New("first")
	fail := func(key string, v any) error {
		got = append(got, save{key, v})
		if key == "lamp_monitor" {
			return failFirst
		}
		return nil
	}
	_, _, err = lampDrop(lampTestMons, 100, 500, 300, fail)
	if !errors.Is(err, failFirst) || len(got) != 2 {
		t.Errorf("a failed first save: err %v, saves %v", err, got)
	}

	got = nil
	if name, pos, err = lampDrop(nil, 5, 5, 300, ok); err != nil || name != "" || pos != "top-center" || len(got) != 2 {
		t.Errorf("no monitor list: %q %q %v %v", name, pos, err, got)
	}
}

// The window starts clear whatever lamp_backdrop says, so a change of the
// setting needs no restart: card is the page's panel only.
func TestLampStartBackdrop(t *testing.T) {
	k, v := lampStartBackdrop()
	if k != "WEBVIEW2_DEFAULT_BACKGROUND_COLOR" || v != "00000000" {
		t.Errorf("lampStartBackdrop = %q %q", k, v)
	}
	src, err := os.ReadFile("lamp_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "lampBackdropEnv(set.Backdrop)") || !strings.Contains(string(src), "lampStartBackdrop()") {
		t.Error("the Windows window must start clear through lampStartBackdrop, not by the backdrop setting")
	}
}

// The pointer in the page's CSS pixels: relative to the window, divided by the
// window's scale, and it may be far outside the window.
func TestLampPointerCSS(t *testing.T) {
	x, y := lampPointerCSS(3150, 1200, 150, 0, 1.5)
	if x != 2000 || y != 800 {
		t.Errorf("pointer 3000 px right of the window = %v,%v, want 2000,800", x, y)
	}
	x, y = lampPointerCSS(-3000, -10, 100, 0, 0) // scale 0 acts as 1
	if x != -3100 || y != -10 {
		t.Errorf("negative pointer = %v,%v", x, y)
	}
}

// "Access denied" on the parent check counts as alive; a process that cannot be
// opened for any other reason is gone; an opened one is alive only while its
// exit code is 259 (STILL_ACTIVE).
func TestLampAliveFrom(t *testing.T) {
	cases := []struct {
		opened, denied bool
		code           uint32
		want           bool
	}{
		{false, true, 0, true},
		{false, false, 0, false},
		{true, false, 259, true},
		{true, false, 0, false},
		{true, false, 1, false},
		{true, true, 259, true},
	}
	for _, c := range cases {
		if got := lampAliveFrom(c.opened, c.denied, c.code); got != c.want {
			t.Errorf("lampAliveFrom(%v,%v,%d) = %v, want %v", c.opened, c.denied, c.code, got, c.want)
		}
	}
}

// Close: the ledger is written first, lamp_enabled false is saved only for a
// lamp BurnMon Dev started (BURNMON_LAMP_PARENT set), and the lamp ends last.
func TestLampCloseNow(t *testing.T) {
	for _, parentSet := range []bool{true, false} {
		var order []string
		save := func(key string, v any) error {
			order = append(order, "save:"+key+"="+strings.TrimSpace(fmt.Sprint(v)))
			return nil
		}
		lampCloseNow(func() { order = append(order, "ledger") }, parentSet, save, func() { order = append(order, "end") })
		want := "ledger,end"
		if parentSet {
			want = "ledger,save:lamp_enabled=false,end"
		}
		if got := strings.Join(order, ","); got != want {
			t.Errorf("parentSet %v: order %q, want %q", parentSet, got, want)
		}
	}
	// A failed save still ends the lamp.
	ended := false
	lampCloseNow(func() {}, true, func(string, any) error { return errors.New("disk") }, func() { ended = true })
	if !ended {
		t.Error("a failed save kept the lamp open")
	}
}

// A settings change reaches a running lamp within one tick: the engine reads
// the file on every payload, so the second payload shows what the file says
// now (criterion 3), for the size, the theme, the language and the monitor.
func TestEngineSettingsChangeShowsInTheNextPayload(t *testing.T) {
	t.Setenv(settingsDirEnv, "")
	now := time.Date(2026, 3, 2, 12, 3, 10, 0, time.UTC)
	e, setDir := liveFixture(t, now, 120, `{"lamp_size":100,"lamp_theme":"light","lamp_language":"en","lamp_monitor":"M2"}`)
	oldMons := lampMonitors
	lampMonitors = func() []lampMonitor {
		return []lampMonitor{{Name: "M1", Primary: true, W: 100, H: 100, Scale: 1}, {Name: "M2", X: 100, W: 100, H: 100, Scale: 1}}
	}
	defer func() { lampMonitors = oldMons }()
	p := e.payload(now)
	if p.Look.Size != 100 || p.Look.Theme != "light" || p.Look.Monitor != "M2" || p.CarryLines["drag"] != "Put me down." || p.CarryLines["view"] != "Nice view." {
		t.Fatalf("first payload look %+v carry %v", p.Look, p.CarryLines)
	}
	if err := os.WriteFile(filepath.Join(setDir, "burnmon-dev.json"), []byte(`{"lamp_size":200,"lamp_theme":"dark","lamp_language":"nl","lamp_monitor":"gone"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p = e.payload(now.Add(time.Second))
	if p.Look.Size != 200 || p.Look.Theme != "dark" || p.CarryLines["drag"] != "Zet me neer." {
		t.Errorf("second payload look %+v carry %v, want the new file", p.Look, p.CarryLines)
	}
	if p.Look.Monitor != "M1" {
		t.Errorf("a monitor that is gone gave %q, want the primary M1", p.Look.Monitor)
	}
}

// The coffee act shows Glim awake: the pose is working in any mood. Every other
// act and no act keep the mood.
func TestEnginePoseIsWorkingForTheCoffee(t *testing.T) {
	e := newLampEngine(true, "", "")
	now := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct{ act, mood, want string }{
		{lamp.ActCoffee, lamp.MoodAsleep, lamp.MoodWorking},
		{lamp.ActCoffee, lamp.MoodOff, lamp.MoodWorking},
		{"", lamp.MoodAsleep, lamp.MoodAsleep},
		{lamp.ActAlarm, lamp.MoodBlocked, lamp.MoodBlocked},
	} {
		e.act = c.act
		if p := e.finish(lamp.State{Mood: c.mood}, lamp.DefaultSettings(), now, "dark"); p.Pose != c.want {
			t.Errorf("act %q mood %q: pose %q, want %q", c.act, c.mood, p.Pose, c.want)
		}
	}
}

// Grab and swing: the line comes from the pure rule; the engine keeps the time
// of the last protest (60 s memory); calm and off do not swing.
func TestEngineSwingLinesAndTheSixtySecondMemory(t *testing.T) {
	e := newLampEngine(true, "", "")
	e.lastSet = lamp.Settings{Language: "en", Motion: "lively"}
	t0 := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		deg     float64
		changes int
		at      time.Duration
		line    string
		hard    bool
	}{
		{30, 0, 0, "That tickles.", false},
		{60, 0, 10 * time.Second, "Don't do that.", true},
		{59, 3, 69 * time.Second, "I said don't.", true},   // 59 s after the last protest
		{60, 0, 130 * time.Second, "Don't do that.", true}, // 61 s after the last
		{20, 1, 131 * time.Second, "That tickles.", false},
	} {
		got := e.swing(c.deg, c.changes, t0.Add(c.at))
		if got.Line != c.line || got.Hard != c.hard {
			t.Errorf("swing(%v,%d) at %v = %+v, want %q hard %v", c.deg, c.changes, c.at, got, c.line, c.hard)
		}
	}
	e.lastSet.Language = "nl"
	if got := e.swing(60, 0, t0.Add(500*time.Second)); got.Line != "Niet doen." {
		t.Errorf("Dutch protest = %q, want Niet doen.", got.Line)
	}
	for _, m := range []string{"calm", "off"} {
		e.lastSet.Motion = m
		if got := e.swing(60, 3, t0.Add(900*time.Second)); got.Line != "" || got.Hard {
			t.Errorf("motion %s swung: %+v", m, got)
		}
	}
}

// The pointer is read only while Glim is down and lamp_motion is lively.
func TestEngineEyesOnOnlyWhileDownAndLively(t *testing.T) {
	for _, c := range []struct {
		mood, motion, quiet string
		want                bool
	}{
		{lamp.MoodBlocked, "lively", "hide", true},
		{lamp.MoodWorking, "lively", "hide", false}, // quiet: Glim is up
		{lamp.MoodWorking, "lively", "stay", true},
		{lamp.MoodBlocked, "calm", "hide", false},
		{lamp.MoodBlocked, "off", "hide", false},
	} {
		e := newLampEngine(true, "", "")
		e.lastSt = lamp.State{Mood: c.mood}
		e.lastSet = lamp.Settings{Motion: c.motion, Quiet: c.quiet}
		if got := e.eyesOn(); got != c.want {
			t.Errorf("eyesOn %+v = %v, want %v", c, got, c.want)
		}
	}
}

// The pointer is read only to point the eyes. With a marker value behind the
// pointer seam, one live engine tick and the ledger close must leave no file
// that holds it. This covers the engine and the ledger only: that the window
// code passes the pointer on to the page and nowhere else is read from
// lamp_window_windows.go (lampCursor feeds lampPointerCSS and Eval, and the
// drag saves only a monitor name and a position).
func TestPointerReachesNoFile(t *testing.T) {
	dir := t.TempDir()
	const markerX, markerY = 987654, 876543
	oldCursor := lampCursor
	lampCursor = func() (int, int, bool) { return markerX, markerY, true }
	defer func() { lampCursor = oldCursor }()

	e := newLampEngine(false, dir, filepath.Join(dir, "snapshot.json"))
	e.ledger = lamp.LoadLedger(filepath.Join(dir, lamp.LedgerFile))
	if x, y, ok := lampCursor(); ok {
		cx, cy := lampPointerCSS(x, y, 0, 0, 1)
		_ = cx + cy
	}
	e.payload(time.Now())
	e.ledger.Close(time.Now())

	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, m := range []string{"987654", "876543"} {
			if bytes.Contains(b, []byte(m)) {
				t.Errorf("%s holds the pointer marker %s", p, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// L2: the Mac lamp lets clicks through outside the page's rectangles. This is
// the pure part: is a pointer point (window coordinates, origin top left)
// inside any rectangle. A rectangle is half open: left and top edge inside,
// right and bottom edge outside. No rectangle at all means the whole window
// takes clicks, as before.
func TestLampPointIn(t *testing.T) {
	r := []lampRect{{10, 20, 100, 50}}
	for _, c := range []struct {
		x, y int
		want bool
		name string
	}{
		{10, 20, true, "top left corner"},
		{109, 69, true, "last inside pixel"},
		{9, 30, false, "left of the left edge"},
		{50, 19, false, "above the top edge"},
		{110, 30, false, "on the right edge"},
		{50, 70, false, "on the bottom edge"},
		{50, 40, true, "middle"},
	} {
		if got := lampPointIn(r, c.x, c.y); got != c.want {
			t.Errorf("%s: lampPointIn(%d,%d) = %v, want %v", c.name, c.x, c.y, got, c.want)
		}
	}
	for _, empty := range [][]lampRect{nil, {}} {
		if !lampPointIn(empty, 500, 500) {
			t.Error("an empty list must let the whole window take clicks")
		}
	}
}

func TestLampPointInTickerAndGlim(t *testing.T) {
	area := `{"ticker":{"x":0,"y":0,"w":300,"h":30},"glim":{"x":100,"y":60,"w":90,"h":90}}`
	rects := lampRegionRects(area, 1)
	if len(rects) != 2 {
		t.Fatalf("rects = %v, want 2", rects)
	}
	if !lampPointIn(rects, 150, 10) {
		t.Error("a point on the ticker must take clicks")
	}
	if !lampPointIn(rects, 140, 100) {
		t.Error("a point on Glim's box must take clicks")
	}
	if lampPointIn(rects, 140, 45) {
		t.Error("a point in the gap between ticker and Glim must let clicks through")
	}
	if lampPointIn(rects, 250, 100) {
		t.Error("a point beside Glim's box must let clicks through")
	}
}
