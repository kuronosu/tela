package router_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/router"
	"github.com/kuronosu/tela/telatest"
)

// exampleT stands in for the *testing.T that examples do not have.
type exampleT struct {
	testing.TB // nil: the methods telatest uses are below
	cleanups   []func()
}

func (e *exampleT) Helper()                      {}
func (e *exampleT) Name() string                 { return "example" }
func (e *exampleT) Cleanup(f func())             { e.cleanups = append(e.cleanups, f) }
func (e *exampleT) Logf(string, ...any)          {}
func (e *exampleT) Errorf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Fatalf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Failed() bool                 { return false }
func (e *exampleT) close() {
	for i := len(e.cleanups) - 1; i >= 0; i-- {
		e.cleanups[i]()
	}
}

func exampleHome() tela.Node { return H1(Text("Home")) }

func exampleNote() tela.Node {
	id := router.UseParams()["id"]
	return Div(Text("Note "+id), router.Link{To: "..", Children: Text("All notes")})
}

func exampleNotes() tela.Node { return Div(H2(Text("Notes")), router.Outlet()) }

func exampleList() tela.Node {
	return Div(
		router.Link{To: "1", Children: Text("First")},
		router.Link{To: "2", Children: Text("Second")})
}

func exampleApp() tela.Node {
	return router.Router{Children: Div(
		router.Link{To: "/", Children: Text("Home")},
		router.Link{To: "/notes", Children: Text("Notes")},
		router.Routes(
			router.Route{Path: "/", Element: tela.C(exampleHome)},
			router.Route{Path: "/notes", Element: tela.C(exampleNotes), Children: []router.Route{
				{Index: true, Element: tela.C(exampleList)},
				{Path: ":id", Element: tela.C(exampleNote)},
			}},
			router.Route{Path: "*", Element: Text("Not found")},
		))}
}

// An app with a layout, a list and a note: links navigate, relative paths
// resolve from their route, and ":id" gives the note its parameter.
func ExampleRouter() {
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, tela.C(exampleApp), telatest.Size(300, 200))
	app.Click(telatest.ByRole("link", "Notes"))
	app.Click(telatest.ByRole("link", "Second"))
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - link "Home":
	//   - /url: /
	// - link "Notes":
	//   - /url: /notes
	// - heading "Notes" [level=2]
	// - text: Note 2
	// - link "All notes":
	//   - /url: /notes
}

// Matching is ranked, not in order: a static segment beats a parameter, and
// the splat catches the rest.
func ExampleRoutes() {
	t := &exampleT{}
	defer t.close()
	page := func(initial string) tela.Node {
		return router.Router{Initial: initial, Children: router.Routes(
			router.Route{Path: "*", Element: Text("not found")},
			router.Route{Path: "/notes/:id", Element: tela.C(func() tela.Node { return Text("note " + router.UseParams()["id"]) })},
			router.Route{Path: "/notes/new", Element: Text("new note")},
		)}
	}
	for _, at := range []string{"/notes/new", "/notes/42", "/other"} {
		app := telatest.Mount(t, tela.C(func() tela.Node { return page(at) }), telatest.Size(200, 50))
		fmt.Println(at, "->", app.AriaSnapshot())
	}
	// Output:
	// /notes/new -> - text: new note
	// /notes/42 -> - text: note 42
	// /other -> - text: not found
}

// Navigation from code: a handler that goes to a path, replaces the entry or
// goes back, and the history it keeps.
func ExampleUseNavigate() {
	t := &exampleT{}
	defer t.close()
	toolbar := func() tela.Node {
		nav := router.UseNavigate()
		loc := router.UseLocation()
		return Div(
			Button(OnClick(func() { nav.To("/inbox?unread=1") }), Text("Inbox")),
			Button(OnClick(func() { nav.Replace("/archive") }), Text("Archive")),
			Button(Disabled(!nav.CanGoBack()), OnClick(nav.Back), Text("Back")),
			Text(loc.String()))
	}
	app := telatest.Mount(t, tela.C(func() tela.Node { return router.Router{Children: tela.C(toolbar)} }), telatest.Size(300, 100))
	app.Click(telatest.ByRole("button", "Inbox"))
	app.Click(telatest.ByRole("button", "Archive"))
	app.ExpectText("/archive")
	app.Click(telatest.ByRole("button", "Back"))
	app.ExpectText("/")
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - button "Inbox"
	// - button "Archive"
	// - button "Back" [disabled]
	// - text: /
}

// With a Scheme, the links of the app that the OS opens navigate. The test
// plays the OS (macOS Launch Services, or a second start of a
// single-instance app).
func ExampleRouter_scheme() {
	t := &exampleT{}
	defer t.close()
	app := telatest.MountApp(t, tela.App{AppID: "com.example.notes", Width: 300, Height: 100,
		Root: tela.C(func() tela.Node {
			return router.Router{Scheme: "notes", Children: router.Routes(
				router.Route{Path: "/", Element: Text("home")},
				router.Route{Path: "/notes/:id", Element: tela.C(exampleNote)},
			)}
		})})
	app.InjectLaunch(desktop.Launch{Source: desktop.LaunchOpened, Args: []string{"notes://notes/3"}})
	app.ExpectText("Note 3")
	fmt.Println("opened")
	// Output: opened
}
