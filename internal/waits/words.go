package waits

// kindWords names each kind of wait item for a person. The notification and
// the lamp read this one table, so a kind has one word everywhere.
var kindWords = map[string]string{
	KindGate:        "Gate",
	KindSprint:      "Sprint waiting",
	KindFailedCheck: "Failed check",
	KindQuestion:    "Question",
	KindPrompt:      "Permission prompt",
	KindSession:     "Session waiting",
	KindYourTurn:    "Your turn",
}

// KindWord is the word for a kind. ok is false for a kind without one.
func KindWord(kind string) (word string, ok bool) {
	word, ok = kindWords[kind]
	return word, ok
}

// WordFor is the word that names an item: its kind's word, or for a kind
// without one the word of its group ("Your turn" or "Blocked").
func WordFor(it Item) string {
	if w, ok := KindWord(it.Kind); ok {
		return w
	}
	if it.Group == GroupYourTurn {
		return "Your turn"
	}
	return "Blocked"
}

// moodWords names each mood of the lamp in plain words, for a person: the
// labels of the pose sheet. The waits package does not know the lamp, so the
// keys are the mood names the lamp uses.
var moodWords = map[string]string{
	"off": "off", "asleep": "asleep", "working": "working", "done": "done",
	"your_turn": "your turn", "blocked": "blocked", "bored": "bored", "sweating": "sweating",
}

// MoodWord is the plain word for a lamp mood, or "" for a name it does not know.
func MoodWord(mood string) string { return moodWords[mood] }
