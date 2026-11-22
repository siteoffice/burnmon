package main

import (
	"os"
	"path/filepath"
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
	// The burnmon-cloud plugin is retired (2026-11-17): the README says so and
	// no longer sends the reader to install or connect it.
	if !strings.Contains(appFlat, "The `burnmon-cloud` plugin is retired (2026-11-17).") {
		t.Error("cmd\\burnmon\\README.md does not say the burnmon-cloud plugin is retired")
	}
	for _, gone := range []string{"Set it up once:", "Install the plugin \"BurnMon cloud\"", "Start BurnMon once. It creates"} {
		if strings.Contains(appFlat, gone) {
			t.Errorf("cmd\\burnmon\\README.md still tells the reader to %q", gone)
		}
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

// firstHeadingAt returns the byte offset of the first line that starts with #.
func firstHeadingAt(doc string) int {
	if strings.HasPrefix(doc, "#") {
		return 0
	}
	return strings.Index(doc, "\n#")
}

// TestReadmesShowTheLampDemoOnTop is the doc guard for lane scope X1: both
// READMEs show glim-demo.svg above their first heading, by a relative link
// that resolves to the file the lamp's --poses writes.
func TestReadmesShowTheLampDemoOnTop(t *testing.T) {
	for _, tc := range []struct{ file, link string }{
		{"../../README.md", "assets/glim/glim-demo.svg"},
		{"../burnmon-dev/README.md", "../../assets/glim/glim-demo.svg"},
	} {
		doc := readDoc(t, tc.file)
		at := strings.Index(doc, "]("+tc.link+")")
		if at < 0 {
			t.Errorf("%s has no relative image link to %s", tc.file, tc.link)
			continue
		}
		if !strings.HasPrefix(doc, "![") {
			t.Errorf("%s does not start with the demo picture", tc.file)
		}
		if h := firstHeadingAt(doc); h < 0 || at > h {
			t.Errorf("%s: the demo picture is not above the first heading", tc.file)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(tc.file), tc.link)); err != nil {
			t.Errorf("%s links %s, which does not resolve: %v", tc.file, tc.link, err)
		}
	}
}

// TestReadmeLampSection is the doc guard for spec item 7 and X6.
func TestReadmeLampSection(t *testing.T) {
	root := readDoc(t, "../../README.md")
	table, _, _ := strings.Cut(root, "## Screenshots")
	if !strings.Contains(table, "burnmon-dev.exe lamp") || !strings.Contains(flat(table), "`cmd\\burnmon-dev\\README.md`") {
		t.Error("README.md: Which app do I open? has no row for the lamp")
	}
	dev := readDoc(t, "../burnmon-dev/README.md")
	_, sec, ok := strings.Cut(dev, "\n## The lamp\n")
	if !ok {
		t.Fatal("cmd\\burnmon-dev\\README.md has no '## The lamp' section")
	}
	if i := strings.Index(sec, "\n## "); i >= 0 {
		sec = sec[:i]
	}
	sec = flat(sec)
	for _, want := range []string{
		"burnmon-dev.exe lamp --demo", "lamp_enabled", "Do this, Glim does that",
		"This build has no lamp window. The lamp runs in the Windows build and in the Mac app (the build with cgo).",
		"needs BurnMon or BurnMon Dev running", "no control",
		"### Suggest a line", "glim-line.yml", "internal\\lamp\\CREDITS.md",
		"lamp --poses", "lamp --card", "lamp-waits.json",
		"### Goal", "### Where it fits", "### Install", "### Use", "### Settings", "### Next steps and backlog",
		"slim ticker", "48 second", "built, not seen", "BURNMON_LAMP_PARENT", "lamp_backdrop",
		// the glim fix lane: size as a percentage, the monitor, drag, grab and swing, eyes, close
		"100 to 200", "`lamp_monitor`", "Back to the primary screen", "Drag the ticker", "Grab Glim",
		"Don't do that", "his eyes follow the pointer", "Close the lamp", "never stored", "What is seen",
	} {
		if !strings.Contains(sec, want) {
			t.Errorf("the lamp section lacks %q", want)
		}
	}
	for _, mood := range []string{"asleep", "working", "your turn", "blocked", "bored", "sweating", "done"} {
		if !strings.Contains(sec, mood) {
			t.Errorf("the table Do this, Glim does that lacks the mood %q", mood)
		}
	}
	// The table of settings names every lamp key, with its default.
	for _, key := range []string{"lamp_enabled", "lamp_position", "lamp_language", "lamp_lines", "lamp_foreman_items",
		"lamp_burn_floor_tpm", "lamp_ticker_rows", "lamp_size", "lamp_theme", "lamp_motion", "lamp_quiet", "lamp_backdrop"} {
		if !strings.Contains(sec, "| `"+key+"` |") {
			t.Errorf("the settings table has no row for %s", key)
		}
	}
	for _, act := range []string{"alarm", "knock", "stamp", "welcome", "wake", "slip", "coffee", "peek"} {
		if !strings.Contains(sec, "`"+act+"`") {
			t.Errorf("the lamp section does not name the act %s", act)
		}
	}
	if strings.Contains(readDoc(t, "../../README.md")+readDoc(t, "../burnmon-dev/README.md"), "24 second") {
		t.Error("a README still says the demo loop is 24 seconds")
	}
}

// TestLampLicenceCreditsAndIssueForm guards LICENSE-ASSETS.md, CREDITS.md and
// the glim-line issue form (X6), and that none of them has an em dash.
func TestLampLicenceCreditsAndIssueForm(t *testing.T) {
	lic := flat(readDoc(t, "../../LICENSE-ASSETS.md"))
	for _, want := range []string{
		"MIT", "Glim", "Copyright 2026 Wilco de Tree, ZeroNonsense.dev", "all rights reserved",
		"assets\\glim\\", "internal\\lamp", "own name", "internal\\lamp\\CREDITS.md",
	} {
		if !strings.Contains(lic, want) {
			t.Errorf("LICENSE-ASSETS.md lacks %q", want)
		}
	}
	credits := flat(readDoc(t, "../../internal/lamp/CREDITS.md"))
	if !strings.Contains(credits, "MIT") || !strings.Contains(credits, "credit") {
		t.Error("internal\\lamp\\CREDITS.md does not say MIT and how a credit is given")
	}
	form := readDoc(t, "../../.github/ISSUE_TEMPLATE/glim-line.yml")
	for _, want := range []string{"id: mood", "id: line_en", "id: line_nl", "id: credit", "labels: [\"glim-line\"]"} {
		if !strings.Contains(form, want) {
			t.Errorf("glim-line.yml lacks %q", want)
		}
	}
	for name, doc := range map[string]string{"LICENSE-ASSETS.md": lic, "CREDITS.md": credits, "glim-line.yml": form} {
		if strings.ContainsRune(doc, 0x2014) {
			t.Errorf("%s contains an em dash", name)
		}
	}
}
