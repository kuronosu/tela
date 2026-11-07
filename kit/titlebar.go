package kit

import (
	"fmt"
	"math"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// TitleBarStyle is the recipe of [TitleBar]. Every field but Root and
// Variants is a map from a look (Windows, GNOME, MacOS, or one the recipe
// adds) to the classes of one part, so a look is one entry in each.
type TitleBarStyle struct {
	// Root is what every look has: the width, no text selection.
	Root string
	// Variants are the looks of the bar: Windows, GNOME and MacOS. Their
	// heights are 32, 47 and 40 px, the ones of the OS's own bars. Plain
	// is added to the platform's look, after it, and takes away its
	// background and borders.
	Variants map[Variant]string
	// Windowed and Maximized are added to the bar while the window is
	// neither maximized nor fullscreen, and while it is: the outline a
	// floating window has and a maximized one does not.
	Windowed, Maximized map[Variant]string
	// Inactive is added while the window is not focused.
	Inactive map[Variant]string
	// Leading, Middle and End are the three regions of the bar: the start
	// (with the window controls on a look that puts them there), the title
	// and content, and the end (the trailing slot and the controls).
	Leading, Middle, End map[Variant]string
	// Title, Content and Trailing are the parts inside them: the title's
	// text, the content slot's box and the trailing slot's box.
	Title, Content, Trailing map[Variant]string
}

func init() {
	// The centered looks (GNOME, MacOS) lay the bar out as a grid of
	// 1fr auto 1fr, so the title is centered whatever the two sides hold.
	const (
		leading = "col-start-1 row-start-1 flex items-center gap-1"
		middle  = "col-start-2 row-start-1 flex min-w-0 items-center justify-center gap-2"
		end     = "col-start-3 row-start-1 flex items-center justify-end gap-1"
	)
	defaultStyle.TitleBar = TitleBarStyle{
		Root: "w-full shrink-0 text-foreground select-none",
		Variants: map[Variant]string{
			Windows: "flex h-8 items-center bg-background text-xs",
			GNOME:   "grid h-[47px] grid-cols-[1fr_auto_1fr] items-center border-b border-border bg-muted text-sm font-semibold",
			MacOS:   "grid h-10 grid-cols-[1fr_auto_1fr] items-center border-b border-border bg-muted text-[13px] font-semibold",
			Plain:   "border-0 bg-transparent",
		},
		Windowed: map[Variant]string{
			Windows: "border-t border-border",
			GNOME:   "border-t border-border",
			MacOS:   "border-t border-border",
		},
		Maximized: map[Variant]string{},
		Inactive: map[Variant]string{
			Windows: "text-muted-foreground",
			GNOME:   "bg-background text-muted-foreground",
			MacOS:   "bg-background text-muted-foreground",
			Plain:   "text-muted-foreground",
		},
		Leading: map[Variant]string{
			Windows: "flex shrink-0 items-center gap-2 ps-3",
			GNOME:   leading + " ps-2",
			MacOS:   leading,
		},
		Middle: map[Variant]string{
			Windows: "flex min-w-0 grow items-center gap-3 ps-3",
			GNOME:   middle,
			MacOS:   middle,
		},
		End: map[Variant]string{
			Windows: "flex shrink-0 items-center",
			GNOME:   end,
			MacOS:   end + " pe-2",
		},
		Title: map[Variant]string{
			Windows: "min-w-0 truncate",
			GNOME:   "min-w-0 truncate",
			MacOS:   "min-w-0 truncate",
		},
		Content: map[Variant]string{
			Windows: "flex min-w-0 grow items-center justify-center",
			GNOME:   "flex min-w-0 items-center",
			MacOS:   "flex min-w-0 items-center",
		},
		Trailing: map[Variant]string{
			Windows: "flex items-center gap-1 pe-2",
			GNOME:   "flex items-center gap-1",
			MacOS:   "flex items-center gap-1",
		},
	}
}

// TitleBar is the title bar of a window that draws its own: a
// [tela.Window] or [tela.App] with Frameless, or a window on a Wayland
// desktop whose compositor draws no title bars (the window's
// State().ClientSideDecorations). It is a drag area (h.WindowDrag): the OS
// moves the window when it is dragged, maximizes or restores it on a
// double click and opens the window menu on a right click, so the bar does
// nothing for those, and every interactive thing inside it (a button, a
// search field, tabs) stays clickable, because the buttons, fields and
// anything with a handler are holes in the drag area by themselves.
//
//	func Notes() tela.Node {
//	    return h.Div(h.Class("flex h-screen flex-col"),
//	        kit.TitleBar{
//	            Leading:  kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: lucide.Menu, AriaLabel: "Menu"},
//	            Title:    h.Text("Notes"),
//	            Trailing: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: lucide.Settings, AriaLabel: "Settings"},
//	        },
//	        h.Div(h.Class("min-h-0 grow"), content),
//	    )
//	}
//
// The bar draws [WindowControls] at the side the platform keeps them (the
// end, in the reading direction, and the start on macOS; ControlsSide
// overrides it), so under h.Dir("rtl") the whole bar mirrors. Its look is
// the platform's by default (Windows, GNOME or MacOS: the title at the
// start on Windows, centered on the others; Variant picks another), it
// dims while the window is not focused, and loses its outline while the
// window is maximized or fullscreen.
//
// Variant Plain is the bar of an app with a design of its own: the
// platform's arrangement (heights, the title at the start or centered, the
// controls at the end or, as dots, at the start), and nothing else. It has
// no background, border or outline, so the surface under it shows (give the
// bar one with Class: "bg-sidebar"), and its window controls are ghost
// buttons in the app's theme colors. Look picks another platform's
// arrangement.
//
// To show the bar only where the window has no title bar of its own, read
// the window state and leave it out otherwise:
//
//	if tela.UseWindow().State().ClientSideDecorations { ... kit.TitleBar{...} ... }
//
// A window with tela.App.TitleBarOverlay on macOS keeps the OS's own
// buttons (the traffic lights) over the bar, and on Windows 11 DWM's
// caption buttons at the end: its State().TitleBarControls
// is the rectangle they cover, and the bar draws no controls of its own
// and keeps that room free instead, at the side the look keeps its controls
// (ControlsSide: the start on macOS, so the leading slot starts after the
// traffic lights; they stay at the window's left under rtl). Where the OS
// draws no buttons there, the rectangle is empty and the bar draws its
// controls as usual.
//
// A fullscreen window usually has no bar either: leave it out on
// State().Fullscreen. In tela snapshot there is no window: the bar is
// drawn focused and not maximized, and its buttons do nothing.
type TitleBar struct {
	// Variant is the look: Default (the platform's), Windows, GNOME,
	// MacOS or Plain, or one the active [Style] adds. Plain is the
	// platform's arrangement with no background, border or outline of its
	// own, and quiet window controls: for an app whose own surface shows.
	Variant Variant
	// Look is the look whose arrangement Plain follows: Windows, GNOME or
	// MacOS. Empty is the platform's. It is read only with Variant Plain.
	Look Variant
	// Title is the window's name, truncated when there is no room.
	Title tela.Component
	// Leading is drawn at the start, before the title: a menu button, the
	// app's icon, a back button. On MacOS it follows the window controls.
	Leading tela.Component
	// Children is the content beside the title that takes the room
	// left: a search field, tabs, a menu bar. On Windows it is centered
	// in that room.
	Children tela.Component
	// Trailing is drawn at the end, before the window controls: actions
	// such as share or settings.
	Trailing tela.Component
	// ControlsSide is "start" or "end": the side of the bar that has the
	// window controls. Empty is the look's own (the end, except MacOS).
	ControlsSide string
	// HideMinimize and HideMaximize leave those buttons out (see
	// [WindowControls]).
	HideMinimize, HideMaximize bool
	// State, when set, is the window state to draw in place of the
	// window's own: Maximized, Fullscreen and Focused. It is for previews
	// and tests; leave it nil in an app.
	State *tela.WindowState
	// TitleClass, LeadingClass, ContentClass and TrailingClass are added
	// to the part after the recipe's classes.
	TitleClass, LeadingClass, ContentClass, TrailingClass string
	// Class is added to the bar after the recipe's classes ("bg-card").
	Class string
	// Props are more props for the bar element.
	Props tela.Child
}

// controlsAtStart reports whether the window controls are at the start of
// the bar: the ControlsSide asked for, else the look's own.
func controlsAtStart(lk Variant, side string) bool {
	switch side {
	case "start":
		return true
	case "end":
		return false
	}
	return lk == MacOS
}

// checkSide validates TitleBar.ControlsSide: dev mode panics on another
// value.
func checkSide(side string) {
	switch side {
	case "", "start", "end":
	default:
		if devMode() {
			panic(fmt.Sprintf("kit: TitleBar.ControlsSide %q is not a side: use \"start\" or \"end\" (or leave it empty for the look's own)", side))
		}
	}
}

// Build returns the bar.
func (t TitleBar) Build() tela.Node {
	st := useWindowState(t.State)
	s := &UseStyle().TitleBar
	lk, plain := resolvePlain(t.Variant, t.Look)
	root := pick("TitleBar", "Variant", s.Variants, lk)
	checkSide(t.ControlsSide)
	atStart := controlsAtStart(lk, t.ControlsSide)

	// The outline and the dimmed background are the platform's own; Plain
	// leaves them out and only dims the text.
	state, inactive, quiet := s.Windowed[lk], s.Inactive[lk], ""
	if st.Maximized || st.Fullscreen {
		state = s.Maximized[lk]
	}
	controls := WindowControls{Variant: lk, HideMinimize: t.HideMinimize, HideMaximize: t.HideMaximize, State: t.State}
	// The OS's own window buttons over the bar (TitleBarOverlay): their
	// room is kept free at the physical side they are on, and the bar draws
	// no controls.
	reserve, reserveLead := titleBarRoom(st.TitleBarControls, tela.UseDirection() == "rtl")
	if plain {
		state, inactive, quiet = "", s.Inactive[Plain], s.Variants[Plain]
		controls.Variant, controls.Look = Plain, lk
	}

	kids := []tela.Child{cls(s.Root, root, quiet, state, when(!st.Focused, inactive), t.Class), h.WindowDrag(), t.Props}
	if atStart && reserve == nil || reserve != nil && reserveLead || t.Leading != nil {
		lead := []tela.Child{cls(s.Leading[lk], t.LeadingClass)}
		switch {
		case reserve != nil && reserveLead:
			lead = append(lead, reserve)
		case atStart && reserve == nil:
			lead = append(lead, controls)
		}
		kids = append(kids, h.Div(append(lead, t.Leading)...))
	}
	var title, content tela.Child
	if t.Title != nil {
		title = h.Div(cls(s.Title[lk], t.TitleClass), t.Title)
	}
	if t.Children != nil {
		content = h.Div(cls(s.Content[lk], t.ContentClass), t.Children)
	}
	kids = append(kids, h.Div(cls(s.Middle[lk]), title, content))
	end := []tela.Child{cls(s.End[lk])}
	if t.Trailing != nil {
		end = append(end, h.Div(cls(s.Trailing[lk], t.TrailingClass), t.Trailing))
	}
	switch {
	case reserve != nil && !reserveLead:
		end = append(end, reserve)
	case !atStart && reserve == nil:
		end = append(end, controls)
	}
	return h.Div(append(kids, h.Div(end...))...)
}

// titleBarRoom returns the empty box that keeps the room of the OS's window
// buttons (rectangle r in the window, State().TitleBarControls) free, and
// whether it goes in the leading region of the bar. The side is where the
// buttons are, whatever the look draws: at the window's left when the
// rectangle starts within its own width of the left edge (macOS's traffic
// lights), at the right otherwise; the leading region is the left one
// except under rtl. The box is nil without buttons. It covers the window's
// edge up to the end of the buttons plus a gap, at the left; at the right
// it covers the buttons and the gap the left side has (the window's width
// is not known here).
func titleBarRoom(r tela.Rect, rtl bool) (box tela.Child, lead bool) {
	if !(r.Width > 0 && r.Height > 0) {
		return nil, false
	}
	const gap = 8
	left := r.X < r.Width
	w := float64(r.X+r.Width) + gap
	if !left {
		w = float64(r.Width) + 2*gap
	}
	n := int(math.Ceil(w))
	return h.Div(cls(fmt.Sprintf("w-[%dpx] shrink-0", n)), h.AriaHidden(true)), left != rtl
}
