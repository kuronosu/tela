# Hooks

Hooks keep state and run code across builds, with the names and semantics of
React's. A **component** is a plain function that returns a [`tela.Node`](https://pkg.go.dev/github.com/kuronosu/tela#Node) and may
call hooks; the sections below follow the order you usually need them.

## The rules

Hooks follow React's rules: call them at the top level of a component
function, in the same order on every build, never inside conditions or loops.
A hook called outside a component build panics with an explanation; in dev
mode (`TELA_DEV=1`) a change of the hook order panics with both call sites.

Everything runs on the UI thread except the methods of [`tela.Setter`](https://pkg.go.dev/github.com/kuronosu/tela#Setter),
[`tela.Dispatcher`](https://pkg.go.dev/github.com/kuronosu/tela#Dispatcher) and [`tela.Value`](https://pkg.go.dev/github.com/kuronosu/tela#Value), and [`tela.Post`](https://pkg.go.dev/github.com/kuronosu/tela#Post), which are safe from any
goroutine.

## Components

A function that returns a node becomes a **component boundary**, an element
with its own hook state that rebuilds on its own, when you wrap it:

```go
func Header() tela.Node { return H1(Text("Inbox")) }

type CardProps struct{ Title string }

func Card(p CardProps) tela.Node {
	return Div(Class("rounded-lg border p-4"), Text(p.Title))
}

func Screen() tela.Node {
	return Div(
		tela.C(Header),
		tela.CP(Card, CardProps{Title: "Unread"}),
	)
}
```

- [`tela.C`](https://pkg.go.dev/github.com/kuronosu/tela#C) wraps a function without props. It builds whenever its parent builds.
- [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) wraps a function with **comparable** props. When the parent builds
  again with props `==` to the previous ones, and neither the component's own
  state nor a context it reads changed, the build is skipped (automatic
  skipping, as in Jetpack Compose). Pass a named function and put everything
  it uses in the props; dev mode reports anonymous functions.
- [`tela.Memo`](https://pkg.go.dev/github.com/kuronosu/tela#Memo) is [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) for props that are not comparable: you give the
  comparison function.
- Calling a component function directly, `Header()`, runs it inside the
  caller, as a custom hook: its hooks belong to the caller.
- [`tela.WithKey`](https://pkg.go.dev/github.com/kuronosu/tela#WithKey) keys a component or fragment, which cannot take the [`Key`](https://pkg.go.dev/github.com/kuronosu/tela/h#Key) prop.

## State: UseState

[`tela.UseState`](https://pkg.go.dev/github.com/kuronosu/tela#UseState) returns the current state and a [`tela.Setter`](https://pkg.go.dev/github.com/kuronosu/tela#Setter). The first build
stores the initial value; later builds return the latest one.

```go
func Clicks() tela.Node {
	count, setCount := tela.UseState(0)

	return Button(
		OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
		Textf("Clicked %d times", count),
	)
}
```

`Set(v)` replaces the state. `Update(f)` computes it from the state left by the
previous queued update, which is what you want when several updates may be
queued before the next build (events in one frame, goroutines):

```go
func Counter() tela.Node {
	count, setCount := tela.UseState(0)
	add := func(tela.MouseEvent) {
		setCount.Update(func(n int) int { return n + 1 })
	}
	return Button(OnClick(add), Textf("%d", count))
}
```

For comparable types, a new state `==` to the current one does not rebuild. A
[`tela.Setter`](https://pkg.go.dev/github.com/kuronosu/tela#Setter) is a small comparable value, the same on every build, so it can be
stored, passed to goroutines, and put in the props of a [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) component.

[`tela.UseStateSet`](https://pkg.go.dev/github.com/kuronosu/tela#UseStateSet) and [`tela.UseStateUpdate`](https://pkg.go.dev/github.com/kuronosu/tela#UseStateUpdate) return the setter's `Set` and
`Update` methods as plain functions when you only need one of them:

```go
func NameField() tela.Node {
	name, setName := tela.UseStateSet("")
	return Input(Value(name), OnChange(func(e tela.ChangeEvent) { setName(e.Value) }))
}
```

## State machines: UseReducer

[`tela.UseReducer`](https://pkg.go.dev/github.com/kuronosu/tela#UseReducer) takes a pure reducer and returns the state with a
[`tela.Dispatcher`](https://pkg.go.dev/github.com/kuronosu/tela#Dispatcher). Queued actions apply in order on the UI thread.

```go
type action struct{ kind string }

func reduce(n int, a action) int {
	switch a.kind {
	case "inc":
		return n + 1
	case "reset":
		return 0
	}
	return n
}

func Stepper() tela.Node {
	count, dispatch := tela.UseReducer(reduce, 0)
	return Div(
		Textf("%d", count),
		Button(OnClick(func(tela.MouseEvent) { dispatch.Dispatch(action{"inc"}) }), Text("+")),
		Button(OnClick(func(tela.MouseEvent) { dispatch.Dispatch(action{"reset"}) }), Text("Reset")),
	)
}
```

## Effects: UseEffect and UseLayoutEffect

[`tela.UseEffect`](https://pkg.go.dev/github.com/kuronosu/tela#UseEffect) runs a function after the frame is presented, when its
dependencies changed since the previous build. The function may return a
cleanup, which runs before the effect runs again and when the component
unmounts. The dependencies follow React:

```go
tela.UseEffect(f)          // after every build
tela.UseEffect(f, nil)     // only after the first build (React's [])
tela.UseEffect(f, id, n)   // when id or n changed (compared with ==)
```

A single `nil` is the idiom for subscriptions and timers: the effect runs on
mount and the cleanup on unmount. State set from a goroutine is safe:

```go
func Clock() tela.Node {
	now, setNow := tela.UseState(time.Now())

	tela.UseEffect(func() func() {
		tick := time.NewTicker(time.Second)
		done := make(chan struct{})
		go func() {
			for {
				select {
				case t := <-tick.C:
					setNow.Set(t) // safe from any goroutine
				case <-done:
					return
				}
			}
		}()
		return func() { tick.Stop(); close(done) }
	}, nil)

	return Span(Class("tabular-nums"), Text(now.Format("15:04:05")))
}
```

Dependencies that cannot be compared (functions, slices, maps) count as
changed on every build, and dev mode says so: pass an ID or a comparable value
instead. Within a frame, the effects of children run before their parents', and
effects of a component whose build was skipped do not run.

[`tela.UseLayoutEffect`](https://pkg.go.dev/github.com/kuronosu/tela#UseLayoutEffect) has the same shape but runs after layout and before
paint, for code that needs the geometry.

## Memoization: UseMemo, UseCallback, UseRef

[`tela.UseMemo`](https://pkg.go.dev/github.com/kuronosu/tela#UseMemo) caches a computation until its dependencies change:

```go
type Todo struct {
	ID   int
	Text string
}

func Filtered(todos []Todo, query string) tela.Node {
	visible := tela.UseMemo(func() []Todo {
		var out []Todo
		for _, t := range todos {
			if strings.Contains(strings.ToLower(t.Text), strings.ToLower(query)) {
				out = append(out, t)
			}
		}
		return out
	}, len(todos), query)

	return Map(visible, func(t Todo) tela.Node { return Div(Key(t.ID), Text(t.Text)) })
}
```

[`tela.UseCallback`](https://pkg.go.dev/github.com/kuronosu/tela#UseCallback) returns a [`tela.Callback`](https://pkg.go.dev/github.com/kuronosu/tela#Callback): one stable function, the same on
every build, that calls the latest function you gave it. Because a Callback is
comparable, it can travel in the props of a [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) component without
defeating its skipping. Call `cb.Fn()` to get the function:

```go
type ToolbarProps struct {
	OnSave tela.Callback[func(tela.MouseEvent)]
}

func Toolbar(p ToolbarProps) tela.Node {
	return Button(OnClick(p.OnSave.Fn()), Text("Save"))
}

func Editor() tela.Node {
	draft, setDraft := tela.UseState("")
	onSave := tela.UseCallback(func(tela.MouseEvent) { save(draft) })
	return Div(
		Input(Bind(draft, setDraft)),
		tela.CP(Toolbar, ToolbarProps{OnSave: onSave}), // does not rebuild while typing
	)
}

func save(string) {}
```

[`tela.UseRef`](https://pkg.go.dev/github.com/kuronosu/tela#UseRef) returns a [`tela.Ref`](https://pkg.go.dev/github.com/kuronosu/tela#Ref), a box whose `Current` field persists across
builds. Changing it does not rebuild: use it for values that rendering does not
show, such as a timer or the previous value of a prop.

```go
func RenderCount() tela.Node {
	renders := tela.UseRef(0)
	renders.Current++
	return Textf("built %d times", renders.Current)
}
```

## Context

[`tela.CreateContext`](https://pkg.go.dev/github.com/kuronosu/tela#CreateContext) makes a context with a default value; create contexts once,
at package level. `Provider(value, children...)` gives the value to a subtree
and [`tela.UseContext`](https://pkg.go.dev/github.com/kuronosu/tela#UseContext) reads the nearest one. When the provided value changes,
only the components that read it rebuild, even if the components between skip
their build.

```go
var Theme = tela.CreateContext("light")

func App() tela.Node {
	theme, setTheme := tela.UseState("dark")
	return Theme.Provider(theme,
		tela.C(Page),
		Button(OnClick(func(tela.MouseEvent) { setTheme.Set("light") }), Text("Light")),
	)
}

func Page() tela.Node {
	theme := tela.UseContext(Theme)
	return P(Textf("The theme is %s", theme))
}
```

## Identity: UseId

[`tela.UseId`](https://pkg.go.dev/github.com/kuronosu/tela#UseId) returns a string that is unique in the process and the same on every
build (and the same for the same mount order, so snapshots are deterministic).
Use it to relate elements, such as a label and its field:

```go
func Field(label string) tela.Node {
	id := tela.UseId()
	return Div(
		Label(For(id), Text(label)),
		Input(Id(id)),
	)
}
```

## Handles on elements: UseElementRef

[`tela.UseElementRef`](https://pkg.go.dev/github.com/kuronosu/tela#UseElementRef) returns an [`tela.ElementRef`](https://pkg.go.dev/github.com/kuronosu/tela#ElementRef). Attach it with [`Ref`](https://pkg.go.dev/github.com/kuronosu/tela/h#Ref) and act
on the element from handlers and effects: focus it, scroll it, read its bounds,
read or select the text of a field.

```go
func Search() tela.Node {
	input := tela.UseElementRef()

	tela.UseEffect(func() func() {
		input.Focus()
		return nil
	}, nil)

	return Input(Ref(input), Type("search"), Placeholder("Search"),
		Class("rounded-md border px-2 py-1"))
}
```

Its methods are: [`ElementRef.Focus`](API-Reference-tela#func-elementref-focus), [`ElementRef.Blur`](API-Reference-tela#func-elementref-blur),
[`ElementRef.Focused`](API-Reference-tela#func-elementref-focused), [`ElementRef.Bounds`](API-Reference-tela#func-elementref-bounds), the scroll methods
[`ElementRef.ScrollTo`](API-Reference-tela#func-elementref-scrollto), [`ElementRef.ScrollBy`](API-Reference-tela#func-elementref-scrollby),
[`ElementRef.ScrollOffset`](API-Reference-tela#func-elementref-scrolloffset), [`ElementRef.ScrollIntoView`](API-Reference-tela#func-elementref-scrollintoview) and
[`ElementRef.ScrollToIndex`](API-Reference-tela#func-elementref-scrolltoindex), and, for text fields, [`ElementRef.Value`](API-Reference-tela#func-elementref-value),
[`ElementRef.Select`](API-Reference-tela#func-elementref-select), [`ElementRef.SetSelectionRange`](API-Reference-tela#func-elementref-setselectionrange) and
[`ElementRef.SelectionRange`](API-Reference-tela#func-elementref-selectionrange). The scroll methods are explained in
[Scrolling and VirtualList](Scrolling-and-VirtualList) and the text ones in
[Forms and Text Input](Forms-and-Text-Input).

## Observable values: UseValue

Some state changes every frame: a drag offset, a scroll position, download
progress. Rebuilding a component 60 times a second for that is wasteful.
[`tela.UseValue`](https://pkg.go.dev/github.com/kuronosu/tela#UseValue) returns a [`tela.Value`](https://pkg.go.dev/github.com/kuronosu/tela#Value), an observable cell, and **where you read
it decides what a change redoes**:

| `v.Get()` called in... | A change of `v` redoes... |
|---|---|
| the component's body | the component's build, like `UseState` |
| a layout closure ([`tela.WidthFn`](https://pkg.go.dev/github.com/kuronosu/tela#WidthFn), [`tela.HeightFn`](https://pkg.go.dev/github.com/kuronosu/tela#HeightFn)) | that element's layout only |
| a paint closure ([`tela.TranslateFn`](https://pkg.go.dev/github.com/kuronosu/tela#TranslateFn), [`tela.OpacityFn`](https://pkg.go.dev/github.com/kuronosu/tela#OpacityFn)) | that element's paint only |
| an event handler, effect or goroutine | nothing; `Get` just returns the value |

A progress bar fed by a goroutine, which lays out the bar again without
building `Download`:

```go
func Download() tela.Node {
	progress := tela.UseValue(0.0) // from 0 to 1

	tela.UseEffect(func() func() {
		ctx, cancel := context.WithCancel(context.Background())
		go fetch(ctx, func(done float64) { progress.Set(done) }) // any goroutine
		return cancel
	}, nil)

	return Div(Class("h-2 w-64 overflow-hidden rounded-full bg-slate-200"),
		Div(Class("h-full bg-indigo-500"),
			Style(tela.WidthFn(func() float32 { return float32(256 * progress.Get()) }))))
}

func fetch(ctx context.Context, report func(float64)) {
	for i := 0; i <= 10 && ctx.Err() == nil; i++ {
		report(float64(i) / 10)
		time.Sleep(100 * time.Millisecond)
	}
}
```

`Set` and `Update` are safe from any goroutine and queue the change, in order
with state updates. `Get` returns the value the UI thread applied last; use
[`Value.Latest`](API-Reference-tela#func-value-latest) in a handler that has just called `Set` and wants the new
value. A [`tela.Value`](https://pkg.go.dev/github.com/kuronosu/tela#Value) is comparable (the same pointer on every build), so it can
go in the props of a [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) component or through a context.

Text can be deferred the same way with [`TextFn`](https://pkg.go.dev/github.com/kuronosu/tela/h#TextFn), and a field can be bound to a
`Value[string]` with [`BindValue`](https://pkg.go.dev/github.com/kuronosu/tela/h#BindValue).

### Scroll offsets and bounds

[`tela.UseScroll`](https://pkg.go.dev/github.com/kuronosu/tela#UseScroll) gives two Values that follow the scroll offsets of an element,
and [`tela.UseBounds`](https://pkg.go.dev/github.com/kuronosu/tela#UseBounds) a Value of its [`tela.Rect`](https://pkg.go.dev/github.com/kuronosu/tela#Rect) in the window (a ResizeObserver
that also sees position):

```go
func FadingHeader(rows tela.Node) tela.Node {
	list := tela.UseElementRef()
	_, y := tela.UseScroll(list)

	header := Div(Class("h-16 bg-white shadow"),
		Style(tela.OpacityFn(func() float32 {
			return 1 - min(y.Get()/200, 1) // repaints only
		})))
	return Div(Class("flex h-screen flex-col"), header,
		Div(Ref(list), Class("grow overflow-y-auto"), rows))
}
```

## Environment: UseColorScheme and UseReducedMotion

[`tela.UseColorScheme`](https://pkg.go.dev/github.com/kuronosu/tela#UseColorScheme) returns the effective color scheme, `tela.Light` or
`tela.Dark` (the app's choice or the system's preference), and
[`tela.UseReducedMotion`](https://pkg.go.dev/github.com/kuronosu/tela#UseReducedMotion) whether the system asks for reduced motion. The component
builds again when they change; nothing else does. Classes do not need them
(`dark:` and `motion-reduce:` follow the same settings); use them for what
classes cannot do, such as picking an image.
[`tela.SetColorScheme`](https://pkg.go.dev/github.com/kuronosu/tela#SetColorScheme) changes the app's scheme from any goroutine; see
[Styling](Styling#dark-mode).

## Background work: Post

[`tela.Post`](https://pkg.go.dev/github.com/kuronosu/tela#Post) runs a function on the UI thread, in order with state updates. It is
the way to touch UI-thread state, such as a [`tela.Ref`](https://pkg.go.dev/github.com/kuronosu/tela#Ref), from a goroutine:

```go
func load(cache *tela.Ref[[]byte]) {
	go func() {
		data := []byte("downloaded")
		tela.Post(func() { cache.Current = data })
	}()
}
```

Functions posted before [`tela.Run`](https://pkg.go.dev/github.com/kuronosu/tela#Run) starts run once the window is up. In tests,
[`telatest.Mount`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Mount) delivers them.

## See also

- The API reference of every hook: [API-Reference-tela](API-Reference-tela#functions).
- [Testing](Testing) shows how to assert on state without a window.
