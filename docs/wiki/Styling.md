# Styling

Tela styles elements with the vocabulary of **Tailwind CSS v4**. There is no
cascade, no selectors and no stylesheet: an element's style comes from three
sources, applied in this order, and the last one wins:

1. The element's defaults (a `Button` has the pointer cursor and no
   background; headings are unstyled, like Tailwind's Preflight).
2. The classes of [`Class`](https://pkg.go.dev/github.com/kuronosu/tela/h#Class), in order of appearance. `Class` may repeat; a later
   class wins over an earlier one that sets the same property.
3. The typed props of [`Style`](https://pkg.go.dev/github.com/kuronosu/tela/h#Style), for values computed in code.

## Classes

```go
func Card(title, body string) tela.Node {
	return Div(Class("flex flex-col gap-2 rounded-xl border border-slate-200 bg-white p-6 shadow-md"),
		H2(Class("text-lg font-semibold text-slate-900"), Text(title)),
		P(Class("text-sm leading-6 text-slate-500"), Text(body)),
	)
}
```

What is supported: spacing, sizing, colors (with opacity as `bg-white/15`),
borders, radius, shadows, gradients (`bg-linear-to-br from-indigo-500
to-fuchsia-600`), typography, block and flex layout, relative and absolute
positioning, overflow, transforms, transitions (`transition`, `duration-*`, `ease-*`, `delay-*`, also with `hover:` and `dark:`), the `animate-spin`, `animate-ping`, `animate-pulse` and `animate-bounce` classes, and arbitrary values (`w-[calc(50%-8px)]`,
`bg-[#1e1e2e]`). The vocabulary is generated from Tailwind v4's theme; the v3
aliases that mean the same thing are accepted (`bg-gradient-to-br`). Grid,
`fixed`/`sticky` positioning and custom keyframes are not implemented
yet; check the [roadmap](https://github.com/kuronosu/tela/blob/main/docs/ROADMAP.md)
before relying on them.

A class Tela does not know is not silently ignored: in dev mode
(`TELA_DEV=1`, or `telatest.Dev()` in a test) it is reported with the closest
valid class, for example `unknown class "rounded-xxl"; did you mean
"rounded-2xl"?`.

## Typed style props

Static styling belongs in classes. For values that come from code (a measured
width, a color from state), [`Style`](https://pkg.go.dev/github.com/kuronosu/tela/h#Style) takes the typed props of package `tela`.
They apply after the classes, so they win over a class that sets the same
property. Lengths are logical pixels.

```go
func Meter(width float32, accent tela.Color) tela.Node {
	return Div(Class("h-2 rounded-full bg-slate-200"),
		Div(Class("h-2 rounded-full"),
			Style(tela.Width(width), tela.Bg(accent))),
	)
}
```

The props, by area:

| Area | Props |
|---|---|
| Size | [`tela.Width`](https://pkg.go.dev/github.com/kuronosu/tela#Width), [`tela.Height`](https://pkg.go.dev/github.com/kuronosu/tela#Height), [`tela.MinWidth`](https://pkg.go.dev/github.com/kuronosu/tela#MinWidth), [`tela.MinHeight`](https://pkg.go.dev/github.com/kuronosu/tela#MinHeight), [`tela.MaxWidth`](https://pkg.go.dev/github.com/kuronosu/tela#MaxWidth), [`tela.MaxHeight`](https://pkg.go.dev/github.com/kuronosu/tela#MaxHeight) |
| Spacing | [`tela.Padding`](https://pkg.go.dev/github.com/kuronosu/tela#Padding), [`tela.PaddingXY`](https://pkg.go.dev/github.com/kuronosu/tela#PaddingXY), [`tela.Margin`](https://pkg.go.dev/github.com/kuronosu/tela#Margin), [`tela.Gap`](https://pkg.go.dev/github.com/kuronosu/tela#Gap) |
| Position | [`tela.Left`](https://pkg.go.dev/github.com/kuronosu/tela#Left), [`tela.Top`](https://pkg.go.dev/github.com/kuronosu/tela#Top), [`tela.Right`](https://pkg.go.dev/github.com/kuronosu/tela#Right), [`tela.Bottom`](https://pkg.go.dev/github.com/kuronosu/tela#Bottom), [`tela.ZIndex`](https://pkg.go.dev/github.com/kuronosu/tela#ZIndex) |
| Color | [`tela.Bg`](https://pkg.go.dev/github.com/kuronosu/tela#Bg), [`tela.BackgroundGradient`](https://pkg.go.dev/github.com/kuronosu/tela#BackgroundGradient), [`tela.TextColor`](https://pkg.go.dev/github.com/kuronosu/tela#TextColor), [`tela.BorderColor`](https://pkg.go.dev/github.com/kuronosu/tela#BorderColor), [`tela.Opacity`](https://pkg.go.dev/github.com/kuronosu/tela#Opacity) |
| Border and shadow | [`tela.BorderWidth`](https://pkg.go.dev/github.com/kuronosu/tela#BorderWidth), [`tela.Radius`](https://pkg.go.dev/github.com/kuronosu/tela#Radius), [`tela.BoxShadow`](https://pkg.go.dev/github.com/kuronosu/tela#BoxShadow) |
| Transform | [`tela.Translate`](https://pkg.go.dev/github.com/kuronosu/tela#Translate), [`tela.Scale`](https://pkg.go.dev/github.com/kuronosu/tela#Scale), [`tela.Rotate`](https://pkg.go.dev/github.com/kuronosu/tela#Rotate) |
| Text | [`tela.FontSize`](https://pkg.go.dev/github.com/kuronosu/tela#FontSize), [`tela.FontWeight`](https://pkg.go.dev/github.com/kuronosu/tela#FontWeight) |
| Deferred (see [Hooks](Hooks#observable-values-usevalue)) | [`tela.TranslateFn`](https://pkg.go.dev/github.com/kuronosu/tela#TranslateFn), [`tela.OpacityFn`](https://pkg.go.dev/github.com/kuronosu/tela#OpacityFn), [`tela.WidthFn`](https://pkg.go.dev/github.com/kuronosu/tela#WidthFn), [`tela.HeightFn`](https://pkg.go.dev/github.com/kuronosu/tela#HeightFn) |

### Colors, gradients and shadows

A [`tela.Color`](https://pkg.go.dev/github.com/kuronosu/tela#Color) is built with [`tela.Hex`](https://pkg.go.dev/github.com/kuronosu/tela#Hex) (`"#rgb"`, `"#rrggbb"`, with or without
alpha; it panics on a malformed literal), [`tela.RGB`](https://pkg.go.dev/github.com/kuronosu/tela#RGB), [`tela.RGBA`](https://pkg.go.dev/github.com/kuronosu/tela#RGBA) or [`tela.OKLCH`](https://pkg.go.dev/github.com/kuronosu/tela#OKLCH)
(the notation of Tailwind v4's palette). [`tela.Transparent`](https://pkg.go.dev/github.com/kuronosu/tela#Transparent), [`tela.Black`](https://pkg.go.dev/github.com/kuronosu/tela#Black) and
[`tela.White`](https://pkg.go.dev/github.com/kuronosu/tela#White) are predefined. Colors are extended sRGB, so a color outside the
sRGB gamut keeps its components and is clipped when painted, like browsers do.

```go
func Banner() tela.Node {
	gradient := tela.LinearGradient(135,
		tela.Stop(tela.Hex("#6366f1")),
		tela.StopAt(tela.OKLCH(0.667, 0.295, 322.15), 100),
	)
	glow := tela.Shadow{OffsetY: 8, Blur: 24, Color: tela.RGBA(99, 102, 241, 0.4)}

	return Div(Class("h-32 rounded-2xl"),
		Style(tela.BackgroundGradient(gradient), tela.BoxShadow(glow)),
	)
}
```

[`tela.LinearGradient`](https://pkg.go.dev/github.com/kuronosu/tela#LinearGradient) takes an angle in degrees (0 points up, 90 right, 180
down) and at least two stops from [`tela.Stop`](https://pkg.go.dev/github.com/kuronosu/tela#Stop) (spread evenly, as in CSS) or
[`tela.StopAt`](https://pkg.go.dev/github.com/kuronosu/tela#StopAt) (a percentage of the gradient line). Colors interpolate in Oklab,
like Tailwind v4.

## Variants

A variant prefix makes a class conditional, as in Tailwind: `hover:bg-white/25`.
Variants combine (`dark:md:hover:bg-slate-700`); every part must hold.

| Kind | Variants |
|---|---|
| State | `hover:` `active:` `focus:` `focus-visible:` `focus-within:` `disabled:` `enabled:` `checked:` |
| Structure | `first:` `last:` `only:` `odd:` `even:` |
| Text fields | `read-only:` `placeholder-shown:` `invalid:` `placeholder:` (style the placeholder text) `selection:` (style the selected text) |
| Ancestors and siblings | `group-hover:` and the other state variants after `group-`, on a descendant of an element with the class `group`; `peer-*` for the following siblings of an element with the class `peer` (named groups and peers are not supported) |
| Environment | `dark:` `motion-reduce:` `motion-safe:` and the breakpoints `sm:` `md:` `lg:` `xl:` `2xl:` (by window width) |

```go
func Row(name string) tela.Node {
	return Div(Class("group flex items-center justify-between rounded-lg px-3 py-2 hover:bg-slate-100"),
		Span(Class("text-slate-900"), Text(name)),
		Button(Class("rounded-md px-2 py-1 text-sm text-slate-500 opacity-0 "+
			"group-hover:opacity-100 hover:bg-slate-200 focus-visible:opacity-100"),
			Text("Rename")),
	)
}
```

## Dark mode

`dark:` applies when the system prefers a dark color scheme. Write both looks
with a class pair; the `dark:` class overrides the plain one when dark mode
is on:

```go
func Panel() tela.Node {
	return Div(Class("rounded-xl bg-white p-6 text-slate-900 dark:bg-zinc-900 dark:text-zinc-100"),
		Text("Hello"))
}
```

To check both, render each with the CLI (`tela snapshot --dark`) or in a test
with [`telatest.Dark`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Dark):

```go
package main

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

func TestPanelDark(t *testing.T) {
	app := telatest.Mount(t, tela.C(Panel), telatest.Size(200, 100), telatest.Dark())

	img := app.Screenshot()

	if p := img.RGBAAt(100, 5); p.R > 40 {
		t.Errorf("pixel %v: the dark: class should have made the panel dark", p)
	}
}
```

Set the window background with `RootClass` on the [`tela.App`](https://pkg.go.dev/github.com/kuronosu/tela#App), which styles the
root box that fills the window: `RootClass: "bg-white dark:bg-zinc-950"`.

## Responsive layout

The breakpoint variants look at the window width, so one tree adapts:

```go
func Gallery() tela.Node {
	return Div(Class("flex flex-col gap-4 p-4 md:flex-row"),
		Div(Class("h-24 grow rounded-lg bg-indigo-500")),
		Div(Class("h-24 grow rounded-lg bg-fuchsia-500")),
	)
}
```

## See also

- [Elements and Props](Elements-and-Props) for the defaults each element has.
- [CLI](CLI) to look at the result as a PNG, and `tela inspect --layout` for
  the boxes as JSON.
- The `examples/` directory: `counter` is checked pixel by pixel against
  Chromium rendering the same HTML and Tailwind CSS.
