//go:build !windows && !(darwin && cgo)

package winbind

// VisibleSize is 0, 0 here: no native window uses it on this system, so the
// wanted size stays as it is.
func VisibleSize() (int, int) { return 0, 0 }
