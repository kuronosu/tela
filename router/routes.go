package router

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// matchSet is the routes the current location matched, root first, and the
// parameters they got. It is immutable once built.
type matchSet struct {
	matches []match
	params  Params
}

// routeCtx is where a component is in the matched routes: the set, and the
// index of the route whose element it belongs to.
type routeCtx struct {
	set   *matchSet
	level int
}

var routeContext = tela.CreateContext(routeCtx{})

// Routes renders the element of the route that matches the current
// location, as React Router's <Routes>. The routes are ranked, not tried in
// order: static segments beat parameters, parameters beat a splat ("*"),
// and longer paths beat shorter ones.
//
//	router.Routes(
//	    router.Route{Path: "/", Element: Layout{}, Children: []router.Route{
//	        {Index: true, Element: Home{}},
//	        {Path: "notes/:id", Element: Note{}},
//	    }},
//	    router.Route{Path: "*", Element: NotFound{}},
//	)
//
// A route's element renders inside a [h.Presence] keyed by the route, so
// the screen that a navigation leaves exits through its Exit classes
// (and stays, inert, until they end), and elements with a [h.LayoutID]
// animate to their place in the new screen. The same route with other
// parameters keeps its element, which updates in place.
//
// A Routes inside the element of a route matches the rest of the path,
// after what that route matched (React Router's descendant routes).
// Nothing renders when no route matches: end the table with Path: "*".
func Routes(routes ...Route) tela.Node {
	if len(routes) == 0 {
		panic("router.Routes(): pass at least one router.Route, such as router.Routes(router.Route{Path: \"/\", Element: Home{}})")
	}
	return routesView{routes: routes}
}

type routesView struct{ routes []Route }

func (v routesView) Build() tela.Node {
	rs := useRouter("router.Routes")
	parent := tela.UseContext(routeContext)

	base, skip := parent.set.chain(parent.level)
	segs := pathSegments(rs.loc.Path)
	ms, params := matchRoutes(v.routes, "/"+strings.Join(segs[min(skip, len(segs)):], "/"))
	if len(ms) == 0 {
		return h.Presence() // what was on screen exits
	}
	set := &matchSet{params: Params{}}
	set.matches = append(set.matches, base...)
	for _, m := range ms {
		// The nested table's paths count from where its parent's ended.
		m.consumed += skip
		m.base = "/" + strings.Join(segs[:m.consumed], "/")
		m.id = nestedID(parent, m.id)
		set.matches = append(set.matches, m)
	}
	if parent.set != nil {
		for k, v := range parent.set.params {
			set.params[k] = v
		}
	}
	for k, v := range params {
		set.params[k] = v
	}
	return renderLevel(set, len(base))
}

// chain returns the matches from the root to level (for a nested Routes)
// and how many segments they matched; both are empty for the top table.
func (s *matchSet) chain(level int) ([]match, int) {
	if s == nil || level >= len(s.matches) {
		return nil, 0
	}
	return s.matches[:level+1], s.matches[level].consumed
}

// nestedID prefixes the ids of a nested table's routes with the parent's,
// so the keys stay unique along the chain.
func nestedID(parent routeCtx, id string) string {
	if parent.set == nil {
		return id
	}
	return parent.set.matches[parent.level].id + "|" + id
}

// renderLevel renders the element of the match at level: in a Presence
// child keyed by the route, with the context of what it renders.
func renderLevel(set *matchSet, level int) tela.Node {
	m := set.matches[level]
	var body tela.Node = m.route.Element
	if body == nil {
		body = Outlet()
	}
	return h.Presence(tela.WithKey(routeContext.Provider(routeCtx{set: set, level: level}, body), m.id))
}

// Outlet renders the child route that matched, inside the element of a
// route with Children (React Router's <Outlet />): the layout puts it
// where the screen goes. It renders nothing when no child matched, and
// the screen that was there exits.
func Outlet() tela.Node { return outlet{} }

type outlet struct{}

func (outlet) Build() tela.Node {
	rc := tela.UseContext(routeContext)
	if rc.set == nil {
		panic("router.Outlet: this component is not in the element of a route; use Outlet in the Element of a router.Route that has Children, rendered by router.Routes")
	}
	if rc.level+1 >= len(rc.set.matches) {
		return h.Presence()
	}
	return renderLevel(rc.set, rc.level+1)
}

// UseParams returns the parameters the current location gave to the
// routes above the component (and below it: React Router shares them all),
// such as {"id": "3"} for "/notes/3" matched by "/notes/:id", and the
// splat under "*". Outside a route it is empty. Do not modify the map.
func UseParams() Params {
	rc := tela.UseContext(routeContext)
	if rc.set == nil {
		return Params{}
	}
	return rc.set.params
}

// UseLocation returns the current location: its path, query and fragment.
// It must be called below a [Router].
func UseLocation() Location {
	return useRouter("router.UseLocation").loc
}

// Navigator changes the history of its [Router]. It is comparable and
// stable between builds while the location does not change, so it can be a
// prop or a dependency. UseNavigate returns it.
type Navigator struct {
	set tela.Setter[history]
	// chain are the pathnames relative paths resolve from, joined by NUL.
	chain         string
	loc           Location
	back, forward bool
}

// UseNavigate returns the [Navigator] of the Router above, for handlers and
// effects:
//
//	nav := router.UseNavigate()
//	h.Button(h.OnClick(func() { nav.To("/notes/new") }), h.Text("New"))
//
// Relative paths ("edit", "../other") resolve from the route the component
// is in, where ".." goes up one route, not one segment, as in React
// Router. Calling it outside a Router panics.
func UseNavigate() Navigator {
	rs := useRouter("router.UseNavigate")
	rc := tela.UseContext(routeContext)
	var chain string
	if rc.set != nil {
		chain = strings.Join(routePathnames(rc.set.matches, rc.level), "\x00")
	}
	return Navigator{set: rs.set, chain: chain, loc: rs.loc, back: rs.back, forward: rs.forward}
}

func (n Navigator) check(who string) {
	if n.set.IsZero() {
		panic("router." + who + ": the zero Navigator goes nowhere; get one with router.UseNavigate() inside a router.Router")
	}
}

// resolve resolves a path against the route the Navigator was made in and
// the location now.
func (n Navigator) resolve(to string, cur Location) Location {
	var pathnames []string
	if n.chain != "" {
		pathnames = strings.Split(n.chain, "\x00")
	}
	return resolveTo(to, pathnames, cur)
}

// To goes to path, a new entry of the history, dropping the forward
// entries. path may be absolute ("/notes/3"), relative to the route
// ("edit", "../4") or only a query or fragment ("?page=2", "#top"); see
// [UseNavigate].
func (n Navigator) To(path string) {
	n.check("Navigator.To")
	n.set.Update(func(h history) history { return h.push(n.resolve(path, h.current())) })
}

// Replace goes to path like [Navigator.To], but replaces the current entry,
// so Back skips it (a redirect after a sign-in).
func (n Navigator) Replace(path string) {
	n.check("Navigator.Replace")
	n.set.Update(func(h history) history { return h.replace(n.resolve(path, h.current())) })
}

// Back goes to the previous entry; nothing happens at the first one.
func (n Navigator) Back() {
	n.check("Navigator.Back")
	n.set.Update(func(h history) history { return h.move(-1) })
}

// Forward goes to the next entry; nothing happens at the last one.
func (n Navigator) Forward() {
	n.check("Navigator.Forward")
	n.set.Update(func(h history) history { return h.move(1) })
}

// CanGoBack reports whether there is an entry before the current one, as of
// the build that made the Navigator.
func (n Navigator) CanGoBack() bool { return n.back }

// CanGoForward reports whether there is an entry after the current one, as
// of the build that made the Navigator.
func (n Navigator) CanGoForward() bool { return n.forward }
