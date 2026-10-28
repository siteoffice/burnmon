package graph

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fenceTick = "```"

// verdictDir writes a sprint folder: report.md, sprint.json with the run mode,
// and an optional action log.
func verdictDir(t *testing.T, report, runMode, actionLog string) string {
	t.Helper()
	dir := t.TempDir()
	if report != "" {
		write(t, filepath.Join(dir, "report.md"), report)
	}
	if runMode != "-" {
		write(t, filepath.Join(dir, "sprint.json"), `{"sprint_id":"x","run_mode":"`+runMode+`"}`)
	}
	if actionLog != "" {
		write(t, filepath.Join(dir, "action-log.ndjson"), actionLog)
	}
	return dir
}

func TestCloseOutVerdict(t *testing.T) {
	big := strings.Repeat("x", maxReportBytes+10)
	for name, c := range map[string]struct {
		report, mode, log string
		ok                bool
		reason            string // substring
		parked            int
	}{
		"accepted none":            {"# R\n\n## Close-out\n- none\n", "unattended", "", true, "", 0},
		"accepted items":           {"## Close-out\n- DO done: x\n- ASK answered: q -> a\n- HAND handed: h, verified by test\n- DROP dropped: d because e\n", "attended", "", true, "", 0},
		"accepted unknown mode":    {"## Close-out\n- none\n", "", "", true, "", 0},
		"missing block":            {"# R\nno block here\n", "attended", "", false, "no '## Close-out' section", 0},
		"fenced block is no block": {fenceTick + "\n## Close-out\n- none\n" + fenceTick + "\n", "attended", "", false, "no '## Close-out' section", 0},
		"empty block":              {"## Close-out\n\n## Next\n", "attended", "", false, "no lines", 0},
		"no class":                 {"## Close-out\n- done: x\n", "attended", "", false, "not a Close-out item", 0},
		"unknown class":            {"## Close-out\n- FIX done: x\n", "attended", "", false, "not a Close-out item", 0},
		"bad state pair":           {"## Close-out\n- DO parked: x\n", "attended", "", false, "not a valid Close-out line", 0},
		"empty text":               {"## Close-out\n- DO done:\n", "attended", "", false, "needs text", 0},
		"ask answered no arrow":    {"## Close-out\n- ASK answered: q a\n", "attended", "", false, "ASK answered needs", 0},
		"hand handed no check":     {"## Close-out\n- HAND handed: did it\n", "attended", "", false, "verified by", 0},
		"drop no because":          {"## Close-out\n- DROP dropped: gone\n", "attended", "", false, "because", 0},
		"drop because inside word": {"## Close-out\n- DROP dropped: gone unbecause\n", "attended", "", false, "because", 0},
		"none with items":          {"## Close-out\n- none\n- DO done: x\n", "attended", "", false, "must be the only line", 0},
		"two nones":                {"## Close-out\n- none\n- none\n", "attended", "", false, "must be the only line", 0},
		"open line in block":       {"## Close-out\nOPEN: q\n", "attended", "", false, "OPEN:", 0},
		"open line outside":        {"OPEN: q\n## Close-out\n- none\n", "attended", "", false, "OPEN:", 0},
		"banned heading outside":   {"## Residual risks\n## Close-out\n- none\n", "attended", "", false, "banned heading", 0},
		"second block":             {"## Close-out\n- none\n## Other\n## Close-out\n- none\n", "attended", "", false, "second", 0},
		"parked unattended":        {"## Close-out\n- ASK parked: q\n", "unattended", "", false, "1 parked line", 1},
		"parked attended":          {"## Close-out\n- ASK parked: q\n- HAND parked: h\n\n" + fenceTick + "\nx\n" + fenceTick + "\n", "attended", "", false, "2 parked lines", 2},
		"attended deferred none":   {"## Close-out\n- ASK deferred: q\n", "attended", "", false, "touchpoint_done", 0},
		"attended deferred logged": {"## Close-out\n- ASK deferred: q\n", "attended", "{\"action\":\"touchpoint_done\",\"detail\":\"deferred: q\"}\nnot json\n", true, "", 0},
		"attended deferred other":  {"## Close-out\n- ASK deferred: q\n", "attended", "{\"action\":\"touchpoint_done\",\"detail\":\"asked: q\"}\n{\"action\":\"x\",\"detail\":\"deferred: q\"}\n", false, "touchpoint_done", 0},
		"attended two deferred":    {"## Close-out\n- ASK deferred: q\n- HAND deferred: h\n", "attended", "{\"action\":\"touchpoint_done\",\"detail\":\"deferred: q\"}\n", false, "(1 found for 2)", 0},
		"unattended deferred":      {"## Close-out\n- ASK deferred: q\n", "unattended", "", false, "not allowed in an Unattended", 0},
		"no sprint json":           {"## Close-out\n- none\n", "-", "", false, ReasonNotMirrored, 0},
		"report past the cap":      {big + "\n## Close-out\n- none\n", "attended", "", false, ReasonNotMirrored, 0},
		"vertical tab":             {"## Close-out\n- none\v\n", "attended", "", false, ReasonNotMirrored, 0},
		"no break space":           {"## Close-out\n- DO\u00a0done: x\n", "attended", "", false, ReasonNotMirrored, 0},
		"no report":                {"", "attended", "", false, "no report yet", 0},
	} {
		t.Run(name, func(t *testing.T) {
			dir := verdictDir(t, c.report, c.mode, c.log)
			ok, reason, parked := CloseOutVerdict(dir)
			if ok != c.ok || parked != c.parked || !strings.Contains(reason, c.reason) || (ok && reason != "") {
				t.Errorf("CloseOutVerdict = %v, %q, %d; want %v, %q, %d", ok, reason, parked, c.ok, c.reason, c.parked)
			}
		})
	}
}

func TestCloseOutVerdictBadSprintJSON(t *testing.T) {
	dir := verdictDir(t, "## Close-out\n- none\n", "attended", "")
	write(t, filepath.Join(dir, "sprint.json"), "{not json")
	if ok, reason, _ := CloseOutVerdict(dir); ok || reason != ReasonNotMirrored {
		t.Errorf("= %v, %q", ok, reason)
	}
}

// ReadForeman carries the verdict on a waiting sprint.
func TestReadForemanCarriesCloseVerdict(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sprints", "p", "s")
	write(t, filepath.Join(dir, "sprint.json"), `{"sprint_id":"p__s__1","slug":"s","status":"waiting-on-you","phase":"report","updated_at":"2026-10-23T10:00:00Z"}`)
	write(t, filepath.Join(dir, "report.md"), "## Close-out\n- DO done: x\n")
	sprints, _, err := ReadForeman(root)
	if err != nil || len(sprints) != 1 || !sprints[0].CloseOK || sprints[0].CloseReason != "" {
		t.Fatalf("%+v, %v", sprints, err)
	}
	write(t, filepath.Join(dir, "report.md"), "## Close-out\n- done: x\n")
	sprints, _, _ = ReadForeman(root)
	if sprints[0].CloseOK || !strings.Contains(sprints[0].CloseReason, "not a Close-out item") {
		t.Errorf("%+v", sprints[0])
	}
}

// The Go verdict agrees with Foreman's own close_out.check on the same
// folders. Skipped without Python or Foreman's library next to this checkout.
func TestCloseOutVerdictMatchesForeman(t *testing.T) {
	lib := `C:\ZND\10_holding\sprints\_lib`
	if _, err := os.Stat(filepath.Join(lib, "close_out.py")); err != nil {
		t.Skip("Foreman's close_out.py is not here")
	}
	py, err := exec.LookPath("python")
	if err != nil {
		t.Skip("python is not on PATH")
	}
	const script = `import sys, json, pathlib
sys.path.insert(0, sys.argv[1])
import close_out
d = pathlib.Path(sys.argv[2])
sp = json.loads((d / "sprint.json").read_text(encoding="utf-8"))
rm = str(sp.get("run_mode") or "")
rm = rm if rm in ("attended", "unattended") else "attended"
f = close_out.check(d / "report.md", run_mode=rm, action_log=d / "action-log.ndjson")
if not f and rm == "unattended" and close_out.parked_items(d / "report.md"):
    f = ["parked"]
print("PASS" if not f else "FAIL")
`
	fence := fenceTick
	reports := map[string][2]string{
		"ok":         {"## Close-out\n- DO done: x\n", "attended"},
		"ok none":    {"# T\n## Close-out\n- none\n", "unattended"},
		"no block":   {"# T\n", "attended"},
		"empty":      {"## Close-out\n", "attended"},
		"no class":   {"## Close-out\n- x: y\n", "attended"},
		"bad pair":   {"## Close-out\n- DO handed: y\n", "attended"},
		"parked":     {"## Close-out\n- ASK parked: y\n", "unattended"},
		"parked att": {"## Close-out\n- ASK parked: y\n", "attended"},
		"hand fence": {"## Close-out\n- HAND parked: y\n\n" + fence + "\nz\n" + fence + "\n", "attended"},
		"deferred a": {"## Close-out\n- ASK deferred: y\n", "attended"},
		"deferred u": {"## Close-out\n- ASK deferred: y\n", "unattended"},
		"heading":    {"## Next steps\n## Close-out\n- none\n", "attended"},
		"open out":   {"OPEN: y\n## Close-out\n- none\n", "attended"},
		"answered":   {"## Close-out\n- ASK answered: q\n", "attended"},
		"handed":     {"## Close-out\n- HAND handed: h Verified By x\n", "attended"},
		"drop":       {"## Close-out\n- DROP dropped: d Because e\n", "attended"},
		"continued":  {"## Close-out\n- DO done: x\n  more\n", "attended"},
		"crlf":       {"## Close-out\r\n- DO done: x\r\n", "attended"},
		"fenced":     {fence + "\n## Close-out\n- ASK parked: y\n" + fence + "\n## Close-out\n- none\n", "attended"},
	}
	for name, r := range reports {
		dir := verdictDir(t, r[0], r[1], "")
		out, err := exec.Command(py, "-I", "-c", script, lib, dir).Output()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := strings.TrimSpace(string(out)) == "PASS"
		if ok, reason, _ := CloseOutVerdict(dir); ok != want {
			t.Errorf("%s: BurnMon %v (%q), Foreman pass %v", name, ok, reason, want)
		}
	}
}
