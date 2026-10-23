// Package agentmap is R7 of the agent control plan: it turns R6's nodes[] and
// edges[] into the shape the agent map draws, which is a list of groups
// (one per Foreman sprint or flow, plus one for work with no Foreman link),
// each holding session cards with their subagents nested under them. It is
// pure: it reads no file, opens no store and holds no page code. The same
// Build serves the live snapshot and a window of history, since both are a
// node list and an edge list.
//
// Only a short list of node attributes is copied (agent, state, agent type,
// model, phase, status), so nothing a future attribute holds can reach the page.
package agentmap

import (
	"sort"
	"strings"

	"burnmon/internal/graph"
)

// Counts is the one line the small overview shows.
type Counts struct {
	Sessions  int `json:"sessions"`
	Subagents int `json:"subagents"`
	Waiting   int `json:"waiting"`
	Sprints   int `json:"sprints"`
	Flows     int `json:"flows"`
}

// MCP is one MCP server a session uses and the ports it listens on.
type MCP struct {
	Name  string   `json:"name"`
	Ports []string `json:"ports"`
}

// Card is one session or subagent. Basis says how BurnMon knows the card
// belongs to its group (vendor or inferred); it is empty when it has no group.
type Card struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Vendor     string `json:"vendor,omitempty"`
	Agent      string `json:"agent,omitempty"`
	AgentType  string `json:"agent_type,omitempty"`
	Model      string `json:"model,omitempty"`
	State      string `json:"state,omitempty"`
	Subagent   bool   `json:"subagent"`
	WaitsOnYou bool   `json:"waits_on_you"`
	Basis      string `json:"basis,omitempty"`
	Repo       string `json:"repo,omitempty"`
	Worktree   string `json:"worktree,omitempty"`
	Branch     string `json:"branch,omitempty"`
	MCP        []MCP  `json:"mcp"`
	AgeSeconds int64  `json:"age_seconds"`
	Children   []Card `json:"children"`
}

// Group is one Foreman sprint or flow with the cards that work for it. Kind is
// sprint, flow or none.
type Group struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Status     string `json:"status,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Gate       string `json:"gate,omitempty"`
	WaitsOnYou bool   `json:"waits_on_you"`
	AgeSeconds int64  `json:"age_seconds"`
	Cards      []Card `json:"cards"`
}

// Idle is a sprint or flow in the graph that has no session working for it.
type Idle struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Status     string `json:"status,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Gate       string `json:"gate,omitempty"`
	WaitsOnYou bool   `json:"waits_on_you"`
	AgeSeconds int64  `json:"age_seconds"`
}

// View is what the page draws.
type View struct {
	Groups []Group  `json:"groups"`
	Idle   []Idle   `json:"idle"`
	Counts Counts   `json:"counts"`
	Notes  []string `json:"notes"`
}

// NoGroupLabel names the group for sessions with no Foreman work.
const NoGroupLabel = "No Foreman work"

type model struct {
	nodes    map[string]graph.Node
	out      map[string][]graph.Edge // by from
	spawned  map[string][]string     // parent to children, both session or subagent
	hasPar   map[string]bool
	gateOf   map[string]string // flow id to gate label
	waiters  map[string]bool   // ids that wait on a person, a gate counted as its flow
	visited  map[string]bool
	groupFor map[string]bool // sprint or flow ids that took a card
}

func isAgent(n graph.Node) bool { return n.Kind == graph.KindSession || n.Kind == graph.KindSubagent }

// Build turns a node list and an edge list into the view. Edges whose ends are
// not both in nodes are ignored, and so are repeats of the same edge.
func Build(nodes []graph.Node, edges []graph.Edge) View {
	m := &model{nodes: map[string]graph.Node{}, out: map[string][]graph.Edge{}, spawned: map[string][]string{},
		hasPar: map[string]bool{}, gateOf: map[string]string{}, waiters: map[string]bool{},
		visited: map[string]bool{}, groupFor: map[string]bool{}}
	for _, n := range nodes {
		if _, dup := m.nodes[n.ID]; !dup {
			m.nodes[n.ID] = n
		}
	}
	seenEdge := map[string]bool{}
	for _, e := range edges {
		if _, ok := m.nodes[e.From]; !ok {
			continue
		}
		if _, ok := m.nodes[e.To]; !ok {
			continue
		}
		if seenEdge[e.Key()] {
			continue
		}
		seenEdge[e.Key()] = true
		m.out[e.From] = append(m.out[e.From], e)
	}
	for _, from := range sortedKeys(m.out) {
		for _, e := range m.out[from] {
			switch e.Kind {
			case graph.KindSpawned:
				if isAgent(m.nodes[e.From]) && isAgent(m.nodes[e.To]) && !m.hasPar[e.To] {
					m.spawned[e.From] = append(m.spawned[e.From], e.To)
					m.hasPar[e.To] = true
				}
			case graph.KindPartOf:
				if m.nodes[e.From].Kind == graph.KindGate && m.nodes[e.To].Kind == graph.KindFlow {
					m.gateOf[e.To] = m.nodes[e.From].Label
				}
			}
		}
	}
	for _, from := range sortedKeys(m.out) {
		for _, e := range m.out[from] {
			if e.Kind != graph.KindWaitsOn || m.nodes[e.To].Kind != graph.KindPerson {
				continue
			}
			id := e.From
			if m.nodes[id].Kind == graph.KindGate {
				for _, p := range m.out[id] {
					if p.Kind == graph.KindPartOf {
						id = p.To
					}
				}
			}
			m.waiters[id] = true
		}
	}

	v := View{Groups: []Group{}, Idle: []Idle{}, Notes: []string{
		"Not shown: messages between agents. No tool's local files record who sent what to whom.",
	}}
	for _, id := range sortedKeys2(m.nodes) {
		n := m.nodes[id]
		switch n.Kind {
		case graph.KindSession:
			v.Counts.Sessions++
		case graph.KindSubagent:
			v.Counts.Subagents++
		case graph.KindSprint:
			v.Counts.Sprints++
		case graph.KindFlow:
			v.Counts.Flows++
		}
	}
	v.Counts.Waiting = len(m.waiters)

	// Top cards: agents with no parent here, then whatever a spawn cycle hid.
	tops := []string{}
	for id, n := range m.nodes {
		if isAgent(n) && !m.hasPar[id] {
			tops = append(tops, id)
		}
	}
	m.sortByAge(tops)
	cards := make([]Card, 0, len(tops))
	for _, id := range tops {
		cards = append(cards, m.card(id))
	}
	rest := []string{}
	for id, n := range m.nodes {
		if isAgent(n) && !m.visited[id] {
			rest = append(rest, id)
		}
	}
	m.sortByAge(rest)
	for _, id := range rest {
		if !m.visited[id] {
			cards = append(cards, m.card(id))
		}
	}

	byGroup := map[string]*Group{}
	var order []string
	none := Group{ID: "none", Kind: "none", Label: NoGroupLabel, Cards: []Card{}}
	for _, c := range cards {
		target, basis := m.worksFor(c.ID)
		if target == "" {
			none.Cards = append(none.Cards, c)
			continue
		}
		c.Basis = basis
		g, ok := byGroup[target]
		if !ok {
			n := m.nodes[target]
			g = &Group{ID: target, Kind: n.Kind, Label: n.Label, Status: n.Attrs["status"], Phase: n.Attrs["phase"],
				Gate: m.gateOf[target], WaitsOnYou: m.waiters[target], AgeSeconds: n.AgeSeconds, Cards: []Card{}}
			byGroup[target] = g
			order = append(order, target)
			m.groupFor[target] = true
		}
		g.Cards = append(g.Cards, c)
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := byGroup[order[i]], byGroup[order[j]]
		if a.Cards[0].AgeSeconds != b.Cards[0].AgeSeconds {
			return a.Cards[0].AgeSeconds < b.Cards[0].AgeSeconds
		}
		return a.Label < b.Label
	})
	for _, id := range order {
		v.Groups = append(v.Groups, *byGroup[id])
	}
	if len(none.Cards) > 0 {
		v.Groups = append(v.Groups, none)
	}

	for _, id := range sortedKeys2(m.nodes) {
		n := m.nodes[id]
		if (n.Kind != graph.KindSprint && n.Kind != graph.KindFlow) || m.groupFor[id] {
			continue
		}
		v.Idle = append(v.Idle, Idle{ID: id, Kind: n.Kind, Label: n.Label, Status: n.Attrs["status"], Phase: n.Attrs["phase"],
			Gate: m.gateOf[id], WaitsOnYou: m.waiters[id], AgeSeconds: n.AgeSeconds})
	}
	sort.SliceStable(v.Idle, func(i, j int) bool {
		a, b := v.Idle[i], v.Idle[j]
		if a.WaitsOnYou != b.WaitsOnYou {
			return a.WaitsOnYou
		}
		if a.AgeSeconds != b.AgeSeconds {
			return a.AgeSeconds < b.AgeSeconds
		}
		return a.Label < b.Label
	})
	return v
}

// worksFor is the sprint (first) or flow a card works for, and how that is known.
func (m *model) worksFor(id string) (string, string) {
	best, basis := "", ""
	for _, e := range m.out[id] {
		if e.Kind != graph.KindWorksFor {
			continue
		}
		k := m.nodes[e.To].Kind
		if k != graph.KindSprint && k != graph.KindFlow {
			continue
		}
		if best == "" || (k == graph.KindSprint && m.nodes[best].Kind == graph.KindFlow) {
			best, basis = e.To, e.Basis
		}
	}
	return best, basis
}

func (m *model) sortByAge(ids []string) {
	sort.Slice(ids, func(i, j int) bool {
		a, b := m.nodes[ids[i]], m.nodes[ids[j]]
		if a.AgeSeconds != b.AgeSeconds {
			return a.AgeSeconds < b.AgeSeconds
		}
		return a.ID < b.ID
	})
}

func (m *model) card(id string) Card {
	m.visited[id] = true
	n := m.nodes[id]
	c := Card{ID: id, Label: n.Label, Vendor: n.Vendor, Agent: n.Attrs["agent"], AgentType: n.Attrs["agent_type"],
		Model: n.Attrs["model"], State: n.Attrs["state"], Subagent: n.Kind == graph.KindSubagent,
		WaitsOnYou: m.waiters[id], MCP: []MCP{}, AgeSeconds: n.AgeSeconds, Children: []Card{}}
	for _, e := range m.out[id] {
		switch e.Kind {
		case graph.KindUses:
			if m.nodes[e.To].Kind == graph.KindMCP {
				c.MCP = append(c.MCP, MCP{Name: m.nodes[e.To].Label, Ports: m.portsOf(e.To)})
			}
		case graph.KindTouches:
			switch t := m.nodes[e.To]; t.Kind {
			case graph.KindRepo:
				c.Repo = t.Label
			case graph.KindWorktree:
				c.Worktree = t.Label
				if c.Repo == "" {
					c.Repo = m.repoOf(t.ID)
				}
			case graph.KindBranch:
				c.Branch = t.Label
			}
		}
	}
	sort.Slice(c.MCP, func(i, j int) bool { return c.MCP[i].Name < c.MCP[j].Name })
	kids := append([]string{}, m.spawned[id]...)
	m.sortByAge(kids)
	for _, k := range kids {
		if !m.visited[k] {
			c.Children = append(c.Children, m.card(k))
		}
	}
	return c
}

func (m *model) repoOf(worktree string) string {
	for _, e := range m.out[worktree] {
		if e.Kind == graph.KindPartOf && m.nodes[e.To].Kind == graph.KindRepo {
			return m.nodes[e.To].Label
		}
	}
	return ""
}

// portsOf follows mcp, runs_as, process, listens_on, port. A port id is
// port:<proto>:<n>; the number is what a person reads.
func (m *model) portsOf(mcp string) []string {
	ports := []string{}
	for _, e := range m.out[mcp] {
		if e.Kind != graph.KindRunsAs {
			continue
		}
		for _, l := range m.out[e.To] {
			if l.Kind == graph.KindListensOn && m.nodes[l.To].Kind == graph.KindPort {
				ports = append(ports, l.To[strings.LastIndex(l.To, ":")+1:])
			}
		}
	}
	sort.Strings(ports)
	return ports
}

func sortedKeys(m map[string][]graph.Edge) []string {
	k := make([]string, 0, len(m))
	for id := range m {
		k = append(k, id)
	}
	sort.Strings(k)
	return k
}

func sortedKeys2(m map[string]graph.Node) []string {
	k := make([]string, 0, len(m))
	for id := range m {
		k = append(k, id)
	}
	sort.Strings(k)
	return k
}
