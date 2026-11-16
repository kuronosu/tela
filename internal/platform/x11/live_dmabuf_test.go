//go:build linux && telagpu

package x11

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/dmabuftest"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
	"github.com/kuronosu/tela/internal/vk"
)

// The live check of dmabuf.go, with the Vulkan bindings on the other side
// (ADR-281, ADR-283; the device that renders is the package dmabuftest,
// shared with the Wayland backend): what the unit tests cannot tell is
// whether a real X server imports what a real driver exports, and what it
// does with PresentPixmap. It needs TELA_LIVE=1 and the tag telagpu:
//
//	go test -c -tags telagpu -o bin/x11.test ./internal/platform/x11
//	TELA_LIVE=1 DISPLAY=:0 bin/x11.test -test.run TestLiveDmabuf -test.v
//
// (a test binary in a sandboxed temporary directory is not visible to
// Hyprland). The windows open silently on the workspace nobody looks at
// and take no focus and no input.
//
// Unlike the Wayland test this one sees its pixels wherever the window
// is: GetImage reads the X server's copy of the window, which Xwayland
// keeps for a window on a workspace no monitor shows. Every frame is
// compared, pixel by pixel, with the scene the device rendered.

const liveDmabufClass = "dev.telaui.livex11dmabuf"

// placeDmabufWindow makes the test's windows open silently on the hidden
// workspace on Hyprland, and skips the test when it cannot: they must not
// take the focus on the workspace in use.
func placeDmabufWindow(t *testing.T) {
	t.Helper()
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return
	}
	ws := os.Getenv("TELA_LIVE_WORKSPACE")
	if ws == "" {
		ws = "7"
	}
	rule := `tela_live_x11 = hl.window_rule({ name = "tela-live-x11", match = { class = "` + liveDmabufClass + `" }, workspace = "` + ws + ` silent" })`
	if out, err := exec.Command("hyprctl", "eval", rule).CombinedOutput(); err != nil || strings.Contains(strings.ToLower(string(out)), "error") {
		t.Skipf("hyprctl eval (window rule): %v %s; not opening a window that would take the focus", err, out)
	}
	t.Cleanup(func() {
		exec.Command("hyprctl", "eval", `if tela_live_x11 then tela_live_x11:set_enabled(false) end`).Run()
	})
}

// TestLiveDmabuf presents Vulkan images to the real X server as dmabufs,
// once per physical device. On the device DRI3 names every step must
// work. On another device the buffers cross from one driver to another,
// which needs a modifier both have, or a linear image the server takes
// without a modifier: what happens there is logged and never a failure.
func TestLiveDmabuf(t *testing.T) {
	if os.Getenv("TELA_LIVE") != "1" {
		t.Skip("set TELA_LIVE=1 to run against the real X server")
	}
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY is not set")
	}
	inst, report := dmabuftest.NewInstance(t, "tela live dmabuf")
	devices, err := inst.Devices()
	if err != nil || len(devices) == 0 {
		t.Skipf("no usable Vulkan device: %v", err)
	}
	placeDmabufWindow(t)

	main := liveMainDevice(t, devices)
	for i, info := range devices {
		own := i == main
		name := info.Name
		if own {
			name += " (the server's device)"
		} else {
			name += " (another device)"
		}
		t.Run(name, func(t *testing.T) {
			liveDmabuf(t, inst, info, own)
			report(t)
		})
	}
}

// liveMainDevice opens a backend only to read the feedback, logs it, and
// returns the index of the Vulkan device it names (-1 when it names none
// of them).
func liveMainDevice(t *testing.T, devices []vk.DeviceInfo) int {
	b := liveBackend(t)
	defer b.conn.Close()
	win, err := b.NewWindow(platform.WindowOptions{Title: "tela live dmabuf feedback", AppID: liveDmabufClass, Hidden: true})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	fb, ok := w.DmabufFeedback()
	caps := b.dmabufCaps()
	if !ok {
		t.Skipf("the server presents no dmabufs: %s", caps.reason)
	}
	t.Logf("DRI3 %d.%d and Present %d.%d negotiated (the client asks for %d.%d and %d.%d); update regions (XFixes): %v",
		caps.dri3Major, caps.dri3Minor, caps.presentMajor, caps.presentMinor,
		dri3ClientMajor, dri3ClientMinor, presentClientMajor, presentClientMinor, caps.regions)
	t.Logf("depth %d at %d bits per pixel: %d format and modifier pairs", w.depth, dmabufBpp, len(fb.Formats))
	for _, f := range dmabuftest.Formats {
		t.Logf("  %s: modifiers %#x", f.Name, fb.Modifiers(f.Fourcc))
	}
	if !fb.HasDevice {
		main, _ := vk.SelectDevice(devices, "")
		t.Logf("DRI3 Open named no device; taking Vulkan device %d for it", main)
		return main
	}
	main := -1
	for i, d := range devices {
		if dmabuftest.SameDevice(fb, d) {
			main = i
		}
		t.Logf("  Vulkan device %d: %s", i, d)
	}
	t.Logf("the server's device %d:%d is Vulkan device %d", fb.DeviceMajor, fb.DeviceMinor, main)
	return main
}

// liveWindowDiff reads the window with GetImage and compares every pixel
// with the scene (the bytes are B, G, R, X; one level of tolerance for a
// driver's rounding). It returns "" when they are equal.
func liveWindowDiff(b *Backend, w *Window, s dmabuftest.Scene) string {
	return liveDrawableDiff(b, xproto.Drawable(w.id), s)
}

// liveDrawableDiff is liveWindowDiff for any drawable: a window, or the
// pixmap the server made of a buffer.
func liveDrawableDiff(b *Backend, drawable xproto.Drawable, s dmabuftest.Scene) string {
	geo, err := xproto.GetGeometry(b.conn, drawable).Reply()
	if err != nil {
		return fmt.Sprintf("GetGeometry: %v", err)
	}
	if int(geo.Width) != s.Size.X || int(geo.Height) != s.Size.Y {
		return fmt.Sprintf("the drawable is %dx%d, the frame %v", geo.Width, geo.Height, s.Size)
	}
	img, err := xproto.GetImage(b.conn, xproto.ImageFormatZPixmap, drawable, 0, 0,
		uint16(s.Size.X), uint16(s.Size.Y), 0xffffffff).Reply()
	if err != nil {
		return fmt.Sprintf("GetImage: %v", err)
	}
	if len(img.Data) < 4*s.Size.X*s.Size.Y {
		return fmt.Sprintf("GetImage returned %d bytes for %v", len(img.Data), s.Size)
	}
	near := func(a, b uint8) bool { return a == b || a+1 == b || b+1 == a }
	bad, first := 0, ""
	for y := 0; y < s.Size.Y; y++ {
		for x := 0; x < s.Size.X; x++ {
			p := img.Data[4*(y*s.Size.X+x):]
			want := s.At(image.Pt(x, y))
			if !near(p[2], want.R) || !near(p[1], want.G) || !near(p[0], want.B) {
				if bad == 0 {
					first = fmt.Sprintf("pixel (%d,%d) is RGB %d,%d,%d, the frame has %d,%d,%d", x, y, p[2], p[1], p[0], want.R, want.G, want.B)
				}
				bad++
			}
		}
	}
	if bad > 0 {
		return fmt.Sprintf("%d of %d pixels differ; %s", bad, s.Size.X*s.Size.Y, first)
	}
	return ""
}

// liveDmabuf is one run of the test with one rendering device. own says
// that it is the server's device, where every step must work.
func liveDmabuf(t *testing.T, inst *vk.Instance, info vk.DeviceInfo, own bool) {
	g := dmabuftest.NewGPU(t, inst, info)
	b := liveBackend(t)
	title := "tela live dmabuf"
	win, err := b.NewWindow(platform.WindowOptions{Title: title, AppID: liveDmabufClass, Size: platform.Size{W: 480, H: 320}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	st := &w.dmabuf

	// cross is how a failure of the dmabuf path is taken: a result to
	// report on another device, an error on the server's own.
	cross := func(format string, args ...any) {
		t.Helper()
		if own {
			t.Errorf(format, args...)
		} else {
			t.Logf("crossing devices: "+format, args...)
		}
	}

	var (
		fourcc   uint32
		modifier uint64
		images   []*dmabuftest.Image
		second   platform.Window // the window whose opening resizes the first

		frame     int // the animation's frame, shown by the latest present
		presents  int // dmabuf presents
		puts      int // PutImage presents
		releases  int
		recreated int
		waits     int // ticks spent waiting for a free buffer
		checked   int // frames read back equal to the frame presented
		exposed   int // frames repainted by the backend alone
		step      int
		stepSince time.Time
		lastRead  time.Time
		// lastConfigure is when the window got its latest ConfigureEvent.
		lastConfigure time.Time
		sizes         []image.Point
		serials       []uint64
		// modes is what the server did with each present, by the step at
		// which its answer was seen.
		modes         []string
		seen          [3]int
		pixmapsRead   int
		drawnInBuffer bool
		// The time from a PresentPixmap to its PresentCompleteNotify.
		presentedAt       time.Time
		waiting           uint32
		slowest, answered time.Duration
		timed             int
		started           bool
		finished          bool
	)
	destroyImages := func() {
		for _, li := range images {
			g.Destroy(li)
		}
		images = nil
	}
	defer destroyImages()

	// register creates and registers one more buffer of the window's
	// size.
	register := func() bool {
		size := w.Metrics().Physical()
		li, err := g.NewImage(size)
		if err != nil {
			cross("creating an exportable %v image: %v", size, err)
			return false
		}
		if len(images) == 0 || images[0].Img.Modifier != li.Img.Modifier {
			t.Logf("%v image: modifier %#x, planes %+v", size, li.Img.Modifier, li.Img.Planes)
		}
		modifier = li.Img.Modifier
		li.Buf, err = w.CreateDmabuf(g.Descriptor(li, size, fourcc))
		if err != nil {
			g.Destroy(li)
			cross("the server did not take a %v buffer with modifier %#x: %v", size, li.Img.Modifier, err)
			return false
		}
		if li.Buf.Busy() {
			t.Error("a new buffer is busy")
		}
		images = append(images, li)
		return true
	}

	// presentGPU renders the next frame into a free buffer and presents
	// it; full says the whole buffer changed, else only the patch did. It
	// reports false while it waits for the server to release one.
	presentGPU := func(full bool) bool {
		size := w.Metrics().Physical()
		if len(images) > 0 && images[0].Buf.Size() != size {
			// A ConfigureEvent changed the size: the swapchain is made
			// again. The old buffers go while the server may still hold
			// one, and the front buffer among them.
			destroyImages()
			recreated++
			sizes = append(sizes, size)
			if fb, ok := w.DmabufFeedback(); ok {
				serials = append(serials, fb.Serial)
			}
		}
		var free *dmabuftest.Image
		for _, li := range images {
			if !li.Buf.Busy() {
				free = li
				break
			}
		}
		if free == nil {
			// Two buffers are the start; more when the server holds them
			// (a flip is answered when the next one replaced it). More
			// than four means it releases nothing.
			if len(images) < 2 || len(images) < 4 && waits > 5 {
				if !register() {
					finished = true
					return false
				}
				free = images[len(images)-1]
				waits = 0
			} else {
				waits++
				return false
			}
		}
		waits = 0
		next := frame + 1
		if full {
			next = (frame/3 + 1) * 3 // a new background
		}
		scene := dmabuftest.NewScene(next, size)
		g.Render(free, scene)
		damage := []image.Rectangle{scene.Patch()}
		if full {
			damage = nil
		}
		if free.Buf.Busy() {
			t.Fatal("a buffer became busy while it was rendered into")
		}
		if err := w.PresentDmabuf(free.Buf, damage); err != nil {
			t.Fatalf("PresentDmabuf: %v", err)
		}
		if !free.Buf.Busy() {
			t.Fatalf("a presented buffer is not busy (window hidden? %v)", !w.visible)
		}
		free.Held = true
		frame = next
		presents++
		if st.sent != waiting {
			waiting, presentedAt = st.sent, time.Now()
		}
		return true
	}
	presentPut := func() bool {
		frame++
		scene := dmabuftest.NewScene(frame, w.Metrics().Physical())
		fb := image.NewRGBA(image.Rectangle{Max: scene.Size})
		scene.Draw(fb)
		if err := w.Present(fb, nil); err != nil {
			t.Fatalf("Present: %v", err)
		}
		if st.front != nil {
			t.Error("a Present left the front buffer in place")
		}
		puts++
		return true
	}
	// read compares the window with the frame presented last, again and
	// again until they are equal or 4 seconds passed: a present is shown
	// at the server's next vertical blank, and a window's first frame at
	// the Expose that follows its map.
	read := func(what string, count *int) bool {
		if st.sent != st.completed && time.Since(stepSince) < 4*time.Second {
			return false // the present is still waiting
		}
		if time.Since(lastRead) < 40*time.Millisecond {
			return false
		}
		lastRead = time.Now()
		scene := dmabuftest.NewScene(frame, w.Metrics().Physical())
		diff := liveWindowDiff(b, w, scene)
		if diff == "" {
			*count++
			// The pixmap of the front buffer holds the same frame: what
			// the server imported is what the device rendered.
			if st.front != nil {
				if pd := liveDrawableDiff(b, xproto.Drawable(st.front.pixmap), scene); pd != "" {
					t.Errorf("%s (frame %d): the front buffer's pixmap differs from the frame: %s", what, frame, pd)
				} else {
					pixmapsRead++
				}
			}
			return true
		}
		if time.Since(stepSince) < 4*time.Second {
			return false
		}
		t.Errorf("%s (frame %d): the window differs from the frame presented: %s", what, frame, diff)
		return true
	}
	verify := func(what string) func() bool {
		return func() bool { return read(what, &checked) }
	}
	resized := func(what string) func() bool {
		return func() bool {
			if size := w.Metrics().Physical(); len(images) > 0 && images[0].Buf.Size() != size {
				t.Logf("%s: the window is now %v", what, size)
				return true
			}
			if time.Since(stepSince) > 3*time.Second {
				t.Errorf("%s did not resize the window (a window manager that does not tile?)", what)
				return true
			}
			return false
		}
	}
	wait := func(d time.Duration) func() bool {
		return func() bool { return time.Since(stepSince) > d }
	}
	// sameSize is false once the window changed its size behind a step
	// that compares it with the front buffer's frame.
	sizeBefore := image.Point{}
	sameSize := func() bool { return w.Metrics().Physical() == sizeBefore && w.osSize == sizeBefore }

	steps := []func() bool{
		// The first frame maps the window and is painted at the Expose
		// that follows. A tiling window manager then gives the window its
		// size, and the frame can only be compared where it does not.
		func() bool { return presentGPU(true) },
		func() bool {
			return time.Since(stepSince) > 400*time.Millisecond && time.Since(lastConfigure) > 400*time.Millisecond
		},
		func() bool {
			if size := w.Metrics().Physical(); images[0].Buf.Size() != size || w.osSize != size {
				t.Logf("the window manager resized the mapped window to %v: its first frame, painted at the Expose of the map, is not compared", w.osSize)
				return true
			}
			return read("the first dmabuf, painted at the Expose of the map", &exposed)
		},
		// Frames in a row: whole ones and ones that change the patch.
		func() bool { return presentGPU(true) },
		verify("the first dmabuf at the window's size"),
		func() bool { return presentGPU(false) },
		verify("a dmabuf with partial damage"),
		func() bool { return presentGPU(false) },
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		func() bool { return presentGPU(false) },
		verify("the sixth dmabuf"),
		// Expose: something else draws over the window, and the server
		// asks for a repaint. The backend answers from the front buffer.
		func() bool {
			sizeBefore = w.Metrics().Physical()
			gc, _ := xproto.NewGcontextId(b.conn)
			xproto.CreateGC(b.conn, gc, xproto.Drawable(w.id), xproto.GcForeground, []uint32{0x00ff00})
			xproto.PolyFillRectangle(b.conn, xproto.Drawable(w.id), gc, []xproto.Rectangle{{X: 20, Y: 20, Width: 100, Height: 80}})
			xproto.FreeGC(b.conn, gc)
			if liveWindowDiff(b, w, dmabuftest.NewScene(frame, sizeBefore)) == "" {
				t.Error("drawing over the window did not change it; the Expose check proves nothing")
			}
			// Did the drawing reach the buffer? It does where the server
			// made the buffer the window's own pixmap.
			drawnInBuffer = liveDrawableDiff(b, xproto.Drawable(st.front.pixmap), dmabuftest.NewScene(frame, sizeBefore)) != ""
			xproto.ClearArea(b.conn, true, w.id, 0, 0, 0, 0)
			return true
		},
		func() bool {
			if time.Since(stepSince) < 300*time.Millisecond {
				return false
			}
			diff := liveWindowDiff(b, w, dmabuftest.NewScene(frame, sizeBefore))
			switch {
			case diff == "":
				exposed++
				t.Log("Expose: repainted from the front buffer, without a present")
			case drawnInBuffer:
				// A server may make a flipped buffer the window's own
				// pixmap: the drawing then went into the dmabuf, and
				// nothing else holds the frame.
				t.Logf("Expose: the test's own drawing over the window went into the front buffer (%s), so there is nothing to repaint from; nothing but a client draws there", diff)
			default:
				t.Errorf("Expose: the window was not repainted from the front buffer: %s", diff)
			}
			return true
		},
		func() bool { return presentGPU(true) },
		verify("a dmabuf after the Expose"),
		// Hide and show: the window comes back with the front buffer's
		// frame, without a present.
		func() bool {
			sizeBefore = w.Metrics().Physical()
			w.SetVisible(false)
			return true
		},
		wait(300 * time.Millisecond),
		func() bool {
			if st.front == nil || !st.front.Busy() {
				t.Error("the front buffer of a hidden window is not kept")
			}
			w.SetVisible(true)
			return true
		},
		func() bool {
			if time.Since(stepSince) < 400*time.Millisecond {
				return false
			}
			if !sameSize() {
				t.Logf("hide and show: the window came back at %v instead of %v, so its frame is not compared", w.osSize, sizeBefore)
				return true
			}
			var n int
			if !read("the front buffer's frame after hide and show", &n) {
				return false
			}
			if n > 0 {
				exposed++
				t.Log("hide and show: mapped again and repainted from the front buffer, without a present")
			}
			return true
		},
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		verify("a dmabuf after hide and show"),
		// A resize: a second window of ours makes a tiling window manager
		// split the workspace.
		func() bool {
			var err error
			second, err = b.NewWindow(platform.WindowOptions{Title: title + " (second)", AppID: liveDmabufClass, Size: platform.Size{W: 200, H: 150}})
			if err != nil {
				t.Fatal(err)
			}
			return true
		},
		resized("opening a second window"),
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		verify("a dmabuf after the resize"),
		// PutImage and dmabufs take turns.
		presentPut,
		verify("PutImage after dmabufs"),
		func() bool { return presentGPU(false) },
		verify("a dmabuf after PutImage"),
		func() bool { return presentGPU(false) },
		presentPut,
		func() bool { return presentGPU(true) },
		verify("the last change of kind"),
		// The second window goes: back to the first size.
		func() bool { second.Close(); second = nil; return true },
		resized("closing the second window"),
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		func() bool { return presentGPU(false) },
		verify("the last dmabuf"),
		// Time for the last answers.
		wait(150 * time.Millisecond),
	}

	tick := func() {
		// PresentIdleNotify is handled while the loop reads events: seen
		// here as buffers that are no longer busy.
		for _, li := range images {
			if li.Held && !li.Buf.Busy() {
				li.Held = false
				releases++
			}
		}
		if waiting != 0 && st.completed == waiting {
			d := time.Since(presentedAt)
			waiting, timed, answered, slowest = 0, timed+1, answered+d, max(slowest, d)
		}
		for i, n := range [3]int{st.flips, st.copies, st.skips} {
			for ; seen[i] < n; seen[i]++ {
				modes = append(modes, fmt.Sprintf("%d:%s", step, [3]string{"flip", "copy", "skip"}[i]))
			}
		}
		if finished {
			return
		}
		// One step a tick: the server gets time to answer between two.
		if step < len(steps) {
			if stepSince.IsZero() {
				stepSince = time.Now()
			}
			if steps[step]() {
				step++
				stepSince = time.Time{}
			}
		}
		if step == len(steps) {
			finished = true
		}
	}

	stop := time.AfterFunc(60*time.Second, b.Quit) // safety net
	defer stop.Stop()
	var ticking atomic.Bool
	ticking.Store(true)
	defer ticking.Store(false)
	var ticker func()
	ticker = func() {
		if ticking.Load() {
			b.Wake()
			time.AfterFunc(8*time.Millisecond, ticker)
		}
	}
	runErr := b.Run(platform.HandlerFunc(func(pw platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ConfigureEvent:
			if pw == platform.Window(w) {
				lastConfigure = time.Now()
				t.Logf("configure %v", e.Metrics)
			}
		case platform.FrameEvent:
			if second != nil && pw == second {
				// The second window only has to exist: one frame maps it.
				fb := image.NewRGBA(image.Rectangle{Max: second.Metrics().Physical()})
				fill(fb, 3, fb.Rect)
				if err := second.Present(fb, nil); err != nil {
					t.Fatalf("Present: %v", err)
				}
				return
			}
			if started {
				return
			}
			started = true
			fb, ok := w.DmabufFeedback()
			if !ok {
				t.Error("the window reports no dmabuf feedback")
				finished = true
				b.Quit()
				return
			}
			serials = append(serials, fb.Serial)
			if fourcc, ok = g.Choose(fb, !own); !ok {
				finished = true
				b.Quit()
				return
			}
			sizes = append(sizes, w.Metrics().Physical())
			go ticker()
		case platform.WakeEvent:
			if !started {
				return
			}
			tick()
			if finished {
				// A last round trip: the X errors of everything sent are
				// read before its answer.
				b.conn.Sync()
				b.Quit()
			}
		}
	}))
	if runErr != nil {
		if !own {
			t.Skipf("crossing devices: the connection ended: %v", runErr)
		}
		t.Fatalf("Run: %v", runErr)
	}
	if !started {
		t.Fatal("the window never got its first frame")
	}
	if fourcc == 0 {
		// The lists of both sides are in the log (Choose).
		if own {
			t.Skipf("%s exports no 32-bit color attachment with a modifier the server lists: nothing to present", info.Name)
		}
		t.Skipf("crossing devices: %s and the server have no modifier in common: a renderer on this device presents with PutImage", info.Name)
	}
	t.Logf("format %#x modifier %#x: %d dmabuf presents and %d PutImage presents over the sizes %v (feedback serials %v); %d buffers found free again, %d buffers at the end",
		fourcc, modifier, presents, puts, sizes, serials, releases, len(images))
	t.Logf("the server answered %d presents: %d flips, %d copies, %d skipped (seen at step: %s); %d failed with an X error",
		st.flips+st.copies+st.skips, st.flips, st.copies, st.skips, strings.Join(modes, " "), st.failed)
	if timed > 0 {
		t.Logf("a present was answered (PresentCompleteNotify) after %v on average, %v at most, over %d presents, seen at ticks of 8 ms", answered/time.Duration(timed), slowest, timed)
	}
	t.Logf("%d frames read back from the window with GetImage equal to the frame presented, %d of them also from the front buffer's pixmap; %d repainted by the backend alone; the test's drawing over the window reached the buffer: %v",
		checked, pixmapsRead, exposed, drawnInBuffer)
	if st.failed > 0 {
		cross("%d PresentPixmap requests were answered with an X error", st.failed)
	}
	if step < len(steps) {
		cross("the sequence stopped at step %d of %d", step, len(steps))
		return
	}
	if recreated < 2 {
		t.Errorf("the swapchain was made again %d times, want 2 (a resize and back)", recreated)
	}
	// Every buffer but the ones the server still holds was found free
	// again, and none was reused before that (PresentDmabuf panics on a
	// busy one).
	if releases == 0 || releases < presents-recreated*4-4 {
		t.Errorf("%d buffers found free again for %d presents", releases, presents)
	}
	if checked == 0 {
		t.Error("no frame read back from the window matched")
	}
}
