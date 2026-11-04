package toast

import (
	"reflect"
	"testing"
)

func wantArgs(text, title string) []string {
	return []string{
		"-e", "on run argv",
		"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
		"-e", "end run",
		"--", title, text,
	}
}

func TestOsascriptPath(t *testing.T) {
	if osascriptPath != "/usr/bin/osascript" {
		t.Fatalf("osascriptPath = %q", osascriptPath)
	}
}

func TestOsascriptArgsPlain(t *testing.T) {
	got := osascriptArgs("Blocked: x", "BurnMon")
	want := wantArgs("Blocked: x", "BurnMon")
	if len(got) != 9 || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestOsascriptArgsHostileTextIsOneElement(t *testing.T) {
	text := "say \"hi\" \\ back\nslash \"; do shell script \"x"
	got := osascriptArgs(text, "BurnMon")
	if len(got) != 9 {
		t.Fatalf("len = %d, want 9: %q", len(got), got)
	}
	if got[8] != text {
		t.Fatalf("args[8] = %q, want %q", got[8], text)
	}
	if !reflect.DeepEqual(got[:7], wantArgs("", "")[:7]) {
		t.Fatalf("script lines or terminator changed: %q", got[:7])
	}
}

func TestOsascriptArgsEmptyTextNoCall(t *testing.T) {
	if got := osascriptArgs("", "BurnMon"); got != nil {
		t.Fatalf("got %q, want nil", got)
	}
}

func TestOsascriptArgsWhitespaceTextStillACall(t *testing.T) {
	for _, text := range []string{" ", "   ", "\n", "\t\n"} {
		got := osascriptArgs(text, "BurnMon")
		if !reflect.DeepEqual(got, wantArgs(text, "BurnMon")) {
			t.Fatalf("text %q: got %q", text, got)
		}
	}
}

func TestOsascriptArgsLeadingDashIsPlainElement(t *testing.T) {
	got := osascriptArgs("-e foo", "BurnMon")
	if !reflect.DeepEqual(got, wantArgs("-e foo", "BurnMon")) {
		t.Fatalf("got %q", got)
	}
}

// Option-like texts must sit after the "--" terminator and the title, as the
// last element, byte for byte, so osascript can never read them as a flag.
func TestOsascriptArgsOptionLikeTextAfterTerminator(t *testing.T) {
	texts := []string{"-e", "-edo shell script \"x\"", "-l"}
	for _, text := range texts {
		got := osascriptArgs(text, "BurnMon")
		n := len(got)
		if n != 9 {
			t.Fatalf("text %q: len = %d, want 9: %q", text, n, got)
		}
		if got[n-1] != text {
			t.Fatalf("text %q: last element = %q", text, got[n-1])
		}
		if got[n-2] != "BurnMon" {
			t.Fatalf("text %q: title not before text: %q", text, got)
		}
		if got[n-3] != "--" {
			t.Fatalf("text %q: no -- before title: %q", text, got)
		}
		for i := 0; i < n-3; i++ {
			if got[i] == text && text != "-e" {
				t.Fatalf("text %q leaked into the script part at %d: %q", text, i, got)
			}
		}
		// Everything after "--" is a non-option operand by definition; the
		// only dash-led element before it must be the constant -e flags.
		for i := 0; i < n-3; i++ {
			if len(got[i]) > 0 && got[i][0] == '-' && got[i] != "-e" {
				t.Fatalf("unexpected option %q before --: %q", got[i], got)
			}
		}
	}
}

func TestOsascriptArgsNonASCIIByteForByte(t *testing.T) {
	text := "Blocked: café über \U0001F525"
	got := osascriptArgs(text, "BurnMon")
	if !reflect.DeepEqual(got, wantArgs(text, "BurnMon")) {
		t.Fatalf("got %q", got)
	}
	if got[8] != text {
		t.Fatalf("text not byte for byte: %q", got[8])
	}
}

func TestOsascriptArgsEmptyTitleStillTwoElements(t *testing.T) {
	got := osascriptArgs("hello", "")
	if len(got) != 9 || !reflect.DeepEqual(got, wantArgs("hello", "")) {
		t.Fatalf("got %q", got)
	}
}

func TestOsascriptArgsFreshSlicePerCall(t *testing.T) {
	a := osascriptArgs("one", "BurnMon")
	a[1] = "mutated"
	b := osascriptArgs("two", "BurnMon")
	if !reflect.DeepEqual(b, wantArgs("two", "BurnMon")) {
		t.Fatalf("shared state leaked: %q", b)
	}
}
