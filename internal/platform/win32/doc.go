// Package win32 is the Windows platform backend. It talks to the system DLLs
// (user32, gdi32, dwmapi, shcore, kernel32) through golang.org/x/sys/windows
// and syscall.NewCallback, with no cgo, and implements the platform contract
// documented in package platform ("Mapping onto each OS: Win32").
//
// The code that touches the OS is built only on Windows. The files without a
// build constraint (dib.go, dpi.go, msg.go, pointer.go, cursor.go,
// keyboard.go, scancode.go, vkey.go, text.go, systemsettings.go) hold the
// pure logic behind it (pixel conversion, DPI and size math, message
// decoding, the pointer state machine, the cursor table, the keyboard
// translation, line endings, the settings values), so it is tested on every
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
// Windows are opaque and alpha is ignored. The window is shown at its first
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
//   - The window has no menu, so the system keys that would enter the menu
//     loop are swallowed: WM_SYSKEYDOWN and WM_SYSKEYUP are not passed to
//     DefWindowProc, and WM_SYSCOMMAND/SC_KEYMENU is dropped unless it is
//     Alt+Space (the window menu). Alt+F4 and Alt+Space are handed to
//     DefWindowProc and the app does not see the F4 or the space. A lone Alt
//     and F10 are reported as key events; nothing else happens.
//   - Text typed by SendInput with KEYEVENTF_UNICODE (VK_PACKET: touch
//     keyboards, remote desktops) is a key with an unidentified Code, the
//     character as Key and Text. IME composition (WM_IME_*) is left to
//     DefWindowProc for now; a WM_CHAR with no key press before it is an IME
//     result and is ignored.
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
// # Settings
//
// DarkMode is AppsUseLightTheme = 0 under
// HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize (light
// when the value is missing), ReducedMotion is SPI_GETCLIENTAREAANIMATION
// being off, DoubleClickInterval is GetDoubleClickTime. They are read at
// creation and again at every WM_SETTINGCHANGE (broadcast to top-level
// windows only, so each window handles it and the first finds the change); a
// difference queues a SettingsEvent. The title bars follow dark mode with
// DwmSetWindowAttribute(DWMWA_USE_IMMERSIVE_DARK_MODE), attribute 20 and 19
// on Windows 10 1809 and 1903.
//
// # Not yet
//
// IME (WM_IME_*, IMM32) and the IME cursor area arrive with the IME work.
package win32
