package devtools

import (
	"fmt"
	"slices"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/kit"
)

// DetailsPane shows the selected element: where it is in the source, why
// it rendered, its hooks and props, and how it is laid out.
type DetailsPane struct {
	V View
	M *Model
}

// Build returns the pane.
func (d DetailsPane) Build() tela.Node {
	v := d.V
	if v.Selected == 0 {
		return empty(iconCrosshair, "Select an element", "Click a row, or pick one in the app with the crosshair.")
	}
	el, ok := v.selection()
	if !ok {
		return empty(iconLayers, "Element is gone", "It was unmounted by a rebuild. Select another one.")
	}
	detail := v.Detail != nil
	var kids []tela.Child
	kids = append(kids, ElementHeader{El: el})
	if !detail {
		kids = append(kids, Div(Class("px-3 py-4 text-xs text-muted-foreground"), Text("Loading details...")))
	} else {
		if el.Component || len(el.Recent) > 0 {
			kids = append(kids, WhyRendered{El: el, Frame: 0})
		}
		if len(el.Hooks) > 0 {
			kids = append(kids, section("hooks", "Hooks", fmt.Sprint(len(el.Hooks)), Map(el.Hooks, func(hk core.HookInfo) tela.Node { return tela.WithKey(HookRow{Hook: hk}, hk.Index) })))
		}
		if len(el.Props) > 0 {
			kids = append(kids, section("props", "Props", fmt.Sprint(len(el.Props)), Map(el.Props, func(p core.PropInfo) tela.Node {
				return Div(Key(p.Name), Class("flex items-baseline gap-3 px-3 py-0.5 font-mono text-xs"),
					Span(Class("w-24 shrink-0 truncate text-muted-foreground"), Text(p.Name)),
					Span(Class("min-w-0 flex-1 break-words"), Text(p.Value)))
			})))
		}
		switch {
		case v.Box != nil && v.Box.Element == el.ID:
			kids = append(kids, section("box", "Box model", "", BoxModel{Box: *v.Box}))
			kids = append(kids, section("style", "Computed style", "", StyleList{S: v.Box.Style}))
		case el.Component:
			kids = append(kids, Div(Class("px-3 py-4 text-xs text-muted-foreground"),
				Text("A component has no box of its own. Select an element it renders to see its box model and style.")))
		}
	}
	return Div(Class("min-h-0 flex-1 overflow-y-auto"), Role("region"), AriaLabel("Element details"), Fragment(kids...))
}

// selection returns the selected element: its details when they arrived,
// else its row of the tree.
func (v View) selection() (core.ElementInfo, bool) {
	if v.Detail != nil && v.Detail.ID == v.Selected {
		return *v.Detail, true
	}
	if i, ok := v.Index[v.Selected]; ok {
		return v.Els[i], true
	}
	return core.ElementInfo{}, false
}

// ElementHeader names the element and says where in the source it is.
type ElementHeader struct{ El core.ElementInfo }

// Build returns the header.
func (e ElementHeader) Build() tela.Node {
	el := e.El
	pkg, name := kindParts(el.Kind)
	title := Fragment(
		Span(Class("text-muted-foreground"), Text(pkg)),
		Span(Class("font-semibold", ifs(el.Component, "text-violet-700 dark:text-violet-300", "text-sky-700 dark:text-sky-300")), Text(name)),
	)
	kind := "element"
	switch {
	case el.Kind == "text":
		kind = "text"
		title = Span(Class("font-semibold text-emerald-700 dark:text-emerald-300"), Textf("%q", el.Text))
	case el.Component:
		kind = "component"
	default:
		title = Span(Class("font-semibold text-sky-700 dark:text-sky-300"), Text("<"+el.Kind+">"))
	}
	size := ""
	if el.Bounds != nil {
		size = fmt.Sprintf("%gx%g", el.Bounds.Width(), el.Bounds.Height())
	}
	return Div(Class("border-b border-border px-3 py-3"),
		Div(Class("flex items-center gap-2"),
			Div(Class("min-w-0 flex-1 truncate font-mono text-[14px]"), title),
			kit.Badge{Variant: kit.Secondary, Class: "shrink-0 text-[10px] uppercase tracking-wide", Children: Text(kind)},
		),
		Div(Class("mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground"),
			If(!el.Site.IsZero(), Span(Class("font-mono text-foreground/90"), AriaLabel("Source"), Text(el.Site.String()))),
			If(el.Key != "", Span(Class("font-mono text-amber-700 dark:text-amber-300"), Textf("key=%q", el.Key))),
			If(el.Builds > 0, Span(Text(plural(el.Builds, "build")))),
			If(size != "", Span(Class("font-mono"), Text(size))),
		),
		If(el.Classes != "", Div(Class("mt-2 rounded-md bg-muted px-2 py-1.5 font-mono text-[11px] leading-4 break-words text-muted-foreground"), Text(el.Classes))),
	)
}

// WhyRendered lists the builds of a component, newest first, with the
// causes of each in plain words. Frame, when not 0, shows only the build
// of that frame.
type WhyRendered struct {
	El    core.ElementInfo
	Frame int
}

// Build returns the section.
func (w WhyRendered) Build() tela.Node {
	recs := slices.Clone(w.El.Recent)
	slices.Reverse(recs)
	title := "Why it rendered"
	if len(recs) == 0 {
		return section("why", title, "", Div(Class("px-3 py-1 text-xs text-muted-foreground"),
			Text("No build recorded since the DevTools attached. Use the app and the builds appear here.")))
	}
	aside := fmt.Sprintf("last %d of %d builds", len(recs), max(w.El.Builds, len(recs)))
	return section("why", title, aside, Map(recs, func(r core.BuildRecord) tela.Node {
		return tela.WithKey(BuildRow{Rec: r, Newest: r.Frame == recs[0].Frame}, r.Frame)
	}))
}

// BuildRow is one build and its causes.
type BuildRow struct {
	Rec    core.BuildRecord
	Newest bool
}

// Build returns the row.
func (b BuildRow) Build() tela.Node {
	return Div(Key(b.Rec.Frame), Class("flex gap-3 px-3 py-1", ifs(b.Newest, "", "opacity-75")),
		Span(Class("w-16 shrink-0 pt-0.5 font-mono text-[11px] text-muted-foreground"), Textf("frame %d", b.Rec.Frame)),
		Div(Class("flex min-w-0 flex-1 flex-col gap-1"),
			Map(b.Rec.Reasons, func(r core.BuildReason) tela.Node { return tela.WithKey(Reason{R: r}, r.String()) })))
}

// Reason is a cause of a build: its kind as a tag and a sentence.
type Reason struct{ R core.BuildReason }

// Build returns the line.
func (r Reason) Build() tela.Node {
	return Div(Key(r.R.String()), Class("flex items-start gap-2 text-xs leading-5"),
		Span(Class("mt-0.5 inline-flex h-4 shrink-0 items-center rounded px-1.5 font-mono text-[10px] leading-none", causeClass(r.R.Cause)), Text(r.R.Cause.String())),
		Span(Class("min-w-0 break-words"), Text(sentence(r.R))))
}

// sentence is the reason in plain words: core's own text ("UseState #1 at
// counter.go:12 was set"), which starts in lower case after the cause's tag.
func sentence(r core.BuildReason) string { return r.String() }

func causeClass(c core.BuildCause) string {
	switch c {
	case core.CauseState, core.CauseReducer, core.CauseAsync:
		return "bg-sky-500/15 text-sky-700 dark:bg-sky-400/15 dark:text-sky-300"
	case core.CauseProps, core.CauseParent:
		return "bg-amber-500/15 text-amber-700 dark:bg-amber-400/15 dark:text-amber-300"
	case core.CauseContext, core.CauseValue, core.CauseScheme, core.CauseDirection:
		return "bg-violet-500/15 text-violet-700 dark:bg-violet-400/15 dark:text-violet-300"
	case core.CauseMount, core.CauseReload:
		return "bg-emerald-500/15 text-emerald-700 dark:bg-emerald-400/15 dark:text-emerald-300"
	}
	return "bg-muted text-muted-foreground"
}

// HookRow is a hook of a component.
type HookRow struct{ Hook core.HookInfo }

// Build returns the row.
func (h HookRow) Build() tela.Node {
	hk := h.Hook
	return Div(Key(hk.Index), Class("px-3 py-1"),
		Div(Class("flex items-baseline gap-2 font-mono text-xs"),
			Span(Class("w-4 shrink-0 text-right text-muted-foreground"), Textf("%d", hk.Index+1)),
			Span(Class("font-medium text-sky-700 dark:text-sky-300"), Text(hk.Kind)),
			If(!hk.Site.IsZero(), Span(Class("ml-auto text-[11px] text-muted-foreground"), Text(hk.Site.String()))),
		),
		If(hk.Value != "", Div(Class("mt-0.5 pl-6 font-mono text-xs break-words text-muted-foreground"), Text(hk.Value))),
	)
}

// StyleList is the computed style of a box as a list of names and values.
type StyleList struct{ S core.StyleSummary }

// Build returns the list.
func (s StyleList) Build() tela.Node {
	st := s.S
	var kids []tela.Child
	add := func(name, value string) {
		if value != "" {
			kids = append(kids, kv(name, value, ""))
		}
	}
	color := func(name, value string) {
		if value == "" {
			return
		}
		kids = append(kids, Div(Class("flex min-h-5 items-center gap-3 px-3 py-0.5"),
			Span(Class("w-24 shrink-0 text-xs text-muted-foreground"), Text(name)),
			Span(Class("size-3 shrink-0 rounded-sm border border-border"), Style(tela.Bg(tela.Hex(value)))),
			Span(Class("font-mono text-xs"), Text(value))))
	}
	add("display", st.Display)
	add("position", st.Position)
	add("direction", st.Direction)
	add("width", st.Width)
	add("height", st.Height)
	add("min-width", st.MinWidth)
	add("max-width", st.MaxWidth)
	add("flex", st.Flex)
	add("flex item", st.FlexItem)
	add("grid", st.Grid)
	add("grid item", st.GridItem)
	color("color", st.Color)
	color("background", st.Background)
	add("font", st.Font)
	add("text-align", st.TextAlign)
	if st.Opacity != 1 {
		add("opacity", fmtPx(st.Opacity))
	}
	for _, e := range st.Errors {
		kids = append(kids, Div(Class("px-3 py-0.5 text-xs text-destructive"), Text(e)))
	}
	return Div(Fragment(kids...))
}
