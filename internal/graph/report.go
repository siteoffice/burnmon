package graph

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// maxReportBytes caps how much of a sprint's report.md is read.
const maxReportBytes = 1 << 20

var (
	reportFence = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	reportItem  = regexp.MustCompile(`^-\s+(\S+)\s+([A-Za-z]+):\s*(.*)$`)
)

// classStates lists the states each class may carry, as Foreman's close_out.py
// does. Only ASK and HAND take "parked"; a line with another state is not an
// item.
var classStates = map[string][]string{
	"DO":   {"done"},
	"ASK":  {"answered", "parked", "deferred"},
	"HAND": {"handed", "parked", "deferred"},
	"DROP": {"dropped"},
}

// ParkedLines counts the parked lines of the first "## Close-out" block of a
// report, the way Foreman's close_out.parked_items does: fenced code is
// ignored, the block runs to the next "## " line, an item is
// "- <CLASS> <state>: <text>", and a line indented after an item continues it.
// Foreman refuses to close a sprint that has any (an Unattended sprint ends
// waiting-on-you; an Attended one fails the gate), so BurnMon shows no Close
// chip for it.
func ParkedLines(text string) int {
	type line struct {
		s      string
		fenced bool
	}
	var lines []line
	var fenceChar byte
	fenceLen := 0
	for _, s := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		m := reportFence.FindStringSubmatch(s)
		if fenceLen == 0 {
			if m != nil {
				fenceChar, fenceLen = m[1][0], len(m[1])
			}
			lines = append(lines, line{s, m != nil})
			continue
		}
		lines = append(lines, line{s, true})
		if m != nil && m[1][0] == fenceChar && len(m[1]) >= fenceLen && strings.TrimSpace(s) == m[1] {
			fenceLen = 0
		}
	}
	start := -1
	n := 0
	afterItem := false
	for _, l := range lines {
		if l.fenced {
			continue
		}
		if start < 0 {
			if strings.EqualFold(strings.TrimSpace(l.s), "## close-out") {
				start = 1
			}
			continue
		}
		if strings.HasPrefix(l.s, "## ") {
			break
		}
		t := strings.TrimSpace(l.s)
		switch {
		case t == "":
			continue
		case strings.HasPrefix(t, "OPEN:"):
			afterItem = false
			continue
		case (l.s[0] == ' ' || l.s[0] == '\t') && afterItem:
			continue
		}
		afterItem = false
		if strings.EqualFold(t, "- none") {
			continue
		}
		m := reportItem.FindStringSubmatch(l.s)
		if m == nil {
			continue
		}
		state := strings.ToLower(m[2])
		if !slices.Contains(classStates[m[1]], state) {
			continue
		}
		afterItem = true
		if state == "parked" {
			n++
		}
	}
	return n
}

// readReport reads <dir>\report.md, the file Foreman's close-out gate checks,
// and only a regular file inside the sprint folder (no link). It returns the
// path and the parked line count; path is "" when there is no such file.
func readReport(dir string) (path string, parked int) {
	p := filepath.Join(dir, "report.md")
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return "", 0
	}
	f, err := os.Open(p)
	if err != nil {
		return "", 0
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxReportBytes))
	if err != nil {
		return "", 0
	}
	return p, ParkedLines(strings.TrimPrefix(string(b), "\ufeff"))
}
