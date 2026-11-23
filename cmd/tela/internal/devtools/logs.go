package devtools

import (
	"log/slog"
	"strings"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// LogsPanel is the Logs tab: what the app printed and the warnings its trees
// reported, filtered by level and source.
type LogsPanel struct{ panelProps }

// LogFilter chooses the entries of the log that show.
type LogFilter struct {
	// Min is the lowest level shown.
	Min slog.Level
	// Source is "app", "stderr", "tela dev" or "" for all.
	Source string
	// Search matches the message, case-insensitive.
	Search string
}

// Filter returns the entries that pass f, keeping the order.
func (f LogFilter) Filter(all []LogEntry) []LogEntry {
	search := strings.ToLower(strings.TrimSpace(f.Search))
	out := make([]LogEntry, 0, len(all))
	for _, e := range all {
		if e.Level < f.Min || f.Source != "" && e.Source != f.Source {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(e.Msg), search) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Build returns the panel.
func (p LogsPanel) Build() tela.Node {
	v, m := p.V, p.M
	level, setLevel := tela.UseState(slog.LevelDebug)
	source, setSource := tela.UseState("all")
	search, setSearch := tela.UseState("")
	follow, setFollow := tela.UseState(true)
	list := tela.UseElementRef()
	f := LogFilter{Min: level, Source: strings.TrimPrefix(source, "all"), Search: search}
	entries := tela.UseMemo(func() []LogEntry { return f.Filter(v.Logs) }, len(v.Logs), level, source, search)
	n := len(entries)
	tela.UseLayoutEffect(func() func() {
		if follow && n > 0 {
			list.ScrollToIndex(n-1, tela.AlignEnd)
		}
		return nil
	}, n, follow)

	var body tela.Child
	if n == 0 {
		title, hint := "No log yet", "What the app prints and the warnings of its trees appear here."
		if len(v.Logs) > 0 {
			title, hint = "No entry matches", "Lower the level, or clear the search."
		}
		body = Div(Class("min-h-0 flex-1"), empty(iconSearch, title, hint))
	} else {
		body = VirtualList(n, func(int) float32 { return 24 }, func(i int) tela.Node {
			return LogRow{E: entries[i]}
		}, ItemKey(func(i int) uint64 { return entries[i].Seq }), Ref(list), Role("log"), AriaLabel("Log"), Class("min-h-0 flex-1"))
	}
	return Div(Class("flex min-h-0 flex-1 flex-col"),
		Div(Class("flex h-10 shrink-0 items-center gap-2 border-b border-border bg-card px-2"),
			kit.Input{Icon: iconSearch, Placeholder: "Filter messages", AriaLabel: "Filter messages", Value: &search,
				OnValueChange: setSearch.Set, Class: "w-72", InputClass: "h-7 text-xs"},
			kit.ToggleGroup[slog.Level]{AriaLabel: "Level", Value: &level, DisallowEmptySelection: true, Variant: kit.Outline, ItemClass: "h-7 px-2.5 text-xs",
				OnValueChange: setLevel.Set, Items: []kit.ToggleGroupItem[slog.Level]{
					{Value: slog.LevelDebug, Label: "All"}, {Value: slog.LevelInfo, Label: "Info"},
					{Value: slog.LevelWarn, Label: "Warnings"}, {Value: slog.LevelError, Label: "Errors"}}},
			kit.Select[string]{AriaLabel: "Source", Value: &source, OnValueChange: setSource.Set, Size: kit.Sm, Class: "h-7 w-32 text-xs", ContentClass: "text-xs",
				Items: []kit.SelectItem[string]{{Value: "all", Label: "All sources"}, {Value: "app", Label: "App warnings"}, {Value: "stderr", Label: "Standard error"}, {Value: "tela dev", Label: "tela dev"}}},
			Div(Class("flex-1")),
			Label(Class("flex items-center gap-2 text-xs text-muted-foreground"),
				kit.Switch{Size: kit.Sm, Checked: &follow, OnCheckedChange: setFollow.Set, AriaLabel: "Follow the newest entry"}, Text("Follow")),
			kit.Button{Variant: kit.Ghost, Size: kit.Sm, Icon: iconTrash, Class: "h-7 gap-1.5 px-2.5 text-xs", Disabled: len(v.Logs) == 0,
				OnClick: m.ClearLogs, Children: Text("Clear")},
		),
		body,
		Div(Class("flex h-6 shrink-0 items-center border-t border-border bg-card px-3 text-[11px] text-muted-foreground"),
			Textf("%d of %d entries", n, len(v.Logs))),
	)
}

// LogRow is an entry of the log.
type LogRow struct{ E LogEntry }

// Build returns the row.
func (r LogRow) Build() tela.Node {
	e := r.E
	name, pill, tint := "DEBUG", "bg-muted text-muted-foreground", ""
	switch {
	case e.Level >= slog.LevelError:
		name, pill, tint = "ERROR", "bg-red-500/15 text-red-700 dark:bg-red-400/15 dark:text-red-300", "bg-red-500/[0.06]"
	case e.Level >= slog.LevelWarn:
		name, pill, tint = "WARN", "bg-amber-500/15 text-amber-700 dark:bg-amber-400/15 dark:text-amber-300", "bg-amber-500/[0.05]"
	case e.Level >= slog.LevelInfo:
		name, pill = "INFO", "bg-sky-500/15 text-sky-700 dark:bg-sky-400/15 dark:text-sky-300"
	}
	return Div(Class("flex items-baseline gap-3 border-b border-border/50 px-3 py-1 font-mono text-xs leading-5", tint),
		Span(Class("shrink-0 text-muted-foreground tabular-nums"), Text(e.At.Format("15:04:05.000"))),
		Span(Class("w-12 shrink-0 rounded px-1 text-center text-[10px] leading-4", pill), Text(name)),
		Span(Class("w-12 shrink-0 text-muted-foreground"), Text(e.Source)),
		Span(Class("min-w-0 flex-1 break-words whitespace-pre-wrap"), Text(e.Msg)),
	)
}
