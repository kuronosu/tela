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
// A component Element calls its function when its own state or a
// context it reads changes, and when its parent builds it again, unless
// the build can be skipped (ARCHITECTURE section 7): a ComponentProps
// node (tela.CP) whose props are == to the previous ones, a Memo node
// whose equal function says so, or the very same node (an identical
// desc, for any component) are not built; a Component node (tela.C) is
// built with its parent otherwise, since its closure may capture changed
// values. A skipped component keeps the new node, so its next build of
// its own runs the newest closure. A primitive Element owns a Box, the
// render object: its ComputedStyle, its layout.Style and layout data,
// and its content.
//
// # Hooks
//
// Hooks keep their state in slots on the component Element being built,
// by call order (ADR-040): UseState (UseStateSet, UseStateUpdate),
// UseReducer, UseEffect, UseLayoutEffect, UseMemo, UseCallback, UseRef,
// UseContext and UseId. A function called directly from a component, not
// through Component, runs inside that component's build: its hooks are
// the component's (a custom hook).
//
// State: a Setter or Dispatcher is a comparable handle on its slot, safe
// from any goroutine. Set, Update and Dispatch queue an update and wake
// the UI thread (the backend's Wake); Tree.ApplyUpdates (on the
// WakeEvent and at the start of each Frame) applies the queue in order,
// each updater or reducer receiving the state left by the previous
// update, drops the updates of unmounted components, and marks the
// components whose state changed (not == for comparable types) for
// building, which requests a frame. Post queues a function on the same
// queue of the Tree that receives posts (ReceivePosts; Run's tree);
// until one does, posted functions wait in order.
//
// Builds: marking a component needsBuild marks its ancestors dirtyKids.
// The build phase descends from the root along those marks, so
// components build in tree order, a parent before its children, also
// through components that skip their build or primitives whose node did
// not change (React's childLanes); the queue only catches what the
// descent misses.
//
// Effects: a build records the effect slots whose dependencies changed
// (none: every build; compared element-wise with ==; a different count or
// a value that cannot be compared counts as changed) and queues them on
// the Tree when it ends, after those of the components built inside it,
// so children's effects run before their parents'. Frame runs the layout
// effects after layout and before paint; the passive ones wait for
// FlushEffects, which a Host calls after Present and Render after
// rasterizing. Each commit runs the cleanups of unmounted components
// (parents first), then the cleanups of the effects that run again, then
// the effects. Frame and Mount first flush passive effects that a
// previous frame left; Unmount runs every cleanup.
//
// Context: a Provider is a component Element without a function that
// holds its value and its subscribers; UseContext subscribes the reading
// component to the nearest Provider (fixed for its life, since Elements
// never move to another parent). A new value that is not == to the old
// one marks only the subscribers for building.
//
// Dev mode (Config.Dev, TELA_DEV=1) records each slot's hook kind and call
// site on the first build and panics with both call sites when a later
// build calls another kind or another number of hooks; it reports
// dependencies that cannot be compared, changing dependency counts, and
// ComponentProps or Memo nodes made from anonymous functions or method
// values, once per call site. Outside dev mode, a different hook count or
// slot type still panics, without call sites.
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
// approximations (grid as block, fixed as absolute). space-x-* and space-y-* are gaps: a flex container uses its
// gap, and a block container adds the gap to the end margin of every
// in-flow child but the last (Tailwind's margins). Between inline
// elements of a paragraph, space-x-* becomes an empty inline box.
//
// # Pipeline
//
// Tree.Frame runs Build, Style, box structure, Layout, the layout effects
// and Paint (ARCHITECTURE section 8) on what is dirty: components marked
// needsBuild (in tree order), primitives marked needsStyle, primitives
// whose structure or paragraph may have changed, and the layout engine's
// own dirty nodes (MarkDirty on changed layout styles, children lists and
// paragraphs). Paint emits one display list for the whole tree (layers
// and damage arrive in phase 4): CSS box painting (shadows, backgrounds,
// borders, outlines), effects (transforms, opacity, filters, backdrop
// filters), overflow clips and a simplified CSS stacking order, with box
// edges snapped to device pixels at the tree's scale (SetScale); the
// comment at the top of paint.go is the reference. A Frame with nothing
// dirty returns the previous list.
//
// A Host connects a Tree to a platform window: WakeEvent applies state
// updates, ConfigureEvent sets the viewport and the scale, FrameEvent
// runs Frame, rasterizes at the window scale into a reused framebuffer,
// presents and runs the passive effects.
// A tree requests a frame (Window.RequestFrame) only when something
// becomes dirty, so an idle window costs no CPU. Run opens a window with
// the native backend and quits when it is closed. Render does everything
// without a window and returns the display list and the image; Inspect
// returns the laid-out box tree as plain data.
//
// # Threads
//
// A Tree belongs to its UI thread: only the Setter and Dispatcher
// methods, Post (and SetWake) may be called from other goroutines.
// Updaters, reducers, posted functions, effects and Callback forwarders
// run on the UI thread. Builds of all Trees in the process are serialized
// by one mutex, because hooks find the Element being built through a
// package variable.
package core
