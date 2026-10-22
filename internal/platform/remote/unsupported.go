//go:build !linux && !darwin && !windows

package remote

import (
	"fmt"
	"net"
	"os"

	"github.com/kuronosu/tela/internal/platform"
)

// Other systems have no shared memory implementation: Dial and NewHost
// fail with an error wrapping platform.ErrUnavailable.

var errNoShm = fmt.Errorf("remote: shared memory is not implemented on this system: %w", platform.ErrUnavailable)

type noTransport struct{ *net.UnixConn }

func newTransport(c *net.UnixConn) transport         { return noTransport{c} }
func (noTransport) write(b []byte, f *os.File) error { return errNoShm }
func (noTransport) takeFile() *os.File               { return nil }

func createSegment(size int) (*segment, *os.File, string, error) { return nil, nil, "", errNoShm }

func openSegment(f *os.File, name string, size int) (*segment, error) { return nil, errNoShm }

const supported = false
