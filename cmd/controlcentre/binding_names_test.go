//go:build windows || darwin

package main

import (
	"reflect"
	"sort"
	"testing"

	"burnmon/internal/winbind"
)

func TestCCBindingNamesMatchPage(t *testing.T) {
	if len(ccBindingNames) != 9 {
		t.Fatalf("ccBindingNames has %d names, want 9", len(ccBindingNames))
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
