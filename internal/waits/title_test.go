package waits

import "testing"

func TestShortID(t *testing.T) {
	cases := []struct{ id, want string }{
		{"session:anthropic:ec3c0d3c-c753-54e3-b994-c3a39e7505fc", "ec3c0d3c"},
		{"ec3c0d3c-c753-54e3-b994-c3a39e7505fc", "ec3c0d3c"},
		{"agentview:ab12", "ab12"},
		{"agentview:pid:3", "3"},
		{"x:\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9", "\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9"},
		{"", ""},
	}
	for _, c := range cases {
		if got := ShortID(c.id); got != c.want {
			t.Errorf("ShortID(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestSessionTitle(t *testing.T) {
	const bare = "ec3c0d3c-c753-54e3-b994-c3a39e7505fc"
	const id = "session:anthropic:" + bare
	cases := []struct{ folder, agent, id, want string }{
		{"burnmon", "claude-code", id, "burnmon ec3c0d3c"},
		// A cowork-cloud session has no project folder: the graph labels it with the bare id.
		{bare, "cowork-cloud", id, "cowork-cloud ec3c0d3c"},
		{"", "", id, "session ec3c0d3c"},
		{bare, "", id, "session ec3c0d3c"},
		{"", "cowork-cloud", id, "cowork-cloud ec3c0d3c"},
		// An agent-only node without a session id is labelled with its whole id.
		{"agentview:ab12", "", "agentview:ab12", "session ab12"},
		// Toast text is built from the title, so it stays one line.
		{"my\nproj", "", id, "my proj ec3c0d3c"},
		{"  ", "", id, "session ec3c0d3c"},
	}
	for _, c := range cases {
		if got := SessionTitle(c.folder, c.agent, c.id); got != c.want {
			t.Errorf("SessionTitle(%q, %q, %q) = %q, want %q", c.folder, c.agent, c.id, got, c.want)
		}
	}
}
