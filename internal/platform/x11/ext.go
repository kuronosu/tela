//go:build linux

package x11

import (
	"encoding/binary"
	"errors"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// XInput2 and XKB have no Go packages in the vendored xgb (ADR-025): their
// few requests are encoded by hand here and in xi2.go and xkbext.go, and
// their events arrive as xgb.RawEvent (the patched reader frames
// GenericEvents). The connection is little-endian (xgb's setup).

var le = binary.LittleEndian

// extension is a server extension found with QueryExtension.
type extension struct {
	present    bool
	opcode     byte // major opcode
	firstEvent byte
}

// queryExtension asks the server for an extension by name.
func (b *Backend) queryExtension(name string) extension {
	r, err := xproto.QueryExtension(b.conn, uint16(len(name)), name).Reply()
	if err != nil || r == nil || !r.Present {
		return extension{}
	}
	return extension{present: true, opcode: r.MajorOpcode, firstEvent: r.FirstEvent}
}

// newRequest returns a zeroed request of n bytes (a multiple of 4) with
// the major and minor opcodes and the length field set.
func newRequest(major, minor byte, n int) []byte {
	req := make([]byte, n)
	req[0], req[1] = major, minor
	le.PutUint16(req[2:], uint16(n/4))
	return req
}

var errShortReply = errors.New("x11: short reply")

// roundTrip sends a hand-written request that has a reply and waits for
// it. The request is checked: an X error is returned as the error.
func (b *Backend) roundTrip(req []byte, minLen int) ([]byte, error) {
	cookie := b.conn.NewCookie(true, true)
	b.conn.NewRequest(req, cookie)
	r, err := cookie.Reply()
	if err != nil {
		return nil, err
	}
	if len(r) < minLen {
		return nil, errShortReply
	}
	return r, nil
}

// sendRequest sends a hand-written request without a reply. Errors arrive
// as events and are ignored like those of other unchecked requests.
func (b *Backend) sendRequest(req []byte) {
	b.conn.NewRequest(req, b.conn.NewCookie(false, false))
}

// rawEvent dispatches an event the vendored xgb does not decode.
func (b *Backend) rawEvent(raw xgb.RawEvent) {
	switch {
	case len(raw) < 32:
	case raw.Code() == xgb.GenericEventCode:
		if b.xi.present && raw[1] == b.xi.opcode {
			b.xiEvent(raw)
		}
	case b.xkb.present && raw.Code() == b.xkb.firstEvent:
		b.xkbEvent(raw)
	}
}
