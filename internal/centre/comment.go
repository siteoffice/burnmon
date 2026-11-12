package centre

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MaxAttachments is the most attachments one comment carries.
const MaxAttachments = 5

// MaxAttachmentBytes is the largest attachment, decoded.
const MaxAttachmentBytes = 10 << 20

// commentsDir is the folder under root that holds comment records.
const commentsDir = "_comments"

var commentKinds = map[string]bool{
	"unit": true, "part": true, "file": true, "block": true,
	"flow": true, "sprint": true, "date": true, "plan": true,
}

// Target is what a comment is about. Path is root-relative with forward
// slashes; Block is a 1-based block number for kind "block".
type Target struct {
	Kind   string `json:"kind"`
	UnitID string `json:"unit_id"`
	Path   string `json:"path"`
	Block  int    `json:"block"`
	Label  string `json:"label"`
}

// AttachmentIn is one attachment as the page sends it, Data in base64.
type AttachmentIn struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

type CommentRequest struct {
	Target      Target         `json:"target"`
	Text        string         `json:"text"`
	Attachments []AttachmentIn `json:"attachments"`
}

// Attachment is one stored attachment; StoredPath is root-relative.
type Attachment struct {
	Name       string `json:"name"`
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256"`
	StoredPath string `json:"stored_path"`
}

// CommentRecord is the JSON file <root>/_comments/<id>.json.
type CommentRecord struct {
	SchemaVersion int          `json:"schemaVersion"`
	ID            string       `json:"id"`
	CreatedAt     string       `json:"created_at"`
	Target        Target       `json:"target"`
	TargetSHA256  string       `json:"target_sha256,omitempty"`
	Text          string       `json:"text"`
	Attachments   []Attachment `json:"attachments"`
	Status        string       `json:"status"`
}

// CommentResult names the record written; Path is root-relative.
type CommentResult struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// WriteComment checks req, then writes its attachments to
// <root>/_comments/files/<id>/ and the record to <root>/_comments/<id>.json
// through a temp file and a rename. A refused request writes nothing; a
// failed write removes the attachment folder again.
func WriteComment(root string, now time.Time, rnd io.Reader, req CommentRequest) (CommentResult, error) {
	t := req.Target
	if !commentKinds[t.Kind] {
		return CommentResult{}, fmt.Errorf("refused comment: unknown target kind %q", t.Kind)
	}
	if strings.TrimSpace(req.Text) == "" {
		return CommentResult{}, errors.New("refused comment: the text is empty")
	}
	if len(req.Attachments) > MaxAttachments {
		return CommentResult{}, fmt.Errorf("refused comment: %d attachments, at most %d", len(req.Attachments), MaxAttachments)
	}
	targetAbs := ""
	if t.Path != "" {
		abs, err := Inside(root, t.Path)
		if err != nil {
			return CommentResult{}, err
		}
		targetAbs = abs
		t.Path = cleanRel(t.Path)
	}
	sum := ""
	if t.Kind == "file" || t.Kind == "block" {
		if targetAbs == "" {
			return CommentResult{}, fmt.Errorf("refused comment: a %s target needs a path", t.Kind)
		}
		s, err := fileSHA256(targetAbs)
		if err != nil {
			return CommentResult{}, fmt.Errorf("refused comment: target %q: %v", t.Path, err)
		}
		sum = s
	}
	decoded := make([][]byte, len(req.Attachments))
	names := make([]string, len(req.Attachments))
	seen := map[string]bool{}
	for i, a := range req.Attachments {
		if base64.StdEncoding.DecodedLen(len(a.Data)) > MaxAttachmentBytes+3 {
			return CommentResult{}, fmt.Errorf("refused comment: attachment %q is larger than 10 MB", a.Name)
		}
		data, err := base64.StdEncoding.DecodeString(a.Data)
		if err != nil {
			return CommentResult{}, fmt.Errorf("refused comment: attachment %q is not base64", a.Name)
		}
		if len(data) > MaxAttachmentBytes {
			return CommentResult{}, fmt.Errorf("refused comment: attachment %q is larger than 10 MB", a.Name)
		}
		decoded[i] = data
		names[i] = uniqueName(sanitizeName(a.Name), seen)
	}
	var raw [4]byte
	if _, err := io.ReadFull(rnd, raw[:]); err != nil {
		return CommentResult{}, fmt.Errorf("comment id: %v", err)
	}
	id := now.UTC().Format("20060102T150405Z") + "_" + hex.EncodeToString(raw[:])

	dir := filepath.Join(root, commentsDir)
	recPath := filepath.Join(dir, id+".json")
	if _, err := os.Lstat(recPath); err == nil {
		return CommentResult{}, fmt.Errorf("refused comment: id %s is already used", id)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return CommentResult{}, err
	}
	if _, err := Inside(root, commentsDir); err != nil {
		return CommentResult{}, err
	}
	filesDir := filepath.Join(dir, "files", id)
	rec := CommentRecord{
		SchemaVersion: 1,
		ID:            id,
		CreatedAt:     now.UTC().Format(time.RFC3339),
		Target:        t,
		TargetSHA256:  sum,
		Text:          req.Text,
		Attachments:   []Attachment{},
		Status:        "open",
	}
	fail := func(err error) (CommentResult, error) {
		_ = os.RemoveAll(filesDir)
		return CommentResult{}, err
	}
	if len(decoded) > 0 {
		if err := os.MkdirAll(filesDir, 0o755); err != nil {
			return fail(err)
		}
		if _, err := Inside(root, commentsDir+"/files/"+id); err != nil {
			return fail(err)
		}
	}
	for i, data := range decoded {
		if err := os.WriteFile(filepath.Join(filesDir, names[i]), data, 0o644); err != nil {
			return fail(err)
		}
		s := sha256.Sum256(data)
		rec.Attachments = append(rec.Attachments, Attachment{
			Name:       names[i],
			Bytes:      int64(len(data)),
			SHA256:     hex.EncodeToString(s[:]),
			StoredPath: commentsDir + "/files/" + id + "/" + names[i],
		})
	}
	body, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fail(err)
	}
	tmp := recPath + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		_ = os.Remove(tmp)
		return fail(err)
	}
	if err := os.Rename(tmp, recPath); err != nil {
		_ = os.Remove(tmp)
		return fail(err)
	}
	return CommentResult{ID: id, Path: commentsDir + "/" + id + ".json"}, nil
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", errors.New("does not open")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errors.New("does not read")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sanitizeName keeps the last path element of name and replaces every
// character outside letters, digits, dot, dash and underscore with "_".
// Leading dots go, so no name is hidden or a folder step.
func sanitizeName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.TrimLeft(b.String(), ".")
	if out == "" {
		out = "file"
	}
	return out
}

// uniqueName returns name, or "<n>_name" with the smallest n from 2 that is
// not taken yet, comparing without case (Windows folders ignore it).
func uniqueName(name string, seen map[string]bool) string {
	out := name
	for n := 2; seen[strings.ToLower(out)]; n++ {
		out = strconv.Itoa(n) + "_" + name
	}
	seen[strings.ToLower(out)] = true
	return out
}
