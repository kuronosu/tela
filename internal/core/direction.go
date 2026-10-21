package core

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Direction (ADR-090). An element's direction comes from its Dir prop,
// HTML's dir attribute, and is inherited (CSS direction, which Tailwind
// has no utility for): style.Compute resolves it into
// ComputedStyle.Text.Direction before the classes run, so the rtl: and
// ltr: variants and the logical utilities see it. Layout mirrors flex,
// grid, block and positioned layout with it (layout.Style.Direction) and
// paragraphs take it as their base direction.

// dirValue is the parsed Dir prop.
type dirValue uint8

const (
	dirNone dirValue = iota // no Dir prop, or an empty or invalid one: inherit
	dirLTR
	dirRTL
	dirAuto
)

// parseDir parses an HTML dir value: ltr, rtl or auto, ASCII
// case-insensitive.
func parseDir(v string) (dirValue, bool) {
	switch strings.ToLower(v) {
	case "ltr":
		return dirLTR, true
	case "rtl":
		return dirRTL, true
	case "auto":
		return dirAuto, true
	case "":
		return dirNone, true
	}
	return dirNone, false
}

// Dir sets the direction of a primitive and its subtree, like HTML's dir
// attribute: "rtl" lays it out right to left (flex rows, grid columns,
// text alignment and the logical utilities ms-*, ps-*, start-*,
// border-s-*, rounded-s-* mirror; rtl: variants apply), "ltr" left to
// right, and "auto" takes the direction of the first strong character
// of the element's text (its value for an Input or Textarea), left to
// right when there is none. The value is case-insensitive, and "" has no
// effect (the element inherits its parent's direction, left to right at
// the root). In dev mode any other value panics; otherwise it is ignored,
// as HTML ignores an invalid dir.
func Dir(dir string) Prop {
	v, ok := parseDir(dir)
	if !ok && devNow() {
		panic(fmt.Sprintf("tela: Dir(%q): the direction is \"rtl\", \"ltr\" or \"auto\"", dir))
	}
	return Prop{name: "Dir", apply: func(d *desc) { d.dir = v }}
}

// styleDir returns the direction el sets for its style computation,
// resolving dir="auto" from its text (and recording it, so that a change
// of the text restyles el: checkAutoDirs).
func (t *Tree) styleDir(el *Element) style.Dir {
	switch el.node.d.dir {
	case dirLTR:
		return style.DirLTR
	case dirRTL:
		return style.DirRTL
	case dirAuto:
		rtl := t.autoRTL(el)
		if t.autoDirs == nil {
			t.autoDirs = map[*Element]bool{}
		}
		t.autoDirs[el] = rtl
		if rtl {
			return style.DirRTL
		}
		return style.DirLTR
	}
	return style.DirInherit
}

// autoRTL reports whether the text of el, which has dir="auto", starts
// with a right-to-left strong character (HTML's auto directionality): the
// value of a text field, else the text below el in tree order, skipping
// the subtrees of elements with a Dir of their own and of text fields.
func (t *Tree) autoRTL(el *Element) bool {
	if f := el.box.fld; f != nil {
		d, _ := text.StrongDirection(f.ed.Value())
		return d == text.DirectionRTL
	}
	d, _ := firstStrongBelow(el)
	return d == text.DirectionRTL
}

// firstStrongBelow returns the direction of the first strong character of
// the text below el (autoRTL's walk).
func firstStrongBelow(el *Element) (text.Direction, bool) {
	for _, c := range el.children {
		switch c.kind {
		case elemText:
			if d, ok := text.StrongDirection(c.text); ok {
				return d, true
			}
		case elemPrimitive:
			if c.node.d.dir != dirNone || c.box.fld != nil {
				continue
			}
			if d, ok := firstStrongBelow(c); ok {
				return d, true
			}
		case elemComponent:
			if d, ok := firstStrongBelow(c); ok {
				return d, true
			}
		}
	}
	return text.DirectionLTR, false
}

// autoDirChanged restyles el when it has dir="auto" and its text now
// starts in the other direction (a field's value changed: typing does not
// build or restyle).
func (t *Tree) autoDirChanged(el *Element) {
	if rtl, ok := t.autoDirs[el]; ok && !el.needsStyle && t.autoRTL(el) != rtl {
		t.markStyle(el)
	}
}

// checkAutoDirs restyles the elements with dir="auto" whose text now
// starts in the other direction (a build changed the text below them, or
// the user typed in a field), and forgets the ones that are gone or no
// longer auto. It runs at the start of the style phase.
func (t *Tree) checkAutoDirs() {
	for el, rtl := range t.autoDirs {
		if !el.mounted || el.node.d.dir != dirAuto {
			delete(t.autoDirs, el)
			continue
		}
		if !el.needsStyle && t.autoRTL(el) != rtl {
			t.markStyle(el)
		}
	}
}
