package controls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burnmon/internal/snapshot"
)

// staleSnap makes the rig's snapshot read as the given status.
func (r *rig) staleSnap(status string) {
	r.env.Snapshot = func() snapshot.Reading { return snapshot.Reading{Status: status} }
}

const refuseStale = "the snapshot is not current, nothing to control"

// gate_refuse_without_snapshot
func TestGateRefuseWithoutSnapshot(t *testing.T) {
	for _, set := range []string{"", GateRefuse} {
		r := newRig(t)
		r.env.GateWithoutSnapshot = set
		r.staleSnap(snapshot.StatusStale)
		r.rebuild()
		if _, err := r.preview(Approve, r.gateTarget()); err == nil || err.Error() != refuseStale {
			t.Errorf("setting %q: err = %v", set, err)
		}
	}
}

// gate_foreman_read_without_snapshot
func TestGateForemanReadWithoutSnapshot(t *testing.T) {
	for _, st := range []string{snapshot.StatusMissing, snapshot.StatusStale, snapshot.StatusUnreadable} {
		r := newRig(t)
		r.env.GateWithoutSnapshot = GateForemanRead
		r.staleSnap(st)
		r.rebuild()
		lib := filepath.Join(r.root, "flows", "_lib")
		dir := filepath.Join(r.root, "flows", "example", "flow-review")
		p, err := r.preview(Approve, r.gateTarget())
		if err != nil {
			t.Fatalf("%s: %v", st, err)
		}
		want := GateArgv(svcPython, lib, dir, "approve", "G2", "t1")
		if p.Command == "" || len(r.runner.runs) != 0 {
			t.Fatalf("%s: %+v", st, p)
		}
		res, err := r.confirm(p.Token)
		if err != nil || res.Failed {
			t.Fatalf("%s: %v %+v", st, err, res)
		}
		if strings.Join(r.runner.runs[0], "\x00") != strings.Join(want, "\x00") {
			t.Errorf("argv = %v", r.runner.runs[0])
		}
		ls := r.auditLines()
		if len(ls) != 2 {
			t.Fatalf("lines = %v", ls)
		}
		for _, l := range ls {
			if l["via"] != "foreman_read" {
				t.Errorf("line without via: %v", l)
			}
			if c, _ := l["command"].(string); strings.Contains(c, "t1") && !strings.Contains(c, "<gate token>") {
				t.Errorf("token in audit command: %q", c)
			}
		}
	}
}

// gate_foreman_read_still_checks
func TestGateForemanReadStillChecks(t *testing.T) {
	cases := []struct {
		name string
		mut  func(r *rig)
		want string
	}{
		{"not pending", func(r *rig) { r.flows[0].GatePending = false }, "the gate is not open for a decision"},
		{"other gate", func(r *rig) { r.flows[0].Gate = "G3" }, "the gate is not open for a decision"},
		{"empty token", func(r *rig) { r.flows[0].GateToken = "" }, "the gate is not open for a decision"},
		{"duplicate flow", func(r *rig) {
			r.flows = append(r.flows, r.flows[0])
		}, errDupFlow},
		{"outside flows", func(r *rig) { r.flows[0].Dir = filepath.Join(os.TempDir(), "elsewhere", "x") }, "the flow folder is outside foreman_root\\flows"},
		{"flow gone", func(r *rig) { r.flows = nil }, "the flow is no longer listed"},
	}
	for _, c := range cases {
		r := newRig(t)
		r.env.GateWithoutSnapshot = GateForemanRead
		r.staleSnap(snapshot.StatusMissing)
		r.rebuild()
		c.mut(r)
		_, err := r.preview(Approve, r.gateTarget())
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
	// A gate id that is not valid text still refuses.
	r := newRig(t)
	r.env.GateWithoutSnapshot = GateForemanRead
	r.staleSnap(snapshot.StatusMissing)
	r.rebuild()
	if _, err := r.preview(Approve, "gate:nocolon"); err == nil {
		t.Error("bad gate id accepted")
	}
	// An accept gate has no approve, as on the snapshot path.
	if _, err := r.preview(Approve, "gate:example__flow__2:G4"); err == nil || !strings.Contains(err.Error(), "no approve for gate G4") {
		t.Errorf("accept gate: %v", err)
	}
}

// foreman_read_never_overrules_snapshot
func TestForemanReadNeverOverrulesSnapshot(t *testing.T) {
	r := newRig(t)
	r.env.GateWithoutSnapshot = GateForemanRead
	r.snap = func(s *snapshot.Snapshot) {
		s.WaitItems = nil
	}
	r.rebuild()
	if _, err := r.preview(Approve, r.gateTarget()); err == nil || err.Error() != "the gate is not a listed wait item" {
		t.Errorf("err = %v", err)
	}
}

// foreman_read_gates_only
func TestForemanReadGatesOnly(t *testing.T) {
	// The close rig has the sprint script and a good sprint, so Close passes its
	// own needs and only the stale snapshot can refuse it.
	c := newCloseRig(t)
	c.env.GateWithoutSnapshot = GateForemanRead
	c.staleSnap(snapshot.StatusStale)
	c.rebuild()
	for _, ctl := range []struct{ name, target string }{{Stop, "ab12"}, {Attach, "ab12"}, {CloseSprint, closeItemID}} {
		_, err := c.preview(ctl.name, ctl.target)
		if err == nil || err.Error() != refuseStale {
			t.Errorf("%s: err = %v, want %q", ctl.name, err, refuseStale)
		}
	}
}

// apps_keep_refuse and audit_via_omitted
func TestAuditViaOmittedOnSnapshotPath(t *testing.T) {
	r := newRig(t)
	p, err := r.preview(Approve, r.gateTarget())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.confirm(p.Token); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.audit)
	if strings.Contains(string(b), `"via"`) {
		t.Errorf("snapshot path wrote a via key: %s", b)
	}
}
