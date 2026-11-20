# Routing

Package `router` moves an app between screens, with React Router's names and a
memory history (ADR-298): a stack of locations with back and forward, no URL
bar. It is its own package, so an app with one screen does not import it.

```go
import "github.com/kuronosu/tela/router"
```

- [A first app](#a-first-app)
- [Routes](#routes)
- [Layouts and outlets](#layouts-and-outlets)
- [Links and navigation](#links-and-navigation)
- [Params, location and the query](#params-location-and-the-query)
- [Back and forward from the mouse and the keyboard](#back-and-forward-from-the-mouse-and-the-keyboard)
- [Screens that come and go](#screens-that-come-and-go)
- [Links from outside the app](#links-from-outside-the-app)
- [History and `tela dev`](#history-and-tela-dev)
- [Testing](#testing)
- [Not covered](#not-covered)

## A first app

```go
func App() tela.Node {
    return router.Router{Children: Div(Class("flex h-full flex-col"),
        Div(Class("flex gap-4 p-3"),
            router.Link{To: "/", Children: Text("Home")},
            router.Link{To: "/notes", Children: Text("Notes")}),
        Div(Class("relative flex-1"),
            router.Routes(
                router.Route{Path: "/", Element: tela.C(Home)},
                router.Route{Path: "/notes", Element: tela.C(NotesLayout), Children: []router.Route{
                    {Index: true, Element: tela.C(NoteList)},
                    {Path: ":id", Element: tela.C(Note)},
                }},
                router.Route{Path: "*", Element: tela.C(NotFound)},
            )))}
}
```

`router.Router` holds the history and gives it to everything below it;
`Initial` is the first location ("/" by default). `router.Routes` renders the
element of the route that matches the current location. Components below the
Router read and change the location with `UseNavigate`, `UseLocation` and
`UseParams`; using them outside a Router panics with the fix.

## Routes

A `router.Route` has a `Path`, an `Element` and optional `Children`. A path is
made of segments:

| Segment | Example | Matches |
|---|---|---|
| static | `notes` | that text, without regard to case |
| parameter | `:id` | one segment of any text; `UseParams()["id"]` |
| splat | `files/*` | the rest of the path; `UseParams()["*"]` |

The routes that match are **ranked**, not tried in order, as in React Router 6:
static segments beat parameters, parameters beat a splat, and longer paths beat
shorter ones. `/notes/new` beats `/notes/:id` wherever it is in the table, and a
route with `Path: "*"` is the fallback that catches what nothing else does.
Trailing slashes do not matter, and parameter values are percent-decoded.
Nothing renders when no route matches, so end the table with `"*"`.

## Layouts and outlets

Routes nest. A child's path is relative to its parent's (or absolute, when it
starts with the whole parent path), and the parent renders the matching child
where it puts `router.Outlet()`:

```go
func NotesLayout() tela.Node {
    return Div(Class("flex gap-4"),
        Div(Text("Folders")),
        Div(Class("flex-1"), router.Outlet()))
}
```

A route without a `Path` is a layout that adds no segment (a shell around
`/login` and `/signup`); `Index: true` is the route that renders at the parent's
own path, the "home" of a layout. A route without an `Element` renders its
outlet.

A `router.Routes` inside the element of a route matches the rest of the path,
after what that route matched, for a part of the app that owns its screens
(React Router's descendant routes).

## Links and navigation

`router.Link` is an `A` element: a link for the accessibility tree, with the
pointer's hand, a tab stop that Enter activates, and a focus ring in the color of
its text (add `focus-visible:` classes in `Class` for another). It is
`aria-current="page"` when its path is the location's.

```go
router.Link{To: "/notes/3", Class: "underline", Children: Text("Groceries")}
router.Link{To: "../4", Children: Text("Next")} // relative
router.Link{To: "?sort=name", Replace: true, Children: Text("By name")}
```

From code, `UseNavigate()` returns a comparable `Navigator`:

```go
nav := router.UseNavigate()
Button(OnClick(func() { nav.To("/notes/new") }), Text("New"))
Button(OnClick(nav.Back), Disabled(!nav.CanGoBack()), Text("Back"))
```

`To` pushes an entry and drops the forward ones, `Replace` swaps the current
entry (a redirect after a sign-in: Back skips it), `Back` and `Forward` move
and do nothing at the ends. `CanGoBack` and `CanGoForward` are those of the
build that made the Navigator.

**Relative paths** resolve like React Router's: from the route the link or the
component is in, and `..` goes up one *route*, not one segment. In the element
of `/notes/:id`, `"edit"` is `/notes/3/edit`, `".."` is `/notes` and `"../4"` is
`/notes/4`. A path that is only a query or a fragment (`"?page=2"`, `"#top"`)
keeps the current path.

## Params, location and the query

```go
id := router.UseParams()["id"]      // "3" for /notes/3
loc := router.UseLocation()
loc.Path, loc.Query, loc.Hash       // "/notes/3", "sort=name", ""
loc.Values().Get("sort")            // the parsed query
```

`UseParams` returns the parameters of all the matched routes, the splat under
`"*"`; do not modify the map. The same route with other parameters keeps its
element, which updates in place.

## Back and forward from the mouse and the keyboard

Inside a Router the mouse's back and forward buttons go back and forward, and so
do Alt+Left and Alt+Right (Command+[ and Command+] with the Mac flavor of key
bindings, see `telatest.PlatformKeys`), as in Chromium, which also decides the
details:

- The key or button must be unhandled: a handler that calls `StopPropagation`
  or `PreventDefault` on the event keeps the router out of it. An app that uses
  the side buttons itself handles `OnPointerUp`, `OnMouseUp` (or the down
  events) and prevents the event.
- Text fields keep their keys. A field that uses a key (Control+Left, or
  Option+Left on a Mac, move its caret; typing; Command+[ is none of them)
  takes it; the keys the field leaves alone, such as Alt+Left on Windows and
  Linux, reach the router, as in Chromium.
- Key repeat does not repeat the navigation, and nothing happens at the ends of
  the history.

## Screens that come and go

The element of every route renders in a `Presence` keyed by the route, so a
navigation is an exit and an entrance:

```go
func NoteScreen() tela.Node {
    return Div(Class("absolute inset-0 transition-opacity duration-150 starting:opacity-0"),
        Exit("opacity-0"),
        /* the screen */)
}
```

The screen that is left stays mounted, inert, until its `Exit` classes finish;
the one that is entered uses `starting:` as any element that mounts. Both are in
the page while they overlap, so position them on top of each other
(`absolute inset-0`) when they should not push each other. A screen without
`Exit` classes (or `UsePresence`) leaves at once.

An element with the same `LayoutID` on two screens glides between them (a
shared element, like Motion's `layoutId`):

```go
// in the list
Div(LayoutID("avatar-"+n.ID), Class("size-10 rounded-md bg-sky-500"))
// in the note
Div(LayoutID("avatar-"+n.ID), Class("size-24 rounded-xl bg-sky-500"))
```

## Links from outside the app

With `Scheme: "notes"`, the app's links of that scheme that the operating system
opens are navigations to their path:

```go
router.Router{Scheme: "notes", Children: /* ... */}
// notes://notes/3      -> /notes/3
// notes:/notes/3       -> /notes/3
// notes:open?id=3      -> /open?id=3   (the query is kept)
```

They arrive through `desktop.UseLaunch`: a second start of a single-instance app
or the OS opening the link. `desktop.Register` with the scheme makes the OS send
them (see [Desktop Integration](Desktop-Integration)). Links of other schemes
are left to the app's own `UseLaunch`. A deep link pushes an entry, so Back
returns to where the app was.

## History and `tela dev`

The history is hook state of the Router, plain and exported, so `tela dev` keeps
it when it reloads the app. It is not saved when the app exits: to open on the
last screen, store `UseLocation().String()` in a setting and give it to
`Initial`.

## Testing

```go
app := telatest.Mount(t, tela.C(App), telatest.Size(480, 320))
app.Click(telatest.ByRole("link", "Notes"))
app.ExpectText("Notes list")
app.Press("Alt+ArrowLeft")                                         // back
app.Click(telatest.ByText("Notes list"), telatest.MouseBack())     // back, with the mouse
app.InjectLaunch(desktop.Launch{Source: desktop.LaunchOpened,      // a deep link
    Args: []string{"notes://notes/3"}})
```

`telatest.MouseBack()` and `telatest.MouseForward()` are options of `Click` and
`ClickAt`: real pointer events of the side buttons. With `NoSettle` the exit of
the screen that is left can be seen frame by frame, and `app.Bounds` of an
element with a `LayoutID` follows its glide.

## Not covered

- Browser history, hash routes and URLs: the history is in memory.
- Optional segments (`:id?`), case-sensitive routes, data loaders and actions.
- `NavLink`: a `Link` is `aria-current="page"` on an exact path match; for
  styling by prefix, compare `UseLocation().Path` yourself.
- Blocking a navigation (unsaved changes): keep the state in the app and
  disable the links, or navigate from a handler after asking.
