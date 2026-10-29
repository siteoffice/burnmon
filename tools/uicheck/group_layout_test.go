package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// layout/d42: moved from check_d42_test.go.

// Each d42 decision gets one stubbed measure per failure branch that must make
// that branch fail (and, where one call can report several problems, no other
// branch of the same call), plus a measure that passes. The branch IDs (S1, D1,
// ...) are the ones listed in findings.md under Task 8.

func d42Map(kv ...any) map[string]float64 {
	m := map[string]float64{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1].(float64)
	}
	return m
}

func d42Copy(m map[string]float64) map[string]float64 {
	if m == nil {
		return nil
	}
	c := map[string]float64{}
	for k, v := range m {
		c[k] = v
	}
	return c
}

func d42Clone(r d42Read) d42Read {
	c := r
	c.H, c.Lists, c.State = d42Copy(r.H), d42Copy(r.Lists), d42Copy(r.State)
	c.Handles = append([]d42Handle(nil), r.Handles...)
	c.Items = append([]d42Item(nil), r.Items...)
	return c
}

// d42Base is a page with every part drawn, three lists, one shown handle
// (burn_30min / vendor_strip, 40 px of room) and one hidden handle.
func d42Base() d42Read {
	return d42Read{
		H:       d42Map("burn_30min", 200.0, "vendor_strip", 100.0, "activity_heatmap", 120.0, "turn_ticker", 150.0, "system_chart", 300.0, "process_groups", 200.0),
		Lists:   d42Map("waits", 100.0, "outcomes", 100.0, "sessions", 100.0),
		Handles: []d42Handle{{Above: "burn_30min", Below: "vendor_strip", Min: -20, Max: 40}, {Above: "activity_heatmap", Below: "turn_ticker", Hidden: true}},
		Drawn:   1,
	}
}

var d42Hdl = d42Handle{Above: "burn_30min", Below: "vendor_strip", Min: -20, Max: 40}

// d42WantFail fails the test unless err holds want and none of the others.
func d42WantFail(t *testing.T, err error, want string, others []string) {
	t.Helper()
	if err == nil {
		t.Fatalf("passed, want a failure with %q", want)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "d42: ") || !strings.Contains(msg, want) {
		t.Fatalf("error %q, want it to say %q", msg, want)
	}
	for _, o := range others {
		if !strings.Contains(want, o) && strings.Contains(msg, o) {
			t.Fatalf("error %q also says %q, want only %q", msg, o, want)
		}
	}
}

func TestD42CheckStart(t *testing.T) {
	p, err := d42CheckStart(d42StartMeasure{Tag: "1920x1080", R: d42Base()})
	if err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	if p.Handle != d42Hdl || p.Room != 40 || p.Shown != 1 {
		t.Fatalf("pick = %+v", p)
	}
	cases := []struct {
		name, want string
		mut        func(r *d42Read)
	}{
		{"S1_part_not_drawn", "not every part is drawn at 1920x1080", func(r *d42Read) { r.H["turn_ticker"] = -1 }},
		{"S2_lists_not_drawn", "lists are not all drawn", func(r *d42Read) { delete(r.Lists, "sessions") }},
		{"S3_hidden_with_range", "is hidden but has a range of -5 to 5 px", func(r *d42Read) { r.Handles[1].Min, r.Handles[1].Max = -5, 5 }},
		{"S4_dead_bar", "is drawn as a dead bar", func(r *d42Read) { r.Handles[0].Min, r.Handles[0].Max = 0, 0 }},
		{"S5_drawn_count", "the page draws 2 handles, __bdevFitHandles reports 1 shown", func(r *d42Read) { r.Drawn = 2 }},
		{"S6_room_under_12", "no handle has 12 px of room (shown 1, largest 10 px)", func(r *d42Read) { r.Handles[0].Min, r.Handles[0].Max = -5, 10 }},
		{"S6_none_shown", "no handle has 12 px of room (shown 0, largest -1 px)", func(r *d42Read) { r.Handles = r.Handles[1:]; r.Drawn = 0 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := d42Clone(d42Base())
			c.mut(&r)
			_, err := d42CheckStart(d42StartMeasure{Tag: "1920x1080", R: r})
			d42WantFail(t, err, c.want, nil)
		})
	}
}

// d42DragPass is a +10 px drag: burn_30min +10, vendor_strip -4, the lists
// -6 (2 each), the other column +6, no overflow.
func d42DragPass() d42DragMeasure {
	after := d42Clone(d42Base())
	after.H["burn_30min"], after.H["vendor_strip"], after.H["system_chart"] = 210, 96, 306
	for k := range after.Lists {
		after.Lists[k] = 98
	}
	return d42DragMeasure{Hdl: d42Hdl, Dy: 10, Before: d42Base(), After: after}
}

func TestD42CheckDrag(t *testing.T) {
	if err := d42CheckDrag(d42DragPass()); err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	up := d42DragPass()
	up.Dy = -10
	up.After.H["burn_30min"], up.After.H["vendor_strip"] = 196, 110
	if err := d42CheckDrag(up); err != nil {
		t.Fatalf("pass measure (drag up) failed: %v", err)
	}
	if d := up.deltas(); d.Grow != "vendor_strip" || d.Part != "burn_30min" {
		t.Fatalf("drag up grows %s and takes from %s", d.Grow, d.Part)
	}
	all := []string{"burn_30min grew by", "px, want between", "do not add up", "the lists grew", "is not part of the drag", "the other column changed", "the page overflows"}
	setLists := func(a *d42Read, v float64) {
		for k := range a.Lists {
			a.Lists[k] = v
		}
	}
	cases := []struct {
		name, want string
		mut        func(a *d42Read)
	}{
		{"D1_grow_off", "burn_30min grew by 15.0 px, want 10", func(a *d42Read) { a.H["burn_30min"] = 215 }},
		{"D2_partner_grew", "vendor_strip changed by 2.0 px, want between -10 and 0", func(a *d42Read) {
			a.H["vendor_strip"], a.H["system_chart"] = 102, 312
			setLists(a, 96)
		}},
		{"D2_partner_gave_too_much", "vendor_strip changed by -13.0 px, want between -10 and 0", func(a *d42Read) {
			a.H["vendor_strip"], a.H["system_chart"] = 87, 300
			setLists(a, 100)
			a.Lists["waits"] = 101
		}},
		{"D3_sum_off", "partner 4.0 px and lists 9.0 px do not add up to 10", func(a *d42Read) {
			a.H["system_chart"] = 309
			setLists(a, 97)
		}},
		{"D4_lists_grew", "the lists grew by 1.5 px during a drag", func(a *d42Read) {
			a.H["vendor_strip"], a.H["system_chart"] = 88, 300
			setLists(a, 100.5)
		}},
		{"D5_other_part_moved", "activity_heatmap moved 3.0 px but is not part of the drag", func(a *d42Read) { a.H["activity_heatmap"] = 123 }},
		{"D6_other_column_off", "the other column changed by 0.0 px but the lists gave 6.0 px", func(a *d42Read) { a.H["system_chart"] = 300 }},
		{"D7_overflow", "the page overflows by 2.0 px", func(a *d42Read) { a.BodyOver = 2 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := d42DragPass()
			c.mut(&m.After)
			d42WantFail(t, d42CheckDrag(m), c.want, all)
		})
	}
}

func d42SavedPass() d42SavedMeasure {
	return d42SavedMeasure{File: d42Map("burn_30min", 0.4, "vendor_strip", 0.1, "host_lists", 0.5), Hdl: d42Hdl, DLists: -6}
}

func TestD42CheckSaved(t *testing.T) {
	if err := d42CheckSaved(d42SavedPass()); err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	cases := []struct {
		name, want string
		mut        func(m *d42SavedMeasure)
	}{
		{"V1_no_heights", "the view file has no heights_standard after the drag", func(m *d42SavedMeasure) { m.File = nil }},
		{"V2_share_out_of_range", "heights_standard[vendor_strip] = 1.5, want a value in (0, 1]", func(m *d42SavedMeasure) { m.File["vendor_strip"] = 1.5 }},
		{"V2_share_missing", "heights_standard[burn_30min] = 0, want a value in (0, 1]", func(m *d42SavedMeasure) { delete(m.File, "burn_30min") }},
		{"V3_no_host_lists", "the lists gave 6.0 px but heights_standard[host_lists] = 0", func(m *d42SavedMeasure) { delete(m.File, "host_lists") }},
		{"V4_heights_plain", "the view file has heights_plain", func(m *d42SavedMeasure) { m.Plain = true }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := d42SavedPass()
			m.File = d42Copy(m.File)
			c.mut(&m)
			d42WantFail(t, d42CheckSaved(m), c.want, nil)
		})
	}
}

func TestD42CheckBinding(t *testing.T) {
	file := d42Map("burn_30min", 0.4, "vendor_strip", 0.1)
	if err := d42CheckBinding(d42Copy(file), file); err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	t.Run("B1_key_count", func(t *testing.T) {
		d42WantFail(t, d42CheckBinding(d42Map("burn_30min", 0.4), file), "bdevSections returned", nil)
	})
	t.Run("B2_value", func(t *testing.T) {
		d42WantFail(t, d42CheckBinding(d42Map("burn_30min", 0.4, "vendor_strip", 0.2), file), "bdevSections heights_standard[vendor_strip] = 0.2, the file 0.1", nil)
	})
}

func d42ReloadPass() d42ReloadMeasure {
	file := d42Map("burn_30min", 0.4, "vendor_strip", 0.1)
	return d42ReloadMeasure{Ready: true, State: d42Copy(file), File: file}
}

func TestD42CheckReloaded(t *testing.T) {
	if err := d42CheckReloaded(d42ReloadPass()); err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	cases := []struct {
		name, want string
		mut        func(m *d42ReloadMeasure)
	}{
		{"L1_not_reloaded", `window.__d42Marker is still "before the reload"`, func(m *d42ReloadMeasure) { m.Marker = "before the reload" }},
		{"L2_not_back", "did not come back after location.reload within 20 s", func(m *d42ReloadMeasure) { m.Ready = false }},
		{"L3_no_state", "the reloaded page loaded no heights_standard", func(m *d42ReloadMeasure) { m.State = nil }},
		{"L4_key_count", "the reloaded page holds map[", func(m *d42ReloadMeasure) { m.State["host_lists"] = 0.5 }},
		{"L5_value", "the reloaded page holds heights[burn_30min] = 0.5, the file 0.4", func(m *d42ReloadMeasure) { m.State["burn_30min"] = 0.5 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := d42ReloadPass()
			c.mut(&m)
			d42WantFail(t, d42CheckReloaded(m), c.want, nil)
		})
	}
	file := d42Map("burn_30min", 0.4, "vendor_strip", 0.1)
	if err := d42CheckFileKept(d42Copy(file), file); err != nil {
		t.Fatalf("pass measure (file kept) failed: %v", err)
	}
	t.Run("L6_file_changed", func(t *testing.T) {
		d42WantFail(t, d42CheckFileKept(d42Map("burn_30min", 0.4, "vendor_strip", 0.2), file), "the file changed by the reload", nil)
	})
}

// d42LayoutPass: saved shares 0.4/0.1 of a burn column of 1.0, the saved page
// holds them exactly, host_lists 0.5 of a 420 px range gives 210 px, and the
// lists gave 210 px against the default; burn_30min is 100 px over its default.
func d42LayoutPass() d42LayoutMeasure {
	file := d42Map("burn_30min", 0.4, "vendor_strip", 0.1, "activity_heatmap", 0.2, "turn_ticker", 0.3, "host_lists", 0.5)
	def := d42Base()
	def.H = d42Map("burn_30min", 300.0, "vendor_strip", 150.0, "activity_heatmap", 250.0, "turn_ticker", 300.0, "system_chart", 300.0, "process_groups", 200.0)
	def.Lists = d42Map("waits", 150.0, "outcomes", 150.0, "sessions", 150.0)
	for _, id := range []string{"waits", "outcomes", "sessions"} {
		def.Items = append(def.Items, d42Item{ID: id, Kind: "seclist", H: 150, Floor: 60, Nat: 200})
	}
	saved := d42Clone(def)
	saved.H = d42Map("burn_30min", 400.0, "vendor_strip", 100.0, "activity_heatmap", 200.0, "turn_ticker", 300.0, "system_chart", 300.0, "process_groups", 200.0)
	saved.Lists = d42Map("waits", 80.0, "outcomes", 80.0, "sessions", 80.0)
	return d42LayoutMeasure{Hdl: d42Hdl, Grow: "burn_30min", File: file, State: d42Copy(file), Default: def, Saved: saved}
}

func TestD42CheckReloadLayout(t *testing.T) {
	lists, err := d42CheckReloadLayout(d42LayoutPass())
	if err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	if lists.Got != 210 || lists.Want != 210 {
		t.Fatalf("lists = %+v, want 210 and 210", lists)
	}
	all := []string{"has share", "the lists gave", "taller than its default"}
	cases := []struct {
		name, want string
		mut        func(m *d42LayoutMeasure)
	}{
		{"R1_share_off", "burn_30min has share 0.4340 of its column, saved 0.4000", func(m *d42LayoutMeasure) { m.Saved.H["burn_30min"] = 460 }},
		{"R2_list_draw_off", "the lists gave 180.0 px relative to the default", func(m *d42LayoutMeasure) {
			for k := range m.Saved.Lists {
				m.Saved.Lists[k] = 90
			}
		}},
		{"R3_layout_not_visible", "burn_30min is only 1.0 px taller than its default", func(m *d42LayoutMeasure) { m.Default.H["burn_30min"] = 399 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := d42LayoutPass()
			c.mut(&m)
			_, err := d42CheckReloadLayout(m)
			d42WantFail(t, err, c.want, all)
		})
	}
}

func d42ResizePass() d42ResizeMeasure {
	file := d42Map("burn_30min", 0.4, "vendor_strip", 0.1, "activity_heatmap", 0.2, "turn_ticker", 0.3, "host_lists", 0.5)
	r := d42Base()
	r.H = d42Map("burn_30min", 400.0, "vendor_strip", 100.0, "activity_heatmap", 200.0, "turn_ticker", 300.0, "system_chart", 300.0, "process_groups", 200.0)
	r.State = d42Copy(file)
	for _, id := range []string{"waits", "outcomes", "sessions"} {
		r.Items = append(r.Items, d42Item{ID: id, Kind: "seclist", H: 150, Floor: 60, Nat: 200})
	}
	return d42ResizeMeasure{Hdl: d42Hdl, File: file, OnDisk: d42Copy(file), R: r}
}

func TestD42CheckResize(t *testing.T) {
	rs, err := d42CheckResize(d42ResizePass())
	if err != nil || rs.ByFloor || rs.Share != 0.4 || rs.Saved != 0.4 {
		t.Fatalf("pass measure: %+v, %v", rs, err)
	}
	floor := d42ResizePass()
	floor.R.H["burn_30min"] = 600
	for i := range floor.R.Items {
		floor.R.Items[i].H = 60
	}
	if rs, err := d42CheckResize(floor); err != nil || !rs.ByFloor {
		t.Fatalf("pass measure (lists at floor): %+v, %v", rs, err)
	}
	cases := []struct {
		name, want string
		mut        func(m *d42ResizeMeasure)
	}{
		{"Z1_file_changed", "the resize changed the saved heights", func(m *d42ResizeMeasure) { m.OnDisk["burn_30min"] = 0.41 }},
		{"Z1_page_changed", "the resize changed the saved heights", func(m *d42ResizeMeasure) { m.R.State["vendor_strip"] = 0.2 }},
		{"Z2_share_off", "after the resize the share is 0.5000 (saved 0.4000) and the lists are not at their floor", func(m *d42ResizeMeasure) { m.R.H["burn_30min"] = 600 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := d42ResizePass()
			m.R = d42Clone(m.R)
			m.OnDisk = d42Copy(m.OnDisk)
			c.mut(&m)
			_, err := d42CheckResize(m)
			d42WantFail(t, err, c.want, nil)
		})
	}
}

func TestD42CheckBack(t *testing.T) {
	r2 := d42Base()
	near := d42Clone(r2)
	near.H["system_chart"] += 2
	if err := d42CheckBack(r2, near, "1920x1080"); err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	t.Run("K1_back_differs", func(t *testing.T) {
		off := d42Clone(r2)
		off.H["system_chart"] += 3
		d42WantFail(t, d42CheckBack(r2, off, "1920x1080"), "back at 1920x1080 system_chart differs by 3.0 px", nil)
	})
}

func TestD42CheckReset(t *testing.T) {
	def := d42Base()
	near := d42Clone(def)
	near.Lists["waits"] += 2
	if err := d42CheckResetLayout(def, near); err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	if err := d42CheckResetCleared(nil, false, nil); err != nil {
		t.Fatalf("pass measure (cleared) failed: %v", err)
	}
	t.Run("T1_layout_differs", func(t *testing.T) {
		off := d42Clone(def)
		off.Lists["waits"] += 3
		d42WantFail(t, d42CheckResetLayout(def, off), "after Reset heights list waits differs by 3.0 px from the default layout", nil)
	})
	t.Run("T2_file_has_heights", func(t *testing.T) {
		d42WantFail(t, d42CheckResetCleared(d42Map("burn_30min", 0.4), false, nil), "the view file still has heights", nil)
	})
	t.Run("T2_file_has_plain", func(t *testing.T) {
		d42WantFail(t, d42CheckResetCleared(nil, true, nil), "the view file still has heights (standard map[], plain true)", nil)
	})
	t.Run("T3_page_state", func(t *testing.T) {
		d42WantFail(t, d42CheckResetCleared(nil, false, d42Map("burn_30min", 0.4)), "the page state still holds", nil)
	})
}

// d42PartnerBase is step 8 before the drag: the activity_heatmap /
// turn_ticker handle drawn, activity_heatmap 50 px above its floor of 120.
func d42PartnerBase() d42Read {
	r := d42Clone(d42Base())
	r.H["activity_heatmap"] = 170
	r.Handles = []d42Handle{{Above: "activity_heatmap", Below: "turn_ticker", Min: -80, Max: 30}}
	r.Items = []d42Item{{ID: "activityHeatmapRow", Kind: "fixed", H: 170, Floor: 120, Nat: 120}}
	return r
}

func TestD42PartnerShares(t *testing.T) {
	s := d42PartnerShares(d42Base())
	// burn column: 200 + 100 + 150 = 450 beside the partner.
	want := d42Map("activity_heatmap", 0.5, "host_lists", 1.0, "burn_30min", 0.2222, "vendor_strip", 0.1111, "turn_ticker", 0.1667)
	if len(s) != len(want) {
		t.Fatalf("shares %v, want %v", s, want)
	}
	for k, v := range want {
		if s[k] != v {
			t.Fatalf("shares %v, want %v", s, want)
		}
	}
}

func TestD42CheckPartnerRoom(t *testing.T) {
	hdl, dy, err := d42CheckPartnerRoom(d42PartnerBase())
	if err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	if hdl.Above != "activity_heatmap" || hdl.Below != "turn_ticker" || dy != -40 {
		t.Fatalf("handle %+v, dy %v, want activity_heatmap / turn_ticker and -40", hdl, dy)
	}
	small := d42PartnerBase()
	small.H["activity_heatmap"] = 135.5
	if _, dy, err := d42CheckPartnerRoom(small); err != nil || dy != -13 {
		t.Fatalf("15.5 px of room: dy %v, err %v, want -13 and a pass", dy, err)
	}
	cases := []struct {
		name, want string
		mut        func(r *d42Read)
	}{
		{"P1_handle_missing", "handle is not drawn with the saved shares", func(r *d42Read) { r.Handles = nil }},
		{"P1_handle_hidden", "handle is not drawn with the saved shares", func(r *d42Read) { r.Handles[0].Hidden = true }},
		{"P2_no_item", "the fit pass has no item activityHeatmapRow", func(r *d42Read) { r.Items = nil }},
		{"P3_no_room", "left activity_heatmap 7.0 px above its floor of 120 px", func(r *d42Read) { r.H["activity_heatmap"] = 127 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := d42PartnerBase()
			c.mut(&r)
			_, _, err := d42CheckPartnerRoom(r)
			d42WantFail(t, err, c.want, nil)
		})
	}
}

// d42PartnerDragPass is a -20 px drag on activity_heatmap / turn_ticker:
// turn_ticker +20, activity_heatmap -20, the lists and the other column still.
func d42PartnerDragPass() d42DragMeasure {
	before := d42PartnerBase()
	after := d42Clone(before)
	after.H["turn_ticker"], after.H["activity_heatmap"] = 170, 150
	return d42DragMeasure{Hdl: before.Handles[0], Dy: -20, Before: before, After: after}
}

func TestD42CheckPartnerDrag(t *testing.T) {
	if err := d42CheckPartnerDrag(d42PartnerDragPass()); err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	if err := d42CheckDrag(d42PartnerDragPass()); err != nil {
		t.Fatalf("pass measure fails step 3's checks: %v", err)
	}
	all := []string{"activity_heatmap gave", "the lists changed by"}
	cases := []struct {
		name, want string
		mut        func(a *d42Read)
	}{
		{"E1_partner_gave_less", "activity_heatmap gave 12.0 px, want 20 (the whole drag)", func(a *d42Read) {
			a.H["activity_heatmap"] = 158
			a.Lists["waits"] = 99
		}},
		{"E2_lists_moved", "the lists changed by -2.0 px, want under 2", func(a *d42Read) { a.Lists["waits"] = 98 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := d42PartnerDragPass()
			m.After = d42Clone(m.After)
			c.mut(&m.After)
			d42WantFail(t, d42CheckPartnerDrag(m), c.want, all)
		})
	}
}

func TestD42CheckSharesSaved(t *testing.T) {
	sent := d42Map("activity_heatmap", 0.5, "host_lists", 1.0, "turn_ticker", 0.2)
	if err := d42CheckSharesSaved("ok", sent, d42Copy(sent)); err != nil {
		t.Fatalf("pass measure failed: %v", err)
	}
	t.Run("W1_not_answered", func(t *testing.T) {
		d42WantFail(t, d42CheckSharesSaved("", sent, d42Copy(sent)), "bdevSetSections did not save the shares within 10 s", nil)
	})
	t.Run("W1_binding_error", func(t *testing.T) {
		d42WantFail(t, d42CheckSharesSaved("error boom", sent, d42Copy(sent)), `did not save the shares within 10 s ("error boom")`, nil)
	})
	t.Run("W2_file_missing_key", func(t *testing.T) {
		f := d42Copy(sent)
		delete(f, "host_lists")
		d42WantFail(t, d42CheckSharesSaved("ok", sent, f), "partner leg: the view file holds", nil)
	})
	t.Run("W2_file_value_differs", func(t *testing.T) {
		f := d42Copy(sent)
		f["activity_heatmap"] = 0.4
		d42WantFail(t, d42CheckSharesSaved("ok", sent, f), "partner leg: the view file holds", nil)
	})
}

// layout/d42: moved from check_d42_snapshot_test.go.

// d42Snapshot counts its turns back from d42TurnsBase, not the clock: two
// builds are the same bytes, and the newest turn sits 20 s before the base.
func TestD42SnapshotIsFixed(t *testing.T) {
	a, err := d42Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	b, err := d42Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("two builds differ (%d and %d bytes)", len(a), len(b))
	}
	var m struct {
		Burn struct {
			Turns []struct {
				At string `json:"at"`
			} `json:"turns"`
		} `json:"burn"`
	}
	if err := json.Unmarshal(a, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Burn.Turns) != 24 {
		t.Fatalf("burn.turns has %d turns, want 24", len(m.Burn.Turns))
	}
	if got, want := m.Burn.Turns[0].At, "2026-10-23T09:57:40Z"; got != want {
		t.Fatalf("first turn at %s, want %s", got, want)
	}
	if got, want := m.Burn.Turns[23].At, "2026-10-23T09:50:00Z"; got != want {
		t.Fatalf("last turn at %s, want %s", got, want)
	}
}
