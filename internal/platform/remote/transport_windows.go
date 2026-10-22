//go:build windows

package remote

import (
	"errors"
	"net"
	"os"
)

// winTransport is an AF_UNIX socket (Windows 10 1803 and later). Windows'
// AF_UNIX has no descriptor passing; shared memory travels by name
// instead (shm_windows.go).
type winTransport struct{ *net.UnixConn }

func newTransport(c *net.UnixConn) transport { return winTransport{c} }

func (t winTransport) write(b []byte, f *os.File) error {
	if f != nil {
		f.Close()
		return errors.New("remote: descriptor passing is not supported on Windows")
	}
	_, err := t.UnixConn.Write(b)
	return err
}

func (winTransport) takeFile() *os.File { return nil }
