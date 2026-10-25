//go:build windows

package main

import (
	"strings"
	"testing"
)

// TestAssemblePageSplicesStation: page.html carries the marker exactly once
// and the assembled page carries both atlases, the startup theme and
// station.js in its place, in that order.
func TestAssemblePageSplicesStation(t *testing.T) {
	if n := strings.Count(pageHTML, stationMarker); n != 1 {
		t.Fatalf("page.html has %d %s markers, want 1", n, stationMarker)
	}
	for _, theme := range []string{"site", "space"} {
		page := assemblePage(theme, "dark")
		if strings.Contains(page, stationMarker) {
			t.Fatal("marker left in the assembled page")
		}
		want := `<script>window.BM_STATION_THEME = "` + theme + `";</script>`
		if n := strings.Count(page, want); n != 1 {
			t.Fatalf("theme %s: %d theme scripts %q, want 1", theme, n, want)
		}
		order := []string{"window.BM_STATION_ATLAS =", "window.BM_STATION_ATLAS_SITE =", "window.BM_STATION_THEME =", "window.BMStation ="}
		last := -1
		for _, s := range order {
			i := strings.Index(page, s)
			if i < 0 {
				t.Fatalf("assembled page lacks %s", s)
			}
			if i < last {
				t.Fatalf("%s is out of order (atlases, theme, then station.js)", s)
			}
			last = i
		}
	}
}

// TestAssemblePageThemeIsClamped: the theme lands in a script string, so
// anything but the two known names must fall back to "space".
func TestAssemblePageThemeIsClamped(t *testing.T) {
	page := assemblePage(`x";alert(1);//`, "dark")
	if strings.Contains(page, "alert(1)") {
		t.Fatal("unknown theme reached the page")
	}
	if !strings.Contains(page, `window.BM_STATION_THEME = "space";`) {
		t.Fatal("unknown theme did not fall back to space")
	}
}

// TestAssemblePageUITheme: light sets data-theme on <html> in the markup, so
// the page is light before its first paint; dark (and anything unknown)
// leaves the tag as it is.
func TestAssemblePageUITheme(t *testing.T) {
	const plain, light = `<html lang="en">`, `<html lang="en" data-theme="light">`
	if n := strings.Count(pageHTML, plain); n != 1 {
		t.Fatalf("page.html has %d %s tags, want 1", n, plain)
	}
	page := assemblePage("site", "light")
	if !strings.Contains(page, light) || strings.Contains(page, plain) {
		t.Fatal("light: <html> tag lacks data-theme")
	}
	for _, ui := range []string{"dark", "", "blue"} {
		page = assemblePage("site", ui)
		if !strings.Contains(page, plain) || strings.Contains(page, light) {
			t.Fatalf("ui theme %q: <html> tag changed", ui)
		}
	}
}
