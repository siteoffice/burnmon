//go:build !windows

package toast

import "errors"

// Notifier is a stub off Windows: Start always fails, so the app runs on
// without toasts.
type Notifier struct{}

// Start reports that toasts need Windows.
func Start(appName string, onClick func()) (*Notifier, error) {
	return nil, errors.New("toasts are Windows only")
}

// Show does nothing.
func (n *Notifier) Show(text string) {}

// Close does nothing.
func (n *Notifier) Close() {}
