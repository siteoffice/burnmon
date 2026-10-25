//go:build windows

// item_files.go is the Go side of the spec and result links in Waits on you
// and Outcomes. The page names a file; this code opens it only when the newest
// snapshot lists that exact path as a spec_path or result_path of a wait or
// outcome item. The page input is untrusted: nothing else is opened.
package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"burnmon/internal/snapshot"
)

// itemFileExts are the only file types a link may open. Everything else,
// executables and scripts included, is refused even when the snapshot lists it.
var itemFileExts = map[string]bool{".md": true, ".html": true, ".txt": true}

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

// plainDrivePath reports whether p is `X:\...` with no UNC prefix, device
// prefix (`\.\`, `\?\`) or stream (`:` after the drive letter).
func plainDrivePath(p string) bool {
	if len(p) < 4 || p[1] != ':' || p[2] != '\\' {
		return false
	}
	c := p[0]
	if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		return false
	}
	return !strings.ContainsAny(p[3:], ":?*<>|\"")
}

// itemFileAllowed returns nil only when the snapshot at snapPath reads ok at
// now, lists path (compared cleaned and case-folded) as a spec or result path,
// the path is a plain absolute drive path, ends in an allowed type, and names
// an existing regular file.
func itemFileAllowed(snapPath string, now time.Time, path string) error {
	r := snapshot.Load(snapPath, now)
	if r.Status != snapshot.StatusOK || r.Snapshot == nil {
		return errors.New("snapshot is not current")
	}
	want := strings.ToLower(filepath.Clean(path))
	if strings.TrimSpace(path) == "" {
		return errors.New("path is not in the snapshot")
	}
	var listed string
	for _, list := range [][]struct{ spec, result string }{pathsOf(r.Snapshot)} {
		for _, it := range list {
			for _, p := range []string{it.spec, it.result} {
				if p != "" && strings.ToLower(filepath.Clean(p)) == want {
					listed = filepath.Clean(p)
				}
			}
		}
	}
	if listed == "" {
		return errors.New("path is not in the snapshot")
	}
	if !plainDrivePath(listed) {
		return errors.New("path is not absolute")
	}
	if !itemFileExts[strings.ToLower(filepath.Ext(listed))] {
		return errors.New("file type not allowed")
	}
	fi, err := os.Stat(listed)
	if err != nil || !fi.Mode().IsRegular() {
		return errors.New("not a file")
	}
	// A link or junction must not lead to a type that is not allowed.
	if real, err := filepath.EvalSymlinks(listed); err == nil && !itemFileExts[strings.ToLower(filepath.Ext(real))] {
		return errors.New("file type not allowed")
	}
	return nil
}

func pathsOf(s *snapshot.Snapshot) []struct{ spec, result string } {
	var out []struct{ spec, result string }
	for _, it := range s.WaitItems {
		out = append(out, struct{ spec, result string }{it.SpecPath, it.ResultPath})
	}
	for _, it := range s.OutcomeItems {
		out = append(out, struct{ spec, result string }{it.SpecPath, it.ResultPath})
	}
	return out
}

// openItemFileChecked is what the binding calls: check, then open the
// snapshot's own spelling of the path (not the page's).
func openItemFileChecked(snapPath string, now time.Time, path string) error {
	if err := itemFileAllowed(snapPath, now, path); err != nil {
		return err
	}
	return openItemFile(filepath.Clean(path))
}

func bdevOpenItemFile(snapPath, path string) error {
	err := openItemFileChecked(snapPath, time.Now(), path)
	if err != nil {
		log.Println("bdevOpenItemFile:", err)
	}
	return err
}
