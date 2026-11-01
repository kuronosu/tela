package desktop

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// Native menus (ADR-209, ADR-213): the OS's context menus and window menu
// bars, where the OS has them. Tela draws its own menus (kit's
// ContextMenu and Menubar) everywhere; these are what those use when an
// app asks for native menus, and they report ErrUnsupported where there
// are none, so the caller draws its own instead.

// PopupMenu shows items as the OS's context menu of w's window, with its
// top-left corner at at (logical pixels in the window's coordinates, such
// as a pointer event's position). It returns at once; when the user
// chooses an item, its OnSelect runs on the UI thread, then done (if not
// nil) with chosen true; when the user dismisses the menu, done runs with
// chosen false.
//
//	err := desktop.PopupMenu(w, tela.Point{X: e.X, Y: e.Y}, []desktop.MenuItem{
//	    {Label: "Copy", Accelerator: "Ctrl+C", OnSelect: copySelection},
//	    {Label: "Paste", Accelerator: "Ctrl+V", OnSelect: paste},
//	}, nil)
//	if errors.Is(err, desktop.ErrUnsupported) {
//	    setMenuOpen.Set(true) // draw the menu instead
//	}
//
// The zero WindowHandle means the window the call comes from. The
// returned error says the menu could not be shown, and done is then never
// called: no running app, or no native context menus (errors.Is(err,
// [ErrUnsupported]): Linux, where Tela draws every menu). Call it on the
// UI thread. Tests answer the menu with telatest's App.ChoosePopupItem and
// App.DismissPopupMenu.
func PopupMenu(w tela.WindowHandle, at tela.Point, items []MenuItem, done func(chosen bool, err error)) error {
	if len(items) == 0 {
		panic("desktop.PopupMenu with no items: give at least one, such as []desktop.MenuItem{{Label: \"Copy\", OnSelect: copySelection}}")
	}
	if at.X != at.X || at.Y != at.Y {
		panic(fmt.Sprintf("desktop.PopupMenu at %v: the position must be a number (logical pixels in the window)", at))
	}
	menu, actions := platformMenu("desktop.PopupMenu", items)
	d, err := running("PopupMenu")
	if err != nil {
		return err
	}
	pw := d.PlatformWindow(window(d, w))
	if pw == nil {
		return errors.New("desktop.PopupMenu: the window is not open")
	}
	err = pw.PopupMenu(platform.Point{X: at.X, Y: at.Y}, menu, func(item platform.MenuItemID, err error) {
		if err != nil {
			if done != nil {
				done(false, wrap("PopupMenu", err))
			}
			return
		}
		if item != 0 {
			if f := actions[item]; f != nil {
				f()
			}
		}
		if done != nil {
			done(item != 0, nil)
		}
	})
	return wrap("PopupMenu", err)
}

// MenuBar is a component that gives the window it is in the OS's menu bar
// while it is mounted (a Windows window with a title bar), follows its
// fields, and removes the bar when it unmounts. Menus are the top-level
// entries, each with its Submenu; choosing an item runs its OnSelect on
// the UI thread. It renders nothing where it is declared.
//
//	desktop.MenuBar{
//	    Menus: []desktop.MenuItem{
//	        {Label: "File", Submenu: []desktop.MenuItem{
//	            {Label: "New", Accelerator: "Ctrl+N", OnSelect: newNote},
//	            {Separator: true},
//	            {Label: "Quit", OnSelect: tela.Quit},
//	        }},
//	    },
//	    OnError: func(err error) { setDrawnMenu.Set(true) },
//	}
//
// Most desktops have no native window menu bars (Linux; frameless
// windows, which draw their own title bar; macOS, whose menu bar belongs
// to the app), and where there is no window of a running app (tela
// snapshot): there OnError receives an error wrapping [ErrUnsupported]
// after the first build, and the app draws its menu instead (kit's
// Menubar does). Accelerator texts are only shown: handle the keys
// yourself. Tests choose items with telatest's App.ChooseMenuItem.
type MenuBar struct {
	// Menus are the bar's top-level entries ("File", "Edit"), each with
	// its Submenu.
	Menus []MenuItem
	// OnError receives the error when the bar cannot be shown or
	// updated (ErrUnsupported where windows have no native menu bar).
	// Nil reports it as a dev warning.
	OnError func(err error)
}

// Build sets the menu bar while the component is mounted (see [MenuBar]).
func (m MenuBar) Build() tela.Node {
	w := tela.UseWindow()
	s := tela.UseOnce(func() *menuBarState { return &menuBarState{} })
	s.props = m
	menu, actions := platformMenu("desktop.MenuBar", m.Menus)
	tela.UseMount(func() func() {
		d, ok := core.DesktopOf(w)
		if !ok {
			// tela snapshot, or a tree without windows: there is no menu
			// bar to set, so whoever draws one instead is told.
			if f := s.props.OnError; f != nil {
				f(fmt.Errorf("desktop: MenuBar: %w (no window of a running app)", ErrUnsupported))
			}
			return nil
		}
		s.d, s.w = d, w
		s.stop = d.ListenWindow(w, s.event)
		bars := d.Local(menuBarsKey{}, func() any { return map[tela.WindowHandle]*menuBarState{} }).(map[tela.WindowHandle]*menuBarState)
		if o := bars[w]; o != nil && o != s {
			d.Warn("desktop-menubar-twice", func() string {
				return "desktop: two desktop.MenuBar components are mounted in one window; the later one replaces the earlier one's menu (mount one MenuBar per window)"
			})
		}
		bars[w], s.bars = s, bars
		return s.remove
	})
	tela.UseEffectDo(func() {
		s.actions = actions
		s.sync(menu)
	})
	return nil
}

type menuBarsKey struct{}

// menuBarState is a mounted MenuBar's link to its window.
type menuBarState struct {
	d       core.Desktop
	w       tela.WindowHandle
	stop    func()
	props   MenuBar
	actions map[platform.MenuItemID]func()
	shown   []platform.MenuItem
	// bars are the app's mounted MenuBars by window: only a window's
	// latest one sets its menu and runs its items.
	bars map[tela.WindowHandle]*menuBarState
	// set: the window shows shown; tried: SetMenu was called with shown
	// (failed or not); unsupported: the window has no native menu bar, so
	// nothing is tried again.
	set, tried, unsupported bool
}

// sync gives the window menu, unless it shows it already.
func (s *menuBarState) sync(menu []platform.MenuItem) {
	if s.d.IsZero() || s.d.Ended() || s.unsupported || s.bars[s.w] != s {
		return
	}
	if s.tried && sameMenu(menu, s.shown) {
		return
	}
	pw := s.d.PlatformWindow(s.w)
	if pw == nil {
		return
	}
	s.shown, s.tried = menu, true
	err := pw.SetMenu(menu)
	s.set = err == nil
	if err != nil {
		s.unsupported = errors.Is(err, errors.ErrUnsupported)
		s.report(wrap("MenuBar", err))
	}
}

func (s *menuBarState) report(err error) {
	if f := s.props.OnError; f != nil {
		f(err)
		return
	}
	s.d.Warn("desktop-menubar:"+err.Error(), func() string {
		return fmt.Sprintf("%v; set desktop.MenuBar's OnError to draw the menu instead (kit.Menubar)", err)
	})
}

func (s *menuBarState) remove() {
	if s.stop != nil {
		s.stop()
	}
	if s.bars[s.w] == s {
		delete(s.bars, s.w)
		if s.set && !s.d.Ended() {
			if pw := s.d.PlatformWindow(s.w); pw != nil {
				_ = pw.SetMenu(nil)
			}
		}
	}
	s.set = false
}

// event runs the OnSelect of a chosen item.
func (s *menuBarState) event(e platform.Event) {
	me, ok := e.(platform.MenuEvent)
	if !ok || !s.set || s.bars[s.w] != s {
		return
	}
	if f := s.actions[me.Item]; f != nil {
		f()
	}
}
