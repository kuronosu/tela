package core

import (
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// Window styling (ADR-187): the colors of the OS title bar and border and
// the system backdrop (Windows 11), as fields of WindowSpec.

// Backdrop is the system material behind a window (tela.Backdrop).
type Backdrop = platform.Backdrop

// The backdrops (tela.BackdropDefault and its siblings).
const (
	BackdropDefault = platform.BackdropDefault
	BackdropNone    = platform.BackdropNone
	BackdropMica    = platform.BackdropMica
	BackdropMicaAlt = platform.BackdropMicaAlt
	BackdropAcrylic = platform.BackdropAcrylic
)

// TitleBarColors are the colors of the OS-drawn title bar and border
// (tela.TitleBarColors). The zero Color, and any color with zero alpha, is
// the system's; the alpha of the others is ignored.
type TitleBarColors struct {
	Background, Text, Border gfx.Color
}

// platformColors converts the colors for the platform.
func (c TitleBarColors) platform() platform.TitleBarColors {
	return platform.TitleBarColors{
		Background: windowColor(c.Background),
		Text:       windowColor(c.Text),
		Border:     windowColor(c.Border),
	}
}

// windowColor converts c to 8-bit sRGB; a color with no alpha (or NaN) is
// the system's.
func windowColor(c gfx.Color) platform.WindowColor {
	if !(c.A > 0) {
		return platform.WindowColor{}
	}
	return platform.WindowColor{R: to8(c.R), G: to8(c.G), B: to8(c.B), Set: true}
}

func to8(v float32) uint8 {
	f := math.Round(float64(v) * 255)
	switch {
	case !(f > 0):
		return 0
	case f > 255:
		return 255
	}
	return uint8(f)
}

// syncCanvas makes the tree's canvas transparent when the platform window
// shows the alpha of the frames (platform.AlphaPresenter), and opaque (as
// always) when it does not.
func (v *winView) syncCanvas() {
	on := false
	if p, ok := v.win.(platform.AlphaPresenter); ok {
		on = p.PresentsAlpha()
	}
	v.tree.SetTransparentCanvas(on)
}

// SetTransparentCanvas makes the tree render its window background with
// its alpha, over transparent pixels: a root without a background leaves
// the window transparent (premultiplied {0,0,0,0}) for the system
// backdrop to show. Off, the default, the background is composed over
// white and the frame is opaque. Call it on the UI thread.
func (t *Tree) SetTransparentCanvas(on bool) {
	if t.transCanvas == on {
		return
	}
	t.transCanvas = on
	t.lay.needFull = true
	t.lay.ras = nil // rasterize the whole frame again
	t.dirty()
}

// clearColor is the color the frame is cleared to before the display
// list draws over it: the canvas, which the list fills again, is opaque,
// or transparent for a transparent canvas (filling a translucent canvas
// over itself would count its alpha twice).
func (t *Tree) clearColor() gfx.Color {
	if t.transCanvas {
		return gfx.Transparent
	}
	return t.CanvasColor()
}

// backdropList names the valid backdrops as the public constants.
func backdropList() string {
	names := platform.BackdropNames()
	for i, n := range names {
		names[i] = "tela.Backdrop" + n
	}
	return strings.Join(names, ", ")
}
