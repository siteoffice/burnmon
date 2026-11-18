//go:build darwin && cgo

package main

import "os"

// runLampWindow is a stub until the Mac lamp is built (plan C19, turn T8):
// the lamp subcommand builds on a Mac with cgo and says what the other
// platforms say.
func runLampWindow(lampOpts) int { return lampUnsupported(os.Stderr) }
