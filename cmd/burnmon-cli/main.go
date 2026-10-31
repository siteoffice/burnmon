// burnmon-cli builds your own Claude usage and cost dashboard from the session
// transcripts Cowork and Claude Code already write on this machine.
//
// Standalone port of claude_usage_extract.py (schema 1) plus the dashboard
// template of the Cowork "Claude Cost (User)" artifact. Nothing leaves your
// computer: no API key, no admin rights, no network calls.
//
// This is deliberately personal. It reads only your own transcript folders
// and refuses no one, because there is nothing to refuse: other people's
// usage is simply not on your disk.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"burnmon/internal/dataset"
	"burnmon/internal/export"
	"burnmon/internal/graph"
	"burnmon/internal/insight"
	"burnmon/internal/live"
	"burnmon/internal/merge"
	"burnmon/internal/mergereport"
	"burnmon/internal/pricing"
	"burnmon/internal/report"
	"burnmon/internal/store"
	"burnmon/internal/waits"
)

const version = "0.4.0-beta.2"

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ", ") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "price-check" {
		os.Exit(runPriceCheck(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "live" {
		os.Exit(runLive(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "reown" {
		os.Exit(runReown(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "graph" {
		os.Exit(runGraph(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "waits" {
		os.Exit(runWaits(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "backfill-parents" {
		os.Exit(runBackfillParents(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "tools" {
		os.Exit(runTools(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "insight" {
		os.Exit(runInsight(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "export" {
		os.Exit(runExport(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "merge" {
		os.Exit(runMerge(os.Args[2:]))
		return
	}
	os.Exit(run())
}

// runLive does one full Collect pass (so the store is as current as a
// one-shot process can make it, same as `report`), then prints the Now
// page's JSON snapshot once: "for tests and for a later TUI" (v0.1 spec,
// Step 3).
func runLive(args []string) int {
	fs := flag.NewFlagSet("live", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file (default: burnmon.json next to the exe, if present)")
	jsonOut := fs.Bool("json", false, "print the snapshot as JSON (the only supported form today)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfg, err := loadConfig(*cfgPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
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
	opts := dataset.CollectOpts{Seat: "Standard", MonthsN: 1, RefreshSlow: true}
	if _, err := cache.Collect(&cfg, opts, nil); err != nil {
		var seatErr *dataset.SeatError
		if !errors.As(err, &seatErr) && !errors.Is(err, dataset.ErrNoSessions) {
			fmt.Fprintln(os.Stderr, "could not collect:", err)
			return 1
		}
		// No sessions inside the reporting window is fine here: the store
		// may still hold events (an old session), or none at all, either
		// way the snapshot below is what actually answers "is anything
		// running now".
	}

	// S3: the same windowed query and SessionTotals path the app uses (F1),
	// not AllEvents, which re-groups the whole store on every call.
	now := time.Now()
	events, err := st.EventsSince(now.Add(-live.ChartWindow))
	if err != nil {
		fmt.Fprintln(os.Stderr, "internal error:", err)
		return 1
	}
	snap := live.BuildSnapshot(events, &cfg, now)
	if err := live.ApplySessionTotals(snap.Sessions, st, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "internal error:", err)
		return 1
	}

	if !*jsonOut {
		fmt.Fprintln(os.Stderr, "live currently only supports -json")
	}
	b, err := json.MarshalIndent(snap, "", " ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "internal error:", err)
		return 1
	}
	fmt.Println(string(b))
	return 0
}

// runReown re-applies burnmon.json's owner rules (P6) to every event already
// in the store, for after an owner rule change: without this, an existing
// session keeps whatever owner ingest gave it at the time, even once the
// rules that produced it are edited.
func runReown(args []string) int {
	fs := flag.NewFlagSet("reown", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file (default: burnmon.json next to the exe, if present)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfg, err := loadConfig(*cfgPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
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

	// Memoize ClientFor per project: ReownEvents calls it once per stored
	// row, and a store can hold hundreds of thousands of rows sharing far
	// fewer distinct project paths (same reasoning as dataset.ingest's own
	// clientCache).
	clientCache := map[string]string{}
	clientFor := func(project string) string {
		if v, ok := clientCache[project]; ok {
			return v
		}
		v := cfg.ClientFor(project)
		clientCache[project] = v
		return v
	}
	n, err := st.ReownEvents(cfg.OwnerFor, clientFor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not reown events:", err)
		return 1
	}
	fmt.Printf("reowned %d event(s)\n", n)
	return 0
}

// runGraph prints the relation graph (R6) for a past window as JSON: the
// sessions and subagents from the store, the live-only edges it recorded,
// and Foreman sprints when a Foreman folder is configured (foreman_root in
// burnmon.json, or -foreman).
func runGraph(args []string) int {
	fs := flag.NewFlagSet("graph", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file (default: burnmon.json next to the exe, if present)")
	since := fs.String("since", "24h", "window to show, e.g. 2h or 7d")
	foreman := fs.String("foreman", "", "Foreman folder (overrides foreman_root in the config)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	d, err := parseSinceDuration(*since)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cfg, err := loadConfig(*cfgPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	root := cfg.ForemanRoot
	if *foreman != "" {
		root = *foreman
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
	now := time.Now()
	hi := graph.HistoryInput{From: now.Add(-d), To: now, Now: now, RepoFor: graph.NewRepoLocator().For}
	if home, err := os.UserHomeDir(); err == nil {
		hi.SubagentFor = graph.NewSubagentMeta(filepath.Join(home, ".claude", "projects")).Lookup
	}
	if root != "" {
		sprints, _, err := graph.ReadForeman(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "could not read Foreman state:", err)
			return 1
		}
		hi.Sprints = sprints
	}
	g, err := graph.History(st, hi)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not build the graph:", err)
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(g); err != nil {
		return 1
	}
	return 0
}

// runWaits prints the Foreman waits and outcomes (R9) for the live check:
// open gates, waiting sprints, failed checks and sprints finished in the last
// 24 hours. Read only: no store, no sessions, no agent view.
func runWaits(args []string) int { return runWaitsTo(os.Stdout, args) }

func runWaitsTo(w io.Writer, args []string) int {
	fs := flag.NewFlagSet("waits", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "config file (default: burnmon.json next to the exe, if present)")
	foreman := fs.String("foreman", "", "Foreman folder (overrides foreman_root in the config)")
	jsonOut := fs.Bool("json", false, "print the items as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	cfg, err := loadConfig(*cfgPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	root := cfg.ForemanRoot
	if *foreman != "" {
		root = *foreman
	}
	if root == "" {
		fmt.Fprintln(os.Stderr, "no Foreman folder: pass -foreman <dir> or set foreman_root in burnmon.json")
		return 1
	}
	sprints, flows, err := graph.ReadForeman(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not read Foreman state:", err)
		return 1
	}
	wait, outcomes := waits.Build(waits.Input{Now: time.Now(), Sprints: sprints, Flows: flows})
	if *jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		out := struct {
			WaitItems    []waits.Item `json:"wait_items"`
			OutcomeItems []waits.Item `json:"outcome_items"`
		}{wait, outcomes}
		if err := enc.Encode(out); err != nil {
			return 1
		}
		return 0
	}
	fmt.Fprint(w, waitsReport(wait, outcomes))
	return 0
}

// waitsReport is the text form: one line per item under two headings, then
// the counts line. A failed check is counted once per id even when it is in
// both lists.
func waitsReport(wait, outcomes []waits.Item) string {
	var b strings.Builder
	line := func(it waits.Item) {
		spec, result := it.SpecPath, it.ResultPath
		if spec == "" {
			spec = "-"
		}
		if result == "" {
			result = "-"
		}
		fmt.Fprintf(&b, "%s  %s  %s  %s  spec=%s  result=%s\n", it.Kind, it.Title, it.Detail,
			waitsAge(it.AgeSeconds), spec, result)
	}
	b.WriteString("Waits on you\n")
	for _, it := range wait {
		line(it)
	}
	b.WriteString("Outcomes, last 24 h\n")
	for _, it := range outcomes {
		line(it)
	}
	gates, sprints, finished := 0, 0, 0
	checks := map[string]bool{}
	for _, it := range wait {
		switch it.Kind {
		case waits.KindGate:
			gates++
		case waits.KindSprint:
			sprints++
		case waits.KindFailedCheck:
			checks[it.ID] = true
		}
	}
	for _, it := range outcomes {
		switch it.Kind {
		case waits.KindFinished:
			finished++
		case waits.KindFailedCheck:
			checks[it.ID] = true
		}
	}
	fmt.Fprintf(&b, "open gates: %d, waiting sprints: %d, failed checks: %d, finished last 24 h: %d\n",
		gates, sprints, len(checks), finished)
	return b.String()
}

func waitsAge(s int64) string {
	d := time.Duration(s) * time.Second
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%ds", s)
}

// runBackfillParents sets the parent session on Claude subagent rows stored
// before the adapter read it (R6). Without -apply it is a dry run: it only
// prints how many rows it would change. Copy the store file first.
func runBackfillParents(args []string) int {
	fs := flag.NewFlagSet("backfill-parents", flag.ExitOnError)
	apply := fs.Bool("apply", false, "write the change (default: dry run, count only)")
	if err := fs.Parse(args); err != nil {
		return 1
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
	n, err := st.BackfillClaudeParents(*apply)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not backfill parents:", err)
		return 1
	}
	if *apply {
		fmt.Printf("store %s: set the parent on %d row(s)\n", storePath, n)
	} else {
		fmt.Printf("store %s: dry run, %d row(s) would get a parent\n", storePath, n)
	}
	return 0
}

// parseSinceDuration parses `tools`'s -since flag: a bare integer with a "d"
// suffix (30d) alongside anything time.ParseDuration already accepts, since
// Go's own duration parser has no day unit.
func parseSinceDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("bad day count in %q: %w", s, err)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// runTools prints S2's tool_calls totals since the given lookback window:
// tool name, calls, sessions, and input/result bytes, the same
// store.ToolCallTotals query a later Tools tab will read. Does a full
// Collect pass first (same as live and price-check) so the store is as
// current as a one-shot process can make it.
func runTools(args []string) int {
	fs := flag.NewFlagSet("tools", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file (default: burnmon.json next to the exe, if present)")
	since := fs.String("since", "30d", "lookback window, e.g. 30d, 7d, 24h")
	jsonOut := fs.Bool("json", false, "print as JSON instead of a table")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	dur, err := parseSinceDuration(*since)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid -since:", err)
		return 1
	}

	cfg, err := loadConfig(*cfgPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
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
	opts := dataset.CollectOpts{Seat: "Standard", MonthsN: 1, RefreshSlow: true}
	if _, err := cache.Collect(&cfg, opts, nil); err != nil {
		var seatErr *dataset.SeatError
		if !errors.As(err, &seatErr) && !errors.Is(err, dataset.ErrNoSessions) {
			fmt.Fprintln(os.Stderr, "could not collect:", err)
			return 1
		}
	}

	rows, err := st.ToolCallTotals(time.Now().Add(-dur))
	if err != nil {
		fmt.Fprintln(os.Stderr, "internal error:", err)
		return 1
	}

	if *jsonOut {
		b, err := json.MarshalIndent(rows, "", " ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "internal error:", err)
			return 1
		}
		fmt.Println(string(b))
		return 0
	}

	fmt.Printf("%-28s %8s %8s %14s %14s\n", "tool", "calls", "sessions", "input bytes", "result bytes")
	for _, r := range rows {
		fmt.Printf("%-28s %8d %8d %14d %14d\n", r.Tool, r.Calls, r.Sessions, r.InputBytes, r.ResultBytes)
	}
	return 0
}

// runInsight prints v0.2 I1/I2's findings for one session: `burnmon-cli
// insight <session-id> --json`. Does a full Collect pass first (same as
// live and tools) so the store is as current as a one-shot process can
// make it, then reads that one session's whole history through
// store.EventsForSession rather than AllEvents.
func runInsight(args []string) int {
	// `insight <session-id> --json` puts the flag after the positional
	// argument; Go's flag package stops parsing flags at the first
	// non-flag argument, so the session id and the flags are split out by
	// hand before fs.Parse ever sees them, rather than requiring
	// `insight --json <session-id>`.
	var sessionID string
	var flagArgs []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") || sessionID != "" {
			flagArgs = append(flagArgs, a)
			continue
		}
		sessionID = a
	}

	fs := flag.NewFlagSet("insight", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file (default: burnmon.json next to the exe, if present)")
	jsonOut := fs.Bool("json", false, "print findings as JSON instead of a table")
	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}
	if sessionID == "" {
		fmt.Fprintln(os.Stderr, "usage: burnmon-cli insight <session-id> [--json]")
		return 1
	}

	cfg, err := loadConfig(*cfgPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
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
	opts := dataset.CollectOpts{Seat: "Standard", MonthsN: 1, RefreshSlow: true}
	if _, err := cache.Collect(&cfg, opts, nil); err != nil {
		var seatErr *dataset.SeatError
		if !errors.As(err, &seatErr) && !errors.Is(err, dataset.ErrNoSessions) {
			fmt.Fprintln(os.Stderr, "could not collect:", err)
			return 1
		}
	}

	events, err := st.EventsForSession(sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "internal error:", err)
		return 1
	}
	findings := insight.Analyze(events, &cfg)

	if *jsonOut {
		b, err := json.MarshalIndent(findings, "", " ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "internal error:", err)
			return 1
		}
		fmt.Println(string(b))
		return 0
	}

	if len(findings) == 0 {
		fmt.Println("no findings for session", sessionID)
		return 0
	}
	fmt.Printf("%-6s %-12s %-25s %-10s %s\n", "turn", "kind", "at", "confidence", "cause")
	for _, f := range findings {
		fmt.Printf("%-6d %-12s %-25s %-10.2f %s\n", f.Turn, f.Kind, f.At.Format(time.RFC3339), f.Confidence, f.Cause)
	}
	return 0
}

// runExport prints K4's export: `burnmon-cli export --since <date> --until
// <date> --owner <name>... --label <name> --out <file>`. Does a full
// Collect pass first (same as live, tools and insight) so the store is as
// current as a one-shot process can make it. When owner rules exist, --owner
// is required and the command refuses without it, so a ClientA row never
// leaves this laptop by accident (P6's wall rule, v0.2 spec 2.2).
func runExport(args []string) int {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file (default: burnmon.json next to the exe, if present)")
	since := fs.String("since", "", "earliest local day to include, YYYY-MM-DD (default: no lower bound)")
	until := fs.String("until", "", "latest local day to include, YYYY-MM-DD, inclusive (default: no upper bound)")
	label := fs.String("label", export.DefaultLabel, "this machine's label in the export; never the hostname")
	out := fs.String("out", "", "output file path")
	var owners multiFlag
	fs.Var(&owners, "owner", "owner to include; repeatable, required when owner rules exist")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "export: --out is required")
		return 1
	}

	cfg, err := loadConfig(*cfgPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	f := export.Filter{Since: *since, Until: *until, Owners: owners}
	if err := export.Validate(&cfg, f); err != nil {
		fmt.Fprintln(os.Stderr, exportValidateText(err))
		return 1
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
	opts := dataset.CollectOpts{Seat: "Standard", MonthsN: 1, RefreshSlow: true}
	if _, err := cache.Collect(&cfg, opts, nil); err != nil {
		var seatErr *dataset.SeatError
		if !errors.As(err, &seatErr) && !errors.Is(err, dataset.ErrNoSessions) {
			fmt.Fprintln(os.Stderr, "could not collect:", err)
			return 1
		}
	}

	doc, err := writeExport(st, &cfg, f, *label, *out, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, exportFailureText(err, *out))
		return 1
	}
	fmt.Printf("Wrote %s with %d row(s), label %q\n", *out, len(doc.Rows), doc.Label)
	return 0
}

// writeExport selects, builds and writes one export file through the same
// path the apps use, and returns the document it wrote.
func writeExport(st *store.Store, cfg *pricing.Config, f export.Filter, label, out string, now time.Time) (export.Doc, error) {
	events, err := st.AllEvents()
	if err != nil {
		return export.Doc{}, err
	}
	doc := export.Build(export.Select(events, f), cfg, label, now)
	b, err := export.Encode(doc)
	if err != nil {
		return export.Doc{}, err
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return export.Doc{}, &exportWriteError{err}
	}
	return doc, nil
}

// exportWriteError marks the one failure the CLI reports as "could not write":
// the file write itself.
type exportWriteError struct{ err error }

func (e *exportWriteError) Error() string { return e.err.Error() }
func (e *exportWriteError) Unwrap() error { return e.err }

// exportValidateText is what the CLI prints when export.Validate refuses. The
// missing-owner words are the CLI's own (--owner), not the shared error text.
func exportValidateText(err error) string {
	if errors.Is(err, export.ErrOwnerRequired) {
		return "export: owner rules are configured; --owner is required " +
			"(repeat --owner for more than one), so a ClientA row never leaves by accident"
	}
	return "export: " + err.Error()
}

// exportFailureText is what the CLI prints when writeExport fails: "could not
// write" for the file write, "internal error:" for everything else.
func exportFailureText(err error, out string) string {
	var we *exportWriteError
	if errors.As(err, &we) {
		return "could not write " + out + " : " + err.Error()
	}
	return "internal error: " + err.Error()
}

// runMerge combines K4 export files into K5's merged.json plus a static,
// offline report.html: `burnmon-cli merge <file>... --out <dir>`. Refuses
// files whose schema value differs (merge.Merge).
func runMerge(args []string) int {
	var files []string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
			// Every merge flag today (-out) takes a value; a bare flag with
			// no "=" is followed by its value as the next token, which must
			// not be mistaken for a positional file argument.
			if !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		files = append(files, a)
	}

	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	outDir := fs.String("out", "", "output directory for merged.json and report.html")
	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "usage: burnmon-cli merge <file>... --out <dir>")
		return 1
	}
	if *outDir == "" {
		fmt.Fprintln(os.Stderr, "merge: --out is required")
		return 1
	}

	var docs []export.Doc
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "could not read", f, ":", err)
			return 1
		}
		var d export.Doc
		if err := json.Unmarshal(b, &d); err != nil {
			fmt.Fprintln(os.Stderr, "could not parse", f, ":", err)
			return 1
		}
		docs = append(docs, d)
	}

	m, err := merge.Merge(docs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "could not create", *outDir, ":", err)
		return 1
	}
	mergedPath := filepath.Join(*outDir, "merged.json")
	mb, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "internal error:", err)
		return 1
	}
	if err := os.WriteFile(mergedPath, mb, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "could not write", mergedPath, ":", err)
		return 1
	}
	reportPath := filepath.Join(*outDir, "report.html")
	if err := mergereport.Write(reportPath, m); err != nil {
		fmt.Fprintln(os.Stderr, "could not write", reportPath, ":", err)
		return 1
	}
	fmt.Println("Wrote", mergedPath)
	fmt.Println("Wrote", reportPath)
	return 0
}

// runPriceCheck prints every compiled-in (or burnmon.json-overridden) price
// book with the date it was last checked, per the v0.1 Step 2 done-when:
// "price-check CLI command prints every book with its date."
func runPriceCheck(args []string) int {
	fs := flag.NewFlagSet("price-check", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file (default: burnmon.json next to the exe, if present)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfg, err := loadConfig(*cfgPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}

	fmt.Println("Anthropic price book (family, v0.2), dated", pricing.AnthropicPriceBookDate)
	for _, fam := range cfg.Families() {
		p := cfg.Prices[fam]
		fmt.Printf("  %-10s in %6.2f  out %6.2f  USD/MTok\n", p.Label, p.In, p.Out)
	}

	fmt.Println()
	fmt.Println("OpenAI price book (family, v0.2), dated", pricing.OpenAIPriceBookDate)
	models := make([]string, 0, len(cfg.OpenAIPrices))
	for m := range cfg.OpenAIPrices {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		p := cfg.OpenAIPrices[m]
		fmt.Printf("  %-20s (%-14s) in %6.3f  cached-in %6.3f  out %6.3f  USD/MTok\n",
			m, p.Label, p.In, p.CachedIn, p.Out)
	}

	fmt.Println()
	fmt.Println("Anthropic API list book, dated", cfg.AnthropicBook.Date, "source", cfg.AnthropicBook.Source)
	printAnthropicBook(cfg.AnthropicBook)

	fmt.Println()
	fmt.Println("OpenAI API list book, dated", cfg.OpenAIBook.Date, "source", cfg.OpenAIBook.Source)
	printOpenAIBook(cfg.OpenAIBook)

	fmt.Println()
	fmt.Println("GitHub Copilot AI credits book, dated", cfg.CopilotCredits.Date, "source", cfg.CopilotCredits.Source)
	fmt.Println("  plans, dated", cfg.CopilotCredits.Date, "source", cfg.CopilotCredits.PlansSource)
	printCopilotPlans(cfg.CopilotCredits)
	printCopilotCreditModels(cfg.CopilotCredits)

	fmt.Println()
	if cfg.SubscriptionConfigured() {
		fmt.Printf("Subscription share book, calibrated %s, source %s\n", cfg.Subscription.CalibratedOn, cfg.Subscription.Source)
	} else {
		fmt.Println("Subscription share book: not configured (Defaults' illustrative example, calibrated_on \"example\")")
	}

	fmt.Println()
	fmt.Println("Hermes and any vendor above with no book: tokens only.")
	return 0
}

func printAnthropicBook(b pricing.AnthropicAPIBook) {
	models := make([]string, 0, len(b.Models))
	for m := range b.Models {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		r := b.Models[m]
		fmt.Printf("  %-28s (%-18s) in %6.2f  cache-write(5m/1h) %5.2f/%5.2f  cache-read %5.3f  out %6.2f  USD/MTok\n",
			m, r.Label, r.In, r.CacheWrite5m, r.CacheWrite1h, r.CacheRead, r.Out)
	}
}

func printOpenAIBook(b pricing.OpenAIAPIBook) {
	models := make([]string, 0, len(b.Models))
	for m := range b.Models {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		r := b.Models[m]
		fmt.Printf("  %-20s (%-14s) in %6.3f  cached-in %6.3f  out %6.3f  USD/MTok\n",
			m, r.Label, r.In, r.CachedIn, r.Out)
	}
}

func printCopilotPlans(b pricing.CreditBook) {
	plans := make([]string, 0, len(b.Plans))
	for p := range b.Plans {
		plans = append(plans, p)
	}
	sort.Strings(plans)
	for _, p := range plans {
		plan := b.Plans[p]
		fmt.Printf("  %-6s $%-6.2f/month  %.0f credits/month\n", p, plan.MonthlyUSD, plan.MonthlyCredits)
	}
}

func printCopilotCreditModels(b pricing.CreditBook) {
	models := make([]string, 0, len(b.Models))
	for m := range b.Models {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		r := b.Models[m]
		fmt.Printf("  %-28s (%-16s) in %6.3f  cached-in %6.3f  cache-write %6.3f  out %6.2f  USD-equiv/MTok (1 credit = $%.2f)\n",
			m, r.Label, r.In, r.CachedIn, r.CacheWrite, r.Out, b.CreditUnitUSD())
	}
}

func run() int {
	monthsN := flag.Int("months", 2, "how many months to include, counting the current one")
	seat := flag.String("seat", "Standard", "your own seat tier (Standard or Premium)")
	var sources multiFlag
	flag.Var(&sources, "source", "extra folder to scan; repeatable, overrides auto-detect")
	outDir := flag.String("out", "", "report folder (default: reports next to the exe)")
	jsonOut := flag.String("json", "", "also write the raw dataset to this JSON path")
	cfgPath := flag.String("config", "", "config file (default: burnmon.json next to the exe, if present)")
	noOpen := flag.Bool("no-open", false, "do not open the report in the browser")
	quiet := flag.Bool("quiet", false, "suppress progress output")
	noCache := flag.Bool("no-cache", false, "re-read every transcript from the start, "+
		"ignoring the store's cursors")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("burnmon-cli " + version)
		return 0
	}

	cfg, err := loadConfig(*cfgPath, *quiet)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}

	// An explicit -seat wins; otherwise a your_seat set in burnmon.json
	// wins over the flag's own "Standard" default, so a shared config for a
	// team that is all on one tier (see the Groundwork Kit) never needs
	// -seat passed by hand. flag.Visit is the only way to tell "passed at
	// its default value" from "not passed at all".
	effectiveSeat := *seat
	seatFlagSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "seat" {
			seatFlagSet = true
		}
	})
	if !seatFlagSet && cfg.Subscription.YourSeat != "" {
		effectiveSeat = cfg.Subscription.YourSeat
	}

	// The CLI runs once and exits, so this Cache only lives for one Collect
	// call; the store on disk is what carries the benefit across runs, same
	// file the app binary also reads and writes.
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

	// Best-effort cleanup of the old gob-based parse cache, replaced by the
	// SQLite store above; ignore any error, missing is the expected case on
	// every run after the first.
	_ = os.Remove(filepath.Join(filepath.Dir(storePath), "parsecache.gob"))

	cache := dataset.Cache{Store: st}
	progress := func(done, total int) {
		if *quiet {
			return
		}
		if done == 0 {
			fmt.Printf("Scanning %d folder(s), found %d session files.\n", len(cache.Sources), total)
			return
		}
		if done%200 == 0 {
			fmt.Printf("  ...%d/%d\n", done, total)
		}
	}

	// A one-shot run always does a full pass: there is no ticker here to
	// spread the WSL cost over, so RefreshSlow is always true. See "Two
	// refresh cadences" in docs/2026-08-17_wsl-source-detection-design.md,
	// which only applies to the app's own interval tickers.
	opts := dataset.CollectOpts{
		Seat:        effectiveSeat,
		MonthsN:     *monthsN,
		Sources:     []string(sources),
		RefreshSlow: true,
		ForceFull:   *noCache,
	}
	payload, err := cache.Collect(&cfg, opts, progress)
	if err != nil {
		var seatErr *dataset.SeatError
		switch {
		case errors.As(err, &seatErr):
			fmt.Fprintf(os.Stderr, "unknown seat tier %q, valid: %s\n", seatErr.Seat, strings.Join(seatErr.Valid, ", "))
			return 1
		case errors.Is(err, dataset.ErrNoSources):
			fmt.Fprintln(os.Stderr, "No Claude transcript folders found on this machine.")
			fmt.Fprintln(os.Stderr, "If you only use Claude in the browser, there is nothing to read:")
			fmt.Fprintln(os.Stderr, "those chats keep no local transcript. See claude.ai/settings/usage instead.")
			return 2
		case errors.Is(err, dataset.ErrNoFiles):
			fmt.Fprintln(os.Stderr, "No session transcripts found.")
			return 2
		case errors.Is(err, dataset.ErrNoSessions):
			fmt.Fprintln(os.Stderr, "No sessions inside the reporting window; nothing to show.")
			return 2
		default:
			fmt.Fprintln(os.Stderr, "internal error:", err)
			return 1
		}
	}

	if *jsonOut != "" {
		b, err := json.MarshalIndent(payload, "", " ")
		if err == nil {
			err = os.WriteFile(*jsonOut, b, 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "could not write", *jsonOut, ":", err)
			return 1
		}
		if !*quiet {
			fmt.Println("Wrote", *jsonOut)
		}
	}

	blob, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintln(os.Stderr, "internal error:", err)
		return 1
	}

	dir := *outDir
	if dir == "" {
		dir = defaultReportDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "cannot create report folder", dir, ":", err)
		return 1
	}
	htmlPath := filepath.Join(dir, "burnmon-report-"+time.Now().Format("20060102-150405")+".html")
	if err := report.Write(htmlPath, blob); err != nil {
		fmt.Fprintln(os.Stderr, "could not write report:", err)
		return 1
	}

	printSummary(&cfg, payload, htmlPath, *quiet)

	if !*noOpen {
		openBrowser(htmlPath)
	}
	return 0
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// loadConfig prefers burnmon.json, exe-adjacent then cwd, falling back to
// claudecost.json (the pre-rename config name, v0.1.2 and earlier) in the
// same two locations so an existing portable folder keeps working.
func loadConfig(explicit string, quiet bool) (pricing.Config, error) {
	path := explicit
	if path == "" {
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			if cand := filepath.Join(exeDir, "burnmon.json"); fileExists(cand) {
				path = cand
			} else if cand := filepath.Join(exeDir, "claudecost.json"); fileExists(cand) {
				path = cand
			}
		}
	}
	if path == "" {
		if fileExists("burnmon.json") {
			path = "burnmon.json"
		} else if fileExists("claudecost.json") {
			path = "claudecost.json"
		}
	}
	cfg, err := pricing.Load(path)
	if err == nil && path != "" && !quiet {
		fmt.Println("Using config overrides from", path)
	}
	return cfg, err
}

func defaultReportDir() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "reports")
		if os.MkdirAll(dir, 0o755) == nil {
			probe := filepath.Join(dir, ".probe")
			if f, err := os.Create(probe); err == nil {
				f.Close()
				os.Remove(probe)
				return dir
			}
		}
	}
	if lad := os.Getenv("LOCALAPPDATA"); lad != "" {
		return filepath.Join(lad, "burnmon", "reports")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".burnmon", "reports")
}

func openBrowser(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	_ = cmd.Start()
}

// ---------------------------------------------------------------------------
// Terminal summary
// ---------------------------------------------------------------------------

func printSummary(cfg *pricing.Config, p dataset.Payload, htmlPath string, quiet bool) {
	if quiet {
		fmt.Println(htmlPath)
		return
	}

	eurPerUSD := cfg.FXUSDEUR
	if cfg.Subscription.MonthlySubscriptionUSD > 0 {
		eurPerUSD = cfg.Subscription.MonthlySubscriptionEUR / cfg.Subscription.MonthlySubscriptionUSD
	}

	t := p.Totals
	fmt.Println()
	fmt.Printf("  window    %s to %s\n", p.WindowFrom, p.WindowTo)
	fmt.Printf("  sessions  %d, API calls %d, tokens %.2fB\n", t.Sessions, t.Calls, float64(t.Tokens)/1e9)
	fmt.Printf("  subscription cost EUR %.2f (USD %.2f)   API list equivalent USD %.2f\n",
		t.CostSub*eurPerUSD, t.CostSub, t.Cost)
	fmt.Println()
	fmt.Println("  month      sub EUR    sub USD    list USD    calls   tokens  sessions")
	keys := make([]string, 0, len(p.Months))
	for k := range p.Months {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		m := p.Months[k]
		fmt.Printf("  %s  %9.2f  %9.2f  %10.2f  %7d  %6.2fB  %8d\n",
			k, m.CostSub*eurPerUSD, m.CostSub, m.Cost, m.Calls, float64(m.Tokens)/1e9, m.Sessions)
	}

	// The single most useful observation: cost share sitting in marathon sessions.
	var marathon float64
	for _, s := range p.Sessions {
		if s.Long {
			marathon += s.CostSub
		}
	}
	if t.CostSub > 0 {
		share := marathon / t.CostSub
		if share > 0.45 {
			fmt.Println()
			fmt.Printf("  Note: %.0f%% of your cost sits in sessions past %d calls. An agent session\n",
				share*100, p.LongSessionCalls)
			fmt.Println("  re-sends its whole context every turn, so per-call cost climbs steeply as a")
			fmt.Println("  session grows. Finishing a topic and starting a new session is the fix,")
			fmt.Println("  not switching to a cheaper model.")
		}
	}

	fmt.Println()
	fmt.Println("  Report:", htmlPath)
	fmt.Println("  These are allocated shares of a flat monthly invoice, not money owed.")
}
