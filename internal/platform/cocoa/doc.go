// Package cocoa is the macOS platform backend. It talks to AppKit,
// QuartzCore, CoreGraphics and CoreFoundation through the Objective-C
// runtime and purego (via the leaf bindings package internal/darwin), with
// no cgo, and implements the platform contract documented in package
// platform ("Mapping onto each OS: Cocoa"). The code that touches the OS is
// built only on darwin; cursor.go (the cursor table), input.go (the
// pointer and scroll rules) and ime.go (the IME rules) are pure and tested
// on every OS.
//
// # Status (phase 14, ADR-222)
//
// Implemented: the app and its event loop, windows, presentation, HiDPI,
// keyboard, pointer and scroll input, cursors, frame pacing with a display
// link, the clipboard (NSPasteboard with every Data type,
// clipboard_darwin.go over the pure mapping in pasteboard.go), the system
// settings with their change events (settings_darwin.go) and the IME
// (NSTextInputClient, ime_darwin.go over the pure rules of ime.go). Window
// control
// (minimize, zoom, fullscreen, positions, child and modal windows enforced
// by the OS), native menus, drag and drop and the desktop services are the
// macOS desktop integration phase. Until then those requests are validated
// like on every backend and do nothing, and the services return
// ErrUnsupported.
//
// # Loop
//
// The UI thread is the process's main thread: [New] must be called there
// (tela.Run is, since package tela locks the main goroutine in an init
// function), and it locks the calling goroutine to it until Run returns.
// Run calls [NSApp run], which owns the loop. The backend is called back
// from AppKit (the window delegate and the content view), from the display
// links, and from a version 0 CFRunLoopSource on the main run loop, in
// kCFRunLoopCommonModes so that it also fires inside AppKit's tracking
// loops (live resize). Wake and Quit signal the source from any goroutine
// (CFRunLoopSourceSignal + CFRunLoopWakeUp), and so does the pacing timer:
// a Go timer set to the shared loop's pacing deadline (Pace) for a frame no
// display link paces, or to the stand-in deadline of a link that has not
// ticked, and stopped otherwise, so an idle app never wakes up. The
// source's perform callback runs a delivery pass of the shared loop
// (internal/platform/internal/loop). A CFRunLoopTimer paced the base
// backend, but on the macOS CI runner it fired about every 100 ms whatever
// its fire date was (10 Hz animations); the Go timer runs on the backend's
// own clock, so the frame is due when it fires.
//
// Frames are paced by a display link per window (link_darwin.go): a frame
// request the loop does not deliver at once starts the window's link, each
// tick records the display signal and runs a pass, and a link nothing
// waits for is stopped after the pass (no ticks while idle). The link is
// -[NSView displayLinkWithTarget:selector:] on macOS 14 and later (a
// CADisplayLink on the main run loop that follows the view's screen), and
// a CVDisplayLink on macOS 13 (its callback, on a CoreVideo thread, only
// flags the window and signals the run loop source; it follows the
// window's screen through windowDidChangeScreen:). A window not on screen
// yet is paced by the timer, and so is everything when no link can be
// made; when a link stops ticking while a frame waits (an occluded window)
// the timer stands in for it after 100 ms, so frames slow down instead of
// stopping.
//
// A delegate callback that queues events (a resize, a close request, a
// focus change) runs a delivery pass itself, so a live resize delivers its
// ConfigureEvent and the forced FrameEvent, and the app presents, before
// AppKit continues. Quit stops the app with [NSApp stop:] and posts an
// application-defined event so that run returns. A panic in the handler
// cannot unwind through the Objective-C frames above the callbacks: the
// callback recovers it, stops the app and Run panics again with the value.
// Every callback runs inside its own autorelease pool.
//
// # Windows and presentation
//
// A window is an NSWindow (titled, closable, miniaturizable and, unless
// FixedSize, resizable; borderless when Frameless) whose content view is a
// registered NSView subclass that hosts a CALayer (the layer is set before
// wantsLayer, so AppKit never draws into it; the GPU backend will give the
// view a CAMetalLayer instead and nothing else changes). The view is
// flipped, the layer's contentsScale follows backingScaleFactor, and
// its contentsGravity pins the frame to the top-left corner, pixel for
// pixel. Sizes are points (logical) and the scale is backingScaleFactor.
// Present keeps a copy of the frame (damage-only presents update it),
// copies it into a CGImage in the sRGB color space
// (kCGImageAlphaPremultipliedLast, Go's image.RGBA layout) and sets it as
// the layer's contents inside a CATransaction without implicit animations.
// The window is ordered front at its first Present. Closing a window
// releases its objects at the next pass, outside any AppKit method that
// might still use them.
//
// # Input
//
// The content view is the window's first responder and receives the
// NSResponder methods (input_darwin.go, whose top comment lists the AppKit
// quirks handled, following winit):
//
//   - Keyboard: keyDown:, keyUp: and flagsChanged: go through the macOS
//     half of internal/keyboard (virtual key codes, AppKit's characters,
//     the modifier keys from the change of modifierFlags) and the window's
//     held-key set: key events only while the window is key, synthetic
//     releases of the held keys before FocusEvent{false}, no release for a
//     key pressed before the focus came. ISO keyboards are read from
//     HIToolbox (KBGetLayoutType). Command and Control key downs are taken
//     in performKeyEquivalent:, where AppKit offers them first (some never
//     reach keyDown:), and the NSApplication subclass TelaApplication hands
//     the key window the releases of keys let go while Command is held,
//     which AppKit's sendEvent: swallows. Repeats are AppKit's (isARepeat).
//     While a text field has an IME session, key downs without Command go
//     through the input method first (next section).
//   - Pointer: the left, right and other buttons (buttonNumber 2 middle, 3
//     back, 4 forward), drags, moves and enter/leave from one tracking area
//     (active always, so hover works in inactive windows; enabled during
//     drags). Positions are points in the flipped view; a press keeps the
//     pointer (AppKit sends the drags and the release to the view that got
//     the press) and a leave during it waits for the release. The first
//     click on an inactive window reaches the app (acceptsFirstMouse:).
//     Clicks are counted by the layers above with the shared double-click
//     rule (ADR-192). A Control-click stays a primary press with ModCtrl,
//     as AppKit and Chromium's event builder report it; core makes it a
//     context click under the Mac flavor (OnContextMenu, no click).
//   - Scroll: precise deltas (touchpads, the Magic Mouse) are Delta in
//     points, other wheels Steps in lines, both with the DOM's signs (the
//     user's natural scrolling setting is already in AppKit's deltas); the
//     touchpad's gesture phases map to ScrollPhaseUpdate and End (an End
//     only after the window's gesture delivered an update: a touch that
//     never scrolled, mayBegin then cancelled, ends nothing), and the
//     momentum events AppKit adds after the fingers lift are dropped, since
//     the layers above fling from the gesture (ADR-057).
//   - Cursors: cursor rectangles, and CursorNone hides the cursor while the
//     pointer is in the view (+[NSCursor hide], balanced by unhide).
//
// # IME
//
// TelaView conforms to NSTextInputClient (declared at class registration,
// so NSView gives it an NSTextInputContext; should the runtime lack the
// protocol, the view makes its own context). The session the app hears of
// (IMESessionEvent) is the window's key status plus a non-nil text input
// state whose purpose is not a password; the protocol calls are ignored
// outside it.
//
//   - Each protocol call that changes the composition is one IMEEvent:
//     setMarkedText:selectedRange:replacementRange: the Preedit, with the
//     selected range as its cursor (empty text: the composition ends);
//     insertText:replacementRange: a Commit, ending the composition;
//     unmarkText a Commit of the marked text (NSTextView keeps it). The
//     strings are NSString or NSAttributedString and the ranges UTF-16
//     code units of the document the IME sees, the committed text with
//     the preedit at the caret; ime.go converts them to the contract's
//     UTF-8 offsets (the selected range of setMarkedText: is mapped over
//     the NULs the backend drops), turns a replacement range that covers
//     the preedit or the selection into DeleteBefore/DeleteAfter, and
//     applies its own events to a copy of the surrounding text, so several
//     calls between two states of the app read a consistent document. At
//     the end of the delivery pass that gave the app those events, the
//     copy is the app's last reported state again, also when the app
//     rejected the text and reported the same state as before.
//   - markedRange, selectedRange and attributedSubstringForProposedRange:
//     answer from that document (selectedRange is NSNotFound without
//     surrounding text, as in winit); firstRectForCharacterRange: is the
//     cursor area converted to screen coordinates for every range;
//     validAttributesForMarkedText is empty and characterIndexForPoint:
//     NSNotFound.
//   - Keys: during a session a key down without Command goes to
//     interpretKeyEvents: and its KeyEvent is decided when that returns
//     (imeInput.endKey): delivered after doCommandBySelector: (the IME
//     passed it on: Backspace, arrows, Return, also after a commit, as
//     Korean IMEs do), not delivered when the IME composed or a
//     composition was in progress (the release is then dropped too), and
//     delivered with its Text and no IMEEvent when insertText: brought
//     that same text without a composition, so typing with the US layout
//     gives the same events with and without a field. A dead key becomes
//     the marked accent and then a commit of the composed character.
//   - Calls outside a key event (a candidate clicked, the emoji picker, a
//     commit when the window resigns key) run a delivery pass.
//   - The session ends before FocusEvent{false} and starts after
//     FocusEvent{true}; SetTextInput(nil) ends it and a new state restarts
//     it. Ending it during a composition, and ResetTextInput, forget the
//     preedit first and then discardMarkedText, ignoring what the input
//     method calls back meanwhile. A cursor area change is
//     invalidateCharacterCoordinates at the end of the pass.
//   - A password field has no session (its keys never reach the input
//     method) and turns HIToolbox's secure event input on while the window
//     is key, as NSSecureTextField does. Purpose and Multiline change
//     nothing else.
//
// Not reported yet: pen and tablet fields (a pen moves the pointer as a
// mouse), and a PointerCancel when the OS takes a press away.
package cocoa
