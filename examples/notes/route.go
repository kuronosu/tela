package main

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/router"
)

// Where the user is, as a router location (the memory history of
// router.Router, so the mouse's back and forward buttons and Alt+Left and
// Alt+Right move between the places):
//
//	/                      the overview of all the notes
//	/?in=folder:Home       the overview of a folder ("view:pinned", "view:unsaved", "tag:travel")
//	/notes/3               note 3, with the list of all the notes
//	/notes/3?in=tag:travel note 3, with the list of the notes of a filter

// place is a location of the app: the filter whose notes are shown, and the
// note that is open (0: none, the overview).
type place struct {
	Filter filter
	ID     int
}

// String is the router location of the place.
func (p place) String() string {
	path := "/"
	if p.ID > 0 {
		path = "/notes/" + strconv.Itoa(p.ID)
	}
	if p.Filter == viewAll {
		return path
	}
	return path + "?" + url.Values{"in": {p.Filter.Kind + ":" + p.Filter.Name}}.Encode()
}

// placeOf is the place a location names; anything it does not understand
// is the overview of all the notes.
func placeOf(loc router.Location) place {
	var p place
	if rest, ok := strings.CutPrefix(loc.Path, "/notes/"); ok {
		if id, err := strconv.Atoi(strings.Trim(rest, "/")); err == nil && id > 0 {
			p.ID = id
		}
	}
	p.Filter = viewAll
	if kind, name, ok := strings.Cut(loc.Values().Get("in"), ":"); ok && name != "" {
		switch kind {
		case kindView:
			if f := (filter{kindView, name}); f == viewPinned || f == viewUnsaved {
				p.Filter = f
			}
		case kindFolder, kindTag:
			p.Filter = filter{kind, name}
		}
	}
	return p
}
