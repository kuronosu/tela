package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// Inert makes an element and its subtree inert when true, like HTML's
// inert attribute: they are not hit by the pointer (a press goes to what
// is under them), cannot take the focus (a focused element in them loses
// it) and leave the Tab order, and assistive technologies, telatest's
// locators and the aria snapshot do not see them. They are still painted:
// dim them with classes if they should look disabled. Content rendered
// from inside the element through a Portal is inert too.
//
//	cls := "space-y-4"
//	if saving {
//	    cls += " opacity-50"
//	}
//	Div(Inert(saving), Class(cls), fields)
func Inert(inert bool) tela.Prop { return core.Inert(inert) }
