package agentmap

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"burnmon/internal/graph"
	"burnmon/internal/waits"
)

// Layout is the map drawn as boxes in columns with lines between them. It
// holds cells (column and row), not pixels: the page turns a cell into a
// position. Everything a person reads is already worded here, in Standard or
// in Plain, so the page only draws strings and the words are tested in Go.

// DefaultMaxBoxes is the size above which groups fold shut.
const DefaultMaxBoxes = 40

// MaxSubagentBoxes is how many subagents one parent shows a box each for; more
// are one box that opens on a click.
const MaxSubagentBoxes = 10

// noneNote is the dim note in a column that holds no box.
const noneNote = "none in this window"

// Evidence is what the live snapshot says about one session or subagent,
// keyed by its node id. A session the snapshot does not list has none and its
// figures read "unknown".
type Evidence struct {
	Model      string
	Tokens     int64
	CostEUR    float64
	CostLabel  string
	Source     string
	AgeSeconds int64
	Known      bool
}

// LayoutInput is the graph, what is known about its sessions, the frames the
// person opened, and the words to use.
type LayoutInput struct {
	Nodes    []graph.Node
	Edges    []graph.Edge
	Evidence map[string]Evidence
	Open     map[string]bool // frame ids the person opened; never folded
	Plain    bool
	MaxBoxes int // 0 means DefaultMaxBoxes
	// Counts is the one count of waiting things, from waits.CountOf. The You box
	// and the counts in the layout read it; nil means zero.
	Counts *waits.Counts
}

// Fact is one line of the evidence panel.
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Box is one box. Kind is harness, session, sub, tool, programs, folded or
// you. Line is the one short line of numbers under the title.
type Box struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Pill  string `json:"pill,omitempty"`
	Line  string `json:"line,omitempty"`
	// Tool is the name of the tool a live session is running, as shown: the
	// name only, and an MCP tool as "MCP tool" so a server's name stays out.
	Tool       string `json:"tool,omitempty"`
	Column     int    `json:"column"`
	Row        int    `json:"row"`
	Group      string `json:"group,omitempty"`
	WaitsOnYou bool   `json:"waits_on_you"`
	Facts      []Fact `json:"facts"`
	// Depth is 0 for a session, 1 for a direct subagent, 2 and up deeper; every
	// subagent sits in column 2 and the page indents by depth.
	Depth int `json:"depth,omitempty"`
	// Tag is the small word naming the kind of box.
	Tag string `json:"tag"`
	// Wait is blocked, your_turn, working or done.
	Wait string `json:"wait,omitempty"`
}

// Line joins two boxes. From is a box id, or a frame id for a frame that
// waits on you. Word is the plain word written on it. You marks the highlighted
// lines to the You box.
type Line struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Word  string `json:"word"`
	Basis string `json:"basis,omitempty"`
	You   bool   `json:"you"`
	// Style is solid, or dotted for a running subagent reporting back.
	Style string `json:"style"`
}

// Frame is the light frame around the agents of one sprint or flow. Count is
// the boxes inside, also when it is folded shut.
type Frame struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Status     string `json:"status,omitempty"`
	Explain    string `json:"explain"`
	WaitsOnYou bool   `json:"waits_on_you"`
	Folded     bool   `json:"folded"`
	Count      int    `json:"count"`
	FirstRow   int    `json:"first_row"`
	LastRow    int    `json:"last_row"`
	FirstCol   int    `json:"first_col"`
	LastCol    int    `json:"last_col"`
}

// Layout is what the page draws.
type Layout struct {
	Columns []string `json:"columns"`
	// ColumnNotes has one entry per column: "" or "none in this window".
	ColumnNotes []string `json:"column_notes"`
	Boxes       []Box    `json:"boxes"`
	Lines       []Line   `json:"lines"`
	Frames      []Frame  `json:"frames"`
	Idle        []Idle   `json:"idle"`
	Counts      Counts   `json:"counts"`
	Notes       []string `json:"notes"`
}

type wording struct {
	plain                       bool
	colSub, colTools            string
	waitsPill, waitsWord        string
	subKind, toolKind, toolPill string
	harnessTag, sessionTag      string
	frameKind                   map[string]string
	stateWord                   map[string]string
}

var plainStates = map[string]string{
	"arriving": "starting up", "reading": "reading files", "fetching": "looking things up", "planning": "planning",
	"coding": "writing code", "running": "running a command", "thinking": "thinking", "delegating": "handing work to a helper",
	"waiting": "waiting for you", "dormant": "idle", "compacting": "tidying its memory",
}

func wordsFor(plain bool) wording {
	if plain {
		return wording{plain: true, colSub: "Helper", colTools: "Programs and tool servers", harnessTag: "app", sessionTag: "agent",
			waitsPill: "waiting for you", waitsWord: "waiting for", subKind: "helper", toolKind: "tool server", toolPill: "tool server",
			frameKind: map[string]string{"sprint": "Foreman job", "flow": "Foreman job"}, stateWord: plainStates}
	}
	return wording{colSub: "Subagents", colTools: "Programs and servers", harnessTag: "tool", sessionTag: "session",
		waitsPill: "waits on you", waitsWord: "waits on", subKind: "subagent", toolKind: "MCP server", toolPill: "MCP",
		frameKind: map[string]string{"sprint": "Foreman sprint", "flow": "Foreman flow"}}
}

// cardState is the word for a card's state: a subagent whose turn ended is
// done, never waiting for a person.
func (w wording) cardState(c Card) string {
	if c.Subagent && c.State == "waiting" {
		if w.plain {
			return "finished"
		}
		return "done"
	}
	return w.state(c.State)
}

var toolNameOK = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,24}$`)

// toolLabel is what the map shows for a tool name. An MCP tool's raw name holds
// its server's name, so it reads "MCP tool"; a name that is not a plain short
// identifier reads "tool". Arguments are never in the name, and never read.
func toolLabel(name string, plain bool) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(name), "mcp__") {
		if plain {
			return "tool server call"
		}
		return "MCP tool"
	}
	if !toolNameOK.MatchString(name) {
		return "tool"
	}
	return name
}

func (w wording) state(s string) string {
	if w.stateWord != nil {
		if p, ok := w.stateWord[s]; ok {
			return p
		}
	}
	return s
}

func fmtInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func fmtDur(sec int64) string {
	if sec < 0 {
		sec = 0
	}
	switch {
	case sec < 60:
		return fmt.Sprintf("%d s", sec)
	case sec < 3600:
		if s := sec % 60; s != 0 {
			return fmt.Sprintf("%d min %d s", sec/60, s)
		}
		return fmt.Sprintf("%d min", sec/60)
	}
	if mm := (sec % 3600) / 60; mm != 0 {
		return fmt.Sprintf("%d h %d min", sec/3600, mm)
	}
	return fmt.Sprintf("%d h", sec/3600)
}

// programsOf counts every process started, directly or not, by the processes a
// session runs in, and how many there are per image name. Command lines are
// never read.
func (m *model) programsOf(session string) (int, map[string]int) {
	names := map[string]int{}
	seen := map[string]bool{}
	count := 0
	var walk func(id string)
	walk = func(id string) {
		for _, e := range m.out[id] {
			if e.Kind == graph.KindStarted && !seen[e.To] {
				seen[e.To] = true
				count++
				names[m.nodes[e.To].Label]++
				walk(e.To)
			}
		}
	}
	for _, e := range m.out[session] {
		if e.Kind == graph.KindRunsIn && m.nodes[e.To].Kind == graph.KindProcess {
			walk(e.To)
		}
	}
	return count, names
}

func (m *model) harnessOf(session string) string {
	for _, e := range m.out[session] {
		if e.Kind == graph.KindRunsIn && m.nodes[e.To].Kind == graph.KindHarness {
			return e.To
		}
	}
	return ""
}

type builder struct {
	m        *model
	in       LayoutInput
	w        wording
	l        Layout
	toolsCol int // always 3
	lineSeen map[string]bool
}

func (b *builder) addLine(from, to, word, basis string, you bool) {
	b.addStyled(from, to, word, basis, "solid", you)
}

func (b *builder) addStyled(from, to, word, basis, style string, you bool) {
	k := from + "|" + word + "|" + to
	if b.lineSeen[k] {
		return
	}
	b.lineSeen[k] = true
	b.l.Lines = append(b.l.Lines, Line{From: from, To: to, Word: word, Basis: basis, You: you, Style: style})
}

// subsFolded says whether a card's subagents are one closed box: more than
// MaxSubagentBoxes of them, and the person has not opened it.
func (b *builder) subsFolded(c Card) bool {
	return len(c.Children) > MaxSubagentBoxes && !b.in.Open["subs:"+c.ID]
}

// maxMCPBoxes is how many MCP servers a session gets a box each for; more are
// one box, listing each server in its evidence.
const maxMCPBoxes = 3

func mcpBoxes(c Card) int {
	if len(c.MCP) > maxMCPBoxes {
		return 1
	}
	return len(c.MCP)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (b *builder) toolCount(c Card) int {
	n := mcpBoxes(c)
	if p, _ := b.m.programsOf(c.ID); p > 0 {
		n++
	}
	return n
}

func (b *builder) boxCount(c Card) int {
	n := 1 + b.toolCount(c)
	if b.subsFolded(c) {
		return n + 1
	}
	for _, k := range c.Children {
		n += b.boxCount(k)
	}
	return n
}

func (b *builder) agentCount(c Card) int {
	n := 1
	for _, k := range c.Children {
		n += b.agentCount(k)
	}
	return n
}

func (b *builder) height(c Card) int {
	h := b.toolCount(c)
	if b.subsFolded(c) {
		h++
	} else {
		for _, k := range c.Children {
			h += b.height(k)
		}
	}
	if h < 1 {
		h = 1
	}
	return h
}

func waitingCards(cs []Card) int {
	n := 0
	for _, c := range cs {
		if c.WaitsOnYou {
			n++
		}
		n += waitingCards(c.Children)
	}
	return n
}

func (b *builder) groupWaits(g Group) bool { return g.WaitsOnYou || waitingCards(g.Cards) > 0 }

func (b *builder) groupBoxes(g Group) int {
	n := 0
	for _, c := range g.Cards {
		n += b.boxCount(c)
	}
	return n
}

func (b *builder) groupAgents(g Group) int {
	n := 0
	for _, c := range g.Cards {
		n += b.agentCount(c)
	}
	return n
}

func (b *builder) groupInferred(g Group) bool {
	for _, c := range g.Cards {
		if c.Basis == "inferred" {
			return true
		}
	}
	return false
}

func frameID(g Group) string { return "frame:" + g.ID }

func (b *builder) explain(g Group) string {
	switch g.Kind {
	case "none":
		if b.w.plain {
			return "No Foreman job is linked to these."
		}
		return "No Foreman sprint or flow is linked to these."
	case "project":
		if g.Label == "Other" {
			if b.w.plain {
				return "Helpers whose main session is not in this window. No Foreman job is linked."
			}
			return "Subagents whose parent is not in this window. No Foreman sprint or flow is linked."
		}
		if b.w.plain {
			return fmt.Sprintf("Work in the folder \"%s\". No Foreman job is linked.", g.Label)
		}
		return fmt.Sprintf("Sessions in the folder \"%s\". No Foreman sprint or flow is linked.", g.Label)
	}
	what := b.w.frameKind[g.Kind]
	var s string
	if b.w.plain {
		s = fmt.Sprintf("These agents are doing the %s \"%s\"", what, g.Label)
	} else {
		s = fmt.Sprintf("Agents here work for the %s \"%s\"", what, g.Label)
	}
	switch {
	case g.Status != "" && g.Phase != "":
		s += fmt.Sprintf(" (%s, %s)", g.Status, g.Phase)
	case g.Status != "":
		s += fmt.Sprintf(" (%s)", g.Status)
	}
	s += "."
	switch {
	case b.groupInferred(g) && b.w.plain:
		s += " BurnMon guessed the link from the folder."
	case b.groupInferred(g):
		s += " BurnMon matched them by folder, Foreman did not name them."
	case b.w.plain:
		s += " Foreman names these agents."
	default:
		s += " Foreman names these agents."
	}
	return s
}

func (b *builder) worksFor(g Group, c Card) string {
	how := "named by Foreman"
	if c.Basis == "inferred" {
		how = "matched by BurnMon from the folder"
	}
	return fmt.Sprintf("%s (%s)", g.Label, how)
}

func (b *builder) cardFacts(c Card, g Group) []Fact {
	ev, has := b.in.Evidence[c.ID]
	has = has && ev.Known
	kind := "session"
	if c.Subagent {
		kind = b.w.subKind
	}
	agent := c.Agent
	if c.AgentType != "" {
		agent = strings.TrimSpace(agent + " (" + c.AgentType + ")")
	}
	model := c.Model
	if has && ev.Model != "" {
		model = ev.Model
	}
	val := func(s string) string {
		if s == "" {
			return "unknown"
		}
		return s
	}
	label := "API price equivalent"
	if has && ev.CostLabel != "" {
		label = ev.CostLabel
	}
	tokens, price, source := "unknown", "unknown", "unknown"
	if has {
		tokens = fmtInt(ev.Tokens)
		price = fmt.Sprintf("€%.2f", ev.CostEUR)
		source = fmt.Sprintf("%s, %s old", val(ev.Source), fmtDur(ev.AgeSeconds))
	}
	f := []Fact{
		{"Kind", kind}, {"Agent", val(agent)}, {"Model", val(model)}, {"State", b.w.cardState(c)},
		{"Tokens", tokens}, {label, price}, {"Last active", fmtDur(c.AgeSeconds) + " ago"}, {"Source", source},
	}
	if tl := toolLabel(c.Tool, b.w.plain); tl != "" {
		f = append(f, Fact{"Current tool", tl})
	}
	if strings.EqualFold(c.Agent, "cowork-cloud") {
		// A known gap: the cloud transcript has no pending-question marker.
		f = append(f, Fact{"Questions", "questions in Cowork cloud are not visible"})
	}
	if g.Kind == "sprint" || g.Kind == "flow" {
		f = append(f, Fact{"Works for", b.worksFor(g, c)})
	}
	if c.Repo != "" {
		f = append(f, Fact{"Repo", c.Repo})
	}
	if c.Worktree != "" {
		f = append(f, Fact{"Worktree", c.Worktree})
	}
	if c.Branch != "" {
		f = append(f, Fact{"Branch", c.Branch})
	}
	return f
}

func (b *builder) cardLine(c Card) string {
	ago := fmtDur(c.AgeSeconds) + " ago"
	if ev, ok := b.in.Evidence[c.ID]; ok && ev.Known {
		return fmt.Sprintf("%s tokens - %s", fmtInt(ev.Tokens), ago)
	}
	return ago
}

func (b *builder) cardTitle(c Card) string {
	if c.Subagent {
		if c.AgentType != "" {
			return c.AgentType
		}
		if c.Label != "" {
			return c.Label
		}
		return b.w.subKind
	}
	if c.Label != "" {
		return c.Label
	}
	return c.Agent
}

// place puts a card and what belongs to it: its tools under its own row, its
// children after them. It returns the rows used.
func (b *builder) place(c Card, g Group, fid string, row, depth int) int {
	pill := b.w.cardState(c)
	wait := b.waitOf(c)
	switch {
	case c.WaitsOnYou:
		pill = b.w.waitsPill
	case wait == "your_turn":
		pill = "your turn"
	case wait == "done" && !c.Subagent:
		pill = "can be closed"
	}
	kind, tag, col := "session", b.w.sessionTag, 1
	if c.Subagent {
		kind, tag, col = "sub", b.w.subKind, 2
	}
	b.l.Boxes = append(b.l.Boxes, Box{ID: c.ID, Kind: kind, Title: b.cardTitle(c), Pill: pill, Line: b.cardLine(c),
		Tool: toolLabel(c.Tool, b.w.plain), Column: col, Row: row, Group: fid, WaitsOnYou: c.WaitsOnYou, Facts: b.cardFacts(c, g),
		Depth: depth, Tag: tag, Wait: wait})
	r := row
	if len(c.MCP) > maxMCPBoxes {
		id := "mcp:all@" + c.ID
		withPort := 0
		facts := []Fact{{"Kind", b.w.toolKind + "s"}, {"Source", "mcp_config"}}
		for _, mc := range c.MCP {
			if len(mc.Ports) > 0 {
				withPort++
				facts = append(facts, Fact{mc.Name, "port " + strings.Join(mc.Ports, ", ")})
			} else {
				facts = append(facts, Fact{mc.Name, "no port seen"})
			}
		}
		title := fmt.Sprintf("%d %ss", len(c.MCP), b.w.toolKind)
		b.l.Boxes = append(b.l.Boxes, Box{ID: id, Kind: "tool", Title: title, Pill: b.w.toolPill,
			Line: fmt.Sprintf("%d with a port", withPort), Column: b.toolsCol, Row: r, Group: fid, Facts: facts, Tag: b.w.toolPill})
		b.addLine(c.ID, id, "uses", "inferred", false)
		r++
	}
	for _, mc := range c.MCP {
		if len(c.MCP) > maxMCPBoxes {
			break
		}
		id := "mcp:" + mc.Name + "@" + c.ID
		line := "no port seen"
		if len(mc.Ports) > 0 {
			line = "port " + strings.Join(mc.Ports, ", ")
		}
		facts := []Fact{{"Kind", b.w.toolKind}, {"Name", mc.Name}, {"Source", b.sourceOf("mcp:" + mc.Name)}}
		if len(mc.Ports) > 0 {
			facts = append(facts, Fact{"Listens on", strings.Join(mc.Ports, ", ")})
		} else {
			facts = append(facts, Fact{"Listens on", "unknown"})
		}
		b.l.Boxes = append(b.l.Boxes, Box{ID: id, Kind: "tool", Title: mc.Name, Pill: b.w.toolPill, Line: line,
			Column: b.toolsCol, Row: r, Group: fid, Facts: facts, Tag: b.w.toolPill})
		b.addLine(c.ID, id, "uses", "inferred", false)
		r++
	}
	if n, names := b.m.programsOf(c.ID); n > 0 {
		id := "programs:" + c.ID
		title := fmt.Sprintf("%d programs", n)
		if n == 1 {
			title = "1 program"
		}
		facts := []Fact{{"Kind", "programs started by this " + map[bool]string{true: b.w.subKind, false: "session"}[c.Subagent]},
			{"Source", "sysmon"}}
		keys := make([]string, 0, len(names))
		for k := range names {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if names[keys[i]] != names[keys[j]] {
				return names[keys[i]] > names[keys[j]]
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			facts = append(facts, Fact{k, fmt.Sprintf("%d", names[k])})
		}
		facts = append(facts, Fact{"Command lines", "never recorded"})
		b.l.Boxes = append(b.l.Boxes, Box{ID: id, Kind: "programs", Title: title, Pill: "programs", Line: "started here",
			Column: b.toolsCol, Row: r, Group: fid, Facts: facts, Tag: "programs"})
		b.addLine(c.ID, id, "started", "vendor", false)
		r++
	}
	cr := row + b.toolCount(c)
	if b.subsFolded(c) {
		id := "fold:subs:" + c.ID
		n := len(c.Children)
		title := fmt.Sprintf("%d %ss", n, b.w.subKind)
		facts := []Fact{{"Kind", "closed group"}, {"Contains", fmt.Sprintf("%d %ss", n, b.w.subKind)}}
		b.l.Boxes = append(b.l.Boxes, Box{ID: id, Kind: "folded", Title: title, Pill: "closed", Line: "click to open",
			Column: 2, Row: cr, Group: fid, Facts: facts, Depth: depth + 1, Tag: "group"})
		b.addLine(c.ID, id, "starts", "vendor", false)
	} else {
		for _, k := range c.Children {
			h := b.place(k, g, fid, cr, depth+1)
			b.addLine(c.ID, k.ID, "starts", "vendor", false)
			if k.Running {
				b.addStyled(k.ID, c.ID, "reports to", "vendor", "dotted", false)
			}
			cr += h
		}
	}
	return b.height(c)
}

// waitOf is the wait word of a box: what the graph says, with the open
// waits_on edge winning for older data that has no wait attribute. A subagent
// is done once its turn ended and working until then.
func (b *builder) waitOf(c Card) string {
	switch {
	case c.WaitsOnYou:
		return "blocked"
	case c.CanClose:
		return "done"
	case c.Subagent:
		if c.State == "waiting" || c.State == "dormant" {
			return "done"
		}
		return "working"
	}
	return c.Wait
}

func (b *builder) sourceOf(nodeID string) string {
	if n, ok := b.m.nodes[nodeID]; ok && n.Source != "" {
		return n.Source
	}
	return "unknown"
}

// BuildLayout turns a graph into boxes, lines and frames. Groups fold shut,
// largest first, while the map has more than MaxBoxes boxes; a group that
// waits on you folds last, and a frame the person opened never folds.
func BuildLayout(in LayoutInput) Layout {
	v := Build(in.Nodes, in.Edges)
	v.Groups = splitUnlinked(v.Groups)
	b := &builder{m: newModel(in.Nodes, in.Edges), in: in, w: wordsFor(in.Plain), lineSeen: map[string]bool{}}
	b.l = Layout{Columns: []string{}, Boxes: []Box{}, Lines: []Line{}, Frames: []Frame{}, Idle: v.Idle, Counts: v.Counts, Notes: v.Notes}
	max := in.MaxBoxes
	if max <= 0 {
		max = DefaultMaxBoxes
	}
	var counts waits.Counts
	if in.Counts != nil {
		counts = *in.Counts
	}
	b.l.Counts.Waiting, b.l.Counts.YourTurn = counts.Blocked, counts.YourTurn
	youBox := counts.Blocked+counts.YourTurn > 0

	harnesses := map[string]bool{}
	for _, g := range v.Groups {
		for _, c := range g.Cards {
			if h := b.m.harnessOf(c.ID); h != "" {
				harnesses[h] = true
			}
		}
	}
	folded := map[string]bool{}
	total := func() int {
		n := len(harnesses)
		if youBox {
			n++
		}
		for _, g := range v.Groups {
			if folded[g.ID] {
				n++
			} else {
				n += b.groupBoxes(g)
			}
		}
		return n
	}
	for total() > max {
		pick := -1
		for i, g := range v.Groups {
			if folded[g.ID] || in.Open[frameID(g)] || b.groupBoxes(g) <= 1 {
				continue
			}
			if pick < 0 {
				pick = i
				continue
			}
			p := v.Groups[pick]
			wi, wp := b.groupWaits(g), b.groupWaits(p)
			switch {
			case wi != wp:
				if !wi {
					pick = i
				}
			case b.groupBoxes(g) != b.groupBoxes(p):
				if b.groupBoxes(g) > b.groupBoxes(p) {
					pick = i
				}
			case g.Label < p.Label:
				pick = i
			}
		}
		if pick < 0 {
			break
		}
		folded[v.Groups[pick].ID] = true
	}

	b.l.Columns = append(b.l.Columns, "Tool", "Session", b.w.colSub, b.w.colTools, "You")
	b.toolsCol = 3
	youCol := 4

	harnessRow := map[string]int{}
	harnessRoots := map[string]int{}
	youRows := []int{}
	row := 0
	for _, g := range v.Groups {
		fid := frameID(g)
		start := row
		fr := Frame{ID: fid, Kind: g.Kind, Label: g.Label, Status: g.Status, Explain: b.explain(g), WaitsOnYou: g.WaitsOnYou,
			Count: b.groupBoxes(g), FirstRow: start, FirstCol: 1, LastCol: b.toolsCol}
		if folded[g.ID] {
			id := "fold:" + fid
			agents := b.groupAgents(g)
			facts := []Fact{{"Kind", "closed group"}, {"Contains", fmt.Sprintf("%d agents, %d boxes", agents, fr.Count)}, {"What it is", fr.Explain}}
			wc := waitingCards(g.Cards)
			pill := "closed"
			if b.groupWaits(g) {
				pill = b.w.waitsPill
			}
			b.l.Boxes = append(b.l.Boxes, Box{ID: id, Kind: "folded", Title: g.Label, Pill: pill,
				Line: plural(agents, "agent", "agents") + " - click to open", Column: 1, Row: row, Group: fid, WaitsOnYou: b.groupWaits(g), Facts: facts, Tag: "group"})
			fr.Folded, fr.LastRow, fr.LastCol = true, row, 1
			for _, c := range g.Cards {
				if h := b.m.harnessOf(c.ID); h != "" {
					b.addLine(h, id, "runs", "vendor", false)
					if r, ok := harnessRow[h]; !ok || row < r {
						harnessRow[h] = row
					}
					harnessRoots[h]++
				}
			}
			if wc > 0 || g.WaitsOnYou {
				b.addLine(id, "you", b.w.waitsWord, "vendor", true)
				youRows = append(youRows, row)
			}
			row++
		} else {
			for _, c := range g.Cards {
				h := b.place(c, g, fid, row, 0)
				if hid := b.m.harnessOf(c.ID); hid != "" {
					b.addLine(hid, c.ID, "runs", "vendor", false)
					if r, ok := harnessRow[hid]; !ok || row < r {
						harnessRow[hid] = row
					}
					harnessRoots[hid]++
				}
				row += h
			}
			fr.LastRow = row - 1
			cards := 0
			var walk func(cs []Card)
			walk = func(cs []Card) {
				for _, c := range cs {
					if c.WaitsOnYou {
						b.addLine(c.ID, "you", b.w.waitsWord, "vendor", true)
						if bx, ok := b.boxRow(c.ID); ok {
							youRows = append(youRows, bx)
						}
						cards++
					}
					walk(c.Children)
				}
			}
			walk(g.Cards)
			if cards == 0 && g.WaitsOnYou {
				b.addLine(fid, "you", b.w.waitsWord, "vendor", true)
				youRows = append(youRows, start)
			}
		}
		b.l.Frames = append(b.l.Frames, fr)
	}
	row = b.blockedBoxes(v.Idle, row, &youRows)

	hids := make([]string, 0, len(harnessRow))
	for h := range harnessRow {
		hids = append(hids, h)
	}
	sort.Strings(hids)
	boxes := make([]Box, 0, len(b.l.Boxes)+len(hids)+1)
	for _, h := range hids {
		n := b.m.nodes[h]
		boxes = append(boxes, Box{ID: h, Kind: "harness", Title: n.Label, Pill: "running", Line: plural(harnessRoots[h], "session", "sessions"),
			Column: 0, Row: harnessRow[h], Facts: []Fact{{"Kind", "tool"}, {"Name", n.Label}, {"Source", b.sourceOf(h)}}, Tag: b.w.harnessTag})
	}
	boxes = append(boxes, b.l.Boxes...)
	if youBox {
		youPill := ""
		if counts.Blocked > 0 {
			youPill = b.w.waitsPill
		}
		r := 0
		if len(youRows) > 0 {
			r = youRows[0]
			for _, y := range youRows {
				if y < r {
					r = y
				}
			}
		}
		boxes = append(boxes, Box{ID: "you", Kind: "you", Title: "You", Pill: youPill, Line: fmt.Sprintf("%d %s you, %d your turn", counts.Blocked, map[bool]string{true: "waiting for", false: "waiting on"}[b.w.plain], counts.YourTurn),
			Column: youCol, Row: r, WaitsOnYou: counts.Blocked > 0, Facts: b.youFacts(), Tag: "you"})
	}
	b.l.Boxes = boxes
	b.l.ColumnNotes = b.columnNotes()
	return b.l
}

// columnNotes gives each column its note: "none in this window" when no box
// sits in it.
func (b *builder) columnNotes() []string {
	notes := make([]string, len(b.l.Columns))
	used := make([]bool, len(b.l.Columns))
	for _, bx := range b.l.Boxes {
		if bx.Column >= 0 && bx.Column < len(used) {
			used[bx.Column] = true
		}
	}
	for i := range notes {
		if !used[i] {
			notes[i] = noneNote
		}
	}
	return notes
}

func (b *builder) boxRow(id string) (int, bool) {
	for _, bx := range b.l.Boxes {
		if bx.ID == id {
			return bx.Row, true
		}
	}
	return 0, false
}

// youFacts lists everything that waits on the person, by kind and name.
func (b *builder) youFacts() []Fact {
	ids := make([]string, 0, len(b.m.waiters))
	for id := range b.m.waiters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var facts []Fact
	for _, id := range ids {
		n := b.m.nodes[id]
		kind := n.Kind
		if kind == graph.KindSession || kind == graph.KindSubagent {
			kind = "agent"
		}
		if kind == graph.KindSprint && b.w.plain {
			kind = "job"
		}
		facts = append(facts, Fact{kind, fmt.Sprintf("%s (%s old)", n.Label, fmtDur(n.AgeSeconds))})
	}
	return facts
}

// splitUnlinked turns the one "No Foreman work" group into one group per
// project folder (the session's label), so folding acts on a project and not on
// everything at once. A subagent whose parent is not here goes to "Other". With
// one project or none the group is left alone.
func splitUnlinked(groups []Group) []Group {
	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		if g.Kind != "none" {
			out = append(out, g)
			continue
		}
		byKey := map[string]*Group{}
		var keys []string
		for _, c := range g.Cards {
			key := c.Label
			if c.Subagent || key == "" {
				key = "Other"
			}
			p, ok := byKey[key]
			if !ok {
				p = &Group{ID: "project:" + key, Kind: "project", Label: key, Cards: []Card{}}
				byKey[key] = p
				keys = append(keys, key)
			}
			p.Cards = append(p.Cards, c)
		}
		if len(keys) < 2 {
			out = append(out, g)
			continue
		}
		sort.SliceStable(keys, func(i, j int) bool {
			a, b := byKey[keys[i]], byKey[keys[j]]
			if (keys[i] == "Other") != (keys[j] == "Other") {
				return keys[j] == "Other"
			}
			if a.Cards[0].AgeSeconds != b.Cards[0].AgeSeconds {
				return a.Cards[0].AgeSeconds < b.Cards[0].AgeSeconds
			}
			return a.Label < b.Label
		})
		for _, k := range keys {
			out = append(out, *byKey[k])
		}
	}
	return out
}

// blockedBoxes draws the Blocked Foreman things no session works for: a gate
// or waiting sprint that shows as an idle row, and a failed check. Each gets
// a box and its amber line into You, so the lines equal the Blocked count. The
// idle row stays listed. It returns the next free row.
func (b *builder) blockedBoxes(idle []Idle, row int, youRows *[]int) int {
	for _, i := range idle {
		if !i.WaitsOnYou {
			continue
		}
		facts := []Fact{{"Kind", i.Kind}, {"Status", i.Status}, {"Source", b.sourceOf(i.ID)}}
		if i.Gate != "" {
			facts = append(facts, Fact{"Gate", i.Gate})
		}
		line := i.Status
		if i.Gate != "" {
			line = "gate " + i.Gate
		}
		b.l.Boxes = append(b.l.Boxes, Box{ID: i.ID, Kind: i.Kind, Title: i.Label, Pill: b.w.waitsPill, Line: line,
			Column: 1, Row: row, WaitsOnYou: true, Facts: facts, Tag: i.Kind, Wait: "blocked"})
		b.addLine(i.ID, "you", b.w.waitsWord, "vendor", true)
		*youRows = append(*youRows, row)
		row++
	}
	ids := make([]string, 0, len(b.m.waiters))
	for id, n := range b.m.waiters {
		if n && b.m.nodes[id].Kind == graph.KindCheck {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := b.m.nodes[id]
		title, status := n.Label, ""
		for _, e := range b.m.out[id] {
			if e.Kind == graph.KindPartOf && b.m.nodes[e.To].Kind == graph.KindSprint {
				title = b.m.nodes[e.To].Label + ": " + n.Label
				status = b.m.nodes[e.To].Attrs["status"]
			}
		}
		b.l.Boxes = append(b.l.Boxes, Box{ID: id, Kind: "check", Title: title, Pill: b.w.waitsPill, Line: "failed check",
			Column: 1, Row: row, WaitsOnYou: true, Facts: []Fact{{"Kind", "failed check"}, {"Sprint status", status}, {"Source", b.sourceOf(id)}},
			Tag: "check", Wait: "blocked"})
		b.addLine(id, "you", b.w.waitsWord, "vendor", true)
		*youRows = append(*youRows, row)
		row++
	}
	return row
}
