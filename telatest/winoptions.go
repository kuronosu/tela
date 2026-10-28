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
}

// coreOptions returns the core options of the window (without the root,
// the size and the tree configuration).
func (w windowConfig) coreOptions() core.Options {
	return core.Options{
		Title: w.title, AppID: w.appID,
		MinWidth: w.minW, MinHeight: w.minH, MaxWidth: w.maxW, MaxHeight: w.maxH,
		FixedSize: w.fixed, Frameless: w.frameless, AlwaysOnTop: w.onTop, Hidden: w.hidden,
		Icon: w.icon, TitleBar: w.titleBar, Backdrop: w.backdrop, Placement: w.placement, OnClose: w.onClose,
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

// Screen sets the logical size of the fake screen: what maximized and
// fullscreen windows grow to (default 1920 by 1080).
func Screen(width, height float32) Option {
	return func(c *config) { c.screen = platform.Size{W: width, H: height} }
}
