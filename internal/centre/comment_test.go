package centre

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var commentNow = time.Date(2026, 11, 11, 12, 34, 56, 0, time.UTC)

func commentRnd() *bytes.Reader {
	return bytes.NewReader([]byte{0xde, 0xad, 0xbe, 0xef})
}

func sumHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func attach(name string, data []byte) AttachmentIn {
	return AttachmentIn{Name: name, Data: base64.StdEncoding.EncodeToString(data)}
}

func TestWriteCommentTwoAttachments(t *testing.T) {
	root := fixtureTree(t)
	one := []byte("hello kettle\n")
	two := []byte{1, 2, 3, 4, 5}
	req := CommentRequest{
		Target:      Target{Kind: "file", UnitID: "projects/kettle", Path: "projects/kettle/STATUS.md", Label: "STATUS.md"},
		Text:        "Is the gate review still needed?",
		Attachments: []AttachmentIn{attach("one.txt", one), attach("two.png", two)},
	}
	res, err := WriteComment(root, commentNow, commentRnd(), req)
	if err != nil {
		t.Fatal(err)
	}
	const id = "20261111T123456Z_deadbeef"
	if res.ID != id || res.Path != "_comments/"+id+".json" {
		t.Fatalf("result %+v", res)
	}
	raw, err := os.ReadFile(filepath.Join(root, "_comments", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec CommentRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"schemaVersion": 1`)) || rec.SchemaVersion != 1 {
		t.Errorf("schemaVersion missing in %s", raw)
	}
	status, err := os.ReadFile(filepath.Join(root, "projects", "kettle", "STATUS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != id || rec.Status != "open" || rec.CreatedAt != "2026-11-11T12:34:56Z" || rec.Text != req.Text {
		t.Errorf("record %+v", rec)
	}
	if rec.Target != req.Target {
		t.Errorf("target %+v, want %+v", rec.Target, req.Target)
	}
	if rec.TargetSHA256 != sumHex(status) {
		t.Errorf("target_sha256 %q, want the sha256 of STATUS.md", rec.TargetSHA256)
	}
	if len(rec.Attachments) != 2 {
		t.Fatalf("attachments %+v", rec.Attachments)
	}
	for i, want := range [][]byte{one, two} {
		a := rec.Attachments[i]
		name := []string{"one.txt", "two.png"}[i]
		if a.Name != name || a.Bytes != int64(len(want)) || a.SHA256 != sumHex(want) {
			t.Errorf("attachment %d = %+v", i, a)
		}
		if a.StoredPath != "_comments/files/"+id+"/"+name {
			t.Errorf("stored path %q", a.StoredPath)
		}
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(a.StoredPath)))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("stored file %s: %v, bytes %v", a.StoredPath, err, got)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "_comments"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left: %s", e.Name())
		}
	}
}

func TestWriteCommentNoTargetHash(t *testing.T) {
	root := fixtureTree(t)
	req := CommentRequest{Target: Target{Kind: "unit", UnitID: "projects/kettle", Path: "projects/kettle"}, Text: "note"}
	res, err := WriteComment(root, commentNow, commentRnd(), req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(res.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("target_sha256")) {
		t.Errorf("unit target got a target_sha256: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"attachments": []`)) {
		t.Errorf("attachments not an empty list: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(root, "_comments", "files")); err == nil {
		t.Error("files folder made for a comment without attachments")
	}
}

func TestWriteCommentLimits(t *testing.T) {
	file := Target{Kind: "file", Path: "projects/kettle/STATUS.md"}
	six := make([]AttachmentIn, 6)
	for i := range six {
		six[i] = attach("f"+twoDigits(i)+".txt", []byte("x"))
	}
	cases := map[string]CommentRequest{
		"6 attachments":    {Target: file, Text: "t", Attachments: six},
		"10 MB + 1 byte":   {Target: file, Text: "t", Attachments: []AttachmentIn{attach("big.bin", make([]byte, MaxAttachmentBytes+1))}},
		"empty text":       {Target: file, Text: "  \n"},
		"unknown kind":     {Target: Target{Kind: "nope"}, Text: "t"},
		"path escapes":     {Target: Target{Kind: "file", Path: "../x"}, Text: "t"},
		"bad base64":       {Target: file, Text: "t", Attachments: []AttachmentIn{{Name: "a.txt", Data: "%%%"}}},
		"file without one": {Target: Target{Kind: "block"}, Text: "t"},
	}
	for name, req := range cases {
		root := fixtureTree(t)
		if _, err := WriteComment(root, commentNow, commentRnd(), req); err == nil {
			t.Errorf("%s: written, want a refusal", name)
		}
		if _, err := os.Stat(filepath.Join(root, "_comments")); err == nil {
			t.Errorf("%s: _comments made for a refusal", name)
		}
	}
	root := fixtureTree(t)
	five := six[:5]
	if _, err := WriteComment(root, commentNow, commentRnd(), CommentRequest{Target: file, Text: "t", Attachments: five}); err != nil {
		t.Errorf("5 attachments refused: %v", err)
	}
}

func TestWriteCommentSanitizesNames(t *testing.T) {
	root := fixtureTree(t)
	req := CommentRequest{
		Target: Target{Kind: "plan", Label: "g1"},
		Text:   "t",
		Attachments: []AttachmentIn{
			attach(`..\evil name?.png`, []byte("e")),
			attach("a.png", []byte("1")),
			attach("a.png", []byte("2")),
		},
	}
	if _, err := WriteComment(root, commentNow, commentRnd(), req); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "_comments", "20261111T123456Z_deadbeef.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec CommentRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	want := []string{"evil_name_.png", "a.png", "2_a.png"}
	for i, a := range rec.Attachments {
		if a.Name != want[i] {
			t.Errorf("attachment %d named %q, want %q", i, a.Name, want[i])
		}
		if _, err := os.Stat(filepath.Join(root, "_comments", "files", "20261111T123456Z_deadbeef", want[i])); err != nil {
			t.Errorf("stored %s: %v", want[i], err)
		}
	}
}
