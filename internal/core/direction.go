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
//
// Components read the direction at their position with UseDirection
// (ADR-155), as UseColorScheme reads the scheme: the Dir of the nearest
// primitive ancestor that has one, dir="auto" as the style phase resolved
// it. Their slots (dirSlot) keep what the build returned, so that a Dir
// change, or an auto element resolving the other way, builds again only
// the readers that saw another direction.

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
		panic(dirMessage(dir))
	}
	return Prop{name: "Dir", apply: func(d *desc) { d.dir = v }}
}

// dirMessage is the dev-mode message of an invalid Dir value (CheckDir).
func dirMessage(dir string) string {
	return fmt.Sprintf("tela: Dir(%q): the direction is \"rtl\", \"ltr\" or \"auto\"", dir)
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
		prev, had := t.autoDirs[el]
		t.autoDirs[el] = rtl
		el.autoStale = false
		if t.dirHooks && (!had || prev != rtl) {
			// UseDirection readers below built with the previous result,
			// or with ltr before the first one (dirAt).
			t.markDirReaders(el, rtl)
		}
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
	t.autoWalks++
	d, _ := t.firstStrongBelow(el)
	return d == text.DirectionRTL
}

// firstStrongBelow returns the direction of the first strong character of
// the text below el (autoRTL's walk). A TextFn whose Values changed is
// evaluated first (textOf), as the paragraph will be.
func (t *Tree) firstStrongBelow(el *Element) (text.Direction, bool) {
	for _, c := range el.children {
		switch c.kind {
		case elemText:
			if d, ok := text.StrongDirection(t.textOf(c)); ok {
				return d, true
			}
		case elemPrimitive:
			if c.node.d.dir != dirNone || c.box.fld != nil {
				continue
			}
			if d, ok := t.firstStrongBelow(c); ok {
				return d, true
			}
		case elemComponent:
			if d, ok := t.firstStrongBelow(c); ok {
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

// markAutoDir records that the text below from changed (a text changed,
// children were added, removed or moved, or a child's Dir changed): the
// nearest primitive at or above from with a Dir of its own, if its Dir is
// "auto", is marked autoStale, so that the next style phase resolves its
// direction again. A primitive with another Dir hides the change from the
// elements above it, as firstStrongBelow skips its subtree.
func (t *Tree) markAutoDir(from *Element) {
	if len(t.autoDirs) == 0 {
		return
	}
	for p := from; p != nil; p = p.parent {
		if p.kind != elemPrimitive || p.node.d.dir == dirNone {
			continue
		}
		if p.node.d.dir == dirAuto && !p.autoStale {
			p.autoStale, t.autoStale = true, true
		}
		return
	}
}

// checkAutoDirs restyles the elements with dir="auto" marked autoStale
// whose text now starts in the other direction, and forgets the ones that
// are gone or no longer auto. It runs at the start of the style phase when
// one is marked, so that a frame which changed no text below them (a
// hover, an animation) walks no text.
func (t *Tree) checkAutoDirs() {
	t.autoStale = false
	for el, rtl := range t.autoDirs {
		if !el.mounted || el.node.d.dir != dirAuto {
			delete(t.autoDirs, el)
			continue
		}
		if !el.autoStale {
			continue
		}
		el.autoStale = false
		if !el.needsStyle && t.autoRTL(el) != rtl {
			t.markStyle(el)
		}
	}
}

// ---------------------------------------------------------------------
// UseDirection (ADR-155).

// dirSlot is the hook slot of UseDirection: it marks the component as a
// reader of the direction at its position and holds the direction its
// last build returned.
type dirSlot struct{ rtl bool }

// UseDirection returns the direction at the component's position, "ltr"
// or "rtl": the Dir prop of its nearest primitive ancestor that has one,
// with "auto" resolved as the style phase resolves it, else "ltr". The
// component builds again when it changes.
func UseDirection() string {
	el, i := nextSlot(hookDirection)
	if i == len(el.hooks) {
		el.hooks = append(el.hooks, &dirSlot{})
		el.tree.dirHooks = true
	}
	s := el.hooks[i].(*dirSlot)
	s.rtl = el.tree.dirAt(el.parent)
	if s.rtl {
		return "rtl"
	}
	return "ltr"
}

// dirAt reports whether the direction at el, a primitive or a position
// below it, is right to left: the Dir of the nearest primitive at or
// above el that has one. A dir="auto" element gives the direction its
// last style computation resolved; before its first one (it mounts in
// this build, and its text may come from the components being built)
// it gives ltr, HTML's direction of a text without strong characters,
// and the style phase builds the readers again in the same frame when it
// resolves to rtl (markDirReaders, Tree.Frame).
func (t *Tree) dirAt(el *Element) bool {
	for p := el; p != nil; p = p.parent {
		if p.kind != elemPrimitive || p.node.d == nil {
			continue
		}
		switch p.node.d.dir {
		case dirLTR:
			return false
		case dirRTL:
			return true
		case dirAuto:
			return t.autoDirs[p]
		}
	}
	return false
}

// markDirReaders builds again the components below el whose nearest
// primitive with a Dir is el (or, when el has none, that have no Dir
// between them and el) and whose UseDirection did not return the
// direction rtl gives. A subtree with a Dir of its own is skipped: its
// readers do not depend on el. Called in the style phase (dir="auto"
// resolved), it asks the frame to build again (dirRebuild).
func (t *Tree) markDirReaders(el *Element, rtl bool) {
	var walk func(e *Element)
	walk = func(e *Element) {
		for _, c := range e.children {
			switch c.kind {
			case elemPrimitive:
				if c.node.d.dir != dirNone {
					continue
				}
			case elemComponent:
				for _, h := range c.hooks {
					if s, ok := h.(*dirSlot); ok && s.rtl != rtl {
						t.markBuild(c)
						if t.inStyle {
							t.dirRebuild = true
						}
						break
					}
				}
			default:
				continue
			}
			walk(c)
		}
	}
	walk(el)
}

// dirChanged follows a change of el's Dir prop (update, during the build):
// the readers below it that read another direction build again.
func (t *Tree) dirChanged(el *Element) {
	if t.dirHooks {
		t.markDirReaders(el, t.dirAt(el))
	}
}
