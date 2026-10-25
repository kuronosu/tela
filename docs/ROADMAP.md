# Tela Roadmap

> **Living** document, updated at the end of every session. Design lives in
> [`ARCHITECTURE.md`](ARCHITECTURE.md); this file only tracks **what is left and
> where we are**.

## Current status

- **Phase in progress:** none; phase 10 (`kit`) is done: `v0.10.0` tagged 2026-10-25 on c094ab4 by the owner (not pushed).
- **Phase 10 delivered (2026-10-25):** package `kit` (34 components on shadcn/ui `new-york-v4`, checked against Chromium in both schemes and scales; recipes, `StyleProvider`, trigger slots, logical utilities and `DirectionProvider`; ADRs 129, 130, 136–143, 151), the `aria` hooks it needed (radio, toggle group, slider, progress, meter, menu bar, context menu, calendars, color; ADRs 132–135), engine and i18n additions (`h.AriaInvalid`, `h.AriaSort`, first day of week, month/weekday names, `MonthYear`; ADR-131), kit's class order as Tailwind's stylesheet (ADR-140), the examples on kit (dashboard with a glass recipe, settings, pricing, signup, player, board, i18n, native with three recipes; ADRs 144–149), the `examples/kit` gallery and the Kit wiki guide (ADR-150), review fixes (ADR-152) and `tela.DevMode` (ADR-153).
- **Phase 10 done criteria:** `examples/dashboard` (glass recipe, look preserved against its old golden) and `examples/native` (Win11, Adwaita, Breeze recipes) are built only on kit; settings uses the default style; no migrated example has a hand-made control or drawn icon (charts, cover art and decoration remain); hello, counter, shapes, text, layout, motion, virtuallist and aria keep bit-identical goldens. A `/code-review high` found 10 issues (worst: a 26 MB binary committed by an agent, removed from the unpushed history; `Toaster{Expand: true}` never closing toasts); all fixed with regression tests.
- **Phase 10 known limitations:** no submenus; dashboard p95 23.1 → 26.7 ms on this machine (fewer cheap frames; scroll frames +9%; the old page already missed 16 ms here); `tela snapshot` does not show toasts set by effects (posts); kit components keep aria hook state in refs that `tela dev` cannot keep (silent resets); a flex item with `aspect-ratio` and `w-full` does not shrink like Chromium's (calendar cells, a stretched `w-fit` Calendar is too tall); `flex-1` in an auto-height column collapses; arbitrary `bg-[linear-gradient(…)]` interpolates in Oklab where CSS uses sRGB; text with `leading-none` sits one device pixel off Chromium; `telatest` has no right click or long press; `Props` takes one prop (`MergeProps` is internal); the README screenshots of the migrated examples are old.
- **Phase 10 owner review (2026-10-25):** the owner decided the API review (ADR-154): `tela.UseDirection()` replaces `kit.DirectionProvider` (ADR-155); `Default…` fields are pointers where zero is valid, `Dialog.HideClose`, `Card.TitleLevel`, `tela.MergeProps`, `Table.RowProps`, `AriaLabel`/`ID` fields (ADR-156); README screenshots regenerated.
- **Next step:** plan phase 11 (the API gaps and engine issues of ADRs 144–150 are candidates for `v0.10.x` and phase 11).
- **Phase 9:** (dev loop, `tela dev`) is done: `v0.9.0` and `v0.9.1` (Windows fixes) tagged 2026-10-22, not pushed. On Windows `tela dev` reloads `examples/dashboard` in ~1.0–1.1 s with Microsoft Defender excluding `%TEMP%` and the Go build cache (~2.2 s without: every new build is scanned on its first run, ~690 ms); `tela dev` prints how to exclude them when a reload's build is slow to start (ADR-125, ADR-126).
- **Phase 9 delivered (2026-10-22):** `internal/platform/remote`, a platform backend over IPC with shared-memory frames, input freeze/promote and an overlay hook (ADR-112); hook and engine state captured and restored in `core` with React Fast Refresh's rule, compiled only under the `teladev` tag (ADR-113); the child runtime and `App.DevState` (ADR-114); the `tela dev` supervisor with native watchers, background builds and swap (ADR-115); faster dev builds and concurrent font parsing (ADR-116); the error overlay (ADR-117); text fields reveal their caret after layout (ADR-118, ADR-122); review fixes (ADR-119 to ADR-124).
- **Phase 9 verified:** `examples/dashboard` reloads live on Wayland and X11 (Xwayland) in 433–678 ms from save to the new frame (headless median 588–621 ms; was 855 ms before T6), the same window (Hyprland address and supervisor PID) throughout, scroll, focus, caret and typed text kept, a component that gains a hook resets alone; a build error docks an overlay over the running app and the next good save clears it in the same present; capture bursts show one transition, no flash. Release build of `examples/dashboard` (`-trimpath`) is 25,634,957 bytes vs 25,622,954 at `v0.8.2` (+12 KB, every changed symbol from the phase's non-dev fixes or `App.DevState`), no dev-loop symbol, `encoding/gob` absent (`TestReleaseBuildHasNoDevLoop`).
- **Phase 9 known limitations:** Windows not run on a real machine yet (kit ready: remote, watcher and supervisor test binaries; live `tela dev` there needs Go on the VM); macOS has a polling watcher and an untested shared-memory path; MinSize/FixedSize are not re-applied on a swap; Win32 live resize lags (WM_SIZE cannot wait for the child); a hidden window starves the pending child (15 s timeout); an IME composition is lost across a swap; i18n's runtime locale wins over a `SetLocale` added in code; `aria.Selection` and other values with unexported fields reset silently; a renamed component resets; a VirtualList's measured sizes are not kept; `OnScroll` fires once for a restored offset; dev builds cannot be debugged (no DWARF); a focus that restyles a field still reveals its caret one frame late (IME caret area stale for a frame); a `UseLayoutEffect` that narrows a field and focuses it in the same frame reveals with the old width; `ElementRef.SetSelectionRange` from a handler, posted function or effect applies queued updates at once (`flushDiscrete`); `tela dev`'s output detach cannot unblock a copy stuck on a terminal paused with Ctrl+S; `wtype` types wrong keys into Xwayland windows (Hyprland gives Xwayland a `pc+us` keymap; not Tela).
- **Phase 8 done** (2026-10-21): the Lucide set (1,902 icons, `icons/lucide` module) renders within mean 0.91 levels per channel of Chromium at scale 1 and 0.55 at scale 2 (24 px; worst icon 3.10; 16 px measured once at 0.93/0.52); `examples/aria` builds a menu, a combobox and a modal dialog (plus tabs, a tooltip, a select, a switch and a multi-select list box) only from `aria` hooks, and its tests drive them by keyboard and assert full aria snapshots and focus restoration. Race detector (21 packages, `examples/aria` included) and arm64 (qemu: gfx, raster, svg) pass. A `/code-review high` found 10 issues (worst: a touch pan still delivered pointerup, so popovers closed while scrolling; an unhashable `SrcFS` panicked under the asset lock and deadlocked the next load); 9 fixed with regression tests, 1 measured and kept (ADR-107, ADR-108), plus programmatic focus and `active:` after a pan.
- **Phase 8 delivered:** `internal/svg` with `<style>`/`<switch>` and gradient spread modes (ADR-095); the same-frame anchored positioning engine, overlay stack, interact-outside, modality, `h.Inert`, `UseSize`, widget aria props (ADR-096); width and `aria-*:` variants, `h.Range`, `tela.StopPropagation`, phase-7 removals (ADR-097); `h.Icon`/`tela.Icon`, `Img` sources, `Tint`, `object-fit`, CSS masks, SVG paints (ADR-098, ADR-103); `aria.UseFloating` (ADR-099); press, hover, focus ring, focus scope, buttons, toggles and `checked:` (ADR-100); overlays, dialog, popover, triggers, tooltip, `ElementRef.Click` (ADR-101); `tela icons gen` and Lucide (ADR-102); list box, menu, tabs, `aria.Selection` (ADR-104), select and combobox (ADR-105); `examples/aria` and hook-made default buttons (ADR-106); telatest settles in app time, `ActiveDescendant`/`HasPopup`/`Controls` states.
- **Phase 8 known limitations:** SVG has no text, filters, images, patterns or markers; an SVG with only a viewBox takes it as its natural size (Chromium stretches it); `fill`/`stroke` do not transition; downscaled images and mask tiles use mip levels, not Chromium's filters; menus have no submenus or press-drag-release; Shift+Tab into a collection focuses its first item; horizontal lists cannot be virtualized; no `aria-posinset`/`aria-setsize`; auto-focused items in a popover scroll into view one frame late; a floating element inside a transformed ancestor is misplaced (use `h.Portal`); `UseFloating`'s `size` can end on another placement than Floating UI's reset loop (2 of 90 oracle cases); `MatchWidth` sets an exact width; the `icons-lucide` CI job is unverified until pushed and the module needs the `v0.8.0` tag (it builds through `replace` until then); no live Wayland, X11 or Windows run this phase (headless and telatest only).
- **Phase 7 done** (2026-10-21): all 15 examples build and pass on the new API with every golden bit-identical; no example drops `Run`'s error (12 → 0), concatenates classes (148 → 0), wraps `CP` in `WithKey` (13 → 0) or ignores a handler's event (60 → 0); dev mode is on by default in development builds and every `telatest` test, so class, role, key and easing typos are reported; every renamed symbol is deprecated for one phase and the CHANGELOG has a Migration table; `examples/i18n` switches English/Spanish/Arabic at runtime with plurals, dates, numbers and mirrored layout. Race detector and arm64 (qemu) pass. A `/code-review high` found 9 issues (worst: a struct component embedding an interface crashed on its first build); all fixed with regression tests (ADR-093).
- **Phase 7 delivered:** dev mode by default, `tela.Main`, `UseMount`/`UseOnce`/`UseEffectDo`/`UseUpdateEffect`/`UseInterval`/`UseGo`, boundary and context diagnostics (ADR-085); `telatest` ergonomics (ADR-086); `tela/i18n` on `golang.org/x/text` (ADR-087); struct components, generic keys, `ID`/`UseID`/`KeyName`/`KeyCode`, `tela.Once`, `UseSpring(target, Initial)` (ADR-088); shadcn theme tokens, `ClassAliases`, `h.Scheme`, `UseTheme` (ADR-089); right-to-left layout and `h.Dir` (ADR-090); `Class(parts...)`, `func()` handlers, `Toggle`, keyed `C`/`CP`/`Memo`, `MapKeyed`, `Easing`, `UsePresenceTween`, `VirtualList` keys/heights/roles (ADR-091); `Preview{Root, Wrap}`, `var App` for the CLI, `MountApp`/`FromApp` (ADR-092); review fixes (ADR-093); `App.Root` as a component and `If` returning `Child` (ADR-094).
- **Phase 7 known limitations and inputs for later phases:** periodic blocking work has no off-UI-thread helper (`UseGo` + ticker); `func()` handlers cannot stop propagation; `tela snapshot` runs no effects; class props in struct-component fields need concatenation; dot-importing `h` takes names from the whole package (`Header`); `VirtualList` render returns a Node (struct rows need `Fragment`); no keyed index map (`h.Range`); `tela.Base` (compile-time check of Prop-in-Component fields) left open; `telatest.NoDev` does not affect roots built before `Mount`; plural data is CLDR 32 (x/text) while dates are CLDR 48; no Hebrew font; `Dir` on inline elements does not isolate bidi; tokens not adopted by the examples (pixels).
- **Hard stops:** the host lost power on 2026-10-21 00:38 with three agents at CPUQuota 800%; 90 empty git objects (worktree data only) were repaired without loss. Limits are back to 400% and `-p 2`; only the main session runs the race detector and qemu.
- **Phase 6 done** (2026-10-19): `examples/dashboard` (glassmorphism over an image backdrop, transitions, layout animations, springs, Presence dialog and toast) runs at 60 fps at 1080p on CPU: a headless 1920×1080 x1 scripted session of 644 frames has p95 14.6 ms on 4 CPUs (13.8 ms on 18); live at 1920×1080 physical the `fps` overlay reads 12.3–14.4 ms per frame on Wayland while scrolling (16.1–19.8 ms on X11). Wayland fractional scaling verified live at 1.5 (windows pixel-identical to `tela snapshot --scale 1.5`, scrolled frames identical to full redraws). arm64 (qemu) and the race detector pass. A `/code-review high` of the phase found 7 issues (worst: a child's own color transition snapped under an inherited change; evicted `BgImage` entries stayed reachable); all fixed with regression tests, plus T10's finding that opacity did not fade backdrop filters (ADR-084, now within 0.4 mean levels of Chromium).
- **Phase 6 delivered:** CSS grid ported from Taffy (553/553 grid fixtures match Chromium; ADR-073); raster blend modes, mask layers and focal gradients, exact COLR emoji (ADR-074); Wayland fractional scaling (ADR-075); `internal/anim`, CSS transitions, `animate-*`, reduced motion (ADR-076); `fixed`/`sticky`, `h.Portal`, color-scheme API (ADR-077); fast downsampled blur and parallel damage-aware backdrop filters, `drop-shadow` σ = radius (ADR-078); `mix-blend-mode`, `[clip-path:…]`, background images via `tela.Assets`/`tela.BgImage`, `bg-clip-text` (ADR-079); `UseSpring`/`UseTween`/`Animate`, `starting:`, `Presence`/`Exit`/`UsePresence`, `telatest.NoSettle` (ADR-080); `AnimateLayout`/`LayoutID` (ADR-081); `OnDone`, `Velocity`, `BgFn`, `TextColorFn` (ADR-082); backdrop faded by opacity (ADR-084); `examples/motion`, `examples/layout`, the new `examples/dashboard`.
- **Phase 6 known limitations:** scale 2 does not reach 60 fps while scrolling glass pages (p95 36–48 ms at 2880×1800 and 4K): backdrop blur inside a layer disables banded raster, scrolling over images/backdrops cannot pixel-shift, `drawShadow` does interior work, parallel efficiency is ~2× on 4 workers; no Windows run this phase (VM off); `wtype` keys did not reach an Xwayland window (wheel did; unverified whether Tela or wtype); 3D transforms are out of scope; `grid-template-areas` is unreachable from classes; layout animations: no crossfade on `LayoutID` handoff, inline elements not animated, breakpoints animate on every resize configure; `starting:` ignores an ancestor's `display: none` ending; exits wait only for transitions (not keyframes); a box's own `filter`/`mix-blend-mode` do not apply to its backdrop; repeated image tiles are drawn per tile (seams differ from Chromium by ~1 level at x2); color-glyph alpha only on cached sprites; `Img` still takes only `image.Image` (no files, SVG or tint).
- **API gaps found by the phase-6 examples** (scheduled: phase 7 and 8 below): `UseBounds` changes with scrolling (no scroll-independent size); no `max-*:` variants; `telatest` queries settle under `NoSettle`; no SVG/icons (phase 8); window control and tray (phase 11); tooltips (phase 8's `aria`, phase 10's `kit`); component API proposals in `API_PROPOSALS.md` (owner to decide in phase 7, T3).
- **Phase 4 done** (2026-10-13): the §1 `Counter` is interactive by mouse and keyboard, live on Wayland and X11 and through `telatest`; scrolling at 800×600 logical x2, p95 frame time with raster over 1200 scripted frames is 2.28 ms for a 100,000-row `VirtualList` and 2.58 ms for a 10,000-row plain list (target ≤ 16 ms); live in a 2832×1692 window the `fps` overlay reads 6.5 ms (VirtualList) and 8.5 ms (plain list) per frame. Windows after interactive scrolling are pixel-identical to `tela snapshot` on Wayland and X11; 0 idle CPU; arm64 (qemu) and Wine pass. A `/code-review high` of the phase found 7 issues (worst: X11 `XI_DeviceChanged` decoded at the wrong offsets, with a test that shared the mistake); all fixed with regression tests.
- **Phase 4 delivered:** a shared platform event loop and `KeyEvent` with W3C names (ADR-050); hooks with comparable handles (`Setter`, `Dispatcher`, `Callback`, `Ref`, `ElementRef`), `deps ...any`, `C`/`CP`/`Memo` skipping, `Post`, dev hook-order checks (ADR-051); `UseValue` with phase-deferred reads (ADR-053); a pure-Go XKB subset checked against libxkbcommon, Wayland keyboard with Compose and client-side repeat (ADR-052); vendored xgb, XInput2 scrolling and the X11 keyboard (ADR-054); DOM pointer events with a Flutter gesture arena and interaction variants (ADR-055); focus, Tab order and keyboard dispatch (ADR-056); scrolling with viewport scrolling, overlay/classic scrollbars, smooth wheel and kinetic touchpad (ADR-057); a semantics tree and Playwright aria snapshots, `tela inspect --aria` by default (ADR-058); `telatest` and a public headless entry point, so the CLI works outside the tela module (ADR-059); `h.VirtualList` (ADR-060); layers as cached display lists with exact damage and pixel shifts (ADR-061); debug overlays (ADR-062); five example screens (`dashboard`, `pricing`, `settings`, `player`, `board`) plus `virtuallist`. The race detector is allowed for verification only (AGENTS.md); `staticcheck` is clean and part of the checks.
- **Phase 4 known limitations:** classic scrollbar gutters square off rounded corners; positioned descendants can paint over overlay scrollbars; no `sticky`; X11 has no touchpad fling (no gesture-end signal); gradients and images in scrolled content are re-rasterized on scroll (`examples/shapes` stays ~180 ms per frame at x2); a plain 10k-row list peaks at ~840 MB RSS after mounting ~90k elements; hit-testing got ~20% slower with scroll spaces (4.2 µs on 10k); X11 multi-layout AltGr levels are approximate (core mapping, ADR-054); `telatest` cannot isolate `tela.Post` between parallel tests; `Type` produces no text until phase 5's inputs.
- **Phase 3 done** (2026-10-12): the §1 `Counter` (`examples/counter`) renders like its HTML + Tailwind twin in Chromium 152: every box edge within 0.01 px (text baselines at scale 2 within 0.5 px, ADR-048), pixels equal except glyph antialiasing (whole-image mean 0.23 levels at x1, 0.21 at x2). The window is pixel-identical to `tela snapshot` on Wayland (scale 2) and X11, 0 idle CPU ticks; arm64 (qemu) and Wine pass.
- **Phase 3 delivered:** Tailwind v4.3.3 theme generator (ADR-037); `internal/style` (ComputedStyle, interned class parser with arbitrary values, `/NN`, variants evaluated against a context, suggestions, Preflight defaults; ADRs 038, 042); `internal/layout` (block, flex, relative/absolute, intrinsic sizes, caching, relayout boundaries; 828/828 included Taffy v0.14.0 fixtures match Chromium; ADRs 039, 043); `internal/core` (Node/Element/Box, keyed reconciliation, `UseState`, frame-on-demand scheduler, paragraphs, headless render, inspect walk; ADRs 040, 044) with CSS box painting matched against Chromium on a 56-cell gallery (ADR-045) and a raster `BackdropFilter`; public `tela` and `h`; `tela snapshot` (Previews/Preview/Scene, `--dark`, `--preview`) and `tela inspect` (ADR-047). Determinism fixes: stdlib trig and color conversions replaced by `internal/fmath` (moved from `gfx/internal`), pinned by digests (ADR-041); colors are extended sRGB like Chromium (ADR-046).
- **Phase 2 done** (2026-10-12): mixed-script paragraphs (Latin, Arabic, CJK, emoji) render correctly and are covered by goldens (`internal/text`, `examples/text`).
- **Phase 2 delivered:** embedded fonts (`internal/assets/fonts`: Inter, JetBrains Mono, Noto Sans Arabic, Noto Color Emoji COLRv1; CJK subset for tests only), `gfx.DrawGlyphRun` + `gfx.Font` and the raster glyph cache (1/4 px subpixel x, whole-px baselines; ADR-029), `internal/text` font layer (variable instances, deterministic glyf/gvar port, COLRv1 to DisplayList, CSS matching, per-grapheme fallback; ADRs 030–032), Parley-style paragraph layout (bidi, HarfBuzz via go-text, UAX #14, white-space, justify, ellipsis, line-clamp, inline boxes, intrinsic sizes; ADR-034), cursors/selection/hit-testing/navigation (ADR-036), optional system fonts (ADR-035), `examples/text`. go-text pinned to an unreleased commit for a kerning/`ccmp` fix (ADR-033). `gfx.Affine` made fusion-safe.
- **Verified:** text layout matches headless Chromium on 34 of 38 reference paragraphs within 0.052 px (the rest bounded and explained); carets and selections match Chromium 152 (1,216 carets, 144 selections; 9,743 of 9,838 clicks); COLRv1 emoji within mean 0.2–1.6 levels of Chromium (flags up to 3.7, soft-light approximated). Text digests identical on amd64, amd64 v3 and arm64 (qemu); text tests pass under Wine. The `examples/text` window is pixel-identical to `tela snapshot` on Wayland (scale 2) and X11 except the CJK label (system bold CJK face); idle CPU 0 ticks.
- **Known limitations:** phase 1's list still applies (stroke self-overlap pixels, `Rotate(π/2)` not exact, conic atan per pixel, no mipmaps, `Path.Stroke` allocs, rotated shadows slow, `DrawShadow` edge clipping vs Blink, σ/offset caps, blur layers in bands, worker scratch memory; X11 back/forward lost-release ambiguity). Text: COLR blend composites (flags) and non-solid SrcIn masks are approximated and focal radial gradients emulated until `gfx`/`raster` gain blend modes, a focal gradient and mask layers (`exactBlend` in `internal/text`); line ends are not reshaped (≤ 0.4 px vs Chromium); URL breaks follow UAX #14, not Blink's table; no hyphenation, skip-ink or synthetic bold/italic; CJK glyph forms ignore language (JP vs SC); interned faces are never evicted (each variable-font instance keeps its own Face and glyph caches, so animating an axis grows memory; bound it with phase 6's animations); system font files are read fully (no mmap); serif/cursive/fantasy generics have no system mapping; go-text's `WordIterator` drops a word starting where the previous ends (worked around; report upstream); `SelectedText` returns source bytes (collapsed spaces included); an inline box at a wrapped line's edge has one side without a cursor; Windows/macOS system font discovery only cross-compiled; glyph raster switches between cached and path modes under rotation (up to 1/8 px x, 1/2 px y jump); text blending is ~77% of warm text cost (a coverage LUT would help). Phase 3: grid lays out as block and `fixed`/`sticky` as absolute/relative; inline elements contribute only their text style (no inline backgrounds, padding or borders; no atomic inline-blocks inside paragraphs; planned in phase 5); `bg-clip-text` and `url()` backgrounds (planned in phase 6) and `mix-blend-mode` are not painted; a viewport change restyles every element; cold mount of 10k elements ~55 ms (shaping 37%); Large blurs are slow: `examples/shapes` rasterizes in ~180 ms per frame at scale 2 (two `blur-3xl` glows of 28rem), so resizing lags; phase 4's damage tracking makes unchanged glows free, a faster blur is still needed for full frames.
- **API gaps found by the phase-4 examples** (to triage when planning phase 5): (1) element size/bounds in handlers or hooks (`ElementRef.Bounds()`, a `DragEvent` drop target) — the player's seek bar hard-codes its width; (2) no phase-deferred text (`TextFn`), so a time label needs its own rebuilding component; (3) CSS border triangles (transparent adjacent borders need mitered joins) draw rectangles; (4) negative `space-x-*` has no effect; (5) conditional props are awkward (`If` takes only nodes; no `Prop` alias in `h`); (6) no indexed `Map`; (7) `Value.Get` in a handler returns the last applied value, not the latest set; (8) inspect rects ignore transforms.
- **Phase 5 done criteria** (2026-10-13): every example runs on real Windows 11 (VM, job runner): 9 of 11 windows are pixel-identical to `tela snapshot` and the other two differ for known reasons (player's live progress, text's system CJK fonts); all example test binaries pass natively with bit-exact goldens. Japanese typed through the IME into a real `Input`: Mozc on Hyprland (にほんご → 日本語, `OnChange` with `IsComposing`) and Microsoft IME on Windows in `examples/signup` (preedit underlined, candidate list under the caret, 日本語 committed). Wine runs of the examples started cleanly before Wine was banned locally.
- **Phase 5 delivered:** win32 backend (window, DPI, pointer, cursors, keyboard with layouts/AltGr/dead keys, clipboard, settings, IMM32 IME; ADRs 063, 067, 071); Wayland clipboard (ADR-064) and `text-input-v3` IME (ADR-068); `internal/text/edit` (ADR-065); inline decorations and atomic inline-blocks (ADR-066); `Input`, `Textarea`, `Form`, `Label` with the owner's API (ADR-069); the phase-4 API gaps (ADR-070); CI workflows (unverified until pushed); `examples/signup`; the VM job runner (outside the repo).
- **Phase 5 known limitations:** no local Wine (three hard freezes of this host on 2026-10-13 started seconds after Wine launches, suspected `ntsync` on kernel 7.2; CI keeps its Wine job); `invalid:` without `required`/`pattern`; `number` inputs are hints only; no X11 IME (phase 13); untested: Korean/Chinese IMEs on Windows, IMM32 reconversion, DPI ≠ 1 IME placement; `telatest` cannot register a CJK font, so goldens show CJK only through system fonts in live windows; left Ctrl+Alt is not AltGr on Windows (contract choice); a paragraph with atomic inlines re-breaks lines on every layout request; `UseBounds` readers see changes one frame late.
- **Phase-5 plan:** T0 VM job runner · T1 win32 base · T2 Wayland clipboard · T3 `internal/text/edit` · T4 inline decorations and atomic inline-blocks · T5 win32 keyboard, clipboard, settings · T6 IME contract + Wayland `text-input-v3` · T7 `Input`/`Textarea`/`Form`/`Label` · T8 IMM32 IME · T9 phase-4 API gaps (all eight) · T10 CI (Wine, `windows-latest`) · T11 forms example and Windows validation. Waves: A (T1–T4), B (T5, T6), C (T7, T9), D (T8, T10, T11).
- **Phase-6 plan** (approved 2026-10-18; owner's API decisions taken up front, transforms stay 2D, component-API proposals deferred to before phase 7): T1 grid (tela-hard, ADR-073) · T2 raster blend modes, mask layers, focal gradient (ADR-074) · T3 Wayland fractional scaling (ADR-075) · T4 `internal/anim`, CSS transitions, `animate-*`, `motion-reduce` (ADR-076) · T5 fast large-σ blur (ADR-078) · T6 core `mix-blend-mode`, `[clip-path:…]`, background images via `tela.Assets`/`tela.BgImage`, `bg-clip-text` (ADR-079) · T7 `UseSpring`/`UseTween`/`Animate`, `Presence` with `starting:` and `Exit`, `UsePresence` · T8 `fixed`, `sticky`, `h.Portal`, `UseColorScheme`/`SetColorScheme`/`UseReducedMotion` (ADR-077) · T9 `AnimateLayout`, `LayoutID` · T10 dashboard example, 60 fps at 1080p, live and Windows checks. No limit on parallel agents (owner, 2026-10-18).
- **Roadmap revision (2026-10-20):** a gap analysis against JavaFX, the Microsoft stack, Wails, Tauri and Electron, and a review of `examples/` for public-API problems, produced the plan below. Phase 7 is new (public API polish, before `aria` and `kit`, with theme tokens inside it); the old phases 7–8 and the desktop-integration, distribution and media phases were split by size (phases 5 and 6 had 10–11 tasks); desktop integration comes before macOS; "Road to v1.0" says when the API freezes. Phases 8 onwards are unplanned; order and scope are confirmed when each phase is planned. `tela add` is deferred. Phase numbers moved: "phase 8" in older text (macOS, X11 IME) is now phase 13.

## Phases

Each phase ends with verifiable criteria. A phase is not done unless its criteria pass with `CGO_ENABLED=0`. Before starting a task, read the matching row in ARCHITECTURE §20 (references).

### Phase 1: Pixels on screen

- [x] `internal/gfx`: geometry types, `Color` (hex and OKLCH parsing), `Path`, `RRect`, `DisplayList` and its commands
- [x] `internal/gfx/raster`: analytic-AA path fill, rect/rrect fast paths, linear/radial/conic gradients, clip, transform, opacity layers
- [x] Stroking (joins, caps, dashes) following kurbo (ADR-026)
- [x] Analytic erf rounded-rect shadows (incl. inset); separable Gaussian blur for other shapes (ADR-028)
- [x] Parallel tile rasterization (horizontal bands, dynamic scheduling)
- [x] `internal/platform`: `Backend`/`Window` interface and `headless` backend
- [x] Wayland XML → Go protocol generator (+ `wire` runtime)
- [x] `internal/platform/wayland`: connection, registry, `xdg_shell`, `wl_shm` via memfd, frame callbacks, pointer, integer scale
- [x] `internal/platform/x11`: window, `PutImage`, basic events
- [x] `examples/shapes` and a first `tela snapshot` (no UI tree yet: a fixed DisplayList, contract `Scene(w, h float32) *gfx.DisplayList` until `Preview()` in phase 3)

**Done when:**
1. A scene with rounded cards, gradients, shadows and blur looks identical on Wayland, X11 and the headless PNG.
2. Rasterizer goldens pass.
3. CPU is at 0% when idle.

### Phase 2: Text

- [x] Embedded fonts (sans, mono, emoji) with licenses in `internal/assets/fonts`
- [x] Shaping and bidi with `go-text/typesetting`; UAX #14 line breaking; font fallback
- [x] Parley-style text layout: styled runs → lines, cursor positions, selection rects, hit-testing
- [x] Glyph raster cache with subpixel positioning
- [x] Ellipsis and line-clamp
- [x] Optional system font discovery

**Done when:** mixed-script paragraphs (Latin, Arabic, CJK, emoji) render correctly, covered by goldens.

### Phase 3: Tree, style, layout

- [x] Tailwind v4 theme generator (`go generate`): default theme + class vocabulary
- [x] `internal/style`: `ComputedStyle`; class parser (spacing, sizing, colors, borders, radius, shadows, typography, flex, grid); arbitrary values; `/NN` opacity; theme and tokens
- [x] Suggestions for unknown classes
- [x] `internal/layout`: constraints protocol, block, full flex, absolute/relative positioning; port Taffy flex fixtures
- [x] `internal/core`: Node/Element/Box, keyed reconciliation, phased scheduler with dirty flags
- [x] Packages `tela` and `h`:
  - `Run`, `App`, `Node`, `Child`
  - Elements: `Div`, `Span`, `Text`, `Textf`, `H1`–`H6`, `P`, `Button`, `Img`
  - Props and helpers: `Class`, `Style`, `Key`, `If`, `Map`, `Fragment`
- [x] `tela snapshot` and `tela inspect --layout` on components (`Preview()` contract)

**Done when:** the `Counter` example from ARCHITECTURE §1 (non-interactive) renders identically to its HTML + Tailwind equivalent, compared visually against browser screenshots.

### Phase 4: Interaction and state

- [x] Hooks:
  - `UseState`, `UseReducer`, `UseEffect`, `UseLayoutEffect`, `UseMemo`, `UseCallback`, `UseRef`, `UseContext`, `UseId`
  - Component boundaries `tela.C` and `tela.CP`
  - Dev-mode detection of invalid hook order
- [x] Automatic skipping for comparable props; `UseValue` with phase-deferred reads
- [x] Input:
  - Hit-testing, event bubbling, `OnClick` and friends
  - Gesture arena (tap, drag, long-press)
- [x] Variants `hover:`, `active:`, `focus:`, `focus-visible:`, `disabled:`, `group-*`
- [x] Keyboard:
  - Focus and tab order
  - Physical/logical/text model
  - XKB parser (Wayland) and X11 keyboard
  - Vendor a patched `jezek/xgb` (core, `xproto`, `bigreq`) that reads GenericEvents, before XInput2 (ADR-025)
- [x] Scrolling:
  - Scroll (`overflow-auto`) with its own layer and smooth scrolling (scrolling done in T7a; the layer is T7b)
  - Repaint boundaries and damage tracking (done in T7b)
  - `h.VirtualList` (done in T8)
- [x] Semantics tree; `tela inspect --aria` (Playwright-style YAML)
- [x] `telatest`: `Mount`, `Click`, `Type`, `ByRole`, `ByText`, `Golden`, `AriaSnapshot`, controllable clock, auto-wait
- [x] Debug overlays (`TELA_DEBUG=layout,paint-flash,layers,fps,semantics`)

**Done when:** `Counter` is interactive, and a 100,000-row `VirtualList` (10,000 without virtualization) scrolls smoothly on CPU rendering.

### Phase 5: Windows and text editing

- [x] `win32` backend: window, message loop, present, per-monitor DPI v2, keyboard, mouse, cursor, clipboard
- [x] `Input` and `Textarea`: cursor, selection, undo/redo, clipboard
- [x] IME on Wayland (`text-input-v3`) and Windows (IMM32)
- [x] Inline element decorations in paragraphs: backgrounds, padding, borders and radii of `Span`/`Code`/`A` (e.g. `<code class="bg-slate-100 px-1 rounded">`), and atomic inline-blocks (buttons, badges) inside a paragraph (revisits ADR-044)
- [x] CI with `go test -exec wine` and a `windows-latest` runner

**Done when:** examples run on Windows (Wine and real), and Japanese text can be typed via IME on Wayland and Windows.

### Phase 6: Advanced CSS and animation

- [x] Full grid (port Taffy grid fixtures) (T1)
- [x] `fixed` and `sticky` positioning (T8)
- [x] Animation:
  - [x] Transitions (`transition`, `duration-*`, `ease-*`) (T4)
  - [x] `UseSpring`, `UseTween`, `Animate`, `Presence` (T7; `UseAnimation` became `UseTween`/`Animate` by the owner's choice)
  - [x] `animate-*` classes (T4)
- [x] Layout animations: `AnimateLayout`, `LayoutID` (T9)
- [x] Effects (T2 raster blend modes, T6 core; transforms stay 2D by the owner's choice): `backdrop-filter`, `filter`, `mix-blend-mode`, basic `clip-path`, full transforms
- [x] Background images (`bg-[url(...)]`, `bg-cover`/`contain`, position) and `bg-clip-text` (gradient text)
- [x] Portal layer, z-index and stacking contexts (T8; z-index and stacking contexts since phase 3)
- [x] Responsive breakpoints, `dark:`, `motion-reduce:` (variants since phases 3–4; T4 reduced motion, T8 color scheme API)
- [x] Wayland fractional scaling (live at scale 1.5 on Hyprland 0.56: `settings` and `virtuallist` windows pixel-identical to `tela snapshot --scale 1.5`, scrolled frames identical to full redraws)

**Done when:** a "dashboard" example with glassmorphism, layout animations and transitions runs at 60 fps at 1080p on CPU rendering.

### Phase 7: Public API polish (`v0.7.0`)

Goal: remove the traps that compile and fail silently, the boilerplate every
example repeats, and the naming collisions, while the user base is one
repository. Evidence: the review of 14 examples.
Rendering must not change: every golden stays bit-identical.

#### T1 Safety net (additive)

- [x] `tela.Main(app)`: runs `Run`, prints the error, exits 1 (12 of 13 examples drop `Run`'s error today)
- [x] Dev mode: `App.Dev`, on by default when the binary is not a release build (detected with `debug.ReadBuildInfo`; the exact rule is an ADR); `telatest.Mount` defaults to dev with `NoDev()` to opt out. Unknown classes, roles, keys and `Ease` strings then report
- [x] `telatest.Mount` given a component call (`Mount(t, Counter())`) says "wrap the root in `tela.C`" instead of the misleading hook panic
- [x] `IsZero()` on `Setter` and `Callback`; a dev warning when `UseContext` reads a context with no Provider and a zero-unsafe default (`dashboard` keeps a `Setter` in a context default)
- [x] Dev warning for a `Label` that neither wraps nor points at a control, and for keyless or duplicate-key lists
- [x] "Mount only" idioms: `UseMount(f)`, `UseOnce(init)`
- [x] Effects without cleanup or without the first run: `UseEffectDo(f, deps...)`, `UseUpdateEffect(f, deps...)`; periodic work with `UseInterval(d, f)` or an effect that receives a `context.Context`
- [x] Docs: fix stale or contradictory text (Getting-Started `Preview` form, `shapes` "phase 6" comment, `UseValue(0.0)` in the package doc, the sealed-`Child` error message); document `UseValue` structs as stable custom-hook state, `Fragment` for wrapper children and `If(cond, Class(...))`

#### T2 Ergonomics (additive; existing code keeps compiling)

- [x] `Class(parts ...string)` joins with one space and skips empty parts (141 concatenation sites; two opposite trailing-space conventions in one package)
- [x] Components take props: `CP(fn, props, Key(id))`, `C(fn, Key(id))`, `Memo(..., Key(id))`; `MapKeyed(items, keyFn, render)` (11 `WithKey(CP(...))` sites)
- [x] Handlers without the event: `OnClick`, pointer and key handlers accept `func()` or `func(Event)` (about 50 of 61 handlers ignore the event); `Toggle(setter)` (six `Set(!x)` sites lose a click when two land in one frame)
- [x] `Bind` accepts `func(string)` so a `Dispatcher` can use it; one shape for `Setter` and `*Value`
- [x] `type Easing string` with constants and `CubicBezier(...)`; `UsePresenceTween` for the manual-exit toast (copied verbatim in `motion` and `dashboard`); `Scale`/`Map` helpers for `Value` so `HeightFn(h.Scale(36))` replaces the closure; `UseSpring(target, …)` infers its type
- [x] Style packs: `tela.Box(x, y, w, h)`, `tela.Size(w, h)`; a `StyleProp` accepted directly as a child
- [x] `h.Nodes([]tela.Node)` to pass a node slice as children
- [x] `VirtualList`: `ItemKey` so state follows the item, not the index; `list`/`listitem` roles by default; `ItemHeight(px)` for fixed rows
- [x] `telatest`: `Bounds` returns `tela.Rect` (as `ElementRef.Bounds` does), `MountFunc`, `GoldenScales(name, 1, 2)`, `h.TestID` + `ByTestID`, `ClickAt`/`DragFromTo`, `PixelAt`, `Measure(func())` for build/layout deltas, `Checked(true)`-style state asserts instead of string matching on the aria snapshot

#### T3 Source-breaking decisions (owner's choice and an ADR each)

Present as options in `API_PROPOSALS.md` before any agent starts, as with the
hooks and text-field shapes.

- [x] `Key[K comparable]` / `WithKey` generic, so a non-comparable key fails at compile time instead of panicking
- [x] ~~`Value.Get()` inside handlers and effects returns what `Latest()` returns~~ (owner kept `Get`; dev warns on pending changes instead, ADR-085) (three examples work around the stale read; `player` has a latent bug); builds and `…Fn` closures keep the applied value
- [x] Reject `nil` as a dependency list (deprecated with a dev warning and `tela.Once`; rejected in phase 8)
- [x] Name collisions between `h` and `tela` under dot-import: `Key`, `Code`, `Value`, `Ref`, `Id`/`UseId` against Go's `ID`, `telatest.Scale/Dark/ColorScheme`; decide which side renames
- [x] Remove `UseStateSet`/`UseStateUpdate` (unused in 29 `UseState` calls) or document why they stay
- [x] One `Preview` convention (component call vs `tela.C(...)`); `tela snapshot` reads `var App = tela.App{...}` for size, `RootClass` and `ColorScheme`, so the PNG matches the window
- [x] The component API of `API_PROPOSALS.md` (already pending)
- [x] A `Deprecated:` period of one phase for anything removed, then removal in phase 8 (`Id`, `UseId`, `tela.Key`, `tela.Code`, nil deps)

#### T4 Theme tokens (design decision, ADR)

Prerequisite of `kit`. Today a theme is a struct of class strings
reflection over `Look` structs (`examples/native`)
or doubled `dark:` pairs (`settings`, `pricing`, `virtuallist`).

- [x] `App.Theme`: semantic color, radius and font tokens with light and dark values, usable as classes (`bg-surface`, `text-fg`, `bg-primary`), following the color scheme without `dark:` pairs
- [x] `App.ClassAliases` so `text-muted` can expand to a class set
- [x] A scoped scheme for a subtree (`Scheme(tela.Dark)`), so a dark panel can sit in a light app
- [x] `UseTheme()` to read tokens in Go (hex values for tinting icons and canvases)

#### T5 Previews and seeded state

- [x] Start a preview in a given state without a global variable or a second provider that copies the real one
- [x] `tela snapshot` and `telatest.MountApp` read the same `App` as `Main`
- [x] Shape decided with the owner (options: `Previews` entries with initial state and a wrapper, or a `tela.Seed` context)

#### T6 Migration and validation

- [x] Migrate all 14 examples; their goldens stay bit-identical
- [x] CHANGELOG "Migration" section with before/after for every breaking change

#### T7 i18n / l10n (pure Go, independent of the rest)

- [x] Message catalogs, plurals, locale-aware date/number/list formatting, runtime locale switching, `rtl:` layout mirroring

**Done when:**
1. All examples build and pass with the new API, goldens unchanged.
2. No example drops `Run`'s error, concatenates class strings, wraps `CP` in `WithKey` or uses an event-ignoring handler.
3. A typo in a class, role, key or `Ease` is reported in a default `go run` of a development build and in every `telatest` test.
4. Previews need no global.
5. Every removed symbol has a `Deprecated:` release behind it, and the CHANGELOG says how to migrate (no automatic rewriter; the owner chose release notes).
6. A translated example switches locale at runtime, formats plurals, dates and numbers per locale, and mirrors its layout under `rtl:`.

---

### Phase 8: Icons, SVG, `aria` (`v0.8.0`)

Built on the polished API and the theme tokens. Foundation for `kit`.

- [x] SVG rendering: paths, gradients, `currentColor`, transforms, `viewBox`; parsing and rasterizing on the existing `DisplayList` (conformance checked against Chromium like the other renderers) (T1, ADR-095; `<style>`/`<switch>` in T4)
- [x] `Icon` component with a bundled icon set (licenses in `THIRD_PARTY.md`) (T4 `h.Icon`/`tela.Icon`, ADR-098; T6 `tela icons gen` + Lucide as the separate module `icons/lucide`, ADR-102)
- [x] `Img` from files, `embed.FS`, bytes and SVG, with **tint** (T4: `SrcPath`/`SrcBytes`/`SrcFS`, `Tint`, `mask-*`, `object-fit`)
- [x] `aria` behavior hooks: `UsePress`, `UseHover`, `UseFocusRing`, `UseFocusScope`, `UseOverlay`, `UseDialog`, `UseTooltip` (T5a, T5b; plus `UseButton`, toggles, `UsePopover`, `UseOverlayTrigger`, `UseMenuTrigger`, `ElementRef.Click`)
- [x] `aria` collections: `UseListBox`, `UseMenu`, `UseSelect`, `UseTabs`, `UseCombobox` (T8a, T8b; `aria.Selection[K]`)
- [x] `aria.UseFloating` (Floating UI port: offset, flip, shift, size, arrow) (T7 on T2's same-frame engine)
- [x] Phase-6 API gaps still open: scroll-independent `UseBounds`, `max-*:` variants, `telatest` queries under `NoSettle` (T2 `UseSize`, T3)
- [x] Phase-7 deprecations removed (`Id`, `UseId`, `tela.Key`, `tela.Code`; `nil` deps panic in dev) and inputs: `h.Range`, `VirtualList` struct rows, `tela.StopPropagation` (T3)

**Done when:** an SVG icon set renders within a few levels of Chromium at scales 1 and 2, and a dialog, a menu and a combobox built only from `aria` hooks pass keyboard and aria-snapshot tests.

### Phase 9: Dev loop, `tela dev` (`v0.9.0`)

Moved before `kit` by the owner (2026-10-21): iterating on components with their state kept pays off from `kit` on. Options in `DEVMODE_PROPOSAL.md` (G1–G9), decided by the owner on 2026-10-22; recorded in ADR-111. Release builds must not change: the dev loop compiles only under a `teladev` build tag. Go cannot swap code in a running process without cgo, so the build-and-restart is made invisible instead.

- [x] `tela dev [pkg]`: watch, build in the background while the running app stays interactive, swap only after a successful build
- [x] The window survives reloads: a supervisor owns it and runs the app as a child over a remote backend (shared-memory frames, proxied events, clipboard, IME, settings); the new child's first frame replaces the old one with no visible close or reopen
- [x] State across reloads: hook slots (`UseState`, `UseReducer`, `UseValue`, `UseRef`) matched by component path, hook index and type, plus fields, scroll, focus, scheme, theme and locale; a component whose hooks changed resets (React Fast Refresh's rule); dev mode reports slots that cannot be kept; an app-level save/restore hook for state outside the tree
- [x] Build errors and panics shown as an overlay over the last good frame, never closing the window
- [x] `--preview NAME` to iterate on one `Previews` entry; terminal keys for reload, reload without state, quit

**Done when:** editing `examples/dashboard` (a class, a component's markup, a new hook) updates the open window within 1.5 s of saving with no visible close or reopen, the scroll position, focused field and typed text kept; a build error shows over the last good frame and the next good save clears it; on Linux (Wayland and X11) and Windows; a release build of the same app has the same size as before the phase (no dev-loop code linked). Stretch goal: under 1 s.

### Phase 10: `kit` (`v0.10.0`)

Shipped as a package of the core module. `tela add` is deferred (see "Deferred"),
so each component is written as one self-contained file with no imports from
`internal/` and tokens instead of hard-coded colors; that keeps a later move to
copied-source distribution cheap. Design decided by the owner on 2026-10-25 (K1–K17, ADR-129):
struct components with slots, shadcn/ui `new-york-v4` as the look checked against Chromium,
recipes (`kit.DefaultStyle`, `kit.StyleProvider`) for customization, the app-screen examples
migrated.

- [x] T1 foundation: shared types, recipes, `kit.Theme()`, Chromium harness, golden sheets; Button, Badge, Card, Avatar, Label, Input, Textarea, Separator, Skeleton, Field
- [x] T2 engine: `h.AriaInvalid` + `aria-invalid:`, `h.AriaSort`, `i18n.FirstDayOfWeek`
- [x] T3 aria: `UseRadioGroup`/`UseRadio`, `UseToggleGroup`, `UseSlider`, `UseProgressBar`/`UseMeter`
- [x] T4 Checkbox, Switch, RadioGroup, ToggleGroup, Slider, Progress
- [x] T5 Dialog, AlertDialog, Popover, Tooltip, Toaster/`UseToast`, Select, Tabs
- [x] T6 aria `UseMenuBar`, `UseContextMenuTrigger` (ADR-133); DropdownMenu, Menubar, ContextMenu (ADR-141)
- [x] T7 Table (sort, sticky header and leading columns)
- [x] T8 aria `UseCalendar`/`UseRangeCalendar` (ADR-134); Calendar, DatePicker (ADR-142)
- [x] T9 aria `UseColorArea`/`UseColorSlider` (ADR-135); ColorPicker, ColorInput (ADR-143)
- [x] D1–D5 examples on kit: dashboard (glass style, ADR-144), settings + pricing (ADR-145), signup + player (ADR-146), board (ADR-147), i18n (ADR-148), native (three styles, ADR-149)
- [x] D6 `examples/kit` gallery (a `Previews` entry per component), wiki Kit page, README (ADR-150); RTL sweep of the recipes (ADR-151)
- [x] Light and dark previews of every component as goldens

**Done when:** `examples/dashboard` (a glass style) and `examples/native` (Win11, Adwaita, Breeze styles) are built only on `kit`, `examples/settings` uses the default style, no migrated example has a hand-made control (segmented control, switch, radio, confirm row, toast, menu) or hand-drawn icon, and hello, counter, shapes, text, layout, motion, virtuallist and aria keep bit-identical goldens.

### Phase 11: CLI tooling, LLM docs, full screens (`v0.11.0`)

- [ ] `tela lint`: static check of class strings, roles and keys, so typos fail in CI even with dev mode off
- [ ] `tela mcp`: MCP server over `snapshot`, `inspect` and `telatest` queries
- [ ] `docs/llms.txt` matching the final API
- [ ] `examples/` with full screens (dashboard, login, settings, chat)

**Done when:** an agent with no prior context, given only `llms.txt` and `tela mcp`, builds a beautiful settings screen in under 3 snapshot iterations.

### Phase 12: Desktop essentials (`v0.12.0`)

Linux (Wayland, X11) and Windows. macOS items are finished in phase 14.

- [ ] App lifecycle: `tela.Quit()`, `tela.OpenURL/OpenPath` (default application), close-to-tray
- [ ] Window control: minimize, maximize, fullscreen, move, resize, always-on-top, min/max size, a custom title bar with drag regions and window-control buttons
- [ ] Multiple windows (modal and child windows, per-window state)
- [ ] Native dialogs: open/save file and folder pickers, message boxes (XDG Desktop Portal, common item dialogs)
- [ ] System tray and OS notifications (StatusNotifierItem, `Shell_NotifyIcon`)
- [ ] App data: per-OS config/data/cache paths, a small settings store with change events, persisted window state

**Done when:** a "notes" example uses a tray icon, native open/save dialogs, two windows and a persisted window position on Linux and Windows.

### Phase 13: Desktop integration, the rest (`v0.13.0`)

- [ ] Drag and drop with the OS (files in, out, between windows); clipboard beyond text (images, files, rich text)
- [ ] Native application and context menus where the OS has them, with a drawn fallback
- [ ] Global shortcuts, single-instance apps, deep links and file associations
- [ ] System theme and accent color, high contrast, text scale, with change notifications
- [ ] Secure storage: Credential Manager, Secret Service (pure Go, no cgo)

**Done when:** the "notes" example accepts a dropped file, registers a global shortcut, reopens in its running instance from a deep link and keeps a token in secure storage, on Linux and Windows.

### Phase 14: macOS (and X11 IME) (`v0.14.0`)

- [ ] `cocoa` backend via purego, with CI on `macos-latest`: window, event loop, present, HiDPI, keyboard, pointer, clipboard, settings
- [ ] IME on macOS (`NSTextInputClient`) and on X11 (XIM)
- [ ] macOS parity for every phase-12 and phase-13 item

**Done when:** every example runs on macOS with snapshots pixel-identical up to text antialiasing, Japanese can be typed through the IME on macOS and on X11, and the phase-12/13 examples run on all three platforms.

### Phase 15: Accessibility (`v0.15.0`)

Strictly after phase 14: the bridges for AT-SPI, UI Automation and NSAccessibility are verified together (the owner chose strict order over starting the Linux and Windows bridges in parallel).

- [ ] AT-SPI over D-Bus (pure Go)
- [ ] UI Automation over COM
- [ ] NSAccessibility
- [ ] The semantics tree covers `aria` and `kit` roles, states, actions and live regions

**Done when:** a screen reader reads, navigates and operates the `signup` example on Linux, Windows and macOS.

### Phase 16: GPU backend (`v0.16.0`)

Independent of phases 14–15 and the hardest technical item (`tela-hard`).

- [ ] Vulkan (Linux, Windows) and Metal (macOS) via purego/syscall over the same DisplayList, with the fixed shader set of ARCHITECTURE §13
- [ ] D3D11 on Windows optional
- [ ] The CPU rasterizer stays the reference for goldens; GPU output is checked against it within a tolerance

**Done when:** the `dashboard` example renders at 60 fps at scale 2 on the GPU backend (the CPU one does not reach it) and GPU snapshots stay within the agreed tolerance of the CPU goldens.

### Phase 17: Packaging and auto-update (`v0.17.0`)

- [ ] `tela build` / `tela package`: installers and bundles (`.msi`/MSIX, `.app`/`.dmg`, AppImage, flatpak, `.deb`), icons, metadata, code-signing hooks
- [ ] Windows targets are GUI programs by default (`-H=windowsgui`, no console window next to the app; ADR-127), with panics and the app's log written to a file in its data directory, since a GUI program has no standard output
- [ ] Auto-update: signed manifests, delta or full downloads, atomic replace, rollback
- [ ] Crash reporting hook and structured logging

**Done when:** a sample app is packaged for three platforms from one command and updates itself from a signed manifest.

### Phase 18: DevTools (`v0.18.0`)

- [ ] Live inspector: element tree, computed style, hooks and state, re-render counts, accessibility tree
- [ ] Frame profiler
- [ ] Connect to a running app and pick an element

**Done when:** DevTools shows why a given element re-rendered and where a frame spent its time.

### Phase 19: Canvas and PDF (`v0.19.0`)

Both sit on the `DisplayList`, so they are cheap next to the rest.

- [ ] `Canvas` (2D): immediate-mode drawing surface (paths, gradients, text, images) with an animation-frame callback
- [ ] Printing and PDF export: a vector PDF backend over the DisplayList (fonts embedded, gradients, clip, opacity), print dialogs, page-break CSS subset

**Done when:** a `Canvas` example animates at 60 fps and a page exports to a PDF that two viewers render identically.

### Phase 20: Audio and video (`v0.20.0`)

Highest uncertainty of the roadmap: pure-Go decoders limit the codecs.

- [ ] Audio playback and capture: pure-Go decoders (WAV, MP3, OGG/Vorbis, FLAC, Opus where one exists), output through ALSA/PulseAudio/PipeWire, WASAPI, CoreAudio
- [ ] Video playback: patent-free subset first (VP8/VP9/AV1 if a pure-Go decoder is viable), A/V sync, a `Video` component with `kit` controls; an ADR settles codec scope before work starts
- [ ] 3D (exploratory, ADR first; transforms stay 2D until then) lives here or in the backlog

**Done when:** a "player" example plays an audio file and a video with synced controls.

## Road to v1.0

`v1.0.0` is not scheduled; these gate it.

- Phase 7's breaking changes are done and their deprecations removed. From phase 10 (`kit` on the final API) on, **breaking changes need an ADR and a deprecation cycle**.
- The API freezes when phases 7–15 are done: stable `tela`, `h`, `aria`, `kit` and `telatest`; Linux (Wayland and X11), Windows and macOS supported; accessibility working on all three. The GPU backend (16) changes no public API and does not gate it.
- Godoc and wiki cover every exported symbol with a runnable example; `llms.txt` matches the API.
- At least two non-trivial applications outside the repository build against each release in CI.

## Separate packages

Optional, outside the core module. Until `tela add` exists (see "Deferred") they are plain Go modules that applications import. They are not scheduled; each starts when a real application needs it.

- **Charts:** line, bar, area, pie, scatter, axes, tooltips, built on `Canvas` (phase 19) or plain `Div`/SVG until then
- **DataGrid / TreeView:** sortable, resizable, editable cells, row selection, grouping, tree expansion, on top of `VirtualList`; the simple `Table` in `kit` stays in core

## Deferred (not discarded)

Out of scope for now, but kept open: a decision to add the platform items needs an ADR amending the non-goals in ARCHITECTURE §1, and the platform layer must not preclude them.

- **WebAssembly / web:** a canvas-backed backend
- **Android / iOS:** touch, virtual keyboard, app lifecycle, safe areas
- **Embedded web content:** a webview or HTML component (contradicts "no webview", ADR-001)
- **`tela add` and a component registry.** Picked up when `kit` has real users or a third party wants to publish components. `kit` is written meanwhile as self-contained files (no `internal/` imports, tokens instead of fixed colors), so a move to copied-source distribution stays cheap. Design questions for the ADR (options go to `API_PROPOSALS.md`):
  - **Unit of distribution:** copied source (shadcn style), Go modules, or a hybrid (copied source for UI components, modules for logic-heavy packages)
  - **Registry format:** static JSON index plus files over HTTPS with content hashes, the Go module proxy, or a git repository
  - **Updates:** a lockfile with per-file hashes and a three-way `tela diff` / `tela update`, so local edits survive
  - **Dependencies:** between items, on Go modules (pure Go only) and on theme tokens
  - **Trust:** diff before writing, no install-time code execution, hash pinning, optional signatures, a registry allowlist, a license recorded per item
  - **Third-party registries**, registry authoring tools, and non-interactive / `--json` modes with `tela mcp` tools for agents

## Discarded

Considered on 2026-10-20 and not planned. They can be reopened by the owner.

- **Visual designer** (Scene Builder / XAML Designer style): `Preview()`, `tela snapshot` and `tela inspect` serve the same loop for code and agents
- **Component gallery** (Storybook style): partly covered by `Preview()` and the `examples/`

## Session log

One line per session: date, what was done, what is pending.

- **2026-10-11:** project created with ARCHITECTURE and ROADMAP. Docs moved to English; references (§20) and ADRs 012–021 added. No code yet.
- **2026-10-11:** phase 1 implemented (gfx, raster with shadows/filters/parallel bands, platform + headless, Wayland, X11, auto selection, `examples/shapes`, `tela snapshot`); ADRs 022–028. Verified: goldens bit-identical on amd64 and arm64 (qemu), shadows within ±3 of Chromium away from edges, Wayland and X11 windows pixel-identical to `tela snapshot`, 0% idle CPU. A test that peaked at 2.8 GiB froze the machine next to the qemu run; fixed. Windows VM run passed the same day: phase 1 done. A `/code-review high internal/platform` found 8 defects (worst: Wayland loop spinning when a frame is requested without a Present), all fixed with regression tests and checked live on Hyprland.
- **2026-10-12:** phase 2 implemented and verified (fonts, glyph raster cache, `internal/text` font layer, paragraph layout, cursor/selection, system fonts, `examples/text`); ADRs 029–036; `gfx.Affine` fusion fix; go-text pinned to e49e2bd. Checked against Chromium (layout, carets, emoji), arm64 (qemu), Wine, and Wayland/X11 windows on Hyprland. Phase 2 done. A `/code-review high` of phase 2 found 7 issues (worst: a malformed cmap range hung the system font scan; the first system font scan blocked registered-font queries under the collection lock); 6 fixed with regression tests, face eviction recorded as a known limitation. Pending: phase-3 plan.
- **2026-10-12:** phase-2 `/code-review` fixes; phase 3 implemented and verified (Tailwind theme generator, `style`, `layout` with Taffy fixtures, `core` with CSS box painting against Chromium, `tela`/`h`, `tela snapshot`/`inspect`, `examples/counter`); ADRs 037–048. Found and fixed cross-architecture differences in stdlib trig and color conversions (ADR-041) and switched to Chromium-like extended sRGB colors (ADR-046). Phase 3 done. A `/code-review high` of phase 3 found 7 issues (worst: a data race between raster bands on per-pixel backdrop filters; a layout cache key collision between negative available space and known sizes); 6 fixed with regression tests, the region-render cost recorded as a known limitation. Pending: phase-4 plan.
- **2026-10-12:** versioning adopted (ADR-049): `CHANGELOG.md`, `tela version`, retroactive tags v0.1.0–v0.3.0. `examples/shapes` rewritten with the phase-3 public API (window pixel-identical to `tela snapshot` on Wayland and X11, 0 idle CPU); the phase-1 scene became a raster golden test. Pending: phase-4 plan.
- **2026-10-12:** `examples/shapes` made responsive (fixed-size cards wrap; tiled 701x846 windows overflowed); a suspected Wayland hang was not reproducible on an empty visible workspace (same tiled layout, resizes, closes in ~100 ms) and was most likely the test script addressing windows by title while the user ran the same example. Pending: phase-4 plan.
- **2026-10-12:** phase-3 leftovers: `calc()` of a length and a percentage in `layout` (`UnitCalc`, a Chromium-measured case) and in core's mapping (v0.3.1); inline decorations assigned to phase 5, background images and `bg-clip-text` to phase 6. Pending: phase-4 plan.
- **2026-10-12/13:** phase 4 implemented and verified (T1–T12 plus five example screens): platform loop, hooks, `UseValue`, XKB and keyboards, X11 XInput2, pointer input and gestures, focus, scrolling, layers and damage, semantics, `telatest`, `VirtualList`, debug overlays; ADRs 050–062. The owner chose the hooks' API (comparable `Setter` with `Set`/`Update`, wrappers, `Dispatcher`, latest-fn `Callback`); later API decisions were taken while the owner was away and are listed for review. Verified live on Hyprland (Wayland and X11), arm64 (qemu), Wine, `-race`. A `/code-review high` found 7 issues, all fixed. Phase 4 done; CHANGELOG moved to v0.4.0 and tagged `v0.4.0` with the owner's approval.
- **2026-10-13:** owner's `examples/hello` reviewed (English copy, `Preview()`, README, goldens, 800x450) and used as the README hero (`docs/images/hello.png`); README polished for GitHub (clone step, Windows/macOS wording). `LICENSE` added (BSD-3-Clause, 2026, Andres Martinez) with a README section. Pending: phase-5 plan.
- **2026-10-13:** phase 5 planned (owner chose the text-field and API-gap shapes up front) and implemented in parallel worktrees (T1–T11): win32 backend with keyboard, clipboard, settings and IMM32 IME; Wayland clipboard and text-input-v3; `internal/text/edit`; inline decorations; `Input`/`Textarea`/`Form`/`Label`; phase-4 API gaps; CI workflows; `examples/signup`; ADRs 063–071. Verified on real Windows 11 through a VM job runner and live on Hyprland (Mozc). The host hard-froze three times; the trigger was local Wine runs (now banned; suspected ntsync), not memory. `/code-review high` found 9 issues (worst: Alt+numpad characters lost on Windows, Alt chords typing into fields), all fixed with regression tests (ADR-072). CHANGELOG moved to v0.5.0.
- **2026-10-14:** `v0.5.0` tagged on 8a4bf5d with the owner's approval. Pending: phase-6 plan.
- **2026-10-14:** first CI runs on GitHub: windows-latest failed on CRLF checkouts (fixed with `.gitattributes`), the race job timed out, the Wine job hung on inherited pipes and lacked a keyboard layout; all fixed, all 12 jobs green. CHANGELOG v0.5.1. `v0.5.1` tagged on dc43ae2. Pending: a VM rerun of the live keyboard and IME tests (job 012, VM was off), phase-6 plan.
- **2026-10-14:** public API documented: `docs/wiki/` (12 guide pages + 3 generated API references via `internal/gen/wikidoc`, stdlib only), 24 runnable examples in `h` and `telatest`, pkg.go.dev fixes and badge. Known doc gaps: alias types (`MouseEvent`, `Value`...) are opaque on pkg.go.dev, detached top comments in `events.go`/`fields.go` appear nowhere, dot-import collisions (`Name`, `Label`, `Value`, `Type`, `Code`, `Key`) are not warned about, no `aria-*:` variants. Wiki not yet copied to `tela.wiki.git`. Next: `examples/native` (file explorer with Windows 11, Adwaita and Breeze themes, code-only, no new API).
- **2026-10-14:** `examples/native` (file explorer, Win11/Adwaita/Breeze x light/dark, 6 goldens + 3 at x2, interaction tests, arm64 checked). API gaps found: system dark mode is not readable from Go (only through the `dark:` variant), no public window-chrome controls or header-bar drag, no `clip-path`, `HasText("")` and partial `ExpectAria` missing. Fidelity to the real platforms is from memory, not measured; no real Windows run. Next: a reusable `tela/theme` or `kit` (owner's call), then phase 6.
- **2026-10-14:** `AppID` of the examples moved to `dev.telaui.*` (new domain); `v0.5.2` tagged with the owner's approval. Pending: wiki copy to `tela.wiki.git`, phase-6 plan.
- **2026-10-18:** phase 6 planned and approved (T1–T10, API decisions up front). T2 (blend modes, masks, focal gradients; exact COLR emoji) and T3 (Wayland fractional scaling, grid-snapped translations, snapped selections/scrollbars) verified and merged; T1, T4, T5, T6, T8 running in parallel worktrees.
- **2026-10-18:** T4 (`internal/anim`, CSS transitions, `animate-*`, reduced motion; ADR-076) verified (incl. arm64) and merged. Two thermal shutdowns interrupted the agents; T1, T8 resumed from their worktrees, T5, T6 restarted, T7 started. Open owner question from T4: a public way to snapshot mid-transition in `telatest`.
- **2026-10-18:** T1 grid (Taffy port, 553/553 included grid fixtures and 630/630 flex fixtures match Chromium; ADR-073) verified (incl. arm64) and merged. Follow-ups: `grid-template-areas` unreachable from classes (no Tailwind utility); typed grid `Style` props would be a public-API decision.
- **2026-10-18:** owner paused work 17:12–19:52. Merged after verification (build, vet, staticcheck, tests, cross-builds, race, arm64): T7 (ADR-080), T8 (ADR-077; rebased by its agent, then onto T7 by the main session), T6 (ADR-079), T7b `OnDone`/`Velocity`/`BgFn`/`TextColorFn` (ADR-082; a `text/paint.go` conflict with T6 resolved). Owner decisions: `telatest.NoSettle`, T7b API, layout default spring 500/25 (Motion's own default is a 450 ms tween). Task board kept in `TASKS.local.md`.
- **2026-10-18:** T5 (fast blur, plus `drop-shadow` σ = radius like Chromium) and T9 (layout animations; rebased by its agent with sticky/fixed/Portal/clip-path/BgFn checks) verified (race, arm64) and merged. Every phase-6 feature item is in; T10 remains.
- **2026-10-18:** T3 live check at a fractional monitor scale (eDP-1 switched to 1.5 at runtime with the owner's permission, then restored to 2): Wayland live tests report `fractional 180/120, viewport true` with buffer = PhysicalSize; `examples/settings` and `examples/virtuallist` windows (960×640 at 100,100) are pixel-identical to `tela snapshot --scale 1.5` (0 differing pixels), and after keyboard scrolling (incl. a jump to row ~99,990) identical to a full redraw. T10 running.
- **2026-10-18:** T10 merged (`examples/dashboard` glass showcase; raster image fast path, bit-identical). **Phase 6 done criterion met:** headless 1920×1080 x1 scripted session of 644 frames p95 14.6 ms on 4 CPUs (13.8 ms on 18; p99 18.8, scrolling the heaviest scene at ~14–15 ms with full repaints); live at 1920×1080 physical (monitor at scale 1, restored afterwards) the `fps` overlay reads 12.3–14.4 ms per frame on Wayland while scrolling, 16.1–19.8 ms on X11 (PutImage; pointer also hovering). Not reached: scale 2 (p95 36–48 ms). `wtype` keys did not reach the Xwayland window (wheel did); unverified whether that is wtype or Tela. Windows VM was off: no Windows run this phase. T10's engine notes for later: backdrop blur inside a layer disables banded raster; scrolling over images/backdrops cannot pixel-shift; `drawShadow` does interior work; an `opacity-0` box still blurs its backdrop.
- **2026-10-19:** phase-close `/code-review high` (7 findings) plus T10's backdrop-opacity finding fixed with regression tests (ADR-084; Chromium-measured), verified (race, arm64) and merged. Phase 6 done; CHANGELOG moved to v0.6.0; tag proposed to the owner.
- **2026-10-19:** `v0.6.0` tagged on d8cd278 and pushed with main at the owner's request.
- **2026-10-19:** docs brought up to v0.6.0 (README, wiki Hooks/Elements pages, ARCHITECTURE's `UseAnimation` line); `docs/wiki` copied to `tela.wiki.git` (75b4ddf; it had been synced at v0.5.2 already) and pushed with main at the owner's request.
- **2026-10-20:** roadmap revised after a gap analysis against JavaFX, the Microsoft stack, Wails, Tauri and Electron; the owner chose which doubtful features enter. Added phases 9 (desktop integration), 10 (distribution, auto-update, DevTools) and 11 (media and graphics: canvas, audio, video, print/PDF, exploratory 3D); widened phase 7 (icons/SVG, more `kit`, `tela add` registry); added "Separate packages" (charts, DataGrid/TreeView), "Deferred" (WASM, mobile, webview) and "Discarded" (visual designer, component gallery). The done criteria of phases 8–11 are proposals, to be confirmed when each phase is planned. Pending: decide `API_PROPOSALS.md`, plan phase 7.
- **2026-10-20:** proposal applied: phases 7–19 replace the old 7–11 (public API polish first; desktop integration before macOS; macOS, accessibility and GPU split; `kit` in core with `tela add` deferred), plus "Road to v1.0". README references to "phase 8" now say phase 13. Pending: decide the phase-7 T3 options, plan phase 7.
- **2026-10-20:** open roadmap decisions settled by the owner: i18n stays in phase 7 (T7, now with a done criterion); dev mode on by default in development builds, release detected with `debug.ReadBuildInfo`; no `tela fix`, release notes only; accessibility strictly after macOS. Pending: decide the phase-7 T3 options, plan phase 7.
- **2026-10-20:** phase 7 planned and approved; the owner decided D1–D12 (struct components, `Children tela.Component`, `Value.Get` unchanged with a dev warning, `ID`/`KeyName`/`KeyCode`, `x/text` for i18n…). Wave A started (T1, T2b, T7a in worktrees).
- **2026-10-21:** the host lost power at 00:38 (three agents, CPUQuota 800%): 90 empty git objects in worktree data, repaired without loss (main intact). T4 theme tokens (ADR-089), T3 struct components/generic keys/renames/`tela.Once`/`UseSpring(target)` (ADR-088) and T7b right-to-left layout with `examples/i18n` (ADR-090) verified and merged; wave C (T2a, T5) started.
- **2026-10-21:** wave C (T2a ergonomics ADR-091, T5 previews ADR-092) and T6 (15 examples migrated with goldens identical) merged; `/code-review high` found 9 issues, all fixed (ADR-093); the owner chose `App.Root` as a component and `If` returning `Child` (ADR-094). Race and arm64 pass. Phase 7 done; CHANGELOG moved to v0.7.0; tag proposed.
- **2026-10-21:** `v0.7.0` tagged on bc1f753 with the owner's approval (not pushed). Then main, the tag and the wiki (e40b769) were pushed at the owner's request. Pending: phase-8 plan.
- **2026-10-21:** phase 8 planned (owner decided E1–E7: broad static SVG, general icon API with Lucide as a separate module, `h.Icon` + `tela.Icon{SVG}`, `Img` sources and tint, `aria` option structs with integrated state, same-frame floating) and implemented: T1–T8b merged (ADRs 095–105) after verification (build, vet, staticcheck, tests, cross-builds; race and arm64 on the rendering and engine code). Found and fixed: telatest cut finite animations on slow machines (real-time limit; now app time), the Floating UI oracle test used 3.5 GB. Pending: T9a example, phase close.
- **2026-10-21:** phase 8 finished: T8a/T8b collections, T9a `examples/aria` (done criterion); `/code-review high` (10 findings: 9 fixed, ADR-107/108); telatest widget states and hook-made submit buttons (owner); race and arm64 pass. Phase 8 done; CHANGELOG moved to v0.8.0; tag proposed.
- **2026-10-21:** `v0.8.0` and `icons/lucide/v1.61.0` tagged on 92afd5b with the owner's approval (not pushed). Pending: phase-9 plan.
- **2026-10-21:** the owner found `TELA_DEBUG=layers` panicking on any window with a text field (since phase 5): fixed, `v0.8.1`. A second `/code-review high` over the review fixes (eb596dc..92afd5b) found 9 more (tab typeahead not selecting, a tree-wide float retry counter, the asset lock still released without `defer`, a tap after a touch scroll counted as a double click, boundary events during a pan, dead code); all fixed with regression tests (ADR-109, ADR-110; also `tela.BgImage`'s identical lock bug); race passes; `v0.8.2`. Pending: phase-9 plan.
- **2026-10-21:** roadmap revised: a dev loop (`tela dev` with the window and state kept across rebuilds) becomes phase 9 (`v0.9.0`) at the owner's request; `kit` and every later phase move up by one (macOS is now phase 14). Options in `DEVMODE_PROPOSAL.md`. Nothing implemented.
- **2026-10-22:** phase 9 planned and approved (T0–T7, waves A–D); ADR-111 records G1–G9, the `teladev` tag rule and the release-size check; exiting `Presence` children are not kept across reloads (owner). Wave A started.
- **2026-10-22 (overnight, owner asleep after approving):** phase 9 implemented: T1 remote backend, T2 core state, T4 supervisor, T3 child runtime and `App.DevState`, T5 error overlay, T6 speed (dashboard 855 → 588–621 ms), T7 live validation on Wayland and X11 through a virtual Hyprland output (all done criteria met on Linux; release size +12 KB vs v0.8.2, no dev-loop code); an AutoFocus caret bug found by T2 fixed (15 example goldens: caret visible, text in place; ADR-118). `/code-review high` (10 findings), a review of its fixes (9) and a review of those fixes (10) handled (ADR-119 to ADR-124; ADR-119's text restored after an in-place edit). Pending: Windows VM run, the tag.
- **2026-10-22:** the owner started the Windows VM and installed Go: native tests and a live `tela dev` run of `examples/dashboard` (same hwnd across 4 reloads and a build error; state kept; reloads ~2.2 s from the share: build 1.4 s, start→first frame 0.7–0.8 s); 3 Windows failures found. The owner asked to tag `v0.9.0` now and ship the Windows fixes as patch versions. Tagged `v0.9.0`.
- **2026-10-22:** `v0.9.1`: Windows fixes of `tela dev` (named mappings kept until the host maps them, the Windows watcher knows its directories and 8.3 names, a private build directory, the Defender hint for slow reload starts), with a review of them (ADR-125, ADR-126); verified locally and on the Windows VM (remote and watcher suites twice, 45 supervisor tests; live reloads 1.0–1.1 s). A heavy VM job (two 89 MB tree copies) blocked the owner's Windows session once; later jobs use prebuilt test binaries only. The host's disk filled up (71 GB Go build cache); the owner cleared it.
- **2026-10-22:** a console window opens next to a Tela app started from Explorer on Windows (Go builds console programs by default). Documented `-ldflags=-H=windowsgui` (Getting Started "Ship it", README); `tela build` will set it with a log file (phase 17). The owner rejected hiding the console at run time: the flash would make Tela apps look like malware (ADR-127).
- **2026-10-25:** `tela new <dir>` (the owner's request, ADR-128): templates embedded in `cmd/tela/templates/app`, `-module`, `-skip-deps`, `go get` + `go mod tidy` at the binary's release version. Verified: unit tests, an offline test that vets and tests a generated app against this checkout and runs `tela inspect` on it, and a real `tela new` + `go test` against the published v0.9.1; `staticcheck` is not installed on this host (not run).
- **2026-10-25:** phase 10 planned with the owner: struct components with slots, shadcn `new-york-v4` checked against Chromium, recipes for customization (the dashboard keeps a glass look on kit), app-screen examples migrated and the layer examples kept (K1–K17, ADR-129). Nothing implemented.
- **2026-10-25 (owner asleep from ~02:30, cron at 11:01):** phase 10 implemented with up to 7 agents in parallel: kit foundation, engine, all aria hooks, every kit component, the RTL sweep, eight examples migrated, the gallery and wiki; a weekly API limit stopped the agents from about 05:00 to 11:00. `/code-review high` (10 findings) fixed (ADR-152); `tela.DevMode` keeps kit free of `internal/` (ADR-153). A 26 MB binary an agent committed was removed from the unpushed history (backup branch `backup/pre-native-binary-removal`). Pending: the owner's answers, then `v0.10.0`.
- **2026-10-25:** the owner reviewed the phase's API on a review page (D1–D9, A1–A5; ADR-154); `tela.UseDirection` (ADR-155) and the kit changes (ADR-156) merged and verified; README screenshots and status updated; CHANGELOG moved to `v0.10.0`; the owner tagged `v0.10.0` on c094ab4. Pending: phase-11 plan; delete the local branch `backup/pre-native-binary-removal`.
