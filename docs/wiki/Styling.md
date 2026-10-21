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
to-fuchsia-600`), typography, block, flex and grid layout, positioning
(`relative`, `absolute`, `fixed`, `sticky`; see [Positioning](#positioning)),
overflow, transforms, effects (filters, `mix-blend-*`,
`[clip-path:…]`, see [Effects and background images](#effects-and-background-images)), background images, transitions (`transition`, `duration-*`, `ease-*`, `delay-*`, also with `hover:` and `dark:`), the `animate-spin`, `animate-ping`, `animate-pulse` and `animate-bounce` classes, and arbitrary values (`w-[calc(50%-8px)]`,
`bg-[#1e1e2e]`). The vocabulary is generated from Tailwind v4's theme; the v3
aliases that mean the same thing are accepted (`bg-gradient-to-br`). The
theme's semantic colors (`bg-primary`, `text-muted-foreground`) and the app's
class aliases are described in [Theming](Theming). Custom
keyframes are not implemented
yet; check the [roadmap](https://github.com/kuronosu/tela/blob/main/docs/ROADMAP.md)
before relying on them.

A class Tela does not know is not silently ignored: in dev mode (on in every
development build and every `telatest` test; see
[Getting Started](Getting-Started#dev-mode)) it is reported with the closest
valid class, for example `unknown class "rounded-xxl"; did you mean
"rounded-2xl"?`, and in a test it fails the test.

[`Class`](https://pkg.go.dev/github.com/kuronosu/tela/h#Class) takes several strings and joins them with one space, skipping
the empty ones, so a class held in a variable needs no concatenation:

```go
tone := "text-slate-500"
if active {
	tone = "text-indigo-600 font-semibold"
}
Div(Class("flex items-center gap-2 rounded-lg px-3 py-1", tone))
```

### Classes that depend on state

[`If`](https://pkg.go.dev/github.com/kuronosu/tela/h#If) takes props as well as nodes, so a class that depends on state is one
more prop, without concatenating strings; [`IfElse`](https://pkg.go.dev/github.com/kuronosu/tela/h#IfElse) picks one of two:

```go
func Tab(label string, active bool) tela.Node {
	return Button(Class("rounded-md px-3 py-1.5 text-sm"),
		If(active, Class("bg-white shadow-sm")),
		IfElse(active, Class("text-slate-900"), Class("text-slate-500 hover:text-slate-700")),
		Text(label))
}
```

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

A [`tela.StyleProp`](https://pkg.go.dev/github.com/kuronosu/tela#StyleProp) is also a child, so an element takes it
directly and `If` takes it too: `Div(tela.Width(width), tela.Bg(accent),
If(dim, tela.Opacity(0.5)))` is the same as the `Style(...)` form.
[`tela.WH`](https://pkg.go.dev/github.com/kuronosu/tela#WH)`(w, h)` sets the width and the height, and
[`tela.Box`](https://pkg.go.dev/github.com/kuronosu/tela#Box)`(x, y, w, h)` the left, top, width and height of a
positioned element (a rectangle from `ElementRef.Bounds` fits it):

```go
Div(Class("absolute rounded-md ring-2 ring-indigo-500"), tela.Box(r.X, r.Y, r.Width, r.Height))
```

`WH` is not called `Size` because `tela.Size` is the type of animated sizes.

For deferred props, [`h.Scale`](https://pkg.go.dev/github.com/kuronosu/tela/h#Scale)`(v, 36)` and
[`tela.MapValue`](https://pkg.go.dev/github.com/kuronosu/tela#MapValue)`(v, f)` read a `Value` where the prop is
evaluated, replacing the closure `func() float32 { return 36 * v.Get() }`:
`Style(tela.HeightFn(Scale(open, 36)))`.

The props, by area:

| Area | Props |
|---|---|
| Size | [`tela.Width`](https://pkg.go.dev/github.com/kuronosu/tela#Width), [`tela.Height`](https://pkg.go.dev/github.com/kuronosu/tela#Height), [`tela.MinWidth`](https://pkg.go.dev/github.com/kuronosu/tela#MinWidth), [`tela.MinHeight`](https://pkg.go.dev/github.com/kuronosu/tela#MinHeight), [`tela.MaxWidth`](https://pkg.go.dev/github.com/kuronosu/tela#MaxWidth), [`tela.MaxHeight`](https://pkg.go.dev/github.com/kuronosu/tela#MaxHeight) |
| Spacing | [`tela.Padding`](https://pkg.go.dev/github.com/kuronosu/tela#Padding), [`tela.PaddingXY`](https://pkg.go.dev/github.com/kuronosu/tela#PaddingXY), [`tela.Margin`](https://pkg.go.dev/github.com/kuronosu/tela#Margin), [`tela.Gap`](https://pkg.go.dev/github.com/kuronosu/tela#Gap) |
| Position | [`tela.Left`](https://pkg.go.dev/github.com/kuronosu/tela#Left), [`tela.Top`](https://pkg.go.dev/github.com/kuronosu/tela#Top), [`tela.Right`](https://pkg.go.dev/github.com/kuronosu/tela#Right), [`tela.Bottom`](https://pkg.go.dev/github.com/kuronosu/tela#Bottom), [`tela.ZIndex`](https://pkg.go.dev/github.com/kuronosu/tela#ZIndex) |
| Color | [`tela.Bg`](https://pkg.go.dev/github.com/kuronosu/tela#Bg), [`tela.BackgroundGradient`](https://pkg.go.dev/github.com/kuronosu/tela#BackgroundGradient), [`tela.BgImage`](https://pkg.go.dev/github.com/kuronosu/tela#BgImage), [`tela.TextColor`](https://pkg.go.dev/github.com/kuronosu/tela#TextColor), [`tela.BorderColor`](https://pkg.go.dev/github.com/kuronosu/tela#BorderColor), [`tela.Opacity`](https://pkg.go.dev/github.com/kuronosu/tela#Opacity) |
| Border and shadow | [`tela.BorderWidth`](https://pkg.go.dev/github.com/kuronosu/tela#BorderWidth), [`tela.Radius`](https://pkg.go.dev/github.com/kuronosu/tela#Radius), [`tela.BoxShadow`](https://pkg.go.dev/github.com/kuronosu/tela#BoxShadow) |
| Transform | [`tela.Translate`](https://pkg.go.dev/github.com/kuronosu/tela#Translate), [`tela.Scale`](https://pkg.go.dev/github.com/kuronosu/tela#Scale), [`tela.Rotate`](https://pkg.go.dev/github.com/kuronosu/tela#Rotate) |
| Text | [`tela.FontSize`](https://pkg.go.dev/github.com/kuronosu/tela#FontSize), [`tela.FontWeight`](https://pkg.go.dev/github.com/kuronosu/tela#FontWeight) |
| Deferred (see [Hooks](Hooks#observable-values-usevalue)) | [`tela.TranslateFn`](https://pkg.go.dev/github.com/kuronosu/tela#TranslateFn), [`tela.OpacityFn`](https://pkg.go.dev/github.com/kuronosu/tela#OpacityFn), [`tela.BgFn`](https://pkg.go.dev/github.com/kuronosu/tela#BgFn), [`tela.TextColorFn`](https://pkg.go.dev/github.com/kuronosu/tela#TextColorFn), [`tela.WidthFn`](https://pkg.go.dev/github.com/kuronosu/tela#WidthFn), [`tela.HeightFn`](https://pkg.go.dev/github.com/kuronosu/tela#HeightFn) |

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

## Effects and background images

**Background images.** `bg-[url(img/hero.jpg)]` loads a file from the file
systems registered with [`tela.Assets`](https://pkg.go.dev/github.com/kuronosu/tela#Assets), usually an embedded directory
registered from `init`, so `tela.Run`, `tela snapshot`, `tela inspect` and
`telatest` all find it:

```go
//go:embed img
var images embed.FS

func init() { tela.Assets(images) }

func Hero() tela.Node {
	return Div(Class("h-64 rounded-2xl bg-[url(img/hero.jpg)] bg-cover bg-center"),
		H1(Class("p-8 text-4xl font-bold text-white"), Text("Welcome")))
}
```

Paths are relative to the root of the file system (`/img/hero.jpg` and
`./img/hero.jpg` are the same file). Several `Assets` calls are searched in
the order they were made. PNG, JPEG, GIF (first frame) and WebP are decoded
once and cached. A missing file paints nothing and is reported in dev mode
with the path and the registered file systems. For an image you already
decoded, use `Style(tela.BgImage(img))`; the background classes apply to it
too. Sizes (`bg-cover`, `bg-contain`, `bg-auto`, `bg-[length:96px_auto]`),
positions (`bg-center`, `bg-bottom-right`, `bg-[position:25%_75%]`), repeat
modes (`bg-repeat`, `bg-no-repeat`, `bg-repeat-x`, `bg-repeat-y`,
`bg-repeat-round`, `bg-repeat-space`), `bg-origin-*` and `bg-clip-*` work as
in CSS, and `bg-[linear-gradient(to_bottom,transparent,rgb(0_0_0/0.6)),url(img/hero.jpg)]`
stacks several layers, the first on top. `bg-blend-*` blends the layers with
each other and with the background color.

**Gradient text.** `bg-clip-text text-transparent` paints the background
through the glyphs of the element's text, with gradients or images, over
several lines and in inline spans:

```go
H1(Class("bg-linear-to-r from-indigo-500 to-pink-500 bg-clip-text text-5xl font-extrabold text-transparent"),
	Text("Tela"))
```

**Blending.** `mix-blend-multiply`, `mix-blend-screen` and the other modes
blend an element with what is painted under it in its stacking context; add
`isolate` (or any other stacking context: `relative z-10`, opacity, a
transform) to a parent to keep the blend inside it.

**Clip paths.** Tailwind has no clip-path utility, so it is written as the
arbitrary property, with underscores for spaces:
`[clip-path:circle(50%)]`, `[clip-path:ellipse(50%_35%_at_50%_50%)]`,
`[clip-path:inset(8px_round_1rem)]`,
`[clip-path:polygon(0_0,100%_0,100%_80%,0_100%)]` (optionally
`polygon(evenodd,…)`). Shapes are relative to the border box; the clip cuts
everything the element paints (shadows and outline included) and its
clicks. Variants work on it (`hover:[clip-path:circle(75%)]`), and
`transition-[clip-path]` animates between shapes of the same kind. No other
arbitrary property is accepted.

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
| Direction | `rtl:` `ltr:`: the element's direction, set with [`Dir`](https://pkg.go.dev/github.com/kuronosu/tela/h#Dir) on it or an ancestor (see [Right to left](#right-to-left)) |

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

## Positioning

Positioning follows CSS, measured against Chromium:

- `relative` offsets a box by its insets; `absolute` places it against its
  nearest positioned ancestor (or one with a transform or a filter, which
  contains positioned descendants in CSS).
- `fixed` places a box against the window: scrolling the page or any
  container does not move it, and the overflow clips of its ancestors do not
  cut it. A transformed or filtered ancestor (`translate-x-4`, `blur-sm`,
  `backdrop-blur`) becomes its containing block instead, as in browsers. With
  auto insets it stays at its static position.
- `sticky` keeps a box in flow and, while its scroll container scrolls,
  pushes it inside the scrollport by its insets (`top-0`, `bottom-4`,
  `left-0`...), never out of its parent: a section header sticks while its
  section scrolls by and is pushed out by the section's end. It works in
  `overflow-auto` boxes, in the window, and for the items of
  [`h.VirtualList`](Scrolling-and-VirtualList).

```go
func Page() tela.Node {
	return Div(
		Div(Class("sticky top-0 z-10 h-12 bg-white/90 px-4 shadow"), Text("Header")),
		Div(Class("h-[2000px] p-4"), Text("Content")),
		Button(Class("fixed right-4 bottom-4 rounded-full bg-indigo-600 px-4 py-2 text-white"), Text("New")),
	)
}
```

For overlays that must cover everything (modals, menus, toasts), render
them through [`h.Portal`](Elements-and-Props#portals), which also escapes the
stacking contexts and z-index of the component that renders them.

## Dark mode

`dark:` applies when the color scheme is dark: the system's preference,
unless the app chooses one with `ColorScheme` on the [`tela.App`](https://pkg.go.dev/github.com/kuronosu/tela#App)
(`tela.Light`, `tela.Dark`; the zero value `tela.System` follows the system)
or switches at run time with [`tela.SetColorScheme`](https://pkg.go.dev/github.com/kuronosu/tela#SetColorScheme)
(safe from any goroutine). The theme's semantic colors follow the scheme
by themselves (`bg-card text-card-foreground border-border`, shadcn/ui's
names), and [`h.Scheme`](https://pkg.go.dev/github.com/kuronosu/tela/h#Scheme)
gives part of the window a scheme of its own; see [Theming](Theming). With
palette colors, write both looks with a class pair; the `dark:` class
overrides the plain one when dark mode is on:

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

A component that needs the scheme in Go (to pick an image, say) reads it with
[`tela.UseColorScheme`](https://pkg.go.dev/github.com/kuronosu/tela#UseColorScheme),
which rebuilds it when the scheme changes; [`tela.UseReducedMotion`](https://pkg.go.dev/github.com/kuronosu/tela#UseReducedMotion)
does the same for the system's reduced-motion setting (`motion-reduce:`).
A toggle:

```go
func ThemeToggle() tela.Node {
	dark := tela.UseColorScheme() == tela.Dark
	next, label := tela.Dark, "Dark mode"
	if dark {
		next, label = tela.Light, "Light mode"
	}
	return Button(OnClick(func(tela.MouseEvent) { tela.SetColorScheme(next) }), Text(label))
}
```

In tests, `telatest.Dark()` makes the system prefer dark and
`telatest.ColorScheme(tela.Dark)` sets the app's scheme.

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

## Right to left

`Dir("rtl")` on an element lays it and its subtree out right to left, like
HTML's `dir` attribute: flex rows start at the right, grid column 1 is the
rightmost, text aligns right, absolutely positioned boxes take their static
position from the right and scroll containers scroll from the right. The
logical classes follow the direction; the physical ones do not:

| Logical (start is the right in `rtl`) | Physical (never mirrored) |
|---|---|
| `ms-*` `me-*` `ps-*` `pe-*` | `ml-*` `mr-*` `pl-*` `pr-*` |
| `start-*` `end-*` `inset-s-*` `inset-e-*` | `left-*` `right-*` |
| `border-s-*` `border-e-*` (widths and colors) | `border-l-*` `border-r-*` |
| `rounded-s-*` `rounded-e-*` `rounded-ss-*` `rounded-se-*` `rounded-es-*` `rounded-ee-*` | `rounded-l-*` `rounded-tl-*` ... |
| `text-start` `text-end` (and the default alignment) | `text-left` `text-right` |
| `space-x-*` (margins on the inline end), `justify-start`, `justify-end`, `gap-*` | `translate-x-*`, `bg-linear-to-r`, `origin-left` |

The block-direction classes `mbs-*` `mbe-*` `pbs-*` `pbe-*` `inset-bs-*`
`inset-be-*` `border-bs-*` `border-be-*` are top and bottom. Use `rtl:` and
`ltr:` for what has no logical class, such as turning an arrow:

```go
Button(Class("flex items-center gap-2"),
	Div(Class("rtl:rotate-180"), Text("←")), // points back in both directions
	Span(Text(t("back"))),
)
```

With [`i18n`](Internationalization), the direction comes from the locale:
`Dir(i18n.Dir(i18n.UseLocale()).String())`.

## See also

- [Elements and Props](Elements-and-Props) for the defaults each element has.
- [CLI](CLI) to look at the result as a PNG, and `tela inspect --layout` for
  the boxes as JSON.
- The `examples/` directory: `counter` is checked pixel by pixel against
  Chromium rendering the same HTML and Tailwind CSS.
