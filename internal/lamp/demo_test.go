package lamp

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"burnmon/internal/waits"
)

// demoSegs is the loop of 48 seconds: the first second, the end (exclusive)
// and the mood of each segment.
var demoSegs = []struct {
	from, to int
	mood     string
}{
	{0, 4, MoodAsleep}, {4, 10, MoodWorking}, {10, 13, MoodYourTurn}, {13, 16, MoodBlocked},
	{16, 19, MoodDone}, {19, 31, MoodBored}, {31, 35, MoodSweating}, {35, 38, MoodYourTurn},
	{38, 47, MoodAsleep}, {47, 48, MoodDone},
}

// demoActStarts is the second each act starts at in the loop.
var demoActStarts = map[string]int{
	ActWake: 4, ActKnock: 10, ActAlarm: 13, ActStamp: 16,
	ActSlip: 24, ActWelcome: 35, ActCoffee: 38, ActPeek: 42,
}

func TestDemoTimelineMoodsInOrder(t *testing.T) {
	if DemoSeconds != 48 {
		t.Fatalf("DemoSeconds = %d, want 48", DemoSeconds)
	}
	for _, seg := range demoSegs {
		for sec := seg.from; sec < seg.to; sec++ {
			st := DemoState(sec)
			if st.Mood != seg.mood {
				t.Errorf("second %d: mood %q, want %q", sec, st.Mood, seg.mood)
			}
			if !st.Demo {
				t.Errorf("second %d: demo is false", sec)
			}
			if st.Headline == "" {
				t.Errorf("second %d: no headline", sec)
			}
		}
	}
}

func TestDemoWallGrowsFromTenToFourteenThenFifteen(t *testing.T) {
	for sec := 19; sec < 24; sec++ {
		if got, want := DemoState(sec).WallBricks, 10+sec-19; got != want {
			t.Errorf("second %d: %d bricks, want %d", sec, got, want)
		}
	}
	for sec := 24; sec < 31; sec++ {
		if got := DemoState(sec).LongestWaitMin; got < slipMinutes {
			t.Errorf("second %d: longest wait %d minutes, want %d or more for the slip", sec, got, slipMinutes)
		}
	}
}

func TestDemoPlaysEveryActOnceInTheLoop(t *testing.T) {
	runs := map[string]int{}
	prev, prevSeq := "", int64(0)
	seqOf := map[string]int64{}
	for sec := 0; sec < DemoSeconds; sec++ {
		act, seq := DemoAct(sec)
		if act != "" {
			if act != prev {
				runs[act]++
				if _, dup := seqOf[act]; !dup {
					seqOf[act] = seq
				}
				if seq == prevSeq {
					t.Errorf("second %d: act %s keeps the seq %d of the act before", sec, act, seq)
				}
			} else if seq != prevSeq {
				t.Errorf("second %d: seq changed inside the act %s", sec, act)
			}
			if act != prev && sec != demoActStarts[act] {
				t.Errorf("second %d: act %s starts here, want second %d", sec, act, demoActStarts[act])
			}
		}
		prev, prevSeq = act, seq
	}
	for act, start := range demoActStarts {
		if runs[act] != 1 {
			t.Errorf("act %s plays %d times in the loop, want 1", act, runs[act])
		}
		end := start + int(ActSeconds(act)+0.999)
		for sec := start; sec < end; sec++ {
			if got, _ := DemoAct(sec); got != act {
				t.Errorf("second %d: act %q, want %s (starts at %d, %.1f s)", sec, got, act, start, ActSeconds(act))
			}
		}
		if got, _ := DemoAct(end); got == act {
			t.Errorf("act %s still plays at second %d, after its length", act, end)
		}
	}
	if len(runs) != len(actSeconds) {
		t.Errorf("the loop plays %d acts, the engine has %d", len(runs), len(actSeconds))
	}
}

func TestDemoActSeqDiffersFromLoopToLoop(t *testing.T) {
	a1, s1 := DemoAct(10)
	a2, s2 := DemoAct(10 + DemoSeconds)
	if a1 != ActKnock || a2 != ActKnock || s1 == s2 {
		t.Fatalf("loop 1 %q %d, loop 2 %q %d: want knock twice with different seq", a1, s1, a2, s2)
	}
	if a, _ := DemoAct(-DemoSeconds + 10); a != ActKnock {
		t.Fatalf("DemoAct before the first loop = %q, want knock", a)
	}
}

func TestDemoShowsEveryStateThatLastsOnce(t *testing.T) {
	runs := map[string]int{}
	at := map[string][]int{}
	var prev map[string]bool
	for sec := 0; sec < DemoSeconds; sec++ {
		cur := map[string]bool{}
		for _, h := range Holds(DemoState(sec), DemoSettings(sec)) {
			cur[h] = true
			at[h] = append(at[h], sec)
			if !prev[h] {
				runs[h]++
			}
		}
		prev = cur
	}
	for _, h := range []string{HoldWall, HoldSweat, HoldSleep, HoldDots} {
		if runs[h] != 1 {
			t.Errorf("state %s starts %d times in the loop (seconds %v), want 1", h, runs[h], at[h])
		}
	}
	for _, h := range at[HoldSleep] {
		if h >= 4 {
			t.Errorf("sleep holds at second %d, want only the first segment", h)
		}
	}
}

func TestDemoSettingsKeepGlimDownThenHideHim(t *testing.T) {
	for sec := 0; sec < 10; sec++ {
		if Quiet(DemoState(sec), DemoSettings(sec)) {
			t.Errorf("second %d: the demo hides Glim, want him down", sec)
		}
	}
	for sec := 38; sec < 47; sec++ {
		if !Quiet(DemoState(sec), DemoSettings(sec)) {
			t.Errorf("second %d: the demo shows Glim, want the lamp quiet and hidden for coffee and peek", sec)
		}
	}
	if DemoState(35).Counted < 1 {
		t.Error("welcome needs a counted item at second 35")
	}
	if DemoState(38).Counted != 0 {
		t.Error("coffee needs no counted item at second 38")
	}
}

func TestDemoStateWrapsAfter48Seconds(t *testing.T) {
	if DemoState(48).Mood != DemoState(0).Mood || DemoState(-1).Mood != DemoState(47).Mood {
		t.Fatal("DemoState does not wrap at 48 seconds")
	}
}

func TestDemoUsesNoRealNames(t *testing.T) {
	for sec := 0; sec < DemoSeconds; sec++ {
		for _, r := range DemoState(sec).Rows {
			if !strings.Contains(r.ID, "demo") && r.Kind != RowKindMore {
				t.Errorf("second %d: row id %q is not an invented demo id", sec, r.ID)
			}
		}
	}
}

func TestWritePosesWritesTenFiles(t *testing.T) {
	dir := t.TempDir()
	if err := WritePoses(dir); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 10 {
		t.Fatalf("WritePoses wrote %d files, want 10", len(ents))
	}
	for _, m := range AllMoods {
		b, err := os.ReadFile(filepath.Join(dir, "glim-"+m+".svg"))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, `width="256" height="256"`) || !strings.Contains(s, `viewBox="0 0 256 256"`) {
			t.Errorf("glim-%s.svg is not 256 by 256", m)
		}
		if !strings.Contains(s, "glim-"+m) {
			t.Errorf("glim-%s.svg does not hold the %s Glim", m, m)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "glim-sheet.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `width="1200" height="630"`) {
		t.Error("glim-sheet.svg is not 1200 by 630")
	}
	for _, m := range AllMoods {
		if !strings.Contains(string(b), "glim-"+m) {
			t.Errorf("glim-sheet.svg misses mood %s", m)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "glim-demo.svg")); err != nil {
		t.Error("glim-demo.svg is missing")
	}
}

func TestWritePosesRefusesMissingFolder(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if err := WritePoses(missing); err == nil {
		t.Fatal("WritePoses accepted a folder that does not exist")
	}
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("WritePoses made the missing folder")
	}
}

func TestWritePosesRefusesAFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WritePoses(f); err == nil {
		t.Fatal("WritePoses accepted a file as the folder")
	}
}

func TestPosesHoldNoScriptImageOrOutsideResource(t *testing.T) {
	dir := t.TempDir()
	if err := WritePoses(dir); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		s := string(b)
		for _, bad := range []string{"<script", "<image", "http://", "https://", emDash} {
			// the xmlns of an SVG is the one allowed http:// text
			if bad == "http://" {
				s = strings.ReplaceAll(s, `xmlns="http://www.w3.org/2000/svg"`, "")
			}
			if strings.Contains(s, bad) {
				t.Errorf("%s holds %q", e.Name(), bad)
			}
		}
		for _, m := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(s, -1) {
			if !strings.HasPrefix(m[1], "#") {
				t.Errorf("%s has an href to %q", e.Name(), m[1])
			}
		}
	}
}

func TestDemoSVG(t *testing.T) {
	s := DemoSVG()
	if len(s) >= 300*1024 {
		t.Errorf("glim-demo.svg is %d bytes, want under 300 KB", len(s))
	}
	if !strings.Contains(s, "@keyframes") {
		t.Error("no @keyframes")
	}
	for _, bad := range []string{"<script", "<image", "https://", emDash} {
		if strings.Contains(s, bad) {
			t.Errorf("demo svg holds %q", bad)
		}
	}
	frames := strings.Split(s, `<g class="f"`)[1:]
	if len(frames) != DemoSeconds {
		t.Fatalf("demo svg has %d frames, want %d", len(frames), DemoSeconds)
	}
	for i, f := range frames {
		if !strings.Contains(f, ">demo<") {
			t.Errorf("frame %d has no word demo", i)
		}
		if i < len(frames)-1 && strings.Contains(f, `<defs`) {
			t.Errorf("frame %d holds defs", i)
		}
	}
	for act := range demoActStarts {
		if !strings.Contains(s, "@keyframes act-"+act+"{") {
			t.Errorf("no keyframes for the act %s", act)
		}
		sec := demoActStarts[act]
		if !strings.Contains(frames[sec], `a-`+act+`"`) {
			t.Errorf("the frame of second %d does not carry the class a-%s", sec, act)
		}
	}
	for _, h := range []string{HoldSweat, HoldSleep, HoldDots} {
		if !strings.Contains(s, "@keyframes hold-"+h+"{") {
			t.Errorf("no keyframes for the state %s", h)
		}
	}
	for _, m := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(s, -1) {
		if !strings.HasPrefix(m[1], "#") {
			t.Errorf("demo svg has an href to %q", m[1])
		}
	}
}

func TestDemoSVGShowsActLinesAndTheTicker(t *testing.T) {
	s := DemoSVG()
	for _, want := range []string{
		"Somebody started something.", "I am not bored. I am testing the rail.", "Welcome back.", "Ten o&#39;clock. Coffee.",
	} {
		if !strings.Contains(s, want) && !strings.Contains(s, strings.ReplaceAll(want, "&#39;", "'")) {
			t.Errorf("the demo svg does not show the line %q", want)
		}
	}
}

func TestPosesAreAllTheSameSizeAndSheetUsesPlainWords(t *testing.T) {
	dir := t.TempDir()
	if err := WritePoses(dir); err != nil {
		t.Fatal(err)
	}
	for _, m := range AllMoods {
		b, _ := os.ReadFile(filepath.Join(dir, "glim-"+m+".svg"))
		if !strings.HasPrefix(string(b), svgHead+`viewBox="0 0 256 256" width="256" height="256">`) {
			t.Errorf("glim-%s.svg has another size than the other poses", m)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "glim-sheet.svg"))
	sheet := string(b)
	texts := regexp.MustCompile(`<text[^>]*font-size="22"[^>]*>([^<]*)</text>`).FindAllStringSubmatch(sheet, -1)
	if len(texts) != len(AllMoods) {
		t.Fatalf("the sheet has %d labels, want %d", len(texts), len(AllMoods))
	}
	for i, m := range AllMoods {
		want := waits.MoodWord(m)
		if want == "" || texts[i][1] != want {
			t.Errorf("sheet label %d is %q, want %q", i, texts[i][1], want)
		}
		if strings.Contains(texts[i][1], "_") {
			t.Errorf("sheet label %q has an underscore", texts[i][1])
		}
	}
	if !strings.Contains(sheet, ">your turn<") {
		t.Error(`the sheet does not say "your turn"`)
	}
}

func TestDemoSVGIsStable(t *testing.T) {
	if DemoSVG() != DemoSVG() {
		t.Fatal("DemoSVG is not deterministic")
	}
}

// TestDemoCoffeeShowsGlimAwake: at the coffee seconds the demo draws the
// working pose (open lids), not the asleep pose of the mood.
func TestDemoCoffeeShowsGlimAwake(t *testing.T) {
	s := DemoSVG()
	frames := strings.Split(s, `<g class="f"`)[1:]
	start := demoActStarts[ActCoffee]
	for sec := start; sec < start+int(ActSeconds(ActCoffee)); sec++ {
		if !strings.Contains(frames[sec], `href="#gl-working"`) {
			t.Errorf("second %d (coffee) does not draw the working pose", sec)
		}
		if strings.Contains(frames[sec], `href="#gl-asleep"`) {
			t.Errorf("second %d (coffee) draws Glim asleep", sec)
		}
	}
	// the rig of the working pose has open lids: scaleY(1) is only set for off and asleep
	if rig := Glim(ActMood(ActCoffee, MoodAsleep), 0, "dark"); !strings.HasPrefix(rig, `<svg class="glim glim-working mood-working `) {
		t.Error("the coffee pose is not the working rig")
	}
}

// heightOf reads the height of every glim picture placed in an svg text.
func heightsOfGlims(s string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`<svg class="glim [^>]*? height="([0-9.]+)"`).FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestSheetAndPoseFilesDrawEveryPoseAtOneHeight(t *testing.T) {
	dir := t.TempDir()
	if err := WritePoses(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "glim-sheet.svg"))
	hs := heightsOfGlims(string(b))
	if len(hs) != len(AllMoods) {
		t.Fatalf("the sheet holds %d pictures, want %d", len(hs), len(AllMoods))
	}
	for i, h := range hs {
		if h != hs[0] {
			t.Errorf("sheet pose %s is %s high, the first is %s", AllMoods[i], h, hs[0])
		}
	}
	first := ""
	for _, m := range AllMoods {
		b, _ := os.ReadFile(filepath.Join(dir, "glim-"+m+".svg"))
		h := heightsOfGlims(string(b))
		if len(h) != 1 {
			t.Fatalf("glim-%s.svg holds %d pictures, want 1", m, len(h))
		}
		if first == "" {
			first = h[0]
		}
		if h[0] != first {
			t.Errorf("glim-%s.svg draws Glim %s high, the first pose file %s", m, h[0], first)
		}
	}
}
