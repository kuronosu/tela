// Tests of Tela's patch that passes file descriptors (see fd.go).

//go:build unix

package xgb_test

import (
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
)

// unixServer is fakeServer over a pair of unix sockets, the transport
// that carries file descriptors: it accepts the connection setup and
// returns the server's end.
func unixServer(t *testing.T) (*xgb.Conn, *net.UnixConn) {
	t.Helper()
	t.Setenv("XAUTHORITY", t.TempDir()+"/none")
	pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	end := func(fd int) *net.UnixConn {
		f := os.NewFile(uintptr(fd), "x11")
		defer f.Close() // FileConn duplicates it
		c, err := net.FileConn(f)
		if err != nil {
			t.Fatal(err)
		}
		return c.(*net.UnixConn)
	}
	client, server := end(pair[0]), end(pair[1])
	type result struct {
		c   *xgb.Conn
		err error
	}
	done := make(chan result, 1)
	go func() {
		c, err := xgb.NewConnNet(client)
		done <- result{c, err}
	}()
	req := make([]byte, 12)
	if _, err := io.ReadFull(server, req); err != nil {
		t.Fatal(err)
	}
	pad := func(n int) int { return (n + 3) &^ 3 }
	auth := make([]byte, pad(int(le.Uint16(req[6:])))+pad(int(le.Uint16(req[8:]))))
	if _, err := io.ReadFull(server, auth); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 8+32)
	reply[0] = 1
	le.PutUint16(reply[2:], 11)
	le.PutUint16(reply[6:], 8)
	le.PutUint32(reply[12:], 0x200000)
	le.PutUint32(reply[16:], 0x1fffff)
	if _, err := server.Write(reply); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.err != nil {
		t.Fatalf("NewConnNet: %v", r.err)
	}
	return r.c, server
}

// readMsg reads exactly n bytes from the server's end with the file
// descriptors that came with them.
func readMsg(t *testing.T, server *net.UnixConn, n int) ([]byte, []int) {
	t.Helper()
	server.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf, oob := make([]byte, n), make([]byte, syscall.CmsgSpace(4*8))
	var fds []int
	for read := 0; read < n; {
		k, oobn, _, _, err := server.ReadMsgUnix(buf[read:], oob)
		if err != nil {
			t.Fatalf("the server read %d of %d bytes: %v", read, n, err)
		}
		read += k
		msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			t.Fatal(err)
		}
		for i := range msgs {
			got, err := syscall.ParseUnixRights(&msgs[i])
			if err != nil {
				t.Fatal(err)
			}
			fds = append(fds, got...)
		}
	}
	t.Cleanup(func() {
		for _, fd := range fds {
			syscall.Close(fd)
		}
	})
	return buf, fds
}

// sameFile reports whether two descriptors are open on the same file.
func sameFile(t *testing.T, a, b int) bool {
	t.Helper()
	var sa, sb syscall.Stat_t
	if err := syscall.Fstat(a, &sa); err != nil {
		t.Fatalf("fstat %d: %v", a, err)
	}
	if err := syscall.Fstat(b, &sb); err != nil {
		t.Fatalf("fstat %d: %v", b, err)
	}
	return sa.Dev == sb.Dev && sa.Ino == sb.Ino
}

func tempFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// TestRequestFds: the file descriptors of a request arrive with its
// bytes, in order, one per mention (the same descriptor twice is two),
// and stay open on the caller's side.
func TestRequestFds(t *testing.T) {
	c, server := unixServer(t)
	defer shutdown(c, server)
	if !c.CanPassFds() {
		t.Fatal("a connection over a unix socket cannot pass file descriptors")
	}
	a, b := tempFile(t), tempFile(t)
	fds := []int{int(a.Fd()), int(b.Fd()), int(a.Fd())}

	type got struct {
		buf []byte
		fds []int
	}
	read := make(chan got, 1)
	go func() {
		buf, fds := readMsg(t, server, 64)
		read <- got{buf, fds}
	}()
	req := make([]byte, 64)
	req[0], req[1] = 147, 7
	le.PutUint16(req[2:], 16)
	req[63] = 0x5a
	c.NewRequestFds(req, fds, c.NewCookie(false, false))
	// NewRequestFds returned: the request is written, and the caller may
	// close its descriptors at once.
	g := <-read
	if g.buf[0] != 147 || g.buf[63] != 0x5a {
		t.Errorf("the server read % x", g.buf)
	}
	if len(g.fds) != 3 {
		t.Fatalf("the server received %d file descriptors, want 3", len(g.fds))
	}
	for i, want := range []*os.File{a, b, a} {
		if !sameFile(t, g.fds[i], int(want.Fd())) {
			t.Errorf("descriptor %d is not the file sent", i)
		}
	}
	if _, err := a.WriteString("still open"); err != nil {
		t.Errorf("the caller's descriptor after the request: %v", err)
	}

	// A request without descriptors follows undisturbed.
	go func() {
		buf, fds := readMsg(t, server, 4)
		read <- got{buf, fds}
	}()
	c.NewRequest([]byte{43, 0, 1, 0}, c.NewCookie(false, false))
	if g := <-read; g.buf[0] != 43 || len(g.fds) != 0 {
		t.Errorf("the next request arrived as % x with %d descriptors", g.buf, len(g.fds))
	}
}

// TestReplyFds: the file descriptor of a reply reaches the cookie of its
// request, also when it arrived with the bytes of an event before the
// reply, and the replies around it get none.
func TestReplyFds(t *testing.T) {
	c, server := unixServer(t)
	defer shutdown(c, server)
	f := tempFile(t)

	reply := func(seq uint16, nfd byte) []byte {
		r := make([]byte, 32)
		r[0], r[1] = 1, nfd
		le.PutUint16(r[2:], seq)
		return r
	}
	go func() {
		// Three requests of 4 bytes: a plain one, DRI3 Open, a plain one.
		readMsg(t, server, 12)
		server.Write(reply(1, 0))
		// The server writes what it queued in one sendmsg, the
		// descriptors with its first byte: here an event comes first.
		ev := make([]byte, 32)
		ev[0] = 85
		out := append(ev, reply(2, 1)...)
		if _, _, err := server.WriteMsgUnix(out, syscall.UnixRights(int(f.Fd())), nil); err != nil {
			t.Error(err)
		}
		server.Write(reply(3, 0))
	}()
	first, open, last := c.NewCookie(true, true), c.NewCookieFds(true), c.NewCookie(true, true)
	c.NewRequest([]byte{43, 0, 1, 0}, first)
	c.NewRequest([]byte{147, 1, 1, 0}, open)
	c.NewRequest([]byte{43, 0, 1, 0}, last)

	if _, err := first.Reply(); err != nil {
		t.Fatalf("the first reply: %v", err)
	}
	r, fds, err := open.ReplyFds()
	if err != nil || len(r) != 32 {
		t.Fatalf("ReplyFds = %d bytes, %v", len(r), err)
	}
	if len(fds) != 1 {
		t.Fatalf("ReplyFds returned %d file descriptors, want 1", len(fds))
	}
	defer syscall.Close(fds[0])
	if !sameFile(t, fds[0], int(f.Fd())) {
		t.Error("the descriptor of the reply is not the file sent")
	}
	if flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fds[0]), syscall.F_GETFD, 0); errno != 0 || flags&syscall.FD_CLOEXEC == 0 {
		t.Errorf("the descriptor of the reply is not close-on-exec (flags %#x, %v)", flags, errno)
	}
	if _, err := last.Reply(); err != nil {
		t.Fatalf("the last reply: %v", err)
	}
	if e, _ := waitEvent(t, c); e == nil {
		t.Error("the event that carried the descriptor was lost")
	}
}

// TestReplyFdsError: an error instead of the reply returns no
// descriptors, and ReplyFds does not wait for any.
func TestReplyFdsError(t *testing.T) {
	c, server := unixServer(t)
	defer shutdown(c, server)
	go func() {
		readMsg(t, server, 4)
		e := make([]byte, 32)
		e[1] = 8 // BadMatch, which DRI3 Open answers a remote client with
		le.PutUint16(e[2:], 1)
		server.Write(e)
	}()
	cookie := c.NewCookieFds(true)
	c.NewRequest([]byte{147, 1, 1, 0}, cookie)
	done := make(chan error, 1)
	go func() {
		_, fds, err := cookie.ReplyFds()
		if len(fds) != 0 {
			t.Errorf("an error returned %d descriptors", len(fds))
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("ReplyFds returned no error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReplyFds blocked on an error")
	}
}

// TestFdsWithoutUnixSocket: a transport that cannot carry descriptors
// says so, sends the request without them, and returns the reply without
// any.
func TestFdsWithoutUnixSocket(t *testing.T) {
	c, server := fakeServer(t) // a net.Pipe
	defer shutdown(c, server)
	if c.CanPassFds() {
		t.Fatal("a pipe claims to pass file descriptors")
	}
	f := tempFile(t)
	go func() {
		req := make([]byte, 4)
		if _, err := io.ReadFull(server, req); err != nil {
			return
		}
		r := make([]byte, 32)
		r[0], r[1] = 1, 1 // one descriptor announced, none can come
		le.PutUint16(r[2:], 1)
		server.Write(r)
	}()
	cookie := c.NewCookieFds(true)
	c.NewRequestFds([]byte{147, 1, 1, 0}, []int{int(f.Fd())}, cookie)
	done := make(chan int, 1)
	go func() {
		_, fds, err := cookie.ReplyFds()
		if err != nil {
			t.Errorf("ReplyFds: %v", err)
		}
		done <- len(fds)
	}()
	select {
	case n := <-done:
		if n != 0 {
			t.Errorf("%d descriptors over a pipe", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReplyFds blocked without a unix socket")
	}
}
