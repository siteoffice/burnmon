package graph

import (
	"encoding/json"
	"strings"
	"time"

	"burnmon/internal/store"
)

// rebuildable reports whether the events table gives this edge back on its
// own, so it need not be stored: a harness on the device, a session in its
// harness, a subagent's parent.
func rebuildable(e Edge) bool {
	switch e.Kind {
	case KindRunsOn, KindSpawned:
		return true
	case KindRunsIn:
		return strings.HasPrefix(e.From, "session:") && strings.HasPrefix(e.To, "harness:")
	}
	return false
}

// Recorded is the part of g that only a live tick can see, in the store's
// shape: every other edge, and the nodes at both ends of those edges.
func (g Graph) Recorded() ([]store.GraphNode, []store.GraphEdge) {
	ids := map[string]bool{}
	edges := []store.GraphEdge{}
	for _, e := range g.Edges {
		if rebuildable(e) {
			continue
		}
		edges = append(edges, store.GraphEdge{From: e.From, To: e.To, Kind: e.Kind, Basis: e.Basis, Source: e.Source})
		ids[e.From], ids[e.To] = true, true
	}
	nodes := []store.GraphNode{}
	for _, n := range g.Nodes {
		if !ids[n.ID] {
			continue
		}
		attrs := "{}"
		if len(n.Attrs) > 0 {
			if b, err := json.Marshal(n.Attrs); err == nil {
				attrs = string(b)
			}
		}
		nodes = append(nodes, store.GraphNode{ID: n.ID, Kind: n.Kind, Label: n.Label, Vendor: n.Vendor, Attrs: attrs, Source: n.Source})
	}
	return nodes, edges
}

// HistoryInput is the window and the non-store facts History joins in.
type HistoryInput struct {
	From, To, Now time.Time
	RepoFor       func(cwd string) (Repo, bool)
	Sprints       []Sprint
	SubagentFor   func(parent, id string) (SubagentInfo, bool)
	Flows         []Flow
	MergedFor     func(repo, branch string) (merged, known bool)
}

// History is the graph for a past window: sessions and subagents from the
// events table (so it survives deleted transcripts), joined with Foreman
// sprints and repos as Build does, plus every live-only edge the store
// recorded in the window. Ages count from the last time a thing was seen.
func History(st *store.Store, hi HistoryInput) (Graph, error) {
	hs, err := st.SessionsBetween(hi.From, hi.To)
	if err != nil {
		return Graph{}, err
	}
	sessions := make([]Session, 0, len(hs))
	for _, h := range hs {
		sessions = append(sessions, Session{Vendor: h.Vendor, Agent: h.Agent, ID: h.SessionID, ParentID: h.ParentID,
			Project: h.Project, FirstActivity: h.First, LastActivity: h.Last})
	}
	g := Build(Input{Now: hi.Now, Sessions: sessions, RepoFor: hi.RepoFor, Sprints: hi.Sprints, Flows: hi.Flows,
		SubagentFor: hi.SubagentFor, MergedFor: hi.MergedFor})

	nodes, edges, err := st.GraphBetween(hi.From, hi.To)
	if err != nil {
		return Graph{}, err
	}
	b := newBuilder(Input{Now: hi.Now})
	b.g = g
	for i, n := range g.Nodes {
		b.nodes[n.ID] = i
	}
	for i, e := range g.Edges {
		b.edges[e.Key()] = i
	}
	for _, n := range nodes {
		var attrs map[string]string
		_ = json.Unmarshal([]byte(n.Attrs), &attrs)
		b.node(Node{ID: n.ID, Kind: n.Kind, Label: n.Label, Vendor: n.Vendor, Attrs: attrs, Source: n.Source, AgeSeconds: b.age(n.LastSeen)})
	}
	for _, e := range edges {
		b.edge(Edge{From: e.From, To: e.To, Kind: e.Kind, Basis: e.Basis, Source: e.Source, AgeSeconds: b.age(e.LastSeen),
			FirstSeen: utc(e.FirstSeen), LastSeen: utc(e.LastSeen)})
	}
	return b.g, nil
}
