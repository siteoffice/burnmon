// Package historyadmin holds the two requests both apps' Settings pages send:
// delete history by scope, and export to a chosen folder. The apps only
// decode the request and call here, so the CLI-equal export and the
// confirmation rule live in one place.
package historyadmin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"burnmon/internal/export"
	"burnmon/internal/pricing"
	"burnmon/internal/store"
)

// Delete scopes.
const (
	ScopeOlderThan = "older_than"
	ScopeSession   = "session"
	ScopeAll       = "all"
)

// DeleteRequest is what the UI sends. Days is read for older_than only,
// SessionID for session only.
type DeleteRequest struct {
	Scope     string `json:"scope"`
	Days      int    `json:"days"`
	SessionID string `json:"session_id"`
	Confirm   bool   `json:"confirm"`
}

// DeleteResult counts what a delete removed.
type DeleteResult struct {
	Events    int `json:"events"`
	ToolCalls int `json:"tool_calls"`
}

// ErrNotConfirmed is the refusal for a delete request without confirm: true.
var ErrNotConfirmed = errors.New("delete not confirmed")

// Delete removes history by req.Scope. Without Confirm it touches nothing.
// Cursors are never removed, so a delete is not undone by the next scan.
func Delete(st *store.Store, req DeleteRequest, now time.Time) (DeleteResult, error) {
	if !req.Confirm {
		return DeleteResult{}, ErrNotConfirmed
	}
	var res DeleteResult
	var err error
	switch req.Scope {
	case ScopeOlderThan:
		if req.Days < 1 {
			return res, errors.New("older_than needs days of at least 1")
		}
		res.Events, res.ToolCalls, err = st.DeleteEventsBefore(now.Add(-time.Duration(req.Days) * 24 * time.Hour))
	case ScopeSession:
		if req.SessionID == "" {
			return res, errors.New("session needs a session id")
		}
		res.Events, res.ToolCalls, err = st.DeleteSession(req.SessionID)
	case ScopeAll:
		res.Events, res.ToolCalls, err = st.DeleteAllEvents()
	default:
		return res, fmt.Errorf("unknown delete scope %q", req.Scope)
	}
	return res, err
}

// ExportRequest is what the UI sends. Since and Until are local calendar days
// (YYYY-MM-DD), both inclusive, empty meaning unbounded.
type ExportRequest struct {
	Dir    string   `json:"dir"`
	Since  string   `json:"since"`
	Until  string   `json:"until"`
	Owners []string `json:"owners"`
	Label  string   `json:"label"`
}

// ExportResult names the file written and how many rows it holds.
type ExportResult struct {
	Path string `json:"path"`
	Rows int    `json:"rows"`
}

// Export writes the same file `burnmon-cli export` writes, into req.Dir.
// The wall rule applies: with owner rules configured, an owner is required.
func Export(st *store.Store, cfg *pricing.Config, req ExportRequest, now time.Time) (ExportResult, error) {
	f := export.Filter{Since: req.Since, Until: req.Until, Owners: req.Owners}
	if err := export.Validate(cfg, f); err != nil {
		return ExportResult{}, err
	}
	events, err := st.AllEvents()
	if err != nil {
		return ExportResult{}, err
	}
	doc := export.Build(export.Select(events, f), cfg, req.Label, now)
	path, err := export.WriteToDir(req.Dir, doc, now)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: path, Rows: len(doc.Rows)}, nil
}

// DefaultExportDir is the folder the export field is prefilled with: the
// user's Downloads folder, "" when the home folder is unknown.
func DefaultExportDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Downloads")
}
