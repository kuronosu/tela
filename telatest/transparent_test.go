package telatest_test

import (
	"image"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// floatingCard is the root of a transparent widget: a rounded card with
// shadow-xl in the middle of a window larger than it, so the shadow fits.
func floatingCard(clicks, misses *int) func() tela.Node {
	return func() tela.Node {
		return Div(Class("flex h-full items-center justify-center p-10"), OnClick(func() { *misses++ }),
			Div(Class("flex flex-col gap-3 w-56 p-6 rounded-2xl bg-white shadow-xl text-stone-800"),
				Span(Class("text-sm font-medium text-stone-500"), Text("Now playing")),
				Button(Class("py-2 rounded-lg bg-stone-800 text-stone-100"), OnClick(func() { *clicks++ }), Text("Pause")),
			),
		)
	}
}

// TestTransparentWindowGolden: a Transparent window keeps the frame's
// alpha (the golden is a PNG with transparent pixels around the card and a
// translucent shadow), and clicks on the transparent margin and the shadow
// go below the window while the card takes them.
func TestTransparentWindowGolden(t *testing.T) {
	var clicks, misses int
	app := telatest.Mount(t, tela.C(floatingCard(&clicks, &misses)), telatest.Transparent(), telatest.Size(320, 200), telatest.Scale(2))
	if !app.WindowState().Transparent || !app.WindowState().ClientSideDecorations {
		t.Fatalf("WindowState = %+v, want Transparent and ClientSideDecorations", app.WindowState())
	}
	img := app.Screenshot()
	at := func(x, y int) uint8 { return img.RGBAAt(x, y).A }
	if a := at(2, 2); a != 0 {
		t.Errorf("the window's corner has alpha %d, want 0 (the root has no background)", a)
	}
	if a := at(320, 200); a != 255 {
		t.Errorf("the card's middle has alpha %d, want 255", a)
	}
	// Just below the card, its shadow: visible, under the input alpha.
	bottom := cardBottom(img)
	if a := at(320, bottom+8); a == 0 || a >= 64 {
		t.Errorf("the shadow below the card has alpha %d, want between 0 and 64", a)
	}
	app.Golden("transparent_card.png")

	if app.TakesInput(5, 5) || app.TakesInput(160, float32(bottom+8)/2) {
		t.Error("the transparent margin or the shadow takes input")
	}
	if !app.TakesInput(160, 100) {
		t.Error("the card takes no input")
	}
	app.ClickAt(5, 5)
	app.ClickAt(160, float32(bottom+8)/2)
	if misses != 0 {
		t.Errorf("%d clicks on transparent pixels reached the app", misses)
	}
	app.Click(telatest.ByRole("button", "Pause"))
	if clicks != 1 || misses != 1 {
		t.Errorf("clicks %d, misses %d after a click on the button, want 1 and 1 (it bubbles to the root)", clicks, misses)
	}
}

// cardBottom returns the first row below the card's middle (physical
// pixels) where the alpha drops below 255.
func cardBottom(img *image.RGBA) int {
	x := img.Rect.Dx() / 2
	y := img.Rect.Dy() / 2
	for y < img.Rect.Dy() && img.RGBAAt(x, y).A == 255 {
		y++
	}
	return y
}

// TestTransparentWindowWithoutCompositing: where the desktop cannot show
// transparency the window renders opaque (white where the root has no
// background), reports it, and takes input everywhere; it comes back when
// compositing does.
func TestTransparentWindowWithoutCompositing(t *testing.T) {
	var clicks, misses int
	app := telatest.Mount(t, tela.C(floatingCard(&clicks, &misses)), telatest.Transparent(), telatest.NoCompositing(), telatest.Size(320, 200))
	if app.WindowState().Transparent {
		t.Fatal("WindowState().Transparent without compositing")
	}
	if c := app.Screenshot().RGBAAt(2, 2); c.A != 255 || c.R != 255 {
		t.Errorf("the corner is %v without compositing, want opaque white", c)
	}
	if !app.TakesInput(5, 5) {
		t.Error("an opaque window lets input through")
	}
	app.ClickAt(5, 5)
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
	app.SetCompositing(true)
	if !app.WindowState().Transparent {
		t.Fatal("WindowState().Transparent stays false with compositing")
	}
	if a := app.Screenshot().RGBAAt(2, 2).A; a != 0 {
		t.Errorf("the corner has alpha %d with compositing, want 0", a)
	}
}

// MountApp takes the App's Transparent field; Transparent without
// Frameless is a mistake Run reports.
func TestMountAppTransparent(t *testing.T) {
	app := telatest.MountApp(t, tela.App{Transparent: true, Frameless: true, Width: 100, Height: 80,
		Root: tela.C(func() tela.Node { return Div(Class("m-4 h-10 rounded bg-sky-500")) })})
	if !app.WindowState().Transparent {
		t.Fatal("MountApp dropped Transparent")
	}
	if app.TakesInput(1, 1) || !app.TakesInput(20, 20) {
		t.Error("the input mask does not follow the frame")
	}
}

// Render (tela snapshot) draws a Transparent app with its alpha too.
func TestRenderTransparentApp(t *testing.T) {
	var clicks, misses int
	app := tela.App{Transparent: true, Frameless: true, Width: 320, Height: 200, Root: tela.C(floatingCard(&clicks, &misses))}
	img, err := telatest.Render(app.Root, telatest.FromApp(app))
	if err != nil {
		t.Fatal(err)
	}
	if a := img.RGBAAt(2, 2).A; a != 0 {
		t.Errorf("the corner has alpha %d, want 0", a)
	}
	if a := img.RGBAAt(160, 100).A; a != 255 {
		t.Errorf("the card has alpha %d, want 255", a)
	}
}

// TestTransparentWindowResize (ADR-257): h.WindowResize on the card makes
// a band inside its pixels the window's resize handles. A press on the
// band starts an OS resize (and is not the app's) while the rounded
// corner's transparent pixels still pass the pointer below; the content
// inside the band keeps its clicks.
func TestTransparentWindowResize(t *testing.T) {
	var clicks, misses int
	root := func() tela.Node {
		return Div(Class("h-full p-4"), OnClick(func() { misses++ }),
			Div(Class("flex h-full flex-col rounded-2xl bg-white p-6"), WindowResize(), OnClick(func() { clicks++ }),
				Div(Class("h-8"), WindowDrag()),
			),
		)
	}
	app := telatest.Mount(t, tela.C(root), telatest.Transparent(), telatest.Size(200, 120))
	if k := app.HitAt(18, 60); k != telatest.HitResizeLeft {
		t.Errorf("HitAt the card's left edge = %v, want resize-left", k)
	}
	if k := app.HitAt(100, 18); k != telatest.HitResizeTop {
		t.Errorf("HitAt the card's top edge, over its drag area = %v, want resize-top", k)
	}
	if k := app.HitAt(174, 101); k != telatest.HitResizeBottomRight {
		t.Errorf("HitAt the bottom-right corner = %v, want resize-bottom-right", k)
	}
	app.ClickAt(18, 60)
	app.ClickAt(174, 101) // inside the rounding
	if got := app.ResizeStarts(); len(got) != 2 || got[0] != tela.EdgeLeft || got[1] != tela.EdgeBottom|tela.EdgeRight {
		t.Errorf("ResizeStarts = %v, want [left bottom-right]", got)
	}
	// The corner pixel of the rounded card is transparent: below the
	// window, not a resize.
	if app.TakesInput(16.5, 16.5) {
		t.Error("the rounded corner's outer pixel takes input")
	}
	app.ClickAt(16.5, 16.5)
	if n := len(app.ResizeStarts()); n != 2 {
		t.Errorf("%d resizes after a click through the corner, want 2", n)
	}
	app.ClickAt(100, 80)
	if clicks != 1 || misses != 1 {
		t.Errorf("clicks %d, misses %d, want the one click inside the card (bubbling to the root)", clicks, misses)
	}
}
