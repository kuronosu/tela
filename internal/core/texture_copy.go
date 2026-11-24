package core

import (
	"encoding/binary"
	"fmt"
	"runtime/debug"
)

// The copy of the mapped memory of imported buffers (mappedTexture.read):
// dmabufs (texture_linux.go) and IOSurfaces (texture_iosurface.go).

// copyMapped copies a picture of w x h pixels of 4 bytes, whose rows are
// stride bytes apart in src, to dst as RGBA with rows of 4*w bytes; swap and
// opaque are convertRow's. src is memory mapped from elsewhere: should it go
// meanwhile (a device unplugged), it returns an error instead of crashing,
// which names it what ("a dmabuf").
func copyMapped(dst []byte, what string, src []byte, w, h, stride int, swap, opaque bool) (err error) {
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	defer func() {
		if r := recover(); r != nil {
			if f, ok := r.(interface{ Addr() uintptr }); ok {
				err = fmt.Errorf("the memory of %s is gone (a fault at 0x%x)", what, f.Addr())
				return
			}
			panic(r)
		}
	}()
	row := 4 * w
	for y := range h {
		s, d := src[y*stride:][:row], dst[y*row:][:row]
		if !swap && !opaque {
			copy(d, s)
		} else {
			convertRow(d, s, swap, opaque)
		}
	}
	return nil
}

// convertRow copies the pixels of src to dst as RGBA: with swap they are
// blue first, and with opaque their fourth byte means nothing.
func convertRow(dst, src []byte, swap, opaque bool) {
	// perf: eight bytes at a time would halve it.
	var alpha uint32
	if opaque {
		alpha = 0xff000000
	}
	for i := 0; i+4 <= len(src) && i+4 <= len(dst); i += 4 {
		v := binary.LittleEndian.Uint32(src[i:])
		if swap {
			v = v&0xff00ff00 | v>>16&0xff | v&0xff<<16
		}
		binary.LittleEndian.PutUint32(dst[i:], v|alpha)
	}
}
