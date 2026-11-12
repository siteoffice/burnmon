//go:build windows || darwin

package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestPageWaiting(t *testing.T) {
	pw := pageFunc(t, "pageWaiting")
	for _, want := range []string{"state.waiting", "counts.longest_wait_days", "pager(", "The longest waits",
		"'Project'", "'Flow'", "'Stage'", "'Waits'", "waitDetail("} {
		if !strings.Contains(pw, want) {
			t.Errorf("pageWaiting lacks %s", want)
		}
	}
	for _, bad := range []string{".sort(", ".reverse("} {
		if strings.Contains(pw, bad) {
			t.Errorf("pageWaiting reorders rows with %s", bad)
		}
	}
	wd := pageFunc(t, "waitDetail")
	for _, want := range []string{"'Approve'", "'Send back'", "'primary'", "decide('approve'", "decide('send_back'"} {
		if !strings.Contains(wd, want) {
			t.Errorf("waitDetail lacks %s", want)
		}
	}
	d := pageFunc(t, "decide")
	for _, want := range []string{"api.preview(", "api.run(", "'Confirm'", "'Cancel'", "audit_path", "loadState()", "setTimeout(", ", 500)"} {
		if !strings.Contains(d, want) {
			t.Errorf("decide lacks %s", want)
		}
	}
	// The page sends only control with project and slug (a gate) or with the
	// agent id (Stop, Task 12), and the token: never a command, a path or an argv.
	if !strings.Contains(d, "api.preview(req)") {
		t.Fatal("decide must call api.preview with the request it built")
	}
	m := regexp.MustCompile(`var req = stop \? \{([^}]*)\} : \{([^}]*)\};`).FindStringSubmatch(d)
	if m == nil {
		t.Fatal("decide does not build req from two object literals")
	}
	for i, allowed := range []map[string]bool{{"control": true, "agent": true}, {"control": true, "project": true, "slug": true}} {
		for _, kv := range strings.Split(m[i+1], ",") {
			key := strings.TrimSpace(strings.SplitN(kv, ":", 2)[0])
			if !allowed[key] {
				t.Errorf("api.preview sends %q", key)
			}
		}
	}
	if !regexp.MustCompile(`api\.run\(\w+\.token\)`).MatchString(d) {
		t.Error("decide must call api.run with the preview token only")
	}
}

// TestPageDecideAuditLineGuarded: the audit line shows only with a path, and
// the "longest waits" sentence only for a wait of 0 days or more.
func TestPageDecideAuditLineGuarded(t *testing.T) {
	d := pageFunc(t, "decide")
	if !regexp.MustCompile(`if \(rv\.audit_path\) lines\.push\(\['Audit file ' \+ rv\.audit_path`).MatchString(d) {
		t.Error("decide must add the audit file line only when audit_path is set")
	}
	pw := pageFunc(t, "pageWaiting")
	if !strings.Contains(pw, "n && d >= 0 ?") {
		t.Error("pageWaiting must leave the longest-waits sentence out when longest_wait_days < 0")
	}
}
