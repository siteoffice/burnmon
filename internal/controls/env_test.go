package controls

import (
	"errors"
	"runtime"
	"testing"

	"burnmon/internal/pricing"
)

// TestWhyElevatedText pins the elevation refusal the controls service gives.
func TestWhyElevatedText(t *testing.T) {
	if WhyElevated != "BurnMon Dev runs as administrator: controls are off" {
		t.Errorf("WhyElevated = %q", WhyElevated)
	}
}

// TestNewEnvFields: the fields both apps used to set by hand.
func TestNewEnvFields(t *testing.T) {
	cfg := pricing.Config{AgentView: true, ForemanRoot: `C:\example\foreman`}
	switches := func() map[string]bool { return map[string]bool{"stop": true} }
	failLook := func(string) (string, error) { return "", errors.New("not found") }

	env := NewEnv(cfg, "snap.json", "audit.ndjson", switches, failLook)

	if !env.AgentViewOn || env.ForemanRoot != cfg.ForemanRoot {
		t.Errorf("AgentViewOn=%v ForemanRoot=%q", env.AgentViewOn, env.ForemanRoot)
	}
	if env.AuditPath != "audit.ndjson" {
		t.Errorf("AuditPath = %q", env.AuditPath)
	}
	if env.ReadForeman == nil || env.Snapshot == nil || env.Self == nil ||
		env.Elevated == nil || env.Runner == nil || env.Merged == nil {
		t.Errorf("a required field is nil: %+v", env)
	}
	if got := env.Switches(); !got["stop"] {
		t.Errorf("Switches() = %v", got)
	}
	if _, ok := env.Runner.(ExecRunner); !ok {
		t.Errorf("Runner = %T, want ExecRunner", env.Runner)
	}
	// Nothing resolves: both paths stay empty and the agent view stays as configured.
	if env.ClaudePath != "" || env.PythonPath != "" {
		t.Errorf("ClaudePath=%q PythonPath=%q, want empty", env.ClaudePath, env.PythonPath)
	}
	if env.AgentView != nil {
		t.Error("AgentView must stay unset when claude does not resolve")
	}
}

// TestNewEnvResolvesPython: a PATH lookup that finds python fills PythonPath.
func TestNewEnvResolvesPython(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("controls run on Windows and macOS")
	}
	look := func(name string) (string, error) {
		if name == "python" || name == "python3" || name == "python.exe" {
			return `C:\py\` + name, nil
		}
		return "", errors.New("not found")
	}
	want, werr := ResolvePython(look, runtime.GOOS)
	env := NewEnv(pricing.Config{}, "s", "a", func() map[string]bool { return nil }, look)
	if werr != nil {
		if env.PythonPath != "" {
			t.Errorf("PythonPath = %q, want empty", env.PythonPath)
		}
		return
	}
	if env.PythonPath != want {
		t.Errorf("PythonPath = %q, want %q", env.PythonPath, want)
	}
}
