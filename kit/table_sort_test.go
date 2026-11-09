package kit_test

import (
	"testing"

	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

var sortSpec = tableSpec{name: "t", cols: []tcol{{header: "Invoice", sort: true}, {header: "Status", sort: true}, {header: "Amount", right: true, sort: true}},
	rows: [][]string{
		{"INV001", "Paid", "$250.00"},
		{"INV002", "Pending", "$150.00"},
		{"INV003", "Paid", "$350.00"},
	}}

// sortCell is a table of sortSpec with edit applied, in a cell of its own.
func sortCell(name string, sort kit.SortState, edit func(*kit.Table[trow])) cell {
	s := sortSpec
	s.name, s.sort = name, sort
	t := s.component()
	if edit != nil {
		edit(&t)
	}
	return cell{name: name, width: 420, probes: []string{name + "-table"}, tela: t}
}

func tableSortSheet() *sheet {
	return &sheet{name: "table_sort", width: 900, cells: []cell{
		sortCell("default", kit.SortState{Column: "Invoice", Direction: kit.Ascending}, nil),
		sortCell("hide-unsorted", kit.SortState{Column: "Invoice", Direction: kit.Ascending}, func(t *kit.Table[trow]) { t.HideUnsortedIcon = true }),
		sortCell("other", kit.SortState{Column: "Status", Direction: kit.Other}, nil),
		sortCell("variants", kit.SortState{Column: "Status", Direction: kit.Other}, func(t *kit.Table[trow]) {
			t.HeadClass = "aria-[sort=none]:text-muted-foreground aria-[sort=other]:text-amber-600 aria-[sort=ascending]:text-primary aria-[sort=descending]:text-primary"
		}),
	}}
}

func TestTableSortGolden(t *testing.T) { tableSortSheet().golden(t) }

func tableSortRTLSheet() *sheet {
	s := tableSortSheet()
	s.name, s.rtl = "table_sort_rtl", true
	s.cells = s.cells[1:2]
	return s
}

func TestTableSortRTLGolden(t *testing.T) { tableSortRTLSheet().golden(t) }

// AlwaysSorted removes the unsorted step of the cycle.
func TestTableAlwaysSortedNeverUnsorts(t *testing.T) {
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.AlwaysSorted = true
		tb.DefaultSort = kit.SortState{Column: "Amount", Direction: kit.Ascending}
	}))
	head := telatest.ByRole("columnheader", "Amount")
	btn := telatest.ByRole("button", "Amount")
	app.Expect(head, telatest.Sort("ascending"))
	for i, want := range []string{"descending", "ascending", "descending", "ascending"} {
		app.Click(btn)
		app.Expect(head, telatest.Sort(want))
		if i == 0 {
			if got := order(app); got != "INV004 INV003 INV001 INV002" {
				t.Errorf("descending order %q", got)
			}
		}
	}
	// Another column starts ascending and the first is unsorted: a table
	// is sorted by one column.
	app.Click(telatest.ByRole("button", "Invoice"))
	app.Expect(telatest.ByRole("columnheader", "Invoice"), telatest.Sort("ascending"))
	app.Expect(head, telatest.Sort("none"))
	app.Click(telatest.ByRole("button", "Invoice"))
	app.Click(telatest.ByRole("button", "Invoice"))
	app.Expect(telatest.ByRole("columnheader", "Invoice"), telatest.Sort("ascending"))
}

// An unsorted table that must always be sorted starts when a header is
// pressed, and is sorted for good from then on.
func TestTableAlwaysSortedStartsUnsorted(t *testing.T) {
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) { tb.AlwaysSorted = true }))
	app.Expect(telatest.ByRole("columnheader", "Invoice"), telatest.Sort("none"))
	app.Click(telatest.ByRole("button", "Invoice"))
	app.Expect(telatest.ByRole("columnheader", "Invoice"), telatest.Sort("ascending"))
}

func TestTableAlwaysSortedReportsTheChange(t *testing.T) {
	var got []kit.SortState
	sorted := kit.SortState{Column: "Invoice", Direction: kit.Descending}
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.AlwaysSorted = true
		tb.Sort = &sorted
		tb.OnSortChange = func(s kit.SortState) { got = append(got, s) }
	}))
	app.Click(telatest.ByRole("button", "Invoice"))
	if len(got) != 1 || got[0] != (kit.SortState{Column: "Invoice", Direction: kit.Ascending}) {
		t.Errorf("OnSortChange got %v, want ascending", got)
	}
}

// HideUnsortedIcon keeps the arrow's place, so a header does not change
// width when it is sorted.
func TestTableHideUnsortedIconKeepsTheWidth(t *testing.T) {
	width := func(hide bool, sort kit.SortState) float32 {
		app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
			tb.HideUnsortedIcon = hide
			tb.DefaultSort = sort
		}))
		return app.Bounds(telatest.ByRole("button", "Status")).Width
	}
	unsorted, sorted := kit.SortState{}, kit.SortState{Column: "Status", Direction: kit.Ascending}
	if a, b := width(true, unsorted), width(false, unsorted); a != b {
		t.Errorf("a hidden arrow changes the unsorted header from %v to %v px", b, a)
	}
	if a, b := width(true, unsorted), width(true, sorted); a != b {
		t.Errorf("the header is %v px unsorted and %v px sorted", a, b)
	}
}

// The arrow of an unsorted header shows on hover, and the sorted one always.
func TestTableHideUnsortedIconShowsOnHover(t *testing.T) {
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.HideUnsortedIcon = true
		tb.DefaultSort = kit.SortState{Column: "Invoice", Direction: kit.Ascending}
	}), telatest.Size(600, 400))
	arrow := func(name string) bool {
		// The arrow is the part of the button right of its text; look for
		// ink in a column of pixels there.
		r := app.Bounds(telatest.ByRole("button", name))
		img := app.Screenshot()
		sc := float32(img.Bounds().Dx()) / 600
		x0, x1 := int((r.X+r.Width-30)*sc), int((r.X+r.Width-10)*sc)
		y0, y1 := int((r.Y+8)*sc), int((r.Y+r.Height-8)*sc)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if c := img.RGBAAt(x, y); c.R < 200 {
					return true
				}
			}
		}
		return false
	}
	if !arrow("Invoice") {
		t.Error("the sorted header shows no arrow")
	}
	if arrow("Status") {
		t.Error("an unsorted header shows its arrow with the hidden option on")
	}
	app.Hover(telatest.ByRole("columnheader", "Status"))
	if !arrow("Status") {
		t.Error("the arrow does not show when the pointer is over the header")
	}
}

func TestTableUnsortedHeadersSayNone(t *testing.T) {
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.DefaultSort = kit.SortState{Column: "Invoice", Direction: kit.Descending}
	}))
	app.Expect(telatest.ByRole("columnheader", "Invoice"), telatest.Sort("descending"))
	app.Expect(telatest.ByRole("columnheader", "Status"), telatest.Sort("none"))
	// The variant of "none" is for the headers that say it: a sortable one.
	app = mountTable(t, invoiceTable(func(tb *kit.Table[trow]) { tb.HeadClass = "aria-[sort=none]:w-40" }))
	if w := app.Bounds(telatest.ByRole("columnheader", "Status")).Width; w != 160 {
		t.Errorf("aria-[sort=none]: did not match a sortable, unsorted header: %v px", w)
	}
	if w := app.Bounds(telatest.ByRole("columnheader", "Method")).Width; w == 160 {
		t.Error("aria-[sort=none]: matched a column that cannot be sorted")
	}
}

// A column sorted by an order the table does not know says "other": it
// leaves the rows as given and the next press starts ascending.
func TestTableSortOther(t *testing.T) {
	var got []kit.SortState
	state := kit.SortState{Column: "Status", Direction: kit.Other}
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.Sort = &state
		tb.OnSortChange = func(s kit.SortState) { got = append(got, s) }
		tb.HeadClass = "aria-[sort=other]:w-40"
	}))
	app.Expect(telatest.ByRole("columnheader", "Status"), telatest.Sort("other"))
	app.Expect(telatest.ByRole("columnheader", "Invoice"), telatest.Sort("none"))
	if got := order(app); got != "INV001 INV002 INV003 INV004" {
		t.Errorf("the rows were reordered for another order: %q", got)
	}
	if w := app.Bounds(telatest.ByRole("columnheader", "Status")).Width; w != 160 {
		t.Errorf("aria-[sort=other]: did not match: %v px", w)
	}
	app.Click(telatest.ByRole("button", "Status"))
	if len(got) != 1 || got[0] != (kit.SortState{Column: "Status", Direction: kit.Ascending}) {
		t.Errorf("OnSortChange got %v, want Status ascending", got)
	}
}

func TestTableSortVariantsOnTheHeader(t *testing.T) {
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.HeadClass = "aria-[sort=ascending]:w-40 aria-[sort=descending]:w-48"
	}))
	head := telatest.ByRole("columnheader", "Amount")
	app.Click(telatest.ByRole("button", "Amount"))
	if w := app.Bounds(head).Width; w != 160 {
		t.Errorf("ascending: %v px", w)
	}
	app.Click(telatest.ByRole("button", "Amount"))
	if w := app.Bounds(head).Width; w != 192 {
		t.Errorf("descending: %v px", w)
	}
}
