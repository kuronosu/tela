// Tela patch (ADR-025): file descriptors on requests and replies, which
// DRI3 passes (PixmapFromBuffers sends the dmabufs of a pixmap, the reply
// of Open carries the DRM device). This file is Tela's own; the rest of
// the package is vendored from github.com/jezek/xgb v1.3.1 (see LICENSE
// and THIRD_PARTY.md). fd_unix.go has the socket side.
//
// The X protocol sends descriptors as ancillary data of the connection's
// unix socket (SCM_RIGHTS), beside the bytes of the request or the reply
// they belong to, as libxcb does (xcb_send_fd, xcb_*_reply_fds):
//
//   - A request's descriptors go with the first byte of the request, in
//     one sendmsg: the server reads them when it reads the request, and
//     drops what a request did not claim before it reads the next one.
//   - A reply's descriptors arrive no later than its first byte (the
//     server queues them before the reply's bytes and sends both in one
//     write, possibly behind events queued earlier), so every read of the
//     reader takes ancillary data and keeps the descriptors in a queue. A
//     reply to a request made with NewCookieFds takes as many as the
//     reply's second byte says (the nfd field of every such reply).

package xgb

// NewRequestFds is NewRequest for a request that carries file descriptors.
// Like NewRequest it returns once the request is written to the socket (or
// the connection is gone), and the kernel holds its own references from
// then on: the descriptors stay the caller's, who may close them when the
// call returns. On a connection that cannot carry descriptors (CanPassFds)
// the request is sent without them, and the server answers it with an
// error.
func (c *Conn) NewRequestFds(buf []byte, fds []int, cookie *Cookie) {
	c.newRequest(buf, fds, cookie)
}

// NewCookieFds is NewCookie(checked, true) for a request whose reply
// carries file descriptors: ReplyFds returns them.
func (c *Conn) NewCookieFds(checked bool) *Cookie {
	cookie := c.NewCookie(checked, true)
	cookie.fdChan = make(chan []int, 1)
	return cookie
}

// ReplyFds is Reply for a cookie made by NewCookieFds: it also returns the
// file descriptors of the reply, which the caller owns and closes (they
// have the close-on-exec flag). They are fewer than the reply announces
// when the connection cannot carry descriptors.
func (c Cookie) ReplyFds() ([]byte, []int, error) {
	reply, err := c.Reply()
	if err != nil || reply == nil || c.fdChan == nil {
		return reply, nil, err
	}
	// The reader queued them before the reply.
	return reply, <-c.fdChan, nil
}

// takeFds removes the first n descriptors the reader received and returns
// them. Reader goroutine only.
func (c *Conn) takeFds(n int) []int {
	n = min(n, len(c.inFds))
	fds := append([]int(nil), c.inFds[:n]...)
	c.inFds = c.inFds[n:]
	return fds
}
