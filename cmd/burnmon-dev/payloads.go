//go:build windows || darwin

package main

import (
	"log"
	"time"

	"burnmon/internal/history"
	"burnmon/internal/todo"
)

// Microsoft To Do panel payloads (section 9). Kept small and purpose-built
// per call rather than reusing internal/todo's own structs directly: the
// unexported deviceCode/interval/expiresIn fields on todo.DeviceCodeInfo
// never need to reach the page at all (main.go keeps the whole struct
// server-side to hand to FinishLogin).
type todoStatusPayload struct {
	Enabled  bool `json:"enabled"`
	SignedIn bool `json:"signed_in"`
}

type todoLoginStartPayload struct {
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Message         string `json:"message"`
}

type todoLoginDonePayload struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type todoTasksPayload struct {
	Items []todo.Item `json:"items"`
	Error string      `json:"error,omitempty"`
}

// progStart marks process entry, so every startupMark call below logs an
// elapsed time from the same zero point (section 11 step 1: "measure
// first" before changing any startup ordering). Read only from main's own
// goroutine before the window is up; the sampling goroutine's one
// first-sample mark (startSampling) reads it too, but only ever after
// main() has already set it, so no lock is needed.
var progStart = time.Now()

func startupMark(step string) {
	log.Printf("startup: %s at %v", step, time.Since(progStart).Round(time.Millisecond))
}

// heatmapPayload is bdevActivityHeatmap's return shape: renderHeatmap
// (page.html) only ever reads payload.rows, so this carries just that
// field rather than the rest of history.Payload (Totals, Vendors, Owners),
// which the merged closed+today rows below don't populate meaningfully.
type heatmapPayload struct {
	Rows []history.Row `json:"rows"`
}
