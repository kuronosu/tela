//go:build linux

package wire

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"
)

// Wire constants. See wayland-client / connection.c in libwayland.
const (
	// HeaderSize is the size of a message header: object ID, then
	// size<<16|opcode, both in native byte order.
	HeaderSize = 8
	// MaxMessageSize is the largest message libwayland accepts, header included.
	MaxMessageSize = 4096
	// MaxFdsPerMessage mirrors libwayland's MAX_FDS_OUT: the most file
	// descriptors that fit in one sendmsg.
	MaxFdsPerMessage = 28
	// DisplayID is the ID of the wl_display singleton.
	DisplayID = 1
	// ServerIDStart is the first object ID the server allocates.
	ServerIDStart = 0xFF000000
)

var nativeEndian = binary.NativeEndian

func pad4(n int) int { return (n + 3) &^ 3 }

// Reader decodes the arguments of one event. Methods never fail individually;
// the first problem is remembered and returned by [Reader.Err], so generated
// code can decode every argument and check once. After an error all methods
// return zero values.
type Reader struct {
	c   *Conn
	buf []byte
	off int
	err error
}

// Err returns the first decoding error, if any.
func (r *Reader) Err() error { return r.err }

func (r *Reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

func (r *Reader) word() (uint32, bool) {
	if r.err != nil {
		return 0, false
	}
	if len(r.buf)-r.off < 4 {
		r.fail(errTruncated)
		return 0, false
	}
	v := nativeEndian.Uint32(r.buf[r.off:])
	r.off += 4
	return v, true
}

// Int reads an int argument.
func (r *Reader) Int() int32 { v, _ := r.word(); return int32(v) }

// Uint reads a uint argument.
func (r *Reader) Uint() uint32 { v, _ := r.word(); return v }

// Fixed reads a fixed argument.
func (r *Reader) Fixed() Fixed { v, _ := r.word(); return Fixed(int32(v)) }

// bytes reads a length-prefixed, padded blob. null reports a zero length.
func (r *Reader) bytes() (data []byte, null bool) {
	n, ok := r.word()
	if !ok {
		return nil, false
	}
	if n == 0 {
		return nil, true
	}
	padded := pad4(int(n))
	if int(n) < 0 || padded < 0 || len(r.buf)-r.off < padded {
		r.fail(errTruncated)
		return nil, false
	}
	data = r.buf[r.off : r.off+int(n)]
	r.off += padded
	return data, false
}

// String reads a string argument. A null string reads as "".
func (r *Reader) String() string {
	s := r.StringPtr()
	if s == nil {
		return ""
	}
	return *s
}

// StringPtr reads a nullable string argument: nil for null, a pointer to
// the (possibly empty) string otherwise.
func (r *Reader) StringPtr() *string {
	data, null := r.bytes()
	if r.err != nil || null {
		return nil
	}
	if data[len(data)-1] != 0 {
		r.fail(fmt.Errorf("wayland: malformed message: string is not NUL-terminated"))
		return nil
	}
	s := string(data[:len(data)-1])
	return &s
}

// Array reads an array argument. The result is a copy that the caller owns.
// An empty array reads as nil.
func (r *Reader) Array() []byte {
	data, _ := r.bytes()
	if len(data) == 0 {
		return nil
	}
	return append([]byte(nil), data...)
}

// Uint32s decodes an array argument that holds native-endian uint32 values,
// such as the states of xdg_toplevel.configure. Trailing bytes that do not
// make a whole value are ignored.
func Uint32s(array []byte) []uint32 {
	out := make([]uint32, len(array)/4)
	for i := range out {
		out[i] = nativeEndian.Uint32(array[4*i:])
	}
	return out
}

// Object reads an object argument and resolves it to the live proxy. It
// returns nil for a null object and for objects that are unknown or already
// destroyed on this side.
func (r *Reader) Object() Object {
	id, ok := r.word()
	if !ok || id == 0 {
		return nil
	}
	if s := r.c.objects[id]; s != nil {
		return s.obj
	}
	return nil
}

// NewID reads the ID of a server-created object without binding anything.
func (r *Reader) NewID() uint32 { v, _ := r.word(); return v }

// NewObject reads a new_id argument of an event and binds obj to it with
// the given version (events pass the version of the object they arrived on).
func (r *Reader) NewObject(obj Object, version uint32) {
	id, ok := r.word()
	if !ok {
		return
	}
	if id < ServerIDStart {
		r.fail(fmt.Errorf("wayland: server created object with id %d, below the server range %#x", id, ServerIDStart))
		return
	}
	if s := r.c.objects[id]; s != nil && s.obj != nil {
		r.fail(fmt.Errorf("wayland: server reused live object id %d (%s)", id, s.obj.Base()))
		return
	}
	r.c.adopt(obj, id, version)
}

// Fd takes the next received file descriptor. The caller owns it and must
// close it (use [CloseFd]). It returns -1 and records an error when the
// compositor sent no descriptor.
func (r *Reader) Fd() int {
	if r.err != nil {
		return -1
	}
	fd, ok := r.c.popFd()
	if !ok {
		r.fail(fmt.Errorf("wayland: malformed message: event carries a file descriptor argument but none was received"))
		return -1
	}
	return fd
}

// CloseFd closes a file descriptor, ignoring errors and negative values. It
// is for generated code that has to drop an event's descriptor.
func CloseFd(fd int) {
	if fd >= 0 {
		_ = unix.Close(fd)
	}
}

// Writer encodes one request into the connection's output buffer. Obtain it
// from [Proxy.Begin], append the arguments in protocol order and call
// [Writer.Done]. Only one Writer is active at a time.
type Writer struct {
	c      *Conn
	start  int // offset of the header in c.out
	opcode uint16
	fds    []int // duplicated descriptors owned by the message
	err    error
}

func (w *Writer) word(v uint32) { w.c.out = nativeEndian.AppendUint32(w.c.out, v) }

func (w *Writer) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

// Int writes an int argument.
func (w *Writer) Int(v int32) { w.word(uint32(v)) }

// Uint writes a uint argument.
func (w *Writer) Uint(v uint32) { w.word(v) }

// Fixed writes a fixed argument.
func (w *Writer) Fixed(v Fixed) { w.word(uint32(int32(v))) }

// Object writes an object argument; id 0 is the null object.
func (w *Writer) Object(id uint32) { w.word(id) }

func (w *Writer) bytes(data []byte, nul bool) {
	n := len(data)
	if nul {
		n++
	}
	w.word(uint32(n))
	w.c.out = append(w.c.out, data...)
	if nul {
		w.c.out = append(w.c.out, 0)
	}
	for i := n; i < pad4(n); i++ {
		w.c.out = append(w.c.out, 0)
	}
}

// String writes a non-null string argument. The empty string is encoded as a
// single NUL byte, which differs from null.
func (w *Writer) String(s string) {
	w.word(uint32(len(s) + 1))
	w.c.out = append(w.c.out, s...)
	w.c.out = append(w.c.out, 0)
	for i := len(s) + 1; i < pad4(len(s)+1); i++ {
		w.c.out = append(w.c.out, 0)
	}
}

// StringPtr writes a nullable string argument: nil is null.
func (w *Writer) StringPtr(s *string) {
	if s == nil {
		w.word(0)
		return
	}
	w.String(*s)
}

// Array writes an array argument. A nil or empty slice is a zero-length array.
func (w *Writer) Array(data []byte) { w.bytes(data, false) }

// NewObject writes a typed new_id argument: it allocates an ID, binds obj to
// it with the given version and writes the ID. Requests that create an
// object pass the version of the parent.
func (w *Writer) NewObject(obj Object, version uint32) {
	id := w.c.allocID()
	w.c.adopt(obj, id, version)
	w.word(id)
}

// NewObjectUntyped writes an untyped new_id argument (wl_registry.bind): the
// interface name, the version and the new ID.
func (w *Writer) NewObjectUntyped(obj Object, version uint32) {
	iface := obj.Descriptor()
	if version > iface.Version {
		panic(fmt.Sprintf("wayland: cannot bind %s at version %d: these bindings only know up to version %d; "+
			"bind min(advertised, %d)", iface.Name, version, iface.Version, iface.Version))
	}
	w.String(iface.Name)
	w.word(version)
	id := w.c.allocID()
	w.c.adopt(obj, id, version)
	w.word(id)
}

// Fd attaches a file descriptor to the message. The runtime duplicates it, so
// the caller keeps ownership of fd and may close it right after the call.
func (w *Writer) Fd(fd int) {
	if len(w.fds) >= MaxFdsPerMessage {
		w.fail(fmt.Errorf("wayland: a message cannot carry more than %d file descriptors", MaxFdsPerMessage))
		return
	}
	dup, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		w.fail(fmt.Errorf("wayland: cannot duplicate file descriptor %d: %w", fd, err))
		return
	}
	w.fds = append(w.fds, dup)
}

// Done finishes the message: it patches the header and queues the message for
// the next [Conn.Flush]. If encoding failed (message too large, bad
// descriptor), the message is dropped and the error is reported by Flush.
func (w *Writer) Done() {
	c := w.c
	size := len(c.out) - w.start
	switch {
	case w.err == nil && size > MaxMessageSize:
		w.fail(fmt.Errorf("wayland: request of %d bytes exceeds the %d byte protocol limit", size, MaxMessageSize))
	}
	if w.err != nil || c.err != nil {
		if w.err != nil && c.sendErr == nil {
			c.sendErr = w.err
		}
		c.out = c.out[:w.start]
		for _, fd := range w.fds {
			CloseFd(fd)
		}
		w.fds = nil
		return
	}
	nativeEndian.PutUint32(c.out[w.start+4:], uint32(size)<<16|uint32(w.opcode))
	if len(w.fds) > 0 {
		c.outFds = append(c.outFds, fdMsg{start: w.start, fds: w.fds})
		w.fds = nil
	}
}
