//go:build linux

package x11

import (
	"image"
	"slices"
	"testing"
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
