package tela

import "github.com/kuronosu/tela/internal/core"

// UseDirection returns the writing direction at the component's position,
// "ltr" or "rtl": the one the nearest element above it with an h.Dir prop
// gives, else "ltr". Under h.Dir("auto") it is the direction that element
// resolves from its text; the first build of a component below an auto
// element that has not been styled yet reads "ltr", and when the element
// resolves to "rtl" the component builds again before the frame is laid
// out, so the wrong direction never reaches the screen. Content of an
// h.Portal follows the position where the Portal is declared. The
// component builds again when the direction changes (a Dir prop above it,
// or the text of an auto element), like with a state hook; nothing else
// does. It is a hook: call it at the top level of a component.
//
// Classes do not need it: the logical utilities (ms-*, ps-*, start-*,
// rounded-s-*...) and the rtl: and ltr: variants already follow the
// direction. Use it for what classes cannot do, such as the arrow keys
// or the drag of a slider:
//
//	func Stepper() tela.Node {
//	    next := "ArrowRight"
//	    if tela.UseDirection() == "rtl" {
//	        next = "ArrowLeft"
//	    }
//	    ...
//	}
//
// It is a string, the vocabulary of h.Dir and of Radix's useDirection, so
// the value passes as it is to h.Dir and to the Direction fields of kit
// and aria.
func UseDirection() string { return core.UseDirection() }
