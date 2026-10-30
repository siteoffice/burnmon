package doccheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The three exes and their Windows file properties carry one version
// (decided 2026-10-30 for v0.4.0-beta.1). A bump that misses one file fails here.
var versionConst = regexp.MustCompile(`(?m)^\s*(?:const\s+)?version\s*=\s*"([^"]+)"`)

func readRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func jsonString(t *testing.T, src, rel, key string) string {
	t.Helper()
	m := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*"([^"]+)"`).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("%s: no %q", rel, key)
	}
	return m[1]
}

func TestOneVersionClock(t *testing.T) {
	root, err := FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	consts := []string{"cmd/burnmon/app.go", "cmd/burnmon-cli/main.go", "cmd/burnmon-dev/app.go"}
	var want string
	for _, rel := range consts {
		m := versionConst.FindStringSubmatch(readRepoFile(t, root, rel))
		if m == nil {
			t.Fatalf("%s: no version constant", rel)
		}
		if want == "" {
			want = m[1]
		} else if m[1] != want {
			t.Errorf("%s: version %q, %s has %q", rel, m[1], consts[0], want)
		}
	}
	numeric := strings.SplitN(want, "-", 2)[0] + ".0"
	for _, rel := range []string{"winres/app.json", "winres/cli.json", "winres/burnmon-dev.json"} {
		src := readRepoFile(t, root, rel)
		if got := jsonString(t, src, rel, "ProductVersion"); got != want {
			t.Errorf("%s: ProductVersion %q, want %q", rel, got, want)
		}
		for _, key := range []string{"version", "file_version", "product_version", "FileVersion"} {
			if got := jsonString(t, src, rel, key); got != numeric {
				t.Errorf("%s: %s %q, want %q", rel, key, got, numeric)
			}
		}
	}
}
