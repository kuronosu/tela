//go:build linux || darwin

package remote

import (
	"fmt"
	"net"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// unixTransport is a Unix socket that passes descriptors with SCM_RIGHTS.
// The kernel attaches a descriptor to the first byte of the sendmsg that
// carried it and a recvmsg never returns bytes from two such messages, so
// the read that returns a message's first byte also returns its
// descriptor: by the time the message is decoded, its file is queued.
// net's ReadMsgUnix sets close-on-exec on received descriptors.
type unixTransport struct {
	*net.UnixConn
	oob []byte

	mu    sync.Mutex // Close runs on any goroutine, Read on the reader's
	files []*os.File
}

func newTransport(c *net.UnixConn) transport {
	return &unixTransport{UnixConn: c, oob: make([]byte, unix.CmsgSpace(4*4))}
}

func (t *unixTransport) Read(p []byte) (int, error) {
	n, oobn, flags, _, err := t.ReadMsgUnix(p, t.oob)
	if oobn > 0 {
		if perr := t.parseRights(t.oob[:oobn]); perr != nil && err == nil {
			err = perr
		}
	}
	if flags&unix.MSG_CTRUNC != 0 && err == nil {
		err = fmt.Errorf("remote: control message truncated (descriptors lost)")
	}
	return n, err
}

func (t *unixTransport) parseRights(oob []byte) error {
	msgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return fmt.Errorf("remote: control message: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range msgs {
		fds, err := unix.ParseUnixRights(&msgs[i])
		if err != nil {
			continue
		}
		for _, fd := range fds {
			t.files = append(t.files, os.NewFile(uintptr(fd), "tela-remote-shm"))
		}
	}
	return nil
}

func (t *unixTransport) write(b []byte, f *os.File) error {
	if f == nil {
		_, err := t.UnixConn.Write(b)
		return err
	}
	n, _, err := t.WriteMsgUnix(b, unix.UnixRights(int(f.Fd())), nil)
	if err != nil {
		return err
	}
	if n < len(b) {
		_, err = t.UnixConn.Write(b[n:])
	}
	return err
}

func (t *unixTransport) takeFile() *os.File {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.files) == 0 {
		return nil
	}
	f := t.files[0]
	t.files = t.files[1:]
	return f
}

func (t *unixTransport) Close() error {
	t.mu.Lock()
	for _, f := range t.files {
		f.Close()
	}
	t.files = nil
	t.mu.Unlock()
	return t.UnixConn.Close()
}
