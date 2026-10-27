package kit

import (
	"fmt"

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
	// heights are 32, 47 and 40 px, the ones of the OS's own bars.
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
// To show the bar only where the window has no title bar of its own, read
// the window state and leave it out otherwise:
//
//	if tela.UseWindow().State().ClientSideDecorations { ... kit.TitleBar{...} ... }
//
// A fullscreen window usually has no bar either: leave it out on
// State().Fullscreen. In tela snapshot there is no window: the bar is
// drawn focused and not maximized, and its buttons do nothing.
type TitleBar struct {
	// Variant is the look: Default (the platform's), Windows, GNOME or
	// MacOS, or one the active [Style] adds.
	Variant Variant
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
	lk := resolveLook(t.Variant)
	root := pick("TitleBar", "Variant", s.Variants, lk)
	checkSide(t.ControlsSide)
	atStart := controlsAtStart(lk, t.ControlsSide)

	state := s.Windowed[lk]
	if st.Maximized || st.Fullscreen {
		state = s.Maximized[lk]
	}
	controls := WindowControls{Variant: lk, HideMinimize: t.HideMinimize, HideMaximize: t.HideMaximize, State: t.State}

	kids := []tela.Child{cls(s.Root, root, state, when(!st.Focused, s.Inactive[lk]), t.Class), h.WindowDrag(), t.Props}
	if atStart || t.Leading != nil {
		lead := []tela.Child{cls(s.Leading[lk], t.LeadingClass)}
		if atStart {
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
	if !atStart {
		end = append(end, controls)
	}
	return h.Div(append(kids, h.Div(end...))...)
}
