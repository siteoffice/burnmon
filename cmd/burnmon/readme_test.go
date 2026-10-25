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
