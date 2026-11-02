package desktop

import (
	"fmt"
	"image"
	"reflect"
	"slices"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// Tray is a component that shows an icon in the system tray while it is
// mounted (the Windows notification area, the StatusNotifierItem host of
// KDE, GNOME with the AppIndicator extension, and most Linux panels),
// updates it when its fields change, and removes it when it unmounts.
// It renders nothing where it is declared; put it anywhere in the app:
//
//	func (n Notes) Build() tela.Node {
//	    win := tela.UseWindow()
//	    muted, setMuted := tela.UseState(false)
//	    return Div(
//	        desktop.Tray{
//	            Icon:    appIcon, // []image.Image at 16, 24 and 32 px
//	            Tooltip: "Notes",
//	            OnClick: func() { win.Show() },
//	            Menu: []desktop.MenuItem{
//	                {Label: "Show Notes", OnSelect: func() { win.Show() }},
//	                {Label: "Mute reminders", Checkbox: true, Checked: muted,
//	                    OnSelect: func() { setMuted.Set(!muted) }},
//	                {Separator: true},
//	                {Label: "Quit", OnSelect: tela.Quit},
//	            },
//	        },
//	        // ...the window's content
//	    )
//	}
//
// With App.OnClose hiding the main window (OnClose: func(w
// tela.WindowHandle) { w.Hide() }), the app keeps running in the tray.
// Not every desktop has a tray: when the icon cannot be shown, OnError
// receives an error wrapping [ErrUnsupported] (GNOME without the
// AppIndicator extension); then do not hide the window on close, or the
// app cannot be reached. tela snapshot and windowless trees show no tray.
// Tests read the tray with telatest's App.Tray and click it with
// App.ClickTray and App.ClickTrayItem.
type Tray struct {
	// Icon is the tray image, the same picture at one or more sizes (16,
	// 24 and 32 pixels cover the usual trays; the OS picks). Required.
	Icon []image.Image
	// Tooltip is shown when the pointer rests on the icon.
	Tooltip string
	// Menu is the icon's context menu (a right click; on some Linux
	// panels any click). Empty: no menu.
	Menu []MenuItem
	// OnClick runs when the user clicks the icon (Windows, KDE; some
	// Linux panels open the menu instead and never report a click).
	OnClick func()
	// OnError receives the error when the icon cannot be shown or
	// updated. Nil reports it as a dev warning.
	OnError func(err error)
}

// MenuItem is an item of a native menu: a tray icon's ([Tray]), a
// window's menu bar ([MenuBar]) or a context menu ([PopupMenu]). The
// backend never toggles a checkbox: OnSelect updates the state that
// renders Checked.
type MenuItem struct {
	// Label is the item's text (plain text: "&" and "_" are shown as
	// they are).
	Label string
	// Accelerator is the text of the item's keyboard shortcut, shown at
	// its right ("Ctrl+S"; [ParseKeys] writes it the usual way). It is
	// display only: the app handles the key itself (h.OnKeyDown). Tray
	// menus may not show it. "CmdOrCtrl+N" is the portable spelling
	// (Command on a Mac, Ctrl elsewhere). One exception: in the app's menu
	// bar on macOS ([AppMenuBar]) it is the item's key equivalent, so the
	// key chooses the item and does not reach the app as a key event: do
	// not handle it a second time.
	Accelerator string
	// Role makes the item, or its submenu, a standard one: Undo, Copy,
	// Minimize, the Edit menu (see [Role]). An empty Label gets the
	// role's. RoleNone, the zero value, is an item of the app's own.
	Role Role
	// Separator makes the item a separator line; the other fields are
	// ignored.
	Separator bool
	// Checkbox makes the item a check item, showing a check mark when
	// Checked.
	Checkbox, Checked bool
	// Disabled shows the item grayed out; it cannot be chosen.
	Disabled bool
	// Submenu, if not empty, makes the item open a submenu (OnSelect is
	// then never called).
	Submenu []MenuItem
	// OnSelect runs when the user chooses the item.
	OnSelect func()
}

// Build shows the icon while the component is mounted (see [Tray]).
func (t Tray) Build() tela.Node {
	if len(t.Icon) == 0 {
		panic("desktop.Tray has no Icon: give it the picture at a few sizes, such as desktop.Tray{Icon: []image.Image{icon16, icon24, icon32}, Tooltip: \"Notes\"}")
	}
	w := tela.UseWindow()
	s := tela.UseOnce(func() *trayState { return &trayState{} })
	s.props = t // the latest callbacks
	opts, actions := trayOptions(t)
	tela.UseMount(func() func() {
		d, ok := core.DesktopOf(w)
		if !ok {
			return nil // tela snapshot, or a tree without windows
		}
		s.d, s.win = d, w
		s.stop = d.Listen(s.event)
		return s.remove
	})
	// After every build: sync compares the options with what the backend
	// shows.
	tela.UseEffectDo(func() {
		s.actions = actions
		s.sync(opts)
	})
	return nil
}

// trayState is a mounted Tray's link to the backend.
type trayState struct {
	d       core.Desktop
	stop    func()
	id      platform.TrayID
	shown   platform.TrayOptions // what the backend shows
	failed  bool                 // NewTray failed: not retried until the options change
	props   Tray
	actions menuActions
	win     tela.WindowHandle
}

// sync shows opts, or updates the icon when they differ from what it
// shows.
func (s *trayState) sync(opts platform.TrayOptions) {
	if s.d.IsZero() || s.d.Ended() {
		return
	}
	if (s.id != 0 || s.failed) && sameTray(opts, s.shown) {
		return
	}
	s.shown = opts
	if s.id == 0 {
		id, err := s.d.Services().NewTray(opts)
		if err != nil {
			s.failed = true
			s.report(wrap("Tray", err))
			return
		}
		s.id, s.failed = id, false
		return
	}
	if err := s.d.Services().UpdateTray(s.id, opts); err != nil {
		s.report(wrap("Tray", err))
	}
}

func (s *trayState) report(err error) {
	if f := s.props.OnError; f != nil {
		f(err)
		return
	}
	s.d.Warn("desktop-tray:"+err.Error(), func() string {
		return fmt.Sprintf("%v; set desktop.Tray's OnError to handle it (keep the main window reachable when the tray is unavailable)", err)
	})
}

func (s *trayState) remove() {
	if s.stop != nil {
		s.stop()
	}
	if s.id != 0 && !s.d.Ended() {
		_ = s.d.Services().RemoveTray(s.id)
	}
	s.id = 0
}

// event runs the callback of a click on the icon or of a chosen item.
func (s *trayState) event(e platform.Event) {
	te, ok := e.(platform.TrayEvent)
	if !ok || s.id == 0 || te.Tray != s.id {
		return
	}
	switch te.Kind {
	case platform.TrayActivated:
		if f := s.props.OnClick; f != nil {
			f()
		}
	case platform.TrayMenuItem:
		s.actions.run(te.Item, roleInKey(s.win))
	}
}

// trayOptions returns the platform options of t, numbering the items that
// can be chosen from 1 in menu order, and their OnSelect by number.
func trayOptions(t Tray) (platform.TrayOptions, menuActions) {
	menu, actions := platformMenu("desktop.Tray", t.Menu)
	return platform.TrayOptions{Icon: t.Icon, Tooltip: t.Tooltip, Menu: menu}, actions
}

// sameTray reports whether a and b show the same icon, tooltip and menu.
// Images are compared by identity (the same image values): pass the same
// images on every build, not new ones.
func sameTray(a, b platform.TrayOptions) bool {
	return a.Tooltip == b.Tooltip &&
		slices.EqualFunc(a.Icon, b.Icon, sameImage) &&
		sameMenu(a.Menu, b.Menu)
}

func sameMenu(a, b []platform.MenuItem) bool {
	return slices.EqualFunc(a, b, func(x, y platform.MenuItem) bool {
		return x.ID == y.ID && x.Label == y.Label && x.Accelerator == y.Accelerator && x.Separator == y.Separator &&
			x.Checkable == y.Checkable && x.Checked == y.Checked && x.Disabled == y.Disabled &&
			sameMenu(x.Submenu, y.Submenu)
	})
}

// sameImage compares two images by identity, without panicking on images
// of non-comparable types (which count as different).
func sameImage(a, b image.Image) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta := reflect.TypeOf(a)
	if ta != reflect.TypeOf(b) || !ta.Comparable() {
		return false
	}
	return a == b
}
