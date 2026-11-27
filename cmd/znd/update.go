package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"burnmon/internal/controls"
)

// version is the program version, printed by --version. The update route
// itself belongs to the update grill, not to this program.
const version = "0.1.0"

// auditEnv is what the audit-writing commands need from the machine.
type auditEnv struct {
	Path string
	Now  func() time.Time
}

const updateUsage = "usage: znd update-mods --from <folder> --to <mod folder> (--dry-run | --yes)"

// modsManifest is herdr-app\mods.json: per mod folder name, a SHA-256 for each
// file path, written with forward slashes and relative to the mod folder.
type modsManifest struct {
	Mods map[string]struct {
		Files map[string]string `json:"files"`
	} `json:"mods"`
}

func readManifest(path string) (modsManifest, error) {
	var m modsManifest
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

func fileHash(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// safeName reports whether s is one plain path element.
func safeName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\:`) && !strings.HasPrefix(s, "-")
}

// safeRel reports whether s is a relative forward-slash path that stays inside its folder.
func safeRel(s string) bool {
	if s == "" || strings.HasPrefix(s, "/") || strings.ContainsAny(s, `\:`) {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if !safeName(part) {
			return false
		}
	}
	return true
}

// updStep is one planned action, printed by --dry-run and run by --yes.
type updStep struct {
	kind, mod, rel string // keep, aside, copy
}

func (s updStep) String() string {
	switch s.kind {
	case "keep":
		return fmt.Sprintf("keep %s/%s (edited by you) in znd-kept", s.mod, s.rel)
	case "aside":
		return fmt.Sprintf("set aside the old folder %s in znd-old", s.mod)
	}
	return fmt.Sprintf("copy %s/%s", s.mod, s.rel)
}

// runUpdateMods replaces our mod folders from a local folder. A file you edited
// moves to znd-kept, the old folder is renamed aside, and nothing is deleted.
// It makes no network call. Every check on the source runs before any change.
func runUpdateMods(args []string, stdout, stderr io.Writer, d deps) int {
	var from, to string
	var dry, yes bool
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--dry-run":
			dry = true
		case "--yes":
			yes = true
		case "--from", "--to":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "missing value for %s\n%s\n", a, updateUsage)
				return exitUsage
			}
			i++
			if a == "--from" {
				from = args[i]
			} else {
				to = args[i]
			}
		default:
			fmt.Fprintf(stderr, "unknown flag: %s\n%s\n", a, updateUsage)
			return exitUsage
		}
	}
	if from == "" || to == "" || dry == yes {
		fmt.Fprintln(stderr, "give --from, --to and exactly one of --dry-run and --yes\n"+updateUsage)
		return exitUsage
	}
	man, err := readManifest(filepath.Join(from, "herdr-app", "mods.json"))
	if err != nil {
		fmt.Fprintln(stderr, "refused: cannot read herdr-app\\mods.json in --from: "+err.Error())
		return exitRefused
	}
	parent := filepath.Dir(filepath.Clean(to))
	installedPath := filepath.Join(parent, "znd-installed.json")
	installed, _ := readManifest(installedPath) // absent is fine: no baseline yet

	names := make([]string, 0, len(man.Mods))
	for n := range man.Mods {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(stderr, "refused: the manifest names no mod")
		return exitRefused
	}

	// Plan, checking the whole source first.
	var steps []updStep
	var edits []updStep
	for _, n := range names {
		if !safeName(n) {
			fmt.Fprintf(stderr, "refused: bad mod folder name %q\n", n)
			return exitRefused
		}
		files := man.Mods[n].Files
		rels := make([]string, 0, len(files))
		for rel, want := range files {
			if !safeRel(rel) {
				fmt.Fprintf(stderr, "refused: bad file path %q in mod %s\n", rel, n)
				return exitRefused
			}
			got, err := fileHash(filepath.Join(from, n, filepath.FromSlash(rel)))
			if err != nil {
				fmt.Fprintf(stderr, "refused: cannot read %s/%s in --from: %v\n", n, rel, err)
				return exitRefused
			}
			if got != want {
				fmt.Fprintf(stderr, "refused: hash of %s/%s differs from the manifest\n", n, rel)
				return exitRefused
			}
			rels = append(rels, rel)
		}
		sort.Strings(rels)
		dst := filepath.Join(to, n)
		if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
			base := installed.Mods[n].Files
			if base == nil {
				base = files // no installed manifest: anything unlike the new files counts as an edit
			}
			var found []string
			filepath.WalkDir(dst, func(p string, e fs.DirEntry, err error) error {
				if err == nil && !e.IsDir() {
					rel, _ := filepath.Rel(dst, p)
					rel = filepath.ToSlash(rel)
					if h, herr := fileHash(p); herr != nil || h != base[rel] {
						found = append(found, rel)
					}
				}
				return nil
			})
			sort.Strings(found)
			for _, rel := range found {
				st := updStep{"keep", n, rel}
				steps = append(steps, st)
				edits = append(edits, st)
			}
			steps = append(steps, updStep{"aside", n, ""})
		}
		for _, rel := range rels {
			steps = append(steps, updStep{"copy", n, rel})
		}
	}

	for _, s := range steps {
		fmt.Fprintln(stdout, s)
	}
	if dry {
		return exitOK
	}

	env := d.audit()
	af, err := controls.OpenAudit(env.Path)
	if err != nil {
		fmt.Fprintln(stderr, "refused: the audit line cannot be written, nothing runs: "+err.Error())
		return exitRefused
	}
	defer af.Close()
	cmd := fmt.Sprintf("update-mods --from %s --to %s (%d steps)", from, to, len(steps))
	start := controls.AuditLine{Time: env.Now().UTC().Format(time.RFC3339), Control: "update_mods", Target: to, Command: cmd, Phase: "start"}
	if err := controls.WriteAudit(af, start); err != nil {
		fmt.Fprintln(stderr, "refused: the audit line cannot be written, nothing runs: "+err.Error())
		return exitRefused
	}

	result := controls.AuditLine{Time: start.Time, Control: "update_mods", Target: to, Command: cmd}
	code := exitOK
	if err := applyUpdate(from, to, parent, env.Now().UTC().Format("20060102T150405Z"), names, man, edits); err != nil {
		result.ExitCode = 1
		result.Refused = err.Error()
		fmt.Fprintln(stderr, "update stopped: "+err.Error())
		code = exitFailed
	} else if err := mergeInstalled(installedPath, installed, man, names); err != nil {
		result.ExitCode = 1
		result.Refused = err.Error()
		fmt.Fprintln(stderr, "the installed manifest was not written: "+err.Error())
		code = exitFailed
	}
	result.Time = env.Now().UTC().Format(time.RFC3339)
	if err := controls.WriteAudit(af, result); err != nil {
		fmt.Fprintln(stderr, "the result line could not be written: "+err.Error())
		return exitFailed
	}
	return code
}

// applyUpdate moves edits, sets old folders aside and copies the new ones. It
// only renames and copies; it never removes a file.
func applyUpdate(from, to, parent, stamp string, names []string, man modsManifest, edits []updStep) error {
	for _, e := range edits {
		src := filepath.Join(to, e.mod, filepath.FromSlash(e.rel))
		dst := filepath.Join(parent, "znd-kept", stamp, e.mod, filepath.FromSlash(e.rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	for _, n := range names {
		dst := filepath.Join(to, n)
		if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
			aside := filepath.Join(parent, "znd-old", stamp, n)
			if err := os.MkdirAll(filepath.Dir(aside), 0o755); err != nil {
				return err
			}
			if err := os.Rename(dst, aside); err != nil {
				return err
			}
		}
		for rel, want := range man.Mods[n].Files {
			b, err := os.ReadFile(filepath.Join(from, n, filepath.FromSlash(rel)))
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != want {
				return fmt.Errorf("hash of %s/%s changed during the update", n, rel)
			}
			out := filepath.Join(dst, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(out, b, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergeInstalled records the hashes just installed, next to the mod place, so
// the next update can tell an edit from our own file.
func mergeInstalled(path string, old, man modsManifest, names []string) error {
	if old.Mods == nil {
		old.Mods = man.Mods
	} else {
		for _, n := range names {
			old.Mods[n] = man.Mods[n]
		}
	}
	b, err := json.MarshalIndent(old, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

const auditUsage = "usage: znd audit --control <name> --target <text> --exit <code>"

// runAudit appends one audit line for a caller that ran something itself. It
// runs nothing, so it has no dry run.
func runAudit(args []string, stdout, stderr io.Writer, d deps) int {
	var control, target string
	var exit int
	var gotControl, gotTarget, gotExit bool
	for i := 0; i < len(args); i++ {
		a := args[i]
		if (a != "--control" && a != "--target" && a != "--exit") || i+1 >= len(args) {
			fmt.Fprintf(stderr, "unknown flag or missing value: %s\n%s\n", a, auditUsage)
			return exitUsage
		}
		i++
		switch a {
		case "--control":
			control, gotControl = args[i], true
		case "--target":
			target, gotTarget = args[i], true
		case "--exit":
			if _, err := fmt.Sscanf(args[i], "%d", &exit); err != nil || fmt.Sprint(exit) != args[i] {
				fmt.Fprintln(stderr, "--exit must be a whole number\n"+auditUsage)
				return exitUsage
			}
			gotExit = true
		}
	}
	if !gotControl || !gotTarget || !gotExit {
		fmt.Fprintln(stderr, auditUsage)
		return exitUsage
	}
	if len(control) < 1 || len(control) > 32 || strings.Trim(control, "abcdefghijklmnopqrstuvwxyz_") != "" {
		fmt.Fprintln(stderr, "the control name must match [a-z_]{1,32}")
		return exitUsage
	}
	if strings.ContainsAny(target, "\r\n") {
		fmt.Fprintln(stderr, "the target must not hold a line break")
		return exitUsage
	}
	if r := []rune(target); len(r) > 200 {
		target = string(r[:200])
	}
	env := d.audit()
	af, err := controls.OpenAudit(env.Path)
	if err != nil {
		fmt.Fprintln(stderr, "the audit line cannot be written: "+err.Error())
		return exitFailed
	}
	defer af.Close()
	line := controls.AuditLine{Time: env.Now().UTC().Format(time.RFC3339), Control: control, Target: target, ExitCode: exit}
	if err := controls.WriteAudit(af, line); err != nil {
		fmt.Fprintln(stderr, "the audit line cannot be written: "+err.Error())
		return exitFailed
	}
	return exitOK
}
