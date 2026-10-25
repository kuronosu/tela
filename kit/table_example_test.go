package kit_test

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// Invoice is the row type of ExampleTable.
type Invoice struct {
	ID, Status string
	Cents      int
}

func ExampleTable() {
	t := &exampleT{}
	defer t.close()
	invoices := []Invoice{{"INV001", "Paid", 25000}, {"INV002", "Pending", 15000}, {"INV003", "Unpaid", 35000}}
	page := func() tela.Node {
		return h.Div(h.Class("w-[420px] p-6"), kit.Table[Invoice]{
			Caption: "A list of your recent invoices.",
			Rows:    invoices,
			RowKey:  func(i Invoice) any { return i.ID },
			Columns: []kit.Column[Invoice]{
				{Header: "Invoice", Cell: func(i Invoice) tela.Node { return h.Text(i.ID) },
					Sort: func(a, b Invoice) int { return strings.Compare(a.ID, b.ID) }},
				{Header: "Status", Cell: func(i Invoice) tela.Node { return h.Text(i.Status) }},
				{Header: "Amount", Align: kit.AlignRight,
					Cell: func(i Invoice) tela.Node { return h.Text(fmt.Sprintf("$%.2f", float64(i.Cents)/100)) },
					Sort: func(a, b Invoice) int { return cmp.Compare(a.Cents, b.Cents) }},
			},
		})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Amount")) // ascending
	app.Click(telatest.ByRole("button", "Amount")) // descending
	app.Expect(telatest.ByRole("columnheader", "Amount"), telatest.Sort("descending"))
	app.ExpectAria(`
		- table "A list of your recent invoices.":
		  - caption: A list of your recent invoices.
		  - rowgroup:
		    - row "Invoice Status Amount":
		      - columnheader "Invoice":
		        - button "Invoice"
		      - columnheader "Status"
		      - columnheader "Amount":
		        - button "Amount"
		  - rowgroup:
		    - row "INV003 Unpaid $350.00":
		      - cell "INV003"
		      - cell "Unpaid"
		      - cell "$350.00"
		    - row "INV001 Paid $250.00":
		      - cell "INV001"
		      - cell "Paid"
		      - cell "$250.00"
		    - row "INV002 Pending $150.00":
		      - cell "INV002"
		      - cell "Pending"
		      - cell "$150.00"
	`)
	fmt.Println("sorted by amount, largest first")
	// Output: sorted by amount, largest first
}
