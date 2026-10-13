// Package win32 is the Windows platform backend. It talks to the system DLLs
// (user32, gdi32, dwmapi, shcore, kernel32) through golang.org/x/sys/windows
// and syscall.NewCallback, with no cgo, and implements the platform contract
// documented in package platform ("Mapping onto each OS: Win32").
//
// The code that touches the OS is built only on Windows. The files without a
// build constraint (dib.go, dpi.go, msg.go, pointer.go, cursor.go) hold the
// pure logic behind it (pixel conversion, DPI and size math, message
// decoding, the pointer state machine, the cursor table), so it is tested on
// every OS; the Windows files are exercised by the shared platformtest
// conformance suite under Wine (and on a real Windows machine).
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
// # Not yet
//
// Keyboard (KeyEvent), IME, the clipboard and system settings (dark mode,
// reduced motion) arrive with the text editing work: the message switch in
// wndproc.go is where they plug in, and Backend.Settings and
// Backend.Clipboard are their entry points.
package win32
