// Package canvas is the drawing API of h.Canvas (ADR-319): the names and
// the rules of the browser's Canvas 2D (the HTML Standard,
// CanvasRenderingContext2D), recorded into a gfx.DisplayList.
//
// It knows gfx, svg and text, and nothing above them (ARCHITECTURE section
// 5). Package core decides when a canvas draws, and gives each draw what it
// starts from (a [Frame]): the size, the clock, the colors and the font of
// the element's style, and who converts images (an [Env]).
//
// # A draw
//
// A [Recorder] runs the application's function with a [Canvas] and returns
// the list it recorded, in the canvas's own coordinates (logical pixels,
// (0, 0) at its top left) and clipped to its size. One Recorder serves all
// the canvases of a window, one draw at a time: what it keeps between draws
// is scratch (the Builder, the path's storage, the stack of Save) and what it
// shaped (text.go), so a draw costs its display list and one Canvas.
//
// The Canvas itself is never used again. A program that keeps one after its
// draw, or gives it to a goroutine, holds an object that records nothing: its
// methods panic in dev mode and do nothing otherwise, also while a later
// draw runs. (Two goroutines that use a Canvas during its draw race, as with
// any value that does not say otherwise.)
//
// # State
//
// The properties of the browser's context are the exported fields of Canvas.
// The type state is what Save keeps: those fields, the line dash, the
// transform and how many clips are open. A property added to Canvas goes in
// state, Canvas.state and Canvas.setState, and its initial value in
// initialState.
//
// Invalid values are the exception to the browser's rules: where it ignores
// an assignment (a width of zero, an alpha of 2) and keeps the value before,
// a field holds what it was given, so it is judged when something is drawn.
// Each field says how.
//
// # Coordinates
//
// The API is float64, the list float32. The transform is kept in float64 and
// applied to the points of a path when they are added (path.go), as in the
// browser, so that only the transformed points are rounded. A drawing is
// recorded in the canvas's coordinates when it can be, and under its
// transform when that distorts it: lines, text, images, the circles of
// gradients (style.go). Every product is rounded explicitly (ADR-041) and
// sines come from fmath, so a list is the same on every architecture.
//
// # Compositing
//
// Most draws are a plain list. One that used another operation than
// source-over, or cleared a part of itself, is a layer, so that it meets only
// itself; that is known at the end of the draw, when its list is wrapped
// (Recorder.finish). draw.go is the drawing model.
package canvas
