package controlsenv

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burnmon/internal/controls"
	"burnmon/internal/pricing"
)

// controlsenv_moved: the moved glue returns what cmd\controlcentre returned.
func TestNewDecisionEnvMatchesOldGlue(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "burnmon-dev-view.json"), []byte(`{"controls":{"approve":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	foreman := t.TempDir()
	audit := filepath.Join(t.TempDir(), "controls-audit.ndjson")
	look := func(name string) (string, error) {
		if name == "python" || name == "python3" {
			if os.PathSeparator == '\\' {
				return `C:\example\python.exe`, nil
			}
			return "/example/" + name, nil
		}
		return "", errors.New("not found")
	}
	env := NewDecisionEnv(pricing.Config{ForemanRoot: foreman, AgentView: true}, dataDir, "snap.json", audit, look)
	if env.ForemanRoot != foreman || !env.AgentViewOn || env.AuditPath != audit {
		t.Errorf("env ForemanRoot %q AgentViewOn %v AuditPath %q", env.ForemanRoot, env.AgentViewOn, env.AuditPath)
	}
	if env.PythonPath == "" || env.ClaudePath != "" {
		t.Errorf("PythonPath %q ClaudePath %q", env.PythonPath, env.ClaudePath)
	}
	if env.Snapshot == nil || env.Switches == nil || env.Self == nil || env.Elevated == nil ||
		env.Runner == nil || env.ReadForeman == nil || env.Merged == nil {
		t.Fatal("env leaves a function nil")
	}
	if sw := env.Switches(); !sw[controls.Approve] || sw[controls.SendBack] {
		t.Errorf("switches = %v", sw)
	}
	if env.GateWithoutSnapshot != "" {
		t.Errorf("GateWithoutSnapshot = %q, the apps keep refuse", env.GateWithoutSnapshot)
	}
}

func TestLoadBurnmonConfigOrder(t *testing.T) {
	write := func(dir, name, root string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"foreman_root":"`+root+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	exe, data := t.TempDir(), t.TempDir()
	write(exe, "burnmon.json", "exe-burnmon")
	write(data, "burnmon.json", "data-burnmon")
	if got := LoadBurnmonConfig(exe, data).ForemanRoot; got != "exe-burnmon" {
		t.Errorf("exe wins: got %q", got)
	}
	if got := LoadBurnmonConfig(t.TempDir(), data).ForemanRoot; got != "data-burnmon" {
		t.Errorf("data folder: got %q", got)
	}
}

// zndconfig_default_refuse
func TestZndConfigDefaultRefuse(t *testing.T) {
	for name, body := range map[string]string{"absent": "", "empty object": `{}`, "empty value": `{"gate_without_snapshot":""}`} {
		dir := t.TempDir()
		if name != "absent" {
			if err := os.WriteFile(filepath.Join(dir, "znd.json"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		z, err := LoadZndConfig("", dir)
		if err != nil || z.GateWithoutSnapshot != controls.GateRefuse {
			t.Errorf("%s: got %+v, %v", name, z, err)
		}
	}
}

// zndconfig_bad_value
func TestZndConfigBadValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "znd.json"), []byte(`{"gate_without_snapshot":"maybe"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadZndConfig("", dir)
	if err == nil || !strings.Contains(err.Error(), "maybe") {
		t.Errorf("err = %v, want one that names the value", err)
	}
}

// zndconfig_tiers
func TestZndConfigTiers(t *testing.T) {
	exe, data := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(exe, "znd.json"), []byte(`{"gate_without_snapshot":"foreman_read"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "znd.json"), []byte(`{"gate_without_snapshot":"refuse"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	z, err := LoadZndConfig(exe, data)
	if err != nil || z.GateWithoutSnapshot != controls.GateForemanRead {
		t.Errorf("exe tier: %+v, %v", z, err)
	}
	z, err = LoadZndConfig(t.TempDir(), data)
	if err != nil || z.GateWithoutSnapshot != controls.GateRefuse {
		t.Errorf("data tier: %+v, %v", z, err)
	}
}
