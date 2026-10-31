//go:build windows

package watch

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// notifyMask is what recursiveRoot asks ReadDirectoryChangesW to report:
// enough to see a new file, a write to an existing one, and a rename into
// place (Codex and Claude both write session files by create-then-append,
// never rename-into-place, but a rename is cheap to also catch).
//
// FILE_NOTIFY_CHANGE_SIZE (WS2 follow-up, 2026-09-29, item 1): an append
// to a file its writer holds open (Codex keeps a rollout open for the
// whole chat) does not move last-write, so LAST_WRITE alone never reports
// it. Size changes are also reported only when the cache flushes, so this
// narrows the gap rather than closing it; the tail poll (tail.go) is the
// real fix.
const notifyMask = windows.FILE_NOTIFY_CHANGE_FILE_NAME |
	windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_LAST_WRITE |
	windows.FILE_NOTIFY_CHANGE_SIZE

// bufSize is the overlapped read buffer per root. 64KB is the largest size
// still guaranteed to work over SMB (the same ceiling fsnotify's own
// Windows backend documents), and one buffer covers every change anywhere
// under the whole recursive subtree between two reads, not one directory's
// worth as before.
const bufSize = 64 * 1024

// notifyInfoHeaderSize is sizeof(NextEntryOffset)+sizeof(Action)+
// sizeof(FileNameLength), the three fixed uint32 fields FILE_NOTIFY_INFORMATION
// carries before its variable-length FileName. decode never dereferences a
// FileNotifyInformation, or slices its FileName, without first checking this
// many bytes (and then FileNameLength more) actually fit in what the kernel
// said it wrote.
const notifyInfoHeaderSize = 12

// recursiveRoot is one outstanding, recursive ReadDirectoryChangesW read
// against a single native adapter root. ov must be the first field:
// GetQueuedCompletionStatus hands back a *windows.Overlapped, which is
// reinterpreted as a *recursiveRoot as soon as it comes back (the same
// trick fsnotify's own Windows backend uses for the identical reason: the
// overlapped struct's address IS the containing struct's address when it
// is the first field).
//
// mu guards handle and closed together with every ReadDirectoryChanges call
// against this root (issueReadLocked) and the Close sequence, so a read is
// never issued against a handle Close has already started tearing down, and
// Close never closes a handle out from under a read run just issued (a
// review of the first version of this file caught both: CancelIo only
// cancels I/O issued by the calling thread, never the reader goroutine's own
// re-arms, so Close's old CancelIo call was a no-op, and there was no lock
// stopping the two from interleaving at all).
type recursiveRoot struct {
	ov windows.Overlapped

	mu      sync.Mutex
	handle  windows.Handle
	root    string
	buf     []byte
	closed  bool
	reading bool // true exactly while a ReadDirectoryChanges call is outstanding
}

// issueReadLocked issues (or re-issues) rr's recursive read. Caller must
// hold rr.mu.
func (rr *recursiveRoot) issueReadLocked() error {
	err := windows.ReadDirectoryChanges(rr.handle, &rr.buf[0], uint32(len(rr.buf)),
		true, notifyMask, nil, &rr.ov, 0)
	if err != nil {
		return os.NewSyscallError("ReadDirectoryChanges", err)
	}
	return nil
}

// windowsRecursiveBackend is the Windows nativeBackend: one recursiveRoot
// per configured native adapter root, added once at construction, sharing a
// single I/O completion port and one reader goroutine. Replaces WS3
// hypothesis 1's one-fsnotify-watch-per-subdirectory design; see watch.go's
// package doc and nativeBackend doc for the measured handle counts.
//
// fsnotify v1.10.1 (this module's vendored version) does carry an internal
// recursive-watch code path (backend_windows.go's recursivePath, reachable
// through the undocumented `root + "\..."` Add() convention), but its own
// doc comment says plainly: "Recursive watching is not currently enabled
// through fsnotify's public API; the recursive code path is gated and only
// exercised by fsnotify's own tests." Tried directly (a throwaway program:
// Add(root+`\...`), then create a new nested folder and a file inside it
// back to back, the same F7 race TestWatcher_NewNestedDayFolderRace below
// reproduces): it reported only one event, for the top-level folder, with
// the wrong path (the literal "..." segment left in Event.Name), and never
// delivered the nested file's own event at all. Confirmed unusable rather
// than assumed; hence the small hand-rolled watcher below instead.
type windowsRecursiveBackend struct {
	port     windows.Handle
	onChange OnChange
	match    func(path string) bool // which paths decode reports

	mu    sync.Mutex
	roots []*recursiveRoot

	wg sync.WaitGroup // the reader goroutine (run) itself

	// draining counts reads Close has just cancelled but whose final
	// completion has not yet been observed by run. Close waits on it before
	// posting the shutdown signal, so every kernel write into a root's ov/
	// buf is guaranteed to have already happened before this backend (and
	// the memory the kernel was writing into) can become unreachable. A
	// second review caught that closing a handle unblocks a pending
	// overlapped read but does not itself guarantee the completion has
	// already been delivered through the port; without this wait, a test
	// that repeatedly constructs and Stops a Watcher (as this package's own
	// test suite does) could in principle free memory the kernel was still
	// writing to a moment later. Not a real production risk (the app never
	// calls Stop; the watcher lives for the process's whole life and is
	// torn down by process exit, not this code), but worth being correct
	// about since the test suite exercises it constantly.
	draining sync.WaitGroup
}

func newNativeBackend(onChange OnChange, match func(string) bool) (nativeBackend, error) {
	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if err != nil {
		return nil, os.NewSyscallError("CreateIoCompletionPort", err)
	}
	b := &windowsRecursiveBackend{port: port, onChange: onChange, match: match}
	b.wg.Add(1)
	go b.run()
	return b, nil
}

// AddRoot opens root with FILE_FLAG_OVERLAPPED, associates it with the
// shared completion port, and issues the first recursive read. A root that
// does not exist or cannot be opened (permission denied), or whose first
// read fails to arm, is logged and skipped, its handle closed, and it is
// never added to b.roots: it falls back to being caught by the 15-minute
// full rescan, same as an unwatchable subdirectory was under the old
// per-directory design.
func (b *windowsRecursiveBackend) AddRoot(root string) error {
	root = filepath.Clean(root)
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(p,
		windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		log.Printf("watch: could not open %s for a recursive watch: %v", root, err)
		return os.NewSyscallError("CreateFile", err)
	}
	if _, err := windows.CreateIoCompletionPort(handle, b.port, 0, 0); err != nil {
		windows.CloseHandle(handle)
		log.Printf("watch: could not associate %s with the completion port: %v", root, err)
		return os.NewSyscallError("CreateIoCompletionPort", err)
	}
	rr := &recursiveRoot{handle: handle, root: root, buf: make([]byte, bufSize)}
	rr.mu.Lock()
	err = rr.issueReadLocked()
	if err == nil {
		rr.reading = true
	}
	rr.mu.Unlock()
	if err != nil {
		windows.CloseHandle(handle)
		log.Printf("watch: could not start watching %s: %v", root, err)
		return err
	}
	b.mu.Lock()
	b.roots = append(b.roots, rr)
	b.mu.Unlock()
	return nil
}

func (b *windowsRecursiveBackend) WatchCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.roots)
}

// run is the single I/O thread for every root this backend owns: it blocks
// on the shared completion port, decodes whichever root's buffer just
// completed into a list of changed matching paths, re-arms that root's read,
// then dispatches onChange for each path. Re-arming before dispatch (rather
// than after, the first version of this file's own order) matters: onChange
// is dataset.Cache.IngestFile, which can take tens to hundreds of
// milliseconds on a large file, and with one shared 64KB buffer per root
// (covering every subdirectory under it, unlike the old per-directory
// design's own small buffer per folder) leaving the read un-armed for that
// long widens the window for a kernel-side buffer overflow from "one
// folder's changes lost" to "every change under the whole root lost until
// the next full rescan".
//
// Exits once every root's handle has been closed and its final (aborted or
// stale-success) completion has drained, i.e. after Close has run.
func (b *windowsRecursiveBackend) run() {
	defer b.wg.Done()
	runtime.LockOSThread()
	for {
		var n uint32
		var key uintptr
		var ov *windows.Overlapped
		err := windows.GetQueuedCompletionStatus(b.port, &n, &key, &ov, windows.INFINITE)
		if ov == nil {
			if err != nil {
				// A real, unexpected failure (for example
				// ERROR_ABANDONED_WAIT_0 if the port handle were ever
				// closed with this goroutine still waiting on it, which
				// Close's own ordering is designed to avoid): log it, since
				// silently returning here means live watching is dead with
				// no trace until the process is restarted.
				log.Printf("watch: GetQueuedCompletionStatus: %v", err)
			}
			// The port itself was closed (Close, after every root's handle
			// is already gone) or a spurious wakeup with no associated
			// overlapped: either way, nothing left to read.
			return
		}
		rr := (*recursiveRoot)(unsafe.Pointer(ov))

		rr.mu.Lock()
		rr.reading = false // this completion's read is no longer outstanding
		alreadyClosed := rr.closed
		rr.mu.Unlock()
		if alreadyClosed {
			// Close already started tearing this root down and is waiting
			// on b.draining for exactly this completion (whatever (err, n)
			// it happens to carry, not reliably ERROR_OPERATION_ABORTED,
			// see Close's own comment): not real data, no re-arm.
			b.draining.Done()
			continue
		}

		if err != nil {
			// Try once to recover rather than permanently killing this
			// root's live watching on a transient error (a review caught
			// the first version of this file giving up for good here,
			// silently falling back to the 15-minute rescan forever
			// instead of just until the next successful read): re-arm
			// immediately, the same as the success path below does.
			rr.mu.Lock()
			var rearmErr error
			if !rr.closed {
				if rearmErr = rr.issueReadLocked(); rearmErr == nil {
					rr.reading = true
				}
			}
			rr.mu.Unlock()
			if rearmErr != nil {
				log.Printf("watch: ReadDirectoryChanges error on %s: %v (re-arm also failed: %v); live watching of this root has stopped, falling back to the periodic full rescan", rr.root, err, rearmErr)
			} else {
				log.Printf("watch: ReadDirectoryChanges error on %s, re-armed: %v", rr.root, err)
			}
			continue
		}

		paths := b.decode(rr, n)
		rr.mu.Lock()
		if !rr.closed {
			if err := rr.issueReadLocked(); err != nil {
				log.Printf("watch: could not re-arm the watch on %s: %v; live watching of this root has stopped, falling back to the periodic full rescan", rr.root, err)
			} else {
				rr.reading = true
			}
		}
		rr.mu.Unlock()
		for _, p := range paths {
			b.onChange(p)
		}
	}
}

// decode walks one completed FILE_NOTIFY_INFORMATION buffer (n bytes) and
// returns every create, write, or rename-into-place path b.match accepts,
// found anywhere under rr.root, at any depth (the entire benefit of the recursive
// flag on ReadDirectoryChanges over the old per-directory design), without
// calling onChange itself: run re-arms the read against rr.buf before
// dispatching any of them, so nothing here may retain a reference into
// rr.buf past this call.
func (b *windowsRecursiveBackend) decode(rr *recursiveRoot, n uint32) []string {
	if n == 0 {
		// The kernel's own notification buffer overflowed (too many changes
		// between two reads to fit bufSize): some changes under rr.root
		// were silently dropped. The 15-minute full rescan is the safety
		// net, exactly as it always was for a permission-denied subfolder
		// under the old design.
		log.Printf("watch: notification buffer overflowed on %s; some changes may be missed until the next full rescan", rr.root)
		return nil
	}
	// n is the kernel's own "bytes actually written" count, trusted for a
	// well-formed buffer, but clamped and walked in uint64 below anyway: a
	// second review pointed out the uint32 arithmetic this loop used to do
	// (offset+notifyInfoHeaderSize+raw.FileNameLength, offset+=
	// raw.NextEntryOffset) could in principle wrap around for a corrupt
	// buffer, turning a bounds check meant to reject it into one that
	// passes. None of this is reachable with a genuine kernel-written
	// buffer; it costs nothing to not depend on that.
	if n64 := uint64(n); n64 > uint64(len(rr.buf)) {
		n = uint32(len(rr.buf))
	}
	n64 := uint64(n)
	var out []string
	var offset uint64
	for {
		if offset+notifyInfoHeaderSize > n64 {
			break
		}
		raw := (*windows.FileNotifyInformation)(unsafe.Pointer(&rr.buf[offset]))
		if offset+notifyInfoHeaderSize+uint64(raw.FileNameLength) > n64 {
			break
		}
		nameLen := raw.FileNameLength / 2
		nameUTF16 := unsafe.Slice(&raw.FileName, nameLen)
		name := windows.UTF16ToString(nameUTF16)
		full := filepath.Join(rr.root, name)
		switch raw.Action {
		case windows.FILE_ACTION_ADDED, windows.FILE_ACTION_MODIFIED, windows.FILE_ACTION_RENAMED_NEW_NAME:
			if b.match(full) {
				out = append(out, full)
			}
		}
		if raw.NextEntryOffset < notifyInfoHeaderSize {
			// 0 is the kernel's own "no more entries" marker; anything
			// else smaller than one entry's fixed header cannot be a real
			// offset to the next one (it would either re-read this same
			// entry or land inside it), so treat it the same as "no more
			// entries" rather than looping on it.
			break
		}
		offset += uint64(raw.NextEntryOffset)
		if offset >= n64 {
			break
		}
	}
	return out
}

// Close cancels and closes every root's handle, waits for each one that
// actually had a read outstanding to deliver its final completion (b.draining,
// drained by run), then posts a null completion packet to wake run out of
// its blocking wait, waits for it to exit, and only then closes the shared
// completion port. Order matters twice over:
//
//   - Waiting for every real drain before posting the shutdown signal, not
//     just after: closing a handle unblocks a pending overlapped read but
//     does not itself guarantee the completion has already been delivered
//     through the port (a second review caught the first version of this
//     fix racing its own shutdown packet against those real completions,
//     which could let this backend, and the memory the kernel was still
//     writing into, become unreachable before the kernel was actually done
//     with it).
//   - Closing the port only after run has exited: closing a completion port
//     while a thread is still blocked inside GetQueuedCompletionStatus on it
//     is not a documented, safe way to unblock that call, unlike posting a
//     packet with a nil overlapped, which run already treats as its own
//     shutdown signal.
//
// windows.CancelIoEx, not CancelIo: CancelIo only cancels I/O issued by the
// calling thread, never anything issued by run's own re-arms (a review of
// this file's first version caught that its CancelIo call was consequently
// a no-op, CloseHandle alone forcing the observed completion). Each root's
// own mu is held for the whole cancel-and-close sequence, the same lock
// issueReadLocked takes, so a concurrent re-arm in run either finishes and
// gets properly cancelled here, or never starts because it sees closed
// already true; either way no read is ever issued against, or left
// outstanding on, a handle this function has closed.
func (b *windowsRecursiveBackend) Close() {
	b.mu.Lock()
	roots := b.roots
	b.mu.Unlock()
	for _, rr := range roots {
		rr.mu.Lock()
		rr.closed = true
		if rr.reading {
			// A read truly was outstanding at this instant (rr.reading is
			// only ever true while one genuinely is, see run's own upkeep
			// of it), so IOCP's contract guarantees exactly one more
			// completion for it, cancelled or not; run's own alreadyClosed
			// branch calls Done for it. Added while rr.mu is still held
			// (alpha.7, found as a flaky "negative WaitGroup counter"
			// panic in this package's tests): run reads rr.closed under
			// the same lock, so it can no longer reach that Done before
			// this Add.
			b.draining.Add(1)
		}
		windows.CancelIoEx(rr.handle, nil)
		windows.CloseHandle(rr.handle)
		rr.mu.Unlock()
	}
	b.draining.Wait()
	if err := windows.PostQueuedCompletionStatus(b.port, 0, 0, nil); err != nil {
		// run's wg.Wait() below would otherwise block forever: with no
		// wakeup delivered, it stays parked in GetQueuedCompletionStatus.
		// Nothing left to do but say so; the process still has every
		// root's handle closed, so no live watching leaks past this call
		// either way.
		log.Printf("watch: PostQueuedCompletionStatus (shutdown signal): %v; not waiting for the reader goroutine to exit", err)
		return
	}
	b.wg.Wait()
	windows.CloseHandle(b.port)
}

// sizeByHandle opens path for attributes only, sharing read, write and
// delete so the writer is never blocked, and reads its size from the open
// handle (GetFileInformationByHandle), the tail poll's source of truth.
func sizeByHandle(path string) (int64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return 0, err
	}
	return int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow), nil
}
