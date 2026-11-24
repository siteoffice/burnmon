package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type updRig struct {
	t     *testing.T
	from  string // source folder, holds herdr-app/mods.json
	place string // the mod place, inside a parent that holds znd-kept
	audit string
	d     deps
}

func hashOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// newUpdRig writes a source folder with one mod "alpha" holding main.js and
// lib/x.js, and a manifest that names both with their hashes.
func newUpdRig(t *testing.T) *updRig {
	t.Helper()
	root := t.TempDir()
	r := &updRig{t: t, from: filepath.Join(root, "src"), place: filepath.Join(root, "home", "mods"),
		audit: filepath.Join(root, "controls-audit.ndjson")}
	writeFile(t, filepath.Join(r.from, "alpha", "main.js"), "new main")
	writeFile(t, filepath.Join(r.from, "alpha", "lib", "x.js"), "new x")
	r.setManifest(map[string]string{"main.js": "new main", "lib/x.js": "new x"})
	if err := os.MkdirAll(r.place, 0o755); err != nil {
		t.Fatal(err)
	}
	r.d = deps{audit: func() auditEnv {
		return auditEnv{Path: r.audit, Now: func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }}
	}}
	return r
}

func (r *updRig) setManifest(files map[string]string) {
	hs := map[string]string{}
	for rel, body := range files {
		hs[rel] = hashOf(body)
	}
	b, _ := json.Marshal(map[string]any{"mods": map[string]any{"alpha": map[string]any{"files": hs}}})
	writeFile(r.t, filepath.Join(r.from, "herdr-app", "mods.json"), string(b))
}

func (r *updRig) run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(append([]string{"update-mods"}, args...), &out, &errb, r.d)
	return code, out.String(), errb.String()
}

// install puts an old copy of alpha in the place, with an installed manifest.
func (r *updRig) install() {
	writeFile(r.t, filepath.Join(r.place, "alpha", "main.js"), "old main")
	writeFile(r.t, filepath.Join(r.place, "alpha", "lib", "x.js"), "old x")
	b, _ := json.Marshal(map[string]any{"mods": map[string]any{"alpha": map[string]any{"files": map[string]string{
		"main.js": hashOf("old main"), "lib/x.js": hashOf("old x")}}}})
	writeFile(r.t, filepath.Join(filepath.Dir(r.place), "znd-installed.json"), string(b))
}

// snapshot lists every file under root with its content.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			rel, _ := filepath.Rel(root, p)
			m[filepath.ToSlash(rel)] = readFile(t, p)
		}
		return nil
	})
	return m
}

func TestUpdateKeepsEdits(t *testing.T) {
	r := newUpdRig(t)
	r.install()
	writeFile(t, filepath.Join(r.place, "alpha", "main.js"), "my edit") // differs from installed manifest
	before := tree(t, filepath.Dir(r.place))

	code, out, errs := r.run("--from", r.from, "--to", r.place, "--yes")
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	// the new folder matches the manifest
	if got := readFile(t, filepath.Join(r.place, "alpha", "main.js")); got != "new main" {
		t.Errorf("main.js = %q", got)
	}
	if got := readFile(t, filepath.Join(r.place, "alpha", "lib", "x.js")); got != "new x" {
		t.Errorf("x.js = %q", got)
	}
	parent := filepath.Dir(r.place)
	after := tree(t, parent)
	// the edit lives on in znd-kept, the untouched old file in the aside folder
	var keptEdit, asideOld bool
	for p, body := range after {
		if strings.HasPrefix(p, "znd-kept/") && strings.HasSuffix(p, "alpha/main.js") && body == "my edit" {
			keptEdit = true
		}
		if strings.HasPrefix(p, "znd-old/") && strings.HasSuffix(p, "alpha/lib/x.js") && body == "old x" {
			asideOld = true
		}
		if strings.HasPrefix(p, "znd-kept/") && strings.HasSuffix(p, "lib/x.js") {
			t.Errorf("an untouched file was kept as an edit: %s", p)
		}
	}
	if !keptEdit {
		t.Errorf("edit not kept: %v", after)
	}
	if !asideOld {
		t.Errorf("old folder not set aside: %v", after)
	}
	// nothing deleted: every body that existed before still exists somewhere
	have := map[string]bool{}
	for _, body := range after {
		have[body] = true
	}
	for p, body := range before {
		if p == "znd-installed.json" { // rewritten in place with the new hashes
			continue
		}
		if !have[body] {
			t.Errorf("content of %s is gone", p)
		}
	}
	if len(r.lines()) != 2 {
		t.Errorf("want a start and a result audit line, got %v", r.lines())
	}
}

func (r *updRig) lines() []string {
	b, err := os.ReadFile(r.audit)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func TestUpdateFreshInstall(t *testing.T) {
	r := newUpdRig(t)
	if code, out, errs := r.run("--from", r.from, "--to", r.place, "--yes"); code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	if got := readFile(t, filepath.Join(r.place, "alpha", "main.js")); got != "new main" {
		t.Errorf("main.js = %q", got)
	}
}

func TestUpdateDryRun(t *testing.T) {
	r := newUpdRig(t)
	r.install()
	writeFile(t, filepath.Join(r.place, "alpha", "main.js"), "my edit")
	before := tree(t, filepath.Dir(r.place))
	code, out, _ := r.run("--from", r.from, "--to", r.place, "--dry-run")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"keep", "alpha", "main.js", "copy"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run output lacks %q: %s", want, out)
		}
	}
	after := tree(t, filepath.Dir(r.place))
	if len(after) != len(before) {
		t.Errorf("dry run changed files: %v", after)
	}
	for p, b := range before {
		if after[p] != b {
			t.Errorf("dry run changed %s", p)
		}
	}
	if r.lines() != nil {
		t.Error("dry run wrote an audit line")
	}
}

func TestUpdateRequiresOneFlag(t *testing.T) {
	r := newUpdRig(t)
	for _, args := range [][]string{{"--from", r.from, "--to", r.place}, {"--from", r.from, "--to", r.place, "--dry-run", "--yes"}} {
		if code, _, _ := r.run(args...); code != exitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	if code, _, _ := r.run("--yes"); code != exitUsage {
		t.Errorf("missing --from and --to must give exit 2, got %d", code)
	}
}

func TestUpdateRefusesBadSource(t *testing.T) {
	r := newUpdRig(t)
	r.install()
	before := tree(t, filepath.Dir(r.place))
	// source file differs from its manifest hash
	writeFile(t, filepath.Join(r.from, "alpha", "main.js"), "tampered")
	if code, _, errs := r.run("--from", r.from, "--to", r.place, "--yes"); code != exitRefused || !strings.Contains(errs, "hash") {
		t.Errorf("hash mismatch: exit %d, %q", code, errs)
	}
	// a path that leaves the mod folder
	writeFile(t, filepath.Join(r.from, "herdr-app", "mods.json"),
		`{"mods":{"alpha":{"files":{"../evil.js":"`+hashOf("x")+`"}}}}`)
	if code, _, _ := r.run("--from", r.from, "--to", r.place, "--yes"); code != exitRefused {
		t.Errorf("escaping path: exit %d", code)
	}
	// a mod name with a separator
	writeFile(t, filepath.Join(r.from, "herdr-app", "mods.json"),
		`{"mods":{"a/b":{"files":{"main.js":"`+hashOf("x")+`"}}}}`)
	if code, _, _ := r.run("--from", r.from, "--to", r.place, "--yes"); code != exitRefused {
		t.Errorf("bad mod name: exit %d", code)
	}
	after := tree(t, filepath.Dir(r.place))
	if len(after) != len(before) {
		t.Error("a refused update changed files")
	}
}

func TestUpdateAuditFirst(t *testing.T) {
	r := newUpdRig(t)
	r.install()
	r.audit = filepath.Join(t.TempDir(), "no-such-dir", "a.ndjson") // cannot be opened
	r.d.audit = func() auditEnv { return auditEnv{Path: r.audit, Now: time.Now} }
	before := tree(t, filepath.Dir(r.place))
	if code, _, _ := r.run("--from", r.from, "--to", r.place, "--yes"); code != exitRefused {
		t.Errorf("exit %d, want %d", code, exitRefused)
	}
	if got := readFile(t, filepath.Join(r.place, "alpha", "main.js")); got != "old main" {
		t.Error("update ran although the audit line could not be written")
	}
	if len(tree(t, filepath.Dir(r.place))) != len(before) {
		t.Error("files changed")
	}
}

func TestVersionFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--version"}, &out, &errb, deps{}); code != exitOK || !strings.HasPrefix(out.String(), "znd ") {
		t.Errorf("exit %d, out %q", code, out.String())
	}
}

type auditRig struct {
	path string
	d    deps
}

func newAuditRig(t *testing.T) *auditRig {
	r := &auditRig{path: filepath.Join(t.TempDir(), "a.ndjson")}
	r.d = deps{audit: func() auditEnv {
		return auditEnv{Path: r.path, Now: func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }}
	}}
	return r
}

func (r *auditRig) run(args ...string) (int, string) {
	var out, errb bytes.Buffer
	code := run(append([]string{"audit"}, args...), &out, &errb, r.d)
	return code, errb.String()
}

func TestAuditValidates(t *testing.T) {
	r := newAuditRig(t)
	bad := [][]string{
		{"--control", "Bad-Name", "--target", "t", "--exit", "0"},
		{"--control", "", "--target", "t", "--exit", "0"},
		{"--control", strings.Repeat("a", 33), "--target", "t", "--exit", "0"},
		{"--control", "quick_job", "--target", "line\nbreak", "--exit", "0"},
		{"--control", "quick_job", "--target", "t"},
		{"--control", "quick_job", "--target", "t", "--exit", "x"},
		{"--control", "quick_job", "--exit", "0"},
		{"--control", "quick_job", "--target", "t", "--exit", "0", "--bogus", "1"},
	}
	for _, args := range bad {
		if code, _ := r.run(args...); code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, exitUsage)
		}
	}
	if _, err := os.Stat(r.path); err == nil {
		t.Error("a bad call wrote a line")
	}
}

func TestAuditWritesOneLine(t *testing.T) {
	r := newAuditRig(t)
	long := strings.Repeat("x", 250)
	if code, errs := r.run("--control", "quick_job", "--target", long, "--exit", "7"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errs)
	}
	lines := strings.Split(strings.TrimRight(readFile(t, r.path), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("lines: %v", lines)
	}
	var l struct {
		Control, Target, Time string
		ExitCode              int `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &l); err != nil {
		t.Fatal(err)
	}
	if l.Control != "quick_job" || l.ExitCode != 7 || len(l.Target) != 200 || l.Time == "" {
		t.Errorf("line: %+v", l)
	}
}
