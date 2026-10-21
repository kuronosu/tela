# Elements and Props

Package `h` holds the elements, props and helpers of a Tela UI. It is meant to
be **dot-imported**, so markup reads like JSX:

```go
import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)
```

To keep collisions with your own package unlikely, `h` exports only element
constructors, props and a few helpers. Everything else (the app, hooks,
events, colors, typed style props) lives in package `tela`.

## Building a tree

An element takes its **props and children in one list, in any order**:

```go
func Greeting(name string) tela.Node {
	return Div(Class("flex items-center gap-3 rounded-xl bg-white p-4 shadow-md"),
		H2(Class("text-lg font-semibold text-slate-900"), Textf("Hello, %s", name)),
		If(name == "", P(Class("text-slate-500"), Text("Who are you?"))),
	)
}
```

- Props apply in order: a later [`Class`](https://pkg.go.dev/github.com/kuronosu/tela/h#Class) wins over an earlier one, and
  [`Style`](https://pkg.go.dev/github.com/kuronosu/tela/h#Style) wins over every class.
- Children are **nodes and components**: an element, a text, or a
  [`tela.Component`](https://pkg.go.dev/github.com/kuronosu/tela#Component) value such as `Card{Title: "Inbox"}` (a struct with a `Build`
  method, a component with its own state; see [Hooks](Hooks#components)). The
  parameters are `tela.Child`, an interface (a `Build() tela.Node` method)
  that a bare `string` does not satisfy, so passing one is a compile error:
  wrap text in [`Text`](https://pkg.go.dev/github.com/kuronosu/tela/h#Text) or [`Textf`](https://pkg.go.dev/github.com/kuronosu/tela/h#Textf). The compiler names the internal type behind
  the alias, so `Div("hello")` reads: `cannot use "hello" (constant of type
  string) as tela.Child value in argument to Div: string does not implement
  core.Child (missing method Build)`.
- Elements have the defaults of Tailwind's Preflight: no margins, padding or
  borders, headings in the surrounding font size, block images. Style
  everything with [classes](Styling).

## Elements

| Element | What it is |
|---|---|
| [`Div`](https://pkg.go.dev/github.com/kuronosu/tela/h#Div) | A block container, the general-purpose box. Its children stack vertically unless classes say otherwise (`flex`) |
| [`Span`](https://pkg.go.dev/github.com/kuronosu/tela/h#Span) | An inline container. Inside a paragraph it styles a run of text; as a flex child it is a flex item; with `inline-block` it sits on the line as a box |
| [`Text`](https://pkg.go.dev/github.com/kuronosu/tela/h#Text), [`Textf`](https://pkg.go.dev/github.com/kuronosu/tela/h#Textf) | A text node (`Textf` formats like `fmt.Sprintf`). It is laid out in the paragraph of its nearest block ancestor and styled by its parent |
| [`H1`](https://pkg.go.dev/github.com/kuronosu/tela/h#H1) to [`H6`](https://pkg.go.dev/github.com/kuronosu/tela/h#H6) | Headings, with their level in the semantics tree. Unstyled: size and weight come from classes |
| [`P`](https://pkg.go.dev/github.com/kuronosu/tela/h#P) | A paragraph of text and inline elements |
| [`Button`](https://pkg.go.dev/github.com/kuronosu/tela/h#Button) | A button: no background, border or padding of its own, fits its content, shows the pointer cursor, focusable, clicks with Enter and Space |
| [`Img`](https://pkg.go.dev/github.com/kuronosu/tela/h#Img) | An image: PNG, JPEG, GIF, WebP or SVG (vector) from [`SrcPath`](https://pkg.go.dev/github.com/kuronosu/tela/h#SrcPath), [`SrcBytes`](https://pkg.go.dev/github.com/kuronosu/tela/h#SrcBytes), [`SrcFS`](https://pkg.go.dev/github.com/kuronosu/tela/h#SrcFS) or a decoded [`Src`](https://pkg.go.dev/github.com/kuronosu/tela/h#Src); without sizes it is drawn at its own size, shrunk to fit its container |
| [`Icon`](https://pkg.go.dev/github.com/kuronosu/tela/h#Icon) | An SVG icon ([`tela.Icon`](https://pkg.go.dev/github.com/kuronosu/tela#Icon)): 1em by 1em unless sized, colored by the text color |
| [`Strong`](https://pkg.go.dev/github.com/kuronosu/tela/h#Strong), [`Em`](https://pkg.go.dev/github.com/kuronosu/tela/h#Em), [`Code`](https://pkg.go.dev/github.com/kuronosu/tela/h#Code) | Inline bold, italic and monospace text |
| [`A`](https://pkg.go.dev/github.com/kuronosu/tela/h#A) | A link: inline text with a destination from [`Href`](https://pkg.go.dev/github.com/kuronosu/tela/h#Href). Unstyled; Tela does not open links by itself yet, handle `OnClick` |
| [`Input`](https://pkg.go.dev/github.com/kuronosu/tela/h#Input), [`Textarea`](https://pkg.go.dev/github.com/kuronosu/tela/h#Textarea), [`Form`](https://pkg.go.dev/github.com/kuronosu/tela/h#Form), [`Label`](https://pkg.go.dev/github.com/kuronosu/tela/h#Label) | Text fields and forms: see [Forms and Text Input](Forms-and-Text-Input) |
| [`VirtualList`](https://pkg.go.dev/github.com/kuronosu/tela/h#VirtualList) | A scrolling list that builds only the rows in view: see [Scrolling and VirtualList](Scrolling-and-VirtualList) |

Rich text mixes inline elements in a paragraph; a `Span` with a background and
rounded corners is a CSS inline box that wraps across lines:

```go
func Note() tela.Node {
	return P(Class("max-w-md leading-7 text-slate-700"),
		Text("Press "),
		Code(Class("rounded bg-slate-100 px-1"), Text("Esc")),
		Text(" to "),
		Strong(Text("close")),
		Text(" the "),
		A(Href("https://pkg.go.dev/github.com/kuronosu/tela"), Class("text-indigo-600 underline"), Text("dialog")),
		Text("."),
	)
}
```

### Images

An image comes from a file of the file systems registered with
[`tela.Assets`](https://pkg.go.dev/github.com/kuronosu/tela#Assets) (the same lookup as `bg-[url(...)]`), from bytes, from
any `fs.FS`, or from an `image.Image` you decoded (pass the same value on
every build). The format is recognized by its content; SVG stays vector, so
it is sharp at every size and scale:

```go
//go:embed img
var images embed.FS

func init() { tela.Assets(images) }

func Header(avatar image.Image) tela.Node {
	return Div(Class("flex items-center gap-3"),
		Img(SrcPath("img/logo.svg"), Alt("Acme"), Class("h-8")), // the width follows the ratio
		Img(Src(avatar), Alt("Ada Lovelace"), Class("size-10 rounded-full object-cover")),
	)
}
```

Files are decoded once and cached (by path, by content for `SrcBytes`, by file
system and path for `SrcFS`); a missing or undecodable source shows nothing
and is reported once in the dev log. `object-contain`, `object-cover`,
`object-none`, `object-scale-down` and `object-top-left`, `object-[25%_75%]`...
fit the image in its box as in CSS. [`Tint`](https://pkg.go.dev/github.com/kuronosu/tela/h#Tint) paints a monochrome image in one
color through its alpha: `Img(SrcPath("icons/claude.png"), Tint(tela.Hex("#D97757")), Class("size-5"))`.

### Icons

[`Icon`](https://pkg.go.dev/github.com/kuronosu/tela/h#Icon) shows a [`tela.Icon`](https://pkg.go.dev/github.com/kuronosu/tela#Icon), the text of an SVG document that can be
package-level data (`var Check = tela.Icon{SVG: "<svg ...>"}`, or
`tela.MustSVG(data)` for an embedded file). It is parsed once and cached by its
text. An icon is 1em by 1em, so it follows the font size like an icon font,
unless `size-*`, `w-*`/`h-*` or a style sizes it; it is a block, like an svg
under Preflight (`Class("inline")` puts it on a line of text). Its
`currentColor` is the text color, so `text-*` classes, tokens and transitions
recolor it, and `fill-*`, `stroke-*` and `stroke-[1.5]` (on the icon or on an
ancestor such as the button) override the document's own paints:

```go
Button(Class("inline-flex items-center gap-2 rounded-md bg-primary px-3 py-2 text-sm text-primary-foreground"),
	Icon(Check, Class("size-4 stroke-[1.5]")), Text("Save"))
```

Icons are decorative (hidden from accessibility) unless they have an
`AriaLabel` or `AriaLabelledBy`, which make them images with that name.

The [Icons](Icons) page covers the Lucide set (`github.com/kuronosu/tela/icons/lucide`),
generating a set from SVG files with `tela icons gen`, and the cost in a binary.

## Props

| Prop | Does |
|---|---|
| [`Class`](https://pkg.go.dev/github.com/kuronosu/tela/h#Class) | Adds Tailwind classes |
| [`Style`](https://pkg.go.dev/github.com/kuronosu/tela/h#Style) | Adds typed style props from package `tela` |
| [`Key`](https://pkg.go.dev/github.com/kuronosu/tela/h#Key) | Gives an element an identity among its siblings |
| [`Disabled`](https://pkg.go.dev/github.com/kuronosu/tela/h#Disabled) | Disables an element and its subtree for the pointer and the focus |
| [`Scheme`](https://pkg.go.dev/github.com/kuronosu/tela/h#Scheme) | Gives an element and its subtree their own color scheme (`tela.Light`, `tela.Dark`): see [Theming](Theming) |
| [`Dir`](https://pkg.go.dev/github.com/kuronosu/tela/h#Dir) | Direction of an element and its subtree (`"rtl"`, `"ltr"`, `"auto"`), like HTML's `dir`: see [Internationalization](Internationalization#right-to-left) |
| [`Src`](https://pkg.go.dev/github.com/kuronosu/tela/h#Src), [`SrcPath`](https://pkg.go.dev/github.com/kuronosu/tela/h#SrcPath), [`SrcBytes`](https://pkg.go.dev/github.com/kuronosu/tela/h#SrcBytes), [`SrcFS`](https://pkg.go.dev/github.com/kuronosu/tela/h#SrcFS), [`Tint`](https://pkg.go.dev/github.com/kuronosu/tela/h#Tint), [`Alt`](https://pkg.go.dev/github.com/kuronosu/tela/h#Alt), [`Href`](https://pkg.go.dev/github.com/kuronosu/tela/h#Href) | Image sources, a tint, the image's alternative text, a link's destination |
| [`ID`](https://pkg.go.dev/github.com/kuronosu/tela/h#ID), [`For`](https://pkg.go.dev/github.com/kuronosu/tela/h#For), [`Name`](https://pkg.go.dev/github.com/kuronosu/tela/h#Name) | Identifier of an element, the field a label is for, name of a field in its form |
| [`TabIndex`](https://pkg.go.dev/github.com/kuronosu/tela/h#TabIndex), [`AutoFocus`](https://pkg.go.dev/github.com/kuronosu/tela/h#AutoFocus), [`Ref`](https://pkg.go.dev/github.com/kuronosu/tela/h#Ref) | Focus order, focus on appearance, a handle on the element |
| [`Role`](https://pkg.go.dev/github.com/kuronosu/tela/h#Role) and the `Aria...` props | Semantics: see [Accessibility](Accessibility) |
| [`Value`](https://pkg.go.dev/github.com/kuronosu/tela/h#Value), [`DefaultValue`](https://pkg.go.dev/github.com/kuronosu/tela/h#DefaultValue), [`Bind`](https://pkg.go.dev/github.com/kuronosu/tela/h#Bind), [`BindValue`](https://pkg.go.dev/github.com/kuronosu/tela/h#BindValue), [`Type`](https://pkg.go.dev/github.com/kuronosu/tela/h#Type), [`Rows`](https://pkg.go.dev/github.com/kuronosu/tela/h#Rows), [`Placeholder`](https://pkg.go.dev/github.com/kuronosu/tela/h#Placeholder), [`ReadOnly`](https://pkg.go.dev/github.com/kuronosu/tela/h#ReadOnly), [`MaxLength`](https://pkg.go.dev/github.com/kuronosu/tela/h#MaxLength), [`AutoComplete`](https://pkg.go.dev/github.com/kuronosu/tela/h#AutoComplete), [`SpellCheck`](https://pkg.go.dev/github.com/kuronosu/tela/h#SpellCheck) | Text fields: see [Forms and Text Input](Forms-and-Text-Input) |
| `On...` handlers | See [Events](Events) |
| [`Overscan`](https://pkg.go.dev/github.com/kuronosu/tela/h#Overscan), [`ItemKey`](https://pkg.go.dev/github.com/kuronosu/tela/h#ItemKey), [`ItemHeight`](https://pkg.go.dev/github.com/kuronosu/tela/h#ItemHeight) | How many extra items a `VirtualList` builds, what keys its items, and a fixed item height |
| [`AnimateLayout`](https://pkg.go.dev/github.com/kuronosu/tela/h#AnimateLayout), [`LayoutID`](https://pkg.go.dev/github.com/kuronosu/tela/h#LayoutID), [`Exit`](https://pkg.go.dev/github.com/kuronosu/tela/h#Exit) | Layout animations, shared-element transitions and exit classes under a `Presence`: see [Animation](Animation) |

### Keys

When a list changes, the element with the same [`Key`](https://pkg.go.dev/github.com/kuronosu/tela/h#Key) keeps its state wherever
it moves. The key is a comparable value, such as an item ID (not its index
when the list reorders): `Key` is generic, so a slice or a map does not
compile. Dev mode warns about a list of two or more nodes without keys and
about duplicate keys among siblings, once per call site of the `Map` that made
the list. A component or fragment cannot take the `Key` prop; use
[`tela.WithKey`](https://pkg.go.dev/github.com/kuronosu/tela#WithKey), which also keys a struct component and makes a node of it:

```go
Map(cards, func(c CardData) tela.Node { return tela.WithKey(Card{Title: c.Title}, c.ID) })
```

A component built with `tela.C`, `tela.CP` or `tela.Memo` takes the key as its
last argument: `tela.CP(Row, RowProps{...}, Key(row.ID))`. And for a list of
components [`MapKeyed`](https://pkg.go.dev/github.com/kuronosu/tela/h#MapKeyed) does the keying:

```go
MapKeyed(todos, func(t Todo) int { return t.ID }, func(t Todo) tela.Component {
	return TodoRow{Todo: t} // a struct component, or tela.CP(TodoRow, t)
})
```

## Helpers

### If and IfElse

[`If`](https://pkg.go.dev/github.com/kuronosu/tela/h#If) returns a node or a prop when a condition holds, and the zero value
otherwise. The zero node renders nothing but keeps the positions of the
siblings after it, so toggling the condition does not remount them. It works
for props too. [`IfElse`](https://pkg.go.dev/github.com/kuronosu/tela/h#IfElse) picks between two:

```go
func Status(loading bool, err error, selected bool) tela.Node {
	return Div(
		If(loading, Span(Text("Loading..."))),
		If(selected, Class("ring-2 ring-indigo-500")),
		IfElse(loading, Class("opacity-50"), Class("opacity-100")),
		If(err != nil, P(Class("text-red-600"), Text(errorText(err)))),
	)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
```

Every argument of a call is evaluated: `If(user != nil, Text(user.Name))`
dereferences a nil `user`. Use an ordinary `if` for that case.

`If` and `IfElse` return a `tela.Child`, so they take a node, a struct
component (`If(loading, Spinner{})`) or a prop (`If(active, Class("ring-2"))`),
and when the branch is absent they render and apply nothing. A function
component that returns one wraps it: `return Fragment(If(...))`.

### Map and MapIndex

[`Map`](https://pkg.go.dev/github.com/kuronosu/tela/h#Map) turns a slice into a list of nodes; [`MapIndex`](https://pkg.go.dev/github.com/kuronosu/tela/h#MapIndex) also gives the index. Give
each node a `Key`.

```go
type Step struct {
	ID    int
	Title string
}

func Steps(steps []Step) tela.Node {
	return Div(
		MapIndex(steps, func(i int, s Step) tela.Node {
			return Div(Key(s.ID), Textf("%d. %s", i+1, s.Title))
		}),
	)
}
```

### Range

[`Range`](https://pkg.go.dev/github.com/kuronosu/tela/h#Range)`(n, render)` returns `render(0)` to `render(n-1)` as one list keyed by
index, for items with no data behind them (skeleton rows, a star rating); a
static range has no missing-key warning. `render` returns a `tela.Node` or a
`tela.Component`, so a struct component needs no `Fragment`:

```go
Range(5, func(i int) tela.Component { return Star{Filled: i < rating} })
```

For data that changes, key by the item's identity with `Map` or `MapKeyed`.

### MapKeyed and Nodes

[`MapKeyed`](https://pkg.go.dev/github.com/kuronosu/tela/h#MapKeyed)`(items, key, render)` is `Map` for components: it keys each
one by `key(item)`, so reordering, inserting and removing keeps the state of
the others. `render` returns a node or a struct component; the key type is
any comparable type. [`Nodes`](https://pkg.go.dev/github.com/kuronosu/tela/h#Nodes)`(ns)` passes a `[]tela.Node` built in a loop as
one child (Go does not spread it into `Div(...)`); the nodes have no box of
their own, like those of `Fragment`.

```go
var cells []tela.Node
for _, c := range columns {
	cells = append(cells, Div(Class("px-3"), Text(c)))
}
Div(Class("flex"), Nodes(cells))
```

### Fragment and TextFn

[`Fragment`](https://pkg.go.dev/github.com/kuronosu/tela/h#Fragment) groups children without a box of their own: they become children of
the fragment's parent (it takes only `Key` as a prop). It is also how a
wrapper puts the children it receives next to its own, since Go does not allow
`Div(Class("..."), H2(...), children...)`:

```go
// Card has no hooks, so it is a plain function that wraps its children.
func Card(title string, children ...tela.Child) tela.Node {
	return Div(Class("rounded-xl border p-4"),
		H2(Class("font-semibold"), Text(title)),
		Fragment(children...))
}
```

It puts several children in one field of type `tela.Component`
(`Panel{Children: Fragment(Text("a"), Span(Text("b")))}`), and makes a node of
a struct component where a `tela.Node` is expected
(`return Fragment(Panel{Title: "Inbox"})`).

[`TextFn`](https://pkg.go.dev/github.com/kuronosu/tela/h#TextFn) is a text node
whose content is read in a later phase, so a change of a `tela.Value` it reads
lays out only the paragraph, without building any component; see
[Hooks](Hooks#observable-values-usevalue).

```go
func Elapsed(seconds *tela.Value[float64]) tela.Node {
	return Span(Class("tabular-nums"), TextFn(func() string {
		return fmt.Sprintf("%.1f s", seconds.Get())
	}))
}
```

### Portals

[`Portal`](https://pkg.go.dev/github.com/kuronosu/tela/h#Portal) shows its children in a layer above the whole window, like React's
`createPortal` into `document.body`: the overlay is not clipped by
`overflow-hidden` ancestors and not covered by their z-index. The layer is laid
out against the window, so `fixed inset-0` covers it and `absolute` content
positions against it; later portals are above earlier ones; clicks on the
layer's empty parts reach what is under it.

Everything else follows the component tree: the content keeps its state,
reads the context of the Portal's ancestors, inherits their text color and
font, and its events bubble to the Portal's ancestors. Tab order and the aria
snapshot put it after the window's content.

```go
func Menu() tela.Node {
	open, setOpen := tela.UseState(false)
	return Div(Class("relative overflow-hidden"),
		Button(OnClick(func(tela.MouseEvent) { setOpen.Set(!open) }), Text("Menu")),
		If(open, Portal(
			Div(Class("fixed inset-0"), OnClick(func(tela.MouseEvent) { setOpen.Set(false) })),
			Div(Role("menu"), Class("fixed top-12 left-4 rounded-lg bg-white p-2 shadow-xl"),
				Button(Role("menuitem"), Text("Rename"))),
		)),
	)
}
```

### Presence

[`Presence`](https://pkg.go.dev/github.com/kuronosu/tela/h#Presence) keeps a removed child mounted while it runs its
exit (the classes of its [`Exit`](https://pkg.go.dev/github.com/kuronosu/tela/h#Exit) prop, or a component's own exit
through `tela.UsePresence`), then unmounts it. Enter animations need no
Presence: a `starting:` class gives a newly shown element the style it
transitions from. See [Animation](Animation#exiting-presence-and-exit).

## See also

- [API-Reference-h](API-Reference-h) lists every symbol with its doc comment.
- [Styling](Styling) for what goes in `Class`.
