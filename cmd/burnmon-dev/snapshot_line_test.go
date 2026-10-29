//go:build windows

package main

import (
	"strings"
	"testing"

	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
)

// One row per row of the B7.3 sentence table, in its order, plus the edge
// cases the table names (unknown owner, error with its own period).
func TestLineForTable(t *testing.T) {
	sv := func(state string) snapshotwriter.StatusView { return snapshotwriter.StatusView{State: state} }
	rd := func(status string, age int64) snapshot.Reading {
		return snapshot.Reading{Status: status, AgeSeconds: age}
	}
	owner := snapshotwriter.StatusView{State: snapshotwriter.StateOther, OwnerExe: "burnmon.exe", OwnerPID: 4242}
	failed := snapshotwriter.StatusView{State: snapshotwriter.StateFailed, Error: "disk full"}
	cases := []struct {
		name string
		ws   snapshotwriter.StatusView
		r    snapshot.Reading
		want snapshotLine
	}{
		{"starting, ok", sv("starting"), rd("ok", 3), snapshotLine{"Starting up: writing the first snapshot.", false}},
		{"starting, missing file", sv("starting"), rd("missing", 0), snapshotLine{"Starting up: writing the first snapshot.", false}},
		{"starting, stale", sv("starting"), rd("stale", 40920), snapshotLine{"Starting up: writing the first snapshot.", false}},
		{"lock error", snapshotwriter.StatusView{State: "lock_error", Error: "access denied"}, rd("stale", 90),
			snapshotLine{"Cannot take the snapshot lock: access denied.", true}},
		{"lock error, error ends in a period", snapshotwriter.StatusView{State: "lock_error", Error: "access denied."}, rd("ok", 1),
			snapshotLine{"Cannot take the snapshot lock: access denied.", true}},
		{"failed, ok", failed, rd("ok", 4), snapshotLine{"Snapshot write failed: disk full. Last good snapshot 4 s old.", true}},
		{"failed, stale", failed, rd("stale", 125), snapshotLine{"Snapshot write failed: disk full. Last good snapshot 2 min 5 s old.", true}},
		{"failed, missing", failed, rd("missing", 0), snapshotLine{"Snapshot write failed: disk full. No good snapshot on disk.", true}},
		{"failed, unreadable", failed, snapshot.Reading{Status: "unreadable", Error: "bad json"},
			snapshotLine{"Snapshot write failed: disk full. No good snapshot on disk.", true}},
		{"failed, unsupported", failed, rd("unsupported_version", 0), snapshotLine{"Snapshot write failed: disk full. No good snapshot on disk.", true}},
		{"writing, ok", sv("writing"), rd("ok", 5), snapshotLine{"snapshot 5 s old", false}},
		{"other, ok", owner, rd("ok", 7), snapshotLine{"snapshot 7 s old", false}},
		{"other, stale", owner, rd("stale", 3720), snapshotLine{"burnmon.exe (PID 4242) holds the snapshot lock but has not written for 1 h 2 min.", true}},
		{"other, missing", owner, rd("missing", 0), snapshotLine{"burnmon.exe (PID 4242) holds the snapshot lock but has not written a snapshot yet.", true}},
		{"other with unknown owner, stale", sv("other"), rd("stale", 60),
			snapshotLine{"Another process holds the snapshot lock but has not written for 1 min.", true}},
		{"other with unknown owner, missing", sv("other"), rd("missing", 0),
			snapshotLine{"Another process holds the snapshot lock but has not written a snapshot yet.", true}},
		{"writing, stale", sv("writing"), rd("stale", 20), snapshotLine{"This window writes the snapshot but has not written for 20 s.", true}},
		{"writing, missing", sv("writing"), rd("missing", 0), snapshotLine{"This window wrote the snapshot but the file is gone.", true}},
		{"writing, unsupported", sv("writing"), rd("unsupported_version", 0), snapshotLine{"snapshot is a newer version than this window reads", true}},
		{"unknown, unsupported", sv(""), rd("unsupported_version", 0), snapshotLine{"snapshot is a newer version than this window reads", true}},
		{"other, unreadable", owner, snapshot.Reading{Status: "unreadable", Error: "bad json"}, snapshotLine{"snapshot file cannot be read: bad json", true}},
		{"unknown, unreadable without error", sv(""), rd("unreadable", 0), snapshotLine{"snapshot file cannot be read", true}},
		{"unknown, ok", sv(""), rd("ok", 9), snapshotLine{"snapshot 9 s old", false}},
		{"unknown, stale", sv(""), rd("stale", 40920), snapshotLine{"No process has written the snapshot for 11 h 22 min.", true}},
		{"unknown, missing", sv(""), rd("missing", 0), snapshotLine{"No snapshot file yet.", true}},
	}
	for _, c := range cases {
		if got := lineFor(c.ws, c.r); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

// No writer state and reading together ever says "not running": while this
// window runs, a BurnMon is running.
func TestLineForNeverNotRunning(t *testing.T) {
	states := []snapshotwriter.StatusView{
		{}, {State: snapshotwriter.StateStarting}, {State: snapshotwriter.StateWriting},
		{State: snapshotwriter.StateOther}, {State: snapshotwriter.StateOther, OwnerExe: "burnmon.exe", OwnerPID: 7},
		{State: snapshotwriter.StateFailed, Error: "x"}, {State: snapshotwriter.StateLockError, Error: "y"},
	}
	readings := []string{snapshot.StatusOK, snapshot.StatusStale, snapshot.StatusMissing, snapshot.StatusUnreadable, snapshot.StatusUnsupported}
	for _, ws := range states {
		for _, st := range readings {
			got := lineFor(ws, snapshot.Reading{Status: st, AgeSeconds: 100, Error: "e"})
			if got.Text == "" || strings.Contains(strings.ToLower(got.Text), "not running") {
				t.Errorf("state %q, reading %q: %q", ws.State, st, got.Text)
			}
		}
	}
}

// fmtAge matches page.html's secDur.
func TestFmtAge(t *testing.T) {
	for sec, want := range map[int64]string{-5: "0 s", 0: "0 s", 59: "59 s", 60: "1 min", 61: "1 min 1 s",
		3599: "59 min 59 s", 3600: "1 h", 3660: "1 h 1 min", 3661: "1 h 1 min", 40920: "11 h 22 min"} {
		if got := fmtAge(sec); got != want {
			t.Errorf("fmtAge(%d) = %q, want %q", sec, got, want)
		}
	}
}
