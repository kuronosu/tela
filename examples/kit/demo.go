package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// The pages of the gallery are made of demos: a small caption and a stage
// that holds the components, the way shadcn/ui's documentation shows them.
// Everything is laid out with h elements and the theme's tokens
// (bg-card, border-border, text-muted-foreground), so one set of classes
// serves both color schemes and both styles.

// Stage classes: a row that wraps, a column and a grid of columns.
const (
	row = "flex flex-wrap items-center gap-4"
	col = "flex flex-col gap-4"
	// colStart is a column whose items are as wide as they are, not stretched
	// to the stage: a button that opens something. A w-fit calendar needs it
	// too, since in a stretching column it is laid out taller than it should
	// be (see ADR-150).
	colStart = "flex flex-col items-start gap-4"
	// rowTop is a row whose items start at the top, such as a few calendars.
	rowTop = "flex flex-wrap items-start gap-6"
)

// demo is a captioned stage. class replaces the default layout of the
// stage (a wrapping row) when it is not empty.
func demo(title, note, class string, children ...tela.Child) tela.Node {
	if class == "" {
		class = row
	}
	return Div(Class("flex min-w-0 flex-col gap-3"),
		Div(Dir("ltr"), Class("flex flex-col gap-1"),
			H3(Class("text-sm leading-none font-medium"), Text(title)),
			If(note != "", P(Class("text-sm text-muted-foreground"), Text(note))),
		),
		Div(Class("min-w-0 rounded-xl border border-border bg-card/50 p-6 dark:bg-card/30", class), Fragment(children...)),
	)
}

// snippet is a few lines of Go in a stage, left to right and in the mono
// font whatever the language of the window is: code is not translated.
func snippet(code string) tela.Node {
	return Div(Dir("ltr"), Class("w-full overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 font-mono text-xs leading-relaxed whitespace-pre"),
		Text(strings.ReplaceAll(code, "\t", "    ")))
}

// grid lays out demos in two columns when the window is wide enough.
func grid(demos ...tela.Child) tela.Node {
	return Div(Class("grid grid-cols-1 items-start gap-x-8 gap-y-8 xl:grid-cols-2"), Fragment(demos...))
}

// stack lays out demos in one column.
func stack(demos ...tela.Child) tela.Node {
	return Div(Class("flex flex-col gap-8"), Fragment(demos...))
}

// ic is an icon of the given size class, in the text color.
func ic(icon tela.Icon, class ...string) tela.Node {
	return Icon(icon, Class("shrink-0"), Class(class...))
}

// muted is a line of secondary text.
func muted(s string) tela.Node {
	return P(Class("text-sm text-muted-foreground"), Text(s))
}

// readout is a small line that shows the state a demo has just changed:
// "Selected: October 25, 2026".
func readout(s string) tela.Node {
	return P(Class("text-sm text-muted-foreground tabular-nums"), Text(s))
}

// portraits are the pictures the Avatar demos use: a head and shoulders over
// a gradient, drawn here so that the gallery needs no image file and the
// pictures are the same on every run. Each has its own two colors.
var portraits = [][2]color.RGBA{
	{{0xfb, 0xbf, 0x24, 255}, {0xea, 0x58, 0x0c, 255}},
	{{0x38, 0xbd, 0xf8, 255}, {0x4f, 0x46, 0xe5, 255}},
	{{0x34, 0xd3, 0x99, 255}, {0x0d, 0x94, 0x88, 255}},
	{{0xf4, 0x72, 0xb6, 255}, {0x7c, 0x3a, 0xed, 255}},
}

var portraitBytes sync.Map

// portrait is the PNG of the i-th picture (a 96 px square).
func portrait(i int) []byte {
	if b, ok := portraitBytes.Load(i); ok {
		return b.([]byte)
	}
	const n = 96
	c := portraits[i%len(portraits)]
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	mix := func(a, b uint8, t float64) uint8 { return uint8(float64(a)*(1-t) + float64(b)*t) }
	for y := range n {
		for x := range n {
			t := float64(x+y) / float64(2*n)
			px := color.RGBA{mix(c[0].R, c[1].R, t), mix(c[0].G, c[1].G, t), mix(c[0].B, c[1].B, t), 255}
			// The head: a disc; the shoulders: a wide ellipse below it.
			dx, dy := float64(x)-48, float64(y)-38
			shoulders := (float64(x)-48)*(float64(x)-48)/(36*36) + (float64(y)-100)*(float64(y)-100)/(34*34)
			if dx*dx+dy*dy < 17*17 || shoulders < 1 {
				px = color.RGBA{0xff, 0xff, 0xff, 255}
				if dx*dx+dy*dy >= 17*17 {
					px = color.RGBA{0xf5, 0xf5, 0xf4, 255}
				}
			}
			img.SetRGBA(x, y, px)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	portraitBytes.Store(i, buf.Bytes())
	return buf.Bytes()
}
