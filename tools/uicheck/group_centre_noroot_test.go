package main

import "testing"

func TestCentreNoRootProblems(t *testing.T) {
	good := centreNoRootRead{Lead: "Settings. No folder is chosen yet.", LeadW: 700, LeadFull: 260,
		RootHelp: "Pick the folder to show with Browse, or type it, then Save.", InnerW: 1024}
	if p := centreNoRootProblems(good); len(p) != 0 {
		t.Fatalf("a good read has problems: %v", p)
	}
	// What the Mac check saw: the long lead, wider than its box.
	cut := good
	cut.Lead = "Settings. No folder is chosen yet: pick the folder to show with Browse, or type it, then Save."
	cut.LeadFull = 820
	if p := centreNoRootProblems(cut); len(p) == 0 {
		t.Error("a cut lead was not reported")
	}
	for name, mut := range map[string]func(*centreNoRootRead){
		"wrong lead":       func(r *centreNoRootRead) { r.Lead = "Settings." },
		"no how-to":        func(r *centreNoRootRead) { r.RootHelp = "The folder every page reads." },
		"lead one px over": func(r *centreNoRootRead) { r.LeadFull = r.LeadW + 5 },
	} {
		r := good
		mut(&r)
		if p := centreNoRootProblems(r); len(p) == 0 {
			t.Errorf("%s was not reported", name)
		}
	}
}

func TestCentreNoRootIsACentreCheck(t *testing.T) {
	if checks["centre/noroot"] == nil || !isCentreCheck("centre/noroot") {
		t.Fatal("centre/noroot is not registered as a Control Centre check")
	}
	if isDevCheck("centre/noroot") || isLampCheck("centre/noroot") {
		t.Fatal("centre/noroot is counted as another exe's check")
	}
}
