//go:build windows

package main

import "golang.org/x/sys/windows"

// openItemFile opens path with the OS default handler. ShellExecute takes the
// path as one argument, so no shell parses it. Tests swap it for a recorder.
var openItemFile = func(path string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
