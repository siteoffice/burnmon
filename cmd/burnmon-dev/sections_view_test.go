//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/sections"
	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

// Saving the theme must keep the sections, and saving the sections must keep
// the theme: both live in burnmon-dev-view.json.
func TestThemeAndSectionsShareTheViewFile(t *testing.T) {
	dir := t.TempDir()
	if err := saveUITheme(dir, "light"); err != nil {
		t.Fatal(err)
	}
	if err := saveSections(dir, sections.Default().WithPlain(true)); err != nil {
		t.Fatal(err)
	}
	if got := loadUITheme(dir); got != "light" {
		t.Errorf("theme after saving sections = %q, want light", got)
	}
	if err := saveUITheme(dir, "dark"); err != nil {
		t.Fatal(err)
	}
	if got := loadSections(dir).Mode(); got != "plain" {
		t.Errorf("mode after saving theme = %q, want plain", got)
	}
}

func TestSectionsPayloadFromFrozenExample(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join("..", "..", "testdata", "snapshot_example.json")
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)

	p := sectionsPayloadFor(dir, snapPath, now)
	if p.Mode != "standard" || p.Plain {
		t.Errorf("first run mode %q, want standard", p.Mode)
	}
	if len(p.Parts) != 15 || !p.Parts["git"] {
		t.Errorf("current list %v: standard shows every part", p.Parts)
	}
	if len(p.Groups) != 4 {
		t.Errorf("%d groups, want 4", len(p.Groups))
	}
	if !p.Scrollbar {
		t.Error("the overall scrollbar is on by default")
	}
	if p.Reading.Status != snapshot.StatusOK || p.Reading.AgeSeconds != 7 {
		t.Errorf("reading %+v", p.Reading)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"mode", "plain_mode", "overall_scrollbar", "parts", "parts_standard", "parts_plain", "groups", "reading"} {
		if _, ok := m[k]; !ok {
			t.Errorf("payload has no %q", k)
		}
	}
	for _, k := range []string{"sections", "order", "sections_standard", "sections_plain"} {
		if _, ok := m[k]; ok {
			t.Errorf("payload still has %q", k)
		}
	}
	// In Plain the page's current list is the Plain one.
	if err := saveSections(dir, sections.Default().WithPlain(true)); err != nil {
		t.Fatal(err)
	}
	if q := sectionsPayloadFor(dir, snapPath, now); q.Mode != "plain" || q.Parts["git"] {
		t.Errorf("plain payload: mode %q, git %v", q.Mode, q.Parts["git"])
	}
}

func TestSetSectionsFromPage(t *testing.T) {
	dir := t.TempDir()
	on, off := true, false

	// The toggle.
	if err := applySectionsChange(dir, sectionsChange{Plain: &on}); err != nil {
		t.Fatal(err)
	}
	if got := loadSections(dir); got.Mode() != "plain" {
		t.Errorf("after toggle on: %+v", got)
	}
	// One switch in one mode leaves the other mode alone.
	if err := applySectionsChange(dir, sectionsChange{Mode: "plain", Section: "turn_ticker", On: &on}); err != nil {
		t.Fatal(err)
	}
	if err := applySectionsChange(dir, sectionsChange{Mode: "standard", Section: "burn", On: &off}); err != nil {
		t.Fatal(err)
	}
	got := loadSections(dir)
	if !got.OnIn("plain", "turn_ticker") || got.OnIn("standard", "burn") || !got.OnIn("standard", "turn_ticker") || !got.OnIn("plain", "burn") {
		t.Errorf("per-mode switches wrong: %+v", got)
	}
	if got.Mode() != "plain" {
		t.Error("a switch changed the mode")
	}
	// The scrollbar switch leaves the mode and the lists alone.
	if err := applySectionsChange(dir, sectionsChange{Scrollbar: &off}); err != nil {
		t.Fatal(err)
	}
	sb := loadSections(dir)
	if sb.Scrollbar || sb.Mode() != "plain" || !sb.OnIn("plain", "turn_ticker") || sb.OnIn("standard", "burn") {
		t.Errorf("scrollbar off changed more than the flag: %+v", sb)
	}
	if err := applySectionsChange(dir, sectionsChange{Scrollbar: &on}); err != nil {
		t.Fatal(err)
	}
	if !loadSections(dir).Scrollbar {
		t.Error("scrollbar back on not saved")
	}
	// The toggle back.
	if err := applySectionsChange(dir, sectionsChange{Plain: &off}); err != nil {
		t.Fatal(err)
	}
	if loadSections(dir).Mode() != "standard" {
		t.Error("toggle off not saved")
	}
	// Garbage is refused and leaves the file alone.
	before := loadSections(dir)
	for _, bad := range []sectionsChange{
		{Mode: "wild", Section: "git", On: &on}, {Mode: "plain", Section: "nope", On: &on},
		{Mode: "plain", Section: "git"}, {Section: "git", On: &on}, {},
		{Scrollbar: &on, Plain: &on},
		{Scrollbar: &on, Mode: "plain", Section: "git", On: &on},
	} {
		if err := applySectionsChange(dir, bad); err == nil {
			t.Errorf("%+v: want an error", bad)
		}
	}
	after := loadSections(dir)
	if after.Mode() != before.Mode() || after.Scrollbar != before.Scrollbar ||
		after.OnIn("plain", "git") != before.OnIn("plain", "git") || after.OnIn("standard", "burn") != before.OnIn("standard", "burn") {
		t.Errorf("a refused change altered the file: %+v", after)
	}
}

// The live agent map rides along in the sections payload so the small
// overview redraws with the same 5 second poll, and it is absent when there is
// no current snapshot (a stale file is never drawn as current).
func TestSectionsPayloadCarriesAgentMap(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join("..", "..", "testdata", "snapshot_example.json")
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)

	p := sectionsPayloadFor(dir, snapPath, now)
	if p.AgentMap == nil || len(p.AgentMap.Groups) != 1 || p.AgentMap.Groups[0].Label != "sprint-example" {
		t.Fatalf("agent map from the frozen example: %+v", p.AgentMap)
	}
	if p.AgentMap.Counts.Subagents != 1 {
		t.Errorf("counts %+v", p.AgentMap.Counts)
	}

	stale := sectionsPayloadFor(dir, snapPath, now.Add(time.Hour))
	if stale.Reading.Status == snapshot.StatusOK || stale.AgentMap != nil {
		t.Errorf("stale reading %q still carries a map", stale.Reading.Status)
	}
	missing := sectionsPayloadFor(dir, filepath.Join(dir, "none.json"), now)
	if missing.AgentMap != nil {
		t.Error("missing file still carries a map")
	}
}

// The page must tag every part so the gear can switch it, and carry the gear
// controls for the scrollbar and the Station start theme.
func TestPageTagsEveryPart(t *testing.T) {
	for _, id := range []string{"burn_30min", "vendor_strip", "activity", "heatmap", "turn_ticker", "system", "process_groups", "todo"} {
		if !strings.Contains(pageHTML, `data-part="`+id+`"`) {
			t.Errorf("page lacks data-part=%q", id)
		}
	}
	for _, want := range []string{
		`var SEC_ORDER = ['waits','outcomes','burn','git','agents_map','sessions'];`,
		`id="secScrollbar"`,
		`name="secStationStart"`,
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %s", want)
		}
	}
}

func TestPageHasScrollbarFitPass(t *testing.T) {
	for _, want := range []string{
		`var FIT_OFF_ORDER = ['todo','process_groups','processes','heatmap','activity','sessions','git','outcomes','agents_map','burn','turn_ticker','vendor_strip','system','burn_30min','waits'];`,
		`window.__bdevFit = `,
		`bdevSetStationTheme:', e);`,
		`r.checked = r.value === secState.stationTheme`,
		`arow.classList.toggle('split-one'`,
		`body.scroll-on #pageScroll`,
		`classList.toggle('scroll-on'`,
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if strings.Contains(pageHTML, "sec-standard") {
		t.Errorf("page still uses the body.sec-standard class")
	}
}
// Review findings of iteration 3: the host fills the window only with the
// scrollbar off, the greedy pass puts parts back, and an empty zone leaves no shell.
func TestPageFitPassReviewFixes(t *testing.T) {
	for _, want := range []string{
		`classList.toggle('sec-fill', hideLegacy && !secState.scrollbar)`,
		`function secHostSync()`,
		`function fitShells(hidden)`,
		`function fitRow(hidden)`,
		`hidden.slice().reverse().forEach(function(id){`,
		`hidden.splice(at, 0, id);`,
		`drop(document.getElementById('legacy'), b && s && gone(['todo']));`,
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if strings.Contains(pageHTML, "toggle('sec-fill', hideLegacy)") {
		t.Errorf("sec-fill still ignores the scrollbar switch")
	}
}

func TestPageDrawsWaitAndOutcomeItems(t *testing.T) {
	for _, want := range []string{"wait_items", "outcome_items", "function secItemRow(", `class="secfile"`,
		"bdevOpenItemFile", "'Decision needed'", "'Approval needed'", "this list may be behind", "This list is "} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, bad := range []string{"coming (R9)", "Questions and approvals from your flows are coming."} {
		if strings.Contains(pageHTML, bad) {
			t.Errorf("page still contains %q", bad)
		}
	}
}

func TestPageEscapesItemText(t *testing.T) {
	i := strings.Index(pageHTML, "function secItemRow(")
	if i < 0 {
		t.Fatal("no secItemRow")
	}
	body := pageHTML[i:]
	if j := strings.Index(body, "\n  function "); j > 0 {
		body = body[:j]
	}
	for _, want := range []string{"esc(it.title)", "esc(it.detail"} {
		if !strings.Contains(body, want) {
			t.Errorf("secItemRow lacks %q", want)
		}
	}
	if !strings.Contains(body, `data-path="' + esc(`) {
		t.Error("secItemRow does not escape the data-path value")
	}
	if strings.Contains(body, "innerHTML") {
		t.Error("secItemRow assigns innerHTML")
	}
}

func TestPageHasNoItemControls(t *testing.T) {
	for _, bad := range []string{"bdevApprove", "bdevStop", "bdevReply"} {
		if strings.Contains(pageHTML, bad) {
			t.Errorf("page has %q (B4 is out of scope)", bad)
		}
	}
}

// Review findings of Task 7 and iteration 2: the person's actions panel is not
// the one the fit pass drops first, the tooltip carries what the row shows, a
// prompt row does not say its kind twice, and no list is cut off: every item
// is a row, inside a bounded scroll area, with the total in the header.
func TestPageListsEveryItemInBoundedScrollArea(t *testing.T) {
	for _, want := range []string{
		`var shown = plain ? word : txt;`,
		`var full = shown + ' ' + (it.title || '')`,
		`String(it.detail).toLowerCase() === k[0].toLowerCase()`,
		`part += secItemRow(it, false);`,
		`o.forEach(function(it){ rows += secItemRow(it, true); });`,
		`.seclist{`,
		`overflow-y:auto`,
		`secCount(`,
	} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	for _, bad := range []string{`SEC_ROWS_CAP`, `secMoreLine`, `.slice(0, SEC_ROWS_CAP)`} {
		if strings.Contains(pageHTML, bad) {
			t.Errorf("page still has the row cap: %s", bad)
		}
	}
}

// A finished item whose detail starts with a stopped status must not read as
// plain "Done" in the plain wording.
func TestPageNamesStoppedOutcomeStatus(t *testing.T) {
	for _, want := range []string{`/^(capped|failed|escalated),/`, `'Stopped, '`} {
		if !strings.Contains(pageHTML, want) {
			t.Fatalf("page lacks %q", want)
		}
	}
}

// ---- B4 Task 8: control chips, preview line, result, gear block ----------

// pageFuncBody returns the page script text of one function, up to the next
// two-space-indented function.
func pageFuncBody(t *testing.T, name string) string {
	t.Helper()
	i := strings.Index(pageHTML, "function "+name+"(")
	if i < 0 {
		t.Fatalf("page has no function %s", name)
	}
	body := pageHTML[i+1:]
	if j := strings.Index(body, "\n  function "); j > 0 {
		body = body[:j]
	}
	return body
}

func TestPageHasControls(t *testing.T) {
	for _, want := range []string{"bdevControlPreview", "bdevControlRun", "function ctlChips(", "function ctlLines(",
		"agent_sessions", `class="ctlcmd"`, "again within 5 s", "again within 5 seconds", "'Send back'",
		"Stop a session", "data-ctl-sw", "Attach to a session", "Approve a gate", "Send a gate back",
		"claude_path", "agent_view", `class="ctl ctlgo"`, ">Confirm<"} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, bad := range []string{"bdevReply", "'reply'", `"reply"`, "data-ctl-sw=\"reply\""} {
		if strings.Contains(pageHTML, bad) {
			t.Errorf("page has %q: reply is a parked ASK", bad)
		}
	}
}

func TestPageNeverSendsACommand(t *testing.T) {
	// Every call of the run binding passes the stored token, nothing else.
	if n := strings.Count(pageHTML, "bdevControlRun("); n != 1 {
		t.Errorf("%d calls of bdevControlRun(, want exactly 1", n)
	}
	if !strings.Contains(pageHTML, "window.bdevControlRun(p.token)") {
		t.Error("the run call is not window.bdevControlRun(p.token)")
	}
	// Every call of the preview binding passes exactly control and target.
	if n := strings.Count(pageHTML, "bdevControlPreview("); n != 1 {
		t.Errorf("%d calls of bdevControlPreview(, want exactly 1", n)
	}
	if !strings.Contains(pageHTML, "window.bdevControlPreview({control: c, target: t})") {
		t.Error("the preview call is not {control: c, target: t}")
	}
	body := pageFuncBody(t, "ctlChips") + pageFuncBody(t, "ctlLines")
	for _, bad := range []string{"argv", "data-cmd", "data-pid", "data-path", "data-command"} {
		if strings.Contains(body, bad) {
			t.Errorf("control markup carries %q", bad)
		}
	}
}

func TestPageEscapesControlText(t *testing.T) {
	chips := pageFuncBody(t, "ctlChips")
	lines := pageFuncBody(t, "ctlLines")
	fill := pageFuncBody(t, "ctlFill")
	for _, want := range []string{`data-target="' + esc(`, `data-ctl="' + esc(`} {
		if !strings.Contains(chips, want) {
			t.Errorf("ctlChips lacks %s", want)
		}
		if strings.Contains(chips, "innerHTML") {
			t.Error("ctlChips assigns innerHTML")
		}
	}
	for _, want := range []string{`data-ctlid="' + esc(`, `data-target="' + esc(`} {
		if !strings.Contains(lines, want) {
			t.Errorf("ctlLines lacks %s", want)
		}
	}
	// The command, label and result text never enter markup: ctlFill sets them as text.
	for _, bad := range []string{"p.command", "p.label", ".text"} {
		if strings.Contains(lines, bad) {
			t.Errorf("ctlLines interpolates %s into markup", bad)
		}
	}
	if !strings.Contains(fill, ".textContent = ") || strings.Contains(fill, "innerHTML") {
		t.Error("ctlFill must set text with textContent and never innerHTML")
	}
}

func TestPageControlsOffUnlessOnAndReady(t *testing.T) {
	chips := pageFuncBody(t, "ctlChips")
	for _, want := range []string{"ctlState.on[c] && ctlState.ready[c]", "ctlState.verdicts[c]", "hasOwnProperty", "'background'"} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if !strings.Contains(chips, "ctlState.on[c] && ctlState.ready[c]") {
		t.Error("ctlChips does not require on AND ready")
	}
	// Chips only for waits rows, never for outcome rows.
	if !strings.Contains(pageHTML, "if(!outcome") {
		t.Error("secItemRow does not keep outcome rows free of controls")
	}
}

func TestPageConfirmIsSeparateDebouncedAndExpires(t *testing.T) {
	for _, want := range []string{"var CTL_GAP = 400;", "var CTL_TTL = 5000;", "data-ctlgo=", " disabled", "Date.now() - p.at < CTL_GAP",
		"delete ctlPending[key]", "setTimeout(secRender, CTL_TTL + 50)", "ctlPrune()", "preview expired", "Took too long"} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	// A chip press only previews: the click handler never runs from a chip.
	h := pageFuncBody(t, "ctlClick")
	if !strings.Contains(h, "data-ctlgo") {
		t.Error("ctlClick has no confirm branch")
	}
	if strings.Contains(h, "bdevControlRun(") {
		t.Error("ctlClick calls the run binding directly instead of through ctlRun")
	}
}

func TestPageControlWordsAndGear(t *testing.T) {
	for _, want := range []string{"stop:['stop','Stop']", "attach:['attach','Open']", "approve:['approve','Approve']",
		"send_back:['send back','Send back']", `id="secControls"`, "secControlsSync(", "{ control: t.getAttribute('data-ctl-sw'), on: t.checked }"} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, bad := range []string{"data-ctl-sw=\"reply\"", "ctlState.verdicts.send_back.G3"} {
		if strings.Contains(pageHTML, bad) {
			t.Errorf("page has %q", bad)
		}
	}
}

// Review Focus 2: the amber number is one number in every place the window
// shows it: the reading's wait_counts, the list's blocked items, the small
// overview in the sections payload, the full live map's counts and its You box.
func TestFourPlacesOneCount(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join("..", "..", "testdata", "snapshot_example.json")
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)

	p := sectionsPayloadFor(dir, snapPath, now)
	if p.Reading.Snapshot == nil || p.AgentMap == nil {
		t.Fatal("no snapshot or no overview map")
	}
	listed := 0
	for _, it := range p.Reading.Snapshot.WaitItems {
		if it.Group == waits.GroupBlocked {
			listed++
		}
	}
	wc := p.Reading.Snapshot.WaitCounts
	if listed == 0 || wc != waits.CountOf(p.Reading.Snapshot.WaitItems) || wc.Blocked != listed {
		t.Fatalf("wait_counts %+v, blocked items listed %d", wc, listed)
	}
	if p.AgentMap.Counts.Waiting != listed || p.AgentMap.Counts.YourTurn != wc.YourTurn {
		t.Errorf("overview counts %+v, want waiting %d your turn %d", p.AgentMap.Counts, listed, wc.YourTurn)
	}
	m, err := agentMapFor(agentMapEnv{DataDir: dir, SnapPath: snapPath}, agentMapRequest{Source: "live"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if m.Layout.Counts.Waiting != listed || m.Layout.Counts.YourTurn != wc.YourTurn {
		t.Errorf("full map counts %+v, want waiting %d", m.Layout.Counts, listed)
	}
	y, ok := boxIn(m.Layout, "you")
	if !ok || !strings.HasPrefix(y.Line, fmt.Sprintf("%d waiting on you", listed)) {
		t.Errorf("You box %+v (found %v), want it to start with %d", y, ok, listed)
	}
}

// ---- B6 Task 9: Waits on you is the ranked your-move queue ---------------

func TestPageWaitsTiers(t *testing.T) {
	for _, want := range []string{"'Blocked'", "'Your turn'", "'Done, needs close-out'", "'Done, can be closed'", "wait_counts", "your turn"} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if body := pageFuncBody(t, "secPanelCount"); strings.Contains(body, "wait_items") {
		t.Error("the Waits on you header count still reads wait_items, want wait_counts")
	}
	if body := pageFuncBody(t, "secBodyWaits"); !strings.Contains(body, "secItemRow(it, false)") {
		t.Error("secBodyWaits does not draw its rows with secItemRow")
	}
}

func TestPageCloseChipOnlyOnCloseOut(t *testing.T) {
	for _, want := range []string{`close_sprint:['Close sprint','Close this job']`, `kind === 'closeout'`, `c === 'close_sprint'`} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	body := pageFuncBody(t, "ctlChips")
	if !strings.Contains(body, "closeout") || !strings.Contains(body, "ctlState.foreman") {
		t.Error("ctlChips has no closeout branch gated on ctlState.foreman")
	}
	if !strings.Contains(pageHTML, "'Close sprint'") {
		t.Error("page lacks the Close sprint word")
	}
}

func TestPageHidesForemanPartsWithoutForemanRoot(t *testing.T) {
	for _, want := range []string{`[data-ctl-needs]`, `ctlState.foreman`, `foreman:!!cp.foreman`} {
		if !strings.Contains(pageHTML, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if body := pageFuncBody(t, "secControlsSync"); !strings.Contains(body, "data-ctl-needs") {
		t.Error("secControlsSync does not hide the gear lines that need Foreman")
	}
}

func TestPageCostLineEscaped(t *testing.T) {
	body := pageFuncBody(t, "secItemRow")
	for _, want := range []string{"cost_tokens", "cost_partial", "at least ", "tokens burned while waiting", "idle "} {
		if !strings.Contains(body+pageFuncBody(t, "secCostLine"), want) {
			t.Errorf("the cost line lacks %q", want)
		}
	}
	if !strings.Contains(body, "esc(cost)") {
		t.Error("secItemRow does not escape the cost line")
	}
	if strings.Contains(body, "innerHTML") {
		t.Error("secItemRow assigns innerHTML")
	}
}