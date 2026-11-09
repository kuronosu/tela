package main

import (
	"strings"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The Introduction: the page the gallery opens on, in the spirit of
// shadcn/ui's docs and of examples/hello. A hero (an eyebrow in small tracked
// capitals, a very large headline, a tagline, a hairline and a quiet line),
// then what kit is, how an app uses it, and the components, each a button that
// opens its page. Only theme tokens, so it follows the scheme and the style.
// Its texts are written with tr (English here, Spanish and Arabic in the
// catalogs); the code and the names of the components stay as they are.

// quickStart is the code the page shows.
const quickStart = `var theme = kit.Theme()

func Hello() tela.Node { return tela.C(hello) }

func hello() tela.Node {
	name, setName := tela.UseState("")
	return Div(Class("flex max-w-sm flex-col gap-3 p-6"),
		kit.Input{Placeholder: "Your name", Value: &name,
			OnValueChange: setName.Set},
		kit.Button{Children: Textf("Hello, %s", name)})
}

func main() {
	tela.Main(tela.App{Title: "Hello", Theme: &theme,
		RootClass: "bg-background text-foreground", Root: Hello()})
}`

func introPage() tela.Node {
	goTo := tela.UseContext(navigate)
	tr := trFor(i18n.UseFormatter())
	return Div(Class("flex flex-col"),
		// The hero.
		Div(Class("flex min-h-[26rem] flex-col justify-center px-4 py-10"),
			Span(Class("text-sm font-medium tracking-widest text-muted-foreground"), Text("TELA / KIT")),
			H1(Class("mt-4 text-8xl font-bold tracking-tight"), Text(tr("intro.hello", "Hello, kit."))),
			Span(Class("mt-2 text-4xl text-muted-foreground"), Text(tr("intro.tagline", "Components, woven with Go."))),
			Div(Class("my-8 h-px bg-border")),
			Span(Class("text-sm text-muted-foreground"),
				Text(tr("intro.count", "{n} components. Light and dark. Every one restyled with a class string.", "n", len(pages)))),
		),
		// What it is, how to use it, and the components.
		Div(Class("flex max-w-3xl flex-col gap-12 px-4 pt-6 pb-16"),
			section(tr("intro.whatTitle", "What it is"),
				P(Class("text-base leading-relaxed text-muted-foreground"),
					Text(tr("intro.what1", "kit is shadcn/ui for Go: buttons, dialogs, menus, date pickers and the rest, with the same look. "+
						"Each component is built on the behavior hooks of package aria, which handle the keyboard, the focus and what a screen reader reads, "+
						"and is styled by a recipe, a plain set of class strings, and by the theme's color tokens, so light and dark need no extra classes."))),
				P(Class("text-base leading-relaxed text-muted-foreground"),
					Text(tr("intro.what2", "Change a recipe and every component of that kind changes: the Rounded style in the header is kit.DefaultStyle() with about forty edited class strings. "+
						"Switch the language in the header to see the same pages in Spanish and, mirrored, in Arabic.")))),
			section(tr("intro.useTitle", "Use it"),
				P(Class("text-base leading-relaxed text-muted-foreground"),
					Text(tr("intro.use", "Give the app kit.Theme(), then write components as structs or h elements."))),
				Div(Class("overflow-x-auto rounded-xl border border-border bg-muted/50 p-5"),
					Div(Dir("ltr"), Class("font-mono text-sm leading-relaxed whitespace-pre"), Text(strings.ReplaceAll(quickStart, "\t", "    "))))),
			section(tr("intro.componentsTitle", "Components"),
				P(Class("text-base leading-relaxed text-muted-foreground"),
					Text(tr("intro.pages", "{n} pages, each with its variants, sizes and states. Choose one.", "n", len(pages)))),
				Div(Class("grid grid-cols-2 gap-2 md:grid-cols-3 xl:grid-cols-4"), Fragment(componentButtons(goTo)...))),
		),
	)
}

// section is a heading over its content.
func section(title string, children ...tela.Child) tela.Node {
	return Div(Class("flex flex-col gap-4"),
		H2(Class("text-2xl font-semibold tracking-tight"), Text(title)),
		Fragment(children...))
}

// componentButtons are the names of the components, each a button that
// selects its page.
func componentButtons(goTo tela.Setter[string]) []tela.Child {
	out := make([]tela.Child, len(pages))
	for i, p := range pages {
		out[i] = kit.Button{
			Variant: kit.Outline, Size: kit.Sm, Class: "justify-start", Props: Key(p.Name),
			OnClick: func() {
				if goTo != (tela.Setter[string]{}) {
					goTo.Set(p.Name)
				}
			},
			Children: Text(p.Label),
		}
	}
	return out
}
