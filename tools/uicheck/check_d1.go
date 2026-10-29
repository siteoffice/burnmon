package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// d1: the header EXPORT button actually writes an export bundle. Clicks
// #exportBtn through the dev eval channel (its own addEventListener path,
// a genuine functional check, same reasoning check_w2.go's own doc comment
// gives for using el.click() over hardware SendInput here), waits for
// #exportStatus to report success, then confirms all three files exist on
// disk with a positive size (the binding's own dataDir\exports\ folder,
// design doc's bdevExport).
func init() {
	checks["d1"] = func(hwnd uintptr, args []string) error {
		if err := ensureWindowSizeWH(hwnd, 1152, 2048); err != nil {
			return err
		}

		if _, err := evalRaw(`document.getElementById('exportBtn').click()`); err != nil {
			return fmt.Errorf("click export button: %w", err)
		}

		var status string
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			s, err := evalString(`document.getElementById('exportStatus').textContent`)
			if err != nil {
				return fmt.Errorf("read export status: %w", err)
			}
			status = s
			if strings.Contains(status, "exported") || strings.Contains(status, "failed") {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Println("uicheck: export status:", status)
		if !strings.Contains(status, "exported") {
			return fmt.Errorf("export did not report success within 30s, last status: %q", status)
		}

		sd, err := devSettingsDir()
		if err != nil {
			return err
		}
		exportsDir := filepath.Join(sd, "exports")
		entries, err := os.ReadDir(exportsDir)
		if err != nil {
			return fmt.Errorf("read %s: %w", exportsDir, err)
		}
		var newest os.DirEntry
		var newestMod time.Time
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "burnmon-dev_") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(newestMod) {
				newestMod = info.ModTime()
				newest = e
			}
		}
		if newest == nil {
			return fmt.Errorf("no burnmon-dev_* export folder found under %s", exportsDir)
		}
		dir := filepath.Join(exportsDir, newest.Name())
		for _, f := range []string{"summary.md", "data.json", "daily.csv"} {
			fi, err := os.Stat(filepath.Join(dir, f))
			if err != nil {
				return fmt.Errorf("%s: %w", f, err)
			}
			if fi.Size() <= 0 {
				return fmt.Errorf("%s is empty", f)
			}
			fmt.Printf("uicheck: %s: %d bytes\n", f, fi.Size())
		}

		if _, err := screenshot(hwnd, "d1-export-done"); err != nil {
			return err
		}
		return nil
	}
}
