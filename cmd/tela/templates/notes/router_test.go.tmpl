package main

import (
	"testing"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The history of the screens: the mouse's side buttons and Alt+Left and
// Alt+Right move between the notes and the overviews that were open.
func TestBackAndForward(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.ExpectValue(editor, welcomeText) // the first note of the first run

	a.Click(card("Groceries"))
	a.ExpectValue(editor, groceriesText)
	a.Click(railItem("Ideas"))
	a.ExpectRole("heading", "Ideas") // the overview of a folder
	a.ExpectCount(editor, 0)
	a.Click(card("Weekend in the mountains"))
	a.ExpectValue(titleBox, "Weekend in the mountains")

	// Back with the mouse, to the overview, with its filter.
	a.Click(telatest.ByRole("navigation", "Notes"), telatest.MouseBack())
	a.ExpectRole("heading", "Ideas")
	a.ExpectCount(editor, 0)
	// Back with the keyboard, to Groceries, which was beside the welcome note.
	a.Press("Alt+ArrowLeft")
	a.ExpectValue(editor, groceriesText)
	a.Press("Alt+ArrowLeft")
	a.ExpectValue(editor, welcomeText)
	a.Press("Alt+ArrowLeft") // the first place: nothing before it
	a.ExpectValue(editor, welcomeText)

	// Forward the same ways.
	a.Press("Alt+ArrowRight")
	a.ExpectValue(editor, groceriesText)
	a.Click(editor, telatest.MouseForward())
	a.ExpectRole("heading", "Ideas")
	a.Press("Alt+ArrowRight")
	a.ExpectValue(titleBox, "Weekend in the mountains")

	// A new place drops what was forward.
	a.Press("Alt+ArrowLeft")
	a.Press("Alt+ArrowLeft")
	a.ExpectValue(editor, groceriesText)
	a.Click(railItem("Pinned"))
	a.ExpectRole("heading", "Pinned")
	a.Press("Alt+ArrowRight") // nothing forward
	a.ExpectRole("heading", "Pinned")
}

// The Mac's keys are Cmd+[ and Cmd+].
func TestBackAndForwardOnAMac(t *testing.T) {
	sandbox(t)
	a := mountLook(t, kit.MacOS)
	a.Click(card("Groceries"))
	a.ExpectValue(editor, groceriesText)
	a.Press("Meta+[")
	a.ExpectValue(editor, welcomeText)
	a.Press("Meta+]")
	a.ExpectValue(editor, groceriesText)
	a.Press("Alt+ArrowLeft") // not the Mac's keys
	a.ExpectValue(editor, groceriesText)
}

// Deleting the open note puts the next one in its place in the history, so
// Back never returns to a note that is gone; Undo opens it again.
func TestBackAfterDeletingTheOpenNote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Groceries"))
	a.Click(deleteBtn)
	a.ExpectCount(card("Groceries"), 0)
	a.ExpectValue(titleBox, "Weekend in the mountains") // the next note of the list is open
	a.Press("Alt+ArrowLeft")
	a.ExpectValue(editor, welcomeText)
	a.Press("Alt+ArrowRight")
	a.ExpectValue(titleBox, "Weekend in the mountains")
}

// Links that open notes are places too: Back leaves them.
func TestLinkIsAPlace(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.InjectLaunch(desktop.Launch{Args: []string{"tela-notes://open?id=3"}})
	a.ExpectValue(editor, groceriesText)
	a.Press("Alt+ArrowLeft")
	a.ExpectValue(editor, welcomeText)
}

// The note that was open is open at the next start, in the filter it was
// found in.
func TestPlaceIsRemembered(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(railItem("Home"))
	a.Click(card("Groceries"))
	a.Close()

	b := mount(t)
	b.ExpectValue(editor, groceriesText)
}

func TestPlaceOf(t *testing.T) {
	for _, c := range []struct {
		p    place
		want string
	}{
		{place{viewAll, 0}, "/"},
		{place{viewPinned, 0}, "/?in=view%3Apinned"},
		{place{filter{kindFolder, "Getting started"}, 0}, "/?in=folder%3AGetting+started"},
		{place{viewAll, 3}, "/notes/3"},
		{place{filter{kindTag, "travel"}, 4}, "/notes/4?in=tag%3Atravel"},
	} {
		if got := c.p.String(); got != c.want {
			t.Errorf("%+v is %q, want %q", c.p, got, c.want)
		}
	}
}
