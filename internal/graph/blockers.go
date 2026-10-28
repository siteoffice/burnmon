package graph

// Blocker kinds.
const (
	BlockerGate   = "gate"
	BlockerSprint = "sprint"
	BlockerCheck  = "failed_check"
)

// Blocker is one Foreman thing that waits on the person. NodeID is the graph
// node that carries its waits_on edge.
type Blocker struct {
	Kind     string // BlockerGate, BlockerSprint or BlockerCheck
	NodeID   string // gate:<flow>:<gate>, sprint:<id> or check:<sprint>:<test|verify>
	FlowID   string // gate only
	SprintID string // sprint and check
	Check    string // check only: "test" or "verify"
}

// SprintStopped reports whether a sprint status means the loop gave up.
func SprintStopped(status string) bool {
	return status == "capped" || status == "failed" || status == "escalated"
}

// stoppedWithFailedCheck reports whether a sprint gives a check Blocker.
func stoppedWithFailedCheck(sp Sprint) bool {
	return SprintStopped(sp.Status) && ((sp.Test != nil && sp.Test.Failed) || (sp.Verify != nil && sp.Verify.Failed))
}

// ForemanBlockers is the one decision: which Foreman things wait on the
// person. The map draws a box and a line for each, and the wait list counts
// each. closeout and gone are sprint node ids (sprint:<id>) that Build marked
// closeout or branch_gone. Order: gates in flow order, then sprints in input
// order, each sprint before its failed checks.
func ForemanBlockers(sprints []Sprint, flows []Flow, closeout, gone map[string]bool) []Blocker {
	out := []Blocker{}
	for _, f := range flows {
		if f.GatePending {
			out = append(out, Blocker{Kind: BlockerGate, NodeID: "gate:" + f.ID + ":" + f.Gate, FlowID: f.ID})
		}
	}
	for _, s := range sprints {
		sid := "sprint:" + s.ID
		if s.Status == "waiting-on-you" && !closeout[sid] && !gone[sid] {
			out = append(out, Blocker{Kind: BlockerSprint, NodeID: sid, SprintID: s.ID})
		}
		if !SprintStopped(s.Status) {
			continue
		}
		for _, c := range []struct {
			name string
			chk  *Check
		}{{"test", s.Test}, {"verify", s.Verify}} {
			if c.chk != nil && c.chk.Failed {
				out = append(out, Blocker{Kind: BlockerCheck, NodeID: "check:" + s.ID + ":" + c.name, SprintID: s.ID, Check: c.name})
			}
		}
	}
	return out
}
