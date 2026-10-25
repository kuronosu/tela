package main

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/native/icons"
	. "github.com/kuronosu/tela/h"
)

// The icons are Lucide's, from the subset in icons/ (generated with
// `tela icons gen`). A look does not draw them differently: it colors them
// with its tokens (folder, kind-doc, kind-pdf...), and a look that wants
// another shape picks another icon.

// kindIcon returns the icon of a kind of entry and the token color that
// paints it.
func kindIcon(k Kind) (tela.Icon, string) {
	switch k {
	case KindFolder:
		return icons.Folder, "text-folder fill-folder/30"
	case KindDoc:
		return icons.FileText, "text-kind-doc"
	case KindSheet:
		return icons.FileSpreadsheet, "text-kind-sheet"
	case KindPDF:
		return icons.FileText, "text-kind-pdf"
	case KindImage:
		return icons.FileImage, "text-kind-image"
	case KindAudio:
		return icons.FileAudio, "text-kind-audio"
	case KindVideo:
		return icons.FileVideo, "text-kind-video"
	case KindArchive:
		return icons.FileArchive, "text-kind-archive"
	case KindCode:
		return icons.FileCode, "text-kind-code"
	}
	return icons.FileText, "text-kind-text"
}

// noSort is the arrow of a sortable column header that is not the sorted
// one: an empty drawing, so that only the sorted column shows an arrow (as
// every platform's file list does) and the headers do not move when the
// order changes.
var noSort = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"></svg>`}

// EntryIcon draws the icon of an entry at px pixels; extra is more classes,
// for the look (the icon of a selected row). The stroke of a big icon is
// thinner, so that it stays a line drawing.
func EntryIcon(e *Entry, px float32, extra ...string) tela.Node {
	ic, color := kindIcon(e.Kind)
	stroke := "stroke-[1.5]"
	if px > 32 {
		stroke = "stroke-[0.8]"
	}
	return Icon(ic, Class("shrink-0", stroke, color, strings.Join(extra, " ")), Style(tela.Width(px), tela.Height(px)))
}
