//go:build linux && telagpu

package wayland

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/dmabuftest"
	"github.com/kuronosu/tela/internal/vk"
)

// The live check of dmabuf.go, with the Vulkan bindings on the other side
// (ADR-281, ADR-283; the device that renders is the package dmabuftest,
// shared with the X11 backend): what the unit tests cannot tell is whether
// a real compositor imports what a real driver exports. It needs TELA_LIVE=1 and
// the tag telagpu:
//
//	go test -c -tags telagpu -o bin/wayland.test ./internal/platform/wayland
//	TELA_LIVE=1 bin/wayland.test -test.run TestLiveDmabuf -test.v
//
// (a test binary in a sandboxed temporary directory is not visible to
// Hyprland). The windows open silently on the workspace nobody looks at,
// as the other live tests' do, and take no focus and no input.
//
// A window on a hidden workspace is suspended: the compositor sends it no
// frame callbacks, so the test presents from a timer, and nothing shows
// its pixels: grim reads outputs, and Hyprland 0.56 answers neither
// ext-image-copy-capture-v1 (with a toplevel source) nor its own
// hyprland_toplevel_export_manager_v1 for a window it does not draw (no
// frame within 12 s; tried on 2026-11-15). What the test checks then is
// the protocol: every buffer is taken, no protocol error, releases
// arrive, and no buffer is reused before its release. When the workspace
// is on a monitor it also compares the screen (grim) with the frame it
// presented, like TestLivePixels; that branch says in the log whether it
// ran.

const liveDmabufAppID = "dev.telaui.livedmabuf"

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
	rule := `tela_live_dmabuf = hl.window_rule({ name = "tela-live-dmabuf", match = { class = "` + liveDmabufAppID + `" }, workspace = "` + ws + ` silent" })`
	if out, err := exec.Command("hyprctl", "eval", rule).CombinedOutput(); err != nil || strings.Contains(strings.ToLower(string(out)), "error") {
		t.Skipf("hyprctl eval (window rule): %v %s; not opening a window that would take the focus", err, out)
	}
	t.Cleanup(func() {
		exec.Command("hyprctl", "eval", `if tela_live_dmabuf then tela_live_dmabuf:set_enabled(false) end`).Run()
	})
}

// TestLiveDmabuf presents Vulkan images to the real compositor as
// dmabufs, once per physical device. On the device the compositor named
// as its main device every step must work. On another device the buffers
// cross from one driver to another, which needs a modifier both have, or
// a linear image that the compositor takes without a modifier, and a
// compositor that imports foreign memory: what happens there is logged
// and never a failure.
func TestLiveDmabuf(t *testing.T) {
	skipUnlessLive(t)
	inst, report := dmabuftest.NewInstance(t, "tela live dmabuf")
	devices, err := inst.Devices()
	if err != nil || len(devices) == 0 {
		t.Skipf("no usable Vulkan device: %v", err)
	}
	placeDmabufWindow(t)

	// The compositor's main device decides which run must pass.
	main := liveMainDevice(t, devices)
	for i, info := range devices {
		own := i == main
		name := info.Name
		if own {
			name += " (the compositor's device)"
		} else {
			name += " (another device)"
		}
		t.Run(name, func(t *testing.T) {
			liveDmabuf(t, inst, info, own)
			report(t)
		})
	}
}

// liveMainDevice opens a backend only to read the default feedback, logs
// it, and returns the index of the Vulkan device it names (-1 when it
// names none of them).
func liveMainDevice(t *testing.T, devices []vk.DeviceInfo) int {
	b := liveBackend(t)
	defer b.release()
	win, err := b.NewWindow(platform.WindowOptions{Title: "tela live dmabuf feedback", AppID: liveDmabufAppID, Hidden: true})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	fb, ok := w.DmabufFeedback()
	if !ok {
		t.Skip("the compositor reports no dmabuf feedback (no zwp_linux_dmabuf_v1, or nothing listed)")
	}
	t.Logf("zwp_linux_dmabuf_v1 version %d (the compositor has %d); %d format and modifier pairs", b.dmabuf.wl.Version(), b.dmabuf.version, len(fb.Formats))
	for _, f := range dmabuftest.Formats {
		t.Logf("  %s: modifiers %#x", f.Name, fb.Modifiers(f.Fourcc))
	}
	if !fb.HasDevice {
		// The renderer's choice then (ADR-281, ADR-288): the first discrete one, else the first integrated one.
		main, _ := vk.SelectDevice(devices, "")
		t.Logf("the compositor names no main device; taking Vulkan device %d for it", main)
		return main
	}
	main := -1
	for i, d := range devices {
		if dmabuftest.SameDevice(fb, d) {
			main = i
		}
		t.Logf("  Vulkan device %d: %s", i, d)
	}
	t.Logf("main device %d:%d is Vulkan device %d", fb.DeviceMajor, fb.DeviceMinor, main)
	return main
}

// liveDmabuf is one run of the test with one rendering device. own says
// that it is the compositor's device, where every step must work.
func liveDmabuf(t *testing.T, inst *vk.Instance, info vk.DeviceInfo, own bool) {
	g := dmabuftest.NewGPU(t, inst, info)
	b := liveBackend(t)
	title := "tela live dmabuf"
	win, err := b.NewWindow(platform.WindowOptions{Title: title, AppID: liveDmabufAppID, Size: platform.Size{W: 480, H: 320}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)

	// cross is how a failure of the dmabuf path is taken: a result to
	// report on another device, an error on the compositor's own.
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
		// onScreen: the window's workspace is on a monitor, so the screen
		// can be compared; asked once, at the first comparison.
		onScreen, asked bool

		frame      int // the animation's frame, shown by the latest present
		presents   int // dmabuf presents
		shm        int // wl_shm presents
		releases   int
		recreated  int
		callbacks  int // FrameEvents of the window after its first
		waits      int // ticks spent waiting for a free buffer
		checked    int // screenshots equal to the frame presented
		step       int
		stepSince  time.Time
		sizes      []image.Point
		started    bool
		finished   bool
		firstFrame bool
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
			cross("the compositor did not take a %v buffer with modifier %#x: %v", size, li.Img.Modifier, err)
			return false
		}
		images = append(images, li)
		return true
	}

	// presentGPU renders the next frame into a free buffer and presents
	// it; full says the whole buffer changed, else only the patch did. It
	// reports false while it waits for the compositor to release one.
	presentGPU := func(full bool) bool {
		size := w.Metrics().Physical()
		if len(images) > 0 && images[0].Buf.Size() != size {
			// A ConfigureEvent changed the size: the swapchain is made
			// again. The old buffers go while the compositor may still
			// hold one.
			destroyImages()
			recreated++
			sizes = append(sizes, size)
		}
		var free *dmabuftest.Image
		for _, li := range images {
			if !li.Buf.Busy() {
				free = li
				break
			}
		}
		if free == nil {
			// Two buffers are the start; a third when the compositor
			// holds both. More than three means it releases nothing.
			if len(images) < 2 || len(images) < 3 && waits > 5 {
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
			t.Fatalf("a presented buffer is not busy (window hidden? %v)", !w.showable())
		}
		free.Held = true
		frame = next
		presents++
		return true
	}
	presentShm := func() bool {
		frame++
		scene := dmabuftest.NewScene(frame, w.Metrics().Physical())
		fb := image.NewRGBA(image.Rectangle{Max: scene.Size})
		scene.Draw(fb)
		if err := w.Present(fb, nil); err != nil {
			t.Fatalf("Present: %v", err)
		}
		shm++
		return true
	}
	// verify compares the screen with the frame presented last, when the
	// window is on a monitor. The first comparison waits for the
	// compositor's open animation, the others for a frame or two.
	verify := func(what string) bool {
		if !asked {
			asked = true
			onScreen = liveOnScreen(t, title)
		}
		if !onScreen {
			return true
		}
		settle := 250 * time.Millisecond
		if checked == 0 {
			settle = 800 * time.Millisecond
		}
		if time.Since(stepSince) < settle {
			return false
		}
		c, err := findWindow(title)
		var img image.Image
		if err == nil {
			img, err = screenshot(c)
		}
		if err != nil {
			t.Errorf("%s: %v", what, err)
			return true
		}
		if bad := liveCompare(img, dmabuftest.NewScene(frame, w.Metrics().Physical())); len(bad) > 0 {
			t.Errorf("%s (frame %d): the screen differs from the frame presented: %s", what, frame, strings.Join(bad, "; "))
		} else {
			checked++
		}
		return true
	}
	resized := func(what string) func() bool {
		return func() bool {
			if size := w.Metrics().Physical(); len(images) > 0 && images[0].Buf.Size() != size {
				t.Logf("%s: the window is now %v", what, size)
				return true
			}
			if time.Since(stepSince) > 3*time.Second {
				t.Errorf("%s did not resize the window (a compositor that does not tile?)", what)
				return true
			}
			return false
		}
	}

	steps := []func() bool{
		// Frames in a row: whole ones and ones that change the patch.
		func() bool { return presentGPU(true) },
		func() bool { return verify("the first dmabuf") },
		func() bool { return presentGPU(false) },
		func() bool { return verify("a dmabuf with partial damage") },
		func() bool { return presentGPU(false) },
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		func() bool { return presentGPU(false) },
		func() bool { return verify("the sixth dmabuf") },
		// A resize: a second window of ours makes a tiling compositor
		// split the workspace.
		func() bool {
			var err error
			second, err = b.NewWindow(platform.WindowOptions{Title: title + " (second)", AppID: liveDmabufAppID, Size: platform.Size{W: 200, H: 150}})
			if err != nil {
				t.Fatal(err)
			}
			return true
		},
		resized("opening a second window"),
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		func() bool { return verify("a dmabuf after the resize") },
		// wl_shm and dmabufs take turns.
		presentShm,
		func() bool { return verify("wl_shm after dmabufs") },
		func() bool { return presentGPU(false) },
		func() bool { return verify("a dmabuf after wl_shm") },
		func() bool { return presentGPU(false) },
		presentShm,
		func() bool { return presentGPU(true) },
		func() bool { return verify("the last change of kind") },
		// The second window goes: back to the first size.
		func() bool { second.Close(); second = nil; return true },
		resized("closing the second window"),
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		func() bool { return presentGPU(false) },
		func() bool { return verify("the last dmabuf") },
		// Time for the last releases.
		func() bool { return time.Since(stepSince) > 150*time.Millisecond },
	}

	tick := func() {
		// Releases arrive while the loop dispatches: seen here as buffers
		// that are no longer busy.
		for _, li := range images {
			if li.Held && !li.Buf.Busy() {
				li.Held = false
				releases++
			}
		}
		if finished {
			return
		}
		// One step a tick: the compositor gets time to answer between two.
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

	stop := time.AfterFunc(40*time.Second, b.Quit) // safety net
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
				t.Logf("configure %v (%v px)", e.Metrics, e.Metrics.Physical())
			}
		case platform.WindowStateEvent:
			if pw == platform.Window(w) {
				t.Logf("state %v", e.State)
			}
		case platform.FrameEvent:
			if second != nil && pw == second {
				// The second window only has to exist: one frame maps it.
				fb := image.NewRGBA(image.Rectangle{Max: second.Metrics().Physical()})
				quadrants(fb)
				if err := second.Present(fb, nil); err != nil {
					t.Fatalf("Present: %v", err)
				}
				return
			}
			if firstFrame {
				callbacks++
			}
			firstFrame = true
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
			if got := dmabuftest.SameDevice(fb, info); got != own {
				t.Logf("the window's feedback names the device %d:%d (this device: %v)", fb.DeviceMajor, fb.DeviceMinor, got)
			}
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
				// A last round trip: a protocol error for anything sent
				// would arrive before its answer.
				if err := b.conn.RoundtripTimeout(2 * time.Second); err != nil {
					t.Errorf("after the last present: %v", err)
				}
				logWorkspace(t)
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
		// The lists of both sides are in the log (choose).
		if own {
			t.Skipf("%s exports no 32-bit color attachment with a modifier the compositor lists: nothing to present", info.Name)
		}
		t.Skipf("crossing devices: %s and the compositor have no explicit modifier in common: a renderer on this device presents with wl_shm", info.Name)
	}
	t.Logf("format %#x modifier %#x: %d dmabuf presents and %d wl_shm presents over the sizes %v; %d releases seen, %d buffers at the end, %d frame callbacks, %d screenshots equal to the frame",
		fourcc, modifier, presents, shm, sizes, releases, len(images), callbacks, checked)
	if step < len(steps) {
		cross("the sequence stopped at step %d of %d", step, len(steps))
		return
	}
	if recreated < 2 {
		t.Errorf("the swapchain was made again %d times, want 2 (a resize and back)", recreated)
	}
	// Every buffer but the ones on screen was released: the compositor
	// gives buffers back, and none was reused before that (PresentDmabuf
	// panics on a busy one).
	if releases == 0 || releases < presents-recreated*3-3 {
		t.Errorf("%d releases for %d presents", releases, presents)
	}
	if !onScreen {
		t.Log("the pixels were NOT looked at: the window's workspace is on no monitor, and the compositor captures no window it does not draw")
	} else if checked == 0 {
		t.Error("the window was on screen and no screenshot matched a frame")
	}
	if callbacks == 0 {
		t.Log("no frame callback after the first frame: the window is on a hidden workspace (suspended), so the presents were paced by a timer")
	}
}

// liveOnScreen reports whether the window of this process with the title
// is on a workspace some monitor shows (Hyprland only: elsewhere the
// answer is no, and the screen is not compared).
func liveOnScreen(t *testing.T, title string) bool {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return false
	}
	var clients []struct {
		Pid       int    `json:"pid"`
		Title     string `json:"title"`
		Workspace struct {
			ID int `json:"id"`
		} `json:"workspace"`
	}
	var monitors []struct {
		ActiveWorkspace struct {
			ID int `json:"id"`
		} `json:"activeWorkspace"`
	}
	for _, q := range []struct {
		what string
		into any
	}{{"clients", &clients}, {"monitors", &monitors}} {
		out, err := exec.Command("hyprctl", q.what, "-j").Output()
		if err == nil {
			err = json.Unmarshal(out, q.into)
		}
		if err != nil {
			t.Logf("hyprctl %s: %v", q.what, err)
			return false
		}
	}
	for _, c := range clients {
		if c.Pid != os.Getpid() || c.Title != title {
			continue
		}
		for _, m := range monitors {
			if m.ActiveWorkspace.ID == c.Workspace.ID {
				return true
			}
		}
	}
	return false
}

// liveCompare compares a screenshot of the window with a scene at the
// middle of each of its areas (the corner squares, the patch, and the
// background between them), with the tolerance of TestLivePixels: the
// desktop may dim or tint a window that is not focused. It returns what
// differs.
func liveCompare(img image.Image, s dmabuftest.Scene) []string {
	bounds := img.Bounds()
	var bad []string
	for _, at := range [][2]float64{
		{0.125, 0.125}, {0.875, 0.125}, {0.125, 0.875}, {0.875, 0.875}, // the corner squares
		{0.5, 0.5},                                                         // the patch
		{0.5, 0.125}, {0.125, 0.5}, {0.875, 0.5}, {0.5, 0.875}, {0.3, 0.3}, // the background
	} {
		x := bounds.Min.X + int(at[0]*float64(bounds.Dx()))
		y := bounds.Min.Y + int(at[1]*float64(bounds.Dy()))
		r, g, b, _ := img.At(x, y).RGBA()
		got := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
		want := s.At(image.Pt(int(at[0]*float64(s.Size.X)), int(at[1]*float64(s.Size.Y))))
		if !near(got, want, 24) {
			bad = append(bad, fmt.Sprintf("at (%.3f, %.3f) the screen has %v, the frame %v", at[0], at[1], got, want))
		}
	}
	return bad
}
