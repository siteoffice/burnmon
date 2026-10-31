package main

import (
	"strings"
	"testing"
)

// d46's fake preview command has the shape and the length of a real gate
// command (python.exe -I -c "<GateScript>" and five arguments), about 600
// characters, so the case tests the line a person really gets.
func TestD46CommandShape(t *testing.T) {
	cmd := d46Command()
	if n := len(cmd); n < 550 || n > 800 {
		t.Errorf("the fake gate command is %d characters, want about 600", n)
	}
	if !strings.Contains(cmd, `python.exe -I -c "import sys; sys.path.insert(0, sys.argv[1]);`) || !strings.HasSuffix(cmd, "-D38C") {
		t.Errorf("the fake gate command does not have the python -I -c shape: %q", cmd)
	}
	for _, r := range cmd {
		if r > 127 {
			t.Fatalf("the fake gate command holds %q, uicheck stays ASCII", r)
		}
	}
}

// d44's keys reach the press only when BurnMon Dev is the foreground window,
// so Enter never lands in another window (B8 Task 8 fix round 1).
func TestKeyIfForeground(t *testing.T) {
	var pressed []uint16
	press := func(vk uint16) error { pressed = append(pressed, vk); return nil }
	if err := keyIfForeground(7, vkReturn, func() uintptr { return 9 }, press); err == nil {
		t.Error("another window in front: no error")
	}
	if len(pressed) != 0 {
		t.Fatalf("another window in front: pressed %v, want nothing", pressed)
	}
	if err := keyIfForeground(7, vkReturn, func() uintptr { return 7 }, press); err != nil {
		t.Fatalf("BurnMon Dev in front: %v", err)
	}
	if len(pressed) != 1 || pressed[0] != vkReturn {
		t.Fatalf("BurnMon Dev in front: pressed %v, want [%#x]", pressed, vkReturn)
	}
}
