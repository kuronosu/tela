# Architecture decision records

Decisions that shape Tela, one file per ADR (`NNN-title.md`). `docs/ARCHITECTURE.md` (§1–§20) is the current design and cites ADRs by number; open an ADR only when a task touches what it decided. Moved here from ARCHITECTURE §21 by ADR-165, text unchanged.

**Adding one:** append `NNN-short-title.md` with the next number and a row below. Keep it short: the decision, why, and what it changes (aim for about 1 KB; details belong in doc comments and tests). A changed decision gets a new ADR that says "Supersedes ADR-NNN" or "Amends ADR-NNN"; old ADRs are never deleted, and the superseded one's status line is updated. Number 083 was never used.

| ADR | Date | Decision | Status |
|---|---|---|---|
| [001](001-pure-go-cgo-enabled-0-no-webview-or-native-toolk.md) | 2026-10-11 | Pure Go, `CGO_ENABLED=0`, no webview or native toolkits |  |
| [002](002-cpu-rendering-as-initial-and-reference-backend.md) | 2026-10-11 | CPU rendering as initial and reference backend |  |
| [003](003-react-like-api.md) | 2026-10-11 | React-like API |  |
| [004](004-internally-three-trees.md) | 2026-10-11 | Internally three trees |  |
| [005](005-constraint-based-layout-protocol.md) | 2026-10-11 | Constraint-based layout protocol |  |
| [006](006-css-subset-of-10.md) | 2026-10-11 | CSS subset of §10 |  |
| [007](007-color-blending-in-srgb-like-browsers.md) | 2026-10-11 | Color blending in sRGB, like browsers |  |
| [008](008-embedded-fonts-for-tests-and-snapshots.md) | 2026-10-11 | Embedded fonts for tests and snapshots |  |
| [009](009-wayland-primary-and-x11-fallback-on-linux.md) | 2026-10-11 | Wayland primary and X11 fallback on Linux |  |
| [010](010-semantics-tree-from-the-start-exportable.md) | 2026-10-11 | Semantics tree from the start, exportable |  |
| [011](011-everything-in-internal-except-tela-h-kit-telates.md) | 2026-10-11 | Everything in `internal/` except `tela`, `h`, `kit`, `telatest`, `cmd/tela` |  |
| [012](012-all-project-docs-in-english.md) | 2026-10-11 | All project docs in English |  |
| [013](013-public-aria-package-of-unstyled-behavior-hooks-f.md) | 2026-10-11 | Public `aria` package of unstyled behavior hooks + floating positioning |  |
| [014](014-kit-components-distributed-as-source-via-tela-ad.md) | 2026-10-11 | `kit` components distributed as source via `tela add` |  |
| [015](015-automatic-build-skipping-for-comparable-props-an.md) | 2026-10-11 | Automatic build skipping for comparable props, and phase-deferred reads via `UseValue` |  |
| [016](016-layout-animations.md) | 2026-10-11 | Layout animations |  |
| [017](017-analytic-erf-shadows-for-rounded-rects.md) | 2026-10-11 | Analytic erf shadows for rounded rects |  |
| [018](018-default-theme-and-class-vocabulary-generated-fro.md) | 2026-10-11 | Default theme and class vocabulary generated from Tailwind v4 source via `go generate` |  |
| [019](019-wayland-protocol-bindings-generated-from-officia.md) | 2026-10-11 | Wayland protocol bindings generated from official XML |  |
| [020](020-agent-interfaces.md) | 2026-10-11 | Agent interfaces |  |
| [021](021-keyboard-events-split-into-physical-key-logical.md) | 2026-10-11 | Keyboard events split into physical key, logical key and text |  |
| [022](022-goldens-are-regenerated-with-tela-update-1-go-te.md) | 2026-10-11 | Goldens are regenerated with `TELA_UPDATE=1 go test ./...`, not an `-update` flag |  |
| [023](023-each-gradient-carries-its-interpolation-color-sp.md) | 2026-10-11 | Each gradient carries its interpolation color space |  |
| [024](024-callback-driven-platform-loop.md) | 2026-10-11 | Callback-driven platform loop |  |
| [025](025-in-phase-4-vendor-a-patched-copy-of-jezek-xgb.md) | 2026-10-11 | In phase 4, vendor a patched copy of `jezek/xgb` |  |
| [026](026-stroking-flattens-curves-and-offsets-the-polylin.md) | 2026-10-11 | Stroking flattens curves and offsets the polyline |  |
| [027](027-floating-point-code-in-gfx-and-raster-is-fusion.md) | 2026-10-11 | Floating-point code in `gfx` and `raster` is fusion-safe |  |
| [028](028-shadows-and-filters-follow-css.md) | 2026-10-11 | Shadows and filters follow CSS |  |
| [029](029-glyph-raster.md) | 2026-10-11 | Glyph raster |  |
| [030](030-internal-text-loads-truetype-outlines.md) | 2026-10-11 | `internal/text` loads TrueType outlines |  |
| [031](031-font-selection.md) | 2026-10-11 | Font selection |  |
| [032](032-colrv1-glyphs-are-converted-to-displaylists.md) | 2026-10-11 | COLRv1 glyphs are converted to DisplayLists |  |
| [033](033-pin-go-text-typesetting-to-commit-e49e2bd.md) | 2026-10-12 | Pin `go-text/typesetting` to commit e49e2bd |  |
| [034](034-paragraph-layout-follows-parley.md) | 2026-10-12 | Paragraph layout follows Parley |  |
| [035](035-system-fonts-are-opt-in-per-collection.md) | 2026-10-12 | System fonts are opt-in per `Collection` |  |
| [036](036-text-editing-model.md) | 2026-10-12 | Text editing model |  |
| [037](037-tailwind-v4-3-3-theme-css-and-preflight-css-are.md) | 2026-10-12 | Tailwind v4.3.3 `theme.css` and `preflight.css` are vendored in… |  |
| [038](038-primitive-defaults-follow-tailwind-s-preflight.md) | 2026-10-12 | Primitive defaults follow Tailwind's Preflight |  |
| [039](039-internal-layout-defines-its-own-style.md) | 2026-10-12 | `internal/layout` defines its own `Style` |  |
| [040](040-a-minimal-usestate.md) | 2026-10-12 | A minimal `UseState` | superseded by [051](051-phase-4-hooks.md) |
| [041](041-fusion-safety.md) | 2026-10-12 | Fusion safety |  |
| [042](042-inherited-style-properties-follow-css-which-is-w.md) | 2026-10-12 | Inherited style properties follow CSS, which is wider than §10's list |  |
| [043](043-internal-layout-implements-9-s-protocol-with-taf.md) | 2026-10-12 | `internal/layout` implements §9's protocol with Taffy's request model |  |
| [044](044-internal-core-reconciliation-and-box-generation.md) | 2026-10-12 | `internal/core` reconciliation and box generation |  |
| [045](045-box-painting-follows-blink-where-css-leaves-room.md) | 2026-10-12 | Box painting follows Blink where CSS leaves room |  |
| [046](046-colors-are-extended-srgb.md) | 2026-10-12 | Colors are extended sRGB |  |
| [047](047-public-api-of-phase-3.md) | 2026-10-12 | Public API of phase 3 |  |
| [048](048-accepted-deviation-of-adr-034.md) | 2026-10-12 | Accepted deviation of ADR-034 | superseded by [159](159-line-boxes-in-device-pixels-and-an-input-s-inner.md) |
| [049](049-versioning.md) | 2026-10-12 | Versioning |  |
| [050](050-platform-loop-and-keyboard-events.md) | 2026-10-12 | Platform loop and keyboard events |  |
| [051](051-phase-4-hooks.md) | 2026-10-12 | Phase-4 hooks |  |
| [052](052-linux-key-translation-is-a-pure-go-xkb-subset-in.md) | 2026-10-12 | Linux key translation is a pure-Go XKB subset in `internal/platform/internal/xkb` |  |
| [053](053-usevalue-t-returns-a-comparable-value-t.md) | 2026-10-12 | `UseValue[T]` returns a comparable `*Value[T]` |  |
| [054](054-x11-input.md) | 2026-10-12 | X11 input |  |
| [055](055-pointer-input-follows-the-dom-for-targets-and-pr.md) | 2026-10-12 | Pointer input follows the DOM for targets and propagation |  |
| [056](056-keyboard-dispatch-and-focus-follow-the-dom-and-h.md) | 2026-10-12 | Keyboard dispatch and focus follow the DOM and HTML |  |
| [057](057-scrolling.md) | 2026-10-12 | Scrolling |  |
| [058](058-semantics-tree-and-aria-snapshots.md) | 2026-10-12 | Semantics tree and aria snapshots |  |
| [059](059-telatest-and-a-public-headless-entry-point.md) | 2026-10-13 | `telatest` and a public headless entry point |  |
| [060](060-virtualization.md) | 2026-10-13 | Virtualization |  |
| [061](061-layers-and-damage.md) | 2026-10-13 | Layers and damage |  |
| [062](062-debug-overlays.md) | 2026-10-13 | Debug overlays |  |
| [063](063-win32-backend-base.md) | 2026-10-13 | Win32 backend base |  |
| [064](064-wayland-clipboard-over-wl-data-device.md) | 2026-10-13 | Wayland clipboard over `wl_data_device` |  |
| [065](065-text-editing-model.md) | 2026-10-13 | Text editing model |  |
| [066](066-inline-layout.md) | 2026-10-13 | Inline layout |  |
| [067](067-win32-keyboard-clipboard-and-settings.md) | 2026-10-13 | Win32 keyboard, clipboard and settings |  |
| [068](068-platform-text-input-and-ime.md) | 2026-10-13 | Platform text input and IME |  |
| [069](069-text-fields-and-forms.md) | 2026-10-13 | Text fields and forms |  |
| [070](070-phase-4-api-gaps.md) | 2026-10-13 | Phase-4 API gaps |  |
| [071](071-win32-ime-through-imm32.md) | 2026-10-13 | Win32 IME through IMM32 |  |
| [072](072-phase-5-review-fixes.md) | 2026-10-13 | Phase-5 review fixes |  |
| [073](073-css-grid.md) | 2026-10-18 | CSS grid |  |
| [074](074-blend-modes-mask-layers-and-focal-gradients.md) | 2026-10-18 | Blend modes, mask layers and focal gradients |  |
| [075](075-fractional-scaling.md) | 2026-10-18 | Fractional scaling |  |
| [076](076-animation-layer-and-implicit-transitions.md) | 2026-10-18 | Animation layer and implicit transitions |  |
| [077](077-fixed-and-sticky-positioning-portals-and-the-col.md) | 2026-10-18 | Fixed and sticky positioning, portals and the color scheme |  |
| [078](078-fast-large-blurs.md) | 2026-10-18 | Fast large blurs |  |
| [079](079-css-effects-in-core-painting.md) | 2026-10-18 | CSS effects in `core` painting |  |
| [080](080-explicit-animations-enter-and-exit.md) | 2026-10-18 | Explicit animations, enter and exit |  |
| [081](081-layout-animations.md) | 2026-10-18 | Layout animations |  |
| [082](082-animation-completion-initial-velocity-and-deferr.md) | 2026-10-18 | Animation completion, initial velocity and deferred colors |  |
| [084](084-a-box-s-opacity-fades-its-backdrop-filter.md) | 2026-10-19 | A box's opacity fades its backdrop filter |  |
| [085](085-safety-net.md) | 2026-10-20 | Safety net |  |
| [086](086-telatest-ergonomics.md) | 2026-10-20 | `telatest` ergonomics |  |
| [087](087-internationalization.md) | 2026-10-20 | Internationalization |  |
| [088](088-source-breaking-api-decisions.md) | 2026-10-21 | Source-breaking API decisions |  |
| [089](089-theme-tokens-scoped-color-schemes-and-class-alia.md) | 2026-10-21 | Theme tokens, scoped color schemes and class aliases |  |
| [090](090-right-to-left-layout.md) | 2026-10-21 | Right-to-left layout |  |
| [091](091-ergonomics.md) | 2026-10-21 | Ergonomics |  |
| [092](092-previews-seeded-state-and-one-app.md) | 2026-10-21 | Previews, seeded state and one `App` |  |
| [093](093-phase-7-review-fixes.md) | 2026-10-21 | Phase-7 review fixes |  |
| [094](094-app-root-is-a-component-and-h-if-h-ifelse-return.md) | 2026-10-21 | `App.Root` is a `Component` and `h.If`/`h.IfElse` return a `Child` |  |
| [095](095-svg-documents.md) | 2026-10-21 | SVG documents |  |
| [096](096-engine-hooks-for-package-aria.md) | 2026-10-21 | Engine hooks for package `aria` |  |
| [097](097-api-gaps-class-variants-and-the-phase-7-removals.md) | 2026-10-21 | API gaps, class variants and the phase-7 removals |  |
| [098](098-images-icons-and-masks.md) | 2026-10-21 | Images, icons and masks |  |
| [099](099-aria-usefloating.md) | 2026-10-21 | `aria.UseFloating` |  |
| [100](100-behavior-hooks-of-package-aria.md) | 2026-10-21 | Behavior hooks of package `aria` |  |
| [101](101-overlay-hooks-of-package-aria-and-the-follow-ups.md) | 2026-10-21 | Overlay hooks of package `aria`, and the follow-ups to ADR-100 |  |
| [102](102-icon-generator-and-the-lucide-module.md) | 2026-10-21 | Icon generator and the Lucide module |  |
| [103](103-gradient-mask-classes-follow-the-class-order.md) | 2026-10-21 | Gradient-mask classes follow the class order |  |
| [104](104-collection-hooks-of-package-aria.md) | 2026-10-21 | Collection hooks of package `aria` |  |
| [105](105-aria-useselect-and-aria-usecombobox.md) | 2026-10-21 | `aria.UseSelect` and `aria.UseCombobox` |  |
| [106](106-examples-aria-and-its-follow-ups.md) | 2026-10-21 | `examples/aria` and its follow-ups |  |
| [107](107-phase-8-review-fixes-core-and-two-owner-approved.md) | 2026-10-21 | Phase-8 review fixes, core, and two owner-approved follow-ups |  |
| [108](108-phase-8-review-fixes-in-aria.md) | 2026-10-21 | Phase-8 review fixes in `aria` |  |
| [109](109-second-phase-8-review-core.md) | 2026-10-21 | Second phase-8 review, core |  |
| [110](110-second-phase-8-review-in-aria.md) | 2026-10-21 | Second phase-8 review in `aria` |  |
| [111](111-dev-loop-tela-dev.md) | 2026-10-22 | Dev loop, `tela dev` |  |
| [112](112-remote-backend-internal-platform-remote.md) | 2026-10-22 | Remote backend, `internal/platform/remote` |  |
| [113](113-dev-loop-state.md) | 2026-10-22 | Dev-loop state |  |
| [114](114-the-tela-dev-child-runtime-and-app-devstate.md) | 2026-10-22 | The `tela dev` child runtime and `App.DevState` |  |
| [115](115-the-tela-dev-supervisor.md) | 2026-10-22 | The `tela dev` supervisor |  |
| [116](116-fast-tela-dev-reloads.md) | 2026-10-22 | Fast `tela dev` reloads |  |
| [117](117-the-error-overlay-of-tela-dev.md) | 2026-10-22 | The error overlay of `tela dev` |  |
| [118](118-a-text-field-reveals-its-caret-after-layout.md) | 2026-10-22 | A text field reveals its caret after layout |  |
| [119](119-tela-dev-children-s-output-and-build-tags.md) | 2026-10-22 | `tela dev` children's output and build tags |  |
| [120](120-phase-9-review-fixes-in-internal-core-and-intern.md) | 2026-10-22 | Phase-9 review fixes in `internal/core` and `internal/text/edit` |  |
| [121](121-second-phase-9-review-of-tela-dev.md) | 2026-10-22 | Second phase-9 review of `tela dev` |  |
| [122](122-caret-reveals-held-by-passive-effects-run-when-t.md) | 2026-10-22 | Caret reveals held by passive effects run when the effects end |  |
| [123](123-third-phase-9-review-of-tela-dev.md) | 2026-10-22 | Third phase-9 review of `tela dev` |  |
| [124](124-caret-reveals-outside-a-frame-wait-for-queued-up.md) | 2026-10-22 | Caret reveals outside a frame wait for queued updates, and a frame with nothing to lay… |  |
| [125](125-windows-fixes-of-tela-dev.md) | 2026-10-22 | Windows fixes of `tela dev` |  |
| [126](126-review-of-the-v0-9-1-windows-fixes.md) | 2026-10-22 | Review of the v0.9.1 Windows fixes |  |
| [127](127-windows-console-window.md) | 2026-10-22 | Windows console window |  |
| [128](128-tela-new-dir-creates-an-app.md) | 2026-10-25 | `tela new <dir>` creates an app |  |
| [129](129-kit-design.md) | 2026-10-25 | `kit` design |  |
| [130](130-kit-foundation-and-the-simple-components.md) | 2026-10-25 | `kit` foundation and the simple components |  |
| [131](131-engine-additions-for-kit.md) | 2026-10-25 | Engine additions for `kit` |  |
| [132](132-radio-toggle-group-slider-progress-and-meter-hoo.md) | 2026-10-25 | Radio, toggle group, slider, progress and meter hooks of package `aria` |  |
| [133](133-menu-bar-and-context-menu-hooks-of-package-aria.md) | 2026-10-25 | Menu bar and context menu hooks of package `aria` |  |
| [134](134-calendar-hooks-of-package-aria-and-formatter-mon.md) | 2026-10-25 | Calendar hooks of package `aria` and `Formatter.MonthYear` |  |
| [135](135-color-hooks-of-package-aria.md) | 2026-10-25 | Color hooks of package `aria` |  |
| [136](136-kit-choice-controls.md) | 2026-10-25 | `kit` choice controls |  |
| [137](137-kit-overlays.md) | 2026-10-25 | `kit` overlays |  |
| [138](138-kit-select-tabs-and-toaster.md) | 2026-10-25 | `kit` Select, Tabs and Toaster |  |
| [139](139-kit-table-t.md) | 2026-10-25 | `kit.Table[T]` |  |
| [140](140-kit-class-order-follows-tailwind-s-stylesheet.md) | 2026-10-25 | Kit class order follows Tailwind's stylesheet |  |
| [141](141-kit-menus.md) | 2026-10-25 | `kit` menus |  |
| [142](142-kit-calendar-rangecalendar-datepicker-and-datera.md) | 2026-10-25 | `kit` Calendar, RangeCalendar, DatePicker and DateRangePicker |  |
| [143](143-kit-color-picker.md) | 2026-10-25 | `kit` color picker |  |
| [144](144-examples-dashboard-rebuilt-on-kit-with-a-glass-s.md) | 2026-10-25 | `examples/dashboard` rebuilt on `kit` with a glass style |  |
| [145](145-examples-settings-and-pricing-on-kit.md) | 2026-10-25 | Examples `settings` and `pricing` on `kit` |  |
| [146](146-examples-signup-and-player-on-kit.md) | 2026-10-25 | Examples `signup` and `player` on `kit` |  |
| [147](147-example-board-on-kit.md) | 2026-10-25 | Example `board` on `kit` |  |
| [148](148-example-i18n-on-kit.md) | 2026-10-25 | Example `i18n` on `kit` |  |
| [149](149-example-native-rebuilt-on-kit-one-recipe-per-loo.md) | 2026-10-25 | Example `native` rebuilt on `kit`, one recipe per look |  |
| [150](150-the-examples-kit-gallery-and-the-kit-documentati.md) | 2026-10-25 | The `examples/kit` gallery and the kit documentation |  |
| [151](151-right-to-left-sweep-of-kit.md) | 2026-10-25 | Right-to-left sweep of `kit` | superseded by [155](155-tela-usedirection.md) |
| [152](152-phase-10-review-fixes.md) | 2026-10-25 | Phase-10 review fixes |  |
| [153](153-tela-devmode.md) | 2026-10-25 | `tela.DevMode()` |  |
| [154](154-the-owner-s-api-review-before-v0-10-0.md) | 2026-10-25 | The owner's API review before `v0.10.0` |  |
| [155](155-tela-usedirection.md) | 2026-10-25 | `tela.UseDirection()` |  |
| [156](156-the-owner-s-api-review-applied.md) | 2026-10-25 | The owner's API review applied |  |
| [157](157-three-flex-fixes-for-v0-10-1-engine-only-no-publ.md) | 2026-10-25 | Three flex fixes for `v0.10.1`, engine only, no public API change |  |
| [158](158-gradient-color-space-of-arbitrary-gradients.md) | 2026-10-25 | Gradient color space of arbitrary gradients |  |
| [159](159-line-boxes-in-device-pixels-and-an-input-s-inner.md) | 2026-10-25 | Line boxes in device pixels, and an Input's inner-editor line height |  |
| [160](160-focus-containment-beyond-tab.md) | 2026-10-25 | Focus containment beyond Tab |  |
| [161](161-fast-calendars.md) | 2026-10-25 | Fast calendars |  |
| [162](162-the-code-on-a-colorinput-s-button-and-the-picker.md) | 2026-10-25 | The code on a `ColorInput`'s button and the picker's hex field follow a drag, and the… |  |
| [163](163-a-menu-scrolls-only-when-the-window-has-no-room.md) | 2026-10-25 | A menu scrolls only when the window has no room for it |  |
| [164](164-what-a-floating-element-s-place-publishes-is-rea.md) | 2026-10-25 | What a floating element's Place publishes is read in the same frame, and a menu clears… |  |
| [165](165-adrs-in-their-own-files-a-short-roadmap.md) | 2026-10-25 | ADRs in their own files, a short roadmap, reading on demand |  |
| [166](166-text-wrap-balance-and-pretty-follow-blink.md) | 2026-10-25 | `text-wrap: balance` and `pretty` follow Blink |  |
| [167](167-any-mouse-button-dismisses-an-overlay.md) | 2026-10-25 | Any mouse button dismisses an overlay; a context menu's long press is touch and pen only |  |
| [168](168-focus-after-the-focused-element-of-a-containing-scope-unmounts.md) | 2026-10-25 | Focus after the focused element of a containing scope unmounts |  |
| [169](169-invalid-skips-fields-barred-from-constraint-validation.md) | 2026-10-25 | `invalid:` skips fields barred from constraint validation |  |
| [170](170-telatest-right-click-tap-and-long-press.md) | 2026-10-26 | `telatest` right click, tap and long press |  |
| [171](171-tela-usetimeout-and-h-ariacurrent.md) | 2026-10-26 | `tela.UseTimeout` and `h.AriaCurrent` |  |
| [172](172-kit-field-control-after-and-fieldset.md) | 2026-10-26 | `kit.Field` ControlAfter and `kit.FieldSet` |  |
| [173](173-tela-lint.md) | 2026-10-26 | `tela lint` |  |
| [174](174-tela-mcp.md) | 2026-10-26 | `tela mcp` |  |
| [175](175-a-func-preview-is-a-component.md) | 2026-10-26 | A `func() tela.Node` Preview is a component |  |
| [176](176-layout-effect-scrolls-and-absolute-inline-boxes.md) | 2026-10-26 | Layout-effect scrolls and absolute inline boxes |  |
| [177](177-llms-txt.md) | 2026-10-26 | `docs/llms.txt` and how it is kept true |  |
| [178](178-desktop-essentials-design.md) | 2026-10-27 | Desktop essentials design (phase 12): declarative windows, package `desktop`, D-Bus via godbus |  |
| [179](179-platform-contract-v2.md) | 2026-10-27 | Platform contract v2: window control, hit regions, restore tokens, desktop services |  |
| [180](180-multi-window-core-and-api.md) | 2026-10-27 | Multiple windows in core (a Tree per window, one element tree) and the public API: `tela.Window`, `UseWindow`, `Quit`, `OpenURL`, title bar props |  |
| [181](181-package-desktop.md) | 2026-10-27 | Package `desktop`: dialogs, drawn message box, notifications, `Tray`, app folders, settings store and `UseSetting`, persisted window placement; core hooks for it |  |
| [182](182-kit-titlebar-and-windowcontrols.md) | 2026-10-27 | `kit.TitleBar` and `kit.WindowControls`: custom title bar and window buttons in Windows, GNOME and macOS looks |  |
| [183](183-telatest-for-several-windows.md) | 2026-10-27 | `telatest` for several windows: an `*App` per window, window actions and assertions, quit, `MountApp` window fields |  |
| [184](184-tela-dev-windows-and-services.md) | 2026-10-27 | `tela dev` with phase-12 windows and services: remote protocol 3, child windows kept by element path, close to tray, tray and notification claims across reloads |  |
| [185](185-windows-without-frames-commit.md) | 2026-10-27 | Hidden and minimized windows still build and run their effects: the Host commits their trees without raster or present, paced like frames |  |
| [186](186-wayland-hide-destroys-the-role.md) | 2026-10-27 | Wayland hides a window by destroying its xdg_toplevel and xdg_surface and shows it with new ones (Hyprland never configures a re-initial commit) |  |
| [187](187-windows-11-title-bar-and-backdrop.md) | 2026-10-27 | Windows 11 title bar colors (`TitleBar`) and system backdrop (`Backdrop`: Mica, Mica Alt, Acrylic) on `tela.App` and `tela.Window`; client-area backdrop through DWM glass and presented alpha; remote protocol 4 |  |
| [188](188-a-textarea-that-does-not-wrap-scrolls-horizontally.md) | 2026-10-28 | A Textarea whose lines do not wrap scrolls horizontally; classic gutters follow edits; a pointer focus does not reveal the old caret |  |
| [189](189-leaf-packages-below-every-layer.md) | 2026-10-28 | Leaf packages under `internal/` that import nothing from Tela (`fmath`, `utf16x`, `editdist`) may be imported by every layer, `platform` included |  |
| [191](191-the-plain-outside-press-listener-is-removed.md) | 2026-10-29 | `core.InteractOutside`, `OutsideOptions` and `IsTopOverlay` are removed: presses outside reach only the top overlay through `core.Overlay` (amends ADR-096) |  |
