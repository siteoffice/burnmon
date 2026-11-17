package main

import (
	"fmt"
	"strings"
	"time"
)

// charts/d49: the Cowork (Cloud) row of the vendor strip shows "not measured"
// instead of 0 after the last measured cloud row, and a + on the period that
// holds it (02_roadmap\2026-11-17_cowork_cloud_not_measured_marker_spec.md).
// The payload is the shape vendorstrip.Build returns for last row 2026-11-12:
// today and week not measured, month 779.5M with a +, Total month with a +.
// A local row in the same payload stays a plain number with no tooltip.
func caseD49(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)
	const script = `(function(){
  var nowMs = Date.now();
  var until = '2026-11-12 17:43';
  var tip = 'measured until ' + until + ', later cloud sessions are not measured. Turn on Read Cowork (Cloud) sessions in Settings to read them.';
  var snap = { now: nowMs, hist_gap_ms: 25000, sysmon_history: [], process_groups_now: [], process_groups_history: [],
    vendor_strip: { generated_at: new Date(nowMs).toISOString(), cloud_measured_until: until, cloud_pull: 'off', cloud_tip: tip, rows: [
      { agent: 'claude-code', agent_label: 'Claude Code', today: 1200000, week: 2400000, month: 9000000 },
      { agent: 'cowork-cloud', agent_label: 'Cowork (Cloud)', today: 0, week: 0, month: 779500000,
        today_note: 'not_measured', week_note: 'not_measured', month_note: 'partial' }],
      total: { agent: '', agent_label: 'Total', today: 1200000, week: 2400000, month: 788500000, month_note: 'partial' } },
    activity_heatmap: { rows: [] }, todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
    burn: { generated_at: new Date(nowMs).toISOString(), running_window_seconds: 1800, sessions: [],
      window_start: new Date(nowMs - 1800000).toISOString(), bucket_seconds: 60, chart: [], turns: [] } };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  var out = {};
  Array.prototype.forEach.call(document.querySelectorAll('#vendorStrip tr'), function(tr){
    var tds = tr.querySelectorAll('td');
    if(tds.length < 4) return;
    var name = tds[0].textContent.trim();
    out[name] = [1, 2, 3].map(function(i){ return { text: tds[i].textContent, title: tds[i].getAttribute('title') || '' }; });
  });
  return out;
})()`
	type cell struct {
		Text  string `json:"text"`
		Title string `json:"title"`
	}
	var out map[string][]cell
	if err := evalInto(script, &out); err != nil {
		return fmt.Errorf("d49: eval: %w", err)
	}
	// The tween runs 600 ms; REDUCED_MOTION or a settled value both end on the final text.
	time.Sleep(900 * time.Millisecond)
	if err := evalInto(`(function(){
  var out = {};
  Array.prototype.forEach.call(document.querySelectorAll('#vendorStrip tr'), function(tr){
    var tds = tr.querySelectorAll('td');
    if(tds.length < 4) return;
    out[tds[0].textContent.trim()] = [1, 2, 3].map(function(i){ return { text: tds[i].textContent, title: tds[i].getAttribute('title') || '' }; });
  });
  return out;
})()`, &out); err != nil {
		return fmt.Errorf("d49: eval after tween: %w", err)
	}
	if _, err := screenshot(hwnd, "d49-cloud-strip"); err != nil {
		return err
	}
	tip := "measured until 2026-11-12 17:43, later cloud sessions are not measured. Turn on Read Cowork (Cloud) sessions in Settings to read them."
	var errs []string
	want := func(row string, i int, text, title string) {
		cells, ok := out[row]
		if !ok || i >= len(cells) {
			errs = append(errs, fmt.Sprintf("no %q row", row))
			return
		}
		if cells[i].Text != text || cells[i].Title != title {
			errs = append(errs, fmt.Sprintf("%s cell %d = %q (title %q), want %q (title %q)", row, i, cells[i].Text, cells[i].Title, text, title))
		}
	}
	want("Cowork (Cloud)", 0, "not measured", tip)
	want("Cowork (Cloud)", 1, "not measured", tip)
	want("Cowork (Cloud)", 2, "779.5M+", tip)
	want("Total", 0, "1.2M", "")
	want("Total", 1, "2.4M", "")
	want("Total", 2, "788.5M+", tip)
	want("Claude Code", 0, "1.2M", "")
	fmt.Printf("uicheck: d49 cells: %v\n", out)
	if len(errs) > 0 {
		return fmt.Errorf("d49: %s", strings.Join(errs, "; "))
	}
	fmt.Println("uicheck: d49 ok: not measured for today and week, 779.5M+ for the month, Total month +, local row plain")
	return nil
}
