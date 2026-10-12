// Package tela is the public API of Tela, a desktop UI framework written
// in pure Go: no cgo, no webview, a React-like component model and
// Tailwind CSS styling.
//
// A UI is a tree of [Node] values built by function components. Elements
// (Div, Button, Text...) live in the companion package h, designed to be
// dot-imported; this package holds the application ([App], [Run]), state
// ([UseState]), events, the typed style props for h.Style, colors, and the
// [Previews] that `tela snapshot` and `tela inspect` render.
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
//	            OnClick(func(tela.MouseEvent) { setCount(count + 1) }),
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
// [HeightFn]. Colors are extended sRGB; see [Color].
//
// # Hooks
//
// Hooks follow React's rules: call them at the top level of a component
// function, in the same order on every build, never inside conditions or
// loops. A hook called outside a component build panics with an
// explanation. Phase 3 provides [UseState]; the rest of the hooks and the
// component constructors arrive with phase 4.
//
// # Snapshots and inspection
//
// A package that defines `func Preview() tela.Node`, or a [Previews]
// variable, can be rendered headlessly with `tela snapshot` and its layout
// printed with `tela inspect`. See the documentation of cmd/tela.
//
// # Stability
//
// [Node], [Child], [Prop], [StyleProp], [Color], [Shadow], [Gradient] and
// [GradientStop] are aliases of types defined in Tela's internal packages
// (the packages cannot be imported, but their exported fields and methods
// are part of the API shown here). Construct them with the functions of
// this package rather than with composite literals where one exists.
package tela
