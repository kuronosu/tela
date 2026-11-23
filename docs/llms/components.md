# Recipe: components, lists and hook patterns

Questions: how to write a struct or a function component and pass children; keys; lists, `Map` and `VirtualList`; keyboard shortcuts; raw text fields; a reducer, a context, scrolling to the newest item; custom hooks.

**A keyed list and a keyboard shortcut.** `Map` needs a `Key` on each item, and `If` evaluates all its arguments:

```go
type Todo struct {
	ID   int
	Text string
	Done bool
}

func TodoList(todos []Todo, remove func(id int)) tela.Node {
	return Div(Role("list"), AriaLabel("Todos"), Class("flex flex-col gap-2"),
		If(len(todos) == 0, P(Class("text-sm text-muted-foreground"), Text("Nothing to do."))),
		Map(todos, func(t Todo) tela.Node {
			return Div(Key(t.ID), Role("listitem"),
				Class("flex items-center justify-between rounded-md border border-border px-3 py-2"),
				Span(Class("text-sm"), If(t.Done, Class("text-muted-foreground line-through")), Text(t.Text)),
				Button(Class("text-sm text-destructive hover:underline"), AriaLabel("Remove "+t.Text),
					OnClick(func() { remove(t.ID) }), Text("Remove")))
		}))
}

func Shortcuts(save func()) tela.Node {
	return Div(TabIndex(0), Class("rounded-md p-4 outline-none focus-visible:ring-2 focus-visible:ring-ring"),
		OnKeyDown(func(e tela.KeyboardEvent) {
			if e.CmdOrCtrl() && e.Key == "s" { // Cmd on a Mac, Ctrl elsewhere
				e.PreventDefault()
				save()
			}
		}),
		Text("Press Ctrl+S to save"))
}
```

**Raw text fields** (prefer `kit.Input` and `kit.Field`, which are styled and wired):

```go
func Search() tela.Node {
	q, setQ := tela.UseState("")
	return Form(OnSubmit(func() { fmt.Println("search", q) }),
		Input(Type("search"), Placeholder("Search"), AriaLabel("Search"), Bind(q, setQ),
			Class("h-9 w-64 rounded-md border border-input bg-transparent px-3 text-sm placeholder:text-muted-foreground",
				"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50")))
}
```

**Long lists:** `VirtualList(count, estimateSize, renderItem, props...)` builds only the visible rows; it is the scroll container (style it like a `Div`). `renderItem` returns a `tela.Node` (a struct component is one) and must not call hooks (return a struct component for rows with state). `ItemKey(func(i int) K)` keys rows by identity.

```go
type Row struct{ Name string }

func (r Row) Build() tela.Node {
	return Div(Class("flex h-12 items-center border-b border-border px-4 text-sm"), Text(r.Name))
}

func People(names []string) tela.Node {
	return VirtualList(len(names), func(int) float32 { return 48 },
		func(i int) tela.Node { return Row{Name: names[i]} },
		ItemKey(func(i int) string { return names[i] }),
		Class("h-96 rounded-lg border border-border"))
}
```

**Struct components** are the default: the fields are the props, `Build` has a value receiver, children are a field of type `tela.Component` (the same type as `tela.Node`; several via `Fragment(...)`). A struct component is a `tela.Node`, so it can be returned from any function or `Build`, passed to `If`, `Map`, `tela.C` and `Root`, and used as a child, with no wrapper. A struct value used as a child is its own component, with its own state, and its build is skipped while its fields are `==` to the previous ones (func or slice fields make it rebuild every time; that is fine).

```go
type Stat struct {
	Label, Value, Trend string
}

func (s Stat) Build() tela.Node {
	return Div(Class("flex flex-col gap-1 rounded-xl border border-border bg-card p-5 shadow-xs"),
		P(Class("text-sm text-muted-foreground"), Text(s.Label)),
		P(Class("text-2xl font-semibold tracking-tight tabular-nums"), Text(s.Value)),
		P(Class("text-xs font-medium text-emerald-600 dark:text-emerald-400"), Text(s.Trend)))
}

type Section struct {
	Title    string
	Children tela.Component
}

func (s Section) Build() tela.Node {
	open, setOpen := tela.UseState(true)
	return Div(Class("flex flex-col gap-3"),
		Button(Class("text-left text-sm font-semibold"), OnClick(tela.Toggle(setOpen)), AriaExpanded(open), Text(s.Title)),
		If(open, s.Children))
}

// A struct component is a tela.Node: return it as it is.
// Never call Section{...}.Build(): its hooks would run in the caller.
func Overview() tela.Node {
	return Section{Title: "This week", Children: Div(Class("grid grid-cols-3 gap-4"),
		Stat{Label: "Revenue", Value: "$12,480", Trend: "+8.1%"},
		Stat{Label: "Orders", Value: "342", Trend: "+3.4%"},
		Stat{Label: "Refunds", Value: "4", Trend: "-1.2%"})}
}
```

**Function components** use `tela.C(fn)` (no props) or `tela.CP(fn, props)` (comparable props, skipped while `==`). Write the wrap once in a public function, so callers never call the private one:

```go
func Clock() tela.Node { return tela.C(clock) }

func clock() tela.Node {
	now, setNow := tela.UseState(time.Now())
	tela.UseInterval(time.Second, func() { setNow.Set(time.Now()) })
	return Span(Class("tabular-nums"), Text(now.Format("15:04:05")))
}

type BadgeProps struct{ Count int }

func Unread(p BadgeProps) tela.Node { return tela.CP(unread, p) }

func unread(p BadgeProps) tela.Node {
	return Span(Class("rounded-full bg-primary px-2 text-xs font-medium text-primary-foreground"), Textf("%d", p.Count))
}
```

Keys: an element takes `Key(id)`; a component or fragment is keyed with `tela.WithKey(Card{...}, id)`, or `tela.C(fn, Key(id))`, `tela.CP(fn, p, Key(id))`, or `MapKeyed`.

A custom hook is a function named `UseXxx`/`useXxx` that calls hooks. `tela.Post(f)` runs `f` on the UI thread from any goroutine.

**A reducer, a context and scrolling to the newest item** (`UseLayoutEffect` runs after layout and before paint, so the frame that shows a new note shows it in view):

```go
type action struct {
	kind string
	text string
}

func reduce(items []string, a action) []string {
	switch a.kind {
	case "add":
		return append(append([]string(nil), items...), a.text)
	case "clear":
		return nil
	}
	return items
}

var Mode = tela.CreateContext("light")

func DarkNotes() tela.Node { return Mode.Provider("dark", Notes()) }

func Notes() tela.Node { return tela.C(notes) }

func notes() tela.Node {
	items, dispatch := tela.UseReducer(reduce, []string{"Buy milk"})
	draft, setDraft := tela.UseState("")
	last := tela.UseElementRef()
	tela.UseLayoutEffect(func() func() {
		last.ScrollIntoView() // the frame that shows a new note shows it in view
		return nil
	}, len(items))
	mode := tela.UseContext(Mode)
	return Div(Class("flex h-80 flex-col gap-3 p-4"),
		P(Class("text-xs text-muted-foreground"), Textf("Mode: %s", mode)),
		Div(Class("flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto"),
			MapIndex(items, func(i int, s string) tela.Node {
				return P(Key(i), If(i == len(items)-1, Ref(last)), Class("text-sm"), Text(s))
			})),
		kit.Input{Placeholder: "New note", AriaLabel: "New note", Value: &draft, OnValueChange: setDraft.Set},
		kit.Button{Disabled: draft == "", Children: Text("Add"), OnClick: func() {
			dispatch.Dispatch(action{kind: "add", text: draft})
			setDraft.Set("")
		}})
}
```
