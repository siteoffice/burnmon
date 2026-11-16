//go:build windows || darwin

package main

import (
	"log"

	"burnmon/internal/ingest"
	"burnmon/internal/pricing"
	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/store"
)

// cfgSnapshot copies the pricing config under the app's lock.
func (a *app) cfgSnapshot() pricing.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// newSnapshotWriter wires the shared snapshot writer (internal\ingest) to this
// app: its pricing config and the config file it loaded. The writer's process
// sampler is its own: a.procSampler belongs to startSampling's goroutine.
func (a *app) newSnapshotWriter(st *store.Store, path string) *snapshotwriter.Writer {
	return ingest.NewSnapshotWriter(st, path, a.cfgSnapshot, a.loadedCfg)
}

// startSnapshotWriter keeps %USERPROFILE%\.burnmon\snapshot.json fresh every
// snapshotwriter.Tick while this process holds the single-writer lock; when
// another process holds it, this one tries again each tick and takes over
// when the owner dies. It opens no port and makes no model call. The writer
// state goes to a.snapStatus, made first so a path failure still shows.
func (a *app) startSnapshotWriter(st *store.Store) {
	a.snapStatus = snapshotwriter.NewStatus()
	path, err := snapshot.DefaultPath()
	if err != nil {
		log.Println("snapshot: could not resolve the path:", err)
		a.snapStatus.Fail(err)
		return
	}
	lockPath, err := snapshotwriter.DefaultLockPath()
	if err != nil {
		log.Println("snapshot: could not resolve the lock path:", err)
		a.snapStatus.Fail(err)
		return
	}
	w := a.newSnapshotWriter(st, path)
	w.Status, w.Started = a.snapStatus, a.started
	go w.Run(snapshotwriter.NewLock(lockPath), "burnmon-dev.exe", nil)
}