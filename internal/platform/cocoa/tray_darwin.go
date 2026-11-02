//go:build darwin

package cocoa

import (
	"fmt"
	"image"
	"image/draw"
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Tray icons are NSStatusItems of the system status bar (the right side of
// the menu bar).
//
//   - The image is an NSImage trayIconPoints tall (18 points in the usual
//     22-point bar), with a bitmap representation at 1x and at 2x, each
//     made from the image of TrayOptions.Icon that fits it best
//     (platform.PickIcon), so it is sharp on every display. It keeps its
//     colors: the contract has no template (monochrome) images.
//   - With a menu, a click opens it, as macOS status items do (the item's
//     menu property); there is no TrayActivated then, as on Linux hosts
//     that open the menu on any click. Without one, a primary click is
//     TrayActivated (the button's action). The menu is an NSMenu of the
//     MenuItems: separators, disabled items (autoenablesItems off), check
//     marks (the state; the backend never toggles it), submenus; a chosen
//     item is a TrayEvent. Accelerators are not shown: key equivalents of
//     a status menu would act as shortcuts while the menu is open.
//   - The tooltip is the button's toolTip.
//   - Trays are removed when Run returns.

// trayIcon is a tray icon of the backend.
type trayIcon struct {
	b      *Backend
	id     platform.TrayID
	item   darwin.ID // NSStatusItem (owned: +1)
	target darwin.ID // TelaTrayTarget (owned): the button's and the menu items' target
	menu   []platform.MenuItem
}

// trsel holds the selectors of the tray code.
var trsel = struct {
	systemStatusBar, statusItemWithLength, removeStatusItem, thickness, button darwin.SEL
	setImage, setToolTip, setMenu, initWithSize, addRepresentation, setSize    darwin.SEL
	initWithCGImage, initWithTitle, initWithTitleAction, separatorItem         darwin.SEL
	setAutoenablesItems, setTag, tag, setEnabled, setState, setSubmenu         darwin.SEL
	addItem, setTarget, setAction, setVisible                                  darwin.SEL
}{
	systemStatusBar:      darwin.Sel("systemStatusBar"),
	statusItemWithLength: darwin.Sel("statusItemWithLength:"),
	removeStatusItem:     darwin.Sel("removeStatusItem:"),
	thickness:            darwin.Sel("thickness"),
	button:               darwin.Sel("button"),
	setImage:             darwin.Sel("setImage:"),
	setToolTip:           darwin.Sel("setToolTip:"),
	setMenu:              darwin.Sel("setMenu:"),
	initWithSize:         darwin.Sel("initWithSize:"),
	addRepresentation:    darwin.Sel("addRepresentation:"),
	setSize:              darwin.Sel("setSize:"),
	initWithCGImage:      darwin.Sel("initWithCGImage:"),
	initWithTitle:        darwin.Sel("initWithTitle:"),
	initWithTitleAction:  darwin.Sel("initWithTitle:action:keyEquivalent:"),
	separatorItem:        darwin.Sel("separatorItem"),
	setAutoenablesItems:  darwin.Sel("setAutoenablesItems:"),
	setTag:               darwin.Sel("setTag:"),
	tag:                  darwin.Sel("tag"),
	setEnabled:           darwin.Sel("setEnabled:"),
	setState:             darwin.Sel("setState:"),
	setSubmenu:           darwin.Sel("setSubmenu:"),
	addItem:              darwin.Sel("addItem:"),
	setTarget:            darwin.Sel("setTarget:"),
	setAction:            darwin.Sel("setAction:"),
	setVisible:           darwin.Sel("setVisible:"),
}

func statusBar() darwin.ID {
	return darwin.SendClass(darwin.GetClass("NSStatusBar"), trsel.systemStatusBar)
}

// NewTray adds a status item.
func (d desktop) NewTray(opts platform.TrayOptions) (platform.TrayID, error) {
	platform.CheckTrayOptions(opts)
	b := d.b
	if b.closed {
		return 0, errClosed
	}
	if err := setupDesktop(); err != nil {
		return 0, errNoHelpers("NewTray", err)
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	bar := statusBar()
	if bar == 0 {
		return 0, fmt.Errorf("cocoa: NewTray: no system status bar: %w", platform.ErrUnsupported)
	}
	item := darwin.Retain(darwin.Send(bar, trsel.statusItemWithLength, float64(darwin.NSVariableStatusItemLength)))
	if item == 0 || darwin.Send(item, trsel.button) == 0 {
		darwin.Release(item)
		return 0, fmt.Errorf("cocoa: NewTray: the status bar refused a new item: %w", platform.ErrUnsupported)
	}
	b.desk.lastTray++
	t := &trayIcon{b: b, id: b.desk.lastTray, item: item}
	t.target = darwin.Send(darwin.SendClass(deskClasses.trayTarget, sel.alloc), sel.init)
	deskObjects.Lock()
	deskObjects.trays[t.target] = t
	deskObjects.Unlock()
	if b.desk.trays == nil {
		b.desk.trays = map[platform.TrayID]*trayIcon{}
	}
	b.desk.trays[t.id] = t
	t.apply(opts)
	return t.id, nil
}

// UpdateTray replaces the image, tooltip and menu of a status item.
func (d desktop) UpdateTray(id platform.TrayID, opts platform.TrayOptions) error {
	platform.CheckTrayOptions(opts)
	t := d.b.desk.trays[id]
	if t == nil || id == 0 {
		return fmt.Errorf("cocoa: UpdateTray: tray %d does not exist (it was removed, or Run returned)", id)
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	t.apply(opts)
	return nil
}

// RemoveTray removes a status item; unknown IDs are ignored.
func (d desktop) RemoveTray(id platform.TrayID) error {
	if t := d.b.desk.trays[id]; t != nil {
		pool := darwin.PoolPush()
		t.remove()
		darwin.PoolPop(pool)
		delete(d.b.desk.trays, id)
	}
	return nil
}

// apply sets the item's image, tooltip and menu (or click action).
func (t *trayIcon) apply(opts platform.TrayOptions) {
	button := darwin.Send(t.item, trsel.button)
	side := trayIconPoints(darwin.SendFloat(statusBar(), trsel.thickness))
	if img := t.b.trayImage(opts.Icon, side); img != 0 {
		darwin.Send(button, trsel.setImage, img)
		darwin.Release(img)
	}
	if opts.Tooltip != "" {
		darwin.Send(button, trsel.setToolTip, darwin.AutoString(opts.Tooltip))
	} else {
		darwin.Send(button, trsel.setToolTip, darwin.ID(0))
	}
	t.menu = cloneMenuItems(opts.Menu)
	if len(opts.Menu) > 0 {
		menu := newNSMenu(opts.Menu, t.target, deskClasses.selTrayItem)
		darwin.Send(t.item, trsel.setMenu, menu)
		darwin.Release(menu) // the item keeps it
		darwin.Send(button, trsel.setAction, darwin.SEL(0))
	} else {
		darwin.Send(t.item, trsel.setMenu, darwin.ID(0))
		darwin.Send(button, trsel.setTarget, t.target)
		darwin.Send(button, trsel.setAction, deskClasses.selTray)
	}
}

// remove takes the item off the status bar and releases it.
func (t *trayIcon) remove() {
	deskObjects.Lock()
	delete(deskObjects.trays, t.target)
	deskObjects.Unlock()
	if button := darwin.Send(t.item, trsel.button); button != 0 {
		darwin.Send(button, trsel.setTarget, darwin.ID(0))
	}
	darwin.Send(t.item, trsel.setMenu, darwin.ID(0))
	darwin.Send(statusBar(), trsel.removeStatusItem, t.item)
	// The target may be inside one of its own actions (a handler that
	// removes the tray it was clicked in), and the item's menu with it.
	t.b.releaseLater(t.item, t.target)
	t.item, t.target = 0, 0
}

// trayImage returns an NSImage (+1) side points tall made from images, with
// a bitmap representation at 1x and 2x (one when the same image fits
// both), or 0 when it cannot be made.
func (b *Backend) trayImage(images []image.Image, side float64) darwin.ID {
	first := platform.PickIcon(images, int(math.Ceil(side)))
	if first == nil {
		return 0
	}
	fb := first.Bounds()
	w, h := trayImageSize(fb.Dx(), fb.Dy(), side)
	size := darwin.CGSize{Width: w, Height: h}
	img := darwin.Send(darwin.SendClass(darwin.GetClass("NSImage"), sel.alloc), trsel.initWithSize, size)
	if img == 0 {
		return 0
	}
	repClass := darwin.GetClass("NSBitmapImageRep")
	var used []image.Image
	for _, scale := range []float64{1, 2} {
		src := platform.PickIcon(images, int(math.Ceil(side*scale)))
		if slices.Contains(used, src) {
			continue
		}
		used = append(used, src)
		cg, err := b.newImage(premultiplied(src))
		if err != nil {
			continue
		}
		rep := darwin.Send(darwin.SendClass(repClass, sel.alloc), trsel.initWithCGImage, darwin.ID(cg))
		darwin.CGImageRelease(cg) // the representation keeps it
		if rep == 0 {
			continue
		}
		darwin.Send(rep, trsel.setSize, size)
		darwin.Send(img, trsel.addRepresentation, rep)
		darwin.Release(rep)
	}
	return img
}

// premultiplied returns img as an image.RGBA (premultiplied) with its
// top-left at (0, 0), what newImage takes.
func premultiplied(img image.Image) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Rect, img, b.Min, draw.Src)
	return out
}

// newNSMenu returns an NSMenu (+1) of items whose activatable entries
// send action to target with their ID as the tag. Labels are plain text
// (NSMenuItem has no mnemonics); Accelerator is not shown.
func newNSMenu(items []platform.MenuItem, target darwin.ID, action darwin.SEL) darwin.ID {
	menuClass, itemClass := darwin.GetClass("NSMenu"), darwin.GetClass("NSMenuItem")
	menu := darwin.Send(darwin.SendClass(menuClass, sel.alloc), trsel.initWithTitle, darwin.AutoString(""))
	darwin.Send(menu, trsel.setAutoenablesItems, false)
	for _, it := range items {
		if it.Separator {
			darwin.Send(menu, trsel.addItem, darwin.SendClass(itemClass, trsel.separatorItem))
			continue
		}
		var act darwin.SEL
		if len(it.Submenu) == 0 {
			act = action
		}
		mi := darwin.Send(darwin.SendClass(itemClass, sel.alloc), trsel.initWithTitleAction,
			darwin.AutoString(it.Label), act, darwin.AutoString(""))
		if len(it.Submenu) > 0 {
			sub := newNSMenu(it.Submenu, target, action)
			darwin.Send(mi, trsel.setSubmenu, sub)
			darwin.Release(sub)
		} else {
			darwin.Send(mi, trsel.setTarget, target)
			darwin.Send(mi, trsel.setTag, int(it.ID))
		}
		darwin.Send(mi, trsel.setEnabled, !it.Disabled)
		if it.Checkable {
			state := darwin.NSControlStateValueOff
			if it.Checked {
				state = darwin.NSControlStateValueOn
			}
			darwin.Send(mi, trsel.setState, state)
		}
		darwin.Send(menu, trsel.addItem, mi)
		darwin.Release(mi)
	}
	return menu
}

// cloneMenuItems returns a deep copy of a menu, so the backend keeps what
// it shows whatever the app does with its slice.
func cloneMenuItems(items []platform.MenuItem) []platform.MenuItem {
	if len(items) == 0 {
		return nil
	}
	out := slices.Clone(items)
	for i := range out {
		out[i].Submenu = cloneMenuItems(out[i].Submenu)
	}
	return out
}

// trayOf returns the tray of a TelaTrayTarget.
func trayOf(target darwin.ID) *trayIcon {
	deskObjects.Lock()
	defer deskObjects.Unlock()
	return deskObjects.trays[target]
}

// trayClicked is the button's action of a tray without a menu.
func trayClicked(self darwin.ID, _ darwin.SEL, _ darwin.ID) {
	t := trayOf(self)
	if t == nil {
		return
	}
	t.b.guard(func() { t.b.appEvent(platform.TrayEvent{Tray: t.id, Kind: platform.TrayActivated}) })
}

// trayItemChosen is the action of a tray menu's items: the tag is the
// item's ID. Only enabled items of the current menu are reported.
func trayItemChosen(self darwin.ID, _ darwin.SEL, sender darwin.ID) {
	t := trayOf(self)
	if t == nil {
		return
	}
	t.b.guard(func() {
		id := platform.MenuItemID(darwin.SendInt(sender, trsel.tag))
		if it, ok := platform.FindMenuItem(t.menu, id); ok && !it.Disabled {
			t.b.appEvent(platform.TrayEvent{Tray: t.id, Kind: platform.TrayMenuItem, Item: id})
		}
	})
}
