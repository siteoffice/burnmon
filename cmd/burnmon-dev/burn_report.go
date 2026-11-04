//go:build windows

// burn_report.go is the Burn Report writer (OF15): the last 14 days as a zip
// holding one offline HTML page and bundle.json, redacted unless the page
// asks for names. The page (page.html) calls bdevBurnReport.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"burnmon/internal/devexport"
	"burnmon/internal/pricing"
	"burnmon/internal/store"
	"burnmon/internal/sysmon"
)

type burnReportRequest struct {
	Dir       string `json:"dir"`
	ShowNames bool   `json:"show_names"`
}

type burnReportOutcome struct {
	Path         string `json:"path"`
	DaysWithData int    `json:"days_with_data"`
	Redacted     bool   `json:"redacted"`
	Error        string `json:"error,omitempty"`
}

// writeBurnReport builds the 14 day bundle ending at now and writes
// <dir>/burn-report-YYYY-MM-DD.zip. A second press the same day replaces it.
func writeBurnReport(st *store.Store, sys *sysmon.Store, cfg *pricing.Config, req burnReportRequest, now time.Time, salt string) burnReportOutcome {
	if !filepath.IsAbs(req.Dir) {
		return burnReportOutcome{Error: fmt.Sprintf("report folder must be an absolute path, got %q", req.Dir)}
	}
	if err := os.MkdirAll(req.Dir, 0o755); err != nil {
		return burnReportOutcome{Error: err.Error()}
	}
	since, until := devexport.BurnReportWindow(now)
	b, err := buildExportBundle(st, sys, cfg, since, until)
	if err != nil {
		return burnReportOutcome{Error: err.Error()}
	}
	if !req.ShowNames {
		b = devexport.Redact(b, salt)
	}
	path := filepath.Join(req.Dir, "burn-report-"+now.Format("2006-01-02")+".zip")
	if err := devexport.WriteBurnReportZip(path, b); err != nil {
		return burnReportOutcome{Error: err.Error()}
	}
	return burnReportOutcome{Path: path, DaysWithData: devexport.DaysWithData(b), Redacted: !req.ShowNames}
}
