//go:build linux

package x11

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// fakePortal is the freedesktop side of the shortcuts.
type fakePortal struct {
	err   error
	calls []string
}

func (p *fakePortal) RegisterShortcut(s platform.Shortcut) error {
	p.calls = append(p.calls, "register "+s.ID)
	return p.err
}

func (p *fakePortal) UnregisterShortcut(id string) error {
	p.calls = append(p.calls, "unregister "+id)
	return nil
}

func TestAccelGrabs(t *testing.T) {
	km, err := xkb.NewCoreKeymap(deMapping())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		a     platform.Accelerator
		kcs   []byte
		mods  uint16
		locks []uint16
	}{
		{platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt, Key: "a"}, []byte{kcA}, 0x0c, []uint16{0, 2, 0x10, 0x12}},
		{platform.Accelerator{Mods: platform.ModShift | platform.ModSuper, Key: "z"}, []byte{kcY}, 0x41, []uint16{0, 2, 0x10, 0x12}},
		{platform.Accelerator{Mods: platform.ModCtrl, Key: platform.KeyEnter}, []byte{kcEnter}, 0x04, []uint16{0, 2, 0x10, 0x12}},
		{platform.Accelerator{Mods: platform.ModCtrl, Key: " "}, []byte{kcSpace}, 0x04, []uint16{0, 2, 0x10, 0x12}},
		{platform.Accelerator{Mods: platform.ModCtrl, Key: "x"}, nil, 0x04, []uint16{0, 2, 0x10, 0x12}},
	} {
		kcs, mods, locks := accelGrabs(km, c.a)
		if !slices.Equal(kcs, c.kcs) || mods != c.mods || !slices.Equal(locks, c.locks) {
			t.Errorf("accelGrabs(%v) = %v %#x %#x, want %v %#x %#x", c.a, kcs, mods, locks, c.kcs, c.mods, c.locks)
		}
	}
}

// shortcutRig is a data rig with the German-like keymap.
func shortcutRig(detectable bool) (*dataRig, *fakePortal) {
	g := newDataRig()
	g.b.kbd.fetch = func() (xkb.CoreMapping, error) { return deMapping(), nil }
	g.b.xkb = xkbExt{extension: extension{present: true, opcode: 135, firstEvent: xkbFirstEvent}, ok: true, detectable: detectable}
	return g, &fakePortal{err: platform.ErrUnsupported}
}

func rootPress(kc byte, state uint16, time uint32) xproto.KeyPressEvent {
	return xproto.KeyPressEvent{Detail: xproto.Keycode(kc), Event: 1, Root: 1, State: state, Time: xproto.Timestamp(time)}
}

func rootRelease(kc byte, state uint16, time uint32) xproto.KeyReleaseEvent {
	return xproto.KeyReleaseEvent{Detail: xproto.Keycode(kc), Event: 1, Root: 1, State: state, Time: xproto.Timestamp(time)}
}

var ctrlAltA = platform.Shortcut{ID: "show", Accelerator: platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt, Key: "a"}, Description: "Show"}

func TestShortcutGrabs(t *testing.T) {
	g, portal := shortcutRig(true)
	if err := g.b.registerShortcut(portal, ctrlAltA); err != nil {
		t.Fatal(err)
	}
	if got, w := g.x.grabs(), "38/0xc 38/0xe 38/0x1c 38/0x1e"; got != w {
		t.Errorf("grabs = %s, want %s", got, w)
	}
	if got, w := g.pass(), `status show bound "Ctrl+Alt+A"`; got != w {
		t.Errorf("status = %q, want %q", got, w)
	}
	// Presses with NumLock on match; a repeat is dropped; the release
	// is reported.
	got := g.feed(rootPress(kcA, 0x1c, 1), rootPress(kcA, 0x1c, 2), rootRelease(kcA, 0x1c, 3))
	if w := "shortcut show pressed; shortcut show released"; got != w {
		t.Errorf("keys = %q, want %q", got, w)
	}
	// The same combination for another ID of the app: taken.
	dup := ctrlAltA
	dup.ID = "dup"
	g.b.registerShortcut(portal, dup)
	if got := g.pass(); !strings.HasPrefix(got, `status dup taken "" (the app's shortcut "show" uses Ctrl+Alt+A)`) {
		t.Errorf("duplicate = %q", got)
	}
	// Another client holds Ctrl+Enter with Lock: taken, nothing grabbed.
	g.x.taken[[2]int{kcEnter, 0x6}] = true
	enter := platform.Shortcut{ID: "enter", Accelerator: platform.Accelerator{Mods: platform.ModCtrl, Key: platform.KeyEnter}, Description: "Enter"}
	g.b.registerShortcut(portal, enter)
	if got := g.pass(); !strings.HasPrefix(got, `status enter taken`) {
		t.Errorf("BadAccess = %q", got)
	}
	if got, w := g.x.grabs(), "38/0xc 38/0xe 38/0x1c 38/0x1e"; got != w {
		t.Errorf("grabs after BadAccess = %s", got)
	}
	// A key of no layout: failed.
	g.b.registerShortcut(portal, platform.Shortcut{ID: "x", Accelerator: platform.Accelerator{Key: "x"}, Description: "X"})
	if got := g.pass(); !strings.HasPrefix(got, `status x failed`) {
		t.Errorf("missing key = %q", got)
	}

	// The keyboard mapping changes: the grabs follow the key.
	moved := deMapping()
	per := moved.KeysymsPerKeycode
	copy(moved.Keysyms[(kcA+1-8)*per:], moved.Keysyms[(kcA-8)*per:(kcA-7)*per])
	clear(moved.Keysyms[(kcA-8)*per : (kcA-7)*per])
	g.b.kbd.fetch = func() (xkb.CoreMapping, error) { return moved, nil }
	g.feed(xproto.MappingNotifyEvent{Request: xproto.MappingKeyboard})
	if got, w := g.x.grabs(), "39/0xc 39/0xe 39/0x1c 39/0x1e"; got != w {
		t.Errorf("grabs after the mapping change = %s, want %s", got, w)
	}

	// Unregister releases the grabs; unknown IDs are ignored.
	if err := g.b.unregisterShortcut(portal, "show"); err != nil {
		t.Fatal(err)
	}
	if err := g.b.unregisterShortcut(portal, "nope"); err != nil {
		t.Fatal(err)
	}
	if got := g.x.grabs(); got != "" {
		t.Errorf("grabs after unregister = %s", got)
	}
	if got := g.feed(rootPress(kcA+1, 0xc, 9)); got != "" {
		t.Errorf("press after unregister = %q", got)
	}
	if len(portal.calls) != 1 {
		t.Errorf("the portal was asked again after ErrUnsupported: %v", portal.calls)
	}
}

func TestShortcutRepeatWithoutDetectable(t *testing.T) {
	g, portal := shortcutRig(false)
	g.b.registerShortcut(portal, ctrlAltA)
	g.pass()
	// A server repeat is a release and a press at the same time.
	got := g.feed(rootPress(kcA, 0xc, 1), rootRelease(kcA, 0xc, 5), rootPress(kcA, 0xc, 5), rootRelease(kcA, 0xc, 9))
	if w := "shortcut show pressed; shortcut show released"; got != w {
		t.Errorf("keys = %q, want %q", got, w)
	}
}

func TestShortcutPortalRefusesAppID(t *testing.T) {
	// xdg-desktop-portal 1.22 refuses an app ID without a desktop entry:
	// the key is grabbed instead, with one status (bound), and later
	// shortcuts skip the portal.
	g, portal := shortcutRig(true)
	portal.err = fmt.Errorf("%w: no entry dev.example.app.desktop", platform.ErrAppNotRegistered)
	if err := g.b.registerShortcut(portal, ctrlAltA); err != nil {
		t.Fatal(err)
	}
	if got, w := g.pass(), `status show bound "Ctrl+Alt+A"`; got != w {
		t.Errorf("status = %q, want %q (one status, no failure first)", got, w)
	}
	if got := g.x.grabs(); got == "" {
		t.Error("nothing was grabbed")
	}
	enter := platform.Shortcut{ID: "enter", Accelerator: platform.Accelerator{Mods: platform.ModCtrl, Key: platform.KeyEnter}, Description: "Enter"}
	if err := g.b.registerShortcut(portal, enter); err != nil {
		t.Fatal(err)
	}
	if w := []string{"register show"}; !slices.Equal(portal.calls, w) {
		t.Errorf("portal calls = %v, want %v", portal.calls, w)
	}
	// Any other error is the caller's.
	g2, portal2 := shortcutRig(true)
	portal2.err = errors.New("boom")
	if err := g2.b.registerShortcut(portal2, ctrlAltA); err == nil || g2.x.grabs() != "" {
		t.Errorf("another portal error = %v, grabs %q; want the error and no grab", err, g2.x.grabs())
	}
}

func TestShortcutPortalFirst(t *testing.T) {
	g, portal := shortcutRig(true)
	portal.err = nil
	if err := g.b.registerShortcut(portal, ctrlAltA); err != nil {
		t.Fatal(err)
	}
	g.b.unregisterShortcut(portal, "show")
	if got := g.x.grabs(); got != "" || g.pass() != "" {
		t.Errorf("the portal's shortcut was grabbed too: %s", got)
	}
	if w := []string{"register show", "unregister show"}; !slices.Equal(portal.calls, w) {
		t.Errorf("portal calls = %v, want %v", portal.calls, w)
	}
}

func TestActivateWithToken(t *testing.T) {
	if ts, ok := tokenTime("tela-1234-host-_TIME98765"); !ok || ts != 98765 {
		t.Errorf("tokenTime = %d, %v", ts, ok)
	}
	if _, ok := tokenTime("plain-token"); ok {
		t.Errorf("tokenTime of a token without _TIME")
	}
	if got := startupChunks("remove: ID=abcdefghij"); len(got) != 2 || string(got[1][:2]) != "j\x00" || len(got[1]) != 20 {
		t.Errorf("startupChunks = %q", got)
	}
	if got := startupQuote(`a "b"`); got != `"a \"b\""` {
		t.Errorf("startupQuote = %s", got)
	}

	g := newDataRig()
	f := g.b.srv.(*fakeServer)
	g.w.mapped, g.w.visible = true, true
	f.take()
	g.w.ActivateWithToken("tela-1-host-_TIME42")
	if got, w := f.take(), `prop _NET_STARTUP_ID win=7 = "tela-1-host-_TIME42"; msg _NET_ACTIVE_WINDOW win=7 [1 42 0]`; got != w {
		t.Errorf("window requests = %q, want %q", got, w)
	}
	if got, w := g.x.take(), `create selection window; msg _NET_STARTUP_INFO_BEGIN to=1 "remove: ID=tela-1-ho"; msg _NET_STARTUP_INFO to=1 "st-_TIME42"`; got != w {
		t.Errorf("startup messages = %q, want %q", got, w)
	}
}
