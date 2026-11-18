package lamp

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDemoTimelineMoodsInOrder(t *testing.T) {
	want := []string{
		MoodAsleep, MoodWorking, MoodYourTurn, MoodBlocked,
		MoodBored, MoodSweating, MoodDone, MoodWorking,
	}
	if DemoSeconds != 24 {
		t.Fatalf("DemoSeconds = %d, want 24", DemoSeconds)
	}
	for sec := 0; sec < DemoSeconds; sec++ {
		st := DemoState(sec)
		if st.Mood != want[sec/3] {
			t.Errorf("second %d: mood %q, want %q", sec, st.Mood, want[sec/3])
		}
		if !st.Demo {
			t.Errorf("second %d: demo is false", sec)
		}
		if st.Headline == "" {
			t.Errorf("second %d: no headline", sec)
		}
	}
}

func TestDemoWallGrowsFromTenToTwenty(t *testing.T) {
	got := []int{DemoState(12).WallBricks, DemoState(13).WallBricks, DemoState(14).WallBricks}
	if got[0] != 10 || got[1] != 15 || got[2] != 20 {
		t.Fatalf("wall bricks over the bored segment = %v, want [10 15 20]", got)
	}
}

func TestDemoStateWrapsAfter24Seconds(t *testing.T) {
	if DemoState(24).Mood != DemoState(0).Mood || DemoState(-1).Mood != DemoState(23).Mood {
		t.Fatal("DemoState does not wrap at 24 seconds")
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
		for _, bad := range []string{"<script", "<image", "http://", "https://", "—"} {
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
	if len(s) >= 200*1024 {
		t.Errorf("glim-demo.svg is %d bytes, want under 200 KB", len(s))
	}
	if !strings.Contains(s, "@keyframes") {
		t.Error("no @keyframes")
	}
	for _, bad := range []string{"<script", "<image", "https://", "—"} {
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
	for _, m := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(s, -1) {
		if !strings.HasPrefix(m[1], "#") {
			t.Errorf("demo svg has an href to %q", m[1])
		}
	}
}

func TestDemoSVGIsStable(t *testing.T) {
	if DemoSVG() != DemoSVG() {
		t.Fatal("DemoSVG is not deterministic")
	}
}
