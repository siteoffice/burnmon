// Package snapshotwriter builds one tick's snapshot from the store, the
// process sampler and the relation graph's own sources, and writes it to
// snapshot.json. It opens no port and makes no model call.
package snapshotwriter

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"time"

	"burnmon/internal/gitrefs"
	"burnmon/internal/graph"
	"burnmon/internal/live"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
	"burnmon/internal/waits"
)

// Tick is how often the snapshot file is rewritten (R4, agent control
// plan). snapshot.StaleAfterTicks of these old and a reader reports the file
// as stale.
const Tick = 5 * time.Second

// foremanEvery is how often the Foreman state files are read again. One read
// takes 250 to 680 ms on the real tree (92 sprints, 56 flows), so it is not
// done every tick, but a new gate must show within two ticks (R9): 1.5 ticks
// leaves room for tick jitter, so the read is due on the second tick at the
// latest.
const foremanEvery = Tick + Tick/2

// AgentViewFirstPoll is the agent_view source's error until the writer's
// first poll has finished. Its items are missing until then, so a reader
// that compares lists (the toast rules) skips such a snapshot.
const AgentViewFirstPoll = "no poll has finished yet"

// GraphReaders holds the relation graph's own sources (R6) between ticks.
// Every reader is local disk or the Windows process and TCP tables: no port,
// no network, no model call.
type GraphReaders struct {
	PidDir      string
	ForemanRoot string
	Rows        func() ([]sysmon.ProcRow, time.Time)
	Listen      func() ([]graph.Listen, error)
	// AgentView runs "claude agents --json" (see ClaudeAgentView); nil means
	// no poller and no agent_view source. Now is the poller's clock (nil:
	// time.Now).
	AgentView func(ctx context.Context) ([]byte, error)
	Now       func() time.Time

	agentViewRefused error // why EnableAgentView started no poller

	poll  agentPoll
	mcp   *graph.MCPConfig
	meta  *graph.SubagentMeta
	repos *graph.RepoLocator
	// merged answers "is this branch in main" from the repo's own files.
	merged *gitrefs.Checker

	foremanAt  time.Time
	sprints    []graph.Sprint
	flows      []graph.Flow
	foremanErr error
}

// NewGraphReaders reads Claude Code's files under home (the user's profile
// folder) and Foreman's state under foremanRoot ("" for none).
func NewGraphReaders(home, foremanRoot string) *GraphReaders {
	return &GraphReaders{
		PidDir:      filepath.Join(home, ".claude", "sessions"),
		ForemanRoot: foremanRoot,
		mcp:         graph.NewMCPConfig(filepath.Join(home, ".claude.json")),
		meta:        graph.NewSubagentMeta(filepath.Join(home, ".claude", "projects")),
		repos:       graph.NewRepoLocator(),
		merged:      gitrefs.NewChecker(),
		Listen:      graph.Listening,
	}
}

func (gr *GraphReaders) foreman(now time.Time) ([]graph.Sprint, []graph.Flow, time.Time, error) {
	if now.Sub(gr.foremanAt) >= foremanEvery {
		gr.sprints, gr.flows, gr.foremanErr = graph.ReadForeman(gr.ForemanRoot)
		gr.foremanAt = now
	}
	return gr.sprints, gr.flows, gr.foremanAt, gr.foremanErr
}

func source(name, stamp string, err error) snapshot.Source {
	s := snapshot.Source{Name: name, OK: err == nil, ReadAt: stamp}
	if err != nil {
		s.Error = err.Error()
	}
	return s
}

// FlatSessions lists live sessions and their subagents for the graph.
func FlatSessions(ss []*live.Session) []graph.Session {
	var out []graph.Session
	var walk func(ls *live.Session, parent string)
	walk = func(ls *live.Session, parent string) {
		last, _ := time.Parse(time.RFC3339, ls.LastTurn)
		first, _ := time.Parse(time.RFC3339, ls.Start)
		out = append(out, graph.Session{Vendor: ls.Vendor, Agent: ls.Agent, ID: ls.SessionID, ParentID: parent,
			Project: ls.Project, State: ls.Stage, Tool: ls.StageTool, FirstActivity: first, LastActivity: last, AskOpen: ls.AskOpen})
		for _, sub := range ls.Subagents {
			walk(sub, ls.SessionID)
		}
	}
	for _, ls := range ss {
		walk(ls, "")
	}
	return out
}

// TokensSince returns the cost-of-waiting function for waits.Input: the tokens
// (input, output, cache write, cache read, reasoning) the given session nodes
// burned since a time, summed from events, the tick's live window. An item
// older than the window gets partial = true: the writer cannot see further
// back, and says so instead of guessing.
func TokensSince(events []schema.Event, now time.Time) func(ids []string, since time.Time) (int64, bool) {
	return func(ids []string, since time.Time) (int64, bool) {
		want := make(map[string]bool, len(ids))
		for _, id := range ids {
			want[id] = true
		}
		var total int64
		for _, e := range events {
			if e.At.Before(since) || !want["session:"+e.Vendor+":"+e.SessionID] {
				continue
			}
			total += e.Input + e.Output
			for _, p := range []*int64{e.CacheWrite, e.CacheRead, e.Reasoning} {
				if p != nil {
					total += *p
				}
			}
		}
		return total, since.Before(now.Add(-live.ChartWindow))
	}
}

// buildGraph reads the graph's own sources, builds the live graph, records
// its live-only edges in the store and returns the graph plus one source
// entry per input. A failing source drops its own edges and is reported;
// it never stops the tick.
func (gr *GraphReaders) buildGraph(st *store.Store, sessions []graph.Session, events []schema.Event, now time.Time, stamp string) (graph.Graph, []snapshot.Source, []waits.Item, []waits.Item, error) {
	in := graph.Input{Now: now, Sessions: sessions, MCPFor: gr.mcp.For, RepoFor: gr.repos.For, SubagentFor: gr.meta.Lookup,
		MergedFor: gr.merged.Merged, BranchGone: gr.merged.BranchGone, MergedByMessage: gr.merged.MergedByMessage}
	if gr.Rows != nil {
		rows, at := gr.Rows()
		in.ProcsAt = at
		for _, r := range rows {
			in.Procs = append(in.Procs, graph.Proc{PID: r.PID, PPID: r.PPID, Name: r.Name, Cmdline: r.Cmdline,
				CreateTime: r.CreateTime, Harness: string(r.Harness)})
		}
	}
	pids, pidErr := graph.ReadPidFiles(gr.PidDir)
	in.Pids = pids
	listens, netErr := gr.Listen()
	in.Listens = listens
	sources := []snapshot.Source{source(graph.SourceClaudePids, stamp, pidErr)}
	if gr.ForemanRoot != "" {
		sprints, flows, readAt, ferr := gr.foreman(now)
		if ferr == nil {
			in.Sprints, in.Flows = sprints, flows
		}
		sources = append(sources, source(graph.SourceForeman, readAt.UTC().Format(time.RFC3339), ferr))
	}

	var agentEntries []waits.AgentEntry
	if gr.AgentView != nil {
		gr.startAgentViewIfDue(now)
		entries, _, _, _ := gr.agentViewState()
		agentEntries = entries
		for _, e := range entries {
			in.Agents = append(in.Agents, graph.AgentState{SessionID: e.SessionID, State: e.State, Status: e.Status, WaitingFor: e.WaitingFor, Key: e.Key()})
		}
	}
	in.Tick = Tick
	g := graph.Build(in)
	sources = append(sources, source(graph.SourceMCPConfig, stamp, gr.mcp.Err()),
		source(graph.SourceNetTables, stamp, netErr), source(graph.SourceGit, stamp, nil))

	wi := waits.Input{Now: now, Sprints: in.Sprints, Flows: in.Flows, Graph: g, TokensSince: TokensSince(events, now)}
	if gr.AgentView != nil {
		_, at, done, aerr := gr.agentViewState()
		wi.Agents = agentEntries
		if !done {
			sources = append(sources, source(waits.SourceAgentView, stamp, errors.New(AgentViewFirstPoll)))
		} else {
			sources = append(sources, source(waits.SourceAgentView, at.UTC().Format(time.RFC3339), aerr))
		}
	}
	if gr.AgentView == nil && gr.agentViewRefused != nil {
		sources = append(sources, source(waits.SourceAgentView, stamp, gr.agentViewRefused))
	}
	waitItems, outcomeItems := waits.Build(wi)

	nodes, edges := g.Recorded()
	first, err := st.RecordGraph(nodes, edges, now)
	if err == nil {
		g.Stamp(first)
	}
	return g, sources, waitItems, outcomeItems, err
}

// Writer rewrites the snapshot file from its inputs.
type Writer struct {
	Store  *store.Store
	Path   string // snapshot.json
	Tick   time.Duration
	Config func() pricing.Config
	Procs  func() ([]sysmon.ProcessGroupSample, error)
	Graph  *GraphReaders    // nil: device node only
	Now    func() time.Time // nil: time.Now
	// Status is updated on every lock attempt and write (nil: not tracked).
	Status *Status
	// Started is when the process started, for the first-write log line
	// (zero: when Run began).
	Started time.Time

	kick chan struct{} // made by Run; Kick sends on it
}

// Run is the writer loop. Every w.Tick, a process that does not hold lock
// tries to take it; the one that holds it writes with Once and logs a write
// error. A dead owner's lock is free at the next tick, so another process
// takes over there. A Foreman change (watchForeman) or Kick runs the same
// step at once, with a fresh Foreman read. Run returns when stop closes (a
// nil stop never does), releasing the lock.
func (w *Writer) Run(lock *Lock, exe string, stop <-chan struct{}) {
	started := w.Started
	if started.IsZero() {
		started = time.Now()
	}
	wrote := false
	defer lock.Release()
	ticker := time.NewTicker(w.Tick)
	defer ticker.Stop()
	// Both exes run this loop and burnmon.exe may hold the lock, so the
	// Foreman watch lives here and not in BurnMon Dev alone.
	w.kick = make(chan struct{}, 1)
	if stopWatch := w.watchForeman(); stopWatch != nil {
		defer stopWatch()
	}
	// The first attempt and write run before the first ticker wait, so a
	// fresh window does not show the old file for a whole tick.
	for {
		if !lock.Held() {
			if ok, err := lock.TryAcquire(exe); err != nil {
				log.Println("snapshot: lock failed:", err)
				w.Status.setLockError(err)
			} else if ok {
				log.Println("snapshot: this process writes the snapshot")
			} else {
				w.Status.setOther(ReadOwner(lock.Path()))
			}
		}
		if lock.Held() {
			if err := w.Once(); err != nil {
				log.Println("snapshot: write failed:", err)
				w.Status.setFailed(err)
			} else {
				w.Status.setWrote(time.Now())
				if !wrote {
					wrote = true
					log.Printf("snapshot: first write %d ms after start", time.Since(started).Milliseconds())
				}
			}
		}
		select {
		case <-stop:
			return
		case <-ticker.C:
		case <-w.kick:
			w.Graph.forceForeman()
		}
	}
}

// Once builds one tick's snapshot from the store and the process sampler and
// writes it to w.Path. A failing input does not stop the write: the file
// then carries that source as not ok with its error, so a reader sees the
// fault instead of silence. Only a failed file write is returned. A nil
// w.Graph leaves the relation graph's own sources out (the graph then has
// only the device node).
func (w *Writer) Once() error {
	st, gr := w.Store, w.Graph
	cfg := w.Config()
	nowFn := w.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()
	stamp := now.UTC().Format(time.RFC3339)
	storeSrc := snapshot.Source{Name: "store", OK: true, ReadAt: stamp}
	sysSrc := snapshot.Source{Name: "sysmon", OK: true, ReadAt: stamp}

	var snap live.Snapshot
	events, err := st.EventsSince(now.Add(-live.ChartWindow))
	if err != nil {
		storeSrc.OK, storeSrc.Error = false, err.Error()
	} else {
		snap = live.BuildSnapshot(events, &cfg, now)
		if err := live.ApplySessionTotals(snap.Sessions, st, &cfg); err != nil {
			storeSrc.OK, storeSrc.Error = false, err.Error()
		}
		if err := live.ApplyStages(snap.Sessions, st, now); err != nil {
			storeSrc.OK, storeSrc.Error = false, err.Error()
		}
	}

	groups, err := w.Procs()
	if err != nil {
		sysSrc.OK, sysSrc.Error = false, err.Error()
	}

	sources := []snapshot.Source{storeSrc, sysSrc}
	g := graph.Build(graph.Input{Now: now})
	waitItems, outcomeItems := []waits.Item{}, []waits.Item{}
	if gr != nil {
		var more []snapshot.Source
		var rerr error
		g, more, waitItems, outcomeItems, rerr = gr.buildGraph(st, FlatSessions(snap.Sessions), events, now, stamp)
		if rerr != nil && sources[0].OK {
			sources[0].OK, sources[0].Error = false, "record graph: "+rerr.Error()
		}
		sources = append(sources, more...)
	}

	return snapshot.Write(w.Path, snapshot.Build(snapshot.Input{
		Now:         now,
		TickSeconds: int(w.Tick / time.Second),
		FXUSDEUR:    cfg.FXUSDEUR,
		Live:        snap,
		Sources:     sources,
		Processes:   groups,
		Graph:       g,

		WaitItems:    waitItems,
		OutcomeItems: outcomeItems,

		AgentSessions: gr.agentSessions(),
		AgentView:     gr.agentViewStatus(),
	}))
}
