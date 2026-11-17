package main

import (
	"fmt"
	"strings"
	"time"
)

// charts/d50: the cloud pull switch (02_roadmap\2026-11-17_cowork_cloud_pull_step5_spec.md).
// The gear holds "Read Cowork (Cloud) sessions" with its three lines, and with
// the pull on and good the Cowork (Cloud) row shows plain numbers: no "not
// measured", no +, no tooltip. The check reads the page only; it never clicks
// the switch, which writes the shared burnmon.json.
func caseD50(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)
	const script = `(function(){
  var nowMs = Date.now();
  var snap = { now: nowMs, hist_gap_ms: 25000, sysmon_history: [], process_groups_now: [], process_groups_history: [],
    vendor_strip: { generated_at: new Date(nowMs).toISOString(), cloud_measured_until: '2026-11-12 17:43', cloud_pull: 'ok', rows: [
      { agent: 'claude-code', agent_label: 'Claude Code', today: 1200000, week: 2400000, month: 9000000 },
      { agent: 'cowork-cloud', agent_label: 'Cowork (Cloud)', today: 17787118, week: 84000000, month: 800000000 }],
      total: { agent: '', agent_label: 'Total', today: 18987118, week: 86400000, month: 809000000 } },
    activity_heatmap: { rows: [] }, todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
    burn: { generated_at: new Date(nowMs).toISOString(), running_window_seconds: 1800, sessions: [],
      window_start: new Date(nowMs - 1800000).toISOString(), bucket_seconds: 60, chart: [], turns: [] } };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  return true;
})()`
	var painted bool
	if err := evalInto(script, &painted); err != nil {
		return fmt.Errorf("d50: paint: %w", err)
	}
	// The tween runs 600 ms; wait for the final text.
	time.Sleep(900 * time.Millisecond)
	var out struct {
		Switch bool     `json:"switch"`
		Label  string   `json:"label"`
		Hints  []string `json:"hints"`
		Cells  []string `json:"cells"`
		Titles []string `json:"titles"`
	}
	if err := evalInto(`(function(){
  var cb = document.getElementById('secCloudPull');
  var row = cb ? cb.closest('.setcloud') : null;
  var o = { switch: !!cb, label: cb ? cb.parentElement.textContent.trim() : '', hints: [], cells: [], titles: [] };
  if(row) Array.prototype.forEach.call(row.querySelectorAll('.sechint'), function(h){ o.hints.push(h.textContent.trim()); });
  Array.prototype.forEach.call(document.querySelectorAll('#vendorStrip tr'), function(tr){
    var tds = tr.querySelectorAll('td');
    if(tds.length < 4 || tds[0].textContent.trim() !== 'Cowork (Cloud)') return;
    for(var i = 1; i <= 3; i++){ o.cells.push(tds[i].textContent); o.titles.push(tds[i].getAttribute('title') || ''); }
  });
  return o;
})()`, &out); err != nil {
		return fmt.Errorf("d50: read: %w", err)
	}
	if _, err := screenshot(hwnd, "d50-cloud-pull-on"); err != nil {
		return err
	}
	var errs []string
	if !out.Switch || out.Label != "Read Cowork (Cloud) sessions" {
		errs = append(errs, fmt.Sprintf("switch %v with label %q", out.Switch, out.Label))
	}
	if len(out.Hints) != 3 {
		errs = append(errs, fmt.Sprintf("%d lines under the switch, want 3", len(out.Hints)))
	}
	if strings.Join(out.Cells, "|") != "17.8M|84.0M|800.0M" {
		errs = append(errs, fmt.Sprintf("Cowork (Cloud) cells %v, want plain 17.8M, 84.0M, 800.0M", out.Cells))
	}
	for i, tt := range out.Titles {
		if tt != "" {
			errs = append(errs, fmt.Sprintf("cell %d has tooltip %q, want none", i, tt))
		}
	}
	fmt.Printf("uicheck: d50 read: %+v\n", out)
	if len(errs) > 0 {
		return fmt.Errorf("d50: %s", strings.Join(errs, "; "))
	}
	fmt.Println("uicheck: d50 ok: switch with three lines in the gear, Cowork (Cloud) plain numbers with the pull on")
	return nil
}
