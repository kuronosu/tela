package kit_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// invoiceTable is a table of four invoices with sortable Invoice, Status
// and Amount columns; extra edits it.
func invoiceTable(extra func(*kit.Table[trow])) tela.Component {
	spec := tableSpec{name: "t", cols: []tcol{{header: "Invoice", sort: true}, {header: "Status", sort: true}, {header: "Method"}, {header: "Amount", right: true, sort: true}},
		rows: [][]string{
			{"INV001", "Paid", "Credit Card", "$250.00"},
			{"INV002", "Pending", "PayPal", "$150.00"},
			{"INV003", "Paid", "Bank Transfer", "$350.00"},
			{"INV004", "Pending", "Credit Card", "$450.00"},
		}}
	t := spec.component()
	if extra != nil {
		extra(&t)
	}
	return t
}

func mountTable(t *testing.T, c tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(600, 400), telatest.Theme(kit.Theme())}, opts...)
	return telatest.MountFunc(t, func() tela.Node { return h.Div(h.Class("p-4"), c) }, opts...)
}

// order is the invoice numbers of the body rows, in the order shown.
func order(app *telatest.App) string {
	var ids []string
	for _, line := range strings.Split(app.AriaSnapshot(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `- row "INV`) {
			ids = append(ids, line[len(`- row "`):len(`- row "INV001`)])
		}
	}
	return strings.Join(ids, " ")
}

func TestTableSortCycle(t *testing.T) {
	app := mountTable(t, invoiceTable(nil))
	head := telatest.ByRole("columnheader", "Amount")
	btn := telatest.ByRole("button", "Amount")
	if got := order(app); got != "INV001 INV002 INV003 INV004" {
		t.Fatalf("unsorted order %q", got)
	}
	app.Expect(head, telatest.Sort("none"))
	steps := []struct{ sort, order string }{
		{"ascending", "INV002 INV001 INV003 INV004"},
		{"descending", "INV004 INV003 INV001 INV002"},
		{"none", "INV001 INV002 INV003 INV004"},
		{"ascending", "INV002 INV001 INV003 INV004"},
	}
	for i, s := range steps {
		app.Click(btn)
		app.Expect(head, telatest.Sort(s.sort))
		if got := order(app); got != s.order {
			t.Errorf("click %d: order %q, want %q", i+1, got, s.order)
		}
	}
	// Another column starts ascending and takes the sort from the first.
	app.Click(telatest.ByRole("button", "Invoice"))
	app.Expect(telatest.ByRole("columnheader", "Invoice"), telatest.Sort("ascending"))
	app.Expect(head, telatest.Sort("none"))
}

func TestTableSortByKeyboard(t *testing.T) {
	app := mountTable(t, invoiceTable(nil))
	btn := telatest.ByRole("button", "Amount")
	app.Focus(btn)
	app.Press("Enter")
	app.Expect(telatest.ByRole("columnheader", "Amount"), telatest.Sort("ascending"))
	app.Press("Space")
	app.Expect(telatest.ByRole("columnheader", "Amount"), telatest.Sort("descending"))
	// Tab order: Method has no button, so Shift+Tab from Amount goes to Status.
	app.Press("Shift+Tab")
	app.ExpectFocused(telatest.ByRole("button", "Status"))
}

func TestTableSortIsStable(t *testing.T) {
	app := mountTable(t, invoiceTable(nil))
	app.Click(telatest.ByRole("button", "Status"))
	// Paid: 1, 3; Pending: 2, 4.
	if got := order(app); got != "INV001 INV003 INV002 INV004" {
		t.Errorf("ascending %q", got)
	}
	app.Click(telatest.ByRole("button", "Status"))
	// Descending keeps the original order among equal rows.
	if got := order(app); got != "INV002 INV004 INV001 INV003" {
		t.Errorf("descending %q", got)
	}
}

func TestTableControlledSort(t *testing.T) {
	var reported []kit.SortState
	state := kit.SortState{Column: "Invoice", Direction: kit.Descending}
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(invoiceTable(func(tb *kit.Table[trow]) {
			tb.Sort = &state
			tb.OnSortChange = func(s kit.SortState) { reported = append(reported, s) }
		}))
	}, telatest.Size(600, 400), telatest.Theme(kit.Theme()))
	if got := order(app); got != "INV004 INV003 INV002 INV001" {
		t.Fatalf("order %q", got)
	}
	// The caller does not store the new state: the table stays as it is.
	app.Click(telatest.ByRole("button", "Amount"))
	if len(reported) != 1 || reported[0] != (kit.SortState{Column: "Amount", Direction: kit.Ascending}) {
		t.Fatalf("reported %v", reported)
	}
	app.Expect(telatest.ByRole("columnheader", "Invoice"), telatest.Sort("descending"))
	if got := order(app); got != "INV004 INV003 INV002 INV001" {
		t.Errorf("the table followed a change the caller refused: %q", got)
	}
}

func TestTableControlledSortFollowsState(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		sort, setSort := tela.UseState(kit.SortState{})
		return h.Div(invoiceTable(func(tb *kit.Table[trow]) {
			tb.Sort = &sort
			tb.OnSortChange = setSort.Set
		}))
	}, telatest.Size(600, 400), telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Amount"))
	app.Click(telatest.ByRole("button", "Amount"))
	app.Expect(telatest.ByRole("columnheader", "Amount"), telatest.Sort("descending"))
	if got := order(app); got != "INV004 INV003 INV001 INV002" {
		t.Errorf("order %q", got)
	}
}

func TestTableUncontrolledReportsAndDefault(t *testing.T) {
	var reported []kit.SortState
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.DefaultSort = kit.SortState{Column: "Amount", Direction: kit.Descending}
		tb.OnSortChange = func(s kit.SortState) { reported = append(reported, s) }
	}))
	app.Expect(telatest.ByRole("columnheader", "Amount"), telatest.Sort("descending"))
	if got := order(app); got != "INV004 INV003 INV001 INV002" {
		t.Errorf("default sort order %q", got)
	}
	app.Click(telatest.ByRole("button", "Amount"))
	if len(reported) != 1 || reported[0] != (kit.SortState{}) {
		t.Errorf("reported %v, want the unsorted state", reported)
	}
	if got := order(app); got != "INV001 INV002 INV003 INV004" {
		t.Errorf("order after the third state %q", got)
	}
}

// A column without a Sort func but Sortable shows and reports the state and
// leaves the rows alone.
func TestTableManualSort(t *testing.T) {
	var reported kit.SortState
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.Columns[2].Sortable = true
		tb.OnSortChange = func(s kit.SortState) { reported = s }
	}))
	app.Click(telatest.ByRole("button", "Method"))
	app.Expect(telatest.ByRole("columnheader", "Method"), telatest.Sort("ascending"))
	if reported.Column != "Method" {
		t.Errorf("reported %v", reported)
	}
	if got := order(app); got != "INV001 INV002 INV003 INV004" {
		t.Errorf("rows were reordered: %q", got)
	}
}

func TestTableAriaSnapshot(t *testing.T) {
	app := mountTable(t, tableSpec{name: "a", caption: "Recent invoices.", cols: invoiceCols(""), rows: invoiceRows[:2], selected: 2,
		footer: []tfoot{{"Total", 3}, {"$400.00", 1}}}.component())
	// What Playwright 1.61.1 prints for the equivalent HTML table (a
	// <table> with a <caption>, <thead>, <tbody>, <tfoot>, and a row with
	// aria-selected), compared by hand with its output.
	app.ExpectAria(`
		- table "Recent invoices.":
		  - caption: Recent invoices.
		  - rowgroup:
		    - row "Invoice Status Method Amount":
		      - columnheader "Invoice"
		      - columnheader "Status"
		      - columnheader "Method"
		      - columnheader "Amount"
		  - rowgroup:
		    - row "INV001 Paid Credit Card $250.00":
		      - cell "INV001"
		      - cell "Paid"
		      - cell "Credit Card"
		      - cell "$250.00"
		    - row "INV002 Pending PayPal $150.00" [selected]:
		      - cell "INV002"
		      - cell "Pending"
		      - cell "PayPal"
		      - cell "$150.00"
		  - rowgroup:
		    - row "Total $400.00":
		      - cell "Total"
		      - cell "$400.00"
	`)
}

func TestTableAriaSnapshotSortable(t *testing.T) {
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.AriaLabel = "Invoices"
		tb.Rows = tb.Rows[:1]
		tb.DefaultSort = kit.SortState{Column: "Invoice", Direction: kit.Ascending}
	}))
	app.ExpectAria(`
		- table "Invoices":
		  - rowgroup:
		    - row "Invoice Status Method Amount":
		      - columnheader "Invoice":
		        - button "Invoice"
		      - columnheader "Status":
		        - button "Status"
		      - columnheader "Method"
		      - columnheader "Amount":
		        - button "Amount"
		  - rowgroup:
		    - row "INV001 Paid Credit Card $250.00":
		      - cell "INV001"
		      - cell "Paid"
		      - cell "Credit Card"
		      - cell "$250.00"
	`)
}

// counter is a component with state of its own, to see it follow its row.
type counter struct{ label string }

func (c counter) Build() tela.Node {
	n, set := tela.UseState(0)
	return h.Button(h.AriaLabel("count "+c.label), h.OnClick(func() { set.Set(n + 1) }), h.Textf("%s:%d", c.label, n))
}

func TestTableRowStateFollowsTheRow(t *testing.T) {
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.Columns[2].Cell = func(r trow) tela.Node { return h.Fragment(counter{r.v[0]}) }
	}))
	app.Click(telatest.ByRole("button", "count INV002"))
	app.Click(telatest.ByRole("button", "count INV002"))
	app.Click(telatest.ByRole("button", "Amount")) // INV002 ($150) comes first
	if got := order(app); got != "INV002 INV001 INV003 INV004" {
		t.Fatalf("order %q", got)
	}
	app.ExpectText("INV002:2")
	app.ExpectText("INV001:0")
}

func TestTableEmpty(t *testing.T) {
	app := mountTable(t, invoiceTable(func(tb *kit.Table[trow]) {
		tb.Rows = nil
		tb.Empty = h.Text("No results.")
	}))
	app.ExpectText("No results.")
	b := app.Bounds(telatest.ByText("No results."))
	tb := app.Bounds(telatest.ByRole("table", ""))
	if b.Width == 0 || b.X < tb.X || b.X+b.Width > tb.X+tb.Width {
		t.Errorf("the empty text %v is outside the table %v", b, tb)
	}
}

func TestTableNoColumns(t *testing.T) {
	mountTable(t, kit.Table[trow]{Rows: nil})
}

func TestTableMistakesPanicInDevMode(t *testing.T) {
	col := func(id string, sticky bool, w string) kit.Column[trow] {
		return kit.Column[trow]{ID: id, Header: id, Sticky: sticky, Width: w, Cell: func(r trow) tela.Node { return h.Text(r.v[0]) }}
	}
	for _, tc := range []struct {
		name string
		cols []kit.Column[trow]
		want string
	}{
		{"no cell", []kit.Column[trow]{{Header: "A"}}, `column 0 ("A") has no Cell`},
		{"no name", []kit.Column[trow]{{Cell: col("", false, "").Cell}}, "neither ID nor Header"},
		{"duplicate", []kit.Column[trow]{col("a", false, ""), col("a", false, "")}, `both named "a"`},
		{"sticky after plain", []kit.Column[trow]{col("a", false, ""), col("b", true, "")}, `column "b" is Sticky but follows a column that is not`},
		{"sticky width", []kit.Column[trow]{col("a", true, "1fr"), col("b", true, "")}, `Width must be in px`},
	} {
		cols := tc.cols
		_, err := telatest.AriaSnapshot(tela.C(func() tela.Node { return h.Fragment(kit.Table[trow]{Columns: cols}) }))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
}
