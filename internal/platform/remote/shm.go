package remote

import (
	"crypto/rand"
	"encoding/binary"
)

// Shared memory
//
// A window's pixels travel through one shared segment per framebuffer
// size, created by the child and mapped by the host: a memfd on Linux and
// an unlinked temporary file on macOS, both passed as a descriptor over
// the socket (SCM_RIGHTS), and a named file mapping on Windows, passed by
// name. The child maps it read-write, the host read-only. See child.go
// for the buffering protocol.

// segment is a mapping of a shared memory segment.
type segment struct {
	data    []byte
	release func()
}

// close unmaps the segment. Safe on nil and twice.
func (s *segment) close() {
	if s != nil && s.release != nil {
		s.release()
		s.release = nil
		s.data = nil
	}
}

func randUint32() uint32 {
	var b [4]byte
	rand.Read(b[:])
	return binary.LittleEndian.Uint32(b[:])
}
