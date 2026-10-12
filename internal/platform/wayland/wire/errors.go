//go:build linux

package wire

import (
	"errors"
	"fmt"
)

// ErrWouldBlock is returned by [Conn.Flush] when the socket cannot take more
// data right now. Poll [Conn.Fd] for POLLOUT and call Flush again. It is not a
// failure: unsent data stays buffered.
var ErrWouldBlock = errors.New("wayland: socket would block")

// ProtocolError is the decoded wl_display.error event. It is fatal: the
// compositor disconnects the client after sending it, and every later call on
// the [Conn] returns the same error.
type ProtocolError struct {
	ObjectID  uint32 // object the error was raised on
	Interface string // interface name of that object, or "" when unknown
	Code      uint32 // interface-specific error code (see the interface's Error enum)
	Message   string // human-readable description from the compositor
}

func (e *ProtocolError) Error() string {
	obj := fmt.Sprintf("object %d", e.ObjectID)
	if e.Interface != "" {
		obj = fmt.Sprintf("%s#%d", e.Interface, e.ObjectID)
	}
	return fmt.Sprintf("wayland protocol error on %s (code %d): %s", obj, e.Code, e.Message)
}

// errDroppedProxy marks a request on a proxy whose creating request was
// dropped. Writer.Done discards it without reporting it: Flush reports the
// error that dropped the creating request.
var errDroppedProxy = errors.New("wayland: request on an object whose creation was dropped")

// errTruncated reports a message whose body is shorter than its signature.
var errTruncated = errors.New("wayland: malformed message: body shorter than its arguments")

// UnknownEvent is returned by generated Dispatch methods for an opcode the
// bound interface does not define.
func UnknownEvent(o Object, opcode uint16) error {
	return fmt.Errorf("wayland: %s#%d: unknown event opcode %d", o.Descriptor().Name, o.Base().ID(), opcode)
}
