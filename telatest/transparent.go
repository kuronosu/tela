package telatest

import "github.com/kuronosu/tela/internal/platform"

// Transparent windows (tela.App.Transparent, ADR-249) in tests.

// Transparent opens the main window transparent and frameless
// (tela.App.Transparent with Frameless): the root has no background, so
// the screenshots and goldens keep the frame's alpha, transparent where
// the app paints nothing, and pointer actions over pixels whose alpha is
// under a quarter (platform.InputAlpha) go to what is below the window:
// the app gets nothing ([App.TakesInput]). [tela.WindowState].Transparent
// is true unless [NoCompositing].
//
//	app := telatest.Mount(t, Widget{}, telatest.Transparent(), telatest.Size(360, 240))
//	app.Golden("widget.png") // a PNG with alpha: the card and its shadow
func Transparent() Option {
	return func(c *config) { c.win.transparent, c.win.frameless = true, true }
}

// NoCompositing plays a desktop that cannot show transparency, as X11
// without a compositing manager: a [Transparent] window is an opaque
// frameless one, its [tela.WindowState].Transparent is false and it takes
// input everywhere. [App.SetCompositing] changes it while the app runs.
func NoCompositing() Option { return func(c *config) { c.noCompositing = true } }

// SetCompositing plays a compositing manager starting (on) or stopping
// under a [Transparent] window: the window's
// [tela.WindowState].Transparent follows, it renders opaque while off, and
// the app settles. It does nothing for other windows.
func (a *App) SetCompositing(on bool) {
	a.tb.Helper()
	a.mustBeOpen()
	a.win.SetCompositing(on)
	a.settleAfter()
}

// TakesInput reports whether a pointer event at (x, y), logical window
// coordinates, reaches the app: always, except in a [Transparent] window
// shown with transparency, where only the pixels of the presented frame
// whose alpha is at least a quarter take input (the rest goes to what is
// below the window). It settles the app first.
func (a *App) TakesInput(x, y float32) bool {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleAfter()
	return a.win.TakesInput(platform.Point{X: x, Y: y})
}
