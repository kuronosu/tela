package core

import (
	"image"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
)

// The title bar colors and the backdrop (ADR-187) reach the OS window when it
// opens and when they change, and only then.
func TestWindowStyleReachesPlatform(t *testing.T) {
	var setStep Setter[int]
	navy := gfx.RGB8(0x1e, 0x1b, 0x4b)
	root := Component(func() Node {
		step, set := UseState(0)
		setStep = set
		spec := WindowSpec{Title: "S", OnClose: func() {}, Root: div(Text("x")),
			TitleBar: TitleBarColors{Background: navy}, Backdrop: BackdropMica}
		if step == 1 {
			spec.TitleBar = TitleBarColors{Background: navy, Text: gfx.White}
			spec.Backdrop = BackdropAcrylic
		}
		return div(win(spec))
	})
	x := newWinHarness(t, Options{Root: root})
	c := x.child(0)
	want := platform.TitleBarColors{Background: platform.WindowColor{R: 0x1e, G: 0x1b, B: 0x4b, Set: true}}
	if c.TitleBar() != want || c.Backdrop() != platform.BackdropMica {
		t.Fatalf("at open: title bar %+v, backdrop %v; want %+v, Mica", c.TitleBar(), c.Backdrop(), want)
	}
	if n := c.StyleChanges(); n != 0 {
		t.Errorf("opening the window made %d style requests; the options carry them", n)
	}
	x.drain() // a build with nothing changed asks for nothing
	if n := c.StyleChanges(); n != 0 {
		t.Errorf("an unchanged build made %d style requests", n)
	}
	setStep.Set(1)
	x.drain()
	want.Text = platform.WindowColor{R: 255, G: 255, B: 255, Set: true}
	if c.TitleBar() != want || c.Backdrop() != platform.BackdropAcrylic || c.StyleChanges() != 2 {
		t.Errorf("after the change: title bar %+v, backdrop %v, %d requests; want %+v, Acrylic, 2", c.TitleBar(), c.Backdrop(), c.StyleChanges(), want)
	}
	setStep.Set(0)
	x.drain()
	if c.Backdrop() != platform.BackdropMica {
		t.Errorf("back: backdrop %v, want Mica", c.Backdrop())
	}
}

// A transparent canvas leaves the pixels the app draws nothing on
// transparent, composes a translucent root once, and an opaque canvas (the
// default) is unchanged: opaque white without a background.
func TestTransparentCanvas(t *testing.T) {
	px := func(img *image.RGBA, x, y int) [4]uint8 {
		o := img.PixOffset(x, y)
		return [4]uint8{img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3]}
	}
	newTree := func(rootClass string, root Node) *Tree {
		tr := NewTree(Config{Fonts: testFonts, RootClass: rootClass})
		tr.SetViewport(gfx.Size{W: 40, H: 20})
		tr.Mount(root)
		return tr
	}
	render := func(rootClass string, transparent bool) *image.RGBA {
		tr := newTree(rootClass, div(Class("h-20 w-20"), div(Class("absolute left-0 top-0 size-[10px] bg-[#ff0000]"))))
		tr.SetTransparentCanvas(transparent)
		tr.Frame()
		fb := image.NewRGBA(image.Rect(0, 0, 40, 20))
		var r raster.Renderer
		tr.RasterizeFrame(fb, &r, true)
		return fb
	}
	opaque := render("", false)
	if got := px(opaque, 30, 15); got != [4]uint8{255, 255, 255, 255} {
		t.Errorf("default canvas: pixel %v, want opaque white", got)
	}
	glass := render("", true)
	if got := px(glass, 30, 15); got != [4]uint8{0, 0, 0, 0} {
		t.Errorf("transparent canvas, no background: pixel %v, want {0 0 0 0}", got)
	}
	if got := px(glass, 5, 5); got != [4]uint8{255, 0, 0, 255} {
		t.Errorf("transparent canvas, a red box: pixel %v, want opaque red", got)
	}
	half := render("bg-[#0000ff]/50", true)
	if got := px(half, 30, 15); got[3] < 126 || got[3] > 129 || got[2] < 126 || got[2] > 129 || got[0] != 0 || got[1] != 0 {
		t.Errorf("transparent canvas, 50%% blue root: pixel %v, want premultiplied {0 0 ~128 ~128} (composed once)", got)
	}
	// Turning it off again restores the opaque frame.
	tr := newTree("", div(Class("h-20 w-20")))
	tr.SetTransparentCanvas(true)
	tr.Frame()
	fb := image.NewRGBA(image.Rect(0, 0, 40, 20))
	var r raster.Renderer
	tr.RasterizeFrame(fb, &r, true)
	tr.SetTransparentCanvas(false)
	tr.Frame()
	tr.RasterizeFrame(fb, &r, false)
	if got := px(fb, 30, 15); got != [4]uint8{255, 255, 255, 255} {
		t.Errorf("canvas switched back to opaque: pixel %v, want opaque white", got)
	}
}

// The main window gets them from the options.
func TestMainWindowStyle(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(Text("x")),
		TitleBar: TitleBarColors{Border: gfx.RGB8(1, 2, 3)}, Backdrop: BackdropMicaAlt})
	w := x.windows()[0]
	if got := w.TitleBar().Border; got != (platform.WindowColor{R: 1, G: 2, B: 3, Set: true}) || w.Backdrop() != platform.BackdropMicaAlt {
		t.Errorf("main window: border %+v, backdrop %v", got, w.Backdrop())
	}
}

func TestWindowColorConversion(t *testing.T) {
	for _, tc := range []struct {
		in   gfx.Color
		want platform.WindowColor
	}{
		{gfx.Color{}, platform.WindowColor{}},        // the zero Color is the system's
		{gfx.Transparent, platform.WindowColor{}},    // so is any color with no alpha
		{gfx.Black, platform.WindowColor{Set: true}}, // opaque black is a color
		{gfx.RGB8(255, 128, 0), platform.WindowColor{R: 255, G: 128, Set: true}},
		{gfx.Color{R: 2, G: -1, B: 0.5, A: 0.3}, platform.WindowColor{R: 255, B: 128, Set: true}}, // clamped; alpha ignored
	} {
		if got := windowColor(tc.in); got != tc.want {
			t.Errorf("windowColor(%+v) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

// An invalid Backdrop panics with the valid names, for a Window and for the
// App.
func TestBackdropValidation(t *testing.T) {
	check := func(name string, f func()) {
		t.Helper()
		defer func() {
			msg, _ := recover().(string)
			for _, want := range []string{"Backdrop", "tela.BackdropMica", "tela.BackdropAcrylic"} {
				if !strings.Contains(msg, want) {
					t.Errorf("%s: panic %q does not mention %q", name, msg, want)
				}
			}
		}()
		f()
	}
	check("window", func() {
		s := WindowSpec{Backdrop: Backdrop(9)}
		checkWindowSpec("tela.Window", "S", &s)
	})
	check("app", func() {
		s := WindowSpec{Backdrop: Backdrop(200)}
		checkWindowSpec("tela.App", "", &s)
	})
}
