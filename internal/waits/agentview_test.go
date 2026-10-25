package waits

import "testing"

const twoEntries = `[{"cwd":"C:\\example\\a","kind":"background","startedAt":1761213600000,"id":"ab12","state":"blocked","pid":42,"status":"waiting","waitingFor":"permission prompt","sessionId":"s1","name":"ignored"},{"cwd":"C:\\example\\b","kind":"interactive","startedAt":1761213600000,"pid":7,"status":"busy"}]`

func TestParseAgentView(t *testing.T) {
	es, err := ParseAgentView([]byte(twoEntries))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 {
		t.Fatalf("entries = %+v", es)
	}
	a, b := es[0], es[1]
	if a.Cwd != `C:\example\a` || a.Kind != "background" || a.ID != "ab12" || a.State != "blocked" ||
		a.PID != 42 || a.Status != "waiting" || a.WaitingFor != "permission prompt" ||
		a.SessionID != "s1" || a.StartedAt != 1761213600000 {
		t.Fatalf("a = %+v", a)
	}
	if b.PID != 7 || b.Status != "busy" || b.SessionID != "" || b.ID != "" {
		t.Fatalf("b = %+v", b)
	}
	if a.Key() != "session:anthropic:s1" || b.Key() != "agentview:pid:7" {
		t.Fatalf("keys = %q %q", a.Key(), b.Key())
	}
	if (AgentEntry{ID: "x"}).Key() != "agentview:x" {
		t.Fatal("id key")
	}
	if !a.Waiting() || b.Waiting() {
		t.Fatal("Waiting")
	}
}

func TestParseAgentViewRejectsNonJSON(t *testing.T) {
	for _, in := range []string{"warning: update available\n" + twoEntries, "", `{"a":1}`, "boom"} {
		if _, err := ParseAgentView([]byte(in)); err == nil {
			t.Errorf("no error for %q", in)
		}
	}
	es, err := ParseAgentView([]byte("[]"))
	if err != nil || es == nil || len(es) != 0 {
		t.Fatalf("empty array: %v %v", es, err)
	}
}
