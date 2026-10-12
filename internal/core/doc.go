// Package core is Tela's runtime (ARCHITECTURE sections 6 to 8): the three
// trees, reconciliation, hooks, the frame pipeline and its scheduler, and
// the hosts that run a tree in a window or headless. The public packages
// tela and h are thin wrappers over it.
//
// core imports gfx, gfx/raster, text, style, layout and platform (with
// platform/auto for Run); nothing but tela and h imports core
// (ARCHITECTURE section 5).
//
// # Nodes, Elements and Boxes
//
// A Node is an immutable description built by the constructors:
// Primitive (a primitive such as Div, H1, Button or Img, with Props and children),
// Text, Component (a func() Node; ComponentProps for func(P) Node) and
// Fragment, List and Map (children without a box). If returns the zero
// Node, which renders nothing. Children and props share the sealed Child
// interface.
//
// An Element is the persistent instance of a Node. Reconciliation
// (Tree.reconcile) matches the new children of an Element with the old
// ones by identity and type: a child with a key is matched by key among
// the children of the same list, a child without one by position. The
// direct children of a primitive form one list in which a zero Node is a
// hole that keeps the positions of the siblings after it (so If(cond, A)
// does not remount them); each fragment child (List, Map, Fragment) forms
// its own list, so a growing list does not shift its siblings. A match of
// the same type is updated and keeps its state; otherwise the old Element
// is unmounted and a new one mounted. In dev mode a list from Map or List
// with two or more children without keys is reported once per parent, as
// are duplicate keys.
//
// A component Element calls its function on every build of its parent
// (there is no build skipping before phase 4) and when its own state
// changes. A primitive Element owns a Box, the render object: its
// ComputedStyle, its layout.Style and layout data, and its content.
//
// # Hooks
//
// Hooks keep their state in slots on the component Element being built,
// by call order (ADR-040). UseState is the only hook of phase 3. Its
// setter is safe from any goroutine: it queues the update and wakes the
// UI thread (the backend's Wake); the update is applied on the UI thread
// (Tree.ApplyUpdates, on the WakeEvent and at the start of each Frame) and
// marks the component needsBuild, which requests a frame. A function
// called directly from a component, not through Component, runs inside
// that component's build: its hooks are the component's (a custom hook).
// A component that calls a different number of hooks than in its
// previous build panics with an explanation; the full order check is
// phase 4's.
//
// # Boxes and paragraphs
//
// The box phase (structure.go) derives the box tree from the Elements
// after styles are known: components are transparent, display: contents
// lifts its children, a block container whose content is all inline is
// one paragraph (one text.Layout whose spans carry each inline element's
// text style), mixed content gets anonymous paragraph boxes, and text runs
// of flex containers become anonymous flex items. Inline elements inside a
// paragraph contribute only their text style; their box properties
// (background, padding, border, margin) are not laid out or painted.
// Inline-block, inline-flex and inline-grid boxes, images and buttons are
// block-level boxes, never atomic inlines of a paragraph.
//
// A paragraph is shaped again (text.NewLayout) only when its text, a text
// style or its paragraph style changes; a new width only breaks its lines
// again (text.Layout.BreakLines), and a width at or above its max-content
// width only realigns them.
//
// # Style and layout styles
//
// The style phase computes the ComputedStyle of each dirty primitive with
// style.Compute: Preflight defaults of its kind, its classes (compiled by
// the tree's parser, interned) and typed props, inheriting from its
// nearest primitive ancestor. When the inherited properties change, the
// children are restyled. Structural variants (first:, last:, only:, odd:,
// even:) are evaluated against the primitive siblings; the interaction
// variants (hover:...) wait for phase 4's input. Unknown classes are
// reported once per class string to Config.Log (stderr in dev mode).
//
// mapLayoutStyle maps a ComputedStyle onto layout.Style; see it for the
// approximations (grid as block, fixed as absolute, calc() with both
// parts). space-x-* and space-y-* are gaps: a flex container uses its
// gap, and a block container adds the gap to the end margin of every
// in-flow child but the last (Tailwind's margins). Between inline
// elements of a paragraph, space-x-* becomes an empty inline box.
//
// # Pipeline
//
// Tree.Frame runs Build, Style, box structure, Layout and Paint
// (ARCHITECTURE section 8) on what is dirty: components marked
// needsBuild (shallowest first), primitives marked needsStyle, primitives
// whose structure or paragraph may have changed, and the layout engine's
// own dirty nodes (MarkDirty on changed layout styles, children lists and
// paragraphs). Paint emits one display list for the whole tree (layers
// and damage arrive in phase 4); paint.go documents the hook for the
// painting of borders, radii, shadows and effects. A Frame with nothing
// dirty returns the previous list.
//
// A Host connects a Tree to a platform window: WakeEvent applies state
// updates, ConfigureEvent sets the viewport, FrameEvent runs Frame,
// rasterizes at the window scale into a reused framebuffer and presents.
// A tree requests a frame (Window.RequestFrame) only when something
// becomes dirty, so an idle window costs no CPU. Run opens a window with
// the native backend and quits when it is closed. Render does everything
// without a window and returns the display list and the image; Inspect
// returns the laid-out box tree as plain data.
//
// # Threads
//
// A Tree belongs to its UI thread: only the state setters (and SetWake)
// may be called from other goroutines. Builds of all Trees in the process
// are serialized by one mutex, because hooks find the Element being built
// through a package variable.
package core
