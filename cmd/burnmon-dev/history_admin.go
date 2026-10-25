//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"time"

	"burnmon/internal/historyadmin"
	"burnmon/internal/store"
)

// History delete and export for the gear dialog (B1), the dev app's twin of
// cmd\burnmon\history_admin.go. The rules themselves live in
// internal/historyadmin; the header EXPORT button (bdevExport, the AI
// bundle) is a different export and stays as it is.

// deleteHistory removes history by scope. The Go side refuses without
// confirm: true (historyadmin.ErrNotConfirmed), whatever the page sent. On
// success the closed-day heatmap cache is dropped, so deleted days leave the
// heatmap now and not at the next UTC midnight.
func (a *app) deleteHistory(st *store.Store, req historyadmin.DeleteRequest) (historyadmin.DeleteResult, error) {
	res, err := historyadmin.Delete(st, req, time.Now())
	if err != nil {
		return res, err
	}
	a.heatmapMu.Lock()
	a.heatmapClosed = nil
	a.heatmapAsOf = ""
	a.heatmapMu.Unlock()
	return res, nil
}

// exportHistory writes the same file burnmon-cli export writes into
// req.Dir. Only an absolute folder is accepted from the app: a relative one
// would resolve against whatever the process working directory is.
func (a *app) exportHistory(st *store.Store, req historyadmin.ExportRequest, now time.Time) (historyadmin.ExportResult, error) {
	if !filepath.IsAbs(req.Dir) {
		return historyadmin.ExportResult{}, fmt.Errorf("export folder must be an absolute path (for example %s), got %q",
			historyadmin.DefaultExportDir(), req.Dir)
	}
	cfg := a.cfgSnapshot()
	return historyadmin.Export(st, &cfg, req, now)
}

// historyExportOutcome is what bdevExportHistory resolves the page with: a
// result or an error text, since the binding returns before the export runs.
type historyExportOutcome struct {
	Path  string `json:"path"`
	Rows  int    `json:"rows"`
	Error string `json:"error,omitempty"`
}

func (a *app) historyExportOutcome(st *store.Store, req historyadmin.ExportRequest) historyExportOutcome {
	res, err := a.exportHistory(st, req, time.Now())
	if err != nil {
		return historyExportOutcome{Error: err.Error()}
	}
	return historyExportOutcome{Path: res.Path, Rows: res.Rows}
}

// historyInfoPayload is bdevHistoryInfo's return shape: what the History
// block shows when the gear dialog opens.
type historyInfoPayload struct {
	KeepHistory bool     `json:"keep_history"`
	ExportDir   string   `json:"export_dir"`
	Owners      []string `json:"owners"`
}

// historyInfo lists cfg.Owners' owner names, first appearance order, each
// once: the export form's checkboxes.
func (a *app) historyInfo() historyInfoPayload {
	cfg := a.cfgSnapshot()
	out := historyInfoPayload{KeepHistory: cfg.KeepHistory, ExportDir: historyadmin.DefaultExportDir(), Owners: []string{}}
	seen := map[string]bool{}
	for _, r := range cfg.Owners {
		if r.Owner != "" && !seen[r.Owner] {
			seen[r.Owner] = true
			out.Owners = append(out.Owners, r.Owner)
		}
	}
	return out
}
