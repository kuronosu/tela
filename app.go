package tela

import (
	"github.com/kuronosu/tela/internal/core"
)

// App describes an application window for [Run].
type App struct {
	// Title is the window title.
	Title string
	// AppID identifies the application to the desktop environment (the
	// Wayland app_id, the X11 WM_CLASS). Use a reverse-DNS name such as
	// "com.example.notes". Empty lets the platform pick one.
	AppID string
	// Width and Height are the requested size of the window in logical
	// pixels. Zero means the platform's default size.
	Width, Height float32
	// RootClass are Tailwind classes for the root, which fills the
	// window. Use it to give the window a background ("bg-slate-900") or
	// a text style; without it the window is white and the text black.
	RootClass string
	// Root is the root component. It is built as a component of its own,
	// so it may use hooks.
	Root func() Node
}

// Run opens a window, shows app.Root and runs the event loop until the
// window is closed. It must be called from the main goroutine, normally
// from main. The window is drawn by Tela's own renderer on the native
// backend: Wayland or X11 on Linux, Win32 and Cocoa elsewhere.
//
// Run returns an error when the platform cannot open a window (for
// example, no display is available). It panics when app.Root is nil.
func Run(app App) error {
	if app.Root == nil {
		panic("tela: Run(App{...}) needs a Root component; for example tela.Run(tela.App{Title: \"Demo\", Root: Counter}) where Counter is a func() tela.Node")
	}
	return core.Run(core.Options{
		Config: core.Config{RootClass: app.RootClass},
		Title:  app.Title,
		AppID:  app.AppID,
		Width:  app.Width,
		Height: app.Height,
		Root:   core.Component(app.Root),
	})
}
