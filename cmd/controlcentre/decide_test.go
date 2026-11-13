//go:build windows || darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"burnmon/internal/controls"
	"burnmon/internal/pricing"
	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

const gateFlowReview = "gate:example__flow__2:G2"

func TestGateTargetMatch(t *testing.T) {
	rd := snapshot.Load(exampleSnap, rigT0)
	if rd.Status != snapshot.StatusOK {
		t.Fatalf("example snapshot status %s", rd.Status)
	}
	items := rd.Snapshot.WaitItems
	got, err := gateTarget(items, "example", "flow-review")
	if err != nil || got != gateFlowReview {
		t.Fatalf("gateTarget = %q, %v", got, err)
	}
	if _, err := gateTarget(items, "example", "no-such-flow"); err == nil || err.Error() != "this gate is not in the BurnMon snapshot: start BurnMon Dev" {
		t.Errorf("unknown slug err = %v", err)
	}
	if _, err := gateTarget(items, "other", "flow-review"); err == nil {
		t.Error("a slug under another project must not match")
	}
	two := []waits.Item{
		{Kind: "gate", ID: "gate:a:G2", Title: "flow-review", Project: "example"},
		{Kind: "sprint", ID: "sprint:x", Title: "flow-review", Project: "example"},
		{Kind: "gate", ID: "gate:b:G2", Title: "flow-review", Project: "example"},
	}
	if _, err := gateTarget(two, "example", "flow-review"); err == nil || err.Error() != "more than one gate matches this flow" {
		t.Errorf("two matches err = %v", err)
	}
}

func checkDecision(t *testing.T, control string) {
	r := newDecisionRig(t)
	pr, err := previewDecision(r.be, DecisionRequest{Control: control, Project: "example", Slug: "flow-review"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Token == "" || pr.Command == "" {
		t.Fatalf("preview = %+v", pr)
	}
	if n := r.runner.calls(); n != 0 {
		t.Fatalf("preview ran the runner %d times", n)
	}
	r.clock.add(time.Second)
	rv, err := runDecision(r.be, pr.Token)
	if err != nil {
		t.Fatal(err)
	}
	if rv.ExitCode != 0 || rv.Refused != "" || rv.AuditPath != r.audit {
		t.Fatalf("run = %+v", rv)
	}
	if n := r.runner.calls(); n != 1 {
		t.Fatalf("runner ran %d times, want 1", n)
	}
	lines := r.auditLines()
	if len(lines) != 2 {
		t.Fatalf("audit lines = %v, want 2", lines)
	}
	if lines[0]["phase"] != "start" {
		t.Errorf("first line phase = %v", lines[0]["phase"])
	}
	if _, ok := lines[1]["phase"]; ok {
		t.Errorf("result line has a phase: %v", lines[1])
	}
	for _, l := range lines {
		if l["control"] != control || l["target"] != gateFlowReview {
			t.Errorf("audit line %v", l)
		}
	}
}

func TestApproveWritesStartAndResult(t *testing.T) { checkDecision(t, controls.Approve) }

func TestSendBackWritesStartAndResult(t *testing.T) { checkDecision(t, controls.SendBack) }

func TestCancelWritesNothing(t *testing.T) {
	r := newDecisionRig(t)
	if _, err := previewDecision(r.be, DecisionRequest{Control: controls.Approve, Project: "example", Slug: "flow-review"}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(r.audit); err == nil && len(b) > 0 {
		t.Errorf("audit file after preview only: %q", b)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if n := r.runner.calls(); n != 0 {
		t.Errorf("runner ran %d times", n)
	}
}

func TestExpiredTokenRefusedAndAudited(t *testing.T) {
	r := newDecisionRig(t)
	pr, err := previewDecision(r.be, DecisionRequest{Control: controls.Approve, Project: "example", Slug: "flow-review"})
	if err != nil {
		t.Fatal(err)
	}
	r.clock.add(6 * time.Second)
	rv, err := runDecision(r.be, pr.Token)
	if err == nil || rv.Refused == "" {
		t.Fatalf("expired token: run = %+v, err %v", rv, err)
	}
	if rv.AuditPath != r.audit {
		t.Errorf("AuditPath = %q", rv.AuditPath)
	}
	lines := r.auditLines()
	if len(lines) != 1 {
		t.Fatalf("audit lines = %v, want 1", lines)
	}
	if s, _ := lines[0]["refused"].(string); s == "" {
		t.Errorf("audit line has no refusal: %v", lines[0])
	}
	if n := r.runner.calls(); n != 0 {
		t.Errorf("runner ran %d times", n)
	}
}

// TestUnknownTokenRefused: internal\controls audits only refusals for tokens
// it issued (choices item 22), so this checks the refusal only.
func TestUnknownTokenRefused(t *testing.T) {
	r := newDecisionRig(t)
	rv, err := runDecision(r.be, "nope")
	if err == nil || rv.Refused == "" {
		t.Fatalf("unknown token: run = %+v, err %v", rv, err)
	}
	if n := r.runner.calls(); n != 0 {
		t.Errorf("runner ran %d times", n)
	}
}

func TestPreviewGateWithoutSnapshotItem(t *testing.T) {
	r := newDecisionRig(t)
	be := r.be
	be.snapPath = filepath.Join(t.TempDir(), "snapshot.json")
	_, err := previewDecision(be, DecisionRequest{Control: controls.Approve, Project: "example", Slug: "flow-review"})
	if err == nil || err.Error() != "BurnMon Dev is not running, so the gate cannot be checked" {
		t.Errorf("missing snapshot err = %v", err)
	}
	_, err = previewDecision(r.be, DecisionRequest{Control: controls.SendBack, Project: "example", Slug: "no-such-flow"})
	if err == nil || err.Error() != "this gate is not in the BurnMon snapshot: start BurnMon Dev" {
		t.Errorf("unknown slug err = %v", err)
	}
	if _, err := os.Stat(r.audit); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("audit file exists: %v", err)
	}
	if n := r.runner.calls(); n != 0 {
		t.Errorf("runner ran %d times", n)
	}
}

func TestPreviewRefusesBadRequests(t *testing.T) {
	r := newDecisionRig(t)
	for _, c := range []struct {
		req  DecisionRequest
		want string
	}{
		{DecisionRequest{Control: "stop", Project: "example", Slug: "flow-review"}, "stop needs an agent"},
		{DecisionRequest{Control: "attach", Agent: "ab12"}, "unknown control"},
		{DecisionRequest{Control: "close_sprint", Project: "example", Slug: "flow-review"}, "unknown control"},
		{DecisionRequest{Control: controls.Approve, Slug: "flow-review"}, "approve needs a project and a flow"},
		{DecisionRequest{Control: controls.SendBack, Project: "example"}, "send_back needs a project and a flow"},
	} {
		if _, err := previewDecision(r.be, c.req); err == nil || err.Error() != c.want {
			t.Errorf("%+v: err = %v, want %q", c.req, err, c.want)
		}
	}
	if r.runner.calls() != 0 || r.auditLines() != nil {
		t.Error("a refused request ran or audited")
	}
}

func TestDecisionsOffWithoutService(t *testing.T) {
	e := bindEnv{snapPath: exampleSnap, now: func() time.Time { return rigT0 }, svcWhy: "the BurnMon data folder cannot be found"}
	want := "decisions are off: the BurnMon data folder cannot be found"
	if _, err := previewDecision(e, DecisionRequest{Control: controls.Approve, Project: "example", Slug: "flow-review"}); err == nil || err.Error() != want {
		t.Errorf("preview err = %v", err)
	}
	if rv, err := runDecision(e, "nope"); err == nil || err.Error() != want || rv.Refused != want {
		t.Errorf("run = %+v, err %v", rv, err)
	}
}

func TestDecisionEnvMatchesBurnmonDev(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "burnmon-dev-view.json"), []byte(`{"controls":{"approve":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	foreman := t.TempDir()
	audit := filepath.Join(t.TempDir(), "controls-audit.ndjson")
	look := func(name string) (string, error) {
		if name == "python" {
			return `C:\example\python.exe`, nil
		}
		return "", errors.New("not found")
	}
	env := newDecisionEnv(pricing.Config{ForemanRoot: foreman, AgentView: true}, dataDir, exampleSnap, audit, look)
	if env.ForemanRoot != foreman || !env.AgentViewOn || env.AuditPath != audit {
		t.Errorf("env ForemanRoot %q AgentViewOn %v AuditPath %q", env.ForemanRoot, env.AgentViewOn, env.AuditPath)
	}
	if env.PythonPath != `C:\example\python.exe` || env.ClaudePath != "" {
		t.Errorf("PythonPath %q ClaudePath %q", env.PythonPath, env.ClaudePath)
	}
	if env.Snapshot == nil || env.Switches == nil || env.Self == nil || env.Elevated == nil ||
		env.Runner == nil || env.ReadForeman == nil || env.Merged == nil {
		t.Fatal("env leaves a function nil")
	}
	if sw := env.Switches(); !sw[controls.Approve] || sw[controls.SendBack] {
		t.Errorf("switches = %v", sw)
	}
}

func TestLoadBurnmonConfigOrder(t *testing.T) {
	write := func(dir, name, root string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"foreman_root":"`+root+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	exe, data := t.TempDir(), t.TempDir()
	write(exe, "burnmon.json", "exe-burnmon")
	write(exe, "claudecost.json", "exe-claudecost")
	write(data, "burnmon.json", "data-burnmon")
	if got := loadBurnmonConfig(exe, data).ForemanRoot; got != "exe-burnmon" {
		t.Errorf("exe burnmon.json: got %q", got)
	}
	exe2 := t.TempDir()
	write(exe2, "claudecost.json", "exe-claudecost")
	if got := loadBurnmonConfig(exe2, data).ForemanRoot; got != "exe-claudecost" {
		t.Errorf("exe claudecost.json before dataDir: got %q", got)
	}
	if got := loadBurnmonConfig(t.TempDir(), data).ForemanRoot; got != "data-burnmon" {
		t.Errorf("dataDir burnmon.json: got %q", got)
	}
	data2 := t.TempDir()
	write(data2, "claudecost.json", "data-claudecost")
	if got := loadBurnmonConfig("", data2).ForemanRoot; got != "data-claudecost" {
		t.Errorf("dataDir claudecost.json: got %q", got)
	}
	if got := loadBurnmonConfig("", t.TempDir()); got.ForemanRoot != "" || !strings.EqualFold(got.WSLScan, pricing.Defaults().WSLScan) {
		t.Errorf("no file: got %+v", got.ForemanRoot)
	}
}

func TestPreviewStaleSnapshotRefused(t *testing.T) {
	r := newDecisionRig(t)
	r.clock.add(time.Duration(snapshot.StaleAfterTicks*5+5) * time.Second)
	_, err := previewDecision(r.be, DecisionRequest{Control: controls.Approve, Project: "example", Slug: "flow-review"})
	if err == nil || err.Error() != "BurnMon Dev is not running, so the gate cannot be checked" {
		t.Errorf("stale snapshot err = %v", err)
	}
	if _, err := os.Stat(r.audit); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("audit file exists: %v", err)
	}
	if n := r.runner.calls(); n != 0 {
		t.Errorf("runner ran %d times", n)
	}
}

// TestRunBindingRefusalKeepsAuditPath: the ccRun handler turns a refusal
// into a view with a nil error, so the page draws the refusal and the audit
// file path (spec item 6).
func TestRunBindingRefusalKeepsAuditPath(t *testing.T) {
	r := newDecisionRig(t)
	pr, err := previewDecision(r.be, DecisionRequest{Control: controls.Approve, Project: "example", Slug: "flow-review"})
	if err != nil {
		t.Fatal(err)
	}
	r.clock.add(6 * time.Second)
	rv, err := runBinding(r.be, pr.Token)
	if err != nil || rv.Refused == "" || rv.AuditPath != r.audit {
		t.Fatalf("refused run: view %+v, err %v", rv, err)
	}
	if len(r.auditLines()) != 1 || r.runner.calls() != 0 {
		t.Error("refused run must write one audit line and run nothing")
	}
	off := bindEnv{svcWhy: "x"}
	if rv, err := runBinding(off, "nope"); err != nil || rv.Refused == "" || rv.AuditPath != "" {
		t.Errorf("decisions off: view %+v, err %v", rv, err)
	}
}

// TestElevatedReasonRewritten: the elevation refusal from internal/controls
// names BurnMon Dev; the Control Centre shows its own name (spec item 3).
func TestElevatedReasonRewritten(t *testing.T) {
	r := newDecisionRig(t)
	r.env.Elevated = func() bool { return true }
	r.svc = controls.NewService(r.env)
	r.be.svc = r.svc
	_, err := previewDecision(r.be, DecisionRequest{Control: controls.Stop, Agent: "ab12"})
	if err == nil || err.Error() != ccElevatedReason {
		t.Fatalf("elevated preview err = %v, want %q", err, ccElevatedReason)
	}
	if got := "Not possible: " + err.Error(); got != "Not possible: the Control Centre runs as administrator, so controls are off. Start it from a normal window." {
		t.Errorf("pane text = %q", got)
	}
	if n := r.runner.calls(); n != 0 {
		t.Errorf("runner ran %d times", n)
	}
}

func TestElevatedReasonRewrittenOnRun(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the fixture cwd is a Windows path; the folder checks parse it only on Windows")
	}
	r := newDecisionRig(t)
	elevated := false
	r.env.Elevated = func() bool { return elevated }
	r.svc = controls.NewService(r.env)
	r.be.svc = r.svc
	view, _ := json.Marshal([]map[string]any{{"kind": "background", "id": "ab12", "cwd": `C:\example\other`,
		"state": "blocked", "status": "waiting", "pid": 4242, "startedAt": 1761213600000}})
	r.svc.Env.AgentView = func(ctx context.Context) ([]byte, error) { return view, nil }
	pr, err := previewDecision(r.be, DecisionRequest{Control: controls.Stop, Agent: "ab12"})
	if err != nil {
		t.Fatal(err)
	}
	elevated = true
	r.clock.add(time.Second)
	rv, err := runDecision(r.be, pr.Token)
	if rv.Refused != ccElevatedReason {
		t.Errorf("Refused = %q, want %q", rv.Refused, ccElevatedReason)
	}
	if err == nil || err.Error() != ccElevatedReason {
		t.Errorf("err = %v, want %q", err, ccElevatedReason)
	}
	if n := r.runner.calls(); n != 0 {
		t.Errorf("runner ran %d times", n)
	}
}

func TestOtherReasonsPassThrough(t *testing.T) {
	for _, s := range []string{
		"",
		"switched off in the gear",
		"needs claude.exe: set claude_path in burnmon.json",
		"cannot tell whether BurnMon Dev is elevated: controls are off",
		controlsElevatedReason + " ",
		"x " + controlsElevatedReason,
	} {
		if got := rewriteReason(s); got != s {
			t.Errorf("rewriteReason(%q) = %q", s, got)
		}
	}
	r := newDecisionRig(t)
	r.env.Self = nil
	r.svc = controls.NewService(r.env)
	r.be.svc = r.svc
	want := "cannot check BurnMon Dev's own process tree: controls are off"
	if _, err := previewDecision(r.be, DecisionRequest{Control: controls.Stop, Agent: "ab12"}); err == nil || err.Error() != want {
		t.Errorf("no Self: err = %v, want %q", err, want)
	}
}
