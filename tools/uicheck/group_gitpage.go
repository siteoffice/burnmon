package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// The gitpage group drives controlcentre.exe (its own window title, its own
// dev eval port 9335 behind CONTROLCENTRE_UICHECK, started by
// scripts\uicheck-centre.ps1 on a fixture root of five real git repos). It
// proves what the Git page shows in the real window: the loading bar, the
// strip against the rows, the filter, the lane dots and the detail pane, and
// that nothing scrolls sideways. Nothing here changes a repo.

// evalAddrCentre is controlcentre.exe's dev eval server
// (cmd\controlcentre\uicheck_devserver.go), a third port beside 9333 and 9334.
const evalAddrCentre = "127.0.0.1:9335"

// centreNames holds every name that targets controlcentre.exe.
var centreNames = map[string]bool{}

func isCentreCheck(name string) bool { return centreNames[name] }

type gitpageRow struct {
	Name string   `json:"name"`
	Stop string   `json:"stop"`
	Dots []string `json:"dots"`
}

type gitpageStrip struct {
	N       int    `json:"n"`
	Pressed bool   `json:"pressed"`
	Label   string `json:"label"`
}

type gitpageRead struct {
	Rows    []gitpageRow   `json:"rows"`
	Strip   []gitpageStrip `json:"strip"`
	Big     string         `json:"big"`
	Loading bool           `json:"loading"`
	Cut     []string       `json:"cut"` // strip text that does not fit its cell
}

// gitpageCutProblems: the strip's headline, its second line and its stage
// names are drawn in full. Hover is for long notes, not for these.
func gitpageCutProblems(r gitpageRead) []string {
	return append([]string(nil), r.Cut...)
}

type gitpageWidth struct {
	PaneScrollW float64  `json:"paneScrollW"`
	PaneClientW float64  `json:"paneClientW"`
	DocScrollW  float64  `json:"docScrollW"`
	InnerW      float64  `json:"innerW"`
	Cut         []string `json:"cut"`
	// CutNotes are the "Stops at" texts that are cut with an ellipsis. Hover
	// shows them in full, so they are printed, not failed.
	CutNotes []string `json:"cutNotes"`
}

type gitpageWant struct {
	Stop string // the start of the Stops at text
	Dots []string
}

// gitpageWantLanes is what the fixture's five repos must read. The launcher
// builds them (scripts\uicheck-centre.ps1) and internal\centre's own fixture
// tests pin the same rules in Go; pull requests are off, so Review is "off".
// A dot is done, active, warn, bad, todo or off.
var gitpageWantLanes = map[string]gitpageWant{
	"pub":    {"Published", []string{"done", "done", "done", "off", "done", "done"}},
	"dirty":  {"Changes: 1 file", []string{"active", "todo", "done", "off", "done", "done"}},
	"ahead":  {"Saved: 1 to share", []string{"done", "active", "todo", "off", "done", "done"}},
	"behind": {"Shared: 1 new online", []string{"done", "done", "warn", "off", "done", "done"}},
	"feat":   {"Saved: 1 to share", []string{"done", "active", "todo", "off", "todo", "todo"}},
}

// gitpageReadJS reads the Git page the way a person sees it: the rows (name,
// where it stops, the six lane dots), the strip counts and the headline.
const gitpageReadJS = `(function(){
  var cls = function(d){ var s = ['done', 'active', 'warn', 'bad', 'off']; for (var i = 0; i < s.length; i++) if (d.classList.contains(s[i])) return s[i]; return 'todo'; };
  var rows = [].map.call(document.querySelectorAll('#pane .rw[data-repo]'), function(r){
    return {name: (r.getAttribute('data-repo') || ''), stop: ((r.querySelector('.gst') || {}).textContent || ''),
      dots: [].map.call(r.querySelectorAll('.lane .dot'), cls)};
  });
  var strip = [].map.call(document.querySelectorAll('#pane .lstrip button.lst'), function(b){
    return {n: parseInt(b.querySelector('b').textContent, 10), pressed: b.getAttribute('aria-pressed') === 'true', label: b.querySelector('span').textContent};
  });
  var big = document.querySelector('#pane .lbig b');
  var cut = [];
  [].forEach.call(document.querySelectorAll('#pane .lstrip .lbig b, #pane .lstrip .lbig span, #pane .lstrip button.lst span'), function(e){
    if (e.scrollWidth > e.clientWidth + 1) cut.push(e.tagName.toLowerCase() + ' "' + e.textContent + '" is cut: ' + e.scrollWidth + ' px of text in ' + e.clientWidth + ' px');
  });
  return {rows: rows, strip: strip, big: big ? big.textContent : '', loading: !!document.querySelector('#pane .loadbar'), cut: cut};
})()`

const gitpageWidthJS = `(function(){
  var p = document.getElementById('pane'), cut = [];
  [].forEach.call(p.querySelectorAll('.rw'), function(r){
    var rr = r.getBoundingClientRect();
    [].forEach.call(r.children, function(c){
      var cr = c.getBoundingClientRect();
      if (cr.right > rr.right + 1) cut.push((c.className || c.tagName) + ' ends ' + Math.round(cr.right - rr.right) + ' px past its row');
    });
  });
  var s = p.querySelector('.lstrip');
  if (s) { var sr = s.getBoundingClientRect(); [].forEach.call(s.children, function(c){ var cr = c.getBoundingClientRect(); if (cr.right > sr.right + 1) cut.push('strip cell ends ' + Math.round(cr.right - sr.right) + ' px past the strip'); }); }
  var notes = [];
  [].forEach.call(p.querySelectorAll('.rw .gst'), function(e){ if (e.scrollWidth > e.clientWidth + 1) notes.push(e.textContent); });
  return {paneScrollW: p.scrollWidth, paneClientW: p.clientWidth, docScrollW: document.documentElement.scrollWidth, innerW: window.innerWidth, cut: cut, cutNotes: notes};
})()`

const gitpageDetailJS = `(function(){
  var cls = function(d){ var s = ['done', 'active', 'warn', 'bad', 'off']; for (var i = 0; i < s.length; i++) if (d.classList.contains(s[i])) return s[i]; return 'todo'; };
  var nx = document.querySelector('#det .next');
  return {title: (document.querySelector('#det h3') || {}).textContent || '', next: nx ? nx.textContent : '',
    stages: [].map.call(document.querySelectorAll('#det .vlane .vs'), function(v){ return (v.querySelector('.vn') || {}).textContent; }),
    dots: [].map.call(document.querySelectorAll('#det .vlane .vs .dot'), cls)};
})()`

func gitpageStopsAt(stop, stage string) bool {
	return stop == stage || strings.HasPrefix(stop, stage+":")
}

// gitpageStripProblems: with no filter, the strip says exactly what the rows
// say. total is the number of repos in the fixture.
func gitpageStripProblems(r gitpageRead, total int) []string {
	var bad []string
	if len(r.Strip) != len(laneStageNames) {
		return []string{fmt.Sprintf("the strip has %d stages, want %d", len(r.Strip), len(laneStageNames))}
	}
	if len(r.Rows) != total {
		bad = append(bad, fmt.Sprintf("the page shows %d repos, the fixture has %d", len(r.Rows), total))
	}
	open := 0
	for _, row := range r.Rows {
		if row.Stop != "Published" {
			open++
		}
	}
	want := fmt.Sprintf("%d of %d repos open", open, total)
	if open == 0 {
		want = fmt.Sprintf("All %d repos published", total)
	}
	if r.Big != want {
		bad = append(bad, fmt.Sprintf("the headline reads %q, the rows say %q", r.Big, want))
	}
	sum := 0
	for i, st := range laneStageNames {
		n := 0
		for _, row := range r.Rows {
			if gitpageStopsAt(row.Stop, st) {
				n++
			}
		}
		sum += r.Strip[i].N
		if r.Strip[i].N != n {
			bad = append(bad, fmt.Sprintf("the strip counts %d at %s, the rows show %d", r.Strip[i].N, st, n))
		}
	}
	if sum != open {
		bad = append(bad, fmt.Sprintf("the strip counts add up to %d, %d repos are open", sum, open))
	}
	return bad
}

// laneStageNames are the six stages, in the order of the strip and the lane.
var laneStageNames = []string{"Changes", "Saved", "Shared", "Review", "Merged", "Released"}

// gitpageLaneProblems: every row reads what the fixture table says, and no
// row is a stranger.
func gitpageLaneProblems(r gitpageRead) []string {
	var bad []string
	seen := map[string]bool{}
	for _, row := range r.Rows {
		want, ok := gitpageWantLanes[row.Name]
		if !ok {
			bad = append(bad, "the page shows a repo that is not in the fixture: "+row.Name)
			continue
		}
		seen[row.Name] = true
		if !strings.HasPrefix(row.Stop, want.Stop) {
			bad = append(bad, fmt.Sprintf("%s stops at %q, want %q", row.Name, row.Stop, want.Stop))
		}
		if strings.Join(row.Dots, ",") != strings.Join(want.Dots, ",") {
			bad = append(bad, fmt.Sprintf("%s lane is %v, want %v", row.Name, row.Dots, want.Dots))
		}
	}
	for name := range gitpageWantLanes {
		if !seen[name] {
			bad = append(bad, "the fixture repo is missing from the page: "+name)
		}
	}
	return bad
}

// gitpageFilterProblems: after a click on a stage, the rows are exactly the
// repos that stop there.
func gitpageFilterProblems(r gitpageRead, stage string, want []string) []string {
	var bad []string
	got := []string{}
	for _, row := range r.Rows {
		got = append(got, row.Name)
		if !gitpageStopsAt(row.Stop, stage) {
			bad = append(bad, fmt.Sprintf("%s shows under the %s filter but stops at %q", row.Name, stage, row.Stop))
		}
	}
	sort.Strings(got)
	w := append([]string(nil), want...)
	sort.Strings(w)
	if strings.Join(got, ",") != strings.Join(w, ",") {
		bad = append(bad, fmt.Sprintf("the %s filter shows %v, want %v", stage, got, w))
	}
	return bad
}

// gitpageWidthProblems: no pane, page or cell reaches past its box, so there
// is no horizontal scrollbar.
func gitpageWidthProblems(w gitpageWidth) []string {
	var bad []string
	if w.PaneScrollW > w.PaneClientW+1 {
		bad = append(bad, fmt.Sprintf("the pane is %.0f px wide inside %.0f px", w.PaneScrollW, w.PaneClientW))
	}
	if w.DocScrollW > w.InnerW+1 {
		bad = append(bad, fmt.Sprintf("the page is %.0f px wide in a %.0f px window", w.DocScrollW, w.InnerW))
	}
	bad = append(bad, w.Cut...)
	return bad
}

type gitpageCase struct {
	Name string
	Run  checkFunc
}

var gitpageCases = []gitpageCase{
	{"loading", caseGitpageLoading},
	{"strip", caseGitpageStrip},
	{"lane", caseGitpageLane},
	{"width", caseGitpageWidth},
}

func init() {
	checks["gitpage"] = runGitpageGroup
	centreNames["gitpage"] = true
	for _, c := range gitpageCases {
		c := c
		checks["gitpage/"+c.Name] = c.Run
		centreNames["gitpage/"+c.Name] = true
	}
}

func runGitpageGroup(hwnd uintptr, args []string) error {
	names := make([]string, len(gitpageCases))
	for i, c := range gitpageCases {
		names[i] = c.Name
	}
	fmt.Printf("uicheck: group gitpage: %s\n", strings.Join(names, " "))
	for _, c := range gitpageCases {
		fmt.Printf("--- case gitpage/%s ---\n", c.Name)
		if err := c.Run(hwnd, args); err != nil {
			return fmt.Errorf("gitpage/%s: %w", c.Name, err)
		}
		fmt.Printf("uicheck: gitpage/%s OK\n", c.Name)
	}
	return nil
}

// gitpageOpen sizes the window, then opens the Git page with a real click on
// its menu item and waits until the reading is done and the rows are drawn.
func gitpageOpen(hwnd uintptr, w, h int32) (gitpageRead, error) {
	// The port opens before the page has loaded: wait for the menu.
	ready := false
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		if ok, err := evalBool(`!!document.querySelector('#sidem button[title="Git"]')`); err == nil && ok {
			ready = true
			break
		}
	}
	if !ready {
		return gitpageRead{}, fmt.Errorf("the Control Centre page never showed its menu in 30 s")
	}
	if err := ensureWindowSizeWH(hwnd, w, h); err != nil {
		return gitpageRead{}, err
	}
	time.Sleep(400 * time.Millisecond)
	if err := clickSelector(hwnd, `#sidem button[title="Git"]`); err != nil {
		return gitpageRead{}, fmt.Errorf("open the Git page: %w", err)
	}
	return gitpageWait(len(gitpageWantLanes))
}

// gitpageWait polls until the page has stopped loading and shows want rows.
func gitpageWait(want int) (gitpageRead, error) {
	var r gitpageRead
	var lastErr error
	for deadline := time.Now().Add(40 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		r = gitpageRead{}
		if lastErr = evalInto(gitpageReadJS, &r); lastErr != nil {
			continue
		}
		if !r.Loading && len(r.Strip) == len(laneStageNames) && len(r.Rows) == want {
			return r, nil
		}
	}
	return r, fmt.Errorf("the Git page never settled with %d rows in 40 s (last %+v, err %v)", want, r, lastErr)
}

func failed(name string, bad []string) error {
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %d problem(s):\n%s", name, len(bad), strings.Join(bad, "\n"))
}

// gitpage/loading: the bar is in the pane the moment the page is asked for,
// again on Refresh, and gone once the rows are drawn.
func caseGitpageLoading(hwnd uintptr, args []string) error {
	if _, err := gitpageOpen(hwnd, 1280, 800); err != nil {
		return err
	}
	// The click and the read are one script: the reading can finish within a
	// moment, so a second eval may find the bar already gone.
	const readBar = `var b = document.querySelector('#pane .loadbar'), i = document.querySelector('#pane .loadbar i');
  return {has: !!b, role: b ? b.getAttribute('role') : '', name: i ? getComputedStyle(i).animationName : '',
    reduce: matchMedia('(prefers-reduced-motion: reduce)').matches};`
	for _, how := range []struct{ name, js string }{
		{"opening the page", `(function(){ document.querySelector('#sidem button[title="Git"]').click(); ` + readBar + ` })()`},
		{"Refresh", `(function(){ document.querySelector('.lead .lbtn').click(); ` + readBar + ` })()`},
	} {
		var anim struct {
			Has    bool   `json:"has"`
			Name   string `json:"name"`
			Reduce bool   `json:"reduce"`
			Role   string `json:"role"`
		}
		if err := evalInto(how.js, &anim); err != nil {
			return fmt.Errorf("loading: %s: %w", how.name, err)
		}
		if !anim.Has {
			return fmt.Errorf("loading: no loading bar in the pane right after %s", how.name)
		}
		if anim.Role != "progressbar" {
			return fmt.Errorf("loading: the bar has role %q, want progressbar", anim.Role)
		}
		if !anim.Reduce && anim.Name != "loadmove" {
			return fmt.Errorf("loading: the bar animation is %q, want loadmove", anim.Name)
		}
		if _, err := gitpageWait(len(gitpageWantLanes)); err != nil {
			return fmt.Errorf("loading: after %s: %w", how.name, err)
		}
		gone, err := evalBool(`!document.querySelector('#pane .loadbar')`)
		if err != nil {
			return err
		}
		if !gone {
			return fmt.Errorf("loading: the bar is still in the pane after the rows were drawn")
		}
	}
	return nil
}

// gitpage/strip: the strip against the rows, then the filter by a real click
// on a stage, and a second click that clears it.
func caseGitpageStrip(hwnd uintptr, args []string) error {
	r, err := gitpageOpen(hwnd, 1280, 800)
	if err != nil {
		return err
	}
	if _, err := screenshot(hwnd, "gitpage-strip"); err != nil {
		return err
	}
	if bad := append(gitpageStripProblems(r, len(gitpageWantLanes)), gitpageCutProblems(r)...); len(bad) > 0 {
		return failed("strip", bad)
	}
	stage := `#pane .lstrip button.lst[title^="Saved:"]`
	if err := clickSelector(hwnd, stage); err != nil {
		return fmt.Errorf("click the Saved stage: %w", err)
	}
	var f gitpageRead
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		f = gitpageRead{}
		if err := evalInto(gitpageReadJS, &f); err != nil {
			return err
		}
		if !f.Loading && len(f.Rows) == 2 {
			break
		}
	}
	if bad := gitpageFilterProblems(f, "Saved", []string{"ahead", "feat"}); len(bad) > 0 {
		return failed("strip filter", bad)
	}
	if len(f.Strip) == len(laneStageNames) && !f.Strip[1].Pressed {
		return fmt.Errorf("strip filter: the Saved button is not marked pressed")
	}
	if _, err := screenshot(hwnd, "gitpage-strip-filtered"); err != nil {
		return err
	}
	if err := clickSelector(hwnd, stage); err != nil {
		return fmt.Errorf("click the Saved stage again: %w", err)
	}
	back, err := gitpageWait(len(gitpageWantLanes))
	if err != nil {
		return fmt.Errorf("strip: after the second click: %w", err)
	}
	for i, s := range back.Strip {
		if s.Pressed {
			return fmt.Errorf("strip: %s is still pressed after the second click", laneStageNames[i])
		}
	}
	return failed("strip after clearing", gitpageStripProblems(back, len(gitpageWantLanes)))
}

// gitpage/lane: every row's dots and stop, then a real click on one row and
// its lane in the detail pane.
func caseGitpageLane(hwnd uintptr, args []string) error {
	r, err := gitpageOpen(hwnd, 1280, 800)
	if err != nil {
		return err
	}
	if bad := gitpageLaneProblems(r); len(bad) > 0 {
		return failed("lane", bad)
	}
	if err := clickSelector(hwnd, `#pane .rw[data-repo="dirty"]`); err != nil {
		return fmt.Errorf("click the dirty repo: %w", err)
	}
	time.Sleep(400 * time.Millisecond)
	var d struct {
		Title  string   `json:"title"`
		Next   string   `json:"next"`
		Stages []string `json:"stages"`
		Dots   []string `json:"dots"`
	}
	if err := evalInto(gitpageDetailJS, &d); err != nil {
		return err
	}
	var bad []string
	if d.Title != "dirty" {
		bad = append(bad, fmt.Sprintf("the detail pane is for %q, want dirty", d.Title))
	}
	if strings.Join(d.Stages, ",") != strings.Join(laneStageNames, ",") {
		bad = append(bad, fmt.Sprintf("the detail lane has the stages %v, want %v", d.Stages, laneStageNames))
	}
	if want := gitpageWantLanes["dirty"].Dots; strings.Join(d.Dots, ",") != strings.Join(want, ",") {
		bad = append(bad, fmt.Sprintf("the detail lane dots are %v, want %v", d.Dots, want))
	}
	if !strings.HasPrefix(d.Next, "Save the changes") {
		bad = append(bad, fmt.Sprintf("the next line reads %q, want it to start with \"Save the changes\"", d.Next))
	}
	if _, err := screenshot(hwnd, "gitpage-lane-detail"); err != nil {
		return err
	}
	return failed("lane detail", bad)
}

// gitpage/width: no horizontal scroll at 1280, 1600 and 2560 CSS px wide. A
// size the screen cannot hold prints SKIPPED and is not a pass.
func caseGitpageWidth(hwnd uintptr, args []string) error {
	var fails []string
	ran := 0
	for _, sz := range [][2]int32{{1280, 800}, {1600, 900}, {2560, 1440}} {
		tag := fmt.Sprintf("%dx%d", sz[0], sz[1])
		if _, err := gitpageOpen(hwnd, sz[0], sz[1]); err != nil {
			return err
		}
		if windowCapped {
			fmt.Printf("uicheck: gitpage/width: SKIPPED %s, this screen is too small\n", tag)
			continue
		}
		ran++
		time.Sleep(600 * time.Millisecond)
		var w gitpageWidth
		if err := evalInto(gitpageWidthJS, &w); err != nil {
			return fmt.Errorf("width %s: %w", tag, err)
		}
		if _, err := screenshot(hwnd, "gitpage-"+tag); err != nil {
			return err
		}
		fmt.Printf("uicheck: gitpage/width: %s: %d Stops at note(s) cut, hover shows them: %v\n", tag, len(w.CutNotes), w.CutNotes)
		for _, b := range gitpageWidthProblems(w) {
			fails = append(fails, tag+": "+b)
		}
	}
	if ran == 0 {
		fmt.Println("uicheck: gitpage/width: no size fit this screen, nothing checked")
	}
	return failed("width", fails)
}
