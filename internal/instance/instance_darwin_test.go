//go:build darwin

package instance

import (
	"path/filepath"
	"testing"
)

func TestAcquireTwice(t *testing.T) {
	p := filepath.Join(t.TempDir(), "burnmon.lock")
	held, err := Acquire(p)
	if err != nil || held {
		t.Fatalf("first Acquire = %v, %v; want false, nil", held, err)
	}
	held, err = Acquire(p)
	if err != nil || !held {
		t.Fatalf("second Acquire = %v, %v; want true, nil", held, err)
	}
}
