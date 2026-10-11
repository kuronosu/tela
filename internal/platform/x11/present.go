//go:build linux

package x11

import (
	"encoding/binary"
	"image"

	"github.com/jezek/xgb/xproto"
)

// PutImage is encoded by hand instead of with xproto.PutImage because xgb's
// generated requests only have the 16-bit length field: a request over
// 256 KiB silently gets a truncated length. With BIG-REQUESTS enabled, a
// request whose length field is 0 carries a 32-bit length after the
// header (BIG-REQUESTS extension, section 2). Encoding it ourselves also
// lets the backend reuse one request buffer: xgb's NewRequest returns only
// after the bytes are written to the socket (or the connection is gone), so
// the buffer is free again when it returns.

const (
	putImageOpcode = 72
	putImageHeader = 24 // bytes, standard length field
	bigHeaderExtra = 4  // the extra CARD32 length of a big request
)

// putImageHeaderLen returns the header size of a PutImage request.
func putImageHeaderLen(big bool) int {
	if big {
		return putImageHeader + bigHeaderExtra
	}
	return putImageHeader
}

// splitForRequests splits r into rectangles whose 32-bpp ZPixmap payload
// plus a header of headerLen bytes fits in maxBytes. It prefers full-width
// bands of rows, and splits rows into column runs only when a single row
// does not fit (very small request limits or very wide windows).
func splitForRequests(r image.Rectangle, maxBytes, headerLen int) []image.Rectangle {
	if r.Empty() {
		return nil
	}
	avail := maxBytes - headerLen
	if avail < 4 {
		panic("x11: request limit too small for one pixel")
	}
	rowBytes := 4 * r.Dx()
	if rowBytes <= avail {
		rows := avail / rowBytes
		out := make([]image.Rectangle, 0, (r.Dy()+rows-1)/rows)
		for y := r.Min.Y; y < r.Max.Y; y += rows {
			out = append(out, image.Rect(r.Min.X, y, r.Max.X, min(y+rows, r.Max.Y)))
		}
		return out
	}
	cols := avail / 4
	var out []image.Rectangle
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x += cols {
			out = append(out, image.Rect(x, y, min(x+cols, r.Max.X), y+1))
		}
	}
	return out
}

// encodePutImage writes a PutImage request (ZPixmap, depth 24, 32 bpp) for
// rectangle r of the BGRX image pix (stride 4*pixW) into buf, growing it
// if needed, and returns the request bytes. All fields use the client byte
// order, which xgb sets to little-endian at connection setup.
func encodePutImage(buf []byte, big bool, drawable xproto.Drawable, gc xproto.Gcontext, depth byte, pix []byte, pixW int, r image.Rectangle) []byte {
	hdr := putImageHeaderLen(big)
	rowBytes := 4 * r.Dx()
	total := hdr + rowBytes*r.Dy()
	if cap(buf) < total {
		buf = make([]byte, total)
	}
	buf = buf[:total]
	le := binary.LittleEndian
	buf[0] = putImageOpcode
	buf[1] = xproto.ImageFormatZPixmap
	var o int
	if big {
		le.PutUint16(buf[2:], 0)
		le.PutUint32(buf[4:], uint32(total/4))
		o = 8
	} else {
		le.PutUint16(buf[2:], uint16(total/4))
		o = 4
	}
	le.PutUint32(buf[o:], uint32(drawable))
	le.PutUint32(buf[o+4:], uint32(gc))
	le.PutUint16(buf[o+8:], uint16(r.Dx()))
	le.PutUint16(buf[o+10:], uint16(r.Dy()))
	le.PutUint16(buf[o+12:], uint16(int16(r.Min.X)))
	le.PutUint16(buf[o+14:], uint16(int16(r.Min.Y)))
	buf[o+16] = 0 // left-pad
	buf[o+17] = depth
	buf[o+18], buf[o+19] = 0, 0
	d := buf[hdr:]
	stride := 4 * pixW
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := y*stride + 4*r.Min.X
		copy(d[:rowBytes], pix[s:s+rowBytes])
		d = d[rowBytes:]
	}
	return buf
}

// putImage sends rectangle r of the retained copy to the window, split to
// the request size limit.
func (w *Window) putImage(r image.Rectangle) {
	b := w.b
	r = r.Intersect(image.Rectangle{Max: w.pixSize})
	if r.Empty() || w.pix == nil {
		return
	}
	for _, c := range splitForRequests(r, b.maxRequest, putImageHeaderLen(b.bigRequests)) {
		// A request that fits the 16-bit length is sent in the standard
		// form even when BIG-REQUESTS is on.
		big := b.bigRequests && putImageHeader+4*c.Dx()*c.Dy() > 0xffff*4
		b.reqBuf = encodePutImage(b.reqBuf, big, xproto.Drawable(w.id), w.gc, b.depth, w.pix, w.pixSize.X, c)
		b.conn.NewRequest(b.reqBuf, b.conn.NewCookie(false, false))
	}
}

// flushExpose repaints the accumulated Expose region from the retained
// copy.
func (w *Window) flushExpose() {
	r := w.expose
	w.expose = image.Rectangle{}
	if !r.Empty() {
		w.putImage(r)
	}
}
