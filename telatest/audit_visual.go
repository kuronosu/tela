package telatest

import (
	"fmt"
	"image"
	"math"
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
)

// The rules about what is on screen (ADR-312): the text's contrast, text
// that is cut off or lies over other text, targets over targets, clicks that
// a keyboard cannot make, and the focus indicator. They read the laid-out
// tree (core.Tree.AuditTexts) and, for the focus, the rendered frames. Each
// is a heuristic that reports what a designer would agree is a bug; the
// limits of each are in its doc comment, and ADR-312 lists them.

func init() {
	register(
		&ruleDef{Rule: Rule{ID: "color-contrast", Description: "text has a contrast ratio of at least 4.5:1 against its background, 3:1 for large text (WCAG 2.2 success criterion 1.4.3)", Impact: "serious",
			Tags: []string{"wcag2aa", "wcag143"}, Default: true},
			all: checkContrast},
		&ruleDef{Rule: Rule{ID: "click-events-have-key-events", Description: "an element that reacts to a click can take the focus, so that the keyboard can operate it (WCAG 2.1.1)", Impact: "serious",
			Tags: []string{"wcag2a", "wcag211", "aria"}, Default: true},
			all: checkClickKeys},
		&ruleDef{Rule: Rule{ID: "text-clipped", Description: "no text is cut off by an overflow clip or by the window's edge", Impact: "moderate",
			Tags: []string{"layout", "wcag1412"}},
			all: checkTextClipped},
		&ruleDef{Rule: Rule{ID: "text-overlap", Description: "no text in normal flow lies over another text", Impact: "moderate",
			Tags: []string{"layout"}},
			all: checkTextOverlap},
		&ruleDef{Rule: Rule{ID: "target-overlap", Description: "no target (button, link, field...) in normal flow lies over another", Impact: "moderate",
			Tags: []string{"layout"}},
			all: checkTargetOverlap},
		&ruleDef{Rule: Rule{ID: "focus-visible", Description: "every element the Tab key reaches shows that it has the focus (WCAG 2.2 success criterion 2.4.7)", Impact: "serious",
			Tags: []string{"wcag2aa", "wcag247"}},
			all: checkFocusVisible},
	)
}

// ---------------------------------------------------------------------
// Texts.

// textNode returns the semantics node of text t, or nil.
func (c *auditCtx) textNode(t core.AuditText) *semantics.Node {
	if t.Node == 0 {
		return nil
	}
	return c.byID[t.Node]
}

// usable reports whether the rules judge text t: its element is on screen,
// not excluded and the text is not hidden for assistive technologies.
func (c *auditCtx) usable(t core.AuditText) bool {
	n := c.textNode(t)
	if n == nil {
		return true
	}
	for p := n; p != nil; p = c.parent[p] {
		if p.Has(semantics.StateHidden) && !p.Decorative || c.excluded[p.ID] {
			return false
		}
	}
	return true
}

// textElement names a text for a violation: its first words quoted, and
// the element that holds it.
func textElement(t core.AuditText, snippet string) string {
	if snippet == "" {
		snippet = t.Text
	}
	s := fmt.Sprintf("text %q", ellipsize(snippet, 40))
	if t.Kind != "" {
		s += " in " + t.Kind
		if t.Classes != "" {
			s += fmt.Sprintf(" (class %q)", ellipsize(t.Classes, 60))
		}
	}
	return s
}

func ellipsize(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// union returns the bounds of the lines of a text.
func linesBounds(lines []gfx.Rect) gfx.Rect {
	var u gfx.Rect
	for _, r := range lines {
		u = u.Union(r)
	}
	return u
}

// reportText adds a violation on text t.
func (c *auditCtx) reportText(t core.AuditText, snippet, message, fix string) {
	c.reportAt(c.textNode(t), textElement(t, snippet), t.Src, linesBounds(t.Lines), message, fix)
}

func hex(c gfx.Color) string {
	c = c.Clamp()
	b := func(v float32) int { return int(math.Round(float64(v) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", b(c.R), b(c.G), b(c.B))
}

// luminance is the relative luminance of an sRGB color (WCAG 2.2's
// definition).
func luminance(c gfx.Color) float64 {
	lin := func(v float32) float64 {
		x := float64(min(max(v, 0), 1))
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// contrastRatio is WCAG's (L1 + 0.05) / (L2 + 0.05), lighter over darker.
func contrastRatio(a, b gfx.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// checkContrast implements WCAG 1.4.3 on the colors the frame composites:
// the text color (with its alpha and the opacity around it) over the
// backgrounds behind it. Large text (24 px, or 18.66 px and bold) needs
// 3:1, other text 4.5:1. Text over an image, a gradient, a backdrop blur
// or a filter, over an out-of-flow box with no opaque background, or
// painted with a gradient, is not judged (axe reports it as incomplete);
// nor is disabled text, text faded to 60% or less by opacity (the look of
// an inactive control, such as the label of a disabled checkbox, which the
// tree does not tie to the control), or text in a hidden element. Only text is checked: the 3:1 of borders, icons and
// focus rings (1.4.11) is not.
func checkContrast(c *auditCtx) {
	for _, t := range c.a.tree.AuditTexts() {
		if !c.usable(t) {
			continue
		}
		if n := c.textNode(t); n != nil && c.disabled(n) {
			continue
		}
		for _, r := range t.Runs {
			if !r.Known || r.Opacity <= 0.6 {
				continue // not judged: unknown colors, or the faded look of an inactive control
			}
			need := 4.5
			if r.Size >= 24 || (r.Size >= 18.66 && r.Weight >= 700) {
				need = 3
			}
			ratio := contrastRatio(r.Fg, r.Bg)
			if ratio+0.005 >= need {
				continue
			}
			size := "text"
			if need == 3 {
				size = "large text"
			}
			c.reportText(t, r.Text,
				fmt.Sprintf("%s is %s on %s: contrast %.2f:1, %s needs %g:1 (WCAG 2.2 AA 1.4.3)", size, hex(r.Fg), hex(r.Bg), ratio, size, need),
				fmt.Sprintf("raise the contrast to %g:1: darken the text color or lighten its background (a theme's foreground over its background does)", need))
		}
	}
}

// checkTextClipped reports text whose glyphs are cut by an overflow clip
// (overflow hidden or clip) of a box around it, or by the window when no
// scroll container reaches it. Text that truncates on purpose (text-overflow,
// line-clamp), text in a scroll container (a scroll away), visually hidden
// text (a clip box of a few pixels: sr-only), and text under a transform
// (an off-canvas drawer, an animation) are not reported. Lines are measured
// by their glyphs, ascent to descent, with 1.5 px of tolerance across and
// 2 px along; a line box cut where no ink is does not count.
func checkTextClipped(c *auditCtx) {
	// A layout wider than the window cuts many texts for one reason: the
	// first few are reported, the rest as one.
	const windowCuts = 3
	var cut int
	var more gfx.Rect
	var moreN int // how many more
	for _, t := range c.a.tree.AuditTexts() {
		if !c.usable(t) || t.Truncates {
			continue
		}
		visW, visH := t.Clip.Width(), t.Clip.Height()
		if visW <= 4 || visH <= 4 {
			continue // sr-only, or a box of nothing
		}
		var cutR, cutL, cutT, cutB float32
		var lines int
		for _, ln := range t.Lines {
			l, r := max(t.Clip.X0-ln.X0, 0), max(ln.X1-t.Clip.X1, 0)
			tp, bt := max(t.Clip.Y0-ln.Y0, 0), max(ln.Y1-t.Clip.Y1, 0)
			if l > 1.5 || r > 1.5 || tp > 2 || bt > 2 {
				lines++
				cutL, cutR, cutT, cutB = max(cutL, l), max(cutR, r), max(cutT, tp), max(cutB, bt)
			}
		}
		if lines == 0 {
			continue
		}
		// Horizontal cuts win over vertical ones in the message: they are
		// the usual cause.
		var side string
		var px float32
		var by core.AuditBox
		switch {
		case cutR > 1.5:
			side, px, by = "right", cutR, t.ClipBy[2]
		case cutL > 1.5:
			side, px, by = "left", cutL, t.ClipBy[0]
		case cutB > 2:
			side, px, by = "bottom", cutB, t.ClipBy[3]
		default:
			side, px, by = "top", cutT, t.ClipBy[1]
		}
		if by.Kind == "" {
			if cut++; cut > windowCuts {
				more = more.Union(linesBounds(t.Lines))
				moreN++
				continue
			}
		}
		what := "the window's edge (no scroll container holds it)"
		fix := "give the text room (w-full, min-w-0 in a flex row), let it wrap (remove whitespace-nowrap), or hold it in a scroll container"
		if by.Kind != "" {
			what = fmt.Sprintf("the %s", by.Kind)
			if by.Classes != "" {
				what += fmt.Sprintf(" with class %q", ellipsize(by.Classes, 60))
			}
			if by.Src != "" {
				what += " at " + by.Src
			}
			fix = "give the box room for the text (a taller or wider box, min-h/min-w, no fixed size), let it wrap, truncate it on purpose (truncate, line-clamp-N), or make the box scroll (overflow-auto)"
		}
		c.reportText(t, "",
			fmt.Sprintf("clipped on the %s by %s: %.0f px of %d line(s) are out of sight", side, what, px, lines), fix)
	}
	if moreN > 0 {
		c.reportAt(nil, "more text cut off by the window's edge", "", more,
			"more texts are cut off by the window's edge: the layout is wider or taller than the window",
			"find the box that does not shrink (a fixed width, whitespace-nowrap, a min-w), or hold the content in a scroll container")
	}
}

// checkTextOverlap reports two texts in normal flow whose glyphs lie over
// each other: a block too short for its lines, a negative margin, a grid cell
// that is too narrow. Text over an out-of-flow box (absolute, fixed, sticky,
// floating, in a portal), over a text of another portal, and a text and the
// text of the element that holds it (a button in a sentence) are overlays or
// nesting, not overlaps. An overlap counts when the glyph rectangles of two
// lines share a third of the smaller one and at least 3 px each way, in the
// part both are visible in.
func checkTextOverlap(c *auditCtx) {
	texts := c.a.tree.AuditTexts()
	nested := func(a, b core.AuditText) bool {
		for o := a.Outer; o >= 0; o = texts[o].Outer {
			if o == b.Index {
				return true
			}
		}
		for o := b.Outer; o >= 0; o = texts[o].Outer {
			if o == a.Index {
				return true
			}
		}
		return false
	}
	for i, a := range texts {
		if !a.Flow || !c.usable(a) {
			continue
		}
		for _, b := range texts[i+1:] {
			if !b.Flow || b.Layer != a.Layer || !c.usable(b) || nested(a, b) {
				continue
			}
			if overlapOf(a, b) {
				c.reportText(a, "",
					fmt.Sprintf("lies over the text %q", ellipsize(b.Text, 40)),
					"give the first block room for its lines (no fixed height; min-h instead of h), or space the two apart; check it at narrow widths and with large text")
			}
		}
	}
}

// overlapOf reports whether a line of a and a line of b overlap on screen.
func overlapOf(a, b core.AuditText) bool {
	for _, la := range a.Lines {
		for _, lb := range b.Lines {
			x := la.Intersect(lb)
			if a.Scroller != 0 && a.Scroller == b.Scroller {
				x = x.Intersect(a.Clip).Intersect(b.Clip) // one scroll content: wherever it is
			} else {
				x = x.Intersect(a.Visible).Intersect(b.Visible) // only what is in sight lies over
			}
			if x.Width() < 3 || x.Height() < 3 {
				continue
			}
			if small := min(la.Area(), lb.Area()); x.Area() >= small/3 {
				return true
			}
		}
	}
	return false
}

// checkTargetOverlap reports two targets in normal flow whose boxes share a
// quarter of the smaller one and at least 3 px each way, neither inside the
// other (a button in a card). Overlays (portals, out-of-flow boxes, a
// popover over its page) and disabled targets are not judged.
func checkTargetOverlap(c *auditCtx) {
	flows := c.a.tree.AuditFlows()
	type target struct {
		n *semantics.Node
		f core.AuditFlow
	}
	var all []target
	c.root.Walk(func(n *semantics.Node) bool {
		if n.Has(semantics.StateHidden) || c.excluded[n.ID] {
			return false
		}
		if n.Kind == semantics.KindElement && slices.Contains(targetRoles, n.Role) && !n.Bounds.Empty() && !n.Inline && !c.disabled(n) {
			if f, ok := flows[n.ID]; ok && f.Flow {
				all = append(all, target{n, f})
			}
		}
		return true
	})
	inside := func(a, b *semantics.Node) bool {
		for p := a; p != nil; p = c.parent[p] {
			if p == b {
				return true
			}
		}
		for p := b; p != nil; p = c.parent[p] {
			if p == a {
				return true
			}
		}
		return false
	}
	for i, a := range all {
		for _, b := range all[i+1:] {
			if a.f.Layer != b.f.Layer || inside(a.n, b.n) {
				continue
			}
			x := a.n.Bounds.Intersect(b.n.Bounds)
			if x.Width() < 3 || x.Height() < 3 || x.Area() < min(a.n.Bounds.Area(), b.n.Bounds.Area())/4 {
				continue
			}
			c.report(a.n, fmt.Sprintf("lies over %s (%.0fx%.0f px in common)", c.describe(b.n), x.Width(), x.Height()),
				"space the targets apart (gap-2, margins), or give the row room to wrap (flex-wrap); check it at narrow widths")
		}
	}
}

// checkClickKeys reports an element that has a click handler and cannot
// take the focus, so a keyboard user cannot operate it (jsx-a11y's
// click-events-have-key-events, axe's keyboard rules). Not reported: an
// element inside something focusable (the button handles the keys), one
// with a focusable element inside (a card whose button is the keyboard
// path), a disabled one, and a box that covers most of the window (the
// backdrop of a dialog, closed by Escape).
func checkClickKeys(c *auditCtx) {
	win := c.a.tree.Viewport()
	winArea := win.W * win.H
	var hasFocusable func(n *semantics.Node) bool
	hasFocusable = func(n *semantics.Node) bool {
		for _, k := range n.Children {
			if k.Has(semantics.StateFocusable) || hasFocusable(k) {
				return true
			}
		}
		return false
	}
	c.root.Walk(func(n *semantics.Node) bool {
		if n.Has(semantics.StateHidden) || c.excluded[n.ID] {
			return false
		}
		if n.Kind != semantics.KindElement || !n.Can(semantics.ActionClick) || n.Has(semantics.StateFocusable) || n.Bounds.Empty() {
			return true
		}
		if c.disabled(n) || n.Bounds.Area() >= 0.6*winArea || hasFocusable(n) {
			return true
		}
		for p := c.parent[n]; p != nil; p = c.parent[p] {
			if p.Has(semantics.StateFocusable) {
				return true
			}
		}
		switch n.Role {
		case "label", "none", "presentation", "generic", "text", "paragraph", "listitem", "row", "cell", "gridcell", "img", "region", "group", "":
		default:
			return true // another role: the focus rules of the role judge it
		}
		c.report(n, "reacts to a click but cannot take the focus: a keyboard cannot operate it",
			`use a Button (or an A with Href), or give it h.Role("button"), h.TabIndex(0) and an OnKeyDown for Enter and Space`)
		return true
	})
}

// ---------------------------------------------------------------------
// Focus.

// checkFocusVisible presses Tab through the screen, from no focus, and for
// each element it reaches compares the frame with the one before any
// focus: when nothing around the element changed (a quarter of a
// logical pixel squared is nothing; a caret is not a ring), the element shows
// no focus indicator. "Around" is the element and the component it is the
// only focusable part of (a field in its bordered wrapper), 8 px out. At
// most 100 stops are tried; the focus returns where it was. An element that
// scrolls when focused is compared where it ends. A text field (textbox,
// searchbox, combobox, spinbutton) is not judged: its caret shows the focus,
// as a browser's does, even where the field has no ring of its own.
func checkFocusVisible(c *auditCtx) {
	a := c.a
	count := 0
	c.root.Walk(func(n *semantics.Node) bool {
		if n.Has(semantics.StateHidden) {
			return false
		}
		if n.Kind == semantics.KindElement && n.Has(semantics.StateFocusable) && !c.disabled(n) {
			count++
		}
		return true
	})
	if count == 0 {
		return
	}
	var orig semantics.NodeID
	c.root.Walk(func(n *semantics.Node) bool {
		if n.Kind == semantics.KindElement && n.Has(semantics.StateFocused) {
			orig = n.ID
		}
		return true
	})
	defer func() {
		a.tree.Blur()
		if orig != 0 {
			_ = a.tree.FocusNode(orig)
		}
		a.settleAfter()
	}()
	a.tree.Blur()
	base := cloneRGBA(a.Screenshot())
	win := a.tree.Viewport()
	k := float64(base.Bounds().Dx()) / float64(win.W)
	seen := map[semantics.NodeID]bool{}
	for step := 0; step < min(count+2, 100); step++ {
		a.Press("Tab")
		root := a.tree.Semantics()
		var n *semantics.Node
		root.Walk(func(x *semantics.Node) bool {
			if n == nil && x.Kind == semantics.KindElement && x.Has(semantics.StateFocused) {
				n = x
			}
			return n == nil
		})
		if n == nil || seen[n.ID] {
			return
		}
		seen[n.ID] = true
		if c.isExcluded(c.byID[n.ID]) || n.Bounds.Empty() {
			continue
		}
		switch n.Role {
		case "textbox", "searchbox", "combobox", "spinbutton":
			continue // the caret is the indicator of a text field
		}
		img := a.Screenshot()
		if changedPixels(base, img, c.focusRegion(n, win.W*win.H), k) >= 24 {
			continue
		}
		old := c.byID[n.ID]
		if old == nil {
			old = n
		}
		c.report(old, "reached with Tab and nothing around it changes: no focus indicator (WCAG 2.2 2.4.7)",
			`style the focused state: focus-visible:ring-2 focus-visible:ring-ring (or focus-visible:outline-2), and keep outline-none only with one`)
	}
}

// focusRegion is the window rectangle in which a focus indicator of n
// appears: n's bounds grown to the largest ancestor that holds no other
// focusable element and is smaller than a quarter of the window, 8 px out.
func (c *auditCtx) focusRegion(n *semantics.Node, winArea float32) gfx.Rect {
	r := n.Bounds
	focusables := func(p *semantics.Node) (k int) {
		p.Walk(func(x *semantics.Node) bool {
			if x.Kind == semantics.KindElement && x.Has(semantics.StateFocusable) && !x.Has(semantics.StateHidden) {
				k++
			}
			return true
		})
		return k
	}
	for p := c.parent[n]; p != nil && p.Kind == semantics.KindElement && !p.Bounds.Empty(); p = c.parent[p] {
		if p.Bounds.Area() > winArea/4 || focusables(p) > 1 {
			break
		}
		r = r.Union(p.Bounds)
	}
	return r.Outset(8)
}

func cloneRGBA(img *image.RGBA) *image.RGBA {
	out := image.NewRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	return out
}

// changedPixels counts the pixels of region r (logical pixels, k device
// pixels each) that differ between two frames by more than 5/255 in a
// channel, as logical pixels squared.
func changedPixels(a, b *image.RGBA, r gfx.Rect, k float64) float64 {
	rect := image.Rect(int(math.Floor(float64(r.X0)*k)), int(math.Floor(float64(r.Y0)*k)),
		int(math.Ceil(float64(r.X1)*k)), int(math.Ceil(float64(r.Y1)*k))).Intersect(a.Rect).Intersect(b.Rect)
	n := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		ia, ib := a.PixOffset(rect.Min.X, y), b.PixOffset(rect.Min.X, y)
		for x := rect.Min.X; x < rect.Max.X; x++ {
			for ch := range 4 {
				d := int(a.Pix[ia+ch]) - int(b.Pix[ib+ch])
				if d > 5 || d < -5 {
					n++
					break
				}
			}
			ia, ib = ia+4, ib+4
		}
	}
	return float64(n) / (k * k)
}
