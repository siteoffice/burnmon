package gitrefs

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// writeLoose writes body as a loose object of type typ and returns its name.
func writeLoose(t *testing.T, gitDir, typ string, body []byte) string {
	t.Helper()
	sha := objectSHA(typ, body)
	var raw bytes.Buffer
	fmt.Fprintf(&raw, "%s %d\x00", typ, len(body))
	raw.Write(body)
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(raw.Bytes())
	zw.Close()
	p := looseObjectPath(gitDir, sha)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, z.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return sha
}

func TestCommitInfoReadsCommitterTimeAndSubject(t *testing.T) {
	_, gd := newRepo(t)
	body := []byte("tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\n" +
		"author A <a@example> 1700000000 +0100\n" +
		"committer C <c@example> 1763200000 +0100\n\n" +
		"fix(centre): the subject line\n\nThe body.\n")
	sha := writeLoose(t, gd, "commit", body)
	c, err := CommitInfo(gd, sha)
	if err != nil {
		t.Fatal(err)
	}
	if !c.When.Equal(time.Unix(1763200000, 0)) {
		t.Errorf("When = %v, want committer time 1763200000", c.When)
	}
	if c.Subject != "fix(centre): the subject line" {
		t.Errorf("Subject = %q", c.Subject)
	}
}

func TestCommitInfoRefusesNonCommit(t *testing.T) {
	_, gd := newRepo(t)
	sha := writeLoose(t, gd, "blob", []byte("hello\n"))
	if _, err := CommitInfo(gd, sha); err == nil {
		t.Error("CommitInfo on a blob: want an error")
	}
	if _, err := CommitInfo(gd, "nothex"); err == nil {
		t.Error("CommitInfo on a bad name: want an error")
	}
}

func TestListRefsLooseAndPacked(t *testing.T) {
	_, gd := newRepo(t)
	a := writeCommit(t, gd)
	b := writeCommit(t, gd, a)
	write := func(rel, s string) {
		p := filepath.Join(gd, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("refs/heads/main", b+"\n")
	write("refs/heads/sprint/x", a+"\n")
	write("refs/tags/v1", a+"\n")
	// packed: one shadowed by a loose file, one only packed, a peeled line.
	write("packed-refs", "# pack-refs with: peeled fully-peeled sorted\n"+
		a+" refs/heads/main\n"+
		a+" refs/heads/old\n"+
		b+" refs/tags/v2\n^"+a+"\n")
	heads, err := ListRefs(gd, "refs/heads/")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"main": b, "sprint/x": a, "old": a}
	if !reflect.DeepEqual(heads, want) {
		t.Errorf("heads = %v, want %v", heads, want)
	}
	tags, _ := ListRefs(gd, "refs/tags/")
	if !reflect.DeepEqual(tags, map[string]string{"v1": a, "v2": b}) {
		t.Errorf("tags = %v", tags)
	}
}

func TestListRefsNoRefs(t *testing.T) {
	_, gd := newRepo(t)
	got, err := ListRefs(gd, "refs/tags/")
	if err != nil || len(got) != 0 {
		t.Errorf("ListRefs = %v, %v; want empty, nil", got, err)
	}
	if _, err := ListRefs(gd, "../x/"); err == nil {
		t.Error("a prefix outside refs/ must be refused")
	}
}
