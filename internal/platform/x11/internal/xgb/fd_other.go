// Tela patch (ADR-025): fd.go on systems without unix sockets that pass
// file descriptors. The X11 backend is built for Linux only; this keeps
// the package compiling elsewhere.

//go:build !unix

package xgb

import "io"

// CanPassFds reports false: no transport here carries file descriptors.
func (c *Conn) CanPassFds() bool { return false }

func (c *Conn) writeFds(buf []byte, fds []int) error { return c.writeBuffer(buf) }

func (c *Conn) readFull(buf []byte) (int, error) { return io.ReadFull(c.conn, buf) }

func (c *Conn) closeFds() {}
