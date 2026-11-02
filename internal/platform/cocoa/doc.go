// Package cocoa is the macOS platform backend. It talks to AppKit,
// QuartzCore, CoreGraphics and CoreFoundation through the Objective-C
// runtime and purego (via the leaf bindings package internal/darwin), with
// no cgo, and implements the platform contract documented in package
// platform ("Mapping onto each OS: Cocoa"). The code that touches the OS is
// built only on darwin; cursor.go (the cursor table), input.go (the
// pointer and scroll rules), ime.go (the IME rules), windowctl.go (the
// window control rules) and dnd.go (the drag and drop rules) are pure and
// tested on every OS.
//
// # Status (phases 14 and 15, ADR-222, ADR-224)
//
// Implemented: the app and its event loop, windows, presentation, HiDPI,
// keyboard, pointer and scroll input, cursors, frame pacing with a display
// link, the clipboard (NSPasteboard with every Data type,
// clipboard_darwin.go over the pure mapping in pasteboard.go), the system
// settings with their change events (settings_darwin.go), the IME
// (NSTextInputClient, ime_darwin.go over the pure rules of ime.go),
// window control (windowctl_darwin.go over windowctl.go: minimize, zoom,
// native full screen, the floating level, size limits, positions,
// activation, frameless windows, custom title bars, child and modal
// windows, vibrancy backdrops), drag and drop in both directions
// (dnd_darwin.go over dnd.go; see "Drag and drop" below), the desktop
// services of phase 12 with the secrets and the trash (see "Desktop
// services" below), and global shortcuts, single instance, the app ID and
// Launch Services' launches (see "Shortcuts, single instance and
// launches"). Native menus and the associations are the next tasks of
// the macOS desktop integration phase (ADR-224); until then those
// requests are validated like on every backend and do nothing, and the
// services return ErrUnsupported (UnsupportedDesktop).
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
// FixedSize, resizable; with a full-size content view and a hidden title
// bar when Frameless, see "Window control") whose content view is a plain
// layer-backed container holding TelaView, a registered NSView subclass
// that hosts a CALayer (the layer is set before wantsLayer, so AppKit
// never draws into it; the GPU backend will give the view a CAMetalLayer
// instead and nothing else changes), kept at the container's size; a
// backdrop's NSVisualEffectView goes under it in the container. The view
// is flipped, the layer's contentsScale follows backingScaleFactor, and
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
// # Window control
//
// windowctl_darwin.go, whose top comment lists the AppKit quirks handled
// (after winit's window_delegate.rs and Electron's native_window_mac.mm).
// The requests of package platform map onto AppKit, and the state comes
// back through the window delegate as WindowStateEvents (the shared
// loop's UpdateState; a notification that changes state and size queues
// the state first):
//
//   - Minimize is miniaturize: (a window never shown is miniaturized when
//     it shows) and Activate brings it back with deminiaturize:, then
//     unhides the app, activates it ([NSApp activate] on macOS 14 and
//     later, activateIgnoringOtherApps: on 13; the system may decline) and
//     makes the window key and front. Minimized windows get no frames.
//   - SetMaximized is zoom:, sent at the end of the pass when the zoom
//     differs. The zoom is tracked, not read from isZoomed, which also
//     holds for a window at its maximum size (the size limits clamp the
//     standard frame): every zoom is announced by windowShouldZoom:toFrame:
//     and ends when isZoomed reaches its side; any other resize keeps it
//     only while isZoomed holds. Maximizing a window already at its
//     standard frame only changes the state. FixedSize windows do not zoom
//     (AppKit disables their zoom button).
//   - SetFullscreen is native full screen (toggleFullScreen:, with the
//     FullScreenPrimary collection behavior; a FixedSize window is
//     resizable for its stay), applied at the end of the pass. A request
//     during a transition waits for windowDid…FullScreen: (or the Fail
//     notifications) and 300 ms of settling (a toggle sent from the Did
//     notification was lost on macOS 26), one AppKit ignores is given up
//     after a second, and transitions the user starts become the wanted
//     state. While in full screen and settling, State keeps the zoom,
//     position and minimization from before (isMiniaturized was seen true
//     around a transition) and reads them again after.
//   - SetAlwaysOnTop is the floating window level. The size limits are
//     contentMinSize/contentMaxSize (minSize/maxSize of a frameless
//     window's frame, which is its content), with the maximum raised to
//     the minimum; a window outside new limits is resized into them.
//   - Positions are points, y down from the top-left corner of the
//     primary screen (CoreGraphics' global display space; windowctl.go):
//     the top-left of the content area, rounded to whole points, so a
//     reported position given back puts the window in the same place on
//     any screen whatever its scale. HasPosition is always true.
//   - Frameless windows are titled windows with a full-size content view,
//     a transparent title bar, no title, no separator and the standard
//     buttons hidden, as Electron's: the system's shadow, rounded corners
//     and edge resizing (unless FixedSize) come with the titled style, and
//     they become key and main; State reports ClientSideDecorations. The
//     content view covers the frame, so their sizes and limits are the
//     frame's (AppKit's content-rectangle conversions still count a title
//     bar there). The title bar strip sends presses to TelaView, which
//     never moves the window by itself (mouseDownCanMoveWindow).
//   - TitleBarOverlay windows (overlay_darwin.go) are frameless windows
//     with the standard buttons shown, and also report
//     ClientSideDecorations. The buttons are where AppKit puts them, or,
//     when a HitCaption region along the top spans them, centered
//     vertically in its height with AppKit's horizontal inset (Electron's
//     trafficLightPosition): the title bar container view and the views
//     down to the buttons are made as tall as the caption and the
//     buttons' frames moved, again at every pass and delegate
//     notification, since AppKit lays the title bar out anew.
//     TitleBarControls is the buttons' union in TelaView's coordinates
//     (where they are placed), reported with the state; it is empty in
//     full screen (AppKit's auto-hiding title bar holds the buttons, and
//     AppKit's place is restored before the window enters it). The
//     buttons sit above TelaView in the theme frame, so presses on them
//     are AppKit's: the close button asks through windowShouldClose:
//     (a CloseRequestedEvent), minimize and zoom are tracked as any.
//   - Hit regions: a primary press in a HitCaption region starts
//     performWindowDragWithEvent: (the window server moves the window and
//     may keep the release), a primary double click there by the shared
//     rule (platform.CaptionClicks) performs the user's title bar action
//     (AppleActionOnDoubleClick: Maximize and Fill zoom, Minimize
//     minimizes, None does nothing; the older AppleMiniaturizeOnDoubleClick
//     when unset), and other buttons and Control-clicks there are
//     swallowed (macOS has no window menu). Neither the press nor its
//     drags and release are delivered; a move without buttons ends the
//     sequence. HitMinimize, HitMaximize, HitClose and HitClient regions
//     are content; hover is delivered everywhere.
//   - A child window is an AppKit child window (addChildWindow:ordered:
//     NSWindowAbove) while it and its parent are shown, and is detached
//     when hidden or closed: AppKit keeps it above the parent, moves it
//     with the parent and hides it with the parent's minimization. One
//     without a position opens centered on its parent. Separate windows,
//     not sheets (ADR-224).
//   - A modal child, while shown, disables its parent's standard buttons
//     (EnableWindow(parent, FALSE) on Win32), and whenever the parent
//     becomes key (a click, Command-`, Mission Control), is clicked or is
//     asked to close, the child is brought forward with the focus instead
//     (the parent reports no focus and no close request). Hiding or
//     closing it enables the buttons again and gives the focus back to the
//     parent. Core drops the parent's input as on every backend.
//   - Backdrops: Mica, MicaAlt and Acrylic put an NSVisualEffectView
//     (behind-window blending, following the window's active state) with
//     the material of platform.Backdrop under TelaView, and make the
//     window non-opaque with a clear background and TelaView non-opaque,
//     so the frame's premultiplied alpha shows the material
//     (platform.AlphaPresenter); default and none remove it and make the
//     window opaque again. SetTitleBarColors does nothing.
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
//     calls between two states of the app read a consistent document. As
//     soon as the app has handled the last queued event whose edit the
//     copy holds (the backend's handler wraps the app's), the copy is the
//     app's last reported state again, also when the app rejected the text
//     and reported the same state as before; edits that never reach the
//     app are forgotten at the end of a pass that leaves nothing queued.
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
// mouse), and a PointerCancel when the OS takes a press away (other than
// a drag's).
//
// # Drag and drop
//
// dnd_darwin.go, whose top comment lists the AppKit quirks handled (after
// Chromium's web_drag_dest_mac and web_drag_source_mac); the Data mapping
// is the clipboard's (pasteboard.go).
//
//   - Target: TelaView is the window's NSDraggingDestination, registered
//     for the well-known pasteboard types and the custom types of the
//     app's own drags (AppKit calls a view only for a drag offering one of
//     its registered types, so a drag of another app with only custom
//     types is not seen). draggingEntered: is DragEnter and
//     draggingUpdated: DragOver when the position (flipped view points),
//     the modifiers or the allowed effects changed; both deliver the event
//     synchronously (a pass inside the call; inside a running pass the
//     event waits and the previous answer is returned) and return the
//     sticky answer of SetDropEffect as an operation of the source's mask.
//     Allowed is draggingSourceOperationMask (generic counts as move);
//     Types are the dragging pasteboard's, normalized as the clipboard's.
//     draggingExited: is DragLeave; performDragOperation: reads the types
//     the app named (else the well-known ones) with the clipboard's
//     readers, delivers DragDrop and returns whether the final effect
//     accepts; nothing follows a drop. A pointer over the window leaves
//     before DragEnter, and no pointer events are delivered while the drag
//     is over it.
//   - Source: StartDrag needs a held press in the window (its NSEvent, or
//     its latest drag's, is kept for beginDraggingSessionWithItems:);
//     one drag at a time. The items are the clipboard's (one per URI, as
//     Finder writes files), the image the first item's contents at the
//     hotspot under the pointer. The window's sequence ends with a
//     PointerCancel and the rest of it is dropped; the view is the
//     NSDraggingSource, and draggingSession:endedAtPoint:operation: is the
//     DragDoneEvent. The app's own windows get the target events with
//     FromApp, and read its Data as it is.
//
// # Desktop services
//
// services_darwin.go has the overview; dialog.go and services.go are the
// pure halves, tested on every OS.
//
//   - OpenURI and OpenPath: -[NSWorkspace openURL:] (a folder opens in the
//     Finder).
//   - File dialogs: NSOpenPanel and NSSavePanel (dialog_darwin.go). With a
//     parent window on screen, a sheet on it; without one, a separate
//     modeless panel (beginWithCompletionHandler:), so the loop runs as
//     usual. Filters become allowed content types (UTType from "*.ext");
//     an open panel filters other globs with its delegate, a save panel
//     drops them; several filters get a "Format:" popup. done runs once,
//     from a later pass; cancel is nil and nil.
//   - Message boxes: NSAlert, a sheet on the parent or -[NSAlert runModal]
//     started from a run loop block outside any pass (the loop's source is
//     in the common modes, so passes run inside the modal session).
//   - Trays: NSStatusItems (tray_darwin.go) with a 1x and 2x image, the
//     tooltip, and an NSMenu, which opens on any click; without a menu a
//     click is TrayActivated.
//   - Secrets: the login keychain's generic passwords, service the app ID
//     and account the name (secret_darwin.go); any goroutine.
//   - Trash: -[NSFileManager trashItemAtURL:resultingItemURL:error:]
//     (trash_darwin.go), which never deletes; any goroutine.
//   - Notifications: UNUserNotificationCenter, only from an app bundle with
//     a bundle identifier (notify_darwin.go); without one Notify returns
//     ErrUnsupported naming tela run.
//
// # Shortcuts, single instance and launches
//
// shortcut_darwin.go and launch_darwin.go have the details and the quirks;
// shortcut.go, instance.go and instance_unix.go are the pure halves
// (instance_unix.go builds on Linux too, for its tests).
//
//   - Global shortcuts are Carbon hot keys (RegisterEventHotKey on the
//     application event target, with kEventHotKeyExclusive): no
//     permission prompt, presses and releases. Super is Command, Alt
//     Option; a named key has its fixed virtual key code, a character the
//     code of the key that types it in the current keyboard layout
//     (UCKeyTranslate over every key code, plain then with Shift, cached
//     per input source), else the ANSI layout's. The status is queued:
//     ShortcutBound with the trigger as macOS shows it ("⌃⌥⌘K"),
//     ShortcutTaken for eventHotKeyExistsErr (another app, or this one,
//     holds the combination), ShortcutFailed otherwise. One Carbon handler,
//     installed once per process, turns kEventHotKeyPressed and Released
//     into ShortcutEvents (a held hot key is one press). The hot keys are
//     unregistered when Run returns.
//   - SetAppID keeps the ID; no service needs it yet.
//   - Single instance: an exclusive flock on <TMPDIR>/tela-<app ID>.lock
//     (the per-user temporary folder; a hash of the ID when it is not a
//     plain name or the socket path would pass sun_path's 104 bytes) makes
//     the first process, which listens on tela-<app ID>.sock beside it
//     (mode 0600, a stale one removed first). A second process connects,
//     gets the first's PID, yields the activation to it
//     (yieldActivationToApplication:, macOS 14), sends its Launch and
//     waits for the ack (versioned frames, 1 MiB and 3 s at most), then
//     returns false. The forwarded launch hops to the main thread and is a
//     LaunchEvent. Run's return closes the listener (removing the socket)
//     and releases the lock.
//   - Launches: the app delegate's application:openURLs: (the URLs and
//     files Launch Services hands a bundled app) is one LaunchEvent, file
//     URLs as paths, whether or not the app claimed single instance; the
//     URLs that start the app come the same way, after the start's own
//     Launch. Launches that arrive before the first delivery pass ended
//     wait for it, so the listeners mounted with the first frame get them.
//     NSTreatUnknownArgumentsAsOpen is registered as NO, so AppKit does
//     not turn the command-line arguments into launches too.
package cocoa
