package kit_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// stickySpec is a wide, tall table: two sticky columns of 100 and 120 px
// and four of 140 px, in a container of 360 by 200 px.
func stickySpec(rows int) tableSpec {
	data := make([][]string, rows)
	for i := range data {
		data[i] = []string{fmt.Sprintf("INV%03d", i+1), []string{"Paid", "Pending", "Unpaid"}[i%3], "Credit Card", "PayPal", "Bank Transfer", fmt.Sprintf("$%d.00", 100+i*50)}
	}
	return tableSpec{name: "s", cols: []tcol{
		{header: "Invoice", width: "100px", sticky: true},
		{header: "Status", width: "120px", sticky: true},
		{header: "Method", width: "140px"},
		{header: "Backup", width: "140px"},
		{header: "Account", width: "140px"},
		{header: "Amount", width: "140px", right: true, sort: true},
	}, rows: data, caption: "Sticky header and two sticky columns."}
}

func stickyTable(rows int, extra func(*kit.Table[trow])) tela.Component {
	t := stickySpec(rows).component()
	t.Class = "h-[200px] w-[360px] rounded-md border border-border"
	if extra != nil {
		extra(&t)
	}
	return t
}

func mountSticky(t *testing.T, c tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(400, 240), telatest.Theme(kit.Theme()), telatest.RootClass("bg-background text-foreground"),
		telatest.InstantScroll()}, opts...)
	return telatest.MountFunc(t, func() tela.Node { return h.Div(h.Class("p-5"), c) }, opts...)
}

func TestTableStickyHeaderAndColumns(t *testing.T) {
	app := mountSticky(t, stickyTable(20, nil))
	head := func(c int) tela.Rect { return app.Bounds(telatest.ByTestID(fmt.Sprintf("s-h%d", c))) }
	cell := func(r, c int) tela.Rect { return app.Bounds(telatest.ByTestID(fmt.Sprintf("s-r%dc%d", r, c))) }
	root := app.Bounds(telatest.ByTestID("s-table"))
	root.X, root.Y = root.X+1, root.Y+1 // inside the 1 px border

	// At rest the header is at the top and the columns at the left.
	if head(0).X != root.X || head(0).Y != root.Y {
		t.Fatalf("at rest the first header is at %v, the table at %v", head(0), root)
	}
	if head(1).X != root.X+100 || head(2).X != root.X+220 {
		t.Fatalf("at rest the columns are at %v %v", head(1), head(2))
	}
	row5 := cell(5, 0).Y

	// Scroll down 120 px and right 200 px.
	app.Scroll(telatest.ByTestID("s-table"), 200, 120)
	if h0 := head(0); h0.X != root.X || h0.Y != root.Y {
		t.Errorf("scrolled: the sticky first header moved to %v, want (%v, %v)", h0, root.X, root.Y)
	}
	if h1 := head(1); h1.X != root.X+100 || h1.Y != root.Y {
		t.Errorf("scrolled: the sticky second header is at %v, want x %v", h1, root.X+100)
	}
	if h2 := head(2); h2.Y != root.Y || h2.X >= root.X+220 {
		t.Errorf("scrolled: the third header is at %v: it keeps its row and scrolls sideways under the sticky ones", h2)
	}
	if c := cell(5, 0); c.X != root.X || c.Y != row5-120 {
		t.Errorf("scrolled: a sticky cell is at %v, want x %v and y %v", c, root.X, row5-120)
	}
	if c := cell(5, 1); c.X != root.X+100 {
		t.Errorf("scrolled: the second sticky column's cell is at x %v, want %v", c.X, root.X+100)
	}
	if c := cell(5, 5); c.X >= root.X+220 && c.X != root.X+100+120+140*3-200 {
		t.Errorf("scrolled: the last column is at x %v", c.X)
	}
}

func TestTableStickyHover(t *testing.T) {
	app := mountSticky(t, stickyTable(8, nil))
	c0 := app.Bounds(telatest.ByTestID("s-r2c0"))
	pixel := func() (r, g, b uint8) {
		img := app.Screenshot()
		p := img.RGBAAt(int(c0.X+c0.Width-3), int(c0.Y+3))
		return p.R, p.G, p.B
	}
	r0, g0, b0 := pixel()
	app.Hover(telatest.ByTestID("s-r2c3"))
	time.Sleep(300 * time.Millisecond) // the colors transition
	r1, g1, b1 := pixel()
	if r0 == r1 && g0 == g1 && b0 == b1 {
		t.Fatalf("hovering a row did not change the color of its sticky cell: %d %d %d", r0, g0, b0)
	}
	// The sticky cell is opaque: its hovered color is the background with
	// the row's muted/50 over it, the same as a cell that does not stick.
	c3 := app.Bounds(telatest.ByTestID("s-r2c3"))
	img := app.Screenshot()
	p := img.RGBAAt(int(c3.X+c3.Width-3), int(c3.Y+3))
	if int(p.R)-int(r1) > 1 || int(r1)-int(p.R) > 1 {
		t.Errorf("a hovered sticky cell is %d %d %d, a plain one %d %d %d", r1, g1, b1, p.R, p.G, p.B)
	}
}

func TestTableStickySheet(t *testing.T) {
	s := stickySpec(20)
	for _, sc := range []string{"light", "dark"} {
		opts := []telatest.Option{telatest.Size(420, 250), telatest.Theme(kit.Theme()), telatest.RootClass("bg-background text-foreground"), telatest.InstantScroll()}
		if sc == "dark" {
			opts = append(opts, telatest.Dark())
		}
		app := telatest.MountFunc(t, func() tela.Node {
			return h.Div(h.Class("p-5"), kit.Table[trow]{
				Rows: s.rowData(), RowKey: func(r trow) any { return r.i }, Columns: s.columns(), Caption: s.caption,
				Class: "h-[200px] w-[380px] rounded-md border border-border", Selected: func(r trow) bool { return r.i == 6 },
				Props: h.TestID("s-table"),
			})
		}, opts...)
		app.Scroll(telatest.ByTestID("s-table"), 150, 100)
		app.GoldenScales("table_sticky_"+sc, 1, 2)
	}
}

func BenchmarkTableMount1000(b *testing.B) {
	s := stickySpec(1000)
	for range b.N {
		app := telatest.MountFunc(b, func() tela.Node {
			return h.Div(h.Class("p-5"), kit.Table[trow]{Rows: s.rowData(), RowKey: func(r trow) any { return r.i }, Columns: s.columns(),
				Class: "h-[400px] w-[600px]"})
		}, telatest.Size(700, 460), telatest.Theme(kit.Theme()))
		_ = app
	}
}

func TestTableMount1000Rows(t *testing.T) {
	s := stickySpec(1000)
	start := time.Now()
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("p-5"), kit.Table[trow]{Rows: s.rowData(), RowKey: func(r trow) any { return r.i }, Columns: s.columns(),
			Class: "h-[400px] w-[600px]", Props: h.TestID("big")})
	}, telatest.Size(700, 460), telatest.Theme(kit.Theme()))
	mount := time.Since(start)
	start = time.Now()
	app.Scroll(telatest.ByTestID("big"), 0, 400)
	scroll := time.Since(start)
	start = time.Now()
	app.Click(telatest.ByRole("button", "Amount"))
	sorted := time.Since(start)
	t.Logf("1000 rows x 6 columns: mount %v, scroll %v, a click that sorts %v", mount, scroll, sorted)
	if mount > 5*time.Second {
		t.Errorf("mounting 1000 rows took %v", mount)
	}
}
