package h_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/semantics"
)

type vrowProps struct{ I int }

var vrowBuilt = map[int]int{}

func vrow(p vrowProps) tela.Node {
	vrowBuilt[p.I]++
	return Div(Class("h-[30px]"), Textf("Row %d", p.I))
}

// TestVirtualListAPI drives h.VirtualList, h.Overscan and
// ElementRef.ScrollToIndex through the public API.
func TestVirtualListAPI(t *testing.T) {
	clear(vrowBuilt)
	var list tela.ElementRef
	root := func() tela.Node {
		list = tela.UseElementRef()
		return Div(Class("flex h-screen flex-col"),
			VirtualList(100_000,
				func(int) float32 { return 30 },
				func(i int) tela.Node { return tela.CP(vrow, vrowProps{i}) },
				Ref(list), Role("list"), AriaLabel("Rows"), Class("flex-1"), Overscan(2)),
		)
	}
	a := openWith(t, root, core.Config{InstantScroll: true})
	a.settle()
	snap := func() string { return semantics.Snapshot(a.h.Tree().Semantics()) }
	// 300 px of 30 px rows: rows 0..9 and two more.
	if got := strings.Count(snap(), "listitem"); got != 12 {
		t.Fatalf("%d rows built, want 12:\n%s", got, snap())
	}
	a.wheel(200, 150, 2) // 106 px: rows 3..13 visible
	a.settle()
	if _, y := list.ScrollOffset(); y != 106 {
		t.Errorf("offset %v after two notches", y)
	}
	for i := 0; i < 12; i++ {
		if vrowBuilt[i] != 1 {
			t.Errorf("row %d built %d times", i, vrowBuilt[i])
		}
	}
	list.ScrollToIndex(50_000, tela.AlignCenter)
	a.settle()
	if _, y := list.ScrollOffset(); y != 50_000*30+15-150 {
		t.Errorf("ScrollToIndex(50000, AlignCenter): offset %v, want %v", y, 50_000*30+15-150)
	}
	if s := snap(); !strings.Contains(s, "Row 50000") || strings.Contains(s, "Row 0\n") {
		t.Errorf("rows around 50000 not built:\n%s", s)
	}
	list.ScrollToIndex(-3, tela.AlignStart)
	a.settle()
	if _, y := list.ScrollOffset(); y != 0 {
		t.Errorf("ScrollToIndex(-3): offset %v", y)
	}
	if tela.AlignEnd.String() != "AlignEnd" {
		t.Errorf("AlignEnd prints %q", tela.AlignEnd)
	}
}
