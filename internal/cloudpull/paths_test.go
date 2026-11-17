package cloudpull

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setHome points the home folder and the Claude config folder at a temp dir.
func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	return home
}

func TestDefaultPaths(t *testing.T) {
	home := setHome(t)
	pull := filepath.Join(home, ".burnmon", "cloud-pull")
	if got := PullDir(); got != pull {
		t.Errorf("PullDir = %q, want %q", got, pull)
	}
	want := filepath.Join(home, ".claude", "projects", ProjectDirName(pull))
	if got := ProjectDir(); got != want {
		t.Errorf("ProjectDir = %q, want %q", got, want)
	}
	if got := StatePath(); got != filepath.Join(home, ".burnmon", "cloud-pull-state.json") {
		t.Errorf("StatePath = %q", got)
	}
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	if got := ProjectDir(); got != filepath.Join(cfg, "projects", ProjectDirName(pull)) {
		t.Errorf("CLAUDE_CONFIG_DIR not honoured: %q", got)
	}
}

func TestMapPullFile(t *testing.T) {
	setHome(t)
	st := &State{Sessions: map[string]*Entry{
		"session_A": {SessionID: "aaaa-1", Path: "x", PulledAt: time.Now()},
	}}
	if err := st.Save(StatePath()); err != nil {
		t.Fatal(err)
	}
	pd := ProjectDir()
	cases := []struct {
		path         string
		cloud        string
		isPull, mapd bool
	}{
		{filepath.Join(pd, "aaaa-1.jsonl"), "session_A", true, true},
		{filepath.Join(pd, "bbbb-2.jsonl"), "", true, false},
		{filepath.Join(pd, "aaaa-1", "subagents", "agent-x.jsonl"), "", true, false},
		{filepath.Join(filepath.Dir(pd), "other", "aaaa-1.jsonl"), "", false, false},
	}
	for _, c := range cases {
		cloud, isPull, mapped := MapPullFile(c.path)
		if cloud != c.cloud || isPull != c.isPull || mapped != c.mapd {
			t.Errorf("MapPullFile(%s) = %q, %v, %v; want %q, %v, %v", c.path, cloud, isPull, mapped, c.cloud, c.isPull, c.mapd)
		}
	}
	// No state file: a pull file is a pull file, but not mapped.
	if err := os.Remove(StatePath()); err != nil {
		t.Fatal(err)
	}
	if _, isPull, mapped := MapPullFile(filepath.Join(pd, "aaaa-1.jsonl")); !isPull || mapped {
		t.Errorf("no state: isPull %v mapped %v; want true, false", isPull, mapped)
	}
}
