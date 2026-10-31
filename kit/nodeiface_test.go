package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// A struct component is a tela.Node (ADR-205): a Cell callback and a
// function that returns tela.Node return kit components as they are,
// without a Fragment.

// statusBadge returns a kit component from a function that returns a Node.
func statusBadge(s string) tela.Node {
	if s == "Paid" {
		return kit.Badge{Children: h.Text(s)}
	}
	return kit.Badge{Variant: kit.Outline, Children: h.Text(s)}
}

func TestCellReturnsAStructComponent(t *testing.T) {
	tbl := invoiceTable(func(tb *kit.Table[trow]) {
		tb.Columns = append(tb.Columns,
			kit.Column[trow]{ID: "badge", Header: "Badge", Cell: func(r trow) tela.Node { return statusBadge(r.v[1]) }},
			kit.Column[trow]{ID: "open", Header: "Open", Cell: func(r trow) tela.Node {
				return kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: h.Text("Open " + r.v[0])}
			}})
	})
	app := mountTable(t, tbl)
	app.ExpectRole("button", "Open INV004")
	app.ExpectCount(telatest.ByText("Pending"), 4) // the Status column and the badges
}
