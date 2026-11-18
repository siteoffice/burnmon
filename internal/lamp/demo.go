package lamp

import (
	"strconv"
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

// DemoSeconds is the length of the demo loop. It plays every act and every
// state that lasts once: asleep 0 to 4, wake 4 to 6, working with dots to 10,
// knock 10 to 13, alarm 13 to 16, stamp 16 to 19, bored with the wall 19 to
// 24, slip 24 to 31, sweating 31 to 35, welcome 35 to 38, coffee 38 to 42,
// peek 42 to 47, done 47 to 48.
const DemoSeconds = 48

// demoBase is the made-up clock of the demo. It is a multiple of five minutes,
// so the pick of a line does not change inside one loop.
var demoBase = time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)

// DemoBase is the fixed clock DemoState works at. The lamp uses it to pick
// the next line in the demo.
func DemoBase() time.Time { return demoBase }

// demoActs lists the acts of the loop in time order with the second each
// starts at. An act plays for ActSeconds from there.
var demoActs = []struct {
	act   string
	start int
}{
	{ActWake, 4}, {ActKnock, 10}, {ActAlarm, 13}, {ActStamp, 16},
	{ActSlip, 24}, {ActWelcome, 35}, {ActCoffee, 38}, {ActPeek, 42},
}

// demoMood is the mood of one second of the loop.
func demoMood(sec int) string {
	switch {
	case sec < 4:
		return MoodAsleep
	case sec < 10:
		return MoodWorking
	case sec < 13:
		return MoodYourTurn
	case sec < 16:
		return MoodBlocked
	case sec < 19:
		return MoodDone
	case sec < 31:
		return MoodBored
	case sec < 35:
		return MoodSweating
	case sec < 38:
		return MoodYourTurn
	case sec < 47:
		return MoodAsleep
	}
	return MoodDone
}

func demoWrap(sec int) int { return ((sec % DemoSeconds) + DemoSeconds) % DemoSeconds }

// DemoSettings are the settings of one second of the demo: the defaults, but
// Glim hangs (lamp_quiet stay) for the first ten seconds so sleep, wake and
// dots show, and is hidden for coffee and peek.
func DemoSettings(sec int) Settings {
	s := DefaultSettings()
	if demoWrap(sec) < 10 {
		s.Quiet = "stay"
	}
	return s
}

// DemoAct is the act that plays at second sec of the demo ("" for none) and a
// number that differs between two acts and between two loops, so a page
// restarts the animation. sec counts from the start and may pass the loop.
func DemoAct(sec int) (act string, seq int64) {
	w := demoWrap(sec)
	loop := int64((sec - w) / DemoSeconds)
	for i, d := range demoActs {
		if w >= d.start && float64(w) < float64(d.start)+ActSeconds(d.act) {
			return d.act, loop*int64(len(demoActs)) + int64(i) + 1
		}
	}
	return "", 0
}

// DemoState is the state of one second of the demo, from a made-up snapshot
// run through Compute with the settings of DemoSettings. The second wraps at
// DemoSeconds. Every id in it is invented.
func DemoState(sec int) State {
	sec = demoWrap(sec)
	mood := demoMood(sec)
	now := demoBase.Add(time.Duration(sec) * time.Second)
	snap := &snapshot.Snapshot{
		SchemaVersion: snapshot.SchemaVersion,
		WrittenAt:     now.UTC().Format(time.RFC3339),
		TickSeconds:   2,
		Burn:          snapshot.Burn{TokensPerMinute: 8000, Cost: snapshot.Cost{USD: 0.42, EUR: 0.39}},
	}
	var samples []float64
	add := func(it waits.Item) {
		it.Since = now.Add(-time.Duration(it.AgeSeconds) * time.Second).UTC().Format(time.RFC3339)
		it.Source = "demo"
		it.Group = groupFor(it.Kind)
		snap.WaitItems = append(snap.WaitItems, it)
	}
	switch mood {
	case MoodWorking:
		for i := 1; i <= 3; i++ {
			snap.Sessions = append(snap.Sessions, snapshot.Session{
				ID: "demo-" + strconv.Itoa(i), Vendor: "claude", State: "working",
			})
		}
	case MoodYourTurn:
		add(waits.Item{Kind: waits.KindYourTurn, ID: "session:claude:demo-1", Title: "Review the made-up change", AgeSeconds: 90})
	case MoodBlocked:
		add(waits.Item{Kind: waits.KindQuestion, ID: "session:codex:demo-2", Title: "Which made-up option?", AgeSeconds: 240})
	case MoodBored:
		minutes := 10 + sec - 19 // 10 minutes at second 19, one more a second: 15 is the slip at 24
		add(waits.Item{Kind: waits.KindQuestion, ID: "session:claude:demo-3", Title: "Still waiting on a made-up answer", AgeSeconds: int64(minutes) * 60})
	case MoodSweating:
		// 400000 a minute over a usual 100000 is 4 times the usual and above
		// the default floor of 300000, so the demo sweats at the default.
		snap.Burn.TokensPerMinute = 400000
		samples = make([]float64, minSamples)
		for i := range samples {
			samples[i] = 100000
		}
	case MoodDone:
		add(waits.Item{Kind: waits.KindFinished, ID: "session:claude:demo-4", Title: "A made-up task finished", AgeSeconds: 30})
	}
	snap.WaitCounts = waits.CountOf(snap.WaitItems)
	return Compute(Input{Snap: snap, Now: now, Settings: DemoSettings(sec), BurnSamples: samples, Demo: true})
}

// groupFor gives a demo item its group. Real items get theirs from the
// snapshot writer; the demo only builds the few kinds above.
func groupFor(kind string) string {
	switch kind {
	case waits.KindQuestion:
		return waits.GroupBlocked
	case waits.KindFinished:
		return waits.GroupDone
	}
	return waits.GroupYourTurn
}
