package router_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/router"
	"github.com/kuronosu/tela/telatest"
)

// The notes app of these tests: a home, a notes layout with a list (its
// index route) and a note per id, and a not-found fallback.

func home() tela.Node { return H1(Text("Home screen")) }

func notesLayout() tela.Node {
	return Div(Class("flex flex-col gap-2"),
		H2(Text("Notes layout")),
		router.Outlet())
}

func notesList() tela.Node {
	return Div(Text("Notes list"),
		router.Link{To: "3", Children: Text("open three")})
}

func note() tela.Node {
	id := router.UseParams()["id"]
	return Div(Text("Note "+id), router.Link{To: "..", Children: Text("all notes")},
		router.Link{To: "../4", Children: Text("note four")})
}

func notFound() tela.Node {
	return Div(Text("Not found: " + router.UseLocation().Path))
}

func routes() tela.Node {
	return router.Routes(
		router.Route{Path: "/", Element: tela.C(home)},
		router.Route{Path: "/notes", Element: tela.C(notesLayout), Children: []router.Route{
			{Index: true, Element: tela.C(notesList)},
			{Path: ":id", Element: tela.C(note)},
		}},
		router.Route{Path: "*", Element: tela.C(notFound)},
	)
}

// bar is the app's own navigation: links, the history buttons and the
// location.
func bar() tela.Node {
	nav := router.UseNavigate()
	loc := router.UseLocation()
	return Div(Class("flex items-center gap-3 p-2"),
		router.Link{To: "/", Children: Text("Home")},
		router.Link{To: "/notes", Children: Text("Notes")},
		Button(Disabled(!nav.CanGoBack()), OnClick(nav.Back), Text("Back")),
		Button(Disabled(!nav.CanGoForward()), OnClick(nav.Forward), Text("Forward")),
		Span(TestID("location"), Text(loc.String())))
}

// probe stores the navigator of its place in the tree in dst, for tests
// that navigate from code.
func probe(dst *router.Navigator) tela.Node {
	return tela.C(func() tela.Node {
		*dst = router.UseNavigate()
		return nil
	})
}

// shell is the app: a Router around the bar and the routes, with the
// extra nodes in the page.
func shell(initial, scheme string, nav *router.Navigator, extra ...tela.Child) tela.Node {
	kids := []tela.Child{tela.C(bar)}
	kids = append(kids, extra...)
	kids = append(kids, Div(Class("relative flex-1 p-3"), routes()))
	if nav != nil {
		kids = append(kids, probe(nav))
	}
	return router.Router{Initial: initial, Scheme: scheme,
		Children: Div(append([]tela.Child{Class("flex h-full flex-col text-sm")}, kids...)...)}
}

func mountShell(t *testing.T, nav *router.Navigator, extra []tela.Child, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.Mount(t, tela.C(func() tela.Node { return shell("/", "", nav, extra...) }),
		append([]telatest.Option{telatest.Size(520, 300)}, opts...)...)
}

func expectLocation(t *testing.T, app *telatest.App, want string) {
	t.Helper()
	app.ExpectCount(telatest.ByText(want, telatest.Exact()), 1)
}

func TestLinksNavigateAndHistoryButtonsWork(t *testing.T) {
	app := mountShell(t, nil, nil)
	app.ExpectText("Home screen")
	app.Expect(telatest.ByRole("button", "Back"), telatest.Disabled(true))
	app.Expect(telatest.ByRole("button", "Forward"), telatest.Disabled(true))

	app.Click(telatest.ByRole("link", "Notes"))
	app.ExpectText("Notes layout")
	app.ExpectText("Notes list")
	app.ExpectNoText("Home screen")

	app.Click(telatest.ByRole("link", "open three")) // relative: /notes/3
	app.ExpectText("Note 3")
	app.ExpectText("Notes layout")
	app.Expect(telatest.ByRole("button", "Back"), telatest.Disabled(false))

	app.Click(telatest.ByRole("button", "Back"))
	app.ExpectText("Notes list")
	app.Expect(telatest.ByRole("button", "Forward"), telatest.Disabled(false))
	app.Click(telatest.ByRole("button", "Forward"))
	app.ExpectText("Note 3")
	app.Expect(telatest.ByRole("button", "Forward"), telatest.Disabled(true))

	// Navigating from the middle drops the forward entries.
	app.Click(telatest.ByRole("button", "Back"))
	app.Click(telatest.ByRole("link", "Home"))
	app.ExpectText("Home screen")
	app.Expect(telatest.ByRole("button", "Forward"), telatest.Disabled(true))
	app.Click(telatest.ByRole("button", "Back"))
	app.ExpectText("Notes list")
}

func TestRelativeLinksResolveFromTheirRoute(t *testing.T) {
	app := mountShell(t, nil, nil)
	app.Click(telatest.ByRole("link", "Notes"))
	app.Click(telatest.ByRole("link", "open three"))
	app.ExpectText("Note 3")
	// ".." goes up one route: from /notes/:id to /notes, and "../4" to a
	// sibling note.
	app.Click(telatest.ByRole("link", "note four"))
	app.ExpectText("Note 4")
	app.Click(telatest.ByRole("link", "all notes"))
	app.ExpectText("Notes list")
}

func TestLinkIsALinkAndTheCurrentPage(t *testing.T) {
	app := mountShell(t, nil, nil)
	app.Expect(telatest.ByRole("link", "Home"), telatest.Current("page"))
	app.Expect(telatest.ByRole("link", "Notes"), telatest.Current("false"))
	app.Click(telatest.ByRole("link", "Notes"))
	app.Expect(telatest.ByRole("link", "Notes"), telatest.Current("page"))
	app.Expect(telatest.ByRole("link", "Home"), telatest.Current("false"))
}

func TestLinkWorksFromTheKeyboard(t *testing.T) {
	app := mountShell(t, nil, nil)
	app.Press("Tab") // Home
	app.Press("Tab") // Notes
	app.Expect(telatest.ByRole("link", "Notes"), telatest.Focused(true))
	app.Press("Enter")
	app.ExpectText("Notes list")
	// Space does not follow a link, as in a browser.
	app.Press("Tab") // Back
	app.Press("Tab") // Forward (disabled) is skipped: the next link
	app.Press("Space")
	app.ExpectText("Notes list")
}

func TestLinkToTheCurrentPageReplaces(t *testing.T) {
	app := mountShell(t, nil, nil)
	app.Click(telatest.ByRole("link", "Notes"))
	app.Click(telatest.ByRole("link", "Notes"))
	app.Click(telatest.ByRole("link", "Notes"))
	app.Click(telatest.ByRole("button", "Back"))
	app.ExpectText("Home screen") // not a stack of three copies of /notes
}

func TestNavigatorTo(t *testing.T) {
	var nav router.Navigator
	app := mountShell(t, &nav, nil)
	nav.To("/notes/7?tab=2#top")
	expectLocation(t, app, "/notes/7?tab=2#top")
	app.ExpectText("Note 7")
	nav.Replace("/notes/8")
	app.ExpectText("Note 8")
	nav.Back()
	app.ExpectText("Home screen")
	nav.Forward()
	app.ExpectText("Note 8")
	if !nav.CanGoBack() || nav.CanGoForward() {
		t.Errorf("CanGoBack %v, CanGoForward %v at the last entry", nav.CanGoBack(), nav.CanGoForward())
	}
	// Only a query: the same path with another query.
	nav.To("?page=2")
	expectLocation(t, app, "/notes/8?page=2")
	// Relative to the route the navigator was made in: the root.
	nav.To("notes")
	app.ExpectText("Notes list")
}

func TestNavigatorBeyondTheEndsDoesNothing(t *testing.T) {
	var nav router.Navigator
	app := mountShell(t, &nav, nil)
	nav.Back()
	nav.Forward()
	app.ExpectText("Home screen")
}

func TestParamsLocationAndSplat(t *testing.T) {
	var params router.Params
	var loc router.Location
	page := func() tela.Node {
		return router.Router{Initial: "/files/a%20b/docs/readme.md?raw=1&raw=2#top", Children: router.Routes(
			router.Route{Path: "/files/:name/*", Element: tela.C(func() tela.Node {
				params, loc = router.UseParams(), router.UseLocation()
				return Text("files")
			})})}
	}
	telatest.Mount(t, tela.C(page), telatest.Size(200, 60))
	if params["name"] != "a b" || params["*"] != "docs/readme.md" {
		t.Errorf("params %v", params)
	}
	if loc.Path != "/files/a%20b/docs/readme.md" || loc.Query != "raw=1&raw=2" || loc.Hash != "top" {
		t.Errorf("location %+v", loc)
	}
	if got := loc.Values()["raw"]; len(got) != 2 || got[1] != "2" {
		t.Errorf("query values %v", got)
	}
}

func TestNothingMatchedRendersNothing(t *testing.T) {
	page := func() tela.Node {
		return router.Router{Initial: "/nope", Children: Div(Text("shell"), router.Routes(
			router.Route{Path: "/", Element: tela.C(home)}))}
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(200, 60))
	app.ExpectText("shell")
	app.ExpectNoText("Home screen")
}

func TestFallbackRoute(t *testing.T) {
	page := func() tela.Node { return router.Router{Initial: "/a/b/c", Children: routes()} }
	app := telatest.Mount(t, tela.C(page), telatest.Size(300, 80))
	app.ExpectText("Not found: /a/b/c")
}

func TestNestedRoutesInAnElement(t *testing.T) {
	inner := func() tela.Node {
		return router.Routes(
			router.Route{Path: "a", Element: Text("inner a")},
			router.Route{Path: "b/:x", Element: tela.C(func() tela.Node { return Text("inner b " + router.UseParams()["x"]) })},
		)
	}
	page := func() tela.Node {
		return router.Router{Initial: "/app/b/9", Children: router.Routes(
			router.Route{Path: "/app/*", Element: Div(Text("app"), tela.C(inner))})}
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(300, 80))
	app.ExpectText("app")
	app.ExpectText("inner b 9")
}

func TestUsingTheRouterOutsideOfOnePanicsWithTheFix(t *testing.T) {
	for name, f := range map[string]func() tela.Node{
		"UseNavigate": func() tela.Node { router.UseNavigate(); return nil },
		"UseLocation": func() tela.Node { router.UseLocation(); return nil },
		"Routes":      func() tela.Node { return router.Routes(router.Route{Path: "/"}) },
	} {
		func() {
			defer func() {
				msg, _ := recover().(string)
				if msg == "" {
					t.Errorf("%s: no panic outside a Router", name)
				} else if want := "router.Router"; !contains(msg, want) {
					t.Errorf("%s: panic %q does not name %s", name, msg, want)
				}
			}()
			telatest.Mount(t, tela.C(f), telatest.AllowDev("panic"))
		}()
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// Back and forward from the mouse and the keyboard.

// visited goes to /notes and /notes/3: two entries back to the home.
func visited(t *testing.T, extra []tela.Child, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := mountShell(t, nil, extra, opts...)
	app.Click(telatest.ByRole("link", "Notes"))
	app.Click(telatest.ByRole("link", "open three"))
	app.ExpectText("Note 3")
	return app
}

func TestMouseSideButtonsGoBackAndForward(t *testing.T) {
	app := visited(t, nil)
	app.Click(telatest.ByText("Note 3"), telatest.MouseBack())
	app.ExpectText("Notes list")
	app.Click(telatest.ByText("Notes list"), telatest.MouseBack())
	app.ExpectText("Home screen")
	// At the first entry there is nowhere to go.
	app.Click(telatest.ByText("Home screen"), telatest.MouseBack())
	app.ExpectText("Home screen")
	app.ClickAt(5, 5, telatest.MouseForward()) // anywhere in the window
	app.ExpectText("Notes list")
	app.Click(telatest.ByText("Notes list"), telatest.MouseForward())
	app.ExpectText("Note 3")
	app.Click(telatest.ByText("Note 3"), telatest.MouseForward()) // at the last entry
	app.ExpectText("Note 3")
}

func TestMouseSideButtonsYieldToHandlers(t *testing.T) {
	for _, tc := range []struct {
		name string
		prop tela.Prop
	}{
		{"pointerdown prevented", OnPointerDown(func(e tela.PointerEvent) { e.PreventDefault() })},
		{"mousedown stopped", OnMouseDown(func(e tela.MouseEvent) { e.StopPropagation() })},
		{"pointerup prevented", OnPointerUp(func(e tela.PointerEvent) { e.PreventDefault() })},
		{"mouseup prevented", OnMouseUp(func(e tela.MouseEvent) { e.PreventDefault() })},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mine := Div(TestID("mine"), Class("h-10 w-40 bg-slate-100"), tc.prop, Text("handled here"))
			app := visited(t, []tela.Child{mine})
			app.Click(telatest.ByTestID("mine"), telatest.MouseBack())
			app.ExpectText("Note 3") // the handler took the button
			app.Click(telatest.ByText("Note 3"), telatest.MouseBack())
			app.ExpectText("Notes list")
		})
	}
}

func TestSideButtonsAreLeftToTheAppsOwnHandlerOfTheOtherButton(t *testing.T) {
	// A handler that only looks at the primary button does not take the
	// back button.
	clicks := 0
	mine := Div(TestID("mine"), Class("h-10 w-40 bg-slate-100"), OnClick(func() { clicks++ }), Text("click me"))
	app := visited(t, []tela.Child{mine})
	app.Click(telatest.ByTestID("mine"), telatest.MouseBack())
	app.ExpectText("Notes list")
	if clicks != 0 {
		t.Errorf("%d clicks for the back button", clicks)
	}
}

func TestAltArrowsGoBackAndForward(t *testing.T) {
	app := visited(t, nil, telatest.PlatformKeys("linux"))
	app.Press("Alt+ArrowLeft")
	app.ExpectText("Notes list")
	app.Press("Alt+ArrowLeft")
	app.ExpectText("Home screen")
	app.Press("Alt+ArrowLeft") // nothing before
	app.ExpectText("Home screen")
	app.Press("Alt+ArrowRight")
	app.ExpectText("Notes list")
	app.Press("Alt+ArrowRight")
	app.ExpectText("Note 3")
	// Other chords are not navigation.
	app.Press("Alt+Shift+ArrowLeft")
	app.Press("Control+Alt+ArrowLeft")
	app.Press("ArrowLeft")
	app.Press("Meta+[")
	app.ExpectText("Note 3")
}

func TestCommandBracketsGoBackAndForwardUnderTheMacFlavor(t *testing.T) {
	app := visited(t, nil, telatest.PlatformKeys("darwin"))
	app.Press("Meta+[")
	app.ExpectText("Notes list")
	app.Press("Meta+[")
	app.ExpectText("Home screen")
	app.Press("Meta+]")
	app.ExpectText("Notes list")
	app.Press("ControlOrMeta+]")
	app.ExpectText("Note 3")
	// Option+Left is a word move on a Mac, not navigation; so is Control+[.
	app.Press("Alt+ArrowLeft")
	app.Press("Control+[")
	app.Press("Meta+Shift+[")
	app.ExpectText("Note 3")
}

func TestKeyboardNavigationYieldsToHandlers(t *testing.T) {
	for _, tc := range []struct {
		name string
		prop tela.Prop
	}{
		{"prevented", OnKeyDown(func(e tela.KeyboardEvent) { e.PreventDefault() })},
		{"stopped", OnKeyDown(func(e tela.KeyboardEvent) { e.StopPropagation() })},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box := Div(TestID("box"), TabIndex(0), Class("h-10 w-40 bg-slate-100"), tc.prop, Text("takes keys"))
			app := visited(t, []tela.Child{box}, telatest.PlatformKeys("linux"))
			app.Focus(telatest.ByTestID("box"))
			app.Press("Alt+ArrowLeft")
			app.ExpectText("Note 3")
			app.Click(telatest.ByText("Note 3")) // the focus leaves the box
			app.Press("Alt+ArrowLeft")
			app.ExpectText("Notes list")
		})
	}
}

// Chromium navigates on Alt+Left in a text field on Windows and Linux (the
// editing commands have no binding for it) and on Command+[ on a Mac; the
// keys the field does use (Option+Left on a Mac, Control+Left elsewhere)
// move its caret and are left to it.
func TestTextFieldsKeepTheirOwnKeys(t *testing.T) {
	typed := func(app *telatest.App, name string) {
		app.Focus(telatest.ByRole("textbox", name))
		app.Type("one two")
	}
	field := func() []tela.Child { return []tela.Child{Input(AriaLabel("Search"), Class("h-8 w-40 border"))} }

	t.Run("linux navigates", func(t *testing.T) {
		app := visited(t, field(), telatest.PlatformKeys("linux"))
		typed(app, "Search")
		app.Press("Alt+ArrowLeft")
		app.ExpectText("Notes list")
		app.Expect(telatest.ByRole("textbox", "Search"), telatest.HasValue("one two"))
	})
	t.Run("the field's word move is its own", func(t *testing.T) {
		app := visited(t, field(), telatest.PlatformKeys("linux"))
		typed(app, "Search")
		app.Press("Control+ArrowLeft")
		app.Press("Control+ArrowLeft")
		app.Press("x")
		app.Expect(telatest.ByRole("textbox", "Search"), telatest.HasValue("xone two"))
		app.ExpectText("Note 3")
	})
	t.Run("mac: Option+Left moves the caret", func(t *testing.T) {
		app := visited(t, field(), telatest.PlatformKeys("darwin"))
		typed(app, "Search")
		app.Press("Alt+ArrowLeft")
		app.Press("x")
		app.Expect(telatest.ByRole("textbox", "Search"), telatest.HasValue("one xtwo"))
		app.ExpectText("Note 3")
	})
	t.Run("mac: Command+[ navigates", func(t *testing.T) {
		app := visited(t, field(), telatest.PlatformKeys("darwin"))
		typed(app, "Search")
		app.Press("Meta+[")
		app.ExpectText("Notes list")
	})
	t.Run("a handler on the field takes the key", func(t *testing.T) {
		taken := Input(AriaLabel("Search"), Class("h-8 w-40 border"),
			OnKeyDown(func(e tela.KeyboardEvent) {
				if e.Alt && e.Key == tela.KeyArrowLeft {
					e.PreventDefault()
				}
			}))
		app := visited(t, []tela.Child{taken}, telatest.PlatformKeys("linux"))
		typed(app, "Search")
		app.Press("Alt+ArrowLeft")
		app.ExpectText("Note 3")
	})
}

// ---------------------------------------------------------------------
// Deep links.

func TestDeepLinksOfTheSchemeOpenTheirPath(t *testing.T) {
	var nav router.Navigator
	app := telatest.MountApp(t, tela.App{AppID: "com.example.deeplinks", Width: 520, Height: 300,
		Root: tela.C(func() tela.Node { return shell("/", "notes", &nav) })})
	app.ExpectText("Home screen")
	for _, tc := range []struct{ link, want string }{
		{"notes://notes/3", "/notes/3"},
		{"notes:/notes/4", "/notes/4"},
		{"notes:notes/5?pin=1", "/notes/5?pin=1"},
		{"NOTES:///notes", "/notes"},
		{"notes:open?id=3", "/open?id=3"},
		{"notes:", "/"},
	} {
		app.InjectLaunch(desktop.Launch{Args: []string{tc.link}, Source: desktop.LaunchOpened})
		expectLocation(t, app, tc.want)
	}
	// Other launches are not the router's.
	app.InjectLaunch(desktop.Launch{Args: []string{"notes://notes/9"}, Source: desktop.LaunchOpened})
	expectLocation(t, app, "/notes/9")
	for _, a := range [][]string{{"https://example.com/notes/1"}, {"mynotes://notes/1"}, {"--flag", "/tmp/a.txt"}, {"notes"}} {
		app.InjectLaunch(desktop.Launch{Args: a, Source: desktop.LaunchOpened})
		expectLocation(t, app, "/notes/9")
	}
	// A forwarded start (single instance) works the same, and the link can
	// be undone with Back.
	app.InjectLaunch(desktop.Launch{Args: []string{"-x", "notes://notes/2"}})
	expectLocation(t, app, "/notes/2")
	app.ExpectText("Note 2")
	nav.Back()
	expectLocation(t, app, "/notes/9")
}

func TestWithoutASchemeLinksAreTheAppsOwn(t *testing.T) {
	var got []string
	app := telatest.MountApp(t, tela.App{AppID: "com.example.deeplinks2", Width: 520, Height: 300,
		Root: tela.C(func() tela.Node {
			return router.Router{Children: tela.C(func() tela.Node {
				desktop.UseLaunch(func(l desktop.Launch) { got = append(got, l.URLs()...) })
				return Div(tela.C(bar), routes())
			})}
		})})
	app.InjectLaunch(desktop.Launch{Args: []string{"notes://notes/3"}, Source: desktop.LaunchOpened})
	app.ExpectText("Home screen")
	if len(got) != 1 || got[0] != "notes://notes/3" {
		t.Errorf("the app's UseLaunch got %q", got)
	}
}
