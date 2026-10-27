# Tela Architecture

> Stable document: **what** Tela is and **how** it is built. Progress lives in
> [`ROADMAP.md`](ROADMAP.md). To change a decision, add an ADR in
> [`docs/adr/`](adr/README.md) (§21) instead of rewriting the old one.

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

    return Div(Class("flex flex-col items-center gap-6 p-10 rounded-3xl",
        "bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl"),
        H1(Class("text-5xl font-bold text-white tracking-tight"),
            Textf("Count: %d", count)),
        Button(Class("px-6 py-3 rounded-xl bg-white/15 text-white backdrop-blur",
            "hover:bg-white/25 active:scale-95 transition"),
            OnClick(func() { setCount.Update(func(n int) int { return n + 1 }) }),
            Text("Increment"),
        ),
    )
}

func main() {
    tela.Main(tela.App{Title: "Demo", Width: 800, Height: 600, Root: tela.C(Counter)})
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
   - **Windows:** system DLLs (`user32`, `gdi32`, `imm32`, `dwmapi`, `shcore`, `kernel32`; `shell32`, `ole32`, `comdlg32` for desktop services, ADR-178) via `golang.org/x/sys/windows` / `syscall`.
   - **macOS:** Objective-C runtime and system frameworks (AppKit, QuartzCore, CoreGraphics) via `github.com/ebitengine/purego`.
3. **Dependencies:** pure Go only, permissive licenses (MIT, BSD, Apache-2.0, OFL for fonts). Approved list below; new ones need an ADR.
4. **No webview, no Chromium, no external layout engines.**

### Approved dependencies

| Module | Use |
|---|---|
| `golang.org/x/sys` | Unix and Windows syscalls |
| `golang.org/x/image` | Image decoders, font utilities |
| `github.com/go-text/typesetting` | Shaping (HarfBuzz port), bidi, segmentation, font discovery |
| `golang.org/x/text` | `i18n`: language tags and matching, CLDR plural rules, number formatting (ADR-087) |
| `github.com/ebitengine/purego` | macOS backend only (and a future GPU backend) |
| `github.com/godbus/dbus/v5` | Linux D-Bus: desktop services (portal, notifications, tray; ADR-178) and accessibility (AT-SPI) |
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
│ cmd/tela        CLI: snapshot, inspect, icons, lint, dev, add │
│ telatest        Mount without window, simulate input, goldens │
├───────────────────────────────────────────────────────────────┤
│ kit             Styled components (shadcn model; recipes for  │
│                 customization, ADR-129)                       │
├───────────────────────────────────────────────────────────────┤
│ i18n            Message catalogs (ICU), plurals, locale       │
│                 formatting, runtime locale switching          │
├───────────────────────────────────────────────────────────────┤
│ aria            Unstyled behavior hooks (React Aria model) +  │
│                 floating positioning (Floating UI model)      │
├───────────────────────────────────────────────────────────────┤
│ desktop         Dialogs, tray, notifications, app paths,      │
│                 settings store (ADR-178)                      │
├───────────────────────────────────────────────────────────────┤
│ tela   +   h    Public API: Run, App, Window, hooks │ elements│
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
│ internal/svg     SVG documents → DisplayList commands         │
├───────────────────────────────────────────────────────────────┤
│ internal/gfx     Geometry, color, paths, DisplayList          │
│ internal/gfx/raster   CPU rasterizer, tiles, layers           │
├───────────────────────────────────────────────────────────────┤
│ internal/platform  Backend interface + events                 │
│   ├ wayland ├ x11 ├ win32 ├ cocoa ├ headless └ remote (dev)   │
└───────────────────────────────────────────────────────────────┘
```

### Dependency rules

A package may only import lower layers. Order, bottom to top:

```
platform → gfx → svg → text → style → layout → {anim, gesture, semantics} → core → tela/h → desktop → i18n → aria → kit → telatest → cmd/tela
```

- `platform` does not know `gfx`; it receives an `*image.RGBA` framebuffer and damage rects.
- `layout` does not know `core`; it works on a layout-node interface that `core` implements.
- **Public API:** `tela`, `h`, `desktop`, `i18n`, `aria`, `kit`, `telatest`, `cmd/tela`.
- `internal/icongen` (the engine of `tela icons gen`) imports only `internal/svg` and is used only by `cmd/tela`. `internal/lint` (the engine of `tela lint`, ADR-173) imports `internal/style`, `internal/semantics` and `internal/core` for their validation functions, type-checks with the standard library, and is used only by `cmd/tela` and by the test that checks `docs/llms.txt` (`internal/gen/llmsdoc`, ADR-177). `icons/lucide` is a separate module (ADR-102): outside this list, it may import only the public packages (`tela`, `h`, `telatest`), and nothing in the core module imports it.
- **Everything else lives in `internal/`** until its API stabilizes. Making a package public needs an ADR. The clearest candidate is `gfx`, for custom painting.

### Repository layout

```
tela/
├── tela.go, app.go, hooks.go, node.go, events.go   # package tela
├── h/                    # elements and props (designed for dot-import)
├── desktop/              # OS services: dialogs, tray, notifications, paths, settings store (ADR-178)
├── i18n/                 # message catalogs, plurals, locale formatting (ADR-087)
│   └── internal/gen/     # go:generate: date and list patterns, week data from vendored CLDR JSON
├── aria/                 # behavior hooks, floating positioning
├── kit/                  # styled components (also the `tela add` registry)
├── telatest/
├── internal/
│   ├── platform/{wayland,x11,win32,cocoa,headless}/
│   │   └── wayland/protocols/   # generated from protocol XML (§15)
│   ├── gfx/  gfx/raster/  svg/
│   ├── icongen/          # SVG directory -> Go file of tela.Icon variables, behind `tela icons gen` (ADR-102)
│   ├── text/  style/  layout/  anim/  gesture/  semantics/  core/
│   ├── style/gen/        # go:generate from Tailwind v4 theme (§10)
│   └── assets/fonts/     # embedded fonts (go:embed)
├── cmd/tela/
├── icons/lucide/         # separate module (own go.mod): Lucide as tela.Icon variables (ADR-102)
├── examples/             # full screens: dashboard, login, settings, chat…
└── docs/
```

## 6. The three trees

```
 Component (Build / func) ──build──▶ Node (ephemeral)
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
- **Primitives vs components:** primitives (`Div`, `Text`, `Img`…) create a Box; components (struct values with a `Build` method, or functions) only produce other Nodes. A struct component's type is its type for reconciliation (ADR-088).
- **Ownership:** an Element owns its Box. The Box tree may contain anonymous Boxes with no Element (scroll layers, portals).
- **Windows:** each window (the main one, and each mounted `tela.Window`) has its own `core.Tree`: box tree, pipeline, input, focus, overlays and semantics. The Trees of an app form one element tree: a child window's root hangs below its `Window` element for context, and Value reads are tracked app-wide (ADR-180).

## 7. Component and state model

- **A component is a struct value with a `Build() tela.Node` method** (value receiver; `tela.Component`, whose fields are its props and whose children are a field of type `tela.Component`), **or a function**: `func() tela.Node` or `func(P) tela.Node` (ADR-088).
- **Component boundary:** hook state belongs to the component Element being built.
  - Every `Component` value given to an element, and `tela.C(Counter)` and `tela.CP(Card, CardProps{...})`, create their own boundary with their own state.
  - Calling `Counter()` or `Card{}.Build()` directly inside another component runs it in the parent's build: its hooks would take the parent's slots. Dev mode panics with the fix when a hook is called from a function that is neither the component being built (its `Build`, its function, or a function literal inside them) nor a *custom hook* named `UseXxx`/`useXxx`, and detects hook-order changes between builds (ADR-085, ADR-088).
- **Automatic skipping (from Jetpack Compose):** a struct component, and `tela.CP[P comparable]`, compare the new value or props to the old with `==` and skip the build if equal and no state/context it read changed. Free memoization. Non-comparable values always rebuild (for struct components the comparison never panics, ADR-088); `tela.Memo` covers custom equality.
- **Hooks:** `UseState`, `UseReducer`, `UseEffect`, `UseLayoutEffect`, `UseMemo`, `UseCallback`, `UseRef`, `UseContext`, `UseID`, `UseValue` (below), `UseSpring`/`UseTween`/`UsePresence`/`UsePresenceTween` (§12; the owner chose these over a `UseAnimation` controller, ADR-080, ADR-091), and `UseMount`, `UseOnce`, `UseEffectDo`, `UseUpdateEffect`, `UseInterval`, `UseTimeout` (the one-shot sibling of `UseInterval`, ADR-171), `UseGo` (ADR-085), and `UseDirection`, the direction at the component's position (ADR-155). `tela.Toggle(setOpen)` is a `func()` that flips a bool state with `Update`, and `C`, `CP` and `Memo` take an optional last `Key(id)` (ADR-091). Generic: `count, setCount := tela.UseState(0)`, where `setCount` is a comparable `tela.Setter[int]` with `Set(v)` and `Update(fn)`; `UseStateSet`/`UseStateUpdate` return one of them as a plain func (ADR-051). Same rules as React: same order every build, never inside conditionals or loops.
- **Phase-deferred reads (from Compose):** `tela.UseValue[T]` returns an observable value. Read during build, it schedules a rebuild like state. Read inside a style closure evaluated in a later phase (`Style(tela.TranslateFn(func() (x, y float32) { return pos.Get() }))`), a change only invalidates that phase: layout or paint, no rebuild. This is how drags, scroll-linked effects and progress bars stay smooth.
- **Context:** `tela.CreateContext[T](def)`, `ctx.Provider(value, children...)`, `tela.UseContext(ctx)`. Only Elements that read a context rebuild when it changes (dependency tracking, like `InheritedWidget`).
- **Concurrency:**
  - The whole tree lives on the **UI thread**.
  - State setters and `UseValue.Set` are safe from any goroutine: they enqueue the update and request a frame.
  - `tela.Post(func())` runs arbitrary code on the UI thread.
- **Effects:** `UseEffect` runs after the frame is presented; `UseLayoutEffect` after layout, before paint. Cleanups run on unmount or before the effect re-runs.
- **Reloads:** under `tela dev`, a hook slot is the same slot in the next build when its component's path from the root and its whole hook signature (kind and value type of every hook) are unchanged; then serializable state is kept (ADR-113).

## 8. Frame pipeline and scheduler

```
event/vsync
  │
  ├─1 Input       platform events → hit-test → gestures → handlers
  ├─2 Animate     tickers advance with the frame clock
  ├─3 Build       Elements marked needsBuild, tree order
  ├─4 Style       resolve ComputedStyle for dirty Boxes
  ├─5 Layout      Boxes with needsLayout (honoring relayout boundaries)
  ├─6 Paint       layers with needsPaint → one DisplayList per layer;
  │               floating elements placed first (float pass, ADR-096)
  ├─7 Semantics   built on demand for consumers (ADR-058)
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
- **Relayout boundary:** a Box with an explicit px width and height and `overflow` other than visible/clip in both axes does not propagate relayout to its ancestors unless its size or baseline changes (ADR-043; tight constraints alone are not a boundary, since a flex item's are derived from its content).

### Algorithms (CSS semantics outside)

**Taffy** (Rust) is the implementation reference; its test fixtures (derived from Web Platform Tests) are the conformance suite.

| Algorithm | Scope |
|---|---|
| **Flex** | Complete: direction, wrap, grow, shrink, basis, `gap`, `justify-*`, `align-*`, `order`, `margin: auto` |
| **Grid** | Tracks with `px`, `%`, `fr`, `auto`, `min-content`, `max-content`, `minmax()`, `fit-content()`, `repeat()` with `auto-fill`/`auto-fit`; explicit and auto placement (dense too); `span`; named lines and areas; `gap`; self and content alignment; absolutely positioned items in grid areas (ADR-073) |
| **Block** | Simple vertical stacking, **no margin collapsing** |
| **Positioning** | `relative`, `absolute` (to nearest positioned ancestor), `fixed` (to window), `sticky`; a transformed or filtered ancestor contains both (ADR-077) |
| **Overflow** | `visible`, `hidden`, `clip`, `scroll`, `auto`. A scroller is a Box with its own layer and smooth scrolling |

- **Box model:** always `box-sizing: border-box`.
- **Direction:** `h.Dir` (HTML's `dir`: `rtl`, `ltr`, `auto`) sets the inline base direction, inherited like CSS `direction`. Under `rtl`, flex rows, grid columns, block auto margins, absolute static positions and scroll origins mirror as in Chromium, and logical utilities resolve against it (ADR-090). Components read it with `tela.UseDirection()` (ADR-155).
- **Defaults:** `display` is `block`, like an HTML `div`. Inline elements (`Span`, `A`) only exist inside a text context (§11); `absolute` and `fixed` ones are blockified like any positioned box, and out-of-flow boxes do not split the inline content around them (ADR-176).
- **Virtualization (from TanStack Virtual / Flutter slivers):** `h.VirtualList(count, estimateSize, renderItem)` (`renderItem` returns a `tela.Node` or a `tela.Component`, ADR-097) builds only visible items plus overscan. It supports variable item sizes measured on the fly, keeps scroll position stable when measurements correct estimates, and provides `ScrollToIndex`.

## 10. Style system

### Style sources and precedence

Resolved in this order; on conflict, **last wins**:

1. Primitive defaults (e.g. `Button` has no background but has `cursor-pointer`).
2. Classes in `Class("...")`, in order of appearance. `Class` may repeat; values concatenate.
3. Typed props in `Style(...)` for dynamic values: `Style(tela.Width(w), tela.Bg(c))`.

There is **no cascade, specificity or selectors**. Inheritance applies only to the properties CSS inherits: text properties (`color`, `font-*`, `line-height`, `letter-spacing`, `text-align`, `text-transform`, `white-space`, `text-decoration`…) plus `cursor`, `visibility`, `pointer-events`, `user-select` (ADR-042), `direction` (set with `h.Dir`, ADR-090) and the SVG paints `fill`, `stroke` and `stroke-width` (ADR-098).

### Classes (Tailwind v4 vocabulary)

- **Reference:** Tailwind CSS v4: palette, spacing, radius, shadow and type scales. Unambiguous v3 aliases are accepted (e.g. `bg-gradient-to-br` → `bg-linear-to-br`).
- **Variants:**
  - State: `hover:`, `active:`, `focus:`, `focus-visible:`, `focus-within:`, `disabled:`, `checked:` (an element whose `h.AriaChecked` is true: Tela has no native checkbox; ADR-100), and the `aria-*` variants `aria-busy:`, `aria-checked:`, `aria-disabled:`, `aria-expanded:`, `aria-hidden:`, `aria-invalid:`, `aria-pressed:`, `aria-readonly:`, `aria-required:`, `aria-selected:` (also `group-aria-*:`, `peer-aria-*:`), which match when the element's `h.Aria…` prop is true (ADR-097, ADR-131), and `aria-[sort=ascending]:`/`aria-[sort=descending]:` for `h.AriaSort` (ADR-131) and `aria-[current=page]:` (also `step`, `location`, `date`, `time`, `true`) for `h.AriaCurrent` (ADR-171).
  - Structural: `first:`, `last:`, `odd:`, `even:`, `group-hover:`/`group-*` (ancestor has class `group`), `peer-*` (preceding siblings only).
  - Environment: `dark:`, `motion-reduce:`, breakpoints `sm: md: lg: xl: 2xl:` (window width at least), their `max-sm:` … `max-2xl:` (window width below the breakpoint), the arbitrary `min-[600px]:` and `max-[40rem]:`, all stackable (`md:max-xl:`; ADR-097), `starting:` (CSS `@starting-style`, §12), `rtl:`/`ltr:` (the element's direction, ADR-090).
- **Arbitrary values:** `w-[320px]`, `bg-[#1e1e2e]`, `grid-cols-[200px_1fr]`. The only arbitrary property is `[clip-path:…]` (ADR-079). **Color opacity:** `bg-white/15`.
- **Parsing:**
  - Each class string is parsed once and interned (cache: `string` → slice of ops on `ComputedStyle`).
  - Variants produce conditional styles evaluated against Element state (hover, focus…).
- **Unknown classes:** a visible error in dev mode (log and inspector) with the closest class by edit distance. `tela lint` catches them statically in literals.

### Theme and tokens

- The theme is a Go struct: colors, spacing, radii, shadows, fonts, breakpoints, easing curves.
- **Generated, not hand-copied:** `go generate` in `internal/style/gen` reads the Tailwind v4 theme source (CSS, vendored) and emits the Go default theme. The class vocabulary used for suggestions is the parser's supported utilities expanded with the theme keys (ADR-037). OKLCH colors are converted to extended sRGB at generation time, without gamut mapping (ADR-046).
- Semantic color tokens with shadcn/ui's names (`bg-primary`, `text-muted-foreground`, `border-border`) work in every color utility and resolve when a style is computed, for the color scheme of the element: the app's, or a subtree's `h.Scheme`. `App.Theme` (colors, radius, fonts) can be swapped at runtime (`tela.SetTheme`); `App.ClassAliases` names class combinations; `UseTheme` reads the tokens in Go (ADR-089).

### CSS subset

**Fully implemented:**
- **Layout:** flex, grid, positioning (including `sticky`), overflow and scrolling.
- **Sizing and spacing:** `width`, `height`, `min-*`/`max-*`, `aspect-ratio`, padding, margin (no collapsing), `gap`.
- **Backgrounds and borders:** colors; linear, radial and conic gradients; multiple background layers; background images (`bg-[url(…)]` from the file systems registered with `tela.Assets`, SVG files included, or `tela.BgImage`; size, position, repeat, origin, clip, `bg-clip-text`, `bg-blend-*`; ADR-079, ADR-098); per-side borders; per-corner `border-radius`; `outline` with offset.
- **Effects:** multiple and `inset` `box-shadow`, `opacity`, `filter` (`blur`, `brightness`, `contrast`, `grayscale`, `saturate`, `drop-shadow`…), `backdrop-filter`, `mix-blend-mode`, `clip-path` basic shapes (written as the arbitrary property `[clip-path:circle(50%)]`, ADR-079), masks (`mask-[url(…)]` and Tailwind v4's gradient masks with their size, position, repeat, origin, clip, mode and composite utilities; ADR-098).
- **Images and icons:** `Img` from decoded images, `tela.Assets` paths, bytes or any `fs.FS` (PNG, JPEG, GIF, WebP and SVG, which stays vector), `object-fit` and `object-position`, `h.Tint`; `h.Icon` with `tela.Icon` SVG documents; the SVG paint utilities `fill-*`, `stroke-*` and `stroke-<n>` (ADR-098).
- **Motion:** 2D `transform` (`translate`, `scale`, `rotate`, `skew`, `origin`), `transition` on any animatable property.
- **Interaction:** `cursor`, `pointer-events`, `user-select`, `visibility`, `z-index` within parent.
- **Text:** see §11.

**Limited:**
- **`calc()`:** only `min()`, `max()`, `clamp()` and add/subtract of two units.
- **Keyframes:** via the Go API, plus predefined classes (`animate-spin`, `animate-pulse`, `animate-ping`, `animate-bounce`).
- **Inline:** rich text with decorated inline boxes and atomic inlines (§11); no `vertical-align` on spans.
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
| SVG `url()` filters, `clip-path` with `url()`, `path()` or reference boxes | Native filters and basic shapes `[clip-path:inset()/circle()/ellipse()/polygon()]` |
| Units `cm`, `in`, `pt`, `pc`, `ex` | Supported units (below) |
| Vertical `writing-mode`, ruby, multi-column, `shape-outside` | Nothing |

**Supported units:** `px` (logical pixels, multiplied by display scale), `rem` (16px default, theme-configurable), `em`, `%`, `vw`, `vh`, `fr`, `ch` (approximate).

## 11. Text

- **Shaping, bidi, segmentation:** `go-text/typesetting`. A paragraph's bidi base level is its block's direction (`h.Dir`, ADR-090); `text-start`/`text-end` and the default alignment follow it.
- **Text layout (modeled on Linebender's Parley):** the layer above shaping that `go-text` does not provide:
  - styled runs → shaped lines;
  - line breaking per UAX #14 (word wrap, `break-words`);
  - cursor positions, selection rectangles, hit-testing (point → character index);
  - word/line navigation.
- **Fonts:**
  - **Embedded** (`go:embed`, OFL): Inter as default sans, a monospace font, a color emoji font. Chosen in the text phase.
  - System fonts are available (discovery via `go-text` fontscan), but **snapshots and tests use embedded fonts only**, so they are deterministic across machines.
  - **Fallback:** a per-script family list, embedded fonts last.
- **Paragraphs:** a text node contains styled runs (`Text`, `Span`, `Strong`, `Em`, `Code`, `A`) and atomic inline-level boxes (`inline-block`, `inline-flex`, buttons, inline images: laid out like any box and placed on the line by `vertical-align`). Inline elements are CSS inline boxes: their horizontal margins, borders and padding take space on the line, and their backgrounds, borders and radii are painted per line fragment (ADR-066).
- **Line boxes:** CSS 2.1 §10.8, quantized like Blink's in device pixels at the display scale: font ascent, descent and line gap rounded to whole device pixels, half-leadings rounded down to whole device pixels; layout results stay in logical px, so they depend on the scale as Chromium's do (ADR-159).
- **Properties:** `font-family`, `font-size`, `font-weight` (incl. variable fonts), `font-style`, `line-height`, `letter-spacing`, `text-align`, `text-transform`, `text-decoration`, `white-space`, `text-wrap` (`text-balance` and `text-pretty` break lines as Blink does, ADR-166), `text-overflow: ellipsis`, `line-clamp-*`, `tabular-nums`.
- **Glyph raster:**
  - Same path machinery as the rasterizer.
  - Glyph cache keyed by (font, size, quantized subpixel offset), respecting display scale. Under scale/translate transforms the pen x snaps to 1/4 device pixel and the baseline y to whole device pixels; other transforms and large glyphs fill the outline directly (ADR-029).
  - Grayscale antialiasing (no RGB subpixel) so output is identical across platforms.
- **Text editing (`Input`, `Textarea`):** cursor, mouse/keyboard selection, undo/redo, clipboard. **IME** composition arrives from the backend as `IMEEvent` batches (delete surrounding text, commit, new preedit; an empty preedit ends the composition) and `IMESessionEvent`s; the field reports its caret area, purpose and surrounding text with `Window.SetTextInput` (ADR-068).

## 12. Animation

- **Clock:** a `Ticker` bound to the scheduler's frame clock; frames are requested while any ticker is active. With `motion-reduce` on (system or theme preference), transitions take 0 time. The pieces (easing, springs, interpolation, keyframes, the ticker) live in `internal/anim`, which `core` drives (ADR-076).
- **Implicit transitions:** `transition`, `duration-*`, `ease-*` and `delay-*` interpolate animatable `ComputedStyle` properties on change, following CSS Transitions Level 1 (retargeting, reversing), and `animate-spin`, `animate-ping`, `animate-pulse` and `animate-bounce` run Tailwind's keyframes (same mechanism as Flutter's `AnimatedContainer`; ADR-076).
- **Explicit animation:**
  - No `UseAnimation` controller object: the owner chose declarative hooks plus an imperative `Animate` (ADR-080); repeating motions use `animate-*` classes or a loop of `Animate` with `OnDone`.
  - `UseSpring(target, opts...)` and `UseTween`: follow a target that changes between builds, interruptible, keeping velocity, starting at the target or at the `Initial(v)` option (ADR-088); `Animate(v, target, opts...)` animates any `UseValue` from handlers, effects or goroutines. Typed over `float32`, `Color`, `Point`, `Size`, `Rect` (ADR-080); `OnDone` and `Velocity` options, and the paint-phase color props `BgFn` and `TextColorFn` (ADR-082).
  - All of these feed `UseValue` (§7), so they animate without rebuilds.
- **Enter/exit:** `Presence` keeps a removed child mounted, inert, until its exit ends (like `AnimatePresence`): its `Exit` classes transition and its `UsePresence` components call `done`. Entrances use the `starting:` variant (CSS `@starting-style`) on any box that starts being rendered (ADR-080).
- **Layout animations (from Motion):**
  - `AnimateLayout()` on a Box animates any change in its laid-out position/size (reorder, expand) from old box to new box, FLIP style, using transforms: layout runs once per change, and the box is drawn through a translation and scale (radii and shadows corrected, nested animated boxes counter-scaled) that goes to the identity; its frames only composite (ADR-081).
  - `LayoutID("x")` animates an element between two places in the tree (shared element transition): a box that mounts starts from the box of the element with its ID that unmounted in the same commit or is exiting.
  - Trivial for Tela because it owns layout and knows both boxes.
- **Performance:** animating `transform` and `opacity` only touches layer compositing. Animating layout properties is possible but costlier.

## 13. Graphics and rendering

### DisplayList

- The paint phase emits commands into one DisplayList per layer:
  - Shapes: `FillRect`, `FillRRect`, `FillPath`, `StrokePath`.
  - Content: `DrawGlyphRun`, `DrawImage`.
  - Shadows: `DrawShadow` (incl. inset).
  - State: `PushClip(RRect|Path)`/`PopClip`, `PushTransform`/`Pop`, `PushLayer(opacity, blend, filter, mask)`/`PopLayer`, `BackdropFilter`. A mask is painted content (a display list) used as alpha or luminance coverage of the layer (ADR-074).
- **Paints:** solid color or gradient (linear, radial, two-point focal, conic; with stops and an interpolation color space, Oklab by default like Tailwind v4's named utilities, sRGB for gradients written as CSS text unless they say `in oklab`; ADR-023, ADR-158). Linear, radial and focal gradients pad, reflect or repeat (ADR-095).
- **Coordinates:** float32 in logical pixels; display scale is applied at raster time.

### CPU rasterizer (initial and reference backend)

- **Antialiasing:** analytic per-cell coverage (approach of font-rs, tiny-skia, vello_cpu sparse strips). Axis-aligned rects and rounded rects have fast paths.
- **Stroking:** path stroking (joins, caps, dashes) and curve flattening follow kurbo's algorithms; curves are flattened before offsetting (ADR-026).
- **Color:** premultiplied RGBA8 in sRGB. **Blending happens in sRGB space, like browsers**, so colors and gradients match Tailwind on the web; blend modes follow W3C Compositing and Blending on those values (ADR-074). Colors are extended sRGB until rasterization: solid paints clip per channel, gradients interpolate unclipped stops and clip each pixel, as Chromium does (ADR-046).
- **Determinism:** float code in `gfx`, `raster` and every layer that feeds them is fusion-safe (ADR-041) (explicit rounding conversions; transcendental functions from `internal/fmath`), so pixels are bit-identical across CPU architectures (ADR-027).
- **Parallelism:** tiles rasterized by parallel goroutines.
- **Shadows:**
  - Rounded-rect shadows (the common case, all Tailwind `shadow-*`) use **Evan Wallace's closed-form analytic Gaussian** (erf-based): no blur pass, nearly free.
  - Arbitrary-shape shadows (`drop-shadow`), `filter: blur` and `backdrop-blur` use a separable Gaussian approximation (three box-blur passes; an exact kernel for small radii; large radii blurred at a reduced resolution, and backdrop filters computing only the pixels they draw, in parallel bands, ADR-078). Results are not cached yet; layer caching (below) will cover them (ADR-028).
- **Layers and caching:**
  - A Box is a repaint boundary if it has animated `transform`/`opacity`, is a scroll container, is explicitly marked, or a dev heuristic suggests it (frequent repaints).
  - Each layer keeps its raster; unchanged layers are only composited. Refined by ADR-061: layers are cached display lists and the persistent framebuffer is the only raster cache, so incremental frames stay bit-identical to full ones.
- **Damage tracking:** the union of changed rects is computed; only that region is rasterized and presented. The backend receives the damage list (Wayland `wl_surface.damage_buffer` and equivalents).
- **z-index and stacking contexts:**
  - Paint in tree order; `z-*` reorders among siblings.
  - A positioned Box or flex item with `z-*`, or a Box with `opacity` < 1, `transform`, `filter`, `backdrop-filter`, a blend mode, a `clip-path`, a mask or `isolate`, creates a stacking context (ADR-045, ADR-079, ADR-098).
  - Overlays (menus, tooltips, modals) mount into a **portal layer** above the root via `h.Portal(...)` (ADR-077).

### GPU backend (future)

- Consumes the **same DisplayList**; nothing above `gfx` changes.
- **Fixed set of precompiled shaders** (rounded rect with border and analytic shadow as one primitive, like GPUI's quads; gradient; blur; glyph atlas; image; composite). No runtime shader generation (Impeller's lesson).
- Vulkan (Linux, Windows), Metal (macOS), optional D3D11 (Windows), via purego/syscall.
- The CPU rasterizer stays the reference for goldens and headless.

## 14. Input, events and focus

- **Events:**
  - Pointer: `MouseEvent`, `PointerEvent` (mouse, touchpad, pen).
  - Scroll: `WheelEvent` with precise deltas.
  - Keyboard: `KeyboardEvent` (ADR-056) following winit's model: **physical key** (scancode-based position), **logical key** (layout-dependent meaning) and **text** (produced characters) as three separate fields, plus modifiers and repeat flag.
  - Text fields and forms: `InputEvent` (beforeinput), `ChangeEvent` (React's onChange), `CompositionEvent`, `SelectEvent`, `SubmitEvent` (ADR-069).
  - Focus: `FocusEvent`.
- **Propagation:**
  - Hit-testing over the Box tree honors clips, transforms and `pointer-events`.
  - Events bubble from target to root, with `e.StopPropagation()` and `e.PreventDefault()`.
  - No capture phase (can be added by ADR if needed).
- **Gestures:** a Flutter-style **gesture arena**: tap, double-tap, long-press, drag and pan recognizers compete for a pointer sequence; one wins. High-level handlers (`OnClick`, `OnDrag`…) are built on it.
- **Focus:** a focus tree with tab order following tree order plus `TabIndex`; focus traps for modals (a containing focus scope keeps the focus inside against Tab, presses and scripts, and takes it back when the focused element unmounts; ADR-160, ADR-168); `focus-visible` only activates on keyboard navigation (text fields match it on mouse focus too, as in Chromium; ADR-069). A modal overlay makes everything outside it inert, and `h.Inert` (HTML's `inert`) a subtree: not hit, not focusable, hidden from the semantics tree (ADR-096).
- **Cursors:** declared by style (`cursor-*`), resolved by the backend.

### Behavior hooks (`aria` package, React Aria model)

Component *behavior* is separate from *styling*. `aria` provides unstyled hooks that implement interaction and accessibility correctly; `kit` adds the look.

- `UsePress`: a consistent "press" across mouse, touch, pen and keyboard; cancels when the pointer leaves.
- `UseHover`: ignores touch-emulated hover.
- `UseFocusRing`, `UseFocusScope` (trap and restore focus; a `FocusManager` moves it).
- `UseButton`, `UseToggleButton`, `UseCheckbox`, `UseSwitch`: a press plus role, aria state, tab stop and an integrated, controlled or uncontrolled, toggle state.
- Every hook takes an options struct and returns a struct whose `Props` field is one `tela.Prop` (several merged with `core.MergeProps`); see ADR-100.
- Collections: `UseListBox`, `UseMenu`, `UseTabs` (ADR-104), `UseSelect`, `UseCombobox` (ADR-105), with arrow-key navigation and typeahead.
- Choices and ranges: `UseRadioGroup`, `UseToggleGroup`, `UseSlider`, `UseProgressBar`, `UseMeter` (ADR-132).
- Menus beyond the menu button: `UseMenuBar` and `UseContextMenuTrigger` (ADR-133).
- Dates: `UseCalendar` and `UseRangeCalendar` over the comparable `aria.Date` type, with the names, headings and first weekday of `i18n` (ADR-134).
- Colors: `UseColorState` (the color a picker shares), `UseColorArea`, `UseColorSlider`, `UseColorField`, with the `HSB` and `HSL` types and hex parsing (ADR-135).
- `UseOverlay` (dismiss on outside press or Esc, modality), `UseDialog`, `UsePopover`, `UseOverlayTrigger`, `UseMenuTrigger`, `UseTooltipTrigger` (ADR-101).
- **Floating positioning (port of Floating UI, ADR-099):** `UseFloating(anchor, opts...)` with `Placement`, `AnchorRect` (virtual anchors), `Offset`, `Flip`, `Shift`, `Size`, `Arrow`, `Hide` and `AutoPlacement` as functional options applied in the order given, and a result with `Props`, `ArrowProps` and the final `Placement`. Every popover, tooltip, menu and select uses it.
- **Engine (ADR-096):** `aria` imports `internal/core` for what hooks cannot do alone: anchored positioning in the same frame (a float pass in the paint phase runs the ported `computePosition` on the painted rectangles; autoUpdate without frames of its own), `UseSize`, interact-outside, the overlay stack (Escape and outside presses reach the top overlay only), modality, focus scopes with auto-focus and the focus manager.

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
    SetTextInput(*TextInputState); ResetTextInput() // IME: focused field's caret area, purpose, surrounding text
    Close()
}

type Handler interface{ HandleEvent(w Window, e Event) } // never called re-entrantly
```

- **Events** (sealed set): `ConfigureEvent` (size and/or scale; implies a frame), `FrameEvent`, `CloseRequestedEvent`, `FocusEvent`, `WakeEvent`, `SettingsEvent`, `PointerEvent` (DOM-style `Action`: move, enter, leave, down, up, cancel), `ScrollEvent` (precise `Delta`, discrete `Steps`, gesture `Phase`; ADR-057), `KeyEvent` (ADR-050), `IMEEvent` and `IMESessionEvent` (ADR-068), `WindowStateEvent`, and the app-scoped `NotificationEvent` and `TrayEvent` (ADR-179).
- **Sizing rule:** `PhysicalSize(logical, scale)` rounds logical × scale half away from zero (the `wp_fractional_scale_v1` rule). Every backend sizes framebuffers with it.
- `Run` does not return when the last window closes; quitting is app policy (`core`/`tela`).
- Backends share the OS-independent half of the loop (`internal/platform/internal/loop`: queue, wake/quit, frame pacing; ADR-050) and keep only their OS I/O.
- **Desktop (phase 12, ADR-178):** windows also take state (minimize, maximize, fullscreen, visibility, always-on-top, min/max size, position where the OS allows it), a parent and modality, and hit regions for custom title bars; backends expose OS services (open URI, file dialogs, message boxes, notifications, tray icons). The Linux services are shared by Wayland and X11 over D-Bus. `Backend.Desktop()` returns them; window control methods are requests whose outcome arrives as `WindowStateEvent`/`ConfigureEvent` (ADR-179). The exact contract is in `doc.go`.
- The IME is driven by `Window.SetTextInput` (cursor area, purpose, surrounding text; nil = no text field) and `ResetTextInput` (ADR-068).

### Backends

| Backend | Transport | Notes |
|---|---|---|
| `wayland` (primary on Linux) | Protocol over `$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY` | `xdg_shell`, `wl_shm` (memfd buffers), `wp_viewporter` + `wp_fractional_scale_v1`, `xdg-decoration` (if absent, Tela draws its own decorations), `wp_cursor_shape_v1`, `wl_data_device` (clipboard), `zwp_text_input_v3` (IME). **Protocol bindings are generated from the official XML** (a Go `wayland-scanner`, see `rajveermalviya/go-wayland`), so adding a protocol is trivial. **Risk:** the keyboard sends an XKB keymap, so a **minimal pure-Go XKB parser** is needed (reference: `kbvm`, a pure-Rust XKB implementation) |
| `x11` (Linux fallback) | X11 protocol over socket, via `jezek/xgb` | `PutImage`/MIT-SHM, XInput2 for smooth scrolling, XKB or core keyboard mapping, `_NET_WM_*`, selections for clipboard. IME (XIM) comes later |
| `win32` | `x/sys/windows` + `syscall.NewCallback` | `CreateWindowExW`, `GetMessageW` loop, `SetDIBitsToDevice`/`StretchDIBits`, per-monitor DPI v2, IMM32, Win32 clipboard, `DwmFlush` for pacing. Reference: Ebitengine's no-cgo Windows code |
| `cocoa` | `purego` + ObjC runtime | `NSApplication`, `NSWindow`, layer-backed `NSView`, present via `CALayer.contents` (`CGImage`), `NSTextInputClient` (IME), `CVDisplayLink`. Main thread only. Reference: Ebitengine's purego macOS code |
| `headless` | Memory | Tests, snapshots, CLI. Configurable size and scale, controllable clock |
| `remote` (dev loop only) | AF_UNIX socket to a host process; pixels in shared memory (memfd, named file mapping) | The child backend of `tela dev`: windows, events, clock, clipboard, IME and settings are forwarded to a `remote.Host` that owns the real windows on another backend and swaps children under them (ADR-111, ADR-112) |

- **Linux backend selection:** `WAYLAND_DISPLAY` set → Wayland; else `DISPLAY` → X11. Override with `TELA_BACKEND=wayland|x11|headless`.
- **HiDPI:** everything works in logical pixels. The framebuffer is allocated in physical pixels (`PhysicalSize(logical, scale)`); fractional scales (Wayland `wp_fractional_scale_v1` + `wp_viewporter`, Windows per-monitor DPI) render on the device pixel grid like integer ones (ADR-075).
- **Platform quirks:** before implementing a backend feature (DPI, IME, keyboard, decorations), check how winit handles it; its code and issues document most OS quirks.

## 16. Semantics, accessibility and agents

- Any Box may contribute a semantics node. Primitives have implicit roles (`Button` → `button`, `Input` → `textbox`, `H1` → `heading` level 1), overridable with `Role(...)`, `AriaLabel(...)`, `AriaChecked(...)` and the other `Aria…` props of `h` (ADR-058).
- **A semantics node holds:** role, label, value, description; states (focused, disabled, checked, expanded, selected, invalid…; sort for headers); actions (click, focus, setValue, scroll); bounds in window coordinates.
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
| `tela lint [-json] [-tests] [packages]` | Static form of the dev-mode checks on constant strings: unknown classes (aliases and theme colors of the linted packages known), roles, aria tokens, duplicate or missing keys, hook misuse; `file:line:col: rule: message`, exit 1 when anything is reported, `//tela:ignore rule` (ADR-173) |
| `tela dev [pkg] [--size WxH] [--scale N] [--dark] [--preview NAME] [--no-state] [-v]` | Watch, rebuild in the background, swap the app process under a window that never closes, keeping hook and engine state; errors as an overlay over the last good frame. Go can't swap code in a process, so the restart is made invisible (ADR-111); state outside the tree: `App.DevState` (ADR-114) |
| `tela new [-module path] [-skip-deps] <dir>` | Create an app: `go.mod` (tela at this binary's release version, or the latest), a counter in `main.go` with `var App`, a `telatest` test, README and `.gitignore`, from templates embedded in the binary; runs `go get` and `go mod tidy` (ADR-128) |
| `tela icons gen -pkg <name> [-o dir] <dir>` | Generate a Go file with one `tela.Icon` variable per SVG file: validated, minified, deterministic (ADR-102) |
| `tela add <component>...` | Copy `kit` component source into the user's project (shadcn model), rewriting the package name |
| `tela mcp [pkg] [--size WxH] [--scale N] [--dark] [--preview NAME] [-v]` | MCP server over stdio driving a headless instance of the screen that stays up between calls: `snapshot` (aria YAML with `[ref=eN]`), `screenshot` (PNG), `click`, `hover`, `fill`, `press`, `scroll`, `drag`, `resize`, `open`, `wait`, `inspect`, `lint`; rebuilds and remounts when the sources change (ADR-174) |

### Contract for `snapshot`, `inspect`, `mcp`

The package exposes `func Preview() tela.Node` (mounted as `tela.C(Preview)`, so it may use hooks: ADR-175) or `func Preview() tela.Component`, or a `tela.Previews` variable with several named screens (`tela.Preview{Root, Wrap}`: ADR-092). A package-level `var App = tela.App{...}` gives the window of the render (size, root class, color scheme, theme, class aliases; ADR-092), and alone its `Root` (a `tela.Component`: `tela.C(Screen)` or a struct component, ADR-094) is the screen. The CLI generates a temporary `main` that mounts it on the chosen backend.

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
- Controllable clock (`app.Advance(300*time.Millisecond)`) for animations; `NoSettle()` captures their frames: with it, actions, screenshots and the queries (`AriaSnapshot`, `Count`, `Bounds`, `Stats`, `Cursor`) handle only the work due, and only the wait before an action settles fully (ADR-080, ADR-097).
- **Auto-wait (from Playwright):** actions and expectations wait for pending frames and running animations to settle (with a timeout) before acting or asserting; under `NoSettle` an expectation's first try handles only the work due and the clock moves only between failed tries.
- `app.AriaSnapshot()` returns the same YAML as `tela inspect` for assertion.
- State assertions without strings (ADR-086): `app.Expect(loc, telatest.Checked(true), telatest.Disabled(false))` (also `Pressed`, `Selected`, `Expanded`, `Focused`, `Invalid`, `Sort`, `Current`, `HasName`, `HasValue`); `ByTestID` finds elements by `h.TestID`; `Bounds` returns a `tela.Rect`; `ClickAt`, `DragFromTo`, `PixelAt`, `Measure`, `GoldenScales` and `MountFunc` shorten the rest.
- Several windows (ADR-183): `app.Windows()`, `app.Window(title)` and `app.WaitForWindow(f)` return an `*App` scoped to one window; `ExpectWindowState`, `RequestClose`, `HitRegions` and `ExpectQuit` cover window-level behavior through the headless window manager.

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
  - Locally: `GOOS=windows go test -exec wine ./...`, `wine` to run examples (CI runs the tests under Wine with a null graphics driver and no display, not `xvfb-run`).
  - Wine is not real Windows (DPI, DWM, IME and accessibility differ), so there is periodic validation on `windows-latest` runners.
- **macOS:** `macos-latest` runners, with `cocoa` backend snapshots to validate presentation.
- **Benchmarks:** typical-scene raster (dashboard, long scrolling list, heavy blur), 10k-node layout, paragraph shaping. Regressions are tracked.

## 19. Conventions

- **Language:** everything in English: code, identifiers, comments, commits and docs.
- **Public API:**
  - React/Tailwind names when an equivalent exists.
  - Functional options for configuration.
  - No `interface{}` in public API where a closed type works.
  - Children and props share the interface `tela.Child` (a `Build() tela.Node` method, implemented by `Node`, `Prop` and every `Component`; no longer sealed, ADR-088), so `Div("x")` is still a compile error.
- **Package `h`:** designed for dot-import (`. "github.com/kuronosu/tela/h"`). It holds only elements, props and helpers (`If`, `Map`, `Range`, `Fragment`, `VirtualList`) to minimize collisions. `If` and `IfElse` take and return a `tela.Child` (ADR-094). Every `On…` handler takes a `func()` or a `func` of its event, or the `tela.StopPropagation(f)` wrapper (ADR-097); `Class` takes several strings and a `tela.StyleProp` is a child (ADR-091).
- **Errors:** programmer errors detectable in dev mode panic with an actionable message; likely mistakes that work as written are dev warnings; platform/resource errors are returned as `error`. Dev mode is on unless the binary is a release build (ADR-085).
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
| SVG | **resvg / usvg** (MIT/Apache) | Normalization rules of static SVG (`internal/svg`, ADR-095): `use`/`symbol` instancing, gradient `href` chains and bad stops, units, invalid geometry and references |
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

The decisions live in [`docs/adr/`](adr/README.md), one file per ADR, with an index of one line each (ADR-165). They are append-only: a changed decision gets a new ADR ("Supersedes ADR-NNN" or "Amends ADR-NNN") and a minimal edit of the section above that it changes. Read the index, then only the ADRs a task touches.
