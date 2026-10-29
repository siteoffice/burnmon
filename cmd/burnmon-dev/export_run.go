//go:build windows

// export_run.go is phase 4's export path (design doc "Export (AI-ready
// bundle)"): buildExportBundle gathers everything internal/devexport.Assemble
// needs for one [since, until) window, shared by both the CLI
// `burnmon-dev.exe export` subcommand (runExport below) and the header
// button's bdevExport binding (main.go), so the two can never drift apart.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"burnmon/internal/advisor"
	"burnmon/internal/dataset"
	"burnmon/internal/devexport"
	"burnmon/internal/live"
	"burnmon/internal/pricing"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

// parseExportTime accepts a bare local day (YYYY-MM-DD) or a full RFC3339
// timestamp: burnmon-cli export's own day-string convention, extended,
// since a 24-hour export benefits from time-of-day granularity that a bare
// day cannot express. endOfDay shifts a bare-day match forward by one local
// day, so `--until 2026-09-24` means "through the end of that day"
// (matching burnmon-cli export's own inclusive-whole-day convention for
// --until) rather than that day's first instant, which Assemble's
// exclusive upper bound would otherwise silently exclude entirely (found
// by review, 2026-09-24). A caller passing an explicit RFC3339 timestamp
// already names an exact instant, so endOfDay never applies to that form.
//
// WS2 item 5 ("local time everywhere"): this used to parse the bare-day
// form as UTC (time.Parse's own default with no Location in the layout)
// and add a UTC calendar day (always exactly 24h, AddDate on a UTC-located
// time) - modeled on burnmon-cli export's own pre-WS3 convention, but never
// itself updated when that command's flag was fixed to mean a local day.
// time.ParseInLocation plus AddDate on the resulting local-located time
// fixes both: a local calendar day, added as a real local day (23h/25h
// across a DST transition, not always 24h).
func parseExportTime(s string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		if endOfDay {
			t = t.AddDate(0, 0, 1)
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("could not parse %q as RFC3339 or YYYY-MM-DD", s)
}

// loadOrCreateRedactSalt returns a per-install random salt for
// devexport.Redact, persisted at dataDir\redact_salt so repeated exports on
// this machine hash the same name to the same token (Redact's own "stable"
// promise) while staying unguessable to anyone without this file: project,
// owner and client names are typically short and drawn from a small,
// guessable space (a client folder name, an owner slug), so an unsalted
// hash truncated to 8 hex characters is practically reversible by brute
// force otherwise (found by review, 2026-09-24). Generated once with
// crypto/rand; any read or write failure falls back to an empty salt
// (Redact still runs, just without that extra protection) rather than
// failing the export outright.
func loadOrCreateRedactSalt(dataDir string) string {
	path := filepath.Join(dataDir, "redact_salt")
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Println("redact salt: crypto/rand failed, redacting without a salt:", err)
		return ""
	}
	salt := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(salt), 0o600); err != nil {
		log.Println("redact salt: could not persist, using this run's salt only:", err)
	}
	return salt
}

// filterSamplesBefore and filterGroupsBefore trim sysmon's own
// lower-bounded-only Recent* queries down to an exact upper bound, so the
// export window is [since, until) on both ends.
func filterSamplesBefore(in []sysmon.Sample, until time.Time) []sysmon.Sample {
	out := make([]sysmon.Sample, 0, len(in))
	for _, s := range in {
		if s.Ts.Before(until) {
			out = append(out, s)
		}
	}
	return out
}

func filterGroupsBefore(in []sysmon.ProcessGroupSample, until time.Time) []sysmon.ProcessGroupSample {
	out := make([]sysmon.ProcessGroupSample, 0, len(in))
	for _, s := range in {
		if s.Ts.Before(until) {
			out = append(out, s)
		}
	}
	return out
}

// buildExportBundle gathers events and system/process-group samples for
// [since, until), runs the advisor over that same whole window, and
// assembles the export bundle. Never redacts: a caller that wants a
// redacted export calls devexport.Redact itself, with a salt from
// loadOrCreateRedactSalt, since only the caller knows dataDir.
func buildExportBundle(st *store.Store, sys *sysmon.Store, cfg *pricing.Config, since, until time.Time) (devexport.Bundle, error) {
	events, err := st.EventsSince(since)
	if err != nil {
		return devexport.Bundle{}, fmt.Errorf("events: %w", err)
	}
	sysHistory, err := sys.RecentSamples(since)
	if err != nil {
		return devexport.Bundle{}, fmt.Errorf("system samples: %w", err)
	}
	sysHistory = filterSamplesBefore(sysHistory, until)
	groups, err := sys.RecentProcessGroups(since)
	if err != nil {
		return devexport.Bundle{}, fmt.Errorf("process group samples: %w", err)
	}
	groups = filterGroupsBefore(groups, until)

	turns := live.BuildTurns(events, cfg, since, until)
	findings := advisor.Analyze(advisor.Input{
		Now: until, Turns: turns, SysmonHistory: sysHistory, ProcessGroups: groups,
	}, cfg, advisor.DefaultThresholds)

	machine := sysmon.CollectMachineProfile()
	b := devexport.Assemble(events, cfg, sysHistory, groups, machine, since, until, time.Now(), findings)
	return b, nil
}

// runExport is `burnmon-dev.exe export --since --until --out DIR [--redact]`
// (design doc "Export (AI-ready bundle)"). Runs one full Collect pass first,
// the same recipe burnmon-cli's own export command uses, so the store is as
// current as a one-shot process can make it even when burnmon-dev.exe's own
// GUI is closed.
//
// burnmon-dev.exe is built with -H windowsgui (build.ps1), the same as
// burnmon.exe, so a plain `.\burnmon-dev.exe export ...` from a shell can
// return immediately with no visible stdout: a GUI-subsystem process is not
// attached to the calling console the way a normal console-subsystem exe
// is. This command's own output (the "Wrote ..." line and file sizes below)
// still exists and this process still waits for the real work to finish
// either way; only its visibility depends on how the caller launches it.
// From PowerShell, `Start-Process -Wait -RedirectStandardOutput <file> ...`
// (or plain output redirection, `... export ... > out.txt`) reliably
// captures it; a bare foreground invocation may not (found by review,
// 2026-09-24, confirmed empirically against a real build this session).
func runExport(args []string) int {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	sinceStr := fs.String("since", "", "window start, local day (YYYY-MM-DD) or RFC3339 (default: 24h before --until)")
	untilStr := fs.String("until", "", "window end, local day (YYYY-MM-DD) or RFC3339, exclusive (default: now)")
	out := fs.String("out", "", "output directory")
	redact := fs.Bool("redact", false, "replace project, owner and client names with stable hashes")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "export: --out is required")
		return 1
	}

	until := time.Now()
	if *untilStr != "" {
		t, err := parseExportTime(*untilStr, true)
		if err != nil {
			fmt.Fprintln(os.Stderr, "export: --until:", err)
			return 1
		}
		until = t
	}
	since := until.Add(-24 * time.Hour)
	if *sinceStr != "" {
		t, err := parseExportTime(*sinceStr, false)
		if err != nil {
			fmt.Fprintln(os.Stderr, "export: --since:", err)
			return 1
		}
		since = t
	}
	if !since.Before(until) {
		fmt.Fprintln(os.Stderr, "export: --since must be before --until")
		return 1
	}

	dataDir := appDataDir()
	cfg, cfgFile := loadConfig(dataDir)
	if cfgFile != "" {
		log.Println("using config overrides from", cfgFile)
	}

	storePath, err := store.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "internal error:", err)
		return 1
	}
	st, err := store.Open(storePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not open the local store at", storePath, ":", err)
		return 1
	}
	defer st.Close()

	cache := dataset.Cache{Store: st}
	opts := dataset.CollectOpts{Seat: "Standard", MonthsN: 2, RefreshSlow: true}
	if _, err := cache.Collect(&cfg, opts, nil); err != nil {
		var seatErr *dataset.SeatError
		if !errors.As(err, &seatErr) && !errors.Is(err, dataset.ErrNoSessions) {
			fmt.Fprintln(os.Stderr, "could not collect:", err)
			return 1
		}
	}

	sysPath, err := sysmon.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "internal error:", err)
		return 1
	}
	sys, err := sysmon.Open(sysPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not open the sysmon store at", sysPath, ":", err)
		return 1
	}
	defer sys.Close()

	b, err := buildExportBundle(st, sys, &cfg, since, until)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not build the export:", err)
		return 1
	}
	if *redact {
		b = devexport.Redact(b, loadOrCreateRedactSalt(dataDir))
	}
	res, err := devexport.Write(*out, b)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not write the export:", err)
		return 1
	}
	fmt.Printf("Wrote %s\n", res.Dir)
	fmt.Printf("  summary.md  %d bytes\n", res.SummaryBytes)
	fmt.Printf("  data.json   %d bytes\n", res.DataJSONBytes)
	fmt.Printf("  daily.csv   %d bytes\n", res.DailyCSVBytes)
	return 0
}
