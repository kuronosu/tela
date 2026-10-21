package core

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/svg"
)

// iconList returns a list of 100 rows with an icon and a label, in a
// component whose state toggles the text color of the list.
func iconList(set *Setter[bool], icons bool) Node {
	return Component(func() Node {
		on, s := UseState(false)
		*set = s
		cls := "flex flex-col text-slate-700"
		if on {
			cls = "flex flex-col text-indigo-600"
		}
		rows := make([]Child, 0, 101)
		rows = append(rows, Class(cls))
		for i := range 100 {
			ic := div(Class("size-4"))
			if icons {
				ic = icon(checkSVG, Class("size-4 stroke-[1.5]"))
			}
			rows = append(rows, div(Key(i), Class("flex items-center gap-2 text-sm"), ic, Text(fmt.Sprintf("Row %d", i))))
		}
		return div(rows...)
	})
}

// BenchmarkIconListFrame measures a frame that recolors 100 icons (and
// their labels): restyle, a new record of each icon's document for the
// new currentColor, paint; no parse.
func BenchmarkIconListFrame(b *testing.B) { benchList(b, true) }

// BenchmarkPlainListFrame is BenchmarkIconListFrame with an empty div in
// place of each icon: the difference is what the icons cost.
func BenchmarkPlainListFrame(b *testing.B) { benchList(b, false) }

func benchList(b *testing.B, icons bool) {
	var set Setter[bool]
	tr := newTestTree(b, 400, 2400, iconList(&set, icons), nil)
	tr.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		set.Set(i%2 == 0)
		tr.Frame()
	}
}

// BenchmarkIconListRaster measures rasterizing the 100-icon list at scale
// 2 (the icons and the text).
func BenchmarkIconListRaster(b *testing.B) {
	var set Setter[bool]
	tr := newTestTree(b, 400, 2400, iconList(&set, true), nil)
	dl, _ := tr.Frame()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Rasterize(dl, 2)
	}
}

// BenchmarkSVGImgResize measures a frame that resizes an SVG Img: layout
// and paint, the recorded document placed at the new size (no parse, no
// new record).
func BenchmarkSVGImgResize(b *testing.B) {
	var set Setter[bool]
	app := Component(func() Node {
		big, s := UseState(false)
		set = s
		w := "w-64"
		if big {
			w = "w-96"
		}
		return img(SrcBytes([]byte(wideSVG)), Class(w))
	})
	tr := newTestTree(b, 800, 600, app, nil)
	tr.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		set.Set(i%2 == 0)
		tr.Frame()
	}
}

// BenchmarkIconPaint measures painting one 24 px icon from its recorded
// list (what each icon of a frame costs once restyled).
func BenchmarkIconPaint(b *testing.B) {
	doc, err := ParseIcon(checkSVG)
	if err != nil {
		b.Fatal(err)
	}
	list := doc.Record(svg.Options{CurrentColor: gfx.Black})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl := gfx.NewBuilder()
		doc.DrawRecorded(bl, gfx.RectXYWH(10, 10, 24, 24), list)
		bl.Finish()
	}
}
