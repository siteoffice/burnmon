//go:build windows || darwin

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"burnmon/internal/centre"
)

// writeBoard writes a board page of n bytes under root at rel.
func writeBoard(t *testing.T, root, rel string, n int) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Repeat("x", n)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCcBoardRefusesOutsideRoot(t *testing.T) {
	base := t.TempDir()
	r := filepath.Join(base, "root")
	writeBoard(t, r, "_board/flows/p/s/index.html", 10)
	writeBoard(t, base, "x/_board/index.html", 10)
	e := bindEnv{root: r}
	if _, err := boardPage(e, "../x/_board/index.html"); err == nil {
		t.Error("a relative path outside root was not refused")
	}
	if _, err := boardPage(e, filepath.Join(base, "x", "_board", "index.html")); err == nil {
		t.Error("an absolute path outside root was not refused")
	}
}

func TestCcBoardRefusesOver2MB(t *testing.T) {
	r := t.TempDir()
	writeBoard(t, r, "_board/flows/p/big/index.html", centre.MaxBoardBytes+1)
	writeBoard(t, r, "_board/flows/p/s/index.html", 10)
	e := bindEnv{root: r}
	if centre.MaxBoardBytes != 2<<20 {
		t.Fatalf("MaxBoardBytes is %d, want 2 MB", centre.MaxBoardBytes)
	}
	if _, err := boardPage(e, "_board/flows/p/big/index.html"); err == nil {
		t.Error("a board page of 2 MB + 1 byte was not refused")
	}
	got, err := boardPage(e, "_board/flows/p/s/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if got.HTML != strings.Repeat("x", 10) {
		t.Errorf("board page HTML = %q, want the 10 bytes", got.HTML)
	}
}

// TestPageBoardSandbox: the board page runs in a sandboxed iframe through
// srcdoc, scripts allowed, never the page's own origin.
func TestPageBoardSandbox(t *testing.T) {
	mustContain(t, "pageBoard", "api.board(", "h('iframe', 'boardframe')", "setAttribute('sandbox', 'allow-scripts')", ".srcdoc = ", ".html", "drawSeq")
	body := pageFunc(t, "pageBoard")
	if strings.Index(body, "setAttribute('sandbox'") > strings.Index(body, ".srcdoc = ") {
		t.Error("pageBoard sets srcdoc before the sandbox attribute")
	}
	if strings.Index(body, ".srcdoc = ") > strings.Index(body, "pane.appendChild(f)") {
		t.Error("pageBoard adds the iframe before it has its sandbox and srcdoc")
	}
	if strings.Contains(pageHTML, "allow-same-origin") {
		t.Error("page.html mentions allow-same-origin")
	}
	if regexp.MustCompile(`\b(innerHTML|outerHTML|insertAdjacentHTML|document\.write)\b`).MatchString(pageHTML) {
		t.Error("page.html writes HTML into the outer page")
	}
	if n := strings.Count(pageHTML, "'iframe'"); n != 1 {
		t.Errorf("page.html makes %d iframes, want exactly the board iframe", n)
	}
	if strings.Contains(strings.ToLower(pageHTML), "<iframe") {
		t.Error("page.html has an iframe in its markup; the board iframe is made in pageBoard")
	}
}

// TestRowWithoutBoard: Open board only when the row has a board page, else the
// row says so and offers nothing.
func TestRowWithoutBoard(t *testing.T) {
	mustContain(t, "rowDetail", "r.board", "'Open board \\u203A'", "go('board'", "'This row has no board page.'")
}

// TestCrumbsBoard: the breadcrumb runs root, unit, row slug; the unit part
// leads back to the unit view on the row's own tab.
func TestCrumbsBoard(t *testing.T) {
	mustContain(t, "crumbs", "view.page === 'board'", "a.row.slug", "go('unit', a.row.unit, a.fl ? 'Flows' : 'Sprints')")
	mustContain(t, "go", "sub = {tab: tab || 'Structure'")
}
