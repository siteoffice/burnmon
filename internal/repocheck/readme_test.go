package repocheck

import (
	"os"
	"strings"
	"testing"
)

func TestReadmeMacBannerLine(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	i := strings.Index(text, "\n## BurnMon Dev")
	if i < 0 {
		t.Fatal("no BurnMon Dev section")
	}
	sec := text[i+1:]
	if j := strings.Index(sec[3:], "\n## "); j >= 0 {
		sec = sec[:j+3]
	}
	const want = "On macOS the banner is attributed to Script Editor and a click does nothing."
	if !strings.Contains(sec, want) {
		t.Errorf("BurnMon Dev section lacks %q", want)
	}
}
