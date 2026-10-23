// Package snapshot is R4 of the agent control plan: one read-only JSON file,
// rewritten atomically on every tick of BurnMon core, that other tools (the
// Fleet mod, lane B) read instead of opening BurnMon's store. It holds no
// logic of its own beyond reshaping what internal/live and internal/sysmon
// already produced. No ports, no model calls, no tokens. Field meanings:
// docs\snapshot_schema.md.
package snapshot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/live"
	"burnmon/internal/sysmon"
)

// SchemaVersion is bumped on any change a reader could trip over. Version 1
// is frozen at the end of R4; R6 added nodes and edges as new fields only.
const SchemaVersion = 1

// CostLabel is the plain label every cost carries: the figure is what the
// tokens would cost at API list price, not what anyone was billed.
const CostLabel = "API price equivalent"

// StaleAfterTicks is how many ticks old the file may be before a reader says
// BurnMon is not running.
const StaleAfterTicks = 3

// burnWindow is the span Burn looks back over.
const burnWindow = 5 * time.Minute

const (
	sourceStore  = "store"
	sourceSysmon = "sysmon"
)

// Source is one input BurnMon read this tick and whether that worked.
type Source struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	ReadAt string `json:"read_at"` // RFC3339 UTC
	Error  string `json:"error,omitempty"`
}

// Cost is a price figure with its label. USD is the figure BurnMon computes;
// EUR is USD times the configured rate.
type Cost struct {
	USD   float64 `json:"usd"`
	EUR   float64 `json:"eur"`
	Label string  `json:"label"`
}

// Session is one running session or subagent. Subagents are listed flat with
// ParentID set, so a reader never has to walk a tree.
type Session struct {
	Vendor       string `json:"vendor"`
	Agent        string `json:"agent"`
	ID           string `json:"id"`
	ParentID     string `json:"parent_id,omitempty"`
	Project      string `json:"project,omitempty"`
	Model        string `json:"model,omitempty"`
	State        string `json:"state"`
	StateSince   string `json:"state_since,omitempty"` // RFC3339 UTC
	Tokens       int64  `json:"tokens"`
	Cost         Cost   `json:"cost"`
	LastActivity string `json:"last_activity"` // RFC3339 UTC
	Source       string `json:"source"`
	AgeSeconds   int64  `json:"age_seconds"` // seconds since last_activity
}

// Burn is the recent token rate across every session, running or not.
type Burn struct {
	TokensLast5Min  int64   `json:"tokens_last_5min"`
	TokensPerMinute float64 `json:"tokens_per_minute"`
	Cost            Cost    `json:"cost"` // last 5 minutes
	Source          string  `json:"source"`
	AgeSeconds      int64   `json:"age_seconds"` // seconds since the store was read
}

// Process is one harness's summed process load from the last sample.
type Process struct {
	Harness    string  `json:"harness"`
	CPUPct     float64 `json:"cpu_pct"`
	MemMB      float64 `json:"mem_mb"`
	IOBps      float64 `json:"io_bps"`
	Source     string  `json:"source"`
	AgeSeconds int64   `json:"age_seconds"` // seconds since the sample
}

// Wait is a session whose turn ended and who is waiting for a person.
type Wait struct {
	Vendor         string `json:"vendor"`
	SessionID      string `json:"session_id"`
	Project        string `json:"project,omitempty"`
	Since          string `json:"since"` // RFC3339 UTC
	WaitingSeconds int64  `json:"waiting_seconds"`
	Source         string `json:"source"`
	AgeSeconds     int64  `json:"age_seconds"` // seconds since the session's last activity
}

// Snapshot is the whole file.
type Snapshot struct {
	SchemaVersion int       `json:"schema_version"`
	WrittenAt     string    `json:"written_at"` // RFC3339 UTC
	TickSeconds   int       `json:"tick_seconds"`
	Sources       []Source  `json:"sources"`
	Sessions      []Session `json:"sessions"`
	Burn          Burn      `json:"burn"`
	Processes     []Process `json:"processes"`
	WaitsOnYou    []Wait    `json:"waits_on_you"`

	// R6, added to version 1 as new fields only: the live relation graph.
	Nodes []graph.Node `json:"nodes"`
	Edges []graph.Edge `json:"edges"`
}

// Input is everything Build needs, already collected by the caller.
type Input struct {
	Now         time.Time
	TickSeconds int
	FXUSDEUR    float64
	// Live must have had ApplySessionTotals and ApplyStages applied by the
	// caller; Build only reshapes it.
	Live      live.Snapshot
	Sources   []Source
	Processes []sysmon.ProcessGroupSample
	Graph     graph.Graph
}

func cost(usd, fx float64) Cost {
	return Cost{USD: usd, EUR: usd * fx, Label: CostLabel}
}

func utc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func ageSeconds(now, t time.Time) int64 {
	if t.IsZero() || t.After(now) {
		return 0
	}
	return int64(now.Sub(t) / time.Second)
}

// Build reshapes one tick's inputs into a Snapshot. It never returns nil
// lists, so the file always encodes them as arrays.
func Build(in Input) Snapshot {
	s := Snapshot{
		SchemaVersion: SchemaVersion,
		WrittenAt:     utc(in.Now),
		TickSeconds:   in.TickSeconds,
		Sources:       append([]Source{}, in.Sources...),
		Sessions:      []Session{},
		Processes:     []Process{},
		WaitsOnYou:    []Wait{},
		Nodes:         append([]graph.Node{}, in.Graph.Nodes...),
		Edges:         append([]graph.Edge{}, in.Graph.Edges...),
	}

	var walk func(ls *live.Session, parent string)
	walk = func(ls *live.Session, parent string) {
		last, _ := time.Parse(time.RFC3339, ls.LastTurn)
		age := ageSeconds(in.Now, last)
		s.Sessions = append(s.Sessions, Session{
			Vendor: ls.Vendor, Agent: ls.Agent, ID: ls.SessionID, ParentID: parent,
			Project: ls.Project, Model: ls.Model, State: ls.Stage, StateSince: ls.StageSince,
			Tokens: ls.Tokens, Cost: cost(ls.Cost, in.FXUSDEUR),
			LastActivity: ls.LastTurn, Source: sourceStore, AgeSeconds: age,
		})
		if ls.Stage == "waiting" {
			since, _ := time.Parse(time.RFC3339, ls.StageSince)
			s.WaitsOnYou = append(s.WaitsOnYou, Wait{
				Vendor: ls.Vendor, SessionID: ls.SessionID, Project: ls.Project,
				Since: ls.StageSince, WaitingSeconds: ageSeconds(in.Now, since),
				Source: sourceStore, AgeSeconds: age,
			})
		}
		for _, sub := range ls.Subagents {
			walk(sub, ls.SessionID)
		}
	}
	for _, ls := range in.Live.Sessions {
		walk(ls, "")
	}

	var tokens int64
	var usd float64
	cutoff := in.Now.Add(-burnWindow)
	for _, b := range in.Live.Chart {
		at, err := time.Parse(time.RFC3339, b.At)
		if err != nil || !at.After(cutoff) {
			continue
		}
		for _, c := range b.BySession {
			tokens += c.Fresh + c.CacheWrite + c.CacheRead + c.Output
		}
		usd += b.Cost
	}
	gen, _ := time.Parse(time.RFC3339, in.Live.GeneratedAt)
	s.Burn = Burn{
		TokensLast5Min:  tokens,
		TokensPerMinute: float64(tokens) / burnWindow.Minutes(),
		Cost:            cost(usd, in.FXUSDEUR),
		Source:          sourceStore,
		AgeSeconds:      ageSeconds(in.Now, gen),
	}

	for _, p := range in.Processes {
		s.Processes = append(s.Processes, Process{
			Harness: string(p.Harness), CPUPct: p.CPUPct, MemMB: p.MemMB, IOBps: p.IOBps,
			Source: sourceSysmon, AgeSeconds: ageSeconds(in.Now, p.Ts),
		})
	}
	return s
}

// IsStale reports whether the file is older than StaleAfterTicks ticks at
// now, which is how a reader tells that BurnMon is not running.
func (s Snapshot) IsStale(now time.Time) bool {
	at, err := time.Parse(time.RFC3339, s.WrittenAt)
	if err != nil {
		return true
	}
	return now.Sub(at) > time.Duration(StaleAfterTicks*s.TickSeconds)*time.Second
}

// DefaultPath is %USERPROFILE%\.burnmon\snapshot.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".burnmon", "snapshot.json"), nil
}

// Write replaces path with s: a temp file in the same folder, then a rename,
// so a reader sees the old file or the new one, never half of one. On Windows
// a rename over a file a reader has open can fail for a moment, so it retries
// briefly.
func Write(path string, s Snapshot) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "snapshot-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(append(b, '\n'))
	if werr == nil {
		werr = tmp.Sync()
	}
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmpName)
		return err
	}
	var rerr error
	for i := 0; i < 5; i++ {
		if rerr = os.Rename(tmpName, path); rerr == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	os.Remove(tmpName)
	return rerr
}

// Read loads a snapshot file. A missing file returns an error for which
// os.IsNotExist is true.
func Read(path string) (Snapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}

// Reading status values, as the page sees them.
const (
	StatusOK          = "ok"
	StatusStale       = "stale"
	StatusMissing     = "missing"
	StatusUnreadable  = "unreadable"
	StatusUnsupported = "unsupported_version"
)

// Reading is what a reader gets back from Load: the file's state, its age,
// and the snapshot only when it is fresh and a version this code understands.
type Reading struct {
	Status     string    `json:"status"`
	AgeSeconds int64     `json:"age_seconds"` // seconds since written_at; 0 when unknown
	Error      string    `json:"error,omitempty"`
	Snapshot   *Snapshot `json:"snapshot,omitempty"`
}

// Load reads the snapshot file at path and judges it at now. A stale file
// means BurnMon is not running, so its numbers are withheld rather than shown
// as current. Load never returns an error: every fault is a Status.
func Load(path string, now time.Time) Reading {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Reading{Status: StatusMissing}
	}
	if err != nil {
		return Reading{Status: StatusUnreadable, Error: err.Error()}
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return Reading{Status: StatusUnreadable, Error: err.Error()}
	}
	if s.SchemaVersion != SchemaVersion {
		return Reading{Status: StatusUnsupported}
	}
	at, err := time.Parse(time.RFC3339, s.WrittenAt)
	if err != nil {
		return Reading{Status: StatusUnreadable, Error: "written_at: " + err.Error()}
	}
	age := ageSeconds(now, at)
	if s.IsStale(now) {
		return Reading{Status: StatusStale, AgeSeconds: age}
	}
	return Reading{Status: StatusOK, AgeSeconds: age, Snapshot: &s}
}
