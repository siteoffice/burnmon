package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/schema"
)

func testConfig(t *testing.T) pricing.Config {
	t.Helper()
	cfg, err := pricing.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestBuild_DefaultLabel(t *testing.T) {
	cfg := testConfig(t)
	doc := Build(nil, &cfg, "", time.Now())
	if doc.Label != DefaultLabel {
		t.Fatalf("Label = %q, want %q", doc.Label, DefaultLabel)
	}
	if doc.Schema != Schema {
		t.Fatalf("Schema = %d, want %d", doc.Schema, Schema)
	}
}

func TestBuild_GroupsByDayOwnerClientVendorModel(t *testing.T) {
	cfg := testConfig(t)
	day1 := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	events := []schema.Event{
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r1", At: day1, Model: "claude-sonnet-5",
			Owner: "ZND", Client: "burnmon", Input: 100, Output: 10},
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r2", At: day1.Add(2 * time.Minute), Model: "claude-sonnet-5",
			Owner: "ZND", Client: "burnmon", Input: 50, Output: 5},
		{Vendor: "anthropic", SessionID: "s2", RequestID: "r3", At: day2, Model: "claude-sonnet-5",
			Owner: "ZND", Client: "burnmon", Input: 200, Output: 20},
		{Vendor: "openai", SessionID: "s3", RequestID: "r4", At: day1, Model: "gpt-5.6-terra",
			Owner: "ClientA", Client: "unassigned", Input: 300, Output: 30},
	}
	doc := Build(events, &cfg, "dev-1", time.Now())
	if len(doc.Rows) != 3 {
		t.Fatalf("got %d rows, want 3 (two events share day/owner/client/vendor/model)", len(doc.Rows))
	}
	var found bool
	for _, r := range doc.Rows {
		if r.Day == "2026-09-20" && r.Owner == "ZND" && r.Client == "burnmon" && r.Vendor == "anthropic" {
			found = true
			if r.Input != 150 || r.Output != 15 {
				t.Fatalf("merged row Input/Output = %d/%d, want 150/15", r.Input, r.Output)
			}
			if r.ActiveMinutes != 2 {
				t.Fatalf("ActiveMinutes = %v, want 2 (a 2-minute gap under the default cutoff)", r.ActiveMinutes)
			}
		}
	}
	if !found {
		t.Fatal("did not find the merged 2026-09-20/ZND/burnmon/anthropic row")
	}
}

func TestBuild_SyntheticToolOnlyEventsExcluded(t *testing.T) {
	cfg := testConfig(t)
	events := []schema.Event{
		{Vendor: "anthropic", SessionID: "s1", RequestID: "tool-1", At: time.Now(), Model: "", Input: 0, Output: 0},
	}
	doc := Build(events, &cfg, "dev-1", time.Now())
	if len(doc.Rows) != 0 {
		t.Fatalf("got %d rows, want 0 (synthetic tool-only event must not produce a row)", len(doc.Rows))
	}
}

// TestExportedJSON_NoPathsOrSessionIDs is the leak scanner the session
// prompt asked for: it builds a document from events carrying a realistic
// Windows path and a UUID-shaped session id in fields Row never exposes
// (Project, SessionID, RequestID, Title all live on schema.Event, not on
// export.Row), marshals the document the same way the CLI will, and fails if
// either a path separator or a UUID-shaped session id pattern survives into
// the bytes actually written to disk.
func TestExportedJSON_NoPathsOrSessionIDs(t *testing.T) {
	cfg := testConfig(t)
	events := []schema.Event{
		{
			Vendor: "anthropic", SessionID: "4b1f2e3a-9c1d-4a2b-8e3f-1a2b3c4d5e6f",
			RequestID: "req-1", At: time.Now(), Model: "claude-sonnet-5",
			Project: `C:\ZND\projects\burnmon`, Title: "fix the export bug",
			Owner: "ZND", Client: "burnmon", Input: 100, Output: 10,
		},
	}
	doc := Build(events, &cfg, "dev-1", time.Now())
	b, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)

	pathSep := regexp.MustCompile(`[\\/]`)
	if pathSep.MatchString(out) {
		t.Fatalf("export JSON contains a path separator:\n%s", out)
	}
	uuid := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	if uuid.MatchString(out) {
		t.Fatalf("export JSON contains a UUID-shaped session id:\n%s", out)
	}
}

func TestPriceBookDates(t *testing.T) {
	cfg := testConfig(t)
	dates := PriceBookDates(&cfg)
	for _, key := range []string{"anthropic_api", "openai_api", "copilot_credits"} {
		if dates[key] == "" {
			t.Fatalf("PriceBookDates()[%q] is empty, want the compiled-in book date", key)
		}
	}
}

func selectEvents() []schema.Event {
	at := func(day int) time.Time { return time.Date(2026, 10, day, 12, 0, 0, 0, time.Local) }
	return []schema.Event{
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r1", At: at(18), Model: "m", Owner: "wilco", Input: 1, Output: 1},
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r2", At: at(19), Model: "m", Owner: "other", Input: 1, Output: 1},
		{Vendor: "anthropic", SessionID: "s1", RequestID: "r3", At: at(20), Model: "m", Owner: "wilco", Input: 1, Output: 1},
	}
}

func TestSelectMatchesCLIRules(t *testing.T) {
	events := selectEvents()
	if got := Select(events, Filter{}); len(got) != 3 {
		t.Fatalf("empty filter kept %d events, want 3", len(got))
	}
	// Since and until are both inclusive local days.
	got := Select(events, Filter{Since: "2026-10-19", Until: "2026-10-19"})
	if len(got) != 1 || got[0].RequestID != "r2" {
		t.Fatalf("since=until=19th kept %+v, want only r2", got)
	}
	got = Select(events, Filter{Since: "2026-10-19"})
	if len(got) != 2 {
		t.Fatalf("since 19th kept %d events, want 2", len(got))
	}
	got = Select(events, Filter{Until: "2026-10-18"})
	if len(got) != 1 || got[0].RequestID != "r1" {
		t.Fatalf("until 18th kept %+v, want only r1", got)
	}
	got = Select(events, Filter{Owners: []string{"wilco"}})
	if len(got) != 2 {
		t.Fatalf("owner wilco kept %d events, want 2", len(got))
	}
}

func TestValidateRequiresOwnerWithRules(t *testing.T) {
	cfg := testConfig(t)
	if err := Validate(&cfg, Filter{}); err != nil {
		t.Fatalf("no owner rules: Validate = %v, want nil", err)
	}
	cfg.Owners = []pricing.OwnerRule{{Match: `C:\x\*`, Owner: "wilco"}}
	if err := Validate(&cfg, Filter{}); !errors.Is(err, ErrOwnerRequired) {
		t.Fatalf("owner rules, no owner: Validate = %v, want ErrOwnerRequired", err)
	}
	if err := Validate(&cfg, Filter{Owners: []string{"wilco"}}); err != nil {
		t.Fatalf("owner rules, owner given: Validate = %v, want nil", err)
	}
}

func TestEncodeIsCLIIndent(t *testing.T) {
	cfg := testConfig(t)
	doc := Build(selectEvents(), &cfg, "dev-1", time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	want, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode bytes differ from json.MarshalIndent(doc, \"\", \" \")")
	}
}

func TestWriteToDirRefusesMissingFolder(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	if _, err := WriteToDir(filepath.Join(dir, "nope"), Doc{Label: "dev-1"}, now); err == nil {
		t.Fatal("missing folder: err = nil, want one")
	}
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteToDir(file, Doc{Label: "dev-1"}, now); err == nil {
		t.Fatal("a file instead of a folder: err = nil, want one")
	}
}

func TestWriteToDirWritesNamedFile(t *testing.T) {
	cfg := testConfig(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	doc := Build(selectEvents(), &cfg, "dev-1", now)
	dir := t.TempDir()
	path, err := WriteToDir(dir, doc, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, FileName("dev-1", now)); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Encode(doc)
	if !bytes.Equal(got, want) {
		t.Fatal("file content differs from Encode(doc)")
	}
}