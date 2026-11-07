package core

import (
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// The fallback title bar (ADR-230, ADR-238): a framed window whose OS
// draws no title bar (WindowState.ClientSideDecorations while the window
// was opened neither Frameless nor TitleBarOverlay: GNOME on Wayland) gets
// one drawn by core, in the app's theme, unless the window's own tree has
// a title bar: an h.WindowDrag area of its own (kit.TitleBar). Winit's
// sctk-adwaita frame and Electron's fallback frame do the same.
//
// The bar is shown while the window is framed, reports client-side
// decorations, is not fullscreen and its tree has no mounted WindowDrag
// prop other than the bar's own (chromeState.drags). The decision is made
// at the end of the build phase (syncChrome), from the mounted props, so
// an app bar that mounts or unmounts hides or shows the fallback in the
// same frame: no frame shows both bars, or none. A tree without a window
// (Render, tela snapshot) never shows it, nor a headless window unless
// the fake window manager reports client-side decorations.
//
// The bar is a second child of the root element, after the app's root
// (which keeps its place, so showing or hiding the bar never remounts the
// app), keyed so it matches only itself. The window's content is laid out
// below it, as the browser lays a page out below its own chrome:
//
//   - The root box gets a top border of the bar's height in its layout
//     style only (not in its computed style, so nothing is painted and
//     the root's paint geometry, its clip, stays the window): its content
//     box, which h-full fills and in-flow content starts in, and its
//     padding box, against which fixed and absolute boxes (and portal
//     layers: dialogs, popovers) are placed, start below the bar. The
//     root scrolls in that area and its scrollbars stay in it.
//   - The style context's viewport (vw and vh units, h-screen,
//     breakpoints) is the content area, the window minus the bar (Tree.
//     contentSize); floating elements are kept inside it too.
//   - The bar itself is a fixed box at the top of the window, in the
//     root's border band, above every box of the app but the portal
//     layers (z-index one less than theirs; it stays in the band, so it
//     overlaps nothing the layout put there).
//
// The bar is a drag area (the OS moves the window, maximizes it on a
// double click and opens the window menu on a right click), its buttons
// are window controls, holes in the drag area that act on a click, and
// the backends' resize band at the top edge wins over both (a press there
// resizes). It stays alive under a modal overlay (Element.outside): as the
// OS's own title bar, its buttons work while a dialog is open. Its buttons
// follow the desktop's button-layout (SystemSettings.TitleBarButtons),
// minimize, maximize and close at the end without one; a modal window
// has only close and a FixedSize window no maximize, as GTK's header bar.
// It follows the direction of the app's outermost element (the Dir of the
// root component's element), mirroring like kit.TitleBar.

// chromeHeight is the height of the fallback title bar, in logical pixels
// (ADR-238: Slim).
const chromeHeight = 36

// chromeKey is the key of the bar among the root's children: a type of
// its own, so no key of the app's root content matches it.
type chromeKey struct{}

// chromeState is a tree's fallback title bar.
type chromeState struct {
	// content is the app's root node (the root's first child, Mount's).
	content node
	// shown: the bar is the root's second child; props are the props it
	// was built with, h the height the layout reserves for it.
	shown bool
	props barProps
	h     float32
	// drags counts the mounted WindowDrag props that are not the bar's.
	drags int
	// check: syncChrome must decide again at the end of the next build
	// phase (the window state, the title, the button layout or drags
	// changed).
	check bool
	// buttons is the desktop's button layout (SystemSettings).
	buttons string
}

// barProps are the props of the bar's component (comparable: it builds
// again only when they change, or the window state it reads).
type barProps struct {
	title, buttons string
	rtl            bool
	// noMin and noMax leave out minimize and maximize.
	noMin, noMax bool
}

// rootNode returns the node of the root element: the root classes, the
// app's root and, while it is shown, the bar.
func (t *Tree) rootNode() node {
	kids := make([]node, 1, 2)
	kids[0] = t.chrome.content
	if t.chrome.shown {
		kids = append(kids, nodeOf(ComponentProps(fallbackBar, t.chrome.props, Key(chromeKey{})), "the title bar is"))
	}
	return node{&desc{kind: nodeElement, elem: style.KindRoot, classes: t.rootClasses, children: kids}}
}

// chromeWant reports whether the tree's window shows the bar now, and
// with which props.
func (t *Tree) chromeWant() (barProps, bool) {
	v := t.wv
	if v == nil || !v.framed || !v.os.ClientSideDecorations || v.os.Fullscreen || t.chrome.drags > 0 {
		return barProps{}, false
	}
	return barProps{
		title: v.title, buttons: t.chrome.buttons, rtl: t.contentRTL(),
		noMin: v.modal, noMax: v.modal || v.fixed,
	}, true
}

// noteChrome asks for a decision at the next build phase when the bar
// should change (UI thread, outside the build: window state, title and
// settings changes).
func (t *Tree) noteChrome() {
	c := &t.chrome
	if c.check || t.root == nil {
		return
	}
	if p, want := t.chromeWant(); want != c.shown || want && p != c.props {
		c.check = true
		t.dirty()
	}
}

// chromeDrag counts a WindowDrag prop of el going from old to now
// (regionChanged); the bar's own drag area does not count.
func (t *Tree) chromeDrag(el *Element, old, now uint8) {
	if el.node.d.chrome {
		return
	}
	c := &t.chrome
	switch {
	case old != regionDrag && now == regionDrag:
		c.drags++
		if c.drags == 1 {
			c.check = true
			t.dirty()
		}
	case old == regionDrag && now != regionDrag:
		c.drags--
		if c.drags == 0 {
			c.check = true
			t.dirty()
		}
	}
}

// syncChrome shows, updates or hides the bar (under the build lock, at the
// end of the build phase and of Mount): the root is reconciled with or
// without it, and the layout and the viewport units follow.
func (t *Tree) syncChrome() {
	c := &t.chrome
	c.check = false
	if t.root == nil {
		return
	}
	p, want := t.chromeWant()
	if want == c.shown && (!want || p == c.props) {
		return
	}
	c.shown, c.props = want, p
	t.update(t.root, t.rootNode())
	h := float32(0)
	if want {
		h = chromeHeight
	}
	t.setChromeHeight(h)
}

// resetChrome forgets the bar when the tree unmounts its content.
func (t *Tree) resetChrome() {
	c := &t.chrome
	c.content, c.shown, c.props, c.check = node{}, false, barProps{}, false
	t.setChromeHeight(0)
}

// setChromeHeight reserves h logical pixels at the top of the window for
// the bar: the viewport units and breakpoints see the rest, and the root
// lays out below it (remapPhase).
func (t *Tree) setChromeHeight(h float32) {
	if h == t.chrome.h {
		return
	}
	t.chrome.h = h
	t.cx.Viewport = t.contentSize()
	t.restyleAll()
}

// contentSize returns the logical size of the window's content area: the
// viewport without the fallback title bar.
func (t *Tree) contentSize() gfx.Size {
	return gfx.Size{W: t.viewport.W, H: max(t.viewport.H-t.chrome.h, 0)}
}

// contentArea returns the content area in window coordinates: the window
// below the fallback title bar.
func (t *Tree) contentArea() gfx.Rect {
	return gfx.Rect{Y0: min(t.chrome.h, t.viewport.H), X1: t.viewport.W, Y1: t.viewport.H}
}

// chromeBorder adds the bar's band to the root's layout style ls.
func (t *Tree) chromeBorder(ls *layout.Style) {
	h := t.chrome.h
	if h <= 0 {
		return
	}
	if b := ls.Border.Top; b.Unit == layout.UnitPx {
		ls.Border.Top = layout.Px(b.Value + h)
	} else {
		ls.Border.Top = layout.Px(h)
	}
}

// chromeStyle places the bar's box (computeStyle): fixed over the root's
// border band, the window's width, above every box of the app but the
// portal layers.
func chromeStyle(cs *style.ComputedStyle) {
	zero := style.Dim{Kind: style.DimLength}
	cs.Position = style.PositionFixed
	cs.Inset = style.Sides[style.Dim]{Top: style.Dim{Kind: style.DimLength, LP: style.LP{Px: -chromeHeight}}, Right: zero, Left: zero}
	cs.Height = style.Dim{Kind: style.DimLength, LP: style.LP{Px: chromeHeight}}
	cs.MinHeight, cs.MaxHeight = style.Dim{}, style.Dim{}
	cs.Margin = style.Sides[style.Dim]{Top: zero, Right: zero, Bottom: zero, Left: zero}
	cs.ZIndex = style.ZIndex{Explicit: true, Value: portalZ - 1}
}

// contentRTL reports whether the app's outermost element (the first
// primitive of the root's content, through components) lays out right to
// left: its Dir, or the direction its style resolved.
func (t *Tree) contentRTL() bool {
	if t.root == nil || len(t.root.children) == 0 {
		return false
	}
	el := t.root.children[0]
	for el != nil && el.kind == elemComponent {
		if len(el.children) == 0 {
			return false
		}
		el = el.children[0]
	}
	if el == nil || el.kind != elemPrimitive || el.node.d.chrome {
		return false
	}
	switch el.node.d.dir {
	case dirRTL:
		return true
	case dirAuto:
		return el.box.cs != nil && el.box.cs.Text.Direction == text.DirectionRTL
	}
	return false
}

// chromeBarProp marks the root of the bar: its drag area, which hides
// nothing (chromeDrag), and its place (chromeStyle).
func chromeBarProp() Prop {
	return Prop{name: "WindowDrag", apply: func(d *desc) { d.winRegion, d.chrome = regionDrag, true }}
}

// The glyphs of the bar's buttons: kit's GNOME glyphs (kit/windowcontrols.go).
const (
	glyphMinimize = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path d="M4.5 10.75h7" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>`
	glyphMaximize = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><rect x="4.25" y="4.25" width="7.5" height="7.5" rx="1.25" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>`
	glyphRestore  = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round">` +
		`<rect x="3.75" y="6.25" width="6" height="6" rx="1"/>` +
		`<path d="M6.25 4.25v-.5a1 1 0 0 1 1-1h4.5a1 1 0 0 1 1 1v4.5a1 1 0 0 1-1 1h-.5" stroke-linecap="round"/></g></svg>`
	glyphClose = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path d="M4.75 4.75l6.5 6.5M11.25 4.75l-6.5 6.5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>`
)

// The classes of the bar (ADR-238's look B, Slim), in theme tokens so it
// follows the app's theme and color scheme. Lengths are in px: the band
// the layout reserves does not grow with the text scale.
const (
	barClass      = "grid grid-cols-[1fr_auto_1fr] items-center border-b border-border bg-background text-[13px] font-medium select-none"
	barStartClass = "flex min-w-0 items-center justify-start gap-[2px] ps-[4px]"
	barEndClass   = "flex min-w-0 items-center justify-end gap-[2px] pe-[4px]"
	barTitleClass = "min-w-0 truncate"
	barButtonBase = "flex size-[28px] shrink-0 items-center justify-center rounded-md outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/60"
	barHover      = " hover:bg-accent hover:text-accent-foreground active:bg-accent/70"
	barCloseHover = " hover:bg-destructive/15 hover:text-destructive active:bg-destructive/25"
	barIconClass  = "pointer-events-none size-[16px] shrink-0"
	barText       = " text-muted-foreground"
	barTextDim    = " text-muted-foreground/60"
)

// barLayout is a parsed button layout: the controls at the start and at
// the end of the bar, in order.
type barLayout struct{ start, end []WindowControl }

// parseButtonLayout parses GNOME's button-layout (and GTK's
// gtk-decoration-layout): the names before the colon at the start, those
// after it at the end, separated by commas. minimize, maximize and close
// are the names drawn; others (appmenu, menu, icon, spacer) and repeats
// are left out, and a string without a colon is all start. Empty is the
// default, minimize, maximize and close at the end.
func parseButtonLayout(s string) barLayout {
	if strings.TrimSpace(s) == "" {
		return barLayout{end: []WindowControl{WindowMinimize, WindowMaximize, WindowClose}}
	}
	var l barLayout
	var seen [WindowClose + 1]bool
	side := func(part string) []WindowControl {
		var out []WindowControl
		for _, name := range strings.Split(part, ",") {
			var c WindowControl
			switch strings.TrimSpace(name) {
			case "minimize":
				c = WindowMinimize
			case "maximize":
				c = WindowMaximize
			case "close":
				c = WindowClose
			default:
				continue
			}
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
		return out
	}
	start, end, _ := strings.Cut(s, ":")
	end, _, _ = strings.Cut(end, ":") // GTK reads two parts only
	l.start = side(start)
	l.end = side(end)
	return l
}

// fallbackBar builds the bar: the title centered between the two sides,
// the window buttons at the sides the layout puts them.
func fallbackBar(p barProps) Node {
	st := UseWindow().State()
	dim := barText
	if !st.Focused {
		dim = barTextDim
	}
	l := parseButtonLayout(p.buttons)
	button := func(c WindowControl) Child {
		label, glyph, hover := "Minimize", glyphMinimize, barHover
		switch c {
		case WindowMaximize:
			label, glyph = "Maximize", glyphMaximize
			if st.Maximized {
				label, glyph = "Restore", glyphRestore
			}
		case WindowClose:
			label, glyph, hover = "Close", glyphClose, barCloseHover
		}
		return Primitive(style.KindButton, Class(barButtonBase+dim+hover), FieldType("button"),
			WindowControlProp(c), AriaLabel(label),
			Primitive(style.KindSVG, IconSrc(glyph), Class(barIconClass)))
	}
	side := func(cls string, cs []WindowControl) Node {
		kids := []Child{Class(cls)}
		n := 0
		for _, c := range cs {
			if c == WindowMinimize && p.noMin || c == WindowMaximize && p.noMax {
				continue
			}
			kids = append(kids, button(c))
			n++
		}
		if n > 0 {
			kids = append(kids, Role("group"), AriaLabel("Window controls"))
		}
		return Primitive(style.KindDiv, kids...)
	}
	dir := "ltr"
	if p.rtl {
		dir = "rtl"
	}
	return Primitive(style.KindDiv, chromeBarProp(), Class(barClass+dim), Dir(dir),
		side(barStartClass, l.start),
		Primitive(style.KindSpan, Class(barTitleClass), Text(p.title)),
		side(barEndClass, l.end),
	)
}
