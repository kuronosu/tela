// Package watch reports changes of files under a set of directory trees,
// debounced into batches: the file watcher of tela dev (ADR-115).
//
// Watching is native where the OS has a recursive notification API:
// inotify on Linux (one watch per directory, added for new directories as
// they appear) and ReadDirectoryChangesW on Windows (one handle per root,
// with its subtree). Elsewhere, and on request ([Options.Poll]), the trees
// are polled. It needs no dependency besides golang.org/x/sys.
//
// What counts as a change is the caller's: [Options.Filter] sees every
// path, on the watcher's goroutines, before the debounce, so a file that
// changes all the time but does not matter (a log the app writes) never
// delays a batch. Editors that save by writing a temporary file and
// renaming it over the original (vim, GoLand, VS Code with some settings)
// produce a create or a rename of the original's name, which is reported
// like a write.
package watch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"
)

// Options configures a Watcher.
type Options struct {
	// Filter reports whether a change of path matters. dir is true for a
	// directory that disappeared from a watched tree (moved away, or, on
	// Windows, a removed name that may have been one); files inside a new
	// directory are reported one by one. Nil accepts everything. Called on
	// the watcher's goroutines: it must be safe for concurrent use.
	Filter func(path string, dir bool) bool
	// SkipDir reports whether a directory inside a root is not watched,
	// with its subtree (".git", the build directory). It is never called
	// for a root. Nil watches everything. Same goroutine rules as Filter.
	SkipDir func(path string) bool
	// Debounce is how long the watcher waits after the last change before
	// it delivers a batch. Zero means 100 ms.
	Debounce time.Duration
	// MaxDelay bounds the wait from the first change of a batch, so that
	// changes that never stop still produce batches. Zero means 1 s.
	MaxDelay time.Duration
	// Poll makes the watcher poll even where native watching works.
	Poll bool
	// PollInterval is the polling period. Zero means 300 ms.
	PollInterval time.Duration
}

// Batch is a debounced set of changes.
type Batch struct {
	// Paths are the changed paths that passed the filter, sorted and
	// unique.
	Paths []string
	// First is when the first change of the batch was seen.
	First time.Time
	// Overflow says the OS dropped events: anything may have changed.
	Overflow bool
}

// Watcher watches directory trees. Create it with New.
type Watcher struct {
	opts   Options
	out    chan Batch
	errs   chan error
	native bool

	mu       sync.Mutex
	impl     impl
	roots    []string
	pending  map[string]struct{}
	overflow bool
	first    time.Time
	last     time.Time
	timer    *time.Timer
	queued   *Batch        // a batch waiting for the consumer
	kick     chan struct{} // wakes the delivery goroutine
	closed   bool
	done     chan struct{}
}

// impl is a watching mechanism.
type impl interface {
	// setRoots replaces the watched trees (the caller already compared the
	// lists).
	setRoots(roots []string) error
	close() error
}

// ErrClosed is returned by the methods of a closed Watcher.
var ErrClosed = errors.New("watch: watcher closed")

// New returns a Watcher of the given roots (directories; more can be set
// with SetRoots). It falls back to polling when native watching is not
// available.
func New(opts Options, roots ...string) (*Watcher, error) {
	if opts.Debounce <= 0 {
		opts.Debounce = 100 * time.Millisecond
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = time.Second
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 300 * time.Millisecond
	}
	w := &Watcher{
		opts:    opts,
		out:     make(chan Batch),
		errs:    make(chan error, 16),
		pending: map[string]struct{}{},
		kick:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	var im impl
	if !opts.Poll {
		var err error
		im, err = newNative(w)
		if err != nil {
			w.error(err)
			im = nil
		} else {
			w.native = im != nil
		}
	}
	if im == nil {
		im = newPoller(w)
	}
	w.impl = im
	go w.deliver()
	if err := w.SetRoots(roots); err != nil {
		w.Close()
		return nil, err
	}
	return w, nil
}

// Native reports whether the watcher uses the OS's notifications rather
// than polling.
func (w *Watcher) Native() bool { return w.native }

// Events returns the channel of batches. It is closed by Close.
func (w *Watcher) Events() <-chan Batch { return w.out }

// Errors returns non-fatal errors (a directory that could not be watched,
// a watch limit). Errors are dropped when nobody reads them.
func (w *Watcher) Errors() <-chan error { return w.errs }

// Roots returns the watched roots.
func (w *Watcher) Roots() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.roots)
}

// SetRoots replaces the watched trees. Roots are cleaned; duplicates are
// dropped; a root that does not exist is reported on Errors and skipped.
// Changes made while the watches are replaced may be missed.
func (w *Watcher) SetRoots(roots []string) error {
	var rs []string
	for _, r := range roots {
		r = filepath.Clean(r)
		if !slices.Contains(rs, r) {
			rs = append(rs, r)
		}
	}
	sort.Strings(rs)
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return ErrClosed
	}
	if slices.Equal(rs, w.roots) {
		w.mu.Unlock()
		return nil
	}
	w.roots = rs
	im := w.impl
	w.mu.Unlock()
	var ok []string
	for _, r := range rs {
		if fi, err := os.Stat(r); err != nil || !fi.IsDir() {
			if err == nil {
				err = errors.New("not a directory")
			}
			w.error(&fs.PathError{Op: "watch", Path: r, Err: err})
			continue
		}
		ok = append(ok, r)
	}
	return im.setRoots(ok)
}

// Close stops watching and closes Events. Safe to call more than once.
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
	}
	im := w.impl
	w.mu.Unlock()
	close(w.done)
	return im.close()
}

// changed records a change of path (called by the implementations).
func (w *Watcher) changed(path string, dir bool) {
	if f := w.opts.Filter; f != nil && !f(path, dir) {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.pending[path] = struct{}{}
	w.arm()
}

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

// arm (re)starts the debounce timer. w.mu is held.
func (w *Watcher) arm() {
	now := time.Now()
	if w.first.IsZero() {
		w.first = now
	}
	w.last = now
	d := w.opts.Debounce
	if rest := w.opts.MaxDelay - now.Sub(w.first); rest < d {
		d = max(rest, 0)
	}
	if w.timer == nil {
		w.timer = time.AfterFunc(d, w.fire)
	} else {
		w.timer.Reset(d)
	}
}

// fire moves the pending changes into the queued batch.
func (w *Watcher) fire() {
	w.mu.Lock()
	if w.closed || (len(w.pending) == 0 && !w.overflow) {
		w.mu.Unlock()
		return
	}
	// A change may have re-armed the timer after it fired: wait for the
	// quiet period unless the batch is due.
	now := time.Now()
	if quiet := now.Sub(w.last); quiet < w.opts.Debounce && now.Sub(w.first) < w.opts.MaxDelay {
		w.timer.Reset(w.opts.Debounce - quiet)
		w.mu.Unlock()
		return
	}
	b := w.queued
	if b == nil {
		b = &Batch{First: w.first}
	}
	for p := range w.pending {
		b.Paths = append(b.Paths, p)
	}
	sort.Strings(b.Paths)
	b.Paths = slices.Compact(b.Paths)
	b.Overflow = b.Overflow || w.overflow
	w.queued = b
	clear(w.pending)
	w.overflow = false
	w.first = time.Time{}
	w.mu.Unlock()
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// deliver sends queued batches to the consumer; batches that become due
// while the consumer is busy merge into one.
func (w *Watcher) deliver() {
	defer close(w.out)
	for {
		select {
		case <-w.done:
			return
		case <-w.kick:
		}
		w.mu.Lock()
		b := w.queued
		w.queued = nil
		w.mu.Unlock()
		if b == nil {
			continue
		}
		select {
		case w.out <- *b:
		case <-w.done:
			return
		}
	}
}

// error reports a non-fatal error.
func (w *Watcher) error(err error) {
	select {
	case w.errs <- err:
	default:
	}
}

// skip reports whether the directory path inside a root is skipped.
func (w *Watcher) skip(path string) bool {
	return w.opts.SkipDir != nil && w.opts.SkipDir(path)
}

// walk visits the tree at root: dir is called for every directory that is
// not skipped (root included) before its entries are read, and file, if
// not nil, for every other entry. Errors of vanished entries are ignored.
func (w *Watcher) walk(root string, dir func(path string) error, file func(path string, d fs.DirEntry)) {
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root || errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			w.error(err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != root && w.skip(path) {
				return filepath.SkipDir
			}
			if dir != nil {
				if err := dir(path); err != nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if file != nil {
			file(path, d)
		}
		return nil
	})
}

// within reports whether path is root or inside it.
func within(path, root string) bool {
	if path == root {
		return true
	}
	if len(path) <= len(root) || path[:len(root)] != root {
		return false
	}
	return os.IsPathSeparator(path[len(root)]) || os.IsPathSeparator(root[len(root)-1])
}
