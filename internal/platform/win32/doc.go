// Package win32 is the Windows platform backend. It talks to the system DLLs
// (user32, gdi32, imm32, dwmapi, shcore, kernel32; shell32 and ole32 for the
// desktop services; advapi32 for Credential Manager; combase for WinRT
// toasts) through
// golang.org/x/sys/windows
// and syscall.NewCallback, with no cgo, and implements the platform contract
// documented in package platform ("Mapping onto each OS: Win32").
//
// The code that touches the OS is built only on Windows. The files without a
// build constraint (dib.go, dpi.go, msg.go, pointer.go, cursor.go,
// keyboard.go, scancode.go, vkey.go, text.go, systemsettings.go, ime.go,
// frame.go, dataformat.go, droptarget.go, menu.go, style.go, dialog.go,
// shell.go, tray.go, toast.go, hotkey.go, instance.go, secret.go, trash.go) hold the
// pure logic behind it (pixel conversion, DPI and size math, message
// decoding, the pointer state machine, the cursor table, the keyboard
// translation, line endings, the settings values, the IME events, window
// placement, hit-testing and icon pixels, the clipboard formats, the drop
// session, the menu commands, the window styling, and the pure halves of
// the dialogs, shell calls, trays, toasts, hotkeys, single instance,
// secrets and trash), so it is tested on every
// OS; the Windows files are exercised by the shared platformtest
// conformance suite under Wine (and on a real Windows machine, where the
// live_*_test.go files type on the real keyboard path with SendInput).
//
// # Loop
//
// The UI thread is the thread that called [New]; New locks the goroutine to
// its OS thread (Win32 binds windows and their message queue to the thread
// that created them) until Run returns. Run is a GetMessageW /
// TranslateMessage / DispatchMessageW loop. A single WndProc, installed with
// syscall.NewCallback, serves every window and one hidden message-only
// window owned by the Backend. WndProc translates messages into the shared
// loop's queue (internal/platform/internal/loop), then runs a delivery pass
// (the pump): queued events, due frames, and arming the frame pacer. Because
// the pump runs inside WndProc and not only in the Run loop, it also runs
// while DefWindowProc is in one of its modal loops (interactive move and
// resize, window menus), during which the Run loop is blocked and only
// WndProc is called.
//
// A panic in the handler cannot unwind through the foreign frames between
// DispatchMessageW and the callback. WndProc recovers it, stops the loop
// (PostQuitMessage, which modal loops honor) and Run panics again with the
// original value once the loop is out.
//
// The handler is never called re-entrantly: a message that Windows delivers
// synchronously from inside a handler call (SetWindowPos sends WM_SIZE,
// ShowWindow sends WM_SIZE, WM_SETFOCUS and WM_PAINT) only queues its event,
// and a follow-up pass delivers it after the handler returns.
//
// # Wake, Quit and pacing
//
// Wake and Quit post a private message (WM_APP) to the message-only window
// with PostMessageW. They coalesce: at most one such message is in flight,
// so a thousand Wake calls cannot fill the 10,000-message queue. A window,
// not PostThreadMessageW, because thread messages are dropped while a modal
// loop runs. Quit sets the loop's flag and the pump answers with
// PostQuitMessage.
//
// A requested frame that is not due is paced by the display: a helper
// goroutine calls DwmFlush (which blocks until the compositor's next
// vsync) and posts a private message, which WndProc turns into the loop's
// display signal. The goroutine exists only while a frame is waiting, so an
// idle loop sleeps in GetMessageW (0% CPU). If DwmFlush returns at once
// (composition off, an emulator), the goroutine sleeps for the rest of the
// refresh interval instead. Posted messages are dispatched by modal loops
// too, so animations keep running during a live resize.
//
// # Sizes and DPI
//
// The process opts in to per-monitor DPI awareness v2, falling back to
// per-monitor (Windows 8.1) and system (Vista) awareness. Windows work in
// physical pixels: the scale is dpi / 96, the logical size of a window is
// platform.LogicalSize(client rect, scale), and the framebuffer is
// platform.PhysicalSize of that, which maps back to the client rect.
// WindowOptions sizes are logical and describe the client area; the outer
// size comes from AdjustWindowRectExForDpi. WM_SIZE and WM_DPICHANGED
// produce a ConfigureEvent and, in the same pass, the FrameEvent it forces,
// so the app repaints before DefWindowProc continues.
//
// # Present
//
// The backend keeps a top-down 32-bit BGRA copy of the window content (a
// plain Go slice used as the bits of SetDIBitsToDevice). Present converts the
// damaged rectangles from the premultiplied RGBA framebuffer into it and
// blits them; WM_PAINT repaints from the copy without involving the app.
// Windows are opaque and alpha is ignored, except under a client-area
// backdrop (Mica, MicaAlt, Acrylic; windowstyle.go), where the copy keeps
// the frame's premultiplied alpha. The window is shown at its first
// Present.
//
// # Pointer
//
// Mouse messages (WM_MOUSEMOVE, buttons, WM_MOUSELEAVE via TrackMouseEvent,
// WM_MOUSEWHEEL and WM_MOUSEHWHEEL). A press captures the mouse (SetCapture)
// until every button is released, so drags keep reporting outside the
// window; a leave that happens during the capture is reported after the
// release, like the other backends. WM_POINTER (touch and pen) is not used
// yet: touch arrives as the mouse messages Windows synthesizes.
//
// # Keyboard
//
// A key press is several messages, and a KeyEvent needs them all
// (keyboard.go): WM_KEYDOWN or WM_SYSKEYDOWN gives the physical key and the
// virtual key, and the character messages that TranslateMessage posts with
// it (WM_CHAR, WM_SYSCHAR, WM_DEADCHAR, WM_SYSDEADCHAR) give the text. They
// are already in the queue, ahead of every other posted or input message, when
// the key message is dispatched, so the translator peeks at the queue
// (PeekMessageW with PM_NOREMOVE, over the range of the character messages
// only, because a key release that is queued already would hide them from a
// wider range; the PM_QS_* flags are not used, they miss characters that a
// previous peek has "seen"): a press followed by character messages is held
// until they arrive, any other press is delivered at once.
//
//   - Code comes from the scan code with the extended-key flag folded in
//     (0xE000), through the table in scancode.go (the W3C table, Firefox's
//     and winit's); unknown keys carry the scan code. Location comes from
//     Code. Windows reports NumLock as extended and Pause as plain 0x45, and
//     swaps their virtual keys while Ctrl is held, so Key is taken from Code
//     for those two (as winit does). The fake Shift presses of nav keys
//     (scan codes 0xE02A and 0xE036, PS/2 keyboards) are ignored. The fake
//     Shift release and press that Windows puts around Shift+numpad key with
//     NumLock on (the key becomes End, and Shift is up while it is pressed)
//     are real messages and are reported, as browsers report them.
//   - Key is the character the layout gives the key with Shift and Caps Lock
//     but without Ctrl and Alt: ToUnicodeEx on a copy of the keyboard state
//     with those removed, with wFlags 4 (Windows 10 1607), which reads a
//     pending dead-key sequence without finishing it and never starts one
//     (checked on Windows 11 by TestToUnicodeDeadKeyState). Where the flag is
//     ignored, a call that finds a dead key is repeated, which consumes the
//     accent again. A key with a name is named from its virtual key
//     (vkey.go). Numpad keys with NumLock on type
//     characters through the layout (the decimal key is "." or ","), with
//     NumLock off they are the navigation keys under their numpad Code
//     ("End" for Numpad1), as in browsers.
//   - Text is what the character messages carry, so a dead key and the key
//     after it that does not compose type both ("´x"; the key is then the
//     layout's "x", winit's rule), and a composed press has the composed
//     character as Key and Text ("é"). With Ctrl held Windows turns letters
//     into control codes, so Text is the layout's character instead. Control
//     characters are never text (Enter, Tab, Backspace and Escape give ""),
//     except that Windows types the pending accent before them, so Enter
//     after a dead acute accent has Key "Enter" and Text "´". Dead keys have
//     Key "Dead" and no Text; their release too (the kernel still holds the
//     accent then, and asking the layout again would answer with it).
//   - AltGr: a layout with AltGr (found by asking ToUnicodeEx whether Ctrl+Alt
//     changes a character, once per layout) sends a left Ctrl and a right Alt
//     with the same message time. The Ctrl is recognized by peeking at the
//     next key message and produces no event; the right Alt is AltGraph, and
//     keyboardModifiers reports neither Ctrl nor Alt while it is held. A left
//     Ctrl whose press the window delivered is real and is reported. Only the
//     right Alt is AltGr: Windows types AltGr's characters for Ctrl+Alt too
//     (both on the left), but Tela reports Ctrl+Alt, as the other backends.
//   - Repeat is the previous-state bit of WM_KEYDOWN. Modifiers and locks are
//     non-repeating (ADR-050): their repeats are dropped. The keyboard repeats
//     only the key pressed last, so pressing Shift while holding "a" is
//     expected to stop the repeat of "a" there (hardware typematic cannot be
//     injected with SendInput), unlike Wayland and X11. Windows also merges
//     the key-downs that wait in the queue into one message with a repeat
//     count (bits 0 to 15 of lParam); a slow handler sees one repeat for
//     several.
//   - Focus: WM_KILLFOCUS releases every held key synthetically before the
//     FocusEvent, and WM_SETFOCUS presses nothing. A window without the
//     keyboard focus reports no keys (SetFocus(NULL) sends them to the active
//     window as system keys). The modal loops of the
//     system (the window menu, moving and resizing) swallow key releases, so
//     WM_EXITMENULOOP and WM_EXITSIZEMOVE release the held keys that are no
//     longer down.
//   - The system keys are not passed to DefWindowProc (WM_SYSKEYDOWN and
//     WM_SYSKEYUP): the app sees every key, and what would enter the menu
//     loop of a window without a menu bar is dropped (WM_SYSCOMMAND with
//     SC_KEYMENU, unless it is Alt+Space, the window menu). Alt+F4 and
//     Alt+Space are handed to DefWindowProc and the app does not see the F4
//     or the space. A lone Alt and F10 are reported as key events; with no
//     menu bar nothing else happens. A window with a menu bar (SetMenu)
//     opens it itself with SC_KEYMENU, the command DefWindowProc would
//     send (ADR-234; menuKeys in keyboard.go): when a lone Alt or F10 is
//     released with no other key, mouse button (in the client area or on
//     the frame, side buttons too) or wheel between (an AltGr, and Alt
//     pressed with Ctrl or Shift held, do not count; Shift+F10 and Alt+F10
//     are not F10), and at Alt and the access key of one of its menus
//     (WM_SYSCHAR; the bar's labels get access keys from the backend,
//     buildBarMenu). The app has got the keys as KeyEvents first.
//   - Text typed by SendInput with KEYEVENTF_UNICODE (VK_PACKET: touch
//     keyboards, remote desktops) is a key with an unidentified Code, the
//     character as Key and Text. A WM_CHAR with no key press before it is
//     not a key's text (an IME result that some IME sent anyway, a posted
//     message) and is ignored; IME text arrives as IMEEvents (# IME).
//   - Alt codes (Alt held while typing a number on the numpad, NumLock on
//     or off): the digits come as WM_SYSKEYDOWN without WM_SYSCHAR, and
//     their presses have Key "0".."9" but no Text, so a field does not
//     insert them; the character is the WM_CHAR that TranslateMessage
//     posts with the release of Alt. The translator remembers that a
//     numpad key was pressed with Alt (and without Ctrl), and the release
//     of Alt that finds a WM_CHAR waiting delivers it, after the Alt
//     release, as a press and a release of its own with an unidentified
//     Code (0x10000 plus its first code unit, as VK_PACKET text) and the
//     character as Key and Text; Mods no longer has Alt, so it is typed.
//     A code for a control character (Alt+013) types nothing. This is
//     what Notepad and Chromium do with the WM_CHAR (Chromium types it as
//     a char event with no keydown); a press with Alt and no WM_SYSCHAR
//     has no Text in general, since Windows typed nothing for it.
//
// # Window control
//
// windowctl.go and frame.go (ADR-178, ADR-179). The state is reported with
// the loop's UpdateState at every WM_SIZE (before its ConfigureEvent) and
// WM_MOVE, and after the requests that change it without them: Minimized
// is IsIconic, Maximized IsZoomed (in fullscreen, the state the window
// returns to; minimized, where IsZoomed is false, the placement's
// WPF_RESTORETOMAXIMIZED), ClientSideDecorations WindowOptions.Frameless
// or TitleBarOverlay, TitleBarControls DWM's caption buttons of a
// TitleBarOverlay window (see "Title bar overlay" below).
// Position is always known: the content origin (ClientToScreen) of the
// normal placement, recorded while the window is normal; a minimized,
// maximized or fullscreen window reports where its content goes when it
// is normal again, and SetPosition and SetSize on such a window change
// rcNormalPosition (SetWindowPlacement; its workspace coordinates are
// offset by the work area of the monitor, as Chromium converts them)
// instead of moving it.
//
//   - Requests on a window that is not shown (before its first Present, or
//     hidden) cannot use ShowWindow, which would show it unpainted: Minimize
//     and SetMaximized are remembered and become the show command of the
//     next show. WindowOptions.Maximized creates the window with
//     WS_MAXIMIZE, so its first ConfigureEvent has the maximized size. The
//     first show activates the window (SW_SHOW), later ones (SetVisible)
//     do not (SW_SHOWNA), except for a modal window.
//   - Fullscreen is borderless (winit, Chromium, Raymond Chen's recipe):
//     the styles and GetWindowPlacement are saved, WS_OVERLAPPEDWINDOW,
//     WS_MAXIMIZE and the WS_EX edges are removed (clearing WS_MAXIMIZE
//     through the style avoids a restore animation), and the window covers
//     the monitor's rectangle; leaving puts back the removed bits only (the
//     topmost and disabled states may have changed) and the placement, with
//     SW_SHOWMAXIMIZED if it returns to maximized. Size limits do not apply
//     in fullscreen, and a DPI change keeps the monitor rectangle.
//   - Size limits answer WM_GETMINMAXINFO as outer track sizes at the
//     current DPI (Windows clamps the maximized size to them too).
//     SetPosition computes the frame offset with AdjustWindowRectExForDpi;
//     when the move changes the DPI (WM_DPICHANGED resizes the frame), it
//     moves once more so the content lands where it was asked.
//   - WindowOptions.Position is checked against the monitors present at
//     creation (screens_windows.go): the work areas (MONITORINFO.rcWork of
//     each EnumDisplayMonitors monitor, the MONITORINFOF_PRIMARY one
//     first) and the outer window at the DPI of the monitor under the
//     position go to platform.FitOnScreens, and a window that moves is
//     fitted again at the DPI of the monitor it lands on. CreateWindowExW
//     itself places a window anywhere, off every monitor too. SetPosition
//     is not checked.
//   - Activate restores a minimized window and calls SetForegroundWindow,
//     which Windows refuses unless the process may take the foreground (it
//     is the foreground process, got the last input event, or was allowed
//     by AllowSetForegroundWindow); then the task bar button flashes
//     (FlashWindowEx FLASHW_TRAY|FLASHW_TIMERNOFG) until the user comes.
//   - Icons: ICON_SMALL and ICON_BIG at the window's DPI
//     (GetSystemMetricsForDpi SM_CXSMICON, SM_CXICON), picked with
//     platform.PickIcon, scaled to the exact size (Catmull-Rom on
//     premultiplied pixels) and made with CreateIconIndirect from a 32-bit
//     DIB section and an empty mask, as GLFW does; rebuilt at WM_DPICHANGED.
//
// Frameless windows keep WS_OVERLAPPEDWINDOW (snap, the minimize
// animation, the system menu, Windows 11's rounded corners), and
// WM_NCCALCSIZE returns the whole window as the client area, also during
// CreateWindowExW: NewWindow registers the window for the thread before the
// call, and WndProc adopts it at its first message. The frame is not
// extended into the client area (a one-pixel top margin hid Windows 11's
// snap layouts flyout from the maximize button of a window in its normal
// state), except on the left, right and bottom under a client-area
// backdrop (glassMargins); DWMWA_WINDOW_CORNER_PREFERENCE asks for rounded corners. A maximized frameless
// window would cover its resize borders, which Windows puts outside the
// work area (onto the next monitor, under the task bar), so its client
// area is clipped to the work area of its monitor (Chromium's approach;
// Windows Terminal insets by the frame size instead).
//
// When DWM does not compose, a frameless window never lets DefWindowProc
// paint its caption, which would land on the client area: it answers
// WM_NCPAINT and the undocumented WM_NCUAHDRAWCAPTION and
// WM_NCUAHDRAWFRAME itself, passes lParam -1 to DefWindowProc for
// WM_NCACTIVATE, and runs WM_SETTEXT and WM_SETICON with the window
// temporarily not visible. With composition these go to DefWindowProc
// unchanged (answering WM_NCPAINT makes it paint a classic caption when a
// modal dialog deactivates the window), and every activation change
// repaints the client area from the retained copy (redraw.go). What the OS lost of a window (a
// reconnected remote session, a display, theme or composition change,
// resume from sleep: WM_DISPLAYCHANGE, WM_THEMECHANGED,
// WM_DWMCOMPOSITIONCHANGED, WM_POWERBROADCAST, WM_SETTINGCHANGE,
// activation) is painted again from the retained copy and the next Present
// is a full one; the corner preference is set again. The retained copy is
// opaque (alpha 0xFF), so no pixel can be transparent to DWM, except under
// a client-area backdrop (dib.go).
//
// WM_NCHITTEST asks DefWindowProc first (the OS frame of decorated windows);
// inside the client area, the app's window buttons (not at the outermost
// pixel), then the resize band of a frameless, resizable, normal
// window (SM_CXSIZEFRAME plus SM_CXPADDEDBORDER at the DPI, corners twice as
// long, Windows Terminal's width) come before the other hit regions, which answer
// HTCAPTION, HTMINBUTTON, HTMAXBUTTON (the Windows 11 snap layouts flyout),
// HTCLOSE or HTCLIENT. In fullscreen a caption region is HTCLIENT, so the
// window cannot be dragged or maximized from it (Chromium). Caption presses
// go to DefWindowProc: its move loop, double-click maximize, the system menu
// on a right click (WM_CONTEXTMENU). Over the app's own regions the
// non-client messages become client pointer events: WM_NCMOUSEMOVE is a
// move (and still reaches DefWindowProc, which drives the snap layouts
// flyout), the button messages over HTMINBUTTON, HTMAXBUTTON and HTCLOSE are
// presses and releases (double clicks are presses) that never reach
// DefWindowProc, which would track and draw classic buttons; the press
// captures the mouse, so the release is a client message. The window tracks
// the leave of the client area or, over its title bar regions, of the
// non-client area (TME_NONCLIENT), one at a time; a WM_MOUSELEAVE or
// WM_NCMOUSELEAVE of a window with hit regions is a leave only when the
// cursor is no longer over the window's client area or regions
// (WindowFromPoint and the hit test), so crossing between them is not.
// WM_SETCURSOR shows the app's cursor over its regions too, and Windows'
// resize cursors over the band.
//
// # Title bar overlay
//
// A TitleBarOverlay window (ADR-236; overlay.go, overlay_windows.go) keeps
// DWM's caption buttons over the app's title bar on Windows 11 (build
// 22000; the window must also see DWM composing at creation). It is a
// decorated window whose caption is client area: WM_NCCALCSIZE lets
// DefWindowProc compute the side and bottom resize frame and moves the top
// back to the window's top (Windows Terminal's NonClientIslandWindow).
// DwmExtendFrameIntoClientArea extends the frame at the top by a decorated
// window's caption height at the DPI plus the resize frame (the sheet of
// glass under a client-area backdrop); DwmDefWindowProc sees every message
// first (the "Custom Window Frame Using DWM" sample) and the non-client
// button messages it claims end there.
//
// DWM draws its buttons in the extended frame under the client pixels, on
// a fill of DWMWA_CAPTION_COLOR, so they show only where the presented
// pixels are transparent (Windows 11 26200: with opaque pixels there are no
// buttons at all). The window therefore presents the buttons' rectangle
// fully transparent (zero in all four bytes: DWM composes those pixels as
// premultiplied), blitting zeros there while the retained copy keeps the
// app's pixels, so a moved rectangle only needs a repaint; and it sets
// DWMWA_CAPTION_COLOR to the color the app's bar has: the app's
// TitleBar.Background when it gave one, else the pixel of the retained
// frame captionGap (4) logical pixels left of the buttons, in their middle
// row, read at every Present and state report and sent to DWM only when it
// changes. The buttons' glyphs follow DWMWA_USE_IMMERSIVE_DARK_MODE, set
// from that same color (captionDark: the glyph of the better contrast),
// else from the color scheme, so they follow the app's theme where it
// differs from the system's (the attribute is their only control:
// DWMWA_TEXT_COLOR colors the caption text, which an overlay window does
// not show; ADR-252); DWM draws them gray while the window is inactive. A window with a client-area backdrop keeps the alpha the app
// renders (no hole, no sampled color: its bar is the material); its
// buttons show where the app leaves its bar transparent.
//
// Maximized, Windows puts the frame outside the work area on every side.
// The client area's sides and bottom are clipped to the work area, but its
// top stays at the window's top: with the top clipped, DwmDefWindowProc
// answered nothing (no hover, no presses, no snap layouts) and DWM drew no
// buttons. The rows above the work area (the hidden band, the frame's
// thickness: 8 px at 100%) are not the app's: Metrics is the size below
// them, the app's frame is blitted below them (the band itself is
// transparent), and every position is translated (pointer and wheel
// positions, the hit test, TitleBarControls, the IME's caret and forms,
// drag and drop points), so the app sees the work area as at any other
// time. The hit test answers the band as caption (reachable only from a
// monitor above).
//
// WM_NCHITTEST asks DwmDefWindowProc first, which answers HTMINBUTTON,
// HTMAXBUTTON (Windows 11's snap layouts flyout) and HTCLOSE over its
// buttons; then DefWindowProc the side and bottom frame; inside the client
// area the resize band along the top only (not maximized, not FixedSize),
// the rest of the buttons' rectangle as caption, and the hit regions.
// WindowState.TitleBarControls is DWMWA_CAPTION_BUTTON_BOUNDS (relative to
// the window rectangle) in the app's logical coordinates, clipped to the
// app's area, read at every state report (WM_SIZE, WM_MOVE,
// WM_DPICHANGED, theme and composition changes) and again at the next
// three Presents, since DWM may lay its buttons out after the message (and
// a window never shown has none to report); empty in fullscreen (no
// caption, no hole, no band), the last one while minimized. The pointer
// over that rectangle is not the app's: moves there are a leave, no press
// is delivered, the cursor is the system's. The title bar's text and
// border colors are applied as for any window. Inside the package the
// window is Frameless (opts.Frameless, with Window.overlay set): no menu
// bar (SetMenu is ErrUnsupported), and the composition-off paths above keep
// DefWindowProc from painting a classic caption over it. On Windows 10, or
// without composition or the DWM procedures, the window is Frameless as
// before (OverlayAsFrameless) and TitleBarControls stays empty.
//
// Parents and modality: WindowOptions.Parent is the owner window of
// CreateWindowExW, so Windows keeps the child above it, minimizes it with
// it (WM_SHOWWINDOW SW_PARENTCLOSING pauses its frames) and destroys it
// with it. A child without a position is centered over its parent, inside
// the parent monitor's work area, as dialog boxes are, and is created at
// the parent's DPI. Close closes the children first, in creation order. A
// modal child disables its parent (EnableWindow FALSE; Windows then
// activates the child, with a beep, when the parent is clicked) from just
// before it is shown until just before it is hidden or destroyed, the order
// DialogBox and EndDialog use, so that Windows activates the parent and not
// another application; the parent counts its visible modal children.
//
// # Clipboard
//
// Text goes through CF_UNICODETEXT with the message-only window as the owner.
// ReadText posts the read to the event loop (never inside the call) and
// WriteText stores at once: Windows keeps the data, nothing is rendered on
// demand. OpenClipboard is retried for about 100 ms while another process
// holds it. Line endings: Windows text is CR LF; ReadText converts CR LF and
// lone CR to LF, and WriteText converts every line break to CR LF, as
// browsers do.
//
// Data (ReadTypes, Read, Write; dataformat.go has the pure mapping, tested
// on every OS, and dataobject_windows.go the calls): MIMEText is
// CF_UNICODETEXT as it is, MIMEHTML the registered "HTML Format" (CF_HTML,
// header added and parsed), MIMEPNG the registered "PNG" plus CF_DIBV5
// (Windows synthesizes CF_DIB), read back from CF_DIBV5 or CF_DIB when no
// PNG is offered; MIMEURIList is CF_HDROP (local files only); other types
// are formats registered under their MIME name. Every format is rendered
// into GlobalAlloc memory before OpenClipboard, as WriteText does.
//
// # Drag and drop and menus
//
// OLE (phase 13; dnd_windows.go, menu_windows.go, and the pure halves
// droptarget.go, dataformat.go and menu.go). The UI thread joins OLE's
// single-threaded apartment with OleInitialize at its first window (it
// replaces the dialogs' CoInitializeEx), and every window registers an
// IDropTarget. The COM objects the backend implements (IDropTarget,
// IDropSource, IDataObject) are Go objects behind syscall.NewCallback
// vtables (comserver_windows.go); the enumerator of formats is the shell's
// (SHCreateStdEnumFmtEtc).
//
//   - Target: OLE asks for the effect from inside each IDropTarget call,
//     so the DragEvent is delivered synchronously, like WM_SIZE's
//     ConfigureEvent, and the window's answer (SetDropEffect) returned;
//     during a running pass the previous answer is returned and the event
//     waits. DragOver calls with nothing changed (OLE repeats them on a
//     timer) deliver nothing. A drag of this app is recognized by its data
//     object and read as it is (FromApp). The shell's IDropTargetHelper
//     draws the source's drag image over the window. The calls into another
//     program's data object (EnumFormatEtc at DragEnter, GetData and the
//     reads of a stream at Drop, the helper's) are bounded (ADR-234;
//     stallguard.go, comcancel_windows.go): the UI thread enables COM call
//     cancellation, and a timer cancels the pending call (CoCancelCall)
//     when no call has completed for 5 s, each completed call or chunk
//     starting the wait again. A drag whose formats cannot be listed
//     offers none; a drop whose data cannot be read completes with no data
//     (the reason is kept in the backend's integ.dropErr). No other thread
//     or apartment is involved.
//   - Source: StartDrag ends the window's pointer sequence with a
//     PointerCancel and posts a message; DoDragDrop runs when it is
//     dispatched, outside the pass, and its modal loop keeps delivering.
//     The image goes through IDragSourceHelper. DragDoneEvent carries the
//     effect DoDragDrop reports.
//   - Menus: a menu bar (SetMenu; framed windows only) keeps the client
//     size by growing the window; its WM_COMMAND is a MenuEvent. The bar's
//     items get access keys (the first free letter or digit of each level,
//     an '&' in the native label; a Latin key appended in parentheses to a
//     label of a script typed through an IME, ADR-252; menu.go) and Alt,
//     F10 and Alt and an access key
//     open it (# Keyboard, ADR-234). In dark mode (SystemSettings.DarkMode,
//     without high contrast) the bar is drawn dark: the window answers the
//     undocumented WM_UAHDRAWMENU and WM_UAHDRAWMENUITEM of the themed bar
//     with user32 and gdi32 (menubar_dark_windows.go) and paints the line
//     under the bar after WM_NCPAINT and WM_NCACTIVATE; a change of the
//     theme redraws it. The drop-downs of the bar, context menus and tray
//     menus are the system's, and are dark under the same setting through
//     uxtheme.dll's unnamed exports, by ordinal (ADR-239; menutheme_windows.go):
//     SetPreferredAppMode (ForceDark, ForceLight, or the default under high
//     contrast, set at creation and at each settings change, so the menus
//     follow the backend's reading of the settings), RefreshImmersiveColorPolicyState
//     and FlushMenuThemes. uxtheme loads on Windows 10 1903 and later and
//     only when its exports exist; elsewhere the menus stay light.
//     Popup menus (TrackPopupMenuEx with TPM_RETURNCMD) start outside the
//     pass like the dialogs and answer through the loop.
//
// # Settings
//
// DarkMode is AppsUseLightTheme = 0 under
// HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize (light
// when the value is missing), ReducedMotion is SPI_GETCLIENTAREAANIMATION
// being off, DoubleClickInterval is GetDoubleClickTime (the double-click
// time of the content, ADR-192; caption double clicks are DefWindowProc's,
// with the same setting). They are read at
// creation and again at every WM_SETTINGCHANGE (broadcast to top-level
// windows only, so each window handles it and the first finds the change); a
// difference queues a SettingsEvent. The title bars follow dark mode with
// DwmSetWindowAttribute(DWMWA_USE_IMMERSIVE_DARK_MODE), attribute 20 and 19
// on Windows 10 1809 and 1903.
//
// Settings v2 (ADR-209): AccentColor is HKCU\Software\Microsoft\Windows\DWM\
// AccentColor (ABGR, what the Settings app writes for "Accent color", kept
// when the accent on title bars is off), with DwmGetColorizationColor as the
// fallback where the value does not exist; HighContrast is
// HCF_HIGHCONTRASTON of SPI_GETHIGHCONTRAST; TextScale is the percentage
// HKCU\Software\Microsoft\Accessibility\TextScaleFactor (100 to 225, absent
// is 1). WM_DWMCOLORIZATIONCOLORCHANGED and WM_SYSCOLORCHANGE re-read the
// settings like WM_SETTINGCHANGE does (the tray window too, so a tray-only
// app hears them). settingsFrom is pure and tested on every OS.
//
// # Shortcuts, secrets and single instance
//
// The app-level services of phase 13 (appservices_windows.go; the pure
// parts in hotkey.go, instance.go, secret.go).
//
//   - Global shortcuts: RegisterHotKey on the message-only window with
//     MOD_NOREPEAT. A named key maps to its virtual key by namedVK in
//     reverse; a character goes through VkKeyScanExW of the UI thread's
//     layout, taking the key whose cap shows the character (the shift state
//     it reports is not added), with letters, digits and US punctuation as
//     the fallback where the layout has none. String IDs map to hotkey IDs
//     1 to 0xBFFF. The outcome is always a ShortcutStatusEvent: Bound, Taken
//     (ERROR_HOTKEY_ALREADY_REGISTERED) or Failed (a key the layout lacks);
//     WM_HOTKEY is a ShortcutEvent press (no releases). Run unregisters all.
//     A shortcut is kept even when it failed, and WM_INPUTLANGCHANGE (the
//     thread's layout changed) binds again those declared by character
//     whose virtual key under the new layout is another one, or that had
//     none: every moving key is released first, then each is registered
//     under its new key, so two that trade keys (AZERTY's A and Q) do not
//     collide, and a status that changes is reported again (hotkeyRebind
//     decides, relayoutHotkeys plans, layoutChanged acts; ADR-234). A
//     shortcut whose key only moved stays Bound and reports nothing. Named
//     keys and shortcuts whose key did not move are not touched. A layout switched while the app is in the background reaches
//     the thread when it is next activated.
//   - Secrets: Credential Manager, generic credentials with TargetName
//     "<appID>/<name>", UserName the app ID, CRED_PERSIST_LOCAL_MACHINE.
//     They are plain advapi32 calls, safe from any goroutine.
//   - Single instance: the mutex Local\tela-<app ID> (characters outside
//     letters, digits and ".-_" replaced, with a hash when anything
//     changed). The process that creates it gives its message-only window the
//     title tela-instance-<app ID>; a later process finds the window
//     (FindWindowExW under HWND_MESSAGE, retried for 3 s), calls
//     AllowSetForegroundWindow for its process, and sends the Launch as
//     WM_COPYDATA (dwData 0x544C4101; version byte, Dir, Token, Args as
//     uvarint-prefixed strings) with SendMessageTimeoutW (5 s). The first
//     turns it into a LaunchEvent. Run closes the mutex.
//   - The trash (ADR-216; trash.go, trash_windows.go): IFileOperation's
//     DeleteItem with FOF_ALLOWUNDO, FOF_NO_UI, FOFX_RECYCLEONDELETE and
//     FOFX_ADDUNDORECORD, on a new OS thread in an STA of its own (safe
//     from any goroutine), with a Go IFileOperationProgressSink whose
//     PreDeleteItem returns E_ABORT for an item without
//     TSF_DELETE_RECYCLE_IF_POSSIBLE: the shell would delete it for good,
//     so it stays and the call is ErrUnsupported. trash.go says why not
//     SHFileOperationW.
//
// # IME
//
// IMM32 (ime.go, ime_windows.go; ADR-071), as winit, Chromium's
// IMM32Manager and Firefox's IMMHandler use it. Modern IMEs (Microsoft IME
// for Japanese, Chinese and Korean) are TSF text services; Windows bridges
// them to IMM32 for applications that are not TSF-aware (CUAS), so the
// application sees WM_IME_* messages and input contexts.
//
//   - Session. A new window has no input context (ImmAssociateContextEx
//     with none), so the IME is off without a text field. A state from
//     SetTextInput associates the thread's default context (IACE_DEFAULT);
//     nil and the password purpose remove it again, so a password is typed
//     on the keyboard layout and the IME neither shows nor learns it. The
//     other purposes and the multiline flag do not change the IME. The
//     session is active while the window has the keyboard focus and a
//     context: it ends before the FocusEvent of a focus loss and starts
//     after the one of a gain, and SetTextInput(nil) followed by a state
//     ends and restarts it. SetTextInput and ResetTextInput are recorded
//     and applied once, at the end of the delivery pass that called them.
//   - Ending a session with a composition in progress completes it in the
//     IME (ImmNotifyIME CPS_COMPLETE, as Chromium does when a field loses
//     focus) after the session is over, so its result is not delivered:
//     the app keeps its preedit as text (the platform contract), and the
//     default context, which the thread's windows share, keeps no
//     composition that would come back in the next field or at the next
//     focus. ResetTextInput cancels the composition (CPS_CANCEL) after
//     forgetting it, so the messages that follow change nothing.
//   - WM_IME_SETCONTEXT goes to DefWindowProc without
//     ISC_SHOWUICOMPOSITIONWINDOW: Tela draws the preedit inline, the IME
//     keeps its candidate windows. WM_IME_STARTCOMPOSITION never reaches
//     DefWindowProc (it would open the system composition window).
//   - WM_IME_COMPOSITION is one IMEEvent: GCS_RESULTSTR is Commit and the
//     composition string the new Preedit, both in one event when one
//     message carries both (Japanese IMEs commit a clause and compose the
//     rest). lParam 0, or a result without a composition string, ends the
//     preedit. The preedit cursor is the target clause of GCS_COMPATTR
//     (ATTR_TARGET_CONVERTED or ATTR_TARGET_NOTCONVERTED) as a highlighted
//     range, else GCS_CURSORPOS, else hidden. The message never reaches
//     DefWindowProc, which would turn the result into WM_IME_CHAR and
//     WM_CHAR messages and type it twice; WM_IME_CHAR is dropped for the
//     same reason. WM_IME_ENDCOMPOSITION with a preedit still shown is an
//     empty preedit (cancel); the Microsoft Korean IME sends its result
//     after it (crbug.com/647150), which then arrives as a Commit without a
//     composition.
//   - Keys the IME consumes arrive as VK_PROCESSKEY: a KeyEvent with Key
//     "Process" and no Text, whatever characters follow it.
//   - Placement: the cursor area in physical client pixels (rounded
//     outwards, at least 1x1) is the composition window (CFS_POINT at its
//     top-left; the window itself is hidden), the candidate window
//     (CFS_CANDIDATEPOS and, but for Chinese IMEs, CFS_EXCLUDE with the
//     caret rectangle) and, during a composition with a Chinese or Japanese
//     IME, a hidden system caret, following Chromium's MoveImeWindow. They
//     are sent when the area, the scale or the input language changes and
//     at the start of every composition. WM_IME_REQUEST answers
//     IMR_QUERYCHARPOSITION with the cursor area for every character.
//   - Surrounding text: IMR_DOCUMENTFEED gives the IME the field's text
//     (256 characters on each side of the caret, the preedit at the caret)
//     and IMR_RECONVERTSTRING the selected text to reconvert, when there is
//     a selection and no composition (Chromium's answers); the IME's
//     reconversion then starts a composition whose first preedit replaces
//     the selection.
//
// # Desktop services
//
// Backend.Desktop (services.go; the pure halves in dialog.go, shell.go and
// tray.go are tested on every OS):
//
//   - OpenURI and OpenPath: ShellExecuteW with the "open" verb, after
//     joining COM's single-threaded apartment as its documentation asks.
//     Its error codes are mapped (a missing file wraps fs.ErrNotExist).
//     A URI whose scheme is one letter is refused: ShellExecuteW would
//     take it for a drive path and run the program it names.
//   - File dialogs: IFileOpenDialog and IFileSaveDialog over COM, called
//     through their vtables (com_windows.go), FOS_PICKFOLDERS for folders,
//     FOS_FORCEFILESYSTEM so that every result has a path. Cancel
//     (HRESULT_FROM_WIN32(ERROR_CANCELLED)) is nil and "" with no error.
//     A save dialog appends the selected type's extension to a name typed
//     without one.
//   - Message boxes: MessageBoxW, owned by the parent window (modal to
//     it); without a parent it comes to the foreground.
//   - Dialogs and message boxes start outside the delivery pass that asked
//     for them, one at a time; their modal loops dispatch the backend's
//     messages, so delivery goes on while they are open, and their answers
//     come through the loop. Quit closes them.
//   - Notifications are WinRT toasts (toast_windows.go has the details,
//     ADR-235): the toast XML with the actions as buttons, shown by the
//     ToastNotifier of the app's AppUserModelID (registered per user
//     under HKCU\Software\Classes\AppUserModelId on the first toast; the
//     process's explicit AUMID set by SetAppID), Tag and Group for
//     withdrawing and replacing, agile COM delegates for Activated,
//     Dismissed and Failed whose events reach the UI thread through a
//     private message. A timeout is not NotificationClosed (the toast is
//     in the Action Center, where it can still be clicked while the app
//     runs). Without WinRT or before Windows 10 1809, and when a toast
//     fails to show, the notification is a balloon.
//   - Tray icons and balloons: Shell_NotifyIconW on a hidden window
//     of the backend (tray_windows.go has the details): version 4
//     callbacks, menus with TrackPopupMenuEx, balloons (shown as toasts on
//     Windows 10 and 11) on an icon of each notification's own, which
//     makes its callbacks unambiguous; balloons have no actions. Icons are
//     created at the small-icon size of the primary monitor's DPI and
//     added again when Explorer restarts (TaskbarCreated). They are drawn
//     again at the new size, from the images the backend keeps, and
//     replaced (NIM_MODIFY) when the tray window gets WM_DPICHANGED (the
//     new DPI is in the message), WM_DISPLAYCHANGE or WM_SETTINGCHANGE (the
//     primary monitor's DPI is asked), for tray icons and the small icon
//     of a notification alike; the executable's own icon, which the shell
//     scales, and the balloon's large icon, read once when it shows, stay
//     (ADR-234).
//
// # Accessibility
//
// UI Automation providers implemented as Go COM servers
// (uia_windows.go has the model, ADR-246): activated by the first
// WM_GETOBJECT for UiaRootObjectId, free-threaded (they answer from an
// immutable snapshot of the tree on UIA's threads, never through the UI
// thread), the root a fragment root over the window's host provider, one
// object per exposed node. Pattern calls become AccessActionEvents posted
// to the UI thread; each update raises the UIA events of what changed.
// The OS-independent rules (roles after Core-AAM, properties, patterns,
// event diffing, text units) are pure Go in uia.go and uiatext.go. The
// methods that take doubles get them through small assembly thunks
// (uiathunk_windows_*.s), since syscall callbacks take integer arguments
// only.
//
// # Transparent windows
//
// A transparent window (ADR-249; transparent_windows.go) is a WS_EX_LAYERED
// popup without frame, shadow, border, rounded corners, resize band or
// backdrop, presented with UpdateLayeredWindowIndirect from a DIB section
// instead of the retained DIB and WM_PAINT. Windows passes the pointer
// through its zero-alpha pixels; over the pixels under
// platform.InputAlpha, WM_NCHITTEST sets WS_EX_TRANSPARENT and a 16 ms
// WM_TIMER polls the cursor until it is over the input mask again or off
// the window.
package win32
