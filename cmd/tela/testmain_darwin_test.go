//go:build darwin

package main

import (
	"os"
	"testing"
)

// AppKit runs on the main thread only. The main goroutine is locked to it
// (dev.go's init), and TestMain runs there: serveTests runs the tests on
// another goroutine and serves onMainAsync with the main thread, as tela
// dev's own main does with its event loop. The tests' goroutine exits the
// process itself, so a call that never returns (an event loop that does not
// quit after a failure) cannot keep the test binary alive.

var mainCalls = make(chan func())

func serveTests(m *testing.M) {
	go func() { os.Exit(m.Run()) }()
	for f := range mainCalls {
		f()
	}
}

// onMainAsync runs f on the main thread and returns at once; the channel
// is closed when f returns. f must not call t.Fatal, t.Skip or
// runtime.Goexit, which would end the main goroutine.
func onMainAsync(f func()) <-chan struct{} {
	done := make(chan struct{})
	mainCalls <- func() {
		defer close(done)
		f()
	}
	return done
}
