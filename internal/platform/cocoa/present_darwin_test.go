//go:build darwin

package cocoa

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The IOSurface swap chain (present_darwin.go, ADR-278): presents with
// small damages, several before one capture, a resize and a transparent
// window must show the same pixels a full present of the same frame
// would. Each check reads the window back from the window server
// (CGWindowListCreateImage), compares it with the frame the test keeps,
// and logs the chain: its surfaces, which the window server still uses,
// and the bytes the last present copied.

// damageScene is the frame as the app keeps it and presents it.
type damageScene struct {
	t    *testing.T
	fb   *image.RGBA
	base func(p image.Point) *image.RGBA // the frame at a new size
	// lastDamage is the area of the last present, in bytes.
	lastDamage int
	captures   int  // checks that read the window back
	cgImage    bool // the CGImage path is expected (presentCGImage)
	created    int  // the surfaces created when the previous step was checked
}

// paint is the FrameEvent painter: a full present of the base frame at a
// new size, nothing otherwise (the steps present their damages).
func (s *damageScene) paint(_ *ctl, _ int, w *Window) {
	p := w.Metrics().Physical()
	if s.fb != nil && s.fb.Rect.Size() == p {
		return
	}
	s.fb = s.base(p)
	s.lastDamage = len(s.fb.Pix)
	if err := w.Present(s.fb, nil); err != nil {
		s.t.Errorf("Present: %v", err)
	}
}

// change fills rects of the frame with c (premultiplied) and presents
// them as the damage.
func (s *damageScene) change(w *Window, c color.RGBA, rects ...image.Rectangle) {
	s.lastDamage = 0
	for _, r := range rects {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				s.fb.SetRGBA(x, y, c)
			}
		}
		s.lastDamage += 4 * r.Dx() * r.Dy()
	}
	if err := w.Present(s.fb, rects); err != nil {
		s.t.Errorf("Present: %v", err)
	}
}

// frameMatch is a capture compared with the frame.
type frameMatch struct {
	captured bool
	bad      int // pixels off by more than matchTolerance (a stale region)
	maxDiff  int // largest channel difference, alpha included
	first    string
	note     string
}

// matchTolerance separates color management (a few levels) from a stale
// region (the test's colors differ by far more).
const matchTolerance = 24

// matchInset keeps the comparison away from the window's rounded corners.
const matchInset = 30

func (m frameMatch) String() string {
	if !m.captured {
		return "no capture: " + m.note
	}
	s := fmt.Sprintf("%d pixel(s) off by more than %d, max channel difference %d", m.bad, matchTolerance, m.maxDiff)
	if m.first != "" {
		s += "; first " + m.first
	}
	return s
}

// matchCapture reads the window back and compares it with want inside
// the inset rectangle.
func matchCapture(w *Window, want *image.RGBA) frameMatch {
	r := captureWindow(w)
	if r.pix == nil {
		return frameMatch{note: r.note}
	}
	m := frameMatch{captured: true}
	o := r.pix.Rect.Min
	area := want.Rect.Inset(matchInset)
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			g, e := r.pix.RGBAAt(o.X+x, o.Y+y), want.RGBAAt(x, y)
			d := max(channelDiff(g, e), abs(int(g.A)-int(e.A)))
			m.maxDiff = max(m.maxDiff, d)
			if d > matchTolerance {
				if m.bad == 0 {
					m.first = fmt.Sprintf("at (%d,%d): read %v, presented %v", x, y, g, e)
				}
				m.bad++
			}
		}
	}
	return m
}

func abs(v int) int { return max(v, -v) }

// chainState describes a window's swap chain, for logs.
func chainState(w *Window) string {
	c := &w.chain
	if w.chainErr != nil {
		return "CGImage fallback: " + w.chainErr.Error()
	}
	var use []string
	for i, s := range c.surfs {
		mark := ""
		if i == c.front {
			mark = "*"
		}
		use = append(use, fmt.Sprintf("%d%s in use %v", i, mark, darwin.IOSurfaceIsInUse(s.ref)))
	}
	return fmt.Sprintf("%d surface(s) of %v (%d created; %s), the last present copied %d bytes, %d in all",
		len(c.surfs), c.size, c.created, strings.Join(use, ", "), c.lastCopied, c.totalCopied)
}

// matchStep is a step that does do, then waits until the capture matches
// the frame (or 2 s pass) and reports the comparison. A refused capture
// is a note while no step of the scene read the window back (the pixels
// are then not checked), an error after one did.
func matchStep(t *testing.T, s *damageScene, name string, do func(c *ctl), extra func(c *ctl)) ctlStep {
	return matchStepWhen(t, s, name, do, nil, extra)
}

// matchStepWhen is matchStep that compares only once ready holds.
func matchStepWhen(t *testing.T, s *damageScene, name string, do func(c *ctl), ready func(c *ctl) bool, extra func(c *ctl)) ctlStep {
	var last frameMatch
	return ctlStep{name: name, do: do,
		until: func(c *ctl) bool {
			if !c.after(100*time.Millisecond) || ready != nil && !ready(c) {
				return false
			}
			last = matchCapture(c.w[0], s.fb)
			return last.captured && last.bad == 0 || c.after(2*time.Second)
		},
		check: func(c *ctl) {
			w := c.w[0]
			t.Logf("  %s: %v; %s", name, last, chainState(w))
			if s.lastDamage > 0 {
				t.Logf("  the damage was %d bytes, a full frame %d", s.lastDamage, len(s.fb.Pix))
			}
			if last.captured {
				s.captures++
			}
			switch {
			case !last.captured && s.captures > 0:
				t.Errorf("%s: the capture was refused after earlier steps read the window back: %v", name, last)
			case !last.captured:
				t.Logf("note: the capture was refused; the pixels are not checked")
			case last.bad > 0:
				t.Errorf("%s: the window does not show the presented frame: %v", name, last)
			case last.maxDiff > 2:
				t.Logf("note: the window server changed the pixels by up to %d levels (color management)", last.maxDiff)
			}
			switch {
			case s.cgImage && (w.chainErr == nil || len(w.chain.surfs) != 0):
				t.Errorf("%s: the window uses surfaces with presentCGImage set", name)
			case !s.cgImage && w.chainErr != nil:
				t.Errorf("%s: the window fell back to the CGImage path: %v", name, w.chainErr)
			}
			if extra != nil {
				extra(c)
			}
			s.created = w.chain.created
		}}
}

// TestPresentDamage presents an opaque window's frame, then small damages
// one at a time, two presents before one capture, three rectangles in one
// present and a moving caret, then resizes the window and damages it
// again: every capture must equal the frame.
func TestPresentDamage(t *testing.T) {
	s := &damageScene{t: t, base: testPattern}
	at := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
	red, green, blue := color.RGBA{230, 20, 20, 255}, color.RGBA{20, 200, 40, 255}, color.RGBA{30, 40, 220, 255}
	steps := []ctlStep{
		matchStep(t, s, "the first frame", nil, nil),
		matchStep(t, s, "one small damage", func(c *ctl) { s.change(c.w[0], red, at(40, 40, 20, 12)) }, nil),
		matchStep(t, s, "another small damage", func(c *ctl) { s.change(c.w[0], green, at(150, 50, 16, 16)) }, nil),
		matchStep(t, s, "two presents before one capture", func(c *ctl) {
			s.change(c.w[0], blue, at(60, 120, 30, 10))
			s.change(c.w[0], color.RGBA{250, 200, 0, 255}, at(45, 44, 40, 20)) // over the first damage
		}, nil),
		matchStep(t, s, "three rectangles in one present", func(c *ctl) {
			s.change(c.w[0], color.RGBA{0, 0, 0, 255}, at(35, 35, 5, 5), at(100, 100, 10, 3), at(200, 140, 7, 9))
		}, nil),
	}
	// A caret moving and blinking: one-pixel-wide damages.
	for i := range 6 {
		x := 80 + 3*(i/2)
		c := color.RGBA{0, 0, 0, 255}
		if i%2 == 1 {
			c = color.RGBA{255, 255, 255, 255}
		}
		steps = append(steps, matchStep(t, s, fmt.Sprintf("caret %d", i), func(cc *ctl) { s.change(cc.w[0], c, at(x, 70, 1, 18)) },
			func(cc *ctl) {
				// Unless the present added a surface (a whole frame).
				if w := cc.w[0]; w.chainErr == nil && w.chain.created == s.created && w.chain.lastCopied > 4*len(s.fb.Pix)/10 {
					t.Errorf("a caret present copied %d bytes of a %d-byte frame", w.chain.lastCopied, len(s.fb.Pix))
				}
			}))
	}
	steps = append(steps,
		// Presented while hidden (ADR-185: core presents nothing then,
		// but the backend keeps what it is given), then shown.
		ctlStep{name: "hidden", do: func(c *ctl) { c.w[0].SetVisible(false) },
			until: func(c *ctl) bool { return !c.w[0].shown && c.after(200*time.Millisecond) }},
		matchStepWhen(t, s, "a damage while hidden, then shown", func(c *ctl) {
			s.change(c.w[0], color.RGBA{120, 0, 160, 255}, at(170, 100, 20, 20))
			c.w[0].SetVisible(true)
		}, func(c *ctl) bool { return c.w[0].shown && c.after(400*time.Millisecond) }, nil),
		matchStepWhen(t, s, "a resize", func(c *ctl) { c.w[0].SetSize(platform.Size{W: 300, H: 230}) }, func(c *ctl) bool {
			m := c.w[0].Metrics()
			return sizeIs(m.Size, 300, 230) && m.Physical() == s.fb.Rect.Size()
		}, func(c *ctl) {
			if p := c.w[0].Metrics().Physical(); p != s.fb.Rect.Size() || c.w[0].chain.size != p {
				t.Errorf("after the resize the window is %v, the frame %v, the surfaces %v", p, s.fb.Rect.Size(), c.w[0].chain.size)
			}
		}),
		matchStep(t, s, "a damage after the resize", func(c *ctl) { s.change(c.w[0], red, at(220, 160, 30, 20)) }, nil),
		matchStep(t, s, "another damage after the resize", func(c *ctl) { s.change(c.w[0], green, at(50, 150, 12, 30)) }, nil),
		matchStep(t, s, "a third damage after the resize", func(c *ctl) { s.change(c.w[0], blue, at(120, 40, 40, 6)) }, func(c *ctl) {
			if n := len(c.w[0].chain.surfs); c.w[0].chainErr == nil && (n < 2 || n > maxSurfaces) {
				t.Errorf("the chain holds %d surfaces, want 2 to %d", n, maxSurfaces)
			}
		}),
	)
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela damage", Size: platform.Size{W: 260, H: 190}, HasPosition: true, Position: image.Pt(80, 120)},
	}, s.paint, steps...)
}

// TestPresentDamageTransparent presents a transparent window's card with
// small damages that open a hole, make a translucent patch and close the
// hole: every capture must equal the frame, alpha included, and the input
// mask must follow (ADR-249).
func TestPresentDamageTransparent(t *testing.T) {
	card := image.Rect(40, 35, 200, 150)
	s := &damageScene{t: t, base: func(p image.Point) *image.RGBA {
		fb := image.NewRGBA(image.Rectangle{Max: p})
		for y := card.Min.Y; y < card.Max.Y; y++ {
			for x := card.Min.X; x < card.Max.X; x++ {
				fb.SetRGBA(x, y, color.RGBA{0xf0, 0xe8, 0xe0, 0xff})
			}
		}
		return fb
	}}
	hole := image.Rect(60, 50, 90, 80)
	patch := image.Rect(120, 60, 150, 90)
	// scaled returns the center of r (physical pixels) in points.
	scaled := func(w *Window, r image.Rectangle) platform.Point {
		sc := float32(w.Metrics().Scale)
		return platform.Point{X: float32(r.Min.X+r.Max.X) / 2 / sc, Y: float32(r.Min.Y+r.Max.Y) / 2 / sc}
	}
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela damage transparent", Size: platform.Size{W: 240, H: 190}, Frameless: true, Transparent: true,
			HasPosition: true, Position: image.Pt(420, 140)},
	}, s.paint,
		matchStep(t, s, "the card", nil, func(c *ctl) {
			if !c.w[0].takesInput(scaled(c.w[0], hole)) {
				t.Error("the card takes no input")
			}
		}),
		matchStep(t, s, "a hole in the card", func(c *ctl) { s.change(c.w[0], color.RGBA{}, hole) }, func(c *ctl) {
			if c.w[0].takesInput(scaled(c.w[0], hole)) {
				t.Error("the hole takes input")
			}
		}),
		matchStep(t, s, "a translucent patch", func(c *ctl) { s.change(c.w[0], color.RGBA{0x60, 0x10, 0x10, 0x80}, patch) }, func(c *ctl) {
			if !c.w[0].takesInput(scaled(c.w[0], patch)) {
				t.Error("the translucent patch (alpha 128) takes no input")
			}
		}),
		matchStep(t, s, "the hole closed", func(c *ctl) { s.change(c.w[0], color.RGBA{0x30, 0x50, 0x90, 0xff}, hole) }, func(c *ctl) {
			if !c.w[0].takesInput(scaled(c.w[0], hole)) {
				t.Error("the closed hole takes no input")
			}
		}),
		matchStep(t, s, "the patch cleared", func(c *ctl) { s.change(c.w[0], color.RGBA{}, patch) }, nil),
	)
}

// TestPresentCGImageFallback runs the CGImage path of before
// (presentCGImage) through a damage: the fallback must still work.
func TestPresentCGImageFallback(t *testing.T) {
	presentCGImage = true
	defer func() { presentCGImage = false }()
	s := &damageScene{t: t, base: testPattern, cgImage: true}
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela cgimage", Size: platform.Size{W: 220, H: 170}, HasPosition: true, Position: image.Pt(80, 400)},
	}, s.paint,
		matchStep(t, s, "the first frame", nil, nil),
		matchStep(t, s, "a damage", func(c *ctl) { s.change(c.w[0], color.RGBA{200, 30, 30, 255}, image.Rect(50, 50, 90, 70)) }, nil),
	)
}
