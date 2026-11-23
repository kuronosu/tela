//go:build windows

package remote

import (
	"errors"
	"net"
	"os"
	"sync"
	"time"
)

// winTransport is an AF_UNIX socket (Windows 10 1803 and later). Windows'
// AF_UNIX has no descriptor passing; shared memory travels by name
// instead (shm_windows.go).
//
// Windows' AF_UNIX can lose the end of the stream: a receive pending
// when the peer shuts its write side down or closes sometimes never
// completes, although a receive started after it returns 0 (EOF) at once.
// It is the kernel's (afunix.sys): a blocking recv on its own thread,
// outside Go's poller, misses it too, about once in a thousand ends on
// Windows 11 26200, after shutdown(SD_SEND) or closesocket, with or
// without unread data on either side. The host then never saw a child's
// clean close (CloseWrite) end, the child's Close timed out and closed
// with the host's last frames unread, and that reset the host's reader ("wsarecv: An existing connection was forcibly
// closed by the remote host"). So a Read waits at most eofPoll per
// receive: at the deadline the receive is cancelled (CancelIoEx, which
// keeps data that arrived meanwhile: the receive then completes with it)
// and a new one finds an end that the old one missed. A caller's own
// deadline (SetReadDeadline, the handshake's) still ends the Read with
// os.ErrDeadlineExceeded.
type winTransport struct {
	*net.UnixConn

	mu       sync.Mutex // SetReadDeadline may run on any goroutine
	deadline time.Time  // the caller's read deadline; zero: none
}

// eofPoll bounds how long a receive waits before it is restarted, so an
// end of stream the kernel did not report is seen within eofPoll: well
// inside closeTimeout, which a clean close waits for the peer's EOF.
const eofPoll = 250 * time.Millisecond

func newTransport(c *net.UnixConn) transport { return &winTransport{UnixConn: c} }

func (t *winTransport) Read(p []byte) (int, error) {
	for {
		t.mu.Lock()
		t.armLocked()
		t.mu.Unlock()
		n, err := t.UnixConn.Read(p)
		if err == nil || !errors.Is(err, os.ErrDeadlineExceeded) {
			return n, err
		}
		if n > 0 {
			return n, nil
		}
		t.mu.Lock()
		d := t.deadline
		t.mu.Unlock()
		if !d.IsZero() && !time.Now().Before(d) {
			return 0, err
		}
	}
}

// SetReadDeadline sets the caller's deadline; a Read in progress follows
// it, or its next poll if that comes first.
func (t *winTransport) SetReadDeadline(d time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deadline = d
	return t.armLocked()
}

// armLocked sets the socket's deadline to the caller's or the next poll,
// whichever comes first.
func (t *winTransport) armLocked() error {
	d := time.Now().Add(eofPoll)
	if !t.deadline.IsZero() && t.deadline.Before(d) {
		d = t.deadline
	}
	return t.UnixConn.SetReadDeadline(d)
}

func (t *winTransport) write(b []byte, f *os.File) error {
	if f != nil {
		f.Close()
		return errors.New("remote: descriptor passing is not supported on Windows")
	}
	_, err := t.UnixConn.Write(b)
	return err
}

func (*winTransport) takeFile() *os.File { return nil }
