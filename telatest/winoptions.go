package telatest

import (
	"fmt"
	"image"
	"math"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// windowConfig is what the options say about the main window the way the
// OS sees it: the fields of tela.App that are not about how the content
// looks. [MountApp] fills it from the App; the options below fill it by
// hand.
type windowConfig struct {
	title, appID            string
	minW, minH, maxW, maxH  float32
	fixed, frameless, onTop bool
	hidden                  bool
	icon                    []image.Image
	titleBar                tela.TitleBarColors
	backdrop                tela.Backdrop
	placement               tela.WindowPlacement
	onClose                 func(tela.WindowHandle)
	onQuit                  func()
	overlay                 bool
	transparent             bool
}

// coreOptions returns the core options of the window (without the root,
// the size and the tree configuration).
func (w windowConfig) coreOptions() core.Options {
	return core.Options{
		Window: core.WindowSpec{
			Title:    w.title,
			MinWidth: w.minW, MinHeight: w.minH, MaxWidth: w.maxW, MaxHeight: w.maxH,
			FixedSize: w.fixed, Frameless: w.frameless, AlwaysOnTop: w.onTop, Hidden: w.hidden,
			Icon: w.icon, TitleBar: w.titleBar, Backdrop: w.backdrop, Placement: w.placement,
			TitleBarOverlay: w.overlay, Transparent: w.transparent,
		},
		AppID: w.appID, OnClose: w.onClose, OnQuit: w.onQuit,
	}
}

// validate reports a window configuration that cannot work, with the fix.
func (w windowConfig) validate(screen platform.Size) error {
	for _, f := range [...]struct {
		name string
		v    float32
	}{{"MinSize width", w.minW}, {"MinSize height", w.minH}, {"MaxSize width", w.maxW}, {"MaxSize height", w.maxH}} {
		if f.v < 0 || math.IsNaN(float64(f.v)) || math.IsInf(float64(f.v), 0) {
			return fmt.Errorf("telatest: the %s is %g; limits are logical pixels, finite and not negative (0 means no limit)", f.name, f.v)
		}
	}
	if screen.W < 0 || screen.H < 0 || (screen.W == 0) != (screen.H == 0) {
		return fmt.Errorf("telatest.Screen(%g, %g): give a positive width and height in logical pixels, such as telatest.Screen(1280, 720)", screen.W, screen.H)
	}
	return nil
}

// windowFromApp returns the option that gives the main window the OS-window
// fields of app: [MountApp] adds it to [FromApp].
func windowFromApp(app tela.App) Option {
	icon := append([]image.Image(nil), app.Icon...)
	return func(c *config) {
		c.win = windowConfig{
			title: app.Title, appID: app.AppID,
			minW: app.MinWidth, minH: app.MinHeight, maxW: app.MaxWidth, maxH: app.MaxHeight,
			fixed: app.FixedSize, frameless: app.Frameless, onTop: app.AlwaysOnTop, hidden: app.Hidden,
			icon: icon, titleBar: app.TitleBar, backdrop: app.Backdrop, placement: app.Placement, onClose: app.OnClose,
			onQuit: app.OnQuit, overlay: app.TitleBarOverlay, transparent: app.Transparent,
		}
	}
}

// Title sets the title of the main window (tela.App.Title), which
// [App.Title] and [App.Window] read. The default is "telatest".
func Title(title string) Option { return func(c *config) { c.win.title = title } }

// AppID sets the application identity the window reports to the desktop
// (tela.App.AppID).
func AppID(id string) Option { return func(c *config) { c.win.appID = id } }

// MinSize and MaxSize limit the size the user can give the main window
// (tela.App.MinWidth and MinHeight, MaxWidth and MaxHeight); zero is no
// limit on that side. The fake window manager clamps [App.Resize],
// maximizing and fullscreen to them.
func MinSize(width, height float32) Option {
	return func(c *config) { c.win.minW, c.win.minH = width, height }
}

// MaxSize limits the size of the main window from above; see [MinSize].
func MaxSize(width, height float32) Option {
	return func(c *config) { c.win.maxW, c.win.maxH = width, height }
}

// FixedSize keeps the user from resizing the main window
// (tela.App.FixedSize).
func FixedSize() Option { return func(c *config) { c.win.fixed = true } }

// Frameless gives the main window no OS title bar (tela.App.Frameless):
// its [tela.WindowState] reports ClientSideDecorations, and the content
// draws its own title bar with h.WindowDrag and h.WindowControl.
func Frameless() Option { return func(c *config) { c.win.frameless = true } }

// AlwaysOnTop keeps the main window above the others (tela.App.AlwaysOnTop).
func AlwaysOnTop() Option { return func(c *config) { c.win.onTop = true } }

// StartHidden opens the main window hidden (tela.App.Hidden): an app that
// starts in the tray. The fake window manager gives a hidden window no
// frames, so it draws nothing (no screenshot, no goldens, no actions that
// need a layout) until the app shows it; its components still build and
// its effects run (its tray icon shows, ADR-185).
func StartHidden() Option { return func(c *config) { c.win.hidden = true } }

// Icon sets the icon of the main window (tela.App.Icon): the same picture
// at several sizes.
func Icon(images ...image.Image) Option {
	images = append([]image.Image(nil), images...)
	return func(c *config) { c.win.icon = images }
}

// TitleBar sets the title bar colors of the main window (tela.App.TitleBar).
// The fake window manager only records them: [App.TitleBar] reads them back.
func TitleBar(colors tela.TitleBarColors) Option {
	return func(c *config) { c.win.titleBar = colors }
}

// Backdrop sets the system backdrop of the main window (tela.App.Backdrop).
// The fake window manager only records it: [App.Backdrop] reads it back.
func Backdrop(b tela.Backdrop) Option { return func(c *config) { c.win.backdrop = b } }

// Placement opens the main window where a previous run left it
// (tela.App.Placement): a [tela.WindowPlacement] saved from
// tela.WindowHandle.Placement or [App.Placement]. Its size, when set,
// wins over [Size].
func Placement(p tela.WindowPlacement) Option { return func(c *config) { c.win.placement = p } }

// OnClose sets what closing the main window does (tela.App.OnClose): f runs
// when the user asks to close it ([App.RequestClose], a WindowClose control,
// WindowHandle.Close). Without it the app quits.
func OnClose(f func(tela.WindowHandle)) Option { return func(c *config) { c.win.onClose = f } }

// OnQuit sets what a request to quit does (tela.App.OnQuit): f runs when
// the user or the OS asks the app to quit ([App.RequestQuit]; macOS's
// Cmd+Q and the Dock's Quit). Without it every window gets a close request.
func OnQuit(f func()) Option { return func(c *config) { c.win.onQuit = f } }

// TitleBarOverlay puts the main window's content under a transparent title
// bar with the OS's window buttons over it (tela.App.TitleBarOverlay). The
// fake window manager keeps no buttons there unless the test says where:
// without [TitleBarControls] or [PlatformKeys]("darwin") the window behaves
// as a Frameless one on a platform that cannot do this (Linux, Windows):
// ClientSideDecorations, and an empty [tela.WindowState] TitleBarControls.
// With PlatformKeys("darwin") the buttons are [MacTitleBarControls].
func TitleBarOverlay() Option { return func(c *config) { c.win.overlay = true } }

// MacTitleBarControls is the rectangle of macOS's window buttons (the
// traffic lights) that [TitleBarOverlay] windows get under
// [PlatformKeys]("darwin"): the place and size of the standard buttons
// centered in a 40 px title bar, in logical window coordinates, as
// macOS 26 places them on the macos-latest runner (AppKit's own place
// there is 9,9 60x14 in its 32 pt title bar).
var MacTitleBarControls = tela.Rect{X: 9, Y: 13, Width: 60, Height: 14}

// TitleBarControls gives every TitleBarOverlay window of the app the
// rectangle that the OS's window buttons cover
// ([tela.WindowState].TitleBarControls), in logical window coordinates, as
// macOS's traffic lights do. [App.SetTitleBarControls] changes one
// window's later.
func TitleBarControls(r tela.Rect) Option { return func(c *config) { c.controls = r } }

// WithUnsupported makes the headless desktop lack the given services from
// the start, as [App.Unsupported] does later: for components that look at
// the platform when they mount.
//
//	app := telatest.Mount(t, Root, telatest.WithUnsupported(telatest.AppMenu))
func WithUnsupported(services ...DesktopService) Option {
	return func(c *config) { c.unsupported = append(c.unsupported, services...) }
}

// NoServerDecorations makes the fake window manager draw no title bars, as
// GNOME on Wayland does: every window, framed or not, reports
// ClientSideDecorations, so a test can check that the app draws its own
// bar (kit.TitleBar) there. A framed window without an h.WindowDrag area
// of its own then shows Tela's fallback title bar (see tela.App), with
// its window buttons, and its content below it.
func NoServerDecorations() Option { return func(c *config) { c.noServerDeco = true } }

// Screen sets the logical size of the fake screen: what maximized and
// fullscreen windows grow to (default 1920 by 1080).
func Screen(width, height float32) Option {
	return func(c *config) { c.screen = platform.Size{W: width, H: height} }
}

// Monitors gives the fake desktop monitors whose work areas (the monitor
// without its task bar) are the given rectangles, the primary first, in
// physical pixels of the desktop: the space of [App.Move] and of
// [tela.WindowPlacement] X and Y. A window that opens at a position where
// it could not be grabbed on any of them (a placement saved while another
// monitor was connected) opens on the nearest one instead, as on Windows,
// macOS and X11, and [App.Placement] reports where it went:
//
//	saved := tela.WindowPlacement{Width: 800, Height: 600, X: 2500, Y: 100, HasPosition: true}
//	app := telatest.MountApp(t, tela.App{Placement: saved, Root: Notes{}},
//	    telatest.Monitors(image.Rect(0, 0, 1920, 1040))) // the second monitor is gone
//	// app.Placement() is at 1120,100: on the laptop's screen
//
// Without it no monitors are known and windows open where they are asked
// to. Moves after a window opens ([App.Move], tela.WindowHandle
// SetPosition) are never changed.
func Monitors(workAreas ...image.Rectangle) Option {
	areas := append([]image.Rectangle(nil), workAreas...)
	return func(c *config) { c.monitors = areas }
}
