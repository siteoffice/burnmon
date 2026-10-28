package gitrefs

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var commitSeq int

// objectSHA is the git object name: sha1 of "<type> <len>\x00<body>".
func objectSHA(typ string, body []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s %d\x00", typ, len(body))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// commitObj builds a commit body with the given parents. The message carries a
// counter so two commits with the same parents still get different names.
func commitObj(parents ...string) (string, []byte) {
	commitSeq++
	var b bytes.Buffer
	b.WriteString("tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\n")
	for _, p := range parents {
		b.WriteString("parent " + p + "\n")
	}
	b.WriteString("author example <e@example> 0 +0000\ncommitter example <e@example> 0 +0000\n\n")
	fmt.Fprintf(&b, "example %d\n", commitSeq)
	body := b.Bytes()
	return objectSHA("commit", body), body
}

// newRepo makes <tmp>\.git\objects and <tmp>\.git\refs\heads.
func newRepo(t *testing.T) (repo, gitDir string) {
	t.Helper()
	repo = t.TempDir()
	gitDir = filepath.Join(repo, ".git")
	for _, d := range []string{"objects", filepath.Join("refs", "heads")} {
		if err := os.MkdirAll(filepath.Join(gitDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return repo, gitDir
}

func looseObjectPath(gitDir, sha string) string {
	return filepath.Join(gitDir, "objects", sha[:2], sha[2:])
}

// writeCommit writes a loose, zlib-compressed commit and returns its name.
func writeCommit(t *testing.T, gitDir string, parents ...string) string {
	t.Helper()
	sha, body := commitObj(parents...)
	var raw bytes.Buffer
	fmt.Fprintf(&raw, "commit %d\x00", len(body))
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

func setRef(t *testing.T, gitDir, name, sha string) {
	t.Helper()
	p := filepath.Join(gitDir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(sha+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setPacked(t *testing.T, gitDir string, refs map[string]string) {
	t.Helper()
	names := make([]string, 0, len(refs))
	for n := range refs {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("# pack-refs with: peeled fully-peeled sorted \n")
	for _, n := range names {
		b.WriteString(refs[n] + " " + n + "\n")
		// A peel line follows a tag entry; it must be ignored.
		if strings.HasPrefix(n, "refs/tags/") {
			b.WriteString("^" + strings.Repeat("a", 40) + "\n")
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "packed-refs"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// graph builds the merged and unmerged fixture and returns the names:
// A; B (parent A, sprint/example-merged); C (parent A); M (parents C, B, main);
// D (parent B, sprint/example-open). Refs are not written.
type fixture struct{ a, b, c, m, d string }

func buildGraph(t *testing.T, gitDir string) fixture {
	t.Helper()
	var f fixture
	f.a = writeCommit(t, gitDir)
	f.b = writeCommit(t, gitDir, f.a)
	f.c = writeCommit(t, gitDir, f.a)
	f.m = writeCommit(t, gitDir, f.c, f.b)
	f.d = writeCommit(t, gitDir, f.b)
	return f
}

// writePack writes the given commit bodies as one non-delta pack with a v2
// index, so pack reading is tested without a git binary.
func writePack(t *testing.T, gitDir string, bodies [][]byte) (packPath, idxPath string) {
	t.Helper()
	type ent struct {
		name   [20]byte
		crc    uint32
		offset uint32
	}
	var pack bytes.Buffer
	pack.WriteString("PACK")
	binary.Write(&pack, binary.BigEndian, uint32(2))
	binary.Write(&pack, binary.BigEndian, uint32(len(bodies)))
	var ents []ent
	for _, body := range bodies {
		start := pack.Len()
		size := len(body)
		c := byte(1<<4) | byte(size&15)
		size >>= 4
		if size > 0 {
			c |= 0x80
		}
		pack.WriteByte(c)
		for size > 0 {
			c = byte(size & 0x7f)
			size >>= 7
			if size > 0 {
				c |= 0x80
			}
			pack.WriteByte(c)
		}
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write(body)
		zw.Close()
		pack.Write(z.Bytes())
		var e ent
		raw, _ := hex.DecodeString(objectSHA("commit", body))
		copy(e.name[:], raw)
		e.crc = crc32.ChecksumIEEE(pack.Bytes()[start:])
		e.offset = uint32(start)
		ents = append(ents, e)
	}
	sum := sha1.Sum(pack.Bytes())
	pack.Write(sum[:])

	sort.Slice(ents, func(i, j int) bool { return bytes.Compare(ents[i].name[:], ents[j].name[:]) < 0 })
	var idx bytes.Buffer
	idx.Write([]byte{0xff, 't', 'O', 'c'})
	binary.Write(&idx, binary.BigEndian, uint32(2))
	var fan [256]uint32
	for _, e := range ents {
		for i := int(e.name[0]); i < 256; i++ {
			fan[i]++
		}
	}
	for _, v := range fan {
		binary.Write(&idx, binary.BigEndian, v)
	}
	for _, e := range ents {
		idx.Write(e.name[:])
	}
	for _, e := range ents {
		binary.Write(&idx, binary.BigEndian, e.crc)
	}
	for _, e := range ents {
		binary.Write(&idx, binary.BigEndian, e.offset)
	}
	idx.Write(sum[:])
	isum := sha1.Sum(idx.Bytes())
	idx.Write(isum[:])

	dir := filepath.Join(gitDir, "objects", "pack")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := "pack-" + hex.EncodeToString(sum[:])
	packPath = filepath.Join(dir, base+".pack")
	idxPath = filepath.Join(dir, base+".idx")
	if err := os.WriteFile(packPath, pack.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(idxPath, idx.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return packPath, idxPath
}
