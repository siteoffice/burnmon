package lamp

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// pageHarnessLabel reads the HARNESS_LABEL table of the BurnMon Dev page, so
// the Go copy cannot drift from the names the page shows.
func pageHarnessLabel(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile("../../cmd/burnmon-dev/page.html")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	i := strings.Index(src, "var HARNESS_LABEL = {")
	if i < 0 {
		t.Fatal("HARNESS_LABEL not found in page.html")
	}
	body := src[i:]
	body = body[:strings.Index(body, "};")]
	body = regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(body, "")
	got := map[string]string{}
	for _, m := range regexp.MustCompile(`'([^']+)':\s*'([^']+)'`).FindAllStringSubmatch(body, -1) {
		got[m[1]] = m[2]
	}
	if len(got) == 0 {
		t.Fatal("no entries parsed from HARNESS_LABEL")
	}
	return got
}

func TestVendorNamesEqualPageHarnessLabel(t *testing.T) {
	page := pageHarnessLabel(t)
	if len(page) != len(vendorNames) {
		t.Errorf("page has %d names, lamp has %d", len(page), len(vendorNames))
	}
	for k, v := range page {
		if vendorNames[k] != v {
			t.Errorf("vendor %q: page %q, lamp %q", k, v, vendorNames[k])
		}
	}
}

func TestWho(t *testing.T) {
	cases := []struct{ id, want string }{
		{"session:claude:abc", "Claude Code"},
		{"session:codex:x:y", "Codex"},
		{"session:copilot-vscode:1", "Copilot (VS Code)"},
		{"session:unknownvendor:1", "An agent"},
		{"session:claude", "An agent"},
		{"gate:flow-1:g2", "An agent"},
		{"", "An agent"},
	}
	for _, c := range cases {
		if got := Who(c.id); got != c.want {
			t.Errorf("Who(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}
