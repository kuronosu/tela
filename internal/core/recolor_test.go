package core

import (
	"bytes"
	"strings"
	"testing"
)

// TestRecolorMatchesFreshLayout checks that a paragraph whose text colors
// change (its own, an inherited one, a span's and its underline's) is
// recolored without shaping, breaking its lines or laying it out again,
// and paints exactly the pixels of a tree mounted with those colors
// (ADR-161).
func TestRecolorMatchesFreshLayout(t *testing.T) {
	scene := func(color, spanColor string) Node {
		return div(Class("p-3 "+color),
			p(Key("p"), Class("w-40 text-sm"),
				Text("A paragraph that wraps over "),
				span(Class("underline "+spanColor), Text("a few")),
				Text(" lines keeps them.")))
	}
	var set func(string)
	app := func() Node {
		cls, s := UseStateSet("text-slate-900|text-sky-600")
		set = s
		color, spanColor, _ := strings.Cut(cls, "|")
		return scene(color, spanColor)
	}
	tr := newTestTree(t, 240, 160, Component(app), nil)
	tr.Frame()
	lines := mustFind(t, tr, "p").Lines
	s0 := tr.Stats()
	set("text-rose-500|decoration-amber-500 text-emerald-600")
	tr.Frame()
	d := work(s0, tr.Stats())
	if d.Shapes != 0 || d.Breaks != 0 {
		t.Errorf("a color change shaped %d paragraphs and broke %d, want 0 and 0", d.Shapes, d.Breaks)
	}
	if n := mustFind(t, tr, "p").Lines; n != lines || n < 2 {
		t.Errorf("%d lines after the color change, %d before (want the same, at least 2)", n, lines)
	}
	got := tr.Rasterize(tr.ReferenceFrame(false), 2)

	fresh := newTestTree(t, 240, 160, scene("text-rose-500", "decoration-amber-500 text-emerald-600"), nil)
	fresh.Frame()
	want := fresh.Rasterize(fresh.ReferenceFrame(false), 2)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Error("the recolored paragraph paints other pixels than a fresh one with the same colors")
	}
}
