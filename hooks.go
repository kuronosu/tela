package tela

import (
	"context"
	"time"

	"github.com/kuronosu/tela/internal/core"
)

// Setter sets the state of one [UseState] slot. It is a small comparable
// value, the same on every build of the slot, so it can be stored, passed
// to goroutines and put in props compared by [CP]. Its methods are safe
// from any goroutine: they queue the update and wake the UI thread, which
// applies the queued updates in order before the next build. Updates to a
// component that has been unmounted are dropped. The zero Setter is not
// tied to any state; its methods panic, and IsZero reports it (a Setter
// field left unset, or one read from a context default without a
// Provider).
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
// component that receives it. The zero Callback is tied to no slot: Fn
// panics, and IsZero reports it, for optional callbacks in props:
//
//	if !p.OnClose.IsZero() {
//	    p.OnClose.Fn()()
//	}
type Callback[F any] = core.Callback[F]

// Ref is a mutable box returned by [UseRef]: the same pointer on every
// build. Changing Current does not rebuild.
type Ref[T any] = core.Ref[T]

// ElementRef is a handle on the element it is attached to with h.Ref,
// returned by [UseElementRef]: the same comparable value on every build.
// Its methods act on the element while it is mounted and do nothing (or
// report false) otherwise; call them from event handlers and effects.
//
//   - Focus focuses the element, if it is focusable (a Button, an
//     h.Input or h.Textarea, or an element with h.TabIndex, not disabled
//     or hidden).
//   - Blur removes the focus from the element if it has it.
//   - Focused reports whether the element has the focus of its window.
//   - Click clicks the element like HTML's element.click(): a click event
//     with the primary button, at the center of the element, is dispatched
//     at it (OnClick handlers run and it bubbles; the hooks of package aria
//     see a virtual press), then, unless a handler prevented it, the
//     activation behavior runs: a Button in a Form submits it or resets it
//     by its h.Type, a Label focuses its control and clicks it. No pointer
//     events and no focus change. A disabled or unmounted element ignores
//     it. Call it in a handler or an effect: the state it sets is applied
//     and built by the event or the frame that is running.
//   - ScrollTo(x, y) and ScrollBy(dx, dy) scroll a scrolling element
//     (overflow auto, scroll or hidden) to or by offsets, clamped to its
//     content; ScrollOffset returns its offsets (the DOM's scrollLeft and
//     scrollTop; in a right-to-left element x is 0 at the right and
//     negative toward the left). They jump, or animate when the element
//     has scroll-smooth, and do nothing (or return 0) for other elements.
//   - ScrollIntoView scrolls every scrolling ancestor of the element so
//     that it is visible, each as little as possible (block and inline
//     "nearest").
//   - Bounds returns the element's border box in window coordinates as a
//     [Rect], moved by scrolling and transforms (getBoundingClientRect);
//     the zero Rect before it is laid out. [UseBounds] follows it in a
//     [Value].
//   - ScrollToIndex(i, align) scrolls an h.VirtualList to item i, placed
//     as [ScrollAlign] says, and keeps it there while the items around it
//     are measured; it does nothing for other elements.
//   - Text fields (h.Input, h.Textarea): Value returns the value (the DOM's
//     input.value: mainly for uncontrolled fields), Select selects all of
//     it, SetSelectionRange(start, end) selects part of it and
//     SelectionRange returns the selection, in UTF-8 byte offsets of the
//     value (HTML's select(), setSelectionRange, selectionStart and
//     selectionEnd); on other elements they do nothing and return "" or
//     0, 0.
//
// For example, "/" focuses a search box:
//
//	search := tela.UseElementRef()
//	return Div(OnKeyDown(func(e tela.KeyboardEvent) {
//	        if e.Key == "/" && !search.Focused() {
//	            e.PreventDefault()
//	            search.Focus()
//	            search.Select()
//	        }
//	    }),
//	    Input(Ref(search), Type("search"), Placeholder("Search"), Class("rounded-md border px-2 py-1")),
//	)
//
// A "back to top" button for a list:
//
//	list := tela.UseElementRef()
//	return Div(Class("relative"),
//	    Div(Ref(list), Class("h-96 overflow-y-auto scroll-smooth"), rows),
//	    Button(OnClick(func(tela.MouseEvent) { list.ScrollTo(0, 0) }), Text("Top")))
type ElementRef = core.ElementRef

// ScrollAlign is where [ElementRef].ScrollToIndex puts an item of an
// h.VirtualList in its viewport (TanStack Virtual's align).
type ScrollAlign = core.ScrollAlign

// The alignments of [ElementRef].ScrollToIndex. The first and last items
// include the list's padding: AlignStart on item 0 scrolls to the very
// top, AlignEnd on the last item to the very bottom.
const (
	// AlignAuto scrolls as little as possible: not at all when the item
	// is fully visible, else to align its bottom (when it reaches past
	// the bottom of the viewport) or its top.
	AlignAuto = core.AlignAuto
	// AlignStart puts the item's top at the top of the viewport.
	AlignStart = core.AlignStart
	// AlignCenter centers the item in the viewport.
	AlignCenter = core.AlignCenter
	// AlignEnd puts the item's bottom at the bottom of the viewport.
	AlignEnd = core.AlignEnd
)

// Context passes a value down the tree without props: see [CreateContext].
type Context[T any] = core.Context[T]

// Value is an observable value held by one [UseValue] hook slot:
// v.Get() reads it, v.Set(x) and v.Update(f) change it. The *Value is the
// same pointer on every build, so it is comparable and can be passed in
// props compared by [CP] or through a context. Set and Update are safe
// from any goroutine: they queue the change, in order with the state
// updates, and the UI thread applies it before the next frame. Get is
// safe from any goroutine too and returns the value the UI thread applied
// last (a change still in the queue is not visible yet); Latest returns
// the value as it will be once the queued changes apply, which is what an
// event handler that has just called Set wants to read. Latest never
// subscribes anything.
//
// What a change redoes depends on where Get was called; see [UseValue].
// For comparable types, a value == to the current one changes nothing.
type Value[T any] = core.Value[T]

// Toggle returns a function that flips set's state, for a handler:
// OnClick(tela.Toggle(setOpen)). It uses [Setter].Update, so two clicks
// that land in one frame both count and the state ends where it started,
// which setOpen.Set(!open) does not give (both clicks read the same open).
//
//	open, setOpen := tela.UseState(false)
//	Button(OnClick(tela.Toggle(setOpen)), Text("Menu"))
func Toggle(set Setter[bool]) func() {
	if set.IsZero() {
		panic("tela: Toggle(zero Setter): pass the Setter of a bool returned by tela.UseState, for example tela.Toggle(setOpen)")
	}
	return func() { set.Update(func(b bool) bool { return !b }) }
}

// MapValue returns a function that reads v and applies f, for the deferred
// style props ([WidthFn], [HeightFn], [OpacityFn], [BgFn], [TextColorFn]),
// which take a func with one result: v is read, and tracked, when the prop
// is evaluated in a later phase, not when the component builds.
//
//	Style(tela.HeightFn(tela.MapValue(open, func(t float32) float32 { return 36 + 120*t })))
//	Style(tela.BgFn(tela.MapValue(progress, func(p float32) tela.Color { return mix(from, to, p) })))
//
// h.Scale is MapValue for a float32 times a constant.
func MapValue[T, U any](v *Value[T], f func(T) U) func() U {
	if v == nil {
		panic("tela: MapValue(nil, ...): pass the *tela.Value returned by tela.UseValue, tela.UseSpring or tela.UseTween")
	}
	if f == nil {
		panic("tela: MapValue(v, nil): pass the function that computes the result from the Value, such as func(x float32) float32 { return 36 * x }")
	}
	return func() U { return f(v.Get()) }
}

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
//
// It exists for code that wants React's shape, a setter that is a
// function: to pass it where a func(T) is expected (a callback of a
// library, a field's handler) without wrapping it, and to keep code ported
// from React reading the same. The function is the same on every build,
// but funcs are not comparable in Go, so a props struct holding one makes
// [CP] rebuild its component every time; pass the [Setter] of UseState in
// props instead.
func UseStateSet[T any](initial T) (T, func(T)) { return core.UseStateSet(initial) }

// UseStateUpdate is [UseState] for those who only update from the
// previous state: it returns the Setter's Update method as a plain
// function, made once per slot, for the same reasons as [UseStateSet].
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
//	tela.UseEffect(f)            // after every build
//	tela.UseEffect(f, id, n)     // when id or n changed (compared with ==)
//	tela.UseEffect(f, tela.Once) // only after the first build (React's [])
//	tela.UseMount(f)             // the same
//
// For an effect that runs once, on mount, with its cleanup on unmount (a
// subscription), use [UseMount] or [Once]; [UseEffectDo] takes an effect
// without a cleanup, [UseUpdateEffect] skips the first build, and
// periodic and background work have [UseInterval] and [UseGo]. A single
// nil dependency, UseEffect(f, nil), is rejected in dev mode with the fix
// ([Once]: React reads a null list as "after every render"); outside dev
// mode it keeps meaning "only on mount".
//
// Values that cannot be compared (functions, slices, maps) count as
// changed on every build; dev mode reports them, and a dependency count
// that changes between builds. Pass an ID, a comparable value or a
// [Callback] instead.
//
// Within a frame, the effects of children run before their parents', and
// every cleanup runs before the effects that follow it. Effects of a
// component whose build was skipped do not run. State set by an effect is
// applied in a later frame.
func UseEffect(effect func() func(), deps ...any) { core.UseEffect(effect, deps...) }

// UseMount runs effect once, after the frame that first shows the
// component, and the cleanup it returns (nil for none) when the component
// unmounts: the idiom for subscriptions, listeners and other setup that
// lives as long as the component (React's useEffect(f, [])).
//
//	tela.UseMount(func() func() {
//	    unsubscribe := store.Subscribe(func(s State) { setState.Set(s) })
//	    return unsubscribe
//	})
func UseMount(effect func() func()) { core.UseMount(effect) }

// UseOnce returns init(), called on the first build only; later builds
// return the same value. Use it for what a component creates once and
// keeps: a client, a buffer, the state struct of a custom hook. It is
// [UseMemo] without dependencies; the value is not reactive (store
// what changes in [UseState] or [UseValue]).
//
//	cache := tela.UseOnce(func() map[string]Image { return map[string]Image{} })
func UseOnce[T any](init func() T) T { return core.UseOnce(init) }

// UseEffectDo is [UseEffect] for an effect without a cleanup: f runs after
// the frame is presented when the dependencies changed.
//
//	tela.UseEffectDo(func() { title.Set(fmt.Sprintf("%d unread", n)) }, n)
func UseEffectDo(f func(), deps ...any) { core.UseEffectDo(f, deps...) }

// UseUpdateEffect is [UseEffect] without the run of the first build: the
// effect runs after the later builds whose dependencies changed (react-use's
// useUpdateEffect), and its cleanup before it runs again and on unmount.
// Use it to react to changes, not to the initial value.
//
//	tela.UseUpdateEffect(func() func() {
//	    save(settings) // not on mount, only when settings change
//	    return nil
//	}, settings)
func UseUpdateEffect(effect func() func(), deps ...any) { core.UseUpdateEffect(effect, deps...) }

// UseInterval calls f on the UI thread every d, the first time d after the
// frame that first shows the component, until it unmounts. d <= 0 pauses
// it, and a new d restarts it. f is always the latest build's function, so
// it sees the latest state and needs no dependencies. It runs on the
// window's frame clock, like the long-press timer, with no goroutine:
// telatest's App.Advance moves it, and a headless Render never runs it.
//
//	now, setNow := tela.UseState(time.Now())
//	tela.UseInterval(time.Second, func() { setNow.Set(time.Now()) })
func UseInterval(d time.Duration, f func()) { core.UseInterval(d, f) }

// UseTimeout calls f once on the UI thread, d after the frame that first
// shows the component: the one-shot sibling of [UseInterval], on the same
// frame clock with no goroutine (telatest's App.Advance moves it, and a
// headless Render never runs it). An idle timeout costs one timer, and
// none once it has fired.
//
// d <= 0 cancels it. A change of d or of the dependencies (compared like
// [UseEffect]'s) restarts it from the frame that shows the change, even
// after it fired; with the same d and no dependencies it fires once per
// mount. f is always the latest build's function, so it sees the latest
// state without being a dependency. Unmounting cancels it.
//
// Hide a notice 3 s after it appears, restarting when the message changes:
//
//	tela.UseTimeout(3*time.Second, func() { setVisible.Set(false) }, message)
func UseTimeout(d time.Duration, f func(), deps ...any) { core.UseTimeoutAfter(d, f, deps...) }

// UseGo runs f on a goroutine of its own after the frame is presented,
// when its dependencies changed, like [UseEffect]; ctx is cancelled at
// cleanup, when the dependencies change again or the component unmounts,
// before the next run starts. f must return soon after ctx is done (it is
// not waited for) and must change the UI only through what is safe from
// any goroutine: a [Setter], a [Value], [Post].
//
//	tela.UseGo(func(ctx context.Context) {
//	    for page := range fetchPages(ctx, query) {
//	        setResults.Update(func(r []Result) []Result { return append(r, page...) })
//	    }
//	}, query) // restarts, cancelling the old fetch, when query changes
//
// The dependencies follow UseEffect's: none restarts f after every build,
// and [Once] runs one goroutine for the lifetime of the component.
func UseGo(f func(ctx context.Context), deps ...any) { core.UseGo(f, deps...) }

// UseLayoutEffect is [UseEffect], but the effect runs in the frame after
// layout and before paint, with the same dependencies and cleanups. Its
// state updates are applied in the next frame. The scrolls it asks for
// ([ElementRef]'s ScrollTo, ScrollBy, ScrollIntoView and ScrollToIndex)
// use the layout of its frame, which is painted scrolled: on mount, and
// when the content just grew.
//
//	last := tela.UseElementRef() // on the newest message
//	tela.UseLayoutEffect(func() func() {
//	    last.ScrollIntoView() // the frame that shows a new message shows it in view
//	    return nil
//	}, len(messages))
func UseLayoutEffect(effect func() func(), deps ...any) { core.UseLayoutEffect(effect, deps...) }

// UseMemo returns compute(), computed on the first build and again only
// when the dependencies changed (compared like [UseEffect]'s: none
// recomputes on every build). For a value computed once, use [UseOnce]
// (or UseMemo(compute, tela.Once)).
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
//	return Toolbar(ToolbarProps{OnSave: onSave}) // skips while draft changes
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
//	    tela.UseMount(func() func() {
//	        input.Focus()
//	        return nil
//	    })
//	    return Div(Ref(input), TabIndex(0), Class("rounded-md p-2 focus:ring-2"), Text("Search"))
//	}
func UseElementRef() ElementRef { return core.UseElementRef() }

// UseScroll returns two [Value] objects that follow the scroll offsets of the
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
//	    return Theme.Provider(theme, Page())
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
//
// In dev mode, reading a context without a Provider above warns (once per
// context) when its default is not safe to use: nil, or holding a nil
// function, pointer, map, channel or interface, or a zero [Setter] or
// [Callback], in an exported field. Either provide the context above
// every reader, or give CreateContext a default that works; IsZero tells
// a zero Setter or Callback apart where a missing Provider is expected.
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
//   - inside a paint-phase style closure ([TranslateFn], [OpacityFn],
//     [BgFn], [TextColorFn]):
//     that element is repainted, with no build, style or layout;
//   - anywhere else (event handlers, effects, goroutines): nothing; Get
//     just returns the value.
//
// The reads are taken again on every evaluation, so a closure that stops
// reading a Value stops being redone by it. A progress bar fed by a
// goroutine, which lays out the bar again without building Download:
//
//	func Download(url string) tela.Node {
//	    progress := tela.UseValue(0.0) // from 0 to 1
//	    tela.UseGo(func(ctx context.Context) {
//	        fetch(ctx, url, func(done float64) { progress.Set(done) }) // any goroutine
//	    }, url)
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
//
// Because the *Value is stable and comparable, a struct of Values is a
// good return value for a custom hook: the caller keeps the struct
// between builds, passes it in [CP] props or a context without defeating
// skipping, and each reader redoes only what it read.
//
//	type Drag struct{ X, Y *tela.Value[float32] }
//
//	func UseDrag() Drag {
//	    return Drag{X: tela.UseValue(float32(0)), Y: tela.UseValue(float32(0))}
//	}
//
// In handlers, effects and goroutines, read a Value that may have queued
// changes with Latest: Get returns the value applied last, and dev mode
// warns about a Get that misses queued changes.
func UseValue[T any](initial T) *Value[T] { return core.UseValue(initial) }

// UseID returns a string that identifies this hook slot, for accessibility
// relations between elements (a label and its input). It is the same on
// every build, unique within the process, and the same for the same
// mount order (snapshots are deterministic).
//
//	id := tela.UseID()
//	return Div(Label(For(id), Text("Email")), Input(ID(id)))
func UseID() string { return core.UseID() }

// Once is the dependency of a hook that runs or computes only on the
// first build (React's empty dependency list []):
//
//	tela.UseEffect(f, tela.Once)       // after the first build; cleanup on unmount
//	tela.UseLayoutEffect(f, tela.Once) // the same, before the first paint
//	tela.UseGo(f, tela.Once)           // one goroutine for the component's lifetime
//	tela.UseMemo(f, tela.Once)         // computed once
//
// Its type is unexported, so it is equal to itself on every build and is
// never confused with a value of yours. [UseMount] and [UseOnce] are the
// shorthands for effects and values. It replaces a single nil dependency
// (UseEffect(f, nil)): React reads a null list as "after every render", so
// dev mode panics with this fix, and outside dev mode nil keeps meaning
// "only on mount". With [UseUpdateEffect], which skips the first build,
// Once never runs (dev mode warns).
var Once = core.Once

// C returns a node that builds fn as a component with its own state: a
// component boundary (ARCHITECTURE section 7). Calling fn() directly
// instead runs it inside the caller's build, so its hooks would take the
// caller's slots: dev mode panics when a hook is called from a function
// that is neither the component being built (or a function literal inside
// it) nor a custom hook named UseXxx or useXxx, and says to wrap it.
//
// Call C where the component is defined, in a public function over a
// private one, so that its users write Header() and not tela.C(header):
//
//	func Header() tela.Node { return tela.C(header) }
//
//	func header() tela.Node { ... }
//
//	Div(Header(), Body())
//
// The optional last argument is the component's key, h.Key(id), for a
// component in a list: tela.C(row, Key(id)) is tela.WithKey(tela.C(row),
// id). Anything else but Key panics.
//
// A C component builds whenever its parent builds, since fn may capture
// values that changed; use [CP] for automatic skipping. Only the very same
// node (one kept in a variable or a [UseMemo]) is not built again.
func C(fn func() Node, key ...Child) Node { return core.Component(fn, key...) }

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
//	func Card(p CardProps) tela.Node { return tela.CP(card, p) }
//
//	func card(p CardProps) tela.Node { ... }
//
//	Card(CardProps{Title: "Inbox", OnOpen: onOpen})
//
// Call CP where the component is defined, as above, so that its users never
// write it. Give CP a named function and pass everything it uses in its props:
// variables captured by a closure are not compared, so dev mode reports
// anonymous functions once per call site. Keep props
// comparable by passing [Callback] values instead of funcs, and IDs instead
// of slices or maps; a props value whose == panics counts as changed.
//
// The optional last argument is the component's key, h.Key(id), for the
// items of a list: tela.CP(row, RowProps{...}, Key(row.ID)) is
// tela.WithKey(tela.CP(row, RowProps{...}), row.ID). Anything else but Key
// panics; the key is not a prop of the component and is not compared with
// the props.
func CP[P comparable](fn func(P) Node, props P, key ...Child) Node {
	return core.ComponentProps(fn, props, key...)
}

// Memo is [CP] for props that are not comparable, or that compare in
// another way: the build is skipped when equal(old, new) reports them
// equal. equal must be pure.
//
//	func Chart(p ChartProps) tela.Node {
//	    return tela.Memo(chart, p, func(a, b ChartProps) bool {
//	        return slices.Equal(a.Points, b.Points)
//	    })
//	}
//
// Like [CP], it takes an optional last argument, the key h.Key(id).
func Memo[P any](fn func(P) Node, props P, equal func(a, b P) bool, key ...Child) Node {
	return core.Memo(fn, props, equal, key...)
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
