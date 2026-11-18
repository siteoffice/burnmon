//go:build windows || (darwin && cgo)

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/lamp"
	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
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
	for _, bad := range [][]string{{"--nope"}, {"--poses"}, {"--demo", "--poses", "x"}, {"extra"}} {
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
