package agentmap

import (
	"strings"
	"testing"

	"burnmon/internal/graph"
	"burnmon/internal/waits"
)

// One folder is framed too, with its subagent and MCP server inside; two
// folders still give two frames.
func TestSingleFolderIsFramed(t *testing.T) {
	nodes := []graph.Node{
		sess("s:a", "example-app", "coding"), sub("s:a-sub", "Explore", "coding"),
		{ID: "mcp:example", Kind: graph.KindMCP, Label: "example", Source: "mcp_config"},
	}
	edges := []graph.Edge{edge("s:a", "s:a-sub", graph.KindSpawned), edge("s:a", "mcp:example", graph.KindUses)}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges})
	if len(l.Frames) != 1 || l.Frames[0].Kind != "project" || l.Frames[0].Label != "example-app" {
		t.Fatalf("frames %+v, want one project frame example-app", l.Frames)
	}
	fid := l.Frames[0].ID
	for _, id := range []string{"s:a", "s:a-sub"} {
		if b, ok := boxByID(l, id); !ok || b.Group != fid {
			t.Errorf("box %s group %q (found %v), want %q", id, b.Group, ok, fid)
		}
	}
	inside := 0
	for _, b := range l.Boxes {
		if b.Column == 3 && b.Group == fid {
			inside++
		}
	}
	if inside != 1 {
		t.Errorf("%d MCP boxes inside the frame, want 1", inside)
	}
	two := BuildLayout(LayoutInput{Nodes: []graph.Node{sess("s:a", "example-a", "coding"), sess("s:b", "example-b", "coding")}})
	if len(two.Frames) != 2 {
		t.Errorf("two folders: %d frames, want 2", len(two.Frames))
	}
}

// A Cowork cloud session has no folder: its frame names it by the short
// session id, never the full uuid.
func TestCoworkCloudFrameLabel(t *testing.T) {
	const full = "0a1b2c3d-1111-2222-3333-444455556666"
	n := sess("session:cowork:"+full, "", "coding")
	n.Attrs["agent"] = "cowork-cloud"
	l := BuildLayout(LayoutInput{Nodes: []graph.Node{n}})
	if len(l.Frames) != 1 || l.Frames[0].Label != "Cowork cloud session 0a1b2c3d" {
		t.Fatalf("frames %+v, want the label %q", l.Frames, "Cowork cloud session 0a1b2c3d")
	}
	if strings.Contains(l.Frames[0].Label, full) || strings.Contains(l.Frames[0].Label, "1111") {
		t.Errorf("label %q carries the full uuid", l.Frames[0].Label)
	}
}

// A blocked idle sprint is one box; a plain idle sprint is only listed.
func TestBlockedIdleShownOnce(t *testing.T) {
	nodes := []graph.Node{
		{ID: "person:you", Kind: graph.KindPerson, Label: "You"},
		{ID: "sprint:blocked", Kind: graph.KindSprint, Label: "example-blocked", Attrs: map[string]string{"status": "waiting-on-you"}, AgeSeconds: 5},
		{ID: "sprint:plain", Kind: graph.KindSprint, Label: "example-plain", Attrs: map[string]string{"status": "done"}, AgeSeconds: 9},
	}
	edges := []graph.Edge{edge("sprint:blocked", "person:you", graph.KindWaitsOn)}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Counts: &waits.Counts{Blocked: 1}})
	if b, ok := boxByID(l, "sprint:blocked"); !ok || !b.WaitsOnYou {
		t.Errorf("blocked idle sprint box %+v (found %v), want a box that waits on you", b, ok)
	}
	if _, ok := boxByID(l, "sprint:plain"); ok {
		t.Error("a plain idle sprint has a box")
	}
	if len(l.Idle) != 1 || l.Idle[0].ID != "sprint:plain" {
		t.Errorf("idle list %+v, want only sprint:plain", l.Idle)
	}
}

// A box outside any frame keeps a row gap from the frame above it.
func TestSprintBoxKeepsRowGapBelowFrame(t *testing.T) {
	nodes := []graph.Node{
		{ID: "person:you", Kind: graph.KindPerson, Label: "You"},
		sess("s:a", "example-app", "coding"),
		{ID: "sprint:blocked", Kind: graph.KindSprint, Label: "example-blocked", Attrs: map[string]string{"status": "waiting-on-you"}, AgeSeconds: 5},
	}
	edges := []graph.Edge{edge("sprint:blocked", "person:you", graph.KindWaitsOn)}
	l := BuildLayout(LayoutInput{Nodes: nodes, Edges: edges, Counts: &waits.Counts{Blocked: 1}})
	b, ok := boxByID(l, "sprint:blocked")
	if !ok || len(l.Frames) != 1 {
		t.Fatalf("box found %v, frames %d", ok, len(l.Frames))
	}
	if b.Row-l.Frames[0].LastRow < 2 {
		t.Errorf("box row %d, frame last row %d: want a gap of at least one row", b.Row, l.Frames[0].LastRow)
	}
}

// The idle sprint's project, spec and result links and its close-out flag
// reach the idle list for the map's table.
func TestIdleCarriesSprintLinks(t *testing.T) {
	nodes := []graph.Node{{ID: "sprint:x", Kind: graph.KindSprint, Label: "example-x", AgeSeconds: 3,
		Attrs: map[string]string{"status": "done", "project": "example-proj", "spec_path": `C:\example\spec.md`, "result_path": `C:\example\report.md`, "closeout": "true"}}}
	l := BuildLayout(LayoutInput{Nodes: nodes})
	if len(l.Idle) != 1 {
		t.Fatalf("idle %+v", l.Idle)
	}
	i := l.Idle[0]
	if i.Project != "example-proj" || i.SpecPath != `C:\example\spec.md` || i.ResultPath != `C:\example\report.md` || !i.CloseOut {
		t.Errorf("idle %+v lacks its links", i)
	}
}
