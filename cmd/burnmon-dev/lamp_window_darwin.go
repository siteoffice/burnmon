//go:build darwin && cgo

package main

import (
	"os"

	"burnmon/internal/lamp"
)

// runLampWindow is a stub until the Mac lamp is built (plan C19, turn T8):
// the lamp subcommand builds on a Mac with cgo and says what the other
// platforms say.
func runLampWindow(lampOpts) int { return lampUnsupported(os.Stderr) }

// runCardWindow is a stub for the same reason: the card window runs on
// Windows only in this version (the SVG cards of --card are written anyway).
func runCardWindow(string, lamp.Week, string) int { return lampUnsupported(os.Stderr) }
