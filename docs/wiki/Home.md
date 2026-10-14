# Tela

Tela is a desktop UI framework written in **pure Go**: no cgo, no webview, no
native toolkits. You write function components with React-style hooks and
style them with Tailwind v4 classes; Tela draws every pixel itself, on Linux
(Wayland and X11) and Windows. The same tree also renders without a window, to
a PNG and to a text tree, so both people and LLM agents can build and check
interfaces without trial and error.

![Hello, Tela: the headline example rendered by Tela](https://raw.githubusercontent.com/kuronosu/tela/main/docs/images/hello.png)

```go
func Counter() tela.Node {
	count, setCount := tela.UseState(0)

	return Div(Class("flex flex-col items-center gap-6 p-10 rounded-3xl "+
		"bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl"),
		H1(Class("text-5xl font-bold text-white tracking-tight"),
			Textf("Count: %d", count)),
		Button(Class("px-6 py-3 rounded-xl bg-white/15 text-white hover:bg-white/25 active:scale-95"),
			OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
			Text("Increment"),
		),
	)
}
```

> **Status: pre-1.0.** The public API (`tela`, `h`, `telatest`) may still
> break between minor versions. macOS has no window backend yet (it only
> cross-compiles). See the [project status](https://github.com/kuronosu/tela#project-status).

## Guides

| Page | What you learn |
|---|---|
| [Getting Started](Getting-Started) | Install, write a counter, run it, render it without a window |
| [Styling](Styling) | Tailwind classes, typed style props, variants, dark mode |
| [Hooks](Hooks) | State, effects, memoization, context, refs, observable values |
| [Events](Events) | Pointer, keyboard, focus, drag, long press, wheel |
| [Elements and Props](Elements-and-Props) | Everything package `h` exports and how elements compose |
| [Forms and Text Input](Forms-and-Text-Input) | `Input`, `Textarea`, `Form`, `Label`, IME |
| [Scrolling and VirtualList](Scrolling-and-VirtualList) | Scroll containers, scroll APIs, 100,000-row lists |
| [Accessibility](Accessibility) | Roles, ARIA props, aria snapshots |
| [Testing](Testing) | `telatest`: mount, click, expect, goldens, without a window |
| [CLI](CLI) | `tela snapshot`, `tela inspect`, debug overlays, environment variables |

## API reference

Every symbol is documented on pkg.go.dev. The wiki also carries the same doc
comments as three generated pages, which in addition list the fields and
methods of the event, state and element-handle types (pkg.go.dev shows only
the aliases for those).

| Package | Import path | pkg.go.dev | Wiki |
|---|---|---|---|
| `tela` | `github.com/kuronosu/tela` | [docs](https://pkg.go.dev/github.com/kuronosu/tela) | [API-Reference-tela](API-Reference-tela) |
| `h` | `github.com/kuronosu/tela/h` | [docs](https://pkg.go.dev/github.com/kuronosu/tela/h) | [API-Reference-h](API-Reference-h) |
| `telatest` | `github.com/kuronosu/tela/telatest` | [docs](https://pkg.go.dev/github.com/kuronosu/tela/telatest) | [API-Reference-telatest](API-Reference-telatest) |

The design is in
[`docs/ARCHITECTURE.md`](https://github.com/kuronosu/tela/blob/main/docs/ARCHITECTURE.md);
full screens to imitate are in
[`examples/`](https://github.com/kuronosu/tela/tree/main/examples).
