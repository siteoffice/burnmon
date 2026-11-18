//go:build windows || (darwin && cgo)

// lamp_run.go is the `burnmon-dev lamp` subcommand: the arguments, the
// poses, and the engine that turns the snapshot (or the demo) into the
// payload the lamp page asks for once a second. The window itself is in
// lamp_window_<os>.go. The lamp never writes the snapshot, the store or a
// lock file; the demo writes no file at all.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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
	demo  bool
	poses string
}

func parseLampArgs(args []string) (lampOpts, error) {
	fs := flag.NewFlagSet("lamp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	demo := fs.Bool("demo", false, "")
	poses := fs.String("poses", "", "")
	if err := fs.Parse(args); err != nil {
		return lampOpts{}, err
	}
	if fs.NArg() > 0 {
		return lampOpts{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *demo && *poses != "" {
		return lampOpts{}, errors.New("--demo and --poses cannot be used together")
	}
	return lampOpts{demo: *demo, poses: *poses}, nil
}

const lampUsage = "usage: burnmon-dev lamp [--demo | --poses <folder>]"

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
	return runLampWindow(o)
}

// lampPayload is what the binding lampState returns to the page.
type lampPayload struct {
	State  lamp.State        `json:"state"`
	Theme  string            `json:"theme"`
	Labels map[string]string `json:"labels"`
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

	lastSt   lamp.State
	lastSet  lamp.Settings
	lastNow  time.Time
	lastLine string
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
		return e.finish(st, lamp.DefaultSettings(), lamp.DemoBase(), "dark")
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
	return e.finish(st, set, now, loadUITheme(e.setDir))
}

// finish applies a clicked line that is still in its five minute slot,
// remembers the state for nextLine and builds the payload.
func (e *lampEngine) finish(st lamp.State, set lamp.Settings, lineNow time.Time, theme string) lampPayload {
	if ov, ok := e.over[st.Mood]; ok && set.Lines && ov.slot == lineNow.Unix()/int64(lampLineSlot/time.Second) {
		st.Line = ov.line
	}
	e.lastSt, e.lastSet, e.lastNow, e.lastLine = st, set, lineNow, st.Line
	return lampPayload{State: st, Theme: theme, Labels: lampLabels(set.Language)}
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
