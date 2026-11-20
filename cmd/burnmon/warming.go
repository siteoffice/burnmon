package main

import (
	"errors"
	"html"
	"strings"
	"time"
)

// firstCollectionRetry is how long the first collection waits before it looks
// again, when it found nothing to read. A user who installs BurnMon before
// the first Claude or Codex session sees the warming page say so, and the
// dashboard comes up by itself once a session folder exists.
const firstCollectionRetry = 30 * time.Second

// firstCollection runs rebuild until it succeeds. Each failure is reported
// through show as the text for the warming page, but only when the text
// differs from the one shown last, so the window is not reset on every try.
// A rebuild that is already running elsewhere is not a failure: it is waited
// out. sleep is time.Sleep in the apps and a recorder in the tests.
func firstCollection(rebuild func() error, show func(msg string), every time.Duration, sleep func(time.Duration)) error {
	last := ""
	for {
		err := rebuild()
		if err == nil {
			return nil
		}
		if !errors.Is(err, errRebuildBusy) {
			if msg := startupFailureMessage(err); msg != last {
				last = msg
				show(msg)
			}
		}
		sleep(every)
	}
}

// sourceFoldersLine names, on one line, the folders BurnMon reads on goos.
// It is written out here, not built from the folders that exist, because the
// point is to show where to look when none exists.
func sourceFoldersLine(goos string) string {
	switch goos {
	case "windows":
		return `BurnMon looks in: %USERPROFILE%\.claude\projects (Claude Code), %APPDATA%\Claude (Claude Desktop and Cowork), %USERPROFILE%\.codex\sessions (Codex).`
	case "darwin":
		return "BurnMon looks in: ~/.claude/projects (Claude Code), ~/Library/Application Support/Claude (Claude Desktop and Cowork), ~/.codex/sessions (Codex)."
	default:
		return "BurnMon looks in: ~/.claude/projects (Claude Code), ~/.config/Claude (Claude Desktop and Cowork), ~/.codex/sessions (Codex)."
	}
}

const warmingStartingDiv = `<div id="cc_warm_text" style="white-space:pre-line;margin-top:8px;font-size:13px;color:#9fb4bd">Starting&hellip;</div>`

// warmingFailPageHTML is the warming page with msg already in place of
// "Starting...". The native windows set this page rather than Eval ccWarmFail
// into the one on screen: an Eval that arrives before the first page has
// loaded is lost, and a first collection with no source folder fails within
// milliseconds.
func warmingFailPageHTML(msg string) string {
	page := warmingPageHTML()
	fail := `<div id="cc_warm_text" style="white-space:pre-line;margin-top:8px;font-size:13px;color:#ff8a65">` + html.EscapeString(msg) + `</div>`
	page = strings.Replace(page, warmingStartingDiv, fail, 1)
	return strings.Replace(page, `background:#FFDD32;transition`, `background:#ff8a65;transition`, 1)
}
