package style

import (
	"strconv"

	"github.com/kuronosu/tela/internal/text"
)

// The divide utilities (ADR-203) draw a line between the children of an
// element: divide-x and divide-y, their width (divide-x-2, divide-y-[3px]),
// divide-x-reverse and divide-y-reverse, the color (divide-border,
// divide-zinc-200/50) and the style (divide-dashed).
//
// Tailwind v4 compiles them to rules on the children, with the zero
// specificity of :where(), so any border class of a child wins:
//
//	:where(.divide-x > :not(:last-child)) {
//	  border-inline-style: var(--tw-border-style);
//	  border-inline-start-width: calc(1px * var(--tw-divide-x-reverse));
//	  border-inline-end-width: calc(1px * calc(1 - var(--tw-divide-x-reverse)));
//	}
//	:where(.divide-y > :not(:last-child)) {
//	  border-top-width: calc(1px * var(--tw-divide-y-reverse));
//	  border-bottom-width: calc(1px * calc(1 - var(--tw-divide-y-reverse)));
//	}
//	:where(.divide-red-500 > :not(:last-child)) { border-color: ... }
//	:where(.divide-dashed > :not(:last-child)) { border-style: dashed }
//
// (tailwindcss 4.3.3; v3 put border-top on every child but the first.)
//
// Tela has no selectors, so the classes of the parent compute a Divide in
// its style, and each child that is not the last applies it before its own
// classes (Compute), which is how a child's border classes win. The
// parent's style is the child's Input.Parent; the last child is
// StateLast. A parent restyles its children when its Divide changes, and
// the tree restyles them when its children change (core).

// divideAxis is one direction of the lines: its width in px, and whether
// the line moves to the other side of the child (divide-x-reverse).
type divideAxis struct {
	set     bool
	width   float32
	reverse bool
}

// Divide is the divide-* state of an element: how its children are drawn
// apart (ComputedStyle.Divide, nil when no class sets one). A child that
// is not the last child of the element starts from it, under its own
// classes.
type Divide struct {
	x, y     divideAxis
	style    BorderStyle
	hasStyle bool
	// color is not bound to a color scheme: a token resolves in the
	// child, like any border color.
	color colorSpec
}

// Equal reports whether d and o style the children the same way.
func (d *Divide) Equal(o *Divide) bool {
	if d == nil || o == nil {
		return d == o
	}
	return *d == *o
}

// result returns the divide of a computation (the scratch is a Divide
// too; the reverse flags are separate from the widths, like Tailwind's
// --tw-divide-*-reverse, so that their class order does not matter), or
// nil when no class draws anything.
func (d *Divide) result() *Divide {
	if !d.x.set && !d.y.set && !d.hasStyle && !d.color.set {
		return nil
	}
	r := *d
	return &r
}

// applyDivide starts the border of child e from the divide of its parent.
func (e *env) applyDivide(d *Divide) {
	b := borderSides(e.s) // top, right, bottom, left
	if d.hasStyle {
		for _, s := range b {
			s.Style = d.style
		}
	}
	// Width: the reverse flag swaps the side of the line (start width
	// is w * reverse, end width w * (1 - reverse)).
	pair := func(a divideAxis, start, end *Border) {
		if !a.set {
			return
		}
		start.Width, end.Width = 0, 0
		if a.reverse {
			start.Width = a.width
		} else {
			end.Width = a.width
		}
	}
	if e.s.Text.Direction == text.DirectionRTL {
		pair(d.x, b[1], b[3])
	} else {
		pair(d.x, b[3], b[1])
	}
	pair(d.y, b[0], b[2])
	if d.color.set {
		for slot := slotBorderTop; slot <= slotBorderLeft; slot++ {
			e.sc.colors[slot] = d.color
		}
	}
}

var divideStyles = map[string]BorderStyle{
	"solid": BorderSolid, "dashed": BorderDashed, "dotted": BorderDotted,
	"double": BorderDouble, "none": BorderNone,
}

func registerDivide(r *registry) {
	for _, ax := range []struct {
		root string
		y    bool
	}{{"divide-x", false}, {"divide-y", true}} {
		root, y := ax.root, ax.y
		axis := func(s *Divide) *divideAxis {
			if y {
				return &s.y
			}
			return &s.x
		}
		r.add(root, false, func(c *pctx, a *arg) (Op, error) {
			if a.hasMod {
				return Op{}, errNoMatch
			}
			set := func(w float32) Op {
				return op(func(e *env) { ax := axis(&e.sc.divide); ax.set, ax.width = true, w })
			}
			switch {
			case a.raw == "":
				return set(1), nil
			case a.arb:
				if a.hint != "" && a.hint != "length" && a.hint != "line-width" {
					return Op{}, errNoMatch
				}
				if a.hint == "" && looksLikeColor(a.inner) {
					return Op{}, errNoMatch
				}
				ex, err := parseLengthExpr(a.inner, false)
				if err != nil {
					if err == errNotLength {
						return Op{}, errNoMatch
					}
					return Op{}, err
				}
				return op(func(e *env) { ax := axis(&e.sc.divide); ax.set, ax.width = true, e.lp(&ex).Px }), nil
			case a.value == "reverse":
				return op(func(e *env) { axis(&e.sc.divide).reverse = true }), nil
			}
			// Tailwind takes whole numbers only: divide-x-2, not divide-x-2.5.
			n, err := strconv.Atoi(a.value)
			if err != nil || n < 0 || a.value[0] == '+' {
				return Op{}, errNoMatch
			}
			return set(float32(n)), nil
		}, func(*Theme) []string { return []string{"", "0", "1", "2", "4", "8", "reverse"} })
	}

	// divide-solid, divide-dashed...: the style of the lines.
	keys := map[string]Op{}
	for k, st := range divideStyles {
		keys[k] = op(func(e *env) { d := &e.sc.divide; d.style, d.hasStyle = st, true })
	}
	r.enum("divide", false, keys)

	// divide-border, divide-red-500/50, divide-[#123]: the color.
	r.add("divide", false, func(c *pctx, a *arg) (Op, error) {
		if a.raw == "" {
			return Op{}, errNoMatch
		}
		spec, err := colorArg(c, a)
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) { e.sc.divide.color = spec }), nil
	}, colorKeys)
}
