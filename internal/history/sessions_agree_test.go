package history

import (
	"math"
	"testing"

	"burnmon/internal/adapter/codex"
	"burnmon/internal/dataset"
	"burnmon/internal/pricing"
)

// TestWeekCostEqualsSessionCost_OneCodexSession checks that History and
// Sessions agree on money for one session (D12 of the Mac test): for a week
// that holds one session, the week's cost in History equals that session's
// cost in Sessions, to the cent. The fixture mixes two models of the OpenAI
// price book (gpt-6-astra and gpt-5.6-terra), so both pages must price every
// turn from the same book.
func TestWeekCostEqualsSessionCost_OneCodexSession(t *testing.T) {
	events, _, _, err := codex.Adapter{}.Parse("../../testdata/codex/three-turns.jsonl", 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("fixture gave no events")
	}
	cfg := pricing.Defaults()

	sessions := dataset.SessionsFromEvents(events, &cfg)
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	p := Build(events, &cfg, Filter{Period: "week"})
	if len(p.Rows) != 1 {
		t.Fatalf("got %d week rows, want 1", len(p.Rows))
	}
	if p.Rows[0].CostUSD == nil {
		t.Fatal("History week row has no cost")
	}
	week, sess := *p.Rows[0].CostUSD, sessions[0].Cost
	// All three turns are priced: 2 turns of gpt-5.6-terra (0.0140 + 0.0126)
	// and 1 of gpt-6-astra (0.1150).
	if sessions[0].Unpriced != 0 {
		t.Errorf("Sessions left %d turns unpriced, want 0", sessions[0].Unpriced)
	}
	if math.Abs(sess-0.1416) > 1e-9 || math.Abs(week-0.1416) > 1e-9 {
		t.Errorf("History week %.6f, Sessions %.6f USD, want 0.1416 for all three turns priced", week, sess)
	}
	t.Logf("History week %s: %.6f USD; Sessions: %.6f USD", p.Rows[0].Key, week, sess)
	if math.Round(week*100) != math.Round(sess*100) {
		t.Errorf("History week cost %.4f USD, Sessions cost %.4f USD: they must agree to the cent", week, sess)
	}
}
