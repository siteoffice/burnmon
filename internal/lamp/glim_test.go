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
		if got := strings.Count(s, ` glim-brick"`); got != n {
			t.Errorf("%d bricks asked, %d drawn", n, got)
		}
		if strings.Count(s, ` glim-wall"`) != 1 {
			t.Errorf("%d bricks: want one wall group", n)
		}
	}
	if got := strings.Count(Glim(MoodBored, 31, "dark"), ` glim-brick"`); got != 30 {
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
			for _, bad := range []string{"<image", "href", "<script", "url(", "javascript:", string(rune(0x2014))} {
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

// rigClasses are the class names the page animates (plan of the glim lane,
// turn T4). Every mood draws every part; the style inside the picture shows the
// parts a mood holds, and the page shows the others during an act.
var rigClasses = []string{
	"glim", "rig", "swing", "cord", "handle", "body", "dome", "basep", "feet",
	"eyes", "pup", "lid", "mouth", "arm armL", "arm armR", "mitt", "rays",
	"drop", "zz", "mug", "dots", "dot", "stampi", "rip",
}

func TestGlimClassNamesThePageAnimates(t *testing.T) {
	count := func(mood, class string) int {
		return strings.Count(Glim(mood, 5, "dark"), `class="`+class+`"`)
	}
	for _, m := range allMoods {
		s := Glim(m, 5, "dark")
		if count(m, "glim-eye") != 2 {
			t.Errorf("%s: want two glim-eye", m)
		}
		if !strings.Contains(s, `class="glim glim-`+m+` mood-`+m+` theme-dark"`) {
			t.Errorf("%s: root class", m)
		}
		for _, c := range rigClasses {
			if c == "glim" {
				continue
			}
			if !strings.Contains(s, `class="`+c) && !strings.Contains(s, ` `+c+`"`) {
				t.Errorf("%s: class %q is missing", m, c)
			}
		}
		if got := strings.Count(s, `class="glim-ray`); got != 6 {
			t.Errorf("%s: want six rays, got %d", m, got)
		}
		if got := strings.Count(s, ` r2"`); got != 2 {
			t.Errorf("%s: want two short-set rays, got %d", m, got)
		}
		if strings.Count(s, `class="pup"`) != 2 || strings.Count(s, `class="lid`) != 2 {
			t.Errorf("%s: want two pupils and two lids", m)
		}
		for _, c := range []string{"drop", "zz", "mug", "stampi", "rip"} {
			if strings.Count(s, `class="`+c+`"`) != 1 && strings.Count(s, `class="`+c+` `) != 1 {
				t.Errorf("%s: want one %s", m, c)
			}
		}
	}
	if !strings.Contains(Glim(MoodAsleep, 0, "dark"), ">z z<") {
		t.Error("asleep text is z z")
	}
}

func TestGlimRigHasThemeStyleAndNoOutsideColours(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		s := Glim(MoodWorking, 0, theme)
		if strings.Count(s, "<style>") != 1 {
			t.Fatalf("%s: want one style element", theme)
		}
		for _, want := range []string{".theme-light{", ".theme-dark{", ".mood-blocked .rays", ".mood-sweating .drop", ".mood-asleep .zz"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s: style misses %q", theme, want)
			}
		}
		if !strings.Contains(s, `theme-`+theme+`"`) {
			t.Errorf("%s: root misses its theme class", theme)
		}
	}
}

func TestGlimWallShowsThirtyPlus(t *testing.T) {
	for _, n := range []int{10, 20} {
		if strings.Contains(Glim(MoodBored, n, "dark"), "30+") {
			t.Errorf("%d bricks must not show 30+", n)
		}
	}
	for _, n := range []int{30, 31, 99} {
		s := Glim(MoodBored, n, "dark")
		if !strings.Contains(s, ">30+<") {
			t.Errorf("%d bricks must show 30+", n)
		}
		if got := strings.Count(s, ` glim-brick"`); got != 30 {
			t.Errorf("%d bricks asked, %d drawn", n, got)
		}
	}
}

func TestGlimSizes(t *testing.T) {
	if s := Glim(MoodWorking, 0, "dark"); !strings.Contains(s, `width="200" height="200"`) {
		t.Error("a plain Glim is 200 by 200")
	}
	if s := Glim(MoodBored, 5, "dark"); !strings.Contains(s, `width="320" height="200"`) {
		t.Error("a Glim with the wall is 320 by 200")
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
