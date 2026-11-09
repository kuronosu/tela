//go:build !linux && !windows && !darwin

package watch

// newNative reports that there is no native watcher here (the BSDs and
// the rest poll; macOS uses kqueue, kqueue_darwin.go).
func newNative(w *Watcher) (impl, error) { return nil, nil }
