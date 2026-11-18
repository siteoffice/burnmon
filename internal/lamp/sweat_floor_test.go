package lamp

import (
	"testing"

	"burnmon/internal/snapshot"
)

// The sweat floor defaults to 300000 tokens a minute, and a rate under it
// does not sweat however far it is above the usual burn.
func TestSweatDefaultFloorIs300000(t *testing.T) {
	if got := DefaultSettings().BurnFloorTPM; got != 300000 {
		t.Fatalf("default floor %d, want 300000", got)
	}
	// No file, no key and a null key all give the default.
	for _, in := range []string{"", "{}", `{"lamp_burn_floor_tpm":null}`} {
		if got := ParseSettings([]byte(in)).BurnFloorTPM; got != 300000 {
			t.Errorf("ParseSettings(%q) floor %d, want 300000", in, got)
		}
	}
	// A saved whole number of 0 or more holds, 0 switches the floor off.
	for in, want := range map[string]int{
		`{"lamp_burn_floor_tpm":0}`:       0,
		`{"lamp_burn_floor_tpm":200000}`:  200000,
		`{"lamp_burn_floor_tpm":2500000}`: 2500000, // outside the slider, still holds
		`{"lamp_burn_floor_tpm":-1}`:      300000,
		`{"lamp_burn_floor_tpm":1.5}`:     300000,
		`{"lamp_burn_floor_tpm":"x"}`:     300000,
	} {
		if got := ParseSettings([]byte(in)).BurnFloorTPM; got != want {
			t.Errorf("ParseSettings(%s) floor %d, want %d", in, got, want)
		}
	}
}

// A rate of 250000 with a usual burn of 10000 is 25 times the usual, yet it
// stays under the default floor; with the floor at 200000 the same rate sweats.
func TestSweatRateUnderDefaultFloorDoesNotSweat(t *testing.T) {
	working := []snapshot.Session{session("w", "coding", "")}
	def := DefaultSettings()
	if got := compute(snap(nil, working, 250000), def, samples(120, 10000)); got.Mood != MoodWorking {
		t.Errorf("250000 at the default floor: mood %q, want working", got.Mood)
	}
	low := DefaultSettings()
	low.BurnFloorTPM = 200000
	if got := compute(snap(nil, working, 250000), low, samples(120, 10000)); got.Mood != MoodSweating {
		t.Errorf("250000 with the floor at 200000: mood %q, want sweating", got.Mood)
	}
	// The rule of three times the usual burn is unchanged.
	if got := compute(snap(nil, working, 300000), def, samples(120, 101000)); got.Mood != MoodWorking {
		t.Errorf("under 3 times the usual: mood %q, want working", got.Mood)
	}
	if got := compute(snap(nil, working, 300000), def, samples(120, 100000)); got.Mood != MoodSweating {
		t.Errorf("3 times the usual and at the floor: mood %q, want sweating", got.Mood)
	}
}

// The demo sweats at the default floor, through the state function, and
// every sample of its usual burn is well under the rate.
func TestDemoSweatsAtDefaultFloor(t *testing.T) {
	for sec := 31; sec < 35; sec++ {
		st := DemoState(sec)
		if st.Mood != MoodSweating {
			t.Errorf("second %d: mood %q, want sweating at the default floor", sec, st.Mood)
		}
		if st.BurnRate < float64(DefaultSettings().BurnFloorTPM) {
			t.Errorf("second %d: demo rate %v is under the default floor", sec, st.BurnRate)
		}
	}
}
