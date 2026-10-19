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
- Children are **nodes only**. The parameters are `tela.Child`, an interface
  that a bare `string` does not satisfy, so passing one is a compile error:
  wrap text in [`Text`](https://pkg.go.dev/github.com/kuronosu/tela/h#Text) or [`Textf`](https://pkg.go.dev/github.com/kuronosu/tela/h#Textf).
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
| [`Img`](https://pkg.go.dev/github.com/kuronosu/tela/h#Img) | An image. Give it a source with [`Src`](https://pkg.go.dev/github.com/kuronosu/tela/h#Src); without sizes it is drawn at its own size, shrunk to fit its container |
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

An image comes from any `image.Image`; pass the same value on every build:

```go
var avatar image.Image = image.NewRGBA(image.Rect(0, 0, 40, 40))

func Avatar() tela.Node {
	return Img(Src(avatar), Alt("Ada Lovelace"), Class("size-10 rounded-full"))
}
```

## Props

| Prop | Does |
|---|---|
| [`Class`](https://pkg.go.dev/github.com/kuronosu/tela/h#Class) | Adds Tailwind classes |
| [`Style`](https://pkg.go.dev/github.com/kuronosu/tela/h#Style) | Adds typed style props from package `tela` |
| [`Key`](https://pkg.go.dev/github.com/kuronosu/tela/h#Key) | Gives an element an identity among its siblings |
| [`Disabled`](https://pkg.go.dev/github.com/kuronosu/tela/h#Disabled) | Disables an element and its subtree for the pointer and the focus |
| [`Src`](https://pkg.go.dev/github.com/kuronosu/tela/h#Src), [`Alt`](https://pkg.go.dev/github.com/kuronosu/tela/h#Alt), [`Href`](https://pkg.go.dev/github.com/kuronosu/tela/h#Href) | Image source, its alternative text, a link's destination |
| [`Id`](https://pkg.go.dev/github.com/kuronosu/tela/h#Id), [`For`](https://pkg.go.dev/github.com/kuronosu/tela/h#For), [`Name`](https://pkg.go.dev/github.com/kuronosu/tela/h#Name) | Identifier of an element, the field a label is for, name of a field in its form |
| [`TabIndex`](https://pkg.go.dev/github.com/kuronosu/tela/h#TabIndex), [`AutoFocus`](https://pkg.go.dev/github.com/kuronosu/tela/h#AutoFocus), [`Ref`](https://pkg.go.dev/github.com/kuronosu/tela/h#Ref) | Focus order, focus on appearance, a handle on the element |
| [`Role`](https://pkg.go.dev/github.com/kuronosu/tela/h#Role) and the `Aria...` props | Semantics: see [Accessibility](Accessibility) |
| [`Value`](https://pkg.go.dev/github.com/kuronosu/tela/h#Value), [`DefaultValue`](https://pkg.go.dev/github.com/kuronosu/tela/h#DefaultValue), [`Bind`](https://pkg.go.dev/github.com/kuronosu/tela/h#Bind), [`BindValue`](https://pkg.go.dev/github.com/kuronosu/tela/h#BindValue), [`Type`](https://pkg.go.dev/github.com/kuronosu/tela/h#Type), [`Rows`](https://pkg.go.dev/github.com/kuronosu/tela/h#Rows), [`Placeholder`](https://pkg.go.dev/github.com/kuronosu/tela/h#Placeholder), [`ReadOnly`](https://pkg.go.dev/github.com/kuronosu/tela/h#ReadOnly), [`MaxLength`](https://pkg.go.dev/github.com/kuronosu/tela/h#MaxLength), [`AutoComplete`](https://pkg.go.dev/github.com/kuronosu/tela/h#AutoComplete), [`SpellCheck`](https://pkg.go.dev/github.com/kuronosu/tela/h#SpellCheck) | Text fields: see [Forms and Text Input](Forms-and-Text-Input) |
| `On...` handlers | See [Events](Events) |
| [`Overscan`](https://pkg.go.dev/github.com/kuronosu/tela/h#Overscan) | How many extra items a `VirtualList` builds |

### Keys

When a list changes, the element with the same [`Key`](https://pkg.go.dev/github.com/kuronosu/tela/h#Key) keeps its state wherever
it moves. The key must be a non-nil comparable value, such as an item ID (not
its index when the list reorders). Dev mode reports a list of two or more nodes
without keys. A component or fragment cannot take the `Key` prop; use
[`tela.WithKey`](https://pkg.go.dev/github.com/kuronosu/tela#WithKey).

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

### Fragment and TextFn

[`Fragment`](https://pkg.go.dev/github.com/kuronosu/tela/h#Fragment) groups children without a box of their own: they become children of
the fragment's parent (it takes only `Key` as a prop). [`TextFn`](https://pkg.go.dev/github.com/kuronosu/tela/h#TextFn) is a text node
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

## See also

- [API-Reference-h](API-Reference-h) lists every symbol with its doc comment.
- [Styling](Styling) for what goes in `Class`.
