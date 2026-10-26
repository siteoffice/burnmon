//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"burnmon/internal/controls"
	"burnmon/internal/pricing"
	"burnmon/internal/sections"
)

func boolp(b bool) *bool { return &b }

func failLook(string) (string, error) { return "", errors.New("not found") }

// A control switch change is validated like the other change kinds: one kind
// per call, a known control id, and a refused change leaves the file alone.
func TestSetControlSwitch(t *testing.T) {
	dir := t.TempDir()
	on, off := true, false

	// A refused change on a fresh folder writes nothing at all.
	for _, bad := range []sectionsChange{
		{Control: "reply", On: &on},
		{Control: "stop"},
		{Control: "stop", On: &on, Plain: &on},
		{Control: "stop", On: &on, Scrollbar: &on},
		{Control: "stop", On: &on, Mode: "plain", Section: "git"},
		{Control: "stop", On: &on, Section: "git"},
		{Control: "stop", On: &on, Mode: "plain"},
		{Control: "STOP", On: &on},
		{Control: "stop ", On: &on},
	} {
		if err := applySectionsChange(dir, bad); err == nil {
			t.Errorf("%+v: want an error", bad)
		}
	}
	if _, err := os.Stat(viewStatePath(dir)); !os.IsNotExist(err) {
		t.Fatalf("a refused change created the view file: %v", err)
	}

	if err := applySectionsChange(dir, sectionsChange{Control: "stop", On: &on}); err != nil {
		t.Fatal(err)
	}
	got := loadSections(dir).Controls
	if !got["stop"] || got["attach"] || got["approve"] || got["send_back"] {
		t.Errorf("after stop on: %v", got)
	}

	// Refusals now leave the saved file byte for byte as it was.
	before, err := os.ReadFile(viewStatePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []sectionsChange{
		{Control: "reply", On: &off}, {Control: "stop"},
		{Control: "stop", On: &off, Plain: &on}, {Control: "attach", On: &on, Mode: "plain", Section: "git"},
	} {
		if err := applySectionsChange(dir, bad); err == nil {
			t.Errorf("%+v: want an error", bad)
		}
	}
	after, _ := os.ReadFile(viewStatePath(dir))
	if string(before) != string(after) {
		t.Errorf("a refused change altered the file:\n%s\n---\n%s", before, after)
	}

	if err := applySectionsChange(dir, sectionsChange{Control: "stop", On: &off}); err != nil {
		t.Fatal(err)
	}
	if loadSections(dir).Controls["stop"] {
		t.Error("stop off not saved")
	}
	// The other change kinds still work and leave the switches alone.
	if err := applySectionsChange(dir, sectionsChange{Control: "approve", On: &on}); err != nil {
		t.Fatal(err)
	}
	if err := applySectionsChange(dir, sectionsChange{Plain: &on}); err != nil {
		t.Fatal(err)
	}
	if st := loadSections(dir); !st.Plain || !st.Controls["approve"] {
		t.Errorf("plain toggle lost the switch: %+v", st)
	}
}

func TestPayloadCarriesControls(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join("..", "..", "testdata", "snapshot_example.json")
	now := time.Date(2026, 10, 23, 10, 0, 7, 0, time.UTC)
	stub := func() controls.Availability {
		return controls.Availability{
			Ready: map[string]bool{"stop": false, "attach": true},
			Why:   map[string]string{"stop": "switched off in the gear"},
		}
	}
	// Nothing saved: every switch is off in the payload.
	p := sectionsPayloadWith(dir, snapPath, now, stub)
	for _, id := range controls.All {
		if p.Controls.On[id] {
			t.Errorf("control %s is on in a fresh payload", id)
		}
	}
	if p.Controls.Verdicts["approve"]["G3"] != "proceed" {
		t.Errorf("verdicts %v", p.Controls.Verdicts)
	}
	if p.Controls.Ready["stop"] || !p.Controls.Ready["attach"] || p.Controls.Why["stop"] == "" {
		t.Errorf("ready/why not carried: %+v", p.Controls)
	}
	if err := applySectionsChange(dir, sectionsChange{Control: "stop", On: boolp(true)}); err != nil {
		t.Fatal(err)
	}
	p = sectionsPayloadWith(dir, snapPath, now, stub)
	if !p.Controls.On["stop"] || p.Controls.Ready["stop"] {
		t.Errorf("a switch on does not make a control ready by itself: %+v", p.Controls)
	}
	// No service at all (the plain wrapper): nothing is ready.
	q := sectionsPayloadFor(dir, snapPath, now)
	for _, id := range controls.All {
		if q.Controls.Ready[id] {
			t.Errorf("control %s ready without a service", id)
		}
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Controls struct {
			On       map[string]bool              `json:"on"`
			Ready    map[string]bool              `json:"ready"`
			Verdicts map[string]map[string]string `json:"verdicts"`
		} `json:"controls"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if !m.Controls.On["stop"] || m.Controls.Verdicts["approve"]["G3"] != "proceed" || len(m.Controls.Ready) != len(controls.All) {
		t.Errorf("json controls %+v", m.Controls)
	}
}

func TestNewControlServiceWithoutClaude(t *testing.T) {
	if controls.Elevated() {
		t.Skip("test shell is elevated")
	}
	dir := t.TempDir()
	st := sections.Default()
	for _, id := range controls.All {
		st = st.WithControl(id, true)
	}
	if err := saveSections(dir, st); err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(dir, "store", "burnmon.db")
	snap := filepath.Join(dir, "snap.json")
	svc := newControlServiceWith(pricing.Config{}, dir, snap, storePath, failLook)
	a := svc.Availability()
	if a.Ready["stop"] || a.Why["stop"] != "needs claude.exe: set claude_path in burnmon.json" {
		t.Errorf("stop: ready %v why %q", a.Ready["stop"], a.Why["stop"])
	}
	pyLook := func(string) (string, error) { return `C:\example\python.exe`, nil }
	svc = newControlServiceWith(pricing.Config{}, dir, snap, storePath, pyLook)
	a = svc.Availability()
	if a.Ready["approve"] || a.Why["approve"] != "needs foreman_root in burnmon.json" {
		t.Errorf("approve: ready %v why %q", a.Ready["approve"], a.Why["approve"])
	}
	if got, want := svc.Env.AuditPath, filepath.Join(dir, "store", "controls-audit.ndjson"); got != want {
		t.Errorf("audit path %q, want %q", got, want)
	}
	// Every switch is off by default: with no view file nothing is ready.
	svc = newControlServiceWith(pricing.Config{}, t.TempDir(), snap, storePath, pyLook)
	for _, id := range controls.All {
		if svc.Availability().Ready[id] {
			t.Errorf("%s ready with no view file", id)
		}
	}
	if svc.Availability().Why["stop"] == "" {
		t.Error("no reason given for a control that is not ready")
	}
}

// The page can send only a control name plus a target id, and a token. The Go
// side takes no argv, path, pid or command from it.
func TestBindingInputsAreOnlyControlTargetAndToken(t *testing.T) {
	rt := reflect.TypeOf(controls.PreviewRequest{})
	var tags []string
	for i := 0; i < rt.NumField(); i++ {
		tags = append(tags, rt.Field(i).Tag.Get("json"))
	}
	if !reflect.DeepEqual(tags, []string{"control", "target"}) {
		t.Errorf("PreviewRequest fields %v", tags)
	}
	// Extra fields a page might add are dropped on decode.
	var req controls.PreviewRequest
	raw := `{"control":"stop","target":"ab12","argv":["calc.exe"],"command":"calc.exe","path":"C:\\x\\calc.exe","pid":4}`
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	if req != (controls.PreviewRequest{Control: "stop", Target: "ab12"}) {
		t.Errorf("decoded %+v", req)
	}
	pt := reflect.TypeOf(bdevControlPreview)
	if pt.NumIn() != 2 || pt.In(1) != rt || pt.NumOut() != 2 || pt.Out(0) != reflect.TypeOf(controls.PreviewResult{}) {
		t.Errorf("bdevControlPreview signature %v", pt)
	}
	rn := reflect.TypeOf(bdevControlRun)
	if rn.NumIn() != 2 || rn.In(1).Kind() != reflect.String || rn.NumOut() != 2 || rn.Out(0) != reflect.TypeOf(controls.RunResult{}) {
		t.Errorf("bdevControlRun signature %v", rn)
	}
}

// With nothing usable, both bindings refuse with a clear reason, run nothing
// and write no audit line.
func TestBindingsRefuseWithAReason(t *testing.T) {
	dir := t.TempDir()
	svc := newControlServiceWith(pricing.Config{}, dir, filepath.Join(dir, "snap.json"), filepath.Join(dir, "s", "burnmon.db"), failLook)
	for _, req := range []controls.PreviewRequest{
		{Control: "rm", Target: "ab12"},
		{Control: "stop", Target: "ab12"},
		{Control: "stop", Target: "ab12 --all"},
		{Control: "stop", Target: `C:\Windows\System32\calc.exe`},
		{Control: "", Target: ""},
	} {
		res, err := bdevControlPreview(svc, req)
		if err == nil || strings.TrimSpace(err.Error()) == "" {
			t.Errorf("%+v: want a reason, got %v", req, err)
		}
		if res != (controls.PreviewResult{}) {
			t.Errorf("%+v: refused preview carries data %+v", req, res)
		}
	}
	for _, tok := range []string{"", "deadbeef", "../x", "4242"} {
		res, err := bdevControlRun(svc, tok)
		if err == nil || strings.TrimSpace(err.Error()) == "" {
			t.Errorf("token %q: want a reason, got %v", tok, err)
		}
		if res.Refused == "" || res.ExitCode != -1 || res.Line != "" {
			t.Errorf("token %q: refused run result %+v", tok, res)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "s", "controls-audit.ndjson")); !os.IsNotExist(err) {
		t.Errorf("a refused preview or an unknown token wrote an audit file: %v", err)
	}
}

func TestBindingsAreRegistered(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := strings.ReplaceAll(string(b), "\r\n", "\n")
	for _, want := range []string{`w.Bind("bdevControlPreview"`, `w.Bind("bdevControlRun"`} {
		if !strings.Contains(src, want) {
			t.Errorf("main.go has no %s", want)
		}
	}
	// Registered next to bdevSetSections, and only in this Windows-only file.
	i, j := strings.Index(src, `w.Bind("bdevSetSections"`), strings.Index(src, `w.Bind("bdevControlPreview"`)
	if i < 0 || j < i || j-i > 3000 {
		t.Errorf("bindings are not next to bdevSetSections (%d, %d)", i, j)
	}
	if !strings.HasPrefix(src, "//go:build windows") {
		t.Error("main.go lost its windows build tag")
	}
	cv, err := os.ReadFile("controls_view.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(cv), "//go:build windows") {
		t.Error("controls_view.go must be windows only")
	}
}
