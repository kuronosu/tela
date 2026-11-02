package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/kuronosu/tela/desktop"
)

// notes: links, the deep links of Notes. A second process that is started
// with one (a click in a browser, `xdg-open 'notes://new?text=hi'`, `start
// notes://new?text=hi`, or the program with the link as its argument) hands
// it to the running one, which shows its window with the result
// (desktop.SingleInstance and desktop.UseLaunch).
//
//	notes://new?title=Groceries&text=milk%20and%20eggs   a new note, with that title and text
//	notes://new                                          a new, empty note
//	notes://open?id=3                                    the note with that ID
//
// The query form is the one that is documented; "notes:new?..." and
// "notes:open?id=3", with no slashes, mean the same (some launchers drop
// them), and so does a slash after the action ("notes://open/?id=3", which
// browsers add). A note's link is made by "Copy link" in the context menu
// of its card.

// link is a parsed notes: link.
type link struct {
	// Action is "new" or "open".
	Action string
	// ID is the note to open (open).
	ID int
	// Title and Text are the content of the new note (new).
	Title, Text string
}

// errNotOurs is returned by parseLink for a URL of another scheme: the
// launch gets any URL, and the others are not Notes's to open.
var errNotOurs = errors.New("not a notes: link")

// parseLink parses a notes: link. A URL of another scheme is errNotOurs;
// anything else that is wrong has an error that says how to write it.
func parseLink(raw string) (link, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return link{}, fmt.Errorf("%q is not a valid link", raw)
	}
	if !strings.EqualFold(u.Scheme, "notes") {
		return link{}, errNotOurs
	}
	action := u.Host
	if action == "" {
		action = u.Opaque
	}
	if action == "" {
		action = u.Path
	}
	action = strings.ToLower(strings.Trim(action, "/"))
	q := u.Query()
	switch action {
	case "new":
		return link{Action: "new", Title: strings.TrimSpace(q.Get("title")), Text: q.Get("text")}, nil
	case "open":
		id, err := strconv.Atoi(q.Get("id"))
		if err != nil || id <= 0 {
			return link{}, fmt.Errorf("%q has no note: write notes://open?id=3, with the number of a note", raw)
		}
		return link{Action: "open", ID: id}, nil
	}
	return link{}, fmt.Errorf("%q: Notes has notes://new?title=...&text=... and notes://open?id=3", raw)
}

// The association Notes registers: its URL scheme, and the .md files it
// offers to open when the user asks.
var (
	notesScheme  = desktop.URLScheme("notes")
	markdownType = desktop.FileType(".md", "text/markdown", "Markdown document")
)

// autoRegister makes Notes register its scheme when it starts. main turns it
// on; tests and `tela snapshot` leave it off, so they never write to the
// folders of the user who runs them.
var autoRegister atomic.Bool

// registrationBlocked says why Notes cannot register itself now, or "" when
// it can. What it registers is the path of the running program, and the OS
// would keep running that path after the program is gone: under `go run` the
// program is a temporary build, which `go run` deletes when it ends. (Under
// tela dev desktop.Register refuses by itself, with ErrDevExecutable.)
func registrationBlocked() string {
	if f := blockedBy.Load(); f != nil {
		return (*f)()
	}
	exe, err := os.Executable()
	if err == nil && strings.Contains(filepath.ToSlash(exe), "/go-build") {
		return "This is a temporary build (go run). Build the program with go build -o notes ./examples/notes and run that one to register it."
	}
	return ""
}

// blockedBy replaces registrationBlocked's check: the tests pretend to be
// an installed program, or a temporary one. Atomic because the goroutine
// that registers at start may outlive a test.
var blockedBy atomic.Pointer[func() string]

// registerCalls counts the calls of registerAtStart, for the tests to wait
// for the goroutine that makes them.
var registerCalls atomic.Int32

// registerApp makes the desktop know Notes before it starts (ADR-217): on
// Linux its own desktop entry, which the global shortcuts portal needs for
// Quick note (Windows needs none, and nothing is written there). It runs
// in main, synchronously, so the entry exists before the shortcut is
// registered; it looks first, so a start with the entry up to date writes
// nothing, and an entry Notes did not write (a package's) is left alone.
// Only the built program registers: under go run it is a temporary build,
// and under tela dev RegisterApp refuses by itself (ErrDevExecutable).
func registerApp() {
	if registrationBlocked() != "" {
		return
	}
	_, err := desktop.RegisterApp()
	if err != nil && !errors.Is(err, desktop.ErrUnsupported) && !errors.Is(err, desktop.ErrDevExecutable) {
		fmt.Fprintln(os.Stderr, "notes:", err)
	}
}

// registerAtStart registers the notes: scheme as a handler, unless the user
// turned it off in Preferences, another application holds it, or the
// program is not one to register. Nothing here is the user's to be told:
// Preferences shows the state when they look. It makes Notes a candidate,
// never the default (that is a button of its own), and never takes another
// application's association (no desktop.Replace). It blocks on file or
// registry I/O: call it off the UI thread.
func registerAtStart(wanted bool) {
	registerCalls.Add(1)
	if !autoRegister.Load() || !wanted || registrationBlocked() != "" {
		return
	}
	// ErrDevExecutable (tela dev) and ErrAssociationTaken are expected.
	_, _ = desktop.Register(notesScheme)
}
