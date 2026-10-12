//go:build linux

package wire

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

// fdMsg is a queued message that carries file descriptors.
type fdMsg struct {
	start int   // offset of the message header in Conn.out
	fds   []int // duplicates owned by the connection until sent
}

// slot is one entry of the object map. A live object has obj set. A zombie
// (destroyed client-side, ID not yet released) has obj == nil and remembers
// the interface so events still in flight can be skipped, including their
// file descriptors.
type slot struct {
	obj    Object
	zombie *Interface
}

// Conn is a client connection to a Wayland compositor. It is not safe for
// concurrent use: all calls must come from the thread that owns the
// connection (the UI thread).
type Conn struct {
	fd int

	// err is the sticky fatal error: after it is set no more events are
	// dispatched and nothing is sent. eof is set when the peer closed.
	err     error
	sendErr error
	eof     bool

	objects map[uint32]*slot
	nextID  uint32
	freeIDs []uint32

	out    []byte
	outOff int // bytes of out already sent
	outFds []fdMsg
	writer Writer

	in     []byte
	inOff  int // bytes of in already dispatched
	inFds  []int
	oob    []byte
	reader Reader

	dispatching bool
}

const (
	minRead   = 4096
	initialIn = 16 * 1024
)

// Dial connects to the compositor the environment points at, following
// libwayland: if WAYLAND_SOCKET is set it is an already connected socket
// inherited from the parent; otherwise the socket is $WAYLAND_DISPLAY (default
// "wayland-0") inside $XDG_RUNTIME_DIR, or WAYLAND_DISPLAY itself when that is
// an absolute path.
func Dial() (*Conn, error) {
	if s := os.Getenv("WAYLAND_SOCKET"); s != "" {
		fd, err := strconv.Atoi(s)
		if err != nil || fd < 0 {
			return nil, fmt.Errorf("wayland: WAYLAND_SOCKET=%q is not a file descriptor number", s)
		}
		unix.CloseOnExec(fd)
		// Like libwayland, do not leak the variable to child processes.
		_ = os.Unsetenv("WAYLAND_SOCKET")
		return NewConn(fd)
	}
	display := os.Getenv("WAYLAND_DISPLAY")
	if display == "" {
		display = "wayland-0"
	}
	path := display
	if !filepath.IsAbs(display) {
		dir := os.Getenv("XDG_RUNTIME_DIR")
		if dir == "" {
			return nil, errors.New("wayland: XDG_RUNTIME_DIR is not set, so the compositor socket cannot be located; " +
				"run inside a Wayland session, or set WAYLAND_DISPLAY to an absolute socket path")
		}
		path = filepath.Join(dir, display)
	}
	return DialPath(path)
}

// DialPath connects to the compositor socket at path.
func DialPath(path string) (*Conn, error) {
	if len(path) >= 108 {
		return nil, fmt.Errorf("wayland: socket path %q is too long for a unix socket (limit 107 bytes); "+
			"point WAYLAND_DISPLAY at a shorter path", path)
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("wayland: cannot create socket: %w", err)
	}
	for {
		err = unix.Connect(fd, &unix.SockaddrUnix{Name: path})
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("wayland: cannot connect to %s: %w (is a Wayland compositor running, and is WAYLAND_DISPLAY correct?)", path, err)
	}
	return NewConn(fd)
}

// NewConn wraps an already connected stream socket. The Conn takes ownership
// of fd and switches it to non-blocking, close-on-exec mode.
func NewConn(fd int) (*Conn, error) {
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("wayland: cannot make socket non-blocking: %w", err)
	}
	unix.CloseOnExec(fd)
	c := &Conn{
		fd:      fd,
		objects: map[uint32]*slot{},
		nextID:  DisplayID + 1,
		in:      make([]byte, 0, initialIn),
		oob:     make([]byte, unix.CmsgSpace(4*64)),
	}
	c.writer.c = c
	c.reader.c = c
	return c, nil
}

// Fd returns the socket descriptor, for the backend's poll loop.
func (c *Conn) Fd() int { return c.fd }

// Err returns the fatal error that killed the connection, or nil.
func (c *Conn) Err() error { return c.err }

// Close closes the socket and any descriptors still queued. It is safe to
// call more than once.
func (c *Conn) Close() error {
	if c.fd < 0 {
		return nil
	}
	err := unix.Close(c.fd)
	c.fd = -1
	for _, m := range c.outFds {
		for _, fd := range m.fds {
			CloseFd(fd)
		}
	}
	c.outFds = nil
	for _, fd := range c.inFds {
		CloseFd(fd)
	}
	c.inFds = nil
	if c.err == nil {
		c.err = errors.New("wayland: connection closed")
	}
	return err
}

// SetDisplay binds obj as the wl_display singleton (object ID 1). The
// generated protocols package wraps this as NewWlDisplay. The runtime handles
// wl_display.error and wl_display.delete_id itself, so obj never receives
// events.
func (c *Conn) SetDisplay(obj Object) {
	c.adopt(obj, DisplayID, 1)
}

// Object returns the live object with the given ID, or nil.
func (c *Conn) Object(id uint32) Object {
	if s := c.objects[id]; s != nil {
		return s.obj
	}
	return nil
}

// ---------------------------------------------------------------------------
// Object ID map

func (c *Conn) allocID() uint32 {
	if n := len(c.freeIDs); n > 0 {
		id := c.freeIDs[n-1]
		c.freeIDs = c.freeIDs[:n-1]
		return id
	}
	if c.nextID >= ServerIDStart {
		panic("wayland: client object ID space exhausted")
	}
	id := c.nextID
	c.nextID++
	return id
}

func (c *Conn) adopt(obj Object, id, version uint32) {
	p := obj.Base()
	if p.conn != nil {
		panic(fmt.Sprintf("wayland: object %s is already bound to a connection", p))
	}
	p.conn = c
	p.id = id
	p.version = version
	p.iface = obj.Descriptor()
	c.objects[id] = &slot{obj: obj}
}

// destroyProxy turns a live object into a zombie. Client-created IDs are
// released when wl_display.delete_id arrives. Server-created IDs get no
// delete_id; the zombie stays until the server reuses the ID.
func (c *Conn) destroyProxy(p *Proxy) {
	p.destroyed = true
	if s := c.objects[p.id]; s != nil && s.obj != nil && s.obj.Base() == p {
		s.obj = nil
		s.zombie = p.iface
	}
}

// dropProxy undoes the adoption of an object whose creating request was
// dropped (see Writer.Done): the server never learned its ID, so the ID is
// unregistered and free for reuse at once (no delete_id will come), and the
// proxy is marked destroyed and dropped, so its requests are dropped too.
func (c *Conn) dropProxy(p *Proxy) {
	if s := c.objects[p.id]; s != nil && s.obj != nil && s.obj.Base() == p {
		delete(c.objects, p.id)
		if p.id < ServerIDStart && p.id != DisplayID {
			c.freeIDs = append(c.freeIDs, p.id)
		}
	}
	p.destroyed = true
	p.dropped = true
}

func (c *Conn) handleDeleteID(id uint32) {
	s := c.objects[id]
	if s == nil {
		return
	}
	if s.obj != nil {
		// Destroyed by the server (for example a wl_callback after done).
		p := s.obj.Base()
		p.destroyed = true
	}
	delete(c.objects, id)
	if id < ServerIDStart && id != DisplayID {
		c.freeIDs = append(c.freeIDs, id)
	}
}

func (c *Conn) popFd() (int, bool) {
	if len(c.inFds) == 0 {
		return -1, false
	}
	fd := c.inFds[0]
	c.inFds = c.inFds[1:]
	return fd, true
}

// ---------------------------------------------------------------------------
// Sending

func (c *Conn) begin(id uint32, opcode uint16) *Writer {
	w := &c.writer
	w.start = len(c.out)
	w.opcode = opcode
	w.fds = nil
	w.err = nil
	clear(w.created)
	w.created = w.created[:0]
	c.out = nativeEndian.AppendUint32(c.out, id)
	c.out = nativeEndian.AppendUint32(c.out, 0) // size and opcode, patched by Done
	return w
}

// HasPendingWrites reports whether buffered requests are waiting to be sent.
// While it is true the backend should also poll the socket for POLLOUT.
func (c *Conn) HasPendingWrites() bool { return c.outOff < len(c.out) }

// Flush sends buffered requests with as few sendmsg calls as possible. File
// descriptors travel with the bytes of their message, at most
// [MaxFdsPerMessage] per call. The socket is non-blocking: if it fills up
// Flush returns [ErrWouldBlock] and keeps the rest buffered; wait for POLLOUT
// and call it again. Any other error is fatal.
//
// If the compositor hung up, Flush reports it, but a protocol error it sent
// first may still be waiting in the socket: on a non-nil, non-ErrWouldBlock
// error call Read and Dispatch to retrieve it.
func (c *Conn) Flush() error {
	if c.err != nil {
		return c.err
	}
	if c.sendErr != nil {
		return c.fail(c.sendErr)
	}
	for c.outOff < len(c.out) {
		limit := len(c.out)
		var fds []int
		nmsgs := 0
		for _, m := range c.outFds {
			if len(fds)+len(m.fds) > MaxFdsPerMessage {
				// Keep this message's bytes for the next sendmsg so its
				// descriptors can never arrive after the message.
				limit = m.start
				break
			}
			fds = append(fds, m.fds...)
			nmsgs++
		}
		if limit <= c.outOff {
			return c.fail(errors.New("wayland: internal error: no sendable data in the output queue"))
		}
		var oob []byte
		if len(fds) > 0 {
			oob = unix.UnixRights(fds...)
		}
		n, err := unix.SendmsgN(c.fd, c.out[c.outOff:limit], oob, nil, unix.MSG_NOSIGNAL)
		switch {
		case err == unix.EINTR:
			continue
		case err == unix.EAGAIN:
			return ErrWouldBlock
		case err == unix.EPIPE || err == unix.ECONNRESET:
			// Not sticky on purpose: the compositor usually sends a
			// wl_display.error before it hangs up, and Read/Dispatch must
			// still be able to deliver it.
			return fmt.Errorf("wayland: the compositor closed the connection: %w", err)
		case err != nil:
			return c.fail(fmt.Errorf("wayland: send failed: %w", err))
		}
		// The ancillary data travels with the first byte of the call, so
		// the descriptors are delivered even if the write was short.
		c.outOff += n
		if n > 0 {
			for _, m := range c.outFds[:nmsgs] {
				for _, fd := range m.fds {
					CloseFd(fd)
				}
			}
			c.outFds = c.outFds[nmsgs:]
		}
	}
	c.out = c.out[:0]
	c.outOff = 0
	c.outFds = nil
	return nil
}

// ---------------------------------------------------------------------------
// Receiving

// Read does one non-blocking read of whatever the compositor has sent
// (bytes and file descriptors) into the input buffer and returns the number of
// bytes read; 0 with a nil error means nothing was available. It does not
// dispatch. When the compositor closes the connection it returns an error
// wrapping [io.EOF]; call [Conn.Dispatch] once more to drain complete
// messages that were already buffered.
func (c *Conn) Read() (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	if c.eof {
		return 0, errEOF
	}
	if c.dispatching {
		return 0, errors.New("wayland: Read called from inside an event handler")
	}
	// Compact consumed bytes, then make sure a read can make progress.
	if c.inOff > 0 {
		n := copy(c.in, c.in[c.inOff:])
		c.in = c.in[:n]
		c.inOff = 0
	}
	if cap(c.in)-len(c.in) < minRead {
		grown := make([]byte, len(c.in), 2*cap(c.in)+minRead)
		copy(grown, c.in)
		c.in = grown
	}
	buf := c.in[len(c.in):cap(c.in)]
	for {
		n, oobn, flags, _, err := unix.Recvmsg(c.fd, buf, c.oob, unix.MSG_CMSG_CLOEXEC)
		if err == unix.EINTR {
			continue
		}
		if err == unix.EAGAIN {
			return 0, nil
		}
		if err != nil {
			return 0, c.fail(fmt.Errorf("wayland: receive failed: %w", err))
		}
		if oobn > 0 {
			if err := c.takeFds(c.oob[:oobn]); err != nil {
				return 0, c.fail(err)
			}
		}
		if flags&unix.MSG_CTRUNC != 0 {
			return 0, c.fail(errors.New("wayland: ancillary data truncated: the compositor sent more file descriptors than fit in one read"))
		}
		if n == 0 && oobn == 0 {
			c.eof = true
			return 0, errEOF
		}
		c.in = c.in[:len(c.in)+n]
		return n, nil
	}
}

var errEOF = fmt.Errorf("wayland: connection closed by the compositor: %w", io.EOF)

func (c *Conn) takeFds(oob []byte) error {
	msgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return fmt.Errorf("wayland: malformed ancillary data: %w", err)
	}
	for i := range msgs {
		fds, err := unix.ParseUnixRights(&msgs[i])
		if err != nil {
			continue // not SCM_RIGHTS
		}
		c.inFds = append(c.inFds, fds...)
	}
	return nil
}

// fail records a fatal error. The first one wins.
func (c *Conn) fail(err error) error {
	if c.err == nil {
		c.err = err
	}
	return c.err
}

// Dispatch decodes every complete message in the input buffer and calls the
// handlers of the objects they are addressed to. A partial message stays
// buffered until more bytes arrive. It returns the number of messages
// processed.
//
// A wl_display.error event, a malformed message or an error returned by a
// handler's decoder is fatal: Dispatch returns it (a *[ProtocolError] for
// wl_display.error) and the connection stays dead.
//
// Handlers may send requests and create or destroy objects, but must not call
// Read or Dispatch.
func (c *Conn) Dispatch() (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	if c.dispatching {
		return 0, errors.New("wayland: Dispatch called from inside an event handler")
	}
	c.dispatching = true
	defer func() { c.dispatching = false }()

	count := 0
	for {
		avail := c.in[c.inOff:]
		if len(avail) < HeaderSize {
			break
		}
		id := nativeEndian.Uint32(avail)
		word := nativeEndian.Uint32(avail[4:])
		size := int(word >> 16)
		opcode := uint16(word)
		if size < HeaderSize || size%4 != 0 {
			return count, c.fail(fmt.Errorf("wayland: malformed message for object %d: invalid size %d", id, size))
		}
		if len(avail) < size {
			break
		}
		body := avail[HeaderSize:size]
		c.inOff += size
		count++
		if err := c.dispatchOne(id, opcode, body); err != nil {
			return count, c.fail(err)
		}
	}
	if c.inOff == len(c.in) {
		c.in = c.in[:0]
		c.inOff = 0
	}
	return count, nil
}

func (c *Conn) dispatchOne(id uint32, opcode uint16, body []byte) error {
	if id == DisplayID {
		return c.dispatchDisplay(opcode, body)
	}
	s := c.objects[id]
	if s == nil {
		return fmt.Errorf("wayland: event for unknown object %d (opcode %d)", id, opcode)
	}
	if s.obj == nil {
		return c.skipEvent(s.zombie, opcode, body)
	}
	r := &c.reader
	r.buf, r.off, r.err = body, 0, nil
	obj := s.obj
	if err := obj.Dispatch(opcode, r); err != nil {
		return err
	}
	if r.err != nil {
		return fmt.Errorf("wayland: %s#%d event %d: %w", obj.Descriptor().Name, id, opcode, r.err)
	}
	return nil
}

// dispatchDisplay handles the two wl_display events natively.
func (c *Conn) dispatchDisplay(opcode uint16, body []byte) error {
	r := &c.reader
	r.buf, r.off, r.err = body, 0, nil
	switch opcode {
	case 0: // error(object_id, code, message)
		objID := r.Uint()
		code := r.Uint()
		msg := r.String()
		if r.err != nil {
			return fmt.Errorf("wayland: malformed wl_display.error: %w", r.err)
		}
		pe := &ProtocolError{ObjectID: objID, Code: code, Message: msg}
		if s := c.objects[objID]; s != nil {
			if s.obj != nil {
				pe.Interface = s.obj.Descriptor().Name
			} else if s.zombie != nil {
				pe.Interface = s.zombie.Name
			}
		} else if objID == DisplayID {
			pe.Interface = "wl_display"
		}
		return pe
	case 1: // delete_id(id)
		id := r.Uint()
		if r.err != nil {
			return fmt.Errorf("wayland: malformed wl_display.delete_id: %w", r.err)
		}
		c.handleDeleteID(id)
		return nil
	}
	return fmt.Errorf("wayland: wl_display: unknown event opcode %d", opcode)
}

// skipEvent drops an event addressed to an object destroyed client-side. It
// walks the event signature to close its file descriptors, and it reserves
// the IDs of objects the event would have created so their own events are
// skipped the same way.
func (c *Conn) skipEvent(iface *Interface, opcode uint16, body []byte) error {
	if iface == nil || int(opcode) >= len(iface.Events) {
		return nil
	}
	off := 0
	need := func(n int) error {
		if len(body)-off < n {
			return errTruncated
		}
		return nil
	}
	for _, a := range iface.Events[opcode].Args {
		switch a.Type {
		case ArgInt, ArgUint, ArgFixed, ArgObject:
			if err := need(4); err != nil {
				return err
			}
			off += 4
		case ArgNewID:
			if err := need(4); err != nil {
				return err
			}
			id := nativeEndian.Uint32(body[off:])
			off += 4
			if a.Interface != "" && id >= ServerIDStart {
				c.objects[id] = &slot{zombie: Lookup(a.Interface)}
			}
		case ArgString, ArgArray:
			if err := need(4); err != nil {
				return err
			}
			n := int(nativeEndian.Uint32(body[off:]))
			off += 4
			if err := need(pad4(n)); err != nil {
				return err
			}
			off += pad4(n)
		case ArgFd:
			if fd, ok := c.popFd(); ok {
				CloseFd(fd)
			}
		}
	}
	return nil
}
