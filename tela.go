// Package tela is the public API of Tela, a desktop UI framework written
// in pure Go: no cgo, no webview, a React-like component model and
// Tailwind CSS styling.
//
// A UI is a tree of [Node] values built by components: struct values with
// a Build method ([Component]) and functions. Elements (Div, Button,
// Text...) live in the companion package h, designed to be dot-imported;
// this package holds the application ([App], [Main], [Run]), hooks and
// component boundaries ([Component], [UseState], [C], [CP]...), events
// ([MouseEvent], [PointerEvent], [DragEvent], [KeyboardEvent] with the
// [KeyName] and [KeyCode] constants, [FocusEvent], and the [ChangeEvent],
// [InputEvent], [CompositionEvent], [SelectEvent] and [SubmitEvent] of
// text fields and forms), [ElementRef], the typed style props for h.Style,
// colors, and the [Previews] that `tela snapshot` and `tela inspect`
// render.
//
//	import (
//	    "github.com/kuronosu/tela"
//	    . "github.com/kuronosu/tela/h"
//	)
//
//	func Counter() tela.Node {
//	    count, setCount := tela.UseState(0)
//	    return Div(Class("flex flex-col items-center gap-6 p-10"),
//	        H1(Class("text-5xl font-bold"), Textf("Count: %d", count)),
//	        Button(Class("px-6 py-3 rounded-xl bg-indigo-500 text-white"),
//	            OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
//	            Text("Increment")),
//	    )
//	}
//
//	func main() {
//	    tela.Main(tela.App{Title: "Demo", Width: 800, Height: 600, Root: tela.C(Counter)})
//	}
//
// [Main] runs the app and, if no window can be opened, prints the error
// and exits with status 1; [Run] returns the error instead.
//
// # Styling
//
// Static styling uses Tailwind v4 classes (h.Class). A class that depends
// on state goes in h.If, which takes props as well as nodes, and dynamic
// values go through typed props, which apply after the classes:
//
//	Div(Class("rounded-xl p-4"), If(selected, Class("ring-2 ring-indigo-500")),
//	    Style(tela.Width(w), tela.Bg(tela.Hex("#1e1e2e"))))
//
// h.Class takes several strings and joins them with a space, and a
// [StyleProp] is also a child, so Div(Class("p-4", tone), tela.Width(w))
// needs neither concatenation nor Style; [WH] and [Box] set several at once.
//
// Values that change every frame (a drag position, a progress bar) can be
// deferred to a later phase with [TranslateFn], [OpacityFn], [WidthFn] and
// [HeightFn]: a [UseValue] read inside them redoes only that element's
// layout or paint when it changes ([MapValue] and h.Scale compute from one).
// Colors are extended sRGB; see [Color].
//
// # Components
//
// A component is a struct whose Build method returns a [Node]: its fields
// are its props, and children are an ordinary field of type [Component]
// (several of them through h.Fragment). Every component value given to an
// element is a component boundary: an element with its own hook state,
// rebuilt on its own when that state changes, and skipped when its parent
// builds again with an equal value and nothing it reads changed
// (automatic skipping, as in Jetpack Compose).
//
//	type Card struct {
//	    Title    string
//	    OnOpen   tela.Callback[func()]
//	    Children tela.Component
//	}
//
//	func (c Card) Build() tela.Node {
//	    n, setN := tela.UseState(0)
//	    return Div(Class("rounded-xl border p-4"),
//	        H2(Text(c.Title)),
//	        Button(OnClick(func(tela.MouseEvent) { setN.Set(n + 1) }), Textf("Opened %d times", n)),
//	        c.Children)
//	}
//
//	Div(Card{Title: "Inbox", Children: Fragment(Text("a"), Span(Text("b")))})
//
// Build has a value receiver. The identity of a component among its
// siblings is its type and its key ([WithKey]), and the comparison that
// skips its build never panics: see [Component].
//
// A function returning a [Node] is a component too when it is wrapped in
// [C] (no props) or [CP] (comparable props); [CP] skips like a struct
// component, and [Memo] does the same with a comparison function.
//
//	Div(tela.C(Header), tela.CP(Badge, BadgeProps{Count: 3}))
//
// Calling Build, or a component function, directly runs it inside the
// caller, whose hook slots its hooks would take: in dev mode a hook called
// from a function that is neither the component being built (its Build,
// its function, or a function literal inside them) nor a custom hook
// panics with the fix. A custom hook
// is a function named UseXxx or useXxx that calls hooks for the
// component that calls it; a struct of [Value] pointers is a good thing
// for one to return, since it is stable and comparable:
//
//	type Drag struct{ X, Y *tela.Value[float32] }
//
//	func UseDrag() Drag {
//	    return Drag{X: tela.UseValue(float32(0)), Y: tela.UseValue(float32(0))}
//	}
//
// A plain function without hooks (a wrapper that styles its children)
// needs no boundary; h.Fragment(children...) passes the children it
// receives through.
//
// # Hooks
//
// Hooks keep state and run code across builds, with React's names and
// semantics:
//
//   - [UseState] (and [UseStateSet], [UseStateUpdate]) and [UseReducer]
//     hold state; their [Setter] and [Dispatcher] are comparable, stable
//     and safe from any goroutine.
//   - [UseEffect] runs code after the frame is presented and
//     [UseLayoutEffect] after layout, with cleanups, when their
//     dependencies change; [UseMount] runs once, on mount (with its
//     cleanup on unmount), [UseEffectDo] has no cleanup and
//     [UseUpdateEffect] skips the first build.
//   - [UseInterval] calls a function every period on the UI thread, and
//     [UseGo] runs a function on its own goroutine with a context that
//     is cancelled when its dependencies change or the component
//     unmounts.
//   - [UseMemo] caches a computation and [UseOnce] computes a value once;
//     [UseCallback] gives a stable [Callback] that always calls the latest
//     function; [UseRef] keeps a mutable box.
//   - [CreateContext] and [UseContext] pass values down the tree.
//   - [UseID] gives a stable unique ID.
//   - [UseElementRef] gives an [ElementRef] to attach with h.Ref, to
//     focus or scroll an element from handlers and effects.
//   - [UseValue] holds an observable [Value] for what changes every
//     frame: read in the component's body, a change builds it again; read
//     inside a deferred style closure, a change only lays out or repaints
//     that element.
//   - [UseScroll] gives Values that follow an element's scroll offsets,
//     for effects tied to scrolling that repaint without rebuilding.
//   - [UseBounds] gives a Value of an element's bounds in the window (a
//     [Rect], moved by scrolling and transforms), a ResizeObserver that
//     also sees position; in a handler, [ElementRef].Bounds reads them.
//
// # Scrolling
//
// An element with overflow-auto or overflow-scroll (per axis:
// overflow-x-*, overflow-y-*) scrolls with the wheel, the touchpad (with a
// kinetic fling when the fingers lift), touch, the keyboard (arrows, Page
// Up/Down, Home/End, Space) and its scrollbars; overflow-hidden scrolls
// only from code. The window itself scrolls like a browser page when the
// content is taller or wider than it ([App].RootClass "overflow-hidden"
// turns that off); h-screen and h-full on the outer element still mean
// the window's height. Wheel scrolling chains to the scrolling ancestors at an
// edge unless overscroll-contain stops it. Scrollbars are thin overlay
// bars that appear while scrolling; [App].ClassicScrollbars (or the class
// scrollbar-classic) draws always-visible classic bars that take space.
// User scrolling animates; [App].InstantScroll (or scroll-instant on an
// element) turns animation and flings off, scroll-smooth turns them back
// on, and the system's reduced-motion setting always does. Code scrolls
// with [ElementRef]'s ScrollTo, ScrollBy and ScrollIntoView (animated with
// scroll-smooth, like CSS), and h.OnScroll and h.OnWheel observe it:
//
//	list := tela.UseElementRef()
//	_, y := tela.UseScroll(list)
//	header := Div(Class("h-16 bg-white shadow"), Style(tela.OpacityFn(func() float32 {
//	    return 1 - min(y.Get()/200, 1) // repaints only
//	})))
//	return Div(Class("flex h-screen flex-col"), header,
//	    Div(Ref(list), Class("grow overflow-y-auto"), rows),
//	    Button(OnClick(func(tela.MouseEvent) { list.ScrollTo(0, 0) }), Text("Top")))
//
// Long lists use h.VirtualList, which builds only the rows in view, keeps
// the scroll position stable while rows of unknown height are measured,
// and scrolls to a row with [ElementRef]'s ScrollToIndex and a
// [ScrollAlign].
//
// Text can be deferred the same way with h.TextFn: a Value read inside it
// lays out only the paragraph that holds the text, without building the
// component (a clock, a time label next to a seek bar).
//
// A Value moved from a handler or a goroutine repaints only the element
// whose closure reads it:
//
//	pos := tela.UseValue(float32(0)) // UseValue(0.0) would hold a float64
//	Div(Class("size-8 rounded-full bg-indigo-500"),
//	    Style(tela.TranslateFn(func() (x, y float32) { return pos.Get(), 0 })))
//	// later, from anywhere: pos.Set(120)
//
// Hooks follow React's rules: call them at the top level of a component's
// Build or function, in the same order on every build, never inside
// conditions or loops. A hook called outside a component build panics
// with an explanation (a component called where a node is expected is
// told to use [C], or to pass the struct value); in dev mode a change of
// the hook order panics with both call sites. Everything runs on the UI thread except the Setter, Dispatcher
// and Value methods and [Post], which are safe from any goroutine.
//
// # Dev mode
//
// Dev mode reports the mistakes that compile. It is on unless the program
// is a release build (built with -trimpath, or installed with go install
// pkg@version; go run and go build in a source checkout are development
// builds); [App].Dev overrides that, and TELA_DEV=1 or TELA_DEV=0 overrides
// both. telatest runs in dev mode unless telatest.NoDev says otherwise. In
// dev mode:
//
//   - unknown classes are printed on standard error with the closest
//     class (in telatest they fail the test);
//   - unknown roles, input types and easings, hook-order changes, hooks
//     called outside their component, a pointer given as a [Component]
//     and a [Prop] in a field of type Component panic with the fix;
//   - likely mistakes are printed as warnings, once per call site: lists
//     without keys or with duplicate keys, UseEffect(f, nil) (deprecated:
//     use [Once] or [UseMount]), dependencies that cannot be compared, a [Value]
//     read with Get in a handler while changes are queued (use Latest), a
//     context read without a Provider whose default is not safe to use,
//     and an h.Label that labels no field.
//
// # Text fields and forms
//
// h.Input and h.Textarea are editable text fields with the editing,
// selection, clipboard, undo and IME behavior of a browser's; h.Form
// submits on Enter or a submit button (h.OnSubmit), and h.Label names and
// focuses its field. Their events are [ChangeEvent] (every edit, React's
// onChange), [InputEvent] (before an edit, cancelable), [CompositionEvent],
// [SelectEvent] and [SubmitEvent]; [ElementRef] reads and selects their
// text (Value, Select, SetSelectionRange, SelectionRange). See fields.go
// for an example.
//
// # Snapshots and inspection
//
// A package that defines `func Preview() tela.Node` (or tela.Component),
// or a [Previews] variable of [Preview] values, can be rendered headlessly
// with `tela snapshot`, and `tela inspect` prints its semantics tree as a
// Playwright aria snapshot (roles, names and states set by the h.Role and
// h.Aria... props or implied by the elements) or, with --layout, its layout
// tree. A package-level `var App = tela.App{...}` gives both the window's
// size, root class, color scheme, theme and class aliases, so the image
// matches the window; telatest.MountApp mounts the same App in a test. See
// the documentation of cmd/tela.
//
// # Stability
//
// [Node], [Component], [Child], [Prop], [StyleProp], [Color], [Shadow],
// [Gradient], [GradientStop], [Setter], [Dispatcher], [Callback], [Ref], [Context],
// [Value], [ElementRef], [Rect], [ScrollAlign], [MouseEvent],
// [PointerEvent], [DragEvent], [KeyboardEvent], [FocusEvent], [WheelEvent],
// [ScrollEvent], [InputEvent], [ChangeEvent], [CompositionEvent],
// [SelectEvent], [SubmitEvent], [KeyName], [KeyCode], [KeyLocation],
// [MouseButton], [MouseButtons] and [PointerKind] are aliases of types
// defined in Tela's internal packages (the packages cannot be imported, but
// their exported fields and methods are part of the API shown here).
// Construct them with the functions of this package rather than with
// composite literals where one exists.
package tela
