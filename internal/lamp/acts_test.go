package lamp

import (
	"testing"
	"time"

	"burnmon/internal/waits"
)

// actNow is 12:03:10 local, a unix minute that is not a multiple of 4 and not
// a coffee slot, so a test only meets the act it is about.
var actNow = time.Date(2026, 11, 18, 12, 3, 10, 0, time.UTC)

// actState is a state with the given mood and counts, as Compute would make it.
func actState(mood string, blockedN, turnN, agents int, ids ...string) State {
	return State{
		Mood: mood, Agents: agents,
		Counts:  waits.Counts{Blocked: blockedN, YourTurn: turnN},
		Counted: blockedN + turnN, CountedIDs: ids,
		TickerRows: 1,
	}
}

func actIn(prev, cur State) ActInput {
	return ActInput{Prev: prev, Cur: cur, Now: actNow, Settings: DefaultSettings()}
}

func act(t *testing.T, in ActInput) string {
	t.Helper()
	a, _, _ := NextAct(in)
	return a
}

func TestActAlarmDueAndNot(t *testing.T) {
	prev := actState(MoodBlocked, 1, 0, 1, "a")
	if got := act(t, actIn(prev, actState(MoodBlocked, 2, 0, 1, "a", "b"))); got != "alarm" {
		t.Errorf("blocked 1 to 2: %q, want alarm", got)
	}
	if got := act(t, actIn(prev, actState(MoodBlocked, 1, 0, 1, "a"))); got != "" {
		t.Errorf("blocked unchanged: %q, want none", got)
	}
	if got := act(t, actIn(actState(MoodBlocked, 2, 0, 1, "a", "b"), actState(MoodBlocked, 1, 0, 1, "a"))); got == "alarm" {
		t.Errorf("blocked went down: alarm played")
	}
}

func TestActKnockDueAndNot(t *testing.T) {
	prev := actState(MoodWorking, 0, 0, 1)
	if got := act(t, actIn(prev, actState(MoodYourTurn, 0, 1, 1, "a"))); got != "knock" {
		t.Errorf("your_turn 0 to 1: %q, want knock", got)
	}
	if got := act(t, actIn(actState(MoodYourTurn, 0, 1, 1, "a"), actState(MoodYourTurn, 0, 1, 1, "a"))); got != "" {
		t.Errorf("your_turn unchanged: %q, want none", got)
	}
}

func TestActStampDueAndNot(t *testing.T) {
	prev := actState(MoodBlocked, 2, 0, 1, "a", "b")
	if got := act(t, actIn(prev, actState(MoodBlocked, 1, 0, 1, "b"))); got != "stamp" {
		t.Errorf("item a gone, count down: %q, want stamp", got)
	}
	// An id left and another came: the count did not go down, no stamp.
	if got := act(t, actIn(prev, actState(MoodBlocked, 2, 0, 1, "b", "c"))); got == "stamp" {
		t.Errorf("an item was replaced, count not down: stamp played")
	}
	// The count went down with no id gone (ids empty in both): no stamp.
	if got := act(t, actIn(actState(MoodBlocked, 2, 0, 1), actState(MoodBlocked, 1, 0, 1))); got == "stamp" {
		t.Errorf("no id of the previous state is gone: stamp played")
	}
}

func TestActWelcomeAt299And300(t *testing.T) {
	st := actState(MoodYourTurn, 0, 1, 1, "a")
	in := actIn(st, st)
	in.PrevIdle, in.Idle = 299, 0
	if got := act(t, in); got != "" {
		t.Errorf("idle 299: %q, want none", got)
	}
	in.PrevIdle = 300
	if got := act(t, in); got != "welcome" {
		t.Errorf("idle 300: %q, want welcome", got)
	}
	in.Idle = 5
	if got := act(t, in); got != "" {
		t.Errorf("idle back to 5: %q, want none (needs under 5)", got)
	}
	in.Idle = 4
	nothing := actState(MoodWorking, 0, 0, 1)
	in.Prev, in.Cur = nothing, nothing
	if got := act(t, in); got != "" {
		t.Errorf("welcome with nothing waiting: %q, want none", got)
	}
}

func TestActWakeDueAndNot(t *testing.T) {
	if got := act(t, actIn(actState(MoodAsleep, 0, 0, 0), actState(MoodWorking, 0, 0, 1))); got != "wake" {
		t.Errorf("agents 0 to 1: %q, want wake", got)
	}
	if got := act(t, actIn(actState(MoodWorking, 0, 0, 1), actState(MoodWorking, 0, 0, 2))); got != "" {
		t.Errorf("agents 1 to 2: %q, want none", got)
	}
}

func boredState(minutes int) State {
	st := actState(MoodBored, 0, 1, 1, "a")
	st.LongestWaitMin = minutes
	return st
}

func TestActSlipOncePerBoredStretch(t *testing.T) {
	in := actIn(boredState(14), boredState(14))
	if got := act(t, in); got != "" {
		t.Errorf("14 minutes: %q, want none", got)
	}
	in = actIn(boredState(15), boredState(15))
	a, _, p := NextAct(in)
	if a != "slip" {
		t.Fatalf("15 minutes: %q, want slip", a)
	}
	// The same stretch, a minute later: not again.
	in2 := actIn(boredState(16), boredState(16))
	in2.Played = p
	if got := act(t, in2); got != "" {
		t.Errorf("same stretch: %q, want none", got)
	}
	// The mood leaves bored: the stretch ends.
	out := actIn(boredState(16), actState(MoodYourTurn, 0, 1, 1, "a"))
	out.Played = p
	_, _, p2 := NextAct(out)
	in3 := actIn(boredState(15), boredState(15))
	in3.Played = p2
	if got := act(t, in3); got != "slip" {
		t.Errorf("new stretch: %q, want slip", got)
	}
}

func TestActCoffeeNotWhileAnItemWaitsOncePerSlot(t *testing.T) {
	quiet := actState(MoodAsleep, 0, 0, 0)
	in := actIn(quiet, quiet)
	in.Now = time.Date(2026, 11, 18, 10, 0, 30, 0, time.Local)
	a, _, p := NextAct(in)
	if a != "coffee" {
		t.Fatalf("10:00:30 and nothing waits: %q, want coffee", a)
	}
	in.Now = in.Now.Add(2 * time.Second)
	in.Played = p
	if got := act(t, in); got != "" {
		t.Errorf("second tick of the slot: %q, want none", got)
	}
	in.Now = time.Date(2026, 11, 18, 15, 0, 5, 0, time.Local)
	if got := act(t, in); got != "coffee" {
		t.Errorf("15:00: %q, want coffee", got)
	}
	in.Now = time.Date(2026, 11, 19, 10, 0, 5, 0, time.Local)
	if got := act(t, in); got != "coffee" {
		t.Errorf("the next day 10:00: %q, want coffee", got)
	}
	in.Now = time.Date(2026, 11, 18, 10, 1, 0, 0, time.Local)
	in.Played = Played{}
	if got := act(t, in); got != "" {
		t.Errorf("10:01: %q, want none", got)
	}
	wait := actState(MoodYourTurn, 0, 1, 1, "a")
	in = actIn(wait, wait)
	in.Now = time.Date(2026, 11, 18, 10, 0, 5, 0, time.Local)
	if got := act(t, in); got != "" {
		t.Errorf("an item waits at 10:00: %q, want none", got)
	}
}

// peekAt is the first second of a unix minute that is a multiple of 4.
func peekAt(minute int64) time.Time { return time.Unix(minute*60, 0) }

func TestActPeekOnlyWhenQuietHiddenAndNotOff(t *testing.T) {
	quiet := actState(MoodAsleep, 0, 0, 0)
	in := actIn(quiet, quiet)
	in.Now = peekAt(7000000) // 7000000 mod 4 == 0
	if got := act(t, in); got != "peek" {
		t.Fatalf("quiet, hide, minute mod 4 = 0: %q, want peek", got)
	}
	in.Now = peekAt(7000001)
	if got := act(t, in); got != "" {
		t.Errorf("minute mod 4 = 1: %q, want none", got)
	}
	in.Now = peekAt(7000000)
	in.Settings.Quiet = "stay"
	if got := act(t, in); got != "" {
		t.Errorf("lamp_quiet stay: %q, want none", got)
	}
	in.Settings.Quiet = "hide"
	busy := actState(MoodBlocked, 1, 0, 1, "a")
	in.Prev, in.Cur = busy, busy
	if got := act(t, in); got != "" {
		t.Errorf("not quiet: %q, want none", got)
	}
	off := State{Mood: MoodOff, TickerRows: 1}
	in.Prev, in.Cur = off, off
	if got := act(t, in); got != "" {
		t.Errorf("mood off: %q, want none", got)
	}
}

func TestActPeekFirstTickOfTheMinuteOnly(t *testing.T) {
	quiet := actState(MoodAsleep, 0, 0, 0)
	in := actIn(quiet, quiet)
	in.Now = peekAt(7000000)
	a, _, p := NextAct(in)
	if a != "peek" {
		t.Fatalf("first tick: %q", a)
	}
	in.Now = in.Now.Add(2 * time.Second)
	in.Played = p
	if got := act(t, in); got != "" {
		t.Errorf("second tick of the same minute: %q, want none", got)
	}
	// The first tick found the lamp busy; a quiet second tick must not peek.
	busy := actState(MoodBlocked, 1, 0, 1, "a")
	in = actIn(busy, busy)
	in.Now = peekAt(7000004)
	_, _, p = NextAct(in)
	in = actIn(quiet, quiet)
	in.Now = peekAt(7000004).Add(2 * time.Second)
	in.Played = p
	if got := act(t, in); got != "" {
		t.Errorf("minute already seen at the first tick: %q, want none", got)
	}
}

func TestActOrderWhenSeveralAreDue(t *testing.T) {
	// alarm and knock together: alarm.
	prev := actState(MoodWorking, 0, 0, 1)
	cur := actState(MoodBlocked, 1, 1, 1, "a", "b")
	if got := act(t, actIn(prev, cur)); got != "alarm" {
		t.Errorf("alarm and knock: %q, want alarm", got)
	}
	// welcome and wake together: welcome.
	pw := actState(MoodAsleep, 0, 0, 0)
	cw := actState(MoodYourTurn, 0, 1, 1, "a")
	in := actIn(pw, cw)
	in.PrevIdle, in.Idle = 400, 0
	if got := act(t, in); got != "knock" { // knock (your_turn went up) comes before welcome
		t.Errorf("knock, welcome and wake: %q, want knock", got)
	}
	// welcome and wake with no count change.
	pw2 := actState(MoodAsleep, 0, 0, 0)
	pw2.Counted = 1 // an item waits but the counts did not go up
	pw2.Counts = waits.Counts{YourTurn: 1}
	cw2 := actState(MoodYourTurn, 0, 1, 1, "a")
	in = actIn(pw2, cw2)
	in.PrevIdle, in.Idle = 400, 0
	if got := act(t, in); got != "welcome" {
		t.Errorf("welcome and wake: %q, want welcome", got)
	}
	// slip and peek together: slip.
	b := boredState(20)
	in = actIn(b, b)
	in.Now = peekAt(7000000)
	if got := act(t, in); got != "slip" {
		t.Errorf("slip: %q", got)
	}
}

func TestActNeverInMoodOffOrOnFirstTick(t *testing.T) {
	off := State{Mood: MoodOff, TickerRows: 1}
	in := actIn(actState(MoodBlocked, 0, 0, 0), off)
	if got := act(t, in); got != "" {
		t.Errorf("mood off: %q", got)
	}
	// The first tick has no previous state: no alarm for what was already there.
	if got := act(t, actIn(State{}, actState(MoodBlocked, 3, 0, 2, "a", "b", "c"))); got != "" {
		t.Errorf("first tick: %q, want none", got)
	}
	// The first tick after no data: BurnMon just started, no alarm either.
	if got := act(t, actIn(off, actState(MoodBlocked, 3, 0, 2, "a", "b", "c"))); got != "" {
		t.Errorf("first tick after no data: %q, want none", got)
	}
}

func TestActCalmAndOffPlayNoAct(t *testing.T) {
	prev := actState(MoodWorking, 0, 0, 1)
	cur := actState(MoodBlocked, 1, 0, 1, "a")
	for _, m := range []string{"calm", "off"} {
		in := actIn(prev, cur)
		in.Settings.Motion = m
		if got := act(t, in); got != "" {
			t.Errorf("motion %s: %q, want none", m, got)
		}
	}
	in := actIn(prev, cur)
	in.Settings.Motion = "lively"
	if got := act(t, in); got != "alarm" {
		t.Errorf("lively: %q, want alarm", got)
	}
}

func TestActSeqGoesUpWithEachAct(t *testing.T) {
	prev := actState(MoodWorking, 0, 0, 1)
	cur := actState(MoodBlocked, 1, 0, 1, "a")
	_, _, p := NextAct(actIn(prev, cur))
	if p.Seq != 1 {
		t.Fatalf("seq %d after the first act, want 1", p.Seq)
	}
	in := actIn(prev, cur)
	in.Played = p
	_, _, p = NextAct(in)
	if p.Seq != 2 {
		t.Errorf("seq %d after the second act, want 2", p.Seq)
	}
	_, _, p = NextAct(actIn(cur, cur))
	if p.Seq != 0 {
		t.Errorf("seq %d with no act and a zero Played, want 0", p.Seq)
	}
}

func holdsOf(t *testing.T, st State, s Settings) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, h := range Holds(st, s) {
		out[h] = true
	}
	return out
}

func TestHolds(t *testing.T) {
	def := DefaultSettings()
	stay := def
	stay.Quiet = "stay"

	if h := holdsOf(t, boredState(12), def); !h["wall"] {
		t.Errorf("bored: %v, want wall", h)
	}
	if h := holdsOf(t, State{Mood: MoodSweating}, def); !h["sweat"] {
		t.Errorf("sweating: %v, want sweat", h)
	}
	if h := holdsOf(t, State{Mood: MoodAsleep}, stay); !h["sleep"] {
		t.Errorf("asleep and stay: %v, want sleep", h)
	}
	if h := holdsOf(t, State{Mood: MoodAsleep}, def); h["sleep"] {
		t.Errorf("asleep and hide: sleep held, Glim is up")
	}
	if h := holdsOf(t, State{Mood: MoodOff}, stay); len(h) != 0 {
		t.Errorf("off: %v, want none", h)
	}
	// dots: Glim is down and at least one agent works.
	if h := holdsOf(t, actState(MoodBlocked, 1, 0, 2, "a"), def); !h["dots"] {
		t.Errorf("blocked with 2 agents: %v, want dots", h)
	}
	if h := holdsOf(t, actState(MoodBlocked, 1, 0, 0, "a"), def); h["dots"] {
		t.Errorf("no agents: dots held")
	}
	if h := holdsOf(t, actState(MoodWorking, 0, 0, 1), def); h["dots"] {
		t.Errorf("working, hide: Glim is up, dots held")
	}
	if h := holdsOf(t, actState(MoodWorking, 0, 0, 1), stay); !h["dots"] {
		t.Errorf("working, stay: Glim hangs, want dots")
	}
}

func TestQuietIsTheSliver(t *testing.T) {
	def := DefaultSettings()
	stay := def
	stay.Quiet = "stay"
	for _, tc := range []struct {
		mood string
		s    Settings
		want bool
	}{
		{MoodAsleep, def, true}, {MoodWorking, def, true}, {MoodOff, def, true},
		{MoodAsleep, stay, false}, {MoodOff, stay, false},
		{MoodBlocked, def, false}, {MoodDone, def, false},
	} {
		if got := Quiet(State{Mood: tc.mood}, tc.s); got != tc.want {
			t.Errorf("Quiet(%s, %s) = %v, want %v", tc.mood, tc.s.Quiet, got, tc.want)
		}
	}
}

func TestDotSeconds(t *testing.T) {
	for _, tc := range []struct{ tpm, want float64 }{{0, 4}, {1000, 4}, {100000, 0.3}, {3000, 4}, {60000, 0.5}} {
		if got := DotSeconds(tc.tpm); got != tc.want {
			t.Errorf("DotSeconds(%v) = %v, want %v", tc.tpm, got, tc.want)
		}
	}
}

func TestActSeconds(t *testing.T) {
	want := map[string]float64{"alarm": 2.6, "knock": 1.6, "stamp": 2.4, "welcome": 2.4, "wake": 1.6, "slip": 6.4, "coffee": 4, "peek": 4.4}
	for a, s := range want {
		if got := ActSeconds(a); got != s {
			t.Errorf("ActSeconds(%s) = %v, want %v", a, got, s)
		}
	}
	if ActSeconds("nothing") != 0 {
		t.Error("an unknown act has a length")
	}
}
