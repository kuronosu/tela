# Theming

A theme gives an app **semantic colors** that follow the color scheme, a base
**radius** and its **fonts**. The color names are shadcn/ui's, so the classes
you would write in a shadcn/ui project work as they are:

```go
func Card() tela.Node {
	return Div(Class("rounded-xl border border-border bg-card p-6 text-card-foreground shadow-sm"),
		H3(Class("font-semibold"), Text("Create project")),
		P(Class("text-sm text-muted-foreground"), Text("Deploy your new project in one click.")),
		Button(Class("mt-4 rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90"),
			Text("Deploy")),
	)
}
```

One class serves both schemes: `bg-card` is white in the light scheme and
near-black in the dark one, without a `dark:` pair. Tokens work with every
color utility (`bg-`, `text-`, `border-` and its sides, `outline-`, `ring-`,
`ring-offset-`, `inset-ring-`, `shadow-`, `inset-shadow-`, `drop-shadow-`,
`text-shadow-`, `decoration-`, `caret-`, `from-`/`via-`/`to-`), with the
opacity modifier (`ring-ring/50`, `bg-primary/[0.85]`), under any variant
(`hover:bg-accent`, `placeholder:text-muted-foreground`,
`selection:bg-primary`) and with transitions (`transition-colors` animates
a scheme change). A misspelled token is reported in dev mode like any
unknown class: `unknown class "bg-primry"; did you mean "bg-primary"?`.

## The tokens

Without a theme an app has [`tela.DefaultTheme`](https://pkg.go.dev/github.com/kuronosu/tela#DefaultTheme),
shadcn/ui's **neutral** theme (v4, converted from OKLCH like the Tailwind
palette):

| Token | Use |
|---|---|
| `background`, `foreground` | The page and its text |
| `card`, `card-foreground` | Cards |
| `popover`, `popover-foreground` | Menus, popovers, tooltips |
| `primary`, `primary-foreground` | The main action |
| `secondary`, `secondary-foreground` | Secondary actions |
| `muted`, `muted-foreground` | Subdued backgrounds and text |
| `accent`, `accent-foreground` | Hovered and selected items |
| `destructive` | Dangerous actions and errors |
| `border`, `input`, `ring` | Borders, field borders, focus rings |
| `chart-1` … `chart-5` | Data series |

Give the window the page colors with `RootClass`:

```go
tela.Main(tela.App{Title: "Notes", Root: Notes(), RootClass: "bg-background text-foreground"})
```

## Your own theme

Start from the default theme and change what you need. A
[`tela.SchemeColor`](https://pkg.go.dev/github.com/kuronosu/tela#SchemeColor)
holds the light and the dark value; new names become classes too:

```go
func main() {
	theme := tela.DefaultTheme()
	theme.Colors["primary"] = tela.SchemeColor{Light: tela.OKLCH(0.55, 0.2, 264), Dark: tela.OKLCH(0.7, 0.16, 264)}
	theme.Colors["brand"] = tela.SchemeColor{Light: tela.Hex("#7c3aed"), Dark: tela.Hex("#a78bfa")} // bg-brand
	theme.Radius = 10                                                                               // shadcn/ui's 0.625rem
	theme.FontSans = "Geist"                                                                        // a system font; Inter stays as fallback

	tela.Main(tela.App{Title: "Notes", Root: Notes(), Theme: &theme, RootClass: "bg-background text-foreground"})
}
```

- **Colors.** A token missing from `Colors` keeps its default value, so the
  shadcn/ui names always resolve. A name is lowercase letters, digits and
  dashes and cannot be a palette color (`red-500`, `white`); `Run` panics
  otherwise. Tokens cannot go inside arbitrary values
  (`bg-[linear-gradient(...)]`): use the utilities (`from-primary`).
- **Radius** is shadcn/ui's `--radius` in logical pixels: `rounded-lg` is
  `Radius`, `rounded-sm` 0.6×, `rounded-md` 0.8×, `rounded-xl` 1.4×,
  `rounded-2xl` 1.8×, `rounded-3xl` 2.2× and `rounded-4xl` 2.6×, as in
  shadcn/ui v4's theme. The default, 0, keeps Tailwind's radius scale (so
  existing pages do not change); set 10 for shadcn/ui's look.
- **Fonts.** `FontSans` is the family of `font-sans` and of the default text,
  `FontMono` that of `font-mono` (code, kbd, pre); the embedded Inter and
  JetBrains Mono stay as fallbacks.

[`tela.SetTheme`](https://pkg.go.dev/github.com/kuronosu/tela#SetTheme)
replaces the theme while the app runs (safe from any goroutine, like
`SetColorScheme`): every element is styled again.

## A different scheme for part of the window

[`h.Scheme`](https://pkg.go.dev/github.com/kuronosu/tela/h#Scheme) gives an
element and everything inside it a scheme of its own: a dark sidebar in a
light app, or a light preview inside a dark editor. Tokens and `dark:`
classes inside follow it; a nested `Scheme` wins.

```go
func Layout() tela.Node {
	return Div(Class("flex h-full"),
		Div(Scheme(tela.Dark), Class("w-64 border-r border-border bg-background text-foreground"),
			Sidebar()),
		Content(),
	)
}
```

The element with `Scheme` uses the scheme itself (`bg-background` on it is
the dark background), unlike shadcn/ui's `.dark` variant, which only matches
inside the element. Content of an [`h.Portal`](Elements-and-Props#portals)
follows the scheme of the portal's position.

## Tokens in Go

[`tela.UseTheme`](https://pkg.go.dev/github.com/kuronosu/tela#UseTheme)
returns the theme's values for the scheme at the component's position (the
nearest `Scheme` above it, else the app's), for what classes cannot reach:
canvases, images tinted in Go, computed styles. The component builds again
when the scheme or the theme changes.

```go
func Legend(series []string) tela.Node {
	th := tela.UseTheme()
	return Div(Class("flex gap-3"), MapIndex(series, func(i int, name string) tela.Node {
		return Div(Key(name), Class("flex items-center gap-1.5 text-sm"),
			Div(Class("size-2 rounded-full"), Style(tela.Bg(th.Color(fmt.Sprintf("chart-%d", i%5+1))))),
			Text(name))
	}))
}
```

`Color` also accepts palette names (`"red-500"`); an unknown name panics in
dev mode with the closest token. `Radius()` is the `rounded-lg` radius in
pixels, `FontSans()`/`FontMono()` the families and `Scheme()` the scheme the
values are for. [`tela.UseColorScheme`](https://pkg.go.dev/github.com/kuronosu/tela#UseColorScheme)
also reports the scheme at the component's position.

## Class aliases

`ClassAliases` on the [`tela.App`](https://pkg.go.dev/github.com/kuronosu/tela#App)
names class combinations an app repeats:

```go
tela.App{
	// ...
	ClassAliases: map[string]string{
		"text-muted": "text-sm text-muted-foreground",
		"card-title": "text-lg font-semibold leading-none",
		"btn":        "inline-flex h-9 items-center justify-center rounded-md px-4 text-sm font-medium",
	},
}
```

The rule: **an alias expands where it appears.**

- Later classes still win over its classes, and its classes over earlier
  ones: `Class("btn px-6")` has `px-6`.
- Its variants apply to every class of the expansion:
  `hover:card-title` is `hover:text-lg hover:font-semibold hover:leading-none`,
  and a variant inside the alias combines with them (`dark:text-white` in an
  alias used as `hover:x` needs both).
- An alias may use other aliases. A cycle (`a` → `b` → `a`) is reported like
  an unknown class, and so is a mistake inside an alias, with the alias's name.
- An alias wins over a class of the same name: above, `text-muted` is the
  alias, not the `muted` token as a text color.
- A key is one class name, without variants, spaces or brackets; `Run`
  panics otherwise. Misspelled aliases get suggestions like classes.

## Testing

[`telatest.Theme`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Theme)
and [`telatest.ClassAliases`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ClassAliases)
give a mounted app the same configuration as the `App`:

```go
func TestCardDark(t *testing.T) {
	theme := tela.DefaultTheme()
	theme.Radius = 10
	app := telatest.Mount(t, Card(), telatest.Theme(theme), telatest.ColorScheme(tela.Dark))
	app.GoldenScales("card_dark", 1, 2)
}
```
