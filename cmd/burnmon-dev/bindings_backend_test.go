//go:build windows || (darwin && cgo)

package main

import (
	"reflect"
	"sort"
	"testing"

	"burnmon/internal/winbind"
)

func TestBindDevRegistersSharedList(t *testing.T) {
	rec := &winbind.Recorder{}
	bindDev(devBindEnv{w: rec, a: &app{}, setDir: t.TempDir(), dataDir: t.TempDir()})
	got := append([]string(nil), rec.Names...)
	want := append([]string(nil), devBindingNames...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bound names differ from devBindingNames\n got: %v\nwant: %v", got, want)
	}
}
