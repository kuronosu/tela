package headless

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// ctlBackend is a backend with a small screen, for window control tests.
func ctlBackend() *Backend {
	return New(Options{Size: platform.Size{W: 400, H: 300}, Screen: platform.Size{W: 1000, H: 800}})
}

func expectEvents(t *testing.T, b *Backend, r *recorder, want string) {
	t.Helper()
	b.Drain(r)
	if got := r.take(); got != want {
		t.Errorf("events:\n got %s\nwant %s", got, want)
	}
}

func TestMaximizeFullscreenRestore(t *testing.T) {
	b := ctlBackend()
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	r := &recorder{}
	expectEvents(t, b, r, "w:configure(400x300@1x (400x300 px)) w:frame(0s)")

	// The state is queued before the configure it causes, and the frame
	// forced by the configure sees both.
	w.SetMaximized(true)
	expectEvents(t, b, r, "w:state{maximized} w:configure(1000x800@1x (1000x800 px)) w:frame(0s)")
	if w.State() != (platform.WindowState{Maximized: true}) {
		t.Errorf("State() = %v", w.State())
	}
	// SetSize while maximized changes the size it returns to.
	w.SetSize(platform.Size{W: 500, H: 400})
	expectEvents(t, b, r, "")
	if got := w.RestoreSize(); got != (platform.Size{W: 500, H: 400}) {
		t.Errorf("RestoreSize = %v", got)
	}
	w.SetFullscreen(true)
	expectEvents(t, b, r, "w:state{maximized fullscreen}")
	w.SetFullscreen(false)
	expectEvents(t, b, r, "w:state{maximized}")
	w.SetMaximized(false)
	expectEvents(t, b, r, "w:state{} w:configure(500x400@1x (500x400 px)) w:frame(0s)")
	// Requests that change nothing yield nothing.
	w.SetMaximized(false)
	w.SetFullscreen(false)
	expectEvents(t, b, r, "")
}

func TestMaximizeHonorsMaxSizeFullscreenDoesNot(t *testing.T) {
	b := ctlBackend()
	w := newWindow(t, b, platform.WindowOptions{Title: "w", MaxSize: platform.Size{W: 900, H: 700}})
	r := &recorder{}
	b.Drain(r)
	r.take()
	w.SetMaximized(true)
	expectEvents(t, b, r, "w:state{maximized} w:configure(900x700@1x (900x700 px)) w:frame(0s)")
	w.SetFullscreen(true)
	expectEvents(t, b, r, "w:state{maximized fullscreen} w:configure(1000x800@1x (1000x800 px)) w:frame(0s)")
}

func TestInitialState(t *testing.T) {
	b := ctlBackend()
	w := newWindow(t, b, platform.WindowOptions{Title: "w", Frameless: true, Maximized: true,
		HasPosition: true, Position: image.Pt(10, -20), AlwaysOnTop: true})
	r := &recorder{}
	expectEvents(t, b, r, "w:configure(1000x800@1x (1000x800 px)) w:state{maximized csd at 10,-20} w:frame(0s)")
	if w.RestoreSize() != (platform.Size{W: 400, H: 300}) || !w.AlwaysOnTop() {
		t.Errorf("RestoreSize = %v, AlwaysOnTop = %v", w.RestoreSize(), w.AlwaysOnTop())
	}
	// A plain window has the zero state: no WindowStateEvent.
	newWindow(t, b, platform.WindowOptions{Title: "plain"})
	expectEvents(t, b, r, "plain:configure(400x300@1x (400x300 px)) plain:frame(0s)")
}

func TestMinimizeAndActivate(t *testing.T) {
	b := ctlBackend()
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	r := &recorder{}
	w.SetFocused(true)
	b.Drain(r)
	r.take()
	w.Minimize()
	expectEvents(t, b, r, "w:state{minimized} w:focus(false)")
	w.RequestFrame()
	b.Advance(b.FrameInterval())
	expectEvents(t, b, r, "") // no frames while minimized
	if !w.FrameRequested() {
		t.Errorf("the request made while minimized was dropped")
	}
	w.Activate()
	expectEvents(t, b, r, "w:state{} w:focus(true) w:frame(16.666666ms)")
	if w.Activations() != 1 {
		t.Errorf("Activations = %d", w.Activations())
	}
	// Maximizing a minimized window restores it too.
	w.Minimize()
	w.SetMaximized(true)
	expectEvents(t, b, r, "w:state{maximized} w:focus(false) w:configure(1000x800@1x (1000x800 px)) w:frame(16.666666ms)")
}

func TestActivateMovesFocus(t *testing.T) {
	b := ctlBackend()
	a := newWindow(t, b, platform.WindowOptions{Title: "a"})
	c := newWindow(t, b, platform.WindowOptions{Title: "c"})
	r := &recorder{}
	a.SetFocused(true)
	b.Drain(r)
	r.take()
	c.Activate()
	expectEvents(t, b, r, "a:focus(false) c:focus(true)")
	c.Activate() // already focused: nothing
	expectEvents(t, b, r, "")
	// A hidden window is not given the focus.
	a.SetVisible(false)
	a.Activate()
	expectEvents(t, b, r, "")
}

func TestHiddenWindows(t *testing.T) {
	b := ctlBackend()
	h := newWindow(t, b, platform.WindowOptions{Title: "h", Hidden: true})
	r := &recorder{}
	// The ConfigureEvent is delivered; its frame waits.
	expectEvents(t, b, r, "h:configure(400x300@1x (400x300 px))")
	if h.Visible() || !h.FrameRequested() {
		t.Errorf("Visible = %v, FrameRequested = %v", h.Visible(), h.FrameRequested())
	}
	h.SetVisible(true)
	expectEvents(t, b, r, "h:frame(0s)")
	h.SetFocused(true)
	b.Drain(r)
	r.take()
	h.SetVisible(false)
	h.RequestFrame()
	b.Advance(b.FrameInterval())
	expectEvents(t, b, r, "h:focus(false)")
	h.SetVisible(true)
	expectEvents(t, b, r, "h:frame(16.666666ms)")
}

func TestPositionAndInjectedState(t *testing.T) {
	b := ctlBackend()
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	r := &recorder{}
	b.Drain(r)
	r.take()
	// Moves coalesce into one event with the latest position.
	w.SetPosition(image.Pt(5, 6))
	w.SetPosition(image.Pt(7, 8))
	expectEvents(t, b, r, "w:state{at 7,8}")
	w.SetState(platform.WindowState{ClientSideDecorations: true, HasPosition: true, Position: image.Pt(7, 8)})
	expectEvents(t, b, r, "w:state{csd at 7,8}")
	if w.OSState() != w.State() {
		t.Errorf("OSState %v != State %v after delivery", w.OSState(), w.State())
	}
	w.SetState(platform.WindowState{Fullscreen: true})
	expectEvents(t, b, r, "w:state{fullscreen} w:configure(1000x800@1x (1000x800 px)) w:frame(0s)")
}

func TestSizeLimitsAtRuntime(t *testing.T) {
	b := ctlBackend()
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	r := &recorder{}
	b.Drain(r)
	r.take()
	w.SetMinSize(platform.Size{W: 500, H: 500})
	expectEvents(t, b, r, "w:configure(500x500@1x (500x500 px)) w:frame(0s)")
	// The minimum wins over a smaller maximum.
	w.SetMaxSize(platform.Size{W: 450, H: 450})
	expectEvents(t, b, r, "")
	w.SetMinSize(platform.Size{})
	expectEvents(t, b, r, "w:configure(450x450@1x (450x450 px)) w:frame(0s)")
	w.Resize(platform.Size{W: 2000, H: 100})
	expectEvents(t, b, r, "w:configure(450x100@1x (450x100 px)) w:frame(0s)")
	if min, max := w.SizeLimits(); min != (platform.Size{}) || max != (platform.Size{W: 450, H: 450}) {
		t.Errorf("SizeLimits = %v, %v", min, max)
	}
	expectPanic(t, "SetMinSize", func() { w.SetMinSize(platform.Size{W: -1}) })
}

func TestCaptionRegions(t *testing.T) {
	b := ctlBackend()
	w := newWindow(t, b, platform.WindowOptions{Title: "w", Frameless: true})
	r := &recorder{}
	b.Drain(r)
	r.take()
	regions := []platform.HitRegion{
		{Kind: platform.HitCaption, Rect: platform.Rect{W: 400, H: 30}},
		{Kind: platform.HitClient, Rect: platform.Rect{X: 100, W: 50, H: 30}},
		{Kind: platform.HitClose, Rect: platform.Rect{X: 370, W: 30, H: 30}},
	}
	w.SetHitRegions(regions)
	w.SetHitRegions(append([]platform.HitRegion(nil), regions...))
	if w.HitRegionChanges() != 1 {
		t.Errorf("HitRegionChanges = %d after an equal call, want 1", w.HitRegionChanges())
	}
	press := func(action platform.PointerAction, button platform.PointerButton, x, y float32) {
		w.Inject(platform.PointerEvent{Action: action, Button: button, Pos: platform.Point{X: x, Y: y}})
	}
	// A primary press in the caption is the OS's, up to its release.
	press(platform.PointerDown, platform.ButtonPrimary, 10, 10)
	press(platform.PointerMove, platform.ButtonNone, 60, 12)
	press(platform.PointerUp, platform.ButtonPrimary, 60, 12)
	expectEvents(t, b, r, "")
	if w.CaptionDrags() != 1 {
		t.Errorf("CaptionDrags = %d", w.CaptionDrags())
	}
	// The hole and the button are ordinary content.
	press(platform.PointerDown, platform.ButtonPrimary, 120, 10)
	press(platform.PointerUp, platform.ButtonPrimary, 120, 10)
	press(platform.PointerDown, platform.ButtonPrimary, 380, 10)
	press(platform.PointerUp, platform.ButtonPrimary, 380, 10)
	press(platform.PointerMove, platform.ButtonNone, 10, 10) // hover over the caption
	expectEvents(t, b, r, "w:pointer(down 120,10) w:pointer(up 120,10) w:pointer(down 380,10) w:pointer(up 380,10) w:pointer(move 10,10)")
	// A double click toggles maximize.
	b.Advance(time.Second)
	press(platform.PointerDown, platform.ButtonPrimary, 10, 10)
	press(platform.PointerUp, platform.ButtonPrimary, 10, 10)
	press(platform.PointerDown, platform.ButtonPrimary, 11, 10)
	press(platform.PointerUp, platform.ButtonPrimary, 11, 10)
	expectEvents(t, b, r, "w:state{maximized csd} w:configure(1000x800@1x (1000x800 px)) w:frame(1s)")
	// A secondary press opens the window menu.
	press(platform.PointerDown, platform.ButtonSecondary, 10, 10)
	press(platform.PointerUp, platform.ButtonSecondary, 10, 10)
	expectEvents(t, b, r, "")
	if w.WindowMenus() != 1 || w.CaptionDrags() != 2 {
		t.Errorf("WindowMenus = %d, CaptionDrags = %d", w.WindowMenus(), w.CaptionDrags())
	}
	expectPanic(t, "invalid kind", func() { w.SetHitRegions([]platform.HitRegion{{Kind: 9}}) })
}

func TestParentsAndModality(t *testing.T) {
	b := ctlBackend()
	p := newWindow(t, b, platform.WindowOptions{Title: "p"})
	m := newWindow(t, b, platform.WindowOptions{Title: "m", Parent: p, Modal: true})
	c := newWindow(t, b, platform.WindowOptions{Title: "c", Parent: p})
	g := newWindow(t, b, platform.WindowOptions{Title: "g", Parent: c})
	if m.Parent() != p || len(p.Children()) != 2 || !p.Blocked() || c.Blocked() {
		t.Errorf("Parent = %v, Children = %d, Blocked = %v/%v", m.Parent(), len(p.Children()), p.Blocked(), c.Blocked())
	}
	m.SetVisible(false)
	if p.Blocked() {
		t.Errorf("a hidden modal child blocks its parent")
	}
	expectPanic(t, "another backend", func() {
		newWindow(t, b, platform.WindowOptions{Parent: newWindow(t, New(Options{}), platform.WindowOptions{})})
	})
	expectPanic(t, "Modal without a Parent", func() { newWindow(t, b, platform.WindowOptions{Modal: true}) })
	p.Close()
	for _, w := range []*Window{p, m, c, g} {
		if !w.Closed() {
			t.Errorf("%s is open after its ancestor closed", w.Title())
		}
	}
	r := &recorder{}
	expectEvents(t, b, r, "")
	expectPanic(t, "closed", func() { newWindow(t, b, platform.WindowOptions{Parent: p}) })
}

func TestRestoreToken(t *testing.T) {
	b := ctlBackend()
	a := newWindow(t, b, platform.WindowOptions{})
	c := newWindow(t, b, platform.WindowOptions{RestoreToken: "headless:7"})
	d := newWindow(t, b, platform.WindowOptions{RestoreToken: "wayland:session/name"})
	if a.RestoreToken() != "headless:1" || c.RestoreToken() != "headless:7" || d.RestoreToken() != "headless:3" {
		t.Errorf("tokens = %q %q %q", a.RestoreToken(), c.RestoreToken(), d.RestoreToken())
	}
}

func TestIconIsCopied(t *testing.T) {
	b := ctlBackend()
	w := newWindow(t, b, platform.WindowOptions{})
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.SetRGBA(0, 0, color.RGBA{0xff, 0, 0, 0xff})
	w.SetIcon([]image.Image{img})
	img.SetRGBA(0, 0, color.RGBA{0, 0xff, 0, 0xff})
	icon := w.Icon()
	if len(icon) != 1 {
		t.Fatalf("Icon has %d images", len(icon))
	}
	if r, g, _, _ := icon[0].At(0, 0).RGBA(); r != 0xffff || g != 0 {
		t.Errorf("the icon was not copied: pixel %v", icon[0].At(0, 0))
	}
	w.SetIcon(nil)
	if w.Icon() != nil {
		t.Errorf("SetIcon(nil) kept an icon")
	}
	expectPanic(t, "nil", func() { w.SetIcon([]image.Image{nil}) })
}
