// Package toast tells the person, by a Windows toast, that something new
// waits on them. The rules here decide when a toast fires and what it says;
// they read no clock and do no I/O, so tests drive them on a fake clock.
package toast

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"burnmon/internal/waits"
)

const (
	// Window is how long an item ID stays quiet after its toast.
	Window = 10 * time.Minute
	// MaxText caps the toast text in runes, the "..." of a cut included.
	MaxText = 200
)

// words names each kind in a toast. A kind without one says its group.
var words = map[string]string{
	waits.KindGate:        "Gate",
	waits.KindSprint:      "Sprint waiting",
	waits.KindFailedCheck: "Failed check",
	waits.KindQuestion:    "Question",
	waits.KindPrompt:      "Permission prompt",
	waits.KindSession:     "Session waiting",
	waits.KindYourTurn:    "Your turn",
}

// Gate holds what the rules remember between wait lists. The zero value is
// ready to use. It is not safe for concurrent use.
type Gate struct {
	started bool
	waiting map[string]bool      // IDs in Blocked or Your turn at the last Step
	last    map[string]time.Time // when each ID last had a toast
}

// Step takes the wait list as it is at now and returns the toast to show,
// if any. The first Step only learns what already waits, so a start with a
// full list stays quiet. After that an item fires when it enters Blocked or
// Your turn and its ID had no toast in the last Window; Done never fires.
// Several in one Step become one toast.
func (g *Gate) Step(now time.Time, items []waits.Item) (text string, ok bool) {
	if g.last == nil {
		g.last = map[string]time.Time{}
	}
	for id, at := range g.last {
		if now.Sub(at) >= Window {
			delete(g.last, id) // the app runs for days; old IDs must not pile up
		}
	}
	waiting := map[string]bool{}
	var fire []waits.Item
	for _, it := range items {
		if it.Group != waits.GroupBlocked && it.Group != waits.GroupYourTurn {
			continue
		}
		if waiting[it.ID] {
			continue // one ID twice in a list is one item
		}
		waiting[it.ID] = true
		if !g.started || g.waiting[it.ID] {
			continue
		}
		if _, seen := g.last[it.ID]; seen {
			continue // toasted inside the window: older ones were dropped above
		}
		fire = append(fire, it)
	}
	g.started, g.waiting = true, waiting
	for _, it := range fire {
		g.last[it.ID] = now
	}
	switch len(fire) {
	case 0:
		return "", false
	case 1:
		return line(fire[0]), true
	}
	return strconv.Itoa(len(fire)) + " things wait on you", true
}

// line is "<word>: <title>" on one line, cut to MaxText runes. Nothing else
// of the item goes in: its detail and paths can hold a prompt, a command or
// a folder name.
func line(it waits.Item) string {
	word, ok := words[it.Kind]
	if !ok {
		word = "Blocked"
		if it.Group == waits.GroupYourTurn {
			word = "Your turn"
		}
	}
	s := word + ": " + oneLine(it.Title)
	if r := []rune(s); len(r) > MaxText {
		s = string(r[:MaxText-3]) + "..."
	}
	return s
}

// oneLine turns line breaks, tabs and other control characters into single
// spaces: a title can be a Foreman slug with anything in it.
func oneLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
