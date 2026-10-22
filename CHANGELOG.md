# Changelog

All notable changes to Tela are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/) as Go modules use it: one minor
version per completed roadmap phase while Tela is v0, patch versions for
fixes after a phase closes. Compatibility covers the public API only (`tela`,
`h`, the `tela` command's flags, the `Preview()`/`Previews` contract and the
`tela inspect` JSON); everything under `internal/` may change at any time.
In v0 a minor version may break the public API; such changes are listed
under **Breaking**. See ADR-049 in `docs/ARCHITECTURE.md`.

## [Unreleased]

### Added

- `h.AriaLabelledBy(refs ...tela.ElementRef)` takes several references (`aria-labelledby="a b"`): the name is the text of each mounted one, in the order given, joined by a space. Existing calls with one reference compile unchanged.
- `aria.ComboboxOptions.ButtonLabel`: the accessible name of the toggle button (default "Show suggestions"), so the fixed English string can be translated. It is the only user-visible text the `aria` hooks add.
- Package `aria`, collection hooks (ADR-104): `aria.UseListBox[K]`, `aria.UseMenu[K]` and `aria.UseTabs[K]` hold a keyed list of items (`Keys []K`, `DisabledKeys`, `TextValue`) with a focused key and a selection, and return `Props` for the collection's element and `ItemProps(key)` for each item (tabs: `TabProps` and `PanelProps`), with `IsSelected`, `IsFocused`, `IsFocusVisible`, `IsDisabled`, `Focused`, `FocusKey`, `Focus`, `ClearFocus` and `Section(name)`. React Aria's keyboard and pointer rules: ArrowUp/ArrowDown (or Left/Right for `Orientation: "horizontal"` and tabs, swapped by `Direction: "rtl"`), Home, End, PageUp and PageDown by viewport, typeahead (one-second buffer on the frame clock), `ShouldFocusWrap`, disabled keys skipped, single and multiple selection with `"toggle"` and `"replace"` behavior, Shift ranges, Ctrl+click and Ctrl+A, Escape to clear, `OnAction`, a roving focus (the item's element has the focus) or a virtual one (`ShouldUseVirtualFocus` with `aria-activedescendant`; `ExternalFocus` for a combobox's input, which calls `HandleKeyDown`), scrolling the focused item into view, and keys of a `h.VirtualList` that are not rendered. `UseMenu` takes the focus strategy, `Close` and `MenuRef` of `UseMenuTrigger` (radio and checkbox items with `SelectionMode`, `OnAction`, `CloseOnSelect`); `UseTabs` has automatic and manual keyboard activation, orientation and the tab/panel links.
- Package `aria`, `aria.UseSelect[K]` and `aria.UseCombobox[K]` (ADR-105), the last collection hooks, built on the list box, the menu trigger and the popover. `UseSelect` returns `TriggerProps` (a button that opens with a press, Enter, Space, ArrowDown and ArrowUp; on the closed trigger typing selects the next matching item and ArrowLeft/ArrowRight the previous and next one), `ValueProps` and `Value` (the selected item's text or `Placeholder`), `LabelProps`, `PopoverProps` (placed next to the trigger in the first frame, as wide as it with `MatchWidth`, Escape, a press outside and Tab close it and the focus returns to the trigger), `ListBox` (`Props` and `ItemProps(key)`; the selected item has the focus when it opens) and the open state (`Open`/`DefaultOpen`/`OnOpenChange`); the trigger is named by its value and its label, as in React Aria (`button "Banana Favorite fruit"`). `UseCombobox` returns `InputProps` (role `combobox`, `aria-autocomplete`, `aria-expanded`, `aria-controls`, `aria-activedescendant`; the input keeps the focus while ArrowDown/ArrowUp highlight items, Enter and Tab choose, Escape closes and puts the text back, a blur commits the text or keeps a custom value with `AllowsCustomValue`, Home/End/Space stay with the text field, and nothing navigates while an IME composes), `ButtonProps` (a toggle button that shows every item), `LabelProps`, `PopoverProps`, `ListBox`, `Keys` (the items to render) and the text and selection as controlled or uncontrolled state (`InputValue *string`, `DefaultInputValue`, `OnInputChange`, `Selected`...). Items are `Keys` filtered by the caller or `Items` filtered by the hook with `Filter`: `aria.Contains` (the default), `aria.StartsWith` and `aria.EndsWith` ignore case and diacritics; `MenuTrigger` is `aria.MenuTriggerInput` (typing), `aria.MenuTriggerFocus` or `aria.MenuTriggerManual`. See the Aria page of the wiki.
- `aria.Selection[K]`: an immutable set of keys with `aria.All[K]()` (React Aria's "all"): `SelectionOf`, `Has`, `With`, `Without`, `Toggle`, `Len`, `Keys`, `In(order)`, `First`, `Resolve`, `Equal`. The zero value means "not given" (uncontrolled); a non-zero `Selected` is controlled, with `OnSelectionChange(Selection[K])`.
- Package `aria`, behavior hooks in React Aria's model. Every hook takes an options struct and returns a struct with a `Props` field (one `tela.Prop`: handlers, ref, role and aria props merged, so the caller's own props combine) and its state: `aria.UsePress` (one press for mouse, touch, pen and keyboard: Enter presses on the key down, Space on the key up, repeats ignored; leaving the element ends the press and coming back restarts it, or `ShouldCancelOnPointerExit` cancels it; a touch that becomes a scroll cancels it; a click that no pointer made, from an agent or a `Label`, is a virtual press; it stops propagation unless `PressEvent.ContinuePropagation()`; `OnPressStart`, `OnPressEnd`, `OnPressUp`, `OnPressChange`, `OnPress`, `Disabled`, `PreventFocusOnPress`, and a `Pressed` field), `aria.UseHover` (mouse and pen, not touch), `aria.UseFocusRing` (`Focused` and `FocusVisible`, which shows after the keyboard and not after a click; `Within`), `aria.UseFocusScope` (`Contain`, `Restore`, `AutoFocus`, and a `FocusManager` whose `FocusNext`, `FocusPrevious`, `FocusFirst` and `FocusLast` take `Wrap`, `Tabbable` and `From`), `aria.UseButton`, `aria.UseToggleButton` (`aria-pressed`), `aria.UseCheckbox` and `aria.UseSwitch` (roles `checkbox` and `switch`, `aria-checked`, Space toggles, an `ID` for `Label(For(id))`). The state of the toggles is integrated: uncontrolled with `DefaultSelected` (and told of changes by `OnChange` if given), controlled when `Selected` points to the caller's state (`Selected: &on`). Hooks used together on one element share a `Ref`. See the Aria page of the wiki and ADR-100 and ADR-101.
- Package `aria`, overlay hooks (ADR-101): `aria.UseOverlay` (the overlay stack: Escape and a press outside close the top overlay only, `Dismissable`, `KeyboardDismissDisabled`, `ShouldCloseOnBlur`, `ShouldCloseOnInteractOutside`, and `Modal`, which makes the rest of the window inert), `aria.UseDialog` (`dialog` and `alertdialog` roles, named by the element with `TitleProps` or `AriaLabel`, focus moves in when it opens, Tab contained when modal, focus restored when it closes), `aria.UsePopover` (positioned by `UseFloating` in the first frame, dismissed by Escape, a press outside or the focus leaving, never by its trigger or by what is opened from inside it, in a `Portal` or not; `ArrowProps`), `aria.UseOverlayTrigger` (the open state, uncontrolled with `DefaultOpen` or controlled with `Open: &open`, `OnOpenChange`; the trigger's press, `aria-haspopup`, `aria-expanded` and `aria-controls`), `aria.UseMenuTrigger` (press, long press, Enter, Space, ArrowDown focusing the first item and ArrowUp the last through `FocusStrategy`) and `aria.UseTooltipTrigger` (hover and keyboard focus, React Aria's 1.5 s delay, 0.5 s close delay, warm-up and cool-down shared by the tooltips of a window, Escape, a press closes it, timers on the frame clock so an idle window stays idle). Render overlays in a `Presence` with `Exit` classes to fade them out. See the Aria page of the wiki.
- `tela.ElementRef.Click()`: HTML's `element.click()` from a handler or an effect: it dispatches a click (a virtual press for the `aria` hooks) and runs the activation behavior, so a `Button` in a `Form` submits or resets it by its type and a `Label` forwards to its control. A disabled or unmounted element ignores it.
- `aria.ButtonOptions.Type` (`"button"`, `"submit"`, `"reset"`): a button made with `aria.UseButton` on any element submits or resets its `Form` when pressed.
- Press callbacks (`OnPress`, `OnPressStart`, `OnPressEnd`, `OnPressChange`, `OnPressUp`) on `aria.ToggleOptions`.
- `h.AriaDescribedBy(ref)`: the accessible description of an element from the text of another (a tooltip describes its trigger); `AriaDescription` wins over it.
- Package `aria` (first files): `aria.UseFloating(anchor, opts...)` positions a popover, tooltip, menu or select list next to its anchor with a port of Floating UI's `computePosition` and middleware, inside the frame that shows it (no flash in the wrong place, no frame of its own while an ancestor scrolls, no CPU while idle). Functional options applied in the order given: `Placement("bottom-start")` (the 12 placements; `-start` and `-end` mirror in a right-to-left element), `Offset(4)`, `Flip()`, `Shift(8)`, `Size(func(a aria.Available) { a.MaxHeight(a.Height) })` (the element is limited in the same frame and its content scrolls), `Arrow(ref)`, `Hide()`, `AutoPlacement()` and `AnchorRect(func() tela.Rect)` for virtual anchors such as a context menu at the pointer; the long tail of Floating UI's options goes through one optional options struct per function (`aria.Flip(aria.FlipOptions{Padding: 8})`). It returns `Props` for the floating element, `ArrowProps` for the arrow (positioned in the same frame), the final `Placement` after a flip, `ArrowX`/`ArrowY` and `ReferenceHidden`/`Escaped` values. A 2,400-case oracle generated with the real `@floating-ui/core` checks the port. See the Aria page of the wiki and ADR-099.
- Responsive range variants (Tailwind v4): `max-sm:` … `max-2xl:` (the window is narrower than the breakpoint), the arbitrary `min-[600px]:` and `max-[40rem]:` (px, rem or em), and `min-sm:` … `min-2xl:` (the same as `sm:` … `2xl:`); they stack with each other and with `sm:` … `2xl:` (`md:max-xl:`).
- `aria-*` variants (Tailwind v4): `aria-busy:`, `aria-checked:`, `aria-disabled:`, `aria-expanded:`, `aria-hidden:`, `aria-pressed:`, `aria-readonly:`, `aria-required:` and `aria-selected:`, plus `group-aria-*:` and `peer-aria-*:`. They match when the element's `h.Aria…` prop is true (Tailwind's `[aria-x="true"]`), not an implicit state, and only the elements whose classes test them restyle when it changes. `aria-[checked=true]:` is `aria-checked:`; other arbitrary selectors (`aria-[sort=ascending]:`) report that Tela has no prop for them.
- `h.AriaRequired(bool)`, `h.AriaReadOnly(bool)` and `h.AriaBusy(bool)`: semantics states (not printed by the aria snapshot, as in Playwright) and the matching variants.
- `h.Range(n, render)`: `render(0)` … `render(n-1)` as a list keyed by index, with no missing-key warning; `render` returns a `tela.Node` or a `tela.Component`.
- `h.VirtualList`'s `renderItem` may return a `tela.Component` as well as a `tela.Node` (struct-component rows need no wrapper); existing calls compile unchanged.
- `tela.StopPropagation(f)`: a handler for every `h.On…` prop that stops the event's propagation and then calls `f`: `Button(OnClick(tela.StopPropagation(ctrl.Remove)))`. Its type is `tela.StopHandler`.
- `tela.UseSize(ref)`: a Value with an element's border-box size as laid out, like a ResizeObserver; scrolling and transforms never change it, and it is updated before painting in every frame that lays out (paint-phase readers such as a `TranslateFn` follow in that frame).
- `h.Inert(true)`: HTML's `inert` attribute. The subtree is not hit by the pointer, cannot take the focus (a focused element in it loses it), leaves the Tab order and is hidden from assistive technologies, telatest's locators and the aria snapshot; it is still painted.
- `h.Icon(icon, props...)` and `tela.Icon{SVG: "<svg …>"}`: SVG icons drawn as vectors, 1em by 1em unless sized, colored by the text color (`text-*`, theme tokens, `TextColorFn` and transitions recolor them without parsing again), decorative (hidden from accessibility) unless they have an `AriaLabel` or `AriaLabelledBy`. `tela.ParseSVG(data)` checks a document and `tela.MustSVG(data)` panics on a bad one; icons are parsed once per text and cached.
- `Img` sources: `h.SrcPath(path)` (the file systems registered with `tela.Assets`, like `bg-[url(…)]`), `h.SrcBytes(data)` and `h.SrcFS(fsys, path)`. PNG, JPEG, GIF, WebP and SVG are recognized by their content; SVG stays vector. Sources are decoded once and cached; a missing or undecodable one is reported once in the dev log.
- `h.Tint(color)`: paints an `Img` (raster or SVG) in one color through its alpha, for monochrome icons.
- `object-contain`, `object-cover`, `object-fill`, `object-none`, `object-scale-down` and the `object-*` positions for `Img`.
- SVG paint utilities: `fill-*` and `stroke-*` (palette, tokens, `/NN`, `fill-current`, `fill-none`, arbitrary colors) and `stroke-2`, `stroke-[1.5]` (stroke width), inherited like CSS, so a class on a button reaches its icons.
- CSS masks (Tailwind v4): `mask-[url(…)]` (PNG, JPEG, GIF, WebP or SVG through `tela.Assets`) with `mask-contain`, `mask-cover`, `mask-size-[…]`, positions, repeat modes, `mask-origin-*`, `mask-clip-*`, `mask-luminance` and `mask-add`/`subtract`/`intersect`/`exclude`, and the gradient masks `mask-t-from-*` … `mask-y-to-*`, `mask-linear-*`, `mask-radial-*` and `mask-conic-*`; where two mask classes set the same value, the last in the class string wins, like every other class (Tailwind uses its stylesheet order). `Div(Class("size-5 bg-[#D97757] mask-[url(icons/claude.png)] mask-contain"))` tints an image.
- SVG files work as background images (`bg-[url(img/pattern.svg)]`), drawn as vectors.
- SVG documents (icons, SVG images): `<style>` sheets with type, class, id and universal selectors, and `<switch>`.
- ARIA props for widget patterns: `h.AriaHasPopup` (`"menu"`, `"listbox"`, `"tree"`, `"grid"`, `"dialog"`, `"true"`, `"false"`), `h.AriaControls(ref)`, `h.AriaActiveDescendant(ref)`, `h.AriaModal`, `h.AriaMultiSelectable`, `h.AriaOrientation` and `h.AriaAutoComplete`. Unknown values panic in dev mode with the closest valid one. The aria snapshot does not print them, as Playwright does not, except that `AriaControls` keeps the role of an element with `Role("presentation")`, as in Playwright.
- `tela icons gen -pkg <name> [-o dir] [-r] [-prefix text] [-suffix text] [-skip-invalid] [-minify=false] [-source text] [-license name] [-license-file file] [-deprecated file] <dir>`: generates a Go file with one exported `tela.Icon` variable per SVG file of `<dir>` (named after the file: `arrow-up.svg` is `ArrowUp`), checking each file with Tela's SVG parser, minifying it (the minified document is verified to draw the same) and keeping the output deterministic. `-deprecated file.json` (an object from icon name to the name of the icon that replaces it) adds a `Deprecated: use X.` paragraph to the listed variables, so gopls and staticcheck flag their uses. There is no table of icons by name, so the linker drops the icons a program does not use. See the Icons wiki page.
- The module `github.com/kuronosu/tela/icons/lucide` (in `icons/lucide`, with its own `go.mod`; tags `icons/lucide/vX.Y.Z` follow Lucide's releases, 1.61.0 first; a regeneration without a new Lucide release is a pre-release `vX.Y.Z-tela.N`): Lucide's 2,171 icons as `lucide.Check`, `lucide.ArrowUpRight`…, used as `h.Icon(lucide.Check, Class("size-4"))`. Lucide's 268 renamed icons (`lucide.AlertCircle`, now `lucide.CircleAlert`) are still there, marked `Deprecated: use X.`. A program that shows 50 of them grows by about 30 KiB. Compared with Chromium at 24 px and scales 1 and 2, the icons differ by 0.5 to 0.9 levels per channel on average (worst icon 3.1).

### Changed

- A text field with the `combobox` role (`Input(Role("combobox"))`) shows its text in the aria snapshot and `tela inspect`, as Playwright prints an `input role="combobox"`: `combobox "Fruit": App`. An element may be named by several references (`aria-labelledby="a b"`) inside the framework (`core.AriaLabelledBy`); `h.AriaLabelledBy` still takes one.
- `aria.ToggleOptions.Selected` is a `*bool` (nil: uncontrolled; `Selected: &on` with `OnChange` controls it), so `DefaultSelected` and `OnChange` can be used together. `aria` is new since v0.7.0, so this breaks no released code. An element that receives two different `h.Ref`s (for instance the props of two `aria` hooks that each made their own ref) panics in dev mode, naming both call sites and the fix; before, the last ref silently won.
- An `Img` with both a width and a height (`w-30 h-10`) is drawn at that size, its image fitted by `object-fit` (stretched by default), as in a browser; it used to take the image's aspect ratio for its height.

- Under `telatest.NoSettle()`, `AriaSnapshot`, `Count`, `Bounds`, `Stats` and `Cursor` no longer run the clock for animations: they handle only the work due, like the events of an action. Expectations still poll, but their first try handles only the work due and the clock moves only between failed tries. The wait before an action still settles fully. Tests that relied on an expectation or a query to finish an animation under `NoSettle` call `app.Advance(d)` instead.
- A single `nil` dependency list (`UseEffect(f, nil)`) panics in dev mode with the fix (`tela.Once`, `tela.UseMount` or `tela.UseOnce`); outside dev mode it keeps meaning "mount only". It was a warning in v0.7.0.

### Fixed

- `aria.UseMenuTrigger` focuses its trigger when a mouse or a pen presses it, before the menu opens, so that closing the menu (Escape, choosing an item) gives the focus back to the trigger; it used to give it back to nothing after a mouse open.
- The `checked:` variant, and `peer-checked:` and `group-checked:`, now match an element whose `h.AriaChecked` is true (a checkbox or switch, such as one made with `aria.UseCheckbox`); they never matched anything before, since Tela has no native checkbox. A false or mixed value does not match, and only the elements whose classes test it restyle.
- A touch that becomes a scroll now sends `pointercancel` to the element the finger pressed, as a browser does, so `OnPointerCancel` handlers (and `aria.UsePress`) end the press instead of seeing a release after the scroll.
- `telatest` no longer cuts finite animations short on a slow or busy machine: whether the app goes quiet is decided in app time, so `Mount`, actions and `Golden` wait for transitions and springs to end however long their frames take, and only an app still requesting frames after the timeout of app time is reported as endless. Goldens could be captured mid-animation before.

### Breaking

- Removed `h.Id`, `tela.UseId`, `tela.Key` and `tela.Code`, deprecated in v0.7.0: use `h.ID`, `tela.UseID`, `tela.KeyName` and `tela.KeyCode`.
- `h.VirtualList` is generic over its `renderItem` (`VirtualList[R ~func(int) tela.Node | ~func(int) tela.Component]`): a call that passed `nil` as `renderItem` no longer compiles (it panicked before).

### Migration

| Before (v0.7) | After |
|---|---|
| `Input(Id("email"))` | `Input(ID("email"))` |
| `id := tela.UseId()` | `id := tela.UseID()` |
| `var k tela.Key`, `var c tela.Code` | `var k tela.KeyName`, `var c tela.KeyCode` |
| `tela.UseEffect(f, nil)` (a dev panic now) | `tela.UseEffect(f, tela.Once)` or `tela.UseMount(f)` |
| `tela.UseMemo(f, nil)` | `tela.UseMemo(f, tela.Once)` or `tela.UseOnce(f)` |
| `OnClick(func(e tela.MouseEvent) { e.StopPropagation(); ctrl.Remove() })` | `OnClick(tela.StopPropagation(ctrl.Remove))` (optional) |
| `Map(make([]int, 5), func(int) tela.Node { … })` for a fixed count (a dev warning about keys) | `Range(5, func(i int) tela.Node { … })` |
| `app.ExpectText(…)` or `app.Count(…)` ending an animation under `NoSettle` | `app.Advance(d)` |

## [0.7.0] - 2026-10-21

### Added

- Dev mode is on by default in development builds (`go run`, `go build` in a checkout, test binaries) and off in release builds (`-trimpath` or `go install pkg@version`); `App.Dev` and `TELA_DEV=0/1` override it, and `telatest` mounts in dev mode unless given `telatest.NoDev()`. Unknown classes, roles, input types and easings are reported without setting anything.
- `tela.Main(app)` runs the app, prints the error and exits 1.
- Hooks: `UseMount`, `UseOnce`, `UseEffectDo`, `UseUpdateEffect`, `UseInterval` (on the frame clock, so `telatest.Advance` drives it) and `UseGo` (a goroutine with a `context.Context` cancelled at cleanup); `Setter.IsZero` and `Callback.IsZero`.
- Dev diagnostics: a hook called from a function that is neither the component being built nor a `UseXxx` custom hook panics with the fix (wrap it in `tela.C`/`tela.CP`); mounting a component call (`telatest.Mount(t, Counter())`) explains the missing boundary; warnings for a `UseContext` without a Provider whose default holds a zero `Setter`/nil func, a `Label` that labels no control, keyless or duplicate keys per `Map` call site, `Value.Get` in a handler while changes are queued (use `Latest()`), and `nil` dependency lists.
- Struct components: any value with a `Build() tela.Node` method (value receiver) is a component with its own state, passed as a child like an element: `Div(Card{Title: "Inbox", Children: Fragment(...)})`. Children are an ordinary `tela.Component` field; equal props skip the build like `tela.CP`, compared safely (a non-comparable value counts as changed). Dev mode panics on pointer components, on a Prop in a `tela.Component` field and on a direct `Build()` call. `App.Root` takes a component (`Root: Main{}` or `Root: tela.C(Counter)`), and `h.If`/`h.IfElse` accept struct components (`If(loading, Spinner{})`).
- `tela.Once` as a dependency means "only on mount" for any hook (`tela.UseGo(f, tela.Once)`); `tela.Initial(v)` gives `UseSpring`/`UseTween` a start value.
- `h.ID`, `tela.UseID`, `tela.KeyName` and `tela.KeyCode` (new names; see Deprecated).
- Theme tokens with shadcn/ui's names (`bg-primary`, `text-muted-foreground`, `border-border`, `ring-ring/50`, `from-chart-1`…) in every color utility, with light and dark values that follow the color scheme without `dark:` pairs: `App.Theme`, `tela.DefaultTheme()` (shadcn neutral), `tela.SetTheme`, `tela.UseTheme()`, `App.ClassAliases` (`"text-muted": "text-sm text-muted-foreground"`), `h.Scheme(tela.Dark)` for a subtree with its own scheme, `telatest.Theme`/`telatest.ClassAliases`. Matched against Chromium with shadcn's CSS (mean ≤ 0.95 levels).
- Right-to-left layout: `h.Dir("rtl"|"ltr"|"auto")`, `rtl:`/`ltr:` variants, logical utilities (`ms-`, `me-`, `ps-`, `pe-`, `start-`, `end-`, `border-s/e`, `rounded-s/e…`, plus `inset-s/e` and block-axis ones) and mirrored flex, grid, positioning, text alignment, inputs and scroll containers, matched against Chromium. `examples/i18n` switches between English, Spanish and Arabic at runtime.
- Less boilerplate in `h` and `tela`: `Class(parts...)` joins and skips empty parts; every `On…` handler takes `func()` or `func(Event)`; `tela.Toggle(set)`; `Bind` accepts a `func(string)`; `C`, `CP` and `Memo` take an optional `Key(...)`; `h.MapKeyed(items, key, render)` (render may return a struct component); `h.Nodes(slice)`; `tela.Easing` with `EaseLinear`/`EaseIn`/`EaseOut`/`EaseInOut` and `CubicBezier`; `tela.UsePresenceTween` for manual exits; `tela.MapValue` and `h.Scale` for deferred style values; `tela.Box`/`tela.WH` style packs, and a style prop is accepted directly as a child (`Div(tela.Width(10))`).
- `VirtualList`: `h.ItemKey` so state follows the item, `h.ItemHeight(px)` for fixed rows, and `list`/`listitem` roles by default.
- Previews with seeded state: `tela.Previews` entries are `tela.Preview{Root, Wrap}` (local state through struct-component fields, context state through `Wrap` with a Provider); `func Preview()` may return a struct component; a package's `var App = tela.App{...}` is read by `tela snapshot`/`inspect` (size, `RootClass`, `ColorScheme`, `Theme`, `ClassAliases`, `Dev`) and by `telatest.MountApp`/`telatest.FromApp`, so tests, snapshots and the window share one `App`; `tela snapshot`/`inspect` now print dev warnings and class errors on stderr.
- `telatest`: `MountFunc`, `GoldenScales(name, 1, 2)` (one golden per scale, same app state), `ClickAt`/`DragFromTo` in window coordinates, `PixelAt`, `Measure(func())` for build/layout/paint deltas, and `Expect(locator, Checked(true), Disabled(false), HasValue("x")…)` state assertions with the node's states in the failure message.
- `h.TestID(id)` and `telatest.ByTestID(id)`, test-only ids that never reach the aria snapshot or accessibility output.
- `tela/i18n`: JSON message catalogs with ICU MessageFormat (`plural`, `select`, `selectordinal`), CLDR plural rules, locale-aware numbers, dates, times and lists, the system locale by default, runtime switching with `SetLocale` (only components that call `UseT`/`UseLocale` rebuild), `Provider` for a subtree's locale and `Dir` for text direction. New dependency: `golang.org/x/text`.

### Deprecated

- `h.Id`, `tela.UseId`, `tela.Key` and `tela.Code`: use `h.ID`, `tela.UseID`, `tela.KeyName` and `tela.KeyCode` (so `tela` and `h` can both be dot-imported). Removed in v0.8.0.
- `nil` as a hook's dependency list (mount-only today): use `tela.Once` (`UseEffect(f, tela.Once)`), `tela.UseMount` or `tela.UseOnce`. Dev mode warns now; it will be rejected in v0.8.0.

### Breaking

- `App.Root` is a `tela.Component` instead of a `func() Node`: write `Root: tela.C(Counter)` or `Root: Main{}`.
- `h.If` and `h.IfElse` return a `tela.Child` (an absent branch renders and applies nothing): returning `If(...)` where a `tela.Node` is required needs `Fragment(If(...))`, and `[]tela.Prop{If(...)}` no longer compiles. The optional key argument of `C`/`CP`/`Memo` is a `...tela.Child`.
- `tela.Previews` is a `map[string]tela.Preview`; the CLI no longer wraps `Preview()` in `tela.C` (see Migration).
- `h.OnClick(nil)` and other untyped-nil handlers no longer compile (write `OnClick[func()](nil)` or omit the prop); `tela.StyleProp` is a struct instead of an alias; `VirtualList` rows no longer need (and should not add) `Role("listitem")`.
- `tela.UseSpring` and `tela.UseTween` take the target only; the start value is the `tela.Initial` option. Before: `tela.UseSpring(float32(0), target, opts...)`; after: `tela.UseSpring(target, tela.Initial(float32(0)), opts...)`.
- `h.Key` and `tela.WithKey` are generic over `comparable`: a slice, map or func key no longer compiles (it panicked before). `WithKey` takes a `tela.Component` (a Node or a struct component).
- `tela.Child` is no longer sealed: it is any value with `Build() tela.Node` (`Div(42)` still does not compile). `telatest.Mount`, `Render`, `AriaSnapshot` and `LayoutJSON` take a `tela.Component`.
- The logical utilities `ms-`, `me-`, `ps-`, `pe-`, `start-`, `end-`, `border-s/e` and `rounded-s/e…` follow the element's direction (unchanged in left-to-right content).
- In dev mode, calling a stateful component as a plain function from another component (its hooks landing in the caller's slots) now panics; wrap it in `tela.C`/`tela.CP`. `Preview` functions of the examples return `tela.C(X)`.
- `telatest.Dev()` is a no-op (dev mode is the default); keyless keys, anonymous `CP`/`Memo` functions and non-comparable deps are warnings logged by `telatest` instead of test failures.
- `telatest.App.Bounds` returns a `tela.Rect` (window coordinates, like `ElementRef.Bounds`) instead of four numbers.

### Migration

Every change below fails to compile or is reported by dev mode, so the compiler and `go test` find all the sites.

| Before (v0.6) | After (v0.7) |
|---|---|
| `tela.Run(app)` (error dropped) | `tela.Main(app)` |
| `tela.App{Root: Counter}` | `tela.App{Root: tela.C(Counter)}` or `Root: Main{}` |
| `return If(show, X)` in a function returning `tela.Node` | `return Fragment(If(show, X))` |
| `Class("p-4 " + extra)` | `Class("p-4", extra)` |
| `OnClick(func(tela.MouseEvent) { set.Set(!open) })` | `OnClick(tela.Toggle(set))` |
| `WithKey(tela.CP(Row, item), item.ID)` | `tela.CP(Row, item, Key(item.ID))` or `h.MapKeyed(items, keyFn, render)` |
| `func Card(p CardProps) tela.Node` + `tela.CP(Card, p)` | optionally `type Card struct{...}` + `func (c Card) Build() tela.Node`, used as `Card{...}` |
| `Card()` called directly from another component (its hooks in the caller) | dev panic: `tela.C(Card)` / `tela.CP(Card, p)`, or a struct component |
| `tela.UseEffect(f, nil)` (mount only) | `tela.UseEffect(f, tela.Once)` or `tela.UseMount(f)` |
| `tela.UseSpring(float32(0), target, opts...)` | `tela.UseSpring(target, tela.Initial(float32(0)), opts...)` (same for `UseTween`) |
| `h.Key(sliceOrMap)` | a comparable key (an ID) |
| `h.Id("x")`, `tela.UseId()`, `tela.Key`, `tela.Code` | `h.ID("x")`, `tela.UseID()`, `tela.KeyName`, `tela.KeyCode` |
| `var Previews = tela.Previews{"Login": Login}` | `var Previews = tela.Previews{"Login": {Root: tela.C(Login)}}` |
| `func Preview() tela.Node { return Counter() }` (the CLI wrapped it) | `func Preview() tela.Node { return tela.C(Counter) }` |
| `telatest.Mount(t, tela.C(Previews[name]))` | `telatest.Mount(t, Previews[name])` |
| `x, y, w, h := app.Bounds(loc)` | `r := app.Bounds(loc)` (a `tela.Rect`) |
| `Role("listitem")` on `VirtualList` rows | nothing (default roles) |
| `ms-`/`me-`/`start-`/`end-` meant left/right | they follow `h.Dir`; use `ml-`/`mr-`/`left-`/`right-` for physical sides |

## [0.6.0] - 2026-10-19

### Added

- Fractional scaling on Wayland (`wp_fractional_scale_v1` + `wp_viewporter`): windows render at the compositor's exact scale (1.25, 1.5, 1.75…) instead of a rounded-up integer one, with pixel-shifting scroll frames at fractional scales as at integer ones.
- `fixed` positioning (to the window, or to a transformed/filtered ancestor, as in browsers) and `sticky` positioning (stuck and pushed out within scroll containers, the window and `VirtualList` section headers), matched against Chromium.
- `h.Portal(...)`: content shown above the window's content (modals, menus, tooltips) while state, context and event bubbling follow the component tree.
- Color scheme: `tela.UseColorScheme()`, `tela.UseReducedMotion()`, `tela.SetColorScheme(tela.Light|tela.Dark|tela.System)`, `App.ColorScheme` and `telatest.ColorScheme(...)`; `dark:` follows the app's choice, else the system's.
- Explicit animations: `tela.UseSpring`, `tela.UseTween` and `tela.Animate` drive a `*tela.Value[T]` (`float32`, `Color`, `Point`, `Size`, `Rect`) without rebuilding, with `Stiffness`, `Damping`, `Bounce`, `Duration`, `Ease`, `Spring`, `Velocity` and `OnDone` options; new `tela.Point` and `tela.Size`.
- Enter and exit animations: the `starting:` variant (CSS `@starting-style`), `h.Presence` with `h.Exit(classes)`, and `tela.UsePresence()` for manual exits; `examples/motion`.
- `telatest.NoSettle()` captures frames mid-transition after `Advance`.
- Deferred color props `tela.BgFn` and `tela.TextColorFn`: animated colors repaint without rebuilding.
- `mix-blend-*` and `bg-blend-*`, `[clip-path:…]` with `inset()`, `circle()`, `ellipse()` and `polygon()` (clips painting and hit-testing; transitions with `transition-[clip-path]`), background images from files registered with `tela.Assets(fs.FS)` (`bg-[url(img/hero.jpg)]`, PNG, JPEG, GIF, WebP) or `Style(tela.BgImage(img))` with size, position, repeat, origin and clip, and `bg-clip-text` gradient and image text; matched against Chromium in 62 cells.
- Layout animations: `h.AnimateLayout()` animates an element from its old box to its new one (reorders, expand/collapse) with FLIP transforms, and `h.LayoutID(id)` animates between two elements with the same ID (shared element transitions); the timing comes from `duration-*`/`ease-*` or a default spring. `examples/layout`.
- `examples/dashboard` rebuilt as a phase-6 showcase: grid layout, sticky glass header and sidebar, an image backdrop with `tela.Assets`, `bg-clip-text`, springs for numbers and charts, `AnimateLayout`/`LayoutID` lists and tabs, a `Presence` dialog and toast in a `Portal`, and a Day/Night switch; it renders at 60 fps at 1080p on CPU (p95 frame 14.6 ms headless; 12–14 ms live on Wayland while scrolling).
- Scaled images draw about 4× faster (axis-aligned bilinear sampling reuses blended source rows; pixels unchanged).
- Large blurs (`blur-2xl`, `backdrop-blur-xl`…) are 2–4× faster, backdrop filters render in parallel and only around what changed: a 1080p glass frame rasterizes in about 16 ms (was 46), a small change under a glass panel in about 5 ms (was 40).
- CSS grid: `grid`/`inline-grid` with fixed, `%`, `fr`, `auto`, `min-content`, `max-content`, `minmax()`, `fit-content()` and `repeat()` tracks (incl. `auto-fill`/`auto-fit`), auto and dense placement, spans, named lines (`grid-cols-[[sidebar]_200px_[main]_1fr]`, `col-[main]`), implicit tracks, gaps and alignment; Tailwind v4's `col-N`/`row-N`. All 553 included Taffy grid fixtures match Chromium.
- CSS transitions and animations: `transition`, `duration-*`, `ease-*` and `delay-*` animate colors, opacity, transforms, shadows, filters, sizes and spacing when state, classes or the environment change (CSS Transitions semantics, matched against Chromium); `animate-spin`, `animate-ping`, `animate-pulse` and `animate-bounce`; reduced motion ends transitions at once. Transform and opacity animations repaint without rebuilding or relayout, and no frames are requested once they end.
- All CSS blend modes in the renderer (the 16 `mix-blend-mode` modes plus `plus-lighter`), mask layers and two-point (focal) radial gradients; color emoji now composite exactly (flags went from up to 3.7 to under 1.7 mean levels against Chromium).

### Fixed

- A box's opacity now fades its `backdrop-filter` too (an `opacity-0` glass panel no longer blurs what is behind it), as in Chromium.
- `drop-shadow-*` blurs with its radius as the standard deviation, as browsers do (it was half as wide).
- An absolutely positioned box inside a transformed, filtered or backdrop-filtered ancestor is now positioned against that ancestor, as in browsers.
- Color emoji in transparent or translucent text now disappear or fade like the text, as in Chromium.
- Text selection highlights and scrollbar thumbs snap to device pixels, so selected lines no longer show a light seam at fractional scales (the 1x and 2x selection edges now match Chromium).

## [0.5.2] - 2026-10-14

### Added

- Documentation of the whole public API: wiki sources in `docs/wiki/` (guides plus API reference pages generated by `internal/gen/wikidoc` from the doc comments), runnable `Example*` tests in `h` and `telatest`, and a pkg.go.dev badge in the README.

- `examples/native`: a file explorer in Windows 11 (Fluent), GNOME (Adwaita) and KDE (Breeze) looks, each in light and dark, with `-theme` and `-dark` flags. Code only: themes are values inside the example, no new API.

### Changed

- The examples' application ID is `dev.telaui.<name>` (was `dev.tela.<name>`): the Wayland `app_id` and the X11 `WM_CLASS` change, so window rules and desktop files that match it need updating.

### Fixed

- Doc comments render correctly on pkg.go.dev (reflowed paragraphs, a code block read as a list, a broken `[Value]s` link); the CLI docs no longer say the package must be inside the Tela module.

## [0.5.1] - 2026-10-14

### Fixed

- The repository checks text files out with LF on every OS (`.gitattributes`), so the tests pass on Windows checkouts, where goldens and generated files were read with CRLF.
- CI: the race job runs `-short` with a 45-minute timeout; the Wine job starts the wineserver before `go test` and runs `-short`; live keyboard and IME tests skip on runners without a keyboard layout or a composing IME.

## [0.5.0] - 2026-10-13

### Added

- `examples/hello`: a static headline screen, the hero image of the README.
- Windows backend (`win32`): windows, per-monitor DPI, mouse, wheel, cursors, keyboard (layouts, AltGr, dead keys), clipboard and system settings (dark mode, reduced motion, dark title bar); `tela.Run` picks it on Windows (`TELA_BACKEND=win32|headless`).
- Wayland clipboard (`wl_data_device`), used by the text fields.
- Inline elements in paragraphs (`Span`, `Code`, `A`, …) paint backgrounds, borders, radii and shadows per line fragment, and their horizontal padding, borders and margins take space, like CSS inline boxes; buttons, `inline-block`/`inline-flex` elements and inline images sit on the line as atomic inlines.
- Classes `align-baseline`, `align-top`, `align-middle`, `align-bottom`, `align-text-top`, `align-text-bottom`.
- `tela inspect --layout` and `telatest.LayoutJSON`: paragraphs list their inline `fragments`; atomic inline children carry `inlineBox: true`.
- `ElementRef.Bounds()` and `tela.UseBounds(ref)` (a `*Value[tela.Rect]` that follows layout, scroll and transforms), with the new `tela.Rect`.
- `h.If` accepts props as well as nodes (generic over `tela.Child`); `h.IfElse`, `h.MapIndex`, `h.TextFn` (text read in the layout phase, no rebuild).
- `Value.Latest()`: the value with every queued `Set`/`Update` applied.
- `tela inspect --layout` and `telatest.LayoutJSON` add `bounds` (transformed, scrolled box in window coordinates) when it differs from `rect`.
- Negative `-space-x-*` and `-space-y-*`.
- Text fields: `h.Input` and `h.Textarea` (controlled `Value` + `OnChange` on every edit, uncontrolled `DefaultValue`, `Bind`, `BindValue`; `Type`, `Rows`, `Placeholder`, `ReadOnly`, `MaxLength`, `Name`, `Id`, `AutoComplete`, `SpellCheck`), with selection, undo/redo, clipboard, IME composition, caret blink that stops when idle, and the events `OnBeforeInput` (`tela.InputEvent`), `OnCompositionStart/Update/End` (`tela.CompositionEvent`), `OnSelect` (`tela.SelectEvent`).
- `h.Form` with `OnSubmit` (`tela.SubmitEvent`) and implicit submission; `h.Label` with `For(id)` or a nested control; `Button` `Type("submit"|"button"|"reset")`.
- `ElementRef.Value`, `Select`, `SetSelectionRange`, `SelectionRange`.
- Classes and variants `caret-*`, `field-sizing-content|fixed`, `placeholder:`, `selection:`, `read-only:`, `placeholder-shown:`, `invalid:`.
- `telatest.Fill` and `telatest.ExpectValue`; `Type` now types into text fields.
- IME on Wayland (`text-input-v3`): Japanese and other input methods compose inline in text fields.
- IME on Windows (IMM32): inline composition in text fields, the candidate window at the caret, and the IME off in password fields.
- `examples/signup`: a "Create your account" form screen with every text field feature.
- `resize-none` is accepted (Tela draws no resize grip).
- `LICENSE`: BSD 3-Clause.

### Changed

- A `Button` or `inline-block` alone in a block container now sits on a line box, as in CSS (it gains the line's strut below it).
- Underlines snap to device pixels like Chromium (one crisp row instead of a blurred two-row line).
- `space-x-*`/`space-y-*` are margins on every child but the last in flex containers too (Tailwind v4), not gaps; a child's own margin on that side wins.
- `tela snapshot`, `inspect`, `telatest.Render` and the other headless renders run frames until the tree settles (at most 8), so values set by effects and `UseBounds` show up.
- Alt chords (Alt+letter) no longer type into text fields on Windows and Linux, as in Chromium; AltGr still types.
- With duplicate `Id`s, the first mounted element in tree order is the one `Label(For(id))` finds (HTML's `getElementById`).

### Fixed

- A `Button` takes the accessible name of its `Label`, as in Chromium.
- Borders with different side widths no longer paint one side over the opposite one.
- Windows: Alt+numpad codes (Alt+0233) type their character instead of the digits.
- An IME deletion of surrounding text fires `OnBeforeInput` before the change, like every other edit.

## [0.4.0] - 2026-10-13

### Added

- Hooks `UseReducer`, `UseEffect`, `UseLayoutEffect`, `UseMemo`,
  `UseCallback`, `UseRef`, `CreateContext`/`Provider`/`UseContext`, `UseId`,
  `UseStateSet` and `UseStateUpdate`.
- Component boundaries `tela.C`, `tela.CP` and `tela.Memo`; `CP` and `Memo`
  skip the build when the props are equal and no state or context they read
  changed.
- `tela.Post` runs a function on the UI thread.
- Dev mode (`TELA_DEV=1`): a change in hook order panics with both call
  sites; non-comparable dependencies, a changing dependency count and
  anonymous functions passed to `CP`/`Memo` are reported.
- `tela.UseValue` and `tela.Value[T]`: an observable value (`Get`, `Set`,
  `Update`, safe from any goroutine). Read inside `TranslateFn`/`OpacityFn`,
  a change repaints only; inside `WidthFn`/`HeightFn`, it lays out only that
  element; in a component's body, it rebuilds that component.
- Wayland keyboard input: key events with physical code, logical key, text
  and modifiers for any XKB layout (pure-Go keymap parser), dead keys and
  Compose sequences, and client-side key repeat. Pointer and scroll events
  now carry the keyboard modifiers.
- X11: keyboard input with the same contract as Wayland (layouts, AltGr,
  dead keys, Compose, server key repeat), smooth touchpad and
  high-resolution wheel scrolling through XInput2, and pointer modifiers
  from the real modifier mapping.
- Pointer input: `OnClick`, `OnMouseDown`, `OnMouseUp`, `OnMouseEnter` and
  `OnMouseLeave` now fire, plus the new `OnDoubleClick`, `OnContextMenu`,
  `OnMouseMove`, `OnPointerDown/Move/Up/Cancel/Enter/Leave`,
  `OnDragStart`/`OnDrag`/`OnDragEnd` and `OnLongPress`. Events bubble, with
  `StopPropagation` and `PreventDefault`. `tela.MouseEvent` gains `LocalX/Y`,
  `Buttons`, modifier fields and `ClickCount`; new `tela.PointerEvent`,
  `tela.DragEvent`, `tela.MouseButtons` and `tela.PointerKind`.
- `h.Disabled` and the `hover:`, `active:`, `disabled:`, `enabled:`,
  `group-*` and `peer-*` variants; only elements whose classes test a state
  are restyled. The window cursor follows `cursor-*` under the pointer.
- Keyboard events: `h.OnKeyDown`/`h.OnKeyUp` with `tela.KeyboardEvent`
  (`Key`, `Code`, `Location`, `Repeat`, modifiers); `tela.Key`/`tela.Code`
  constants for every W3C key and code.
- Focus: `h.TabIndex`, `h.AutoFocus`, `h.OnFocus`/`h.OnBlur` (bubbling),
  `tela.UseElementRef` + `h.Ref` with `Focus`/`Blur`/`Focused`; Tab and
  Shift+Tab navigation, Enter/Space activate buttons, a click focuses.
- `focus:`, `focus-visible:` (keyboard navigation, not clicks) and
  `focus-within:` variants, with their `group-`/`peer-` forms.
- The §1 Counter is interactive, by mouse and by keyboard.
- Scrolling: elements with `overflow-auto`/`overflow-scroll` scroll with the
  wheel, the touchpad (with kinetic flings on Wayland), touch, the keyboard
  (arrows, Page Up/Down, Home/End, Space) and their scrollbars;
  `overflow-hidden` scrolls from code. Scrolling chains to scrolling
  ancestors unless `overscroll-contain`/`overscroll-none`. The window
  scrolls like a browser page when its content is larger than the window
  (`RootClass: "overflow-hidden"` turns it off).
- `h.OnWheel` and `h.OnScroll`, with `tela.WheelEvent` and
  `tela.ScrollEvent`; `tela.ElementRef` gains `ScrollTo`, `ScrollBy`,
  `ScrollIntoView` and `ScrollOffset`; keyboard focus scrolls the focused
  element into view; `tela.UseScroll(ref)` returns Values that follow an
  element's scroll offsets, for scroll-linked effects without rebuilds.
- Tela draws the scrollbars: thin overlay bars that fade by default, or
  classic always-visible bars with `tela.App{ClassicScrollbars: true}` or
  `scrollbar-classic` (`scrollbar-overlay` per element). Wheel and keyboard
  scrolling animate; `tela.App{InstantScroll: true}` or `scroll-instant`
  turns that off, `scroll-smooth` animates programmatic scrolls, and the
  system's reduced-motion setting always disables it.
- `h.Role`, `h.Alt` and the `h.Aria…` props (label, labelledby, description,
  hidden, checked, pressed, selected, expanded, disabled, level, value
  now/min/max/text); unknown roles panic in dev mode with a suggestion. A
  semantics tree with implicit roles and accessible names.
- `telatest`, the public package to test UIs without a window: `Mount`,
  role/text/class locators with strictness and suggestions, real pointer
  and key input (`Click`, `Hover`, `Drag`, `Scroll`, `Press`, `Type`), a
  controllable clock, auto-waiting expectations (`ExpectText`,
  `ExpectRole`, `ExpectAria`…), goldens and screenshots; `telatest.Render`,
  `AriaSnapshot` and `LayoutJSON` are a public headless entry point.
- `h.VirtualList` and `h.Overscan`: lists that build only the rows in view,
  with variable heights measured on the fly and a stable scroll position;
  `ElementRef.ScrollToIndex` with `tela.ScrollAlign` (`AlignAuto`,
  `AlignStart`, `AlignCenter`, `AlignEnd`); `examples/virtuallist`, a
  100,000-row directory with jump buttons.
- Debug overlays: `TELA_DEBUG=layout,paint-flash,layers,fps,semantics` (or
  `all`) draws box models, paint flashes, layer borders, a frame meter and
  semantics bounds over the app, at no cost when off; `telatest.Debug(...)`
  turns them on in tests, which otherwise ignore `TELA_DEBUG`.
- Examples `dashboard`, `pricing`, `settings`, `player` and `board`: full
  screens in the shadcn/ui zinc style with dark mode, goldens and
  interaction tests (switches and `disabled:` states, a progress bar driven
  by `UseValue` without rebuilds, drag and drop with `OnDrag*` and
  `TranslateFn`). The notes of `counter`, `shapes` and `text` saying that
  events arrive in phase 4 are gone.

### Changed

- Scrolling a long list with many `overflow-hidden`/`truncate` boxes no
  longer walks every box each frame; unmounting many such boxes is faster.
- Windows rasterize only what changed and pass the damage rectangles to the
  platform: scrolling moves the pixels already drawn and draws only the
  newly exposed strips; a `TranslateFn` drag or a `UseValue` progress bar
  redraws only its old and new place. A 10,000-element list scrolls in
  about 0.6 ms per frame at 800×600 scale 2 (was about 6 ms).
- **Breaking:** `tela inspect` prints a Playwright-style aria snapshot
  (YAML) by default; `--layout` prints the JSON layout tree.
- `tela snapshot` and `tela inspect` work on any module that requires Tela,
  not only inside the Tela module (`Scene` packages still need it).
- **Breaking:** `tela.UseState` returns a `tela.Setter[T]` instead of
  `func(T)`. Write `setCount.Set(v)` or
  `setCount.Update(func(n int) int { return n + 1 })`, or use
  `tela.UseStateSet` for a plain function.

### Fixed

- Rendering a region of a frame with a spreading `backdrop-blur` no longer
  re-renders the whole frame.

### Removed

- The `github.com/jezek/xgb` module dependency: a patched copy of its core,
  `xproto` and `bigreq` is vendored under `internal/` (ADR-025).

## [0.3.1] - 2026-10-12

### Added

- `tela version` prints the module version, the VCS revision and the Go
  version and platform.

### Changed

- `examples/shapes` is rewritten with the public API (`tela`, `h`, Tailwind
  classes): the same 12-card page as components, with real labels; shapes
  that need arbitrary paths are replaced by CSS equivalents (listed in its
  README). The phase-1 DisplayList scene lives on as a golden test of
  `internal/gfx/raster`. Its cards keep their size and wrap, so a tiled
  window of about 700 px shows three columns instead of squeezing four.

### Fixed

- A length with both a pixel and a percentage part (`w-[calc(100%-2rem)]`,
  percentage `space-*` added to a margin) kept only its percentage; layout
  now resolves `calc()` of a length and a percentage like Chromium.

## [0.3.0] - 2026-10-12

Phase 3: tree, style, layout. The ARCHITECTURE §1 `Counter` renders like its
HTML + Tailwind twin in Chromium.

### Added

- Public packages `tela` (`App`, `Run`, `Node`, `Child`, `UseState`,
  `Previews`, colors, typed style props) and `h` (elements, `Class`, `Style`,
  `Key`, `If`, `Map`, `Fragment`, event props, stored until phase 4).
- `tela snapshot` takes `Preview()` or `Previews` packages, with `--dark` and
  `--preview`; `tela inspect` prints the layout tree as JSON.
- `examples/counter`, compared with Chromium (layout within 0.01 px, pixels
  equal except glyph antialiasing).
- Tailwind CSS v4.3.3 theme generator (`go generate ./internal/style`).
- `internal/style`: `ComputedStyle`, an interned Tailwind class parser with
  arbitrary values, `/NN` opacity, variants and suggestions for unknown
  classes; Preflight element defaults.
- `internal/layout`: block, flexbox, relative and absolute positioning,
  intrinsic sizes, caching and relayout boundaries; all 828 included Taffy
  v0.14.0 fixtures match Chromium.
- `internal/core`: Node/Element/Box trees, keyed reconciliation, `UseState`,
  a frame-on-demand scheduler, paragraphs, headless rendering and CSS box
  painting (borders, radii, shadows, rings, outlines, gradients, filters,
  backdrop filters, transforms, clipping, stacking contexts) matched against
  Chromium.
- `internal/gfx/raster`: `BackdropFilter`.

### Changed

- Colors are extended sRGB until rasterization, clipped where Chromium clips
  them (ADR-046), instead of CSS gamut mapping.
- Deterministic trigonometry and color conversions everywhere pixels are
  computed (`internal/fmath`, ADR-041).

### Fixed

- Cross-architecture differences in `gfx.Rotate`, `Skew`, `HueRotate`,
  `Point.Len`, COLR transforms and color conversions.
- Review fixes: a data race between raster bands on per-pixel backdrop
  filters, a layout cache key collision, class-cache eviction of strings in
  use, a non-atomic snapshot write, previews declared in `_test.go` files.

## [0.2.0] - 2026-10-12

Phase 2: text. Mixed-script paragraphs (Latin, Arabic, CJK, emoji) render
correctly.

### Added

- Embedded fonts: Inter, JetBrains Mono, Noto Sans Arabic and Noto Color
  Emoji (COLRv1).
- `gfx.DrawGlyphRun` and a raster glyph cache (1/4 px subpixel positions).
- `internal/text`: variable font instances, COLRv1 glyphs, CSS font matching
  and per-grapheme fallback, Parley-style paragraph layout (bidi, shaping,
  UAX #14 line breaking, white-space, justification, ellipsis, line clamp,
  inline boxes), cursors, selection, hit-testing and navigation, optional
  system fonts.
- `examples/text`.

### Fixed

- Fused multiply-adds in `gfx.Affine`.
- Review fixes: a hang on malformed cmap ranges, system font scans blocking
  registered fonts, CJK region ranking.

## [0.1.0] - 2026-10-11

Phase 1: pixels on screen.

### Added

- `internal/gfx`: geometry, colors, paths, paints and the immutable
  DisplayList.
- `internal/gfx/raster`: deterministic CPU rasterizer with analytic
  antialiasing, gradients, clips, layers, strokes and dashes, analytic
  rounded-rect shadows, Gaussian blur and parallel bands.
- `internal/platform`: the backend interface, and headless, Wayland and X11
  backends.
- `examples/shapes` and `tela snapshot`.

[Unreleased]: https://github.com/kuronosu/tela/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/kuronosu/tela/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/kuronosu/tela/compare/v0.5.2...v0.6.0
[0.5.2]: https://github.com/kuronosu/tela/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/kuronosu/tela/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/kuronosu/tela/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/kuronosu/tela/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/kuronosu/tela/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/kuronosu/tela/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/kuronosu/tela/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/kuronosu/tela/releases/tag/v0.1.0
