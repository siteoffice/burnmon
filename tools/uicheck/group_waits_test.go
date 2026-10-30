package main

import "testing"

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
