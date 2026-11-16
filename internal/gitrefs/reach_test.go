package gitrefs

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeTag writes a loose annotated tag object pointing at target.
func writeTag(t *testing.T, gitDir, target string) string {
	t.Helper()
	body := []byte("object " + target + "\ntype commit\ntag v1\ntagger example <e@example> 0 +0000\n\nmsg\n")
	sha := objectSHA("tag", body)
	var raw bytes.Buffer
	fmt.Fprintf(&raw, "tag %d\x00", len(body))
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

// chain: c1 <- c2 <- c3 (main), side branch s1 off c2, merge m (parents c3, s1).
func reachGraph(t *testing.T, gd string) (c1, c2, c3, s1, m string) {
	c1 = writeCommit(t, gd)
	c2 = writeCommit(t, gd, c1)
	c3 = writeCommit(t, gd, c2)
	s1 = writeCommit(t, gd, c2)
	m = writeCommit(t, gd, c3, s1)
	return
}

func TestReachContains(t *testing.T) {
	_, gd := newRepo(t)
	c1, _, c3, s1, m := reachGraph(t, gd)
	r := OpenReach(gd)
	defer r.Close()
	if in, known := r.Contains(m, s1); !in || !known {
		t.Errorf("merge contains side tip: %v %v", in, known)
	}
	if in, known := r.Contains(c3, s1); in || !known {
		t.Errorf("c3 does not contain side tip: %v %v", in, known)
	}
	if in, known := r.Contains(m, c1); !in || !known {
		t.Errorf("merge contains root: %v %v", in, known)
	}
	if in, known := r.Contains(m, m); !in || !known {
		t.Errorf("a commit contains itself: %v %v", in, known)
	}
}

func TestReachNotIn(t *testing.T) {
	_, gd := newRepo(t)
	_, _, c3, s1, m := reachGraph(t, gd)
	r := OpenReach(gd)
	defer r.Close()
	if n, known := r.NotIn(s1, c3); n != 1 || !known {
		t.Errorf("side tip not in c3 = %d %v, want 1", n, known)
	}
	if n, known := r.NotIn(m, c3); n != 2 || !known { // m and s1
		t.Errorf("merge not in c3 = %d %v, want 2", n, known)
	}
	if n, known := r.NotIn(c3, m); n != 0 || !known {
		t.Errorf("c3 not in merge = %d %v, want 0", n, known)
	}
}

func TestReachUnknownOnMissingObject(t *testing.T) {
	_, gd := newRepo(t)
	r := OpenReach(gd)
	defer r.Close()
	missing := "1111111111111111111111111111111111111111"
	if _, known := r.NotIn(missing, missing[:39]+"2"); known {
		t.Error("a missing object must read as unknown")
	}
}

func TestReachPeel(t *testing.T) {
	_, gd := newRepo(t)
	c1, _, _, _, _ := reachGraph(t, gd)
	tag := writeTag(t, gd, c1)
	r := OpenReach(gd)
	defer r.Close()
	if got, ok := r.Peel(tag); !ok || got != c1 {
		t.Errorf("peel annotated = %s %v", got, ok)
	}
	if got, ok := r.Peel(c1); !ok || got != c1 {
		t.Errorf("peel lightweight = %s %v", got, ok)
	}
}

func TestReachFirstParentSteps(t *testing.T) {
	_, gd := newRepo(t)
	c1, _, c3, s1, m := reachGraph(t, gd)
	r := OpenReach(gd)
	defer r.Close()
	if n, found, known := r.FirstParentSteps(m, c3); n != 1 || !found || !known {
		t.Errorf("m to c3 = %d %v %v, want 1 true true", n, found, known)
	}
	if n, found, known := r.FirstParentSteps(m, c1); n != 3 || !found || !known {
		t.Errorf("m to c1 = %d %v %v, want 3 true true", n, found, known)
	}
	if n, found, known := r.FirstParentSteps(m, m); n != 0 || !found || !known {
		t.Errorf("m to m = %d %v %v", n, found, known)
	}
	// s1 is not on m's first-parent line
	if _, found, known := r.FirstParentSteps(m, s1); found || !known {
		t.Errorf("side tip found on first-parent line: %v %v", found, known)
	}
}
