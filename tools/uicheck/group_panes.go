package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

// panes/d48: Wilco's second look (2026-11-15), at 1600x900 and 2560x1440.
// Standard, every part on, the frozen example through fake mode, the default
// sizes (sections 30/60/60/10, Overall scrollbar on). Per size it proves:
//   - no pane draws outside its box, and no text line, chart or row is cut by
//     the edge of the box (or of a clipping box inside it) that holds it; a
//     list that scrolls in its own box (Waits, Outcomes, Sessions) is read as
//     one block, its rows may sit in the scrolled-away part;
//   - the header is one line (every visible child shares one row);
//   - side by side (1600 px and up) the shared row is the SUM of the two
//     section numbers: Sessions and Processes are drawn at 120 percent of the
//     room, Status at 30;
//   - every gear button sits at the right end of its row, its hint text left
//     of it (Save, the three Deletes, Export, Burn Report);
//   - screenshots d48-<w>x<h>-dark and d48-<w>x<h>-light.
// A size the screen cannot hold prints SKIPPED and is not a pass.

// d48PaneSel lists every pane box (sections.Panes, page.html pctPaneEls), the
// boxes of the status row one by one.
const d48PaneSel = `['#secRow1 > .secpanel', '#secPanels > [data-sec="sessions"]', '.burnchart', '#burnLegend', '#sessionCards',
  '#vendorStripPanel', '#activityHeatmapRow > *', '#turnTickerPanel', '.cpubox', '.histwrap', '#processGroupsPanel',
  '.sysbottom > *', '#processesHost > *', '#todoPanel']`

const d48ReadJS = `(function(){
  var vis = function(e){ if(!e) return false; var c = getComputedStyle(e); return c.display !== 'none' && c.visibility !== 'hidden' && e.getClientRects().length > 0; };
  var clips = function(e){ var c = getComputedStyle(e); return c.overflowY !== 'visible'; };
  var scrolls = function(e){ var o = getComputedStyle(e).overflowY; return o === 'auto' || o === 'scroll'; };
  var name = function(e){ return e.tagName.toLowerCase() + (e.id ? '#' + e.id : '') + (typeof e.className === 'string' && e.className ? '.' + e.className.split(' ')[0] : ''); };
  var bad = [];
  ` + d48PaneSel + `.forEach(function(sel){
    Array.prototype.forEach.call(document.querySelectorAll(sel), function(box){
      if(!vis(box)) return;
      var br = box.getBoundingClientRect();
      Array.prototype.forEach.call(box.querySelectorAll('*'), function(el){
        if(!vis(el)) return;
        var tag = el.tagName.toLowerCase();
        var leaf = tag === 'canvas' || tag === 'svg' || el.classList.contains('heatcell') || (el.children.length === 0 && (el.textContent || '').trim() !== '');
        if(!leaf) return;
        // In a list that scrolls: checked against the list's own bottom while
        // it sits at its top; once scrolled, rows move freely.
        var a, inScroll = null, clip = null;
        for(a = el.parentElement; a && a !== box; a = a.parentElement){ if(scrolls(a)){ inScroll = a; break; } }
        if(inScroll && inScroll.scrollTop > 0) return;
        // The edge is the nearest bottom of every clipping box up to the pane.
        var edge = br.bottom;
        for(a = el.parentElement; a; a = a.parentElement){
          if(clips(a)){ clip = clip || a; edge = Math.min(edge, a.getBoundingClientRect().bottom); }
          if(a === box) break;
        }
        var r = el.getBoundingClientRect();
        if(r.height < 1) return;
        var what = sel + ' ' + name(el) + ' "' + (el.textContent || '').trim().slice(0, 24) + '"';
        if(!clip && r.bottom > br.bottom + 1) bad.push('outside: ' + what + ' ends ' + Math.round(r.bottom - br.bottom) + ' px below its box');
        else if(clip && r.top < edge - 1 && r.bottom > edge + 1) bad.push('cut: ' + what + ' at ' + Math.round(edge - r.top) + ' of ' + Math.round(r.height) + ' px');
      });
    });
  });
  var hd = document.querySelector('header'), kids = Array.prototype.filter.call(hd.children, vis);
  var maxTop = -Infinity, minBot = Infinity;
  kids.forEach(function(k){ var r = k.getBoundingClientRect(); maxTop = Math.max(maxTop, r.top); minBot = Math.min(minBot, r.bottom); });
  var f = window.__bdevFit || {}, s = f.sections || {};
  var px = function(k){ return s[k] ? s[k].px : 0; };
  var gear = document.getElementById('secSettings'), was = gear.classList.contains('open');
  gear.classList.add('open');
  var btns = [];
  ['secSizeSave', 'hist_delete_older', 'hist_delete_session', 'hist_delete_all', 'hist_export', 'hist_burn_report'].forEach(function(id){
    var b = document.getElementById(id), row = b.parentElement, rc = getComputedStyle(row);
    var rr = row.getBoundingClientRect(), bb = b.getBoundingClientRect();
    var right = rr.right - (parseFloat(rc.paddingRight) || 0) - (parseFloat(rc.borderRightWidth) || 0);
    // Only the children on the button's own line count (the History grid holds three lines).
    var leftOf = Array.prototype.every.call(row.children, function(c){
      if(c === b || !vis(c)) return true;
      var cr = c.getBoundingClientRect();
      return cr.bottom <= bb.top || cr.top >= bb.bottom || cr.left < bb.left;
    });
    btns.push({ id: id, gap: Math.round(right - bb.right), leftOf: leftOf });
  });
  if(!was) gear.classList.remove('open');
  return { bad: bad, headerOneLine: maxTop < minBot, headerH: Math.round(hd.getBoundingClientRect().height), headerKids: kids.length,
    two: !!f.two, room: f.room || 0, status: px('status'), sessions: px('sessions_30min'), processes: px('processes_30min'),
    btns: btns, win: window.innerWidth };
})()`

// d48RichJS paints the Sessions and Processes panes full (fake mode, invented
// names): six vendors, ten harnesses for the heatmap and process groups, 120
// days of activity, 30 minutes of system samples, five sessions with cards,
// twenty turns and 14 To Do tasks, so a pane has more rows than room (the
// second look saw the overflow on live data, which the frozen snapshot lacks).
const d48RichJS = `(function(){
  var nowMs = Date.now(); function iso(ms){ return new Date(ms).toISOString(); }
  var HARN = ['claude', 'claude-desktop', 'codex', 'copilot-cli', 'copilot-vscode', 'hermes', 'wsl', 'burnmon-dev', 'node', 'python'];
  var groups = [], sys = [], chart = [], turns = [], days = [], tasks = [];
  for(var t = nowMs - 29 * 60000; t <= nowMs; t += 5000){
    HARN.forEach(function(h, i){ groups.push({ Ts: iso(t), Harness: h, CPUPct: 2 + i, MemMB: 200 + 10 * i, IOBps: 1000 }); });
    sys.push({ Ts: iso(t), CPUPct: 30 + 10 * Math.sin(t / 60000), GPUPct: 10, MemUsedMB: 24000, MemTotalMB: 32000,
      DiskReadBps: 1000000, DiskWriteBps: 500000, NetDownBps: 4000, NetUpBps: 1000 });
  }
  var vendors = ['cowork', 'claude-code', 'codex', 'copilot-cli', 'copilot-vscode', 'hermes'].map(function(a, i){
    return { agent: a, agent_label: a, today: 1000 * (i + 1), week: 9000 * (i + 1), month: 40000 * (i + 1) }; });
  for(var d = 0; d < 120; d++){ var k = iso(nowMs - d * 86400000).slice(0, 10); days.push({ key: k, date: k, tokens: (d % 3) * 100000, cost: 1 }); }
  var sessions = [0, 1, 2, 3, 4].map(function(i){
    return { session_id: 'fake-s' + i, vendor: 'claude-code', agent: 'claude-code', surface: 'claude-code', model: '', project: 'proj-' + i,
      start: iso(nowMs - 600000), last_turn: iso(nowMs - 10000 * i), turn_count: 3 + i, context: 1000, context_window: 200000,
      tokens: 5000 + 1000 * i, cost: 0.1 * (i + 1), cache_hit_ratio: 0 }; });
  for(var i = 0; i < 30; i++){
    var b = {}; sessions.forEach(function(s, j){ b[s.session_id] = { fresh: 800 + 40 * i + 100 * j, cache_write: 0, cache_read: 0, output: 100 }; });
    chart.push({ at: iso(nowMs - 1800000 + i * 60000), by_session: b, cost: 0.01 });
  }
  for(i = 0; i < 20; i++) turns.push({ session_id: 'fake-s' + (i % 5), turn: 40 - i, at: iso(nowMs - i * 60000), agent: 'claude-code', fresh: 1000, output: 200 });
  for(i = 1; i <= 14; i++) tasks.push({ Title: 'Example task ' + i, List: 'Sample list', DueDate: '2026-09-29', Overdue: i % 5 === 0 });
  var snap = { now: nowMs, hist_gap_ms: 25000, sysmon_history: sys, sysmon_now: sys[sys.length - 1],
    process_groups_now: HARN.map(function(h, i){ return { Ts: iso(nowMs), Harness: h, CPUPct: 1 + i, MemMB: 200, IOBps: 0 }; }),
    process_groups_history: groups, vendor_strip: { rows: vendors }, activity_heatmap: { rows: days },
    todo: { enabled: true, signed_in: true }, todo_tasks: { items: tasks }, headline_today: 154166667,
    burn: { generated_at: iso(nowMs), running_window_seconds: 1800, sessions: sessions, window_start: iso(nowMs - 1800000),
      bucket_seconds: 60, chart: chart, turns: turns, chart_agents: {} } };
  window.__bdevEnterFakeMode();
  window.__bdevPaintFake(snap);
  window.dispatchEvent(new Event('resize'));
  return true;
})()`

type d48Btn struct {
	ID     string  `json:"id"`
	Gap    float64 `json:"gap"`
	LeftOf bool    `json:"leftOf"`
}

type d48Read struct {
	Bad           []string `json:"bad"`
	HeaderOneLine bool     `json:"headerOneLine"`
	HeaderH       float64  `json:"headerH"`
	HeaderKids    int      `json:"headerKids"`
	Two           bool     `json:"two"`
	Room          float64  `json:"room"`
	Status        float64  `json:"status"`
	Sessions      float64  `json:"sessions"`
	Processes     float64  `json:"processes"`
	Btns          []d48Btn `json:"btns"`
	Win           float64  `json:"win"`
}

// d48Check returns the problems in one read; pure, so it can be tested.
func d48Check(r d48Read, wide bool) []string {
	bad := append([]string(nil), r.Bad...)
	if !r.HeaderOneLine {
		bad = append(bad, fmt.Sprintf("the header is not one line (%d items, %.0f px tall)", r.HeaderKids, r.HeaderH))
	}
	if wide {
		if !r.Two {
			bad = append(bad, "the window is wide but Sessions and Processes are not side by side")
		} else if want := r.Room * 1.2; math.Abs(r.Sessions-want) > 2 || math.Abs(r.Processes-want) > 2 || math.Abs(r.Status-r.Room*0.3) > 2 {
			bad = append(bad, fmt.Sprintf("side by side: Status %.0f, Sessions %.0f, Processes %.0f px; want %.0f, %.0f, %.0f (30 and 60 + 60 of %.0f)",
				r.Status, r.Sessions, r.Processes, r.Room*0.3, want, want, r.Room))
		}
	}
	for _, b := range r.Btns {
		if math.Abs(b.Gap) > 2 || !b.LeftOf {
			bad = append(bad, fmt.Sprintf("button %s is %.0f px from its row's right end (everything else left of it: %v)", b.ID, b.Gap, b.LeftOf))
		}
	}
	return bad
}

func caseD48(hwnd uintptr, args []string) error {
	snap, err := d34Snapshot(false)
	if err != nil {
		return err
	}
	var fails []string
	ran := 0
	sizes := [][2]int32{{1600, 900}, {2560, 1440}}
	// UICHECK_D48_SIZE=1500x900 checks one other size instead (a screen that
	// holds neither); the side by side rule then applies from 1600 wide.
	if v := os.Getenv("UICHECK_D48_SIZE"); v != "" {
		var w, h int32
		if n, _ := fmt.Sscanf(v, "%dx%d", &w, &h); n == 2 {
			sizes = [][2]int32{{w, h}}
		}
	}
	for _, sz := range sizes {
		tag := fmt.Sprintf("%dx%d", sz[0], sz[1])
		if err := ensureWindowSizeWH(hwnd, sz[0], sz[1]); err != nil {
			return err
		}
		if windowCapped {
			fmt.Printf("uicheck: d48: SKIPPED %s, this screen is too small\n", tag)
			continue
		}
		ran++
		time.Sleep(500 * time.Millisecond)
		payload, err := d41Payload(d29Opts{Status: "ok", Snap: snap, Scrollbar: true}, nil)
		if err != nil {
			return err
		}
		if _, err := d34Apply(payload, snap); err != nil {
			return err
		}
		if _, err := evalRaw(d48RichJS); err != nil {
			return fmt.Errorf("d48 %s: paint: %w", tag, err)
		}
		// The layout pass runs on the resize; the whole-row pass follows each render within a second.
		time.Sleep(1800 * time.Millisecond)
		var r d48Read
		if err := evalInto(d48ReadJS, &r); err != nil {
			return fmt.Errorf("d48 %s: read: %w", tag, err)
		}
		for _, theme := range []string{"dark", "light"} {
			if _, err := evalRaw(`(document.documentElement.setAttribute('data-theme', '` + theme + `'), true)`); err != nil {
				return err
			}
			time.Sleep(300 * time.Millisecond)
			if _, err := screenshot(hwnd, "d48-"+tag+"-"+theme); err != nil {
				return err
			}
		}
		if _, err := evalRaw(`(document.documentElement.removeAttribute('data-theme'), true)`); err != nil {
			return err
		}
		for _, b := range d48Check(r, sz[0] >= 1600) {
			fails = append(fails, tag+": "+b)
		}
	}
	if len(fails) > 0 {
		return fmt.Errorf("d48: %d problem(s):\n%s", len(fails), strings.Join(fails, "\n"))
	}
	if ran == 0 {
		fmt.Println("uicheck: d48: no size fit this screen, nothing checked")
	}
	return nil
}
