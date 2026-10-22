package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// The error overlay under the supervisor: fake children, a remote host on
// the headless backend, and the pixels the real window presents compared
// with the overlay's own rendering over the child's frame.

const overlayBuildError = "# fake\n./main.go:7:2: undefined: oops\n"

func overlayHarness(t *testing.T) *devHarness {
	t.Helper()
	return newDevHarness(t, devOptions{}, func(c *supervisorConfig) { c.overlay = true })
}

// startBig waits for the first build, then gives the window a size the
// panel fits in and waits for the child's frame at that size.
func (h *devHarness) startBig() {
	h.t.Helper()
	h.expect("first start", 1, 0, 0)
	h.window().Resize(platform.Size{W: 640, H: 480})
	h.waitFrame("the first frame at 640x480", h.plain(child(1, 0, 0)))
}

// solid is a frame of one color in physical pixels.
func solid(size image.Point, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rectangle{Max: size})
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	return img
}

// child is the color the fake child of a build shows.
func child(gen, count, clicks int) color.RGBA {
	return color.RGBA{uint8(gen), uint8(count), uint8(clicks), 255}
}

// waitFrame waits until the real window presents exactly want.
func (h *devHarness) waitFrame(what string, want *image.RGBA) {
	h.t.Helper()
	deadline := time.Now().Add(devWait)
	var got *image.RGBA
	for {
		if f := h.window().Frame(); f != nil {
			got = f
			if f.Rect == want.Rect && bytes.Equal(f.Pix, want.Pix) {
				return
			}
		}
		if time.Now().After(deadline) {
			if got == nil {
				h.t.Fatalf("%s: the window presented nothing", what)
			}
			h.t.Fatalf("%s: the window's pixels are not the expected ones (%v vs %v); first differing pixel %v", what, got.Rect, want.Rect, firstDiff(got, want))
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func firstDiff(a, b *image.RGBA) string {
	if a.Rect != b.Rect {
		return "different sizes"
	}
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return image.Pt(x, y).String() + ": got " + sprintRGBA(a.RGBAAt(x, y)) + ", want " + sprintRGBA(b.RGBAAt(x, y))
			}
		}
	}
	return "none"
}

func sprintRGBA(c color.RGBA) string { return fmt.Sprintf("%v", c) }

// withOverlay returns what the window presents when the fake child shows
// base and problem p is on view, at the window's current size and scale.
func (h *devHarness) withOverlay(base color.RGBA, p problem) *image.RGBA {
	h.t.Helper()
	m := h.window().Metrics()
	out, _ := drawOverlay(h.t, p, solid(m.Physical(), base), m.Size, m.Scale)
	return out
}

// plain returns what the window presents with no overlay.
func (h *devHarness) plain(base color.RGBA) *image.RGBA {
	m := h.window().Metrics()
	return solid(m.Physical(), base)
}

func (h *devHarness) pointer(action platform.PointerAction, x, y float32, button platform.PointerButton) {
	h.t.Helper()
	var buttons platform.Buttons
	if action == platform.PointerDown {
		buttons = 1
	}
	h.window().Inject(platform.PointerEvent{Action: action, Pos: platform.Point{X: x, Y: y}, Button: button, Buttons: buttons})
}

func (h *devHarness) clickAt(x, y float32) {
	h.t.Helper()
	h.pointer(platform.PointerDown, x, y, platform.ButtonPrimary)
	h.pointer(platform.PointerUp, x, y, platform.ButtonPrimary)
}

func TestOverlayBuildErrorDocksAndAppStaysInteractive(t *testing.T) {
	h := overlayHarness(t)
	h.startBig()
	h.waitFrame("no overlay yet", h.plain(child(1, 0, 0)))

	h.fb.mu.Lock()
	h.fb.fail[2] = overlayBuildError
	h.fb.mu.Unlock()
	h.s.push(evKey{'r'})
	h.logged("build failed", 1)
	p := h.view.lastProblem()
	if p.kind != problemBuild || !p.live {
		t.Fatalf("problem %+v, want a build error with a live app", p)
	}
	h.waitFrame("the docked panel", h.withOverlay(child(1, 0, 0), p))
	size := h.window().Metrics().Size
	g := newOverlayModel(p).geometry(size)
	if !g.docked || g.panel.y0 < size.H/3 {
		t.Fatalf("panel %+v in a window %v: want it docked at the bottom", g.panel, size)
	}

	// The app above the panel is interactive.
	h.clickAt(10, 10)
	h.waitFrame("a click above the panel reaches the app", h.withOverlay(child(1, 1, 1), p))
	// A click inside the panel does not.
	h.clickAt(size.W/2, g.panel.y0+g.panel.h()/2)
	h.clickAt(size.W/2, g.panel.y1-5)
	time.Sleep(100 * time.Millisecond)
	h.waitFrame("a click on the panel stays out of the app", h.withOverlay(child(1, 1, 1), p))

	// Escape dismisses the overlay; the problem stays on the terminal.
	h.press("Escape", platform.CodeEscape)
	h.waitFrame("Escape dismisses", h.plain(child(1, 1, 1)))
	h.clickAt(size.W/2, g.panel.y1-5)
	h.waitFrame("the app gets the whole window again", h.plain(child(1, 2, 2)))
	if got := h.view.entries(); len(got) != 1 {
		t.Errorf("problems %v, want only the one shown", got)
	}

	// The next good build clears what is left, and a new problem shows
	// again after a dismissal.
	h.s.push(evKey{'r'})
	h.expect("the next good build", 3, 2, 0)
	h.waitFrame("cleared", h.plain(child(3, 2, 0)))
	if got := h.view.entries(); len(got) != 2 || got[1] != "clear" {
		t.Errorf("problems %v", got)
	}
}

func TestOverlayCloseButtonAndClear(t *testing.T) {
	h := overlayHarness(t)
	h.startBig()
	h.fb.mu.Lock()
	h.fb.fail[2] = overlayBuildError
	h.fb.mu.Unlock()
	h.s.push(evKey{'r'})
	h.logged("build failed", 1)
	p := h.view.lastProblem()
	h.waitFrame("the panel", h.withOverlay(child(1, 0, 0), p))
	size := h.window().Metrics().Size
	g := newOverlayModel(p).geometry(size)

	// The press and the release on the close button belong to the
	// overlay.
	cx, cy := (g.close.x0+g.close.x1)/2, (g.close.y0+g.close.y1)/2
	h.clickAt(cx, cy)
	h.waitFrame("the close button dismisses", h.plain(child(1, 0, 0)))
	time.Sleep(50 * time.Millisecond)
	h.expect("the app saw nothing of it", 1, 0, 0)

	// A second failure shows the overlay again.
	h.fb.mu.Lock()
	h.fb.fail[3] = overlayBuildError + "./main.go:9:1: second\n"
	h.fb.mu.Unlock()
	h.s.push(evKey{'r'})
	h.logged("build failed", 2)
	p2 := h.view.lastProblem()
	h.waitFrame("the new problem replaces the dismissed one", h.withOverlay(child(1, 0, 0), p2))

	// A press that starts in the app and is released over the panel is
	// the app's.
	h.pointer(platform.PointerDown, 10, 10, platform.ButtonPrimary)
	h.window().Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: size.W / 2, Y: size.H - 20}, Buttons: 1})
	h.pointer(platform.PointerUp, size.W/2, size.H-20, platform.ButtonPrimary)
	h.waitFrame("the app's own press", h.withOverlay(child(1, 1, 1), p2))
}

func TestOverlayPanicCoversTheWindow(t *testing.T) {
	h := overlayHarness(t)
	h.startBig()
	h.click()
	h.expect("one click", 1, 1, 1)
	h.press("p", platform.CodeKeyP)
	h.logged("the app panicked: boom", 1)
	p := h.view.lastProblem()
	if p.kind != problemPanic || p.live {
		t.Fatalf("problem %+v, want a panic with no live app", p)
	}
	want := h.withOverlay(child(1, 1, 1), p)
	h.waitFrame("the full overlay over the last frame", want)
	size := h.window().Metrics().Size
	if g := newOverlayModel(p).geometry(size); g.docked || g.paint.w() != size.W || g.paint.h() != size.H {
		t.Errorf("geometry %+v: a panic covers the whole window", g)
	}
	// The window never closes, and a click on the dimmed frame outside
	// the panel reaches nobody (no app is alive).
	h.clickAt(2, 2)
	if n := len(h.hb.Windows()); n != 1 {
		t.Errorf("%d windows", n)
	}

	// The next good build starts fresh and clears the overlay.
	h.s.push(evKey{'r'})
	h.expect("fresh start", 2, 0, 0)
	h.waitFrame("cleared", h.plain(child(2, 0, 0)))
}

func TestOverlayResizeRedraws(t *testing.T) {
	h := overlayHarness(t)
	h.startBig()
	h.fb.mu.Lock()
	h.fb.fail[2] = overlayBuildError
	h.fb.mu.Unlock()
	h.s.push(evKey{'r'})
	h.logged("build failed", 1)
	p := h.view.lastProblem()
	h.waitFrame("the panel", h.withOverlay(child(1, 0, 0), p))

	w := h.window()
	// Metrics change when the window handles the ConfigureEvent, so wait
	// for it to present at the new size before computing what it should
	// show.
	waitWidth := func(dx int) {
		t.Helper()
		deadline := time.Now().Add(devWait)
		for f := w.Frame(); f.Rect.Dx() != dx; f = w.Frame() {
			if time.Now().After(deadline) {
				t.Fatalf("the window never presented %d pixels wide: %v", dx, f.Rect)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	w.Resize(platform.Size{W: 500, H: 360})
	waitWidth(500)
	h.waitFrame("the panel at the new size", h.withOverlay(child(1, 0, 0), p))
	w.SetScale(2)
	waitWidth(1000)
	h.waitFrame("the panel at scale 2", h.withOverlay(child(1, 0, 0), p))
}

func TestOverlayCostsNothingWithoutProblem(t *testing.T) {
	h := overlayHarness(t)
	h.startBig()
	h.click()
	h.expect("the app works", 1, 1, 1)
	var active bool
	h.ui(func() { active = h.s.ov.active() })
	if active {
		t.Error("the overlay is active with no problem")
	}
}
