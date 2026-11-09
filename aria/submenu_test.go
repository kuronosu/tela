package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The root menu: New, Share (a submenu: Email, Messages, More, itself a
// submenu: Print, Save), Delete.
var (
	rootKeys  = []string{"new", "share", "delete"}
	shareKeys = []string{"email", "messages", "more"}
	moreKeys  = []string{"print", "save"}
)

// subMenu is one level of the test menu.
type subMenu struct {
	keys []string
	subs map[string][]string
	// root: the trigger's results; sub: the submenu this one is.
	mt  aria.MenuTriggerResult
	sub aria.SubmenuTriggerResult
	dir string
	ev  *events
}

func (m subMenu) Build() tela.Node {
	o := aria.MenuOptions[string]{
		Keys: m.keys, TextValue: capital, Direction: m.dir,
		OnAction: func(k string) { m.ev.add("action:" + k) },
	}
	for k := range m.subs {
		o.SubmenuKeys = append(o.SubmenuKeys, k)
	}
	var pop aria.PopoverResult
	var own tela.Prop
	if m.sub.MenuRef == (tela.ElementRef{}) {
		pop = aria.UsePopover(aria.PopoverOptions{
			Trigger: m.mt.Ref, Ref: m.mt.MenuRef, Open: true, OnClose: m.mt.Close, Placement: "bottom-start", Modal: true,
		})
		o.OnClose, o.AutoFocus, o.FocusStrategy, o.Ref = m.mt.Close, true, m.mt.FocusStrategy, m.mt.MenuRef
		own = m.mt.MenuProps
	} else {
		pop = aria.UsePopover(m.sub.PopoverOptions())
		o.Submenu = m.sub
		own = m.sub.MenuProps
	}
	menu := aria.UseMenu(o)
	kids := []tela.Child{pop.Props, own, menu.Props, Class("w-40 bg-white")}
	for _, k := range m.keys {
		kids = append(kids, Div(menu.ItemProps(k), Key(k), Class("h-8 px-2"), Text(capital(k))))
	}
	// The submenus are children of the menu's element, not of their item:
	// the item would get the submenu's pointer events.
	for _, k := range m.keys {
		inner, ok := m.subs[k]
		if !ok {
			continue
		}
		sub := menu.SubmenuTrigger(k)
		next := map[string][]string{}
		if k == "share" {
			next["more"] = moreKeys
		}
		kids = append(kids, Portal(Key("sub-"+k), If(sub.Open, subMenu{keys: inner, subs: next, sub: sub, dir: m.dir, ev: m.ev})))
	}
	return Div(kids...)
}

func subMenuApp(dir string, ev *events) func() tela.Node {
	return func() tela.Node {
		mt := aria.UseMenuTrigger(aria.MenuTriggerOptions{})
		left := "left-4"
		if dir == "rtl" {
			left = "left-[436px]"
		}
		return Div(Class("relative h-full"), Dir(dir),
			Button(mt.TriggerProps, Class("absolute top-4 h-8 w-24", left), Text("Actions")),
			Portal(If(mt.Open, subMenu{keys: rootKeys, subs: map[string][]string{"share": shareKeys}, mt: mt, dir: dir, ev: ev})))
	}
}

func TestSubmenuKeyboard(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, subMenuApp("", &ev), telatest.Size(600, 400))
	trigger := telatest.ByRole("button", "Actions")
	share := menuitem("Share")
	app.Focus(trigger)
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.ExpectFocused(share)
	app.Expect(share, telatest.Expanded(false))
	// ArrowRight opens it on its first item.
	app.Press("ArrowRight")
	app.ExpectFocused(menuitem("Email"))
	app.Expect(share, telatest.Expanded(true))
	// ArrowLeft closes one level and gives the focus back to the trigger.
	app.Press("ArrowLeft")
	app.ExpectFocused(share)
	app.ExpectCount(menuitem("Email"), 0)
	app.ExpectCount(menuitem("Delete"), 1)
	// Enter and Space open it too; Escape closes one level.
	app.Press("Enter")
	app.ExpectFocused(menuitem("Email"))
	app.Press("Escape")
	app.ExpectFocused(share)
	app.ExpectCount(menuitem("Delete"), 1)
	app.Press("Space")
	app.ExpectFocused(menuitem("Email"))
	// A nested submenu, and typeahead in it: per level.
	app.Press("m")
	app.ExpectFocused(menuitem("Messages"))
	app.Press("ArrowDown")
	app.Press("ArrowRight")
	app.ExpectFocused(menuitem("Print"))
	app.Press("s")
	app.ExpectFocused(menuitem("Save"))
	app.Press("Escape")
	app.ExpectFocused(menuitem("More"))
	app.ExpectCount(menuitem("Email"), 1)
	// Choosing an item closes the whole menu; the focus returns to the
	// menu's trigger.
	app.Press("ArrowUp")
	app.Press("Enter")
	if got := ev.take(); got != "action:messages" {
		t.Errorf("events %q", got)
	}
	app.ExpectCount(menuitem("New"), 0)
	app.ExpectFocused(trigger)
	// Escape at the root closes the menu.
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Press("ArrowRight")
	app.Press("Escape")
	app.Press("Escape")
	app.ExpectCount(menuitem("New"), 0)
	app.ExpectFocused(trigger)
}

func TestSubmenuKeyboardRTL(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, subMenuApp("rtl", &ev), telatest.Size(600, 400))
	share := menuitem("Share")
	app.Focus(telatest.ByRole("button", "Actions"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	// ArrowRight is the key back under rtl: nothing opens.
	app.Press("ArrowRight")
	app.ExpectFocused(share)
	app.Press("ArrowLeft")
	app.ExpectFocused(menuitem("Email"))
	// The submenu opens on the left of its trigger.
	if s, m := app.Bounds(menuitem("Email")), app.Bounds(share); s.X+s.Width > m.X+1 {
		t.Errorf("submenu at %v, trigger at %v: want it on the left", s, m)
	}
	app.Press("ArrowLeft") // not the back key under rtl
	app.ExpectFocused(menuitem("Email"))
	app.Press("ArrowRight")
	app.ExpectFocused(share)
	app.ExpectCount(menuitem("Email"), 0)
}

func TestSubmenuHoverOpensAfterDelay(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, subMenuApp("", &ev), telatest.Size(600, 400))
	share := menuitem("Share")
	app.Click(telatest.ByRole("button", "Actions"))
	app.Hover(share)
	app.Advance(aria.SubmenuDelay / 2)
	app.ExpectCount(menuitem("Email"), 0)
	app.Advance(aria.SubmenuDelay)
	app.ExpectCount(menuitem("Email"), 1)
	// The pointer opened it: the focus stays on the trigger.
	app.ExpectFocused(share)
	// Another item hovered closes it, and one left before the delay does
	// not open it.
	app.Hover(menuitem("New"))
	app.ExpectCount(menuitem("Email"), 0)
	app.Hover(share)
	app.Advance(aria.SubmenuDelay / 2)
	app.Hover(menuitem("Delete"))
	app.Advance(aria.SubmenuDelay * 2)
	app.ExpectCount(menuitem("Email"), 0)
	// A click opens it at once, and on a submenu item chooses it.
	app.Click(share)
	app.ExpectCount(menuitem("Email"), 1)
	app.Click(menuitem("Email"))
	if got := ev.take(); got != "action:email" {
		t.Errorf("events %q", got)
	}
	app.ExpectCount(menuitem("New"), 0)
}

// The open delay ends when the pointer leaves the trigger, out of the menu
// or onto its gap, before it runs out (React Aria and Radix cancel it): the
// submenu does not open beside a trigger that is no longer hovered.
func TestSubmenuHoverLeaveCancelsDelay(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, subMenuApp("", &ev), telatest.Size(600, 400))
	share := menuitem("Share")
	app.Click(telatest.ByRole("button", "Actions"))
	app.Hover(share)
	app.Advance(aria.SubmenuDelay / 2)
	app.MouseMove(580, 380) // out of the menu
	app.Advance(aria.SubmenuDelay * 2)
	app.ExpectCount(menuitem("Email"), 0)
	app.Expect(share, telatest.Expanded(false))
	// Back on the trigger, the delay starts over and opens it.
	app.Hover(share)
	app.Advance(aria.SubmenuDelay * 2)
	app.ExpectCount(menuitem("Email"), 1)
}

// moveTo moves the mouse in steps the grace looks at (one every 60 ms).
func moveTo(app *telatest.App, pts ...[2]float32) {
	for _, p := range pts {
		app.MouseMove(p[0], p[1])
		app.Advance(60 * time.Millisecond)
	}
}

func TestSubmenuPointerGrace(t *testing.T) {
	for _, toward := range []bool{true, false} {
		name := "straight down"
		if toward {
			name = "toward the submenu"
		}
		t.Run(name, func(t *testing.T) {
			var ev events
			app := telatest.MountFunc(t, subMenuApp("", &ev), telatest.Size(600, 400))
			share := menuitem("Share")
			app.Click(telatest.ByRole("button", "Actions"))
			app.Hover(share)
			app.Advance(aria.SubmenuDelay * 2)
			app.ExpectCount(menuitem("Email"), 1)
			r, sub := app.Bounds(share), app.Bounds(menuitem("Email"))
			x, y := r.X+r.Width/2, r.Y+r.Height/2
			if toward {
				// Down and to the right, over Delete, into the submenu.
				moveTo(app, [2]float32{x, y}, [2]float32{x + 10, y + 3}, [2]float32{x + 20, y + 6},
					[2]float32{x + 30, y + 9}, [2]float32{x + 40, y + 22}, [2]float32{x + 55, y + 30})
				app.ExpectCount(menuitem("Email"), 1)
				app.ExpectFocused(share)
				moveTo(app, [2]float32{sub.X + 10, y + 36})
				app.ExpectCount(menuitem("Email"), 1)
				app.ExpectFocused(menuitem("Messages"))
				// Moves inside the submenu are its own: the parent menu's
				// items do not take the focus back.
				moveTo(app, [2]float32{sub.X + 20, y + 40}, [2]float32{sub.X + 30, y + 44})
				app.ExpectFocused(menuitem("Messages"))
				app.Expect(share, telatest.Expanded(true))
				return
			}
			moveTo(app, [2]float32{x, y}, [2]float32{x, y + 10}, [2]float32{x, y + 32})
			app.ExpectFocused(menuitem("Delete"))
			app.ExpectCount(menuitem("Email"), 0)
		})
	}
}

// The grace ends when the mouse rests on its way: the item under it is
// hovered then, and the submenu closes.
func TestSubmenuGraceEndsWhenThePointerRests(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, subMenuApp("", &ev), telatest.Size(600, 400))
	share := menuitem("Share")
	app.Click(telatest.ByRole("button", "Actions"))
	app.Hover(share)
	app.Advance(aria.SubmenuDelay * 2)
	r := app.Bounds(share)
	x, y := r.X+r.Width/2, r.Y+r.Height/2
	moveTo(app, [2]float32{x, y}, [2]float32{x + 10, y + 3}, [2]float32{x + 20, y + 6},
		[2]float32{x + 30, y + 9}, [2]float32{x + 40, y + 22})
	app.ExpectCount(menuitem("Email"), 1)
	app.Advance(2 * time.Second)
	app.ExpectFocused(menuitem("Delete"))
	app.ExpectCount(menuitem("Email"), 0)
}

func TestSubmenuOutsidePressClosesTheChain(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, subMenuApp("", &ev), telatest.Size(600, 400))
	app.Focus(telatest.ByRole("button", "Actions"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Press("ArrowRight")
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Press("ArrowRight")
	app.ExpectFocused(menuitem("Print"))
	// A press on the parent menu is that menu's: it chooses New.
	app.Click(menuitem("New"))
	if got := ev.take(); got != "action:new" {
		t.Errorf("events %q", got)
	}
	app.ExpectCount(menuitem("New"), 0)
	// A press outside every menu closes them all.
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Press("ArrowRight")
	app.ExpectFocused(menuitem("Email"))
	app.ClickAt(590, 390)
	app.ExpectCount(menuitem("New"), 0)
	app.ExpectCount(menuitem("Email"), 0)
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

func TestSubmenuSnapshot(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, subMenuApp("", &ev), telatest.Size(600, 400))
	app.Focus(telatest.ByRole("button", "Actions"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Press("ArrowRight")
	// The page is inert under the modal menu; the submenu is a menu of
	// its own, named by its trigger.
	app.ExpectAria(`
- menu "Actions":
  - menuitem "New"
  - menuitem "Share" [expanded]
  - menuitem "Delete"
- menu "Share":
  - menuitem "Email"
  - menuitem "Messages"
  - menuitem "More"
`)
}
