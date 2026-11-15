package main

import (
	"strings"
	"testing"
)

func d47Pass() (d47Variant, d47Read) {
	v := d47Variants()[0]
	r := d47Read{Room: 800, Sec: map[string]float64{"status": 240, "sessions_30min": 400, "processes_30min": 160},
		Pane: map[string]float64{"burn_30min": 150, "vendor_strip": 75}, PanePx: map[string]float64{"burn_30min": 150.5, "vendor_strip": 75, "system": 99}}
	return v, r
}

func TestD47CheckPasses(t *testing.T) {
	v, r := d47Pass()
	if bad := d47Check(v, r); len(bad) != 0 {
		t.Fatalf("got %v", bad)
	}
}

func TestD47CheckBranches(t *testing.T) {
	for name, c := range map[string]struct {
		mut  func(*d47Read)
		want string
	}{
		"section off by 3": {func(r *d47Read) { r.Sec["sessions_30min"] = 403 }, "sessions_30min drawn 403.0 px, want 400.0"},
		"pane off by 3":    {func(r *d47Read) { r.Pane["vendor_strip"] = 78 }, "pane vendor_strip drawn 78.0 px, want 75.0"},
		"scrolls":          {func(r *d47Read) { r.FitScroll = true }, "page scrolls = true, want false"},
		"error":            {func(r *d47Read) { r.Err = "x" }, `layout error "x", want ""`},
		"no room":          {func(r *d47Read) { r.Room = 0 }, "the layout pass measured no room"},
		"no panes":         {func(r *d47Read) { r.PanePx = nil }, "no pane sizes reported"},
	} {
		v, r := d47Pass()
		c.mut(&r)
		bad := d47Check(v, r)
		if len(bad) == 0 || !strings.Contains(strings.Join(bad, "; "), c.want) {
			t.Errorf("%s: got %v, want %q", name, bad, c.want)
		}
	}
}

// The wide variant expects the side-by-side rule: the row takes the larger
// number, Status the rest (Wilco, 2026-11-15).
func TestD47WideVariantWantsTheLarger(t *testing.T) {
	v := d47Variants()[1]
	if !v.wide() || v.want["status"] != 50 || v.want["sessions_30min"] != 50 || v.want["processes_30min"] != 50 {
		t.Fatalf("wide variant = %+v", v)
	}
	for _, x := range d47Variants() {
		if x.name != "wide" && x.wide() {
			t.Fatalf("%s is wide; only the wide variant may cross 1600 px", x.name)
		}
	}
}
