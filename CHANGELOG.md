# Changelog

All notable changes to Tela are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/) as Go modules use it: while Tela
is v0, a release that breaks the public API raises the minor version and any
other release (features, fixes, a roadmap phase) the patch version (ADR-208). Compatibility covers the public API only (`tela`,
`h`, the `tela` command's flags, the `Preview()`/`Previews` contract and the
`tela inspect` JSON); everything under `internal/` may change at any time.
In v0 a minor version may break the public API; such changes are listed
under **Breaking**. See ADR-049 and ADR-208.

## [Unreleased]

## [0.14.0] - 2026-11-08

Phase 20, API and kit gaps (ADR-247 to ADR-254): submenus, transparent windows, new kit pieces, deferred forms, built-in labels, and app and desktop gaps. It breaks `desktop.Launch.Forwarded`.

### Added

- Transparent windows (ADR-249): `tela.App.Transparent` and `tela.Window.Transparent`, with `Frameless`, leave unpainted pixels showing the desktop, so the root's first element floats with its own radius and `shadow-*`. Clicks on pixels under a quarter opaque go to what is below, on Wayland (input region), X11 (SHAPE input; ARGB visual while a compositing manager runs, else opaque and `UseWindow().State().Transparent` is false), Windows (layered window) and macOS (non-opaque window). Goldens and `tela snapshot` keep alpha; `telatest.Transparent()`, `NoCompositing()` and `App.TakesInput`.
- kit (ADR-248): `kit.Alert` (default, destructive and the new `kit.Success`, `kit.Warning`, `kit.Info` tones, also on Badge), `Badge.Dot`, `Button.Loading` (a spinner, `aria-busy`, presses ignored, size kept), `Input.Icon` and `IconEnd`, `Separator.Caption` ("or continue with"), `Avatar.Name` and `Avatar.Color` (`kit.AvatarAuto` picks one from the name), `kit.AvatarGroup` with a "+N", `Field.LabelContent`, `DescriptionContent`, `LabelEnd` and `DescriptionEnd` slots (a RadioGroup or ToggleGroup inside a Field is named by its label), `Table.AlwaysSorted`, `HideUnsortedIcon` and the `kit.Other` sort.
- Animation delays (ADR-248): `delay-*` also delays `animate-*` (as tw-animate-css does), `animation-delay-<ms>`, `-animation-delay-<ms>` and the arbitrary property `[animation-delay:-0.3s]`; the `aria-[sort=other]:` and `aria-[sort=none]:` variants.
- Submenus (ADR-247): `kit.MenuSub` in dropdown, context and menu bar menus (native submenus where the OS draws the menu), on `aria.UseMenu`'s `SubmenuKeys` and `SubmenuTrigger` with React Aria's keys, hover delay and pointer grace. `kit.MenuItem.Role` makes a standard item (Copy, Paste…) next to the app's own; `desktop.PerformRole`; `tela.UsePlatformKeys()`; `WindowHandle.ActiveElement()`.
- Triggers (ADR-247): Toggle, Checkbox, Switch and Slider are `Trigger`s, and overlays nest (`kit.Tooltip{Trigger: kit.DropdownMenu[K]{Trigger: btn}}`); `aria.ContextMenuTriggerResult.Opening`.
- Deferred forms (ADR-250): `tela.BackgroundGradientFn` (a gradient repainted without rebuild or relayout), `h.AriaValueNowFn` and `h.AriaValueTextFn`, `kit.Progress.ValueFn`, `kit.ToggleGroup`'s `ValueFn`, `SelectedFn` and `Indicator` (a sliding pill with `LayoutID`), and `tela.Preview.Theme` and `ColorScheme`, which win over the App's and `--dark` in `tela snapshot`, `inspect`, `mcp` and `dev`.
- Built-in labels (ADR-251): every text Tela shows by itself (the fallback title bar, message boxes, kit's toasts, the macOS app, Edit and Window menus, the save panel's "Format:") has an ID (`tela.Labels()`) translated by `i18n` catalog keys such as `tela.toast.close`; `tela.Label`, `tela.UseLabels`, `i18n.Formatter.Label`, `kit.Toaster.AriaLabel` and `CloseLabel`. Native message box buttons and macOS menus follow the language.
- looks (ADR-254): `kit.ToggleStyle.IconVariants`; `looks/aqua` tints sidebar glyphs with the accent, draws Sequoia's toolbar search field borderless gray (`looks.Regions.SearchField`) and makes Tahoe dark's destructive text readable (the `destructive-text` token).

- App and desktop (ADR-253): `tela.UseCloseRequest` and `tela.UseQuitRequest` answer a window's close request and the app's quit request inside the component tree, before `App.OnClose`, a Window's `OnClose` and `App.OnQuit`; `desktop.Launch.Source` (`LaunchStart`, `LaunchForwarded`, `LaunchOpened`: macOS opened a link or file with the app); `desktop.AppMenuBar.NativeOnly` (the bar only where it is the app's own, with no window bars elsewhere); `desktop.HandlerRole` and `Association.WithRole` (Editor, Viewer, Shell, None) for schemes and file types; `telatest.Monitors` and `InjectLaunch` with `LaunchOpened`.

### Changed

- Text fields (`Input`, `Textarea`) take text drops by default, as HTML's do; a drop zone that takes the text itself calls `PreventDefault` in `OnDrop` (ADR-253).
- A saved window position is fitted to the monitors present at the next start; `desktop` stores see changes other programs make to their files (merged per key; a key this process changed and has not written wins); app-scoped events (launches, notifications, tray, shortcuts) that arrive before the first frame reach their listeners on every backend (ADR-253).
- A right click, or a Control+click on a Mac, on an unselected list box option selects it before the context menu opens, as Finder does (ADR-247); a context menu opened with Shift+F10 or the Menu key opens below the focused element.
- `CmdOrCtrl` in a kit menu shortcut follows the app's platform keys, not the build machine's; a shortcut written in glyphs (`⌘T`) is announced as its ARIA key shortcuts.
- A trigger's own `Disabled` (`kit.Button{Disabled: true}`) now wins over the overlay's trigger props.
- Numbers and dates use the locale's default numbering system from CLDR 48: `ar-EG` and `ar-SA` give Arabic-Indic digits, as ICU does (ADR-251).
- The fallback title bar follows a `Dir` on the element that is the window's whole content, and a click on it (Wayland) closes an open dialog or popover that closes on outside presses (ADR-252).
- kit: a loading `Button` used as an overlay's trigger opens nothing, from the pointer or the keyboard; a radio menu item with a `Role` panics in dev mode; menu keys must be unique across every submenu level (dev mode).
- Windows menu bars give Chinese, Japanese and Korean labels an appended access key ("ファイル(&F)"), and Cyrillic, Greek, Arabic and Hebrew labels a letter of their own (ADR-252).

### Breaking

- `desktop.Launch.Forwarded` is removed: `Launch.Source` (`LaunchStart`, `LaunchForwarded`, `LaunchOpened`) replaces it and tells a forwarded launch from one the OS opened (ADR-253). The owner waived the deprecation cycle: the library has no outside users yet.

## [0.13.7] - 2026-11-08

Phase 19, Accessibility (ADR-246): screen readers read and operate Tela apps on Linux, Windows and macOS.

### Added

- Accessibility props (ADR-246): `h.AriaLive("polite"|"assertive"|"off")` and `h.AriaAtomic(bool)` for live regions (`Role("status")`, `"log"` and `"alert"` are live without them), `h.AriaKeyShortcuts("Control+S Meta+S")` (dev mode rejects `Ctrl`, `Cmd` and empty keys with the right name), `h.AriaPosInSet(i)` and `h.AriaSetSize(n)` for sets that have only some of their items in the tree (otherwise positions are counted as Chromium does), and `h.AriaLabelFn(func() string)`, a name read when the semantics tree is built, so a `tela.Value` it reads rebuilds nothing. The aria snapshot prints none of them, as Playwright's does not.
- macOS: VoiceOver through NSAccessibility (ADR-246): a Tela window exposes its semantics tree to VoiceOver and every AX client, with roles after W3C Core-AAM, values, text fields with their selection, actions (press, increment, decrement, show menu, focus, set value and selection, expand and collapse, scroll into view), notifications of changes and focus moves, and live-region announcements. Nothing runs until an assistive technology first asks.

- Linux: screen readers (Orca) through AT-SPI. While an assistive technology is enabled (`org.a11y.Status`), Wayland and X11 windows join the accessibility registry: Orca reads their tree (roles after W3C Core-AAM, names, states, text with words, sentences and lines), follows focus, value, text and caret changes, speaks live regions through AT-SPI announcements, and operates the app (activate, focus, check, expand, set text, select, scroll into view). `NO_AT_BRIDGE=1` turns it off.

- Windows: screen readers (Narrator, NVDA) through UI Automation. Elements map to UIA control types after Core-AAM; buttons, checkboxes, text fields, sliders, combo boxes, lists and tabs can be read and operated (Invoke, Toggle, Value, RangeValue, ExpandCollapse, SelectionItem, Selection, ScrollItem, Scroll and Text patterns); focus, name, value and state changes are raised as events, and live regions are announced with UIA notifications. Nothing runs until a UIA client asks for the window.

### Changed

- kit: a menu item's `Shortcut` is announced as its ARIA key shortcuts (`Shift+Cmd+P` is `Shift+Meta+P`); a `MenuRadioGroup`'s `Label` names its group; a `Popover` without `AriaLabel` is named by its trigger, as React Aria's dialogs; the `Toaster`'s region is a polite live region and each toast is named by its title, so a new toast is announced.

## [0.13.6] - 2026-11-08

Phase 18, Font system (ADR-244): no font is in an app's binary unless the app brings it, and text uses the fonts installed on the system.

### Added

- `tela.App.Fonts []tela.Font`: the fonts an app brings, before the system's. `tela.FontData(family, files...)` takes font files in memory (usually `//go:embed`), `tela.FontFS(fsys, family, patterns...)` the files of an `fs.FS` that `fs.Glob` patterns match, read when text first needs the family. `Sans()`, `Serif()`, `Mono()` and `Emoji()` make a font the family of the generic names `ui-sans-serif`/`system-ui`/`sans-serif`, `ui-serif`/`serif`, `ui-monospace`/`monospace` and `emoji`, and `Scripts("Arab", "Hebr")` makes it the fallback for those scripts (the umbrella codes stand for theirs, as in browsers: `Hans` and `Hant` for `Hani`, `Jpan` for `Hani`, `Hira` and `Kana`, `Kore` for `Hang` and `Hani`). The copies of a font share one parse whatever their roles. `Run` returns an error naming a `FontFS` pattern that matches no file before it opens the window; a script code that is not the ISO 15924 code of a script of text panics in dev mode.
- The packages `fonts/inter` (`inter.Sans`, 1.8 MB), `fonts/jetbrainsmono` (`Mono`, 0.6 MB), `fonts/sourceserif` (Source Serif 4, `Serif`, 2.1 MB), `fonts/notoemoji` (`Emoji`, the COLRv1 Noto Color Emoji, 5.0 MB), `fonts/notoarabic` (`Arabic`, 0.8 MB) and `fonts/standard` (`standard.Fonts`, all of them): import the ones an app wants and list them in `App.Fonts`. A font is read and parsed when text first needs it.
- `telatest.Fonts(fonts...)` adds fonts to a mounted app; `telatest.FromApp` takes `App.Fonts`, so `tela snapshot`, `tela inspect` and `tela mcp` render with the package's `var App` fonts.
- Bitmap color glyphs are drawn: CBDT/CBLC and sbix (PNG) strikes, scaled from the closest strike, so the system's emoji fonts work (Noto Color Emoji on most Linux distributions, Apple Color Emoji).
- A new wiki page, Fonts.

### Changed

- Apps no longer embed fonts: the binary of `examples/hello` went from 27.8 to 19.6 MB. Text uses the installed system fonts (on unless the program is a test binary), so an app looks like its OS unless it embeds a font. `tela new` apps use the system's fonts and carry a commented line showing how to embed Inter.
- The theme's font stacks are generic names: `font-sans` is `ui-sans-serif, system-ui, sans-serif`, `font-serif` is `ui-serif, serif` and `font-mono` is `ui-monospace, monospace`, and each generic name (and `emoji`) resolves per operating system to the first installed family of a list (macOS: SF, New York, SF Mono, Apple Color Emoji; Windows: Segoe UI, Cambria, Cascadia Mono or Consolas, Segoe UI Emoji; Linux: the desktop's interface and monospace fonts from the GNOME or KDE settings, which change live, then fontconfig's usual preferences). The system font index loads in the background while the first window opens. `tela inspect --layout` shows `ui-sans-serif` as the `font` of default text instead of `Inter`.
- Tests, `telatest`, `tela snapshot` and `tela mcp` use the app's fonts and then Tela's standard fonts (now with Source Serif 4 as the serif, so `font-serif` draws a serif there), never the system's. No golden changed.
- System font files of 2 MiB or more (emoji, CJK) are mapped instead of read into memory, and bitmap emoji fonts no longer keep a copy of their images: the system's Noto Color Emoji takes 1.7 MiB of heap instead of 22 (ADR-245). On KDE Plasma the desktop's KDE fonts win even when xdg-desktop-portal-gtk is installed.
- Text with no font at all (nothing installed, none embedded, as in a minimal container) draws nothing and the app prints once which package to import.

## [0.13.5] - 2026-11-08

Phase 17, Memory (ADR-240 to ADR-243): an app holds about 30 MiB less in every window, and memory comes back after heavy screens close.

### Changed

- Fonts load each glyph's outline the first time it is drawn, straight from the embedded file, instead of parsing every glyph of every font at start (ADR-243): about 26 MiB less in every app (`examples/hello`: 70 → 43 MiB of heap, 98 → 69 MiB resident), and the fonts are parsed a third faster. Pixels and text layout are unchanged.
- The rasterizer releases the buffers recent frames did not use, and everything but the cached backdrops after a second at rest (ADR-241): after the dashboard's scripted session at 4K the heap goes back to 163 MiB instead of staying at 353 MiB.
- Decoded images (`Img`, `url()` backgrounds and masks, `tela.Assets` paths, `tela.BgImage`, icons) are released once nothing shows them, and decoded again, with the same pixels, when shown again; the most recently used ones are kept up to 4 MiB (ADR-242). Before, path images stayed for the life of the process and the others in caches of up to 256 MiB.
- A window's framebuffer is allocated at its exact size, trimmed half a second after a resize settles, and dropped while the window is hidden (ADR-242).

### Fixed

- A masked layer painted before a backdrop filter in a parallel render could draw before the filter's input was ready, so its pixels could differ from a sequential render.

### Added (development)

- `go run ./internal/testutil/memreport` measures the heap, its peak, the bytes allocated and the resident memory of `examples/hello`, `kit`, `dashboard` and `notes` at x1 and x2, with the live heap split by subsystem, against a checked-in baseline (ADR-240).

## [0.13.4] - 2026-11-07

Phase 16, Platform parity (ADR-230 to ADR-239): input methods on X11 through XIM, touchpad flings and drag images on X11, portal dialogs attached to their Wayland window, WinRT toast notifications with buttons, the Windows menu bar from the keyboard and in dark mode, Windows 11's own caption buttons over an app's title bar, macOS drags from other apps and file promises, and a title bar Tela draws itself where the desktop draws none (GNOME on Wayland). Checked on the owner's Linux machine (Japanese through fcitx5-mozc on X11), a Windows 11 VM and `macos-latest` CI.

### Added

- X11: input methods through XIM (ADR-231). With `XMODIFIERS=@im=<name>` (fcitx, ibus), text fields take Japanese, Chinese and Korean from the input method as on Wayland, Windows and macOS: inline when the server offers on-the-spot preedit, else in the server's own window at the caret. Password fields bypass the input method; dead keys and Compose keep working without one; an input method that quits or restarts does not break typing.
- X11: a touchpad flings when the fingers lift, where the driver reports it (xf86-input-libinput; ADR-232). An `h.Draggable` drag shows its image under the pointer, and a drag with several allowed actions offers `XdndActionAsk` (right or middle button, or Alt).
- Wayland: file dialogs and the other portal requests are attached to their window (xdg-foreign), opening a link or file passes an activation token so the opened app may take the focus, and a window's activation uses the token a notification or tray click brings (ADR-233). A window the compositor suspends (minimized, on a hidden workspace, the screen locked) stops waiting for frames and draws its current state when it comes back.
- Windows: notifications are toasts with action buttons on Windows 10 1809 and later, withdrawn by `CloseNotification` (ADR-235); balloons remain where toasts are unavailable. Notifications the user turned off report `ErrUnsupported`.
- Windows: the native menu bar opens with Alt and F10 and Alt plus an access key (assigned from each label), and is drawn dark under the system's dark mode; its drop-down menus, context menus and tray menus are dark too, through `uxtheme.dll` (Windows 10 1903+, ADR-234, ADR-239).
- Windows 11: `TitleBarOverlay` keeps the system's minimize, maximize and close buttons, with snap layouts, over the app's title bar, restored and maximized, on the bar's own color (`TitleBar.Background`, else sampled from the bar), and reports them in `WindowState.TitleBarControls`; `kit.TitleBar` leaves room for them at the end (ADR-236). On Windows 10 it stays frameless.
- macOS: drops see drags that carry only another app's private types (as `application/x-uti.<type>`), receive file promises from Photos and Mail, and a drag without an image shows the file's or type's icon; a message box without a parent no longer blocks the app's other windows (ADR-237).
- `desktop.Tray.Template` (ADR-237): on macOS the icon is a template image the menu bar tints; ignored elsewhere.
- A fallback title bar (ADR-230, ADR-238): on a desktop whose compositor draws no title bars (GNOME on Wayland), a framed window (neither `Frameless` nor `TitleBarOverlay`) gets a plain one drawn by Tela in the app's theme, with the window's title, minimize, maximize and close in the desktop's button layout, drag, double-click maximize and the window menu; the content is laid out below it (`h-full`, `h-screen` and dialogs see the rest of the window). It hides in full screen and wherever the window has a title bar of its own (an `h.WindowDrag` area, such as `kit.TitleBar`), so apps that draw theirs are unchanged.

### Changed

- Windows: reading a drop from another process gives up after 5 s without progress instead of freezing the window (ADR-234). Tray icons are drawn again at a new display scale, and global shortcuts declared by character follow a keyboard layout change (also on macOS).
- `tela dev` speaks remote protocol 7 (ADR-237): the `tela` command and the app must be built with the same Tela version.

## [0.13.3] - 2026-11-02

Phase 15, the macOS desktop integration (ADR-224 to ADR-228): window control, the traffic lights over an app's own title bar (`TitleBarOverlay`), vibrancy backdrops, the desktop services, one app menu bar with the standard macOS menus and Cmd+Q (`desktop.AppMenuBar`, `tela.App.OnQuit`), drag and drop, global shortcuts, single instance, deep links and file types through Launch Services, the Keychain and the Trash, and `tela build`/`tela run` for a development `.app`. Verified on `macos-latest` CI, where `examples/notes` registers itself and a `tela-notes:` link reaches the running instance; a session on a real Mac is still to come.

### Added

- `tela.App.OnQuit` (ADR-224, ADR-225) runs when the user or the OS asks the app to quit: on macOS Cmd+Q, the app menu's Quit, the Dock's Quit or a logout. Without it every window gets a close request, as before, so an app whose main window has no `OnClose` quits; a close-to-tray app (its `OnClose` hides the window) sets `OnQuit: tela.Quit`, or its own "ask about unsaved changes, then quit".
- `TitleBarOverlay` on `tela.App` and `tela.Window` (ADR-224, ADR-225): the content extends under a transparent title bar while the OS keeps its window buttons over it, as Apple's apps do with the traffic lights. `WindowState.TitleBarControls` is the rectangle the buttons cover, so the app's title bar leaves it free. On macOS the window keeps the system's traffic lights over the content, centered in the height of the app's drag area along the top (`kit.TitleBar`'s), or where macOS puts them without one; the close button asks the app to close, minimize and zoom are the system's, and in full screen the buttons are in the system's auto-hiding title bar (an empty rectangle). Where the OS cannot do this (Windows, Linux) the window is frameless and the rectangle is empty. `Frameless` keeps its meaning; setting both panics with the fix.
- macOS: window control in the `cocoa` backend (ADR-224). Minimize, maximize (zoom), native full screen, always on top, minimum and maximum sizes, positions and activation work, and the window's state comes back to the app (`WindowStateEvent`), so saved windows reopen where they were. Positions on macOS are points from the top-left of the main screen, the same on screens of any scale. A frameless window keeps the system's shadow, rounded corners and resizing from its edges. A custom title bar (`HitCaption` regions) moves the window, and a double click on it does what System Settings says (zoom, minimize or nothing). Child windows stay above their parent and move with it; a modal one disables the parent's window buttons and takes the focus back whenever the parent is clicked. `Backdrop` Mica, Mica Alt and Acrylic show a translucent macOS material behind the window's transparent pixels (window background, under-window background and sidebar).
- macOS: the desktop services in the `cocoa` backend. `tela.OpenURL` and `tela.OpenPath` open links, files and folders (in the Finder); the file dialogs are the native open and save panels, shown as a sheet on the window they belong to, with the filters as file types and a "Format:" menu when there are several; `desktop.ShowMessage` is a native alert (a sheet on its window); `desktop.Tray` is a menu bar item with its menu; `desktop.SetSecret` and `desktop.Secret` keep secrets in the login keychain; `desktop.MoveToTrash` moves files to the Trash, where the Finder's Put Back restores them. Notifications need the app to run from an app bundle: under `go run` they report themselves unsupported and say to use `tela run`.
- macOS: native menus and quitting in the `cocoa` backend (ADR-224, ADR-225). Every Tela app now has the macOS menu bar: the app menu (About, Services, Hide, Hide Others, Show All, Quit) and, until the app sets its own, Edit and Window menus, so Cmd+Q, Cmd+H, Cmd+M and Cmd+C, Cmd+V, Cmd+X, Cmd+Z, Cmd+A work everywhere; Edit's items act on the focused text field exactly as their keys do. The app's own bar (`Desktop.SetAppMenu`, for `desktop.AppMenuBar`) comes after the app menu, its app items (Settings…) go in the app menu, roles are AppKit's own items, and an item's accelerator is its key equivalent (`CmdOrCtrl` is Command): the key chooses the item and does not reach the app as a key event. Context menus (`desktop.PopupMenu`, kit's native context menus) are native. A quit request (Cmd+Q, the Dock, a logout) runs `OnQuit`; an app that quits lets a logout go on and its code after `Run` still runs, an app that stays cancels it, and an unsaved-changes dialog shown from `OnQuit` keeps the request waiting until it is answered. The menu bar's own labels are English for now. A window's own `desktop.MenuBar` stays unsupported on macOS (kit draws it).
- macOS: `desktop.Register`, `Unregister`, `MakeDefault` and `RegistrationStatus` (ADR-228) register the app for its URL schemes and file types, for the current user. The app is its bundle: the scheme or the document type is added to the bundle's `Info.plist` (with a type declaration for an extension no app declares yet), every other key kept, and Launch Services reads the bundle again. Registering makes the app a candidate and never takes another app's default; `MakeDefault` sets it without a prompt. `RegisterApp` registers the bundle with Launch Services and `AppStatus` says whether Launch Services knows the app. The program must run from a bundle made by `tela build` or `tela run`, and the app's ID must be the bundle's: otherwise the calls fail with an error that says what to do (outside a bundle, `ErrUnsupported`).
- `tela build` and `tela run` (ADR-227). `tela build [-o path] [-id ID] [-name NAME] [pkg]` builds the program of a package main for the environment's `GOOS` and `GOARCH`; for darwin (also cross-compiled from Linux or Windows) it wraps it in `<Name>.app`, whose `Info.plist` names the app: the ID (`tela.App.AppID`) and the title, read from the source without running it (also an `App` that a function wraps, as in `tela.Main(desktop.SingleInstance(App))`), or `-id` and `-name`; elsewhere it writes the plain executable. `tela run [pkg] [args...]` builds that bundle in the user cache (`tela/run/<AppID>/<Name>.app`, one stable folder per app) and runs the executable inside it with the terminal, Ctrl+C and exit status; on other systems it builds and runs the program like `go run`. Rebuilding writes only what changed and keeps the keys a running app added to the `Info.plist`. Nothing is signed beyond the linker's ad-hoc signature.
- `desktop.AppMenuBar{Menus, AppItems, OnError}` (ADR-226, on ADR-224's design; Electron's `Menu.setApplicationMenu`) is the app's one menu bar, declared once anywhere in the tree. On macOS it is the menu bar of every window and `AppItems` go in the app menu after About; on Windows it is the bar of every framed window that has no `desktop.MenuBar` of its own, windows opened later included; elsewhere `OnError` gets an error wrapping `ErrUnsupported` and the app draws it. A window's own `desktop.MenuBar` is unsupported on macOS. On macOS an `Accelerator` in the app bar is the item's key equivalent: the key chooses the item and does not reach the app (`CmdOrCtrl+N` is the portable spelling).
- `desktop.Role` and `desktop.MenuItem.Role`, with Electron's names (`RoleUndo`, `RoleRedo`, `RoleCut`, `RoleCopy`, `RolePaste`, `RoleSelectAll`, `RoleMinimize`, `RoleZoom`, `RoleClose`, `RoleEditMenu`, `RoleWindowMenu`, `RoleHelpMenu`): a role item with no `Label` gets the role's English label, and an Edit or Window menu with no `Submenu` gets the standard items. Where the platform only reports a role item (Windows, drawn menus, tests), package desktop performs it: minimize, zoom and close on the window, the edit roles as the focused element's shortcut (Command on a Mac). An item's own `OnSelect` replaces the role. In menus (tray, popup, window bars) too.
- `kit.Menubar` can be the app's bar: `AppMenu`, `AppItems` and `OnAppSelect` (native: `desktop.AppMenuBar`, nothing drawn; else drawn where it is declared), and `MenubarMenu.Role` for the OS's Edit, Window and Help menus.
- `kit.TitleBar` keeps room for the OS's window buttons of a `TitleBarOverlay` window (the traffic lights on macOS: the leading slot starts after them) and draws no controls of its own; `kit.WindowControls` draws nothing there. With no rectangle, nothing changes.
- telatest: `MountApp` applies `TitleBarOverlay` and `OnQuit` (options `TitleBarOverlay()` and `OnQuit(f)` too); `App.RequestQuit()` plays Cmd+Q; `TitleBarControls(rect)` and `App.SetTitleBarControls(rect)` place the OS's buttons, and `PlatformKeys("darwin")` gives overlay windows the Mac's (`telatest.MacTitleBarControls`); `App.AppMenuBar()`, `AppMenuItems()` and `ChooseAppMenuItem(path...)` read and use the app's bar; `WithUnsupported(...)` and `Unsupported(telatest.AppMenu)` play Windows and Linux. The headless desktop has the app bar by default; kit's menus stay drawn unless `PreferNativeMenus()`.
- macOS: drag and drop in the `cocoa` backend. Files from the Finder, text, HTML, images and links from other apps drop on `OnDrop` targets (an `OnDragOver` that calls `PreventDefault` accepts, and the cursor shows copy, move or link as the app and the Option and Command keys choose); an `h.Draggable` element drags its data out to the Finder (files), other apps (text, HTML, PNG, links) and the app's own windows (custom MIME types too), with its picture under the pointer, and `OnDragEnd` reports what the target did. A drag from another app that carries only that app's private types is not seen.
- macOS: global shortcuts, single instance and opened links and files in the `cocoa` backend (ADR-224). `desktop.GlobalShortcut` works without asking for any permission and reports releases too (`OnRelease`); a combination another app holds reports `ShortcutTaken`, and the trigger reads as macOS shows it ("⌃⌥⌘K"). `desktop.SingleInstance` hands a second start's arguments to the running app, which comes to the front. In an app bundle, a link or a file the user opens with the app (from another app or the Finder) reaches `desktop.UseLaunch`, also when it starts the app (then as a launch after the start).
- `examples/notes` on macOS (phase 15's acceptance test): `TitleBarOverlay` instead of `Frameless` (the traffic lights over its title bar; frameless elsewhere), an app menu bar (Settings…, File, Edit, View, Window) declared on macOS, `OnQuit` asking about unsaved files even when it runs in the tray, registration only from its app bundle (`tela run ./examples/notes`), and `TestNotesMacOS`, which builds it with `tela build` and checks the `tela-notes:` registration, a link into the running instance and a second start that forwards its link.
- `examples/notes`: its link scheme is now `tela-notes:` (`tela-notes://new?title=...&text=...`, `tela-notes://open?id=3`) on every platform, because macOS's Notes app owns `notes:` and an app never takes another's association; the old `notes://` links still work as a command-line argument.

### Changed

- `tela dev` speaks remote protocol 6 (ADR-225): the `tela` command and the app must be built with the same Tela version, as with every protocol change.

## [0.13.2] - 2026-11-02

Phase 14, the macOS backend (ADR-220 to ADR-223): a native Cocoa backend without cgo (purego) for macOS 13 or later, with input, a display link, the clipboard, system settings and the IME; `tela dev` in a real macOS window; `looks/aqua` in two designs; one platform flavor for keyboard conventions with the public `CmdOrCtrl()` and `telatest.PlatformKeys`. Verified on `macos-latest` CI, where every example's window matches its headless render pixel for pixel; a session on a real Mac is still to come.

### Added

- `looks/aqua` (ADR-223), the macOS look of the `looks` module, in two designs that an app can mix per part: `aqua.Tahoe` (macOS 26: capsule controls, the toolbar's groups and the sidebar as floating glass, opaque content; the default) and `aqua.Sequoia` (macOS 11 to 15: rounded rectangles, a flat unified toolbar, a plain sidebar). `aqua.Look(opts ...Option)` takes them as options that set every part, and `Controls(d)`, `Chrome(d)` and `GlassContent()` set the controls, the window's frame and glass cards; options apply in order, so `aqua.Look(aqua.Sequoia, aqua.Chrome(aqua.Tahoe))` is a complete look in light and dark. `detect.For("darwin", "")` is Tahoe, and `detect.Look()` on a Mac picks Tahoe from macOS 26 and Sequoia before it (read with `sysctl`, no cgo); `detect.All()` lists both. `looks.Regions` gains `ToolbarSegments`, the class of the toolbar's view switcher, and every look defines the glass tokens (`glass`, `glass-strong`, `glass-selected`, `glass-edge`, `glass-highlight`, `glass-shadow`, `primary-glow`, `control-shadow` and `segment`), which follow the surfaces in the flat looks. The gallery shows Tahoe, Sequoia and a mix (`-look=tahoe|sequoia|mixed`).
- `CmdOrCtrl()` on `tela.KeyboardEvent`, `MouseEvent`, `PointerEvent`, `WheelEvent` and `DragEvent` (ADR-221): the platform's shortcut modifier, Command (`Meta`) on a Mac and `Ctrl` elsewhere, like React Aria's isCtrlKeyPressed. Write `e.CmdOrCtrl() && e.Key == "s"` for a save shortcut that works on every OS.
- `telatest.PlatformKeys(goos)` gives one app another platform's key conventions (text field bindings such as Command+A and Option+Left on a Mac, `CmdOrCtrl`, the aria collections' Command+click and Command+A, and what `Press("ControlOrMeta+…")` presses), so Mac behavior is tested on Linux or Windows; parallel tests with different platforms do not interfere.
- macOS: the native `cocoa` backend (ADR-222), with no cgo: a Tela app on macOS 13 or later opens a real window and draws into it, sharp on Retina displays, instead of needing `TELA_BACKEND=headless`. It adds the `github.com/ebitengine/purego` dependency, used only when building for macOS.
- macOS: the clipboard and system settings in the `cocoa` backend. Copy and paste work with other apps for text, HTML, PNG images (TIFF images from other apps are converted), files and links (as Finder copies them) and custom MIME types; dark mode (`dark:`), the accent color, reduced motion, increased contrast (`contrast-more:`) and the double-click speed follow System Settings, and a change, including an app that sets its own appearance, re-renders the app at once.
- macOS: keyboard, mouse and trackpad input in the `cocoa` backend: keys with their modifiers (Command shortcuts and their releases included), clicks of every mouse button, hover, wheel and trackpad scrolling with Tela's own fling after the fingers lift, a hidden cursor with `cursor-none`, and animations paced by the display's refresh (a display link) that stop using the CPU when idle.
- macOS: `tela dev` shows the app in a real macOS window that stays open across reloads: each new build takes over the same window with its state kept, clicks and keys reach the app, and a build error is drawn over the last good frame.
- macOS: input methods in text fields (`NSTextInputClient`): Japanese, Chinese and Korean input sources compose inline with their candidate window next to the caret, dead keys show their accent until the next key (Option+E, E types "é"), the emoji picker and the character viewer insert into the focused field, and password fields keep input methods off and turn on the system's secure keyboard input.

### Changed

- The examples' shortcuts (`dashboard`'s Ctrl+K and Ctrl+1–3, `notes`' Ctrl+N/S/O and Ctrl+Shift+D, `settings`' Ctrl+S) use `CmdOrCtrl`: Meta alone no longer triggers them on Linux and Windows, and Control alone no longer triggers them on a Mac.

### Fixed

- `tela dev` no longer misses a save made right after a build when it starts watching a new directory (the app's `tela.Assets` directory, or a dependency found by the build): the change used to be dropped and the app was not reloaded until the next save.
- `desktop.MakeDefault` and registration on Linux no longer replace a `mimeapps.list` (or desktop entry) that is a symbolic link they cannot follow, such as a dotfiles link whose file does not exist yet: links are followed as GLib follows them (the target is created if missing), and a loop or an unreadable link is an error that leaves the link alone.
- macOS: a Control+click opens context menus (`h.OnContextMenu`, `aria.UseContextMenuTrigger`, `kit.ContextMenu`), as in Chrome and Safari on a Mac, which matters on a one-button trackpad; it no longer also clicks what it is on. This follows the platform flavor, so `telatest.PlatformKeys("darwin")` tests it on any OS.
- macOS: a two-finger touch on the trackpad that does not scroll no longer breaks the next wheel scroll of a nested scroll container (it could jump to the outer one), and input methods no longer place their composition at the wrong offset after a text field rejected typed text (a field at its `MaxLength`).
- Documentation brought up to date with the code: the README (version, phase table, kit's 37 components, the X11 IME and fling, the layout fixtures), the wiki guides, `docs/llms.txt`, the examples' READMEs and the doc comments that pkg.go.dev shows (`Run` on macOS, theme variables in arbitrary values, `telatest.Render` running frames until the tree is quiet, `tela lint -fix` and `tela icons gen -deprecated` in the usage, `tela snapshot` applying `NativeMenus` and `IgnoreTextScale`). `THIRD_PARTY.md` lists every ported file.

## [0.13.1] - 2026-11-01

Phase 13, desktop integration (ADR-209 to ADR-217): drag and drop with the OS, clipboard data, opt-in native menus, global shortcuts, single instance with links and file types, system settings v2, secrets and the Trash, on Linux and Windows. No public API breaks.

### Added

- HTML drag and drop (ADR-211): `h.OnDragEnter`, `h.OnDragOver`, `h.OnDragLeave` and `h.OnDrop` on any element, with HTML's rules (a drop is accepted when `OnDragOver` calls `PreventDefault`; copy, move and link effects), and `h.Draggable(true)` to make an element a drag source that fills `DragEvent.DataTransfer` in its `OnDragStart`. Elements without `Draggable` keep the drag gesture as before. Drags go through the OS (Wayland, X11 XDND, Win32 OLE), so they work between windows and with other applications. `tela.DataTransfer` holds text, HTML, PNG images, files and custom types.
- System settings v2: `tela.UseHighContrast`, `tela.UseTextScale` and `tela.UseAccentColor`, the `contrast-more:` variant, and the system's text scale applied to `rem` like a browser's default font size (on by default; `App.IgnoreTextScale` turns it off). Linux now reads dark mode, the accent color, contrast, reduced motion and the text scale from the settings portal (before, Linux had no dark-mode detection); Windows adds the accent color, high contrast and the text scale.
- Package `desktop` (ADR-213): the clipboard with every data type (`ReadClipboard`, `WriteClipboard`); `GlobalShortcut`, a component that registers a system-wide shortcut while mounted and reports whether it was bound, taken or refused (Win32 `RegisterHotKey`, the Linux GlobalShortcuts portal, X11 key grabs); `SingleInstance` and `UseLaunch`, so a second start hands its arguments, links and files to the running app and exits; `Register`, `Unregister`, `MakeDefault` and `RegistrationStatus` for `notes:`-style links and file types, per user, never taking another app's association or the default without an explicit call, and usable without a running app; `SetSecret`, `Secret` and `DeleteSecret` in the Windows Credential Manager or the Linux Secret Service; `PopupMenu` and `MenuBar` for native menus where the OS has them.
- Native menus, opt-in (ADR-215): `kit.ContextMenu` and `kit.Menubar` take `Native: kit.NativeOn` (or `kit.NativeOff`) to use the OS menu where one exists, Windows for now (a menu bar only on framed windows), and fall back to the drawn menu elsewhere; `tela.App.NativeMenus` turns them on for the whole app, and a menu's own setting wins. Drawn stays the default. New `aria.ContextMenuTriggerOptions.Intercept` lets a trigger take an opening over.
- `telatest`: drag and drop (`DragTo`, `DragToWindow`, `DropFiles`, `DropData`, `HoverDrag`, `StartDrag`, `DropOn`, `DropOutside`), system settings while mounted (`SetDarkMode`, `SetHighContrast`, `SetTextScale`, `SetAccentColor`…), and the new `desktop` services (`PressShortcut`, `InjectLaunch`, `StoredSecret`, `ChoosePopupItem`, `ChooseMenuItem`, `Registrations`…).
- `tela dev` carries all of it (remote protocol 5, ADR-214): drags, native menus, clipboard data, global shortcuts (kept across reloads, so the portal does not ask again on every save), single instance, secrets and registrations.
- `examples/notes` is phase 13's acceptance test: drop `.txt` and `.md` files (or text) on the window to import them, a Quick note global shortcut (`Ctrl+Alt+N`, with its status in Preferences), one instance that `notes://new?…` and `notes://open?id=N` links and file arguments reach, a "Links and files" section that registers `notes:` and `.md` without taking defaults, and locked notes, encrypted in the library with a key in the system keyring.
- Documentation: a Desktop integration guide in the wiki and on the website, `docs/llms.txt` sections for phase 13 and its tests, `contrast-more:` and the text scale in the Styling guide, the drag and settings helpers in the Testing guide.
- The X11 backend gets a clipboard (it had none, not even for text).
- `desktop.MoveToTrash(path)` (ADR-216) moves a file or folder to the trash (the Recycle Bin on Windows) from any goroutine, and never deletes it: a file the trash cannot take stays where it is, with `ErrUnsupported`. Linux uses the Trash portal, else the freedesktop home trash; Windows uses `IFileOperation` and refuses what it would delete for good. `telatest`: `Trashed`, `FailTrash` and `Unsupported(telatest.Trash)`.
- `examples/notes`: locking a note that was opened from a file detaches it from the file (its path leaves `notes.json`) and asks whether to move the file, which still holds the text, to the Trash. A locked note cannot be saved to a file and never counts as unsaved.
- `desktop.RegisterApp`, `UnregisterApp` and `AppStatus` (ADR-217): on Linux, the app's own desktop entry (`~/.local/share/applications/<AppID>.desktop`, per user, no launcher in the menus, no file types), which the global shortcuts portal needs; written only when called, looking first, and never over another program's entry without `desktop.Replace()`. Scheme and file-type registration share the file. Windows needs no entry and writes nothing. `desktop.ErrAppNotRegistered`. `telatest`: `RegisteredApps`, `HoldAppEntry`, `FailShortcuts` and `SetShortcutUnassigned`.
- `examples/notes` calls `desktop.RegisterApp` at start (in the built program), and Preferences, Shortcuts shows a shortcut the desktop registered without keys (Hyprland) with the `hyprland.conf` line that binds it.

### Fixed

- On Windows, a `desktop` store's write no longer fails when another program holds its file for a moment (an antivirus scanning the file just written, the search indexer): replacing the file is retried for up to a second, as `cmd/go` does.
- On Windows, `tela dev` and `tela mcp` remove the executables of old builds even when Windows still holds one for a moment after its process exited; they retry for up to two seconds, off the supervisor's loop.
- An element whose classes changed its layout (`flex` to `flex flex-col`, `grid` to `flex`) in the same update in which a child's own structure or an image child's size changed (a first child wrapped in a new `Div`, for example) kept its old layout until something else touched it; it is now laid out as a fresh mount would be.

## [0.13.0] - 2026-10-31

The work between phases 12 and 13, released so the new website's docs and landing page match the version `go install …@latest` gets (ADR-208): ADR-205's `tela.Node` change, the `looks` module, the consolidated examples, `kit` and engine improvements, and the website.

### Added

- The project website, `https://telaui.dev` (ADR-206): the guides and API reference of the wiki with search, light and dark themes and checked links, built from `docs/wiki` and published with each release's docs. The Theming guide's link to theme variables pointed to an anchor that does not exist; it now points to their section of the Styling guide.
- The website's landing page and Tela's logo (ADR-207). The page has the install steps with OS tabs and a prompt for coding agents (`https://telaui.dev/llms.txt` now serves `docs/llms.txt`), the counter beside its window, tabs of Go code and the PNG `tela snapshot` makes of it, one terminal for the snapshot, inspect, lint, mcp and dev loop, a carousel of the example apps, and a comparison with Fyne, Gio, Wails and Electron where every cell cites its source. The logo is a manila punched card with a hollow T, one file for light and dark pages, redrawn for small sizes; the README shows it, and the site uses it for the header, favicons and the social card. The README's status and its `tela inspect` example were out of date and are fixed.

- `tela lint -fix` applies the fixes a rule can make safely, and two rules come with ADR-205: `fragment` (a `Fragment(x)` around a single node or component that is no longer needed; it is reported only where removing it type-checks and keeps the child's state) and `prop-node` (a `Prop` where a `tela.Node` is expected, which compiles but panics when used). The repo's 92 redundant wraps were removed with it; `docs/llms.txt` and the wiki no longer tell you to wrap a component.

- The theme's own CSS variables work inside arbitrary values (ADR-202): `var(--color-<token>)` (custom tokens too, with the light or dark value of the element's scheme), `var(--radius)`, `var(--radius-*)`, `var(--spacing)` and `var(--x, fallback)`, in colors, opacity modifiers, gradients and their stops, shadows, lengths and `calc()`; also the `bg-(--color-primary)` shorthand. An unknown token is an error with a suggestion; an app's own variables are still not supported. The dashboard's goal ring now follows the theme's primary color.
- `divide-x`, `divide-y`, their widths and `-reverse`, `divide-<color>` and `divide-solid|dashed|dotted|double|none`, following Tailwind v4 (every child but the last; a child's own border classes win) and compared with Chromium (ADR-203).

- `kit.Plain`, a variant of `kit.TitleBar` and `kit.WindowControls` for apps with a design of their own (ADR-201): the platform's arrangement and glyphs (Windows, GNOME or macOS, chosen by the new `Look` field or the running platform) with no background, border or outline of its own, and quiet ghost window buttons in the theme's colors. `examples/notes` uses it, and the kit gallery's Title Bar page shows it.

- `kit.Table.RowHeight` makes a table virtual (ADR-199): only the rows in view are built, on an `h.VirtualList`, so a frame that rebuilds the page costs the same with 240 or 100,000 rows (about 1.2 ms where a plain 2000-row table took 59 ms). New `h.AriaRowCount` and `h.AriaRowIndex`, which a virtual table sets. Sticky columns are not supported in a virtual table yet.

- `aria.ComboboxOptions.AutoFocusFirst`: while the list is open and nothing is highlighted, the first item is, on opening and after each change of the text, so Enter chooses it (ADR-198).

- New module `github.com/kuronosu/tela/looks` (ADR-197), separate from the core like `icons/lucide`: the looks of Windows 11, GNOME and KDE for `kit`, each in light and dark. A `looks.Look` is a `tela.Theme`, a `kit.Style` and the classes of the window regions kit has no component for (`Regions`: toolbar, location bar, sidebar, content, details pane, status bar); `win11.Look()`, `adwaita.Look()` and `breeze.Look()` return one, and `detect.Look()` picks the running desktop's. `looks/gallery` shows kit's controls in each look and switches between them at run time.

### Changed

- **Breaking:** `tela.Node` is now the interface every renderable implements, `interface{ Build() Node }` (ADR-205). A struct component such as `kit.Button{...}` is a `Node`, so a function, a `Build` method or a `Map`/`Cell` callback declared to return `tela.Node` can return it directly, with no `Fragment(...)` around it; `return If(...)` works too. Elements stay functions (`Div(...)`) and components stay structs; `tela.Component` stays as an alias of `Node` and `tela.Child` is unchanged. Core's benchmarks and allocations are unchanged. Migration:

  | Before | After |
  |---|---|
  | `tela.Node{}` | `nil` |
  | `n.IsZero()` | `n == nil` |
  | `n.Key()`, `n.String()` | removed |
  | `case tela.Node:` in a type switch | also matches struct components and props |
  | a generic union `~func(int) tela.Node \| ~func(int) tela.Component` | `func(int) tela.Node` (`h.Range` and `h.VirtualList` take it) |
  | `n1 == n2` on two Nodes | may panic for non-comparable struct components (`== nil` is safe) |
  | a Prop returned where a Node is expected | compiles, and panics when used, naming the place |
  | `func Preview() tela.Component` | `func Preview() tela.Node` (one form, always mounted as a component; a nil result renders nothing) |

- A paint records again only the layers whose content changed (ADR-204), with the same pixels: a hover, a color transition step or a one-element change used to record every layer of the window (359 on the kit gallery's Calendar page, now about 2; a hover there goes from 1.45 to 0.41 ms). The dashboard's board cards fade their colors again. `TELA_CHECK_LAYERS=1` compares every partial frame with a full recording.

- The CPU renderer keeps the output of a backdrop filter between frames while what lies behind it is unchanged (ADR-200), with the same pixels as before: the dashboard's median frame at 1080p goes from 15 to 4.7 ms at scale 1 and from 48 to 11 ms at scale 2.

- `examples/settings` is now **Darkroom**'s Preferences window (ADR-195): five sections (General, Develop, Export, Safelight, Account), a form that is one comparable value, validation that disables Save, Discard and sign-out confirmations, a theme and an accent applied on Save, and a dark-room voice, built by an LLM (Claude Sonnet) from `docs/llms.txt` and the prompt in its README in two render iterations. It replaces the previous `examples/settings` and `examples/llms/settings`; `docs/images/settings.png` shows it, with new goldens and behavior tests.
- `docs/llms.txt`, from what that experiment found: `kit.AlertDialog`'s `Title`, `Description`, `Cancel` and `Action` are components, not strings; an explicit `App.ColorScheme` overrides `tela snapshot --dark`; the fields of `tela.Preview` and a recipe for state previews (and `--size` to see below the fold); `kit.UseToast()` returns a `kit.ToastHandle`; a `kit.Field` wraps one form control, not an overlay trigger, with the "label + button" row pattern; where to find Lucide names. Its compact settings screen (section 7) is now a block of its own, which `internal/gen/llmsdoc` type-checks, lints and mounts headlessly in dev mode instead of comparing it to an example.

- The Counter example has a new design: a stone card with a "COUNT" label, a large number and a full-width dark button, in place of the gradient card. It applies to `examples/counter`, the README and wiki snippets and image, and the app that `tela new` creates (480x400 window, test clicks Increment).

- The `login` example has a Create account mode (ADR-195): the "Sign up" link turns the card into the longer form (display name, e-mail, password with a strength meter, a bio bound with `BindValue` and a `TextFn` counter, terms, a submit button that waits until the form is valid, the same fake request and a confirmation), and "Sign in" turns it back. Its goldens, aria snapshots and tests include the new mode; the show/hide password button has a tooltip and the "Sign up" link an accessible name that says where it goes.

- The `kit` gallery opens on an Introduction page (ADR-195): a hero in the spirit of `examples/hello` (an eyebrow, a very large headline, a tagline, a hairline), what kit is, a small program that uses it and a button for each component that opens its page; it is made of theme tokens only, so it follows Dark mode and the Rounded style. The header has a Language menu (English, Spanish, Arabic, each named in its own language) that sets the locale and the direction of the whole window, so every page can be seen in Spanish and in right-to-left Arabic; the date pickers, calendars, dialogs and alert dialog take their English defaults from the catalogs. A new Internationalization page shows catalog messages with arguments, plurals (a counter walks Arabic's six categories), an ordinal, nested keys and the formatter's numbers, percentages, dates, times and lists. New previews: `Introduction`, `Internationalization` (and `Internationalization es`, `Internationalization ar`) and `Gallery Arabic`.

- `examples/notes` has a new design, "Graphite" (ADR-195): a rail with All notes, Pinned and Unsaved, the folders and the `#tags` found in the text; a column of note cards with search; and the editor with a title of its own, a folder menu, a pin, a delete button and a status line. With no note open the pane is a grid of cards. The light scheme is hello's stone and the dark one graphite, with an amber accent the Preferences window can change, along with the theme, line spacing and list previews. Notes gain a title, a folder, a pin and a modification date (libraries of the previous version still load); a deleted note can be restored from the toast. The app icon is the new amber badge, and `docs/images/notes.png` shows the new screen.


- `examples/dashboard` has a new design, "Frosted graphite" (ADR-195): three fixed glass panes (rail, main, Pulse) over a calm dune wallpaper with one amber accent, in light and dark. Overview (a headline figure with metric tabs and 48 spring bars, four figures with sparklines, latest orders, a goal ring, traffic and activity), Tasks (a sortable table and a board with drag and drop, card menus, edit, new and delete dialogs, a sprint strip) and Orders; a command palette (Ctrl+K) and tooltips built directly on `aria` hooks; Ctrl+1/2/3 switch views. `docs/images/dashboard.png`, `dashboard-dark.png` and `dashboard-palette.png` show it. Its frame times are in the README; it is heavier on the CPU than the page it replaces, as phase 16 expects.

### Fixed

- A deferred layer (a modal, a card with an opacity or transform animation) inside a scrolling area was recorded again on every scroll frame that moved the area's interest rect, although its list does not depend on it: a frame of such a scroll goes from 0.43 to 0.10 ms.
- Animation steps are now seen by the partial recording of ADR-204 through the same comparison of every `ComputedStyle` field as any restyle, instead of a special case inside the animation code, so an animated property that painting reads can no longer leave a stale layer.

- The backdrop-filter cache (ADR-200) could draw a wrong blur under stacked glass panes: a filter planned as a cache hit missed at draw time (its region was grown by an earlier filter, or it was under a mirroring transform) and was recomputed from pixels the incremental render had not made exact. The planning now checks the whole cache key and binds what is drawn from the cache.

- A virtual `kit.Table` (`RowHeight`) keeps its header and footer aligned with the rows when the scrollbars take layout width (classic scrollbars, the Windows look), left to right and right to left.
- `tela lint`'s `fragment` rule no longer offers to remove a `Fragment` around a single node when the same variable or the function's other returns hold a `Fragment` of several (declared with `var`, `:=`, assigned in a branch or a switch, or captured by a closure): the child would remount when the form changes.

- Dev mode no longer reports a duplicate `ID` when an element takes the place of another with the same ID in one build, such as an input replacing another inside a `kit.Field` that keeps its ID (Darkroom's General and Account sections): the check runs after the build, when the old element is gone. Two elements mounted with one ID are still reported.

- Damage under a `backdrop-blur` (or any backdrop filter that reaches beyond a pixel) no longer grows by the filter's reach when what changed is painted above the filter, such as content moving inside a glass pane: only a change behind the filter does (ADR-200). A card dragged in the dashboard's glass pane repaints its own area instead of about twice it.

- `aria.UseCombobox` consumes Escape only when it used it (closed a list that can close, or reverted the text): a command palette whose list is always open now closes from the keyboard, and reverting the text of a selected item no longer also closes the enclosing dialog (ADR-198).

- `desktop.UseSetting`, `UseStoreSetting` and `Store.Get`: the elements of a stored slice, array or map start from zero values instead of inheriting, by position, the fields of the default's elements that the stored JSON leaves out (an `omitempty` field cleared by the user came back after a resync). Struct fields missing from the stored value still keep the default's.
- `tela lint` learns the custom theme tokens set through a helper function (`set("gold", ...)` where the helper assigns `t.Colors[name]`), not only constant-key assignments.

### Removed

- The examples `board` and `aria` are gone, merged into `dashboard` (ADR-195): the board's model, drag, menu, dialog, bench and damage tests are the dashboard's board tests, and the behaviors `examples/aria` tested that the `aria` package did not are tests of a hooks-only screen in `aria/screen_test.go` (tab order, menus, combobox, dialog, pointer, damage, idle, aria snapshot). `docs/images/aria*.png` and `board.png` are removed.

- The example `native` is gone (ADR-195, ADR-197): its three looks are the `looks` module, and its file explorer is replaced by the `looks` gallery; `docs/images/native-*.png` are replaced by `docs/images/looks-*.png`.

- The example `llms/settings` is gone (ADR-195): its screen stays in `docs/llms.txt` (section 7) and its Lucide subset is `examples/settings/lucide`.
- The example `i18n` is gone, merged into the `kit` gallery (ADR-195): its catalogs are `examples/kit/locales`, and its framework checks (plural categories, formatter output, kit's defaults from the catalog, the calendar of each language, the window mirrored for Arabic, aria snapshots) are tests of the gallery's Internationalization page and Language menu.
- The example `signup` is gone, merged into `login` (ADR-195): its fields and tests live in the Create account mode of `examples/login`, and `docs/images/signup.png` is replaced by `docs/images/login-create-account.png`.
- The examples `pricing`, `motion`, `layout`, `virtuallist` and `shapes` are gone (ADR-195). What they tested is covered by package tests and godoc Examples: frames of `Presence`, drags with `Animate`/`Velocity` and `UsePresenceTween` in `anim_frames_test.go`, layout animation frames in `h/layoutanim_frames_test.go`, `VirtualList` goldens in `h/virtual_golden_test.go` with new `ExampleOverscan` and `ExampleElementRef_ScrollToIndex`, and the full-page paint golden in `h/paint_showcase_test.go`.
- The example `text` is gone (ADR-195). Its content (mixed-script paragraphs, a monospace block with ligatures, truncate and line-clamp, decorations, an inline box, tabular figures) is a golden of `h` elements, and its selection of Arabic text a golden of a right-to-left field in `internal/core`; the rest is covered by the tests of `internal/text` and `internal/core`.
- `tela snapshot` no longer renders a package that only defines `func Scene(w, h float32) *gfx.DisplayList`, the phase-1 contract, which could not be used outside the Tela module (ADR-196). Define `Preview`, `Previews` or `App`.

## [0.12.6] - 2026-10-29

### Fixed

- `tela dev`: the app's clock (`Now`) no longer reads earlier than the time of the frame it is drawing when the machine is loaded (the remote backend estimates the host's clock; it now never goes behind the last frame time).
- Tests: `TestDebounce` of the `tela dev` watcher retries a burst of writes that a loaded runner stalled (it then rightly saw two bursts).

## [0.12.5] - 2026-10-29

CI is green again (tests and module metadata only; no change to the framework's behavior).

### Fixed

- `icons/lucide`: its `go.mod` and `go.sum` list `godbus/dbus`, which the Linux desktop services of Tela need, so the module builds and tests on its own.
- Tests: the ones that need what Wine lacks (AF_UNIX sockets for `tela dev`'s remote host, Windows file-change and rename semantics) are skipped only under Wine; the Win32 pointer tests move the real cursor off their window; the Win32 tests that depend on the shell (common dialogs, notifications, the file-dialog modal check) run with `TELA_LIVE=1`, like the Linux live tests; the `examples/player` and remote-clock tests no longer depend on timing; the Wayland live tests find their window on sway as well as Hyprland and skip when no seat has a keyboard; `TestDPIChanged` works at any monitor scale.

## [0.12.4] - 2026-10-29

Fixes from the codebase review of 2026-10-29 (`docs/reviews/2026-10-29.md`).

### Added

- `telatest.App.LayoutJSONOf(locator)`: the layout tree of one element's box, in the shape of `LayoutJSON` (ADR-194). The `inspect` tool of `tela mcp` uses it for a `ref`, so it returns that element's box instead of guessing it from its bounds and role (with a module on an older Tela it returns the whole layout and the element's bounds).

### Changed

- `rgb()`, `rgba()`, `hsl()`, `hsla()`, `oklch()` and `oklab()` in arbitrary values follow CSS Color 4, the grammar SVG colors already used (ADR-193): mixed separators (`rgb(1, 2 3)`, `rgb(1,2,3 / 0.5)`), empty arguments, mixed numbers and percentages in the comma form, `hsl()` with commas but without `%`, and commas or an alpha without `/` in `oklch()`/`oklab()` are errors, and `none` is accepted in the space form. An HSL channel that falls exactly between two 8-bit levels can round one level differently.
- The hint for a package without `Previews` ends the same way in `tela snapshot`, `tela dev` and `tela mcp`.
- One double-click rule for content and for title bars the app draws (ADR-192): on Windows, `OnDoubleClick` and word selection follow the user's double-click time (they were fixed at 500 ms); on Wayland and X11 a double click on a drawn title bar uses the content's 500 ms and 5 px (it used 400 ms and 4 px). Touch keeps 300 ms and 100 px.
- X11: the resize band of a frameless window is 5 logical pixels, as on Wayland (it was 6), and in a window too small for both bands the nearer side wins.

### Fixed

- Handlers of `InputEvent`, `ChangeEvent`, `CompositionEvent`, `SelectEvent` and `SubmitEvent` passed to `UseCallback` no longer allocate on each call (they went through reflection; ADR-190).
- `tela mcp` gives a ref to an element whose name is `/`.
- `tela mcp` actions on a ref to a text that has since been removed report the node as gone instead of acting on its parent element.
- Errors of color functions in arbitrary values name the real problem (an empty or doubled alpha after `/`, commas in `oklch()`) and their examples parse (the `hsl()` one did not).
- An unknown `aria-[sort=…]:` or `aria-[current=…]:` variant no longer suggests itself or another token that has no variant (`aria-[current=false]:` was offered as the fix for `aria-[current=false]:`).

## [0.12.3] - 2026-10-28

### Changed

- Less work per frame (a codebase-wide cleanup, no change in output): a `Textarea` repaints only its visible lines, and with a selection only the selected lines (selecting all of a long text repainted the whole text once per selected line); events no handler listens to (pointer moves with no handlers) no longer allocate; animated boxes no longer copy their whole style every frame; the damage tracker and the window framebuffer (during an interactive resize) reuse their buffers; shaping reuses the paragraph's grapheme segmentation and font candidates for each run.

### Fixed

- The wiki's API reference prints the constants of a type aliased from an internal package with their comments in place (`tela.ColorScheme`, `tela.MouseButton`, `telatest.HitKind` and others had blank lines inside their comments, or the comments after the closing parenthesis, some of them dropped).

## [0.12.2] - 2026-10-28

### Fixed

- A `Textarea` whose lines do not wrap (`text-nowrap`, `whitespace-pre`, `whitespace-nowrap`) scrolls horizontally, as in Chromium: long lines were clipped at the right edge (`examples/notes` with word wrap off). The wheel with Shift and a touchpad scroll it, the caret stays in view while typing and with Home, End and the arrow keys, a selection drag autoscrolls sideways, and with classic scrollbars the horizontal bar takes its gutter. Clicking into a scrolled, unfocused field no longer scrolls it back to the start first (ADR-188).

## [0.12.1] - 2026-10-28

### Added

- `telatest.NoServerDecorations()` (and `headless.Options.NoServerDecorations`): a fake window manager that draws no title bars, as GNOME on Wayland, so every window reports `ClientSideDecorations` and a test can check the app draws its own.

### Fixed

- Linux tray menus: a changed label or check state of an item that keeps its place (a "Show"/"Hide" item following the window) is sent with `ItemsPropertiesUpdated`; GNOME's AppIndicator extension rereads only the menu's structure on `LayoutUpdated` and kept showing the old label.
- `examples/notes`: the Preferences window had no title bar on GNOME (Wayland draws none there), so it could not be moved or closed; it now draws a `kit.TitleBar` when the window reports `ClientSideDecorations`.

## [0.12.0] - 2026-10-28

### Added

- Multiple windows, declaratively: `tela.Window{Title, Width, Height, MinWidth…, FixedSize, Frameless, AlwaysOnTop, Modal, Hidden, Icon, RootClass, Placement, OnClose, Root}` opens a window while it is mounted and closes it when it unmounts; state and context flow into it, and a `Modal` window blocks input to the window it is declared in (ADR-178, ADR-180).
- `tela.UseWindow()` returns a handle of the component's window: `Minimize`, `Maximize`, `Unmaximize`, `ToggleMaximize`, `SetFullscreen`, `Hide`, `Show`, `Focus`, `Close`, a reactive `State()` and `Placement()` (ADR-180).
- `tela.App` gains `MinWidth`, `MinHeight`, `MaxWidth`, `MaxHeight`, `FixedSize`, `Frameless`, `AlwaysOnTop`, `Hidden`, `Icon`, `Placement` and `OnClose func(tela.WindowHandle)`; with `OnClose: func(w tela.WindowHandle) { w.Hide() }` the app keeps running with its window hidden (close to tray). A nil `OnClose` quits as before (ADR-180).
- `tela.Quit()`, safe from any goroutine; `tela.OpenURL(url)` and `tela.OpenPath(path)` open the default application (ADR-180).
- Custom title bars: `h.WindowDrag()` marks a drag region (its buttons, fields and other interactive descendants stay clickable; `h.WindowNoDrag()` opts out), `h.WindowControl(tela.WindowMinimize|WindowMaximize|WindowClose)` makes a button a window control. Windows answers them natively (Aero Snap, snap layouts on the maximize button), Wayland and X11 through the compositor and window manager (ADR-180).
- New package `desktop` (ADR-181): file and folder dialogs (`OpenFile`, `OpenFiles`, `SaveFile`, `PickFolder` with `Filter`, `Directory`, `DefaultName` options), `ShowMessage` (native on Windows, drawn by Tela as a modal window elsewhere), `Notify` with actions, a declarative `desktop.Tray` with a menu, per-app `ConfigDir`/`DataDir`/`CacheDir`/`StateDir`/`LogDir`, a JSON settings store with change events and the `UseSetting` hook (a field missing from the stored value keeps the default's, slices and maps are replaced whole; a file that cannot be read is never overwritten), and persisted window placement (`PersistApp`, `SavedWindow`, `UseSavePlacement`).
- `kit.TitleBar` and `kit.WindowControls` for frameless windows, in Windows, GNOME and macOS looks chosen by the OS or a `Variant`; a gallery page (ADR-182).
- `telatest` for several windows: `app.Windows()`, `Window(title)`, `WaitForWindow(f)` return an `*App` scoped to one window; window state and actions through the fake window manager, modality, title-bar hit regions, `ExpectQuit`; `MountApp` applies the window fields of `tela.App`; `telatest/desktop.go` answers dialogs and message boxes and clicks notifications and tray items (ADR-183, ADR-181).
- `tela dev` forwards the new window options, window control, hit regions and desktop services; child windows, trays and notifications survive reloads, and a window closed to the tray stays hidden (ADR-184).
- Platform: window state (minimize, maximize, fullscreen, hide, always on top, size limits, position where the OS allows it, icons), frameless windows with resize edges, parent and modal windows, Wayland session restore (`xdg-session-management-v1`), and the desktop services the coming `desktop` package uses: file and folder dialogs (XDG portal, common item dialogs), message boxes (Windows), notifications (`org.freedesktop.Notifications`, balloons) and tray icons (StatusNotifierItem, `Shell_NotifyIconW`) (ADR-179). New dependency: `github.com/godbus/dbus/v5` (Linux).
- Example `notes`: a frameless plain-text notes app with `kit.TitleBar`, native open and save dialogs, a Preferences window whose settings both windows share (`desktop.UseSetting`), a library in the app's data folder, a tray icon whose menu follows the window, close to tray, a question before quitting with unsaved files, a notification on save and remembered placement of both windows; goldens and behavior tests.
- `docs/llms.txt` has a section on windows and desktop services, and the wiki a "Windows and Desktop" guide and a `desktop` API page; Go blocks of one package in `llms.txt` compile together.
- A hidden or minimized window keeps building and running its effects without drawing, so a tray menu follows a hidden main window, a window opens from it, and timers and setting changes reach it; it is drawn in full when shown (ADR-185).

- Windows 11 look: `TitleBar: tela.TitleBarColors{Background, Text, Border}` colors a window's native title bar and border, and `Backdrop: tela.BackdropMica` (`BackdropMicaAlt`, `BackdropAcrylic`, `BackdropNone`) gives it the system material, in the title bar and, where the root has no background, in the content. Fields of `tela.App` and `tela.Window`, applied on change; Windows 10, Linux and headless ignore them; `tela dev` forwards them (protocol 4); `telatest` records them (`app.TitleBar()`, `app.Backdrop()`). A frameless window with a backdrop may lose the snap layouts flyout. The `notes` Preferences window is Mica (ADR-187).

### Fixed

- Wayland: a hidden window shown again stayed invisible on Hyprland, which sends no configure to a surface that commits again after an unmap; hiding now destroys the window's `xdg_toplevel` and `xdg_surface` and showing creates them again, as GTK and Qt do (ADR-186).
- Windows: a window is painted again from its retained copy when the OS discards what it shows (a reconnected remote session, a display, theme or DWM composition change, resume from sleep); the retained copy is always opaque.

## [0.11.0] - 2026-10-26

### Changed

- A `func Preview() tela.Node` is mounted as `tela.C(Preview)` by `tela snapshot`, `inspect`, `dev` and `mcp`, so it may use hooks itself; it panicked with "called outside a component build". `tela inspect --layout` names `Preview` as the root's component (ADR-175).

### Added

- `telatest`: `Right()` makes `Click` and `ClickAt` use the secondary button (Playwright's `button: 'right'`), so a test opens a context menu with `app.Click(area, telatest.Right())`; `app.Tap(loc)` taps with a touch pointer and `app.LongPress(loc)` holds a touch past the long-press threshold (ADR-170).
- `tela.UseTimeout(d, f, deps...)` calls `f` once, `d` after the frame that shows the component, on the frame clock like `UseInterval`; a new `d` or new dependencies restart it, `d <= 0` cancels it (ADR-171).
- `h.AriaCurrent("page")` (`page`, `step`, `location`, `date`, `time`, `true`), the `aria-[current=page]:` class variant and `telatest.Current(value)` for `Expect`; like Playwright, the aria snapshot does not print it (ADR-171).
- `tela lint [-json] [-tests] [packages]` checks the source statically with the runtime's own rules and suggestions: unknown classes (in `h.Class`, kit `Class` fields and recipes, `ClassAliases`), roles, ARIA tokens, `h.Dir`/`h.Type`/`tela.Ease` values, duplicate keys, lists without keys, and React's rules of hooks; only constant values are checked; `//tela:ignore <rule>` silences a line. CI runs it on the repository (ADR-173).
- `tela mcp [pkg]`: a Model Context Protocol server over stdio that keeps the package's screen mounted headlessly for an agent, with Playwright MCP's model: `snapshot` (the aria snapshot with `[ref=eN]`), `screenshot`, `click`, `hover`, `fill`, `press`, `scroll`, `drag`, `resize`, `open` (another preview, dark, size), `wait`, `inspect` and `lint`. It rebuilds when the sources change, returns compile errors and lint findings without losing the last good screen, and reports the dev-mode warnings of each call (ADR-174). For Claude Code: `claude mcp add tela -- tela mcp ./myapp`.
- `telatest`: `App.Resize`, `App.SetScale` and `App.LayoutJSON` (ADR-174).
- `docs/llms.txt`: the whole guide an LLM needs to build Tela screens (the API, the kit catalog, the `tela mcp` loop, a complete settings screen in `examples/llms/settings`, the common mistakes); a test type-checks and lints every Go block in it against the current API and checks that it names every kit component (ADR-177).
- Examples `login` (a sign-in card with a brand panel, `kit.Field` errors, a show/hide password button, a spinner on `tela.UseTimeout`, a wrong-password alert) and `chat` (a messaging app: one reducer, `h.AriaCurrent` on the open conversation, a virtualized message list scrolled to its end, a growing composer, a typing indicator and a reply on `tela.UseTimeout`), with goldens and behavior tests.
- `kit.Field.ControlAfter` puts the control after the label and description of a horizontal field, as shadcn's switch rows do, and `kit.FieldSet` (shadcn's FieldSet and FieldLegend, with `LegendVariant` `legend` or `label`) groups fields under a legend that names a `RadioGroup` or `ToggleGroup` inside it. `examples/settings` uses both; its pixels are unchanged (ADR-172).

### Fixed

- `//tela:ignore class, key -- reason` ignores both rules, and `//tela:ignore -- reason` every rule; only the first name was read (ADR-173).
- The `tela mcp` `click` tool with `x`, `y` and `double` double-clicks (it sent two single clicks); a ref no longer passes to an element that has a ref's old name; after a rebuild that fails to mount, the session keeps describing the running screen; JSON-RPC batches are answered in one array, and the server answers pings while tool calls wait.
- A dev-mode hook-order message that swapped `tela.UseTimeout` and an aria component's timer named the same hook twice; outside dev mode such a swap with `UseInterval` reused the slot silently.
- `tela mcp` says in which color scheme the screen was mounted (`mounted in dark mode`); it said "light" after the app had switched itself to dark with `tela.SetColorScheme`.
- `tela mcp /path/to/app` works when the client starts the server outside the app's module (Claude Desktop starts it in `/`): a directory argument becomes the working directory.
- `ScrollTo`, `ScrollBy` and `ScrollIntoView` called from a `tela.UseLayoutEffect` use the layout of that frame: on mount they did nothing, and after content grew they landed one item short, though `UseLayoutEffect`'s documentation shows that idiom. `ScrollIntoView` and a focus reveal reach an element mounted in the same frame (ADR-176).
- An `absolute` or `fixed` inline element (`Span`, `A`, `Strong`…) is blockified as in CSS (an `inline-flex` one is a flex box), so a status dot made with a `Span` draws like a `Div`; an absolutely positioned box among a paragraph's text no longer splits its lines, and `truncate` and `line-clamp` keep working beside it (ADR-176).

## [0.10.4] - 2026-10-26

### Changed

- A press outside an overlay with the secondary or middle mouse button dismisses it, as a primary press does (Radix's rule): a right click outside an open context menu, dropdown, popover or dismissable dialog closes it. A right click elsewhere on a context-menu area closes the menu and opens it again at the new point, focused; one in another area opens that area's menu (ADR-167).
- `text-balance` and `text-pretty` break lines as Chromium does; they wrapped greedily. The signup quote and wrapped tooltips are balanced (ADR-166).

### Fixed

- A slow mouse click (held over 500 ms) on a button inside a `kit.ContextMenu` area no longer loses its click to the area's long press, which is now recognized for touch and pens only; a pen held down opens the menu. A touch or pen long press elsewhere on the area of an open context menu moves the menu there instead of closing it at the release (ADR-167).
- When the focused element inside a modal dialog (any scope that contains the focus) unmounts, the focus moves to the dialog instead of nowhere, so Escape, shortcuts and Tab keep working; a layout effect that focuses another element in the same commit (a wizard's next step) is the only focus change, and the dialog's focus does not scroll (ADR-168).
- `invalid:` no longer matches read-only or disabled fields, as CSS `:invalid`; disabling an ancestor updates `read-only:` and `invalid:` of the fields inside it (ADR-169).
- Wayland: scroll events from seat version 8 compositors carry the time of their frame. It was 0 or stale, so the wheel latch never timed out: a wheel notch long after the last one kept scrolling the container scrolled before until the pointer moved.

## [0.10.3] - 2026-10-25

### Fixed

- An overlay that flips to its other side in the frame it opens is restyled completely in that frame: classes of the new side that change inherited text styles or `display` reach its children and fields, a text color function is counted once, and an overlay holding a `VirtualList` restarts its entrance from the right side; a placement published again in the frame it was already published in is applied in that frame (ADR-164).
- Keyboard navigation in a scrolling menu no longer jumps back to the first item when the mouse rests over the menu (the leave events that scrolling fires under a still pointer no longer clear the highlight); a controlled `ColorPicker` or `ColorInput` shows the new code in the frame of a drag move, not one frame later; a bad paste in the hex field during a drag restores the current code (ADR-162, ADR-164).
- An `Input` with a zero-percentage height (`h-[0%]`, `h-[calc(0%+20px)]`) places its text as Chromium does (ADR-159).

## [0.10.2] - 2026-10-25

### Added

- `kit.ColorInputStyle` gains the recipe parts `Code` and `CodeAlpha`, the classes of the color code on a `ColorInput`'s button (ADR-162).

### Fixed

- Calendar pages are fast: scrolling a page of `kit.Calendar`s or `kit.RangeCalendar`s took 90–190 ms per frame (up to 530 ms) and takes 2–8 ms, because scrolled content painted in many chunks (every `relative` box) is shifted instead of damaged box by box, and many damage rectangles are merged in linear time; tables scroll faster too. A text whose only change is its color is repainted without shaping it again or laying out the page. A calendar rebuilds only the day cells whose state changed (ADR-161).
- `kit.ColorInput`: the code on the button and the hex field of the picker follow a drag live, controlled or not, without rebuilding the picker; a value being typed in the field is not overwritten. The button keeps one width whatever the color (ADR-162).
- Menus (`kit.ContextMenu`, `DropdownMenu`, `Menubar`) and `kit.Select` no longer scroll or cut their content when they fit within 10 px of the window edge: they open on the side where they fit, flipping if needed, and scroll only when the window has no room for them (ADR-163).
- A popover, menu, select or tooltip that flips to the other side is styled for that side (transform origin, entrance slide) from its first frame, instead of starting its entrance from the wrong side and jumping two frames later. A menu clears its highlighted item when the mouse leaves the item or moves over a disabled one, as Radix does (ADR-164).
- `examples/kit`: the custom radio options no longer move when checked, the row-actions menu button sits at the end of its row, and the context menu demo box can be focused so Shift+F10 opens it; `kit.ContextMenu`'s doc says how the keyboard reaches an area.

## [0.10.1] - 2026-10-25

### Fixed

- Flex layout matches Chromium in three more cases: a zero percentage (`flex-1`, whose basis is `0%`, `basis-[0%]`, `h-[0%]`, `top-[0%]`, a `0%` grid track) is no longer taken for `0px`, so a `flex-1` scroller in an auto-height column gets its content height instead of collapsing to its padding; a flex item with `aspect-ratio` and a percentage size (`w-full aspect-square`) shrinks to fit its line; a `w-fit` (or `w-min`, `w-max`) item in a stretching column is laid out at its own width, so a `kit.Calendar` there is 291 px tall instead of 685. A percentage `min-h-*` of a flex item with an indefinite basis resolves as in Chromium. `kit.Calendar`'s day cells get shadcn's `aspect-square` back (ADR-157).
- Gradients written as CSS (`bg-[linear-gradient(…)]`, `bg-linear-[…]`, `bg-radial-[…]`, `bg-conic-[…]`, gradient masks) interpolate in sRGB, as in CSS and Tailwind; the named utilities (`bg-linear-to-r`, `bg-radial`, `bg-conic`) keep Tailwind's Oklab. `in srgb` and `in oklab` are understood in the gradient's prelude; other color spaces are reported (ADR-158).
- Text lines are placed on the device pixel grid as in Chromium: ascent, descent and half-leading round in device pixels, so text at scale 2 (and fractional scales) sits on Chromium's rows instead of up to one device pixel off; an `Input` with a fixed height centers its text with the font's normal line height, as Chromium's inner editor does. Text in snapshots at scales other than 1, and in fixed-height inputs, moves by one device pixel (ADR-159).
- A modal dialog keeps the focus inside: a press on its scrim or on anything that cannot take the focus leaves it where it was (an alert dialog no longer loses it), and focusing an element outside the dialog from a handler or script brings it back to the last focused element inside (ADR-160).

## [0.10.0] - 2026-10-25

### Added

- Package `kit`: styled components on the theme tokens, in the look of shadcn/ui v4 (`new-york-v4`) and checked against it in Chromium, light and dark, at scales 1 and 2. Every component is a struct with its props as fields, `Class` and `Props` for the root element and slots for its parts: Button, Badge, Card, Avatar, Label, Input, Textarea, Separator, Skeleton, Field, Checkbox, Switch, RadioGroup, Toggle, ToggleGroup (a segmented control), Slider, Progress, Dialog, AlertDialog, Popover, Tooltip, Select, Tabs, Table (sortable, with a sticky header and leading columns), a Toaster with `kit.UseToast`, DropdownMenu, Menubar, ContextMenu, Calendar, RangeCalendar, DatePicker, DateRangePicker, ColorPicker and ColorInput (ADR-129, ADR-130, ADR-136 to ADR-143).
- Customizing `kit`: the whole look is data. `kit.DefaultStyle()` returns the recipes (classes per part, variant, size and state), an app edits them, adds variants, and applies them to a subtree with `kit.StyleProvider`; `kit.Theme()` is shadcn's neutral theme. Trigger slots take any `kit.Trigger` (`kit.Button`, a component with `WithTriggerProps`, or `kit.TriggerFunc`). Recipes use logical utilities and components read `tela.UseDirection()`, so `h.Dir("rtl")` mirrors their drawing, arrow keys and drags; a component's `Direction` field overrides it (ADR-129, ADR-137, ADR-151, ADR-155).
- `examples/kit`: a gallery with a page and a `Previews` entry per component, a second style built from recipe edits and a light/dark switch; the wiki's Kit guide and generated kit API reference (ADR-150).
- `aria`: `UseRadioGroup`, `UseToggleGroup`, `UseSlider`, `UseProgressBar`, `UseMeter` (ADR-132), `UseMenuBar`, `UseContextMenuTrigger` (ADR-133), `UseCalendar`, `UseRangeCalendar` and the `aria.Date` type (ADR-134), `UseColorState`, `UseColorArea`, `UseColorSlider`, `UseColorField`, HSB/HSL and hex helpers (ADR-135).
- `h.AriaInvalid` with the `aria-invalid:` variant, `h.AriaSort` with `aria-[sort=ascending]:` and `aria-[sort=descending]:`, and `telatest.Invalid`, `telatest.Sort`; the aria snapshot prints `[invalid]` as Playwright 1.61 does (ADR-131).
- `i18n`: `FirstDayOfWeek`, `Formatter.FirstDayOfWeek`, `Formatter.MonthName`, `Formatter.WeekdayName` (stand-alone names in four widths) and `Formatter.MonthYear` ("October 2026", "2026年10月") (ADR-131, ADR-134).
- `tela.DevMode()` reports whether dev mode is on where it is called, for component libraries that check their own props (ADR-153).
- `tela.UseDirection()` returns the direction (`"ltr"` or `"rtl"`) at a component's position, from the nearest `h.Dir`, `auto` included; only its readers rebuild when it changes (ADR-155).
- `tela.MergeProps(props...)` makes one prop of several, for fields that take one, such as `Props` of every kit component (ADR-156).

### Changed

- The app-screen examples are built on `kit`: dashboard (with a glass recipe), settings, pricing, signup, player, board, i18n and native (Windows 11, Adwaita and Breeze as three recipes), with Lucide icons generated by `tela icons gen` instead of drawn ones; their goldens changed (ADR-144 to ADR-149).
- `i18n`: the LDML pattern letters `L` and `c` use CLDR's stand-alone month and weekday names, as ICU does (a Finnish full date now says "maanantai"), and `cccccc` the short weekday names (ADR-134, ADR-152).
- `aria` imports `i18n` (for calendar names), which adds about 0.35 MB to an app that uses `aria` without `i18n` (ADR-134).
- `aria.SliderOptions` gains `PageSize` and `LiveValues` (ADR-135, ADR-146).

## [0.9.2] - 2026-10-25

### Added

- `tela new <dir>` creates an app: `go.mod`, a counter in `main.go` with a `var App`, a `telatest` test, a README and a `.gitignore`, then runs `go get` and `go mod tidy` (`-module` sets the module path, `-skip-deps` skips them). The app requires the version of the `tela` binary when it is a release, and the latest otherwise. A directory that is not empty is refused (ADR-128).

## [0.9.1] - 2026-10-22

### Added

- `tela dev` on Windows: when the build of a reload takes more than 400 ms to start, it prints once how to exclude its build directory and the Go build cache from Microsoft Defender's real-time scanning, which scans every new build on its first run (a dashboard reload went from 2.2 s to 1.1 s with them excluded). It never changes the settings itself (ADR-125).

### Changed

- `tela dev` on Windows builds into `%TEMP%\tela-dev` (one directory for every session) and keeps the go command's temporary files there, unless `GOTMPDIR` is set (ADR-125).

### Fixed

- `tela dev` on Windows: a frame an app presents at a new size right before it exits now reaches the window (`-v` reported "OpenFileMapping …: The system cannot find the file specified"), and removing a file without an extension (vim's `4913` probe, `LICENSE`) no longer rebuilds the app (ADR-125).
- `tela dev` on Windows: a package directory deleted or moved away right after `tela dev` starts, or by its 8.3 short name, now rebuilds the app; the Defender note counts only reloads (from a build's start to its connection), so an app that is just slow to start no longer gets it every session; and a `%TEMP%\tela-dev` that is a link or belongs to another user is not used (ADR-126).

## [0.9.0] - 2026-10-22

### Added

- `tela dev [pkg]`: a development loop that feels like fast refresh. It watches the module, rebuilds in the background while the app stays usable, and swaps the new build in under the same window, which never closes. Hook state (`UseState`, `UseReducer`, `UseValue`, `UseRef`), the text, selection and undo history of text fields, scroll offsets, the focus, the runtime color scheme, theme and locale survive the reload; a component whose hooks changed starts fresh, and dev mode names every slot that cannot be kept. Build errors and panics are drawn over the last good frame and printed on the terminal; the next good save clears them. Flags `--size`, `--scale`, `--dark`, `--preview NAME`, `--no-state`, `-v`; terminal keys `r` (reload), `R` (reload without state) and `q` (quit). Linux and Windows (ADR-111 to ADR-117; Windows fixes follow in patch versions).
- `App.DevState{Save, Restore}`: keeps state that lives outside the component tree across `tela dev` reloads. Assign it from a `//go:build teladev` file so release builds link none of it (ADR-114).
- Release builds are unchanged: everything of the dev loop compiles only with the `teladev` build tag, which `tela dev` passes.

### Changed

- Apps start about 20 ms sooner: the embedded fonts are parsed concurrently (the result is identical) (ADR-116).

### Fixed

- A text field reveals its caret after layout: an `AutoFocus` input no longer starts scrolled by 1 px with its caret hidden, a caret moved to the end before the first frame shows the end of the text, a field resized in the same build reveals with its new width, and typing into a `field-sizing-content` input no longer scrolls the text away, and a field focused from an effect, a handler or a posted function that also resizes it reveals its caret with the new size (ADR-118, ADR-120, ADR-122, ADR-124).
- A layout effect that moves the focus in a frame with nothing else to do now applies the newly focused element's `focus:` styles in that frame; before, they waited for the next change (ADR-124).

## [0.8.2] - 2026-10-21

### Removed

- `aria.TabsOptions.TextValue`: a tab list has no typeahead, as in React Aria. Typing a letter used to move the focus without selecting the tab, so the panel disagreed with the focused tab.

### Fixed

- An image or icon load whose cache key panicked when hashed left the asset cache's lock held, so every later `Img`, `Icon` or background-image load deadlocked; the lock is now always released, and a `tela.BgImage` image that is comparable but holds a non-comparable value in an interface field is converted without the cache instead of panicking (ADR-109).
- A floating element whose size limits keep changing no longer keeps other floating elements from getting the follow-up frames that apply their limits: the frames are counted per element (ADR-109).
- A quick tap after a touch scroll is no longer counted as the second click of a double click (a word selected in a text field, `OnDoubleClick`) (ADR-109).
- A touch that scrolls no longer fires `pointerenter`/`pointerleave` (and the mouse ones) on the elements the finger passes over: the pressed element's chain is left right after the `pointercancel`, then nothing more of the touch is dispatched until it lifts (ADR-109).

## [0.8.1] - 2026-10-21

### Fixed

- `TELA_DEBUG=layers` no longer panics on a window with a text field (`Input`, `Textarea`): the layers overlay had no border color for text-field layers since v0.5.0.

## [0.8.0] - 2026-10-21

### Added

- `examples/aria`: a "Command center" screen in the style of shadcn/ui built only from `aria` hooks, `h` elements and theme-token classes (no kit, no hand-written key handling): a project combobox (`UseCombobox`, thirty projects with icons), a tooltip with an arrow, two menus (`UseMenuTrigger`, `UsePopover`, `UseMenu`: commands with icons, sections and a disabled item; checkbox items), a modal dialog with a form (`UseOverlayTrigger`, `UseOverlay`, `UseDialog`, `UseSelect`, `UseSwitch`, `UseButton` with `Type: "submit"`), tabs and a multiple-selection list box, with `Presence` entrances and exits, light and dark. Its tests drive the menu, the combobox and the dialog with the keyboard only and assert their aria snapshots and the focus, and it has goldens at scales 1 and 2 in both schemes, with its Lucide subset in `examples/aria/icons` generated by `tela icons gen`.
- `h.AriaLabelledBy(refs ...tela.ElementRef)` takes several references (`aria-labelledby="a b"`): the name is the text of each mounted one, in the order given, joined by a space. Existing calls with one reference compile unchanged.
- `telatest` states for `App.Expect` that read what the aria snapshot does not print: `telatest.HasPopup(kind)` (aria-haspopup), `telatest.ActiveDescendant(loc)` and `telatest.NoActiveDescendant()` (aria-activedescendant points to the one element `loc` locates, or to none) and `telatest.Controls(loc)` (aria-controls). A failure names the element the reference actually points to.
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

- `aria.UseListBox`, `aria.UseMenu` and `aria.UseTabs` search the text of a key as `fmt.Sprint` prints it when `TextValue` is nil (as `UseSelect` and `UseCombobox` already did), so typeahead works with string keys; `aria.TabsOptions.TextValue` is new. A Space typed within the one-second search window is a character of the search and selects nothing (React Aria's behavior), and `Selection.Len()` of `aria.All` is 0 (use `len(sel.In(keys))`): both are documented.
- A text field with the `combobox` role (`Input(Role("combobox"))`) shows its text in the aria snapshot and `tela inspect`, as Playwright prints an `input role="combobox"`: `combobox "Fruit": App`. An element may be named by several references (`aria-labelledby="a b"`) inside the framework (`core.AriaLabelledBy`); `h.AriaLabelledBy` still takes one.
- `aria.ToggleOptions.Selected` is a `*bool` (nil: uncontrolled; `Selected: &on` with `OnChange` controls it), so `DefaultSelected` and `OnChange` can be used together. `aria` is new since v0.7.0, so this breaks no released code. An element that receives two different `h.Ref`s (for instance the props of two `aria` hooks that each made their own ref) panics in dev mode, naming both call sites and the fix; before, the last ref silently won.
- An `Img` with both a width and a height (`w-30 h-10`) is drawn at that size, its image fitted by `object-fit` (stretched by default), as in a browser; it used to take the image's aspect ratio for its height.

- Under `telatest.NoSettle()`, `AriaSnapshot`, `Count`, `Bounds`, `Stats` and `Cursor` no longer run the clock for animations: they handle only the work due, like the events of an action. Expectations still poll, but their first try handles only the work due and the clock moves only between failed tries. The wait before an action still settles fully. Tests that relied on an expectation or a query to finish an animation under `NoSettle` call `app.Advance(d)` instead.
- A single `nil` dependency list (`UseEffect(f, nil)`) panics in dev mode with the fix (`tela.Once`, `tela.UseMount` or `tela.UseOnce`); outside dev mode it keeps meaning "mount only". It was a warning in v0.7.0.

### Fixed

- `aria` collections: when the focused item goes, the focus no longer lands on an item that the same build disables; it moves like react-stately's `useListState` (the item that took its place, never a disabled one, going back at the end; several items gone at once move it back by their number), and a collection that had the focus keeps it.
- `aria.UseTooltipTrigger`: only Enter and Space on the trigger close the tooltip (a press, as in React Aria); Shift, the arrow keys and shortcuts no longer hide it.
- Enter in a text field no longer presses the first hook-made button of its `Form`: a `Button` with the props of an `aria` press hook (a select's trigger, a switch, a menu button, `UseButton`) is `type="button"` until the hook's `Type` says `"submit"` or `"reset"`, so the form's default button is the first real submit button; an explicit `h.Type` still wins. Enter used to open a select or flip a switch that came first in the form instead of submitting it (ADR-106).
- `aria.UseMenuTrigger` focuses its trigger when a mouse or a pen presses it, before the menu opens, so that closing the menu (Escape, choosing an item) gives the focus back to the trigger; it used to give it back to nothing after a mouse open.
- The `checked:` variant, and `peer-checked:` and `group-checked:`, now match an element whose `h.AriaChecked` is true (a checkbox or switch, such as one made with `aria.UseCheckbox`); they never matched anything before, since Tela has no native checkbox. A false or mixed value does not match, and only the elements whose classes test it restyle.
- A touch that becomes a scroll now sends `pointercancel` to the element the finger pressed, as a browser does, so `OnPointerCancel` handlers (and `aria.UsePress`) end the press instead of seeing a release after the scroll. Nothing more of that touch is dispatched until it lifts (no `pointermove`, `pointerup` or `mouseup`), and an overlay (`aria.UseOverlay`, `UsePopover`, a menu) is no longer dismissed as if pressed outside when the user only scrolled past it. The pressed element's `active:` style ends when the scroll starts, as in Chromium, instead of when the finger lifts.
- `telatest` no longer cuts finite animations short on a slow or busy machine: whether the app goes quiet is decided in app time, so `Mount`, actions and `Golden` wait for transitions and springs to end however long their frames take, and only an app still requesting frames after the timeout of app time is reported as endless. Goldens could be captured mid-animation before.
- `h.SrcFS` with a file system whose type is comparable but whose value holds a map (a struct wrapping an `fstest.MapFS`) no longer panics in the image cache, which also left later image loads deadlocked; such a file system loads without the cache.
- `ElementRef.Click` and the activation of an `aria` button hook, called from an overlay's callbacks (`OnInteractOutside`, `OnEscape`, `OnFocusOutside` and the interact-outside listeners) or from a `UseEffect`, no longer apply their updates and build in the middle of the event or of the effects: the event that is running, or the next step after the effects, does it, as for any handler.
- `ElementRef.Focus` and the `aria` focus manager and focus scopes, called from a handler, an overlay callback or a `UseEffect`, no longer apply the focus handlers' updates and build before the call returns: the focus events fire at once and the event that is running (or the next step after the effects) applies and builds.
- Size limits of a floating element (`aria.Size`) that still changed after the two layouts a frame allows are applied in the following frames instead of never.
- Enter in a text field submits through a submit button that an `aria` hook made from any element: the form's default button is its first submit button in tree order, a `Button` of type submit or, for example, a `Div` with the props of `aria.UseButton(aria.ButtonOptions{Type: "submit"})`, so a form with several fields submits through it as through an HTML submit button. A disabled default button still blocks the submission, as in HTML.

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

[Unreleased]: https://github.com/kuronosu/tela/compare/v0.14.0...HEAD
[0.14.0]: https://github.com/kuronosu/tela/compare/v0.13.7...v0.14.0
[0.13.7]: https://github.com/kuronosu/tela/compare/v0.13.6...v0.13.7
[0.13.6]: https://github.com/kuronosu/tela/compare/v0.13.5...v0.13.6
[0.13.5]: https://github.com/kuronosu/tela/compare/v0.13.4...v0.13.5
[0.13.4]: https://github.com/kuronosu/tela/compare/v0.13.3...v0.13.4
[0.13.3]: https://github.com/kuronosu/tela/compare/v0.13.2...v0.13.3
[0.13.2]: https://github.com/kuronosu/tela/compare/v0.13.1...v0.13.2
[0.13.1]: https://github.com/kuronosu/tela/compare/v0.13.0...v0.13.1
[0.13.0]: https://github.com/kuronosu/tela/compare/v0.12.6...v0.13.0
[0.12.6]: https://github.com/kuronosu/tela/compare/v0.12.5...v0.12.6
[0.12.5]: https://github.com/kuronosu/tela/compare/v0.12.4...v0.12.5
[0.12.4]: https://github.com/kuronosu/tela/compare/v0.12.3...v0.12.4
[0.12.3]: https://github.com/kuronosu/tela/compare/v0.12.2...v0.12.3
[0.12.2]: https://github.com/kuronosu/tela/compare/v0.12.1...v0.12.2
[0.12.1]: https://github.com/kuronosu/tela/compare/v0.12.0...v0.12.1
[0.12.0]: https://github.com/kuronosu/tela/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/kuronosu/tela/compare/v0.10.4...v0.11.0
[0.10.4]: https://github.com/kuronosu/tela/compare/v0.10.3...v0.10.4
[0.10.3]: https://github.com/kuronosu/tela/compare/v0.10.2...v0.10.3
[0.10.2]: https://github.com/kuronosu/tela/compare/v0.10.1...v0.10.2
[0.10.1]: https://github.com/kuronosu/tela/compare/v0.10.0...v0.10.1
[0.10.0]: https://github.com/kuronosu/tela/compare/v0.9.2...v0.10.0
[0.9.2]: https://github.com/kuronosu/tela/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/kuronosu/tela/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/kuronosu/tela/compare/v0.8.2...v0.9.0
[0.8.2]: https://github.com/kuronosu/tela/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/kuronosu/tela/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/kuronosu/tela/compare/v0.7.0...v0.8.0
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
