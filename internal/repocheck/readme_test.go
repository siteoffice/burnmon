package repocheck

import (
	"os"
	"strings"
	"testing"
)

func TestReadmeMacBannerLine(t *testing.T) {
	b, err := os.ReadFile("../../cmd/burnmon-dev/README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(b)), " ")
	const want = "On macOS the banner is attributed to Script Editor and a click does nothing."
	if !strings.Contains(text, want) {
		t.Errorf("cmd\\burnmon-dev\\README.md lacks %q", want)
	}
}
