package devtools

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// kindParts splits "main.Settings" into "main." and "Settings", so the
// package can be dimmed; primitives have none.
func kindParts(kind string) (pkg, name string) {
	end := strings.IndexByte(kind, '[')
	if end < 0 {
		end = len(kind)
	}
	if i := strings.LastIndexByte(kind[:end], '.'); i >= 0 && i < end-1 {
		return kind[:i+1], kind[i+1:]
	}
	return "", kind
}

// fmtMs formats a duration in milliseconds with two decimals.
func fmtMs(d time.Duration) string {
	return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 2, 64) + " ms"
}

// fmtPx formats a length of the layout: 12 or 12.5.
func fmtPx(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) }

// fmtCount formats n with thousands separators.
func fmtCount(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// Section is a titled group of a details pane that folds when its title
// is pressed.
type Section struct {
	Title string
	// Aside is a short note at the end of the title ("5").
	Aside    string
	Children tela.Component
}

// Build returns the section.
func (s Section) Build() tela.Node {
	open, setOpen := tela.UseState(true)
	icon := iconChevronDown
	if !open {
		icon = iconChevronRight
	}
	return Div(Class("border-b border-border"),
		Button(Class("flex h-8 w-full cursor-default items-center gap-1.5 px-3 text-left outline-none hover:bg-accent/40 focus-visible:bg-accent/40"),
			AriaExpanded(open), OnClick(func() { setOpen.Set(!open) }),
			Icon(icon, Class("size-3.5 text-muted-foreground")),
			Span(Class("text-[11px] font-semibold uppercase tracking-wider text-muted-foreground"), Text(s.Title)),
			Span(Class("flex-1")),
			If(s.Aside != "", Span(Class("text-[11px] text-muted-foreground"), Text(s.Aside)))),
		If(open, Div(Class("pb-3"), s.Children)),
	)
}

func section(key, title, aside string, kids ...tela.Child) tela.Node {
	return tela.WithKey(Section{Title: title, Aside: aside, Children: Fragment(kids...)}, key)
}

// empty is the placeholder of a pane with nothing to show.
func empty(icon tela.Icon, title, hint string) tela.Node {
	return Div(Class("flex h-full flex-col items-center justify-center gap-2 px-8 text-center"),
		Icon(icon, Class("size-6 text-muted-foreground/60")),
		Span(Class("text-[13px] font-medium"), Text(title)),
		If(hint != "", Span(Class("max-w-64 text-xs text-muted-foreground"), Text(hint))),
	)
}

// kv is a row of a key and a value, the value in the monospace font.
func kv(key, value string, valueClass string) tela.Node {
	return Div(Class("flex min-h-5 items-baseline gap-3 px-3 py-0.5"),
		Span(Class("w-24 shrink-0 text-xs text-muted-foreground"), Text(key)),
		Span(Class("min-w-0 flex-1 font-mono text-xs break-words", valueClass), Text(value)))
}

// plural is "1 build" or "3 builds".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
