//go:build linux

package x11

import (
	"image"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// monitorsReply encodes an RRGetMonitors reply with the given monitors
// (each with one output, as real ones have).
func monitorsReply(primary int, mons ...image.Rectangle) []byte {
	rep := make([]byte, 32)
	rep[0] = 1
	le.PutUint32(rep[12:], uint32(len(mons)))
	le.PutUint32(rep[16:], uint32(len(mons)))
	for i, m := range mons {
		info := make([]byte, 28)
		le.PutUint32(info[0:], uint32(500+i)) // name
		if i == primary {
			info[4] = 1
		}
		le.PutUint16(info[6:], 1)
		le.PutUint16(info[8:], uint16(int16(m.Min.X)))
		le.PutUint16(info[10:], uint16(int16(m.Min.Y)))
		le.PutUint16(info[12:], uint16(m.Dx()))
		le.PutUint16(info[14:], uint16(m.Dy()))
		le.PutUint32(info[24:], uint32(600+i)) // the output
		rep = append(rep, info...)
	}
	return rep
}

func TestParseMonitors(t *testing.T) {
	left, mid, right := image.Rect(-1920, 0, 0, 1080), image.Rect(0, 0, 2560, 1440), image.Rect(2560, 200, 4480, 1280)
	for _, tt := range []struct {
		name string
		rep  []byte
		want []image.Rectangle
	}{
		{"short", make([]byte, 20), nil},
		{"none", monitorsReply(-1), nil},
		{"one", monitorsReply(0, mid), []image.Rectangle{mid}},
		{"primary first already", monitorsReply(0, mid, left, right), []image.Rectangle{mid, left, right}},
		{"primary moved first", monitorsReply(2, left, mid, right), []image.Rectangle{right, left, mid}},
		{"no primary", monitorsReply(-1, left, mid), []image.Rectangle{left, mid}},
		{"empty monitor skipped", monitorsReply(1, image.Rectangle{}, mid), []image.Rectangle{mid}},
		{"truncated", monitorsReply(0, mid, left)[:32+28+10], []image.Rectangle{mid}},
	} {
		if got := parseMonitors(tt.rep); !slices.Equal(got, tt.want) {
			t.Errorf("%s: parseMonitors = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// workAreas cuts the primary monitor to _NET_WORKAREA of the current
// desktop, as GTK does, and leaves the others whole.
func TestWorkAreas(t *testing.T) {
	b := fakeEmpty(1)
	f := b.srv.(*fakeServer)
	a := &b.atoms
	if got := b.workAreas(); got != nil {
		t.Errorf("no monitors: %v", got)
	}
	laptop, external := image.Rect(0, 0, 1920, 1080), image.Rect(1920, 0, 4480, 1440)
	f.mons = []image.Rectangle{laptop, external}
	if got := b.workAreas(); !slices.Equal(got, f.mons) {
		t.Errorf("without _NET_WORKAREA: %v", got)
	}
	// Two desktops; a 40 px panel at the bottom of the laptop on the
	// second, which is the current one.
	f.set(b.root, a.netWorkarea, 0, 0, 4480, 1440, 0, 0, 4480, 1040)
	f.set(b.root, a.netCurrentDesktop, 1)
	if got, want := b.workAreas(), []image.Rectangle{image.Rect(0, 0, 1920, 1040), external}; !slices.Equal(got, want) {
		t.Errorf("work areas %v, want %v", got, want)
	}
	// A current desktop past the list falls back to the first.
	f.set(b.root, a.netCurrentDesktop, 7)
	if got := b.workAreas(); !slices.Equal(got, f.mons) {
		t.Errorf("bad current desktop: %v", got)
	}
	// A work area off the primary is ignored.
	f.set(b.root, a.netWorkarea, 5000, 0, 100, 100)
	if got := b.workAreas(); !slices.Equal(got, f.mons) {
		t.Errorf("work area off the primary: %v", got)
	}
}

// A window created at a position on a monitor that is gone is mapped on
// the nearest one, in physical pixels; SetPosition is never moved.
func TestCreationPositionOnScreens(t *testing.T) {
	g := newRig(t)
	g.f.mons = []image.Rectangle{image.Rect(0, 0, 1920, 1080)}
	g.f.set(g.b.root, g.b.atoms.netWorkarea, 0, 0, 1920, 1040)
	w := fakeWindow(g.b, 7, image.Pt(800, 600), platform.WindowOptions{Size: platform.Size{W: 800, H: 600},
		HasPosition: true, Position: image.Pt(2500, 900)})
	g.feed()
	g.f.take()
	g.present(w)
	if got, want := g.f.take(), "configure win=7 x=1120 y=440; prop WM_NORMAL_HINTS win=7 = pos=1120,440 gravity=10; prop _NET_WM_STATE win=7 = []; map win=7"; got != want {
		t.Errorf("first Present:\n got %s\nwant %s", got, want)
	}
	w.SetPosition(image.Pt(5000, 5000))
	g.sent("SetPosition", "configure win=7 x=5000 y=5000")

	// On a monitor present: where it was asked.
	w2 := fakeWindow(g.b, 8, image.Pt(800, 600), platform.WindowOptions{Size: platform.Size{W: 800, H: 600},
		HasPosition: true, Position: image.Pt(100, 200)})
	g.feed()
	g.f.take()
	g.present(w2)
	if got, want := g.f.take(), "configure win=8 x=100 y=200; prop WM_NORMAL_HINTS win=8 = pos=100,200 gravity=10; prop _NET_WM_STATE win=8 = []; map win=8"; got != want {
		t.Errorf("first Present on screen:\n got %s\nwant %s", got, want)
	}
}
