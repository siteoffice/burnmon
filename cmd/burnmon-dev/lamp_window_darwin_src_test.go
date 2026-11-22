package main

import (
	"os"
	"strings"
	"testing"
)

// These tests read the Mac lamp source and the page as text, on every system:
// lamp_window_darwin.go does not build on Windows. Nothing here is seen in a
// window and nothing is run on a Mac.

func readLampSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// N4: the clear backdrop makes the web view stop drawing its own background,
// guarded so a web view that does not answer to the key is left alone, with one
// log line for that case. underPageBackgroundColor stays clear.
func TestLampDarwinClearBackdropStopsWebViewBackground(t *testing.T) {
	src := readLampSource(t, "lamp_window_darwin.go")
	i := strings.Index(src, "static void lampStyle(")
	j := strings.Index(src, "static void lampPlace(")
	if i < 0 || j < i {
		t.Fatal("lampStyle not found in lamp_window_darwin.go")
	}
	style := src[i:j]
	clear := style[:strings.Index(style, "return;")]
	for _, want := range []string{
		"underPageBackgroundColor = [NSColor clearColor]",
		"drawsBackground",
		"setValue:[NSNumber numberWithBool:NO] forKey:@\"drawsBackground\"",
		"respondsToSelector:@selector(_setDrawsBackground:)",
		"NSLog(@\"lamp: the web view does not answer to drawsBackground",
	} {
		if !strings.Contains(clear, want) {
			t.Errorf("the clear backdrop branch of lampStyle lacks %q", want)
		}
	}
	card := style[strings.Index(style, "return;"):]
	if strings.Contains(card, "drawsBackground") {
		t.Error("the card backdrop must not touch drawsBackground")
	}
	if !strings.Contains(card, "[w setOpaque:YES]") {
		t.Error("the card backdrop must keep the opaque panel")
	}
}

// The comment says the key is not documented, that wry takes the same route,
// and what the log line means.
func TestLampDarwinDrawsBackgroundCommentSaysWhy(t *testing.T) {
	src := readLampSource(t, "lamp_window_darwin.go")
	for _, want := range []string{"not documented", "wry", "log line"} {
		if !strings.Contains(src, want) {
			t.Errorf("lamp_window_darwin.go does not explain drawsBackground: missing %q", want)
		}
	}
}

// The clear backdrop is the default and the Mac keeps it; lamp_backdrop card
// still selects the card.
func TestLampDarwinBackdropDefaultIsClearAndCardSelectsCard(t *testing.T) {
	src := readLampSource(t, "lamp_window_darwin.go")
	if !strings.Contains(src, "lampBackdropEnv(set.Backdrop)") {
		t.Error("the Mac window must choose its backdrop through lampBackdropEnv(set.Backdrop)")
	}
	if !strings.Contains(src, "C.lampStyle(w.Window(), C.int(style))") || !strings.Contains(src, "style = 1") {
		t.Error("the Mac window must pass clear (1) to lampStyle when glass, card (0) otherwise")
	}
	page := readLampSource(t, "lamp.html")
	if !strings.Contains(page, `data-backdrop="clear"`) {
		t.Error("lamp.html must default to the clear backdrop")
	}
	if !strings.Contains(page, `html[data-backdrop="clear"],html[data-backdrop="clear"] body{background:transparent}`) {
		t.Error("lamp.html must make html and body transparent for the clear backdrop")
	}
	if !strings.Contains(page, `look.backdrop==='card'?'card':'clear'`) {
		t.Error("lamp.html must select the card only for lamp_backdrop card")
	}
	run := readLampSource(t, "lamp_run.go")
	if !strings.Contains(run, `if backdrop == "clear"`) {
		t.Error("lampBackdropEnv must make only clear a glass backdrop")
	}
}
