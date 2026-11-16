package centre

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func readStatus(t *testing.T, name string) StatusInfo {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "gitstatus", name))
	if err != nil {
		t.Fatal(err)
	}
	return ParseStatus(string(data))
}

// TestParseStatusSamples: dirty counts changed, renamed, unmerged and
// untracked entries, never ignored ones; ahead/behind and the upstream come
// from the branch headers; detached and initial are recognised.
func TestParseStatusSamples(t *testing.T) {
	d := readStatus(t, "dirty.txt")
	if d.Dirty != 3 || d.Conflicts != 0 || d.Head != "main" || d.Upstream != "origin/main" || !d.HasAB || d.Ahead != 0 || d.Behind != 0 || d.Unparseable {
		t.Errorf("dirty.txt = %+v", d)
	}
	c := readStatus(t, "conflict_ahead_behind.txt")
	if c.Dirty != 1 || c.Conflicts != 1 || c.Ahead != 3 || c.Behind != 2 || c.Head != "sprint/x" {
		t.Errorf("conflict_ahead_behind.txt = %+v", c)
	}
	x := readStatus(t, "detached.txt")
	if !x.Detached || x.Upstream != "" || x.HasAB || x.Dirty != 0 {
		t.Errorf("detached.txt = %+v", x)
	}
	i := readStatus(t, "initial.txt")
	if !i.Initial || i.Dirty != 1 {
		t.Errorf("initial.txt = %+v", i)
	}
	if s := ParseStatus("# branch.head main\r\n? a\r\n"); s.Dirty != 1 || s.Head != "main" {
		t.Errorf("CRLF output = %+v", s)
	}
	if s := ParseStatus("garbage line\n"); !s.Unparseable {
		t.Error("an unknown line must mark the output unparseable")
	}
}

func TestGitStatusIsTheOneCommand(t *testing.T) {
	want := []string{"--no-optional-locks", "status", "--porcelain=v2", "--branch"}
	if !reflect.DeepEqual(GitStatusArgs, want) {
		t.Errorf("GitStatusArgs = %v, want %v", GitStatusArgs, want)
	}
	if GitStatusTimeout != 5*time.Second {
		t.Errorf("GitStatusTimeout = %v, want 5s", GitStatusTimeout)
	}
	if PRCacheTTL != 5*time.Minute {
		t.Errorf("PRCacheTTL = %v, want 5m", PRCacheTTL)
	}
	wantGh := "pr list --state all --limit 100 --json number,title,headRefName,state,isDraft,reviewDecision,statusCheckRollup,updatedAt"
	if got := strings.Join(GhArgs, " "); got != wantGh {
		t.Errorf("GhArgs = %q", got)
	}
}

func TestAttentionOf(t *testing.T) {
	r := GitRepo{Conflicts: 1, Dirty: 2, Behind: 1, Detached: true}
	if got := AttentionOf(r); !reflect.DeepEqual(got, []string{"conflicts", "dirty", "behind", "detached"}) {
		t.Errorf("AttentionOf = %v", got)
	}
	if got := AttentionOf(GitRepo{Ahead: 4}); len(got) != 0 {
		t.Errorf("ahead only needs no attention, got %v", got)
	}
}

// TestSortReposAttentionThenDateGroupedByUnit: attention first, then the
// newest commit; units grouped in the order of their first repo.
func TestSortReposAttentionThenDateGroupedByUnit(t *testing.T) {
	repos := []GitRepo{
		{ID: "a/old", Unit: "a", LastAt: "2026-11-01T00:00:00Z"},
		{ID: "b/new", Unit: "b", LastAt: "2026-11-14T00:00:00Z"},
		{ID: "a/dirty", Unit: "a", LastAt: "2026-10-01T00:00:00Z", Attention: []string{"dirty"}},
		{ID: "c/none", Unit: "c"},
		{ID: "b/mid", Unit: "b", LastAt: "2026-11-10T00:00:00Z"},
	}
	SortRepos(repos)
	var got []string
	for _, r := range repos {
		got = append(got, r.ID)
	}
	want := []string{"a/dirty", "a/old", "b/new", "b/mid", "c/none"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// TestGitPagesCountUnitHeaders: a unit header line comes before the first
// repo of a unit on every page, and no page passes rows lines.
func TestGitPagesCountUnitHeaders(t *testing.T) {
	repos := []GitRepo{{Unit: "a"}, {Unit: "a"}, {Unit: "a"}, {Unit: "b"}, {Unit: "b"}}
	got := GitPages(repos, 4)
	// page 1: header a + 3 repos; page 2: header b + 2 repos.
	want := []Span{{0, 3}, {3, 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GitPages(4) = %v, want %v", got, want)
	}
	got = GitPages(repos, 3)
	want = []Span{{0, 2}, {2, 3}, {3, 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GitPages(3) = %v, want %v", got, want)
	}
	if got := GitPages(nil, 10); len(got) != 0 {
		t.Errorf("no repos: %v", got)
	}
}

func TestOwningUnit(t *testing.T) {
	units := []Unit{{ID: "."}, {ID: "projects/kettle"}}
	for id, want := range map[string]string{"projects/kettle/app": "projects/kettle", "projects/kettle": "projects/kettle", "other": ".", ".": "."} {
		if got := OwningUnit(id, units); got != want {
			t.Errorf("OwningUnit(%q) = %q, want %q", id, got, want)
		}
	}
	if got := OwningUnit("x/y", []Unit{{ID: "z"}}); got != "" {
		t.Errorf("no unit above: %q", got)
	}
}

func TestGitHubSlug(t *testing.T) {
	cases := map[string]string{
		"https://github.com/siteoffice/burnmon.git": "siteoffice/burnmon",
		"https://github.com/siteoffice/burnmon":     "siteoffice/burnmon",
		"git@github.com:owner/repo.git":             "owner/repo",
		"ssh://git@github.com/owner/repo":           "owner/repo",
		"https://gitlab.com/owner/repo.git":         "",
		"C:/repos/local":                            "",
	}
	for url, want := range cases {
		if got := GitHubSlug([]GitRemote{{Name: "origin", URL: url}}); got != want {
			t.Errorf("GitHubSlug(%q) = %q, want %q", url, got, want)
		}
	}
	two := []GitRemote{{Name: "mirror", URL: "https://github.com/m/x.git"}, {Name: "origin", URL: "https://github.com/o/x.git"}}
	if got := GitHubSlug(two); got != "o/x" {
		t.Errorf("origin must win: %q", got)
	}
}

func TestSortTagsNewestVersionFirst(t *testing.T) {
	tags := []string{"v0.4.0-beta.3", "v0.3.2", "v0.4.0-beta.10", "v0.4.0", "v0.4.0-alpha.10"}
	SortTags(tags)
	want := []string{"v0.4.0", "v0.4.0-beta.10", "v0.4.0-beta.3", "v0.4.0-alpha.10", "v0.3.2"}
	if !reflect.DeepEqual(tags, want) {
		t.Errorf("SortTags = %v, want %v", tags, want)
	}
}

func TestAge(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for d, want := range map[time.Duration]string{12 * time.Minute: "12 min", 5 * time.Hour: "5 h", 72 * time.Hour: "3 d"} {
		if got := Age(now.Add(-d), now); got != want {
			t.Errorf("Age(-%v) = %q, want %q", d, got, want)
		}
	}
	if Age(time.Time{}, now) != "" {
		t.Error("zero time must give no age")
	}
}

func TestParseGhPRsAndChecks(t *testing.T) {
	data := []byte(`[{"number":7,"title":"Git page","headRefName":"work/a","state":"OPEN","isDraft":true,
	"reviewDecision":"APPROVED","statusCheckRollup":[{"status":"COMPLETED","conclusion":"SUCCESS"},{"status":"IN_PROGRESS"}],
	"updatedAt":"2026-11-15T10:00:00Z"}]`)
	prs, err := ParseGhPRs(data)
	if err != nil {
		t.Fatal(err)
	}
	want := GitPR{Number: 7, Title: "Git page", Branch: "work/a", State: "OPEN", Draft: true, Review: "APPROVED", Checks: "pending", UpdatedAt: "2026-11-15T10:00:00Z"}
	if len(prs) != 1 || prs[0] != want {
		t.Errorf("ParseGhPRs = %+v", prs)
	}
	for want, cs := range map[string][]ghCheck{
		"":     nil,
		"pass": {{Status: "COMPLETED", Conclusion: "SUCCESS"}, {State: "SUCCESS"}},
		"fail": {{Status: "IN_PROGRESS"}, {Conclusion: "FAILURE"}},
	} {
		if got := checksOf(cs); got != want {
			t.Errorf("checksOf(%v) = %q, want %q", cs, got, want)
		}
	}
	if _, err := ParseGhPRs([]byte("not json")); err == nil {
		t.Error("bad output must be an error")
	}
}

func TestGhErrorSignedOut(t *testing.T) {
	err := ghError("To get started with GitHub CLI, please run:  gh auth login\n", os.ErrInvalid)
	if err != ErrGhSignedOut {
		t.Errorf("ghError = %v, want ErrGhSignedOut", err)
	}
	if err := ghError("HTTP 404: Not Found\nmore", os.ErrInvalid); err.Error() != "gh: HTTP 404: Not Found" {
		t.Errorf("ghError = %v", err)
	}
}

func TestGitMissingLine(t *testing.T) {
	run, why := GitStatusRunner(func(string) (string, error) { return "", os.ErrNotExist })
	if run != nil || why != GitMissingLine {
		t.Errorf("GitStatusRunner without git = %v, %q", run != nil, why)
	}
}
