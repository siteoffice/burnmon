package graph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ReasonNotMirrored is the verdict reason when a check of Foreman's close-out
// gate cannot be reproduced here, so BurnMon shows no Close chip.
const ReasonNotMirrored = "Foreman's close-out check could not be mirrored"

// maxActionLogBytes caps how much of action-log.ndjson is read, and
// maxSprintJSONBytes how much of sprint.json.
const (
	maxActionLogBytes  = 8 << 20
	maxSprintJSONBytes = 1 << 20
)

var (
	headingRe = regexp.MustCompile(`^ {0,3}#{1,6}(?:\s|$)`)
	becauseRe = regexp.MustCompile(`(?i)because`)
)

// bannedHeadingWords are close_out.py's BANNED_HEADING_WORDS.
var bannedHeadingWords = []string{"residual", "follow-up", "follow up", "next step",
	"open item", "open point", "decisions for", "todo"}

// notMirrorable reports whether the text holds a character close_out.py treats
// differently from this port: Python splitlines() breaks on more characters
// than \n, and its str.strip(), \s and isspace() cover Unicode and a few
// control characters that RE2's \s does not.
func notMirrorable(text string) bool {
	for _, r := range text {
		switch {
		case r == '\v' || r == '\f' || (r >= 0x1c && r <= 0x1f):
			return true
		case r > 0x7e && unicode.IsSpace(r), r == 0x85:
			return true
		}
	}
	return false
}

type reportLine struct {
	s      string
	fenced bool
}

// annotate is close_out._annotate: each line with a flag for fenced code.
func annotate(text string) []reportLine {
	var out []reportLine
	var fenceChar byte
	fenceLen := 0
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	for _, s := range strings.Split(text, "\n") {
		m := reportFence.FindStringSubmatch(s)
		if fenceLen == 0 {
			if m != nil {
				fenceChar, fenceLen = m[1][0], len(m[1])
			}
			out = append(out, reportLine{s, m != nil})
			continue
		}
		out = append(out, reportLine{s, true})
		if m != nil && m[1][0] == fenceChar && len(m[1]) >= fenceLen && strings.TrimSpace(s) == m[1] {
			fenceLen = 0
		}
	}
	return out
}

// blockBounds is close_out._block_bounds: the first unfenced "## Close-out"
// heading and the end of its section (exclusive).
func blockBounds(lines []reportLine) (start, end int, ok bool) {
	start = -1
	for i, l := range lines {
		if l.fenced {
			continue
		}
		if start < 0 {
			if strings.EqualFold(strings.TrimSpace(l.s), "## close-out") {
				start = i
			}
		} else if strings.HasPrefix(l.s, "## ") {
			return start, i, true
		}
	}
	if start < 0 {
		return 0, 0, false
	}
	return start, len(lines), true
}

func fenceFollows(lines []reportLine, j, end int) bool {
	for j < end && strings.TrimSpace(lines[j].s) == "" {
		j++
	}
	return j < end && lines[j].fenced && reportFence.MatchString(lines[j].s)
}

// hasWord reports whether text holds "because" as a whole word, ignoring
// case, with Python's \b: a word character is a letter, digit or underscore.
func hasWord(text string) bool {
	isWord := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	for _, ix := range becauseRe.FindAllStringIndex(text, -1) {
		before, _ := utf8.DecodeLastRuneInString(text[:ix[0]])
		after, _ := utf8.DecodeRuneInString(text[ix[1]:])
		if (ix[0] == 0 || !isWord(before)) && (ix[1] == len(text) || !isWord(after)) {
			return true
		}
	}
	return false
}

type closeItem struct {
	class, state, text string
}

// checkBlock is close_out._check_block.
func checkBlock(lines []reportLine, start, end int) (failures []string, items []closeItem) {
	nones := []int{}
	afterItem := false
	for i := start + 1; i < end; i++ {
		l := lines[i]
		n := i + 1
		if l.fenced || strings.TrimSpace(l.s) == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l.s), "OPEN:") {
			failures = append(failures, fmt.Sprintf("line %d: 'OPEN:' line in the Close-out block", n))
			afterItem = false
			continue
		}
		if (l.s[0] == ' ' || l.s[0] == '\t') && afterItem {
			continue
		}
		afterItem = false
		if strings.EqualFold(strings.TrimSpace(l.s), "- none") {
			nones = append(nones, n)
			continue
		}
		m := reportItem.FindStringSubmatch(l.s)
		if m == nil || classStates[m[1]] == nil {
			failures = append(failures, fmt.Sprintf("line %d: not a Close-out item: %s", n, strings.TrimSpace(l.s)))
			continue
		}
		cls, state, text := m[1], strings.ToLower(m[2]), m[3]
		valid := false
		for _, s := range classStates[cls] {
			valid = valid || s == state
		}
		if !valid {
			failures = append(failures, fmt.Sprintf("line %d: '%s %s' is not a valid Close-out line", n, cls, state))
			continue
		}
		items = append(items, closeItem{cls, state, text})
		afterItem = true
		if strings.TrimSpace(text) == "" {
			failures = append(failures, fmt.Sprintf("line %d: '%s %s' needs text after the colon", n, cls, state))
		}
		if cls == "ASK" && state == "answered" && !strings.Contains(text, "->") {
			failures = append(failures, fmt.Sprintf("line %d: ASK answered needs '<question> -> <answer>'", n))
		}
		if cls == "HAND" && state == "handed" && !strings.Contains(strings.ToLower(text), "verified by") {
			failures = append(failures, fmt.Sprintf("line %d: HAND handed needs 'verified by'", n))
		}
		if cls == "DROP" && !hasWord(text) {
			failures = append(failures, fmt.Sprintf("line %d: DROP dropped needs its reason, with 'because'", n))
		}
		if cls == "HAND" && state == "parked" && !fenceFollows(lines, i+1, end) {
			failures = append(failures, fmt.Sprintf("line %d: HAND parked must be followed by a fenced snippet", n))
		}
	}
	if len(nones) > 0 && (len(items) > 0 || len(nones) > 1) {
		for _, n := range nones {
			failures = append(failures, fmt.Sprintf("line %d: '- none' must be the only line in the Close-out block", n))
		}
	}
	if len(items) == 0 && len(nones) == 0 && len(failures) == 0 {
		failures = append(failures, "the Close-out block has no lines")
	}
	return failures, items
}

// checkOutside is close_out._check_outside.
func checkOutside(lines []reportLine, start, end int) []string {
	var failures []string
	for i, l := range lines {
		if l.fenced || (start <= i && i < end) {
			continue
		}
		n := i + 1
		if i > start && strings.EqualFold(strings.TrimSpace(l.s), "## close-out") {
			failures = append(failures, fmt.Sprintf("line %d: second '## Close-out' section", n))
			continue
		}
		if headingRe.MatchString(l.s) {
			low := strings.ToLower(l.s)
			for _, w := range bannedHeadingWords {
				if strings.Contains(low, w) {
					failures = append(failures, fmt.Sprintf("line %d: banned heading outside the Close-out block: %s", n, strings.TrimSpace(l.s)))
					break
				}
			}
		} else if strings.HasPrefix(strings.TrimLeftFunc(l.s, unicode.IsSpace), "OPEN:") {
			failures = append(failures, fmt.Sprintf("line %d: 'OPEN:' line outside the Close-out block", n))
		}
	}
	return failures
}

// deferredTouchpoints is close_out._deferred_touchpoints: the touchpoint_done
// entries of the action log whose detail starts "deferred:". ok is false when
// the log is larger than the read cap.
func deferredTouchpoints(path string) (n int, ok bool) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return 0, true
	}
	if fi.Size() > maxActionLogBytes {
		return 0, false
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, true
	}
	defer f.Close()
	r := bufio.NewReaderSize(io.LimitReader(f, maxActionLogBytes), 64<<10)
	for {
		line, err := r.ReadString('\n')
		var e struct {
			Action string `json:"action"`
			Detail any    `json:"detail"`
		}
		if json.Unmarshal([]byte(line), &e) == nil && e.Action == "touchpoint_done" {
			if d, isStr := e.Detail.(string); isStr && strings.HasPrefix(d, "deferred:") {
				n++
			}
		}
		if err != nil {
			return n, true
		}
	}
}

// CloseOutVerdict reports whether Foreman's close-out gate would accept this
// sprint folder on a move into phase done, by mirroring close_out.check on
// <dir>\report.md with the run mode of <dir>\sprint.json (attended when it is
// unknown, as Foreman does) and <dir>\action-log.ndjson, plus the Unattended
// rule that parked lines end the sprint waiting-on-you. Read only, bounded.
// ok is false with a reason when the gate would refuse, when there is no
// report, or when a check cannot be mirrored (ReasonNotMirrored). parked is
// the number of parked lines in the Close-out block.
func CloseOutVerdict(dir string) (ok bool, reason string, parked int) {
	p := filepath.Join(dir, "report.md")
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return false, "no report yet", 0
	}
	if fi.Size() > maxReportBytes {
		return false, ReasonNotMirrored, 0
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false, ReasonNotMirrored, 0
	}
	text := strings.TrimPrefix(string(b), "\ufeff")
	parked = ParkedLines(text)
	if parked > 0 {
		return false, fmt.Sprintf("%d parked %s", parked, map[bool]string{true: "line", false: "lines"}[parked == 1]), parked
	}
	if notMirrorable(text) || !utf8.ValidString(text) {
		return false, ReasonNotMirrored, 0
	}
	runMode, mirrored := sprintRunMode(dir)
	if !mirrored {
		return false, ReasonNotMirrored, 0
	}
	lines := annotate(text)
	start, end, found := blockBounds(lines)
	if !found {
		return false, "the report has no '## Close-out' section", 0
	}
	failures, items := checkBlock(lines, start, end)
	var deferred int
	for _, it := range items {
		switch {
		case runMode == "attended" && it.state == "parked":
			failures = append(failures, "'"+it.class+" parked' is not allowed in an Attended run")
		case runMode == "unattended" && it.state == "deferred":
			failures = append(failures, "'"+it.class+" deferred' is not allowed in an Unattended run")
		}
		if it.state == "deferred" {
			deferred++
		}
	}
	if runMode == "attended" && deferred > 0 {
		have, readable := deferredTouchpoints(filepath.Join(dir, "action-log.ndjson"))
		if !readable {
			return false, ReasonNotMirrored, 0
		}
		if have < deferred {
			failures = append(failures, fmt.Sprintf("an Attended run needs a touchpoint_done 'deferred:' log entry per deferred line (%d found for %d)", have, deferred))
		}
	}
	failures = append(failures, checkOutside(lines, start, end)...)
	if len(failures) > 0 {
		reason = "Foreman's close-out gate fails: " + failures[0]
		if len(failures) > 1 {
			reason += fmt.Sprintf(" (and %d more)", len(failures)-1)
		}
		return false, reason, 0
	}
	return true, "", 0
}

// sprintRunMode reads run_mode from <dir>\sprint.json: "unattended" or
// "attended", and attended for a missing or unknown value, as Foreman's
// _require_close_out does. ok is false when sprint.json cannot be read.
func sprintRunMode(dir string) (mode string, ok bool) {
	p := filepath.Join(dir, "sprint.json")
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxSprintJSONBytes {
		return "", false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	var raw struct {
		RunMode any `json:"run_mode"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return "", false
	}
	if s, isStr := raw.RunMode.(string); isStr && s == "unattended" {
		return "unattended", true
	}
	return "attended", true
}
