package devtools

import (
	"strconv"
	"strings"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/kit"
)

// SemRow is a line of the aria snapshot: a node of the semantics tree.
type SemRow struct {
	Depth int
	// Role is "button", "heading", "text"...; Name its accessible name,
	// States its flags ("disabled", "level=1"), Value what follows the
	// colon.
	Role, Name, Value string
	States            []string
	// Element is the element of the node, 0 for text lines.
	Element core.ElementID
}

// ParseSemantics reads the lines of the aria snapshot of a window (the YAML
// of tela inspect): "- button \"Save\" [disabled]", "- textbox \"Name\":
// Ana". Lines it cannot read are kept whole as text.
func ParseSemantics(s Semantics) []SemRow {
	var rows []SemRow
	for i, line := range strings.Split(strings.TrimRight(s.YAML, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		trim := strings.TrimLeft(line, " ")
		row := SemRow{Depth: (len(line) - len(trim)) / 2}
		if i < len(s.Elements) {
			row.Element = s.Elements[i]
		}
		rest, ok := strings.CutPrefix(trim, "- ")
		if !ok {
			row.Role, row.Value = "text", trim
			rows = append(rows, row)
			continue
		}
		end := strings.IndexAny(rest, " :")
		if end < 0 {
			end = len(rest)
		}
		row.Role, rest = rest[:end], rest[end:]
		rest = strings.TrimLeft(rest, " ")
		if q, err := strconv.QuotedPrefix(rest); err == nil {
			row.Name, _ = strconv.Unquote(q)
			rest = strings.TrimLeft(rest[len(q):], " ")
		}
		for strings.HasPrefix(rest, "[") {
			j := strings.IndexByte(rest, ']')
			if j < 0 {
				break
			}
			row.States = append(row.States, rest[1:j])
			rest = strings.TrimLeft(rest[j+1:], " ")
		}
		if v, ok := strings.CutPrefix(rest, ":"); ok {
			row.Value = strings.TrimSpace(v)
		} else if rest != "" {
			row.Value = rest
		}
		rows = append(rows, row)
	}
	return rows
}

// A11yPanel is the Accessibility tab: the semantics tree of the window,
// each node tied to its element, and the details of the selection.
type A11yPanel struct{ panelProps }

// Build returns the panel.
func (p A11yPanel) Build() tela.Node {
	v, m := p.V, p.M
	rows := tela.UseMemo(func() []SemRow { return ParseSemantics(v.Sem) }, v.SemRev)
	var body tela.Child
	if len(rows) == 0 {
		body = Div(Class("min-h-0 flex-1"), empty(iconLayers, "No semantics yet", "The accessibility tree of the window appears here."))
	} else {
		body = VirtualList(len(rows), func(int) float32 { return 26 }, func(i int) tela.Node {
			r := rows[i]
			return SemLine{Row: r, Selected: r.Element != 0 && r.Element == v.Selected, M: m}
		}, ItemHeight(26), Role("tree"), AriaLabel("Accessibility tree"), Class("min-h-0 flex-1"),
			// Off the rows, the app shows the selection's bounds.
			OnMouseLeave(func() { m.Highlight(v.Selected) }))
	}
	// The highlight is this panel's while it shows.
	tela.UseEffect(func() func() { return func() { m.Highlight(0) } }, tela.Once)
	return DetailsSplit{Width: p.Details, OnWidth: p.SetDetails, Pane: DetailsPane{V: v, M: m},
		Main: Fragment(
			Div(Class("flex h-10 shrink-0 items-center gap-2 border-b border-border bg-card px-3"),
				Span(Class("text-xs text-muted-foreground"), Textf("%d nodes", len(rows))),
				Div(Class("flex-1")),
				kit.Button{Variant: kit.Ghost, Size: kit.Xs, Class: "text-xs", OnClick: m.Refresh, Children: Text("Refresh")},
			),
			body,
		)}
}

// SemLine is a node of the semantics tree.
type SemLine struct {
	Row      SemRow
	Selected bool
	M        *Model
}

// Build returns the line.
func (s SemLine) Build() tela.Node {
	r, m := s.Row, s.M
	clickable := r.Element != 0
	isText := r.Role == "text"
	return Div(
		Role("treeitem"), AriaSelected(s.Selected), AriaLevel(r.Depth+1), AriaLabel(strings.TrimSpace(r.Role+" "+r.Name)),
		Class("flex h-[26px] items-center gap-2 pr-3 text-[12px] whitespace-nowrap", ifs(clickable, "cursor-default", ""),
			ifs(s.Selected, "bg-primary/20", ifs(clickable, "hover:bg-accent/60", "")),
			"pl-["+strconv.Itoa(12+r.Depth*16)+"px]"),
		// As in the Elements panel, the node's element shows in the app
		// while the pointer is on its line, and once it is selected.
		If(clickable, OnClick(func() {
			m.Select(r.Element, true)
			m.Highlight(r.Element)
		})),
		OnMouseEnter(func() { m.Highlight(r.Element) }), // a text line has none: 0 clears it
		Span(Class("shrink-0 font-mono", ifs(isText, "text-muted-foreground", "font-medium text-violet-700 dark:text-violet-300")), Text(r.Role)),
		If(r.Name != "", Span(Class("truncate"), Textf("%q", r.Name))),
		Map(r.States, func(st string) tela.Node {
			return Span(Key(st), Class("shrink-0 rounded bg-muted px-1.5 font-mono text-[10px] leading-4 text-muted-foreground"), Text(st))
		}),
		If(r.Value != "", Span(Class("truncate", ifs(isText, "text-foreground/80", "text-muted-foreground")), Text(r.Value))),
	)
}
