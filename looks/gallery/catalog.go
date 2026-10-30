package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/adwaita"
	"github.com/kuronosu/tela/looks/breeze"
	"github.com/kuronosu/tela/looks/detect"
	"github.com/kuronosu/tela/looks/gallery/icons"
	"github.com/kuronosu/tela/looks/win11"
)

// catalogEntry is a look the gallery can show: its key, how the sidebar
// words it and the icon that goes with it.
type catalogEntry struct {
	ID    string
	Title string // as in the sidebar and the status bar
	Icon  tela.Icon
	Look  func() looks.Look
}

// catalog lists the looks in a fixed order.
var catalog = []catalogEntry{
	{ID: "win11", Title: "Windows 11", Icon: icons.LayoutGrid, Look: win11.Look},
	{ID: "adwaita", Title: "GNOME", Icon: icons.PanelLeft, Look: adwaita.Look},
	{ID: "breeze", Title: "KDE", Icon: icons.Settings, Look: breeze.Look},
}

func ids() []string {
	out := make([]string, len(catalog))
	for i, c := range catalog {
		out[i] = c.ID
	}
	return out
}

// byID returns the entry with a key, or nil.
func byID(id string) *catalogEntry {
	for i := range catalog {
		if catalog[i].ID == id {
			return &catalog[i]
		}
	}
	return nil
}

// detected is the entry for the look of the running desktop.
func detected() *catalogEntry {
	name := detect.Look().Name
	for i := range catalog {
		if catalog[i].Look().Name == name {
			return &catalog[i]
		}
	}
	return &catalog[1]
}
