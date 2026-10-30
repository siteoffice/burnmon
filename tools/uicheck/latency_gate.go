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
	"syscall"
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
)

// latencySettle lets the "running" write's own burst and snapshot write end
// before t0, so each round starts from a quiet watcher, the way a sprint that
// ran for a while turns waiting-on-you. A var so tests can drop it.
var latencySettle = time.Second

// probeRemoveRetry is how long removeProbeDir keeps trying one entry. A Go
// reader of sprint.json (BurnMon's own Foreman read) shares read and write but
// not delete, so a delete fails for the few milliseconds it reads.
var probeRemoveRetry = 2 * time.Second

// latencySignals run the probe cleanup before the process ends: Ctrl+C, and
// closing the console window, which Go reports as SIGTERM on Windows.
var latencySignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// probeMarkerWrite writes the marker file; tests swap it to fail.
var probeMarkerWrite = func(path string) error {
	return os.WriteFile(path, []byte(latencyMarkerText), 0o644)
}

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
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", err
	}
	if err := probeMarkerWrite(filepath.Join(dir, latencyMarker)); err != nil {
		// The folder was made just now and holds nothing of anyone else's:
		// remove it, or every later run would refuse it as unmarked.
		return "", errors.Join(err, os.RemoveAll(dir))
	}
	return dir, nil
}

// removeProbeDir removes the probe folder, and only when the marker says it
// is the harness's own. Every other entry goes first and the marker last, so
// a file that will not go (a reader holds it) leaves the folder marked, the
// error says so, and a later run takes the folder over. os.RemoveAll on the
// folder would carry on past that file and take the marker with it.
func removeProbeDir(dir string) error {
	if !hasProbeMarker(dir) {
		return fmt.Errorf("%s has no %s; not removed", dir, latencyMarker)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == latencyMarker {
			continue
		}
		if err := removeRetry(os.RemoveAll, filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("%s stays, with its marker: %w", dir, err)
		}
	}
	if err := removeRetry(os.Remove, filepath.Join(dir, latencyMarker)); err != nil {
		return err
	}
	return removeRetry(os.Remove, dir)
}

// removeRetry calls remove on path until it succeeds or probeRemoveRetry has
// passed, and returns the last error.
func removeRetry(remove func(string) error, path string) error {
	deadline := time.Now().Add(probeRemoveRetry)
	for {
		err := remove(path)
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
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

// latencyProbe is the claimed probe folder. mu orders the sprint.json writes
// and the removal, so a Ctrl+C cleanup never runs during a write (a file the
// harness itself holds open will not delete), and once the removal has run
// no write lands in the folder again.
type latencyProbe struct {
	dir     string
	mu      sync.Mutex
	removed bool
	err     error
}

func (p *latencyProbe) write(status string, at time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.removed {
		return errors.New("the probe was already removed")
	}
	return writeProbeSprint(p.dir, status, at)
}

// remove removes the probe once; a later call returns the first result.
func (p *latencyProbe) remove() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.removed {
		p.removed = true
		p.err = removeProbeDir(p.dir)
	}
	return p.err
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

// latencyReadFn reads the page once; tests swap it for a fake page.
var latencyReadFn = func() (latencyRead, error) {
	var r latencyRead
	err := evalInto(latencyRowJS(), &r)
	return r, err
}

// pageCanShow says why the page cannot show the row in time, nil when it can,
// so such a page fails at once instead of after the timeout.
func pageCanShow(r latencyRead) error {
	switch {
	case !r.Stamp:
		return errors.New("this burnmon-dev.exe has no bdevSnapshotStamp; rebuild with .\\build.ps1")
	case r.Hidden:
		return errors.New("the page is hidden (window minimized?), so its 1 s stamp check is off")
	case !r.Sec:
		return errors.New("the page has no Waits part")
	}
	return nil
}

// waitRow polls the DOM every latencyPoll until the probe's row is there
// (want true) or gone, and returns when it saw that.
func waitRow(want bool, timeout time.Duration) (time.Time, error) {
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(latencyPoll) {
		r, err := latencyReadFn()
		if err != nil {
			return time.Time{}, err
		}
		at := time.Now()
		if err := pageCanShow(r); err != nil {
			return time.Time{}, err
		}
		if r.Row == want {
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
	return latencyGateIn(dir, latencyRounds)
}

// latencyGateIn runs the rounds on the claimed probe folder dir and removes
// the probe at the end, also when a signal stops the run. A probe that could
// not be removed fails the gate, whatever the rounds gave: it is a fake
// Blocked sprint left in the real Foreman tree.
func latencyGateIn(dir string, rounds int) (err error) {
	p := &latencyProbe{dir: dir}
	defer func() {
		if rerr := p.remove(); rerr != nil {
			err = errors.Join(err, fmt.Errorf("probe cleanup failed; delete %s by hand once nothing holds it: %w", dir, rerr))
		}
	}()
	// A signal skips the defer and would leave a fake Blocked sprint in the
	// real Foreman tree.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, latencySignals...)
	defer signal.Stop(sig)
	go func() {
		s := <-sig
		if rerr := p.remove(); rerr != nil {
			fmt.Printf("uicheck: latency/gate stopped by %v; probe cleanup failed, delete %s by hand: %v\n", s, dir, rerr)
		}
		os.Exit(1)
	}()
	fmt.Printf("uicheck: latency/gate probe %s\n", dir)
	ms, err := runLatencyRounds(p, rounds)
	if err != nil {
		return err
	}
	return latencyVerdict(ms)
}

// runLatencyRounds times rounds rounds on p, in ms each.
func runLatencyRounds(p *latencyProbe, rounds int) ([]int64, error) {
	var ms []int64
	for i := 1; i <= rounds; i++ {
		if err := p.write("running", time.Now()); err != nil {
			return ms, fmt.Errorf("round %d: %w", i, err)
		}
		if _, err := waitRow(false, latencyWait); err != nil {
			return ms, fmt.Errorf("round %d: %w", i, err)
		}
		time.Sleep(latencySettle)
		// A row back on the page now is a late snapshot of the last round,
		// not this round's write: timing it would make the round falsely fast.
		r, err := latencyReadFn()
		if err == nil {
			err = pageCanShow(r)
		}
		if err == nil && r.Row {
			err = errors.New("the row is on the page before t0 (a late snapshot); the round would be falsely fast")
		}
		if err != nil {
			return ms, fmt.Errorf("round %d: %w", i, err)
		}
		// t0 is taken before the write, so the time counts the write too.
		t0 := time.Now()
		if err := p.write("waiting-on-you", t0); err != nil {
			return ms, fmt.Errorf("round %d: %w", i, err)
		}
		t1, err := waitRow(true, latencyWait)
		if err != nil {
			return ms, fmt.Errorf("round %d: %w", i, err)
		}
		d := t1.Sub(t0).Milliseconds()
		ms = append(ms, d)
		fmt.Printf("uicheck: latency/gate round %d: %d ms\n", i, d)
	}
	return ms, nil
}

// latencyVerdict prints p95 and max and fails on any round at the limit or over.
func latencyVerdict(ms []int64) error {
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
