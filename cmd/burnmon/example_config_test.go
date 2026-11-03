package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"burnmon/internal/pricing"
)

const exampleConfigPath = "../../burnmon.example.json"

func TestExampleConfigParsesAndListsNewKeysAtDefaults(t *testing.T) {
	b, err := os.ReadFile(exampleConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("example config is not valid JSON: %v", err)
	}
	cfg, err := pricing.Load(exampleConfigPath)
	if err != nil {
		t.Fatalf("pricing.Load on the example config: %v", err)
	}
	if cfg.ForemanRoot != "" || cfg.AgentView || cfg.ClaudePath != "" {
		t.Errorf("new keys not at defaults: %q %v %q", cfg.ForemanRoot, cfg.AgentView, cfg.ClaudePath)
	}
	for _, key := range []string{"foreman_root", "agent_view", "claude_path"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("example config lacks key %q", key)
		}
		if _, ok := raw["_comment_"+key]; !ok {
			t.Errorf("example config lacks _comment_%s", key)
		}
	}
	first, _ := raw["_comment"].(string)
	for _, want := range []string{`%LOCALAPPDATA%`, `burnmon.json`, "wins"} {
		if !strings.Contains(first, want) {
			t.Errorf("first _comment lacks %q", want)
		}
	}
	if strings.ContainsRune(string(b), 0x2014) {
		t.Error("example config contains an em dash")
	}
}
