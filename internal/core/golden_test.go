package core

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// Goldens of the phase-3 pipeline: backgrounds and text only (borders,
// radii, shadows, gradients and effects are painted from T4b on, which
// will update these images). Regenerate with TELA_UPDATE=1 and look at
// them.

// goldenCounter is the Counter of ARCHITECTURE section 1, on a dark page
// so that its white text shows while gradients are not painted.
func goldenCounter() Node {
	counter := func() Node {
		count, setCount := UseState(0)
		return div(Class("flex flex-col items-center gap-6 p-10 rounded-3xl "+
			"bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl"),
			h1(Class("text-5xl font-bold text-white tracking-tight"),
				Textf("Count: %d", count)),
			button(Class("px-6 py-3 rounded-xl bg-white/15 text-white backdrop-blur "+
				"hover:bg-white/25 active:scale-95 transition"),
				On("click", func() { setCount(count + 1) }),
				Text("Increment"),
			),
		)
	}
	return div(Class("flex min-h-screen items-center justify-center bg-slate-900"), Component(counter))
}

func goldenParagraph() Node {
	return div(Class("p-6"),
		p(Class("text-lg leading-7 text-slate-800"),
			Text("Tela draws "), strong(Text("every pixel")), Text(" in "), em(Text("pure Go")),
			Text(": build with "), code(Class("text-pink-600"), Text("CGO_ENABLED=0")),
			Text(", style with "), span(Class("text-indigo-600 underline"), Text("Tailwind classes")),
			Text(" and ship "), span(Class("font-semibold text-emerald-600"), Text("one binary")), Text("."),
		),
		p(Class("mt-4 text-sm text-slate-500 text-center"),
			Text("Centered, smaller and muted text that wraps onto more than one line in a narrow column."),
		),
		div(Class("mt-4 bg-amber-100 px-3 py-2 text-amber-900"),
			Text("Mixed "), strong(Text("inline")), Text(" content"),
			div(Class("my-1 bg-amber-300 text-xs uppercase tracking-widest"), Text("a block in between")),
			Text("gets anonymous paragraph boxes."),
		),
	)
}

func goldenFlex() Node {
	shades := []string{"bg-sky-300", "bg-sky-400", "bg-sky-500", "bg-sky-600", "bg-sky-700",
		"bg-indigo-300", "bg-indigo-400", "bg-indigo-500", "bg-indigo-600", "bg-indigo-700"}
	return div(Class("flex min-h-screen flex-col gap-3 bg-slate-100 p-4"),
		div(Class("flex h-16 gap-3"),
			div(Class("w-16 bg-rose-500")), div(Class("grow bg-amber-400")), div(Class("w-24 bg-sky-500"))),
		div(Class("flex h-20 items-center justify-between bg-white px-3"),
			div(Class("h-10 w-10 bg-violet-500")), div(Class("h-6 w-14 bg-violet-400")), div(Class("h-14 w-10 bg-violet-600"))),
		div(Class("flex flex-wrap gap-2"),
			Map(shades, func(c string) Node { return div(Key(c), Class("h-12 w-12 "+c)) })),
		div(Class("flex h-24 gap-3"),
			div(Class("flex-1 bg-emerald-500")),
			div(Class("flex-2 bg-emerald-400")),
			div(Class("flex w-24 flex-col gap-1"), div(Class("grow bg-teal-300")), div(Class("h-6 bg-teal-600")))),
		div(Class("flex items-end gap-2 bg-white p-2"),
			div(Class("bg-orange-200 px-2 text-sm"), Text("end")),
			div(Class("bg-orange-300 px-2 text-2xl"), Text("aligned")),
			div(Class("bg-orange-400 px-2 text-base"), Text("items"))),
	)
}

func TestGoldens(t *testing.T) {
	for _, tc := range []struct {
		name string
		root func() Node
		w, h float32
	}{
		{"counter", goldenCounter, 400, 300},
		{"paragraph", goldenParagraph, 360, 340},
		{"flex", goldenFlex, 360, 480},
	} {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s_x%g", tc.name, scale), func(t *testing.T) {
				_, img := Render(tc.root(), RenderOptions{
					Config: Config{Fonts: testFonts},
					Width:  tc.w, Height: tc.h, Scale: scale,
				})
				golden.Image(t, fmt.Sprintf("%s_x%g", tc.name, scale), img)
			})
		}
	}
}

func TestRenderHeadless(t *testing.T) {
	dl, img := Render(div(Class("bg-sky-500 p-2"), Text("hi")), RenderOptions{
		Config: Config{Fonts: testFonts, RootClass: "bg-slate-900"},
		Width:  101, Height: 51, Scale: 1.5,
	})
	// The framebuffer rule of the platform: logical x scale, rounded half
	// away from zero.
	if b := img.Bounds(); b.Dx() != 152 || b.Dy() != 77 {
		t.Errorf("image %v, want 152x77", b)
	}
	if dl.Len() < 3 {
		t.Fatalf("display list:\n%v", dl)
	}
	if c, ok := dl.At(0).(gfx.FillRect); !ok || c.Rect != (gfx.Rect{X1: 101, Y1: 51}) || c.Paint.(gfx.Color).String() != "#0f172b" {
		t.Errorf("first command %#v, want the canvas fill", dl.At(0))
	}
	runs := 0
	for _, c := range dl.All() {
		if _, ok := c.(gfx.DrawGlyphRun); ok {
			runs++
		}
	}
	if runs != 1 {
		t.Errorf("%d glyph runs, want 1", runs)
	}
	// The bottom-right pixel, outside the content, is the canvas.
	if c := img.RGBAAt(151, 76); c.R != 0x0f || c.G != 0x17 || c.B != 0x2b {
		t.Errorf("corner pixel %v, want the canvas color", c)
	}
	// An empty Render is a white canvas of the default size.
	_, img = Render(Node{}, RenderOptions{Config: Config{Fonts: testFonts}})
	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 600 || img.RGBAAt(400, 300).R != 255 {
		t.Errorf("empty render %v %v", b, img.RGBAAt(400, 300))
	}
}
