// Package lamp works out what the lamp shows. The state is a pure function of
// the snapshot, a clock, the settings and the recent burn rates; the only
// file it reads is the snapshot.
package lamp

import "encoding/json"

// Settings are the six lamp keys of burnmon-dev.json.
type Settings struct {
	Enabled      bool   // lamp_enabled
	Position     string // lamp_position: top-center, top-left or top-right
	Language     string // lamp_language: en or nl
	Lines        bool   // lamp_lines
	ForemanItems bool   // lamp_foreman_items
	BurnFloorTPM int    // lamp_burn_floor_tpm, a first guess tuned at the first look
}

// DefaultSettings is what a missing file or a bad value gives.
func DefaultSettings() Settings {
	return Settings{Position: "top-center", Language: "en", Lines: true, ForemanItems: true, BurnFloorTPM: 20000}
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
	return s
}
