package kit

import (
	"runtime"
	"strconv"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// The looks of the window controls and the title bar (ADR-182). They are
// [Variant]s, like the looks of every other component: the recipes of
// [WindowControlsStyle] and [TitleBarStyle] define them, and an app adds a
// look by adding a name to their maps. Default, the zero value, is the
// look of the platform the app runs on: Windows on Windows, MacOS on
// macOS, GNOME everywhere else.
const (
	// Windows is Windows 11's: rectangular 46 px buttons that fill the
	// title bar's height, thin glyphs, and a close button that turns red.
	Windows Variant = "windows"
	// GNOME is Adwaita's: round icon buttons in a tall header bar, with
	// the title centered.
	GNOME Variant = "gnome"
	// MacOS is Aqua's: the red, yellow and green dots at the start of the
	// bar, whose glyphs show when the pointer is over them.
	MacOS Variant = "macos"
)

// platformLook returns the look of an operating system, as runtime.GOOS
// names it.
func platformLook(goos string) Variant {
	switch goos {
	case "windows":
		return Windows
	case "darwin":
		return MacOS
	}
	return GNOME
}

// resolveLook returns the look a Variant field asks for: itself, or the platform's
// when it is Default.
func resolveLook(v Variant) Variant {
	if v == Default {
		return platformLook(runtime.GOOS)
	}
	return v
}

// useWindowState returns the state a window component draws: the override it
// was given (for previews and tests), else the window's own. It is a hook
// (it reads the window state, so the component builds again when that
// changes); without a real window, as in tela snapshot, the window is
// focused and nothing else.
func useWindowState(override *tela.WindowState) tela.WindowState {
	st := tela.UseWindow().State()
	if override != nil {
		return *override
	}
	return st
}

// WindowControlsStyle is the recipe of [WindowControls].
type WindowControlsStyle struct {
	// Root is the group the buttons are in.
	Root string
	// Variants are the looks of the group: Windows, GNOME and MacOS.
	Variants map[Variant]string
	// Button is what every button has: layout, transition and focus ring.
	Button string
	// ButtonVariants are the looks of a button: its size, shape, color and
	// hover.
	ButtonVariants map[Variant]string
	// MinimizeVariants, MaximizeVariants and CloseVariants are added to
	// the button of that control, per look: the dot colors of MacOS, the
	// red hover of Windows' close button.
	MinimizeVariants, MaximizeVariants, CloseVariants map[Variant]string
	// Inactive is added to every button while the window is not focused.
	Inactive map[Variant]string
	// Icon is what the icons have, before IconVariants.
	Icon string
	// IconVariants are the icon's size and color, per look.
	IconVariants map[Variant]string
}

func init() {
	defaultStyle.WindowControls = WindowControlsStyle{
		Root: "flex shrink-0 items-center",
		Variants: map[Variant]string{
			Windows: "items-start",
			GNOME:   "gap-2 px-2",
			MacOS:   "group gap-2 px-3",
		},
		Button: "inline-flex shrink-0 items-center justify-center outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/60",
		ButtonVariants: map[Variant]string{
			Windows: "h-8 w-[46px] text-foreground hover:bg-foreground/8 active:bg-foreground/5 focus-visible:ring-inset",
			GNOME:   "size-6 rounded-full bg-foreground/7 text-foreground hover:bg-foreground/12 active:bg-foreground/18",
			MacOS:   "size-3 rounded-full border border-black/10 text-black/60 dark:border-white/10",
		},
		MinimizeVariants: map[Variant]string{MacOS: "bg-[#febc2e]"},
		MaximizeVariants: map[Variant]string{MacOS: "bg-[#28c840]"},
		CloseVariants: map[Variant]string{
			Windows: "hover:bg-[#c42b1c] hover:text-white active:bg-[#c42b1c]/90 active:text-white/80",
			GNOME:   "hover:bg-destructive/15 hover:text-destructive active:bg-destructive/25",
			MacOS:   "bg-[#ff5f57]",
		},
		Inactive: map[Variant]string{
			Windows: "text-muted-foreground",
			GNOME:   "bg-foreground/4 text-muted-foreground",
			MacOS:   "border-foreground/10 bg-foreground/15",
		},
		Icon: "pointer-events-none shrink-0",
		IconVariants: map[Variant]string{
			Windows: "size-2.5",
			GNOME:   "size-4",
			MacOS:   "size-2 opacity-0 transition-opacity group-hover:opacity-100",
		},
	}
}

// The glyphs of the controls, one set per look, as Tela's own SVGs (not
// Lucide's: the OS glyphs are thinner and sit on the pixel grid). Every
// WindowControls icon slot defaults to the one of its look; a look the
// recipes added uses GNOME's.
var controlGlyphs = map[Variant]struct{ minimize, maximize, restore, close tela.Icon }{
	Windows: {
		minimize: svgIcon(10, `<path d="M0 5.5h10" fill="none" stroke="currentColor" stroke-width="1"/>`),
		maximize: svgIcon(10, `<rect x=".5" y=".5" width="9" height="9" rx="1.5" fill="none" stroke="currentColor" stroke-width="1"/>`),
		restore: svgIcon(10, `<g fill="none" stroke="currentColor" stroke-width="1">`+
			`<rect x=".5" y="2.5" width="7" height="7" rx="1.5"/>`+
			`<path d="M2.5 2.5v-1a1 1 0 0 1 1-1h5a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1h-1"/></g>`),
		close: svgIcon(10, `<path d="M.5.5l9 9M9.5.5l-9 9" fill="none" stroke="currentColor" stroke-width="1"/>`),
	},
	GNOME: {
		minimize: svgIcon(16, `<path d="M4.5 10.75h7" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>`),
		maximize: svgIcon(16, `<rect x="4.25" y="4.25" width="7.5" height="7.5" rx="1.25" fill="none" stroke="currentColor" stroke-width="1.5"/>`),
		restore: svgIcon(16, `<g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round">`+
			`<rect x="3.75" y="6.25" width="6" height="6" rx="1"/>`+
			`<path d="M6.25 4.25v-.5a1 1 0 0 1 1-1h4.5a1 1 0 0 1 1 1v4.5a1 1 0 0 1-1 1h-.5" stroke-linecap="round"/></g>`),
		close: svgIcon(16, `<path d="M4.75 4.75l6.5 6.5M11.25 4.75l-6.5 6.5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>`),
	},
	MacOS: {
		minimize: svgIcon(8, `<path d="M1 4h6" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>`),
		maximize: svgIcon(8, `<path d="M1.2 6.8V3.1L4.9 6.8zM6.8 1.2v3.7L3.1 1.2z" fill="currentColor" stroke="currentColor" stroke-width=".6" stroke-linejoin="round"/>`),
		restore:  svgIcon(8, `<path d="M3.6 7.2V3.9H.3zM4.4.8v3.3h3.3z" fill="currentColor" stroke="currentColor" stroke-width=".6" stroke-linejoin="round" transform="translate(.3 0)"/>`),
		close:    svgIcon(8, `<path d="M1.5 1.5l5 5M6.5 1.5l-5 5" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>`),
	},
}

// svgIcon is a size by size SVG document of the given shapes.
func svgIcon(size int, shapes string) tela.Icon {
	s := strconv.Itoa(size)
	return tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="` + s + `" height="` + s + `" viewBox="0 0 ` + s + ` ` + s + `">` + shapes + `</svg>`}
}

// WindowControls are the minimize, maximize (restore while the window is
// maximized) and close buttons of a custom title bar: the ones of
// Windows 11, of GNOME's Adwaita or of macOS, by the platform the app
// runs on or by Variant. [TitleBar] draws them at the side of its
// platform; use WindowControls alone to place them yourself.
//
//	kit.WindowControls{}                         // the platform's look
//	kit.WindowControls{Variant: kit.Windows, HideMinimize: true}
//
// Each button is a real button with the window-control prop of
// h.WindowControl, so a click does what its name says without any handler,
// the OS knows what the button is (Windows 11 shows its snap layouts over
// the maximize button), and the keyboard works. The maximize button's name
// and glyph follow the window's state: "Maximize" and "Restore". The
// group takes no space of its own beyond its buttons, and is not a drag
// area: put it inside a bar that has h.WindowDrag (the buttons are holes
// in it by themselves).
//
// Without a window, as in tela snapshot, the buttons draw and do nothing.
//
// The glyphs, the names and the look are all slots and recipes: the icon
// and label fields replace one, and [WindowControlsStyle] restyles a look
// or adds one.
type WindowControls struct {
	// Variant is the look: Default (the platform's), Windows, GNOME or
	// MacOS, or one the active [Style] adds.
	Variant Variant
	// HideMinimize and HideMaximize leave a button out, for a window that
	// cannot be minimized or maximized (a dialog, a fixed-size window).
	// Close is always there.
	HideMinimize, HideMaximize bool
	// State, when set, is the state to draw in place of the window's:
	// Maximized changes the maximize button, Focused false dims the
	// group. It is for previews and tests; leave it nil in an app.
	State *tela.WindowState
	// MinimizeIcon, MaximizeIcon, RestoreIcon and CloseIcon replace the
	// glyphs of the look.
	MinimizeIcon, MaximizeIcon, RestoreIcon, CloseIcon tela.Icon
	// MinimizeLabel, MaximizeLabel, RestoreLabel and CloseLabel are the
	// buttons' accessible names ("Minimize", "Maximize", "Restore",
	// "Close"); set them to translate.
	MinimizeLabel, MaximizeLabel, RestoreLabel, CloseLabel string
	// AriaLabel names the group ("Window controls").
	AriaLabel string
	// Class is added after the recipe's classes.
	Class string
	// Props are more props for the group element.
	Props tela.Child
}

// Build returns the group of buttons.
func (w WindowControls) Build() tela.Node {
	st := useWindowState(w.State)
	s := &UseStyle().WindowControls
	lk := resolveLook(w.Variant)
	root := pick("WindowControls", "Variant", s.Variants, lk)
	glyphs, ok := controlGlyphs[lk]
	if !ok {
		glyphs = controlGlyphs[GNOME]
	}

	maxIcon, maxLabel := orIcon(w.MaximizeIcon, glyphs.maximize), orLabel(w.MaximizeLabel, "Maximize")
	if st.Maximized {
		maxIcon, maxLabel = orIcon(w.RestoreIcon, glyphs.restore), orLabel(w.RestoreLabel, "Restore")
	}
	minIcon, minLabel := orIcon(w.MinimizeIcon, glyphs.minimize), orLabel(w.MinimizeLabel, "Minimize")
	closeIcon, closeLabel := orIcon(w.CloseIcon, glyphs.close), orLabel(w.CloseLabel, "Close")

	button := func(c tela.WindowControl, label string, ic tela.Icon, extra map[Variant]string) tela.Child {
		return h.Button(
			cls(s.Button, s.ButtonVariants[lk], extra[lk], when(!st.Focused, s.Inactive[lk])),
			h.Type("button"), h.WindowControl(c), h.AriaLabel(label),
			icon(ic, s.Icon, s.IconVariants[lk]))
	}
	minimize := when2(!w.HideMinimize, func() tela.Child { return button(tela.WindowMinimize, minLabel, minIcon, s.MinimizeVariants) })
	maximize := when2(!w.HideMaximize, func() tela.Child { return button(tela.WindowMaximize, maxLabel, maxIcon, s.MaximizeVariants) })
	closeBtn := button(tela.WindowClose, closeLabel, closeIcon, s.CloseVariants)

	label := orLabel(w.AriaLabel, "Window controls")
	kids := []tela.Child{cls(s.Root, root, w.Class), h.Role("group"), h.AriaLabel(label), w.Props}
	if lk == MacOS {
		// Aqua orders them close, minimize, zoom.
		return h.Div(append(kids, closeBtn, minimize, maximize)...)
	}
	return h.Div(append(kids, minimize, maximize, closeBtn)...)
}

func orLabel(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// when2 returns f's child if cond is true, else no child.
func when2(cond bool, f func() tela.Child) tela.Child {
	if cond {
		return f()
	}
	return nil
}
