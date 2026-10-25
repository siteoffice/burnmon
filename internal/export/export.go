// Package export builds K4's export file: one JSON document, aggregated to
// day/owner/client/vendor/model rows, with nothing that leaves a path,
// session id, prompt or project name on the machine it was built on (the
// wall rule, v0.2 spec 2.2 P6: "ClientA usage numbers are Wilco's to read on
// his own laptop").
package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"burnmon/internal/dataset"
	"burnmon/internal/pricing"
	"burnmon/internal/schema"
)

// Schema is the export file format version. Merge (K5) refuses to combine
// files whose Schema differs.
const Schema = 1

// Row is one day/owner/client/vendor/model bucket: tokens by class, cost on
// every basis that bucket's vendor book covers, and active time computed
// directly off the bucket's own events (K2's algorithm, not a whole-session
// figure, since one session's events can span more than one row here).
type Row struct {
	Day    string `json:"day"` // YYYY-MM-DD, local calendar day (Wilco's decision, 2026-09-26: local time everywhere)
	Owner  string `json:"owner,omitempty"`
	Client string `json:"client,omitempty"`
	Vendor string `json:"vendor"`
	Model  string `json:"model"`

	Input      int64 `json:"input"`
	CacheWrite int64 `json:"cache_write"`
	CacheRead  int64 `json:"cache_read"`
	Output     int64 `json:"output"`
	Reasoning  int64 `json:"reasoning"`

	Costs []pricing.BasisCost `json:"costs,omitempty"`

	ActiveMinutes float64 `json:"active_minutes"`
}

// Doc is the whole export file (K4): schema, the label this laptop chose for
// itself, when it was built, the price book dates it priced against, and the
// rows above. No field here, or on Row, carries a path, a session id, a
// prompt or a project name.
type Doc struct {
	Schema         int               `json:"schema"`
	Label          string            `json:"label"`
	ExportedAt     time.Time         `json:"exported_at"`
	PriceBookDates map[string]string `json:"price_book_dates"`
	Rows           []Row             `json:"rows"`
}

// DefaultLabel is K4's default export label: "dev-1", never the hostname
// (spec: "The label defaults to dev-1 and never reads the hostname").
const DefaultLabel = "dev-1"

// PriceBookDates collects every book date cfg carries, keyed by book name,
// for Doc.PriceBookDates.
func PriceBookDates(cfg *pricing.Config) map[string]string {
	dates := map[string]string{
		"anthropic_api":   cfg.AnthropicBook.Date,
		"openai_api":      cfg.OpenAIBook.Date,
		"copilot_credits": cfg.CopilotCredits.Date,
	}
	if cfg.SubscriptionConfigured() {
		dates["subscription"] = cfg.Subscription.CalibratedOn
	}
	return dates
}

// isTurn reports whether e is a real call rather than the claude adapter's
// synthetic tool-only event (model and every token count zero), the same
// filter internal/history uses before aggregating.
func isTurn(e schema.Event) bool {
	return !(e.Model == "" && e.Input == 0 && e.Output == 0)
}

type rowKey struct {
	day, owner, client, vendor, model string
}

// Build groups events into K4's rows: day (local calendar day), owner, client,
// vendor, model. Only real turns are counted (isTurn), matching every other
// aggregation in this codebase. label and exportedAt are carried through to
// Doc unchanged; cfg supplies both the cost function (C2) and the active-time
// idle cutoff (K2).
func Build(events []schema.Event, cfg *pricing.Config, label string, exportedAt time.Time) Doc {
	if label == "" {
		label = DefaultLabel
	}

	groups := map[rowKey][]schema.Event{}
	var order []rowKey
	for _, e := range events {
		if !isTurn(e) {
			continue
		}
		k := rowKey{
			day:    e.At.In(time.Local).Format("2006-01-02"),
			owner:  e.Owner,
			client: e.Client,
			vendor: e.Vendor,
			model:  e.Model,
		}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], e)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.day != b.day {
			return a.day < b.day
		}
		if a.owner != b.owner {
			return a.owner < b.owner
		}
		if a.client != b.client {
			return a.client < b.client
		}
		if a.vendor != b.vendor {
			return a.vendor < b.vendor
		}
		return a.model < b.model
	})

	rows := make([]Row, 0, len(order))
	for _, k := range order {
		group := groups[k]
		row := Row{Day: k.day, Owner: k.owner, Client: k.client, Vendor: k.vendor, Model: k.model}
		for _, e := range group {
			row.Input += e.Input
			if e.CacheWrite != nil {
				row.CacheWrite += *e.CacheWrite
			}
			if e.CacheRead != nil {
				row.CacheRead += *e.CacheRead
			}
			row.Output += e.Output
			if e.Reasoning != nil {
				row.Reasoning += *e.Reasoning
			}
		}
		for _, vc := range cfg.CostForEvents(group) {
			row.Costs = append(row.Costs, vc.Bases...)
		}
		row.ActiveMinutes = dataset.ActiveTimeMinutes(group, cfg)
		rows = append(rows, row)
	}

	return Doc{
		Schema:         Schema,
		Label:          label,
		ExportedAt:     exportedAt,
		PriceBookDates: PriceBookDates(cfg),
		Rows:           rows,
	}
}

// Filter picks which events an export covers. Since and Until are local
// calendar days (YYYY-MM-DD), both inclusive, empty meaning unbounded; Owners
// keeps only those owners, empty meaning every owner.
type Filter struct {
	Since, Until string
	Owners       []string
}

// Select returns the events f lets through, in their original order: the same
// rules the CLI's export loop has always applied.
func Select(events []schema.Event, f Filter) []schema.Event {
	ownerSet := map[string]bool{}
	for _, o := range f.Owners {
		ownerSet[o] = true
	}
	var out []schema.Event
	for _, e := range events {
		day := e.At.In(time.Local).Format("2006-01-02")
		if f.Since != "" && day < f.Since {
			continue
		}
		if f.Until != "" && day > f.Until {
			continue
		}
		if len(ownerSet) > 0 && !ownerSet[e.Owner] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ErrOwnerRequired is the wall rule's refusal (P6): with owner rules
// configured, an export without an owner never starts.
var ErrOwnerRequired = errors.New("owner rules are configured; at least one owner is required, so a ClientA row never leaves by accident")

// Validate refuses a filter the wall rule forbids: owner rules exist in cfg
// and f names no owner.
func Validate(cfg *pricing.Config, f Filter) error {
	if len(cfg.Owners) > 0 && len(f.Owners) == 0 {
		return ErrOwnerRequired
	}
	return nil
}

// Encode renders doc as the export file's bytes: indented by one space, no
// trailing newline, exactly what `burnmon-cli export` writes.
func Encode(doc Doc) ([]byte, error) {
	return json.MarshalIndent(doc, "", " ")
}

// FileName is the name an app gives an export it saves into a folder.
func FileName(label string, now time.Time) string {
	return "burnmon-export-" + label + "-" + now.Local().Format("20060102-150405") + ".json"
}

// WriteToDir saves doc as FileName(doc.Label, now) inside dir and returns the
// full path. dir must already exist: nothing here creates folders.
func WriteToDir(dir string, doc Doc, now time.Time) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a folder", dir)
	}
	b, err := Encode(doc)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, FileName(doc.Label, now))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
