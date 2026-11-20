package router

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// history is the stack of visited locations and where in it the user is:
// the memory history of React Router. Its fields are exported and plain, so
// `tela dev` keeps it across reloads (ADR-113).
type history struct {
	// Entries are the locations as "path?query#hash", oldest first.
	Entries []string
	// Index is the current entry.
	Index int
}

func newHistory(initial string) history {
	if initial == "" {
		initial = "/"
	}
	return history{Entries: []string{parseLocation(initial).absolute().String()}}
}

func (h history) current() Location { return parseLocation(h.Entries[h.Index]) }

// push goes to loc, dropping the entries after the current one (the
// forward history), as a browser does.
func (h history) push(loc Location) history {
	es := append(h.Entries[:h.Index+1:h.Index+1], loc.String())
	return history{Entries: es, Index: len(es) - 1}
}

// replace swaps the current entry for loc.
func (h history) replace(loc Location) history {
	es := append([]string(nil), h.Entries...)
	es[h.Index] = loc.String()
	return history{Entries: es, Index: h.Index}
}

// move goes delta entries, staying in the stack.
func (h history) move(delta int) history {
	i := h.Index + delta
	if i < 0 || i >= len(h.Entries) {
		return h
	}
	return history{Entries: h.Entries, Index: i}
}

// routerState is what a Router gives to the components below it. Its
// fields are unexported: reading it without a Router is the mistake
// useRouter reports, not a default to use.
type routerState struct {
	ok      bool
	loc     Location
	set     tela.Setter[history]
	back    bool
	forward bool
}

var routerCtx = tela.CreateContext(routerState{})

// useRouter returns the state of the nearest Router, panicking with the fix
// when there is none.
func useRouter(who string) routerState {
	rs := tela.UseContext(routerCtx)
	if !rs.ok {
		panic(who + ": there is no router.Router above this component; wrap the app in one, such as tela.App{Root: router.Router{Children: router.Routes(...)}}")
	}
	return rs
}

// Router holds the navigation history of an app, in memory, and gives it
// to the components below: [Routes] renders the screen of the current
// location, [Link] and [UseNavigate] change it, and [UseLocation] and
// [UseParams] read it. The history is hook state, so `tela dev` keeps it
// across reloads.
//
// Inside a Router the mouse's back and forward buttons and Alt+Left and
// Alt+Right (Command+[ and Command+] with the Mac flavor of key bindings)
// go back and forward, as in Chromium: unless a handler stopped or
// prevented the event, or a text field of the window took the key.
//
//	router.Router{Children: router.Routes(
//	    router.Route{Path: "/", Element: Home{}},
//	    router.Route{Path: "/notes/:id", Element: Note{}},
//	    router.Route{Path: "*", Element: NotFound{}},
//	)}
type Router struct {
	// Initial is the location the history starts at, "/" by default. It is
	// read once, when the Router mounts.
	Initial string
	// Scheme, such as "notes", makes the app's links of that scheme (what
	// desktop.UseLaunch gives: "notes://notes/3", "notes:/notes/3",
	// "notes:notes/3?pin=1") open their path as a navigation ("/notes/3").
	// The OS must send them to the app: see desktop.Register. Links of other
	// schemes are left to the app's own UseLaunch.
	Scheme string
	// Children is the app, usually [Routes] or a layout around it.
	Children tela.Node
}

// Build holds the history and gives it to the children.
func (r Router) Build() tela.Node {
	flavor := core.UseFlavor()
	hist, set := tela.UseState(newHistory(r.Initial))
	back, forward := hist.Index > 0, hist.Index < len(hist.Entries)-1

	core.UseUnhandledKeyDown(func(e tela.KeyboardEvent) bool {
		delta := keyDelta(flavor, e)
		if delta == 0 || e.Repeat {
			return false
		}
		return step(set, delta, back, forward)
	})
	core.UseUnhandledMouseButton(func(b tela.MouseButton) bool {
		switch b {
		case tela.MouseButtonBack:
			return step(set, -1, back, forward)
		case tela.MouseButtonForward:
			return step(set, 1, back, forward)
		}
		return false
	})

	state := routerState{ok: true, loc: hist.current(), set: set, back: back, forward: forward}
	if r.Scheme != "" {
		return routerCtx.Provider(state, r.Children, deepLinks{Scheme: r.Scheme})
	}
	return routerCtx.Provider(state, r.Children)
}

// step goes delta entries when the history has them, and reports whether it
// could (an event the router used is not left to the app).
func step(set tela.Setter[history], delta int, back, forward bool) bool {
	if delta < 0 && !back || delta > 0 && !forward {
		return false
	}
	set.Update(func(h history) history { return h.move(delta) })
	return true
}

// keyDelta returns -1 for the key that goes back, 1 for forward and 0 for
// any other, with the bindings of flavor fl: Alt+Left and Alt+Right of
// Chromium on Windows and Linux, Command+[ and Command+] on a Mac. Other
// modifiers make it another shortcut.
func keyDelta(fl platform.Flavor, e tela.KeyboardEvent) int {
	if e.Shift {
		return 0
	}
	if fl == platform.FlavorMac {
		if !e.Meta || e.Ctrl || e.Alt {
			return 0
		}
		switch {
		case e.Key == "[" || e.Code == tela.CodeBracketLeft:
			return -1
		case e.Key == "]" || e.Code == tela.CodeBracketRight:
			return 1
		}
		return 0
	}
	if !e.Alt || e.Ctrl || e.Meta {
		return 0
	}
	switch e.Key {
	case tela.KeyArrowLeft:
		return -1
	case tela.KeyArrowRight:
		return 1
	}
	return 0
}

// deepLinks opens the links of the Router's Scheme that the app is
// launched with. It is a component of its own so that an app without a
// Scheme has no UseLaunch of the router, which would take the start
// launch from its own handlers that mount later.
type deepLinks struct{ Scheme string }

func (d deepLinks) Build() tela.Node {
	rs := useRouter("router.Router")
	desktop.UseLaunch(func(l desktop.Launch) {
		for _, u := range l.URLs() {
			if loc, ok := deepLinkLocation(d.Scheme, u); ok {
				rs.set.Update(func(h history) history { return goTo(h, loc) })
			}
		}
	})
	return nil
}

// goTo pushes loc, or replaces the current entry when it is the same
// location, as a click on the link of the page one is on does.
func goTo(h history, loc Location) history {
	if h.current() == loc {
		return h.replace(loc)
	}
	return h.push(loc)
}

// deepLinkLocation returns the location a link of scheme names:
// "notes://notes/3", "notes:/notes/3" and "notes:notes/3" are "/notes/3",
// and "notes:open?id=3" is "/open?id=3". The scheme is matched without
// regard to case.
func deepLinkLocation(scheme, link string) (Location, bool) {
	n := len(scheme) + 1
	if len(link) < n || link[n-1] != ':' || !strings.EqualFold(link[:n-1], scheme) {
		return Location{}, false
	}
	return parseLocation("/" + strings.TrimLeft(link[n:], "/")), true
}
