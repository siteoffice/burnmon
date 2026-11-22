//go:build windows || (darwin && cgo)

// lamp_run.go is the `burnmon-dev lamp` subcommand: the arguments, the
// poses, and the engine that turns the snapshot (or the demo) into the
// payload the lamp page asks for once a second. The window itself is in
// lamp_window_<os>.go. The lamp never writes the snapshot, the store or a
// lock file; the demo writes no file at all.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"burnmon/internal/lamp"
	"burnmon/internal/snapshot"
)

// The single-instance names of the lamp. They differ from BurnMon Dev's
// (mutexName), so a lamp never blocks BurnMon Dev and the demo can run next
// to the real lamp.
const (
	lampMutexName     = `Local\burnmon-dev-lamp`
	lampDemoMutexName = `Local\burnmon-dev-lamp-demo`
)

const (
	lampBurnWindow = 30 * time.Minute
	lampLineSlot   = 5 * time.Minute
)

type lampOpts struct {
	demo       bool
	poses      string
	card       string // a folder: write the two week cards there
	cardWindow bool   // with card: also open the window that saves them as PNG
}

func parseLampArgs(args []string) (lampOpts, error) {
	fs := flag.NewFlagSet("lamp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	demo := fs.Bool("demo", false, "")
	poses := fs.String("poses", "", "")
	card := fs.String("card", "", "")
	cardWindow := fs.Bool("card-window", false, "")
	if err := fs.Parse(args); err != nil {
		return lampOpts{}, err
	}
	if fs.NArg() > 0 {
		return lampOpts{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *demo && *poses != "" {
		return lampOpts{}, errors.New("--demo and --poses cannot be used together")
	}
	if *card != "" && *poses != "" {
		return lampOpts{}, errors.New("--card and --poses cannot be used together")
	}
	if *cardWindow && *card == "" {
		return lampOpts{}, errors.New("--card-window needs --card <folder>")
	}
	return lampOpts{demo: *demo, poses: *poses, card: *card, cardWindow: *cardWindow}, nil
}

const lampUsage = "usage: burnmon-dev lamp [--demo | --poses <folder> | --card <folder> [--demo] [--card-window]]"

// runLamp is the lamp subcommand. It returns the exit code.
func runLamp(args []string) int {
	o, err := parseLampArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lamp:", err)
		fmt.Fprintln(os.Stderr, lampUsage)
		return 2
	}
	if o.poses != "" {
		if err := lamp.WritePoses(o.poses); err != nil {
			fmt.Fprintln(os.Stderr, "lamp:", err)
			return 1
		}
		fmt.Println("wrote 10 files to", o.poses)
		return 0
	}
	if o.card != "" {
		return runCardAt(o, time.Now())
	}
	return runLampWindow(o)
}

// lampCardBindingNames are the names lamp_card.html calls.
var lampCardBindingNames = []string{"lampCardSvgs", "lampCardSavePNG"}

// lampCards is what the card window's binding lampCardSvgs returns.
type lampCards struct {
	Story    string `json:"story"`
	Wide     string `json:"wide"`
	StoryPNG string `json:"story_png"`
	WidePNG  string `json:"wide_png"`
}

// runCardAt writes the two week cards of the ISO week that holds now into
// o.card. A live card reads the ledger and the language of the settings; with
// --demo it reads neither and writes nothing but the two cards. With
// --card-window the Save PNG window opens after the cards are written.
func runCardAt(o lampOpts, now time.Time) int {
	wk, lang := lamp.DemoWeek(now), "en"
	if !o.demo {
		setDir := settingsDir()
		wk = lamp.WeekOf(lamp.LoadLedger(filepath.Join(setDir, lamp.LedgerFile)).Days(), now)
		if b, err := os.ReadFile(devConfigReadPath(setDir)); err == nil {
			lang = lamp.ParseSettings(b).Language
		}
	}
	names, err := lamp.WriteCards(o.card, wk, lang)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lamp:", err)
		return 1
	}
	fmt.Println("wrote", names[0], "and", names[1], "to", o.card)
	if o.cardWindow {
		return runCardWindow(o.card, wk, lang)
	}
	return 0
}

// lampParentEnv names the variable BurnMon Dev sets for the lamp it starts: its
// own process id. The lamp checks it each tick and ends when that process is gone.
const lampParentEnv = "BURNMON_LAMP_PARENT"

// lampParentGone reports whether the BurnMon Dev named by the variable's value
// is gone. An empty or unreadable value (a lamp started by hand) is never gone.
func lampParentGone(env string, alive func(pid int) bool) bool {
	pid, err := strconv.Atoi(env)
	if err != nil || pid <= 0 {
		return false
	}
	return !alive(pid)
}

// lampChildEnv is base with exactly one BURNMON_LAMP_PARENT entry for pid. It
// does not change base.
func lampChildEnv(base []string, pid int) []string {
	out := make([]string, 0, len(base)+1)
	for _, e := range base {
		if !strings.HasPrefix(e, lampParentEnv+"=") {
			out = append(out, e)
		}
	}
	return append(out, lampParentEnv+"="+strconv.Itoa(pid))
}

// The lamp window's fixed sizes in CSS pixels: the ticker is 300 wide with 26
// per row; Glim's box is lampGlimBox times the size scale, below the ticker.
const (
	lampTickerW = 300
	lampRowH    = 26
	lampGlimBox = 120
	lampMaxRows = 4
)

// lampWindowSize is the window the lamp opens with, before the page reports
// its own size: the ticker at set.TickerRows (1 to 4, anything else is 1 or 4)
// and Glim's box at set.SizePct percent. It also covers the quiet sliver (120 by 6).
func lampWindowSize(set lamp.Settings) (w, h int) {
	rows := set.TickerRows
	if rows < 1 {
		rows = 1
	}
	if rows > lampMaxRows {
		rows = lampMaxRows
	}
	box := int(lampGlimBox*lamp.PctScale(set.SizePct) + 0.5)
	return lampTickerW, rows*lampRowH + box
}

// lampRect is a rectangle in device pixels, relative to the lamp window.
type lampRect struct{ X, Y, W, H int }

type lampBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// lampRegionRects turns the page's lampArea JSON (the ticker, Glim's box, the
// extra box of an act and the open menu, in CSS pixels) into the device
// rectangles the window keeps: scaled, each edge rounded outward so no border
// that falls on a fraction of a device pixel is cut, a missing or empty box
// left out. Bad JSON gives none, and the window then keeps its whole rectangle.
func lampRegionRects(area string, scale float64) []lampRect {
	var a struct {
		Ticker *lampBox `json:"ticker"`
		Glim   *lampBox `json:"glim"`
		Extra  *lampBox `json:"extra"`
		Menu   *lampBox `json:"menu"`
	}
	if err := json.Unmarshal([]byte(area), &a); err != nil {
		return nil
	}
	if scale <= 0 {
		scale = 1
	}
	// A product such as 26.4 * 1.25 can come out a hair above 33; the margin
	// keeps that from costing a whole extra pixel.
	const eps = 1e-6
	var out []lampRect
	for _, b := range []*lampBox{a.Ticker, a.Glim, a.Extra, a.Menu} {
		if b == nil || b.W <= 0 || b.H <= 0 {
			continue
		}
		x := int(math.Floor(b.X*scale + eps))
		y := int(math.Floor(b.Y*scale + eps))
		r := int(math.Ceil((b.X+b.W)*scale - eps))
		bt := int(math.Ceil((b.Y+b.H)*scale - eps))
		out = append(out, lampRect{x, y, r - x, bt - y})
	}
	return out
}

// lampPointIn reports whether the point (x, y), in window coordinates with the
// origin top left, is inside any rectangle. A rectangle is half open: its left
// and top edge are inside, its right and bottom edge are not. No rectangle at
// all means the whole window takes clicks. The Mac lamp uses it to let clicks
// through outside the page's rectangles.
func lampPointIn(rects []lampRect, x, y int) bool {
	if len(rects) == 0 {
		return true
	}
	for _, r := range rects {
		if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
			return true
		}
	}
	return false
}

// lampBackdropEnv is the process variable of the clear backdrop: the WebView2
// default background, fully transparent. The card backdrop sets none.
func lampBackdropEnv(backdrop string) (key, value string, ok bool) {
	if backdrop == "clear" {
		return lampBackdropVar, "00000000", true
	}
	return "", "", false
}

const lampBackdropVar = "WEBVIEW2_DEFAULT_BACKGROUND_COLOR"

// lampStartBackdrop is the background the Windows lamp window always starts
// with: fully transparent. The WebView2 background is fixed when the window is
// made, so the lamp starts clear whatever lamp_backdrop says and the page draws
// the card panel itself. A change of lamp_backdrop then needs no restart.
func lampStartBackdrop() (key, value string) {
	return lampBackdropVar, "00000000"
}

// lampDragPos is where the window goes while the button is down: the pointer
// minus the offset the pointer had inside the window when the drag began.
func lampDragPos(px, py, offX, offY int) (x, y int) {
	return px - offX, py - offY
}

// lampDrop is the drop of a dragged lamp at the pointer (px, py): it snaps to a
// monitor and a place, and saves lamp_monitor (the primary monitor as the empty
// name, so the lamp follows it) and lamp_position through save. Both keys are
// tried even when the first save fails; every failure comes back.
func lampDrop(mons []lampMonitor, px, py, w int, save func(key string, v any) error) (name, pos string, err error) {
	name, pos = lampSnap(mons, px, py, w)
	if m, ok := lampPrimary(mons); ok && m.Name == name {
		name = ""
	}
	e1 := save("lamp_monitor", name)
	e2 := save("lamp_position", pos)
	return name, pos, errors.Join(e1, e2)
}

// lampWithoutEnv is base without the entries for key. It does not change base.
func lampWithoutEnv(base []string, key string) []string {
	out := make([]string, 0, len(base))
	for _, e := range base {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return out
}

// lampIdleFrom is the idle seconds from two 32 bit millisecond tick counts
// (GetTickCount and the time of the last input). The subtraction wraps with
// the counter; a last input after now counts as 0.
func lampIdleFrom(now, last uint32) int {
	d := now - last
	if d > 1<<31 {
		return 0
	}
	return int(d / 1000)
}

// lampIdle is the seam for the idle seconds: the seconds since the last
// keyboard or mouse input on this computer. It is 0 until the window code
// sets the real one (Windows and macOS). It is read each poll and never stored.
var lampIdle = func() int { return 0 }

// lampLook is the look settings and the sweat floor after defaults, as the page reads them.
type lampLook struct {
	Size     int    `json:"size"`    // lamp_size, a percentage from 100 to 200
	Monitor  string `json:"monitor"` // the monitor the lamp hangs on, after a gone one fell back to the primary
	Theme    string `json:"theme"`
	Motion   string `json:"motion"`
	Quiet    string `json:"quiet"`
	Backdrop string `json:"backdrop"`
	Rows     int    `json:"rows"`
	// BurnFloor is lamp_burn_floor_tpm, the tokens a minute from which Glim sweats.
	BurnFloor int `json:"burn_floor"`
}

// lampPayload is what the binding lampState returns to the page. Act is the
// act that plays now ("" for none) and ActSeq goes up by one each time an act
// starts, so the page restarts the animation. Holds are the states that last,
// DotSeconds the gap between two token dots, Quiet whether only the sliver shows.
type lampPayload struct {
	State      lamp.State        `json:"state"`
	Theme      string            `json:"theme"`
	Labels     map[string]string `json:"labels"`
	Act        string            `json:"act"`
	ActSeq     int64             `json:"act_seq"`
	Holds      []string          `json:"holds"`
	DotSeconds float64           `json:"dot_seconds"`
	Quiet      bool              `json:"quiet"`
	Look       lampLook          `json:"look"`
	ActLines   []string          `json:"act_lines"`   // the lines of the act that plays: slip has three, the others one, most acts none
	Pose       string            `json:"pose"`        // the pose Glim is drawn in: the mood, except working for the coffee
	CarryLines map[string]string `json:"carry_lines"` // the two lines of a drag: drag while carried, view after a drop on another monitor
}

// lampSwingResult is what the binding lampSwing answers: the line to show and
// whether the grab was too hard (the page then flails the arms).
type lampSwingResult struct {
	Line string `json:"line"`
	Hard bool   `json:"hard"`
}

// lampActLines are the lines of an act, in the order the page shows them. The
// page works out no line itself. Stamp alternates its two lines by seq.
func lampActLines(act, lang string, count, hour int, seq int64) []string {
	parts := []string{""}
	switch act {
	case "slip":
		parts = []string{"start", "air", "back"}
	case "stamp":
		parts = []string{"a"}
		if seq%2 == 0 {
			parts = []string{"b"}
		}
	}
	var out []string
	for _, p := range parts {
		if l := lamp.ActLine(act, p, lang, count, hour); l != "" {
			out = append(out, l)
		}
	}
	return out
}

type lampBurnSample struct {
	at   time.Time
	rate float64
}

type lampOverride struct {
	slot int64
	line string
}

// lampEngine holds what the lamp process keeps between two polls: the last
// snapshot read, the burn rates of the last 30 minutes, and the line the
// person clicked to, so a click survives the next poll.
type lampEngine struct {
	demo     bool
	start    time.Time // the demo clock starts here
	setDir   string
	snapPath string

	mu       sync.Mutex
	snap     *snapshot.Snapshot
	lastRead time.Time
	lastSeen string // written_at of the last fresh snapshot that gave a sample
	tick     time.Duration
	samples  []lampBurnSample
	over     map[string]lampOverride

	ledger *lamp.Ledger // the wait ledger of a live lamp; nil for the demo

	lastSt   lamp.State
	lastSet  lamp.Settings
	lastNow  time.Time
	lastLine string

	lastProtest time.Time // the last too hard grab: a second one within 60 s gets the second line

	// The act engine: the state and idle seconds of the last tick it judged,
	// what already played, and the act that is playing until actUntil. While
	// an act plays no other starts and the previous state is kept, so a change
	// that happens meanwhile is judged when the act is over.
	prevSt   lamp.State
	prevIdle int
	played   lamp.Played
	act      string
	actUntil time.Time
}

func newLampEngine(demo bool, setDir, snapPath string) *lampEngine {
	return &lampEngine{
		demo: demo, start: time.Now(), setDir: setDir, snapPath: snapPath,
		tick: 2 * time.Second, over: map[string]lampOverride{},
	}
}

// lampRateKey is the label key lamp.html reads for the unit of the burn rate.
const lampRateKey = "tokens"

func lampLabels(lang string) map[string]string {
	text := "tokens a minute"
	if lang == "nl" {
		text = "tokens per minuut"
	}
	return map[string]string{lampRateKey: text}
}

// payload is the state for the page at time now. A live engine reads the
// settings and the theme again on every call, so a change in the gear
// dialog shows without a restart, and the snapshot once per tick.
func (e *lampEngine) payload(now time.Time) lampPayload {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.demo {
		sec := int(now.Sub(e.start) / time.Second)
		st := lamp.DemoState(sec)
		e.act, e.played.Seq = lamp.DemoAct(sec)
		return e.finish(st, lamp.DemoSettings(sec), lamp.DemoBase(), "dark")
	}
	set := lamp.DefaultSettings()
	if b, err := os.ReadFile(devConfigReadPath(e.setDir)); err == nil {
		set = lamp.ParseSettings(b)
	}
	if e.lastRead.IsZero() || now.Sub(e.lastRead) >= e.tick || now.Before(e.lastRead) {
		e.lastRead = now
		e.snap = lamp.Load(e.snapPath, now)
	}
	var rates []float64
	for _, s := range e.samples {
		if now.Sub(s.at) <= lampBurnWindow {
			rates = append(rates, s.rate)
		}
	}
	e.ledger.Observe(now, e.snap, set)
	st := lamp.Compute(lamp.Input{Snap: e.snap, Now: now, Settings: set, BurnSamples: rates})
	if e.snap != nil && !e.snap.IsStale(now) && e.snap.WrittenAt != e.lastSeen {
		e.lastSeen = e.snap.WrittenAt
		e.samples = append(e.samples, lampBurnSample{at: now, rate: e.snap.Burn.TokensPerMinute})
		kept := e.samples[:0]
		for _, s := range e.samples {
			if now.Sub(s.at) <= lampBurnWindow {
				kept = append(kept, s)
			}
		}
		e.samples = kept
		if e.snap.TickSeconds > 0 {
			e.tick = time.Duration(e.snap.TickSeconds) * time.Second
		}
	}
	e.judge(st, set, now)
	return e.finish(st, set, now, loadUITheme(e.setDir))
}

// finish applies a clicked line that is still in its five minute slot,
// remembers the state for nextLine and builds the payload.
func (e *lampEngine) finish(st lamp.State, set lamp.Settings, lineNow time.Time, theme string) lampPayload {
	if ov, ok := e.over[st.Mood]; ok && set.Lines && ov.slot == lineNow.Unix()/int64(lampLineSlot/time.Second) {
		st.Line = ov.line
	}
	e.lastSt, e.lastSet, e.lastNow, e.lastLine = st, set, lineNow, st.Line
	hour := lineNow.Hour()
	if e.demo && e.act == lamp.ActCoffee {
		hour = 10 // the made-up clock stands at 9, the coffee line needs a slot
	}
	return lampPayload{
		State: st, Theme: theme, Labels: lampLabels(set.Language),
		Act: e.act, ActSeq: e.played.Seq, Holds: lamp.Holds(st, set),
		DotSeconds: lamp.DotSeconds(st.BurnRate), Quiet: lamp.Quiet(st, set),
		Pose: lamp.ActMood(e.act, st.Mood),
		CarryLines: map[string]string{
			"drag": lamp.ActLine("carry", "drag", set.Language, 0, 0),
			"view": lamp.ActLine("carry", "view", set.Language, 0, 0),
		},
		Look:     lampLook{Size: set.SizePct, Monitor: lampMonitorUsed(set.Monitor), Theme: set.Theme, Motion: set.Motion, Quiet: set.Quiet, Backdrop: set.Backdrop, Rows: set.TickerRows, BurnFloor: set.BurnFloorTPM},
		ActLines: lampActLines(e.act, set.Language, st.Counted, hour, e.played.Seq),
	}
}

// judge runs the act engine for a live tick at now. The demo does not call it.
func (e *lampEngine) judge(st lamp.State, set lamp.Settings, now time.Time) {
	if e.act != "" && now.Before(e.actUntil) {
		return
	}
	e.act = ""
	idle := lampIdle()
	a, _, p := lamp.NextAct(lamp.ActInput{
		Prev: e.prevSt, Cur: st, Now: now, Idle: idle, PrevIdle: e.prevIdle, Settings: set, Played: e.played,
	})
	e.played = p
	if a != "" {
		e.act = a
		e.actUntil = now.Add(time.Duration(lamp.ActSeconds(a) * float64(time.Second)))
	}
	e.prevSt, e.prevIdle = st, idle
}

// nextLine picks the next line for the mood on show: the first later
// five-minute slot that gives a different line. The pick is kept for the
// rest of the current slot.
func (e *lampEngine) nextLine() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.lastSet.Lines || e.lastSt.Mood == "" {
		return ""
	}
	for k := 1; k <= 40; k++ {
		cand := lamp.Line(e.lastSt, e.lastSet, e.lastNow.Add(time.Duration(k)*lampLineSlot))
		if cand != "" && cand != e.lastLine {
			e.over[e.lastSt.Mood] = lampOverride{slot: e.lastNow.Unix() / int64(lampLineSlot/time.Second), line: cand}
			e.lastLine = cand
			return cand
		}
	}
	return e.lastLine
}

// lampMonitor is one monitor in device pixels: its full rectangle, its device
// name (the value of lamp_monitor) and its scale (1 is 96 dpi).
type lampMonitor struct {
	Name    string
	X, Y    int
	W, H    int
	Primary bool
	Scale   float64
}

// lampMonitors is the seam for the connected monitors. The window code sets
// the real reader; it is empty until then.
var lampMonitors = func() []lampMonitor { return nil }

// lampCursor is the seam for the pointer position in device pixels. The
// lamp reads it to point Glim's eyes and to follow a drag. It is never stored.
var lampCursor = func() (x, y int, ok bool) { return 0, 0, false }

// lampPrimary is the primary monitor, else the first one, else none.
func lampPrimary(mons []lampMonitor) (lampMonitor, bool) {
	for _, m := range mons {
		if m.Primary {
			return m, true
		}
	}
	if len(mons) > 0 {
		return mons[0], true
	}
	return lampMonitor{}, false
}

// lampPlaceOn is where a lamp of width w hangs: the top edge of the monitor
// called name, at left, centre or right (anything else is centre). A name that
// is not in the list, or an empty one, is the primary monitor. used is the name
// of the monitor taken; with no monitors at all the answer is 0, 0 and "".
func lampPlaceOn(mons []lampMonitor, name, position string, w int) (x, y int, used string) {
	m, ok := lampPrimary(mons)
	if name != "" {
		for _, c := range mons {
			if c.Name == name {
				m, ok = c, true
				break
			}
		}
	}
	if !ok {
		return 0, 0, ""
	}
	x = m.X + (m.W-w)/2
	switch position {
	case "top-left":
		x = m.X
	case "top-right":
		x = m.X + m.W - w
	}
	return x, m.Y, m.Name
}

// lampSnap is the monitor and the place a lamp dropped at the pointer (px, py)
// takes: the monitor that holds the pointer (the nearest one when none does),
// and the nearest of left, centre and right by the window's centre. The place
// comes back as the value lamp_position saves. With no monitors it is "" and centre.
func lampSnap(mons []lampMonitor, px, py, w int) (name, position string) {
	if len(mons) == 0 {
		return "", "top-center"
	}
	best, bestD := 0, -1
	for i, m := range mons {
		dx, dy := 0, 0
		if px < m.X {
			dx = m.X - px
		} else if px >= m.X+m.W {
			dx = px - (m.X + m.W - 1)
		}
		if py < m.Y {
			dy = m.Y - py
		} else if py >= m.Y+m.H {
			dy = py - (m.Y + m.H - 1)
		}
		if d := dx*dx + dy*dy; bestD < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	m := mons[best]
	position, dist := "top-left", abs(px-(m.X+w/2))
	for _, c := range []struct {
		pos string
		cx  int
	}{{"top-center", m.X + m.W/2}, {"top-right", m.X + m.W - w/2}} {
		if d := abs(px - c.cx); d < dist {
			position, dist = c.pos, d
		}
	}
	return m.Name, position
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// lampPointerCSS is the pointer (px, py) in the page's CSS pixels, relative to
// the window at (winX, winY) with the given scale. It can lie far outside the
// window. A scale of 0 or less acts as 1.
func lampPointerCSS(px, py, winX, winY int, scale float64) (x, y float64) {
	if scale <= 0 {
		scale = 1
	}
	return float64(px-winX) / scale, float64(py-winY) / scale
}

// lampAliveFrom is whether the parent process runs, from what the operating
// system said when the lamp asked. A process that could not be opened because
// access was denied is alive; one that could not be opened otherwise is gone;
// an opened one is alive only while its exit code is 259 (STILL_ACTIVE).
func lampAliveFrom(opened, denied bool, code uint32) bool {
	if !opened {
		return denied
	}
	return code == 259 || denied
}

// lampCloseNow closes the lamp from its menu: the ledger is written first,
// lamp_enabled false is saved only when BurnMon Dev started this lamp (parentSet),
// and the lamp ends last. A failed save does not keep the lamp open.
func lampCloseNow(writeLedger func(), parentSet bool, save func(key string, v any) error, end func()) {
	if writeLedger != nil {
		writeLedger()
	}
	if parentSet && save != nil {
		if err := save("lamp_enabled", false); err != nil {
			log.Println("lamp: could not save lamp_enabled false:", err)
		}
	}
	end()
}

// lampMonitorUsed is the device name of the monitor the lamp hangs on: the saved
// name when that monitor is connected, else the primary's, else the saved name.
func lampMonitorUsed(saved string) string {
	if _, _, used := lampPlaceOn(lampMonitors(), saved, "", 0); used != "" {
		return used
	}
	return saved
}

// swing is the answer to a grab and swing: the line for the strongest angle
// (degrees) and the direction changes the page counted. A too hard grab
// remembers its time, so a second one under 60 s later gets the second line. In
// motion calm and off Glim does not swing and there is no line.
func (e *lampEngine) swing(maxDeg float64, changes int, now time.Time) lampSwingResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lastSet.Motion == "calm" || e.lastSet.Motion == "off" {
		return lampSwingResult{}
	}
	part := lamp.Protest(maxDeg, changes, now, e.lastProtest, true)
	hard := lamp.ProtestTooHard(maxDeg, changes)
	if hard {
		e.lastProtest = now
	}
	return lampSwingResult{Line: lamp.ActLine("swing", part, e.lastSet.Language, 0, 0), Hard: hard}
}

// eyesOn is whether the lamp reads the pointer now: only while Glim is down
// (not quiet and hidden) and lamp_motion is lively, judged on the last payload.
func (e *lampEngine) eyesOn() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastSet.Motion == "lively" && !lamp.Quiet(e.lastSt, e.lastSet)
}
