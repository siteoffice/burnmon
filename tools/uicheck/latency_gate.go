package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// latency/gate (B8 item 2): how long a sprint that turns waiting-on-you takes
// to show as a row in BurnMon Dev's Waits list, over the live path only: the
// Foreman watch, the snapshot write and the page's 1 s stamp check. It never
// calls __bdevSections.apply, which freezes the page's polling; it reads the
// DOM and nothing else.
//
// The probe sprint goes into the real Foreman root (foreman_root in
// %LOCALAPPDATA%\burnmon\burnmon.json, read only): a temp root would skip the
// real Foreman read (orchestrator Ruling 3). Its folder carries a marker file,
// and a folder without the marker is never written or removed. It runs only
// by its full name: .\scripts\uicheck.ps1 latency/gate

const (
	latencyProject = "burnmon"
	// latencySlug is the probe folder's name. graph.ReadForeman skips a
	// folder whose name starts with "_", so this one must not.
	latencySlug     = "b8-gate-latency-probe"
	latencySprintID = "burnmon__b8-gate-latency-probe__probe"
	latencyMarker   = "probe-by-uicheck.txt"
	latencyRounds   = 10
	latencyLimitMs  = 2000
	latencyWait     = 20 * time.Second
	latencyPoll     = 50 * time.Millisecond
	// latencySettle lets the "running" write's own burst and snapshot write
	// end before t0, so each round starts from a quiet watcher, the way a
	// sprint that ran for a while turns waiting-on-you.
	latencySettle = time.Second
)

const latencyMarkerText = "Made by the BurnMon uicheck case latency/gate (tools\\uicheck\\latency_gate.go).\r\n" +
	"It removes this folder when it ends. If a run was killed, delete the folder.\r\n"

// latencyStats is the nearest-rank p95 (the ceil(0.95 n)-th smallest value)
// and the max of ms, 0 and 0 for none. ms is not reordered.
func latencyStats(ms []int64) (p95, max int64) {
	if len(ms) == 0 {
		return 0, 0
	}
	s := append([]int64(nil), ms...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	rank := (95*len(s) + 99) / 100
	return s[rank-1], s[len(s)-1]
}

// latencyConfigPath is the burnmon.json BurnMon Dev falls back to; the
// harness only reads it.
func latencyConfigPath() (string, error) {
	lad := os.Getenv("LOCALAPPDATA")
	if lad == "" {
		return "", errors.New("LOCALAPPDATA not set")
	}
	return filepath.Join(lad, "burnmon", "burnmon.json"), nil
}

// readForemanRoot reads foreman_root from the config at path; an empty one is
// an error, since without it the page shows no Foreman items at all.
func readForemanRoot(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var c struct {
		ForemanRoot string `json:"foreman_root"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if c.ForemanRoot == "" {
		return "", fmt.Errorf("%s has no foreman_root; latency/gate needs the real Foreman root", path)
	}
	return c.ForemanRoot, nil
}

func latencyProbeDir(root string) string {
	return filepath.Join(root, "sprints", latencyProject, latencySlug)
}

func hasProbeMarker(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, latencyMarker))
	return err == nil && fi.Mode().IsRegular()
}

// claimProbeDir makes the probe folder under root\sprints, the marker first,
// and returns it. A folder an interrupted run left (marker in it) is taken
// again; one without the marker is not ours and is left untouched.
func claimProbeDir(root string) (string, error) {
	sprints := filepath.Join(root, "sprints")
	if fi, err := os.Stat(sprints); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s is not a folder; is foreman_root right?", sprints)
	}
	dir := latencyProbeDir(root)
	_, err := os.Stat(dir)
	switch {
	case err == nil && hasProbeMarker(dir):
		fmt.Printf("uicheck: latency/gate takes over %s, left by an earlier run\n", dir)
		return dir, nil
	case err == nil:
		return "", fmt.Errorf("%s exists without %s: not this harness's folder, left untouched", dir, latencyMarker)
	case !errors.Is(err, fs.ErrNotExist):
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, latencyMarker), []byte(latencyMarkerText), 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

// removeProbeDir removes the probe folder, and only when the marker says it
// is the harness's own.
func removeProbeDir(dir string) error {
	if !hasProbeMarker(dir) {
		return fmt.Errorf("%s has no %s; not removed", dir, latencyMarker)
	}
	return os.RemoveAll(dir)
}

// probeSprintJSON is the probe's sprint.json. Status waiting-on-you is all it
// takes: graph.ForemanBlockers (internal\graph\blockers.go) makes a sprint a
// Blocker when its status is waiting-on-you and it is not close-out or branch
// gone, and waits.Build turns that Blocker into the Blocked item
// sprint:<sprint_id>. Close-out and branch gone both need a worktree branch,
// which the probe has none of, and no action-log.ndjson is needed (it only
// adds the parked detail). Status running gives no item. The shape is
// testdata\latency_probe_sprint.json, proven in
// internal\snapshotwriter\latency_probe_test.go.
func probeSprintJSON(status string, at time.Time) []byte {
	b, _ := json.MarshalIndent(struct {
		SprintID  string `json:"sprint_id"`
		Project   string `json:"project"`
		Slug      string `json:"slug"`
		Phase     string `json:"phase"`
		Status    string `json:"status"`
		UpdatedAt string `json:"updated_at"`
	}{latencySprintID, latencyProject, latencySlug, "dev", status, at.UTC().Format(time.RFC3339)}, "", "  ")
	return append(b, '\n')
}

func writeProbeSprint(dir, status string, at time.Time) error {
	return os.WriteFile(filepath.Join(dir, "sprint.json"), probeSprintJSON(status, at), 0o644)
}

// latencyRead is one DOM read: the Waits part, the probe's row, whether the
// page is hidden (that stops the 1 s stamp check) and whether this build has
// the stamp binding at all.
type latencyRead struct {
	Sec    bool `json:"sec"`
	Row    bool `json:"row"`
	Hidden bool `json:"hidden"`
	Stamp  bool `json:"stamp"`
}

func latencyRowJS() string {
	sel := `.secitem[data-id="sprint:` + latencySprintID + `"]`
	return `(function(){
  var sec = document.querySelector('[data-sec=waits]');
  return {sec: !!sec, row: !!(sec && sec.querySelector(` + jsString(sel) + `)), hidden: !!document.hidden,
    stamp: typeof window.bdevSnapshotStamp === 'function'};
})()`
}

// waitRow polls the DOM every latencyPoll until the probe's row is there
// (want true) or gone, and returns when it saw that. A page that cannot show
// the row in time fails at once instead of after the timeout.
func waitRow(want bool, timeout time.Duration) (time.Time, error) {
	js := latencyRowJS()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(latencyPoll) {
		var r latencyRead
		if err := evalInto(js, &r); err != nil {
			return time.Time{}, err
		}
		at := time.Now()
		switch {
		case !r.Stamp:
			return time.Time{}, errors.New("this burnmon-dev.exe has no bdevSnapshotStamp; rebuild with .\\build.ps1")
		case r.Hidden:
			return time.Time{}, errors.New("the page is hidden (window minimized?), so its 1 s stamp check is off")
		case !r.Sec:
			return time.Time{}, errors.New("the page has no Waits part")
		case r.Row == want:
			return at, nil
		}
	}
	word := "show"
	if !want {
		word = "go"
	}
	return time.Time{}, fmt.Errorf("the row sprint:%s did not %s in %v", latencySprintID, word, timeout)
}

func caseLatencyGate(hwnd uintptr, args []string) error {
	cfg, err := latencyConfigPath()
	if err != nil {
		return err
	}
	root, err := readForemanRoot(cfg)
	if err != nil {
		return err
	}
	dir, err := claimProbeDir(root)
	if err != nil {
		return err
	}
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			if err := removeProbeDir(dir); err != nil {
				fmt.Printf("uicheck: latency/gate could not remove the probe: %v\n", err)
			}
		})
	}
	defer cleanup()
	// Ctrl+C skips the defer and would leave a fake Blocked sprint in the
	// real Foreman tree.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		<-sig
		cleanup()
		os.Exit(1)
	}()
	fmt.Printf("uicheck: latency/gate probe %s\n", dir)

	var ms []int64
	for i := 1; i <= latencyRounds; i++ {
		if err := writeProbeSprint(dir, "running", time.Now()); err != nil {
			return fmt.Errorf("round %d: %w", i, err)
		}
		if _, err := waitRow(false, latencyWait); err != nil {
			return fmt.Errorf("round %d: %w", i, err)
		}
		time.Sleep(latencySettle)
		// t0 is taken before the write, so the time counts the write too.
		t0 := time.Now()
		if err := writeProbeSprint(dir, "waiting-on-you", t0); err != nil {
			return fmt.Errorf("round %d: %w", i, err)
		}
		t1, err := waitRow(true, latencyWait)
		if err != nil {
			return fmt.Errorf("round %d: %w", i, err)
		}
		d := t1.Sub(t0).Milliseconds()
		ms = append(ms, d)
		fmt.Printf("uicheck: latency/gate round %d: %d ms\n", i, d)
	}
	p95, max := latencyStats(ms)
	fmt.Printf("uicheck: latency/gate %d rounds: p95 %d ms, max %d ms (limit: under %d ms)\n", len(ms), p95, max, latencyLimitMs)
	var slow []string
	for i, d := range ms {
		if d >= latencyLimitMs {
			slow = append(slow, fmt.Sprintf("round %d %d ms", i+1, d))
		}
	}
	if len(slow) > 0 {
		return fmt.Errorf("%s: at or over %d ms", strings.Join(slow, ", "), latencyLimitMs)
	}
	return nil
}
