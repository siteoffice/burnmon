package main

import (
	"log"
	"os"
	"os/exec"

	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

// newSnapshotWriter wires the shared snapshot writer to this app: its pricing
// config, a process sampler of its own (used from one goroutine only, as
// sysmon requires) and, when the home folder resolves, the relation graph's
// own sources.
func (a *app) newSnapshotWriter(st *store.Store, path string) *snapshotwriter.Writer {
	cfg := a.cfgSnapshot()
	ps := sysmon.NewProcessSampler()
	copilotVSC := cfg.CopilotVSCodeOtelFile != ""
	w := &snapshotwriter.Writer{
		Store:  st,
		Path:   path,
		Tick:   snapshotwriter.Tick,
		Config: a.cfgSnapshot,
		Procs:  func() ([]sysmon.ProcessGroupSample, error) { return ps.Tick(copilotVSC) },
	}
	if home, err := os.UserHomeDir(); err == nil {
		w.Graph = snapshotwriter.NewGraphReaders(home, cfg.ForemanRoot)
		w.Graph.Rows = ps.Procs
		if cfg.AgentView {
			if p, err := exec.LookPath("claude"); err == nil {
				w.Graph.AgentView = snapshotwriter.ClaudeAgentView(p)
			}
		}
	} else {
		log.Println("snapshot: no home folder, relation graph sources off:", err)
	}
	return w
}

// startSnapshotWriter keeps %USERPROFILE%\.burnmon\snapshot.json fresh every
// snapshotwriter.Tick while this process holds the single-writer lock; when
// another process holds it, this one tries again each tick and takes over
// when the owner dies. It opens no port and makes no model call.
func (a *app) startSnapshotWriter(st *store.Store) {
	path, err := snapshot.DefaultPath()
	if err != nil {
		log.Println("snapshot: could not resolve the path:", err)
		return
	}
	lockPath, err := snapshotwriter.DefaultLockPath()
	if err != nil {
		log.Println("snapshot: could not resolve the lock path:", err)
		return
	}
	w := a.newSnapshotWriter(st, path)
	go w.Run(snapshotwriter.NewLock(lockPath), "burnmon.exe", nil)
}
