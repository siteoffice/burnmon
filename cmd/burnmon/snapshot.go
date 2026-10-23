package main

import (
	"log"
	"time"

	"burnmon/internal/live"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

// snapshotTick is how often the snapshot file is rewritten (R4, agent control
// plan). snapshot.StaleAfterTicks of these old and a reader says BurnMon is
// not running.
const snapshotTick = 5 * time.Second

// snapshotOnce builds one tick's snapshot from the store and the process
// sampler and writes it to path. A failing input does not stop the write: the
// file then carries that source as not ok with its error, so a reader sees
// the fault instead of silence. Only a failed file write is returned.
func (a *app) snapshotOnce(st *store.Store, path string, tick time.Duration, procs func() ([]sysmon.ProcessGroupSample, error)) error {
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

	return snapshot.Write(path, snapshot.Build(snapshot.Input{
		Now:         now,
		TickSeconds: int(tick / time.Second),
		FXUSDEUR:    cfg.FXUSDEUR,
		Live:        snap,
		Sources:     []snapshot.Source{storeSrc, sysSrc},
		Processes:   groups,
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
	ps := sysmon.NewProcessSampler()
	copilotVSC := a.cfgSnapshot().CopilotVSCodeOtelFile != ""
	procs := func() ([]sysmon.ProcessGroupSample, error) { return ps.Tick(copilotVSC) }
	go func() {
		ticker := time.NewTicker(snapshotTick)
		defer ticker.Stop()
		for {
			if err := a.snapshotOnce(st, path, snapshotTick, procs); err != nil {
				log.Println("snapshot: write failed:", err)
			}
			<-ticker.C
		}
	}()
}
