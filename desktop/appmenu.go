package desktop

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// AppMenuBar is a component that gives the app its menu bar while it is
// mounted, declared once anywhere in the tree (ADR-224; Electron's
// Menu.setApplicationMenu), follows its fields, and restores the default
// when it unmounts. It renders nothing where it is declared.
//
//	desktop.AppMenuBar{
//	    AppItems: []desktop.MenuItem{
//	        {Label: "Settings…", Accelerator: "CmdOrCtrl+,", OnSelect: openSettings},
//	    },
//	    Menus: []desktop.MenuItem{
//	        {Label: "File", Submenu: []desktop.MenuItem{
//	            {Label: "New Note", Accelerator: "CmdOrCtrl+N", OnSelect: newNote},
//	            {Separator: true},
//	            {Role: desktop.RoleClose},
//	        }},
//	        {Role: desktop.RoleEditMenu},
//	        {Role: desktop.RoleWindowMenu},
//	    },
//	    OnError: func(err error) { setDrawnMenu.Set(true) },
//	}
//
// Where the platform has an app menu bar (macOS), it is the menu bar of
// every window, and macOS always puts the app's own menu first (About, then
// AppItems such as Settings…, then Services, Hide, Hide Others, Show All
// and Quit). On Windows, where each framed window has a menu bar, it
// becomes the bar of every framed window that has no [MenuBar] of its own,
// including windows that open later; a window's own MenuBar wins on that
// window. AppItems are ignored there. Elsewhere (Linux; frameless windows;
// a tree with no window of a running app, as in tela snapshot) OnError
// receives an error wrapping [ErrUnsupported] after the first build, and
// the app draws its menu instead (kit's Menubar with AppMenu does). Nil
// OnError reports it as a dev warning. Two mounted AppMenuBars: the later
// one wins, with a dev warning.
//
// Choosing an item runs its OnSelect on the UI thread. Role items
// ([Role]) without an OnSelect are carried out by the OS where it can
// (macOS) and by package desktop elsewhere, on the window with the
// keyboard focus.
//
// On macOS an Accelerator is the item's key equivalent: the key chooses
// the item and does not reach the app as a key event, so do not handle the
// key a second time (no h.OnKeyDown for it). "CmdOrCtrl+N" is the portable
// spelling: Command+N on a Mac, Ctrl+N elsewhere, where the accelerator is
// display text and the app handles the key itself. Tests read the bar with
// telatest's App.AppMenuBar and choose items with App.ChooseAppMenuItem.
type AppMenuBar struct {
	// Menus are the bar's top-level entries ("File", "Edit"), each with
	// its Submenu. A menu role (RoleEditMenu, RoleWindowMenu) with no
	// Submenu gets the standard items.
	Menus []MenuItem
	// AppItems are the items of macOS's app menu after About and before
	// Services ("Settings…"), separated from them by macOS. Ignored where
	// there is no app menu. Menu roles are not allowed among them.
	AppItems []MenuItem
	// OnError receives the error when the bar cannot be shown or updated
	// (ErrUnsupported where there is no menu bar to give). Nil reports it
	// as a dev warning.
	OnError func(err error)
}

// Build sets the app's menu bar while the component is mounted.
func (m AppMenuBar) Build() tela.Node {
	w := tela.UseWindow()
	s := tela.UseOnce(func() *appBarState { return &appBarState{} })
	s.props = m
	lists, actions := platformMenus("desktop.AppMenuBar", m.Menus, m.AppItems)
	tela.UseMount(func() func() {
		d, ok := core.DesktopOf(w)
		if !ok {
			// tela snapshot, or a tree without windows: there is no menu
			// bar to set, so whoever draws one instead is told.
			if f := s.props.OnError; f != nil {
				f(fmt.Errorf("desktop: AppMenuBar: %w (no window of a running app)", ErrUnsupported))
			}
			return nil
		}
		s.d, s.w = d, w
		hub := hubOf(d)
		if o := hub.app; o != nil && o != s {
			d.Warn("desktop-appmenubar-twice", func() string {
				return "desktop: two desktop.AppMenuBar components are mounted; the later one replaces the earlier one's menu (declare the app's menu bar once)"
			})
		}
		hub.app, s.hub = s, hub
		return s.remove
	})
	tela.UseEffectDo(func() {
		s.actions = actions
		s.sync(lists[0], lists[1])
	})
	return nil
}

// menuHub is the app's menu bars: each window's own MenuBar and the app's
// AppMenuBar, which coordinate on who sets a window's menu (Local state of
// the app).
type menuHub struct {
	bars map[tela.WindowHandle]*menuBarState
	app  *appBarState
}

type menuHubKey struct{}

func hubOf(d core.Desktop) *menuHub {
	return d.Local(menuHubKey{}, func() any { return &menuHub{bars: map[tela.WindowHandle]*menuBarState{}} }).(*menuHub)
}

// appBarMode is how a mounted AppMenuBar reaches the user.
type appBarMode uint8

const (
	// barUnknown: nothing was tried yet.
	barUnknown appBarMode = iota
	// barApp: the platform has an app menu bar (Desktop.SetAppMenu).
	barApp
	// barWindows: every framed window has the menu (Window.SetMenu).
	barWindows
	// barNone: the platform has neither; the error was reported.
	barNone
)

// appBarState is a mounted AppMenuBar's link to the backend.
type appBarState struct {
	d       core.Desktop
	w       tela.WindowHandle
	hub     *menuHub
	props   AppMenuBar
	actions menuActions
	mode    appBarMode
	tried   bool
	// shown and shownItems are the menus last given to the platform.
	shown, shownItems []platform.MenuItem

	stopApp, stopWatch func()
	// applied: the windows that carry the app's bar (barWindows), each
	// with the stop of its event listener; seen: the windows that have a
	// listener.
	applied map[tela.WindowHandle]bool
	stops   map[tela.WindowHandle]func()
}

// sync gives the platform the menus, unless it has them already.
func (s *appBarState) sync(menu, items []platform.MenuItem) {
	if s.d.IsZero() || s.d.Ended() || s.hub.app != s || s.mode == barNone {
		return
	}
	if s.tried && sameMenu(menu, s.shown) && sameMenu(items, s.shownItems) {
		return
	}
	s.shown, s.shownItems, s.tried = menu, items, true
	switch s.mode {
	case barUnknown:
		err := s.d.Services().SetAppMenu(menu, items)
		switch {
		case err == nil:
			s.mode = barApp
			s.stopApp = s.d.Listen(s.event)
		case errors.Is(err, errors.ErrUnsupported):
			s.startWindows()
		default:
			s.report(wrap("AppMenuBar", err))
		}
	case barApp:
		if err := s.d.Services().SetAppMenu(menu, items); err != nil {
			s.report(wrap("AppMenuBar", err))
		}
	case barWindows:
		s.applied = map[tela.WindowHandle]bool{}
		s.windowsChanged()
	}
}

// startWindows tries the menu as the bar of every window; none takes it,
// the platform has no menu bar to give.
func (s *appBarState) startWindows() {
	s.mode = barWindows
	s.applied = map[tela.WindowHandle]bool{}
	s.stops = map[tela.WindowHandle]func(){}
	if !s.windowsSync() {
		s.stopAll()
		s.mode = barNone
		s.report(fmt.Errorf("desktop: AppMenuBar: %w (the platform has no app menu bar and its windows no menu bars)", ErrUnsupported))
		return
	}
	s.stopWatch = s.d.WatchWindows(s.windowsChanged)
}

// windowsChanged gives the menu to the windows that have no bar of their
// own and lost the app's, after a window opened or closed or lost its own
// bar.
func (s *appBarState) windowsChanged() {
	if s.d.IsZero() || s.d.Ended() || s.hub.app != s || s.mode != barWindows {
		return
	}
	s.windowsSync()
}

// lost tells the bar that window w's own MenuBar left it with an empty
// menu: the app's bar is the window's again.
func (s *appBarState) lost(w tela.WindowHandle) {
	delete(s.applied, w)
	s.windowsChanged()
}

// windowsSync makes the open windows carry the app's bar, except those with
// a MenuBar of their own, and reports whether the platform has window menu
// bars (a window took the menu, or has a MenuBar that did not fail).
func (s *appBarState) windowsSync() (supported bool) {
	open := map[tela.WindowHandle]bool{}
	for _, w := range s.d.Windows() {
		open[w] = true
		if b := s.hub.bars[w]; b != nil {
			// The window's own bar wins; it leaves the app's.
			delete(s.applied, w)
			if !(b.tried && b.unsupported) {
				supported = true
			}
			continue
		}
		if s.applied[w] {
			supported = true
			continue
		}
		pw := s.d.PlatformWindow(w)
		if pw == nil {
			continue
		}
		if err := pw.SetMenu(s.shown); err != nil {
			if !errors.Is(err, errors.ErrUnsupported) {
				s.report(wrap("AppMenuBar", err))
			}
			continue
		}
		supported = true
		s.applied[w] = true
		if s.stops[w] == nil {
			w := w
			s.stops[w] = s.d.ListenWindow(w, func(e platform.Event) { s.windowEvent(w, e) })
		}
	}
	for w := range s.applied {
		if !open[w] {
			delete(s.applied, w)
		}
	}
	for w, stop := range s.stops {
		if !open[w] {
			stop()
			delete(s.stops, w)
		}
	}
	return supported
}

func (s *appBarState) stopAll() {
	for w, stop := range s.stops {
		stop()
		delete(s.stops, w)
	}
	if s.stopWatch != nil {
		s.stopWatch()
		s.stopWatch = nil
	}
	if s.stopApp != nil {
		s.stopApp()
		s.stopApp = nil
	}
}

func (s *appBarState) report(err error) {
	if f := s.props.OnError; f != nil {
		f(err)
		return
	}
	s.d.Warn("desktop-appmenubar:"+err.Error(), func() string {
		return fmt.Sprintf("%v; set desktop.AppMenuBar's OnError to draw the menu instead (kit.Menubar with AppMenu)", err)
	})
}

func (s *appBarState) remove() {
	s.stopAll()
	if s.hub != nil && s.hub.app == s {
		s.hub.app = nil
		if !s.d.Ended() {
			switch s.mode {
			case barApp:
				_ = s.d.Services().SetAppMenu(nil, nil)
			case barWindows:
				for w := range s.applied {
					if s.hub.bars[w] == nil {
						if pw := s.d.PlatformWindow(w); pw != nil {
							_ = pw.SetMenu(nil)
						}
					}
				}
			}
		}
	}
	s.applied = nil
	s.mode = barNone
}

// event runs the OnSelect of an item chosen in the app's menu bar.
func (s *appBarState) event(e platform.Event) {
	ae, ok := e.(platform.AppMenuEvent)
	if !ok || s.mode != barApp || s.hub.app != s {
		return
	}
	s.actions.run(ae.Item, roleInKey(s.w))
}

// windowEvent runs the OnSelect of an item chosen in the menu bar window w
// carries for the app.
func (s *appBarState) windowEvent(w tela.WindowHandle, e platform.Event) {
	me, ok := e.(platform.MenuEvent)
	if !ok || s.mode != barWindows || s.hub.app != s || !s.applied[w] || s.hub.bars[w] != nil {
		return
	}
	s.actions.run(me.Item, roleIn(w))
}
