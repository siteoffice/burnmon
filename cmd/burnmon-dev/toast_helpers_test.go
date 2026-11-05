//go:build windows || darwin

package main

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

func devJSONPath(dir string) string { return filepath.Join(dir, "burnmon-dev.json") }

var toastT0 = time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)

func waitGate(id, title string) waits.Item {
	return waits.Item{Kind: waits.KindGate, ID: id, Title: title, Group: waits.GroupBlocked}
}

func writeToastSnapshot(t *testing.T, path string, at time.Time, items ...waits.Item) {
	t.Helper()
	s := snapshot.Snapshot{SchemaVersion: snapshot.SchemaVersion, WrittenAt: at.UTC().Format(time.RFC3339), TickSeconds: 2, WaitItems: items}
	if err := snapshot.Write(path, s); err != nil {
		t.Fatal(err)
	}
}

// newToastWatch is a watch on a fresh snapshot path and settings folder; the
// test closes backfilled when its startup backfill is meant to be done.
func newToastWatch(t *testing.T, backfilled chan struct{}) (*toastWatch, *[]string) {
	t.Helper()
	shown := &[]string{}
	tw := &toastWatch{path: filepath.Join(t.TempDir(), "snapshot.json"), setDir: t.TempDir(), backfilled: backfilled,
		show: func(s string) { *shown = append(*shown, s) }}
	return tw, shown
}
