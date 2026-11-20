//go:build windows || darwin

package main

import (
	"regexp"
	"strings"
	"testing"
)

// D18: the Git page names a clean checkout at a tag "at tag <name>" in the
// row, the lead line, the hover list and the worktree rows, through one helper,
// and never prints "detached" for it. A detached checkout without a tag still
// reads "detached".
func TestGitPageNamesTheTagNotDetached(t *testing.T) {
	m := regexp.MustCompile(`function headWord\(x, long\) \{[^\n]*\n(?:[^\n]*\n){0,4}?\s*\}`).FindString(pageHTML)
	if m == "" {
		t.Fatal("page.html has no headWord(x, long) helper")
	}
	for _, want := range []string{"x.at_tag", "'at tag '", "'detached HEAD'", "'detached'"} {
		if !strings.Contains(m, want) {
			t.Errorf("headWord lacks %s:\n%s", want, m)
		}
	}
	uses := regexp.MustCompile(`\b[rw]\.detached \?`).FindAllString(pageHTML, -1)
	if len(uses) != 5 {
		t.Fatalf("page.html has %d uses of .detached ?, want the 5 of the Git page", len(uses))
	}
	for _, line := range strings.Split(pageHTML, "\n") {
		if regexp.MustCompile(`\b[rw]\.detached \?`).MatchString(line) && !strings.Contains(line, "headWord(") {
			t.Errorf("a Git page line prints detached without headWord: %s", strings.TrimSpace(line))
		}
	}
}
