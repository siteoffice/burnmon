package waits

import (
	"regexp"
	"strings"
)

var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// bareID is the part of an id after its last ":".
func bareID(id string) string {
	return id[strings.LastIndex(id, ":")+1:]
}

// ShortID tells two sessions of one folder apart in 8 characters. When the
// bare id holds a UUID it is the UUID's first 8 hex digits (Claude), but its
// last 8 for a version-7 UUID (Codex's rollout-<time>-<uuid>), whose head is
// a millisecond clock that sessions started a minute apart share. Without a
// UUID it is the bare id's last 8 runes, all of it when shorter, since ids
// like Hermes' <date>_<time>_<hex> or agent-<hex> share their start.
func ShortID(id string) string {
	b := bareID(id)
	if u := uuidRe.FindString(b); u != "" {
		if u[14] == '7' { // the version digit, first of the third group
			return u[len(u)-8:]
		}
		return u[:8]
	}
	r := []rune(b)
	if len(r) > 8 {
		r = r[len(r)-8:]
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
