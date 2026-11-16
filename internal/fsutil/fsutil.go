// Package fsutil holds small file helpers shared by the apps.
package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// ReadFileRetry reads path, trying again briefly when the read fails for a
// reason other than a missing file. On Windows a read that meets the instant of
// WriteFileAtomic's rename fails with an access error; a caller must not take
// that for a file that cannot be read and fall back to defaults.
func ReadFileRetry(path string) ([]byte, error) {
	var b []byte
	var err error
	for i := 0; i < 20; i++ {
		if b, err = os.ReadFile(path); err == nil || errors.Is(err, fs.ErrNotExist) {
			return b, err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return b, err
}

// WriteFileAtomic writes data to path through a temp file in the same folder
// and a rename, so a concurrent reader sees the old file or the new one and
// never an empty one. The folder must exist. On Windows a rename can fail for
// a moment while another process holds the target open, so it is retried
// briefly; the temp file is removed on any failure.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	if werr == nil {
		werr = tmp.Sync()
	}
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	var rerr error
	for i := 0; i < 5; i++ {
		if rerr = os.Rename(tmpName, path); rerr == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	os.Remove(tmpName)
	return rerr
}
