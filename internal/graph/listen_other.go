//go:build !windows

package graph

// Listening is Windows only for now (the Mac is step 2).
func Listening() ([]Listen, error) { return []Listen{}, nil }
