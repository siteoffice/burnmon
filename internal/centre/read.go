package centre

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// MaxFileBytes is the largest unit file ReadUnitFile returns.
const MaxFileBytes = 512 << 10

// MaxBoardBytes is the largest board page ReadBoard returns.
const MaxBoardBytes = 2 << 20

// Block is one paragraph, heading or fenced code block of a unit file.
type Block struct {
	Text    string `json:"text"`
	Heading bool   `json:"heading"`
}

// FileText is one unit file read as text, split in blocks.
type FileText struct {
	Path     string  `json:"path"`
	Size     int64   `json:"size"`
	Modified string  `json:"modified"`
	Role     string  `json:"role"`
	Blocks   []Block `json:"blocks"`
}

// BoardPage is one board page read as HTML.
type BoardPage struct {
	Path string `json:"path"`
	HTML string `json:"html"`
}

func refuse(rel, reason string) error {
	return fmt.Errorf("refused %q: %s", rel, reason)
}

// segments splits a screen path on both slash kinds.
func segments(rel string) []string {
	return strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' })
}

// Inside resolves rel (root-relative, forward slashes) under root and
// returns the absolute path with links resolved. It refuses an empty or
// absolute path, any ".." segment, a path that does not exist, and a path
// whose link-resolved form lies outside the link-resolved root.
func Inside(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", refuse(rel, "empty path")
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", refuse(rel, "absolute path")
	}
	for _, s := range segments(rel) {
		if s == ".." {
			return "", refuse(rel, "path has a .. segment")
		}
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", refuse(rel, "root does not resolve")
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", refuse(rel, "root does not resolve")
	}
	real, err := filepath.EvalSymlinks(filepath.Join(absRoot, filepath.FromSlash(rel)))
	if err != nil {
		return "", refuse(rel, "path does not exist")
	}
	back, err := filepath.Rel(realRoot, real)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) || filepath.IsAbs(back) {
		return "", refuse(rel, "path lies outside root")
	}
	return real, nil
}

// resolvedRel returns the link-resolved root and real (a path Inside
// returned) relative to it with forward slashes, in the on-disk spelling.
// Checks on folder names use this, not the screen string: Windows finds
// "_ARCHIVE" or "_archive." as the folder "_archive".
func resolvedRel(root, real string) (string, string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("root does not resolve")
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", "", fmt.Errorf("root does not resolve")
	}
	back, err := filepath.Rel(realRoot, real)
	if err != nil {
		return "", "", fmt.Errorf("path lies outside root")
	}
	return realRoot, filepath.ToSlash(back), nil
}

// cleanRel is rel cleaned, with forward slashes.
func cleanRel(rel string) string {
	return path.Clean(strings.ReplaceAll(rel, `\`, "/"))
}

// Blocks splits text on blank lines outside fenced code. A block is a
// heading when its first line starts with "#".
func Blocks(text string) []Block {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	blocks := []Block{}
	var cur []string
	fenced := false
	flush := func() {
		if len(cur) > 0 {
			blocks = append(blocks, Block{Text: strings.Join(cur, "\n"), Heading: strings.HasPrefix(cur[0], "#")})
			cur = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
		}
		if trimmed == "" && !fenced {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return blocks
}

// ReadUnitFile reads one unit file: a file inside root whose folder is a
// unit at most MaxDepth deep, with no skipped segment, that is among the
// folder's unit files and at most MaxFileBytes. Any other path is refused
// with the reason.
func ReadUnitFile(root, rel string) (FileText, error) {
	real, err := Inside(root, rel)
	if err != nil {
		return FileText{}, err
	}
	realRoot, clean, err := resolvedRel(root, real)
	if err != nil {
		return FileText{}, refuse(rel, err.Error())
	}
	folder, base := path.Split(clean)
	folder = strings.TrimSuffix(folder, "/")
	depth := 0
	if folder != "" {
		for _, s := range strings.Split(folder, "/") {
			if Skip(s) {
				return FileText{}, refuse(rel, "path has a skipped folder "+s)
			}
			depth++
		}
	}
	if depth > MaxDepth {
		return FileText{}, refuse(rel, fmt.Sprintf("folder is deeper than %d levels", MaxDepth))
	}
	dir := filepath.Join(realRoot, filepath.FromSlash(folder))
	if info, err := os.Stat(filepath.Join(dir, unitFileName)); err != nil || !info.Mode().IsRegular() {
		return FileText{}, refuse(rel, "folder is not a unit")
	}
	listed := false
	files, _ := ListFiles(dir, time.Time{})
	for _, f := range files {
		if f.Name == base {
			listed = true
			break
		}
	}
	if !listed {
		return FileText{}, refuse(rel, fmt.Sprintf("not in the unit files list (first %d .md and .json files)", MaxFiles))
	}
	data, info, err := readCapped(real, MaxFileBytes)
	if err != nil {
		return FileText{}, refuse(rel, err.Error())
	}
	if data == nil {
		return FileText{}, refuse(rel, "file is larger than 512 KB")
	}
	return FileText{
		Path:     clean,
		Size:     info.Size(),
		Modified: info.ModTime().UTC().Format(time.RFC3339),
		Role:     Role(base),
		Blocks:   Blocks(string(data)),
	}, nil
}

// ReadBoard reads one board page: an index.html inside root with a _board
// segment, at most MaxBoardBytes.
func ReadBoard(root, rel string) (BoardPage, error) {
	real, err := Inside(root, rel)
	if err != nil {
		return BoardPage{}, err
	}
	_, clean, err := resolvedRel(root, real)
	if err != nil {
		return BoardPage{}, refuse(rel, err.Error())
	}
	if path.Base(clean) != "index.html" {
		return BoardPage{}, refuse(rel, "not an index.html")
	}
	board := false
	for _, s := range strings.Split(clean, "/") {
		if s == "_board" {
			board = true
		}
	}
	if !board {
		return BoardPage{}, refuse(rel, "no _board segment in the path")
	}
	data, _, err := readCapped(real, MaxBoardBytes)
	if err != nil {
		return BoardPage{}, refuse(rel, err.Error())
	}
	if data == nil {
		return BoardPage{}, refuse(rel, "file is larger than 2 MB")
	}
	return BoardPage{Path: clean, HTML: string(data)}, nil
}

// readCapped reads a regular file of at most max bytes. A larger file gives
// nil data and no error.
func readCapped(p string, max int64) ([]byte, os.FileInfo, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, nil, fmt.Errorf("file does not open")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("not a regular file")
	}
	if info.Size() > max {
		return nil, info, nil
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, nil, fmt.Errorf("file does not read")
	}
	if int64(len(data)) > max {
		return nil, info, nil
	}
	return data, info, nil
}
