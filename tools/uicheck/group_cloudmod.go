package main

import (
	"fmt"
	"strings"
	"time"
)

// charts/d53: the burnmon-cloud mod's status on the line under the strip
// (decision 3 of 02_roadmap\2026-11-24_cloud_mod_route_plan.md). "no delivery
// yet" rides along only while a pull reason shows; a real delivery shows on
// its own, also with the pull good; with the pull good and no delivery the
// line stays hidden. The payloads are the shapes vendorstrip.Build returns.
const cloudModNone = "cloud mod: no delivery yet"
const cloudModDelivered = "cloud mod: newest delivery 2026-11-24 09:30"

// cloudModLineProblems compares the line with the text it must show; an empty
// want means the line must be hidden.
func cloudModLineProblems(r cloudPartialRead, want string) []string {
	if want == "" {
		if !r.NoteHidden || r.NoteText != "" {
			return []string{fmt.Sprintf("the line is hidden=%v and reads %q, want it hidden", r.NoteHidden, r.NoteText)}
		}
		return nil
	}
	var p []string
	if r.NoteHidden || r.NoteText != want {
		p = append(p, fmt.Sprintf("the line is hidden=%v and reads %q, want %q", r.NoteHidden, r.NoteText, want))
	}
	if r.NoteTitle != want {
		p = append(p, fmt.Sprintf("the line's hover title is %q, want the full text", r.NoteTitle))
	}
	return p
}

// The d52 snapshot with the two cloud_mod fields added to the vendor strip.
const cloudModSnapJS = `(function(pull, tip, notes, mod, delivered){
  var nowMs = Date.now();
  var cc = { agent: 'cowork-cloud', agent_label: 'Cowork (Cloud)', today: 0, week: 0, month: 779500000 };
  var total = { agent: '', agent_label: 'Total', today: 1200000, week: 2400000, month: 788500000 };
  if(notes){ cc.today_note = 'not_measured'; cc.week_note = 'not_measured'; cc.month_note = 'partial';
    total.today_note = 'partial'; total.week_note = 'partial'; total.month_note = 'partial'; }
  var snap = { now: nowMs, hist_gap_ms: 25000, sysmon_history: [], process_groups_now: [], process_groups_history: [],
    vendor_strip: { generated_at: new Date(nowMs).toISOString(), cloud_pull: pull, cloud_tip: tip,
      cloud_mod: mod, cloud_mod_delivered: delivered, rows: [
      { agent: 'claude-code', agent_label: 'Claude Code', today: 1200000, week: 2400000, month: 9000000 }, cc], total: total },
    activity_heatmap: { rows: [] }, todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
    burn: { generated_at: new Date(nowMs).toISOString(), running_window_seconds: 1800, sessions: [],
      window_start: new Date(nowMs - 1800000).toISOString(), bucket_seconds: 60, chart: [], turns: [] } };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  return true;
})`

func caseD53(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)
	cases := []struct {
		name, pull, tip, mod string
		notes, delivered     bool
		want                 string
	}{
		{"partial, no delivery", "partial", cloudPartialTip, cloudModNone, true, false, cloudPartialTip + " | " + cloudModNone},
		{"good, delivered", "ok", "", cloudModDelivered, false, true, cloudModDelivered},
		{"good, no delivery", "ok", "", cloudModNone, false, false, ""},
	}
	var bad []string
	for i, c := range cases {
		var ok bool
		js := fmt.Sprintf("%s(%q, %q, %v, %q, %v)", cloudModSnapJS, c.pull, c.tip, c.notes, c.mod, c.delivered)
		if err := evalInto(js, &ok); err != nil {
			return fmt.Errorf("d53 paint %s: %w", c.name, err)
		}
		time.Sleep(900 * time.Millisecond) // the tween runs 600 ms
		var r cloudPartialRead
		if err := evalInto(cloudPartialReadJS, &r); err != nil {
			return fmt.Errorf("d53 read %s: %w", c.name, err)
		}
		if i == 0 {
			if _, err := screenshot(hwnd, "d53-cloud-mod"); err != nil {
				return err
			}
		}
		fmt.Printf("uicheck: d53 %s read: hidden=%v text=%q\n", c.name, r.NoteHidden, r.NoteText)
		for _, p := range cloudModLineProblems(r, c.want) {
			bad = append(bad, c.name+": "+p)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("d53: %s", strings.Join(bad, "; "))
	}
	fmt.Println("uicheck: d53 ok: no delivery yet appended to the reason, a delivery shown on its own, nothing with the pull good and no delivery")
	return nil
}
