//go:build windows

// item_files.go is the Go side of the spec and result links in Waits on you
// and Outcomes. The page names a file; this code opens it only when the newest
// snapshot lists that exact path as a spec_path, result_path or report_path of a wait or
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

// asciiFold lowercases A-Z only. Unicode folding is not used: the Kelvin sign
// (U+212A) lowercases to "k" there, so a look-alike would match a listed path.
func asciiFold(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// itemFileAllowed returns the snapshot's own spelling of path (cleaned) and a
// nil error only when the snapshot at snapPath reads ok at now, lists path
// (compared cleaned and ASCII case-folded) as a spec, result or report path, the
// listed path is a plain absolute drive path under 260 characters, ends in an
// allowed type, and names an existing regular file that is not a link. The
// caller must open the returned string, never the page's.
func itemFileAllowed(snapPath string, now time.Time, path, foremanRoot string) (string, error) {
	r := snapshot.Load(snapPath, now)
	if r.Status != snapshot.StatusOK || r.Snapshot == nil {
		return "", errors.New("snapshot is not current")
	}
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is not in the snapshot")
	}
	want := asciiFold(filepath.Clean(path))
	var listed string
	onlyReport := true
	for _, it := range pathsOf(r.Snapshot) {
		for i, p := range []string{it.spec, it.result, it.report} {
			if p != "" && asciiFold(filepath.Clean(p)) == want {
				listed = filepath.Clean(p)
				onlyReport = onlyReport && i == 2
			}
		}
	}
	if listed == "" {
		return "", errors.New("path is not in the snapshot")
	}
	if !plainDrivePath(listed) {
		return "", errors.New("path is not absolute")
	}
	if len(listed) >= 260 {
		return "", errors.New("path is too long")
	}
	if !itemFileExts[asciiFold(filepath.Ext(listed))] {
		return "", errors.New("file type not allowed")
	}
	li, err := os.Lstat(listed)
	if err != nil || li.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !li.Mode().IsRegular() {
		return "", errors.New("not a file")
	}
	// Fail closed: a resolution error refuses, and the real target must also
	// be an allowed type (a parent folder may be a link or junction).
	real, err := filepath.EvalSymlinks(listed)
	if err != nil {
		return "", errors.New("not a file")
	}
	if !itemFileExts[asciiFold(filepath.Ext(real))] {
		return "", errors.New("file type not allowed")
	}
	if err := itemFileContained(real, foremanRoot, onlyReport); err != nil {
		return "", err
	}
	return listed, nil
}

const errOutsideForeman = "file is outside the Foreman folders"

// itemFileContained keeps a link inside the Foreman folders. real is the path
// after EvalSymlinks, so a link or junction in any parent cannot lead out. With
// foreman_root set the file must sit in <root>\sprints or <root>\flows, or in a
// folder named "specs" (where Foreman's spec files live, in the hub and in the
// project folders; BurnMon has no configured spec root, so that name is the
// rule). Without foreman_root a report path is refused and a spec or result
// path keeps the earlier rule: the snapshot must list it.
func itemFileContained(real, foremanRoot string, onlyReport bool) error {
	if foremanRoot == "" {
		if onlyReport {
			return errors.New(errOutsideForeman)
		}
		return nil
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(foremanRoot))
	if err != nil || !plainDrivePath(root) {
		return errors.New(errOutsideForeman)
	}
	f := asciiFold(real)
	for _, sub := range []string{"sprints", "flows"} {
		if strings.HasPrefix(f, asciiFold(filepath.Join(root, sub))+`\`) {
			return nil
		}
	}
	dir := filepath.Dir(f)
	for _, part := range strings.Split(dir, `\`) {
		if part == "specs" {
			return nil
		}
	}
	return errors.New(errOutsideForeman)
}

// pathsOf lists the paths the snapshot names per item: spec, result and, for
// a close_out row, the report whose parked lines the row links.
func pathsOf(s *snapshot.Snapshot) []struct{ spec, result, report string } {
	var out []struct{ spec, result, report string }
	for _, it := range s.WaitItems {
		out = append(out, struct{ spec, result, report string }{it.SpecPath, it.ResultPath, it.ReportPath})
	}
	for _, it := range s.OutcomeItems {
		out = append(out, struct{ spec, result, report string }{it.SpecPath, it.ResultPath, ""})
	}
	return out
}

// openItemFileChecked is what the binding calls: check, then open the
// snapshot's own spelling of the path (not the page's).
func openItemFileChecked(snapPath string, now time.Time, path, foremanRoot string) error {
	listed, err := itemFileAllowed(snapPath, now, path, foremanRoot)
	if err != nil {
		return err
	}
	return openItemFile(listed)
}

func bdevOpenItemFile(snapPath, path, foremanRoot string) error {
	err := openItemFileChecked(snapPath, time.Now(), path, foremanRoot)
	if err != nil {
		log.Println("bdevOpenItemFile:", err)
	}
	return err
}
