package remote

import (
	"errors"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/testutil/wine"
)

// connPair returns the two ends of an AF_UNIX stream connection.
func connPair(t *testing.T) (a, b transport) {
	t.Helper()
	wine.Skip(t, "no AF_UNIX sockets (WSAEAFNOSUPPORT)")
	addr := filepath.Join(t.TempDir(), "s")
	ln, err := net.Listen("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()
	ta, err := dialUnix(addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c := <-accepted
	if c == nil {
		t.Fatal("accept failed")
	}
	tb := newTransport(c.(*net.UnixConn))
	t.Cleanup(func() { ta.Close(); tb.Close() })
	return ta, tb
}

// TestCleanCloseWithUnreadData: a side that ends the connection cleanly
// while frames of its peer are still unread in its socket (the host's
// last events to a child that quits) does not make the peer's reader fail
// with a reset: the peer reads EOF, and the closing side's error is
// ErrClosed. Closing the socket at once made the host see the child's
// clean Quit as "recvmsg: connection reset by peer" (Linux, and Windows,
// reset a stream socket closed with unread data); TestDisconnectKeepsLastFrame
// failed so once on a loaded machine.
func TestCleanCloseWithUnreadData(t *testing.T) {
	tc, th := connPair(t)
	child := newConn(tc, newFrameReader(tc))
	host := newConn(th, newFrameReader(th))

	// The host's reader ends as Child.readLoop does: it closes its side.
	hostErr, hostDone := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(hostDone)
		for {
			if _, _, err := host.r.next(); err != nil {
				host.fail(err)
				hostErr <- err
				return
			}
		}
	}()
	// The child's reader reads nothing until the host's reader is done,
	// so the host's frames stay unread while the child closes.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		<-hostDone
		for {
			if _, _, err := child.r.next(); err != nil {
				child.fail(err)
				return
			}
		}
	}()
	for range 8 {
		host.send(newMsg(msgRunning).bytes())
	}
	host.flush(5 * time.Second)
	child.send(newMsg(msgRunning).bytes())

	child.closeClean(readerDone, 5*time.Second)
	select {
	case err := <-hostErr:
		if !errors.Is(err, io.EOF) {
			t.Errorf("the host read %v at the child's clean close, want EOF", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the host's reader did not end")
	}
	if err := child.Err(); err != ErrClosed {
		t.Errorf("the child's connection ended with %v, want ErrClosed", err)
	}
}
