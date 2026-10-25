package kit_test

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's Table parts (new-york-v4/ui/table.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnTableContainer = "relative w-full overflow-x-auto"
	shadcnTable          = "w-full caption-bottom text-sm"
	shadcnTableHeader    = "[&_tr]:border-b"
	shadcnTableBody      = "[&_tr:last-child]:border-0"
	shadcnTableFooter    = "border-t bg-muted/50 font-medium [&>tr]:last:border-b-0"
	shadcnTableRow       = "border-b transition-colors hover:bg-muted/50 has-aria-expanded:bg-muted/50 data-[state=selected]:bg-muted"
	shadcnTableHead      = "h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]"
	shadcnTableCell      = "p-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]"
	shadcnTableCaption   = "mt-4 text-sm text-muted-foreground"
)

// trow is a row of the test tables: its position in the data and its cells.
type trow struct {
	i int
	v []string
}

// tcol is a column of a table cell of a sheet.
type tcol struct {
	header string
	width  string // a px width or a track; "" is auto
	right  bool
	sort   bool
	sticky bool
}

// tfoot is a footer cell: its text and the columns it spans.
type tfoot struct {
	text string
	span int
}

// tableSpec describes a table cell of a sheet, on both sides.
type tableSpec struct {
	name     string
	width    int // the cell's width in px; 0 shrinks it to the table
	cols     []tcol
	rows     [][]string
	caption  string
	footer   []tfoot
	sort     kit.SortState // DefaultSort
	selected int           // 1-based; 0 selects no row
	// fixed is table-fixed on shadcn's side: with every column's width in
	// px (or all equal 1fr) and the cell as wide as their sum, an HTML
	// table and a grid give the columns the same widths.
	fixed bool
	// height is the height of the box the table sits in (0: none).
	height int
	// class is added to the Tela table's root.
	class string
}

func (p tableSpec) columns() []kit.Column[trow] {
	var out []kit.Column[trow]
	for c, col := range p.cols {
		c, col := c, col
		kc := kit.Column[trow]{
			Header: col.header, Width: col.width, Sticky: col.sticky,
			Cell:        func(r trow) tela.Node { return h.Text(r.v[c]) },
			CellProps:   func(r trow) tela.Child { return h.TestID(fmt.Sprintf("%s-r%dc%d", p.name, r.i, c)) },
			HeaderProps: h.TestID(fmt.Sprintf("%s-h%d", p.name, c)),
		}
		if col.right {
			kc.Align = kit.AlignRight
		}
		if col.sort {
			kc.Sort = func(a, b trow) int { return cmp.Compare(a.v[c], b.v[c]) }
		}
		out = append(out, kc)
	}
	// Footers: the first column's cell spans.
	col := 0
	for _, f := range p.footer {
		out[col].Footer = h.Text(f.text)
		out[col].FooterProps = h.TestID(fmt.Sprintf("%s-f%d", p.name, col))
		out[col].FooterSpan = f.span
		col += max(f.span, 1)
	}
	return out
}

func (p tableSpec) rowData() []trow {
	rows := make([]trow, len(p.rows))
	for i, v := range p.rows {
		rows[i] = trow{i, v}
	}
	return rows
}

// sortedRows is the rows in the order the sort state shows them: stable,
// also when descending.
func (p tableSpec) sortedRows() []trow {
	rows := p.rowData()
	for c, col := range p.cols {
		if col.header == p.sort.Column && p.sort.Direction != "" {
			slices.SortStableFunc(rows, func(a, b trow) int {
				r := cmp.Compare(a.v[c], b.v[c])
				if p.sort.Direction == kit.Descending {
					return -r
				}
				return r
			})
		}
	}
	return rows
}

// component is the Tela side.
func (p tableSpec) component() kit.Table[trow] {
	t := kit.Table[trow]{
		Rows: p.rowData(), RowKey: func(r trow) any { return r.i }, Columns: p.columns(),
		Caption: p.caption, DefaultSort: p.sort, Class: p.class, Props: h.TestID(p.name + "-table"),
	}
	if p.selected > 0 {
		t.Selected = func(r trow) bool { return r.i == p.selected-1 }
	}
	return t
}

// markup is shadcn's markup for the same table.
func (p tableSpec) markup() string {
	var sb strings.Builder
	cls := shadcnTable
	if p.fixed {
		cls += " table-fixed"
	}
	fmt.Fprintf(&sb, `<div data-slot="table-container" data-probe="%s-table" class="%s"><table data-slot="table" class="%s">`, p.name, shadcnTableContainer, cls)
	if p.caption != "" {
		fmt.Fprintf(&sb, `<caption data-slot="table-caption" class="%s">%s</caption>`, cn(shadcnTableCaption), p.caption)
	}
	align := func(c tcol) string {
		if c.right {
			return "text-right"
		}
		return ""
	}
	style := func(c tcol) string {
		if p.fixed && strings.HasSuffix(c.width, "px") {
			return fmt.Sprintf(` style="width: %s"`, c.width)
		}
		return ""
	}
	fmt.Fprintf(&sb, `<thead data-slot="table-header" class="%s"><tr data-slot="table-row" class="%s">`, shadcnTableHeader, shadcnTableRow)
	for c, col := range p.cols {
		attrs := ""
		inner := col.header
		if col.sort {
			dir := ""
			ic := tableTestUpDown
			if p.sort.Column == col.header {
				dir = string(p.sort.Direction)
			}
			switch dir {
			case "ascending":
				attrs, ic = ` aria-sort="ascending"`, tableTestUp
			case "descending":
				attrs, ic = ` aria-sort="descending"`, tableTestDown
			}
			inner = buttonHTML(kit.Button{Variant: kit.Ghost}, col.header+svgHTML(ic), "")
		}
		fmt.Fprintf(&sb, `<th data-slot="table-head" data-probe="%s-h%d" class="%s"%s%s>%s</th>`, p.name, c, cn(shadcnTableHead, align(col)), style(col), attrs, inner)
	}
	fmt.Fprintf(&sb, `</tr></thead><tbody data-slot="table-body" class="%s">`, shadcnTableBody)
	for _, tr := range p.sortedRows() {
		r, row := tr.i, tr.v
		state := ""
		if p.selected == r+1 {
			state = ` data-state="selected"`
		}
		fmt.Fprintf(&sb, `<tr data-slot="table-row"%s class="%s">`, state, shadcnTableRow)
		for c, v := range row {
			fmt.Fprintf(&sb, `<td data-slot="table-cell" data-probe="%s-r%dc%d" class="%s">%s</td>`, p.name, r, c, cn(shadcnTableCell, align(p.cols[c])), v)
		}
		sb.WriteString(`</tr>`)
	}
	sb.WriteString(`</tbody>`)
	if len(p.footer) > 0 {
		fmt.Fprintf(&sb, `<tfoot data-slot="table-footer" class="%s"><tr data-slot="table-row" class="%s">`, shadcnTableFooter, shadcnTableRow)
		col := 0
		for _, f := range p.footer {
			span := ""
			if f.span > 1 {
				span = fmt.Sprintf(` colspan="%d"`, f.span)
			}
			fmt.Fprintf(&sb, `<td data-slot="table-cell" data-probe="%s-f%d" class="%s"%s>%s</td>`, p.name, col, cn(shadcnTableCell, align(p.cols[col])), span, f.text)
			col += max(f.span, 1)
		}
		sb.WriteString(`</tr></tfoot>`)
	}
	sb.WriteString(`</table></div>`)
	return sb.String()
}

// probes lists the names the two sides mark.
func (p tableSpec) probes() []string {
	names := []string{p.name + "-table"}
	for c := range p.cols {
		names = append(names, fmt.Sprintf("%s-h%d", p.name, c))
		for r := range p.rows {
			names = append(names, fmt.Sprintf("%s-r%dc%d", p.name, r, c))
		}
	}
	col := 0
	for _, f := range p.footer {
		names = append(names, fmt.Sprintf("%s-f%d", p.name, col))
		col += max(f.span, 1)
	}
	return names
}

// cell is the sheet's cell. With a height the table sits in a box of that
// height on both sides: an HTML table's collapsed borders add up to half a
// pixel to its height (a border is half in the cell above and half in the
// one below, and the table's own edge keeps half of its last one), which
// would move the cells that follow in the sheet, and the comparison is
// of each table's own cells.
func (p tableSpec) cell() cell {
	c := cell{name: p.name, width: p.width, tela: p.component(), html: p.markup(), probes: p.probes()}
	if p.height > 0 {
		cls := fmt.Sprintf("h-[%dpx]", p.height)
		if p.width > 0 {
			cls += " w-full"
		}
		c.tela = h.Div(h.Class(cls), c.tela)
		c.html = fmt.Sprintf(`<div class="%s">%s</div>`, cls, c.html)
	}
	return c
}

// The Lucide icons of the sort headers (ISC), as shadcn's data table draws
// them.
var (
	tableTestUpDown = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21 16-4 4-4-4"/><path d="M17 20V4"/><path d="m3 8 4-4 4 4"/><path d="M7 4v16"/></svg>`}
	tableTestUp     = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m5 12 7-7 7 7"/><path d="M12 19V5"/></svg>`}
	tableTestDown   = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><path d="m19 12-7 7-7-7"/></svg>`}
)

var invoiceRows = [][]string{
	{"INV001", "Paid", "Credit Card", "$250.00"},
	{"INV002", "Pending", "PayPal", "$150.00"},
	{"INV003", "Unpaid", "Bank Transfer", "$350.00"},
	{"INV004", "Paid", "Credit Card", "$450.00"},
}

func invoiceCols(width string) []tcol {
	return []tcol{
		{header: "Invoice", width: width},
		{header: "Status", width: width},
		{header: "Method", width: width},
		{header: "Amount", width: width, right: true},
	}
}

func tableSheet() *sheet {
	cells := []cell{
		// shadcn's Table demo: a caption, four columns and a footer that
		// spans three, in content-sized columns (no widths).
		tableSpec{name: "demo", cols: invoiceCols(""), rows: invoiceRows, caption: "A list of your recent invoices.",
			footer: []tfoot{{"Total", 3}, {"$1,200.00", 1}}, height: 264}.cell(),
		// Columns with px tracks and table-fixed on the other side.
		tableSpec{name: "px", width: 480, cols: []tcol{{header: "Invoice", width: "100px"}, {header: "Status", width: "120px"},
			{header: "Method", width: "160px"}, {header: "Amount", width: "100px", right: true}},
			rows: invoiceRows, fixed: true, height: 190}.cell(),
		// 1fr columns against equal columns.
		tableSpec{name: "fr", width: 480, cols: invoiceCols("1fr"), rows: invoiceRows, fixed: true, selected: 2, height: 190}.cell(),
		// A table with no rows has the header alone.
		tableSpec{name: "no-rows", width: 480, cols: invoiceCols("1fr"), fixed: true, height: 42}.cell(),
		// Sorted: the arrow and the order.
		tableSpec{name: "sorted", width: 500, cols: []tcol{{header: "Invoice", width: "130px", sort: true}, {header: "Status", width: "130px", sort: true},
			{header: "Method", width: "120px"}, {header: "Amount", width: "120px", right: true, sort: true}},
			rows: invoiceRows, fixed: true, sort: kit.SortState{Column: "Status", Direction: kit.Descending}, height: 190}.cell(),
	}
	return &sheet{name: "table", width: 560, cells: cells, edge: [2]float64{1, 1}, mean: [2]float64{6, 3.5}}
}

func TestTableGolden(t *testing.T)   { tableSheet().golden(t) }
func TestTableChromium(t *testing.T) { tableSheet().chromium(t) }
