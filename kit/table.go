package kit

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// The Lucide icons the sorting headers draw (ISC, THIRD_PARTY.md), copied
// unaltered from lucide-static as `tela icons gen` writes them. The icon
// fields of [Table] replace them.
var (
	tableIconSort     = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21 16-4 4-4-4"/><path d="M17 20V4"/><path d="m3 8 4-4 4 4"/><path d="M7 4v16"/></svg>`}
	tableIconSortAsc  = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m5 12 7-7 7 7"/><path d="M12 19V5"/></svg>`}
	tableIconSortDesc = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><path d="m19 12-7 7-7-7"/></svg>`}
)

// ColumnAlign is the horizontal alignment of a [Column]'s header, cells and
// footer. The zero value is left, as shadcn's `text-left`.
type ColumnAlign string

const (
	AlignLeft   ColumnAlign = ""
	AlignCenter ColumnAlign = "center"
	AlignRight  ColumnAlign = "right"
)

// SortDirection is the order a [Table] shows its rows in by a column: the
// values of aria-sort. The zero value is unsorted.
type SortDirection string

const (
	Ascending  SortDirection = "ascending"
	Descending SortDirection = "descending"
)

// SortState is how a [Table] is sorted: by the column with the ID Column, in
// Direction. The zero value is unsorted.
type SortState struct {
	// Column is the ID of the sorted column ("" with no sort).
	Column string
	// Direction is Ascending or Descending ("" with no sort).
	Direction SortDirection
}

// TableStyle is the recipe of [Table] (shadcn/ui's Table, TableHeader,
// TableBody, TableFooter, TableRow, TableHead, TableCell and TableCaption).
//
// shadcn's table is an HTML table; Tela has no table layout (ARCHITECTURE
// §10), so this one is a CSS grid whose rows are display: contents
// elements, as ARIA's table pattern allows: the cells are the grid's items
// and the rows, row groups and header exist for the semantics tree and for
// the classes the cells inherit. The classes shadcn puts on a row or a
// row group and that paint (a row's border, its hover and selected
// backgrounds, the footer's top border and background) are on the cells
// here, with the group-hover: and group-aria-selected: variants of the row,
// since a display: contents element has no box to paint.
type TableStyle struct {
	// Root is the scrolling container (shadcn's table-container). Give the
	// table a height with Class (max-h-96) to scroll it vertically with its
	// header kept in place.
	Root string
	// Table is the grid; the column tracks are added by the component.
	// It is as wide as its columns and at least as wide as the container,
	// so that a sticky column has a containing block as wide as the
	// scrolled content.
	Table string
	// Header, Body and Footer are the row groups (thead, tbody, tfoot) and
	// Row a row (tr); they have no box, so their classes are the inherited
	// ones: text color, font, and the `group` the cells' variants read.
	Header, Body, Footer, Row string
	// Head is a header cell (th) and Cell a body cell (td). Both are flex
	// containers that center their content vertically (`align-middle`). The
	// border under every row is here.
	Head, Cell string
	// CellWithLastRow is added to the cells of the last body row (shadcn's
	// [&_tr:last-child]:border-0).
	CellWithLastRow string
	// FooterCell is added to the footer's cells: its top border and
	// background.
	FooterCell string
	// Aligns are the classes of a column's alignment, by [ColumnAlign].
	Aligns map[ColumnAlign]string
	// HeadSticky is added to every header cell, which sticks to the top
	// of the scrolling container, over the rows that scroll under it; and
	// HeadStickyColumn to the header cell of a sticky column too.
	HeadSticky, HeadStickyColumn string
	// StickyColumn is added to the body cells of a sticky column, with an
	// opaque background so that the cells that scroll under it do not show;
	// StickyColumnHover keeps it opaque on a hovered row (a translucent
	// bg-muted/50 would show them), StickyColumnSelected on a selected
	// one and StickyColumnFooter in the footer.
	StickyColumn, StickyColumnHover, StickyColumnSelected, StickyColumnFooter string
	// Caption is the caption (shadcn's caption-bottom: below the rows).
	Caption string
	// Empty is added to the cell that holds the [Table]'s Empty content
	// when there are no rows.
	Empty string
	// SortButton is added to the classes of the button inside the header
	// of a sortable column, which is a ghost [Button] with the sort arrow
	// after the title, as in shadcn's data-table example. The header cell
	// is a `group` with aria-sort, for group-aria-[sort=ascending]: and
	// group-aria-[sort=descending]: classes.
	SortButton string
}

func init() {
	defaultStyle.Table = TableStyle{
		Root:   "relative w-full overflow-auto",
		Table:  "grid w-max min-w-full text-sm",
		Header: "contents",
		Body:   "contents",
		Footer: "contents font-medium",
		Row:    "group contents",
		Head:   "flex h-10 items-center border-b border-border px-2 text-left font-medium whitespace-nowrap text-foreground",
		Cell: "flex items-center border-b border-border p-2 whitespace-nowrap transition-colors " +
			"group-hover:bg-muted/50 group-aria-selected:bg-muted",
		CellWithLastRow: "border-b-0",
		FooterCell:      "border-t border-border bg-muted/50",
		Aligns: map[ColumnAlign]string{
			AlignLeft:   "",
			AlignCenter: "justify-center text-center",
			AlignRight:  "justify-end text-right",
		},
		HeadSticky:           "sticky top-0 z-20 bg-background",
		HeadStickyColumn:     "z-30",
		StickyColumn:         "sticky z-10 bg-background",
		StickyColumnHover:    "group-hover:bg-background group-hover:bg-linear-to-r group-hover:from-muted/50 group-hover:to-muted/50",
		StickyColumnSelected: "group-aria-selected:bg-muted",
		StickyColumnFooter:   "bg-linear-to-r from-muted/50 to-muted/50",
		Caption:              "col-span-full mt-4 text-center text-sm text-muted-foreground",
		Empty:                "col-span-full h-24 justify-center border-b-0 text-center",
		SortButton:           "",
	}
}

// Column describes a column of a [Table] of rows of type T.
type Column[T any] struct {
	// ID names the column in a [SortState]; it defaults to Header and must
	// be unique in the table.
	ID string
	// Header is the column's title, and the name of its sort button.
	Header string
	// HeaderContent is drawn in place of the Header text, for a header that
	// is not plain text. Header still names the column.
	HeaderContent tela.Component
	// Cell returns the content of the column's cell for a row, such as
	// h.Text(u.Name). It is a flex item of the cell: return one node.
	Cell func(row T) tela.Node
	// Sort orders two rows by this column (negative when a comes first, as
	// strings.Compare and cmp.Compare). A column with a Sort is sortable:
	// its header is a button that cycles the order.
	Sort func(a, b T) int
	// Sortable makes a column without a Sort sortable, for rows that come
	// sorted (from a server): the table shows the sort state and reports a
	// change with OnSortChange, and leaves the rows in the order given.
	Sortable bool
	// Width is the column's grid track: "200px", "1fr", "auto", "minmax(120px, 1fr)",
	// "min-content"... The default is "auto".
	Width string
	// Align is AlignLeft (the default), AlignCenter or AlignRight.
	Align ColumnAlign
	// Sticky keeps the column in place when the table scrolls
	// horizontally. Sticky columns come first, and every sticky column
	// but the last has a Width in px ("160px"), since the next one sticks
	// right after it.
	Sticky bool
	// Footer is the column's cell of the footer row; the row is drawn when
	// any column has one. FooterSpan, 2 or more, makes it span that many
	// columns (shadcn's colSpan), and the columns it covers have no
	// footer cell of their own.
	Footer     tela.Component
	FooterSpan int
	// Class is added to the column's body cells, HeaderClass to its header
	// and FooterClass to its footer cell.
	Class, HeaderClass, FooterClass string
	// CellProps returns more props for the column's cell of a row, such as
	// h.TestID or a handler; HeaderProps and FooterProps are the props of
	// the header and footer cells.
	CellProps                func(row T) tela.Child
	HeaderProps, FooterProps tela.Child
}

func (c Column[T]) id() string {
	if c.ID != "" {
		return c.ID
	}
	return c.Header
}

func (c Column[T]) sortable() bool { return c.Sort != nil || c.Sortable }

// Table is a data table, shadcn/ui's Table with the sorting and the sticky
// header and columns of its data-table and scroll examples, as a typed
// component: columns say how to draw a row, and rows are the data.
//
//	kit.Table[Invoice]{
//	    Caption: "A list of your recent invoices.",
//	    Rows:    invoices,
//	    RowKey:  func(i Invoice) any { return i.ID },
//	    Columns: []kit.Column[Invoice]{
//	        {Header: "Invoice", Cell: func(i Invoice) tela.Node { return h.Text(i.ID) },
//	            Sort: func(a, b Invoice) int { return strings.Compare(a.ID, b.ID) }},
//	        {Header: "Status", Cell: func(i Invoice) tela.Node { return h.Text(i.Status) }},
//	        {Header: "Amount", Align: kit.AlignRight, Cell: func(i Invoice) tela.Node { return h.Text(i.Amount) },
//	            Sort: func(a, b Invoice) int { return cmp.Compare(a.Cents, b.Cents) }},
//	    },
//	}
//
// # Sorting
//
// The header of a column with a Sort (or Sortable) is a button, so Tab
// reaches it and Enter and Space press it. Pressing it cycles the column
// through ascending, descending and unsorted, as TanStack Table (the engine
// of shadcn's data table) does by default, and pressing another column
// starts that one ascending; only one column is sorted at a time. The
// sorted header carries aria-sort and shows an arrow. The sort is stable:
// rows that compare equal keep their order in Rows (also when
// descending). Sorting follows ADR-101's value rule: Sort (a pointer to
// your state) controls it, DefaultSort sets the start and OnSortChange
// reports every change.
//
//	sort, setSort := tela.UseState(kit.SortState{Column: "amount", Direction: kit.Descending})
//	kit.Table[Invoice]{Sort: &sort, OnSortChange: setSort.Set, ...}
//
// # Layout
//
// The table is a CSS grid, one track per column from its Width. A column
// of Width "auto" (the default) is as wide as its widest cell and shares
// the space the container leaves; "1fr" columns divide it by weight, and
// "200px" ones keep their size. The header sticks to the top of the
// container, so a table in a container with a height (Class: "max-h-80")
// scrolls its rows under the header; columns with Sticky stay at the left
// when the table scrolls sideways.
//
// Rows are keyed by RowKey, so the state of the components in a row's cells
// follows the row when the order changes. Without RowKey a row's key is its
// position in the sorted rows.
//
// The look is the pinned shadcn commit's table, with the deviations in
// [TableStyle]. A table has no row selection or editing; Selected only
// styles the rows it names, with aria-selected.
type Table[T any] struct {
	// Rows are the data, in the order to show them when unsorted.
	Rows []T
	// RowKey returns a comparable key that is unique among Rows (an ID);
	// nil keys rows by position.
	RowKey func(row T) any
	// Columns are the table's columns, in order.
	Columns []Column[T]

	// Sort controls the sort (nil: the table keeps it), DefaultSort starts
	// an uncontrolled table sorted, and OnSortChange is called with the new
	// state when a header is pressed.
	Sort         *SortState
	DefaultSort  SortState
	OnSortChange func(SortState)

	// Selected tells whether a row is selected: the row gets the selected
	// look and aria-selected. Nil leaves the rows unselectable.
	Selected func(row T) bool

	// Caption is drawn below the rows, and names the table for assistive
	// technologies. AriaLabel names a table that has none.
	Caption   string
	AriaLabel string
	// Empty is drawn in a row of its own when there are no rows (shadcn's
	// "No results."). Nil draws the header alone.
	Empty tela.Component

	// SortIcon, SortAscIcon and SortDescIcon replace the arrows of a
	// sortable header that is unsorted, ascending and descending.
	SortIcon, SortAscIcon, SortDescIcon tela.Icon

	// Class is added after the recipe's Root classes. The other Class
	// fields do the same for their part: the grid, every header cell,
	// every body cell and the caption.
	Class        string
	TableClass   string
	HeadClass    string
	CellClass    string
	CaptionClass string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the table.
func (t Table[T]) Build() tela.Node {
	s := &UseStyle().Table
	state, setState := tela.UseState(t.DefaultSort)
	captionRef := tela.UseElementRef()
	cur := state
	if t.Sort != nil {
		cur = *t.Sort
	}
	cols := t.Columns
	if len(cols) == 0 {
		return h.Div(cls(s.Root, t.Class), t.Props)
	}
	offsets := t.checkColumns()

	// The tracks.
	tracks := make([]string, len(cols))
	for i, c := range cols {
		w := strings.TrimSpace(c.Width)
		if w == "" {
			w = "auto"
		}
		tracks[i] = strings.ReplaceAll(w, " ", "_")
	}
	gridCols := "grid-cols-[" + strings.Join(tracks, "_") + "]"

	// The order.
	order := make([]int, len(t.Rows))
	for i := range order {
		order[i] = i
	}
	sortedBy := -1
	for i, c := range cols {
		if cur.Direction != "" && c.id() == cur.Column && c.sortable() {
			sortedBy = i
		}
	}
	if sortedBy >= 0 && cols[sortedBy].Sort != nil {
		cmpf, desc := cols[sortedBy].Sort, cur.Direction == Descending
		slices.SortStableFunc(order, func(a, b int) int {
			r := cmpf(t.Rows[a], t.Rows[b])
			if desc {
				return -r
			}
			return r
		})
	}

	// The header.
	headCells := make([]tela.Child, len(cols))
	for i, c := range cols {
		headCells[i] = t.headCell(s, c, i, i == sortedBy, cur, offsets[i], func(next SortState) {
			if t.Sort == nil {
				setState.Set(next)
			}
			if t.OnSortChange != nil {
				t.OnSortChange(next)
			}
		})
	}
	header := h.Div(cls(s.Header), h.Role("rowgroup"),
		h.Div(cls(s.Row), h.Role("row"), h.Fragment(headCells...)))

	// The body.
	rows := make([]tela.Child, len(order))
	for n, i := range order {
		row := t.Rows[i]
		var key any = n
		if t.RowKey != nil {
			key = t.RowKey(row)
		}
		cells := make([]tela.Child, len(cols))
		for j, c := range cols {
			classes := joinClass(s.Cell, s.Aligns[c.Align], t.CellClass, when(c.Sticky, t.stickyClass(s, offsets[j], false)),
				c.Class, when(n == len(order)-1, s.CellWithLastRow))
			var props tela.Child
			if c.CellProps != nil {
				props = c.CellProps(row)
			}
			cells[j] = h.Div(cls(classes), h.Role("cell"), props, c.Cell(row))
		}
		kids := []tela.Child{h.Key(key), cls(s.Row), h.Role("row")}
		if t.Selected != nil {
			kids = append(kids, h.AriaSelected(t.Selected(row)))
		}
		kids = append(kids, h.Fragment(cells...))
		rows[n] = h.Div(kids...)
	}
	if len(rows) == 0 && t.Empty != nil {
		rows = append(rows, h.Div(cls(s.Row), h.Role("row"),
			h.Div(cls(joinClass(s.Cell, t.CellClass, s.Empty)), h.Role("cell"), t.Empty)))
	}
	body := h.Div(cls(s.Body), h.Role("rowgroup"), h.Fragment(rows...))

	parts := []tela.Child{cls(s.Table, gridCols, t.TableClass), h.Role("table")}
	footer := t.footer(s, offsets)
	switch {
	case t.Caption != "":
		// The caption comes first in the tree, as an HTML table's does,
		// and sits in the grid's last row (caption-side: bottom).
		last := 2 + len(rows)
		if footer != nil {
			last++
		}
		parts = append(parts, h.AriaLabelledBy(captionRef),
			h.Div(cls(s.Caption, "row-start-"+strconv.Itoa(last), t.CaptionClass), h.Role("caption"), h.Ref(captionRef), h.Text(t.Caption)))
	case t.AriaLabel != "":
		parts = append(parts, h.AriaLabel(t.AriaLabel))
	}
	parts = append(parts, header, body)
	if footer != nil {
		parts = append(parts, footer)
	}
	return h.Div(cls(s.Root, t.Class), t.Props, h.Div(parts...))
}

// stickyClass is the classes of a cell of a sticky column that starts left
// px from the container's left edge; footer picks the footer's background.
func (t Table[T]) stickyClass(s *TableStyle, left float32, footer bool) string {
	l := stickyLeft(left)
	if footer {
		return joinClass(s.StickyColumn, l, s.StickyColumnFooter)
	}
	return joinClass(s.StickyColumn, l, s.StickyColumnHover, s.StickyColumnSelected)
}

// footer returns the footer row group, or nil when no column has a footer.
func (t Table[T]) footer(s *TableStyle, offsets []float32) tela.Child {
	has := false
	for _, c := range t.Columns {
		has = has || c.Footer != nil
	}
	if !has {
		return nil
	}
	var cells []tela.Child
	for i := 0; i < len(t.Columns); i++ {
		c := t.Columns[i]
		span := max(c.FooterSpan, 1)
		span = min(span, len(t.Columns)-i)
		classes := joinClass(s.Cell, s.Aligns[c.Align], t.CellClass, s.FooterCell, when(span > 1, "col-span-"+strconv.Itoa(span)),
			when(c.Sticky, t.stickyClass(s, offsets[i], true)), c.FooterClass, "border-b-0")
		cells = append(cells, h.Div(cls(classes), h.Role("cell"), c.FooterProps, c.Footer))
		i += span - 1
	}
	return h.Div(cls(s.Footer), h.Role("rowgroup"), h.Div(cls(s.Row), h.Role("row"), h.Fragment(cells...)))
}

// headCell returns the header cell of column c (the i-th).
func (t Table[T]) headCell(s *TableStyle, c Column[T], i int, sorted bool, cur SortState, left float32, set func(SortState)) tela.Child {
	classes := joinClass(s.Head, s.Aligns[c.Align], s.HeadSticky, t.HeadClass,
		when(c.Sticky, joinClass(s.HeadStickyColumn, stickyLeft(left))), when(c.sortable(), "group"), c.HeaderClass)
	kids := []tela.Child{cls(classes), h.Role("columnheader"), c.HeaderProps}
	var content tela.Component = h.Text(c.Header)
	if c.HeaderContent != nil {
		content = c.HeaderContent
	}
	if !c.sortable() {
		return h.Div(append(kids, content)...)
	}
	ic := t.SortIcon
	if ic == (tela.Icon{}) {
		ic = tableIconSort
	}
	if sorted {
		kids = append(kids, h.AriaSort(string(cur.Direction)))
		if cur.Direction == Ascending {
			if ic = t.SortAscIcon; ic == (tela.Icon{}) {
				ic = tableIconSortAsc
			}
		} else if ic = t.SortDescIcon; ic == (tela.Icon{}) {
			ic = tableIconSortDesc
		}
	}
	id := c.id()
	btn := Button{Variant: Ghost, IconEnd: ic, Class: s.SortButton, Children: content,
		OnClick: func() { set(nextSort(cur, id)) }}
	if c.HeaderContent != nil {
		btn.AriaLabel = c.Header
	}
	return h.Div(append(kids, btn)...)
}

// stickyLeft is the left inset class of a sticky cell.
func stickyLeft(left float32) string {
	if left > 0 {
		return "left-[" + strconv.FormatFloat(float64(left), 'f', -1, 32) + "px]"
	}
	return "left-0"
}

// nextSort is the state after pressing the header of column id: ascending,
// then descending, then unsorted (TanStack Table's default cycle); another
// column starts ascending.
func nextSort(cur SortState, id string) SortState {
	switch {
	case cur.Column != id || cur.Direction == "":
		return SortState{Column: id, Direction: Ascending}
	case cur.Direction == Ascending:
		return SortState{Column: id, Direction: Descending}
	}
	return SortState{}
}

// checkColumns validates the columns and returns the left offset of each
// sticky column. Mistakes panic in dev mode with an actionable message and
// are tolerated otherwise.
func (t Table[T]) checkColumns() []float32 {
	offsets := make([]float32, len(t.Columns))
	seen := map[string]int{}
	var left float32
	sticky := true
	dev := devMode()
	for i, c := range t.Columns {
		if dev {
			if c.Cell == nil {
				panic(fmt.Sprintf("kit: Table column %d (%q) has no Cell; set Cell: func(row T) tela.Node { return h.Text(...) }", i, c.Header))
			}
			if c.id() == "" {
				panic(fmt.Sprintf("kit: Table column %d has neither ID nor Header; sorting and keys need a name", i))
			}
			if j, dup := seen[c.id()]; dup {
				panic(fmt.Sprintf("kit: Table columns %d and %d are both named %q; give one a different ID", j, i, c.id()))
			}
			seen[c.id()] = i
		}
		if !c.Sticky {
			sticky = false
			continue
		}
		if !sticky {
			if dev {
				panic(fmt.Sprintf("kit: Table column %q is Sticky but follows a column that is not; sticky columns must be the first ones", c.Header))
			}
			continue
		}
		offsets[i] = left
		w, ok := pxWidth(c.Width)
		if !ok && i+1 < len(t.Columns) && t.Columns[i+1].Sticky && dev {
			panic(fmt.Sprintf("kit: Table column %q is Sticky and followed by another sticky column, so its Width must be in px (\"160px\"), not %q", c.Header, c.Width))
		}
		left += w
	}
	return offsets
}

// pxWidth parses a width like "160px".
func pxWidth(w string) (float32, bool) {
	w = strings.TrimSpace(w)
	n, ok := strings.CutSuffix(w, "px")
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(n), 32)
	if err != nil || v < 0 {
		return 0, false
	}
	return float32(v), true
}
