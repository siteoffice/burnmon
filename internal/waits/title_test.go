package waits

import "testing"

func TestShortID(t *testing.T) {
	cases := []struct{ id, want string }{
		// An id with a UUID in it: the UUID's first 8 hex digits.
		{"session:anthropic:ec3c0d3c-c753-54e3-b994-c3a39e7505fc", "ec3c0d3c"},
		{"ec3c0d3c-c753-54e3-b994-c3a39e7505fc", "ec3c0d3c"},
		{"session:anthropic:EC3C0D3C-C753-54E3-B994-C3A39E7505FC", "EC3C0D3C"},
		// A UUID that is not version 7 keeps its head, even inside a longer id.
		{"session:openai:rollout-2026-10-30T10-11-12-5973b6c0-1a2b-4c3d-8e9f-0123456789ab", "5973b6c0"},
		// Codex: the rollout file name, a timestamp then a version-7 UUID, whose
		// head is a millisecond clock: its last 8 hex digits.
		{"session:openai:rollout-2026-10-30T10-11-12-019a1b2c-3d4e-7f50-8a6b-7c8d9e0f1a2b", "9e0f1a2b"},
		// Two Codex sessions started a second apart share the v7 head, not the tail.
		{"session:openai:rollout-2026-10-30T16-58-25-01a0edac-1f2e-7a3b-8c4d-5e6f7a8b9c0d", "7a8b9c0d"},
		{"session:openai:rollout-2026-10-30T16-58-26-01a0edac-2b3c-7d4e-9f5a-6b7c8d9e0f1a", "8d9e0f1a"},
		// No UUID: the last 8 runes, where sessions of one day or one prefix differ.
		{"session:hermes:20260922_213936_0572d4", "6_0572d4"},
		{"session:anthropic:agent-a1b2c3d4e5f6a7b8", "e5f6a7b8"},
		{"session:anthropic:c1d2e3f4a5b6", "e3f4a5b6"},
		// Shorter than 8 runes: all of it.
		{"agentview:ab12", "ab12"},
		{"agentview:pid:3", "3"},
		{"x:\u00e8\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9", "\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9"},
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
