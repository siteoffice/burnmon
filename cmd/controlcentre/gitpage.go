//go:build windows || darwin

package main

import (
	"errors"
	"os"
	"strings"
	"sync"

	"burnmon/internal/centre"
	"burnmon/internal/snapshot"
)

// rootBox holds the root the pages read; Save on the Settings page changes
// it without a restart. byFlag: --root set it, and Save is refused.
type rootBox struct {
	mu     sync.Mutex
	root   string
	byFlag bool
}

func newRootBox(root string, byFlag bool) *rootBox { return &rootBox{root: root, byFlag: byFlag} }

func (b *rootBox) get() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.root, b.byFlag
}

func (b *rootBox) set(root string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.root = root
}

// rootNow is the root every binding reads: the box when main made one,
// else the fixed root tests pass.
func (e bindEnv) rootNow() string {
	if e.roots != nil {
		r, _ := e.roots.get()
		return r
	}
	return e.root
}

// RootView is what the Settings page shows in the Root row.
type RootView struct {
	Root   string           `json:"root"`    // the root the pages read now
	ByFlag bool             `json:"by_flag"` // set by --root: read-only here
	Check  centre.RootCheck `json:"check"`
}

func rootView(e bindEnv) RootView {
	root, byFlag := e.rootNow(), false
	if e.roots != nil {
		_, byFlag = e.roots.get()
	}
	return RootView{Root: root, ByFlag: byFlag, Check: centre.CheckRoot(root, "")}
}

// ErrRootByFlag is the refusal of a Save while --root is set.
var ErrRootByFlag = errors.New("the root is set by --root; start the Control Centre without --root to change it here")

// rootCheck is the ccRootCheck handler: the check before Save.
func rootCheck(e bindEnv, p string) centre.RootCheck {
	cwd, _ := os.Getwd()
	return centre.CheckRoot(p, cwd)
}

// rootSet is the ccRootSet handler: it checks p, writes it to
// controlcentre.json and points every page at it. The page then reads the
// state again; nothing restarts.
func rootSet(e bindEnv, p string) (RootView, error) {
	if e.roots != nil {
		if _, byFlag := e.roots.get(); byFlag {
			return rootView(e), ErrRootByFlag
		}
	}
	c := rootCheck(e, p)
	if !c.OK {
		return rootView(e), errors.New(c.Line)
	}
	s := e.settings.get()
	s.Root = c.Path
	if err := centre.SaveSettings(e.settingsPath, s); err != nil {
		return rootView(e), err
	}
	e.settings.set(s.Normalized())
	if e.roots != nil {
		e.roots.set(c.Path)
	}
	return rootView(e), nil
}

// BrowseResult is what ccRootBrowse returns: the picked folder, or nothing
// when the dialog was cancelled.
type BrowseResult struct {
	Path   string `json:"path"`
	Picked bool   `json:"picked"`
	Line   string `json:"line"` // why there is no dialog, else ""
}

// rootBrowse is the ccRootBrowse handler: the native folder picker, opened
// in start (the typed folder, else the current root).
func rootBrowse(e bindEnv, start string) BrowseResult {
	if e.pickFolder == nil {
		return BrowseResult{Line: "There is no folder picker here. Type the folder instead."}
	}
	if strings.TrimSpace(start) == "" {
		start = e.rootNow()
	}
	p, ok, err := e.pickFolder(start)
	if err != nil {
		return BrowseResult{Line: "The folder picker did not open: " + err.Error()}
	}
	return BrowseResult{Path: p, Picked: ok}
}

// GitRequest is what the page sends with ccGit.
type GitRequest struct {
	Rows    int    `json:"rows"`
	Refresh bool   `json:"refresh"`
	Stage   string `json:"stage"` // show only the repos that stop at this lane stage, "" for all
}

// gitView is the ccGit handler. The reading runs in the background; the
// page asks again while Loading is true.
func gitView(e bindEnv, req GitRequest) centre.GitView {
	if e.git == nil {
		return centre.GitView{Root: e.rootNow(), Line: "The Git page is not available in this build.", Repos: []centre.GitRepo{}, Pages: []centre.Span{}}
	}
	return e.git.ViewStage(e.rootNow(), e.settings.get().GitPRs, req.Refresh, req.Rows, req.Stage)
}

// gitRepo is the ccGitRepo handler.
func gitRepo(e bindEnv, id string) centre.GitDetailView {
	if e.git == nil {
		return centre.GitDetailView{}
	}
	return e.git.Detail(e.rootNow(), id)
}

// sessionsByFolder maps the last folder name of each session's cwd, the
// "project" field of BurnMon Dev's snapshot, to session labels. The
// snapshot never holds the full cwd, so the Git page matches a worktree to a
// session by folder name only (Wilco, 2026-11-15).
func sessionsByFolder(rd snapshot.Reading) map[string][]string {
	out := map[string][]string{}
	if rd.Status != snapshot.StatusOK || rd.Snapshot == nil {
		return out
	}
	seen := map[string]bool{}
	add := func(project, label string) {
		if project == "" || label == "" || seen[project+"|"+label] {
			return
		}
		seen[project+"|"+label] = true
		out[project] = append(out[project], label)
	}
	for _, a := range rd.Snapshot.AgentSessions {
		id := a.ID
		if id == "" {
			id = shortID(a.SessionID)
		}
		add(a.Project, strings.TrimSpace(a.Kind+" agent "+id))
	}
	for _, s := range rd.Snapshot.Sessions {
		add(s.Project, strings.TrimSpace(s.Vendor+" "+shortID(s.ID)))
	}
	return out
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
