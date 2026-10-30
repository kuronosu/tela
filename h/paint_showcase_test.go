package h_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/damagetest"
	"github.com/kuronosu/tela/internal/testutil/golden"
	"github.com/kuronosu/tela/telatest"
)

// The paint showcase is one static page of twelve cards that paints what
// elements and Tailwind classes can express: shadows (sm to 2xl, colored,
// inset), linear, radial and conic gradients, rings and inset rings,
// blur and backdrop-blur, opacity, rotated and per-corner rounded shapes,
// drop-shadow, dashed borders, clipped children and real text. It is the
// full-page paint golden of the framework: it began as the `shapes` example (the
// phase-1 scene drawn straight into a display list, which lives on in
// internal/gfx/raster/showcase_test.go).

const (
	showcaseW, showcaseH = 960, 640
)

// showcase is the page: twelve cards of a fixed size on a soft gradient,
// with two blurred glows in the corners. The cards wrap and center in the
// window: four columns at 960x640, three at about 700 px.
func showcase() tela.Node {
	cards := []tela.Node{
		psProfileCard(), psChartCard(), psPhotoCard(), psTogglesCard(),
		psProgressCard(), psSearchCard(), psGradientCard(), psSunsetCard(),
		psConicCard(), psGlassCard(), psShapesCard(), psDropCard(),
	}
	cells := []tela.Child{Class("relative flex min-h-screen flex-wrap content-center justify-center gap-6 p-5 md:p-10")}
	for i, c := range cards {
		cells = append(cells, Div(Key(i), Class("flex h-[170px] w-[202px]"), c))
	}
	return Div(Class("relative min-h-screen overflow-hidden bg-linear-to-br from-slate-50 to-indigo-50"),
		// Glows: blurred discs behind the grid, clipped by the page.
		Div(Class("absolute -top-40 -right-24 size-[28rem] rounded-full bg-indigo-200/60 blur-3xl")),
		Div(Class("absolute -bottom-40 -left-24 size-[28rem] rounded-full bg-sky-200/70 blur-3xl")),
		Div(cells...),
	)
}

// TestPaintShowcaseGolden renders the page at the size of a 960x640 window,
// at scales 1 and 2, bit by bit. Dev mode is on: an unknown or unsupported
// class, or any other dev warning, fails the test with core's message.
func TestPaintShowcaseGolden(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			_, img := core.Render(core.Component(showcase), core.RenderOptions{
				Config: core.Config{Dev: true, Log: func(m string) { t.Error(m) }},
				Width:  showcaseW, Height: showcaseH, Scale: scale,
			})
			golden.Image(t, fmt.Sprintf("paint_showcase_x%g", scale), img)
		})
	}
}

// TestPaintShowcaseNoDevMessages renders the page at several window sizes,
// where the cards wrap into other columns and the breakpoint changes the
// padding: no size may produce a class error or any other dev warning.
func TestPaintShowcaseNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{showcaseW, showcaseH}, {701, 846}, {700, 500}, {1280, 800}} {
		core.Render(core.Component(showcase), core.RenderOptions{
			Config: core.Config{Dev: true, Log: func(m string) { t.Errorf("%vx%v: %s", s[0], s[1], m) }},
			Width:  s[0], Height: s[1], Scale: 1,
		})
	}
}

// TestPaintShowcaseScrollDamage scrolls the page (blurred glows, backdrop
// filters, gradients) in a small window, checking every frame against a
// full rendering.
func TestPaintShowcaseScrollDamage(t *testing.T) {
	if testing.Short() {
		t.Skip("full frames of blurs")
	}
	damagetest.ScrollAround(t, func(opts ...telatest.Option) *telatest.App {
		return telatest.Mount(t, showcase(), opts...)
	}, 640, 420, 1, 8)
}

// showcaseWindow opens the page at 1416x846, scale 2, in a headless window.
func showcaseWindow(tb testing.TB) (*headless.Backend, *core.Host, *headless.Window) {
	b := headless.New(headless.Options{Size: platform.Size{W: 1416, H: 846}, Scale: 2})
	host, err := core.NewHost(b, core.Options{Root: tela.C(showcase)})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(host.Tree().Unmount)
	b.Drain(host)
	return b, host, host.Window().(*headless.Window)
}

// BenchmarkPaintShowcaseResize is a frame after a resize of the window at
// 1416x846, scale 2: a new layout and a full rasterization, where two
// blur-3xl glows dominate.
func BenchmarkPaintShowcaseResize(b *testing.B) {
	be, host, win := showcaseWindow(b)
	ws := [2]float32{1416, 1400}
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		i++
		win.Resize(platform.Size{W: ws[i&1], H: 846})
		be.Drain(host)
		be.Advance(be.FrameInterval())
		be.Drain(host)
	}
}

// psProfileCard: shadow-sm, a circular avatar (a clipped landscape with a
// ring), a name, a bio and two buttons.
func psProfileCard() tela.Node {
	return psCard("shadow-sm", "justify-between",
		Div(Class("flex items-center gap-3"),
			// The avatar clips its landscape with overflow-hidden.
			Div(Class("relative size-14 shrink-0 overflow-hidden rounded-full bg-linear-to-b from-sky-300 to-amber-200 ring-[3px] ring-indigo-500"),
				Div(Class("absolute top-2.5 right-3 size-4 rounded-full bg-amber-300")),
				Div(Class("absolute -bottom-4 -left-3 h-9 w-12 rounded-full bg-emerald-400")),
				Div(Class("absolute -right-3 -bottom-5 h-9 w-12 rounded-full bg-emerald-600")),
			),
			psTitle("Ada Lovelace", "Engineer"),
		),
		P(Class("text-xs leading-4 text-slate-500"), Text("Builds tools that make complex things feel simple.")),
		Div(Class("flex gap-2"),
			Div(Class("rounded-full bg-indigo-500 px-4 py-1 text-xs leading-4 font-medium text-white"), Text("Follow")),
			Div(Class("rounded-full bg-slate-100 px-3 py-1 text-xs leading-4 font-medium text-slate-500"), Text("Message")),
		),
	)
}

// psChartCard: shadow-md, a title, a badge and a bar chart made of flex items
// whose heights are percentages.
func psChartCard() tela.Node {
	heights := []string{"h-[34%]", "h-[52%]", "h-[44%]", "h-[72%]", "h-[58%]", "h-full", "h-[66%]"}
	return psCard("shadow-md", "gap-3",
		Div(Class("flex items-start justify-between"),
			psTitle("Revenue", "Last 7 days"),
			Div(Class("flex items-center gap-1 rounded-full bg-emerald-50 px-2 py-0.5 text-xs leading-4 font-medium text-emerald-600"),
				Div(Class("size-1.5 rounded-full bg-emerald-500")),
				Text("+12%"),
			),
		),
		Div(Class("flex flex-1 items-end gap-2"),
			Map(heights, func(h string) tela.Node {
				fill := "bg-indigo-100"
				if h == "h-full" {
					fill = "bg-linear-to-b from-indigo-400 to-indigo-600"
				}
				return Div(Key(h), Class("flex-1 rounded-t-md", fill, h))
			}),
		),
	)
}

// psPhotoCard: shadow-lg, a picture drawn with gradients (sky, sun and two
// waves, clipped by its rounded corners) and a round action button.
func psPhotoCard() tela.Node {
	return Div(Class(psCell, "flex flex-col justify-between rounded-2xl bg-white p-2.5 shadow-lg"),
		Div(Class("relative h-[88px] overflow-hidden rounded-xl bg-linear-to-b from-violet-400 to-pink-300"),
			Div(Class("absolute top-3 right-12 size-7 rounded-full bg-amber-200")),
			Div(Class("absolute -bottom-7 -left-6 h-14 w-[120%] rounded-[50%] bg-indigo-400")),
			Div(Class("absolute -right-8 -bottom-9 h-14 w-[120%] rounded-[50%] bg-indigo-600")),
		),
		Div(Class("flex items-end justify-between px-1.5 pb-1"),
			psTitle("Aurora", "Photo, 24 MB"),
			psArrowButton("size-9 bg-indigo-500 text-white"),
		),
	)
}

// psArrowButton is a round button with an arrow.
func psArrowButton(class string) tela.Node {
	return Div(Class("flex shrink-0 items-center justify-center rounded-full text-base font-semibold", class), Text("→"))
}

// psTogglesCard: shadow-xl, three switches: a rounded track with a white
// knob that has its own shadow.
func psTogglesCard() tela.Node {
	return psCard("shadow-xl", "justify-around",
		psToggle(true, "Notifications", "Push alerts"),
		psToggle(false, "Dark mode", "Follow system"),
		psToggle(true, "Autosave", "Every minute"),
	)
}

func psToggle(on bool, label, hint string) tela.Node {
	track := "bg-slate-300 justify-start"
	if on {
		track = "bg-indigo-500 justify-end"
	}
	return Div(Class("flex items-center gap-3"),
		Div(Class("flex h-[26px] w-[46px] shrink-0 items-center rounded-full p-0.5", track),
			Div(Class("size-[22px] rounded-full bg-white shadow-md")),
		),
		psTitle(label, hint),
	)
}

// psProgressCard: shadow-2xl, three labeled progress bars whose fills are
// gradients.
func psProgressCard() tela.Node {
	return psCard("shadow-2xl", "justify-around",
		psProgress("Design", "78%", "w-[78%]", "from-indigo-500 to-fuchsia-500"),
		psProgress("Research", "46%", "w-[46%]", "from-sky-400 to-emerald-400"),
		psProgress("Launch", "90%", "w-[90%]", "from-amber-400 to-rose-500"),
	)
}

func psProgress(label, value, width, colors string) tela.Node {
	return Div(Class("flex flex-col gap-1.5"),
		Div(Class("flex justify-between text-xs leading-4"),
			Span(Class("font-medium text-slate-700"), Text(label)),
			Span(Class("text-slate-400"), Text(value)),
		),
		Div(Class("h-2.5 rounded-full bg-slate-100"),
			Div(Class("h-full rounded-full bg-linear-to-r", colors, width)),
		),
	)
}

// psSearchCard: an inset shadow and ring (a well) holding a search box and
// raised chips.
func psSearchCard() tela.Node {
	type chip struct{ label, dot string }
	chips := []chip{{"UI", "bg-indigo-500"}, {"Go", "bg-emerald-500"}, {"Ops", "bg-rose-400"}, {"Docs", "bg-amber-400"}, {"AI", "bg-sky-500"}}
	return Div(Class(psCell, "flex flex-col gap-3 rounded-2xl bg-slate-100 p-3.5 inset-shadow-sm inset-ring inset-ring-slate-200"),
		Div(Class("flex items-center gap-2 rounded-full bg-white px-3 py-2 shadow-xs"),
			psMagnifier(),
			Span(Class("text-xs leading-4 text-slate-400"), Text("Search")),
		),
		Div(Class("flex flex-wrap gap-2"),
			Map(chips, func(c chip) tela.Node {
				return Div(Key(c.label), Class("flex items-center gap-1.5 rounded-full bg-white py-1 pr-2.5 pl-2 text-xs leading-4 font-medium text-slate-600 shadow-sm"),
					Div(Class("size-2.5 rounded-full", c.dot)),
					Text(c.label),
				)
			}),
		),
		P(Class("mt-auto text-xs leading-4 text-slate-400"), Text("5 filters")),
	)
}

// psMagnifier is a search icon made of a ring and a rotated handle.
func psMagnifier() tela.Node {
	return Div(Class("relative size-4 shrink-0"),
		Div(Class("absolute top-0 left-0 size-3 rounded-full border-2 border-slate-400")),
		Div(Class("absolute right-0 bottom-0.5 h-[7px] w-0.5 -rotate-45 rounded-full bg-slate-400")),
	)
}

// psGradientCard: bg-linear-to-br with a colored shadow, two translucent
// decorative circles clipped by the card, a badge and a round button.
func psGradientCard() tela.Node {
	return Div(Class(psCell, "relative flex flex-col justify-between overflow-hidden rounded-3xl bg-linear-to-br from-indigo-500 to-fuchsia-600 p-5 shadow-lg shadow-indigo-500/50"),
		Div(Class("absolute -top-14 left-[55%] size-32 rounded-full bg-white/12")),
		Div(Class("absolute -bottom-12 -left-10 size-36 rounded-full bg-white/10")),
		Div(Class("relative flex w-fit items-center gap-1.5 rounded-full border border-white/35 bg-white/15 py-1 pr-2.5 pl-2 text-xs leading-4 font-medium text-white"),
			Div(Class("size-1.5 rounded-full bg-white")),
			Text("New"),
		),
		Div(Class("relative flex items-end justify-between"),
			Div(
				P(Class("text-lg leading-6 font-bold text-white"), Text("Ship faster")),
				P(Class("text-xs text-white/70"), Text("Deploy in one click")),
			),
			psArrowButton("size-9 bg-white/95 text-indigo-600"),
		),
	)
}

// psSunsetCard: a radial gradient with a bright spot, and a frosted pill at
// the bottom that blurs what is behind it (backdrop-blur).
func psSunsetCard() tela.Node {
	return Div(Class(psCell, "relative flex flex-col justify-end overflow-hidden rounded-2xl bg-radial-[at_30%_28%] from-amber-200 via-rose-400 via-48% to-violet-600 p-3.5 shadow-xl shadow-rose-500/35"),
		Div(Class("absolute top-0 left-3 size-24 bg-radial-[circle_closest-side] from-white/85 to-white/0")),
		Div(Class("relative flex items-center gap-2.5 rounded-full border border-white/40 bg-white/20 px-2.5 py-2 backdrop-blur"),
			Div(Class("size-4 shrink-0 rounded-full bg-white")),
			Div(
				P(Class("text-xs leading-4 font-semibold text-white"), Text("Golden hour")),
				P(Class("text-[10px] leading-3 text-white/70"), Text("6:42 PM")),
			),
		),
	)
}

// psConicCard: a ring painted with a conic gradient (bg-conic on a circle with
// a white circle inside), with a knob at its start.
func psConicCard() tela.Node {
	return psCard("shadow-md", "items-center justify-center",
		Div(Class("relative flex size-28 items-center justify-center rounded-full bg-[conic-gradient(#f43f5e,#fbbf24,#34d399,#0ea5e9,#6366f1,#d946ef,#f43f5e)]"),
			Div(Class("flex size-[72px] flex-col items-center justify-center rounded-full bg-white"),
				Span(Class("text-lg leading-6 font-bold text-slate-800"), Text("72%")),
				Span(Class("text-[10px] leading-3 text-slate-400"), Text("of goal")),
			),
			Div(Class("absolute -top-1 left-1/2 flex size-[18px] -translate-x-1/2 items-center justify-center rounded-full bg-white shadow-md"),
				Div(Class("size-2 rounded-full bg-rose-500")),
			),
		),
	)
}

// psGlassCard: blurred color discs on a dark card, and a pane of frosted glass
// (backdrop-blur, translucent fill, hairline border) over them.
func psGlassCard() tela.Node {
	return Div(Class(psCell, "relative overflow-hidden rounded-[20px] bg-linear-to-br from-slate-900 to-slate-950 shadow-xl"),
		Div(Class("absolute top-2 left-2 size-24 rounded-full bg-indigo-500 blur-xl")),
		Div(Class("absolute top-0 right-3 size-20 rounded-full bg-fuchsia-500 blur-xl")),
		Div(Class("absolute -bottom-5 left-14 size-24 rounded-full bg-sky-500 blur-xl")),
		Div(Class("absolute bottom-2 left-0 size-12 rounded-full bg-amber-400 blur-xl")),
		Div(Class("absolute inset-x-[18px] top-[34px] bottom-[34px] flex flex-col gap-2.5 rounded-2xl border border-white/30 bg-white/15 p-3.5 shadow-lg backdrop-blur-xl"),
			Div(Class("flex items-center gap-2.5"),
				Div(Class("size-6 shrink-0 rounded-full bg-white/90")),
				Div(
					P(Class("text-xs leading-4 font-semibold text-white"), Text("Frosted glass")),
					P(Class("text-[10px] leading-3 text-white/60"), Text("backdrop-blur-xl")),
				),
			),
			P(Class("text-[11px] leading-4 text-white/70"), Text("Blurred color behind a hairline border.")),
		),
	)
}

// psShapesCard: shapes that need no paths. A burst (two rounded squares, one
// rotated) with a colored drop shadow, and a blob (a different radius on
// each corner) with a colored shadow.
func psShapesCard() tela.Node {
	return psCard("shadow-md", "justify-between",
		Div(Class("flex flex-1 items-center justify-around"),
			Div(Class("relative size-[76px] drop-shadow-lg drop-shadow-orange-400/60"),
				Div(Class("absolute inset-[9px] rounded-lg bg-linear-to-br from-amber-300 to-orange-400")),
				Div(Class("absolute inset-[9px] rotate-45 rounded-lg bg-linear-to-br from-amber-300 to-orange-400")),
			),
			Div(Class("size-[76px] rounded-tl-[70%] rounded-tr-[40%] rounded-br-[65%] rounded-bl-[45%] bg-linear-to-br from-emerald-400 to-sky-500 shadow-lg shadow-sky-500/50")),
		),
		Div(Class("flex justify-between text-xs leading-4 text-slate-400"),
			Span(Text("Burst")),
			Span(Text("Blob")),
		),
	)
}

// psDropCard: a dashed border around a drop zone. The two wavy strokes of the
// phase-1 scene become a waveform of rounded bars and a zigzag of rotated
// ones (pills of one width, tilted alternately, whose round ends meet).
func psDropCard() tela.Node {
	type bar struct {
		i        int
		h, color string
	}
	wave := []bar{
		{0, "h-3", "bg-sky-400"}, {1, "h-5", "bg-sky-400"}, {2, "h-8", "bg-sky-500"}, {3, "h-10", "bg-blue-500"},
		{4, "h-8", "bg-blue-500"}, {5, "h-5", "bg-indigo-500"}, {6, "h-3", "bg-indigo-500"}, {7, "h-5", "bg-violet-500"},
		{8, "h-8", "bg-violet-500"}, {9, "h-10", "bg-purple-500"}, {10, "h-8", "bg-purple-500"}, {11, "h-5", "bg-fuchsia-500"},
		{12, "h-3", "bg-fuchsia-500"},
	}
	type stroke struct {
		i    int
		tilt string
	}
	var zigzag []stroke
	for i := range 10 {
		tilt := "rotate-35"
		if i%2 == 1 {
			tilt = "-rotate-35"
		}
		zigzag = append(zigzag, stroke{i, tilt})
	}
	return Div(Class(psCell, "flex flex-col justify-around rounded-2xl border-2 border-dashed border-indigo-300 bg-white/60 px-5 py-4"),
		Div(Class("flex items-center justify-between"),
			Map(wave, func(b bar) tela.Node {
				return Div(Key(b.i), Class("w-1.5 rounded-full", b.h, b.color))
			}),
		),
		Div(Class("flex items-center justify-center"),
			Map(zigzag, func(s stroke) tela.Node {
				return Div(Key(s.i), Class("flex w-[15px] justify-center"),
					Div(Class("h-[26px] w-[5px] rounded-full bg-emerald-500", s.tilt)))
			}),
		),
		P(Class("text-center text-xs leading-4 text-indigo-400"), Text("Drop files here")),
	)
}

// psCell is what every card has: all the room of its cell.
const psCell = "min-w-0 flex-1"

// psCard is a white rounded card; shadow is a Tailwind shadow class and
// extra any other classes. Most cards share it, so they share their padding
// and radius.
func psCard(shadow, extra string, children ...tela.Child) tela.Node {
	return Div(append([]tela.Child{Class(psCell, "flex flex-col rounded-2xl bg-white p-4", shadow, extra)}, children...)...)
}

// psTitle is the two-line heading used by several cards.
func psTitle(text, sub string) tela.Node {
	return Div(
		P(Class("text-sm leading-5 font-semibold text-slate-800"), Text(text)),
		P(Class("text-xs text-slate-400"), Text(sub)),
	)
}
