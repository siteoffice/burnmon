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
		"go build -o dist/controlcentre ./cmd/controlcentre",
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
	// The pin moved once, on purpose: release.yml calls package.yml for the
	// startable zips and releases the bare files when packaging fails
	// (TestReleaseCallsPackage reads what changed).
	const want = "c98813e7ab61a676b5d753e2c3ddbb537d1a459bfe1dcd0b960ef6ed492341e8"
	sum := sha256.Sum256([]byte(readLF(t, "../../.github/workflows/release.yml")))
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("release.yml sha256 = %s, want %s", got, want)
	}
}
