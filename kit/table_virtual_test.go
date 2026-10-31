package kit_test

import (
	"cmp"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// vcols are the columns of the virtual test tables: three px columns, one
// that shares the rest, and a right-aligned amount that sorts. Every cell
// has a test ID, v-r<row>c<column>.
func vcols() []kit.Column[trow] {
	widths := []string{"90px", "110px", "", "100px"}
	out := make([]kit.Column[trow], len(widths))
	for c := range out {
		out[c] = kit.Column[trow]{
			Header: fmt.Sprintf("Col%d", c), Width: widths[c],
			Cell:      func(r trow) tela.Node { return h.Text(r.v[c]) },
			CellProps: func(r trow) tela.Child { return h.TestID(fmt.Sprintf("v-r%dc%d", r.i, c)) },
		}
	}
	out[3].Align = kit.AlignRight
	out[3].Sort = func(a, b trow) int { return cmp.Compare(a.v[3], b.v[3]) }
	out[3].Header = "Amount"
	return out
}

// vrows are n rows: the row number, a status, a name and an amount whose
// text order is the reverse of the row order, so sorting moves every row.
func vrows(n int) []trow {
	rows := make([]trow, n)
	for i := range rows {
		rows[i] = trow{i, []string{fmt.Sprintf("R%d", i), []string{"Paid", "Pending", "Unpaid"}[i%3], fmt.Sprintf("Customer %d", i), fmt.Sprintf("$%06d", n-i)}}
	}
	return rows
}

func vtable(n int, extra func(*kit.Table[trow])) kit.Table[trow] {
	t := kit.Table[trow]{Rows: vrows(n), RowKey: func(r trow) any { return r.i }, Columns: vcols(),
		RowHeight: 37, Class: "h-[300px] w-[600px] rounded-md border border-border", Props: h.TestID("v-table")}
	if extra != nil {
		extra(&t)
	}
	return t
}

func mountV(t testing.TB, c tela.Component, opts ...telatest.Option) *telatest.App {
	opts = append([]telatest.Option{telatest.Size(640, 340), telatest.Theme(kit.Theme()),
		telatest.RootClass("bg-background text-foreground"), telatest.InstantScroll()}, opts...)
	return telatest.MountFunc(t, func() tela.Node { return h.Div(h.Class("p-5"), c) }, opts...)
}

// A virtual table of thousands of rows builds the ones in view and a few
// more, and scrolling swaps them.
func TestTableVirtualBuildsOnlyTheWindow(t *testing.T) {
	app := mountV(t, vtable(5000, nil))
	rows := app.Count(telatest.ByRole("row", ""))
	if rows < 8 || rows > 30 {
		t.Errorf("a table of 5000 rows has %d rows in the tree, want the header and the 8 in view plus the overscan", rows)
	}
	app.ExpectCount(telatest.ByTestID("v-r0c0"), 1)
	if n := app.Count(telatest.ByTestID("v-r4999c0")); n != 0 {
		t.Errorf("the last row exists before scrolling (%d)", n)
	}
	app.Scroll(telatest.ByTestID("v-r0c0"), 0, 1e6)
	app.ExpectCount(telatest.ByTestID("v-r4999c0"), 1)
	if n := app.Count(telatest.ByTestID("v-r0c0")); n != 0 {
		t.Errorf("the first row is still built after scrolling to the end (%d)", n)
	}
	// The header stays in place.
	if b := app.Bounds(telatest.ByRole("columnheader", "Col0")); b.Height == 0 || b.Y > 60 {
		t.Errorf("the header is at %v after scrolling", b)
	}
	// The last row is flush with the bottom of the list.
	tb, last := app.Bounds(telatest.ByTestID("v-table")), app.Bounds(telatest.ByTestID("v-r4999c0"))
	if last.Y+last.Height > tb.Y+tb.Height || last.Y+last.Height < tb.Y+tb.Height-4 {
		t.Errorf("the last row ends at %v, the table at %v", last.Y+last.Height, tb.Y+tb.Height)
	}
}

// The virtual rows have the geometry of a plain table's: RowHeight 37 is the
// default look.
func TestTableVirtualMatchesThePlainTable(t *testing.T) {
	plain := func(tb *kit.Table[trow]) { tb.RowHeight = 0 }
	cols := func(tb *kit.Table[trow]) {
		tb.Columns[2].Width = "150px" // px columns only: the plain grid and the rows' agree
	}
	both := func(tb *kit.Table[trow]) { cols(tb); tb.Class = "h-[300px] w-[700px] rounded-md border border-border" }
	pa := mountV(t, vtable(40, func(tb *kit.Table[trow]) { both(tb); plain(tb) }))
	va := mountV(t, vtable(40, both))
	for _, id := range []string{"v-r0c0", "v-r0c3", "v-r1c1", "v-r4c2", "v-r5c3"} {
		pb, vb := pa.Bounds(telatest.ByTestID(id)), va.Bounds(telatest.ByTestID(id))
		if pb != vb {
			t.Errorf("%s: plain table %v, virtual table %v", id, pb, vb)
		}
	}
	ph, vh := pa.Bounds(telatest.ByRole("columnheader", "Amount")), va.Bounds(telatest.ByRole("columnheader", "Amount"))
	if ph != vh {
		t.Errorf("the Amount header: plain table %v, virtual table %v", ph, vh)
	}
}

func TestTableVirtualSorting(t *testing.T) {
	app := mountV(t, vtable(300, nil))
	// Unsorted: row 0 is first. Amount ascending puts the last row first.
	app.Click(telatest.ByRole("button", "Amount"))
	app.ExpectCount(telatest.ByTestID("v-r299c0"), 1)
	if n := app.Count(telatest.ByTestID("v-r0c0")); n != 0 {
		t.Errorf("row 0 is built in a table sorted by an amount that puts it last (%d)", n)
	}
	first, second := app.Bounds(telatest.ByTestID("v-r299c0")), app.Bounds(telatest.ByTestID("v-r298c0"))
	if !(first.Y < second.Y) {
		t.Errorf("row 299 is at y=%v, row 298 at y=%v: want ascending", first.Y, second.Y)
	}
	app.Click(telatest.ByRole("button", "Amount")) // descending: row 0 again
	app.ExpectCount(telatest.ByTestID("v-r0c0"), 1)
	app.Expect(telatest.ByRole("columnheader", "Amount"), telatest.Sort("descending"))
}

// A state in a cell follows its row through a sort (RowKey).
func vtoggle(id int) tela.Node {
	on, set := tela.UseState(false)
	label := fmt.Sprintf("row %d off", id)
	if on {
		label = fmt.Sprintf("row %d on", id)
	}
	return h.Button(h.OnClick(func() { set.Set(!on) }), h.Text(label))
}

func TestTableVirtualRowStateFollowsTheKey(t *testing.T) {
	cols := vcols()
	cols[1].Cell = func(r trow) tela.Node { return tela.CP(vtoggle, r.i) }
	// Twelve rows are all built (the window is the visible ones plus the
	// overscan), so a sort moves them and the state goes with its row; a row
	// that is scrolled out of the window is unmounted and forgets, as in
	// every virtual list.
	app := mountV(t, kit.Table[trow]{Rows: vrows(12), RowKey: func(r trow) any { return r.i }, Columns: cols, RowHeight: 37, Class: "h-[300px] w-[600px]"})
	app.Click(telatest.ByRole("button", "row 1 off"))
	app.ExpectCount(telatest.ByRole("button", "row 1 on"), 1)
	app.Click(telatest.ByRole("button", "Amount")) // ascending: the last row first
	on, off := app.Bounds(telatest.ByRole("button", "row 1 on")), app.Bounds(telatest.ByRole("button", "row 2 off"))
	if on.Y <= off.Y {
		t.Errorf("row 1 (y=%v) is not below row 2 (y=%v) after the sort", on.Y, off.Y)
	}
}

func TestTableVirtualSelectionAndRowHandlers(t *testing.T) {
	selected, clicked := 3, -1
	app := mountV(t, vtable(100, func(tb *kit.Table[trow]) {
		tb.Selected = func(r trow) bool { return r.i == selected }
		tb.RowProps = func(r trow) tela.Child {
			return tela.MergeProps(h.OnClick(func() { clicked = r.i }), h.TabIndex(0), h.AriaLabel(fmt.Sprintf("order %d", r.i)))
		}
	}))
	app.Expect(telatest.ByRole("row", "order 3"), telatest.Selected(true))
	app.Expect(telatest.ByRole("row", "order 2"), telatest.Selected(false))
	app.Click(telatest.ByTestID("v-r4c1"))
	if clicked != 4 {
		t.Errorf("a click on a cell of row 4 reached the handler of row %d", clicked)
	}
	// Tab goes from the sort button in the header to the first row.
	app.Focus(telatest.ByRole("button", "Amount"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("row", "order 0"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("row", "order 1"))
}

// The tree says how many rows the table has and which one each is.
func TestTableVirtualAria(t *testing.T) {
	_, host, _ := rawwin.Mount(t, tela.C(func() tela.Node {
		return h.Div(h.Class("p-5"), vtable(5000, func(tb *kit.Table[trow]) {
			tb.Columns[0].Footer = h.Text("Total")
			tb.Caption = "Orders"
		}))
	}), 640, 400, 0, core.Config{})
	find := func(role string) []*semantics.Node {
		var out []*semantics.Node
		host.Tree().Semantics().Walk(func(n *semantics.Node) bool {
			if n.Role == role {
				out = append(out, n)
			}
			return true
		})
		return out
	}
	table := find("table")
	if len(table) != 1 {
		t.Fatalf("%d tables", len(table))
	}
	if got, want := table[0].RowCount, 5000+1+1; got != want {
		t.Errorf("aria-rowcount is %d, want %d: the header, 5000 rows and the footer", got, want)
	}
	rows := find("row")
	if rows[0].RowIndex != 1 || rows[1].RowIndex != 2 || rows[2].RowIndex != 3 {
		t.Errorf("the first rows are %d %d %d, want 1 2 3", rows[0].RowIndex, rows[1].RowIndex, rows[2].RowIndex)
	}
	var seen []string
	for _, r := range rows {
		seen = append(seen, fmt.Sprint(r.RowIndex))
		if r.RowIndex == 0 {
			t.Errorf("a row without aria-rowindex among %v", seen)
		}
	}
	// The footer is the last row, after the 5000 that are not in the tree.
	if last := rows[len(rows)-1]; last.RowIndex != 5002 {
		t.Errorf("the footer row has aria-rowindex %d, want 5002 (%s)", last.RowIndex, strings.Join(seen, " "))
	}
}

func TestTableVirtualEmptyFooterAndCaption(t *testing.T) {
	app := mountV(t, vtable(0, func(tb *kit.Table[trow]) {
		tb.Empty = h.Text("No results.")
		tb.Caption = "Recent orders."
		tb.Columns[0].Footer = h.Text("Total")
	}))
	app.ExpectCount(telatest.ByText("No results."), 1)
	app.ExpectCount(telatest.ByText("Recent orders."), 1)
	app.ExpectCount(telatest.ByText("Total"), 1)
	tb := app.Bounds(telatest.ByTestID("v-table"))
	e, c := app.Bounds(telatest.ByText("No results.")), app.Bounds(telatest.ByText("Recent orders."))
	if e.X < tb.X || e.X+e.Width > tb.X+tb.Width {
		t.Errorf("the empty text %v is outside the table %v", e, tb)
	}
	if c.Y < e.Y {
		t.Errorf("the caption (y=%v) is above the empty row (y=%v)", c.Y, e.Y)
	}
}

// Columns wider than the table scroll sideways with the header.
func TestTableVirtualScrollsSideways(t *testing.T) {
	app := mountV(t, vtable(200, func(tb *kit.Table[trow]) {
		tb.Class = "h-[300px] w-[300px] rounded-md border border-border"
		tb.Columns[2].Width = "300px"
	}), telatest.Size(340, 340))
	h0, c0 := app.Bounds(telatest.ByRole("columnheader", "Col2")), app.Bounds(telatest.ByTestID("v-r0c2"))
	if h0.X != c0.X || h0.Width != c0.Width {
		t.Fatalf("at rest the header %v and the cell %v of a column differ", h0, c0)
	}
	app.Scroll(telatest.ByTestID("v-r0c2"), 120, 0)
	h1, c1 := app.Bounds(telatest.ByRole("columnheader", "Col2")), app.Bounds(telatest.ByTestID("v-r0c2"))
	if h1.X != c1.X || h1.Width != c1.Width {
		t.Errorf("after scrolling sideways the header %v and the cell %v of a column differ", h1, c1)
	}
	if h1.X >= h0.X {
		t.Errorf("the table did not scroll sideways: the column moved from %v to %v", h0.X, h1.X)
	}
}

func TestTableVirtualMistakesPanicInDevMode(t *testing.T) {
	cell := func(r trow) tela.Node { return h.Text(r.v[0]) }
	for _, tc := range []struct {
		name string
		tb   kit.Table[trow]
		want string
	}{
		{"sticky", kit.Table[trow]{RowHeight: 37, Columns: []kit.Column[trow]{{Header: "A", Sticky: true, Width: "100px", Cell: cell}}}, "does not support"},
		{"negative", kit.Table[trow]{RowHeight: -1, Columns: []kit.Column[trow]{{Header: "A", Cell: cell}}}, "RowHeight is -1"},
	} {
		tb := tc.tb
		_, err := telatest.AriaSnapshot(tela.C(func() tela.Node { return tb }))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
}

func TestTableVirtualGolden(t *testing.T) {
	for _, sc := range []string{"light", "dark"} {
		opts := []telatest.Option{}
		if sc == "dark" {
			opts = append(opts, telatest.Dark())
		}
		app := mountV(t, vtable(5000, func(tb *kit.Table[trow]) {
			tb.Selected = func(r trow) bool { return r.i == 1002 }
			tb.Caption = "5,000 orders, scrolled to row 1000."
			tb.Columns[0].Footer = h.Text("Total")
			tb.Columns[3].Footer = h.Text("$12,500,000")
			tb.Class = "h-[300px] w-[600px] rounded-md border border-border"
		}), opts...)
		app.Scroll(telatest.ByTestID("v-r0c0"), 0, 1000*37)
		app.MouseMove(2, 2)
		app.Advance(2 * time.Second) // the scrollbar fades
		app.GoldenScales("table_virtual_"+sc, 1, 2)
	}
}
