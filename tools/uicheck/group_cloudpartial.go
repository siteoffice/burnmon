package main

import (
	"fmt"
	"strings"
	"time"
)

// charts/d52: the cloud pull is on and some sessions are unread (2026-11-23 lane).
// The Go side marks a period that starts after the newest good copy "not
// measured" and the period that holds it with a +, and the page prints one dim
// line under the strip with the reason, not only a hover. d49 covers the pull
// off, d50 the pull on and good. The payload is the shape vendorstrip.Build
// returns for 86 unread sessions and a newest good copy of 17 Nov.
const cloudPartialTip = "Cowork (Cloud): 86 sessions are not read. Newest failure: Couldn't load the history of cloud session session_X. Check it at https://claude.ai/code/session_X?from=cli"

type cloudPartialRead struct {
	Cells      map[string][]string `json:"cells"`
	NoteText   string              `json:"noteText"`
	NoteTitle  string              `json:"noteTitle"`
	NoteHidden bool                `json:"noteHidden"`
	NoteCut    bool                `json:"noteCut"`
}

func cloudPartialProblems(r cloudPartialRead, wantNote bool, tip string) []string {
	var p []string
	if wantNote {
		cc := r.Cells["Cowork (Cloud)"]
		if len(cc) != 3 || cc[0] != "not measured" || cc[1] != "not measured" || cc[2] != "779.5M+" {
			p = append(p, fmt.Sprintf("Cowork (Cloud) cells %v, want [not measured, not measured, 779.5M+]", cc))
		}
		if tt := r.Cells["Total"]; len(tt) != 3 || tt[2] != "788.5M+" {
			p = append(p, fmt.Sprintf("Total cells %v, want the month to end with +", tt))
		}
		if r.NoteHidden || r.NoteText != tip {
			p = append(p, fmt.Sprintf("the line under the strip is hidden=%v and reads %q, want it shown with %q", r.NoteHidden, r.NoteText, tip))
		}
		if r.NoteTitle != tip {
			p = append(p, fmt.Sprintf("the line's hover title is %q, want the full text", r.NoteTitle))
		}
		if r.NoteCut && r.NoteTitle == "" {
			p = append(p, "the line is cut and has no hover title")
		}
		return p
	}
	if !r.NoteHidden || r.NoteText != "" {
		p = append(p, fmt.Sprintf("the line under the strip is hidden=%v and reads %q, want it hidden when the pull is good", r.NoteHidden, r.NoteText))
	}
	return p
}

const cloudPartialSnapJS = `(function(pull, tip, notes){
  var nowMs = Date.now();
  var cc = { agent: 'cowork-cloud', agent_label: 'Cowork (Cloud)', today: 0, week: 0, month: 779500000 };
  var total = { agent: '', agent_label: 'Total', today: 1200000, week: 2400000, month: 788500000 };
  if(notes){ cc.today_note = 'not_measured'; cc.week_note = 'not_measured'; cc.month_note = 'partial';
    total.today_note = 'partial'; total.week_note = 'partial'; total.month_note = 'partial'; }
  var snap = { now: nowMs, hist_gap_ms: 25000, sysmon_history: [], process_groups_now: [], process_groups_history: [],
    vendor_strip: { generated_at: new Date(nowMs).toISOString(), cloud_pull: pull, cloud_tip: tip, rows: [
      { agent: 'claude-code', agent_label: 'Claude Code', today: 1200000, week: 2400000, month: 9000000 }, cc], total: total },
    activity_heatmap: { rows: [] }, todo: { enabled: false }, todo_tasks: { items: [] }, headline_today: 0,
    burn: { generated_at: new Date(nowMs).toISOString(), running_window_seconds: 1800, sessions: [],
      window_start: new Date(nowMs - 1800000).toISOString(), bucket_seconds: 60, chart: [], turns: [] } };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  return true;
})`

const cloudPartialReadJS = `(function(){
  var cells = {};
  Array.prototype.forEach.call(document.querySelectorAll('#vendorStrip tr'), function(tr){
    var tds = tr.querySelectorAll('td');
    if(tds.length < 4) return;
    cells[tds[0].textContent.trim()] = [1, 2, 3].map(function(i){ return tds[i].textContent; });
  });
  var n = document.getElementById('vendorStripNote');
  return { cells: cells, noteText: n ? n.textContent : '', noteTitle: n ? (n.getAttribute('title') || '') : '',
    noteHidden: n ? n.hidden : true, noteCut: n ? n.scrollWidth > n.clientWidth + 1 : false };
})()`

func caseD52(hwnd uintptr, args []string) error {
	time.Sleep(300 * time.Millisecond)
	var ok bool
	paint := func(pull string, notes bool) (cloudPartialRead, error) {
		tip := ""
		if notes {
			tip = cloudPartialTip
		}
		js := fmt.Sprintf("%s(%q, %q, %v)", cloudPartialSnapJS, pull, tip, notes)
		if err := evalInto(js, &ok); err != nil {
			return cloudPartialRead{}, fmt.Errorf("paint: %w", err)
		}
		time.Sleep(900 * time.Millisecond) // the tween runs 600 ms
		var r cloudPartialRead
		err := evalInto(cloudPartialReadJS, &r)
		return r, err
	}
	partial, err := paint("partial", true)
	if err != nil {
		return fmt.Errorf("d52: %w", err)
	}
	if _, err := screenshot(hwnd, "d52-cloud-partial"); err != nil {
		return err
	}
	fmt.Printf("uicheck: d52 partial read: %+v\n", partial)
	bad := cloudPartialProblems(partial, true, cloudPartialTip)
	good, err := paint("ok", false)
	if err != nil {
		return fmt.Errorf("d52: %w", err)
	}
	bad = append(bad, cloudPartialProblems(good, false, "")...)
	if len(bad) > 0 {
		return fmt.Errorf("d52: %s", strings.Join(bad, "; "))
	}
	fmt.Println("uicheck: d52 ok: not measured for today and week, 779.5M+ for the month, the reason on a line under the strip, the line gone when the pull is good")
	return nil
}
