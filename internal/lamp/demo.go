package lamp

import (
	"strconv"
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

// DemoSeconds is the length of the demo loop: eight segments of three seconds.
const DemoSeconds = 24

// demoBase is the made-up clock of the demo. It is a multiple of five minutes,
// so the pick of a line does not change inside one loop.
var demoBase = time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)

// DemoBase is the fixed clock DemoState works at. The lamp uses it to pick
// the next line in the demo.
func DemoBase() time.Time { return demoBase }

// demoSegments is the loop in the order of the lamp spec: asleep, working,
// your turn, blocked, bored with the wall growing, sweating, done, working.
var demoSegments = []string{
	MoodAsleep, MoodWorking, MoodYourTurn, MoodBlocked,
	MoodBored, MoodSweating, MoodDone, MoodWorking,
}

// DemoState is the state of one second of the demo, from a made-up snapshot
// run through Compute with the default settings. The second wraps at 24.
// Every id in it is invented.
func DemoState(sec int) State {
	sec = ((sec % DemoSeconds) + DemoSeconds) % DemoSeconds
	mood := demoSegments[sec/3]
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
		minutes := 10 + 5*(sec-12) // 10, 15 and 20 minutes on the three seconds
		add(waits.Item{Kind: waits.KindQuestion, ID: "session:claude:demo-3", Title: "Still waiting on a made-up answer", AgeSeconds: int64(minutes) * 60})
	case MoodSweating:
		snap.Burn.TokensPerMinute = 60000
		samples = make([]float64, minSamples)
		for i := range samples {
			samples[i] = 10000
		}
	case MoodDone:
		add(waits.Item{Kind: waits.KindFinished, ID: "session:claude:demo-4", Title: "A made-up task finished", AgeSeconds: 30})
	}
	snap.WaitCounts = waits.CountOf(snap.WaitItems)
	return Compute(Input{Snap: snap, Now: now, Settings: DefaultSettings(), BurnSamples: samples, Demo: true})
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
