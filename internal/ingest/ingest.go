// Package ingest holds the background pollers and the snapshot writer wiring
// that burnmon.exe and burnmon-dev.exe both run. Each app passes a func that
// returns a copy of its pricing config taken under its own lock, so this
// package holds no lock and knows nothing of either app struct.
package ingest

import (
	"log"
	"os"
	"os/exec"
	"time"

	"burnmon/internal/adapter/copilotcli"
	"burnmon/internal/adapter/copilotvsc"
	"burnmon/internal/adapter/hermes"
	"burnmon/internal/cloudpull"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
	"burnmon/internal/snapshotwriter"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

// pollEvery is how often each poller runs.
const pollEvery = 5 * time.Second

// applyPolled stamps one poll's events with their owner (and, for the VS Code
// poll, their client) and upserts them. The config copy is taken only when
// there is something to store, so an idle poll never touches the app's lock.
// name is the log prefix, for example "hermes poll".
func applyPolled(name string, events []schema.Event, err error, st *store.Store, cfgFn func() pricing.Config, withClient bool) {
	if err != nil {
		log.Println(name+":", err)
		return
	}
	if len(events) == 0 {
		return
	}
	cfg := cfgFn()
	for i := range events {
		events[i].Owner = cfg.OwnerFor(events[i].Project)
		if withClient {
			events[i].Client = cfg.ClientFor(events[i].Project)
		}
	}
	if err := st.UpsertEvents(events); err != nil {
		log.Println(name+": upsert:", err)
	}
}

// StartHermesPoll starts a 5-second Hermes poll, a no-op if no Hermes install
// is found (DefaultDBPath returns ""). No fsnotify watch: a SQLite WAL file's
// own writes do not fit watch.Watcher's file-offset, .jsonl-only design (see
// internal/adapter/hermes's package doc). Hermes needs no cursor either:
// PollOnce returns every session's current running totals on every call, and
// UpsertEvents' own "largest output wins" upsert (RequestID fixed to the
// session id) already skips a no-op write when a session has not grown.
func StartHermesPoll(st *store.Store, cfgFn func() pricing.Config) {
	dbPath := hermes.DefaultDBPath()
	if dbPath == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(pollEvery)
		defer ticker.Stop()
		for range ticker.C {
			events, err := hermes.PollOnce(dbPath)
			applyPolled("hermes poll", events, err, st, cfgFn, false)
		}
	}()
}

// StartCopilotCLIPoll starts a 5-second Copilot CLI poll, a no-op if no
// Copilot CLI install is found (DefaultDBPath returns ""). Same shape as
// StartHermesPoll for the same reason: session-store.db is a SQLite WAL file,
// not a .jsonl trail; PollOnce needs no cursor, since it always returns every
// session's current latest-row totals and the store's own upsert skips a
// no-op write when a session has not grown.
func StartCopilotCLIPoll(st *store.Store, cfgFn func() pricing.Config) {
	dbPath := copilotcli.DefaultDBPath()
	if dbPath == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(pollEvery)
		defer ticker.Stop()
		for range ticker.C {
			events, err := copilotcli.PollOnce(dbPath)
			applyPolled("copilot cli poll", events, err, st, cfgFn, false)
		}
	}()
}

// StartCopilotVSCPoll starts a 5-second Copilot-in-VS-Code poll, a no-op when
// the config carries no copilot_vscode_otel_file. Same shape as the other two
// polls, except that copilotvsc.Tail reads only the lines appended since the
// last tick and the store's upsert (largest output per RequestID) makes the
// one full read after a start a no-op for stored rows. It re-reads the config
// each tick, so a changed file name in Settings takes effect, and it also
// stamps the client.
func StartCopilotVSCPoll(st *store.Store, cfgFn func() pricing.Config) {
	if cfgFn().CopilotVSCodeOtelFile == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(pollEvery)
		defer ticker.Stop()
		var tail copilotvsc.Tail
		for range ticker.C {
			cfg := cfgFn() // one copy per tick: the path and the stamping use the same config
			events, err := tail.Poll(cfg.CopilotVSCodeOtelFile)
			applyPolled("copilot vscode poll", events, err, st, func() pricing.Config { return cfg }, true)
		}
	}()
}

// NewSnapshotWriter wires the shared snapshot writer to one app: its pricing
// config, a process sampler of its own (used from the writer goroutine only,
// as sysmon requires) and, when the home folder resolves, the relation
// graph's own sources. loadedCfg is the config file loaded at start ("" for
// none), shown in the graph.
func NewSnapshotWriter(st *store.Store, path string, cfgFn func() pricing.Config, loadedCfg string) *snapshotwriter.Writer {
	cfg := cfgFn()
	ps := sysmon.NewProcessSampler()
	copilotVSC := cfg.CopilotVSCodeOtelFile != ""
	w := &snapshotwriter.Writer{
		Store:  st,
		Path:   path,
		Tick:   snapshotwriter.Tick,
		Config: cfgFn,
		Procs:  func() ([]sysmon.ProcessGroupSample, error) { return ps.Tick(copilotVSC) },
	}
	if home, err := os.UserHomeDir(); err == nil {
		w.Graph = snapshotwriter.NewGraphReaders(home, cfg.ForemanRoot)
		w.Graph.Rows = ps.Procs
		w.Graph.ConfigPath = loadedCfg
		w.Graph.IgnoreDir = cloudpull.PullDir() // the pull's own claude runs are not sessions
		if cfg.AgentView {
			w.Graph.EnableAgentViewWith(cfg.ClaudePath, exec.LookPath)
		}
	} else {
		log.Println("snapshot: no home folder, relation graph sources off:", err)
	}
	return w
}
