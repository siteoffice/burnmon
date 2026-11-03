package agentmap

import (
	"strings"
	"testing"

	"burnmon/internal/graph"
)

// OF23: with no foreman_root the none group reads "Sessions", not "No Foreman
// work". The default (flag off) keeps the old label, also for a root with no
// sprints.
func TestBuildForNoForemanLabelsNoneGroupSessions(t *testing.T) {
	nodes := []graph.Node{sess("s:a", "alpha", "coding")}
	v := BuildFor(nodes, nil, true)
	if len(v.Groups) != 1 || v.Groups[0].Kind != "none" || v.Groups[0].Label != "Sessions" {
		t.Fatalf("no foreman root: groups = %+v, want one none group labelled Sessions", v.Groups)
	}
	v = BuildFor(nodes, nil, false)
	if v.Groups[0].Label != NoGroupLabel {
		t.Errorf("with a root: label %q, want %q", v.Groups[0].Label, NoGroupLabel)
	}
	if v := Build(nodes, nil); v.Groups[0].Label != NoGroupLabel {
		t.Errorf("Build label %q, want %q", v.Groups[0].Label, NoGroupLabel)
	}
}

// OF23: the frame help sentences drop the Foreman clause with no root, in both
// wordings, and keep the rest of the sentence.
func TestLayoutNoForemanDropsForemanHelpSentences(t *testing.T) {
	nodes := []graph.Node{
		sess("s:a1", "alpha", "coding"), sub("s:orphan", "Explore", "coding"),
	}
	for _, plain := range []bool{false, true} {
		l := BuildLayout(LayoutInput{Nodes: nodes, Plain: plain, NoForeman: true})
		if len(l.Frames) != 2 {
			t.Fatalf("plain %v: frames %+v", plain, l.Frames)
		}
		for _, f := range l.Frames {
			if strings.Contains(f.Explain, "Foreman") || strings.Contains(f.Explain, "linked") {
				t.Errorf("plain %v: frame %q explain %q names Foreman", plain, f.Label, f.Explain)
			}
		}
	}
	l := BuildLayout(LayoutInput{Nodes: nodes, NoForeman: true})
	for _, f := range l.Frames {
		switch f.Label {
		case "alpha":
			if f.Explain != `Sessions in the folder "alpha".` {
				t.Errorf("alpha explain %q", f.Explain)
			}
		case "Other":
			if f.Explain != "Subagents whose parent is not in this window." {
				t.Errorf("Other explain %q", f.Explain)
			}
		}
	}
	p := BuildLayout(LayoutInput{Nodes: nodes, Plain: true, NoForeman: true})
	for _, f := range p.Frames {
		switch f.Label {
		case "alpha":
			if f.Explain != `Work in the folder "alpha".` {
				t.Errorf("plain alpha explain %q", f.Explain)
			}
		case "Other":
			if f.Explain != "Helpers whose main session is not in this window." {
				t.Errorf("plain Other explain %q", f.Explain)
			}
		}
	}
	// With a root (flag off) the wording is the old one.
	old := BuildLayout(LayoutInput{Nodes: nodes})
	for _, f := range old.Frames {
		if f.Label == "alpha" && f.Explain != `Sessions in the folder "alpha". No Foreman sprint or flow is linked.` {
			t.Errorf("flag off: alpha explain %q changed", f.Explain)
		}
	}
}
