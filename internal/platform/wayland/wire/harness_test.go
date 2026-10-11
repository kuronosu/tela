//go:build linux

package wire

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// testIface is a hand-written interface that exercises every argument type.
var testIface = Interface{
	Name:    "test_obj",
	Version: 3,
	Requests: []Message{
		{Name: "ints"},
		{Name: "strings"},
		{Name: "object"},
		{Name: "array"},
		{Name: "fd"},
		{Name: "create", Args: []Arg{{Type: ArgNewID, Interface: "test_obj"}}},
		{Name: "destroy", Destructor: true},
		{Name: "newer", Since: 2},
		{Name: "bind"},
	},
	Events: []Message{
		{Name: "all", Args: []Arg{
			{Type: ArgInt}, {Type: ArgUint}, {Type: ArgFixed}, {Type: ArgString},
			{Type: ArgString, Nullable: true}, {Type: ArgString, Nullable: true},
			{Type: ArgObject, Nullable: true, Interface: "test_obj"}, {Type: ArgArray},
		}},
		{Name: "fd", Args: []Arg{{Type: ArgFd}}},
		{Name: "child", Args: []Arg{{Type: ArgNewID, Interface: "test_obj"}}},
		{Name: "ping", Args: []Arg{{Type: ArgUint}}},
		{Name: "fdchild", Args: []Arg{{Type: ArgFd}, {Type: ArgNewID, Interface: "test_obj"}}},
	},
}

func init() { Register(&testIface) }

// Request opcodes of testIface.
const (
	reqInts = iota
	reqStrings
	reqObject
	reqArray
	reqFd
	reqCreate
	reqDestroy
	reqNewer
)

// Event opcodes of testIface.
const (
	evAll = iota
	evFd
	evChild
	evPing
	evFdChild
)

// tobj is a test proxy whose events go to a closure.
type tobj struct {
	Proxy
	handle func(opcode uint16, r *Reader) error
}

func (o *tobj) Descriptor() *Interface { return &testIface }
func (o *tobj) Dispatch(opcode uint16, r *Reader) error {
	if o.handle == nil {
		return nil
	}
	return o.handle(opcode, r)
}

// create sends test_obj.create on o and returns the new child.
func (o *tobj) create() *tobj {
	child := &tobj{}
	w := o.Begin(reqCreate)
	w.NewObject(child, o.Version())
	w.Done()
	return child
}

func (o *tobj) destroy() {
	o.Begin(reqDestroy).Done()
	o.MarkDestroyed()
}

// rawMsg is a request as seen by the fake server.
type rawMsg struct {
	id     uint32
	opcode uint16
	body   []byte
}

// server is a fake compositor on the other end of a socketpair.
type server struct {
	t   *testing.T
	fd  int
	in  []byte
	fds []int
}

// newPair returns a client Conn and the fake server for it.
func newPair(t *testing.T) (*Conn, *server) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewConn(fds[0])
	if err != nil {
		t.Fatal(err)
	}
	tv := unix.NsecToTimeval(int64(3 * time.Second))
	if err := unix.SetsockoptTimeval(fds[1], unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
		t.Fatal(err)
	}
	s := &server{t: t, fd: fds[1]}
	t.Cleanup(func() {
		c.Close()
		unix.Close(s.fd)
		for _, fd := range s.fds {
			unix.Close(fd)
		}
	})
	return c, s
}

// newTestObj binds a tobj as object id 2 without sending anything, as if it
// had been created by an earlier request.
func newTestObj(c *Conn) *tobj {
	o := &tobj{}
	id := c.allocID()
	c.adopt(o, id, testIface.Version)
	return o
}

// fill performs one blocking recvmsg.
func (s *server) fill() {
	s.t.Helper()
	buf := make([]byte, 64*1024)
	oob := make([]byte, unix.CmsgSpace(4*64))
	n, oobn, _, _, err := unix.Recvmsg(s.fd, buf, oob, unix.MSG_CMSG_CLOEXEC)
	if err != nil {
		s.t.Fatalf("server recvmsg: %v", err)
	}
	s.in = append(s.in, buf[:n]...)
	if oobn > 0 {
		msgs, err := unix.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			s.t.Fatal(err)
		}
		for i := range msgs {
			fds, err := unix.ParseUnixRights(&msgs[i])
			if err == nil {
				s.fds = append(s.fds, fds...)
			}
		}
	}
}

// next returns the next complete request, reading more if needed.
func (s *server) next() rawMsg {
	s.t.Helper()
	for {
		if len(s.in) >= 8 {
			size := int(nativeEndian.Uint32(s.in[4:]) >> 16)
			if len(s.in) >= size {
				m := rawMsg{
					id:     nativeEndian.Uint32(s.in),
					opcode: uint16(nativeEndian.Uint32(s.in[4:])),
					body:   append([]byte(nil), s.in[8:size]...),
				}
				s.in = s.in[size:]
				return m
			}
		}
		s.fill()
	}
}

// send writes raw bytes with optional descriptors in one sendmsg.
func (s *server) send(data []byte, fds ...int) {
	s.t.Helper()
	var oob []byte
	if len(fds) > 0 {
		oob = unix.UnixRights(fds...)
	}
	if _, err := unix.SendmsgN(s.fd, data, oob, nil, 0); err != nil {
		s.t.Fatalf("server sendmsg: %v", err)
	}
}

// msg builds a wire message by hand.
type msg struct{ b []byte }

func newMsg(id uint32, opcode uint16) *msg {
	m := &msg{}
	m.u32(id)
	m.u32(uint32(opcode)) // size patched in bytes()
	return m
}

func (m *msg) u32(v uint32) *msg { m.b = nativeEndian.AppendUint32(m.b, v); return m }
func (m *msg) i32(v int32) *msg  { return m.u32(uint32(v)) }

func (m *msg) blob(data []byte, nul bool) *msg {
	n := len(data)
	if nul {
		n++
	}
	m.u32(uint32(n))
	m.b = append(m.b, data...)
	if nul {
		m.b = append(m.b, 0)
	}
	for len(m.b)%4 != 0 {
		m.b = append(m.b, 0)
	}
	return m
}
func (m *msg) str(s string) *msg { return m.blob([]byte(s), true) }
func (m *msg) arr(a []byte) *msg { return m.blob(a, false) }
func (m *msg) null() *msg        { return m.u32(0) }
func (m *msg) bytes() []byte {
	out := append([]byte(nil), m.b...)
	op := nativeEndian.Uint32(out[4:])
	nativeEndian.PutUint32(out[4:], uint32(len(out))<<16|op&0xffff)
	return out
}

// openFds counts this process's open descriptors.
func openFds(t *testing.T) int {
	t.Helper()
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("cannot list /proc/self/fd: %v", err)
	}
	return len(ents)
}

// memfd returns a memfd holding data.
func memfd(t *testing.T, data string) int {
	t.Helper()
	fd, err := unix.MemfdCreate("wire-test", unix.MFD_CLOEXEC)
	if err != nil {
		t.Skipf("memfd_create unavailable: %v", err)
	}
	if _, err := unix.Write(fd, []byte(data)); err != nil {
		t.Fatal(err)
	}
	return fd
}

// readFd reads the whole content of fd from offset 0.
func readFd(t *testing.T, fd int) string {
	t.Helper()
	buf := make([]byte, 4096)
	n, err := unix.Pread(fd, buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	return string(buf[:n])
}

// pump reads and dispatches until want messages were processed.
func pump(t *testing.T, c *Conn, want int) {
	t.Helper()
	total := 0
	deadline := time.Now().Add(3 * time.Second)
	for total < want {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: dispatched %d of %d messages", total, want)
		}
		if err := c.wait(unix.POLLIN, deadline); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Read(); err != nil {
			t.Fatalf("Read: %v", err)
		}
		n, err := c.Dispatch()
		if err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		total += n
	}
}
