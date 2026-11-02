package remote

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/kuronosu/tela/internal/platform"
)

// Platform contract v4 (ADR-225), protocol 6.
//
// Quit requests. The host forwards its backend's QuitRequestedEvent to
// the active child (held while input is frozen, like the other app-scoped
// events), whose layers above run tela.App.OnQuit or send its windows
// close requests; with no active child (the app crashed) it goes to
// HostOptions.OnUnhandled, so the supervisor decides.
//
// The app menu bar. Desktop.SetAppMenu is a round trip, like a window's
// SetMenu: the active child's bar is set on the host's backend and its
// result returned; a pending child's is kept and set at its promotion,
// answered with whether the backend has an app menu bar (learned from the
// last SetAppMenu, or by setting the current bar again, which changes
// nothing). A promotion gives the backend the new active child's bar (the
// default bar if it set none). AppMenuEvents go to the active child.
//
// Title bar overlays. WindowOptions.TitleBarOverlay is a creation-only
// option, like Frameless: a reload keeps the real window only when it is
// unchanged. WindowState.TitleBarControls travels with the window state.

// appMenuBar is an app menu bar: SetAppMenu's menu and app items.
type appMenuBar struct {
	menu, items []platform.MenuItem
}

// hostAppMenu is the app menu bar of the host's backend.
type hostAppMenu struct {
	bar appMenuBar // the bar last set on the backend
	// known: the backend answered a SetAppMenu, with err (nil, or
	// ErrUnsupported where it has no app menu bar).
	known bool
	err   error
}

// --- child side ---

// SetAppMenu asks the host to set its backend's app menu bar (see the top
// of appmenu.go); ErrUnsupported where the backend has none.
func (d childDesktop) SetAppMenu(menu, appItems []platform.MenuItem) error {
	platform.CheckAppMenu(menu, appItems)
	_, err := d.call(dSetAppMenu, 0, func(e *enc) { e.menu(menu); e.menu(appItems) })
	return err
}

// --- host side ---

// setAppMenu serves a child's SetAppMenu. A replaced child's is dropped.
func (h *Host) setAppMenu(c *Child, menu, items []platform.MenuItem, call func(func() error) error) error {
	if c.state == stateDetached {
		return nil
	}
	bar := appMenuBar{menu: menu, items: items}
	c.appMenu = &bar
	if c.state == stateActive {
		return h.applyAppMenu(bar, call)
	}
	if !h.appMenu.known {
		// Learn whether the backend has an app menu bar without changing it.
		return h.applyAppMenu(h.appMenu.bar, call)
	}
	return h.appMenu.err
}

// applyAppMenu sets the backend's app menu bar and remembers whether it
// has one.
func (h *Host) applyAppMenu(bar appMenuBar, call func(func() error) error) error {
	err := call(func() error { return h.b.Desktop().SetAppMenu(bar.menu, bar.items) })
	if err == nil || errors.Is(err, errors.ErrUnsupported) {
		h.appMenu.known, h.appMenu.err = true, err
	}
	if err == nil {
		h.appMenu.bar = bar
	}
	return err
}

// promoteAppMenu gives the backend the app menu bar of c, the new active
// child (the default bar if it set none).
func (h *Host) promoteAppMenu(c *Child) {
	var bar appMenuBar
	if c.appMenu != nil {
		bar = *c.appMenu
	}
	if reflect.DeepEqual(bar, h.appMenu.bar) || (h.appMenu.known && h.appMenu.err != nil) {
		return
	}
	if err := h.applyAppMenu(bar, callSafe); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		h.reportError(fmt.Errorf("remote: setting the app menu bar: %w", err))
	}
}
