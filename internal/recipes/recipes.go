// Package recipes is a Go copy of the five git recipes of snip.py (Groundwork
// engine, v0.7.0): save, share, land, tag and sync. For one request it returns
// the snippet block (what snip.py prints, byte for byte) and the argv list of
// each body line, so a program can run the lines without a shell.
// TestRecipesMatchSnip reads snip.py and fails when this copy drifts.
package recipes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"
)

// Names are the recipes in snip.py's order.
var Names = []string{"save", "share", "land", "tag", "sync"}

// DefaultExpect is the expect line of each recipe when none is given.
var DefaultExpect = map[string]string{
	"save":  "one new commit, and git status --short no longer lists the saved files",
	"share": "the branch is on origin and git status -sb shows no ahead count",
	"land":  "the engine reports the pull request merged and the work branch removed",
	"tag":   "git ls-remote prints one line ending in refs/tags/%s",
	"sync":  "git status -sb shows the branch level with its upstream",
}

// DefaultRunsIn is the "runs in" header line per system.
var DefaultRunsIn = map[string]string{
	"win": "PowerShell 7 on Windows, normal window",
	"mac": "Terminal on the Mac (zsh)",
}

// DefaultNeeds is the "needs" header line when none is given.
const DefaultNeeds = "nothing"

const (
	headerRunsIn = "# runs in: "
	headerCwd    = "# cwd: "
	headerNeeds  = "# needs: "
	recipePrefix = "# recipe: git "
	expectPrefix = "# expect: "
)

// BodyLiterals are the command texts of the recipe bodies. The drift test
// checks that each one still appears in snip.py.
var BodyLiterals = []string{
	"git push -u origin HEAD", "git status -sb", "git status --short",
	"git fetch --prune origin", "git merge --ff-only ", "git add -- ", "git commit -m ",
	"git tag -a ", " -m ", "git push origin ", "git ls-remote --tags origin ",
}

// Request is one recipe call. An empty field is a field not given.
type Request struct {
	Recipe string
	OS     string // "win" or "mac"; empty means this system
	Cwd    string
	RunsIn string
	Needs  string
	Expect string

	Files    []string // save
	Message  string   // save, tag
	Body     string   // save
	CoAuthor string   // save
	Tag      string   // tag
	Subject  string   // land
	Engine   string   // land: the full path of git_flow.py
}

// Tools are the programs argv lines start with. Git and Python are resolved
// absolute paths. DirExists checks the cwd on this system; nil skips the check.
type Tools struct {
	Git, Python string
	DirExists   func(string) bool
}

// Plan is what a recipe call produces. Argv lines run in Dir.
type Plan struct {
	Block string
	Argv  [][]string
	Dir   string
}

// HostOS is "win" on Windows and "mac" elsewhere, as in snip.py. It is a
// variable so a test can reach the other branch without patching runtime.
var HostOS = func() string {
	if runtime.GOOS == "windows" {
		return "win"
	}
	return "mac"
}()

// Refusal is a call the program will not turn into a block.
type Refusal struct{ msg string }

func (r *Refusal) Error() string { return r.msg }

func refuse(format string, a ...any) error { return &Refusal{fmt.Sprintf(format, a...)} }

// IsRefusal reports whether err is a Refusal.
func IsRefusal(err error) bool {
	var r *Refusal
	return errors.As(err, &r)
}

const emDash = "\u2014"

var (
	fenceRun   = regexp.MustCompile("`{3,}|~{3,}")
	winAbsRe   = regexp.MustCompile(`^[A-Za-z]:\\`)
	coAuthorRe = regexp.MustCompile(`^[^<>]+ <[^<>\s]+@[^<>\s]+>$`)
	// PowerShell reads all five of these as a single quote, so each is doubled.
	psQuotes = []string{"'", "\u2018", "\u2019", "\u201a", "\u201b"}
)

func quote(target, v string) string {
	if target == "win" {
		for _, ch := range psQuotes {
			v = strings.ReplaceAll(v, ch, ch+ch)
		}
		return "'" + v + "'"
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func checkChars(name, v string) error {
	for _, ch := range v {
		if (ch < 32 && ch != '\t') || (ch >= 0x80 && ch <= 0x9F) {
			return refuse("%s holds a control character (U+%04X)", name, ch)
		}
	}
	return nil
}

func oneLine(name, v string) error {
	switch {
	case strings.TrimSpace(v) == "":
		return refuse("%s is empty", name)
	case strings.ContainsAny(v, "\n\r"):
		return refuse("%s holds a line break", name)
	case strings.Contains(v, emDash):
		return refuse("%s holds an em dash (U+2014)", name)
	}
	if err := checkChars(name, v); err != nil {
		return err
	}
	if fenceRun.MatchString(v) {
		return refuse("%s holds three or more backticks or tildes, which could close a fence", name)
	}
	return nil
}

// checkCwd returns the folder to run in on this system ("" when the cwd is for
// the other system).
func checkCwd(target, path string, t Tools) (string, error) {
	if err := oneLine("--cwd", path); err != nil {
		return "", err
	}
	if target == "win" {
		if !winAbsRe.MatchString(path) {
			return "", refuse(`--cwd must be a Windows path like X:\... for --os win`)
		}
	} else {
		if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "~/") {
			return "", refuse("--cwd must start with / or ~/ for --os mac")
		}
		if strings.HasPrefix(path, "~/") && strings.ContainsAny(path, "\"$`\\!") {
			return "", refuse("--cwd with ~/ must not hold a double quote, $, !, a backtick or a backslash")
		}
	}
	if target != HostOS {
		return "", nil
	}
	real := path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", refuse("--cwd needs a home folder: %v", err)
		}
		real = filepath.Join(home, path[2:])
	}
	if t.DirExists != nil && !t.DirExists(real) {
		return "", refuse("--cwd folder not found: %s", path)
	}
	return real, nil
}

func cwdCommand(target, path string) string {
	if target == "win" {
		return "Set-Location -LiteralPath " + quote("win", path)
	}
	if strings.HasPrefix(path, "~/") {
		return `cd -- "$HOME/` + path[2:] + `"`
	}
	return "cd -- " + quote("mac", path)
}

func commandBlock(target string, r Request, recipe string, body []string, expect, dir string) (Plan, error) {
	runsIn, needs := DefaultRunsIn[target], DefaultNeeds
	if r.RunsIn != "" {
		if err := oneLine("--runs-in", r.RunsIn); err != nil {
			return Plan{}, err
		}
		runsIn = r.RunsIn
	}
	if r.Needs != "" {
		if err := oneLine("--needs", r.Needs); err != nil {
			return Plan{}, err
		}
		needs = r.Needs
	}
	if err := oneLine("--expect", expect); err != nil {
		return Plan{}, err
	}
	fence := map[string]string{"win": "powershell", "mac": "sh"}[target]
	lines := []string{"```" + fence, headerRunsIn + runsIn, headerCwd + r.Cwd, headerNeeds + needs,
		recipePrefix + recipe, cwdCommand(target, r.Cwd)}
	lines = append(lines, body...)
	lines = append(lines, expectPrefix+expect, "```")
	return Plan{Block: strings.Join(lines, "\n") + "\n", Dir: dir}, nil
}

func utf8Len(s string) int { return utf8.RuneCountInString(s) }
