package devtools

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Toolbar is the top bar: the session and the window the panels show.
type Toolbar struct {
	V View
	M *Model
}

// Build returns the toolbar.
func (t Toolbar) Build() tela.Node {
	v := t.V
	dot, status := "bg-emerald-500", v.Describe()
	switch v.Conn {
	case Connecting:
		dot = "bg-amber-400"
	case Ended:
		dot = "bg-red-500"
	}
	var win tela.Child
	switch {
	case len(v.Windows) > 1:
		items := make([]kit.SelectItem[uint32], len(v.Windows))
		for i, w := range v.Windows {
			items[i] = kit.SelectItem[uint32]{Value: w.ID, Label: windowLabel(w)}
		}
		sel := v.Window
		win = kit.Select[uint32]{Items: items, Value: &sel, OnValueChange: t.M.SelectWindow, Size: kit.Sm,
			AriaLabel: "Window", Class: "h-7 min-w-52 text-xs", ContentClass: "text-xs"}
	case len(v.Windows) == 1:
		win = Span(Class("truncate text-xs text-muted-foreground"), Text(windowLabel(v.Windows[0])))
	}
	return Div(Class("flex h-11 shrink-0 items-center gap-3 border-b border-border bg-card px-3"),
		Div(Class("flex items-center gap-2"),
			Icon(iconLayers, Class("size-4 text-primary")),
			Span(Class("text-[13px] font-semibold tracking-tight"), Text("Tela DevTools"))),
		Div(Class("h-4 w-px bg-border")),
		Div(Class("flex min-w-0 items-center gap-2 text-xs"),
			Span(Class("size-2 shrink-0 rounded-full", dot), AriaLabel(status), Role("img")),
			Span(Class("truncate font-mono text-[12px]"), Text(v.Info.Pkg)),
			Span(Class("shrink-0 text-muted-foreground"), Text(status)),
		),
		Div(Class("flex-1")),
		win,
	)
}

func windowLabel(w Window) string {
	s := w.Title
	if s == "" {
		s = fmt.Sprintf("Window %d", w.ID)
	}
	return fmt.Sprintf("%s  %gx%g", s, w.Width, w.Height)
}

// StatusBar is the line at the bottom: the last error, or that the session
// ended.
type StatusBar struct{ V View }

// Build returns the bar, or nothing when there is nothing to say.
func (s StatusBar) Build() tela.Node {
	switch {
	case s.V.Conn == Ended:
		return Div(Class("flex h-7 shrink-0 items-center gap-2 border-t border-border bg-destructive/10 px-3 text-xs text-destructive"),
			Role("status"), Text("The tela dev session ended. Closing the DevTools."))
	case s.V.Err != "":
		return Div(Class("flex h-7 shrink-0 items-center gap-2 border-t border-border bg-amber-500/10 px-3 text-xs text-amber-700 dark:text-amber-400"),
			Role("status"), Span(Class("truncate"), Text(s.V.Err)))
	}
	return nil
}
