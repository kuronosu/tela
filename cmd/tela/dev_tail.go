package main

import (
	"bytes"
	"io"
	"sync"
)

// tailLimit is how much of a child's standard error is kept.
const tailLimit = 32 << 10

// tailWriter forwards a child's standard error and keeps its last bytes.
// The Go runtime ends a process that panics in a goroutine other than the
// UI thread's (which the child runtime reports itself, devloop.KindPanic)
// or dies of a fatal error with a message on standard error and status 2:
// the supervisor finds it here, so the problem shows like a reported panic
// (ADR-114).
type tailWriter struct {
	w io.Writer

	mu  sync.Mutex
	buf []byte
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > tailLimit {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-tailLimit:]...)
	}
	t.mu.Unlock()
	if t.w == nil {
		return len(p), nil
	}
	return t.w.Write(p)
}

// crash returns the text of the last panic or fatal error the runtime
// printed ("panic: ..." or "fatal error: ..." through the end), or "".
func (t *tailWriter) crash() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	best := -1
	for _, marker := range []string{"panic: ", "fatal error: "} {
		if i := lastLineStart(t.buf, marker); i > best {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return string(bytes.TrimRight(t.buf[best:], "\n"))
}

// lastLineStart returns the index of the last line of b that starts with
// prefix, or -1.
func lastLineStart(b []byte, prefix string) int {
	last := -1
	for i := 0; i < len(b); {
		if bytes.HasPrefix(b[i:], []byte(prefix)) {
			last = i
		}
		j := bytes.IndexByte(b[i:], '\n')
		if j < 0 {
			break
		}
		i += j + 1
	}
	return last
}
