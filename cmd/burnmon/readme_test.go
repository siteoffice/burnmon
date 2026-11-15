package main

import (
	"os"
	"strings"
	"testing"
)

// The README is one file per part (2026-11-15): README.md is the index with
// "Which app do I open?", and each app has its own README under cmd\.

// readDoc returns a README with CRLF replaced, lines kept (for heading cuts).
func readDoc(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// flat joins a text into single-spaced words, so a sentence the README wraps
// over two lines still matches.
func flat(s string) string { return strings.Join(strings.Fields(s), " ") }

var readmeFiles = []string{
	"../../README.md",
	"../burnmon/README.md",
	"../burnmon-dev/README.md",
	"../controlcentre/README.md",
	"../burnmon-cli/README.md",
}

// TestReadmeNamesKeepHistoryAndWhatNeedsBurnMon is the doc guard for B1: the
// BurnMon README says what needs BurnMon and what keep_history does, and no
// README carries an em dash.
func TestReadmeNamesKeepHistoryAndWhatNeedsBurnMon(t *testing.T) {
	readme := flat(readDoc(t, "README.md"))
	for _, want := range []string{
		"## What needs BurnMon",
		"keep_history",
		"deleting a transcript does not delete BurnMon's copy",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("cmd\\burnmon\\README.md lacks %q", want)
		}
	}
	for _, f := range readmeFiles {
		if strings.ContainsRune(readDoc(t, f), 0x2014) {
			t.Errorf("%s contains an em dash", f)
		}
	}
}

// TestReadmeConfigurationNamesBothLocationsAndNewKeys guards the Settings
// first paragraph of the BurnMon README and the stale "not listed yet" phrases.
func TestReadmeConfigurationNamesBothLocationsAndNewKeys(t *testing.T) {
	readme := readDoc(t, "README.md")
	for _, gone := range []string{"does not list them yet", "not in it yet", "read-only-share install"} {
		if strings.Contains(readme, gone) {
			t.Errorf("README still contains %q", gone)
		}
	}
	_, after, ok := strings.Cut(readme, "\n## 5. Settings\n")
	if !ok {
		t.Fatal("cmd\\burnmon\\README.md has no '## 5. Settings' section")
	}
	para, _, _ := strings.Cut(strings.TrimLeft(after, "\n"), "\n\n")
	para = flat(para)
	for _, want := range []string{
		"next to the exe",
		`%LOCALAPPDATA%\burnmon\burnmon.json`,
		"next to the exe wins",
		"`agent_view`",
		"`foreman_root`",
		"`claude_path`",
	} {
		if !strings.Contains(para, want) {
			t.Errorf("Settings first paragraph lacks %q", want)
		}
	}
}

// TestReadmeWhichAppSectionAndCloudWording guards the "Which app do I open?"
// section of the index (OF29) and the cloud timing wording (OF28).
func TestReadmeWhichAppSectionAndCloudWording(t *testing.T) {
	root := readDoc(t, "../../README.md")
	const head = "\n## Which app do I open?\n"
	at := strings.Index(root, head)
	shots := strings.Index(root, "\n## Screenshots\n")
	if at < 0 || shots < 0 || at > shots {
		t.Fatal("README.md needs '## Which app do I open?' before '## Screenshots'")
	}
	if intro := strings.Index(root, "BurnMon is the successor of"); intro < 0 || intro > at {
		t.Error("section must come after the intro")
	}
	section := strings.TrimSpace(root[at+len(head) : shots])
	for _, want := range []string{"burnmon.exe", "burnmon-dev.exe", "history", "Now page",
		"the last 30 minutes", "Waits on you", "agent map", "| **Control Centre** (`controlcentre.exe`)"} {
		if !strings.Contains(section, want) {
			t.Errorf("section lacks %q", want)
		}
	}
	const settings = "Settings live in `burnmon.json`, next to the exe or else in `%LOCALAPPDATA%\\burnmon\\burnmon.json`; see `cmd\\burnmon\\README.md`, section 5."
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
	app := readDoc(t, "README.md")
	appFlat := flat(app)
	for _, f := range readmeFiles {
		all := flat(readDoc(t, f))
		for _, gone := range []string{"about 10 seconds after it is written", "1 to 3 minutes"} {
			if strings.Contains(all, gone) {
				t.Errorf("%s still contains %q", f, gone)
			}
		}
	}
	const approved = "The plugin writes within about 20 seconds; the copy to your laptop took about 3 minutes in one measurement on 2026-11-02."
	if !strings.Contains(appFlat, approved) {
		t.Errorf("cmd\\burnmon\\README.md lacks %q", approved)
	}
	if !strings.Contains(appFlat, "cloud data N min old") {
		t.Error("cmd\\burnmon\\README.md does not name the label `cloud data N min old`")
	}
	if !strings.Contains(app, "\n### macOS and Linux\n") {
		t.Error("macOS heading is gone")
	}
}

// TestReadmeControlCentre guards the Control Centre README (slice 1, item 8)
// and its row in "Which app do I open?".
func TestReadmeControlCentre(t *testing.T) {
	section := flat(readDoc(t, "../controlcentre/README.md"))
	for _, want := range []string{"controlcentre.exe", "--root", "controlcentre.json", "_comments",
		"Approve", "Send back",
		"Decisions need BurnMon Dev running, because the gate and agent checks read its snapshot.",
		"Double-click", "Back", "mouse", "Alt+Left", "waits on you only when its gate is open"} {
		if !strings.Contains(section, want) {
			t.Errorf("Control Centre README lacks %q", want)
		}
	}
	root := readDoc(t, "../../README.md")
	wat := strings.Index(root, "\n## Which app do I open?\n")
	shots := strings.Index(root, "\n## Screenshots\n")
	if wat < 0 || shots < wat {
		t.Fatal("README.md needs '## Which app do I open?' before '## Screenshots'")
	}
	if !strings.Contains(root[wat:shots], "| **Control Centre** (`controlcentre.exe`)") {
		t.Error("Which app do I open? lacks the Control Centre row")
	}
}

// TestReadmeEightSections guards the shared template: every part README has
// the same eight numbered sections in the same order.
func TestReadmeEightSections(t *testing.T) {
	heads := []string{"## 1. What it is", "## 2. Where it fits", "## 3. Install", "## 4. Use",
		"## 5. Settings", "## 6. Limits and known issues", "## 7. Next steps and backlog",
		"## 8. Feedback, licence, contact"}
	for _, f := range readmeFiles[1:] {
		doc := readDoc(t, f)
		pos := -1
		for _, h := range heads {
			i := strings.Index(doc, "\n"+h+"\n")
			if i < 0 || i < pos {
				t.Errorf("%s: heading %q missing or out of order", f, h)
				continue
			}
			pos = i
		}
	}
}
