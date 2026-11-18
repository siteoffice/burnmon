//go:build windows || (darwin && cgo)

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	if moods[0] != lamp.MoodAsleep || moods[4] != lamp.MoodBored || moods[7] != lamp.MoodWorking {
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

func TestLampUnsupportedSaysTheSentenceAndExits2(t *testing.T) {
	var buf bytes.Buffer
	if code := lampUnsupported(&buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if got := strings.TrimSpace(buf.String()); got != "The lamp runs on Windows only in this version." {
		t.Fatalf("message %q", got)
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
	if p.Look.Rows != 1 || p.Look.Size != "medium" || p.Look.Theme != "system" || p.Look.Motion != "lively" || p.Look.Quiet != "hide" || p.Look.Backdrop != "clear" {
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
		size  string
		wantW int
		wantH int
	}{
		{1, "medium", 300, 26 + 180},
		{4, "medium", 300, 104 + 180},
		{1, "small", 300, 26 + 144},
		{2, "large", 300, 52 + 228},
		{0, "bad", 300, 26 + 180}, // bad values are the defaults
		{9, "medium", 300, 104 + 180},
	}
	for _, c := range cases {
		w, h := lampWindowSize(c.rows, c.size)
		if w != c.wantW || h != c.wantH {
			t.Errorf("lampWindowSize(%d,%q) = %dx%d, want %dx%d", c.rows, c.size, w, h, c.wantW, c.wantH)
		}
		if w < 120 || h < 6 {
			t.Errorf("lampWindowSize(%d,%q) = %dx%d does not cover the sliver", c.rows, c.size, w, h)
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

// The first-show focus try: on Windows the lamp child starts hidden so the
// window shows first through SetWindowPos without activation.
func TestLampChildAttrHidesOnWindows(t *testing.T) {
	a := lampChildAttr()
	if runtime.GOOS == "windows" && (a == nil || !a.HideWindow) {
		t.Errorf("lampChildAttr = %+v, want HideWindow on Windows", a)
	}
	if runtime.GOOS != "windows" && a != nil {
		t.Errorf("lampChildAttr = %+v, want nil off Windows", a)
	}
}
