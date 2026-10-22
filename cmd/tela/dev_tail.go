package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
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

// childOutput gives a child pipes for its standard output and error and
// copies them to the supervisor's writers, each stream in order: a child,
// in its own process group, never writes to the terminal itself (ADR-119).
// The pipes are the supervisor's own (os.Pipe), not exec's, so cmd.Wait
// returns at the child's exit without waiting for the copies, which a
// process the child started and left running may keep open (ADR-121).
type childOutput struct {
	r, w []*os.File  // read ends (here) and write ends (the child's)
	dst  []io.Writer // where each read end is copied
	done chan struct{}
	cut  atomic.Bool // detached: the copies stop
}

// newChildOutput sets cmd's standard output (only when stdout is not nil:
// without it the child gets the null device) and standard error to pipes
// copied to stdout and stderr. Call start after cmd.Start, or close if it
// failed.
func newChildOutput(cmd *exec.Cmd, stdout, stderr io.Writer) (*childOutput, error) {
	o := &childOutput{done: make(chan struct{})}
	pipe := func(dst io.Writer) (*os.File, error) {
		r, w, err := os.Pipe()
		if err != nil {
			o.close()
			return nil, err
		}
		o.r, o.w, o.dst = append(o.r, r), append(o.w, w), append(o.dst, dst)
		return w, nil
	}
	if stdout != nil {
		w, err := pipe(stdout)
		if err != nil {
			return nil, err
		}
		cmd.Stdout = w
	}
	w, err := pipe(stderr)
	if err != nil {
		return nil, err
	}
	cmd.Stderr = w
	return o, nil
}

// start closes the write ends, which the child now holds, and copies the
// read ends until the end of each pipe, with spawn (the supervisor's
// goroutines); done is closed when every copy has ended.
func (o *childOutput) start(spawn func(func())) {
	for _, w := range o.w {
		w.Close()
	}
	var wg sync.WaitGroup
	for i, r := range o.r {
		dst := o.dst[i]
		wg.Add(1)
		spawn(func() {
			defer wg.Done()
			io.Copy(cutWriter{o, dst}, r)
			r.Close()
		})
	}
	spawn(func() {
		wg.Wait()
		close(o.done)
	})
}

// close closes every pipe end (the child could not start).
func (o *childOutput) close() {
	for _, f := range append(o.r, o.w...) {
		f.Close()
	}
}

// detach stops copying: what still arrives in the pipes is dropped, and
// the process that holds them gets a broken pipe when it writes.
func (o *childOutput) detach() {
	o.cut.Store(true)
	for _, r := range o.r {
		r.Close()
	}
}

var errDetached = errors.New("the output was detached")

// cutWriter is the destination of a copy, which ends once the output is
// detached.
type cutWriter struct {
	o *childOutput
	w io.Writer
}

func (c cutWriter) Write(p []byte) (int, error) {
	if c.o.cut.Load() {
		return 0, errDetached
	}
	return c.w.Write(p)
}

const (
	// childOutputSettle is how long the classification of a child's exit
	// waits for the end of its output (its last lines, a crash's text in
	// the tail): the end comes at once unless a process the child left
	// running holds the pipes.
	childOutputSettle = 200 * time.Millisecond
	// childOutputDetach is how long after a child's exit its output is
	// still copied while such a process holds the pipes.
	childOutputDetach = time.Second
)

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
