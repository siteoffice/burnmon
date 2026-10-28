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

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
