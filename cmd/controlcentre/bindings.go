//go:build windows || darwin

package main

import (
	"crypto/rand"
	"log"
	"sync"
	"time"

	"burnmon/internal/centre"
	"burnmon/internal/controls"
	"burnmon/internal/winbind"
)

// settingsBox holds the current settings under a mutex: the bindings run on
// the window thread and may run while another call is still saving.
type settingsBox struct {
	mu sync.Mutex
	s  centre.Settings
}

func newSettingsBox(s centre.Settings) *settingsBox { return &settingsBox{s: s} }

func (b *settingsBox) get() centre.Settings {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.s
}

func (b *settingsBox) set(s centre.Settings) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.s = s
}

// bindEnv is what the bindings read from main(): the window, the root (roots
// when main made a box; root is the fixed one tests pass), the folders and
// files, the clock, the controls service (nil when main could not build it,
// svcWhy says why), the settings, the Git page scanner and the native folder
// picker (nil without one).
type bindEnv struct {
	w                                                winbind.Window
	root, dataDir, settingsPath, snapPath, auditPath string
	roots                                            *rootBox
	now                                              func() time.Time
	svc                                              *controls.Service
	svcWhy                                           string
	settings                                         *settingsBox
	git                                              *centre.GitScanner
	pickFolder                                       func(start string) (string, bool, error)
}

func (e bindEnv) clock() time.Time {
	if e.now == nil {
		return time.Now()
	}
	return e.now()
}

// StateRequest is what the page sends with ccState: rows per page.
type StateRequest struct {
	Rows int `json:"rows"`
}

// bindAll registers every cc binding on the window, in the order of
// ccBindingNames (binding_names.go), and nothing else.
func bindAll(e bindEnv) {
	bind := func(name string, f interface{}) {
		if err := e.w.Bind(name, f); err != nil {
			log.Println("could not bind", name+":", err)
		}
	}
	bind("ccState", func(req StateRequest) centre.State {
		return centre.Build(e.rootNow(), centre.Options{Now: e.clock(), Weeks: e.settings.get().Weeks, Rows: req.Rows})
	})
	bind("ccFile", func(path string) (centre.FileText, error) {
		return centre.ReadUnitFile(e.rootNow(), path)
	})
	bind("ccBoard", func(path string) (centre.BoardPage, error) {
		return boardPage(e, path)
	})
	bind("ccSettingsGet", func() centre.Settings {
		return e.settings.get()
	})
	bind("ccSettingsSet", func(s centre.Settings) (centre.Settings, error) {
		return settingsSet(e, s)
	})
	bind("ccComment", func(req centre.CommentRequest) (centre.CommentResult, error) {
		return commentWrite(e, req)
	})
	bind("ccPreview", func(req DecisionRequest) (controls.PreviewResult, error) {
		return previewDecision(e, req)
	})
	bind("ccRun", func(token string) (RunView, error) {
		return runBinding(e, token)
	})
	bind("ccAgents", func(rows int) AgentsView {
		return agentsView(e, rows)
	})
	bind("ccRoot", func() RootView {
		return rootView(e)
	})
	bind("ccRootCheck", func(p string) centre.RootCheck {
		return rootCheck(e, p)
	})
	bind("ccRootSet", func(p string) (RootView, error) {
		return rootSet(e, p)
	})
	bind("ccRootBrowse", func(start string) BrowseResult {
		return rootBrowse(e, start)
	})
	bind("ccGit", func(req GitRequest) centre.GitView {
		return gitView(e, req)
	})
	bind("ccGitRepo", func(id string) centre.GitDetailView {
		return gitRepo(e, id)
	})
}

// settingsSet is the ccSettingsSet handler: it keeps the current Root,
// normalizes, saves to e.settingsPath and returns the saved value.
func settingsSet(e bindEnv, s centre.Settings) (centre.Settings, error) {
	s.Root = e.settings.get().Root
	s = s.Normalized()
	if err := centre.SaveSettings(e.settingsPath, s); err != nil {
		return e.settings.get(), err
	}
	e.settings.set(s)
	return s, nil
}

// runBinding is the ccRun handler. A refusal comes back as a view with a nil
// error: the binding drops the value of a call that errors, and the page must
// draw the refusal and the audit file path (spec item 6).
func runBinding(e bindEnv, token string) (RunView, error) {
	v, err := runDecision(e, token)
	if err != nil && v.Refused != "" {
		return v, nil
	}
	return v, err
}

// commentWrite is the ccComment handler.
func commentWrite(e bindEnv, req centre.CommentRequest) (centre.CommentResult, error) {
	return centre.WriteComment(e.rootNow(), e.clock(), rand.Reader, req)
}

// boardPage is the ccBoard handler.
func boardPage(e bindEnv, path string) (centre.BoardPage, error) {
	return centre.ReadBoard(e.rootNow(), path)
}
