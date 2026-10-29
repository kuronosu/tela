package platform

import (
	"fmt"
	"image"
	"image/draw"
	"time"
)

// WindowState is the OS state of a window, reported by [WindowStateEvent]
// and returned by [Window.State]. The zero value is a normal, decorated
// window at an unknown position.
type WindowState struct {
	// Maximized: the window fills the work area of its output.
	Maximized bool
	// Fullscreen: the window covers its whole output without decorations.
	// A window can be both maximized and fullscreen (it returns to
	// maximized when it leaves fullscreen). A minimized window keeps
	// reporting the Maximized and Fullscreen state it returns to when it
	// is restored (what an app saves as its placement).
	Fullscreen bool
	// Minimized: the window is iconified. Known on Win32 (which also
	// reports an empty ConfigureEvent), X11 (WM_STATE Iconic or
	// _NET_WM_STATE_HIDDEN) and Cocoa; Wayland has no way to know, so it
	// stays false there.
	Minimized bool
	// ClientSideDecorations: the OS draws no title bar or borders for the
	// window, so the app draws its own title bar and declares it with
	// Window.SetHitRegions. Always true for WindowOptions.Frameless
	// windows; also true on Wayland when the compositor has no server-side
	// decorations (no xdg-decoration, or it chose client-side), which is
	// why it is state and not only an option.
	ClientSideDecorations bool
	// HasPosition reports whether Position is known: on X11 and Win32 (and
	// headless once the app placed the window), never on Wayland.
	HasPosition bool
	// Position is the top-left of the content area in physical pixels of
	// the OS's desktop coordinates (Win32 virtual screen, X11 root
	// window). It is meant to be saved and given back to
	// WindowOptions.Position or Window.SetPosition, not to be mixed with
	// logical coordinates.
	Position image.Point
}

func (s WindowState) String() string {
	out := "{"
	add := func(f string) {
		if len(out) > 1 {
			out += " "
		}
		out += f
	}
	if s.Maximized {
		add("maximized")
	}
	if s.Fullscreen {
		add("fullscreen")
	}
	if s.Minimized {
		add("minimized")
	}
	if s.ClientSideDecorations {
		add("csd")
	}
	if s.HasPosition {
		add(fmt.Sprintf("at %d,%d", s.Position.X, s.Position.Y))
	}
	return out + "}"
}

// HitKind is what a [HitRegion] of a custom title bar is to the OS.
type HitKind uint8

const (
	// HitClient is ordinary content. It is what lies outside every
	// region, and a HitClient region punches a hole into the regions
	// listed before it (a search field or a tab strip inside a caption
	// area).
	HitClient HitKind = iota
	// HitCaption is a drag area of the title bar: the OS handles presses
	// in it (move, double-click, window menu).
	HitCaption
	// HitMinimize, HitMaximize and HitClose are the window buttons drawn
	// by the app. Pointer events over them reach the app as usual (its
	// buttons perform the action); the region tells the OS what they are
	// (Windows 11 snap layouts open from the maximize button).
	HitMinimize
	HitMaximize
	HitClose
)

var hitKindNames = [...]string{"client", "caption", "minimize", "maximize", "close"}

func (k HitKind) String() string {
	if int(k) < len(hitKindNames) {
		return hitKindNames[k]
	}
	return fmt.Sprintf("HitKind(%d)", uint8(k))
}

// HitRegion is one rectangle of a custom title bar; see
// [Window.SetHitRegions].
type HitRegion struct {
	Kind HitKind
	// Rect is in logical pixels, window coordinates (like pointer
	// positions).
	Rect Rect
}

// HitTest returns the kind of the last region of regions that contains p
// (later regions are on top), or HitClient when none does. A rectangle
// contains the points from its top-left edge inclusive to its
// bottom-right edge exclusive. Backends use it to answer the OS (Win32
// WM_NCHITTEST) or to decide what a press does (Wayland, X11, headless).
func HitTest(regions []HitRegion, p Point) HitKind {
	for i := len(regions) - 1; i >= 0; i-- {
		r := regions[i].Rect
		if p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H {
			return regions[i].Kind
		}
	}
	return HitClient
}

// CaptionClicks detects the double clicks of primary presses in a
// HitCaption region, for the backends that toggle maximize themselves
// (Wayland, X11, headless), with the double-click rule of content clicks
// (ADR-192). The zero value has no previous press.
type CaptionClicks struct {
	last time.Duration
	pos  Point
	has  bool
}

// Press records a primary caption press at time t and position pos and
// reports whether it completes a double click: the previous press was at
// most s.DoubleClickTime() earlier and at most DoubleClickSlop logical
// pixels away. A press that completes a double click is not recorded, so a
// third press starts over.
func (c *CaptionClicks) Press(t time.Duration, pos Point, s SystemSettings) bool {
	dx, dy := float64(pos.X-c.pos.X), float64(pos.Y-c.pos.Y)
	if c.has && t >= c.last && t-c.last <= s.DoubleClickTime() && float64(dx*dx)+float64(dy*dy) <= DoubleClickSlop*DoubleClickSlop {
		c.has = false
		return true
	}
	c.has, c.last, c.pos = true, t, pos
	return false
}

// Reset forgets the last press (one that became a drag is not half of a
// double click).
func (c *CaptionClicks) Reset() { c.has = false }

// CheckHitRegions panics with an actionable message if a region has an
// invalid kind or a rectangle with a negative, NaN or infinite component.
func CheckHitRegions(regions []HitRegion) {
	for i, r := range regions {
		if r.Kind > HitClose {
			panicf("platform: SetHitRegions: region %d has invalid kind %d; use one of the platform.Hit… constants", i, r.Kind)
		}
		for _, v := range [4]float32{r.Rect.X, r.Rect.Y, r.Rect.W, r.Rect.H} {
			if v != v || v > 1e7 || v < -1e7 {
				panicf("platform: SetHitRegions: region %d has invalid rectangle %v; coordinates must be finite", i, r.Rect)
			}
		}
		if r.Rect.W < 0 || r.Rect.H < 0 {
			panicf("platform: SetHitRegions: region %d has a negative size %v", i, r.Rect)
		}
	}
}

// EqualHitRegions reports whether a and b are the same list (nil and empty
// are equal). Backends use it to ignore repeated SetHitRegions calls.
func EqualHitRegions(a, b []HitRegion) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// MaxIconSide bounds the side of an icon image in pixels (window, tray and
// notification icons). X11 sends every size in one property, so huge
// images are a mistake.
const MaxIconSide = 1024

// CheckIcon panics with an actionable message if images, an icon given at
// several sizes, has a nil image, an empty one or one with a side above
// MaxIconSide. what names the value in the message. An empty list is
// valid (no icon / the default icon).
func CheckIcon(what string, images []image.Image) {
	for i, img := range images {
		if img == nil {
			panicf("platform: %s: image %d is nil", what, i)
		}
		b := img.Bounds()
		if b.Empty() {
			panicf("platform: %s: image %d is empty (%v)", what, i, b)
		}
		if b.Dx() > MaxIconSide || b.Dy() > MaxIconSide {
			panicf("platform: %s: image %d is %dx%d; icon images must be at most %dx%d (give the sizes the OS uses: 16 to 256)", what, i, b.Dx(), b.Dy(), MaxIconSide, MaxIconSide)
		}
	}
}

// PickIcon returns the image of images best suited to a square slot of
// side pixels: the smallest one whose shorter side is at least side, or
// the largest one when none is big enough; the first on ties. It returns
// nil for an empty list.
func PickIcon(images []image.Image, side int) image.Image {
	var best, largest image.Image
	bestSide, largestSide := 0, 0
	for _, img := range images {
		b := img.Bounds()
		s := min(b.Dx(), b.Dy())
		if s >= side && (best == nil || s < bestSide) {
			best, bestSide = img, s
		}
		if largest == nil || s > largestSide {
			largest, largestSide = img, s
		}
	}
	if best != nil {
		return best
	}
	return largest
}

// IconNRGBA returns a copy of img as non-premultiplied RGBA with its
// top-left at (0, 0): the layout closest to what every OS icon format
// wants (X11 _NET_WM_ICON and StatusNotifierItem ARGB32, the
// notification image-data hint, Win32 32-bit icon DIBs).
func IconNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Rect, img, b.Min, draw.Src)
	return out
}

// CopyIcon returns a copy of images (each as IconNRGBA), so a backend can
// keep an icon the caller may modify afterwards. nil for an empty list.
func CopyIcon(images []image.Image) []image.Image {
	if len(images) == 0 {
		return nil
	}
	out := make([]image.Image, len(images))
	for i, img := range images {
		out[i] = IconNRGBA(img)
	}
	return out
}
