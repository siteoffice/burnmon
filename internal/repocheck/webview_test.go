package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebviewGoOnlyInDarwinCgoFiles(t *testing.T) {
	found := 0
	for _, root := range []string{"../../cmd", "../../internal"} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
				return err
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			text := string(b)
			if !strings.Contains(text, `"github.com/webview/webview_go"`) {
				return nil
			}
			if strings.HasSuffix(p, "webview_test.go") {
				return nil
			}
			found++
			first := strings.TrimRight(strings.SplitN(text, "\n", 2)[0], "\r")
			if first != "//go:build darwin && cgo" {
				t.Errorf("%s imports webview_go but first line is %q", p, first)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if found == 0 {
		t.Error("no file imports webview_go")
	}
}
