//go:build windows

// snapshot_line.go turns the writer state of this process and the snapshot
// file's reading into the one line the header, the box below it and the
// agents map note show. The page draws this text and composes none of its
// own, so the sentences live in one place (B7.3 sentence table).
package main

import (
	"fmt"
	"strings"

	"burnmon/internal/snapshot"
	"burnmon/internal/snapshotwriter"
)

// snapshotLine is the sentence and whether it is a warning (amber).
type snapshotLine struct {
	Text string `json:"text"`
	Warn bool   `json:"warn"`
}

// lineFor picks the sentence for writer state ws and reading r; the first
// matching row of the table wins. A state it does not know (the zero view:
// tests, or no Status at all) reads as unknown. Pure: no clock, no disk.
func lineFor(ws snapshotwriter.StatusView, r snapshot.Reading) snapshotLine {
	age := fmtAge(r.AgeSeconds)
	switch ws.State {
	case snapshotwriter.StateStarting:
		return snapshotLine{"Starting up: writing the first snapshot.", false}
	case snapshotwriter.StateLockError:
		return snapshotLine{"Cannot take the snapshot lock" + errTail(ws.Error) + ".", true}
	case snapshotwriter.StateFailed:
		if r.Status == snapshot.StatusOK || r.Status == snapshot.StatusStale {
			return snapshotLine{"Snapshot write failed" + errTail(ws.Error) + ". Last good snapshot " + age + " old.", true}
		}
		return snapshotLine{"Snapshot write failed" + errTail(ws.Error) + ". No good snapshot on disk.", true}
	case snapshotwriter.StateWriting, snapshotwriter.StateOther:
		other := ws.State == snapshotwriter.StateOther
		switch r.Status {
		case snapshot.StatusOK:
			return snapshotLine{"snapshot " + age + " old", false}
		case snapshot.StatusStale:
			if other {
				return snapshotLine{ownerName(ws) + " holds the snapshot lock but has not written for " + age + ".", true}
			}
			return snapshotLine{"This window writes the snapshot but has not written for " + age + ".", true}
		case snapshot.StatusMissing:
			if other {
				return snapshotLine{ownerName(ws) + " holds the snapshot lock but has not written a snapshot yet.", true}
			}
			return snapshotLine{"This window wrote the snapshot but the file is gone.", true}
		}
	default:
		switch r.Status {
		case snapshot.StatusOK:
			return snapshotLine{"snapshot " + age + " old", false}
		case snapshot.StatusStale:
			return snapshotLine{"No process has written the snapshot for " + age + ".", true}
		case snapshot.StatusMissing:
			return snapshotLine{"No snapshot file yet.", true}
		}
	}
	if r.Status == snapshot.StatusUnsupported {
		return snapshotLine{"snapshot is a newer version than this window reads", true}
	}
	// Unreadable, and any status this window does not know.
	text := "snapshot file cannot be read"
	if r.Error != "" {
		text += ": " + r.Error
	}
	return snapshotLine{text, true}
}

// errTail is ": <error>" without a trailing period, so the sentence's own
// period is not doubled; empty for no error.
func errTail(e string) string {
	e = strings.TrimRight(strings.TrimSpace(e), ".")
	if e == "" {
		return ""
	}
	return ": " + e
}

// ownerName names the other lock owner, or says "Another process" when its
// exe or PID could not be read.
func ownerName(ws snapshotwriter.StatusView) string {
	if ws.OwnerExe == "" || ws.OwnerPID <= 0 {
		return "Another process"
	}
	return fmt.Sprintf("%s (PID %d)", ws.OwnerExe, ws.OwnerPID)
}

// fmtAge is page.html's secDur in Go: "N s", "M min S s" (seconds left out
// at 0), "H h M min" (minutes left out at 0).
func fmtAge(sec int64) string {
	if sec < 0 {
		sec = 0
	}
	if sec < 60 {
		return fmt.Sprintf("%d s", sec)
	}
	if sec < 3600 {
		m, s := sec/60, sec%60
		if s == 0 {
			return fmt.Sprintf("%d min", m)
		}
		return fmt.Sprintf("%d min %d s", m, s)
	}
	h, m := sec/3600, (sec%3600)/60
	if m == 0 {
		return fmt.Sprintf("%d h", h)
	}
	return fmt.Sprintf("%d h %d min", h, m)
}
