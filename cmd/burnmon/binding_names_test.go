package main

import (
	"os"
	"strings"
	"testing"

	"burnmon/internal/winbind"
)

func TestAppBindingNamesCoverPage(t *testing.T) {
	var all strings.Builder
	for _, f := range []string{"../../internal/report/template.html", "app.go", "history_admin.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
		all.WriteByte('\n')
	}
	known := map[string]bool{}
	for _, n := range appBindingNames {
		if known[n] {
			t.Errorf("duplicate binding name %q", n)
		}
		known[n] = true
	}
	if len(appBindingNames) != 10 {
		t.Errorf("len(appBindingNames) = %d, want 10", len(appBindingNames))
	}
	for _, c := range winbind.PageCalls(all.String(), "cc", "bm") {
		if !known[c] {
			t.Errorf("page calls %q but appBindingNames does not list it", c)
		}
	}
}
