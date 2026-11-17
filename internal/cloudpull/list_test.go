package cloudpull

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const orgA = "0a0a0a0a-1111-2222-3333-444444444444"
const orgB = "0b0b0b0b-1111-2222-3333-444444444444"

// writeList puts a remote-session-spaces.json for one account and org under root.
func writeList(t *testing.T, root, account, org, body string) {
	t.Helper()
	dir := filepath.Join(root, "local-agent-mode-sessions", account, org)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, listFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSessionListMatchingOrg(t *testing.T) {
	root := t.TempDir()
	writeList(t, root, "acc1", orgA, `{"entries":[
		{"sessionId":"session_B","spaceId":"sp1","folders":["C:\\x"],"memoryEnabled":true},
		{"sessionId":"session_A","folders":[]},
		{"sessionId":"","folders":[]},
		{"sessionId":"session_B"},
		{"sessionId":"local_C"},
		{"spaceId":"sp2"},
		{"sessionId":"session_D"}]}`)
	writeList(t, root, "acc2", orgB, `{"entries":[{"sessionId":"session_OTHER"}]}`)

	got, err := SessionList([]string{root}, orgA)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"session_B", "session_A", "session_D"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SessionList = %v, want %v", got, want)
	}
}

func TestSessionListOtherOrgOnly(t *testing.T) {
	root := t.TempDir()
	writeList(t, root, "acc2", orgB, `{"entries":[{"sessionId":"session_OTHER"}]}`)
	if _, err := SessionList([]string{root}, orgA); !errors.Is(err, ErrNoList) {
		t.Fatalf("err = %v, want ErrNoList", err)
	}
}

func TestSessionListMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if _, err := SessionList([]string{missing}, orgA); !errors.Is(err, ErrNoList) {
		t.Fatalf("err = %v, want ErrNoList", err)
	}
	if _, err := SessionList(nil, orgA); !errors.Is(err, ErrNoList) {
		t.Fatalf("no roots: err = %v, want ErrNoList", err)
	}
}

func TestSessionListBrokenJSON(t *testing.T) {
	root := t.TempDir()
	writeList(t, root, "acc1", orgA, `{"entries":[`)
	_, err := SessionList([]string{root}, orgA)
	if err == nil || errors.Is(err, ErrNoList) {
		t.Fatalf("err = %v, want a parse error", err)
	}
}

// An org id that is not a plain id must never reach the glob: it could match
// another org's folder.
func TestSessionListRejectsBadOrg(t *testing.T) {
	root := t.TempDir()
	writeList(t, root, "acc1", orgA, `{"entries":[{"sessionId":"session_A"}]}`)
	for _, org := range []string{"", "*", "..", orgA + `\..`, "0a0a?a0a"} {
		if _, err := SessionList([]string{root}, org); err == nil {
			t.Errorf("org %q: no error", org)
		}
	}
}

// Two roots (an old and a moved data folder) with the same org: ids merge in
// root order, each once.
func TestSessionListTwoRoots(t *testing.T) {
	r1, r2 := t.TempDir(), t.TempDir()
	writeList(t, r1, "acc1", orgA, `{"entries":[{"sessionId":"session_A"},{"sessionId":"session_B"}]}`)
	writeList(t, r2, "acc1", orgA, `{"entries":[{"sessionId":"session_B"},{"sessionId":"session_C"}]}`)
	got, err := SessionList([]string{r1, r2}, orgA)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"session_A", "session_B", "session_C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SessionList = %v, want %v", got, want)
	}
}

func TestParseAuthOrg(t *testing.T) {
	got, err := ParseAuthOrg([]byte(`{"loggedIn":true,"authMethod":"claude.ai","orgId":"` + orgA + `","orgName":"x"}`))
	if err != nil || got != orgA {
		t.Fatalf("ParseAuthOrg = %q, %v; want %q", got, err, orgA)
	}
	for name, in := range map[string]string{
		"logged out": `{"loggedIn":false,"orgId":"` + orgA + `"}`,
		"no org":     `{"loggedIn":true}`,
		"not json":   `Not logged in`,
	} {
		if _, err := ParseAuthOrg([]byte(in)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestDesktopDataRoots(t *testing.T) {
	lad, ad, home := t.TempDir(), t.TempDir(), t.TempDir()
	mk := func(p string) string {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	moved := mk(filepath.Join(lad, "Claude-Data", "claude_abc"))
	msix := mk(filepath.Join(lad, "Packages", "Claude_abc", "LocalCache", "Roaming", "Claude"))
	roaming := mk(filepath.Join(ad, "Claude"))

	got := desktopDataRoots(lad, ad, home)
	for _, want := range []string{moved, msix, roaming} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("desktopDataRoots missing %s, got %v", want, got)
		}
	}
	for _, g := range got {
		if fi, err := os.Stat(g); err != nil || !fi.IsDir() {
			t.Errorf("desktopDataRoots returned a folder that does not exist: %s", g)
		}
	}
}

// The real list on this laptop, measured 2026-11-17: 141 ids, the hub session
// among them. Skipped on any computer without that file.
func TestSessionListThisLaptop(t *testing.T) {
	const org = "a26a6687-7ab1-41ff-bb66-b6c8ba986ee8"
	got, err := SessionList(DesktopDataRoots(), org)
	if errors.Is(err, ErrNoList) {
		t.Skip("no desktop session list for this org on this computer")
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 141 {
		t.Errorf("got %d ids, want at least 141", len(got))
	}
	hub := false
	for _, id := range got {
		if id == "session_0131Y29YSSRdAPR2VmEG1ET3" {
			hub = true
		}
	}
	if !hub {
		t.Error("hub session session_0131Y29YSSRdAPR2VmEG1ET3 not in the list")
	}
}
