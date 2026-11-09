package main

import (
	"cmp"
	"fmt"
	"sort"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The page of Table. Its stages are written with tr; the data (the invoice
// numbers, the names) is not translated.

// Invoice is a row of the table demo.
type Invoice struct {
	ID, Status, Method string
	Cents              int
}

var invoices = []Invoice{
	{"INV001", "Paid", "Credit Card", 25000},
	{"INV002", "Pending", "PayPal", 15000},
	{"INV003", "Unpaid", "Bank Transfer", 35000},
	{"INV004", "Paid", "Credit Card", 45000},
	{"INV005", "Paid", "PayPal", 55000},
	{"INV006", "Pending", "Bank Transfer", 15000},
	{"INV007", "Unpaid", "Credit Card", 35000},
	{"INV008", "Paid", "PayPal", 12500},
	{"INV009", "Paid", "Credit Card", 9900},
	{"INV010", "Pending", "PayPal", 21000},
}

func money(cents int) string { return fmt.Sprintf("$%d.%02d", cents/100, cents%100) }

// Member is a row of the sort demos.
type Member struct {
	Name, Role string
	Projects   int
}

var members = []Member{
	{"Mei Tanaka", "Design", 12},
	{"Ana García", "Engineering", 8},
	{"Omar Haddad", "Operations", 5},
	{"Lena Fischer", "Engineering", 15},
	{"Tomás Vega", "Design", 3},
}

// Part is a row of the search demo: a component and how often it is used.
type Part struct {
	Name string
	Uses int
}

var parts = []Part{
	{"Avatar", 120}, {"Badge", 85}, {"Button", 410}, {"Calendar", 64}, {"Card", 230},
	{"Checkbox", 97}, {"Dialog", 142}, {"Table", 176},
}

// relevance is how well a part answers a search: the earlier the text
// matches and the shorter the name, the higher (0 is no match).
func relevance(p Part, q string) int {
	i := strings.Index(strings.ToLower(p.Name), strings.ToLower(q))
	if i < 0 {
		return 0
	}
	return max(1, 100-10*i-len(p.Name))
}

func tablePage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	status := func(s string) string {
		return map[string]string{"Paid": tr("table.paid", "Paid"), "Pending": tr("table.pending", "Pending"), "Unpaid": tr("table.unpaid", "Unpaid")}[s]
	}
	method := func(s string) string {
		return map[string]string{"Credit Card": tr("table.card", "Credit Card"), "PayPal": "PayPal", "Bank Transfer": tr("table.bank", "Bank Transfer")}[s]
	}
	statusBadge := func(s string) tela.Node {
		switch s {
		case "Paid":
			return kit.Badge{Variant: kit.Success, Children: Text(status(s))}
		case "Unpaid":
			return kit.Badge{Variant: kit.Destructive, Children: Text(status(s))}
		}
		return kit.Badge{Variant: kit.Outline, Children: Text(status(s))}
	}
	sortBy, setSort := tela.UseState(kit.SortState{Column: "amount", Direction: kit.Descending})
	always, setAlways := tela.UseState(kit.SortState{Column: "name", Direction: kit.Ascending})
	hidden, setHidden := tela.UseState(kit.SortState{})
	query, setQuery := tela.UseState("a")
	found, setFound := tela.UseState(kit.SortState{Column: "relevance", Direction: kit.Other})
	total := 0
	for _, i := range invoices[:5] {
		total += i.Cents
	}

	// The search demo: rows come ordered by relevance (as a server would
	// send them) while the table is in the Other state.
	var results []Part
	for _, p := range parts {
		if query == "" || relevance(p, query) > 0 {
			results = append(results, p)
		}
	}
	sort.SliceStable(results, func(a, b int) bool { return relevance(results[a], query) > relevance(results[b], query) })
	search := func(q string) {
		setQuery.Set(q)
		switch {
		case q == "":
			setFound.Set(kit.SortState{})
		case found.Column == "" || found.Direction == kit.Other:
			setFound.Set(kit.SortState{Column: "relevance", Direction: kit.Other})
		}
	}

	memberCols := func() []kit.Column[Member] {
		return []kit.Column[Member]{
			{ID: "name", Header: tr("table.name", "Name"), Cell: func(m Member) tela.Node { return Text(m.Name) },
				Sort: func(a, b Member) int { return strings.Compare(a.Name, b.Name) }},
			{ID: "role", Header: tr("table.role", "Role"), Cell: func(m Member) tela.Node {
				return Text(map[string]string{"Design": tr("table.design", "Design"), "Engineering": tr("table.eng", "Engineering"), "Operations": tr("table.ops", "Operations")}[m.Role])
			},
				Sort: func(a, b Member) int { return strings.Compare(a.Role, b.Role) }},
			{ID: "projects", Header: tr("table.projects", "Projects"), Align: kit.AlignRight, Class: "tabular-nums", Cell: func(m Member) tela.Node { return Text(f.Number(float64(m.Projects))) },
				Sort: func(a, b Member) int { return cmp.Compare(a.Projects, b.Projects) }},
		}
	}

	return stack(
		demo("Sortable", "A header with a Sort is a button (Enter and Space press it): ascending, descending, unsorted. The sort is the app's through Sort and OnSortChange.", col,
			kit.Table[Invoice]{
				Caption: tr("table.caption", "A list of your recent invoices."), Rows: invoices[:5], RowKey: func(i Invoice) any { return i.ID },
				Sort: &sortBy, OnSortChange: setSort.Set,
				Columns: []kit.Column[Invoice]{
					{ID: "invoice", Header: tr("table.invoice", "Invoice"), Width: "120px", Class: "font-medium", Cell: func(i Invoice) tela.Node { return Text(i.ID) },
						Sort:   func(a, b Invoice) int { return strings.Compare(a.ID, b.ID) },
						Footer: Text(tr("table.total", "Total")), FooterSpan: 3},
					{ID: "status", Header: tr("table.status", "Status"), Cell: func(i Invoice) tela.Node { return statusBadge(i.Status) },
						Sort: func(a, b Invoice) int { return strings.Compare(a.Status, b.Status) }},
					{ID: "method", Header: tr("table.method", "Method"), Cell: func(i Invoice) tela.Node { return Text(method(i.Method)) }},
					{ID: "amount", Header: tr("table.amount", "Amount"), Align: kit.AlignRight, Class: "tabular-nums", Cell: func(i Invoice) tela.Node { return Text(money(i.Cents)) },
						Sort:   func(a, b Invoice) int { return cmp.Compare(a.Cents, b.Cents) },
						Footer: Text(money(total))},
				}}),
		demo("Always sorted", "AlwaysSorted cycles a sorted header between ascending and descending and never back to unsorted, for a table that is always in some order.", col,
			kit.Table[Member]{Class: "max-w-xl", AriaLabel: tr("table.team", "Team"), Rows: members, RowKey: func(m Member) any { return m.Name },
				Sort: &always, OnSortChange: setAlways.Set, AlwaysSorted: true, Columns: memberCols()}),
		demo("Hidden unsorted arrow", "HideUnsortedIcon hides the arrow of the headers that are not sorted (it shows under the pointer and with the focus), keeping its place so nothing moves.", col,
			kit.Table[Member]{Class: "max-w-xl", AriaLabel: tr("table.team2", "Team, sortable"), Rows: members, RowKey: func(m Member) any { return m.Name },
				Sort: &hidden, OnSortChange: setHidden.Set, HideUnsortedIcon: true, Columns: memberCols()}),
		demo("Sorted by something else", "A table whose rows came ordered by relevance is in the Other state: the header says aria-sort=\"other\", which a class such as aria-[sort=other]:text-blue-600 can style. Sorting by another column leaves it.", col,
			Div(Class("flex w-full max-w-xl flex-col gap-3"),
				kit.Input{Icon: icons.Search, Type: "search", Value: &query, OnValueChange: search, Placeholder: tr("table.search", "Search components..."), AriaLabel: tr("table.searchLabel", "Search components")},
				kit.Table[Part]{AriaLabel: tr("table.results", "Search results"), Rows: results, RowKey: func(p Part) any { return p.Name },
					Sort: &found, OnSortChange: setFound.Set,
					Empty: Div(Class("py-6 text-center text-sm text-muted-foreground"), Text(tr("table.empty", "No results."))),
					Columns: []kit.Column[Part]{
						{ID: "name", Header: tr("table.component", "Component"), Class: "font-medium", Cell: func(p Part) tela.Node { return Text(p.Name) },
							Sort: func(a, b Part) int { return strings.Compare(a.Name, b.Name) }},
						{ID: "uses", Header: tr("table.uses", "Uses"), Align: kit.AlignRight, Class: "tabular-nums", Cell: func(p Part) tela.Node { return Text(f.Number(float64(p.Uses))) },
							Sort: func(a, b Part) int { return cmp.Compare(a.Uses, b.Uses) }},
						{ID: "relevance", Header: tr("table.relevance", "Relevance"), Sortable: true, Align: kit.AlignRight, Class: "tabular-nums",
							HeaderClass: "aria-[sort=other]:font-semibold aria-[sort=other]:text-blue-600 dark:aria-[sort=other]:text-blue-400",
							Cell: func(p Part) tela.Node {
								return Text(f.Number(float64(relevance(p, query))/100, i18n.Percent()))
							}},
					}}),
		),
		demo("Scrolling", "In a container with a height the header sticks to the top; Sticky keeps the first column at the left. Scroll the table.", col,
			kit.Table[Invoice]{
				Class: "max-h-64 max-w-xl", Rows: invoices, RowKey: func(i Invoice) any { return i.ID },
				Columns: []kit.Column[Invoice]{
					{Header: tr("table.invoice", "Invoice"), Width: "110px", Sticky: true, Class: "font-medium", Cell: func(i Invoice) tela.Node { return Text(i.ID) }},
					{Header: tr("table.status", "Status"), Width: "120px", Cell: func(i Invoice) tela.Node { return statusBadge(i.Status) }},
					{Header: tr("table.method", "Method"), Width: "160px", Cell: func(i Invoice) tela.Node { return Text(method(i.Method)) }},
					{Header: tr("table.amount", "Amount"), Width: "120px", Align: kit.AlignRight, Class: "tabular-nums", Cell: func(i Invoice) tela.Node { return Text(money(i.Cents)) }},
					{Header: tr("table.action", "Action"), Width: "140px", Cell: func(i Invoice) tela.Node {
						return kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text(tr("table.view", "View {id}", "id", i.ID))}
					}},
				}}),
		demo("Empty", "Empty is drawn in a row of its own.", col,
			kit.Table[Invoice]{Class: "max-w-xl", AriaLabel: tr("table.noInvoices", "No invoices"), Rows: nil,
				Empty: Div(Class("py-6 text-center text-sm text-muted-foreground"), Text(tr("table.empty", "No results."))),
				Columns: []kit.Column[Invoice]{
					{Header: tr("table.invoice", "Invoice"), Cell: func(i Invoice) tela.Node { return Text(i.ID) }},
					{Header: tr("table.amount", "Amount"), Align: kit.AlignRight, Cell: func(i Invoice) tela.Node { return Text(money(i.Cents)) }},
				}}),
	)
}
