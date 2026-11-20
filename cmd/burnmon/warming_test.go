package main

import (
	"strings"
	"testing"
	"time"

	"burnmon/internal/dataset"
)

// The start error must reach the warming page in every window kind (WebView2,
// the Mac web view, browser mode). The three share startupFailureMessage and
// the warming page, so these tests pin both.

func TestStartupFailureMessageNoSourcesNamesToolsAndFolders(t *testing.T) {
	msg := startupFailureMessage(dataset.ErrNoSources)
	for _, want := range []string{"Claude Code", "Codex", "BurnMon looks in:", ".claude", ".codex"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
}

func TestErrNoSourcesMessageNamesOtherTools(t *testing.T) {
	msg := dataset.ErrNoSources.Error()
	for _, want := range []string{"Claude", "Codex"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrNoSources text %q lacks %q", msg, want)
		}
	}
}

func TestSourceFoldersLinePerOS(t *testing.T) {
	cases := map[string][]string{
		"darwin":  {"~/.claude/projects", "~/.codex/sessions", "~/Library/Application Support/Claude"},
		"linux":   {"~/.claude/projects", "~/.codex/sessions", "~/.config/Claude"},
		"windows": {`%USERPROFILE%\.claude\projects`, `%USERPROFILE%\.codex\sessions`, `%APPDATA%\Claude`},
	}
	for goos, wants := range cases {
		line := sourceFoldersLine(goos)
		if strings.Contains(line, "\n") {
			t.Errorf("%s: the folders line must be one line: %q", goos, line)
		}
		for _, want := range wants {
			if !strings.Contains(line, want) {
				t.Errorf("%s: line lacks %q: %s", goos, want, line)
			}
		}
	}
	if strings.Contains(sourceFoldersLine("darwin"), `\`) {
		t.Error("darwin line holds a backslash path")
	}
}

// A window that SetHtml()s the failure page cannot lose the message to a page
// that has not loaded yet, which a later Eval of ccWarmFail could.
func TestWarmingFailPageShowsMessageInPlaceOfStarting(t *testing.T) {
	msg := "No sessions here <b>&</b>\nBurnMon looks in: ~/.claude/projects"
	page := warmingFailPageHTML(msg)
	if !strings.Contains(page, "No sessions here &lt;b&gt;&amp;&lt;/b&gt;") {
		t.Errorf("message missing or not escaped:\n%s", page)
	}
	if !strings.Contains(page, "BurnMon looks in: ~/.claude/projects") {
		t.Error("folders line missing")
	}
	if strings.Contains(page, "Starting&hellip;") {
		t.Error("the failure page still says Starting")
	}
	if !strings.Contains(page, "white-space:pre-line") {
		t.Error("the text element does not keep the line break")
	}
	if !strings.Contains(page, "function ccWarmFail") {
		t.Error("browser mode needs ccWarmFail on the same page")
	}
}

func TestWarmingPageKeepsLineBreakForCcWarmFail(t *testing.T) {
	page := warmingPageHTML()
	if !strings.Contains(page, `id="cc_warm_text" style="white-space:pre-line;`) {
		t.Errorf("cc_warm_text must keep the line break of a failure message:\n%s", page)
	}
}

func TestFirstCollectionRetriesUntilASourceAppears(t *testing.T) {
	var calls int
	var shown []string
	var slept []time.Duration
	rebuild := func() error {
		calls++
		if calls <= 3 {
			return dataset.ErrNoSources
		}
		return nil
	}
	err := firstCollection(rebuild,
		func(msg string) { shown = append(shown, msg) },
		30*time.Second,
		func(d time.Duration) { slept = append(slept, d) })
	if err != nil {
		t.Fatalf("firstCollection = %v, want nil after the 4th try", err)
	}
	if calls != 4 {
		t.Errorf("rebuild called %d times, want 4", calls)
	}
	if len(slept) != 3 || slept[0] != 30*time.Second {
		t.Errorf("slept %v, want three waits of 30s", slept)
	}
	// The same failure three times is shown once: the window is not reset
	// every 30 seconds.
	if len(shown) != 1 || shown[0] != startupFailureMessage(dataset.ErrNoSources) {
		t.Errorf("shown %q, want the no-sources message once", shown)
	}
}

func TestFirstCollectionShowsAChangedFailure(t *testing.T) {
	errs := []error{dataset.ErrNoSources, dataset.ErrNoFiles, nil}
	i := 0
	var shown []string
	_ = firstCollection(func() error { e := errs[i]; i++; return e },
		func(msg string) { shown = append(shown, msg) },
		time.Second, func(time.Duration) {})
	if len(shown) != 2 || shown[0] == shown[1] {
		t.Errorf("shown %q, want two different messages", shown)
	}
}

func TestFirstCollectionDoesNotShowAnythingOnFirstSuccess(t *testing.T) {
	shown := 0
	err := firstCollection(func() error { return nil }, func(string) { shown++ }, time.Second, func(time.Duration) {})
	if err != nil || shown != 0 {
		t.Errorf("err %v shown %d, want nil and 0", err, shown)
	}
}

func TestFirstCollectionBusyIsNotAFailure(t *testing.T) {
	// A rebuild already in flight (the interval tick) is neither shown nor
	// ends the wait: the loop tries again.
	n := 0
	var shown []string
	err := firstCollection(func() error {
		n++
		if n == 1 {
			return errRebuildBusy
		}
		return nil
	}, func(m string) { shown = append(shown, m) }, time.Second, func(time.Duration) {})
	if err != nil || len(shown) != 0 || n != 2 {
		t.Errorf("err %v shown %q calls %d", err, shown, n)
	}
}
