//go:build !windows

package watch

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// fsnotifyBackend is the nativeBackend for B1's browser-mode darwin/linux
// build (untested on real hardware; no live app window there to make the
// handle count visible), unchanged from how every platform worked before
// WS3: one fsnotify watch per directory, added recursively under each root
// at AddRoot time and again, on demand, under any new subdirectory a live
// Create event reports.
type fsnotifyBackend struct {
	onChange OnChange
	match    func(path string) bool // which file paths reach onChange

	fsw       *fsnotify.Watcher
	watchedMu sync.Mutex
	watched   map[string]bool // directories already added to fsw

	wg sync.WaitGroup
}

func newNativeBackend(onChange OnChange, match func(string) bool) (nativeBackend, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	b := &fsnotifyBackend{onChange: onChange, match: match, fsw: fsw, watched: map[string]bool{}}
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.run()
	}()
	return b, nil
}

// AddRoot returns an error when root itself got no watch (missing, or not
// a folder), so WatchFiles can tell a dead watch from a live one, as the
// Windows backend already does. New ignores it.
func (b *fsnotifyBackend) AddRoot(root string) error {
	b.addTree(root, false)
	b.watchedMu.Lock()
	ok := b.watched[root]
	b.watchedMu.Unlock()
	if !ok {
		return fmt.Errorf("watch: %s is not watched", root)
	}
	return nil
}

// addTree adds root and every subdirectory under it to the fsnotify
// watcher. Errors (root missing, a permission-denied subfolder) are logged
// and otherwise ignored: a folder that cannot be watched simply falls back
// to being caught by the 15-minute full rescan, same as before this package
// existed.
//
// notifyExisting, when true, also calls onChange for every file b.match
// accepts already present under root (root's own watch is skipped if already
// registered, but each subdirectory found is walked and watched exactly as
// during startup). This closes the race behind F7 (SESSION_LOG.md, v0.1.2,
// confirmed by TestWatcher_NewNestedDayFolderRace): a rollout writer that
// creates a whole new nested folder (Codex's YYYY/MM/DD) and its first file
// back to back can have the file's own Create event fire, and be silently
// dropped by Windows ReadDirectoryChanges, before fsw.Add on the brand-new
// leaf directory has run. Re-listing the directory right after the watch is
// added catches any file that raced past that window; onChange is safe to
// call again for a file the live watcher (or the full rescan) already saw,
// since dataset.Cache.IngestFile ingests by cursor and a repeat call is a
// no-op. handleEvent always passes true: every directory it sees is one
// fsnotify just told it about, i.e. new since startup, so nothing here
// duplicates the initial backfill.
func (b *fsnotifyBackend) addTree(root string, notifyExisting bool) {
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if notifyExisting && b.match(p) {
				b.onChange(p)
			}
			return nil
		}
		b.watchedMu.Lock()
		already := b.watched[p]
		b.watchedMu.Unlock()
		if already {
			return nil
		}
		if err := b.fsw.Add(p); err != nil {
			log.Printf("watch: could not watch %s: %v", p, err)
			return nil
		}
		b.watchedMu.Lock()
		b.watched[p] = true
		b.watchedMu.Unlock()
		return nil
	})
	if err != nil {
		log.Printf("watch: could not walk %s: %v", root, err)
	}
}

func (b *fsnotifyBackend) WatchCount() int {
	b.watchedMu.Lock()
	defer b.watchedMu.Unlock()
	return len(b.watched)
}

func (b *fsnotifyBackend) Close() {
	b.fsw.Close()
	b.wg.Wait()
}

func (b *fsnotifyBackend) run() {
	for {
		select {
		case ev, ok := <-b.fsw.Events:
			if !ok {
				return
			}
			b.handleEvent(ev)
		case err, ok := <-b.fsw.Errors:
			if !ok {
				return
			}
			log.Printf("watch: fsnotify error: %v", err)
		}
	}
}

func (b *fsnotifyBackend) handleEvent(ev fsnotify.Event) {
	if ev.Op&(fsnotify.Create|fsnotify.Write) == 0 {
		return
	}
	fi, err := os.Stat(ev.Name)
	if err != nil {
		return // gone already, or a rename/remove we don't care about
	}
	if fi.IsDir() {
		if ev.Op&fsnotify.Create != 0 {
			// A new project or session folder: watch it (and anything
			// under it) too, so files written inside it are seen from here
			// on. Pass notifyExisting=true (F7 fix): this directory is new
			// since startup, so any matching file already inside it raced
			// its own Create event past the watch not existing yet and
			// must be picked up here instead.
			b.addTree(ev.Name, true)
		}
		return
	}
	if b.match(ev.Name) {
		b.onChange(ev.Name)
	}
}

// sizeByHandle is the tail poll's size read. Outside Windows an open
// file's size is current either way, so a plain Stat does.
func sizeByHandle(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
