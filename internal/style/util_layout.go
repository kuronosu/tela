package style

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/text"
)

// sortedMapKeys returns the keys of m in order, so that registration does
// not depend on map iteration order.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

type sideMask uint8

const (
	sTop sideMask = 1 << iota
	sRight
	sBottom
	sLeft
	sAll = sTop | sRight | sBottom | sLeft
)

func setSidesDim(s *Sides[Dim], m sideMask, d Dim) {
	if m&sTop != 0 {
		s.Top = d
	}
	if m&sRight != 0 {
		s.Right = d
	}
	if m&sBottom != 0 {
		s.Bottom = d
	}
	if m&sLeft != 0 {
		s.Left = d
	}
}

func setSidesLP(s *Sides[LP], m sideMask, l LP) {
	if m&sTop != 0 {
		s.Top = l
	}
	if m&sRight != 0 {
		s.Right = l
	}
	if m&sBottom != 0 {
		s.Bottom = l
	}
	if m&sLeft != 0 {
		s.Left = l
	}
}

// sideRoots lists the roots of a per-side utility family: "p", "px", "py",
// "pt", "pr", "pb", "pl", "ps", "pe" for prefix "p". Start and end are
// left and right (Tela lays out left to right).
func sideRoots(prefix, sep string) []struct {
	root string
	mask sideMask
} {
	return []struct {
		root string
		mask sideMask
	}{
		{prefix, sAll}, {prefix + sep + "x", sLeft | sRight}, {prefix + sep + "y", sTop | sBottom},
		{prefix + sep + "t", sTop}, {prefix + sep + "r", sRight}, {prefix + sep + "b", sBottom}, {prefix + sep + "l", sLeft},
		{prefix + sep + "s", sLeft}, {prefix + sep + "e", sRight},
	}
}

func registerLayout(r *registry) {
	displays := map[string]Display{
		"block": DisplayBlock, "inline-block": DisplayInlineBlock, "inline": DisplayInline,
		"flex": DisplayFlex, "inline-flex": DisplayInlineFlex, "grid": DisplayGrid,
		"inline-grid": DisplayInlineGrid, "contents": DisplayContents, "hidden": DisplayNone,
		"flow-root": DisplayBlock,
	}
	for _, k := range sortedMapKeys(displays) {
		r.static(k, display(displays[k]))
	}

	positions := map[string]Position{
		"static": PositionStatic, "fixed": PositionFixed, "absolute": PositionAbsolute,
		"relative": PositionRelative, "sticky": PositionSticky,
	}
	for _, k := range sortedMapKeys(positions) {
		p := positions[k]
		r.static(k, op(func(e *env) { e.s.Position = p }))
	}

	// Insets: top, right, bottom, left, inset, inset-x, inset-y, start, end.
	insetOpts := dimAuto | dimFull | dimFrac | dimSpacing | dimNeg
	for _, p := range []struct {
		root string
		mask sideMask
	}{
		{"inset", sAll}, {"inset-x", sLeft | sRight}, {"inset-y", sTop | sBottom},
		{"top", sTop}, {"right", sRight}, {"bottom", sBottom}, {"left", sLeft},
		{"start", sLeft}, {"end", sRight},
	} {
		mask := p.mask
		r.dimUtil(p.root, insetOpts, axisX, func(e *env, d Dim) { setSidesDim(&e.s.Inset, mask, d) })
	}

	// z-index.
	r.add("z", true, func(c *pctx, a *arg) (Op, error) {
		if a.raw == "auto" {
			return op(func(e *env) { e.s.ZIndex = ZIndex{} }), nil
		}
		n, ok := numberArg(a)
		if !ok || n != float32(int32(n)) {
			return Op{}, errNoMatch
		}
		if a.neg {
			n = -n
		}
		z := int32(n)
		return op(func(e *env) { e.s.ZIndex = ZIndex{Explicit: true, Value: z} }), nil
	}, func(*Theme) []string { return []string{"auto", "0", "10", "20", "30", "40", "50"} })

	// Overflow.
	ov := map[string]Overflow{"visible": OverflowVisible, "hidden": OverflowHidden, "clip": OverflowClip,
		"scroll": OverflowScroll, "auto": OverflowAuto}
	for _, root := range []string{"overflow", "overflow-x", "overflow-y"} {
		keys := map[string]Op{}
		for _, k := range sortedMapKeys(ov) {
			v := ov[k]
			switch root {
			case "overflow":
				keys[k] = op(func(e *env) { e.s.OverflowX, e.s.OverflowY = v, v })
			case "overflow-x":
				keys[k] = op(func(e *env) { e.s.OverflowX = v })
			default:
				keys[k] = op(func(e *env) { e.s.OverflowY = v })
			}
		}
		r.enum(root, false, keys)
	}

	r.static("isolate", op(func(e *env) { e.s.Isolate = true }))
	r.static("isolation-auto", op(func(e *env) { e.s.Isolate = false }))
	r.static("visible", op(func(e *env) { e.s.Visibility = VisibilityVisible }))
	r.static("invisible", op(func(e *env) { e.s.Visibility = VisibilityHidden }))
	r.static("collapse", op(func(e *env) { e.s.Visibility = VisibilityCollapse }))

	// Aspect ratio.
	r.add("aspect", false, func(c *pctx, a *arg) (Op, error) {
		var ratio float32
		switch {
		case a.arb && !a.hasMod:
			rv, ok := parseRatio(a.inner)
			if !ok {
				return Op{}, fmt.Errorf("%q is not a ratio; use [4/3] or [1.5]", a.inner)
			}
			ratio = rv
		case a.hasMod:
			rv, ok := parseRatio(a.raw)
			if !ok {
				return Op{}, errNoMatch
			}
			ratio = rv
		case a.value == "auto":
			ratio = 0
		case a.value == "square":
			ratio = 1
		default:
			rv, ok := c.th.Aspect[a.value]
			if !ok {
				return Op{}, errNoMatch
			}
			ratio = rv
		}
		return op(func(e *env) { e.s.AspectRatio = ratio }), nil
	}, func(th *Theme) []string {
		return append([]string{"auto", "square", "3/2", "4/3", "16/9", "21/9"}, th.Keys(NamespaceAspect)...)
	})

	// container: width 100% and, from each breakpoint up, that max-width.
	r.add("container", false, func(c *pctx, a *arg) (Op, error) {
		if a.raw != "" {
			return Op{}, errNoMatch
		}
		var bps []float32
		for _, k := range c.th.Keys(NamespaceBreakpoint) {
			l := c.th.Breakpoints[k]
			bps = append(bps, l.Resolve(c.th.RootFontSize, c.th.RootFontSize))
		}
		slices.Sort(bps)
		return op(func(e *env) {
			e.s.Width = DimPct(100)
			for _, px := range bps {
				if e.viewport.W >= px {
					e.s.MaxWidth = DimPx(px)
				}
			}
		}), nil
	}, func(*Theme) []string { return []string{""} })

	// Screen-reader-only text: visually hidden but still laid out as a
	// 1px box.
	r.static("sr-only", op(func(e *env) {
		s := e.s
		s.Position = PositionAbsolute
		s.Width, s.Height = DimPx(1), DimPx(1)
		s.Padding = AllSides(LP{})
		s.Margin = AllSides(DimPx(-1))
		s.OverflowX, s.OverflowY = OverflowHidden, OverflowHidden
		s.Text.WhiteSpace = text.WhiteSpaceNowrap
		for _, b := range []*Border{&s.Border.Top, &s.Border.Right, &s.Border.Bottom, &s.Border.Left} {
			b.Width = 0
		}
	}))
	r.static("not-sr-only", op(func(e *env) {
		s := e.s
		s.Position = PositionStatic
		s.Width, s.Height = Auto, Auto
		s.Padding = AllSides(LP{})
		s.Margin = AllSides(Dim{Kind: DimLength})
		s.OverflowX, s.OverflowY = OverflowVisible, OverflowVisible
		s.Text.WhiteSpace = text.WhiteSpaceNormal
	}))

	// Padding and margin.
	for _, p := range sideRoots("p", "") {
		mask := p.mask
		r.dimUtil(p.root, dimSpacing, axisX, func(e *env, d Dim) { setSidesLP(&e.s.Padding, mask, d.LP) })
	}
	for _, p := range sideRoots("m", "") {
		mask := p.mask
		r.dimUtil(p.root, dimAuto|dimSpacing|dimNeg, axisX, func(e *env, d Dim) { setSidesDim(&e.s.Margin, mask, d) })
	}

	// Gap. space-x-* and space-y-* are implemented as gap (section 10).
	for _, g := range []struct {
		root  string
		x, y  bool
		space bool
	}{
		{"gap", true, true, false}, {"gap-x", true, false, false}, {"gap-y", false, true, false},
		{"space-x", true, false, true}, {"space-y", false, true, true},
	} {
		g := g
		r.dimUtil(g.root, dimSpacing, axisX, func(e *env, d Dim) {
			if g.x {
				e.s.ColumnGap, e.s.SpaceX = d.LP, g.space
			}
			if g.y {
				e.s.RowGap, e.s.SpaceY = d.LP, g.space
			}
		})
	}
}

// parseRatio parses "16/9", "16 / 9" or "1.5".
func parseRatio(s string) (float32, bool) {
	s = strings.ReplaceAll(s, " ", "")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		a, ok1 := parseFloat(s[:i])
		b, ok2 := parseFloat(s[i+1:])
		if !ok1 || !ok2 || a <= 0 || b <= 0 {
			return 0, false
		}
		return a / b, true
	}
	v, ok := parseFloat(s)
	return v, ok && v > 0
}

// registerSizing registers w, h, size and the min/max variants.
func registerSizing(r *registry) {
	wOpts := dimAuto | dimFull | dimFrac | dimSpacing | dimIntrinsic | dimContainer | dimScreen
	hOpts := dimAuto | dimFull | dimFrac | dimSpacing | dimIntrinsic | dimScreen
	r.dimUtil("w", wOpts, axisX, func(e *env, d Dim) { e.s.Width = d })
	r.dimUtil("h", hOpts, axisY, func(e *env, d Dim) { e.s.Height = d })
	r.dimUtil("size", dimAuto|dimFull|dimFrac|dimSpacing|dimIntrinsic|dimContainer, axisX,
		func(e *env, d Dim) { e.s.Width, e.s.Height = d, d })
	r.dimUtil("min-w", dimAuto|dimFull|dimFrac|dimSpacing|dimIntrinsic|dimContainer|dimScreen, axisX,
		func(e *env, d Dim) { e.s.MinWidth = d })
	r.dimUtil("min-h", dimAuto|dimFull|dimFrac|dimSpacing|dimIntrinsic|dimScreen, axisY,
		func(e *env, d Dim) { e.s.MinHeight = d })
	r.dimUtil("max-w", dimNone|dimFull|dimFrac|dimSpacing|dimIntrinsic|dimContainer|dimScreen, axisX,
		func(e *env, d Dim) { e.s.MaxWidth = d })
	r.dimUtil("max-h", dimNone|dimFull|dimFrac|dimSpacing|dimIntrinsic|dimScreen, axisY,
		func(e *env, d Dim) { e.s.MaxHeight = d })
}
