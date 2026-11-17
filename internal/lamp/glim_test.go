package lamp

import (
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"
)

func glimGolden(t *testing.T, theme string) {
	t.Helper()
	var b strings.Builder
	for _, m := range allMoods {
		bricks := 0
		if m == MoodBored {
			bricks = 20
		}
		b.WriteString("<!-- " + m + " -->\n" + Glim(m, bricks, theme) + "\n")
	}
	path := "testdata/glim_" + theme + ".golden.svg"
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/lamp -update once)", err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != b.String() {
		t.Errorf("%s Glim differs from %s (run with -update after a change you mean)", theme, path)
	}
}

func TestGlimGoldenDark(t *testing.T)  { glimGolden(t, "dark") }
func TestGlimGoldenLight(t *testing.T) { glimGolden(t, "light") }

func TestGlimMoodsDiffer(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		seen := map[string]string{}
		for _, m := range allMoods {
			s := Glim(m, 0, theme)
			if prev, dup := seen[s]; dup {
				t.Errorf("%s: %s and %s draw the same", theme, prev, m)
			}
			seen[s] = m
		}
	}
	if Glim(MoodWorking, 0, "dark") == Glim(MoodWorking, 0, "light") {
		t.Error("the two themes draw the same")
	}
}

func TestGlimWallBricks(t *testing.T) {
	for _, n := range []int{10, 20, 30} {
		s := Glim(MoodBored, n, "dark")
		if got := strings.Count(s, `class="glim-brick"`); got != n {
			t.Errorf("%d bricks asked, %d drawn", n, got)
		}
		if strings.Count(s, `class="glim-wall"`) != 1 {
			t.Errorf("%d bricks: want one wall group", n)
		}
	}
	if got := strings.Count(Glim(MoodBored, 31, "dark"), `class="glim-brick"`); got != 30 {
		t.Errorf("31 bricks must draw 30, drew %d", got)
	}
	for _, m := range allMoods {
		if m == MoodBored {
			continue
		}
		if strings.Contains(Glim(m, 20, "dark"), "glim-wall") {
			t.Errorf("mood %s must not draw the wall", m)
		}
	}
	if strings.Contains(Glim(MoodBored, 0, "dark"), "glim-wall") {
		t.Error("no bricks, no wall")
	}
}

func TestGlimIsSelfContained(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, m := range allMoods {
			s := Glim(m, 12, theme)
			for _, bad := range []string{"<image", "href", "<script", "<style", "url(", "javascript:", "—"} {
				if strings.Contains(s, bad) {
					t.Errorf("%s %s holds %q", theme, m, bad)
				}
			}
			if strings.Contains(strings.Replace(s, `xmlns="http://www.w3.org/2000/svg"`, "", 1), "http") {
				t.Errorf("%s %s points outside", theme, m)
			}
			dec := xml.NewDecoder(strings.NewReader(s))
			for {
				if _, err := dec.Token(); err == io.EOF {
					break
				} else if err != nil {
					t.Errorf("%s %s is not well formed: %v", theme, m, err)
					break
				}
			}
		}
	}
}

func TestGlimClassNamesThePageAnimates(t *testing.T) {
	count := func(mood, class string) int {
		return strings.Count(Glim(mood, 5, "dark"), `class="`+class+`"`)
	}
	for _, m := range allMoods {
		if count(m, "glim-eye") != 2 {
			t.Errorf("%s: want two glim-eye", m)
		}
		if !strings.Contains(Glim(m, 0, "dark"), `class="glim glim-`+m+`"`) {
			t.Errorf("%s: root class", m)
		}
	}
	if count(MoodYourTurn, "glim-ray") != 2 {
		t.Error("your_turn has two rays")
	}
	if count(MoodBlocked, "glim-ray") != 6 {
		t.Error("blocked has six rays")
	}
	for _, m := range []string{MoodOff, MoodAsleep, MoodWorking, MoodDone, MoodBored, MoodSweating} {
		if count(m, "glim-ray") != 0 {
			t.Errorf("%s has no rays", m)
		}
	}
	if count(MoodSweating, "glim-drop") != 1 {
		t.Error("sweating has one drop")
	}
	if count(MoodAsleep, "glim-z") != 1 {
		t.Error("asleep has the z z")
	}
	if !strings.Contains(Glim(MoodAsleep, 0, "dark"), ">z z<") {
		t.Error("asleep text is z z")
	}
}

func TestGlimUnknownInputsFallBack(t *testing.T) {
	if Glim("nonsense", 0, "dark") != Glim(MoodAsleep, 0, "dark") {
		t.Error("an unknown mood draws like asleep")
	}
	if Glim(MoodWorking, 0, "") != Glim(MoodWorking, 0, "dark") {
		t.Error("an unknown theme draws dark")
	}
}
