package repocheck

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func readLF(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func TestMacOSWorkflow(t *testing.T) {
	text := readLF(t, "../../.github/workflows/macos.yml")
	for _, want := range []string{
		"runs-on: macos-latest",
		`CGO_ENABLED: "1"`,
		"go build -o dist/burnmon ./cmd/burnmon",
		"go build -o dist/burnmon-dev ./cmd/burnmon-dev",
		"go build -o dist/burnmon-cli ./cmd/burnmon-cli",
		"go vet ./...",
		"go test ./... -count=1",
		"bash scripts/maccheck.sh --ci",
		`- "sprint/**"`,
		"- main",
		"pull_request:",
		"workflow_dispatch:",
		"contents: read",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("macos.yml lacks %q", want)
		}
	}
	if strings.Contains(text, "contents: write") {
		t.Error("macos.yml must not grant contents: write")
	}
}

func TestReleaseWorkflowUnchanged(t *testing.T) {
	const want = "89c896754e5c65b79be379f47f94841d4c0b2af40d7128be79129473ba9a3f4c"
	sum := sha256.Sum256([]byte(readLF(t, "../../.github/workflows/release.yml")))
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("release.yml sha256 = %s, want %s", got, want)
	}
}
