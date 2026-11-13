// Package centre builds the Control Centre state: units, files, flow and
// sprint rows, the plan and counts, read from a root folder. It has no
// platform code, no network and no clock of its own.
package centre

import "time"

// All paths in the state are relative to root with forward slashes; only
// Root is absolute. These types are the contract for the later hosted
// version; field names and JSON tags are fixed.

type State struct {
	SchemaVersion int            `json:"schema_version"` // 1
	Root          string         `json:"root"`           // absolute
	Now           string         `json:"now"`            // RFC3339 UTC
	Today         string         `json:"today"`          // YYYY-MM-DD in now's location
	RowsPerPage   int            `json:"rows_per_page"`
	Units         []Unit         `json:"units"`   // sorted by id, "." first
	Flows         []FlowRow      `json:"flows"`   // SortFlows order
	Sprints       []SprintRow    `json:"sprints"` // SortSprints order
	Waiting       List           `json:"waiting"` // WaitFlow+key of waiting flow rows (SortFlows order), then WaitSprint+key of waiting sprint rows (SortSprints order)
	Overview      Overview       `json:"overview"`
	Plan          *Plan          `json:"plan"`      // null when absent or refused
	PlanPath      string         `json:"plan_path"` // "siteoffice.plan.json"
	Badges        map[string]int `json:"badges"`    // plan item id -> waiting flow and sprint rows, only > 0
	Planning      Planning       `json:"planning"`
	NextFixed     *PlanDate      `json:"next_fixed"`
	Problems      []Problem      `json:"problems"`
	Counts        Counts         `json:"counts"`
}

// Span is a half-open index range into List.Keys.
type Span struct {
	From int `json:"from"`
	To   int `json:"to"`
}
type List struct {
	Keys  []string `json:"keys"`
	Pages []Span   `json:"pages"`
}

type Unit struct {
	ID          string   `json:"id"` // "." or "projects/kettle"
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Type        string   `json:"type"`
	Parent      string   `json:"parent"`   // parent unit id, "" for none
	Level       int      `json:"level"`    // tree depth via Parent, 0 for a unit without parent
	Depth       int      `json:"depth"`    // folder depth below root, 0 for "."
	Children    []string `json:"children"` // unit ids, sorted
	Item        string   `json:"item"`     // first plan item whose unit names this unit, else ""
	Files       []File   `json:"files"`
	Folders     []Folder `json:"folders"`
	FilePages   []Span   `json:"file_pages"`
	Flows       List     `json:"flows"`
	Sprints     List     `json:"sprints"`
}

type File struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // RFC3339 UTC
	AgeDays  int    `json:"age_days"`
	State    string `json:"state"` // "stub", "stale" or "ok"
}
type Folder struct {
	Name    string `json:"name"`
	Entries int    `json:"entries"`
}

type FlowRow struct {
	Key       string `json:"key"` // project + "/" + slug
	Project   string `json:"project"`
	Slug      string `json:"slug"`
	Stage     string `json:"stage"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
	WaitsDays int    `json:"waits_days"`
	Waiting   bool   `json:"waiting"`
	Closed    bool   `json:"closed"`
	Unit      string `json:"unit"`       // owning unit id
	Index     string `json:"index"`      // index.json the row came from
	Board     string `json:"board"`      // board page path, "" when the file is absent
	StateText string `json:"state_text"` // StaleText for a stale flow status, else ""
}
type SprintRow struct {
	Key       string `json:"key"`
	Project   string `json:"project"`
	Slug      string `json:"slug"`
	Status    string `json:"status"`
	Phase     string `json:"phase"`
	Progress  int    `json:"progress"`
	UpdatedAt string `json:"updated_at"`
	Closed    bool   `json:"closed"`
	Unit      string `json:"unit"`
	Index     string `json:"index"`
	Board     string `json:"board"`
	WaitsDays int    `json:"waits_days"`
	Waiting   bool   `json:"waiting"`
}

type Overview struct {
	PlanColumns []Column `json:"plan_columns"` // "Goals", "Plan", "Products and projects"; nil without plan
	UnitColumns []Column `json:"unit_columns"` // one per Level: "Root", "Level 1", "Level 2", ...
	Default     string   `json:"default"`      // "plan" when Plan != nil, else "units"
}
type Column struct {
	Title string `json:"title"`
	List
}

type Plan struct {
	SchemaVersion int        `json:"schemaVersion"`
	Goals         []PlanNode `json:"goals"`
	Lines         []PlanNode `json:"lines"`
	Items         []PlanNode `json:"items"`
	Links         []PlanLink `json:"links"`
	Dates         []PlanDate `json:"dates"`
	WeekNotes     []WeekNote `json:"weekNotes"`
}
type PlanNode struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Kind   string     `json:"kind"`
	Note   string     `json:"note"`
	Unit   string     `json:"unit,omitempty"`
	UnitID string     `json:"unit_id,omitempty"` // resolved by Build
	Hold   bool       `json:"hold,omitempty"`
	Parts  []PlanPart `json:"parts,omitempty"`
}
type PlanPart struct {
	Title  string `json:"title"`
	Role   string `json:"role"`
	Note   string `json:"note"`
	Unit   string `json:"unit,omitempty"`
	UnitID string `json:"unit_id,omitempty"`
}
type PlanLink struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
	Step     bool   `json:"step,omitempty"`
}
type PlanDate struct {
	Date   string   `json:"date"`
	Time   string   `json:"time,omitempty"`
	Title  string   `json:"title"`
	Short  string   `json:"short,omitempty"`
	Note   string   `json:"note,omitempty"`
	IDs    []string `json:"ids,omitempty"`
	Fixed  bool     `json:"fixed,omitempty"`
	Source string   `json:"source,omitempty"`
}
type WeekNote struct {
	Week string `json:"week"`
	Text string `json:"text"`
}

type Planning struct {
	Weeks  int        `json:"weeks"`
	Monday string     `json:"monday"`
	Today  string     `json:"today"`
	Rows   []PlanWeek `json:"rows"`
}
type PlanWeek struct {
	Week  string    `json:"week"`  // "2026-W46"
	Label string    `json:"label"` // "W46"
	Notes []string  `json:"notes"`
	Days  []PlanDay `json:"days"` // 7, Monday first
}
type PlanDay struct {
	Date    string     `json:"date"`    // "2026-11-09"
	Label   string     `json:"label"`   // "9 Nov"
	Weekday string     `json:"weekday"` // "Mon"
	Today   bool       `json:"today"`
	Weekend bool       `json:"weekend"`
	Dates   []PlanDate `json:"dates"` // by time, then title
}

type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}
type Counts struct {
	Units           int `json:"units"`
	Flows           int `json:"flows"`
	Sprints         int `json:"sprints"`
	Waiting         int `json:"waiting"`
	LongestWaitDays int `json:"longest_wait_days"`
	Problems        int `json:"problems"`
}

type Options struct {
	Now   time.Time
	Weeks int // 2 or 4; anything else means 2
	Rows  int // rows per page; < 1 means 12
}
