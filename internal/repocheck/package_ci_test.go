package repocheck

import (
	"strings"
	"testing"
)

// The startable zips (Mac findings lane, T6): package.yml builds one zip per
// system on a pull request and for release.yml, and scripts/package.sh puts the
// three window programs of macOS in app bundles. These tests read the files as
// text; the packaging itself runs only on the macOS and Windows runners.

func TestPackageWorkflow(t *testing.T) {
	text := readLF(t, "../../.github/workflows/package.yml")
	for _, want := range []string{
		"pull_request:",
		"workflow_call:",
		"runs-on: macos-latest",
		"arch: [arm64, amd64]",
		"bash scripts/package.sh",
		"runs-on: windows-latest",
		`.\build.ps1`,
		"-windows-amd64",
		`packaging\windows\README-FIRST.txt`,
		"burnmon.exe, burnmon-dev.exe, controlcentre.exe, burnmon-cli.exe, burnmon.example.json",
		"actions/upload-artifact@v4",
		"contents: read",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("package.yml lacks %q", want)
		}
	}
	if strings.Contains(text, "contents: write") || strings.Contains(text, "action-gh-release") {
		t.Error("package.yml must release nothing; release.yml attaches the zips")
	}
}

func TestReleaseCallsPackage(t *testing.T) {
	text := readLF(t, "../../.github/workflows/release.yml")
	for _, want := range []string{
		"uses: ./.github/workflows/package.yml",
		"tag: ${{ github.ref_name }}",
		"needs: [build, package]",
		"if: ${{ !cancelled() && needs.build.result == 'success' }}",
		"contents: write",
		`- "v*"`,
		"merge-multiple: true",
		"files: dist/*",
		// The eight bare files of the earlier releases stay.
		`CGO_ENABLED: "0"`,
		"goos: [darwin, linux]",
		"goarch: [amd64, arm64]",
		"softprops/action-gh-release@v2",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("release.yml lacks %q", want)
		}
	}
}

func TestPackageScript(t *testing.T) {
	text := readLF(t, "../../scripts/package.sh")
	for _, want := range []string{
		"set -euo pipefail",
		`bundle "BurnMon" burnmon ./cmd/burnmon dev.zerononsense.burnmon`,
		`bundle "BurnMon Dev" burnmon-dev ./cmd/burnmon-dev dev.zerononsense.burnmon-dev`,
		`bundle "Control Centre" controlcentre ./cmd/controlcentre dev.zerononsense.controlcentre winres/controlcentre.icns`,
		`build "$STAGE/burnmon-cli" ./cmd/burnmon-cli`,
		"CGO_ENABLED=1 GOOS=darwin",
		"CFBundleIdentifier",
		"CFBundleIconFile",
		"CFBundleShortVersionString",
		`NAME="burnmon-$TAG-macos-$ARCH"`,
		"cp burnmon.example.json",
		"cp packaging/macos/README-FIRST.txt",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("package.sh lacks %q", want)
		}
	}
	for i, r := range text {
		if r > 0x7e || (r < 0x20 && r != '\n' && r != '\t') {
			t.Fatalf("package.sh has a non-ASCII or control character at byte %d", i)
		}
	}
}

func TestReadmeFirstTexts(t *testing.T) {
	mac := readLF(t, "../../packaging/macos/README-FIRST.txt")
	for _, want := range []string{"xattr -dr com.apple.quarantine", "not signed", "Double-click"} {
		if !strings.Contains(mac, want) {
			t.Errorf("macOS README-FIRST.txt lacks %q", want)
		}
	}
	win := readLF(t, "../../packaging/windows/README-FIRST.txt")
	for _, want := range []string{"not signed", "not tested", "Double-click"} {
		if !strings.Contains(win, want) {
			t.Errorf("Windows README-FIRST.txt lacks %q", want)
		}
	}
	for name, s := range map[string]string{"macos": mac, "windows": win} {
		if strings.ContainsRune(s, rune(0x2014)) {
			t.Errorf("%s README-FIRST.txt holds an em dash", name)
		}
	}
}
