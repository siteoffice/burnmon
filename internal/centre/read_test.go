package centre

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestInsideRefusesEscapes(t *testing.T) {
	root := fixtureTree(t)
	outside := t.TempDir()
	writeFile(t, outside, "secret.md", "outside root\n")

	refused := []string{
		`..\x`,
		"../x",
		"a/../../x",
		filepath.Join(outside, "secret.md"),
		"/x",
		"",
	}
	for _, rel := range refused {
		if abs, err := Inside(root, rel); err == nil {
			t.Errorf("Inside(%q) = %q, want a refusal", rel, abs)
		}
	}

	abs, err := Inside(root, "projects/kettle/STATUS.md")
	if err != nil {
		t.Fatalf("Inside(projects/kettle/STATUS.md): %v", err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(realRoot, "projects", "kettle", "STATUS.md"); !strings.EqualFold(abs, want) {
		t.Errorf("Inside = %q, want %q", abs, want)
	}

	t.Run("symlink", func(t *testing.T) {
		if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
			t.Skipf("this OS refused to create a symlink (%v); the link escape is not tested here", err)
		}
		if abs, err := Inside(root, "link/secret.md"); err == nil {
			t.Errorf("Inside(link/secret.md) = %q through a link that leaves root, want a refusal", abs)
		} else if !strings.Contains(err.Error(), "outside root") {
			t.Errorf("link refusal %q does not say outside root", err)
		}
	})
	t.Run("junction", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("junctions exist only on Windows")
		}
		out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(root, "junction"), outside).CombinedOutput()
		if err != nil {
			t.Skipf("mklink /J failed (%v: %s); the junction escape is not tested here", err, out)
		}
		if _, err := os.ReadFile(filepath.Join(root, "junction", "secret.md")); err != nil {
			t.Skipf("the junction does not read through (%v)", err)
		}
		if abs, err := Inside(root, "junction/secret.md"); err == nil {
			t.Errorf("Inside(junction/secret.md) = %q through a junction that leaves root, want a refusal", abs)
		}
	})
}

func TestBlocks(t *testing.T) {
	got := Blocks("# T\n\npara one\nline\n\n\n```\na\n\nb\n```\n")
	want := []Block{
		{Text: "# T", Heading: true},
		{Text: "para one\nline"},
		{Text: "```\na\n\nb\n```"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Blocks = %#v, want %#v", got, want)
	}
	if got := Blocks("a\r\n\r\nb\r\n"); len(got) != 2 || got[0].Text != "a" || got[1].Text != "b" {
		t.Errorf("CRLF Blocks = %#v", got)
	}
	if got := Blocks(""); got == nil || len(got) != 0 {
		t.Errorf("empty Blocks = %#v, want an empty list", got)
	}
}

func TestReadUnitFileOK(t *testing.T) {
	root := fixtureTree(t)
	ft, err := ReadUnitFile(root, "projects/kettle/STATUS.md")
	if err != nil {
		t.Fatal(err)
	}
	if ft.Path != "projects/kettle/STATUS.md" || ft.Role != "State" {
		t.Errorf("path %q role %q", ft.Path, ft.Role)
	}
	info, err := os.Stat(filepath.Join(root, "projects", "kettle", "STATUS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if ft.Size != info.Size() || ft.Modified != "2026-11-01T00:00:00Z" {
		t.Errorf("size %d modified %q", ft.Size, ft.Modified)
	}
	if len(ft.Blocks) < 2 || !ft.Blocks[0].Heading || ft.Blocks[0].Text != "# Kettle status" {
		t.Errorf("blocks %#v", ft.Blocks)
	}
}

func TestReadUnitFileRefusals(t *testing.T) {
	root := fixtureTree(t)
	cases := map[string]string{
		"projects/ghost/README.md":            "not a unit",
		"notes.txt":                           "not in the unit files list",
		"projects/kettle/../kettle/STATUS.md": ".. segment",
		"_archive/siteoffice.json":            "skipped folder",
		"deep/l2/l3/l4/l5/siteoffice.json":    "deeper than",
	}
	if runtime.GOOS == "windows" {
		// Windows finds names without case and drops a trailing dot, so these
		// reach the skipped _archive folder on disk.
		cases["_ARCHIVE/siteoffice.json"] = "skipped folder"
		cases["_archive./siteoffice.json"] = "skipped folder"
	}
	for rel, reason := range cases {
		_, err := ReadUnitFile(root, rel)
		if err == nil {
			t.Errorf("ReadUnitFile(%q) read, want a refusal", rel)
			continue
		}
		if !strings.Contains(err.Error(), reason) {
			t.Errorf("ReadUnitFile(%q) = %q, want the reason %q", rel, err, reason)
		}
	}

	many := t.TempDir()
	writeFile(t, many, "siteoffice.json", `{"name":"many"}`)
	for i := 1; i <= 41; i++ {
		writeFile(t, many, "a"+twoDigits(i)+".md", "x\n")
	}
	if _, err := ReadUnitFile(many, "a40.md"); err != nil {
		t.Errorf("40th file refused: %v", err)
	}
	if _, err := ReadUnitFile(many, "a41.md"); err == nil || !strings.Contains(err.Error(), "not in the unit files list") {
		t.Errorf("41st file: %v, want not in the unit files list", err)
	}

	big := t.TempDir()
	writeFile(t, big, "siteoffice.json", `{"name":"big"}`)
	writeFile(t, big, "big.md", strings.Repeat("x", MaxFileBytes+1))
	writeFile(t, big, "fits.md", strings.Repeat("x", MaxFileBytes))
	if _, err := ReadUnitFile(big, "big.md"); err == nil || !strings.Contains(err.Error(), "larger than 512 KB") {
		t.Errorf("big.md: %v, want larger than 512 KB", err)
	}
	if _, err := ReadUnitFile(big, "fits.md"); err != nil {
		t.Errorf("fits.md of exactly 512 KB refused: %v", err)
	}
}

func twoDigits(i int) string {
	return string([]byte{byte('0' + i/10), byte('0' + i%10)})
}

func TestReadBoard(t *testing.T) {
	root := fixtureTree(t)
	rel := "_board/flows/kettle/kettle-brew-gate/index.html"
	page, err := ReadBoard(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if page.Path != rel || page.HTML != string(want) {
		t.Errorf("page %q holds %d bytes, want %d", page.Path, len(page.HTML), len(want))
	}

	outside := t.TempDir()
	writeFile(t, outside, "_board/index.html", "<p>out</p>")
	writeFile(t, root, "projects/kettle/index.html", "<p>no board</p>")
	writeFile(t, root, "_board/big/index.html", strings.Repeat("x", MaxBoardBytes+1))
	cases := map[string]string{
		filepath.Join(outside, "_board", "index.html"): "absolute path",
		"_board/flows/index.json":                      "not an index.html",
		"projects/kettle/index.html":                   "no _board segment",
		"_board/big/index.html":                        "larger than 2 MB",
	}
	for rel, reason := range cases {
		_, err := ReadBoard(root, rel)
		if err == nil {
			t.Errorf("ReadBoard(%q) read, want a refusal", rel)
			continue
		}
		if !strings.Contains(err.Error(), reason) {
			t.Errorf("ReadBoard(%q) = %q, want the reason %q", rel, err, reason)
		}
	}
}
