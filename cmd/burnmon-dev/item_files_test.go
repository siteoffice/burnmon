//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"burnmon/internal/snapshot"
	"burnmon/internal/waits"
)

var itemNow = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

type itemEnv struct {
	snap, spec, report, dir, exe, missing, other string
}

// newItemEnv writes real temp files and a snapshot whose items name them.
// extra items are appended to the wait list.
func newItemEnv(t *testing.T, writtenAt time.Time, extra ...waits.Item) itemEnv {
	t.Helper()
	d := t.TempDir()
	e := itemEnv{
		snap:    filepath.Join(d, "snapshot.json"),
		spec:    filepath.Join(d, "spec.md"),
		report:  filepath.Join(d, "report.md"),
		dir:     filepath.Join(d, "dir.md"),
		exe:     filepath.Join(d, "tool.exe"),
		missing: filepath.Join(d, "gone.md"),
		other:   filepath.Join(d, "other.md"),
	}
	for _, p := range []string{e.spec, e.report, e.exe, e.other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(e.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	wait := []waits.Item{
		{Kind: waits.KindGate, ID: "gate:a:G1", Title: "a", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: e.spec},
		{Kind: waits.KindSprint, ID: "sprint:b", Title: "b", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: e.dir, ResultPath: e.missing},
	}
	wait = append(wait, extra...)
	out := []waits.Item{
		{Kind: waits.KindFinished, ID: "sprint:c", Title: "c", Since: "2026-10-23T09:00:00Z", Source: "foreman", ResultPath: e.report},
		{Kind: waits.KindFinished, ID: "sprint:d", Title: "d", Since: "2026-10-23T09:00:00Z", Source: "foreman", ResultPath: e.exe},
	}
	s := snapshot.Build(snapshot.Input{Now: writtenAt, TickSeconds: 5, WaitItems: wait, OutcomeItems: out})
	if err := snapshot.Write(e.snap, s); err != nil {
		t.Fatal(err)
	}
	return e
}

func stubOpener(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	old := openItemFile
	openItemFile = func(p string) error { calls = append(calls, p); return nil }
	t.Cleanup(func() { openItemFile = old })
	return &calls
}

func TestItemFileAllowedOpensListedFiles(t *testing.T) {
	e := newItemEnv(t, itemNow)
	for _, p := range []string{e.spec, e.report, strings.ToUpper(e.spec), strings.Replace(e.spec, "spec.md", "spec.MD", 1)} {
		if _, err := itemFileAllowed(e.snap, itemNow, p, ""); err != nil {
			t.Errorf("%q should be allowed: %v", p, err)
		}
	}
}

func TestItemFileAllowedRefuses(t *testing.T) {
	// A relative path listed in an item, a UNC path, a device path and a
	// stream, all written into the snapshot on purpose.
	extra := []waits.Item{
		{Kind: waits.KindGate, ID: "g:rel", Title: "r", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: `spec.md`},
		{Kind: waits.KindGate, ID: "g:unc", Title: "u", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: `\\example-host\share\x.md`},
		{Kind: waits.KindGate, ID: "g:dev", Title: "v", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: `\\.\C:\x.md`},
		{Kind: waits.KindGate, ID: "g:nul", Title: "n", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: `NUL.md`},
	}
	e := newItemEnv(t, itemNow, extra...)
	calls := stubOpener(t)

	// Every listed bad extension, written as real files and listed.
	var extPaths []string
	var extItems []waits.Item
	dd := filepath.Dir(e.spec)
	for _, n := range []string{"a.exe", "a.bat", "a.cmd", "a.ps1", "a.lnk", "a.msi", "a.js", "a.vbs", "a.com", "a.scr", "a.hta", "a.reg", "a.dll", "noext"} {
		p := filepath.Join(dd, n)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		extPaths = append(extPaths, p)
		extItems = append(extItems, waits.Item{Kind: waits.KindGate, ID: "g:" + n, Title: n, Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: p})
	}
	e2 := newItemEnv(t, itemNow, append(extra, extItems...)...)
	_ = e2

	cases := []struct {
		name, snap, path, want string
	}{
		{"not listed", e.snap, e.other, "path is not in the snapshot"},
		{"empty", e.snap, "", "path is not in the snapshot"},
		{"relative listed", e.snap, `spec.md`, "path is not absolute"},
		{"unc listed", e.snap, `\\example-host\share\x.md`, "path is not absolute"},
		{"device listed", e.snap, `\\.\C:\x.md`, "path is not absolute"},
		{"reserved name listed", e.snap, `NUL.md`, "path is not absolute"},
		{"folder", e.snap, e.dir, "not a file"},
		{"exe", e.snap, e.exe, "file type not allowed"},
		{"missing", e.snap, e.missing, "not a file"},
		{"dot dot to listed", e.snap, filepath.Join(filepath.Dir(e.spec), "dir.md", "..", "other.md"), "path is not in the snapshot"},
	}
	for _, p := range extPaths {
		// listed in e2's snapshot, not e's
		cases = append(cases, struct{ name, snap, path, want string }{"ext " + filepath.Base(p), e2.snap, p, "file type not allowed"})
	}
	for _, c := range cases {
		_, err := itemFileAllowed(c.snap, itemNow, c.path, "")
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
		if err := openItemFileChecked(c.snap, itemNow, c.path, ""); err == nil {
			t.Errorf("%s: openItemFileChecked returned nil", c.name)
		}
	}

	// Stale snapshot: the listed spec is refused.
	st := newItemEnv(t, itemNow.Add(-20*time.Second))
	if _, err := itemFileAllowed(st.snap, itemNow, st.spec, ""); err == nil || err.Error() != "snapshot is not current" {
		t.Errorf("stale: err = %v", err)
	}
	// Missing snapshot file.
	if _, err := itemFileAllowed(filepath.Join(t.TempDir(), "none.json"), itemNow, e.spec, ""); err == nil || err.Error() != "snapshot is not current" {
		t.Errorf("missing snapshot: err = %v", err)
	}
	if err := openItemFileChecked(st.snap, itemNow, st.spec, ""); err == nil {
		t.Error("stale snapshot opened")
	}
	if len(*calls) != 0 {
		t.Errorf("opener was called for refused paths: %v", *calls)
	}
}

func TestBindingOpensOnlyAllowed(t *testing.T) {
	e := newItemEnv(t, itemNow)
	calls := stubOpener(t)
	if err := openItemFileChecked(e.snap, itemNow, e.other, ""); err == nil {
		t.Error("unlisted path accepted")
	}
	if len(*calls) != 0 {
		t.Fatalf("opener called for refused path: %v", *calls)
	}
	if err := openItemFileChecked(e.snap, itemNow, strings.ToUpper(e.spec), ""); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != e.spec {
		t.Fatalf("calls = %v", *calls)
	}
}

func gateItem(id, spec string) waits.Item {
	return waits.Item{Kind: waits.KindGate, ID: "g:" + id, Title: id, Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: spec}
}

// TestItemFileHostileSpellings lists and requests hostile spellings with real
// values. Every case is refused with the stated error and the opener, a
// recorder, is never called.
func TestItemFileHostileSpellings(t *testing.T) {
	d := t.TempDir()
	write := func(name string) string {
		p := filepath.Join(d, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	plain := write("spec.md")
	kelvin := write("kelvin.md")
	long := `C:\` + strings.Repeat("d", 130) + `\` + strings.Repeat("e", 130) + `\x.md`

	extra := []waits.Item{
		gateItem("ads", `C:\x\spec.md:s`),
		gateItem("dot", plain+"."),
		gateItem("space", plain+" "),
		gateItem("nul", `C:\dir\NUL.md`),
		gateItem("long", long),
		gateItem("kelvin", kelvin),
	}
	link := filepath.Join(d, "link.md")
	symlinkErr := os.Symlink(plain, link)
	if symlinkErr == nil {
		extra = append(extra, gateItem("link", link))
	}
	e := newItemEnv(t, itemNow, extra...)
	calls := stubOpener(t)

	cases := []struct{ name, path, want string }{
		{"ads listed and requested", `C:\x\spec.md:s`, "path is not absolute"},
		{"ads requested, plain listed", plain + ":s", "path is not in the snapshot"},
		{"trailing dot listed and requested", plain + ".", "file type not allowed"},
		{"trailing space listed and requested", plain + " ", "file type not allowed"},
		{"embedded NUL requested", plain + "\x00", "path is not in the snapshot"},
		{"NUL before extension", strings.TrimSuffix(plain, ".md") + "\x00.md", "path is not in the snapshot"},
		{"reserved device name listed", `C:\dir\NUL.md`, "not a file"},
		{"over 260 characters", long, "path is too long"},
		{"kelvin sign look-alike", strings.Replace(kelvin, "kelvin", "\u212Aelvin", 1), "path is not in the snapshot"},
	}
	if symlinkErr == nil {
		cases = append(cases, struct{ name, path, want string }{"symlink to a listed-type file", link, "not a file"})
	}
	for _, c := range cases {
		_, err := itemFileAllowed(e.snap, itemNow, c.path, "")
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
		if err := openItemFileChecked(e.snap, itemNow, c.path, ""); err == nil {
			t.Errorf("%s: openItemFileChecked returned nil", c.name)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("opener called for refused paths: %v", *calls)
	}
	if symlinkErr != nil {
		t.Skipf("symlink creation not permitted here, symlink case not run: %v", symlinkErr)
	}
}

// TestItemFileOpenerGetsListedSpelling asserts that spellings which Clean or
// ASCII case folding make equal still open the snapshot's own spelling.
func TestItemFileOpenerGetsListedSpelling(t *testing.T) {
	e := newItemEnv(t, itemNow)
	calls := stubOpener(t)
	for _, p := range []string{strings.ToUpper(e.spec), strings.ReplaceAll(e.spec, `\`, "/"), filepath.Dir(e.spec) + `\.\spec.md`} {
		*calls = nil
		if err := openItemFileChecked(e.snap, itemNow, p, ""); err != nil {
			t.Errorf("%q: %v", p, err)
			continue
		}
		if len(*calls) != 1 || (*calls)[0] != e.spec {
			t.Errorf("%q: opener got %v, want exactly %q", p, *calls, e.spec)
		}
	}
}

// A close_out row links its report_path (the parked lines link); the same
// rules apply: only a path the snapshot lists, an allowed type, a real file.
func TestItemFileOpensListedReportPath(t *testing.T) {
	d := t.TempDir()
	root := filepath.Join(d, "foreman")
	rep := filepath.Join(root, "sprints", "p", "s1", "report.md")
	other := filepath.Join(root, "sprints", "p", "s1", "other.md")
	if err := os.MkdirAll(filepath.Dir(rep), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{rep, other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	item := waits.Item{Kind: waits.KindCloseOut, ID: "closeout:s1", Title: "s1", Since: "2026-10-23T09:00:00Z",
		Source: "foreman", SprintID: "s1", ReportPath: rep, ParkedLines: 2}
	snap := filepath.Join(d, "snapshot.json")
	s := snapshot.Build(snapshot.Input{Now: itemNow, TickSeconds: 5, WaitItems: []waits.Item{item}})
	if err := snapshot.Write(snap, s); err != nil {
		t.Fatal(err)
	}
	if got, err := itemFileAllowed(snap, itemNow, rep, root); err != nil || got != rep {
		t.Errorf("a listed report_path = %q, %v; want it allowed", got, err)
	}
	if _, err := itemFileAllowed(snap, itemNow, other, root); err == nil {
		t.Error("a path the snapshot does not list was allowed")
	}
}

// ---- B6 fix pass F: containment (S2) ----------------------------------------

const outsideMsg = "file is outside the Foreman folders"

// containEnv writes a Foreman root with sprints and flows folders, an outside
// folder, and a snapshot listing the given paths as spec_path of gate items.
type containEnv struct {
	root, outside, snap string
}

func newContainEnv(t *testing.T) containEnv {
	t.Helper()
	d := t.TempDir()
	e := containEnv{root: filepath.Join(d, "foreman"), outside: filepath.Join(d, "elsewhere"), snap: filepath.Join(d, "snapshot.json")}
	for _, p := range []string{filepath.Join(e.root, "sprints", "p", "s"), filepath.Join(e.root, "flows", "p", "f"), e.outside} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func (e containEnv) write(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// list writes a snapshot with one gate item per spec path and one close_out
// item per report path.
func (e containEnv) list(t *testing.T, specs, reports []string) {
	t.Helper()
	var items []waits.Item
	for i, p := range specs {
		items = append(items, waits.Item{Kind: waits.KindGate, ID: "gate:f:G" + strconv.Itoa(i), Title: "f", Since: "2026-10-23T09:00:00Z", Source: "foreman", SpecPath: p})
	}
	for i, p := range reports {
		items = append(items, waits.Item{Kind: waits.KindCloseOut, ID: "closeout:s" + strconv.Itoa(i), Title: "s", Since: "2026-10-23T09:00:00Z", Source: "foreman", SprintID: "s", ReportPath: p})
	}
	if err := snapshot.Write(e.snap, snapshot.Build(snapshot.Input{Now: itemNow, TickSeconds: 5, WaitItems: items})); err != nil {
		t.Fatal(err)
	}
}

func TestItemFileContainmentInside(t *testing.T) {
	e := newContainEnv(t)
	report := e.write(t, filepath.Join(e.root, "sprints", "p", "s", "report.md"))
	flowSpec := e.write(t, filepath.Join(e.root, "flows", "p", "f", "spec.md"))
	projSpec := e.write(t, filepath.Join(filepath.Dir(e.root), "proj", "docs", "superpowers", "specs", "2026-10-23_x.md"))
	e.list(t, []string{flowSpec, projSpec}, []string{report})
	for _, p := range []string{report, flowSpec, projSpec} {
		if got, err := itemFileAllowed(e.snap, itemNow, p, e.root); err != nil || got != p {
			t.Errorf("%s = %q, %v; want it allowed", p, got, err)
		}
	}
}

func TestItemFileContainmentOutside(t *testing.T) {
	e := newContainEnv(t)
	out := e.write(t, filepath.Join(e.outside, "notes.md"))
	outReport := e.write(t, filepath.Join(e.outside, "report.md"))
	inRootNotSprints := e.write(t, filepath.Join(e.root, "secrets", "x.md"))
	e.list(t, []string{out, inRootNotSprints}, []string{outReport})
	for _, p := range []string{out, outReport, inRootNotSprints} {
		_, err := itemFileAllowed(e.snap, itemNow, p, e.root)
		if err == nil || err.Error() != outsideMsg {
			t.Errorf("%s: err = %v, want %q", p, err, outsideMsg)
		}
		if err := openItemFileChecked(e.snap, itemNow, p, e.root); err == nil {
			t.Errorf("%s: openItemFileChecked returned nil", p)
		}
	}
}

// A listed path that climbs out of the Foreman folders with ".." cleans to a
// path outside them.
func TestItemFileContainmentDotDotEscape(t *testing.T) {
	e := newContainEnv(t)
	secret := e.write(t, filepath.Join(e.root, "secret.md"))
	escape := filepath.Join(e.root, "sprints", "p", "s") + `\..\..\..\secret.md`
	e.list(t, []string{escape}, nil)
	if _, err := itemFileAllowed(e.snap, itemNow, escape, e.root); err == nil || err.Error() != outsideMsg {
		t.Errorf("%s (%s): err = %v, want %q", escape, secret, err, outsideMsg)
	}
}

// A junction inside sprints that points outside is followed by EvalSymlinks:
// the resolved file is outside, so it is refused.
func TestItemFileContainmentJunctionParent(t *testing.T) {
	e := newContainEnv(t)
	target := e.write(t, filepath.Join(e.outside, "stolen.md"))
	link := filepath.Join(e.root, "sprints", "p", "link")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, e.outside).CombinedOutput(); err != nil {
		t.Skipf("cannot make a junction: %v %s", err, out)
	}
	t.Cleanup(func() { _ = os.Remove(link) })
	listed := filepath.Join(link, "stolen.md")
	if _, err := os.Stat(listed); err != nil || target == "" {
		t.Fatalf("junction does not resolve: %v", err)
	}
	e.list(t, []string{listed}, []string{listed})
	// Either EvalSymlinks fails on the junction (it does on a short 8.3 temp
	// path, and the check fails closed with "not a file") or it resolves it
	// and containment refuses; the file must not open either way.
	calls := stubOpener(t)
	if _, err := itemFileAllowed(e.snap, itemNow, listed, e.root); err == nil {
		t.Error("a file behind a junction that leads outside was allowed")
	}
	if err := openItemFileChecked(e.snap, itemNow, listed, e.root); err == nil || len(*calls) != 0 {
		t.Errorf("opened a file behind a junction: %v %v", err, *calls)
	}
}

// Without foreman_root a report path is refused; a spec path keeps B4's rule.
func TestItemFileNoForemanRootRefusesReport(t *testing.T) {
	e := newContainEnv(t)
	report := e.write(t, filepath.Join(e.root, "sprints", "p", "s", "report.md"))
	spec := e.write(t, filepath.Join(e.outside, "spec.md"))
	e.list(t, []string{spec}, []string{report})
	if _, err := itemFileAllowed(e.snap, itemNow, report, ""); err == nil || err.Error() != outsideMsg {
		t.Errorf("report without root: err = %v, want %q", err, outsideMsg)
	}
	if _, err := itemFileAllowed(e.snap, itemNow, spec, ""); err != nil {
		t.Errorf("spec without root: %v", err)
	}
}
