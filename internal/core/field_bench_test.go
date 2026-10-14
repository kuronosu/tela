package core

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
)

// benchFieldTree is a text field above the 10k-element list of the
// benchmarks: uncontrolled, or controlled by a state of the root
// component (Bind), which rebuilds it on every keystroke.
func benchFieldTree(b *testing.B, controlled bool) *Tree {
	setters := make([]func(string), benchRows)
	list := bigList(benchRows, setters)
	var root Node
	if controlled {
		root = Component(func() Node {
			v, set := UseState("")
			return div(input(Key("in"), Class("w-64 border px-2"), Bind(v, set)), list)
		})
	} else {
		root = div(input(Key("in"), Class("w-64 border px-2")), list)
	}
	tr := newTestTree(b, 800, 600, root, nil)
	tr.SetScale(2)
	tr.Frame()
	tr.focusElement(keyed(tr, "in"))
	tr.Frame()
	return tr
}

// benchType types one character (keydown with text, then keyup) and runs
// the frame, rasterizing its damage into fb at scale 2 (an 800x600
// window).
func benchType(b *testing.B, tr *Tree, fb *image.RGBA, r *raster.Renderer, c string) {
	k := platform.Key(c)
	tr.HandleKey(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeUnidentified, Key: k, Text: c})
	tr.HandleKey(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeUnidentified, Key: k})
	tr.Frame()
	tr.RasterizeFrame(fb, r, false)
	tr.FlushEffects()
}

func benchTyping(b *testing.B, controlled bool) {
	tr := benchFieldTree(b, controlled)
	fb := image.NewRGBA(image.Rect(0, 0, 1600, 1200))
	var r raster.Renderer
	tr.RasterizeFrame(fb, &r, true)
	f := keyed(tr, "in").box.fld
	s0 := tr.Stats()
	n := 0
	b.ReportAllocs()
	for b.Loop() {
		if n%40 == 39 {
			// Keep the value short: select all, then type over it.
			f.ed.Select(0, len(f.ed.Value()))
		}
		benchType(b, tr, fb, &r, string(rune('a'+n%26)))
		n++
	}
	s1 := tr.Stats()
	b.ReportMetric(float64(s1.Layouts-s0.Layouts)/float64(n), "layouts/op")
	b.ReportMetric(float64(s1.Builds-s0.Builds)/float64(n), "builds/op")
}

// BenchmarkTypingUncontrolled10k is a keystroke in an uncontrolled field
// over a 10k-element tree: the field's text is shaped again and its layer
// recorded and rasterized; nothing is built or laid out.
func BenchmarkTypingUncontrolled10k(b *testing.B) { benchTyping(b, false) }

// BenchmarkTypingControlled10k is the same keystroke in a field bound to
// a state of the root component (Bind): OnChange rebuilds the root, whose
// list is a reused node, so the frame builds one component and records
// every layer again.
func BenchmarkTypingControlled10k(b *testing.B) { benchTyping(b, true) }
