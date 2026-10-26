package controls

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestVerdictFor(t *testing.T) {
	ok := []struct{ control, gate, verdict string }{
		{Approve, "G1", "approve"}, {Approve, "G2", "approve"}, {Approve, "G3", "proceed"},
		{SendBack, "G1", "reject"}, {SendBack, "G2", "reject"},
	}
	for _, c := range ok {
		v, err := VerdictFor(c.control, c.gate)
		if err != nil || v != c.verdict {
			t.Errorf("VerdictFor(%s,%s) = %q, %v; want %q", c.control, c.gate, v, err, c.verdict)
		}
	}
	bad := []struct{ control, gate string }{
		{SendBack, "G3"}, {Approve, "G4"}, {SendBack, "G4"}, {Approve, "G9"}, {Stop, "G1"}, {Attach, "G2"},
	}
	for _, c := range bad {
		v, err := VerdictFor(c.control, c.gate)
		if err == nil || v != "" {
			t.Errorf("VerdictFor(%s,%s) = %q, %v; want an error", c.control, c.gate, v, err)
		}
	}
	if _, err := VerdictFor(SendBack, "G3"); err == nil || err.Error() != "no send_back for gate G3" {
		t.Errorf("error text = %v", err)
	}
}

func TestVerdictsReturnsACopy(t *testing.T) {
	v := Verdicts()
	v[Approve]["G1"] = "changed"
	delete(v, SendBack)
	got, _ := VerdictFor(Approve, "G1")
	if got != "approve" {
		t.Fatal("Verdicts() exposes the table")
	}
	if _, err := VerdictFor(SendBack, "G1"); err != nil {
		t.Fatal("deleting from the copy changed the table")
	}
}

func TestVerdictsMatchForemanLibrary(t *testing.T) {
	src, err := os.ReadFile(`C:\ZND\10_holding\flows\_lib\flow_state.py`)
	if err != nil {
		t.Skip("Foreman library not present: " + err.Error())
	}
	text := string(src)
	start := strings.Index(text, "VERDICTS_BY_GATE = {")
	if start < 0 {
		t.Skip("VERDICTS_BY_GATE not found")
	}
	block := text[start:]
	if end := strings.Index(block, "}"); end > 0 {
		block = block[:end]
	}
	line := regexp.MustCompile(`"(G\d)":\s*\(([^)]*)\)`)
	word := regexp.MustCompile(`"([a-z]+)"`)
	allowed := map[string][]string{}
	for _, m := range line.FindAllStringSubmatch(block, -1) {
		for _, v := range word.FindAllStringSubmatch(m[2], -1) {
			allowed[m[1]] = append(allowed[m[1]], v[1])
		}
	}
	if len(allowed) == 0 {
		t.Fatal("no gate lines parsed")
	}
	for control, byGate := range Verdicts() {
		for gate, verdict := range byGate {
			found := false
			for _, a := range allowed[gate] {
				if a == verdict {
					found = true
				}
			}
			if !found {
				t.Errorf("%s %s verdict %q is not in Foreman's %v", control, gate, verdict, allowed[gate])
			}
		}
	}
}

func TestGateScriptIsOneASCIILine(t *testing.T) {
	for i := 0; i < len(GateScript); i++ {
		if GateScript[i] > 127 {
			t.Fatalf("non-ASCII byte at %d", i)
		}
	}
	if strings.ContainsAny(GateScript, "\n\r\"") {
		t.Fatal("GateScript holds a newline or a double quote")
	}
}

func TestGateArgv(t *testing.T) {
	got := GateArgv("py.exe", "lib", "flow", "approve", "G2", "t1")
	want := []string{"py.exe", "-I", "-c", GateScript, "lib", "flow", "approve", "G2", "t1"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("GateArgv = %v", got)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const fakeFlowState = `VERDICTS_BY_GATE = {"G2": ("approve", "reject", "park")}

def valid_verdicts(gate_id):
    return VERDICTS_BY_GATE.get(gate_id, ("park",))
`

const fakeFlowTransition = `import json, os

def load_flow(d):
    with open(os.path.join(str(d), "flow.json"), encoding="utf-8") as f:
        return json.load(f)

def cmd_gate(d, verdict, note="", surface="session"):
    with open(os.path.join(str(d), "decided.json"), "w", encoding="utf-8") as f:
        json.dump({"verdict": verdict, "note": note, "surface": surface}, f)
`

func TestGateScriptWithFakeLibrary(t *testing.T) {
	python, err := ResolvePython(exec.LookPath, runtime.GOOS)
	if err != nil {
		t.Skip("no usable python: " + err.Error())
	}
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	work := filepath.Join(root, "work")
	for _, d := range []string{lib, work} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(lib, "flow_state.py"), fakeFlowState)
	writeFile(t, filepath.Join(lib, "flow_transition.py"), fakeFlowTransition)
	// A decoy in the process working folder must not be imported (-I).
	writeFile(t, filepath.Join(work, "flow_transition.py"), "raise SystemExit(99)\n")

	newFlow := func(status, token string) string {
		d := t.TempDir()
		b, _ := json.Marshal(map[string]any{"gate": map[string]string{"id": "G2", "token": token, "status": status}})
		writeFile(t, filepath.Join(d, "flow.json"), string(b))
		return d
	}
	run := func(flow, verdict, token string) int {
		argv := GateArgv(python, lib, flow, verdict, "G2", token)
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = work
		hideWindow(cmd)
		err := cmd.Run()
		if err == nil {
			return 0
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		t.Fatalf("run: %v", err)
		return -1
	}

	flow := newFlow("awaiting-owner", "t1")
	if code := run(flow, "approve", "t1"); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	raw, err := os.ReadFile(filepath.Join(flow, "decided.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["verdict"] != "approve" || got["note"] != "" || got["surface"] != "burnmon" {
		t.Fatalf("decided.json = %v", got)
	}

	cases := []struct {
		name, status, token, verdict string
		code                         int
	}{
		{"other token", "awaiting-owner", "t2", "approve", GateExitChanged},
		{"verdict not allowed", "awaiting-owner", "t1", "ship", GateExitVerdict},
		{"already decided", "decided", "t1", "approve", GateExitChanged},
		{"status neither open nor decided", "weird", "t1", "approve", GateExitChanged},
		{"empty status", "", "t1", "approve", GateExitChanged},
	}
	for _, c := range cases {
		f := newFlow(c.status, "t1")
		if code := run(f, c.verdict, c.token); code != c.code {
			t.Errorf("%s: exit %d, want %d", c.name, code, c.code)
		}
		if _, err := os.Stat(filepath.Join(f, "decided.json")); err == nil {
			t.Errorf("%s: decided.json was written", c.name)
		}
	}
}
