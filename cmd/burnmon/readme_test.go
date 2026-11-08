package main

import (
	"os"
	"strings"
	"testing"
)

// TestReadmeNamesKeepHistoryAndWhatNeedsBurnMon is the doc guard for B1: the
// README says what needs BurnMon and what keep_history does, and carries no
// em dash. The snapshot doc is covered by the later docs pass (B5).
func TestReadmeNamesKeepHistoryAndWhatNeedsBurnMon(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## What needs BurnMon",
		"keep_history",
		"deleting a transcript does not delete BurnMon's copy",
	} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("README.md lacks %q", want)
		}
	}
	if strings.ContainsRune(string(readme), 0x2014) {
		t.Error("README.md contains an em dash")
	}
}

// TestReadmeConfigurationNamesBothLocationsAndNewKeys guards the Configuration
// first paragraph and the stale "not listed yet" phrases.
func TestReadmeConfigurationNamesBothLocationsAndNewKeys(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := strings.ReplaceAll(string(b), "\r\n", "\n")
	for _, gone := range []string{"does not list them yet", "not in it yet", "read-only-share install"} {
		if strings.Contains(readme, gone) {
			t.Errorf("README.md still contains %q", gone)
		}
	}
	_, after, ok := strings.Cut(readme, "\n## Configuration\n")
	if !ok {
		t.Fatal("README.md has no Configuration section")
	}
	para, _, _ := strings.Cut(strings.TrimLeft(after, "\n"), "\n\n")
	for _, want := range []string{
		"next to the exe",
		`%LOCALAPPDATA%\burnmon\burnmon.json`,
		"next to the exe wins",
		"`agent_view`",
		"`foreman_root`",
		"`claude_path`",
	} {
		if !strings.Contains(para, want) {
			t.Errorf("Configuration first paragraph lacks %q", want)
		}
	}
}

// TestReadmeWhichAppSectionAndCloudWording guards the "Which app do I open?"
// section (OF29) and the cloud timing wording (OF28).
func TestReadmeWhichAppSectionAndCloudWording(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := strings.ReplaceAll(string(b), "\r\n", "\n")
	const head = "\n## Which app do I open?\n"
	at := strings.Index(readme, head)
	shots := strings.Index(readme, "\n## Screenshots\n")
	if at < 0 || shots < 0 || at > shots {
		t.Fatal("README.md needs '## Which app do I open?' before '## Screenshots'")
	}
	if intro := strings.Index(readme, "BurnMon is the successor of"); intro < 0 || intro > at {
		t.Error("section must come after the intro")
	}
	section := strings.TrimSpace(readme[at+len(head) : shots])
	for _, want := range []string{"burnmon.exe", "burnmon-dev.exe", "history", "Now page",
		"the last 30 minutes", "Waits on you", "agent map"} {
		if !strings.Contains(section, want) {
			t.Errorf("section lacks %q", want)
		}
	}
	const settings = "Settings live in `burnmon.json`, next to the exe or else in `%LOCALAPPDATA%\\burnmon\\burnmon.json`; see Configuration."
	if !strings.Contains(section, settings) {
		t.Errorf("section lacks %q", settings)
	}
	const last = "BurnMon Dev reads every Claude Code, Codex and Copilot session on this machine, wherever it runs, and shows which ones are waiting on you."
	if !strings.HasSuffix(section, last) {
		t.Errorf("section must end with %q", last)
	}
	if strings.Contains(strings.ToLower(section), "phone") {
		t.Error("section mentions phone, but no phone switch exists")
	}
	for _, gone := range []string{"about 10 seconds after it is written", "1 to 3 minutes"} {
		if strings.Contains(readme, gone) {
			t.Errorf("README.md still contains %q", gone)
		}
	}
	const approved = "The plugin writes within about 20 seconds; the copy to your laptop took about 3 minutes in one measurement on 2026-11-02."
	if !strings.Contains(readme, approved) {
		t.Errorf("README.md lacks %q", approved)
	}
	if !strings.Contains(readme, "cloud data N min old") {
		t.Error("README.md does not name the label `cloud data N min old`")
	}
	if !strings.Contains(readme, "\n### macOS and Linux\n") {
		t.Error("macOS heading is gone")
	}
}
