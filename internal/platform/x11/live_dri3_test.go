//go:build linux

package x11

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// TestLiveDmabufServer logs what the real X server has for presenting
// dmabufs (dmabuf.go): the highest DRI3 and Present versions it speaks,
// Present's capabilities, the device DRI3 Open returns and the modifiers
// it lists for the depths of an opaque and of a transparent window. It
// opens no window and needs no GPU on the client's side, so it tells of
// any server whether TestLiveDmabuf (tag telagpu) can run there. It asks
// on a connection of its own with a client version no server reaches,
// which the backend never does: it declares the versions it speaks.
func TestLiveDmabufServer(t *testing.T) {
	b := liveBackend(t)
	defer b.conn.Close()
	if !b.conn.CanPassFds() {
		t.Skip("the connection is not a unix socket: no file descriptors, no DRI3")
	}
	version := func(name string) (extension, bool) {
		e := b.queryExtension(name)
		if !e.present {
			t.Logf("%s: not present", name)
			return e, false
		}
		r, err := b.roundTrip(versionRequest(e.opcode, 0, 1, 99), 16)
		if err != nil {
			t.Logf("%s: QueryVersion: %v", name, err)
			return e, false
		}
		t.Logf("%s %d.%d (major opcode %d)", name, le.Uint32(r[8:]), le.Uint32(r[12:]), e.opcode)
		return e, true
	}
	dri3, ok := version("DRI3")
	present, okPresent := version("Present")
	if okPresent {
		// QueryCapabilities of the root window's CRTC: 1 async, 2 fence,
		// 4 UST, 8 async may tear, 16 syncobj.
		req := newRequest(present.opcode, 4, 8)
		le.PutUint32(req[4:], uint32(b.root))
		if r, err := b.roundTrip(req, 12); err == nil {
			t.Logf("Present capabilities: %#x", le.Uint32(r[8:]))
		}
	}
	t.Logf("XFixes 2.0 or later (update regions): %v", b.ensureXFixes())
	if !ok {
		return
	}

	cookie := b.conn.NewCookieFds(true)
	b.conn.NewRequest(dri3OpenRequest(dri3.opcode, xproto.Drawable(b.root)), cookie)
	_, fds, err := cookie.ReplyFds()
	if err != nil || len(fds) == 0 {
		t.Logf("DRI3 Open: %d file descriptors, %v", len(fds), err)
	}
	for _, fd := range fds {
		path, _ := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
		major, minor, isNode := fdDevice(fd)
		t.Logf("DRI3 Open: %s, device %d:%d (a character device: %v)", path, major, minor, isNode)
		unix.Close(fd)
	}

	b.dmabuf.dri3Op = dri3.opcode
	for _, depth := range []byte{24, 32} {
		window, screen, err := connServer{b}.modifiers(b.root, depth, dmabufBpp)
		if err != nil {
			t.Logf("GetSupportedModifiers for depth %d: %v", depth, err)
			continue
		}
		t.Logf("depth %d at %d bits per pixel: window modifiers %#x, screen modifiers %#x (%#x is the implicit one, 0 the linear one)",
			depth, dmabufBpp, window, screen, platform.DRMModifierInvalid)
	}
}
