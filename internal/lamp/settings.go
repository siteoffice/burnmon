// Package lamp works out what the lamp shows. The state is a pure function of
// the snapshot, a clock, the settings and the recent burn rates; the only
// file it reads is the snapshot.
package lamp

import "encoding/json"

// Settings are the twelve lamp keys of burnmon-dev.json.
type Settings struct {
	Enabled      bool   // lamp_enabled
	Position     string // lamp_position: top-center, top-left or top-right
	Language     string // lamp_language: en or nl
	Lines        bool   // lamp_lines
	ForemanItems bool   // lamp_foreman_items
	BurnFloorTPM int    // lamp_burn_floor_tpm, a first guess tuned at the first look
	TickerRows   int    // lamp_ticker_rows: 1 to 4
	SizePct      int    // lamp_size as a percentage, 100 to 200; the old words load as 120, 150 and 190
	Theme        string // lamp_theme: system, light or dark
	Motion       string // lamp_motion: lively, calm or off
	Quiet        string // lamp_quiet: hide (sliver, Glim up) or stay (Glim hangs)
	Backdrop     string // lamp_backdrop: clear or card
	Monitor      string // lamp_monitor: device name of the monitor the lamp hangs on; empty is the primary
}

// SizeScale is Glim's size factor for a lamp_size value; a bad value is medium.
func SizeScale(size string) float64 {
	switch size {
	case "small":
		return 1.2
	case "large":
		return 1.9
	}
	return 1.5
}

// PctScale is Glim's size factor for a lamp_size percentage; a value outside
// 100 to 200 is 150 percent.
func PctScale(pct int) float64 {
	if pct < 100 || pct > 200 {
		pct = 150
	}
	return float64(pct) / 100
}

func oneOf(v string, choices ...string) bool {
	for _, c := range choices {
		if v == c {
			return true
		}
	}
	return false
}

// DefaultSettings is what a missing file or a bad value gives.
func DefaultSettings() Settings {
	return Settings{Position: "top-center", Language: "en", Lines: true, ForemanItems: true, BurnFloorTPM: 20000,
		TickerRows: 1, SizePct: 150, Theme: "system", Motion: "lively", Quiet: "hide", Backdrop: "clear"}
}

// ParseSettings reads the lamp keys from the bytes of burnmon-dev.json. A
// missing key or a bad value (wrong type, unknown choice, a negative or
// fractional floor) becomes its default, key by key.
func ParseSettings(b []byte) Settings {
	s := DefaultSettings()
	var raw map[string]json.RawMessage
	if json.Unmarshal(b, &raw) != nil {
		return s
	}
	for k, v := range raw {
		if string(v) == "null" { // null unmarshals into a zero value, not an error
			delete(raw, k)
		}
	}
	var bv bool
	var sv string
	var fv float64
	if json.Unmarshal(raw["lamp_enabled"], &bv) == nil {
		s.Enabled = bv
	}
	if json.Unmarshal(raw["lamp_position"], &sv) == nil && (sv == "top-center" || sv == "top-left" || sv == "top-right") {
		s.Position = sv
	}
	if json.Unmarshal(raw["lamp_language"], &sv) == nil && (sv == "en" || sv == "nl") {
		s.Language = sv
	}
	if json.Unmarshal(raw["lamp_lines"], &bv) == nil {
		s.Lines = bv
	}
	if json.Unmarshal(raw["lamp_foreman_items"], &bv) == nil {
		s.ForemanItems = bv
	}
	if json.Unmarshal(raw["lamp_burn_floor_tpm"], &fv) == nil && fv >= 0 && fv == float64(int(fv)) {
		s.BurnFloorTPM = int(fv)
	}
	if json.Unmarshal(raw["lamp_ticker_rows"], &fv) == nil && fv >= 1 && fv <= 4 && fv == float64(int(fv)) {
		s.TickerRows = int(fv)
	}
	if json.Unmarshal(raw["lamp_size"], &sv) == nil && oneOf(sv, "small", "medium", "large") {
		s.SizePct = map[string]int{"small": 120, "medium": 150, "large": 190}[sv]
	} else if json.Unmarshal(raw["lamp_size"], &fv) == nil && fv >= 100 && fv <= 200 && fv == float64(int(fv)) {
		s.SizePct = int(fv)
	}
	if json.Unmarshal(raw["lamp_theme"], &sv) == nil && oneOf(sv, "system", "light", "dark") {
		s.Theme = sv
	}
	if json.Unmarshal(raw["lamp_motion"], &sv) == nil && oneOf(sv, "lively", "calm", "off") {
		s.Motion = sv
	}
	if json.Unmarshal(raw["lamp_quiet"], &sv) == nil && oneOf(sv, "hide", "stay") {
		s.Quiet = sv
	}
	if json.Unmarshal(raw["lamp_backdrop"], &sv) == nil && oneOf(sv, "clear", "card") {
		s.Backdrop = sv
	}
	if json.Unmarshal(raw["lamp_monitor"], &sv) == nil {
		s.Monitor = sv
	}
	return s
}
