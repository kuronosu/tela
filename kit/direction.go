package kit

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// directionCtx carries the direction of a DirectionProvider ("ltr", "rtl",
// or "" under none) to the components below it.
var directionCtx = tela.CreateContext("")

// DirectionProvider sets the writing direction of a subtree for the layout
// and for the components whose keys and drags depend on it, as Radix's
// DirectionProvider does: Direction is "rtl" or "ltr" (any case, so
// i18n.UseFormatter().Dir().String() works).
//
//	f := i18n.UseFormatter()
//	return kit.DirectionProvider{Direction: f.Dir().String(), Children: App{}}
//
// It lays its children out with h.Dir (it has no box of its own: the
// children are laid out as if they were its parent's), so the layout
// mirrors and the logical utilities of the kit's recipes (ps-*, pe-*, start-*,
// end-*, rounded-s-*, the rtl: variants) follow. The components that move
// with the arrow keys or with a drag along a line (Slider, Tabs, ToggleGroup,
// RadioGroup, Menubar, ColorPicker, and the calendars) read it as well: in
// a right-to-left subtree ArrowLeft goes to the next item and a slider's
// minimum is at the right. A bare h.Dir("rtl") above them mirrors the layout
// and the recipes but not those keys, because a component cannot read its
// direction while it builds; use the provider, or the component's Direction
// field, for them.
type DirectionProvider struct {
	// Direction is "ltr" or "rtl".
	Direction string
	// Children is the subtree.
	Children tela.Component
}

// Build lays the children out in the direction and provides it.
func (p DirectionProvider) Build() tela.Node {
	d := checkDirection("DirectionProvider", p.Direction)
	kids := []tela.Child{h.Class("contents"), h.Dir(d)}
	if p.Children == nil {
		return h.Div(append(kids, directionCtx.Provider(d))...)
	}
	return h.Div(append(kids, directionCtx.Provider(d, p.Children))...)
}

// checkDirection returns the direction a Direction field names, in lower
// case: "ltr" or "rtl". Another value panics in dev mode and is "ltr"
// otherwise, as aria's hooks do.
func checkDirection(component, d string) string {
	switch l := strings.ToLower(d); l {
	case "ltr", "rtl":
		return l
	default:
		if devMode() {
			panic(fmt.Sprintf("kit: %s.Direction %q is not a direction: use \"ltr\" or \"rtl\"", component, d))
		}
		return "ltr"
	}
}

// useDirection is the direction of a component, a hook: its Direction field
// when given, else the nearest [DirectionProvider]'s, else "" (left to
// right, the direction of an h.Dir the component cannot see). rootDir is
// the h.Dir prop the component adds to its root when its own field set the
// direction, so that its layout follows what its keys do (nil otherwise).
func useDirection(component, field string) (dir string, rootDir tela.Child) {
	provided := tela.UseContext(directionCtx)
	if field == "" {
		return provided, nil
	}
	dir = checkDirection(component, field)
	return dir, h.Dir(dir)
}
