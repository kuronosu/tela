// Tela patch (ADR-025): the socket side of fd.go.

//go:build unix

package xgb

import (
	"io"
	"net"
	"syscall"
)

// maxReplyFds is how many descriptors one read can take: more than any
// reply carries (DRI3 BuffersFromPixmap has up to 4; libxcb allows 16).
const maxReplyFds = 16

// CanPassFds reports whether the connection is a unix socket, the only
// transport that carries file descriptors.
func (c *Conn) CanPassFds() bool {
	_, ok := c.conn.(*net.UnixConn)
	return ok
}

// writeFds writes a request with its descriptors: they travel with the
// first sendmsg, and whatever the socket did not take of the bytes follows
// in plain writes.
func (c *Conn) writeFds(buf []byte, fds []int) error {
	uc, ok := c.conn.(*net.UnixConn)
	if !ok || len(fds) == 0 {
		return c.writeBuffer(buf)
	}
	n, _, err := uc.WriteMsgUnix(buf, syscall.UnixRights(fds...), nil)
	if err != nil {
		Logger.Printf("A write error is unrecoverable: %s", err)
		return err
	}
	if n < len(buf) {
		return c.writeBuffer(buf[n:])
	}
	return nil
}

// readFull is io.ReadFull on the connection that also takes the file
// descriptors that arrive with the bytes, into c.inFds. Reader goroutine
// only.
func (c *Conn) readFull(buf []byte) (int, error) {
	uc, ok := c.conn.(*net.UnixConn)
	if !ok {
		return io.ReadFull(c.conn, buf)
	}
	if c.oob == nil {
		c.oob = make([]byte, syscall.CmsgSpace(4*maxReplyFds))
	}
	read := 0
	for read < len(buf) {
		// ReadMsgUnix sets MSG_CMSG_CLOEXEC where the OS has it.
		n, oobn, flags, _, err := uc.ReadMsgUnix(buf[read:], c.oob)
		read += n
		if oobn > 0 {
			if flags&syscall.MSG_CTRUNC != 0 {
				Logger.Printf("File descriptors of a reply were lost: more than %d arrived at once", maxReplyFds)
			}
			if msgs, perr := syscall.ParseSocketControlMessage(c.oob[:oobn]); perr == nil {
				for i := range msgs {
					if fds, ferr := syscall.ParseUnixRights(&msgs[i]); ferr == nil {
						c.inFds = append(c.inFds, fds...)
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF && read > 0 {
				err = io.ErrUnexpectedEOF
			}
			return read, err
		}
		if n == 0 && oobn == 0 {
			return read, io.ErrUnexpectedEOF
		}
	}
	return read, nil
}

// closeFds closes the descriptors nobody took (the reader's exit).
func (c *Conn) closeFds() {
	for _, fd := range c.inFds {
		syscall.Close(fd)
	}
	c.inFds = nil
}
