package waits

import "strings"

// bareID is the part of an id after its last ":".
func bareID(id string) string {
	return id[strings.LastIndex(id, ":")+1:]
}

// ShortID is the bare id cut to 8 runes: enough to tell two sessions of one
// folder apart.
func ShortID(id string) string {
	r := []rune(bareID(id))
	if len(r) > 8 {
		r = r[:8]
	}
	return string(r)
}

// SessionTitle is a session item's title, never its raw id: the folder and
// the short id, else the agent (a cowork-cloud session has no folder), else
// "session". A folder that is the id itself is no folder. The title is one
// line, since a toast is built from it.
func SessionTitle(folder, agent, id string) string {
	name := strings.Join(strings.Fields(folder), " ")
	if name == id || name == bareID(id) {
		name = ""
	}
	if name == "" {
		name = strings.Join(strings.Fields(agent), " ")
	}
	if name == "" {
		name = "session"
	}
	return strings.TrimSpace(name + " " + ShortID(id))
}
