package main

import (
	"strings"
	"testing"
)

// charts/d11: the fit pass budget gets one stubbed tick over 250 ms that must
// fail, one under it that must pass, and one with no mark to time it from.

func d11Tick(todoT, endT float64) []d11Mark {
	return []d11Mark{{Name: "clock", T: 0}, {Name: "todo", T: todoT}, {Name: "tickEnd", T: endT}}
}

func TestD11FitPassOverBudgetFails(t *testing.T) {
	ms, err := d11CheckFitPass(3, d11Tick(100, 400))
	if err == nil {
		t.Fatalf("a 300 ms fit pass passed; want it to fail")
	}
	if ms != 300 {
		t.Fatalf("fit pass = %.1f ms, want 300", ms)
	}
	if !strings.Contains(err.Error(), "d11: tick 3: fit pass took 300 ms, budget 250 ms") {
		t.Fatalf("error = %q", err)
	}
}

func TestD11FitPassUnderBudgetPasses(t *testing.T) {
	ms, err := d11CheckFitPass(0, d11Tick(100, 300))
	if err != nil {
		t.Fatalf("a 200 ms fit pass failed: %v", err)
	}
	if ms != 200 {
		t.Fatalf("fit pass = %.1f ms, want 200", ms)
	}
}

func TestD11FitPassNoTodoMarkFails(t *testing.T) {
	tick := []d11Mark{{Name: "clock", T: 0}, {Name: "tickEnd", T: 10}}
	if _, err := d11CheckFitPass(1, tick); err == nil {
		t.Fatalf("a tick with no todo mark passed; want it to fail")
	}
}
