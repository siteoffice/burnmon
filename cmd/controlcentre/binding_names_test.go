//go:build windows || darwin

package main

import (
	"reflect"
	"sort"
	"testing"

	"burnmon/internal/winbind"
)

func TestCCBindingNamesMatchPage(t *testing.T) {
	// 17 since 2026-11-15: ccControls and ccControlSet (the four controls moved from BurnMon Dev's gear).
	if len(ccBindingNames) != 17 {
		t.Fatalf("ccBindingNames has %d names, want 17", len(ccBindingNames))
	}
	seen := map[string]bool{}
	for _, n := range ccBindingNames {
		if seen[n] {
			t.Errorf("duplicate binding name %q", n)
		}
		seen[n] = true
	}
	want := append([]string(nil), ccBindingNames...)
	sort.Strings(want)
	if got := winbind.PageCalls(pageHTML, "cc"); !reflect.DeepEqual(got, want) {
		t.Fatalf("page calls differ from ccBindingNames\n got: %v\nwant: %v", got, want)
	}
}
