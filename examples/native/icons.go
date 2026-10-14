package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Every icon is drawn with boxes on a 16 unit grid scaled to the requested
// size: no image assets, no icon font. Glyphs use the current text color
// (bg-current, border-current), so they follow the theme's text color and
// whatever state their button is in; file and folder icons use the colors
// of the theme's Look.

// grid places boxes of a 16x16 design on a canvas of px pixels.
type grid struct{ u float32 }

func newGrid(px float32) grid { return grid{px / 16} }

// canvas is the icon's box.
func (g grid) canvas(extra string, kids ...tela.Child) tela.Node {
	c := []tela.Child{
		Class("relative shrink-0 " + extra),
		Style(tela.Width(16*g.u), tela.Height(16*g.u)),
	}
	return Div(append(c, kids...)...)
}

// at is a box of the design at (x, y) of size w x h.
func (g grid) at(x, y, w, h float32, class string, kids ...tela.Child) tela.Node {
	c := []tela.Child{
		Class("absolute " + class),
		Style(tela.Left(x*g.u), tela.Top(y*g.u), tela.Width(w*g.u), tela.Height(h*g.u)),
	}
	return Div(append(c, kids...)...)
}

// bw is an arbitrary border width class for a width of the design.
func (g grid) bw(side string, w float32) string {
	if side != "" {
		side = side + "-"
	}
	return fmt.Sprintf("border-%s[%gpx]", side, w*g.u)
}

// circle is a ring (stroke > 0) or a disc of diameter d with its box at x, y.
func (g grid) circle(x, y, d, stroke float32) tela.Node {
	if stroke > 0 {
		return g.at(x, y, d, d, "rounded-full border-current "+g.bw("", stroke))
	}
	return g.at(x, y, d, d, "rounded-full bg-current")
}

// diamondClip draws the part of a square rotated by 45 degrees (a diamond of
// half diagonal r around cx, cy) that lies inside the box x, y, w, h: how
// triangles are made without clip-path.
func (g grid) diamondClip(x, y, w, h, cx, cy, r float32, fill string) tela.Node {
	side := r * 1.41421356
	return g.at(x, y, w, h, "overflow-hidden",
		Div(Class("absolute rotate-45 "+fill),
			Style(tela.Left((cx-x-side/2)*g.u), tela.Top((cy-y-side/2)*g.u), tela.Width(side*g.u), tela.Height(side*g.u))))
}

// chevron is a V of the given direction ("l", "r", "u", "d") with its apex
// at (ax, ay), legs of the given extent.
func (g grid) chevron(dir string, ax, ay, ext, stroke float32) tela.Node {
	// A square rotated 45 degrees; two of its borders form the V. The apex
	// is a diagonal half away from the square's center.
	half := ext / 1.41421356
	var cx, cy float32
	var sides [2]string
	switch dir {
	case "l":
		cx, cy, sides = ax+half, ay, [2]string{"l", "b"}
	case "r":
		cx, cy, sides = ax-half, ay, [2]string{"t", "r"}
	case "u":
		cx, cy, sides = ax, ay+half, [2]string{"t", "l"}
	default:
		cx, cy, sides = ax, ay-half, [2]string{"b", "r"}
	}
	return g.at(cx-ext/2, cy-ext/2, ext, ext, "rotate-45 border-current "+g.bw(sides[0], stroke)+" "+g.bw(sides[1], stroke))
}

// Glyph draws a monochrome icon by name at px pixels, in the current text
// color. Unknown names draw a plain square so a typo shows.
func Glyph(name string, px float32) tela.Node {
	g := newGrid(px)
	s := float32(1.4) // stroke
	switch name {
	case "back", "forward", "up":
		rot := map[string]float32{"back": 0, "forward": 180, "up": 90}[name]
		return Div(Class("relative shrink-0"), Style(tela.Width(16*g.u), tela.Height(16*g.u), tela.Rotate(rot)),
			g.at(2.4, 8-s/2, 11.6, s, "bg-current rounded-full"),
			g.chevron("l", 2.2, 8, 6.4, s),
		)
	case "refresh":
		return g.canvas("",
			g.at(2.4, 2.4, 11.2, 11.2, "rounded-full border-current border-t-transparent "+g.bw("", s)),
			g.chevron("r", 14.6, 6.4, 3.6, s),
		)
	case "chevron-right":
		return g.canvas("", g.chevron("r", 10.2, 8, 5.2, 1.3))
	case "chevron-down":
		return g.canvas("", g.chevron("d", 8, 10.2, 5.2, 1.3))
	case "chevron-up":
		return g.canvas("", g.chevron("u", 8, 5.8, 5.2, 1.3))
	case "search":
		return g.canvas("",
			g.circle(1.8, 1.8, 8.6, s),
			Div(Class("absolute rotate-45 rounded-full bg-current"),
				Style(tela.Left(9.4*g.u), tela.Top(11.3*g.u), tela.Width(5.6*g.u), tela.Height(s*g.u))),
		)
	case "plus":
		return g.canvas("",
			g.at(2.5, 8-s/2, 11, s, "bg-current rounded-full"),
			g.at(8-s/2, 2.5, s, 11, "bg-current rounded-full"),
		)
	case "home":
		return g.canvas("",
			g.diamondClip(0.6, 1.4, 14.8, 7.4, 8, 8.8, 7.4, "bg-current rounded-[1px]"),
			g.at(3, 8, 10, 6.6, "bg-current rounded-b-[1.5px]"),
		)
	case "desktop":
		return g.canvas("",
			g.at(1.4, 2.2, 13.2, 8.6, "rounded-[1.8px] border-current "+g.bw("", s)),
			g.at(6.3, 11.2, 3.4, 1.8, "bg-current"),
			g.at(4.2, 13.2, 7.6, 1.3, "bg-current rounded-full"),
		)
	case "documents":
		return g.canvas("",
			g.at(3, 1.4, 10, 13.2, "rounded-[1.8px] border-current "+g.bw("", s)),
			g.at(5.4, 5, 5.2, 1.2, "bg-current"),
			g.at(5.4, 7.6, 5.2, 1.2, "bg-current"),
			g.at(5.4, 10.2, 3, 1.2, "bg-current"),
		)
	case "downloads":
		return g.canvas("",
			g.at(7.3, 1.6, s, 8.4, "bg-current rounded-full"),
			g.chevron("d", 8, 11, 5.6, s),
			g.at(2, 11.6, 12, 3, "rounded-b-[2px] border-current "+g.bw("x", s)+" "+g.bw("b", s)),
		)
	case "music":
		return g.canvas("",
			g.circle(2.6, 9.4, 4.6, 0),
			g.at(6.1, 1.8, s, 9.6, "bg-current"),
			g.at(6.1, 1.8, 6.6, 3.2, "bg-current rounded-r-[2px]"),
		)
	case "pictures":
		return g.canvas("",
			g.at(1.4, 2.4, 13.2, 11.2, "overflow-hidden rounded-[2px] border-current "+g.bw("", s),
				g.circle(7.6, 2.2, 2.8, 0),
				g.diamondClip(0, 5.4, 6.8, 4.4, 3.4, 9.4, 4.2, "bg-current"),
				g.diamondClip(4.8, 6.4, 8.2, 3.6, 8.8, 10.4, 4.2, "bg-current"),
			),
		)
	case "videos":
		return g.canvas("",
			g.at(1.4, 3, 13.2, 10, "rounded-[2px] border-current "+g.bw("", s)),
			g.diamondClip(6.2, 5.4, 3.8, 5.2, 6.2, 8, 2.6, "bg-current"),
		)
	case "trash":
		return g.canvas("",
			g.at(2.4, 3.2, 11.2, s, "bg-current rounded-full"),
			g.at(6, 1.4, 4, 1.8, "rounded-t-[1.2px] border-current "+g.bw("x", s)+" "+g.bw("t", s)),
			g.at(3.6, 5, 8.8, 9.6, "rounded-b-[2px] border-current "+g.bw("x", s)+" "+g.bw("b", s)),
			g.at(6.3, 7, 1, 5, "bg-current"),
			g.at(8.7, 7, 1, 5, "bg-current"),
		)
	case "drive":
		return g.canvas("",
			g.at(1.4, 4.6, 13.2, 7.4, "rounded-[2px] border-current "+g.bw("", s)),
			g.circle(3.6, 7.6, 1.6, 0),
			g.at(8, 8.2, 4.2, 1, "bg-current opacity-60"),
		)
	case "sidebar":
		return g.canvas("",
			g.at(1.4, 2.4, 13.2, 11.2, "rounded-[2px] border-current "+g.bw("", s)),
			g.at(5.6, 2.4, s, 11.2, "bg-current"),
		)
	case "menu":
		return g.canvas("",
			g.at(2.2, 3.6, 11.6, s, "bg-current rounded-full"),
			g.at(2.2, 7.3, 11.6, s, "bg-current rounded-full"),
			g.at(2.2, 11, 11.6, s, "bg-current rounded-full"),
		)
	case "more":
		return g.canvas("",
			g.circle(2, 6.7, 2.6, 0), g.circle(6.7, 6.7, 2.6, 0), g.circle(11.4, 6.7, 2.6, 0),
		)
	case "view-list":
		return g.canvas("",
			g.at(2, 3, 2, 2, "bg-current rounded-[0.6px]"), g.at(6, 3.4, 8, 1.3, "bg-current rounded-full"),
			g.at(2, 7, 2, 2, "bg-current rounded-[0.6px]"), g.at(6, 7.4, 8, 1.3, "bg-current rounded-full"),
			g.at(2, 11, 2, 2, "bg-current rounded-[0.6px]"), g.at(6, 11.4, 8, 1.3, "bg-current rounded-full"),
		)
	case "view-grid":
		return g.canvas("",
			g.at(2, 2, 5, 5, "rounded-[1.2px] border-current "+g.bw("", 1.2)),
			g.at(9, 2, 5, 5, "rounded-[1.2px] border-current "+g.bw("", 1.2)),
			g.at(2, 9, 5, 5, "rounded-[1.2px] border-current "+g.bw("", 1.2)),
			g.at(9, 9, 5, 5, "rounded-[1.2px] border-current "+g.bw("", 1.2)),
		)
	case "cut":
		return g.canvas("",
			g.circle(2.2, 9.8, 4.2, 1.2), g.circle(9.6, 9.8, 4.2, 1.2),
			Div(Class("absolute rotate-[24deg] bg-current"), Style(tela.Left(5.6*g.u), tela.Top(1.2*g.u), tela.Width(1.2*g.u), tela.Height(9*g.u))),
			Div(Class("absolute -rotate-[24deg] bg-current"), Style(tela.Left(9.2*g.u), tela.Top(1.2*g.u), tela.Width(1.2*g.u), tela.Height(9*g.u))),
		)
	case "copy":
		return g.canvas("",
			g.at(5.6, 1.6, 8.6, 9.6, "rounded-tr-[2px] rounded-tl-[1px] border-current "+g.bw("t", 1.2)+" "+g.bw("r", 1.2)),
			g.at(1.8, 4.4, 8.8, 9.8, "rounded-[2px] border-current "+g.bw("", 1.2)),
		)
	case "paste":
		return g.canvas("",
			g.at(2.4, 2.8, 11.2, 11.8, "rounded-[2px] border-current "+g.bw("", 1.2)),
			g.at(5.2, 1.2, 5.6, 3, "rounded-[1.2px] bg-current"),
		)
	case "rename":
		return g.canvas("",
			g.at(1.4, 4, 13.2, 8, "rounded-[2px] border-current "+g.bw("", 1.2)),
			g.at(7.4, 6, 1.2, 4, "bg-current"),
		)
	case "share":
		return g.canvas("",
			g.at(2.4, 6.6, 11.2, 7.8, "rounded-[2px] border-current "+g.bw("x", 1.2)+" "+g.bw("b", 1.2)),
			g.at(7.4, 1.8, 1.3, 8.6, "bg-current"),
			g.chevron("u", 8, 1.6, 5, 1.3),
		)
	case "sort":
		return g.canvas("",
			g.at(2, 3.2, 12, 1.3, "bg-current rounded-full"),
			g.at(2, 7.4, 8.4, 1.3, "bg-current rounded-full"),
			g.at(2, 11.6, 4.8, 1.3, "bg-current rounded-full"),
		)
	}
	return g.canvas("bg-current")
}

// FolderIcon is the folder of the theme at px pixels.
func FolderIcon(l *Look, px float32) tela.Node {
	g := newGrid(px)
	return g.canvas("",
		g.at(1, 2.2, 6.4, 4, "rounded-t-[1.6px] "+l.FolderBack),
		g.at(1, 3.6, 14, 10.4, "rounded-[1.8px] "+l.FolderBack),
		g.at(1, 5.8, 14, 8.2, "rounded-[1.8px] "+l.FolderFront),
	)
}

// kindColor is the Look's accent for a kind of file.
func kindColor(l *Look, k Kind) string {
	switch k {
	case KindDoc:
		return l.KindDoc
	case KindSheet:
		return l.KindSheet
	case KindPDF:
		return l.KindPDF
	case KindImage:
		return l.KindImage
	case KindAudio:
		return l.KindAudio
	case KindVideo:
		return l.KindVideo
	case KindArchive:
		return l.KindArchive
	case KindCode:
		return l.KindCode
	}
	return l.KindText
}

// FileIcon is a dog-eared page with a mark for the kind of file.
func FileIcon(l *Look, k Kind, px float32) tela.Node {
	g := newGrid(px)
	ink := kindColor(l, k)
	const x0, x1, y0, y1, fold = 3, 13, 1, 15, 3.4
	var mark []tela.Child
	switch k {
	case KindImage:
		mark = []tela.Child{
			g.at(8.4, 5.8, 2, 2, "rounded-full "+ink),
			g.diamondClip(4, 8.6, 5, 5, 6, 12, 3.6, ink),
			g.diamondClip(7, 9.8, 5, 3.6, 9.6, 13.4, 3.6, ink),
		}
	case KindPDF:
		mark = []tela.Child{g.at(x0, 9.8, x1-x0, y1-9.8, ink+" rounded-b-[1.4px]")}
	case KindArchive:
		mark = []tela.Child{
			g.at(7, 3, 2, 1.3, ink), g.at(7, 5.3, 2, 1.3, ink), g.at(7, 7.6, 2, 1.3, ink),
			g.at(6.2, 10, 3.6, 2.8, "rounded-[0.8px] "+ink),
		}
	case KindAudio:
		mark = []tela.Child{
			g.at(8.6, 5.4, 1.2, 5.8, ink), g.at(8.6, 5.4, 2.6, 1.8, ink+" rounded-r-[1px]"),
			g.at(5.8, 9.6, 3.2, 2.6, "rounded-full "+ink),
		}
	case KindVideo:
		mark = []tela.Child{g.diamondClip(6, 6, 3.4, 5, 6, 8.5, 2.5, ink)}
	case KindCode:
		mark = []tela.Child{
			g.chevron("l", 4.8, 9.2, 3.4, 1.1),
			g.chevron("r", 11.2, 9.2, 3.4, 1.1),
		}
	case KindSheet:
		mark = []tela.Child{
			g.at(5, 6, 6, 1.1, ink), g.at(5, 8.4, 6, 1.1, ink), g.at(5, 10.8, 6, 1.1, ink), g.at(7.5, 6, 1.1, 5.9, ink),
		}
	case KindDoc:
		mark = []tela.Child{
			g.at(5, 6.2, 6, 1.1, ink), g.at(5, 8.4, 6, 1.1, ink), g.at(5, 10.6, 6, 1.1, ink), g.at(5, 12.6, 3.4, 1.1, ink),
		}
	default: // plain text
		mark = []tela.Child{
			g.at(5, 6.2, 6, 1.1, ink), g.at(5, 8.6, 6, 1.1, ink), g.at(5, 11, 4, 1.1, ink),
		}
	}
	edge := l.PageBody + " " + l.PageEdge
	kids := []tela.Child{
		g.at(x0, y0, x1-x0-fold, y1-y0, "rounded-l-[1.4px] "+edge+" "+g.bw("t", 1)+" "+g.bw("l", 1)+" "+g.bw("b", 1)),
		g.at(x1-fold, y0+fold, fold, y1-y0-fold, "rounded-br-[1.4px] "+edge+" "+g.bw("t", 1)+" "+g.bw("r", 1)+" "+g.bw("b", 1)),
		g.diamondClip(x1-fold, y0, fold, fold, x1-fold, y0+fold, fold, l.PageFlap),
	}
	return g.canvas("", append(kids, mark...)...)
}
