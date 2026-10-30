package h_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// End-to-end tests of VirtualList through telatest: real text that wraps,
// measured on the fly, and the pixels of a list scrolled to row 50,000
// (an offset of millions of logical pixels, at a fractional scale). The
// behavior of the list (the window, ScrollToIndex with every alignment,
// keys, focus, Tab, measurements) is checked in internal/core.

const vlCount = 100_000

var (
	vlNames = [...]string{"Ana Abbott", "Ben Bauer", "Chloé Castillo", "Dmitri Dubois", "Elena Eriksen", "Farid Fischer", "Grace García"}
	vlNote  = "Prefers asynchronous updates. Weekly office hours on Thursdays at 15:00 for questions about the data platform or onboarding."
)

// vlHasNote reports whether row i has a note (one row in seven): the rows
// are 65 px high without it, and taller when it wraps.
func vlHasNote(i int) bool { return i%7 == 3 }

func vlEstimate(i int) float32 {
	if vlHasNote(i) {
		return 110
	}
	return 65
}

type vlRow struct{ Index int }

func (r vlRow) Build() tela.Node {
	return Div(Class("flex items-start gap-3 border-b border-zinc-100 px-4 py-3 dark:border-zinc-800"),
		Div(Class("flex size-10 shrink-0 items-center justify-center rounded-full bg-linear-to-br from-violet-400 to-indigo-500 text-sm font-semibold text-white"),
			Text(vlNames[r.Index%len(vlNames)][:1])),
		Div(Class("flex min-w-0 flex-1 flex-col"),
			Div(Class("flex items-baseline gap-2"),
				P(Class("truncate text-sm font-medium"), Text(vlNames[r.Index%len(vlNames)])),
				Span(Class("ml-auto shrink-0 text-xs text-zinc-400 tabular-nums"), Textf("#%d", r.Index+1))),
			P(Class("truncate text-sm text-zinc-500"), Text("Staff engineer")),
			If(vlHasNote(r.Index),
				P(Class("mt-2 rounded-md bg-zinc-50 px-3 py-2 text-sm text-zinc-600 dark:bg-zinc-800/60 dark:text-zinc-300"), Text(vlNote)))))
}

// vlPage is a directory of 100,000 rows under a button that jumps to row
// 50,000, centered.
func vlPage() tela.Node {
	list := tela.UseElementRef()
	return Div(Class("flex h-full flex-col gap-2 bg-zinc-50 p-3 text-zinc-950 dark:bg-zinc-950 dark:text-zinc-50"),
		Button(Class("self-start rounded-md border border-zinc-200 bg-white px-3 py-1 text-sm dark:border-zinc-800 dark:bg-zinc-900"),
			OnClick(func() { list.ScrollToIndex(49_999, tela.AlignCenter) }), Text("Row 50,000")),
		VirtualList(vlCount, vlEstimate, func(i int) tela.Component { return vlRow{i} },
			Ref(list), AriaLabel("People"),
			Class("min-h-0 flex-1 rounded-xl border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900")))
}

func TestVirtualListGolden(t *testing.T) {
	app := telatest.Mount(t, tela.C(vlPage), telatest.Size(420, 340))
	app.Golden("virtuallist_top.png")
	if n := app.Count(telatest.ByRole("listitem", "")); n < 4 || n > 20 {
		t.Errorf("%d rows built of %d, want a screenful", n, vlCount)
	}
}

// TestVirtualListGoldenAfterJump is the list centered on row 50,000, in the
// dark scheme, at scale 1.5: a scroll offset of about 2.6 million logical
// pixels painted at a fractional scale.
func TestVirtualListGoldenAfterJump(t *testing.T) {
	app := telatest.Mount(t, tela.C(vlPage), telatest.Size(420, 340), telatest.Dark(), telatest.Scale(1.5))
	app.Click(telatest.ByRole("button", "Row 50,000"))
	app.Advance(2 * time.Second) // the overlay scrollbar fades
	app.ExpectText("#50000")
	app.Golden("virtuallist_jump_dark_x1.5.png")
}

// TestVirtualListMeasuresWrappedText checks rows whose text wraps: at 360 px
// the notes take several lines, so those rows are taller than their
// estimates, and the rows still follow one another with no gap or overlap.
func TestVirtualListMeasuresWrappedText(t *testing.T) {
	app := telatest.Mount(t, tela.C(vlPage), telatest.Size(360, 620))
	rows := telatest.ByRole("listitem", "")
	row := func(i int) telatest.Locator {
		return rows.Filter(telatest.HasText(fmt.Sprintf("#%d Staff engineer", i+1)))
	}
	app.Scroll(telatest.ByRole("list", "People"), 0, 1200)
	first := -1
	for i := 0; i < 60; i++ {
		if app.Count(row(i)) == 1 {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatal("no row built after scrolling")
	}
	taller := false
	for i := first; app.Count(row(i+1)) == 1; i++ {
		r := app.Bounds(row(i))
		if y := app.Bounds(row(i + 1)).Y; r.Bottom() < y-1 || r.Bottom() > y+1 {
			t.Fatalf("row %d ends at %v, row %d starts at %v", i+1, r.Bottom(), i+2, y)
		}
		if vlHasNote(i) && r.Height > 110+10 {
			taller = true
		}
	}
	if !taller {
		t.Error("no note row is taller than its estimate at 360 px")
	}
}
