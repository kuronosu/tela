// Package wire is the client side of the Wayland wire protocol: connection
// setup, the object ID map, message encoding and decoding, file-descriptor
// passing and buffered, non-blocking I/O.
//
// The runtime is deliberately single-threaded and starts no goroutines. It is
// meant to be driven by the UI thread of the platform backend, which owns all
// blocking: the backend polls [Conn.Fd] for POLLIN (and POLLOUT while
// [Conn.Flush] reports [ErrWouldBlock]), then calls [Conn.Read] and
// [Conn.Dispatch]. That is how Tela keeps 0% CPU while idle. The only
// blocking calls in this package are [Conn.Roundtrip] and
// [Conn.RoundtripTimeout], meant for startup.
//
// The Go bindings generated from the protocol XML (package protocols) build on
// the types here: every generated interface embeds a [Proxy], implements
// [Object], and describes itself with an [Interface] table.
//
// All files except this one are only built on Linux.
package wire
