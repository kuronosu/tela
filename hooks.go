package tela

import "github.com/kuronosu/tela/internal/core"

func init() {
	// UseCallback builds the forwarders of the event handler types
	// without reflection.
	core.RegisterForwarder(func(l *func(MouseEvent)) func(MouseEvent) {
		return func(e MouseEvent) { (*l)(e) }
	})
	core.RegisterForwarder(func(l *func(PointerEvent)) func(PointerEvent) {
		return func(e PointerEvent) { (*l)(e) }
	})
	core.RegisterForwarder(func(l *func(DragEvent)) func(DragEvent) {
		return func(e DragEvent) { (*l)(e) }
	})
	core.RegisterForwarder(func(l *func(KeyboardEvent)) func(KeyboardEvent) {
		return func(e KeyboardEvent) { (*l)(e) }
	})
	core.RegisterForwarder(func(l *func(FocusEvent)) func(FocusEvent) {
		return func(e FocusEvent) { (*l)(e) }
	})
	core.RegisterForwarder(func(l *func(WheelEvent)) func(WheelEvent) {
		return func(e WheelEvent) { (*l)(e) }
	})
	core.RegisterForwarder(func(l *func(ScrollEvent)) func(ScrollEvent) {
		return func(e ScrollEvent) { (*l)(e) }
	})
}

// Setter sets the state of one [UseState] slot. It is a small comparable
// value, the same on every build of the slot, so it can be stored, passed
// to goroutines and put in props compared by [CP]. Its methods are safe
// from any goroutine: they queue the update and wake the UI thread, which
// applies the queued updates in order before the next build. Updates to a
// component that has been unmounted are dropped. The zero Setter is not
// tied to any state; its methods panic.
//
//	setCount.Set(0)
//	setCount.Update(func(n int) int { return n + 1 })
//
// Set(v) replaces the state; Update(f) computes it from the state left by
// the previous queued update, so two queued Update(inc) add 2. f runs on
// the UI thread and must be pure. For comparable types, a new state == to
// the current one does not rebuild.
type Setter[T any] = core.Setter[T]

// Dispatcher sends actions to one [UseReducer] slot: d.Dispatch(action).
// Like [Setter], it is comparable, stable for its slot and safe from any
// goroutine.
type Dispatcher[A any] = core.Dispatcher[A]

// Callback is a stable handle on the latest function given to
// [UseCallback]: cb.Fn() returns one forwarding function, the same on
// every build, that calls the latest function. It is comparable, so a
// props struct holding one stays comparable and [CP] can skip the
// component that receives it.
type Callback[F any] = core.Callback[F]

// Ref is a mutable box returned by [UseRef]: the same pointer on every
// build. Changing Current does not rebuild.
type Ref[T any] = core.Ref[T]

// ElementRef is a handle on the element it is attached to with h.Ref,
// returned by [UseElementRef]: the same comparable value on every build.
// Its methods act on the element while it is mounted and do nothing (or
// report false) otherwise; call them from event handlers and effects.
//
//   - Focus focuses the element, if it is focusable (a Button, or an
//     element with h.TabIndex, not disabled or hidden).
//   - Blur removes the focus from the element if it has it.
//   - Focused reports whether the element has the focus of its window.
//   - ScrollTo(x, y) and ScrollBy(dx, dy) scroll a scrolling element
//     (overflow auto, scroll or hidden) to or by offsets, clamped to its
//     content; ScrollOffset returns its offsets (the DOM's scrollLeft and
//     scrollTop). They jump, or animate when the element has
//     scroll-smooth, and do nothing (or return 0) for other elements.
//   - ScrollIntoView scrolls every scrolling ancestor of the element so
//     that it is visible, each as little as possible (block and inline
//     "nearest").
//
// For example, "/" focuses a search box:
//
//	search := tela.UseElementRef()
//	return Div(OnKeyDown(func(e tela.KeyboardEvent) {
//	        if e.Key == "/" && !search.Focused() {
//	            e.PreventDefault()
//	            search.Focus()
//	        }
//	    }),
//	    Div(Ref(search), TabIndex(0), Class("rounded-md p-2 focus:ring-2"), Text("Search")),
//	)
//
// A "back to top" button for a list:
//
//	list := tela.UseElementRef()
//	return Div(Class("relative"),
//	    Div(Ref(list), Class("h-96 overflow-y-auto scroll-smooth"), rows),
//	    Button(OnClick(func(tela.MouseEvent) { list.ScrollTo(0, 0) }), Text("Top")))
type ElementRef = core.ElementRef

// Context passes a value down the tree without props: see [CreateContext].
type Context[T any] = core.Context[T]

// Value is an observable value held by one [UseValue] hook slot:
// v.Get() reads it, v.Set(x) and v.Update(f) change it. The *Value is the
// same pointer on every build, so it is comparable and can be passed in
// props compared by [CP] or through a context. Set and Update are safe
// from any goroutine: they queue the change, in order with the state
// updates, and the UI thread applies it before the next frame. Get is
// safe from any goroutine too and returns the value the UI thread applied
// last (a change still in the queue is not visible yet).
//
// What a change redoes depends on where Get was called; see [UseValue].
// For comparable types, a value == to the current one changes nothing.
type Value[T any] = core.Value[T]

// UseState returns the state of this hook slot and its [Setter]
// (ARCHITECTURE section 7). The first build stores initial; later builds
// return the latest value set. Setting a different value rebuilds the
// component in the next frame.
//
//	func Counter() tela.Node {
//	    count, setCount := tela.UseState(0)
//	    return Button(OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
//	        Textf("Clicked %d times", count))
//	}
//
// Use setCount.Update(func(n int) int { return n + 1 }) when the new state
// depends on the old one and several updates may be queued before the
// next build (from goroutines, or several events in one frame).
//
// Hooks follow React's rules: call them at the top level of a component
// function, in the same order on every build, never inside conditions or
// loops. UseState panics when called outside a component build or when
// the call order changed since the previous build.
func UseState[T any](initial T) (T, Setter[T]) { return core.UseState(initial) }

// UseStateSet is [UseState] for those who only set whole values: it
// returns the Setter's Set method as a plain function, made once per slot.
//
//	name, setName := tela.UseStateSet("")
//	setName("Ada")
func UseStateSet[T any](initial T) (T, func(T)) { return core.UseStateSet(initial) }

// UseStateUpdate is [UseState] for those who only update from the
// previous state: it returns the Setter's Update method as a plain
// function, made once per slot.
//
//	count, update := tela.UseStateUpdate(0)
//	update(func(n int) int { return n + 1 })
func UseStateUpdate[T any](initial T) (T, func(func(T) T)) { return core.UseStateUpdate(initial) }

// UseReducer returns the state of this hook slot and a [Dispatcher] whose
// actions update it with reducer (React's useReducer). The first build
// stores initial. Queued actions apply in order on the UI thread with the
// reducer of the latest build, each receiving the state left by the
// previous one; reducer must be pure. For comparable states, a result ==
// to the current state does not rebuild.
//
//	type action struct{ kind string }
//
//	func reduce(n int, a action) int {
//	    switch a.kind {
//	    case "inc":
//	        return n + 1
//	    case "reset":
//	        return 0
//	    }
//	    return n
//	}
//
//	count, d := tela.UseReducer(reduce, 0)
//	d.Dispatch(action{"inc"})
func UseReducer[S, A any](reducer func(S, A) S, initial S) (S, Dispatcher[A]) {
	return core.UseReducer(reducer, initial)
}

// UseEffect runs effect after the frame is presented, when its
// dependencies changed since the previous build. effect may return a
// cleanup function (or nil), which runs before the effect runs again and
// when the component unmounts.
//
// The dependencies follow React:
//
//	tela.UseEffect(f)          // after every build
//	tela.UseEffect(f, nil)     // only after the first build (React's [])
//	tela.UseEffect(f, id, n)   // when id or n changed (compared with ==)
//
// A single nil is a constant dependency, so the effect runs only on mount
// and its cleanup only on unmount: the idiom for subscriptions and timers.
//
//	tela.UseEffect(func() func() {
//	    tick := time.NewTicker(time.Second)
//	    done := make(chan struct{})
//	    go func() {
//	        for {
//	            select {
//	            case now := <-tick.C:
//	                setNow.Set(now) // safe from any goroutine
//	            case <-done:
//	                return
//	            }
//	        }
//	    }()
//	    return func() { tick.Stop(); close(done) }
//	}, nil)
//
// Values that cannot be compared (functions, slices, maps) count as
// changed on every build; dev mode (TELA_DEV=1) reports them, and a
// dependency count that changes between builds. Pass an ID, a comparable
// value or a [Callback] instead.
//
// Within a frame, the effects of children run before their parents', and
// every cleanup runs before the effects that follow it. Effects of a
// component whose build was skipped do not run. State set by an effect is
// applied in a later frame.
func UseEffect(effect func() func(), deps ...any) { core.UseEffect(effect, deps...) }

// UseLayoutEffect is [UseEffect], but the effect runs in the frame after
// layout and before paint, with the same dependencies and cleanups. Its
// state updates are applied in the next frame.
//
//	tela.UseLayoutEffect(func() func() {
//	    log.Println("laid out")
//	    return nil
//	}, nil)
func UseLayoutEffect(effect func() func(), deps ...any) { core.UseLayoutEffect(effect, deps...) }

// UseMemo returns compute(), computed on the first build and again only
// when the dependencies changed (compared like [UseEffect]'s: none
// recomputes on every build, a single nil never).
//
//	visible := tela.UseMemo(func() []Todo { return filter(todos, query) }, len(todos), query)
func UseMemo[T any](compute func() T, deps ...any) T { return core.UseMemo(compute, deps...) }

// UseCallback stores fn as the latest function of this hook slot and
// returns the slot's [Callback]. cb.Fn() is one function of type F, the
// same on every build, that calls the latest fn when it is called
// (React's useEffectEvent): handlers keep a stable identity and still see
// the latest build's values. There are no dependencies. F must be a
// function type; UseCallback panics otherwise, and for a nil fn.
//
//	onSave := tela.UseCallback(func(e tela.MouseEvent) { save(draft) })
//	return tela.CP(Toolbar, ToolbarProps{OnSave: onSave}) // skips while draft changes
//
//	// in Toolbar:
//	Button(OnClick(p.OnSave.Fn()), Text("Save"))
//
// Call the forwarder on the UI thread (handlers and effects run there);
// from other goroutines, use [Post].
func UseCallback[F any](fn F) Callback[F] { return core.UseCallback(fn) }

// UseRef returns the [Ref] of this hook slot, a box whose Current field
// persists across builds; the first build sets it to initial. Changing
// Current does not rebuild: use it for values that rendering does not
// show, such as a timer or the previous value of a prop.
//
//	renders := tela.UseRef(0)
//	renders.Current++
func UseRef[T any](initial T) *Ref[T] { return core.UseRef(initial) }

// UseElementRef returns the [ElementRef] of this hook slot, the same on
// every build. Attach it to an element with h.Ref, then act on the element
// from handlers and effects (React's useRef with a DOM element):
//
//	func Search() tela.Node {
//	    input := tela.UseElementRef()
//	    tela.UseEffect(func() func() {
//	        input.Focus()
//	        return nil
//	    }, nil)
//	    return Div(Ref(input), TabIndex(0), Class("rounded-md p-2 focus:ring-2"), Text("Search"))
//	}
func UseElementRef() ElementRef { return core.UseElementRef() }

// UseScroll returns two [Value]s that follow the scroll offsets of the
// element ref is attached to (zero while it is not a mounted scrolling
// element): x is its horizontal offset (scrollLeft) and y its vertical
// one (scrollTop), in logical pixels. They change on the UI thread as the
// element scrolls, so a deferred style closure that reads them repaints
// or lays out only its element, without rebuilding (Motion's useScroll).
// The same Values are returned on every build.
//
//	func Page() tela.Node {
//	    list := tela.UseElementRef()
//	    _, y := tela.UseScroll(list)
//	    header := Div(Class("h-16 bg-white shadow"), Style(tela.OpacityFn(func() float32 {
//	        return 1 - min(y.Get()/200, 1) // fades out over the first 200 px
//	    })))
//	    return Div(Class("flex h-screen flex-col"), header,
//	        Div(Ref(list), Class("grow overflow-y-auto"), rows()))
//	}
//
// Reading y.Get() in the component's body instead rebuilds the component
// on every scrolled frame; use h.OnScroll for logic that runs once per
// frame of scrolling.
func UseScroll(ref ElementRef) (x, y *Value[float32]) { return core.UseScroll(ref) }

// CreateContext returns a new context whose [UseContext] outside any
// Provider returns def. Create contexts once, at package level.
//
//	var Theme = tela.CreateContext("light")
//
//	func App() tela.Node {
//	    theme, _ := tela.UseState("dark")
//	    return Theme.Provider(theme, tela.C(Page))
//	}
//
// ctx.Provider(value, children...) returns a node without a box that
// gives value to its children's subtrees (only h.Key is accepted as a
// prop). When the Provider gets a value that is not == to the previous
// one (values that cannot be compared always count as changed), only the
// components that read it with UseContext build again, even when the
// components between them skip their build.
func CreateContext[T any](def T) *Context[T] { return core.CreateContext(def) }

// UseContext returns the value of the nearest Provider of ctx above the
// component, or the context's default when there is none.
//
//	theme := tela.UseContext(Theme)
func UseContext[T any](ctx *Context[T]) T { return core.UseContext(ctx) }

// UseValue returns the [Value] of this hook slot: an observable value for
// something that changes often, such as a drag offset, a scroll position
// or download progress (ARCHITECTURE section 7, phase-deferred reads, as
// in Jetpack Compose). The first build stores initial; every build returns
// the same *Value.
//
// Where v.Get() is called decides what a change redoes:
//
//   - in the component's body while it builds: the component builds
//     again, like with [UseState] (also when the components above it skip
//     their build);
//   - inside a layout-phase style closure ([WidthFn], [HeightFn]): that
//     element is laid out again, with no build and no style computation;
//   - inside a paint-phase style closure ([TranslateFn], [OpacityFn]):
//     that element is repainted, with no build, style or layout;
//   - anywhere else (event handlers, effects, goroutines): nothing; Get
//     just returns the value.
//
// The reads are taken again on every evaluation, so a closure that stops
// reading a Value stops being redone by it. A progress bar fed by a
// goroutine, which lays out the bar again without building Download:
//
//	func Download() tela.Node {
//	    progress := tela.UseValue(0.0) // from 0 to 1
//	    tela.UseEffect(func() func() {
//	        ctx, cancel := context.WithCancel(context.Background())
//	        go fetch(ctx, func(done float64) { progress.Set(done) }) // any goroutine
//	        return cancel
//	    }, nil)
//	    return Div(Class("h-2 w-64 overflow-hidden rounded-full bg-slate-200"),
//	        Div(Class("h-full bg-indigo-500"),
//	            Style(tela.WidthFn(func() float32 { return float32(256 * progress.Get()) }))))
//	}
//
// A dragged card that moves by repainting only (the offset is set from
// pointer handlers):
//
//	type Offset struct{ X, Y float32 }
//
//	offset := tela.UseValue(Offset{})
//	card := Div(Class("rounded-xl bg-white p-6 shadow-lg"),
//	    Style(tela.TranslateFn(func() (x, y float32) {
//	        p := offset.Get()
//	        return p.X, p.Y
//	    })))
//
// Reading the same Value in the body instead (x := offset.Get()) would
// build the component on every change. Values read by the closures of
// another window's tree are not tracked.
func UseValue[T any](initial T) *Value[T] { return core.UseValue(initial) }

// UseId returns a string that identifies this hook slot, for accessibility
// relations between elements (a label and its input). It is the same on
// every build, unique within the process, and the same for the same
// mount order (snapshots are deterministic).
//
//	id := tela.UseId()
func UseId() string { return core.UseId() }

// C returns a node that builds fn as a component with its own state: a
// component boundary (ARCHITECTURE section 7). Calling fn() directly
// instead runs it inside the caller's build, as a custom hook whose hooks
// belong to the caller.
//
//	Div(tela.C(Header), tela.C(Body))
//
// A C component builds whenever its parent builds, since fn may capture
// values that changed; use [CP] for automatic skipping. Only the very same
// node (one kept in a variable or a [UseMemo]) is not built again.
func C(fn func() Node) Node { return core.Component(fn) }

// CP returns a node that builds fn(props) as a component with its own
// state. When its parent builds again with props == to the previous ones,
// and neither its state nor a context it reads changed, the build is
// skipped (Jetpack Compose's automatic skipping): free memoization.
//
//	type CardProps struct {
//	    Title string
//	    OnOpen tela.Callback[func(tela.MouseEvent)]
//	}
//
//	func Card(p CardProps) tela.Node { ... }
//
//	tela.CP(Card, CardProps{Title: "Inbox", OnOpen: onOpen})
//
// Give CP a named function and pass everything it uses in its props:
// variables captured by a closure are not compared, so dev mode
// (TELA_DEV=1) reports anonymous functions once per call site. Keep props
// comparable by passing [Callback] values instead of funcs, and IDs instead
// of slices or maps; a props value whose == panics counts as changed.
func CP[P comparable](fn func(P) Node, props P) Node { return core.ComponentProps(fn, props) }

// Memo is [CP] for props that are not comparable, or that compare in
// another way: the build is skipped when equal(old, new) reports them
// equal. equal must be pure.
//
//	tela.Memo(Chart, ChartProps{Points: pts}, func(a, b ChartProps) bool {
//	    return slices.Equal(a.Points, b.Points)
//	})
func Memo[P any](fn func(P) Node, props P, equal func(a, b P) bool) Node {
	return core.Memo(fn, props, equal)
}

// Post runs fn on the UI thread of the running application, in order with
// the state updates queued before and after it. It is safe from any
// goroutine; use it to touch UI-thread state (a [Ref], a [Callback]'s
// forwarder) from background work.
//
//	go func() {
//	    data := fetch()
//	    tela.Post(func() { cache.Current = data })
//	}()
//
// Functions posted before [Run] starts run once its window is up. Headless
// trees (tests, snapshots) do not run posted functions unless they receive
// them (telatest.Mount does); until then they wait, in order.
func Post(fn func()) { core.Post(fn) }
