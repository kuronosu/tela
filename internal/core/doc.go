// Package core is Tela's runtime (ARCHITECTURE sections 6 to 8): the three
// trees, reconciliation, hooks, the frame pipeline and its scheduler, and
// the hosts that run a tree in a window or headless. The public packages
// tela and h are thin wrappers over it.
//
// core imports gfx, gfx/raster, text, style, layout, anim, gesture, semantics and
// platform (with platform/auto for Run); nothing but tela and h imports
// core (ARCHITECTURE section 5).
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
// UseContext, UseId, UseValue, UseElementRef (focus.go), UseScroll and
// UseBounds (scroll.go, bounds.go); VirtualList keeps its state in a slot
// too. A function
// called directly from a
// component, not through Component, runs inside that component's build:
// its hooks are the component's (a custom hook).
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
// Values (value.go; ARCHITECTURE section 7, phase-deferred reads after
// Jetpack Compose): UseValue returns a *Value, an observable value whose
// Set and Update go through the same queue as the state updates. The
// Tree tracks the Values read while its UI thread evaluates an observer:
// a component's build, or the deferred props of a box when the layout
// style is mapped (layout phase: WidthFn, HeightFn) or when the box is
// painted (paint phase: TranslateFn, OpacityFn). Each evaluation replaces
// the observer's subscriptions with the Values it read, so a closure that
// stops reading a Value is no longer invalidated by it; unmounting the
// reader, detaching its box, a style without deferred props of that
// phase, or unmounting the Value's component ends them. A change applied
// on the UI thread (not == for comparable types) marks each reader in the
// phase it read in: a build (markBuild, also through components that skip
// their build), a relayout of the box (Tree.remapOne maps its layout
// style again and marks it dirty for layout; no build, no style), or a
// repaint of the box (markPaint; no build, style or layout). Reads
// elsewhere (handlers, effects, goroutines) subscribe nothing. Get from
// another goroutine is race-free (the value is an atomic pointer) and
// returns the value applied last; one that races with a tracked
// evaluation of the same Tree may add a subscription to it, which costs
// at most one extra rebuild, relayout or repaint and is dropped by the
// next evaluation. Reads of a Value of another Tree are not tracked.
// Latest returns the value with the queued Set and Update calls applied
// (each Value keeps its pending operations, queued together with the
// update, so both orders agree) and subscribes nothing.
//
// A third phase of deferred reads is the text of a TextFn (textfn.go): its
// function is evaluated when the box phase assembles the paragraph that
// holds it, with its reads tracked (obsText); a change marks the text's
// parent for the box phase, which assembles the paragraph again and lays
// it out when the text changed, without building or styling anything.
//
// Bounds (bounds.go, ADR-070): ElementRef.Bounds is the border box of an
// element in window coordinates, scrolled and transformed as painted (the
// semantics tree's bounds), and UseBounds a Value that the Tree updates at
// the end of every frame that ran the pipeline (afterFrame, outside Frame,
// like UseScroll's values) when it changed, so the readers it invalidates
// run in the next frame; a frame that moves nothing requests none.
// Render and telatest's headless entry points run those frames (at most
// MaxRenderFrames).
//
// Dev mode (Config.Dev, which callers resolve with ResolveDev: on unless
// the binary is a release build, TELA_DEV=1 or 0 overriding; ADR-085)
// records each slot's hook kind and call site on the first build and
// panics with both call sites when a later build calls another kind or
// another number of hooks; the same first build checks the hook's caller
// (boundary.go) and panics when it is neither the component's function
// nor a custom hook. Dev errors (unknown classes) go to Config.Log and
// dev warnings to Config.Warn: dependencies that cannot be compared,
// changing dependency counts, nil dependency lists, ComponentProps or
// Memo nodes made from anonymous functions or method values, lists
// without keys or with duplicate keys (per call site of Map and List),
// Value.Get with changes queued outside a tracked evaluation, contexts
// read without a Provider whose default is not safe, and Labels without a
// control, each once per call site or per subject. Outside dev mode, a
// different hook count or slot type still panics, without call sites.
//
// # Boxes and paragraphs
//
// The box phase (structure.go) derives the box tree from the Elements
// after styles are known: components are transparent, display: contents
// lifts its children, a block container whose content is all inline-level
// is one paragraph (one text.Layout), mixed content gets anonymous
// paragraph boxes, and text runs of flex containers become anonymous flex
// items (ADR-066, revising ADR-044):
//
//   - Inline elements (display: inline: Span, Strong, Em, Code, A...) are
//     CSS inline boxes of the paragraph: text.Inline entries with their
//     text style and their edges (margin, border and padding in the
//     inline axis), which take space on the lines and break shaping. The
//     layout splits each into fragments (one per line and per bidi run),
//     which inline.go paints with the element's backgrounds, borders,
//     radii and shadows (box-decoration-break: slice), hit-tests (their
//     padding is clickable) and reports (semantics bounds, tela inspect).
//   - Atomic inlines (inline-block, inline-flex and inline-grid
//     primitives, Button by default, Img with display inline or
//     inline-block, in flow) are inline boxes of the paragraph whose boxes
//     are the paragraph box's children: layout.Atoms sizes them
//     (fit-content in the paragraph's width) and the lines place them by
//     vertical-align, their baseline the last line box's (CSS 2.1 section
//     10.8.1). The paragraph paints them line by line, with its text and
//     the inline decorations; those that text-overflow or line-clamp hides
//     are neither painted nor hit (Box.hiddenAtom). In a flex container
//     every child is a flex item, atomic or not.
//
// A paragraph is shaped again (text.NewLayout) only when its text, a text
// style, an inline's edges or its paragraph style changes; a new width
// only breaks its lines again (text.Layout.BreakLines), and a width at or
// above its max-content width only realigns them. A paragraph with atomic
// inlines breaks its lines again on every layout request, since their
// sizes depend on the width; the sizes of its last full layout are put
// back before painting (paragraph.prepare).
//
// # Style and layout styles
//
// The style phase computes the ComputedStyle of each dirty primitive with
// style.Compute: Preflight defaults of its kind, its classes (compiled by
// the tree's parser, interned) and typed props, inheriting from its
// nearest primitive ancestor. When the inherited properties change, the
// children are restyled. Structural variants (first:, last:, only:, odd:,
// even:) are evaluated against the primitive siblings; the interaction
// variants (hover:, active:, focus:, focus-visible:, focus-within:,
// disabled:, enabled: and their group-* and peer-* forms) against the Elements' interaction state (input.go): a
// state change restyles only the elements whose classes test it. Unknown
// classes are reported once per class string to Config.Log (stderr in dev
// mode).
//
// mapLayoutStyle maps a ComputedStyle onto layout.Style; see it for the
// approximations (fixed as absolute). Flex and grid containers make every
// element child an item and wrap each run of text in an anonymous item;
// a grid's track lists become a layout.GridTemplate whose pointer is kept
// while the template is equal (keepGrid). space-x-* and space-y-*
// are Tailwind v4's margins, negative values included (-space-x-2): the
// end margin (inline-end for space-x, block-end for space-y) of every
// child element but the last, in block, flex and grid containers alike, unless
// the child has a margin of its own on that side (ADR-070). Between
// inline-level elements of a paragraph, a positive space-x-* becomes an
// empty inline box.
//
// # Input
//
// Tree.HandlePointer turns platform pointer events into DOM-style events
// (input.go, whose top comment is the reference): hit-testing over the
// painted box tree in reverse paint order (hit.go: clips, rounded
// corners, transforms including deferred ones, pointer-events, visibility,
// inline elements of paragraphs and their padding), bubbling with
// StopPropagation, enter and leave events, pointer and compatibility
// mouse events, implicit
// capture for touch and pens, and the gesture arena of internal/gesture
// (tap for click, double click by click count, drag, long press). After
// each discrete event the queued updates are applied and the build phase
// runs, so the next event's handlers are the new build's. Tree.Cursor is
// the cursor style under the mouse; Tree.NextDeadline and TickGestures
// drive the long-press timeout on the backend clock.
//
// Tree.HandleKey turns platform key events into keydown and keyup events
// (keyboard.go): they target the focused element and bubble, then run
// their default actions (the key bindings of a focused text field, Tab
// navigation, Enter and Space on buttons, scrolling keys).
//
// # Text fields and forms
//
// Input and Textarea (field.go, whose top comment is the reference;
// ADR-069) are leaf boxes (modeField) that own a field: an
// internal/text/edit Editor, the text.Layout of its displayed text,
// rebuilt only when the display or the style changes, and the
// placeholder's. They are sized like Chromium's (size=20, cols=20 and
// rows=2 from the primary font's average and maximum character widths,
// field-sizing-content), an Input centers its line and scrolls it
// horizontally itself, a Textarea is a scroll container whose extents
// come from its text. Values are uncontrolled (DefaultValue), controlled
// (FieldValue, Bind: restored after every event unless the app changed
// it, React's model) or bound to a Value (BindValue: written at once
// without a build). Keys (fieldkeys.go: per-platform bindings, typing,
// edits with OnBeforeInput and OnChange), the pointer (caret, word and
// paragraph selection, drag with autoscroll), the IME (fieldime.go:
// HandleIME and HandleIMESession apply platform.IMEEvent with the
// composition events; TextInput gives the Host the window's
// platform.TextInputState), the clipboard (SetClipboard; asynchronous
// paste) and the blinking caret (530 ms, stopped after 10 s idle, a
// deadline of NextDeadline) all end in edit operations. A field's content
// is a layer of its own (layerField, fieldpaint.go), so typing, moving
// the caret or blinking records one small layer and lays nothing out,
// except for field-sizing-content fields. Forms, labels and element IDs
// (form.go): every click runs the activation behavior of the Button or
// Label on its path (submit, reset, focusing a label's control), Enter in
// a single-line field submits its Form implicitly, and FillNode and
// FieldValue serve telatest.
//
// # Scrolling
//
// Boxes whose overflow is hidden, scroll or auto are scroll containers
// (scroll.go, whose top comment is the reference), and so is the root,
// the viewport (overflow auto unless RootClass sets one): their offsets live on
// the box, are clamped after every layout and move the painted content
// (and so hit-testing and local coordinates). Tree.HandleScroll turns
// platform scroll events into OnWheel events whose default action scrolls
// the nearest container on the target's containing-block chain that can
// move (chaining, overscroll containment, Chromium's latching); wheel
// notches, keys and scrollbar pages animate, touchpad and touch scrolling
// follow the fingers and fling with GTK 4's deceleration when they lift,
// and the instant settings (Config.InstantScroll, scroll-instant,
// reduced motion) turn animation and flings off. Programmatic scrolling
// goes through ElementRef (ScrollTo, ScrollBy, ScrollIntoView) and focus
// reveals the focused element; OnScroll fires once per frame per
// container and UseScroll's Values follow the offsets. Tela draws the
// scrollbars (scrollbar.go): overlay bars that fade, or classic ones in a
// reserved gutter (Config.ClassicScrollbars, scrollbar-classic).
//
// # Virtualization
//
// VirtualList (virtual.go, whose top comment is the reference;
// ARCHITECTURE section 9, after TanStack Virtual) is a component whose
// build returns a scroll container holding a content box as tall as the
// whole list and one wrapper per built item, keyed by index: the visible
// items, an overscan on each side and the focused item. Item sizes
// (estimates, replaced by the measured heights) live in a Fenwick tree,
// so offsets and lookups take O(log n). A scroll offset change checks the
// lists' windows (setScroll) and marks a list for building only when its
// visible items leave the built range, so most scroll frames only paint;
// the scroll phase measures the built items, moves the offset by the size
// changes above the viewport (stable position), keeps ScrollToIndex
// targets aligned, and builds and lays out again within the frame until
// the lists settle.
//
// # Positioning, portals and the environment
//
// Fixed boxes are laid out against the window, or against an ancestor
// with a transform, a filter or a backdrop filter, which contains
// absolute and fixed descendants as in CSS (layout.Style.Fixed and
// ContainingBlock, containingBlock in paint.go); they are painted outside
// every scroll space they escape, so a scroll does not move them. Sticky
// boxes (sticky.go, whose top comment is the reference) are laid out as
// relative ones without offsets and painted in a scroll space of their
// own whose offset is their sticky offset, recomputed on every paint, so
// a scroll moves them in the compositor without recording anything.
// Portal nodes (portal.go) put their content in a layer above the root's
// content, laid out against the window, while state, context and event
// bubbling follow the element tree. The color scheme and reduced motion
// (env.go) are the system's preferences with the app's override of the
// scheme; UseColorScheme and UseReducedMotion read them.
//
// The focus (focus.go) follows HTML: focusable elements (Button,
// TabIndex), the sequential order, focus by pointer press, ElementRef,
// AutoFocus, the fix-up of elements that stop being focusable, the
// window's focus (SetWindowFocused), the focus:, focus-visible: (Chromium's
// heuristic) and focus-within: states, and focus scopes reserved for
// package aria. Focus work queued by a build (AutoFocus, scope
// restoration, fix-up) runs in Frame after the style phase.
//
// # Animations
//
// Implicit CSS transitions and the animate-* classes run on the frame
// clock (anim.go, whose top comment is the reference; the pieces are in
// internal/anim). A restyle of a box whose transition-property covers a
// changed property starts a transition from the current value, with
// retargeting and reversing as in CSS Transitions Level 1; a box that
// mounts does not transition. Tree.Animate (phase 2) advances the running
// ones to the time of the frame and installs the animated style, with
// opacity and transform frames that only mark the box for painting (it is
// a layer of its own while it animates), color and shadow frames that
// record again without a restyle, and size, spacing and inherited
// property frames that restyle the box. A Tree whose frames carry no
// clock (Render) has no transitions. After each frame the Tree asks for
// another while anything animates, so an idle app asks for none;
// infinite animations keep asking while mounted (those inside display:
// none wait until shown), and Tree.OnlyEndlessAnimations lets a test
// runner not wait for them. Reduced motion (Config.ReducedMotion, the
// system setting) removes transitions; the motion-reduce: variants handle
// the animate-* classes. A box that starts being rendered (it mounts, or
// its display: none ends) has no before-change style and starts no
// transition, except from its starting style when its classes have
// starting: rules (CSS @starting-style).
//
// Explicit animations (valueanim.go) drive Values: UseSpring and UseTween
// follow a target that changes between builds, and Animate moves any
// Value from handlers, effects or goroutines. Each running one is an
// anim.Motion advanced by the same ticker, which stores the Value at
// every frame, so its readers are redone in the phase they read it in; a
// new target starts from the position and velocity the running motion
// has (or from a Velocity option), and OnDone is queued when a motion
// comes to rest at its target. BgFn and TextColorFn are paint-phase
// deferred colors; text inherits a TextColorFn when it is painted
// (colorfn.go). A Presence (presence.go) keeps a removed child mounted and inert
// while its Exit classes transition and its UsePresence components run
// their exits, then unmounts it.
//
// Layout animations (layoutanim.go) are found by the paint phase: a box
// with AnimateLayout or LayoutID whose laid-out box changed is drawn
// through a flip, a scale and translation from its new box to the visual
// box of the frame folded into its paint-phase transform, which goes to
// the identity on a spring or a tween. The same ticker marks the moving
// boxes for painting, so their frames only composite.
//
// # Pipeline
//
// Tree.Animate (phase 2, called with the frame clock before Frame)
// advances the transitions and keyframe animations (anim.go), the scroll
// animations, flings and scrollbar fades. Tree.Frame
// fires the pending OnScroll events and runs Build, Style, box structure,
// Layout, the layout effects, the scroll phase (offsets clamped, classic
// gutters, virtual lists measured and their windows moved, elements
// revealed) and Paint (ARCHITECTURE section 8) on what
// is dirty: components marked
// needsBuild (in tree order), primitives marked needsStyle, primitives
// whose structure or paragraph may have changed, boxes whose
// layout-phase deferred props read a Value that changed, and the layout
// engine's own dirty nodes (MarkDirty on changed layout styles, children
// lists and paragraphs). When the only work is repainting (Values read by
// paint-phase deferred props, scroll offsets, scrollbar fades and hovers:
// Tree.markPaint), Frame skips straight to Paint (a paint-only frame:
// Stats.Paints grows, nothing else).
//
// # Layers and damage
//
// Paint (layer.go, whose top comment is the reference) records the tree
// into layers, Flutter's repaint boundaries: the root; the content of
// each scroll container that can move (with the hoisted boxes that move
// with it), painted unscrolled and moved by the compositor; each stacking
// context with paint-phase deferred props (TranslateFn, OpacityFn), whose
// transform and opacity the compositor applies; and the scrollbars. A
// frame that builds, restyles or lays out places every box again and
// records every layer, each skipping the boxes outside its interest rect
// (the visible part of a scroll container grown by a scrollport per
// scrolling axis); a paint-only frame records only the scrollbars, unless
// a layer became stale (content that leaves its interest rect, a box that
// starts or stops being a layer). The compositor appends the layers into
// one display list for the frame (gfx.Builder.AppendList): CSS box
// painting (shadows, backgrounds with gradients and images from the
// registered assets, assets.go, borders, outlines), effects (transforms,
// opacity, filters, backdrop filters, blend modes, clip paths and
// background-clip: text, cssfx.go), overflow clips and a simplified CSS
// stacking order, with box edges snapped to device pixels
// at the tree's scale (SetScale); the comment at the top of paint.go is
// the reference. A Frame with nothing dirty returns the previous list.
//
// Tree.RasterizeFrame brings a framebuffer that holds the previous frame
// to the new one: it matches the commands of the two frames, shifts the
// pixels of scroll containers that moved by whole device pixels over a
// uniform background, and rasterizes only the damage (damage.go); the
// pixels are bit-identical to a full rasterization (ReferenceFrame and the
// property test in layer_test.go check it). Tree.Layers and
// Tree.LastDamage expose the layer tree and each frame's damage (debug
// overlays).
//
// The semantics tree (semantics.go, whose top comment is the reference;
// ARCHITECTURE section 16) is not a phase of Frame: Tree.Semantics builds
// it from the Element and Box trees when a consumer asks (tela inspect
// --aria, telatest, tela mcp) and keeps it while nothing changes, so it
// costs nothing per frame. Its roles and names follow HTML-AAM and AccName
// as Playwright computes them, Tree.AriaSnapshot serializes it as a
// Playwright aria snapshot, and ClickNode, FocusNode and
// ScrollNodeIntoView act on its nodes. The props Role, AriaLabel,
// AriaLabelledBy, the other Aria... props and Alt (aria.go) feed it.
//
// Debug overlays (debug.go, whose top comment is the reference;
// ARCHITECTURE section 17) are chosen with TELA_DEBUG or Config.Debug and
// drawn over the application: layout boxes, paint flashes, layer borders,
// a frame meter and semantics bounds. They are a display list of their
// own, rasterized over a copy of the application's pixels, so they never
// enter the layers, the damage, the layout, the hit tests or the semantics
// of the application, and with none on they cost nothing.
//
// A Host connects a Tree to a platform window: WakeEvent applies state
// updates, ConfigureEvent sets the viewport and the scale, FrameEvent
// runs Frame, brings its framebuffer to the frame (RasterizeFrame; fully
// after a new size or scale), presents it with the damage and runs the
// passive effects (Animate runs first);
// PointerEvent goes to HandlePointer, ScrollEvent to HandleScroll,
// KeyEvent to HandleKey, IMEEvent and IMESessionEvent to HandleIME and
// HandleIMESession, and FocusEvent to
// SetWindowFocused (a Host starts with an unfocused window), and the
// window's cursor follows Tree.Cursor (SetCursor only on a change). After
// every event and frame the Host gives the window the text input state of
// the focused text field (Tree.TextInput: nil first when the field
// changed, ResetTextInput when core finished a composition itself), and
// the backend's clipboard is the tree's. While
// a press waits for a long-press timeout, overlay scrollbars wait to
// fade or a caret waits to blink, the Host keeps one goroutine timer that
// calls Wake at the deadline (the platform layer has no timer API); idle,
// it has none.
// A tree requests a frame (Window.RequestFrame) only when something
// becomes dirty, so an idle window costs no CPU. Run opens a window with
// the native backend and quits when it is closed. Render does everything
// without a window and returns the display list and the image (a full
// rasterization, the reference); Inspect returns the laid-out box tree as
// plain data.
//
// # Threads
//
// A Tree belongs to its UI thread: only the Setter, Dispatcher and Value
// methods, Post (and SetWake) may be called from other goroutines.
// Updaters (of state and Values), reducers, posted functions, effects and
// Callback forwarders run on the UI thread. Builds of all Trees in the
// process are serialized by one mutex, because hooks find the Element
// being built through a package variable. Value reads are tracked per
// Tree, so Trees on different goroutines do not see each other's reads.
package core
