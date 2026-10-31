package toast

import (
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"burnmon/internal/waits"
)

// t0 is the fake clock's start; every test moves it by hand.
var t0 = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func item(kind, id, group, title string) waits.Item {
	return waits.Item{Kind: kind, ID: id, Group: group, Title: title}
}

func gate(id string) waits.Item {
	return item(waits.KindGate, id, waits.GroupBlocked, id)
}

// step runs one Step at t0+d and fails unless it returns want; "" is no toast.
func step(t *testing.T, g *Gate, d time.Duration, want string, items ...waits.Item) {
	t.Helper()
	text, ok := g.Step(t0.Add(d), items)
	if want == "" {
		if ok || text != "" {
			t.Fatalf("at +%v: got toast %q, want none", d, text)
		}
		return
	}
	if !ok || text != want {
		t.Fatalf("at +%v: got (%q, %v), want %q", d, text, ok, want)
	}
}

func TestStep_FirstStepIsBaseline(t *testing.T) {
	var g Gate
	var start []waits.Item
	for i := 0; i < 20; i++ {
		group := waits.GroupBlocked
		if i%2 == 1 {
			group = waits.GroupYourTurn
		}
		start = append(start, item(waits.KindSession, "s"+strconv.Itoa(i), group, "s"+strconv.Itoa(i)))
	}
	step(t, &g, 0, "", start...)
	step(t, &g, time.Second, "", start...)
	// The baseline knows the 20, so only a real newcomer fires.
	step(t, &g, 2*time.Second, "Gate: new", append(start, gate("new"))...)
}

func TestStep_NewWaitingItemSaysKindAndTitle(t *testing.T) {
	cases := []struct{ kind, group, want string }{
		{waits.KindGate, waits.GroupBlocked, "Gate: t"},
		{waits.KindSprint, waits.GroupBlocked, "Sprint waiting: t"},
		{waits.KindFailedCheck, waits.GroupBlocked, "Failed check: t"},
		{waits.KindQuestion, waits.GroupBlocked, "Question: t"},
		{waits.KindPrompt, waits.GroupBlocked, "Permission prompt: t"},
		{waits.KindSession, waits.GroupBlocked, "Session waiting: t"},
		{waits.KindYourTurn, waits.GroupYourTurn, "Your turn: t"},
		// A kind with no word of its own says its group.
		{"novel", waits.GroupBlocked, "Blocked: t"},
		{"novel", waits.GroupYourTurn, "Your turn: t"},
	}
	for _, c := range cases {
		var g Gate
		step(t, &g, 0, "")
		step(t, &g, time.Minute, c.want, item(c.kind, "x", c.group, "t"))
	}
}

func TestStep_DoneNeverFires(t *testing.T) {
	var g Gate
	step(t, &g, 0, "")
	step(t, &g, time.Minute, "",
		item(waits.KindCloseOut, "a", waits.GroupDone, "a"),
		item(waits.KindCanClose, "b", waits.GroupDone, "b"),
		item(waits.KindBranchGone, "c", waits.GroupDone, "c"),
		// The group decides, not the kind.
		item(waits.KindGate, "d", waits.GroupDone, "d"))
	// Done to Blocked is entering Blocked.
	step(t, &g, 2*time.Minute, "Gate: d", gate("d"))
}

func TestStep_OneToastPerIDPerWindow(t *testing.T) {
	var g Gate
	step(t, &g, 0, "")
	step(t, &g, time.Minute, "Gate: a", gate("a"))
	step(t, &g, 2*time.Minute, "")
	// Back 5 minutes after its toast: still inside the window.
	step(t, &g, 6*time.Minute, "", gate("a"))
	step(t, &g, 7*time.Minute, "")
	step(t, &g, time.Minute+Window-time.Second, "", gate("a"))
	step(t, &g, time.Minute+Window-time.Second/2, "")
	// Back 10 minutes after its toast: fires again.
	step(t, &g, time.Minute+Window, "Gate: a", gate("a"))
}

func TestStep_StayingItemFiresOnce(t *testing.T) {
	var g Gate
	step(t, &g, 0, "")
	step(t, &g, time.Minute, "Question: a", item(waits.KindQuestion, "a", waits.GroupBlocked, "a"))
	for _, d := range []time.Duration{2 * time.Minute, 12 * time.Minute, time.Hour, 25 * time.Hour} {
		step(t, &g, d, "", item(waits.KindQuestion, "a", waits.GroupBlocked, "a"))
	}
	// Moving from Blocked to Your turn is no entry: it waited all along.
	step(t, &g, 26*time.Hour, "", item(waits.KindYourTurn, "a", waits.GroupYourTurn, "a"))
}

func TestStep_SeveralInOneStepBecomeOneToast(t *testing.T) {
	var g Gate
	step(t, &g, 0, "")
	step(t, &g, time.Minute, "3 things wait on you", gate("a"), gate("b"), gate("c"))
	step(t, &g, 2*time.Minute, "")
	// The batch counted as each item's toast.
	step(t, &g, 6*time.Minute, "", gate("a"), gate("b"), gate("c"))
}

func TestStep_BatchLeavesOutItemsInsideTheirWindow(t *testing.T) {
	var g Gate
	step(t, &g, 0, "")
	step(t, &g, time.Minute, "Gate: a", gate("a"))
	step(t, &g, 2*time.Minute, "")
	// a is back inside its window, b is new: b alone, by its own words.
	step(t, &g, 5*time.Minute, "Question: b", gate("a"), item(waits.KindQuestion, "b", waits.GroupBlocked, "b"))
	step(t, &g, 6*time.Minute, "")
	step(t, &g, 7*time.Minute, "2 things wait on you", gate("a"), gate("c"), gate("d"))
	// One ID twice in a list is one item.
	step(t, &g, 8*time.Minute, "")
	step(t, &g, 20*time.Minute, "Gate: e", gate("e"), gate("e"))
}

func TestStep_TextIsKindAndTitleOnly(t *testing.T) {
	title := "line one\nline two "
	title += strings.Repeat("\u00e9", 500-utf8.RuneCountInString(title))
	it := gate("a")
	it.Title = title
	it.Detail = "rm -rf x"
	it.SpecPath = `C:\x\spec.md`
	it.ResultPath = `C:\x\result.md`

	var g Gate
	step(t, &g, 0, "")
	text, ok := g.Step(t0.Add(time.Minute), []waits.Item{it})
	if !ok {
		t.Fatal("no toast for a new item")
	}
	if n := utf8.RuneCountInString(text); n != MaxText {
		t.Errorf("text is %d runes, want %d", n, MaxText)
	}
	if !strings.HasPrefix(text, "Gate: line one line two \u00e9") || !strings.HasSuffix(text, "...") {
		t.Errorf("text = %q, want one line from the title, ending in ...", text)
	}
	for _, bad := range []string{"\n", "rm -rf", `C:\x`, "spec.md", "result.md"} {
		if strings.Contains(text, bad) {
			t.Errorf("text %q holds %q", text, bad)
		}
	}
}

func TestStep_TitleIsOneLine(t *testing.T) {
	fit := strings.Repeat("x", MaxText-len("Gate: "))
	cases := []struct{ title, want string }{
		{"a\r\nb\tc", "Gate: a b c"},
		{"  a\u2028b\x00c\x1b  ", "Gate: a b c"},
		// Exactly MaxText runes stays whole; one more is cut.
		{fit, "Gate: " + fit},
		{fit + "y", "Gate: " + fit[:len(fit)-3] + "..."},
	}
	for _, c := range cases {
		it := gate("a")
		it.Title = c.title
		var g Gate
		step(t, &g, 0, "")
		step(t, &g, time.Minute, c.want, it)
	}
}

func TestStep_ForgetsToastsPastTheWindow(t *testing.T) {
	var g Gate
	step(t, &g, 0, "")
	step(t, &g, time.Minute, "2 things wait on you", gate("a"), gate("b"))
	step(t, &g, time.Minute+Window, "")
	// The app runs for days: IDs past their window must not pile up.
	if n := len(g.last); n != 0 {
		t.Errorf("Gate still holds %d old toasts, want 0", n)
	}
}
