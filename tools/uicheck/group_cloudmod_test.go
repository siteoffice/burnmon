package main

import "testing"

// cloudModLineProblems checks what d53 reads from the line under the strip:
// the text it must show (empty means the line is hidden).
func TestCloudModLineProblems(t *testing.T) {
	want := cloudPartialTip + " | cloud mod: no delivery yet"
	if p := cloudModLineProblems(cloudPartialRead{NoteText: want, NoteTitle: want}, want); len(p) != 0 {
		t.Fatalf("a good line reports %v", p)
	}
	if p := cloudModLineProblems(cloudPartialRead{NoteText: cloudPartialTip, NoteTitle: cloudPartialTip}, want); len(p) == 0 {
		t.Fatal("a line without the mod status passes")
	}
	if p := cloudModLineProblems(cloudPartialRead{NoteText: want, NoteTitle: ""}, want); len(p) == 0 {
		t.Fatal("a line without its hover title passes")
	}
	if p := cloudModLineProblems(cloudPartialRead{NoteHidden: true}, ""); len(p) != 0 {
		t.Fatalf("a hidden line where none is wanted reports %v", p)
	}
	if p := cloudModLineProblems(cloudPartialRead{NoteText: "cloud mod: no delivery yet"}, ""); len(p) == 0 {
		t.Fatal("no delivery yet on its own passes")
	}
}
