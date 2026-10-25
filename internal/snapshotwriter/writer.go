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

	"burnmon/internal/graph"
	"burnmon/internal/live"
	"burnmon/internal/pricing"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
	"burnmon/internal/waits"
)

// Tick is how often the snapshot file is rewritten (R4, agent control
// plan). snapshot.StaleAfterTicks of these old and a reader says BurnMon is
// not running.
const Tick = 5 * time.Second

// foremanEvery is how often the Foreman state files are read again. One read
// takes 250 to 680 ms on the real tree (92 sprints, 56 flows), so it is not
// done every tick, but a new gate must show within two ticks (R9): 1.5 ticks
// leaves room for tick jitter, so the read is due on the second tick at the
// latest.
const foremanEvery = Tick + Tick/2

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
		Listen:      graph.Listening,
	}
}

func (gr *GraphReaders) foreman(now time.Time) ([]graph.Sprint, []graph.Flow, error) {
	if now.Sub(gr.foremanAt) >= foremanEvery {
		gr.sprints, gr.flows, gr.foremanErr = graph.ReadForeman(gr.ForemanRoot)
		gr.foremanAt = now
	}
	return gr.sprints, gr.flows, gr.foremanErr
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
			Project: ls.Project, State: ls.Stage, Tool: ls.StageTool, FirstActivity: first, LastActivity: last})
		for _, sub := range ls.Subagents {
			walk(sub, ls.SessionID)
		}
	}
	for _, ls := range ss {
		walk(ls, "")
	}
	return out
}

// buildGraph reads the graph's own sources, builds the live graph, records
// its live-only edges in the store and returns the graph plus one source
// entry per input. A failing source drops its own edges and is reported;
// it never stops the tick.
func (gr *GraphReaders) buildGraph(st *store.Store, sessions []graph.Session, now time.Time, stamp string) (graph.Graph, []snapshot.Source, []waits.Item, []waits.Item, error) {
	in := graph.Input{Now: now, Sessions: sessions, MCPFor: gr.mcp.For, RepoFor: gr.repos.For, SubagentFor: gr.meta.Lookup}
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
		sprints, flows, ferr := gr.foreman(now)
		if ferr == nil {
			in.Sprints, in.Flows = sprints, flows
		}
		sources = append(sources, source(graph.SourceForeman, stamp, ferr))
	}

	g := graph.Build(in)
	sources = append(sources, source(graph.SourceMCPConfig, stamp, gr.mcp.Err()),
		source(graph.SourceNetTables, stamp, netErr), source(graph.SourceGit, stamp, nil))

	wi := waits.Input{Now: now, Sprints: in.Sprints, Flows: in.Flows, Graph: g}
	if gr.AgentView != nil {
		gr.startAgentViewIfDue(now)
		entries, at, done, aerr := gr.agentViewState()
		wi.Agents = entries
		if !done {
			sources = append(sources, source(waits.SourceAgentView, stamp, errors.New("no poll has finished yet")))
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
}

// Run is the writer loop. Every w.Tick, a process that does not hold lock
// tries to take it; the one that holds it writes with Once and logs a write
// error. A dead owner's lock is free at the next tick, so another process
// takes over there. Run returns when stop closes (a nil stop never does),
// releasing the lock.
func (w *Writer) Run(lock *Lock, exe string, stop <-chan struct{}) {
	defer lock.Release()
	ticker := time.NewTicker(w.Tick)
	defer ticker.Stop()
	for {
		if !lock.Held() {
			if ok, err := lock.TryAcquire(exe); err != nil {
				log.Println("snapshot: lock failed:", err)
			} else if ok {
				log.Println("snapshot: this process writes the snapshot")
			}
		}
		if lock.Held() {
			if err := w.Once(); err != nil {
				log.Println("snapshot: write failed:", err)
			}
		}
		select {
		case <-stop:
			return
		case <-ticker.C:
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
		g, more, waitItems, outcomeItems, rerr = gr.buildGraph(st, FlatSessions(snap.Sessions), now, stamp)
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
	}))
}
