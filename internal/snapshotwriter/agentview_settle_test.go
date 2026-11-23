package snapshotwriter_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"burnmon/internal/cloudpull"
	"burnmon/internal/waits"
)

const idleEntry = `[{"cwd":"C:\\example\\c","kind":"interactive","startedAt":1761213600000,"pid":9,"status":"idle","sessionId":"s9"}]`
const busyEntry = `[{"cwd":"C:\\example\\c","kind":"interactive","startedAt":1761213600000,"pid":9,"status":"busy","sessionId":"s9"}]`

// agentViewTurns counts the your-turn items that came from the agent view (the
// fake graph makes its own your-turn item, which is not what these tests probe).
func agentViewTurns(items []waits.Item) int {
	n := 0
	for _, it := range items {
		if it.Kind == waits.KindYourTurn && it.Source == "agent_view" {
			n++
		}
	}
	return n
}

// Between two prompts of a session, and while its UserPromptSubmit hook runs,
// the CLI can report idle for a moment (Wilco, 2026-11-23: the lamp said "1 your
// turn" while the session ran a hook and Herdr showed it Working). An idle entry
// counts as your turn only after a second poll that also saw it idle; one busy
// poll in between starts the count again. This is the only place the rule lives,
// so the lamp, the Waits on you panel and the toasts all follow it.
func TestAgentViewIdleIsYourTurnOnlyAfterTwoPolls(t *testing.T) {
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	out := idleEntry
	gr.AgentView = func(ctx context.Context) ([]byte, error) { return []byte(out), nil }
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr)
	yourTurn := func() bool {
		gr.PollAgentViewNow(context.Background())
		return agentViewTurns(onceRead(t, w).WaitItems) > 0
	}
	if yourTurn() {
		t.Fatal("a first idle poll already counted as your turn")
	}
	if !yourTurn() {
		t.Fatal("a second idle poll did not count as your turn")
	}
	out = busyEntry
	if yourTurn() {
		t.Fatal("a busy entry counted as your turn")
	}
	out = idleEntry
	if yourTurn() {
		t.Fatal("idle straight after busy counted at once; the count must start again")
	}
	if !yourTurn() {
		t.Fatal("idle on two polls in a row did not count")
	}
}

// BurnMon's own pull starts claude in the pull folder. Those runs are not
// sessions of the person: they never show in the agent sessions or the waits.
func TestAgentViewDropsThePullsOwnRuns(t *testing.T) {
	cwd, _ := json.Marshal(cloudpull.PullDir())
	pull := `{"cwd":` + string(cwd) + `,"kind":"interactive","startedAt":1761213600000,"pid":11,"status":"idle","sessionId":"sp"}`
	st := snapshotTestStore(t)
	gr, _ := fakeReaders(t, `{"pid":4100,"sessionId":"s2","procStart":"777"}`)
	gr.IgnoreDir = cloudpull.PullDir() // what internal\ingest sets for the app
	gr.AgentView = func(ctx context.Context) ([]byte, error) { return []byte("[" + pull + "," + idleEntry[1:]), nil }
	w := newWriter(st, filepath.Join(t.TempDir(), "snapshot.json"), noProcesses, gr)
	gr.PollAgentViewNow(context.Background())
	gr.PollAgentViewNow(context.Background())
	got := onceRead(t, w)
	if len(got.AgentSessions) != 1 || got.AgentSessions[0].PID != 9 {
		t.Fatalf("agent sessions = %+v, want only the person's session", got.AgentSessions)
	}
	if n := agentViewTurns(got.WaitItems); n != 1 {
		t.Fatalf("your turn items = %d, want 1 (the pull's run must not count)", n)
	}
}
