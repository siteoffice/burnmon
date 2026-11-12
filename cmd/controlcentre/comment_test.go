//go:build windows || darwin

package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burnmon/internal/centre"
)

func TestCcCommentWritesRecord(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "kettle"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "kettle", "README.md"), []byte("# Kettle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)
	e := bindEnv{root: root, now: func() time.Time { return now }}
	req := centre.CommentRequest{
		Target: centre.Target{Kind: "file", UnitID: "kettle", Path: "kettle/README.md", Label: "README.md"},
		Text:   "Is this still true?",
		Attachments: []centre.AttachmentIn{
			{Name: "note.txt", Data: base64.StdEncoding.EncodeToString([]byte("lamp"))},
			{Name: "note.txt", Data: base64.StdEncoding.EncodeToString([]byte("stove"))},
		},
	}
	res, err := commentWrite(e, req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Path, "_comments/") || !strings.HasSuffix(res.Path, ".json") {
		t.Fatalf("record path %q is not under _comments", res.Path)
	}
	recs, err := filepath.Glob(filepath.Join(root, "_comments", "*.json"))
	if err != nil || len(recs) != 1 {
		t.Fatalf("want one record under _comments, got %v (%v)", recs, err)
	}
	if filepath.Base(recs[0]) != filepath.Base(res.Path) {
		t.Errorf("returned path %q, written %q", res.Path, recs[0])
	}
	body, err := os.ReadFile(recs[0])
	if err != nil {
		t.Fatal(err)
	}
	var rec centre.CommentRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Target.Path != "kettle/README.md" || rec.TargetSHA256 == "" || len(rec.Attachments) != 2 {
		t.Errorf("record = %+v", rec)
	}
	if rec.Attachments[0].Name == rec.Attachments[1].Name {
		t.Error("attachment names are not de-duplicated")
	}
}
