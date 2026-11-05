//go:build !windows

package main

import (
	"errors"
	"image"
)

// Stubs for the Win32 helpers so the DOM-only cases (maccases.go) build and
// run on macOS. Every case that needs a real window screenshot, click or key
// press returns errNotOnThisOS there.

var errNotOnThisOS = errors.New("needs Win32: not on this OS")

const (
	vkEscape = 0x1B
	vkF11    = 0x7A
	vkL      = 0x4C
)

// windowCapped mirrors win32.go: never capped, nothing is resized here.
var windowCapped bool

type noProc struct{}

// Call matches the shape of (*windows.LazyProc).Call and reports no window.
func (noProc) Call(...uintptr) (uintptr, uintptr, error) { return 0, 0, nil }

var procGetForegroundWindow noProc

func findBurnmonWindow() (uintptr, error)    { return 0, nil }
func findBurnmonDevWindow() (uintptr, error) { return 0, nil }
func findBurnmonWindowFast() uintptr         { return 0 }
func isRealBurnmonWindow(uintptr) bool       { return false }
func windowTitleText(uintptr) string         { return "" }

func ensureWindowSize(uintptr) error                 { return nil }
func ensureWindowSizeWH(uintptr, int32, int32) error { return nil }
func bringToFront(uintptr)                           {}

func screenshot(uintptr, string) (image.Image, error) { return nil, errNotOnThisOS }
func looksBlankWhite(image.Image) bool                { return false }
func screenCoords(uintptr, float64, float64) (int32, int32, error) {
	return 0, 0, errNotOnThisOS
}
func clickAt(int32, int32) error              { return errNotOnThisOS }
func dragAt(int32, int32, int32, int32) error { return errNotOnThisOS }
func pressKey(uint16) error                   { return errNotOnThisOS }
