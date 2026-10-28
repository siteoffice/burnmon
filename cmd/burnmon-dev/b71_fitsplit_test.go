//go:build windows

package main

import (
	"encoding/json"
	"math"
	"os/exec"
	"strings"
	"testing"
)

func TestFitSplitWaterFill(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := idlePage()
	s := strings.Index(page, "function fitSplit(")
	if s < 0 {
		t.Fatal("page.html lacks fitSplit")
	}
	e := strings.Index(page[s:], "\n  function ")
	if e < 0 {
		t.Fatal("cannot find the end of fitSplit")
	}
	src := page[s : s+e]
	run := func(list string, total int) []float64 {
		js := src + "\nconsole.log(JSON.stringify(fitSplit(" + list + ", " + itoa(total) + ")));"
		out, err := exec.Command(node, "-e", js).CombinedOutput()
		if err != nil {
			t.Fatalf("node: %v\n%s", err, out)
		}
		var got []float64
		if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &got); err != nil {
			t.Fatalf("bad node output %q: %v", out, err)
		}
		return got
	}
	near := func(name string, got, want []float64) {
		if len(got) != len(want) {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
		for i := range want {
			if math.Abs(got[i]-want[i]) > 0.01 {
				t.Errorf("%s: got %v want %v", name, got, want)
				return
			}
		}
	}
	near("a", run("[{share:.5,floor:60,cap:1e9},{share:.5,floor:60,cap:1e9}]", 400), []float64{200, 200})
	near("b", run("[{share:.9,floor:60,cap:1e9},{share:.1,floor:150,cap:1e9}]", 400), []float64{250, 150})
	near("c", run("[{share:.5,floor:60,cap:1e9},{share:.5,floor:60,cap:100}]", 400), []float64{300, 100})
	d := run("[{share:.2,floor:60,cap:1e9},{share:.3,floor:60,cap:1e9},{share:.5,floor:60,cap:1e9}]", 200)
	sum := 0.0
	for _, v := range d {
		if v < 60-0.01 {
			t.Errorf("d: %v below its floor", d)
		}
		sum += v
	}
	if math.Abs(sum-200) > 0.01 {
		t.Errorf("d: sum %v, want 200", sum)
	}
}

// TestFitDragRangeDrawsFromLists runs the two pure drag functions through node:
// the growing part may take from the host lists once its partner sits at its
// floor, and the lists give in proportion to the room above their floor.
func TestFitDragRangeDrawsFromLists(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	page := idlePage()
	fn := func(name string) string {
		s := strings.Index(page, "function "+name+"(")
		if s < 0 {
			t.Fatalf("page.html lacks %s", name)
		}
		e := strings.Index(page[s:], "\n  function ")
		if e < 0 {
			t.Fatalf("cannot find the end of %s", name)
		}
		return page[s : s+e]
	}
	run := func(expr string) []float64 {
		js := fn("fitDragRange") + "\n" + fn("fitDrawLists") + "\nconsole.log(JSON.stringify(" + expr + "));"
		out, err := exec.Command(node, "-e", js).CombinedOutput()
		if err != nil {
			t.Fatalf("node: %v\n%s", err, out)
		}
		var got []float64
		if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &got); err != nil {
			t.Fatalf("bad node output %q: %v", out, err)
		}
		return got
	}
	eq := func(name string, got, want []float64) {
		if len(got) != len(want) {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
		for i := range want {
			if math.Abs(got[i]-want[i]) > 0.01 {
				t.Errorf("%s: got %v want %v", name, got, want)
				return
			}
		}
	}
	rng := func(a, b string, hA, hB, avail int) []float64 {
		return run("(function(r){ return [r.min, r.max]; })(fitDragRange(" + a + ", " + b + ", " + itoa(hA) + ", " + itoa(hB) + ", " + itoa(avail) + "))")
	}
	chart := "{kind:'chart',floor:60,nat:60}"
	list := "{kind:'list',floor:71,nat:71}"
	// Both parts at their floor: no room without the lists, 57 px with them.
	eq("floors, no lists", rng(chart, list, 60, 71, 0), []float64{0, 0})
	eq("floors, 57 from lists", rng(chart, chart, 60, 60, 57), []float64{-57, 57})
	// The partner gives first: 40 above its floor plus 57 from the lists.
	eq("partner then lists", rng(chart, chart, 60, 100, 57), []float64{-57, 97})
	// A list part never grows beyond its cap, the lists cannot change that.
	eq("cap holds", rng(list, chart, 71, 60, 57), []float64{-57, 0})
	// Drawing: one list with room takes all; two lists share in proportion; never below a floor.
	eq("draw one list", run("fitDrawLists([{floor:77},{floor:60},{floor:91}],[77,60,131],25)"), []float64{77, 60, 106})
	eq("draw two lists", run("fitDrawLists([{floor:60},{floor:60}],[70,90],20)"), []float64{65, 75})
	eq("draw more than the room", run("fitDrawLists([{floor:60},{floor:60}],[70,90],500)"), []float64{60, 60})
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
