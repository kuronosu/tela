# Accessibility

Every element has a place in a **semantics tree**: a role, an accessible name,
a description, states and a value, computed the way browsers do (HTML-AAM and
AccName 1.2, checked against Playwright). The tree is what tests query with
`ByRole`, what `tela inspect` prints, and what an agent reads to understand a
screen. An operating-system accessibility backend (AT-SPI, UI Automation,
NSAccessibility) is not there yet; the tree is built on demand, so it costs
nothing while you do not ask for it.

## Implicit roles

| Element | Role |
|---|---|
| [`Button`](https://pkg.go.dev/github.com/kuronosu/tela/h#Button) | `button` |
| [`H1`](https://pkg.go.dev/github.com/kuronosu/tela/h#H1) to [`H6`](https://pkg.go.dev/github.com/kuronosu/tela/h#H6) | `heading`, with `[level=N]` |
| [`P`](https://pkg.go.dev/github.com/kuronosu/tela/h#P) | `paragraph` |
| [`Img`](https://pkg.go.dev/github.com/kuronosu/tela/h#Img) | `img` (named by [`Alt`](https://pkg.go.dev/github.com/kuronosu/tela/h#Alt)) |
| [`A`](https://pkg.go.dev/github.com/kuronosu/tela/h#A) with [`Href`](https://pkg.go.dev/github.com/kuronosu/tela/h#Href) | `link` |
| [`Input`](https://pkg.go.dev/github.com/kuronosu/tela/h#Input), [`Textarea`](https://pkg.go.dev/github.com/kuronosu/tela/h#Textarea) | `textbox` (named by its [`Label`](https://pkg.go.dev/github.com/kuronosu/tela/h#Label)) |
| [`Div`](https://pkg.go.dev/github.com/kuronosu/tela/h#Div), [`Span`](https://pkg.go.dev/github.com/kuronosu/tela/h#Span) | none |

Elements without a role are left out of the snapshot, and their text is merged
into their parent's.

## Roles and ARIA props

For components built from divs, give the role yourself with [`Role`](https://pkg.go.dev/github.com/kuronosu/tela/h#Role)
(WAI-ARIA 1.2). An unknown role panics in dev mode with the closest valid one.
The props follow React's `aria-*` attributes:

| Prop | Sets |
|---|---|
| [`Role`](https://pkg.go.dev/github.com/kuronosu/tela/h#Role) | The role: `"switch"`, `"dialog"`, `"tab"`, `"progressbar"`... |
| [`AriaLabel`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaLabel) | The accessible name, for elements whose content does not name them (an icon button) |
| [`AriaLabelledBy`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaLabelledBy) | Names an element with the text of another one, attached with [`Ref`](https://pkg.go.dev/github.com/kuronosu/tela/h#Ref) |
| [`AriaDescription`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaDescription) | A longer hint read after the name |
| [`AriaHidden`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaHidden) | Hides the subtree from the semantics tree, not from the screen |
| [`AriaChecked`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaChecked) | Checked state of a checkbox, radio or switch |
| [`AriaPressed`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaPressed) | Makes a button a toggle button and sets whether it is pressed |
| [`AriaSelected`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaSelected) | Selected state of a tab, option, row or cell |
| [`AriaExpanded`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaExpanded) | Whether what a button controls (a menu, a disclosure) is expanded |
| [`AriaDisabled`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaDisabled) | Disabled for assistive technologies without making it inert (`Disabled` does both) |
| [`AriaLevel`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaLevel) | Level of a custom heading, list item, row or tree item |
| [`AriaValueNow`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaValueNow), [`AriaValueMin`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaValueMin), [`AriaValueMax`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaValueMax), [`AriaValueText`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaValueText) | The value of a range widget (`slider`, `progressbar`, `spinbutton`, `meter`) |

```go
func WifiSwitch() tela.Node {
	on, setOn := tela.UseState(true)
	return Button(Role("switch"), AriaChecked(on), AriaLabel("Wi-Fi"),
		Class("h-6 w-11 rounded-full"), IfElse(on, Class("bg-indigo-600"), Class("bg-slate-300")),
		OnClick(func(tela.MouseEvent) { setOn.Set(!on) }),
		Div(Class("size-5 rounded-full bg-white")),
	)
}
```

There are no checkbox, radio, select or slider elements yet (they arrive with
the `kit` components in a later phase); a `Button` or `Div` with a `Role`, the
state props, `TabIndex` and key handlers is how you build them meanwhile.

### A dialog named by its title

```go
func ConfirmDialog(confirm func()) tela.Node {
	title := tela.UseElementRef()
	return Div(Role("dialog"), AriaLabelledBy(title), Class("rounded-xl bg-white p-6 shadow-xl"),
		H2(Ref(title), Class("text-lg font-semibold"), Text("Delete file?")),
		P(Class("text-slate-500"), Text("This cannot be undone.")),
		Button(AutoFocus(true), OnClick(func(tela.MouseEvent) { confirm() }), Text("Delete")),
	)
}
```

### Progress, with a value

```go
func Progress(percent float64) tela.Node {
	return Div(Role("progressbar"), AriaLabel("Upload"),
		AriaValueNow(percent), AriaValueMin(0), AriaValueMax(100),
		Class("h-2 w-64 rounded-full bg-slate-200"),
		Div(Class("h-2 rounded-full bg-indigo-500"), Style(tela.Width(float32(percent)*2.56))),
	)
}
```

## Labels

[`Label`](https://pkg.go.dev/github.com/kuronosu/tela/h#Label) names a text field and focuses it when clicked (see
[Forms and Text Input](Forms-and-Text-Input)). For anything else, use
[`AriaLabel`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaLabel) or [`AriaLabelledBy`](https://pkg.go.dev/github.com/kuronosu/tela/h#AriaLabelledBy). Mark decorative images with `Alt("")`: the
semantics tree leaves them out; [`Alt`](https://pkg.go.dev/github.com/kuronosu/tela/h#Alt) panics on elements that are not an
`Img`.

## Looking at the tree

`tela inspect` prints the tree as a **Playwright aria snapshot**, one line per
role, with the accessible name and the states:

```yaml
- heading "Settings" [level=1]
- switch "Email notifications" [checked]
- button "Save changes" [disabled]
```

From a test, [`App.AriaSnapshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.AriaSnapshot) returns the same text,
[`App.ExpectAria`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ExpectAria) compares it and [`App.AriaGolden`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.AriaGolden) stores long ones
in a file. The headless [`telatest.AriaSnapshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#AriaSnapshot) works without a test. See
[Testing](Testing) and [CLI](CLI).

Queries use the same tree: `telatest.ByRole("switch", "Wi-Fi")` finds the
switch above by what an assistive technology would announce, which is why they
survive restyling.

## Keyboard access

A `Button` is focusable and activates with Enter and Space. Any other element
becomes focusable with [`TabIndex`](https://pkg.go.dev/github.com/kuronosu/tela/h#TabIndex), and you handle the keys with
[`OnKeyDown`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnKeyDown); see [Events](Events#keyboard). The `focus-visible:` variant draws
the focus ring only for keyboard focus.
