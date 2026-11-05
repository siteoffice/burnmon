//go:build windows || (darwin && cgo)

package main

import (
	"reflect"
	"sort"
	"testing"

	"burnmon/internal/winbind"
)

func TestBindAppRegistersSharedList(t *testing.T) {
	rec := &winbind.Recorder{}
	bindApp(rec, &app{}, nil, func() {})
	got := append([]string(nil), rec.Names...)
	want := append([]string(nil), appBindingNames...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bound %v, want %v", got, want)
	}
}
