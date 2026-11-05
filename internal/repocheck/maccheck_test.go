package repocheck

import (
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestMaccheckScriptShape(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/maccheck.sh")
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range raw {
		if b >= 0x80 {
			t.Fatalf("non-ASCII byte 0x%02x at offset %d", b, i)
		}
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if strings.Contains(text, "\r") {
		t.Fatal("stray carriage return after CRLF normalisation")
	}
	if runtime.GOOS != "windows" && strings.Contains(string(raw), "\r") {
		t.Fatal("carriage return in the raw bytes")
	}
	lines := strings.Split(text, "\n")
	if lines[0] != "#!/usr/bin/env bash" {
		t.Fatalf("line 1 = %q", lines[0])
	}
	if lines[1] != "set -euo pipefail" {
		t.Fatalf("line 2 = %q", lines[1])
	}

	for _, want := range []string{
		`if [ "${1:-}" = "--ci" ]; then MODE=ci; fi`,
		"MODE=full",
		"OUT=maccheck-out",
		`REPORT="$OUT/report.md"`,
		"uname -m",
		"go version",
		"sw_vers -productVersion",
		"command -v go",
		"xcode-select -p",
		"BURNMON_UICHECK=1",
		"BURNMON_DEV_UICHECK=1",
		"screencapture",
		"## Manual checks",
		"Window feel",
		"Menus",
		"Sleep and wake",
		"Second screen",
		"OF25 banner",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("script lacks %q", want)
		}
	}
	if n := strings.Count(text, "skipped (ci)"); n != 1 {
		t.Errorf("skipped (ci) occurs %d times, want 1", n)
	}

	var steps, windowSteps []string
	stepRe := regexp.MustCompile(`^step (\S+)`)
	winRe := regexp.MustCompile(`window_step "?([^"\s]+)"?`)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := stepRe.FindStringSubmatch(trimmed); m != nil {
			steps = append(steps, m[1])
		}
		if strings.HasPrefix(trimmed, "window_step()") {
			continue
		}
		if m := winRe.FindStringSubmatch(line); m != nil {
			windowSteps = append(windowSteps, m[1])
		}
	}
	wantSteps := []string{"build-burnmon", "build-burnmon-dev", "build-burnmon-cli", "build-uicheck", "vet", "test"}
	if !reflect.DeepEqual(steps, wantSteps) {
		t.Errorf("step names = %v, want %v", steps, wantSteps)
	}
	wantWin := []string{"start-burnmon", "probe-burnmon", "screenshot-burnmon", "stop-burnmon",
		"start-burnmon-dev", "uicheck-$c", "screenshot-burnmon-dev", "stop-burnmon-dev"}
	if !reflect.DeepEqual(windowSteps, wantWin) {
		t.Errorf("window_step names = %v, want %v", windowSteps, wantWin)
	}

	for _, banned := range []string{"brew ", "go get", "go install", "sudo", "curl", "softwareupdate", "pip install", "port install"} {
		if strings.Contains(text, banned) {
			t.Errorf("script contains %q", banned)
		}
	}
	for _, line := range lines {
		if strings.Contains(line, "xcode-select --install") && !strings.HasPrefix(strings.TrimSpace(line), "printf") {
			t.Errorf("xcode-select --install outside a printf: %q", line)
		}
	}
}

func TestMaccheckOutIgnoresItself(t *testing.T) {
	text := readLF(t, "../../scripts/maccheck.sh")
	ign := strings.Index(text, `> "$OUT/.gitignore"`)
	if ign < 0 {
		t.Fatal(`script never writes "$OUT/.gitignore"`)
	}
	first := strings.Index(text, `"$REPORT"`)
	if first < 0 {
		t.Fatal(`script never writes to "$REPORT"`)
	}
	if ign > first {
		t.Error(`"$OUT/.gitignore" is written after the first use of "$REPORT"`)
	}
}
