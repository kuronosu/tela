//go:build linux || windows

package watch

// lost records that events were dropped: inotify's queue or a
// ReadDirectoryChangesW buffer overflowed. kqueue and the poller drop none.
// lost records that events were dropped.
func (w *Watcher) lost() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.overflow = true
	w.arm()
}
