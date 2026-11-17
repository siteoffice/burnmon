// Package vendorstrip builds the Now page's vendor strip (v0.2 P3): one row
// per vendor seen in the store, tokens today/this week/this month, a total
// row last. Refreshed once a minute from its own timer, not the Now page's
// 2-second poll (bmLive), so it runs one SQL aggregate against the whole
// events table rather than reusing bmLive's windowed read.
package vendorstrip

import (
	"sort"
	"time"

	"burnmon/internal/pricing"
	"burnmon/internal/scan"
	"burnmon/internal/store"
)

// AgentLabel mirrors internal/history.AgentLabel: the same display names,
// since the strip's rows and History's vendor filter must read as the same
// vocabulary.
var AgentLabel = map[string]string{
	"claude-code":    "Claude Code",
	"cowork":         "Cowork",
	"cowork-cloud":   "Cowork (Cloud)",
	"codex":          "Codex",
	"hermes":         "Hermes",
	"copilot-cli":    "Copilot CLI",
	"copilot-vscode": "Copilot (VS Code)",
}

// Row is one vendor's (or, as Total, every vendor's) token totals.
type Row struct {
	Agent      string `json:"agent"` // "" on the Total row
	AgentLabel string `json:"agent_label"`
	Today      int64  `json:"today"`
	Week       int64  `json:"week"`
	Month      int64  `json:"month"`
	// TodayNote, WeekNote and MonthNote mark a Cowork (Cloud) cell whose number
	// cannot be trusted: NoteNotMeasured (the period starts after the last
	// measured cloud row, so 0 means unknown) or NotePartial (the period holds
	// the last measured row, later cloud sessions are missing). The Total row
	// carries NotePartial where the cloud cell does. Empty on every other cell.
	TodayNote string `json:"today_note,omitempty"`
	WeekNote  string `json:"week_note,omitempty"`
	MonthNote string `json:"month_note,omitempty"`
}

// The values of Row's note fields.
const (
	NoteNotMeasured = "not_measured"
	NotePartial     = "partial"
)

// cloudLastMeasured returns the newest Cowork (Cloud) drop row time. A var so
// tests can fix it without drop folders.
var cloudLastMeasured = func() (time.Time, bool) {
	return scan.CloudLastMeasured(scan.CloudDropDirs())
}

// cloudNote classifies one period of the Cowork (Cloud) row against the last
// measured time: a period that starts after it was not measured, a period that
// holds it is partial.
func cloudNote(periodStart, last time.Time) string {
	if periodStart.After(last) {
		return NoteNotMeasured
	}
	return NotePartial
}

// CreditsLeft is C3/K3's account-wide "credits left" figure, present only
// when Config.CopilotPlan names a plan the credit book covers.
type CreditsLeft struct {
	Plan           string  `json:"plan"`
	MonthlyCredits float64 `json:"monthly_credits"`
	Left           float64 `json:"left"`
}

// Payload is bmVendorStrip's return value.
type Payload struct {
	GeneratedAt string `json:"generated_at"`
	Rows        []Row  `json:"rows"`
	Total       Row    `json:"total"`
	// CloudMeasuredUntil is the local date and time of the newest Cowork
	// (Cloud) drop row, for the tooltip on a marked cell. Empty when there is
	// no drop row, and then no cell carries a note.
	CloudMeasuredUntil string `json:"cloud_measured_until,omitempty"`
	// CopilotCreditsLeft is refreshed on the same 1-minute cadence as the
	// rest of this payload (rather than bmLive's 2-second poll, or a fourth
	// backend call the business-mode card would otherwise need): an
	// account-wide plan balance does not need sub-minute freshness. nil when
	// Config.CopilotPlan is unset or unknown.
	CopilotCreditsLeft *CreditsLeft `json:"copilot_credits_left,omitempty"`
}

// dayStart, weekStart and monthStart are LOCAL calendar boundaries (Wilco's
// decision, 2026-09-26: local time everywhere): dayStart is local midnight
// today, weekStart is local Monday 00:00 of the current ISO week (same week
// convention as internal/agg's weekKey and internal/history's "week"
// period), monthStart is the 1st of the current month, local 00:00.
// time.Date with time.Local resolves to the correct UTC instant for that
// specific local calendar date on its own, DST included (a 23h or 25h day
// around the spring/autumn transition changes what instant "local midnight"
// is, not this arithmetic), so no separate DST handling is needed here; the
// result is converted back to UTC only where it is compared against the
// UTC-stored at column (store.VendorStripTotals below).
func dayStart(now time.Time) time.Time {
	now = now.In(time.Local)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
}

func weekStart(now time.Time) time.Time {
	d := dayStart(now)
	// time.Weekday: Sunday=0 ... Saturday=6; ISO weeks start Monday.
	offset := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -offset)
}

func monthStart(now time.Time) time.Time {
	now = now.In(time.Local)
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
}

// Build runs store.VendorStripTotals (one SQL query, grouped by agent) and
// shapes it into Payload, sorted by AgentLabel and with the total row summed
// across every vendor. cfg (added C3/K3) is only used for CopilotCreditsLeft
// below; the token totals themselves are unaffected by it.
func Build(st *store.Store, cfg *pricing.Config, now time.Time) (Payload, error) {
	totals, err := st.VendorStripTotals(dayStart(now), weekStart(now), monthStart(now))
	if err != nil {
		return Payload{}, err
	}

	last, haveLast := cloudLastMeasured()

	rows := make([]Row, 0, len(totals))
	var total Row
	for _, t := range totals {
		label := AgentLabel[t.Agent]
		if label == "" {
			label = t.Agent
		}
		r := Row{Agent: t.Agent, AgentLabel: label, Today: t.Today, Week: t.Week, Month: t.Month}
		if haveLast && t.Agent == "cowork-cloud" {
			r.TodayNote = cloudNote(dayStart(now), last)
			r.WeekNote = cloudNote(weekStart(now), last)
			r.MonthNote = cloudNote(monthStart(now), last)
			if r.TodayNote == NotePartial {
				total.TodayNote = NotePartial
			}
			if r.WeekNote == NotePartial {
				total.WeekNote = NotePartial
			}
			if r.MonthNote == NotePartial {
				total.MonthNote = NotePartial
			}
		}
		rows = append(rows, r)
		total.Today += t.Today
		total.Week += t.Week
		total.Month += t.Month
	}
	total.AgentLabel = "Total"

	sort.Slice(rows, func(i, j int) bool { return rows[i].AgentLabel < rows[j].AgentLabel })

	p := Payload{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Rows:        rows,
		Total:       total,
	}
	if haveLast {
		p.CloudMeasuredUntil = last.In(time.Local).Format("2006-01-02 15:04")
	}
	if cl, ok := creditsLeftForMonth(st, cfg, now); ok {
		p.CopilotCreditsLeft = &cl
	}
	return p, nil
}

// utcMonthStart is GitHub's own Copilot premium-request allowance reset
// boundary (creditsLeftForMonth below), deliberately kept separate from
// monthStart's local calendar: that reset happens on GitHub's own UTC
// calendar, a third-party billing boundary Wilco's own local time zone has
// no bearing on, not a display convention this session's "local time
// everywhere" decision was ever about. A fresh review caught that sharing
// monthStart (switched to time.Local for the display totals above) here
// too would have silently shifted the last one to two hours of every UTC
// month into the wrong month's credits count.
func utcMonthStart(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// creditsLeftForMonth re-reads this calendar month's events (a second query
// beyond VendorStripTotals: the credits calculation needs cache-write/read
// splits per event, which the token-sum SQL aggregate above does not carry)
// and runs cfg.CopilotCreditsLeft over them. Only called at all when
// Config.CopilotPlan is set, so a burnmon.json with no Copilot plan
// configured never pays this extra query.
func creditsLeftForMonth(st *store.Store, cfg *pricing.Config, now time.Time) (CreditsLeft, bool) {
	if cfg == nil || cfg.CopilotPlan == "" {
		return CreditsLeft{}, false
	}
	events, err := st.EventsSince(utcMonthStart(now))
	if err != nil {
		return CreditsLeft{}, false
	}
	left, plan, ok := cfg.CopilotCreditsLeft(events)
	if !ok {
		return CreditsLeft{}, false
	}
	return CreditsLeft{Plan: cfg.CopilotPlan, MonthlyCredits: plan.MonthlyCredits, Left: left}, true
}
