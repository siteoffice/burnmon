package lamp

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"burnmon/internal/snapshot"
	"burnmon/internal/timefmt"
	"burnmon/internal/waits"
)

// The eight moods, in the order the first that holds wins.
const (
	MoodOff       = "off"
	MoodBored     = "bored"
	MoodBlocked   = "blocked"
	MoodYourTurn  = "your_turn"
	MoodSweating  = "sweating"
	MoodDone      = "done"
	MoodWorking   = "working"
	MoodAsleep    = "asleep"
	costLabel     = "API price equivalent"
	boredMinutes  = 10
	maxBricks     = 30
	maxRows       = 6
	titleRunes    = 40
	doneFreshSecs = 600
	minSamples    = 120
	sweatFactor   = 3.0
)

// RowKindMore is the Kind of the last row when more items wait than fit.
const RowKindMore = "more"

// Cost is a price figure with the label that says what it is.
type Cost struct {
	USD   float64 `json:"usd"`
	EUR   float64 `json:"eur"`
	Label string  `json:"label"`
}

// Row is one counted item as the lamp shows it. It carries no detail, no
// project and no path of the item.
type Row struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Word      string `json:"word"`       // the kind word, from waits.WordFor
	Title     string `json:"title"`      // one line, at most 40 runes
	TitleFull string `json:"title_full"` // one line, uncut, for the hover text
	Wait      string `json:"wait"`       // timefmt.Short, for example "12 min"
	WaitExact string `json:"wait_exact"` // timefmt.Exact, for the hover text
}

// State is everything the lamp page needs for one frame. Line is the
// sentence of Glim from lines.go, empty when lamp_lines is off.
type State struct {
	Mood           string       `json:"mood"`
	Headline       string       `json:"headline"`
	Line           string       `json:"line"`
	Rows           []Row        `json:"rows"`
	Counts         waits.Counts `json:"counts"`
	Agents         int          `json:"agents"`
	LongestWaitMin int          `json:"longest_wait_min"`
	WallBricks     int          `json:"wall_bricks"`
	WallOver       bool         `json:"wall_over"` // more than 30 bricks: the wall shows "30+"
	BurnRate       float64      `json:"burn_rate"` // tokens a minute
	Cost5          Cost         `json:"cost_5min"`
	Demo           bool         `json:"demo"`
}

// Input is everything Compute joins.
type Input struct {
	Snap        *snapshot.Snapshot // nil: no data
	Now         time.Time
	Settings    Settings
	BurnSamples []float64 // burn rates of the lamp's earlier fresh reads in the last 30 minutes
	Demo        bool
}

// Load reads the snapshot file. A missing, unreadable, unsupported or stale
// file is no data (nil), never an error.
func Load(path string, now time.Time) *snapshot.Snapshot {
	return snapshot.Load(path, now).Snapshot
}

// Compute turns a snapshot into a lamp state. It takes group and counts from
// the snapshot as they are and never works out a group itself.
func Compute(in Input) State {
	st := State{Rows: []Row{}, Demo: in.Demo, Cost5: Cost{Label: costLabel}}
	snap := in.Snap
	if snap == nil || snap.IsStale(in.Now) {
		st.Mood = MoodOff
		st.Headline = "BurnMon is not running"
		st.Line = Line(st, in.Settings, in.Now)
		return st
	}
	st.BurnRate = snap.Burn.TokensPerMinute
	st.Cost5.USD, st.Cost5.EUR = snap.Burn.Cost.USD, snap.Burn.Cost.EUR

	items := snap.WaitItems
	st.Counts = snap.WaitCounts
	if !in.Settings.ForemanItems {
		items = withoutForeman(items)
		st.Counts = waits.CountOf(items)
	}

	var counted []waits.Item
	var doneFresh bool
	longest := int64(0)
	for _, it := range items {
		switch it.Group {
		case waits.GroupBlocked, waits.GroupYourTurn:
			counted = append(counted, it)
			if liveKind(it.Kind) && it.AgeSeconds > longest {
				longest = it.AgeSeconds
			}
		case waits.GroupDone:
			if it.AgeSeconds < doneFreshSecs {
				doneFresh = true
			}
		}
	}
	sort.SliceStable(counted, func(i, j int) bool {
		a, b := counted[i], counted[j]
		if (a.Group == waits.GroupBlocked) != (b.Group == waits.GroupBlocked) {
			return a.Group == waits.GroupBlocked
		}
		if a.AgeSeconds != b.AgeSeconds {
			return a.AgeSeconds > b.AgeSeconds
		}
		return a.ID < b.ID
	})
	st.Rows = rows(counted)
	st.LongestWaitMin = int(longest / 60)
	for _, s := range snap.Sessions {
		if s.ParentID == "" && s.State != "waiting" && s.State != "dormant" {
			st.Agents++
		}
	}

	switch {
	case st.LongestWaitMin >= boredMinutes:
		st.Mood = MoodBored
		st.WallBricks = min(st.LongestWaitMin, maxBricks)
		st.WallOver = st.LongestWaitMin > maxBricks
	case hasGroup(counted, waits.GroupBlocked):
		st.Mood = MoodBlocked
	case hasGroup(counted, waits.GroupYourTurn):
		st.Mood = MoodYourTurn
	case sweating(st.BurnRate, in.BurnSamples, in.Settings.BurnFloorTPM):
		st.Mood = MoodSweating
	case doneFresh:
		st.Mood = MoodDone
	case st.Agents >= 1:
		st.Mood = MoodWorking
	default:
		st.Mood = MoodAsleep
	}
	st.Headline = headline(st)
	st.Line = Line(st, in.Settings, in.Now)
	return st
}

func headline(st State) string {
	switch st.Mood {
	case MoodBored, MoodBlocked:
		if st.Counts.Blocked == 0 { // bored on your_turn items alone
			return strconv.Itoa(st.Counts.YourTurn) + " your turn"
		}
		h := strconv.Itoa(st.Counts.Blocked) + " blocked"
		if st.Counts.YourTurn > 0 {
			h += ", " + strconv.Itoa(st.Counts.YourTurn) + " your turn"
		}
		return h
	case MoodYourTurn:
		return strconv.Itoa(st.Counts.YourTurn) + " your turn"
	case MoodSweating:
		return rate(st.BurnRate) + " tokens a minute"
	case MoodDone:
		return strconv.Itoa(st.Counts.Done) + " ready to close"
	case MoodWorking:
		if st.Agents == 1 {
			return "1 agent at work"
		}
		return strconv.Itoa(st.Agents) + " agents at work"
	}
	return "No agents running"
}

// rate writes a token rate in whole tokens, or from 1000 in whole thousands
// with k, rounded to the nearest.
func rate(r float64) string {
	n := int(math.Round(r))
	if n >= 1000 {
		return strconv.Itoa(int(math.Round(float64(n)/1000))) + "k"
	}
	return strconv.Itoa(n)
}

func rows(counted []waits.Item) []Row {
	shown := counted
	if len(counted) > maxRows {
		shown = counted[:maxRows-1]
	}
	out := make([]Row, 0, maxRows)
	for _, it := range shown {
		full := oneLine(it.Title)
		out = append(out, Row{
			ID: it.ID, Kind: it.Kind, Word: waits.WordFor(it),
			Title: cut(full), TitleFull: full,
			Wait: timefmt.Short(it.AgeSeconds), WaitExact: timefmt.Exact(it.AgeSeconds),
		})
	}
	if len(counted) > maxRows {
		out = append(out, Row{Kind: RowKindMore, Title: "and " + strconv.Itoa(len(counted)-(maxRows-1)) + " more"})
		out[len(out)-1].TitleFull = out[len(out)-1].Title
	}
	return out
}

// cut keeps a title of up to 40 runes, and shortens a longer one to 40 runes
// ending in "...".
func cut(s string) string {
	r := []rune(s)
	if len(r) <= titleRunes {
		return s
	}
	return string(r[:titleRunes-3]) + "..."
}

// oneLine turns line breaks, tabs and other control characters into single
// spaces: a title can be a Foreman slug with anything in it.
func oneLine(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
}

func liveKind(kind string) bool {
	switch kind {
	case waits.KindQuestion, waits.KindPrompt, waits.KindSession, waits.KindYourTurn:
		return true
	}
	return false
}

func foremanKind(kind string) bool {
	switch kind {
	case waits.KindGate, waits.KindSprint, waits.KindFailedCheck:
		return true
	}
	return false
}

func withoutForeman(items []waits.Item) []waits.Item {
	out := make([]waits.Item, 0, len(items))
	for _, it := range items {
		if !foremanKind(it.Kind) {
			out = append(out, it)
		}
	}
	return out
}

func hasGroup(items []waits.Item, group string) bool {
	for _, it := range items {
		if it.Group == group {
			return true
		}
	}
	return false
}

// sweating: the usual burn is defined (enough samples) and the rate is at
// least three times it and at least the floor.
func sweating(rate float64, samples []float64, floor int) bool {
	if len(samples) < minSamples || rate <= 0 {
		return false
	}
	return rate >= sweatFactor*median(samples) && rate >= float64(floor)
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}
