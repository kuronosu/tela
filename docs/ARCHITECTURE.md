# Tela Architecture

> Stable document: **what** Tela is and **how** it is built. Progress lives in
> [`ROADMAP.md`](ROADMAP.md). To change a decision, append an entry to
> [§21](#21-decision-log-adr) instead of rewriting the old one.

## 1. What Tela is

A desktop UI framework written **only in Go**:

- **No cgo, no webview, no native toolkits** (GTK, Qt, Cocoa widgets, Win32 controls). Tela draws every pixel, like Flutter or a browser.
- **Layout and styling as expressive as modern CSS**: the subset covered by Tailwind CSS.
- **JSX/React-like API**: function components, hooks, Tailwind-style classes.
- **Built so an agent (LLM) can create beautiful UIs** without blind trial and error.
- **Platforms**: Linux (Wayland, X11), Windows, macOS.

```go
import (
    "github.com/kuronosu/tela"
    . "github.com/kuronosu/tela/h"
)

func Counter() tela.Node {
    count, setCount := tela.UseState(0)

    return Div(Class("flex flex-col items-center gap-6 p-10 rounded-3xl "+
        "bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl"),
        H1(Class("text-5xl font-bold text-white tracking-tight"),
            Textf("Count: %d", count)),
        Button(Class("px-6 py-3 rounded-xl bg-white/15 text-white backdrop-blur "+
            "hover:bg-white/25 active:scale-95 transition"),
            OnClick(func(tela.MouseEvent) { setCount(count + 1) }),
            Text("Increment"),
        ),
    )
}

func main() {
    tela.Run(tela.App{Title: "Demo", Width: 800, Height: 600, Root: Counter})
}
```

### Non-goals

- Not a browser: no HTML, DOM, JavaScript, or full CSS cascade.
- No native look per OS; Tela has its own design system.
- No mobile/web in early versions (the platform layer does not preclude them).

## 2. Hard constraints

Contributions that break these are rejected.

1. **`CGO_ENABLED=0` always**, for every build, test and CI job.
2. **OS access only through:**
   - **Linux:** Wayland and X11 protocols directly over Unix sockets, plus syscalls.
   - **Windows:** system DLLs (`user32`, `gdi32`, `imm32`, `dwmapi`, `shcore`, `kernel32`) via `golang.org/x/sys/windows` / `syscall`.
   - **macOS:** Objective-C runtime and system frameworks (AppKit, QuartzCore, CoreGraphics) via `github.com/ebitengine/purego`.
3. **Dependencies:** pure Go only, permissive licenses (MIT, BSD, Apache-2.0, OFL for fonts). Approved list below; new ones need an ADR.
4. **No webview, no Chromium, no external layout engines.**

### Approved dependencies

| Module | Use |
|---|---|
| `golang.org/x/sys` | Unix and Windows syscalls |
| `golang.org/x/image` | Image decoders, font utilities |
| `github.com/go-text/typesetting` | Shaping (HarfBuzz port), bidi, segmentation, font discovery |
| `github.com/ebitengine/purego` | macOS backend only (and a future GPU backend) |
| `github.com/godbus/dbus/v5` | Linux accessibility (AT-SPI), when that phase arrives |
| `github.com/jezek/xgb` | X11 client (chosen in phase 1); from phase 4 a patched copy of its core, `xproto` and `bigreq` is vendored under `internal/` (ADR-025). XInput2 and XKB are hand-written on its connection |

The X11 client (own or `github.com/jezek/xgb`) is chosen in its phase without an ADR, as long as it is pure Go.

**Porting code** from projects in §20 is allowed when the license is compatible (MIT, BSD, Apache-2.0, Unlicense). Keep the original copyright notice in the ported file and list it in `THIRD_PARTY.md`.

## 3. Design principles

1. **Known vocabulary over new vocabulary.** Names copy React (`UseState`, `UseEffect`, `Key`, `OnClick`); classes copy Tailwind v4 verbatim. If an LLM would write it that way in React + Tailwind, it must work in Tela.
2. **Beautiful by default.** Tokens, typography, shadows and components come polished. The easy path is the beautiful path.
3. **Errors that teach.** Unknown classes, misused hooks and impossible layouts yield the cause plus a suggestion (`unknown class "rounded-xxl"; did you mean "rounded-2xl"?`).
4. **Deterministic and observable.** Same tree, same pixels on every platform (embedded fonts, own rasterizer). Everything renders without a window and can be inspected as text.
5. **0% CPU when idle.** No change, no frame.
6. **One-way layered dependencies.** Each layer is tested without the layers above it.
7. **Flutter inside, CSS semantics outside.** Layout protocol, trees and pipeline come from Flutter; what users see (flex, grid, classes) behaves like the browser.

## 4. Glossary

| Term | Meaning |
|---|---|
| **Node** | Immutable, cheap UI description returned by `Div(...)` or a component; recreated every build. ≈ Flutter *Widget*, React *ReactElement*. |
| **Element** | Persistent instance of a Node in the tree. Holds hook state, key, children; performs reconciliation. ≈ Flutter *Element*, React *Fiber*. |
| **Box** | Render object: `ComputedStyle`, layout geometry, knows how to paint itself. ≈ Flutter *RenderObject*. |
| **ComputedStyle** | Typed struct with all style properties resolved (no strings). |
| **DisplayList** | Immutable list of draw commands produced by the paint phase. |
| **Layer** | Subtree painted into its own DisplayList with a cached raster (repaint boundary). |
| **Frame** | One full pipeline pass, from events to present. |
| **Semantics** | Parallel tree of roles, labels, states, actions. Used by accessibility, tests and agents. |
| **Backend** | Platform layer implementation for one OS, or `headless`. |

## 5. Layers and packages

```
┌───────────────────────────────────────────────────────────────┐
│ cmd/tela        CLI: snapshot, inspect, lint, dev, add, mcp   │
│ telatest        Mount without window, simulate input, goldens │
├───────────────────────────────────────────────────────────────┤
│ kit             Styled components (source installed by        │
│                 `tela add`, shadcn model)                     │
├───────────────────────────────────────────────────────────────┤
│ aria            Unstyled behavior hooks (React Aria model) +  │
│                 floating positioning (Floating UI model)      │
├───────────────────────────────────────────────────────────────┤
│ tela   +   h    Public API: Run, App, Node, hooks │ elements  │
├───────────────────────────────────────────────────────────────┤
│ internal/core   Element tree, reconciler, hooks, scheduler,   │
│                 Box tree, event dispatch, focus               │
├──────────────┬───────────────┬───────────────┬────────────────┤
│ internal/    │ internal/     │ internal/     │ internal/      │
│ gesture      │ anim          │ semantics     │ layout         │
├──────────────┴───────────────┴───────────────┴────────────────┤
│ internal/style   ComputedStyle, Tailwind parser, theme/tokens │
├───────────────────────────────────────────────────────────────┤
│ internal/text    Fonts, shaping, paragraphs, glyph cache      │
├───────────────────────────────────────────────────────────────┤
│ internal/gfx     Geometry, color, paths, DisplayList          │
│ internal/gfx/raster   CPU rasterizer, tiles, layers           │
├───────────────────────────────────────────────────────────────┤
│ internal/platform  Backend interface + events                 │
│   ├ wayland   ├ x11   ├ win32   ├ cocoa   └ headless          │
└───────────────────────────────────────────────────────────────┘
```

### Dependency rules

A package may only import lower layers. Order, bottom to top:

```
platform → gfx → text → style → layout → {anim, gesture, semantics} → core → tela/h → aria → kit → telatest → cmd/tela
```

- `platform` does not know `gfx`; it receives an `*image.RGBA` framebuffer and damage rects.
- `layout` does not know `core`; it works on a layout-node interface that `core` implements.
- **Public API:** `tela`, `h`, `aria`, `kit`, `telatest`, `cmd/tela`.
- **Everything else lives in `internal/`** until its API stabilizes. Making a package public needs an ADR. The clearest candidate is `gfx`, for custom painting.

### Repository layout

```
tela/
├── tela.go, app.go, hooks.go, node.go, events.go   # package tela
├── h/                    # elements and props (designed for dot-import)
├── aria/                 # behavior hooks, floating positioning
├── kit/                  # styled components (also the `tela add` registry)
├── telatest/
├── internal/
│   ├── platform/{wayland,x11,win32,cocoa,headless}/
│   │   └── wayland/protocols/   # generated from protocol XML (§15)
│   ├── gfx/  gfx/raster/
│   ├── text/  style/  layout/  anim/  gesture/  semantics/  core/
│   ├── style/gen/        # go:generate from Tailwind v4 theme (§10)
│   └── assets/fonts/     # embedded fonts (go:embed)
├── cmd/tela/
├── examples/             # full screens: dashboard, login, settings, chat…
└── docs/
```

## 6. The three trees

```
 Component (func) ──build──▶ Node (ephemeral)
                                │ reconcile by type + key
                                ▼
                             Element (persistent: hooks, key, children)
                                │ creates / updates
                                ▼
                             Box (ComputedStyle, layout, paint)
                                │ parallel
                                ▼
                             Semantics node
```

- **Reconciliation:** a new child with the same type and key as the old one at that position reuses the Element (and its state); otherwise unmount and mount anew. Lists match by key, falling back to position. Dev mode warns on multi-child lists without keys.
- **Primitives vs components:** primitives (`Div`, `Text`, `Img`…) create a Box; components are functions that only produce other Nodes.
- **Ownership:** an Element owns its Box. The Box tree may contain anonymous Boxes with no Element (scroll layers, portals).

## 7. Component and state model

- **A component is a function**: `func() tela.Node` or `func(P) tela.Node`.
- **Component boundary:** hook state belongs to the component Element being built.
  - `tela.C(Counter)` and `tela.CP(Card, CardProps{...})` create their own boundary with their own state.
  - Calling `Counter()` directly inside another component acts as a *custom hook*: its hooks belong to the parent. This works while call order is stable; dev mode detects hook-order changes between builds and suggests `tela.C`.
- **Automatic skipping (from Jetpack Compose):** `tela.CP[P comparable]` compares new props to old with `==` and skips the build if equal and no state/context it read changed. Free memoization. Non-comparable props always rebuild; `tela.Memo` covers custom equality.
- **Hooks:** `UseState`, `UseReducer`, `UseEffect`, `UseLayoutEffect`, `UseMemo`, `UseCallback`, `UseRef`, `UseContext`, `UseId`, `UseValue` (below), and `UseAnimation`/`UseSpring` (§12). Generic: `count, setCount := tela.UseState(0)`. Same rules as React: same order every build, never inside conditionals or loops.
- **Phase-deferred reads (from Compose):** `tela.UseValue[T]` returns an observable value. Read during build, it schedules a rebuild like state. Read inside a style closure evaluated in a later phase (`Style(tela.TranslateFn(func() (x, y float32) { return pos.Get() }))`), a change only invalidates that phase: layout or paint, no rebuild. This is how drags, scroll-linked effects and progress bars stay smooth.
- **Context:** `tela.CreateContext[T](def)`, `ctx.Provider(value, children...)`, `tela.UseContext(ctx)`. Only Elements that read a context rebuild when it changes (dependency tracking, like `InheritedWidget`).
- **Concurrency:**
  - The whole tree lives on the **UI thread**.
  - State setters and `UseValue.Set` are safe from any goroutine: they enqueue the update and request a frame.
  - `tela.Post(func())` runs arbitrary code on the UI thread.
- **Effects:** `UseEffect` runs after the frame is presented; `UseLayoutEffect` after layout, before paint. Cleanups run on unmount or before the effect re-runs.

## 8. Frame pipeline and scheduler

```
event/vsync
  │
  ├─1 Input       platform events → hit-test → gestures → handlers
  ├─2 Animate     tickers advance with the frame clock
  ├─3 Build       Elements marked needsBuild, depth order
  ├─4 Style       resolve ComputedStyle for dirty Boxes
  ├─5 Layout      Boxes with needsLayout (honoring relayout boundaries)
  ├─6 Paint       layers with needsPaint → one DisplayList per layer
  ├─7 Semantics   update semantics for dirty nodes
  ├─8 Raster      rasterize in parallel (goroutines per tile)
  └─9 Present     composite layers → framebuffer → backend.Present(damage)
       │
       └─ after present: UseEffect and pending cleanups
```

- **Dirty flags:** `needsBuild`, `needsStyle`, `needsLayout`, `needsPaint`, `needsSemantics`. Marking a node dirty requests a frame; each phase processes only marked nodes.
- **Scheduler:**
  - Nothing dirty and no active animations → no vsync request (0% idle CPU).
  - Frames align to the backend's vsync/frame callback: `wl_surface.frame`, `DwmFlush` or a timer on Windows, `CVDisplayLink` on macOS.
- **Threads:**
  - **UI thread:** the OS main thread, locked with `runtime.LockOSThread` in `tela.Run`. It runs the platform event loop, phases 1–7 and present, since Win32 and AppKit require thread affinity.
  - **Raster workers:** a goroutine pool (≈ `GOMAXPROCS`) for phase 8, working from immutable DisplayLists with no shared mutable state.
  - **Pipelining:** while frame *N* rasterizes, the UI thread may process input for frame *N+1*. DisplayLists are immutable, so no races.

## 9. Layout

### Internal protocol (Flutter style)

**Constraints go down, sizes go up, parent sets position.**

```go
type Constraints struct{ MinW, MaxW, MinH, MaxH float32 } // MaxW may be +Inf
// Box.Layout(c Constraints) Size  → parent then assigns Offset
```

- Single pass, O(n), in the common case.
- **Intrinsic sizes** (min-content, max-content) are computed separately, only when an algorithm needs them (e.g. flex-basis `auto`, grid `auto` tracks). Cached per `(Box, Constraints)` and invalidated by `needsLayout`.
- **Relayout boundary:** a Box with tight constraints (min == max), or with an explicit size and `overflow` other than visible, does not propagate relayout to its ancestors.

### Algorithms (CSS semantics outside)

**Taffy** (Rust) is the implementation reference; its test fixtures (derived from Web Platform Tests) are the conformance suite.

| Algorithm | Scope |
|---|---|
| **Flex** | Complete: direction, wrap, grow, shrink, basis, `gap`, `justify-*`, `align-*`, `order`, `margin: auto` |
| **Grid** | Tracks with `px`, `%`, `fr`, `auto`, `min-content`, `max-content`, `minmax()`, `repeat()` with `auto-fill`/`auto-fit`; explicit and auto placement; `span`; named areas; `gap` |
| **Block** | Simple vertical stacking, **no margin collapsing** |
| **Positioning** | `relative`, `absolute` (to nearest positioned ancestor), `fixed` (to window), `sticky` |
| **Overflow** | `visible`, `hidden`, `clip`, `scroll`, `auto`. A scroller is a Box with its own layer and smooth scrolling |

- **Box model:** always `box-sizing: border-box`.
- **Defaults:** `display` is `block`, like an HTML `div`. Inline elements (`Span`, `A`) only exist inside a text context (§11).
- **Virtualization (from TanStack Virtual / Flutter slivers):** `h.VirtualList(count, estimateSize, renderItem)` builds only visible items plus overscan. It supports variable item sizes measured on the fly, keeps scroll position stable when measurements correct estimates, and provides `ScrollToIndex`.

## 10. Style system

### Style sources and precedence

Resolved in this order; on conflict, **last wins**:

1. Primitive defaults (e.g. `Button` has no background but has `cursor-pointer`).
2. Classes in `Class("...")`, in order of appearance. `Class` may repeat; values concatenate.
3. Typed props in `Style(...)` for dynamic values: `Style(tela.Width(w), tela.Bg(c))`.

There is **no cascade, specificity or selectors**. Inheritance applies only to text properties: `color`, `font-*`, `line-height`, `letter-spacing`, `text-align`, `text-transform`, `white-space`, `text-decoration`.

### Classes (Tailwind v4 vocabulary)

- **Reference:** Tailwind CSS v4: palette, spacing, radius, shadow and type scales. Unambiguous v3 aliases are accepted (e.g. `bg-gradient-to-br` → `bg-linear-to-br`).
- **Variants:**
  - State: `hover:`, `active:`, `focus:`, `focus-visible:`, `focus-within:`, `disabled:`, `checked:`.
  - Structural: `first:`, `last:`, `odd:`, `even:`, `group-hover:`/`group-*` (ancestor has class `group`), `peer-*` (preceding siblings only).
  - Environment: `dark:`, `motion-reduce:`, breakpoints `sm: md: lg: xl: 2xl:` (window width).
- **Arbitrary values:** `w-[320px]`, `bg-[#1e1e2e]`, `grid-cols-[200px_1fr]`. **Color opacity:** `bg-white/15`.
- **Parsing:**
  - Each class string is parsed once and interned (cache: `string` → slice of ops on `ComputedStyle`).
  - Variants produce conditional styles evaluated against Element state (hover, focus…).
- **Unknown classes:** a visible error in dev mode (log and inspector) with the closest class by edit distance. `tela lint` catches them statically in literals.

### Theme and tokens

- The theme is a Go struct: colors, spacing, radii, shadows, fonts, breakpoints, easing curves.
- **Generated, not hand-copied:** `go generate` in `internal/style/gen` reads the Tailwind v4 theme source (CSS) and emits the Go default theme and the class vocabulary used for suggestions. OKLCH colors are converted to sRGB at generation time.
- Semantic classes (`bg-primary`, `text-muted`, `border-border`, shadcn style) resolve against the active theme, which can be swapped at runtime (e.g. light/dark).

### CSS subset

**Fully implemented:**
- **Layout:** flex, grid, positioning (including `sticky`), overflow and scrolling.
- **Sizing and spacing:** `width`, `height`, `min-*`/`max-*`, `aspect-ratio`, padding, margin (no collapsing), `gap`.
- **Backgrounds and borders:** colors; linear, radial and conic gradients; multiple background layers; background images (`cover`, `contain`, position); per-side borders; per-corner `border-radius`; `outline` with offset.
- **Effects:** multiple and `inset` `box-shadow`, `opacity`, `filter` (`blur`, `brightness`, `contrast`, `grayscale`, `saturate`, `drop-shadow`…), `backdrop-filter`, `mix-blend-mode` (common modes).
- **Motion:** 2D `transform` (`translate`, `scale`, `rotate`, `skew`, `origin`), `transition` on any animatable property.
- **Interaction:** `cursor`, `pointer-events`, `user-select`, `visibility`, `z-index` within parent.
- **Text:** see §11.

**Limited:**
- **`calc()`:** only `min()`, `max()`, `clamp()` and add/subtract of two units.
- **Keyframes:** via the Go API, plus predefined classes (`animate-spin`, `animate-pulse`, `animate-ping`, `animate-bounce`).
- **Inline:** rich text only (§11).
- **`z-index` / stacking contexts:** simplified model (§13).

**Not implemented (use instead):**

| Feature | Use instead |
|---|---|
| `float`, `clear` | flex or grid |
| Margin collapsing | `gap` (`space-y-*` is implemented as gap) |
| `display: table` | `kit` Table built on grid |
| Cascade, specificity, selectors, `!important`, `@layer` | Explicit styles and variants |
| `::before` / `::after` / `content` / counters | Regular nodes |
| CSS variables / `var()` | Theme tokens and arbitrary values |
| `box-sizing: content-box` | Nothing: always `border-box` |
| `@import`, `@supports`, vendor prefixes, quirks | Nothing |
| `@media print`, `@page` | Nothing |
| SVG `url()` filters and `clip-path` | Native filters and basic shapes (`inset`, `circle`, `ellipse`, `polygon`) |
| Units `cm`, `in`, `pt`, `pc`, `ex` | Supported units (below) |
| Vertical `writing-mode`, ruby, multi-column, `shape-outside` | Nothing |

**Supported units:** `px` (logical pixels, multiplied by display scale), `rem` (16px default, theme-configurable), `em`, `%`, `vw`, `vh`, `fr`, `ch` (approximate).

## 11. Text

- **Shaping, bidi, segmentation:** `go-text/typesetting`.
- **Text layout (modeled on Linebender's Parley):** the layer above shaping that `go-text` does not provide:
  - styled runs → shaped lines;
  - line breaking per UAX #14 (word wrap, `break-words`);
  - cursor positions, selection rectangles, hit-testing (point → character index);
  - word/line navigation.
- **Fonts:**
  - **Embedded** (`go:embed`, OFL): Inter as default sans, a monospace font, a color emoji font. Chosen in the text phase.
  - System fonts are available (discovery via `go-text` fontscan), but **snapshots and tests use embedded fonts only**, so they are deterministic across machines.
  - **Fallback:** a per-script family list, embedded fonts last.
- **Paragraphs:** a text node contains styled runs (`Text`, `Span`, `Strong`, `Em`, `Code`, `A`) and inline icons (atomic boxes on the baseline). No arbitrary `inline-block` with complex layout inside a paragraph.
- **Properties:** `font-family`, `font-size`, `font-weight` (incl. variable fonts), `font-style`, `line-height`, `letter-spacing`, `text-align`, `text-transform`, `text-decoration`, `white-space`, `text-overflow: ellipsis`, `line-clamp-*`, `tabular-nums`.
- **Glyph raster:**
  - Same path machinery as the rasterizer.
  - Glyph cache keyed by (font, size, quantized subpixel offset), respecting display scale. Under scale/translate transforms the pen x snaps to 1/4 device pixel and the baseline y to whole device pixels; other transforms and large glyphs fill the outline directly (ADR-029).
  - Grayscale antialiasing (no RGB subpixel) so output is identical across platforms.
- **Text editing (`Input`, `Textarea`):** cursor, mouse/keyboard selection, undo/redo, clipboard. **IME** composition arrives as `Composition{Start,Update,End}` events from the backend.

## 12. Animation

- **Clock:** a `Ticker` bound to the scheduler's frame clock; frames are requested while any ticker is active. With `motion-reduce` on (system or theme preference), transitions take 0 time.
- **Implicit transitions:** `transition`, `duration-*`, `ease-*` and `delay-*` interpolate animatable `ComputedStyle` properties on change (same mechanism as Flutter's `AnimatedContainer`).
- **Explicit animation:**
  - `UseAnimation`: controller with duration, curve, repeat/reverse.
  - `UseSpring`: interruptible spring physics.
  - Generic typed tweens.
  - All of these feed `UseValue` (§7), so they animate without rebuilds.
- **Enter/exit:** `Presence` keeps an Element mounted until its exit animation ends (like `AnimatePresence`).
- **Layout animations (from Motion):**
  - `AnimateLayout()` on a Box animates any change in its laid-out position/size (reorder, expand) from old box to new box, FLIP style, using transforms.
  - `LayoutID("x")` animates an element between two places in the tree (shared element transition).
  - Trivial for Tela because it owns layout and knows both boxes.
- **Performance:** animating `transform` and `opacity` only touches layer compositing. Animating layout properties is possible but costlier.

## 13. Graphics and rendering

### DisplayList

- The paint phase emits commands into one DisplayList per layer:
  - Shapes: `FillRect`, `FillRRect`, `FillPath`, `StrokePath`.
  - Content: `DrawGlyphRun`, `DrawImage`.
  - Shadows: `DrawShadow` (incl. inset).
  - State: `PushClip(RRect|Path)`/`PopClip`, `PushTransform`/`Pop`, `PushLayer(opacity, blend, filter)`/`PopLayer`, `BackdropFilter`.
- **Paints:** solid color or gradient (linear, radial, conic; with stops and an interpolation color space, Oklab by default like Tailwind v4; ADR-023).
- **Coordinates:** float32 in logical pixels; display scale is applied at raster time.

### CPU rasterizer (initial and reference backend)

- **Antialiasing:** analytic per-cell coverage (approach of font-rs, tiny-skia, vello_cpu sparse strips). Axis-aligned rects and rounded rects have fast paths.
- **Stroking:** path stroking (joins, caps, dashes) and curve flattening follow kurbo's algorithms; curves are flattened before offsetting (ADR-026).
- **Color:** premultiplied RGBA8 in sRGB. **Blending happens in sRGB space, like browsers**, so colors and gradients match Tailwind on the web.
- **Determinism:** float code in `gfx` and `raster` is fusion-safe (explicit rounding conversions; transcendental functions from `internal/gfx/internal/fmath`), so pixels are bit-identical across CPU architectures (ADR-027).
- **Parallelism:** tiles rasterized by parallel goroutines.
- **Shadows:**
  - Rounded-rect shadows (the common case, all Tailwind `shadow-*`) use **Evan Wallace's closed-form analytic Gaussian** (erf-based): no blur pass, nearly free.
  - Arbitrary-shape shadows (`drop-shadow`), `filter: blur` and `backdrop-blur` use a separable Gaussian approximation (three box-blur passes; an exact kernel for small radii). Results are not cached yet; layer caching (below) will cover them (ADR-028).
- **Layers and caching:**
  - A Box is a repaint boundary if it has animated `transform`/`opacity`, is a scroll container, is explicitly marked, or a dev heuristic suggests it (frequent repaints).
  - Each layer keeps its raster; unchanged layers are only composited.
- **Damage tracking:** the union of changed rects is computed; only that region is rasterized and presented. The backend receives the damage list (Wayland `wl_surface.damage_buffer` and equivalents).
- **z-index and stacking contexts:**
  - Paint in tree order; `z-*` reorders among siblings.
  - A Box with `z-*`, `opacity` < 1, `transform`, `filter` or `isolate` creates a stacking context.
  - Overlays (menus, tooltips, modals) mount into a **portal layer** above the root via `tela.Portal(...)`.

### GPU backend (future)

- Consumes the **same DisplayList**; nothing above `gfx` changes.
- **Fixed set of precompiled shaders** (rounded rect with border and analytic shadow as one primitive, like GPUI's quads; gradient; blur; glyph atlas; image; composite). No runtime shader generation (Impeller's lesson).
- Vulkan (Linux, Windows), Metal (macOS), optional D3D11 (Windows), via purego/syscall.
- The CPU rasterizer stays the reference for goldens and headless.

## 14. Input, events and focus

- **Events:**
  - Pointer: `MouseEvent`, `PointerEvent` (mouse, touchpad, pen).
  - Scroll: `WheelEvent` with precise deltas.
  - Keyboard: `KeyEvent` following winit's model: **physical key** (scancode-based position), **logical key** (layout-dependent meaning) and **text** (produced characters) as three separate fields, plus modifiers and repeat flag.
  - Text: `TextInputEvent`, `CompositionEvent`.
  - Focus: `FocusEvent`.
- **Propagation:**
  - Hit-testing over the Box tree honors clips, transforms and `pointer-events`.
  - Events bubble from target to root, with `e.StopPropagation()` and `e.PreventDefault()`.
  - No capture phase (can be added by ADR if needed).
- **Gestures:** a Flutter-style **gesture arena**: tap, double-tap, long-press, drag and pan recognizers compete for a pointer sequence; one wins. High-level handlers (`OnClick`, `OnDrag`…) are built on it.
- **Focus:** a focus tree with tab order following tree order plus `TabIndex`; focus traps for modals; `focus-visible` only activates on keyboard navigation.
- **Cursors:** declared by style (`cursor-*`), resolved by the backend.

### Behavior hooks (`aria` package, React Aria model)

Component *behavior* is separate from *styling*. `aria` provides unstyled hooks that implement interaction and accessibility correctly; `kit` adds the look.

- `UsePress`: a consistent "press" across mouse, touch and keyboard; cancels when the pointer leaves.
- `UseHover`: ignores touch-emulated hover.
- `UseFocusRing`, `UseFocusScope` (trap and restore focus).
- Collections: `UseListBox`, `UseMenu`, `UseSelect`, `UseTabs`, `UseCombobox`, with arrow-key navigation and typeahead.
- `UseDialog`, `UseTooltip`, `UseOverlay` (dismiss on outside click or Esc).
- **Floating positioning (port of Floating UI):** `UseFloating(anchor, options)` with `Offset`, `Flip`, `Shift`, `Size` and `Arrow` middleware. Every popover, tooltip, menu and select uses it.

## 15. Platform layer

### Interface

Defined in `internal/platform`; its `doc.go` holds the full contract. The backend calls the app (ADR-024):

```go
type Backend interface {
    Name() string
    NewWindow(WindowOptions) (Window, error) // shown at its first Present
    Run(Handler) error              // blocks on the main thread until Quit or connection failure
    Quit()                          // safe from any goroutine
    Wake()                          // safe from any goroutine; coalesces into one WakeEvent
    Now() time.Duration             // frame clock: time base of FrameEvent and input events
    Clipboard() Clipboard           // async ReadText, WriteText
    Settings() SystemSettings       // dark mode, reduced motion…; changes arrive as SettingsEvent
}

type Window interface {
    Metrics() Metrics               // logical size + scale from the last delivered ConfigureEvent
    RequestFrame()                  // coalesces; a FrameEvent follows at vsync / frame callback
    Present(fb *image.RGBA, damage []image.Rectangle) error // physical px; copied before return
    SetTitle(string); SetCursor(Cursor); SetSize(Size)
    Close()
}

type Handler interface{ HandleEvent(w Window, e Event) } // never called re-entrantly
```

- **Events** (sealed set): `ConfigureEvent` (size and/or scale; implies a frame), `FrameEvent`, `CloseRequestedEvent`, `FocusEvent`, `WakeEvent`, `SettingsEvent`, `PointerEvent` (DOM-style `Action`: move, enter, leave, down, up, cancel) and `ScrollEvent` (precise `Delta` plus discrete `Steps`). Keyboard and IME events arrive in their phases.
- **Sizing rule:** `PhysicalSize(logical, scale)` rounds logical × scale half away from zero (the `wp_fractional_scale_v1` rule). Every backend sizes framebuffers with it.
- `Run` does not return when the last window closes; quitting is app policy (`core`/`tela`).
- The IME cursor area (`SetIMEArea`) is added with the IME work (phase 5).

### Backends

| Backend | Transport | Notes |
|---|---|---|
| `wayland` (primary on Linux) | Protocol over `$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY` | `xdg_shell`, `wl_shm` (memfd buffers), `wp_viewporter` + `wp_fractional_scale_v1`, `xdg-decoration` (if absent, Tela draws its own decorations), `wp_cursor_shape_v1`, `wl_data_device` (clipboard), `zwp_text_input_v3` (IME). **Protocol bindings are generated from the official XML** (a Go `wayland-scanner`, see `rajveermalviya/go-wayland`), so adding a protocol is trivial. **Risk:** the keyboard sends an XKB keymap, so a **minimal pure-Go XKB parser** is needed (reference: `kbvm`, a pure-Rust XKB implementation) |
| `x11` (Linux fallback) | X11 protocol over socket, via `jezek/xgb` | `PutImage`/MIT-SHM, XInput2 for smooth scrolling, XKB or core keyboard mapping, `_NET_WM_*`, selections for clipboard. IME (XIM) comes later |
| `win32` | `x/sys/windows` + `syscall.NewCallback` | `CreateWindowExW`, `GetMessageW` loop, `SetDIBitsToDevice`/`StretchDIBits`, per-monitor DPI v2, IMM32, Win32 clipboard, `DwmFlush` for pacing. Reference: Ebitengine's no-cgo Windows code |
| `cocoa` | `purego` + ObjC runtime | `NSApplication`, `NSWindow`, layer-backed `NSView`, present via `CALayer.contents` (`CGImage`), `NSTextInputClient` (IME), `CVDisplayLink`. Main thread only. Reference: Ebitengine's purego macOS code |
| `headless` | Memory | Tests, snapshots, CLI. Configurable size and scale, controllable clock |

- **Linux backend selection:** `WAYLAND_DISPLAY` set → Wayland; else `DISPLAY` → X11. Override with `TELA_BACKEND=wayland|x11|headless`.
- **HiDPI:** everything works in logical pixels. The framebuffer is allocated in physical pixels (`logical × scale`); fractional scaling is supported.
- **Platform quirks:** before implementing a backend feature (DPI, IME, keyboard, decorations), check how winit handles it; its code and issues document most OS quirks.

## 16. Semantics, accessibility and agents

- Any Box may contribute a semantics node. Primitives have implicit roles (`Button` → `button`, `Input` → `textbox`, `H1` → `heading` level 1), overridable with `Role(...)`, `AriaLabel(...)`, etc.
- **A semantics node holds:** role, label, value, description; states (focused, disabled, checked, expanded, selected); actions (click, focus, setValue, scroll); bounds in window coordinates.
- **Consumers:**
  1. **OS accessibility** (late phase): AT-SPI over D-Bus (Linux), UI Automation over COM (Windows), NSAccessibility (macOS).
  2. **`telatest`:** queries like `ByRole("button", "Save")`, `ByText("Hello")`.
  3. **Agents:** a Playwright-style **aria snapshot**, a compact YAML LLMs already know (below), and an MCP server (§17).

```yaml
- heading "Settings" [level=1]
- textbox "Email": ana@example.com
- checkbox "Notifications" [checked]
- button "Save"
```

## 17. Tooling

### CLI `cmd/tela`

| Command | Does |
|---|---|
| `tela snapshot <pkg> [--size WxH] [--scale 2] [--dark] -o out.png` | Headless render to PNG |
| `tela inspect <pkg> [--aria\|--layout]` | Aria snapshot (YAML, default) or layout tree as JSON (geometry and computed style) |
| `tela lint <paths>` | Unknown classes in literals, hook misuse |
| `tela dev <pkg>` | Watch, rebuild, restart, restore serialized state when possible. Not true hot reload; Go can't do it |
| `tela add <component>...` | Copy `kit` component source into the user's project (shadcn model), rewriting the package name |
| `tela mcp <pkg>` | MCP server exposing `snapshot` (aria YAML), `screenshot` (PNG), `click`, `type`, `press`, `scroll`, `resize` over a running headless or windowed instance |

### Contract for `snapshot`, `inspect`, `mcp`

The package exposes `func Preview() tela.Node`, or a `tela.Previews` variable with several named screens. The CLI generates a temporary `main` that mounts it on the chosen backend.

### `telatest`

```go
func TestCounter(t *testing.T) {
    app := telatest.Mount(t, tela.C(Counter), telatest.Size(400, 300))
    app.Click(telatest.ByRole("button", "Increment"))
    app.ExpectText("Count: 1")
    app.Golden("counter.png") // compare with testdata/counter.png within tolerance
}
```

- Regenerate goldens with `TELA_UPDATE=1 go test ./...` (ADR-022). On failure, `*_actual.png` and `*_diff.png` are written.
- Controllable clock (`app.Advance(300*time.Millisecond)`) for animations.
- **Auto-wait (from Playwright):** actions and expectations wait for pending frames and running animations to settle (with a timeout) before acting or asserting.
- `app.AriaSnapshot()` returns the same YAML as `tela inspect` for assertion.

### Debug overlays (from Chrome DevTools / Flutter DevTools)

`TELA_DEBUG=layout,paint-flash,layers,fps,semantics`, drawn by Tela's own renderer:

| Flag | Shows |
|---|---|
| `layout` | Box model: margin, border and padding colored |
| `paint-flash` | Flashes repainted regions |
| `layers` | Layer borders |
| `fps` | Frame timing |
| `semantics` | Semantics bounds and labels |

### For agents (LLMs)

This is also a design constraint:
- `docs/llms.txt`: compact API guide with examples.
- `examples/`: full, beautiful screens to imitate.
- Errors with suggestions (§3).
- `kit` components as editable source (`tela add`).
- **Agent loop:** write UI → `tela snapshot` → look at the PNG → `tela inspect` if something is off → fix. Or drive the same loop interactively through `tela mcp`.

## 18. Quality and CI

- **Build matrix** with `CGO_ENABLED=0`: `linux/amd64`, `linux/arm64`, `windows/amd64`, `windows/arm64`, `darwin/amd64`, `darwin/arm64`.
- **Checks:** `go vet`, `staticcheck`, tests and goldens (headless, on Linux).
- **Windows:**
  - Locally: `GOOS=windows go test -exec wine ./...`, `wine` to run examples (`xvfb-run` in CI).
  - Wine is not real Windows (DPI, DWM, IME and accessibility differ), so there is periodic validation on `windows-latest` runners.
- **macOS:** `macos-latest` runners, with `cocoa` backend snapshots to validate presentation.
- **Benchmarks:** typical-scene raster (dashboard, long scrolling list, heavy blur), 10k-node layout, paragraph shaping. Regressions are tracked.

## 19. Conventions

- **Language:** everything in English: code, identifiers, comments, commits and docs.
- **Public API:**
  - React/Tailwind names when an equivalent exists.
  - Functional options for configuration.
  - No `interface{}` in public API where a closed type works.
  - Children and props share the sealed interface `tela.Child`, which gives clear compile errors.
- **Package `h`:** designed for dot-import (`. "github.com/kuronosu/tela/h"`). It holds only elements, props and helpers (`If`, `Map`, `Fragment`, `VirtualList`) to minimize collisions.
- **Errors:** programmer errors detectable in dev mode panic with an actionable message; platform/resource errors are returned as `error`.
- **Tests:** each `internal/` package is tested without upper layers. Layout is checked against fixtures ported from Taffy.
- **Third-party code:** see §2 and §20.

## 20. References by layer

Implementation references, not dependencies. Read the relevant entry before starting the matching work.

| Layer | Reference | What to take |
|---|---|---|
| Architecture | **Flutter** | Three trees, constraints protocol, relayout/repaint boundaries, phase pipeline, gesture arena, semantics tree, goldens |
| Architecture | **Blitz** (Dioxus) | Near-identical stack (Taffy + Parley + Vello) mapping CSS to layout/render; see where it got hard |
| State | **React** | Hooks, reconciliation with keys, context |
| State | **Jetpack Compose** | Skipping on equal props, phase-deferred state reads (§7) |
| Raster | **tiny-skia** (BSD) | Compact CPU port of Skia: AA fill, gradients, blending, pipelines |
| Raster | **vello_cpu** (MIT/Apache) | Sparse-strip rasterization designed for parallelism |
| Geometry | **kurbo** (MIT/Apache) | Stroking (joins, caps, dashes), curve flattening, arc/ellipse math |
| Shadows | **Evan Wallace, "Fast Rounded Rectangle Shadows"** (madebyevan.com) | Closed-form erf shadow for rounded rects (§13) |
| Rendering | **GPUI** (Zed) | Tailwind-like styling outside the web; quad primitive (bg + border + radius + shadow); text layout caching |
| Text | **Parley** (MIT/Apache) | Rich text layout, cursor, selection, hit-testing, navigation (§11) |
| Layout | **Taffy** (MIT) | Flex, grid and block algorithms + WPT-derived fixtures |
| Layout | **TanStack Virtual**, Flutter slivers | List virtualization with measured variable sizes (§9) |
| Platform | **winit** (Apache) | Keyboard event model, DPI/IME/decoration quirks per OS (§14, §15) |
| Platform | **Ebitengine** (Apache) | Working no-cgo Win32 (syscall) and Cocoa (purego) code |
| Platform | **go-wayland** (`rajveermalviya/go-wayland`, BSD) | Pure-Go Wayland client and XML code generator |
| Platform | **kbvm** | Pure (non-libxkbcommon) XKB keymap implementation |
| Platform | **Gio** (MIT/Unlicense) | Go Wayland/X11 handling, IME and text editor design |
| Components | **React Aria** (Apache) | Behavior hooks, press/hover/focus semantics, collections (§14) |
| Components | **Floating UI** (MIT) | Popover positioning middleware (§14) |
| Components | **shadcn/ui** | Components as copied source (`tela add`), semantic tokens, look |
| Animation | **Motion** (formerly framer-motion) | Layout animations (FLIP), `layoutId`, `AnimatePresence`, springs (§12) |
| Styling | **Tailwind CSS v4 source** | Theme and class vocabulary, generated into Go (§10) |
| Tooling | **Playwright** | Role locators, auto-wait, aria snapshot YAML, MCP agent loop (§16, §17) |
| Tooling | **Chrome / Flutter DevTools** | Debug overlays (§17) |

**Deliberately not used:** SwiftUI (its layout model is Flutter's, already covered), Qt/QML and WPF/XAML (heavy property/binding systems), Elm/Redux (app-level concern; `UseReducer` + context suffice), pure signals (Svelte/Solid; §7 deferred reads give the benefit without changing the model), Lottie/Rive (external format; separate package if ever).

## 21. Decision log (ADR)

Append-only: entries are never deleted. A changed decision gets a new entry ("Supersedes ADR-00X").

| # | Date | Decision | Rationale |
|---|---|---|---|
| 001 | 2026-10-11 | Pure Go, `CGO_ENABLED=0`, no webview or native toolkits | Trivial cross-compilation, single binary, full control of rendering |
| 002 | 2026-10-11 | CPU rendering as initial and reference backend; GPU later on the same DisplayList | 100% Go, deterministic, testable; free headless for goldens and agents |
| 003 | 2026-10-11 | React-like API (function components, hooks) and Tailwind v4 vocabulary for styling | What LLMs and most devs already write |
| 004 | 2026-10-11 | Internally three trees (Node, Element, Box) and a phased pipeline with dirty flags, like Flutter | Cheap rebuilds, 0% idle CPU, bounded relayout/repaint |
| 005 | 2026-10-11 | Constraint-based layout protocol; flex, grid, block with CSS semantics, Taffy as reference | Single pass in common case, verifiable conformance |
| 006 | 2026-10-11 | CSS subset of §10: no cascade, selectors, float, tables or margin collapsing | Complexity without benefit for component-based apps |
| 007 | 2026-10-11 | Color blending in sRGB, like browsers | Colors match Tailwind on the web |
| 008 | 2026-10-11 | Embedded fonts for tests and snapshots; system fonts optional at runtime | Deterministic pixels across machines and OSes |
| 009 | 2026-10-11 | Wayland primary and X11 fallback on Linux; Win32 via syscall; macOS via purego | The only no-cgo paths on each OS |
| 010 | 2026-10-11 | Semantics tree from the start, exportable | Shared base for accessibility, tests and agents |
| 011 | 2026-10-11 | Everything in `internal/` except `tela`, `h`, `kit`, `telatest`, `cmd/tela` | Freedom to iterate without breaking users |
| 012 | 2026-10-11 | All project docs in English (previously Spanish) | Fewer tokens for agents, wider audience |
| 013 | 2026-10-11 | Public `aria` package of unstyled behavior hooks + floating positioning (React Aria / Floating UI model); amends 011 | Correct interaction and a11y once, reused by `kit` and user components |
| 014 | 2026-10-11 | `kit` components distributed as source via `tela add` (shadcn model), also importable | Users and agents can read and edit component code |
| 015 | 2026-10-11 | Automatic build skipping for comparable props, and phase-deferred reads via `UseValue` (Compose model) | Free memoization; smooth drags and animations without rebuilds |
| 016 | 2026-10-11 | Layout animations (`AnimateLayout`, `LayoutID`) as a core feature | High visual impact; cheap because Tela owns layout |
| 017 | 2026-10-11 | Analytic erf shadows for rounded rects; blur passes only for other shapes and filters | Tailwind shadows everywhere become nearly free on CPU |
| 018 | 2026-10-11 | Default theme and class vocabulary generated from Tailwind v4 source via `go generate` | Exact values, easy updates, suggestion vocabulary for free |
| 019 | 2026-10-11 | Wayland protocol bindings generated from official XML | Correctness and trivial addition of protocols |
| 020 | 2026-10-11 | Agent interfaces: Playwright-style aria snapshot YAML and a `tela mcp` server | Format LLMs already know; closed build-see-fix loop |
| 021 | 2026-10-11 | Keyboard events split into physical key, logical key and text (winit model) | Avoids years of known keyboard bugs |
| 022 | 2026-10-11 | Goldens are regenerated with `TELA_UPDATE=1 go test ./...`, not an `-update` flag | `go test ./... -update` fails in every test package that does not define the flag |
| 023 | 2026-10-11 | Each gradient carries its interpolation color space (sRGB, Oklab; Oklch later), Oklab by default. Compositing stays in sRGB (ADR-007) | Tailwind v4 gradients interpolate `in oklab`; matching it keeps gradients identical to the web |
| 024 | 2026-10-11 | Callback-driven platform loop: `Backend.Run(Handler)` delivers events on the UI thread instead of the app draining `Window.Events()`; interface fixed in §15 | Cocoa owns the main loop and Win32 modal move/resize loops only run `WndProc`; callbacks are the one model that works on all four OSes (winit's `ApplicationHandler`) |
| 025 | 2026-10-11 | In phase 4, vendor a patched copy of `jezek/xgb` (core, `xproto`, `bigreq`; BSD-3, listed in `THIRD_PARTY.md`) whose reader handles GenericEvents | xgb v1.3.1 reads every event as 32 bytes, so XInput2 and Present GenericEvents would desync the connection; the patch is about 10 lines, far cheaper than our own X11 client. If more xgb limits appear, an own client generated from xcb-proto XML (like ADR-019) replaces it |
| 026 | 2026-10-11 | Stroking flattens curves and offsets the polyline (true-tangent ends, round joins between pieces) instead of kurbo's cubic offset fitting; joins, caps and dashes follow kurbo | The CPU rasterizer fills lines anyway; simpler and provably within tolerance, checked against geometric oracles |
| 027 | 2026-10-11 | Floating-point code in `gfx` and `raster` is fusion-safe: products are rounded explicitly and transcendental functions come from `internal/gfx/internal/fmath` (ports of Go's `math`); arm64 under qemu is the check | Go may fuse multiply-add on arm64 and other architectures, which changes pixels; principle 4 requires the same pixels everywhere |
| 028 | 2026-10-11 | Shadows and filters follow CSS: `DrawShadow` uses σ = blur/2 and the spread radius rule, and always clips with the box's coverage; the content of a filter layer is kept up to the filters' reach beyond the clip active at `PushLayer`, which cuts the filtered result; device σ is capped at 256 px and drop-shadow offsets at 512 px; blur results are not cached until layer caching arrives | Matches Chromium within ±3 levels away from anti-aliased edges on the Tailwind presets; an ancestor clip cuts a filter's output in CSS, not its input; the caps bound surface sizes. Blink skips the clip under an opaque background, so edge pixels can differ (up to 29 levels) until paint passes that fact on |
| 029 | 2026-10-11 | Glyph raster: `gfx` defines a `Font` interface (em-space outlines, bounds, COLR glyphs as DisplayLists) that `text` implements; raster caches A8 masks and color sprites keyed by font instance, device em size and pen x quantized to 1/4 device px, with the baseline snapped to whole device px. Rotation, skew, flips and device em sizes above 256 px fill outlines uncached and unsnapped. Under a gradient paint, color glyphs get a transparent foreground; color glyphs nest at most 3 levels | `gfx` stays independent of go-text; quarter-pixel x positioning is what Skia and browsers use, whole-pixel baselines keep text crisp; masks are bit-identical to filling the snapped outline, so cache state never changes pixels. Text whose transform switches between the two paths (an animated rotation) can jump by up to 1/8 px in x and 1/2 px in y at the switch |
| 030 | 2026-10-11 | `internal/text` loads TrueType outlines (glyf/gvar) and evaluates item variation stores (HVAR, MVAR, COLR) itself, ported from go-text with fusion-safe float64 arithmetic; go-text stays the parser and the shaper. CFF2 outlines and shaper advances still come from go-text | go-text accumulates variation deltas in float32, which Go fuses into multiply-adds on arm64 and amd64 v3: variable-font outlines differed across architectures, breaking ADR-027 |
| 031 | 2026-10-11 | Font selection: CSS Fonts 4 matching (stretch, style, weight; a variable font covers its axis range); fallback per grapheme cluster: requested families, then the per-script list, then user-registered families, then embedded fonts; emoji presentation (VS16, Emoji_Presentation, ZWJ and modifier sequences) prefers the emoji font, VS15 avoids color fonts (Unicode 17 emoji properties vendored). `opsz` follows the font size rounded to whole CSS px. CJK is not embedded: system fonts at runtime, a subset font for goldens | Browser behavior an LLM expects; per-cluster resolution keeps emoji sequences and combining marks in one face; whole-px optical size bounds the number of instances; a CJK font would add 16+ MB per weight |
| 032 | 2026-10-11 | COLRv1 glyphs are converted to DisplayLists. Until `raster` has blend modes (phase 6), blend composites are approximated with source-over (`exactBlend` switch in `internal/text`); `SrcIn` with non-solid masks clips to the mask's outlines; two-point (focal) radial gradients are emulated with banded circles | Noto Color Emoji uses only SrcIn (296 glyphs) and SoftLight (262 flags) composites and 17 focal gradients; the approximations stay within mean 4 levels of Chromium. A focal radial gradient and a mask layer in `gfx` would make them exact |
| 033 | 2026-10-12 | Pin `go-text/typesetting` to commit e49e2bd (pseudo-version after v0.3.5) until a release contains it; move to the next tagged release when one exists | v0.3.5's HarfBuzz set digest skips covered lookup subtables, so Inter's class kerning and Noto Sans Arabic's `ccmp` never applied (Arabic did not join); the commit also gives exact bidi embedding levels. Same approved module, no new code paths |
| 034 | 2026-10-12 | Paragraph layout follows Parley: shape once per paragraph, break lines many times without reshaping (line ends keep their kerning); line boxes are quantized to logical px (ascent, descent, half-leading rounded down, like Blink at 1x); where CSS and convenience differ, CSS wins (the ellipsis uses the block's style, tab stops the block's font, no letter-spacing in cursive scripts); line breaking is UAX #14 without Blink's ASCII pair table | Cheap re-breaking is what flex/grid measuring needs; matches Chromium within 0.052 px on 34 of 38 reference paragraphs, the rest differ by at most 0.4 px at line ends or in URL break points |
| 035 | 2026-10-12 | System fonts are opt-in per `Collection` (`EnableSystemFonts`), refused under `go test` unless `TELA_SYSTEM_FONTS=1`, and come after the embedded fonts in the fallback order (extends ADR-031). Discovery uses go-text `fontscan` lazily (first unknown family or uncovered cluster); only the matched face is parsed, through the same code as embedded fonts. Fallback ranks the families supporting a script once per script (sans designs first, then the `Lang` region for CJK, then fewest characters, then name) so one script draws in one family | Embedded fonts keep default rendering identical on every machine; system fonts only fill gaps (CJK, Hebrew, Indic, Thai). Per-cluster ranking let narrow fonts take single clusters of a script |
| 036 | 2026-10-12 | Text editing model: a cursor is a source byte offset on a grapheme boundary plus an affinity, canonical (Downstream unless Upstream draws a different caret); one caret per cursor with Parley's bidi rule (no split caret); Chromium's rules for line ends (the position after a forced break or a collapsed wrap space starts the next line), Home/End and selection highlights (a space-width extension across line breaks); visual arrows step one cluster, logical movement is available; word movement uses UAX #29 with macOS/GTK conventions plus the Windows ctrl+right variant; carets in truncated or clamped text sit at the ellipsis | Byte offsets match Go strings; canonical affinity makes cursor equality mean same caret. Checked against Chromium 152: 1,216 carets and 144 selections identical, 9,743 of 9,838 clicks give the same offset and the rest are explained (bidi naming, Chromium's far-end carets, sub-pixel ties). Limitation: a box at a wrapped line's edge has one side without a cursor |
