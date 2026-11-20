package router

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// Link is a link that navigates the [Router] when it is activated, by a
// click or by Enter with the focus on it, as React Router's <Link>. It is
// an A element, so it is a "link" for the accessibility tree and the
// pointer shows the hand over it, and a tab stop with a focus ring (the
// current text color; add focus-visible: classes in Class for another).
// Its Href is the resolved path, and it is the current page
// (aria-current="page") when that is the location's path.
//
//	router.Link{To: "/notes/3", Class: "underline", Children: h.Text("Groceries")}
//
// To resolves like [Navigator.To]: relative to the route the link is in.
type Link struct {
	// To is the destination: "/notes/3", "edit", "../4", "?page=2".
	To string
	// Replace replaces the current history entry instead of adding one.
	Replace bool
	// Class adds Tailwind classes to the element.
	Class string
	// Children are the contents of the link.
	Children tela.Node
}

// linkClass is the base look: a ring around the link when the keyboard
// focuses it, in the color of its text.
const linkClass = "rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-current focus-visible:ring-offset-2"

// Build renders the element.
func (l Link) Build() tela.Node {
	nav := UseNavigate()
	target := nav.resolve(l.To, nav.loc)
	activate := func() {
		nav.set.Update(func(hs history) history {
			loc := nav.resolve(l.To, hs.current())
			if l.Replace {
				return hs.replace(loc)
			}
			return goTo(hs, loc)
		})
	}
	current := ""
	if target.Path == nav.loc.Path {
		current = "page"
	}
	return h.A(h.Href(target.String()), h.TabIndex(0), h.Class(linkClass, l.Class), h.AriaCurrent(current),
		h.OnClick(activate),
		h.OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Key == tela.KeyEnter && !e.Repeat && !e.Ctrl && !e.Alt && !e.Meta {
				e.PreventDefault()
				activate()
			}
		}),
		l.Children)
}
