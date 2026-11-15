//go:build linux

package wayland

import (
	"errors"
	"image"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// Formats of the tests' compositor, with an NVIDIA-like and an Intel-like
// modifier.
const (
	modTiled = 0x0300000000606014
	modY     = 0x0100000000000002
)

var (
	xrgbTiled  = platform.DmabufFormat{Fourcc: platform.DRMFormatXRGB8888, Modifier: modTiled}
	xrgbLinear = platform.DmabufFormat{Fourcc: platform.DRMFormatXRGB8888, Modifier: platform.DRMModifierLinear}
	argbTiled  = platform.DmabufFormat{Fourcc: platform.DRMFormatARGB8888, Modifier: modTiled}
	xbgrY      = platform.DmabufFormat{Fourcc: platform.DRMFormatXBGR8888, Modifier: modY}
)

// fakeDmabuf plays the compositor's side of zwp_linux_dmabuf_v1 on a
// fakeComp: it answers the feedback requests with a table and tranches,
// and create with created or failed.
type fakeDmabuf struct {
	t       *testing.T
	version uint32
	// table and tranches are the default feedback: each tranche is a list
	// of indices into the table. device is the main device's dev_t.
	table    []platform.DmabufFormat
	tranches [][]uint16
	device   uint64
	// legacy are the pairs sent at the bind below version 4 (modifier
	// events from version 3, format events below).
	legacy []platform.DmabufFormat

	mu sync.Mutex
	// fail makes create answer failed; mute makes it answer nothing.
	fail, mute bool
	// surfaceFeedback is the latest feedback object asked for a surface.
	surfaceFeedback uint32
	// adds are the add requests of the params objects, and created the
	// wl_buffers made, in order.
	adds    map[uint32][]freq
	created []uint32
	// held are the params objects create was not answered for (mute).
	held []uint32
}

func (d *fakeDmabuf) global() fakeGlobal {
	return fakeGlobal{iface: "zwp_linux_dmabuf_v1", version: d.version, onBind: func(fc *fakeComp, id uint32) {
		for _, p := range d.legacy {
			if d.version >= 3 {
				fc.send(id, 1, p.Fourcc, uint32(p.Modifier>>32), uint32(p.Modifier)) // modifier
			} else {
				fc.send(id, 0, p.Fourcc) // format
			}
		}
	}}
}

// sendFeedback sends one whole feedback update on a feedback object: the
// table (when not nil), the main device, the tranches and done.
func (d *fakeDmabuf) sendFeedback(fc *fakeComp, id uint32, table []platform.DmabufFormat, tranches [][]uint16) {
	if table != nil {
		raw := make([]byte, 16*len(table))
		for i, p := range table {
			ne.PutUint32(raw[16*i:], p.Fourcc)
			ne.PutUint32(raw[16*i+4:], 0xdeadbeef) // padding: not read
			ne.PutUint64(raw[16*i+8:], p.Modifier)
		}
		fd, err := unix.MemfdCreate("fake-format-table", unix.MFD_CLOEXEC)
		if err != nil {
			d.t.Error(err)
			return
		}
		unix.Write(fd, raw)
		fc.sendFds(event(id, 1, uint32(len(raw))), fd) // format_table
		unix.Close(fd)
	}
	dev := ne.AppendUint64(nil, d.device)
	fc.send(id, 2, dev) // main_device
	for i, tr := range tranches {
		fc.send(id, 4, dev) // tranche_target_device
		flags := uint32(0)
		if i == 0 && len(tranches) > 1 {
			flags = uint32(protocols.ZwpLinuxDmabufFeedbackV1TrancheFlagsScanout)
		}
		fc.send(id, 6, flags) // tranche_flags
		var idx []byte
		for _, n := range tr {
			idx = ne.AppendUint16(idx, n)
		}
		fc.send(id, 5, idx) // tranche_formats
		fc.send(id, 3)      // tranche_done
	}
	fc.send(id, 0) // done
}

func (d *fakeDmabuf) respond(fc *fakeComp, r freq) {
	switch {
	case r.is("zwp_linux_dmabuf_v1.get_default_feedback"):
		d.sendFeedback(fc, r.u(0), d.table, d.tranches)
	case r.is("zwp_linux_dmabuf_v1.get_surface_feedback"):
		d.mu.Lock()
		d.surfaceFeedback = r.u(0)
		d.mu.Unlock()
	case r.is("zwp_linux_buffer_params_v1.add"):
		d.mu.Lock()
		if d.adds == nil {
			d.adds = map[uint32][]freq{}
		}
		d.adds[r.id] = append(d.adds[r.id], r)
		d.mu.Unlock()
	case r.is("zwp_linux_buffer_params_v1.create"):
		d.mu.Lock()
		fail, mute := d.fail, d.mute
		if mute {
			d.held = append(d.held, r.id)
		}
		d.mu.Unlock()
		switch {
		case mute:
		case fail:
			fc.send(r.id, 1) // failed
		default:
			d.answerCreated(fc, r.id)
		}
	case r.is("zwp_linux_buffer_params_v1.create_immed"):
		d.t.Errorf("the backend sent %v: a failed import would be a fatal error", r)
	}
}

// answerCreated sends created with a new wl_buffer for a params object.
func (d *fakeDmabuf) answerCreated(fc *fakeComp, params uint32) uint32 {
	buf := fc.newServerObject("wl_buffer")
	d.mu.Lock()
	d.created = append(d.created, buf)
	d.mu.Unlock()
	fc.send(params, 0, buf)
	return buf
}

func (d *fakeDmabuf) set(f func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f()
}

// lastBuffer returns the wl_buffer of the latest created.
func (d *fakeDmabuf) lastBuffer() uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.created) == 0 {
		d.t.Fatal("no dmabuf wl_buffer was created")
	}
	return d.created[len(d.created)-1]
}

// newFakeDmabuf is a version 4 compositor whose main device is 226:128
// (a render node) and which lists a scan-out tranche (XRGB tiled) ahead of
// its main tranche (XRGB tiled again, XRGB linear, ARGB tiled, and a
// duplicate table entry of XRGB linear).
func newFakeDmabuf(t *testing.T) *fakeDmabuf {
	return &fakeDmabuf{
		t:        t,
		version:  4,
		table:    []platform.DmabufFormat{xrgbLinear, xrgbTiled, argbTiled, xrgbLinear, xbgrY},
		tranches: [][]uint16{{1}, {1, 0, 2, 3, 99}},
		device:   unix.Mkdev(226, 128),
	}
}

// dmabufHarness is a harness on a compositor with fakeDmabuf.
func dmabufHarness(t *testing.T, d *fakeDmabuf, globals []fakeGlobal) *harness {
	t.Helper()
	h := newHarness(t, append(globals, d.global()))
	h.fc.setRespond(d.respond)
	return h
}

// fakePlanes returns one plane of a new memfd that stands for a dmabuf of
// a size.X by size.Y buffer of 4 bytes a pixel with the given stride, and
// closes it with the test.
func fakePlanes(t *testing.T, size image.Point, offset, stride uint32) []platform.DmabufPlane {
	t.Helper()
	fd, err := unix.MemfdCreate("fake-dmabuf", unix.MFD_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(fd) })
	if err := unix.Ftruncate(fd, int64(offset)+int64(stride)*int64(size.Y)); err != nil {
		t.Fatal(err)
	}
	return []platform.DmabufPlane{{FD: fd, Offset: offset, Stride: stride}}
}

// register registers a buffer of the window's current size on the UI
// thread and returns it with the wl_buffer the fake created for it.
func register(t *testing.T, h *harness, d *fakeDmabuf, w *Window, format platform.DmabufFormat) (platform.DmabufBuffer, uint32) {
	t.Helper()
	var (
		buf platform.DmabufBuffer
		err error
	)
	h.ui(func() {
		size := w.Metrics().Physical()
		buf, err = w.CreateDmabuf(platform.DmabufDescriptor{
			Size: size, Fourcc: format.Fourcc, Modifier: format.Modifier,
			Planes: fakePlanes(t, size, 0, uint32(4*size.X)),
		})
	})
	if err != nil {
		t.Fatalf("CreateDmabuf: %v", err)
	}
	h.fc.until("zwp_linux_buffer_params_v1.destroy")
	return buf, d.lastBuffer()
}

// sync makes the UI thread dispatch everything the fake sent so far, with
// a round trip. A plain ui call does not: the loop may run it on a
// WakeEvent before it reads the socket.
func (h *harness) sync() {
	h.t.Helper()
	h.ui(func() {
		if err := h.b.conn.RoundtripTimeout(3 * time.Second); err != nil {
			h.t.Errorf("round trip: %v", err)
		}
	})
}

// feedback asks the window for its feedback on the UI thread.
func feedback(h *harness, w *Window) (fb platform.DmabufFeedback, ok bool) {
	h.ui(func() { fb, ok = w.DmabufFeedback() })
	return fb, ok
}

// expectPanic runs f on the UI thread and checks that it panics with a
// message that contains want.
func expectPanic(t *testing.T, h *harness, want string, f func()) {
	t.Helper()
	h.ui(func() {
		defer func() {
			msg, _ := recover().(string)
			if !strings.Contains(msg, want) {
				t.Errorf("panic %q, want one that says %q", msg, want)
			}
		}()
		f()
	})
}

// damages returns the rectangles of the damage_buffer requests.
func damages(reqs []freq) []image.Rectangle {
	var out []image.Rectangle
	for _, r := range reqs {
		if r.is("wl_surface.damage_buffer") {
			out = append(out, image.Rect(int(r.i(0)), int(r.i(1)), int(r.i(0)+r.i(2)), int(r.i(1)+r.i(3))))
		}
	}
	return out
}

// TestDmabufBindsOnFirstUse: an app that presents with wl_shm never binds
// the global; the first DmabufFeedback binds it and asks for the default
// feedback, and for the surface's.
func TestDmabufBindsOnFirstUse(t *testing.T) {
	d := newFakeDmabuf(t)
	h := dmabufHarness(t, d, standardGlobals(1))
	presentFrames(h)
	w := openWindow(t, h, platform.WindowOptions{}, 300, 200, 1)
	for _, r := range h.fc.until("wl_surface.commit") {
		if strings.HasPrefix(r.iface, "zwp_linux_") {
			t.Errorf("a wl_shm app sent %v", r)
		}
	}
	h.ui(func() {
		if h.b.dmabuf.wl != nil {
			t.Error("zwp_linux_dmabuf_v1 is bound before its first use")
		}
	})

	fb, ok := feedback(h, w)
	if !ok {
		t.Fatal("no dmabuf feedback from a compositor with zwp_linux_dmabuf_v1 4")
	}
	reqs := h.fc.until("zwp_linux_dmabuf_v1.get_surface_feedback")
	bind, iBind := find(reqs, "wl_registry.bind")
	_, iDef := find(reqs, "zwp_linux_dmabuf_v1.get_default_feedback")
	if iBind < 0 || bind.args[1] != "zwp_linux_dmabuf_v1" || bind.args[2] != uint32(4) || iDef < iBind {
		t.Errorf("first use = %v, want the bind at version 4, then get_default_feedback", reqs)
	}
	if sf := reqs[len(reqs)-1]; sf.u(1) != h.fc.id("wl_surface") {
		t.Errorf("get_surface_feedback = %v, want the window's surface #%d", sf, h.fc.id("wl_surface"))
	}

	// The default feedback: the main device, and the tranches in order
	// without duplicates (the scan-out tranche's pair first; index 3
	// repeats a table entry and 99 is outside the table).
	want := []platform.DmabufFormat{xrgbTiled, xrgbLinear, argbTiled}
	if !fb.HasDevice || fb.DeviceMajor != 226 || fb.DeviceMinor != 128 || !slices.Equal(fb.Formats, want) || fb.Serial == 0 {
		t.Errorf("feedback = %+v, want device 226:128 and %v", fb, want)
	}
	if !fb.Accepts(platform.DRMFormatXRGB8888, modTiled) || fb.Accepts(platform.DRMFormatXBGR8888, modY) {
		t.Errorf("Accepts is wrong for %v", fb.Formats)
	}
	if got := fb.Modifiers(platform.DRMFormatXRGB8888); !slices.Equal(got, []uint64{modTiled, platform.DRMModifierLinear}) {
		t.Errorf("Modifiers(XRGB) = %#x", got)
	}
}

// TestDmabufSurfaceFeedback: the surface's feedback replaces the default
// one; the Serial changes with the content, and only with it.
func TestDmabufSurfaceFeedback(t *testing.T) {
	d := newFakeDmabuf(t)
	h := dmabufHarness(t, d, standardGlobals(1))
	presentFrames(h)
	w := openWindow(t, h, platform.WindowOptions{}, 300, 200, 1)
	first, _ := feedback(h, w)
	h.fc.until("zwp_linux_dmabuf_v1.get_surface_feedback")
	d.mu.Lock()
	sf := d.surfaceFeedback
	d.mu.Unlock()

	// The same content, with a table of its own: nothing changes.
	d.sendFeedback(h.fc, sf, d.table, d.tranches)
	h.sync()
	h.ui(func() {
		if !w.dmabuf.feedback.ready {
			t.Error("the surface's feedback did not arrive")
		}
	})
	if same, ok := feedback(h, w); !ok || same.Serial != first.Serial {
		t.Errorf("a surface feedback equal to the default one: serial %d, was %d", same.Serial, first.Serial)
	}

	// An update without a table: the indices read the table kept. The
	// compositor now prefers the Y-tiled XBGR and drops the linear pair.
	d.device = unix.Mkdev(226, 129)
	d.sendFeedback(h.fc, sf, nil, [][]uint16{{4}, {1, 2}})
	h.sync()
	next, ok := feedback(h, w)
	want := []platform.DmabufFormat{xbgrY, xrgbTiled, argbTiled}
	if !ok || next.Serial == first.Serial || !slices.Equal(next.Formats, want) || next.DeviceMinor != 129 {
		t.Errorf("updated feedback = %+v (was serial %d), want a new serial, device 226:129 and %v", next, first.Serial, want)
	}
	if again, _ := feedback(h, w); again.Serial != next.Serial {
		t.Errorf("an unchanged feedback changed its serial: %d, was %d", again.Serial, next.Serial)
	}
	// The first answer is untouched: its formats are not overwritten.
	if !slices.Equal(first.Formats, []platform.DmabufFormat{xrgbTiled, xrgbLinear, argbTiled}) {
		t.Errorf("the earlier feedback's formats changed: %v", first.Formats)
	}

	// A second window has its own feedback object, and starts from the
	// default feedback.
	var w2 *Window
	h.ui(func() {
		win, err := h.b.NewWindow(platform.WindowOptions{})
		if err != nil {
			t.Fatal(err)
		}
		w2 = win.(*Window)
	})
	other, ok := feedback(h, w2)
	if !ok || !slices.Equal(other.Formats, first.Formats) || other.DeviceMinor != 128 {
		t.Errorf("a second window's feedback = %+v, want the default one", other)
	}
}

// TestDmabufLegacyVersions: version 3 lists pairs with modifier events
// and has no main device; version 2 lists formats, taken with the
// implicit modifier.
func TestDmabufLegacyVersions(t *testing.T) {
	for _, tc := range []struct {
		version uint32
		want    []platform.DmabufFormat
	}{
		{3, []platform.DmabufFormat{xrgbTiled, xrgbLinear}},
		{2, []platform.DmabufFormat{
			{Fourcc: platform.DRMFormatXRGB8888, Modifier: platform.DRMModifierInvalid},
		}},
	} {
		d := &fakeDmabuf{t: t, version: tc.version, legacy: []platform.DmabufFormat{xrgbTiled, xrgbLinear, xrgbTiled}}
		h := dmabufHarness(t, d, standardGlobals(1))
		presentFrames(h)
		w := openWindow(t, h, platform.WindowOptions{}, 300, 200, 1)
		fb, ok := feedback(h, w)
		if !ok || fb.HasDevice || !slices.Equal(fb.Formats, tc.want) || fb.Serial == 0 {
			t.Errorf("version %d: feedback = %+v, %v; want %v and no device", tc.version, fb, ok, tc.want)
		}
		reqs := h.fc.quiet(30*time.Millisecond, "zwp_linux_dmabuf_v1.get_default_feedback")
		if bind, i := find(reqs, "wl_registry.bind"); i < 0 || bind.args[2] != tc.version {
			t.Errorf("version %d: bind = %v", tc.version, bind)
		}
		if again, _ := feedback(h, w); again.Serial != fb.Serial {
			t.Errorf("version %d: the serial changed without a change", tc.version)
		}
		// Buffers are registered the same way.
		buf, _ := register(t, h, d, w, tc.want[0])
		if buf.Size() != image.Pt(300, 200) {
			t.Errorf("version %d: buffer size %v", tc.version, buf.Size())
		}
	}
}

// TestDmabufUnsupported: what makes a window say that it cannot show
// dmabufs, and what CreateDmabuf refuses before it sends anything.
func TestDmabufUnsupported(t *testing.T) {
	desc := func(w *Window, f platform.DmabufFormat) platform.DmabufDescriptor {
		size := w.Metrics().Physical()
		return platform.DmabufDescriptor{Size: size, Fourcc: f.Fourcc, Modifier: f.Modifier, Planes: fakePlanes(t, size, 0, uint32(4*size.X))}
	}
	unsupported := func(t *testing.T, h *harness, w *Window, d platform.DmabufDescriptor, what string) {
		t.Helper()
		h.ui(func() {
			if buf, err := w.CreateDmabuf(d); !errors.Is(err, platform.ErrUnsupported) || buf != nil {
				t.Errorf("%s: CreateDmabuf = %v, %v; want an error that wraps ErrUnsupported", what, buf, err)
			}
		})
		h.fc.quiet(30*time.Millisecond, "zwp_linux_dmabuf_v1.create_params")
	}

	t.Run("no global", func(t *testing.T) {
		h := newHarness(t, standardGlobals(1))
		presentFrames(h)
		w := openWindow(t, h, platform.WindowOptions{}, 300, 200, 1)
		waitFor[platform.FrameEvent](h)
		if fb, ok := feedback(h, w); ok {
			t.Errorf("feedback %+v without zwp_linux_dmabuf_v1", fb)
		}
		unsupported(t, h, w, desc(w, xrgbTiled), "no global")
	})

	t.Run("transparent window", func(t *testing.T) {
		d := newFakeDmabuf(t)
		h := dmabufHarness(t, d, standardGlobals(1))
		presentFrames(h)
		w, _ := openCtl(t, h, platform.WindowOptions{Transparent: true, Frameless: true}, 300, 200, 1)
		waitFor[platform.FrameEvent](h)
		if fb, ok := feedback(h, w); ok {
			t.Errorf("feedback %+v for a transparent window, whose input region needs the pixels", fb)
		}
		unsupported(t, h, w, desc(w, argbTiled), "transparent window")
		h.fc.quiet(30*time.Millisecond, "wl_registry.bind")
	})

	t.Run("descriptors", func(t *testing.T) {
		d := newFakeDmabuf(t)
		h := dmabufHarness(t, d, standardGlobals(1))
		presentFrames(h)
		w := openWindow(t, h, platform.WindowOptions{}, 300, 200, 1)
		waitFor[platform.FrameEvent](h)
		feedback(h, w)
		h.fc.until("zwp_linux_dmabuf_v1.get_surface_feedback")

		// Not in the feedback: the table has it, no tranche does.
		unsupported(t, h, w, desc(w, xbgrY), "a pair no tranche lists")
		// A known format with another modifier.
		unsupported(t, h, w, desc(w, platform.DmabufFormat{Fourcc: platform.DRMFormatXRGB8888, Modifier: modY}), "an unlisted modifier")
		// Alpha on an opaque window.
		alpha := desc(w, argbTiled)
		alpha.Alpha = true
		unsupported(t, h, w, alpha, "Alpha")

		// Planes that do not fit their dmabuf would be out_of_bounds, a
		// fatal error: an error here, and nothing sent.
		for what, change := range map[string]func(*platform.DmabufPlane){
			"an offset past the end":  func(p *platform.DmabufPlane) { p.Offset = 4 * 300 * 200 },
			"rows longer than the fd": func(p *platform.DmabufPlane) { p.Stride = 4*300 + 4 },
			"a size that overflows":   func(p *platform.DmabufPlane) { p.Offset, p.Stride = 1<<31, 1<<31 },
		} {
			bad := desc(w, xrgbTiled)
			change(&bad.Planes[0])
			h.ui(func() {
				if buf, err := w.CreateDmabuf(bad); err == nil || errors.Is(err, platform.ErrUnsupported) || buf != nil {
					t.Errorf("%s: CreateDmabuf = %v, %v; want a plain error", what, buf, err)
				}
			})
		}
		h.fc.quiet(30*time.Millisecond, "zwp_linux_dmabuf_v1.create_params")

		// What cannot describe a buffer at all panics.
		expectPanic(t, h, "planes", func() {
			w.CreateDmabuf(platform.DmabufDescriptor{Size: image.Pt(300, 200), Fourcc: platform.DRMFormatXRGB8888})
		})
		expectPanic(t, h, "size", func() {
			bad := desc(w, xrgbTiled)
			bad.Size = image.Pt(0, 200)
			w.CreateDmabuf(bad)
		})
	})
}

// TestDmabufCreate: the requests of a registration, its failure, and a
// compositor that does not answer.
func TestDmabufCreate(t *testing.T) {
	d := newFakeDmabuf(t)
	h := dmabufHarness(t, d, standardGlobals(1))
	presentFrames(h)
	w := openWindow(t, h, platform.WindowOptions{}, 300, 200, 1)
	waitFor[platform.FrameEvent](h)
	h.fc.until("wl_surface.commit")
	feedback(h, w)
	h.fc.until("zwp_linux_dmabuf_v1.get_surface_feedback")

	// Two planes in one dmabuf, as a compressed modifier has.
	size := image.Pt(300, 200)
	planes := fakePlanes(t, size, 64, 1216)
	planes = append(planes, platform.DmabufPlane{FD: planes[0].FD, Offset: 16, Stride: 32})
	var buf platform.DmabufBuffer
	h.ui(func() {
		var err error
		buf, err = w.CreateDmabuf(platform.DmabufDescriptor{Size: size, Fourcc: platform.DRMFormatXRGB8888, Modifier: modTiled, Planes: planes})
		if err != nil {
			t.Fatalf("CreateDmabuf: %v", err)
		}
		if buf.Busy() || buf.Size() != size {
			t.Errorf("a new buffer: busy %v, size %v", buf.Busy(), buf.Size())
		}
	})
	reqs := h.fc.until("zwp_linux_buffer_params_v1.destroy")
	var seq []string
	for _, r := range reqs {
		if strings.HasPrefix(r.iface, "zwp_linux_") || r.is("wl_display.sync") {
			seq = append(seq, r.name)
		}
	}
	if want := []string{"create_params", "add", "add", "create", "sync", "destroy"}; !slices.Equal(seq, want) {
		t.Errorf("registration = %v, want %v", seq, want)
	}
	params := reqs[len(reqs)-1].id
	d.mu.Lock()
	adds := d.adds[params]
	d.mu.Unlock()
	for i, want := range [][5]uint32{{0, 64, 1216, modTiled >> 32, modTiled & 0xffffffff}, {1, 16, 32, modTiled >> 32, modTiled & 0xffffffff}} {
		a := adds[i]
		if got := [5]uint32{a.u(1), a.u(2), a.u(3), a.u(4), a.u(5)}; got != want {
			t.Errorf("add %d = %v, want plane, offset, stride, modifier hi and lo %v", i, got, want)
		}
		// The compositor got a descriptor of the same file, and the
		// caller's is still open: it was duplicated, not taken.
		var sent, own unix.Stat_t
		if err := unix.Fstat(int(a.args[0].(fdArg)), &sent); err != nil {
			t.Fatalf("add %d carried no descriptor: %v", i, err)
		}
		if err := unix.Fstat(planes[i].FD, &own); err != nil {
			t.Errorf("the caller's descriptor was closed: %v", err)
		}
		if sent.Ino != own.Ino {
			t.Errorf("add %d carried another file than the plane's", i)
		}
		unix.Close(int(a.args[0].(fdArg)))
	}
	create, _ := find(reqs, "zwp_linux_buffer_params_v1.create")
	if create.i(0) != 300 || create.i(1) != 200 || create.u(2) != platform.DRMFormatXRGB8888 || create.u(3) != 0 {
		t.Errorf("create = %v, want 300x200 XRGB8888 without flags", create)
	}

	// The compositor cannot import: an error that is not ErrUnsupported
	// (the path works, this buffer does not), and nothing left behind.
	d.set(func() { d.fail = true })
	h.ui(func() {
		got, err := w.CreateDmabuf(platform.DmabufDescriptor{Size: size, Fourcc: platform.DRMFormatXRGB8888, Modifier: modTiled, Planes: planes[:1]})
		if err == nil || errors.Is(err, platform.ErrUnsupported) || got != nil {
			t.Errorf("a failed import: CreateDmabuf = %v, %v", got, err)
		}
		if n := len(w.dmabuf.bufs); n != 1 {
			t.Errorf("%d buffers registered after a failure, want the first one only", n)
		}
	})
	h.fc.until("zwp_linux_buffer_params_v1.destroy")

	// The compositor answers create with neither event before the round
	// trips end (it imports later): the call gives up, and the buffer
	// created afterwards is destroyed.
	d.set(func() { d.fail, d.mute = false, true })
	h.ui(func() {
		got, err := w.CreateDmabuf(platform.DmabufDescriptor{Size: size, Fourcc: platform.DRMFormatXRGB8888, Modifier: modTiled, Planes: planes[:1]})
		if err == nil || got != nil {
			t.Errorf("an unanswered create: CreateDmabuf = %v, %v", got, err)
		}
	})
	h.fc.until("zwp_linux_buffer_params_v1.create")
	d.mu.Lock()
	held := d.held[0]
	d.mu.Unlock()
	late := d.answerCreated(h.fc, held)
	reqs = h.fc.until("zwp_linux_buffer_params_v1.destroy")
	if r, i := find(reqs, "wl_buffer.destroy"); i < 0 || r.id != late {
		t.Errorf("a buffer created after the call gave up: %v, want wl_buffer#%d destroyed", reqs, late)
	}
	h.ui(func() {
		if n := len(w.dmabuf.bufs); n != 1 {
			t.Errorf("%d buffers registered, want 1", n)
		}
	})

	// Close destroys what is registered; destroying it again does nothing.
	first := d.created[0]
	h.ui(func() { w.Close() })
	reqs = h.fc.until("zwp_linux_dmabuf_feedback_v1.destroy")
	if !slices.ContainsFunc(reqs, func(r freq) bool { return r.is("wl_buffer.destroy") && r.id == first }) {
		t.Errorf("close = %v, want wl_buffer#%d destroyed", reqs, first)
	}
	h.ui(func() {
		buf.Destroy()
		if buf.Busy() {
			t.Error("a destroyed buffer is busy")
		}
		if _, ok := w.DmabufFeedback(); ok {
			t.Error("a closed window reports dmabuf feedback")
		}
	})
	h.fc.quiet(30*time.Millisecond, "wl_buffer.destroy")
}

// TestDmabufPresent: what a present sends and in which order, damage,
// release, and the panics of the contract.
func TestDmabufPresent(t *testing.T) {
	d := newFakeDmabuf(t)
	h := dmabufHarness(t, d, fractionalGlobals(1))
	win, err := h.b.NewWindow(platform.WindowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	h.start()
	h.fc.until("wl_surface.commit")
	h.preferredScale(180) // 1.5
	h.configure(300, 200, 5)
	if m := waitFor[platform.ConfigureEvent](h).Metrics; m.Physical() != image.Pt(450, 300) {
		t.Fatalf("metrics %+v, want 450x300 physical", m)
	}
	waitFor[platform.FrameEvent](h)
	if _, ok := feedback(h, w); !ok {
		t.Fatal("no feedback")
	}
	a, aID := register(t, h, d, w, xrgbTiled)
	b, bID := register(t, h, d, w, xrgbTiled)

	// The first present: the ack of the configure, the viewport, the
	// opaque region, then the buffer with the whole of it damaged
	// (whatever the damage says: nothing was shown yet), a frame callback
	// and the commit. It maps the window.
	h.ui(func() {
		if err := w.PresentDmabuf(a, []image.Rectangle{image.Rect(0, 0, 10, 10)}); err != nil {
			t.Errorf("PresentDmabuf: %v", err)
		}
		if !a.Busy() || b.Busy() {
			t.Errorf("after presenting a: a busy %v, b busy %v", a.Busy(), b.Busy())
		}
		if !w.mapped {
			t.Error("the first dmabuf present did not map the window")
		}
	})
	reqs := h.fc.until("wl_surface.commit")
	var order []string
	for _, r := range reqs {
		switch r.iface + "." + r.name {
		case "xdg_surface.ack_configure", "wp_viewport.set_destination", "wl_surface.set_opaque_region",
			"wl_surface.attach", "wl_surface.damage_buffer", "wl_surface.frame", "wl_surface.commit":
			order = append(order, r.name)
		}
	}
	if want := []string{"ack_configure", "set_destination", "set_opaque_region", "attach", "damage_buffer", "frame", "commit"}; !slices.Equal(order, want) {
		t.Errorf("first present = %v, want %v", order, want)
	}
	if att, _ := find(reqs, "wl_surface.attach"); att.u(0) != aID {
		t.Errorf("attached #%d, want the buffer #%d", att.u(0), aID)
	}
	if dst, _ := find(reqs, "wp_viewport.set_destination"); dst.i(0) != 300 || dst.i(1) != 200 {
		t.Errorf("viewport destination = %v, want the logical 300x200", dst)
	}
	if got := damages(reqs); !slices.Equal(got, []image.Rectangle{image.Rect(0, 0, 450, 300)}) {
		t.Errorf("first damage = %v, want the whole 450x300 buffer", got)
	}
	region, _ := find(reqs, "wl_region.add")
	if region.i(0) != 0 || region.i(1) != 0 || region.i(2) < 450 || region.i(3) < 300 {
		t.Errorf("opaque region = %v, want all of the surface", region)
	}
	frame, _ := find(reqs, "wl_surface.frame")

	// A busy buffer is not presented again; the other one is, with its
	// damage in buffer pixels, clipped, and nothing else set again.
	expectPanic(t, h, "busy", func() { w.PresentDmabuf(a, nil) })
	h.fc.send(frame.u(0), 0, uint32(0)) // the frame callback
	h.ui(func() {
		if err := w.PresentDmabuf(b, []image.Rectangle{image.Rect(10, 20, 110, 70), image.Rect(400, 250, 500, 400)}); err != nil {
			t.Errorf("PresentDmabuf: %v", err)
		}
	})
	reqs = h.fc.until("wl_surface.commit")
	for _, r := range reqs {
		if r.is("xdg_surface.ack_configure") || r.is("wp_viewport.set_destination") || r.is("wl_surface.set_opaque_region") {
			t.Errorf("the second present sent %v again", r)
		}
	}
	if att, _ := find(reqs, "wl_surface.attach"); att.u(0) != bID {
		t.Errorf("attached #%d, want the buffer #%d", att.u(0), bID)
	}
	if got, want := damages(reqs), []image.Rectangle{image.Rect(10, 20, 110, 70), image.Rect(400, 250, 450, 300)}; !slices.Equal(got, want) {
		t.Errorf("damage = %v, want %v", got, want)
	}

	// The compositor releases the first buffer: it is free again, on the
	// UI thread, without an event.
	h.fc.send(aID, 0) // wl_buffer.release
	h.sync()
	h.ui(func() {
		if a.Busy() || !b.Busy() {
			t.Errorf("after the release of a: a busy %v, b busy %v", a.Busy(), b.Busy())
		}
	})
	h.noEvent(30 * time.Millisecond)

	// Damage that is clipped away presents nothing and leaves the buffer
	// free.
	h.ui(func() {
		if err := w.PresentDmabuf(a, []image.Rectangle{image.Rect(500, 500, 600, 600)}); err != nil || a.Busy() {
			t.Errorf("damage outside the buffer: %v, busy %v", err, a.Busy())
		}
	})
	h.fc.quiet(30*time.Millisecond, "wl_surface.attach")

	// Another window's buffer, and a destroyed one.
	var w2 *Window
	h.ui(func() {
		win, err := h.b.NewWindow(platform.WindowOptions{})
		if err != nil {
			t.Fatal(err)
		}
		w2 = win.(*Window)
	})
	expectPanic(t, h, "not this window's", func() { w2.PresentDmabuf(a, nil) })
	expectPanic(t, h, "not this window's", func() { w.PresentDmabuf(nil, nil) })
	h.ui(func() { a.Destroy() })
	if r := h.fc.until("wl_buffer.destroy"); r[len(r)-1].id != aID {
		t.Errorf("Destroy destroyed %v, want wl_buffer#%d", r[len(r)-1], aID)
	}
	expectPanic(t, h, "destroyed", func() { w.PresentDmabuf(a, nil) })
}

// TestDmabufResize: after a ConfigureEvent the old buffers cannot be
// presented; new ones answer the configure, and the old ones are destroyed
// while the compositor still holds one.
func TestDmabufResize(t *testing.T) {
	d := newFakeDmabuf(t)
	h := dmabufHarness(t, d, standardGlobals(1))
	w := openWindow(t, h, platform.WindowOptions{}, 300, 200, 1)
	waitFor[platform.FrameEvent](h)
	feedback(h, w)
	a, aID := register(t, h, d, w, xrgbTiled)
	h.ui(func() { w.PresentDmabuf(a, nil) })
	h.fc.until("wl_surface.commit")

	h.configure(400, 250, 2)
	waitFor[platform.ConfigureEvent](h)
	waitFor[platform.FrameEvent](h)
	expectPanic(t, h, "300x200 buffer on a window of 400x250", func() { w.PresentDmabuf(a, nil) })

	// The old buffer is destroyed while busy; its late release is dropped.
	h.ui(func() { a.Destroy() })
	h.fc.until("wl_buffer.destroy")
	h.fc.send(aID, 0) // wl_buffer.release for the destroyed proxy
	b, bID := register(t, h, d, w, xrgbTiled)
	h.ui(func() {
		if b.Size() != image.Pt(400, 250) {
			t.Errorf("new buffer size %v", b.Size())
		}
		if err := w.PresentDmabuf(b, []image.Rectangle{image.Rect(0, 0, 5, 5)}); err != nil {
			t.Errorf("PresentDmabuf: %v", err)
		}
	})
	reqs := h.fc.until("wl_surface.commit")
	ack, iAck := find(reqs, "xdg_surface.ack_configure")
	att, iAtt := find(reqs, "wl_surface.attach")
	if iAck < 0 || ack.u(0) != 2 || iAtt < iAck || att.u(0) != bID {
		t.Errorf("present after the resize = %v, want ack_configure(2), then the new buffer #%d", reqs, bID)
	}
	if got := damages(reqs); !slices.Equal(got, []image.Rectangle{image.Rect(0, 0, 400, 250)}) {
		t.Errorf("damage after a ConfigureEvent = %v, want the whole buffer", got)
	}
	// The loop is alive: the dropped release was not a protocol error.
	h.ui(func() {
		if err := h.b.conn.Err(); err != nil {
			t.Errorf("connection error: %v", err)
		}
	})
}

// TestDmabufAlternatesWithShm: Present and PresentDmabuf follow each
// other on one window, both ways.
func TestDmabufAlternatesWithShm(t *testing.T) {
	d := newFakeDmabuf(t)
	h := dmabufHarness(t, d, standardGlobals(1, protocols.WlShmFormatXbgr8888, protocols.WlShmFormatXrgb8888, protocols.WlShmFormatArgb8888))
	w := openWindow(t, h, platform.WindowOptions{}, 300, 200, 1)
	waitFor[platform.FrameEvent](h)
	whole := []image.Rectangle{image.Rect(0, 0, 300, 200)}
	small := []image.Rectangle{image.Rect(10, 10, 20, 20)}

	// wl_shm first.
	h.ui(func() { w.Present(solid(w, 1, 2, 3), nil) })
	reqs := h.fc.until("wl_surface.commit")
	shm1, _ := find(reqs, "wl_surface.attach")
	h.ui(func() {
		if _, ok := w.DmabufFeedback(); !ok {
			t.Error("no feedback")
		}
	})
	a, aID := register(t, h, d, w, xrgbTiled)
	b, bID := register(t, h, d, w, xrgbTiled)

	// A dmabuf after wl_shm: the whole buffer is damaged, and the wl_shm
	// swapchain is given up (its busy buffer when the compositor
	// releases it).
	h.ui(func() {
		if err := w.PresentDmabuf(a, small); err != nil {
			t.Error(err)
		}
		if n := len(w.chain.bufs) + len(w.chain.retired); n != 1 || len(w.chain.bufs) != 0 {
			t.Errorf("the wl_shm chain holds %d buffers and %d retired, want only the busy one, retired", len(w.chain.bufs), len(w.chain.retired))
		}
	})
	reqs = h.fc.until("wl_surface.commit")
	if att, _ := find(reqs, "wl_surface.attach"); att.u(0) != aID {
		t.Errorf("attached #%d, want the dmabuf #%d", att.u(0), aID)
	}
	if got := damages(reqs); !slices.Equal(got, whole) {
		t.Errorf("damage of a dmabuf after wl_shm = %v, want the whole buffer", got)
	}
	h.fc.send(shm1.u(0), 0) // release
	if r := h.fc.until("wl_buffer.destroy"); r[len(r)-1].id != shm1.u(0) {
		t.Errorf("destroyed %v, want the released wl_shm buffer #%d", r[len(r)-1], shm1.u(0))
	}

	// Dmabufs follow each other with their damage.
	h.ui(func() { w.PresentDmabuf(b, small) })
	reqs = h.fc.until("wl_surface.commit")
	if got := damages(reqs); !slices.Equal(got, small) {
		t.Errorf("damage of the second dmabuf = %v, want %v", got, small)
	}
	if att, _ := find(reqs, "wl_surface.attach"); att.u(0) != bID {
		t.Errorf("attached #%d, want the dmabuf #%d", att.u(0), bID)
	}

	// wl_shm after a dmabuf: a new buffer with the whole framebuffer,
	// whatever the damage says.
	h.ui(func() {
		if err := w.Present(solid(w, 9, 8, 7), small); err != nil {
			t.Error(err)
		}
	})
	reqs = h.fc.until("wl_surface.commit")
	shm2, _ := find(reqs, "wl_surface.attach")
	if shm2.u(0) == aID || shm2.u(0) == bID || shm2.u(0) == shm1.u(0) {
		t.Errorf("Present after dmabufs attached #%d, want a new wl_shm buffer", shm2.u(0))
	}
	if got := damages(reqs); !slices.Equal(got, whole) {
		t.Errorf("damage of wl_shm after a dmabuf = %v, want the whole buffer", got)
	}
	px := h.fc.pixels(shm2.u(0))
	for _, at := range []int{0, 4 * (300*100 + 150), 4 * (300*200 - 1)} {
		if px[at] != 9 || px[at+1] != 8 || px[at+2] != 7 {
			t.Errorf("wl_shm pixel at byte %d = %v, want the new frame's 9, 8, 7 everywhere", at, px[at:at+4])
		}
	}
	// And a partial wl_shm present after it is partial again.
	h.ui(func() { w.Present(solid(w, 9, 8, 7), small) })
	if got := damages(h.fc.until("wl_surface.commit")); !slices.Equal(got, small) {
		t.Errorf("damage of the second wl_shm present = %v, want %v", got, small)
	}

	// A dmabuf again: whole.
	h.fc.send(aID, 0)
	h.sync()
	h.ui(func() {
		if a.Busy() {
			t.Error("a is busy after its release")
		}
		w.PresentDmabuf(a, small)
	})
	reqs = h.fc.until("wl_surface.commit")
	if got := damages(reqs); !slices.Equal(got, whole) {
		t.Errorf("damage of a dmabuf after wl_shm = %v, want the whole buffer", got)
	}
}

// TestDmabufHideShow: buffers survive a hide; a present while hidden is
// dropped; shown again, the window asks for a frame and is mapped by its
// present (it has no frame of its own to map with).
func TestDmabufHideShow(t *testing.T) {
	d := newFakeDmabuf(t)
	h := dmabufHarness(t, d, standardGlobals(1))
	w, _ := openCtl(t, h, platform.WindowOptions{}, 300, 200, 1, protocols.XdgToplevelStateActivated)
	waitFor[platform.FrameEvent](h)
	feedback(h, w)
	a, aID := register(t, h, d, w, xrgbTiled)
	b, bID := register(t, h, d, w, xrgbTiled)
	h.ui(func() { w.PresentDmabuf(a, nil) })
	h.fc.until("wl_surface.commit")

	// Hide: a null buffer, the role objects go, the wl_buffers stay.
	h.ui(func() { w.SetVisible(false) })
	reqs := h.fc.until("xdg_surface.destroy")
	if att, i := find(reqs, "wl_surface.attach"); i < 0 || att.u(0) != 0 {
		t.Errorf("hide = %v, want a null buffer attached", names(reqs))
	}
	if _, i := find(reqs, "wl_buffer.destroy"); i >= 0 {
		t.Errorf("hide destroyed a buffer: %v", names(reqs))
	}
	h.fc.send(aID, 0) // the compositor releases what it showed
	h.sync()

	// A present while hidden is dropped and leaves the buffer free.
	h.ui(func() {
		if a.Busy() {
			t.Error("a is busy after its release")
		}
		if err := w.PresentDmabuf(b, nil); err != nil || b.Busy() {
			t.Errorf("a present while hidden: %v, busy %v", err, b.Busy())
		}
	})
	h.fc.quiet(30*time.Millisecond, "wl_surface.attach")

	// Show: new role objects, and at their configure no buffer of the
	// window's own, but a frame for the app.
	h.ui(func() { w.SetVisible(true) })
	reqs = h.fc.until("wl_surface.commit")
	newXs, newTop := ids(t, reqs)
	for len(h.events) > 0 {
		<-h.events
	}
	h.fc.sendRaw(configureFor(newTop, newXs, 300, 200, 9, protocols.XdgToplevelStateActivated))
	waitFor[platform.FrameEvent](h)
	h.ui(func() {
		if w.mapped {
			t.Error("the window was mapped without a frame")
		}
		if err := w.PresentDmabuf(b, []image.Rectangle{image.Rect(0, 0, 4, 4)}); err != nil {
			t.Error(err)
		}
		if !w.mapped || !b.Busy() {
			t.Errorf("after the present: mapped %v, busy %v", w.mapped, b.Busy())
		}
	})
	reqs = h.fc.until("wl_surface.commit")
	for _, r := range reqs {
		if r.is("wl_surface.attach") && r.u(0) != bID {
			t.Errorf("show attached #%d before the app's buffer #%d", r.u(0), bID)
		}
	}
	ack, iAck := find(reqs, "xdg_surface.ack_configure")
	_, iAtt := find(reqs, "wl_surface.attach")
	if iAck < 0 || ack.id != newXs || ack.u(0) != 9 || iAtt < iAck {
		t.Errorf("remap = %v, want ack_configure(9) on #%d, then the buffer", reqs, newXs)
	}
	if got := damages(reqs); !slices.Equal(got, []image.Rectangle{image.Rect(0, 0, 300, 200)}) {
		t.Errorf("damage of the frame that maps = %v, want the whole buffer", got)
	}
}
