package waits

import (
	"strings"
	"testing"
)

func TestKindWordSevenWords(t *testing.T) {
	want := map[string]string{
		KindGate:        "Gate",
		KindSprint:      "Sprint waiting",
		KindFailedCheck: "Failed check",
		KindQuestion:    "Question",
		KindPrompt:      "Permission prompt",
		KindSession:     "Session waiting",
		KindYourTurn:    "Your turn",
	}
	for kind, w := range want {
		got, ok := KindWord(kind)
		if !ok || got != w {
			t.Errorf("KindWord(%q) = %q, %v; want %q, true", kind, got, ok, w)
		}
	}
	if _, ok := KindWord("finished"); ok {
		t.Error("finished has no kind word")
	}
}

func TestWordForFallsBackToGroup(t *testing.T) {
	cases := []struct {
		it   Item
		want string
	}{
		{Item{Kind: KindGate, Group: GroupBlocked}, "Gate"},
		{Item{Kind: "mystery", Group: GroupBlocked}, "Blocked"},
		{Item{Kind: "mystery", Group: GroupYourTurn}, "Your turn"},
		{Item{Kind: "mystery", Group: GroupDone}, "Blocked"},
	}
	for _, c := range cases {
		if got := WordFor(c.it); got != c.want {
			t.Errorf("WordFor(%+v) = %q, want %q", c.it, got, c.want)
		}
	}
}

func TestMoodWordsArePlain(t *testing.T) {
	want := map[string]string{
		"off": "off", "asleep": "asleep", "working": "working", "done": "done",
		"your_turn": "your turn", "blocked": "blocked", "bored": "bored", "sweating": "sweating",
	}
	for mood, w := range want {
		if got := MoodWord(mood); got != w {
			t.Errorf("MoodWord(%q) = %q, want %q", mood, got, w)
		}
		if strings.Contains(MoodWord(mood), "_") {
			t.Errorf("MoodWord(%q) has an underscore", mood)
		}
	}
	if MoodWord("mystery") != "" {
		t.Error("an unknown mood has a word")
	}
}
