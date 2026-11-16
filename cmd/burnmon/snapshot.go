package main

import (
	"log"

	"burnmon/internal/ingest"
	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/store"
)

// newSnapshotWriter wires the shared snapshot writer (internal\ingest) to
// this app: its pricing config and the config file it loaded.
func (a *app) newSnapshotWriter(st *store.Store, path string) *snapshotwriter.Writer {
	return ingest.NewSnapshotWriter(st, path, a.cfgSnapshot, a.loadedCfg)
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
