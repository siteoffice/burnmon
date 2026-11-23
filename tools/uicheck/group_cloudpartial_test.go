package main

import "testing"

func goodPartialRead() cloudPartialRead {
	return cloudPartialRead{
		Cells: map[string][]string{
			"Cowork (Cloud)": {"not measured", "not measured", "779.5M+"},
			"Total":          {"1.2M+", "2.4M+", "788.5M+"},
		},
		NoteText: cloudPartialTip, NoteTitle: cloudPartialTip,
	}
}

func TestCloudPartialProblems(t *testing.T) {
	if p := cloudPartialProblems(goodPartialRead(), true, cloudPartialTip); len(p) != 0 {
		t.Fatalf("a good partial read has problems: %v", p)
	}
	for name, mut := range map[string]func(*cloudPartialRead){
		"today shows 0+":     func(r *cloudPartialRead) { r.Cells["Cowork (Cloud)"][0] = "0+" },
		"week shows a plain": func(r *cloudPartialRead) { r.Cells["Cowork (Cloud)"][1] = "0" },
		"month lost its +":   func(r *cloudPartialRead) { r.Cells["Cowork (Cloud)"][2] = "779.5M" },
		"total lost its +":   func(r *cloudPartialRead) { r.Cells["Total"][2] = "788.5M" },
		"line hidden":        func(r *cloudPartialRead) { r.NoteHidden = true },
		"line empty":         func(r *cloudPartialRead) { r.NoteText = "" },
		"no hover title":     func(r *cloudPartialRead) { r.NoteTitle = "" },
		"wrong reason":       func(r *cloudPartialRead) { r.NoteText = "x" },
	} {
		r := goodPartialRead()
		mut(&r)
		if p := cloudPartialProblems(r, true, cloudPartialTip); len(p) == 0 {
			t.Errorf("%s was not reported", name)
		}
	}
	// With a good pull the line is hidden and empty.
	if p := cloudPartialProblems(cloudPartialRead{NoteHidden: true}, false, ""); len(p) != 0 {
		t.Errorf("a hidden line with a good pull was reported: %v", p)
	}
	if p := cloudPartialProblems(goodPartialRead(), false, ""); len(p) == 0 {
		t.Error("a visible line with a good pull was not reported")
	}
}
