//go:build !windows

package main

// The shell group's d0, d1, d24 and d32 drive the real Win32 window, so on
// other systems they only report that (maccases.go lists what does run).

func caseD0(uintptr, []string) error  { return errNotOnThisOS }
func caseD1(uintptr, []string) error  { return errNotOnThisOS }
func caseD24(uintptr, []string) error { return errNotOnThisOS }
func caseD32(uintptr, []string) error { return errNotOnThisOS }
