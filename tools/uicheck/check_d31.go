package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// d31: the live check of the agent map on a real run, not a fixture. Start it
// while a Foreman sprint with subagents is running. It asks the real
// bdevAgentMap binding for the live map (the snapshot BurnMon core is writing
// now), prints every frame and the line kinds it holds, draws the whole map
// (d31-live-map-all, real project names: stays in testdata\uicheck\out), then
// draws only the frame named by the first argument, with the harness and You
// boxes that belong to it, for the README (d31-live-map-readme). It prints
// every string that shows in that picture so a person can check it for private
// data before it goes anywhere. It asserts nothing about content: what a real
// run shows is the finding. Usage: uicheck d31 <frame label>, default znd-skills.
const d31FilterJS = `(function(label){
  var p = window.__d31, L = p && p.layout;
  if(!L) return {err: 'no layout'};
  var f = (L.frames || []).filter(function(x){ return x.label === label; })[0];
  if(!f) return {err: 'no frame ' + label, frames: (L.frames || []).map(function(x){ return x.label; })};
  var keep = {};
  L.boxes.forEach(function(b){ if(b.group === f.id) keep[b.id] = true; });
  L.lines.forEach(function(l){ if(keep[l.to] && l.from.indexOf('harness:') === 0) keep[l.from] = true; });
  var you = L.lines.filter(function(l){ return l.you && keep[l.from]; });
  if(you.length) keep['you'] = true;
  var boxes = L.boxes.filter(function(b){ return keep[b.id]; }).map(function(b){ return JSON.parse(JSON.stringify(b)); });
  var rows = [];
  boxes.forEach(function(b){ if(rows.indexOf(b.row) < 0) rows.push(b.row); });
  rows.sort(function(a, b){ return a - b; });
  var idx = {}; rows.forEach(function(r, i){ idx[r] = i; });
  boxes.forEach(function(b){
    b.row = idx[b.row];
    if(b.kind === 'harness') b.line = '';
    if(b.kind === 'you') b.line = you.length + ' waiting on you';
  });
  var fr = JSON.parse(JSON.stringify(f));
  fr.first_row = idx[fr.first_row] !== undefined ? idx[fr.first_row] : 0;
  fr.last_row = Math.max.apply(null, boxes.filter(function(b){ return b.group === f.id; }).map(function(b){ return b.row; }));
  var lines = L.lines.filter(function(l){ return keep[l.to] && (keep[l.from] || l.from === f.id); });
  var counts = {sessions: boxes.filter(function(b){ return b.kind === 'session'; }).length, subagents: boxes.filter(function(b){ return b.kind === 'sub'; }).length};
  var layout = {columns: L.columns, boxes: boxes, lines: lines, frames: [fr], idle: [], counts: counts, notes: L.notes};
  window.__bdevMap.setPayload({source: 'live', hours: 0, status: p.status, age_seconds: p.age_seconds, note: '', layout: layout}, 'live');
  var st = document.getElementById('mapStage');
  return {boxes: Array.prototype.map.call(st.querySelectorAll('.mapbox'), function(e){ return e.textContent; }),
    frame: Array.prototype.map.call(st.querySelectorAll('.mapframe'), function(e){ return e.textContent; }),
    words: Array.prototype.map.call(st.querySelectorAll('.mapword'), function(e){ return e.textContent; }),
    heads: Array.prototype.map.call(st.querySelectorAll('.mapcolh'), function(e){ return e.textContent; }),
    src: document.getElementById('mapSrc').textContent, count: document.getElementById('mapCount').textContent,
    note: document.getElementById('mapNote').textContent};
})`

func init() {
	checks["d31"] = func(hwnd uintptr, args []string) error {
		label := "znd-skills"
		if len(args) > 0 {
			label = args[0]
		}
		if err := ensureWindowSizeWH(hwnd, 1280, 900); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		if err := d30Eval(`window.__bdevMap.open('live')`); err != nil {
			return err
		}
		if err := d30Eval(`(function(){ window.__d31 = null; window.bdevAgentMap({source:'live', hours:0, open:[]}).then(function(r){ window.__d31 = r; }, function(e){ window.__d31 = {err: String(e)}; }); })()`); err != nil {
			return err
		}
		var raw json.RawMessage
		for i := 0; ; i++ {
			r, err := evalRaw(`window.__d31`)
			if err != nil {
				return err
			}
			if string(r) != "null" {
				raw = r
				break
			}
			if i >= 30 {
				return fmt.Errorf("d31: bdevAgentMap gave no answer in 30 s")
			}
			time.Sleep(time.Second)
		}
		if err := os.WriteFile(filepath.Join("..", "..", "testdata", "uicheck", "out", "d31-live-layout.json"), raw, 0o644); err != nil {
			return err
		}
		var live struct {
			Err    string `json:"err"`
			Status string `json:"status"`
			Layout struct {
				Frames []struct {
					Label  string `json:"label"`
					Kind   string `json:"kind"`
					Status string `json:"status"`
					Count  int    `json:"count"`
				} `json:"frames"`
				Boxes []struct {
					Kind  string `json:"kind"`
					Title string `json:"title"`
				} `json:"boxes"`
				Lines []struct {
					Word  string `json:"word"`
					Basis string `json:"basis"`
				} `json:"lines"`
				Idle []struct {
					Label string `json:"label"`
				} `json:"idle"`
			} `json:"layout"`
		}
		if err := json.Unmarshal(raw, &live); err != nil {
			return err
		}
		if live.Err != "" {
			return fmt.Errorf("d31: real live map: %s", live.Err)
		}
		fmt.Printf("d31: live map, snapshot %s: %d boxes, %d lines, %d frames, %d idle sprints\n", live.Status, len(live.Layout.Boxes), len(live.Layout.Lines), len(live.Layout.Frames), len(live.Layout.Idle))
		words := map[string]int{}
		for _, l := range live.Layout.Lines {
			words[l.Word+"/"+l.Basis]++
		}
		fmt.Printf("d31: line words (word/basis: count): %v\n", words)
		kinds := map[string]int{}
		for _, b := range live.Layout.Boxes {
			kinds[b.Kind]++
		}
		fmt.Printf("d31: box kinds: %v\n", kinds)
		for _, f := range live.Layout.Frames {
			fmt.Printf("d31: frame %q kind %s status %q holds %d boxes\n", f.Label, f.Kind, f.Status, f.Count)
		}
		sprintFrames := 0
		for _, f := range live.Layout.Frames {
			if f.Kind == "sprint" {
				sprintFrames++
			}
		}
		fmt.Printf("d31: %d sprint frames in the live map (a sprint frame needs a works-for link)\n", sprintFrames)
		if _, err := screenshot(hwnd, "d31-live-map-all"); err != nil {
			return err
		}
		var out struct {
			Err    string   `json:"err"`
			Frames []string `json:"frames"`
			Boxes  []string `json:"boxes"`
			Frame  []string `json:"frame"`
			Words  []string `json:"words"`
			Heads  []string `json:"heads"`
			Src    string   `json:"src"`
			Count  string   `json:"count"`
			Note   string   `json:"note"`
		}
		if err := evalInto(d31FilterJS+`(`+fmt.Sprintf("%q", label)+`)`, &out); err != nil {
			return err
		}
		if out.Err != "" {
			fmt.Printf("d31: no README picture: %s, frames %v\n", out.Err, out.Frames)
			return nil
		}
		time.Sleep(500 * time.Millisecond)
		if _, err := screenshot(hwnd, "d31-live-map-readme"); err != nil {
			return err
		}
		fmt.Println("d31: strings in the README picture, check them for private data:")
		fmt.Println("  columns:", strings.Join(out.Heads, " | "))
		fmt.Println("  frame:", strings.Join(out.Frame, " | "))
		for _, b := range out.Boxes {
			fmt.Println("  box:", b)
		}
		fmt.Println("  line words:", strings.Join(out.Words, ", "))
		fmt.Println("  header:", out.Src, "/", out.Count)
		fmt.Println("  strip note:", out.Note)
		if err := d30Eval(`window.__bdevMap.close()`); err != nil {
			return err
		}
		fmt.Println("d31: ok")
		return nil
	}
}
