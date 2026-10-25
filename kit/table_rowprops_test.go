package kit_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// RowProps puts props on the element of each body row (ADR-154, A2): a
// name, a test id and handlers. The row is a display: contents element:
// pointer and key events reach its handlers by bubbling from the cells,
// and the engine reports its bounds as those of its cells together.
func TestTableRowProps(t *testing.T) {
	var clicked, entered, keys []string
	tbl := invoiceTable(func(tb *kit.Table[trow]) {
		tb.Columns = append(tb.Columns, kit.Column[trow]{ID: "open", Header: "Open", Cell: func(r trow) tela.Node {
			return h.Span(kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: h.Text("Open " + r.v[0])})
		}})
		tb.RowProps = func(r trow) tela.Child {
			id := r.v[0]
			return tela.MergeProps(
				h.AriaLabel("Invoice "+id),
				h.TestID("row-"+id),
				h.OnClick(func() { clicked = append(clicked, id) }),
				h.OnMouseEnter(func() { entered = append(entered, id) }),
				h.OnKeyDown(func(e tela.KeyboardEvent) { keys = append(keys, id+":"+string(e.Key)) }),
			)
		}
	})
	app := mountTable(t, tbl)

	// The row is named: the name replaces the text of its cells.
	app.ExpectCount(telatest.ByRole("row", "Invoice INV002"), 1)

	// A press on a cell reaches the row's handler by bubbling.
	app.Click(telatest.ByText("PayPal"))
	if got := strings.Join(clicked, ","); got != "INV002" {
		t.Errorf("clicked rows %q, want INV002", got)
	}
	app.Hover(telatest.ByText("$350.00"))
	if len(entered) == 0 || entered[len(entered)-1] != "INV003" {
		t.Errorf("entered rows %v, want the last to be INV003", entered)
	}

	// A key pressed on a button in a cell bubbles to the row.
	app.Focus(telatest.ByRole("button", "Open INV003"))
	app.Press("ArrowDown")
	if got := strings.Join(keys, ","); got != "INV003:ArrowDown" {
		t.Errorf("keys %q, want INV003:ArrowDown", got)
	}

	// The row has bounds, those of its cells together, below the header.
	row, cell := app.Bounds(telatest.ByTestID("row-INV001")), app.Bounds(telatest.ByTestID("t-r0c0"))
	if row.Width == 0 || row.Y > cell.Y || row.Bottom() < cell.Bottom() {
		t.Errorf("row bounds %v do not hold its cell %v", row, cell)
	}

	// The props follow the row when the table is sorted.
	app.Click(telatest.ByRole("button", "Amount"))
	app.Click(telatest.ByText("PayPal"))
	if got := strings.Join(clicked, ","); got != "INV002,INV002" {
		t.Errorf("clicked rows %q after sorting, want INV002,INV002", got)
	}
}
