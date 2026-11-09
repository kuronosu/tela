//go:build !linux && !windows

package watch

// newNative reports that there is no native watcher here (macOS: FSEvents
// needs CoreServices through purego, kqueue a descriptor per file; polling
// is enough for the dev loop: tela dev on macOS polls).
func newNative(w *Watcher) (impl, error) { return nil, nil }
