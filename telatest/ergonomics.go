package telatest

import (
	"fmt"
	"image/color"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// MountFunc shows the component fn in a headless window and returns the
// running app: it is Mount(tb, tela.C(fn), opts...), for the common case
// of a test that mounts one component.
//
//	app := telatest.MountFunc(t, Counter, telatest.Size(400, 300))
//
// See [Mount] for what the app does and when it ends.
func MountFunc(tb testing.TB, fn func() tela.Node, opts ...Option) *App {
	tb.Helper()
	return Mount(tb, tela.C(fn), opts...)
}

// GoldenScales compares the window with one golden per display scale:
// testdata/<name>_x<scale>.png, with the scale written like %g (name_x1.png,
// name_x2.png, name_x1.5.png), the naming of the examples' goldens. A name
// with the ".png" extension has it removed first. For each scale the
// window is moved to a display with that scale, as when a user drags it
// between monitors (the logical size is kept, the picture is
// size*scale pixels), settled like [App.Golden] does, and compared; the
// app keeps its state and its original scale at the end. The comparison is
// bit by bit and TELA_UPDATE=1 writes the goldens, as with [App.Golden].
//
//	app.GoldenScales("settings", 1, 2) // testdata/settings_x1.png, settings_x2.png
func (a *App) GoldenScales(name string, scales ...float64) {
	a.tb.Helper()
	a.mustBeOpen()
	if len(scales) == 0 {
		a.fatalf("GoldenScales(%q) needs at least one scale, such as GoldenScales(%q, 1, 2)", name, name)
	}
	for _, s := range scales {
		if !(s > 0) || math.IsInf(s, 0) {
			a.fatalf("GoldenScales(%q): the display scale %g must be positive, such as 1, 1.5 or 2", name, s)
		}
	}
	base := strings.TrimSuffix(name, ".png")
	orig := a.win.Metrics().Scale
	defer func() {
		if !a.closed && a.win.Metrics().Scale != orig {
			a.win.SetScale(orig)
			a.settleAfter()
		}
	}()
	for _, s := range scales {
		a.win.SetScale(s)
		golden.Image(a.tb, fmt.Sprintf("%s_x%g.png", base, s), a.Screenshot())
	}
}

// PixelAt returns the color of the window's current frame at the point
// (x, y), in logical pixels from the top-left corner. The frame is in
// device pixels, so the sampled one is the device pixel that contains the
// point: (floor(x*scale), floor(y*scale)), where scale is the window's
// display scale; at scale 2, PixelAt(10, 10) reads the pixel (20, 20),
// the top-left one of the four that cover the logical pixel. The app is
// settled first (with [NoSettle], only the work due is handled). The color
// is as stored in the frame (alpha-premultiplied; windows are opaque, so
// A is 255). It fails the test when the point is outside the window.
//
//	if c := app.PixelAt(40, 20); c != (color.RGBA{R: 59, G: 130, B: 246, A: 255}) {
//	    t.Errorf("the button is %v, want blue-500", c)
//	}
func (a *App) PixelAt(x, y float32) color.RGBA {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleAfter()
	img := a.win.Frame()
	if img == nil {
		a.fatalf("the window has not presented a frame yet (is the window zero-sized?)")
	}
	scale := a.win.Metrics().Scale
	px, py := int(math.Floor(float64(x)*scale)), int(math.Floor(float64(y)*scale))
	if !(inRange(img.Rect.Min.X, img.Rect.Max.X, px) && inRange(img.Rect.Min.Y, img.Rect.Max.Y, py)) {
		a.fatalf("PixelAt(%g, %g): the point is outside the window, which is %gx%g logical pixels (%dx%d device pixels at scale %g)",
			x, y, float64(img.Rect.Dx())/scale, float64(img.Rect.Dy())/scale, img.Rect.Dx(), img.Rect.Dy(), scale)
	}
	return img.RGBAAt(px, py)
}

func inRange(min, max, v int) bool { return v >= min && v < max }

// Measure runs f, which performs actions on the app, settles the app
// afterwards like an action does (with [NoSettle], only the work due is
// handled) and returns the work the tree did meanwhile: the difference of
// two readings of [App.Stats], so that a test can assert how narrow an
// update was without keeping the readings itself. The app is brought to
// rest before f runs, so work pending from earlier steps is not counted.
//
//	st := app.Measure(func() { app.Click(telatest.ByRole("button", "Increment")) })
//	if st.Builds != 1 { t.Errorf("the click rebuilt %d components, want 1", st.Builds) }
func (a *App) Measure(f func()) Stats {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleAfter()
	before := a.tree.Stats()
	f()
	a.settleAfter()
	return statsDiff(before, a.tree.Stats())
}

// statsDiff returns after minus before, field by field.
func statsDiff(before, after Stats) Stats {
	var d Stats
	b, af, out := reflect.ValueOf(before), reflect.ValueOf(after), reflect.ValueOf(&d).Elem()
	for i := range out.NumField() {
		out.Field(i).SetInt(af.Field(i).Int() - b.Field(i).Int())
	}
	return d
}
