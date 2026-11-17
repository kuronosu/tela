package core

import (
	"bytes"
	"io"
	"log/slog"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
)

// ADR-297: what package desktop reaches through core for logs and crash
// reports. core cannot import desktop, so it offers three process-wide
// hooks that tela.Run's start installs: a tee of Tela's own messages, the
// panics of the UI thread and of UseGo goroutines, and the app's first
// commit.

// logTee is the function given to SetLogTee.
var logTee atomic.Pointer[func(slog.Level, string)]

// SetLogTee makes f receive a copy of every message Tela prints for the
// developer (dev warnings, logged errors, the platform's errors): they
// still go where they went (standard error, or Options.ErrorLog), f only
// sees them too. A message is one line without its newline. nil removes
// the tee. Process-wide; safe from any goroutine; f must be too.
func SetLogTee(f func(level slog.Level, msg string)) {
	if f == nil {
		logTee.Store(nil)
		return
	}
	logTee.Store(&f)
}

// tee gives msg to the tee, if there is one.
func tee(level slog.Level, msg string) {
	if p := logTee.Load(); p != nil {
		(*p)(level, msg)
	}
}

// teeWriter is a Host's log: it writes to w and copies the lines to the
// tee as warnings.
type teeWriter struct{ w io.Writer }

func (t teeWriter) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	for line := range bytes.SplitSeq(bytes.TrimRight(p, "\r\n"), []byte("\n")) {
		if len(line) > 0 {
			tee(slog.LevelWarn, string(line))
		}
	}
	return n, err
}

// hostLog returns the writer a Host logs to: w, with the tee when one is
// installed.
func hostLog(w io.Writer) io.Writer {
	if logTee.Load() == nil {
		return w
	}
	return teeWriter{w}
}

// ---------------------------------------------------------------------
// Panics.

// What OnPanic's functions are told about the goroutine that panicked.
const (
	PanicUI = "the UI thread"
	PanicGo = "a UseGo goroutine"
)

var panicHooks struct {
	mu sync.Mutex
	fs []func(where string, v any, stack []byte)
}

// OnPanic registers f to run when the UI thread (Run's goroutine: the
// handlers, builds and effects it runs) or a UseGo goroutine panics, with
// the panic value and the stack at the panic, before the panic goes on:
// Tela panics again with the same value afterwards, so the program
// crashes as it would have. A panic of f is ignored. Process-wide; safe
// from any goroutine.
func OnPanic(f func(where string, v any, stack []byte)) {
	panicHooks.mu.Lock()
	defer panicHooks.mu.Unlock()
	panicHooks.fs = append(panicHooks.fs, f)
}

// crashRecover is deferred, with a literal call, by Run and by the
// goroutines of UseGo: it gives a panic to the functions of OnPanic and
// panics again with the same value.
func crashRecover(where string) {
	v := recover()
	if v == nil {
		return
	}
	stack := debug.Stack()
	panicHooks.mu.Lock()
	fs := slices.Clone(panicHooks.fs)
	panicHooks.mu.Unlock()
	for _, f := range fs {
		func() {
			defer func() { _ = recover() }()
			f(where, v, stack)
		}()
	}
	panic(v)
}

// ---------------------------------------------------------------------
// The first commit.

var firstCommit struct {
	mu sync.Mutex
	fs []func()
}

// OnFirstCommit registers f to run, on the UI thread, once the main
// window of an app made its first commit (its first frame): package
// desktop hands the crash reports of the previous run to the app there.
// Process-wide; safe from any goroutine.
func OnFirstCommit(f func()) {
	firstCommit.mu.Lock()
	defer firstCommit.mu.Unlock()
	firstCommit.fs = append(firstCommit.fs, f)
}

// releaseFirstCommit runs the functions of OnFirstCommit once, after the
// event that ended the main window's first commit (HandleEvent).
func (h *Host) releaseFirstCommit() {
	if h.firstDone || !h.main.committed || h.ended {
		return
	}
	h.firstDone = true
	firstCommit.mu.Lock()
	fs := slices.Clone(firstCommit.fs)
	firstCommit.mu.Unlock()
	for _, f := range fs {
		f()
	}
}
