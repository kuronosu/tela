package remote

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// transport is the byte stream under a connection: an AF_UNIX stream
// socket. On Unix it also carries file descriptors (SCM_RIGHTS) next to
// the bytes of a message: the shared memory of a window's buffers.
type transport interface {
	// Read reads bytes; descriptors received with them are queued for
	// takeFile.
	Read(p []byte) (int, error)
	// write writes all of b; f, if not nil, travels with b's first byte
	// (Unix only).
	write(b []byte, f *os.File) error
	// takeFile returns the oldest received descriptor not taken yet, or
	// nil.
	takeFile() *os.File
	SetReadDeadline(t time.Time) error
	Close() error
}

// ErrClosed is the error of a connection closed by this side (Backend.Close,
// Child.Close, Host.Close).
var ErrClosed = errors.New("remote: connection closed")

// frameReader splits the stream into frames. It is used by one goroutine
// at a time (the handshake, then the reader goroutine).
type frameReader struct {
	t          transport
	buf        []byte
	start, end int
}

func newFrameReader(t transport) *frameReader {
	return &frameReader{t: t, buf: make([]byte, 64<<10)}
}

// fill reads until at least n bytes are buffered.
func (r *frameReader) fill(n int) error {
	for r.end-r.start < n {
		if r.start > 0 && (r.start == r.end || len(r.buf)-r.start < n) {
			r.end = copy(r.buf, r.buf[r.start:r.end])
			r.start = 0
		}
		if len(r.buf)-r.start < n {
			nb := make([]byte, max(n, 2*len(r.buf)))
			r.end = copy(nb, r.buf[r.start:r.end])
			r.start = 0
			r.buf = nb
		}
		m, err := r.t.Read(r.buf[r.end:])
		r.end += m
		if err != nil {
			if r.end-r.start >= n {
				return nil
			}
			if err == io.EOF && r.end > r.start {
				return io.ErrUnexpectedEOF
			}
			return err
		}
	}
	return nil
}

// next returns the next frame's type and payload. The payload is valid
// until the following call.
func (r *frameReader) next() (msgType, []byte, error) {
	if err := r.fill(4); err != nil {
		return 0, nil, err
	}
	n := int(binary.LittleEndian.Uint32(r.buf[r.start:]))
	if n < 1 || n > maxFrame {
		return 0, nil, fmt.Errorf("remote: invalid frame length %d", n)
	}
	if err := r.fill(4 + n); err != nil {
		return 0, nil, err
	}
	f := r.buf[r.start+4 : r.start+4+n]
	r.start += 4 + n
	if r.start == r.end {
		r.start, r.end = 0, 0
	}
	return msgType(f[0]), f[1:], nil
}

// conn is one side of a connection: a reader (driven by the owner's
// reader goroutine) and a writer goroutine fed by a queue, so senders
// never block on the socket (the host's UI thread must not stall when a
// child stops reading).
type conn struct {
	t transport
	r *frameReader

	mu     sync.Mutex
	queue  []outItem
	err    error // set once, when the connection fails or is closed
	signal chan struct{}
	done   chan struct{} // closed with err
}

type outItem struct {
	b    []byte
	file *os.File   // closed after the write
	done chan error // optional: the write's outcome
}

func newConn(t transport, r *frameReader) *conn {
	c := &conn{t: t, r: r, signal: make(chan struct{}, 1), done: make(chan struct{})}
	go c.writeLoop()
	return c
}

// send queues a frame. It fails only once the connection is gone.
func (c *conn) send(b []byte) error { return c.enqueue(outItem{b: b}) }

// sendFile queues a frame that carries f (Unix descriptor passing). f is
// closed once written, or when the connection fails.
func (c *conn) sendFile(b []byte, f *os.File) error {
	return c.enqueue(outItem{b: b, file: f})
}

// sendWait queues a frame and waits until it is written to the socket (or
// the connection fails). Used for messages that must leave before the
// process may exit (a panic report).
func (c *conn) sendWait(b []byte) error {
	ch := make(chan error, 1)
	if err := c.enqueue(outItem{b: b, done: ch}); err != nil {
		return err
	}
	return <-ch
}

// flush waits until the frames queued so far are written, or timeout.
func (c *conn) flush(timeout time.Duration) {
	ch := make(chan error, 1)
	if c.enqueue(outItem{done: ch}) != nil {
		return
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-ch:
	case <-t.C:
	}
}

func (c *conn) enqueue(it outItem) error {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		if it.file != nil {
			it.file.Close()
		}
		return err
	}
	c.queue = append(c.queue, it)
	c.mu.Unlock()
	select {
	case c.signal <- struct{}{}:
	default:
	}
	return nil
}

// fail closes the connection with err (the first error wins) and fails
// the queued writes.
func (c *conn) fail(err error) {
	if err == nil {
		err = ErrClosed
	}
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return
	}
	c.err = err
	q := c.queue
	c.queue = nil
	close(c.done)
	c.mu.Unlock()
	c.t.Close()
	for _, it := range q {
		finish(it, err)
	}
}

// Err returns the error the connection failed with, or nil while it works.
func (c *conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func finish(it outItem, err error) {
	if it.file != nil {
		it.file.Close()
	}
	if it.done != nil {
		it.done <- err
	}
}

// writeLoop writes queued frames, batching consecutive frames without a
// descriptor into one write.
func (c *conn) writeLoop() {
	var batch []byte
	for {
		select {
		case <-c.signal:
		case <-c.done:
			return
		}
		c.mu.Lock()
		items := c.queue
		c.queue = nil
		c.mu.Unlock()
		for i := 0; i < len(items); {
			if items[i].file != nil {
				err := c.t.write(items[i].b, items[i].file)
				finish(items[i], err)
				if err != nil {
					c.failRest(items[i+1:], err)
					return
				}
				i++
				continue
			}
			j := i
			batch = batch[:0]
			for j < len(items) && items[j].file == nil {
				batch = append(batch, items[j].b...)
				j++
			}
			err := c.t.write(batch, nil)
			for _, it := range items[i:j] {
				finish(it, err)
			}
			if err != nil {
				c.failRest(items[j:], err)
				return
			}
			i = j
		}
		if cap(batch) > 1<<20 {
			batch = nil // do not pin a large payload's buffer
		}
	}
}

func (c *conn) failRest(rest []outItem, err error) {
	c.fail(fmt.Errorf("remote: write: %w", err))
	for _, it := range rest {
		finish(it, err)
	}
}

// DefaultAddr returns a fresh socket path for a host: in $XDG_RUNTIME_DIR
// when it is set (Linux), else in the temporary directory. The name is
// short, because AF_UNIX paths are limited to about 100 bytes.
func DefaultAddr() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, fmt.Sprintf("tela-%d-%08x.sock", os.Getpid(), randUint32()))
}

// dialUnix connects to a host socket.
func dialUnix(addr string, timeout time.Duration) (transport, error) {
	nc, err := net.DialTimeout("unix", addr, timeout)
	if err != nil {
		return nil, err
	}
	return newTransport(nc.(*net.UnixConn)), nil
}
