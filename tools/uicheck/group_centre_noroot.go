package main

import (
	"fmt"
	"strings"
	"time"
)

// centre/noroot: controlcentre.exe started with no root and no saved settings
// opens on Settings (scripts\uicheck-centre.ps1 centre/noroot starts it that
// way). N7 of the Mac check 2026-11-22: the lead line there was one nowrap line
// cut at the pane edge ("Settings. No folder is chosen yet: pic..."). The lead
// must be drawn whole at 1024 wide, and the how-to must be in the Root row.
type centreNoRootRead struct {
	Lead     string  `json:"lead"`
	LeadW    float64 `json:"leadW"`
	LeadFull float64 `json:"leadFull"`
	RootHelp string  `json:"rootHelp"`
	InnerW   float64 `json:"innerW"`
}

func centreNoRootProblems(r centreNoRootRead) []string {
	var p []string
	if !strings.HasPrefix(r.Lead, "Settings. No folder is chosen yet") {
		p = append(p, fmt.Sprintf("the lead reads %q, want it to say Settings and that no folder is chosen", r.Lead))
	}
	if r.LeadFull > r.LeadW+1 {
		p = append(p, fmt.Sprintf("the lead is cut: its text is %.0f px wide in a %.0f px box (%q)", r.LeadFull, r.LeadW, r.Lead))
	}
	if !strings.Contains(r.RootHelp, "Browse") {
		p = append(p, fmt.Sprintf("the Root row help reads %q, want the how-to with Browse", r.RootHelp))
	}
	return p
}

const centreNoRootJS = `(function(){
  var lead = document.getElementById('lead');
  var hp = document.querySelector('.setrow .lb .hp');
  return {lead: lead ? lead.textContent : '', leadW: lead ? lead.clientWidth : 0, leadFull: lead ? lead.scrollWidth : 0,
    rootHelp: hp ? hp.textContent : '', innerW: window.innerWidth};
})()`

func caseCentreNoRoot(hwnd uintptr, args []string) error {
	// The port opens before the page has loaded; Settings is the first page with no root.
	var r centreNoRootRead
	var lastErr error
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		r = centreNoRootRead{}
		if lastErr = evalInto(centreNoRootJS, &r); lastErr == nil && r.Lead != "" && r.RootHelp != "" {
			break
		}
	}
	if r.Lead == "" || r.RootHelp == "" {
		return fmt.Errorf("centre/noroot: Settings never drew (last %+v, err %v)", r, lastErr)
	}
	if err := ensureWindowSizeWH(hwnd, 1024, 768); err != nil {
		return err
	}
	time.Sleep(400 * time.Millisecond)
	if err := evalInto(centreNoRootJS, &r); err != nil {
		return err
	}
	if _, err := screenshot(hwnd, "centre-noroot-1024"); err != nil {
		return err
	}
	fmt.Printf("uicheck: centre/noroot read: %+v\n", r)
	if bad := centreNoRootProblems(r); len(bad) > 0 {
		return fmt.Errorf("centre/noroot: %d problem(s):\n%s", len(bad), strings.Join(bad, "\n"))
	}
	fmt.Println("uicheck: centre/noroot ok: the lead is drawn whole at 1024 wide and the Root row carries the how-to")
	return nil
}

func init() {
	checks["centre/noroot"] = caseCentreNoRoot
	centreNames["centre/noroot"] = true
}
