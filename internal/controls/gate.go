package controls

import "fmt"

// verdicts maps a control and a gate to the Foreman verdict. G3 send back
// (adapt) needs a note and G4 (ship) is followed by a merge, so neither is
// offered. A test checks every verdict against flow_state.VERDICTS_BY_GATE.
var verdicts = map[string]map[string]string{
	Approve:  {"G1": "approve", "G2": "approve", "G3": "proceed"},
	SendBack: {"G1": "reject", "G2": "reject"},
}

// VerdictFor returns the Foreman verdict for a control on a gate.
func VerdictFor(control, gateID string) (string, error) {
	if v, ok := verdicts[control][gateID]; ok {
		return v, nil
	}
	return "", fmt.Errorf("no %s for gate %s", control, gateID)
}

// Verdicts returns a copy of the table for the page payload.
func Verdicts() map[string]map[string]string {
	out := make(map[string]map[string]string, len(verdicts))
	for c, byGate := range verdicts {
		m := make(map[string]string, len(byGate))
		for g, v := range byGate {
			m[g] = v
		}
		out[c] = m
	}
	return out
}

// Gate script exit codes (0 means the decision was written).
const (
	GateExitChanged = 3 // the open gate id or token differs, or the gate is decided
	GateExitVerdict = 4 // the verdict is not valid for the gate
)

// GateScript runs under "python -I -c". Arguments: lib dir, flow dir,
// verdict, gate id, gate token. It puts the lib dir first on sys.path, loads
// the flow, checks the open gate and calls flow_transition.cmd_gate.
const GateScript = "import sys; sys.path.insert(0, sys.argv[1]); " +
	"import flow_transition as ft, flow_state as fs; " +
	"f = ft.load_flow(sys.argv[2]); g = f.get('gate'); g = g if isinstance(g, dict) else {}; " +
	"sys.exit(3) if (g.get('id') != sys.argv[4] or g.get('token') != sys.argv[5] or g.get('status') == 'decided') else None; " +
	"sys.exit(4) if sys.argv[3] not in fs.valid_verdicts(sys.argv[4]) else None; " +
	"ft.cmd_gate(sys.argv[2], sys.argv[3], note='', surface='burnmon'); sys.exit(0)"

// GateArgv builds the command line that runs GateScript.
func GateArgv(pythonPath, libDir, flowDir, verdict, gateID, token string) []string {
	return []string{pythonPath, "-I", "-c", GateScript, libDir, flowDir, verdict, gateID, token}
}
