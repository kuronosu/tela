<!-- The sources behind each cell of compare.json (the comparison table of the landing page), so a maintainer can re-check them. -->

# compare.json: supporting passages (as of 2026-10-31)

Source ids match `site/src/data/compare.json`. Tela cells are from the repo (README, docs/wiki/CLI.md, docs/ROADMAP.md, LICENSE, `git tag` = v0.12.6) and are not repeated here except where noted.

Versions were checked against the Go module proxy (`proxy.golang.org/<module>/@latest`) and `gh release list`:
- fyne.io/fyne/v2 @latest = v2.8.2 (tag 2026-10-07; GitHub release "Latest", published 2026-10-12)
- gioui.org @latest = v0.10.3 (2026-09-28)
- github.com/wailsapp/wails/v3 @latest = v3.0.0 (2026-10-26, GitHub "Latest"); v2 line at v2.17.0 (2026-10-18), still maintained
- electron: v45.2.1 is GitHub "Latest" (published 2026-10-31); v46 is in alpha

wails.io answers 403 to non-browser clients, so the Wails v3 docs are cited from their source in the repo at tag v3.0.0 (`docs/mpress/content/...`); the published pages have the same slugs on wails.io (e.g. https://wails.io/guides/mcp-service/).

---

## 1. How the UI is written

- **Fyne** [1]: "Fyne is an easy-to-use UI toolkit and app API written in Go." README example: `hello := widget.NewLabel("Hello Fyne!")`, `w.SetContent(container.NewVBox(hello, widget.NewButton("Hi!", func() { hello.SetText("Welcome :)") })))`.
- **Gio** [11]: "Gio is a library for writing cross-platform immediate mode GUI-s in Go." [16]: "Gio is a library for implementing immediate mode user interfaces."
- **Wails** [23]: "Your application logic is written in Go, your interface is built with HTML, CSS, and JavaScript (or any frontend framework), and Wails renders it in the operating system's native webview."
- **Electron** [31]: "Electron is a framework for building desktop applications using JavaScript, HTML, and CSS. By embedding Chromium and Node.js into its binary, Electron allows you to maintain one JavaScript codebase..."

## 2. Needs cgo or a C toolchain

- **Fyne** = yes [1]: "To develop apps using Fyne you will need Go version 1.22 or later, a C compiler and your system's development tools." [2] (Quick start prerequisites: Go, a C compiler, a graphics driver): Windows: "The MSYS2 platform is the recommended approach for working on Windows." (installs the MinGW-w64 toolchain); macOS: install Xcode and run `xcode-select --install` for the command line tools; Linux: "You will need to install Go, GCC and the graphics library header files using your package manager."
- **Gio** = partial [12]: "no special compiler is needed, as we don't use CGo for Windows support." [14] `app/os_wayland.go` has a cgo preamble: `#cgo linux pkg-config: wayland-client wayland-cursor`. [15] `app/os_macos.go`: `#cgo LDFLAGS: -framework AppKit -framework QuartzCore` ... `import "C"`. (os_x11.go also uses cgo; os_windows.go does not.)
- **Wails** = partial [24], "CGO Requirements" table: "Windows | No (default) | Native Go. Docker only if CGO_ENABLED=1"; "macOS | Yes | Docker with macOS SDK"; "Linux | Yes | Docker, or native if C compiler available". Also: "Windows is the simplest cross-compilation target because it doesn't require CGO by default." / "macOS builds require CGO for WebView integration" / "Linux builds require CGO for WebView integration."
- **Electron** = no [32]: "During installation, the electron module will call out to @electron/get to download prebuilt binaries of Electron for your platform." [33]: "Native Node.js modules are supported by Electron, but since Electron has a different application binary interface (ABI) from a given Node.js binary ... the native modules you use will need to be recompiled for Electron." (uses `node-gyp` / `@electron/rebuild`). Electron is not Go, so cgo itself does not apply.

## 3. Uses a web engine

- **Fyne** = no [3]: "No web runtime. No JavaScript. Just Go."
- **Gio** = no [11]: "Gio includes an efficient vector renderer based on the Pathfinder project implemented on OpenGL ES and Direct3D 11"; "Text and other shapes are rendered using only their outlines without baking them into texture images".
- **Wails** = yes [23]: "Wails renders it in the operating system's native webview. The result is a small, fast, native-feeling application: no bundled browser"; platform table: Windows "Uses the WebView2 runtime"; Linux "The default stack is GTK4 with WebKitGTK 6.0". [46] (macOS): "That's it! macOS includes WebKit by default."
- **Electron** = yes [31]: "By embedding Chromium and Node.js into its binary".

## 4. Tailwind classes

- **Fyne** = no [4]: "Applications are able to load custom themes that can apply small changes to the standard theme..." / "A theme will need to implement the functions of fyne.Theme interface" (methods Color, Font, Icon, Size).
- **Gio** = no [17]: (material package) "the Theme struct encapsulates the parameters for varying colors, sizes and fonts."
- **Wails** = yes, via the web stack [25] (community templates): "wails-vite-react-ts-tailwind-template - A template for React + TypeScript + Vite + TailwindCSS"; "wails-vite-svelte-tailwind-template - A template using Svelte and Vite with TailwindCSS v3"; others. [35] (Tailwind docs): "Tailwind CSS works by scanning all of your HTML files, JavaScript components, and any other templates for class names, generating the corresponding styles and then writing them to a static CSS file."
- **Electron** = yes, via the web stack [31] (UI is HTML/CSS rendered by Chromium) + [35] as above. No official Electron or Forge Tailwind guide was found; the claim rests on Electron rendering ordinary HTML/CSS.

## 5. React-style components and hooks

- **Fyne** = no [1]: widgets are objects mutated in callbacks (`hello.SetText("Welcome :)")`). [5]: "Data binding is a powerful new addition to the Fyne toolkit that was introduced in version v2.0.0." / "By using data binding we can avoid manually managing many standard objects like Labels, Buttons and Lists."
- **Gio** = no [16]: in Gio "the program is in control of clearing and updating the display"; contrasted with "traditional 'retained mode' libraries [that] own the widgets through implicit library-managed state". The loop "Runs code that lays out and redraws the user interface state".
- **Wails** = yes, via React [22]: "Templates for vanilla, React, Vue and Svelte, each in JavaScript or TypeScript, plus your own custom templates."
- **Electron** = yes, via React [36] (Electron Forge, an electron-org project): "Adding React support to the Webpack template doesn't require a complicated boilerplate to get started."

## 6. Renders headless to images for tests

- **Fyne** = yes [6] `test/test_helper.go`: "AssertRendersToImage ... The test `t` fails if the given image is not equal to the loaded master image. In this case the given image is written into a file in `testdata/failed/<masterFilename>`" (Since 2.3). `AssertObjectRendersToImage` builds `NewCanvasWithPainter(software.NewPainter())`, i.e. a windowless canvas with the software painter.
- **Gio** = yes [18] `gpu/headless/headless.go`: "Package headless implements headless windows for rendering an operation list to an image." API: `NewWindow(width, height)`, `(*Window).Frame(*op.Ops)`, `(*Window).Screenshot(*image.RGBA)`. Contexts per OS: EGL (linux/freebsd/openbsd), D3D11 with `DRIVER_TYPE_HARDWARE` (windows), Metal (darwin), so a GPU API (or a software implementation of one) is required.
- **Wails** = no [26]: "Use Playwright against a server build to exercise your frontend and real Go services in a browser." [27] the MCP tool list includes "`screenshot_dom` | Structural snapshot of the visible page (DOM-based, no pixels)". The testing guide (guides/testing.md) covers Go unit tests and Vitest only; no image capture API found.
- **Electron** = partial [37]: offscreen rendering lets you "obtain the content of a BrowserWindow in a bitmap or a shared GPU texture"; the example's `paint` handler receives an `image` and saves it with `toPNG()`. [38]: "Being based on Chromium, Electron requires a display driver to function. If Chromium can't find a display driver, Electron will fail to launch" (the page sets up Xvfb). Also `webContents.capturePage()` exists (not separately cited).

## 7. Built-in tooling for AI agents

- **Fyne** = no. Absence check: `gh search repos mcp --owner fyne-io` returned nothing; `gh search code mcp --repo fyne-io/fyne` matched only `memcpy`; the website [3] and README [1] mention no agent tooling; a web search found no Fyne MCP server. (Fyne does have `test.AssertRendersToMarkup`, a markup tree of a canvas for tests, not agent tooling.)
- **Gio** = no. Absence check: grep of the v0.10.3 module source for "mcp" found nothing; [11] and [19] mention no agent tooling.
- **Wails** = yes [22]: "An MCP service that lets AI tools drive and inspect a running Wails app." [27]: "Wails v3 has a built-in Model Context Protocol (MCP) server that lets LLM agents ... inspect, test, and drive a running Wails application." / "The built-in MCP server is experimental and the API may change in future releases." / "The MCP server is compiled into your application only when the `mcp` build tag is present." There is also a separate `wails3 mcp` CLI server for project lifecycle tasks.
- **Electron** = no. Absence check: `gh search repos mcp --owner electron` returned nothing; `gh search code mcp --repo electron/electron` matched only `memcpy`; Electron docs [31] mention no agent tooling. Third-party [41]: "MCP server that lets AI assistants automate, debug and screenshot any Electron app over the Chrome DevTools Protocol" (halilural/electron-mcp-server; not an electron-org project).

## 8. Platforms

### Linux
- **Fyne** [2]: Debian/Ubuntu packages `golang gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev`. [8]: "Wayland is now supported by default and will automatically be picked at runtime".
- **Gio** [11]: "Gio supports all the major platforms: Linux, macOS, Windows, Android, iOS, FreeBSD, OpenBSD and WebAssembly." [13]: "For Linux you need development packages for:" Wayland, x11, xkbcommon, GLES, EGL, libXcursor.
- **Wails** [23]: "Linux | AMD64 and ARM64. The default stack is GTK4 with WebKitGTK 6.0".
- **Electron** [34]: "Linux: Electron provides `x64` (`amd64`) and `arm64` binaries for Linux."

### Windows
- **Fyne** [2]: "The MSYS2 platform is the recommended approach for working on Windows." [3]: platform list "Windows macOS Linux iOS Android".
- **Gio** [12]: "no special compiler is needed, as we don't use CGo for Windows support."
- **Wails** [23]: "Windows | AMD64 and ARM64. Uses the WebView2 runtime."
- **Electron** [34]: "Windows (Windows 10 and up): Electron provides `x64` (`amd64`) and `arm64` binaries for Windows."

### macOS
- **Fyne** [2]: install Xcode, then `xcode-select --install` for the command line tools. [3]: platform list.
- **Gio** [11]: platform list. [47]: "Xcode is required for Apple platforms."
- **Wails** [23]: "macOS | 10.15+ on Intel (applications can target 10.13+), 11.0+ on Apple Silicon. Universal binaries are supported." [30]: "macOS | Intel and Apple Silicon". [46]: "Xcode Command Line Tools (required)".
- **Electron** [34]: "macOS (Ventura and up): Electron provides 64-bit Intel and Apple Silicon / ARM binaries for macOS."

### Mobile (iOS, Android)
- **Fyne** = yes [1]: "It is designed to build applications that run on desktop and mobile devices with a single codebase."; `fyne package -os android -appID ...`, `fyne package -os ios -appID ...`. [3]: "iOS Android".
- **Gio** = yes [11]: platform list includes Android and iOS. [12]: mobile builds use the `gogio` tool.
- **Wails** = partial [23]: "iOS and Android | Experimental." [28]: "Wails v3 runs on iOS and Android using the same main.go and frontend you already write for desktop." [30]: "Android and iOS support remains experimental and is outside the stable desktop compatibility contract."
- **Electron** = no [34]: "Each Electron release provides binaries for macOS, Windows, and Linux."

### Web
- **Fyne** = yes [7]: a web app "will bundle a WebAssembly binary which will run in most modern browsers"; "As of release v2.5.0 the web driver is fully supported but is not yet 100% complete." Listed gaps: "File open/save dialog", "Storage of Documents".
- **Gio** = partial [19]: "Immediate mode GUI programs in Go for Android, iOS, macOS, Linux, FreeBSD, OpenBSD, Windows, and WebAssembly (experimental)." [11]: "This requires a browser that supports WebAssembly."
- **Wails** = partial [23]: "You can also serve your application as a regular web app using the server build." [29]: "Wails v3 supports server mode, allowing you to run your application as a pure HTTP server without creating native windows ... This enables deploying the same Wails application to servers, containers, and web browsers."; table: "System tray | Available | Not available", "Native dialogs | Available | Not available", "Application menu | Available | Not available".
- **Electron** = no [34]: binaries for macOS, Windows and Linux only.

## 9. Screen reader support today

- **Fyne** = partial [8] (v2.8.0, July 2026): "Adding accessibility support (currently off by default, use -tags accessibility to test)". [9] `internal/driver/glfw/accessibility_notdarwin.go` is `//go:build !accessibility || (!darwin && !windows)` and only has empty stubs, so the desktop bridge exists for macOS (`accessibility_darwin.m`, AppKit) and Windows (`accessibility_windows.c`, ole32/oleaut32) only. Mobile: `internal/driver/mobile/accessibility_android.go` (`//go:build accessibility && android`) and `accessibility_ios.go` (`accessibility && ios`). Roles in v2.8.2 `accessibility.go`: button, container, link, text. NOTE: the v2.8.0 release blurb says "Linux Wayland testing of Accessibility", but no Linux bridge exists in the v2.8.2 tree; the cell says "not Linux" based on the source.
- **Gio** = partial [20]: "Package semantic provides operations for semantic descriptions of a user interface, to facilitate presentation and interaction in external software such as screen readers." [21] `app/GioView.java`: `@Override public AccessibilityNodeProvider getAccessibilityNodeProvider()`; `app/os_android.go`: "Android needs invalidate to refresh the TalkBack focus indicator." In `app/`, only window.go, os_android.go and GioView.java reference semantic/accessibility; the Windows, macOS, Wayland, X11 and JS backends do not.
- **Wails** = unknown. The v3 docs contain no "accessibility" or "screen reader" page (gh code search over docs/mpress/content). Context only: user issues #4671 "[v2 screen reader] rtl textarea support" (macOS) and #6249 "accessibility: Standard buttons and radio buttons not in keyboard focus order on macOS" suggest webview accessibility works to some degree, but Wails makes no documented claim.
- **Electron** = yes [39]: "Electron applications will automatically enable accessibility features in the presence of assistive technology (e.g. JAWS on Windows or VoiceOver on macOS)." / `app.setAccessibilitySupportEnabled` lets you "manually expose Chrome's accessibility tree to users".

## 10. Stable 1.0+ release

- **Fyne** = yes [10]: release "v2.8.2: Many fixes, optimisations and remove latency on iOS", marked Latest.
- **Gio** = no [19]: "Pre-1.0 tags are provided for reference only, and do not designate releases with ongoing support." / "as the major version is zero: breaking API or behavior changes will increment the *minor* version component." Latest tag v0.10.3.
- **Wails** = yes [22]: "Wails v3.0.0", Latest, published 2026-10-26; [30]: "Wails v3.0.0 — Stable" / "Android and iOS support remains experimental and is outside the stable desktop compatibility contract."
- **Electron** = yes [40]: "electron v45.2.1", Latest, published 2026-10-31.

## Added row: License

- **Fyne** [42]: BSD-3-Clause (GitHub license API, `spdx_id`).
- **Gio** [43]: "This project is provided under the terms of the UNLICENSE or the MIT license".
- **Wails** [44]: MIT (GitHub license API).
- **Electron** [45]: MIT (GitHub license API; README "License: MIT").
- **Tela**: LICENSE "BSD 3-Clause License".
