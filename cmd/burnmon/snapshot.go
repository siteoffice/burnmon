package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"burnmon/internal/graph"
	"burnmon/internal/live"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

// snapshotTick is how often the snapshot file is rewritten (R4, agent control
// plan). snapshot.StaleAfterTicks of these old and a reader says BurnMon is
// not running.
const snapshotTick = 5 * time.Second

// foremanEvery is how often the Foreman state files are read again: they
// change at gate and phase speed, not every tick.
const foremanEvery = 30 * time.Second

// graphReaders holds the relation graph's own sources (R6) between ticks.
// Every reader is local disk or the Windows process and TCP tables: no port,
// no network, no model call.
type graphReaders struct {
	pidDir      string
	foremanRoot string
	mcp         *graph.MCPConfig
	meta        *graph.SubagentMeta
	repos       *graph.RepoLocator
	rows        func() ([]sysmon.ProcRow, time.Time)
	listen      func() ([]graph.Listen, error)

	foremanAt  time.Time
	sprints    []graph.Sprint
	flows      []graph.Flow
	foremanErr error
}

// newGraphReaders reads Claude Code's files under home (the user's profile
// folder) and Foreman's state under foremanRoot ("" for none).
func newGraphReaders(home, foremanRoot string) *graphReaders {
	return &graphReaders{
		pidDir:      filepath.Join(home, ".claude", "sessions"),
		foremanRoot: foremanRoot,
		mcp:         graph.NewMCPConfig(filepath.Join(home, ".claude.json")),
		meta:        graph.NewSubagentMeta(filepath.Join(home, ".claude", "projects")),
		repos:       graph.NewRepoLocator(),
		listen:      graph.Listening,
	}
}

func (gr *graphReaders) foreman(now time.Time) ([]graph.Sprint, []graph.Flow, error) {
	if now.Sub(gr.foremanAt) >= foremanEvery {
		gr.sprints, gr.flows, gr.foremanErr = graph.ReadForeman(gr.foremanRoot)
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

// flatSessions lists live sessions and their subagents for the graph.
func flatSessions(ss []*live.Session) []graph.Session {
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
func (gr *graphReaders) buildGraph(st *store.Store, sessions []graph.Session, now time.Time, stamp string) (graph.Graph, []snapshot.Source, error) {
	in := graph.Input{Now: now, Sessions: sessions, MCPFor: gr.mcp.For, RepoFor: gr.repos.For, SubagentFor: gr.meta.Lookup}
	if gr.rows != nil {
		rows, at := gr.rows()
		in.ProcsAt = at
		for _, r := range rows {
			in.Procs = append(in.Procs, graph.Proc{PID: r.PID, PPID: r.PPID, Name: r.Name, Cmdline: r.Cmdline,
				CreateTime: r.CreateTime, Harness: string(r.Harness)})
		}
	}
	pids, pidErr := graph.ReadPidFiles(gr.pidDir)
	in.Pids = pids
	listens, netErr := gr.listen()
	in.Listens = listens
	sources := []snapshot.Source{source(graph.SourceClaudePids, stamp, pidErr)}
	if gr.foremanRoot != "" {
		sprints, flows, ferr := gr.foreman(now)
		if ferr == nil {
			in.Sprints, in.Flows = sprints, flows
		}
		sources = append(sources, source(graph.SourceForeman, stamp, ferr))
	}

	g := graph.Build(in)
	sources = append(sources, source(graph.SourceMCPConfig, stamp, gr.mcp.Err()),
		source(graph.SourceNetTables, stamp, netErr), source(graph.SourceGit, stamp, nil))

	nodes, edges := g.Recorded()
	first, err := st.RecordGraph(nodes, edges, now)
	if err == nil {
		g.Stamp(first)
	}
	return g, sources, err
}

// snapshotOnce builds one tick's snapshot from the store and the process
// sampler and writes it to path. A failing input does not stop the write: the
// file then carries that source as not ok with its error, so a reader sees
// the fault instead of silence. Only a failed file write is returned. gr nil
// leaves the relation graph's own sources out (the graph then has only the
// device node).
func (a *app) snapshotOnce(st *store.Store, path string, tick time.Duration, procs func() ([]sysmon.ProcessGroupSample, error), gr *graphReaders) error {
	cfg := a.cfgSnapshot()
	now := time.Now()
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

	groups, err := procs()
	if err != nil {
		sysSrc.OK, sysSrc.Error = false, err.Error()
	}

	sources := []snapshot.Source{storeSrc, sysSrc}
	g := graph.Build(graph.Input{Now: now})
	if gr != nil {
		var more []snapshot.Source
		var rerr error
		g, more, rerr = gr.buildGraph(st, flatSessions(snap.Sessions), now, stamp)
		if rerr != nil && sources[0].OK {
			sources[0].OK, sources[0].Error = false, "record graph: "+rerr.Error()
		}
		sources = append(sources, more...)
	}

	return snapshot.Write(path, snapshot.Build(snapshot.Input{
		Now:         now,
		TickSeconds: int(tick / time.Second),
		FXUSDEUR:    cfg.FXUSDEUR,
		Live:        snap,
		Sources:     sources,
		Processes:   groups,
		Graph:       g,
	}))
}

// startSnapshotWriter rewrites %USERPROFILE%\.burnmon\snapshot.json every
// snapshotTick. It opens no port and makes no model call. The process sampler
// is used from this one goroutine only, as sysmon requires.
func (a *app) startSnapshotWriter(st *store.Store) {
	path, err := snapshot.DefaultPath()
	if err != nil {
		log.Println("snapshot: could not resolve the path:", err)
		return
	}
	cfg := a.cfgSnapshot()
	ps := sysmon.NewProcessSampler()
	copilotVSC := cfg.CopilotVSCodeOtelFile != ""
	procs := func() ([]sysmon.ProcessGroupSample, error) { return ps.Tick(copilotVSC) }
	var gr *graphReaders
	if home, err := os.UserHomeDir(); err == nil {
		gr = newGraphReaders(home, cfg.ForemanRoot)
		gr.rows = ps.Procs
	} else {
		log.Println("snapshot: no home folder, relation graph sources off:", err)
	}
	go func() {
		ticker := time.NewTicker(snapshotTick)
		defer ticker.Stop()
		for {
			if err := a.snapshotOnce(st, path, snapshotTick, procs, gr); err != nil {
				log.Println("snapshot: write failed:", err)
			}
			<-ticker.C
		}
	}()
}
