package main

import "testing"

func TestDevSettingsDirSet(t *testing.T) {
	want := t.TempDir()
	t.Setenv(devSettingsEnv, want)
	got, err := devSettingsDir()
	if err != nil {
		t.Fatalf("devSettingsDir: %v", err)
	}
	if got != want {
		t.Fatalf("devSettingsDir = %q, want %q", got, want)
	}
}

// Unset must error, never fall back to the real %LOCALAPPDATA%\burnmon.
func TestDevSettingsDirUnset(t *testing.T) {
	t.Setenv(devSettingsEnv, "")
	got, err := devSettingsDir()
	if err == nil {
		t.Fatalf("devSettingsDir = %q with the variable unset, want an error", got)
	}
}
