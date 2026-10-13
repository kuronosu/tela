// Package tela is the public API of Tela, a desktop UI framework written
// in pure Go: no cgo, no webview, a React-like component model and
// Tailwind CSS styling.
//
// A UI is a tree of [Node] values built by function components. Elements
// (Div, Button, Text...) live in the companion package h, designed to be
// dot-imported; this package holds the application ([App], [Run]), hooks
// and component boundaries ([UseState], [C], [CP]...), events, the typed
// style props for h.Style, colors, and the [Previews] that `tela
// snapshot` and `tela inspect` render.
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
//	    tela.Run(tela.App{Title: "Demo", Width: 800, Height: 600, Root: Counter})
//	}
//
// # Styling
//
// Static styling uses Tailwind v4 classes (h.Class). Dynamic values go
// through typed props, which apply after the classes:
//
//	Div(Class("rounded-xl p-4"), Style(tela.Width(w), tela.Bg(tela.Hex("#1e1e2e"))))
//
// Values that change every frame (a drag position, a progress bar) can be
// deferred to a later phase with [TranslateFn], [OpacityFn], [WidthFn] and
// [HeightFn]: a [UseValue] read inside them redoes only that element's
// layout or paint when it changes. Colors are extended sRGB; see [Color].
//
// # Components
//
// A component is a function returning a [Node]. Wrapped in [C] (no props)
// or [CP] (comparable props), it becomes a component boundary: an element
// with its own hook state, rebuilt on its own when that state changes.
// [CP] skips the build when its props are == to the previous ones and
// nothing it reads changed (automatic skipping, as in Jetpack Compose);
// [Memo] does the same with a comparison function. Calling a component
// function directly runs it inside the caller, as a custom hook.
//
//	Div(tela.C(Header), tela.CP(Card, CardProps{Title: "Inbox"}))
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
//     [UseLayoutEffect] after layout, with cleanups; UseEffect(f, nil)
//     runs only on mount.
//   - [UseMemo] caches a computation; [UseCallback] gives a stable
//     [Callback] that always calls the latest function; [UseRef] keeps a
//     mutable box.
//   - [CreateContext] and [UseContext] pass values down the tree.
//   - [UseId] gives a stable unique ID.
//   - [UseValue] holds an observable [Value] for what changes every
//     frame: read in the component's body, a change builds it again; read
//     inside a deferred style closure, a change only lays out or repaints
//     that element.
//
// A Value moved from a handler or a goroutine repaints only the element
// whose closure reads it:
//
//	pos := tela.UseValue(0.0)
//	Div(Class("size-8 rounded-full bg-indigo-500"),
//	    Style(tela.TranslateFn(func() (x, y float32) { return float32(pos.Get()), 0 })))
//	// later, from anywhere: pos.Set(120)
//
// Hooks follow React's rules: call them at the top level of a component
// function, in the same order on every build, never inside conditions or
// loops. A hook called outside a component build panics with an
// explanation; in dev mode (TELA_DEV=1) a change of the hook order panics
// with both call sites. Everything runs on the UI thread except the
// Setter, Dispatcher and Value methods and [Post], which are safe from any
// goroutine.
//
// # Snapshots and inspection
//
// A package that defines `func Preview() tela.Node`, or a [Previews]
// variable, can be rendered headlessly with `tela snapshot` and its layout
// printed with `tela inspect`. See the documentation of cmd/tela.
//
// # Stability
//
// [Node], [Child], [Prop], [StyleProp], [Color], [Shadow], [Gradient],
// [GradientStop], [Setter], [Dispatcher], [Callback], [Ref], [Context] and
// [Value] are aliases of types defined in Tela's internal packages
// (the packages cannot be imported, but their exported fields and methods
// are part of the API shown here). Construct them with the functions of
// this package rather than with composite literals where one exists.
package tela
