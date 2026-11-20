package router_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/router"
	"github.com/kuronosu/tela/telatest"
)

// A small two-screen app: a list of notes and the note, whose colored
// avatar is shared by the two screens (LayoutID), each screen fading as it
// comes and goes (Exit).

type noteInfo struct{ ID, Title, Body, Color string }

var noteData = []noteInfo{
	{"groceries", "Groceries", "Milk, eggs, coffee and something green.", "bg-emerald-500"},
	{"roof", "Call the roofer", "Ask about the leak above the stairs before the rains start.", "bg-sky-500"},
	{"trip", "Trip to Oaxaca", "Book the bus, the room near the market and the cooking class.", "bg-amber-500"},
}

const screenClass = "absolute inset-0 flex flex-col gap-3 bg-background p-5 transition-opacity duration-150 starting:opacity-0"

func noteList() tela.Node {
	return Div(Class(screenClass), Exit("opacity-0"), TestID("list"),
		H1(Class("text-xl font-semibold"), Text("Notes")),
		Div(Class("flex flex-col gap-2"),
			Map(noteData, func(n noteInfo) tela.Node {
				return tela.WithKey(router.Link{To: "/notes/" + n.ID, Class: "flex items-center gap-3 rounded-lg border border-border bg-card p-3 hover:bg-accent",
					Children: Fragment(
						Div(LayoutID("avatar-"+n.ID), TestID("avatar-"+n.ID), Class("size-10 shrink-0 rounded-md "+n.Color)),
						Span(Class("font-medium"), Text(n.Title)))}, n.ID)
			})))
}

func noteScreen() tela.Node {
	id := router.UseParams()["id"]
	var n noteInfo
	for _, c := range noteData {
		if c.ID == id {
			n = c
		}
	}
	return Div(Class(screenClass), Exit("opacity-0"), TestID("detail"),
		router.Link{To: "/", Class: "text-sm text-muted-foreground underline underline-offset-4", Children: Text("All notes")},
		Div(Class("flex items-center gap-4"),
			Div(LayoutID("avatar-"+n.ID), TestID("hero"), Class("size-24 shrink-0 rounded-xl "+n.Color)),
			H1(Class("text-2xl font-semibold"), Text(n.Title))),
		P(Class("text-muted-foreground"), Text(n.Body)))
}

func notesApp() tela.Node { return notesAppAt("/") }

func notesAppAt(initial string) tela.Node {
	return router.Router{Initial: initial, Children: Div(Class("relative h-full w-full bg-background text-foreground"),
		router.Routes(
			router.Route{Path: "/", Element: tela.C(noteList)},
			router.Route{Path: "/notes/:id", Element: tela.C(noteScreen)},
		))}
}

func TestTwoScreenAppGoldens(t *testing.T) {
	// Alt+Left is the Linux and Windows key (the Mac one is Cmd+[).
	app := telatest.Mount(t, tela.C(notesApp), telatest.Size(420, 300), telatest.PlatformKeys("linux"))
	app.ExpectText("Groceries")
	app.Golden("notes_list.png")
	app.Click(telatest.ByRole("link", "Call the roofer"))
	app.ExpectText("Ask about the leak")
	app.Golden("notes_detail.png")
	app.Press("Alt+ArrowLeft")
	app.ExpectText("Trip to Oaxaca")
}

// A link takes the focus from the keyboard and shows a ring.
func TestLinkFocusRing(t *testing.T) {
	app := telatest.Mount(t, tela.C(notesApp), telatest.Size(420, 300))
	app.Press("Tab")
	app.Press("Tab")
	app.Expect(telatest.ByRole("link", "Call the roofer"), telatest.Focused(true))
	app.Golden("notes_list_focus.png")
}

// The screen that is left exits through its Exit classes, inert, and goes
// when they end; the new one fades in.
func TestLeavingScreenExitsThroughPresence(t *testing.T) {
	app := telatest.Mount(t, tela.C(notesApp), telatest.Size(420, 300), telatest.NoSettle())
	app.Advance(time.Second)
	if n := app.Count(telatest.ByTestID("list")); n != 1 {
		t.Fatalf("%d list screens at the start", n)
	}
	app.Click(telatest.ByRole("link", "Trip to Oaxaca"))
	// Exiting: the list is inert and out of the semantics tree, but still
	// drawn (fading); the note is entering.
	if n := app.Count(telatest.ByTestID("list")); n != 0 {
		t.Errorf("the exiting list is still found by locators (%d)", n)
	}
	mid := app.Screenshot()
	app.Advance(60 * time.Millisecond)
	fading := app.Screenshot()
	if mid.RGBAAt(210, 5) == fading.RGBAAt(210, 5) && mid.RGBAAt(60, 60) == fading.RGBAAt(60, 60) {
		t.Errorf("nothing changed 60ms into the transition")
	}
	app.Advance(400 * time.Millisecond)
	if n := app.Count(telatest.ByTestID("detail")); n != 1 {
		t.Errorf("%d note screens after the transition", n)
	}
	// Mounted elements: the exit really ended (a single screen is left).
	tree := app.AriaSnapshot()
	if contains(tree, "Notes") && contains(tree, `heading "Notes"`) {
		t.Errorf("the list screen is still there:\n%s", tree)
	}
}

// The avatar of the note keeps its box from the list while the new screen
// mounts, and glides to its place (LayoutID across routes).
func TestSharedElementAnimatesBetweenRoutes(t *testing.T) {
	// Where the hero ends: the note's screen at rest.
	rest := telatest.Mount(t, tela.C(func() tela.Node { return notesAppAt("/notes/trip") }), telatest.Size(420, 300))
	to := rest.Bounds(telatest.ByTestID("hero"))

	app := telatest.Mount(t, tela.C(notesApp), telatest.Size(420, 300), telatest.NoSettle())
	app.Advance(time.Second)
	from := app.Bounds(telatest.ByTestID("avatar-trip"))
	app.Click(telatest.ByRole("link", "Trip to Oaxaca"))
	if to.Width <= from.Width {
		t.Fatalf("the hero %v is not larger than the avatar %v", to, from)
	}
	// Right after the navigation the hero is drawn where the avatar was
	// (its bounds are the animated ones), then glides, then rests.
	if got := app.Bounds(telatest.ByTestID("hero")); got != from {
		t.Errorf("the shared element starts at %v, not at the avatar's box %v", got, from)
	}
	app.Advance(80 * time.Millisecond)
	mid := app.Bounds(telatest.ByTestID("hero"))
	if mid.Width <= from.Width || mid.Width >= to.Width {
		t.Errorf("80ms in, the shared element is at %v, not between %v and %v", mid, from, to)
	}
	app.Advance(time.Second)
	if got := app.Bounds(telatest.ByTestID("hero")); got != to {
		t.Errorf("the shared element rests at %v, not at %v", got, to)
	}
}
