package centre

import (
	"reflect"
	"strconv"
	"testing"
)

func TestHistoryAdd(t *testing.T) {
	h := HistoryAdd(nil, View{Page: "overview"})
	if len(h) != 1 || h[0].Page != "overview" {
		t.Fatalf("empty history add: %+v", h)
	}
	h = HistoryAdd(h, View{Page: "unit", Unit: "a", Tab: "Structure"})
	if len(h) != 2 {
		t.Fatalf("page change did not append: %+v", h)
	}
	h = HistoryAdd(h, View{Page: "unit", Unit: "b", Tab: "Structure"})
	if len(h) != 3 || h[2].Unit != "b" {
		t.Fatalf("unit change did not append: %+v", h)
	}
	h = HistoryAdd(h, View{Page: "unit", Unit: "b", Tab: "Flows"})
	if len(h) != 4 || h[3].Tab != "Flows" {
		t.Fatalf("tab change did not append: %+v", h)
	}
	h = HistoryAdd(h, View{Page: "file", File: "b/x.md"})
	h = HistoryAdd(h, View{Page: "file", File: "b/y.md"})
	if len(h) != 6 || h[5].File != "b/y.md" {
		t.Fatalf("file change did not append: %+v", h)
	}
	h = HistoryAdd(h, View{Page: "board", Board: "b/one.html"})
	h = HistoryAdd(h, View{Page: "board", Board: "b/two.html"})
	if len(h) != 8 || h[7].Board != "b/two.html" {
		t.Fatalf("board change did not append: %+v", h)
	}
}

func TestHistoryCap(t *testing.T) {
	var h []View
	for i := 0; i < 51; i++ {
		h = HistoryAdd(h, View{Page: "p" + strconv.Itoa(i)})
	}
	if len(h) != HistoryMax || HistoryMax != 50 {
		t.Fatalf("len %d, max %d, want 50", len(h), HistoryMax)
	}
	if h[0].Page != "p1" {
		t.Errorf("oldest %q, want p1 (p0 dropped)", h[0].Page)
	}
	if h[len(h)-1].Page != "p50" {
		t.Errorf("newest %q, want p50", h[len(h)-1].Page)
	}
}

func TestHistoryAddKeepsInput(t *testing.T) {
	h := []View{{Page: "overview"}, {Page: "unit", Unit: "a"}}
	keep := append([]View(nil), h...)
	HistoryAdd(h, View{Page: "unit", Unit: "a", Sel: "file:x"})
	HistoryAdd(h, View{Page: "waiting"})
	if !reflect.DeepEqual(h, keep) {
		t.Errorf("input changed: %+v", h)
	}
}

func replaceCase(t *testing.T, next View, check func(View) bool) {
	t.Helper()
	base := View{Page: "unit", Unit: "a", Tab: "Structure"}
	h := HistoryAdd(HistoryAdd(nil, View{Page: "overview"}), base)
	got := HistoryAdd(h, next)
	if len(got) != 2 {
		t.Fatalf("len %d, want 2 (replaced, not appended)", len(got))
	}
	if !check(got[1]) {
		t.Errorf("last entry %+v does not hold the new value", got[1])
	}
}

func TestHistorySelectionOnly(t *testing.T) {
	replaceCase(t, View{Page: "unit", Unit: "a", Tab: "Structure", Sel: "file:a/x.md"},
		func(v View) bool { return v.Sel == "file:a/x.md" })
}

func TestHistoryListPageOnly(t *testing.T) {
	replaceCase(t, View{Page: "unit", Unit: "a", Tab: "Structure", ListPage: 2},
		func(v View) bool { return v.ListPage == 2 })
}

func TestHistoryOverviewSwitchOnly(t *testing.T) {
	replaceCase(t, View{Page: "unit", Unit: "a", Tab: "Structure", Overview: "plan"},
		func(v View) bool { return v.Overview == "plan" })
}

func TestHistoryBackEmpty(t *testing.T) {
	for _, h := range [][]View{{}, {{Page: "overview"}}} {
		r := HistoryBack(h, false)
		if r.Moved || r.ClosePopup {
			t.Errorf("len %d: %+v, want not moved", len(h), r)
		}
		if !reflect.DeepEqual(r.History, h) {
			t.Errorf("len %d: history changed to %+v", len(h), r.History)
		}
	}
}

func TestHistoryBack(t *testing.T) {
	h := []View{{Page: "overview"}, {Page: "unit", Unit: "a", Sel: "file:a/x.md"}, {Page: "file", File: "a/x.md"}}
	keep := append([]View(nil), h...)
	r := HistoryBack(h, false)
	if !r.Moved || r.ClosePopup {
		t.Fatalf("%+v, want moved", r)
	}
	if !reflect.DeepEqual(r.Show, h[1]) {
		t.Errorf("show %+v, want %+v", r.Show, h[1])
	}
	if len(r.History) != 2 || !reflect.DeepEqual(r.History[1], h[1]) {
		t.Errorf("history %+v, want the first two", r.History)
	}
	if !reflect.DeepEqual(h, keep) {
		t.Errorf("input changed: %+v", h)
	}
}

func TestHistoryBackPopup(t *testing.T) {
	for _, h := range [][]View{{{Page: "overview"}}, {{Page: "overview"}, {Page: "waiting"}, {Page: "agents"}}} {
		r := HistoryBack(h, true)
		if !r.ClosePopup || r.Moved {
			t.Errorf("len %d: %+v, want ClosePopup and not moved", len(h), r)
		}
		if !reflect.DeepEqual(r.History, h) {
			t.Errorf("len %d: history changed to %+v", len(h), r.History)
		}
	}
}

func TestCanGoBack(t *testing.T) {
	if CanGoBack(nil) || CanGoBack([]View{{Page: "overview"}}) {
		t.Error("CanGoBack true with nothing before the current view")
	}
	if !CanGoBack([]View{{Page: "overview"}, {Page: "waiting"}}) {
		t.Error("CanGoBack false with two entries")
	}
}
