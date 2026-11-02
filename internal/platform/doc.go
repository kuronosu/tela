// Package platform is the lowest layer of Tela: the contract between the
// framework and one OS windowing system (a backend). It knows windows,
// events and framebuffers; it does not know gfx. A backend receives a
// finished *image.RGBA framebuffer plus damage rectangles and shows them.
//
// Backends live in subpackages: wayland, x11, win32, cocoa (macOS, being
// built in phase 14, ADR-222) and headless, plus remote, the IPC backend of
// the dev loop (ADR-112).
// This package only defines the interfaces, the event set and a few helpers
// that every backend uses so they behave identically (sizing rule, option
// validation, framebuffer checks, damage clipping, pixel copies). It imports
// only the standard library. Choosing a backend at startup (TELA_BACKEND,
// WAYLAND_DISPLAY, DISPLAY) is done by the layer that imports the backends,
// since doing it here would create an import cycle.
//
// # Loop contract: the backend calls the app
//
// The backend owns the event loop. The app calls [Backend.Run] with a
// [Handler], and the backend calls Handler.HandleEvent for every event, on
// the UI thread, until [Backend.Quit]. There is no "poll events" or
// "drain Events()" API.
//
// This inversion is forced by two of the four platforms, and costs nothing
// on the other two:
//
//   - Cocoa: [NSApp run] owns the main loop and only ever calls back into the
//     app (delegates, NSView methods, run loop sources). A pull API would
//     need a nested run loop per pull.
//   - Win32: during an interactive move or resize, DefWindowProc runs its own
//     modal message loop and the app's GetMessageW loop is blocked until the
//     user releases the mouse. Only WndProc keeps running. With callbacks, a
//     WM_SIZE inside the modal loop becomes a ConfigureEvent followed by a
//     synchronous FrameEvent and Present, so the window repaints live during
//     the resize. A drained Events() queue would not be serviced until the
//     modal loop ends.
//   - Wayland and X11 are naturally a "read, dispatch, call the handler"
//     loop; winit (ApplicationHandler) and Gio (driver callbacks into
//     ProcessEvent) use the same model on every OS.
//
// One handler method receiving a sealed [Event] set keeps the interfaces
// stable: keyboard, text input and IME events (phases 4 and 5) are new event
// types, not new methods.
//
// The event set: ConfigureEvent, FrameEvent, CloseRequestedEvent,
// FocusEvent, WakeEvent, SettingsEvent, PointerEvent, ScrollEvent,
// KeyEvent, IMEEvent, IMESessionEvent, WindowStateEvent, DragEvent,
// DragDoneEvent, MenuEvent, and the app-scoped NotificationEvent,
// TrayEvent, ShortcutEvent, ShortcutStatusEvent and LaunchEvent.
//
// # Threading
//
//   - The UI thread is the goroutine that calls Run, locked to the OS main
//     thread (tela.Run does runtime.LockOSThread from an init function).
//     Cocoa requires the main thread; Win32 requires the thread that created
//     the windows. Linux backends do not need it but follow the same rule.
//   - All Backend, Window, Clipboard and Desktop methods must be called on
//     the UI thread, before Run (on the same goroutine) or from inside the
//     handler.
//     Exceptions, safe from any goroutine: [Backend.Wake], [Backend.Quit],
//     [Backend.Now]; the secret methods of Desktop (SetSecret, Secret,
//     DeleteSecret), Desktop.MoveToTrash and every method of
//     [Associations], which block on the OS (an unlock prompt, file and
//     registry I/O) and are meant to be called off the UI thread. Desktop() itself is called on the UI
//     thread; the value it returns may be kept for those calls.
//   - The handler is never called re-entrantly. If a method the handler calls
//     makes the OS deliver a message synchronously (Win32 SetWindowPos sends
//     WM_SIZE from inside the call), the backend queues the resulting event
//     and delivers it after the handler returns. Gio's "delivering" flag
//     solves the same problem.
//   - Other goroutines (raster workers, state setters, timers) talk to the UI
//     thread only by enqueueing work in their own structures and calling
//     Wake. Timers (long press, tooltips, cursor blink) are goroutines that
//     call Wake; the platform layer has no timer API.
//   - Panics in the handler propagate out of Run. Backends whose handler
//     calls cross foreign frames (Cocoa via purego, Win32 via
//     syscall.NewCallback) must recover in the callback, stop the native
//     loop, and re-panic from Run with the original value.
//
// # Frames and pacing (0% CPU when idle)
//
// The backend blocks in the OS until there is an input event, a Wake, or a
// requested frame that is due. It never polls on a timer while idle.
//
//   - [Window.RequestFrame] asks for one [FrameEvent]. Requests coalesce; a
//     FrameEvent consumes them all. Animations call RequestFrame again while
//     handling the FrameEvent.
//   - If the window has not drawn recently (no outstanding vsync or frame
//     callback, and no FrameEvent within about one refresh interval), the
//     FrameEvent is delivered on the next loop iteration, after the input
//     events of that iteration. Otherwise it is delivered when the display
//     is ready for a new frame. This holds even when the app skipped
//     Present: a handler that only calls RequestFrame is paced like one
//     that draws, and never makes the loop spin.
//   - A ConfigureEvent implies a frame request; that FrameEvent is delivered
//     even if pacing would delay it, because the OS is waiting for content
//     of the new size.
//   - Inside a FrameEvent the app lays out, paints, rasterizes and calls
//     [Window.Present]. Skipping Present is allowed when nothing changed.
//   - FrameEvent.Time is the frame clock (see [FrameEvent]); it is in the
//     time base of [Backend.Now], like input timestamps.
//
// # Shared loop
//
// The OS-independent part of the loop is one implementation, package
// internal/loop, used by every backend: the FIFO queue of window events, app
// events and deferred callbacks; coalesced Wake (one WakeEvent per pass) and
// Quit checks between deliveries; applying a ConfigureEvent to the window's
// Metrics before the handler sees it and forcing its frame; the frame-due
// rule above; and the deadline of the earliest timer-paced frame for the
// poll timeout. A backend keeps only its OS-specific I/O (reading the socket
// or the message queue, frame callbacks or display links) and decides when
// to run a delivery pass. internal/loop's documentation says how Win32 and
// Cocoa plug in.
//
// # Scrolling
//
// A [ScrollEvent] reports a wheel or a touchpad. Signs follow the DOM
// (positive Y scrolls toward the end of the content). Wheels report notches
// in Steps (fractions for high-resolution wheels), touchpads precise
// distances in Delta. Touchpad gestures with a reported end carry a
// [ScrollPhase]: ScrollPhaseUpdate while the fingers are on the pad and a
// final ScrollPhaseEnd event (usually with zero Delta) when they lift, so
// the layers above can start a kinetic fling; a ScrollPhaseEnd follows at
// least one ScrollPhaseUpdate (a touch that never scrolled ends nothing).
// Wayland reports it from
// wl_pointer.axis_source (finger) and wl_pointer.axis_stop, as GTK 4 does;
// Cocoa from NSEvent.phase, dropping the momentum events AppKit generates
// after the fingers lift (the layers above fling instead); X11 cannot (XInput2 smooth scrolling has no stop signal, and the
// Synaptics driver coasts by itself), so its events have ScrollPhaseNone.
//
// # Keyboard
//
// Key input arrives as [KeyEvent]s for the window that has keyboard focus.
// Every backend honors these rules; the headless backend enforces them on
// injected key events.
//
//   - Three descriptions (ADR-021, winit's model): Code is the physical key
//     ([KeyCode], a W3C "code" value), Key the logical key ([Key], a W3C
//     "key" value, reflecting every modifier but Ctrl), Text the produced
//     text (Ctrl ignored, never a control character, empty on release).
//     Linux backends map scancodes with the shared evdev table
//     (internal/evdev); every backend sets Location from Code.Location().
//     Text that no physical key produced (Windows' injected Unicode
//     input and the character of an Alt code, typed at the release of
//     Alt) is a press with an unidentified Code, the text as Key and
//     Text, followed by its release.
//   - Focus: a window gets KeyEvents only between a FocusEvent with Focused
//     true and the next one with Focused false. On Wayland keyboard focus
//     is wl_keyboard.enter/leave, which can arrive before or after the
//     xdg_toplevel activated state; the backend orders them so that the
//     FocusEvent comes first.
//   - Held keys when focus is lost: like winit (which does it on X11 and
//     Windows), the backend delivers, before the FocusEvent with Focused
//     false, a synthetic release (Synthetic true, no Text, no Mods) for
//     every key whose press it delivered and whose release it did not, in
//     press order, and stops key repeat. Tela requires this on every
//     backend, Wayland included, where the compositor only sends
//     wl_keyboard.leave.
//   - Keys already held when focus is gained produce no presses (winit
//     sends synthetic presses on X11 and Windows, which can fire shortcuts
//     such as Ctrl+W on refocus; Tela follows winit's Wayland behavior and
//     browsers instead), and their releases are dropped: every delivered
//     release follows a delivered press of the same Code (internal/keyboard
//     tracks this). Modifiers held at focus appear in the Mods of later
//     events.
//   - Repeat: the backend generates key repeat, never the app. A repeat is a
//     KeyPress with Repeat true and the Code of the held key; Key and Text
//     are recomputed for the current modifiers, and Mods is the current
//     state. Only the last pressed repeating key repeats; keys the keymap
//     marks as non-repeating (modifiers and locks: Shift, Ctrl, AltGr,
//     Caps Lock) never repeat and neither stop nor restart a repeat, so
//     holding "a" and pressing Shift goes on repeating as "A" (like GTK
//     and winit), and releasing Shift goes back to "a". Repeat stops at
//     the key's release, at the press of another repeating key (which
//     starts repeating), and when focus is lost. Wayland repeats client-side from
//     wl_keyboard.repeat_info (rate 0 disables repeat), with the repeat
//     deadline folded into the poll timeout like a paced frame (no timer
//     goroutine); X11 relies on the server's autorepeat with XKB detectable
//     auto-repeat (presses only, no fake releases in between); Win32 on
//     WM_KEYDOWN's previous-state bit; Cocoa on NSEvent.isARepeat.
//   - Mods is the modifier state after the event.
//   - Dead keys and compose sequences: the dead key's press has Key
//     KeyDead and no Text; intermediate presses have no Text; the final
//     press carries the composed text, as its Text and its Key (like
//     winit). A key that does not continue a sequence abandons it and
//     acts on its own (it types, or starts a new sequence). Where no
//     Compose table exists (Linux without a Compose file), a dead key
//     types its spacing accent ("´") as Key and Text. Input method
//     composition arrives as its own events (next section).
//
// # Text input and IME
//
// An input method (IME: fcitx5 or IBus with Mozc or Pinyin, the Windows
// IMEs, macOS input sources) turns several keys into text through a
// composition: a preedit shown in the field, underlined, until the user
// commits or cancels it. The app drives it with two Window methods and
// receives two events (ADR-068).
//
//   - [Window.SetTextInput] with a [TextInputState] says that a text field
//     of the window has focus: its caret area (logical pixels, window
//     coordinates; the IME places its candidate window next to it), its
//     [TextPurpose] and multiline flag, and optionally its committed text
//     around the caret ([SurroundingText]). nil says no text field has
//     focus and turns the IME off. The app calls it whenever one of the
//     values changes (after layout, so the caret area is current); the
//     backend keeps a copy, ignores calls with an equal state, and sends
//     the OS at most one update per loop iteration (text-input-v3 counts
//     every commit, so updates are not sent per call or per frame).
//   - Moving focus from one field to another is SetTextInput(nil) followed
//     by the new field's state, which restarts the IME session even when
//     both calls fall in the same iteration (text-input-v3 requires an
//     enable per field).
//   - [Window.ResetTextInput] says the app finished or removed the
//     composition itself (a click in the field, a programmatic value
//     change); the IME drops its own composition.
//   - [IMEEvent] is one atomic update from the IME: delete surrounding
//     text, commit text, set the new preedit, applied in that order (see
//     IMEEvent for the mapping onto internal/text/edit). An empty Preedit
//     with no Commit while a composition is in progress means the IME
//     cancelled it.
//   - [IMESessionEvent] reports that the IME session of the window started
//     or ended (focus, SetTextInput, the OS detaching the IME). After an
//     end, a composition in progress gets no more events.
//
// Offsets are UTF-8 bytes everywhere: the surrounding text's cursor and
// anchor, the preedit cursor, the deletion lengths. Backends whose OS
// counts UTF-16 code units (Win32 IMM32, macOS NSTextInputClient) convert
// with internal/utf16x; the layers above never see UTF-16.
//
// Keys and the IME. The IME sees keys before the app. Keys it consumes
// (those that build or navigate a composition) never arrive as KeyEvents;
// keys it passes through arrive as usual. Where the OS still reports a
// consumed key (Win32 sends VK_PROCESSKEY), the backend delivers it with
// Key [KeyProcess] and no Text, and the app neither inserts nor acts on
// it. While a composition is in progress (from the first IMEEvent with a
// non-empty Preedit until one without, or an IMESessionEvent with Active
// false) the app does not insert KeyEvent.Text and runs no editing
// command for keys: the IME owns them.
//
// How each backend maps the contract:
//
//   - Wayland, zwp_text_input_v3 (one text input per seat): enable, the
//     state and commit once the text input entered a window with a state;
//     disable and commit on nil or leave. Preedit, commit and deletion
//     events are latched and delivered as one IMEEvent at done. See the
//     wayland package for serials and the surrounding text window.
//   - Win32, IMM32: WM_IME_COMPOSITION with GCS_RESULTSTR is Commit and
//     with GCS_COMPSTR the Preedit (the target clause of GCS_COMPATTR as
//     the highlighted range, else GCS_CURSORPOS), both in one IMEEvent when
//     one message carries both; WM_IME_ENDCOMPOSITION after a preedit
//     without a result is an empty Preedit (cancel).
//     ImmSetCompositionWindow and ImmSetCandidateWindow take the cursor
//     area in physical client coordinates; nil and the password purpose
//     detach the input context (ImmAssociateContextEx), so the session is
//     not active in a password field; ResetTextInput is
//     ImmNotifyIME(NI_COMPOSITIONSTR, CPS_CANCEL), and a session that ends
//     during a composition completes it in the IME without delivering the
//     result. Surrounding text answers IMR_DOCUMENTFEED and
//     IMR_RECONVERTSTRING. See the win32 package.
//   - macOS, NSTextInputClient on the content view: each call is one
//     IMEEvent. setMarkedText:selectedRange:replacementRange: is a Preedit
//     (selectedRange gives the cursor, a highlighted range when not
//     empty; an empty string cancels), insertText:replacementRange: a
//     Commit with an empty Preedit, unmarkText a Commit of the marked
//     text; a replacement range that covers the preedit (or the
//     selection) becomes DeleteBefore/DeleteAfter, other ranges are
//     ignored. Key downs go through interpretKeyEvents: during a session
//     (not with Command held): a key the IME consumes is not delivered,
//     one it passes on (doCommandBySelector:) is, and one that inserts
//     its own text (insertText: without a composition) is delivered with
//     that Text and no IMEEvent. firstRectForCharacterRange: answers the
//     cursor area in screen coordinates; selectedRange, markedRange and
//     attributedSubstringForProposedRange: read the surrounding text with
//     the preedit at the caret (UTF-16). The session is focus and a
//     non-password state; a password field has none and turns on the
//     secure event input while its window is key. ResetTextInput, and a
//     session that ends during a composition, forget the preedit and then
//     discardMarkedText; a moved cursor area is
//     invalidateCharacterCoordinates, once per pass. See the cocoa
//     package.
//   - X11 has no IME yet (XIM is a task of the Linux and Windows parity
//     phase): SetTextInput and ResetTextInput do nothing.
//   - Headless records the latest state per window and lets tests inject
//     IMEEvents, checked against these rules like keys.
//
// # Window control
//
// Phase 12 (ADR-178, ADR-179) gives windows OS state. The Window methods
// Minimize, SetMaximized, SetFullscreen, SetVisible, SetAlwaysOnTop,
// SetMinSize, SetMaxSize, SetPosition, Activate and SetIcon, and the
// WindowOptions fields of the same names (plus Hidden, Frameless, Parent,
// Modal, RestoreToken), are requests. The OS may refuse or alter them
// (tiling window managers do not maximize, Wayland has no positions,
// focus stealing prevention refuses activation), so the app never assumes
// one was granted: the outcome arrives as a [WindowStateEvent] (state) and
// a [ConfigureEvent] (size), and [Window.State] is the state delivered
// last. Requests equal to the state the backend already asked for are
// free. Every rule of the loop applies: an OS that answers synchronously
// (Win32 ShowWindow sends WM_SIZE) gets its events queued and delivered
// after the handler returns. Backends report state changes with
// internal/loop's UpdateState, which coalesces them; a change of state and
// size from one OS notification queues the state first.
//
//   - Minimize: xdg_toplevel.set_minimized (Wayland cannot report or undo
//     it: State.Minimized stays false and Activate is the way back),
//     WM_CHANGE_STATE IconicState on X11, ShowWindow(SW_MINIMIZE) on Win32
//     (which keeps reporting the empty ConfigureEvent of a minimized
//     window). Minimized windows get no FrameEvents and usually lose the
//     focus.
//   - Maximize and fullscreen: xdg_toplevel.set_maximized and
//     set_fullscreen (the configure's states report them),
//     _NET_WM_STATE_MAXIMIZED_VERT/HORZ and _NET_WM_STATE_FULLSCREEN client
//     messages, ShowWindow(SW_MAXIMIZE/SW_RESTORE) and, for fullscreen, a
//     borderless window covering its monitor with the old style and
//     placement saved (winit's borderless fullscreen). Size is the size the
//     window returns to.
//   - SetVisible(false) hides a window without closing it; the loop gets
//     no FrameEvents for it (internal/loop SetHidden), requests wait, and
//     the focus goes as the OS reports. Showing it again shows the last
//     presented frame (Wayland: a new initial commit, then the retained
//     buffer), or nothing until the first Present for a window never
//     presented. Wayland unmaps by committing a null buffer and destroying
//     the xdg_toplevel and xdg_surface, made anew to show it (ADR-186),
//     X11 unmaps (withdraws) and maps, Win32 uses SW_HIDE/SW_SHOW.
//     WindowOptions.Hidden windows still get their first ConfigureEvent
//     (Wayland: from the initial commit, which maps nothing).
//   - SetAlwaysOnTop: _NET_WM_STATE_ABOVE, HWND_TOPMOST; Wayland has no
//     protocol (no-op).
//   - Size limits: xdg_toplevel.set_min_size/set_max_size, WM_NORMAL_HINTS,
//     WM_GETMINMAXINFO. The minimum wins where the maximum is below it
//     ([ClampSize]).
//   - Positions are the top-left of the content area in physical pixels
//     of the desktop (Win32 virtual screen via ClientToScreen and
//     AdjustWindowRectExForDpi; X11 root coordinates of the client
//     window, set with StaticGravity in WM_NORMAL_HINTS or compensating
//     _NET_FRAME_EXTENTS). They round-trip: a reported position given back
//     puts the window in the same place. Wayland never reports one
//     (HasPosition false) and ignores SetPosition.
//   - Activate: xdg_activation_v1 with a token from the latest input
//     serial (Wayland), a _NET_ACTIVE_WINDOW client message with the last
//     user time (X11), SetForegroundWindow after restoring a minimized
//     window (Win32; FlashWindowEx when Windows refuses). A granted
//     activation is a FocusEvent.
//   - SetIcon: the backend picks the sizes it needs with [PickIcon] and
//     converts with [IconNRGBA]: xdg_toplevel_icon_v1 buffers (without it
//     the compositor uses the .desktop file of the AppID), _NET_WM_ICON
//     with every size, WM_SETICON ICON_SMALL and ICON_BIG at the window's
//     DPI (CreateIconIndirect). It is kept across DPI changes.
//   - Frameless windows have no OS title bar or borders and report
//     State.ClientSideDecorations: xdg-decoration in client-side mode,
//     _MOTIF_WM_HINTS without decorations, WM_NCCALCSIZE returning the
//     whole window as client area while WS_THICKFRAME and WS_CAPTION stay
//     (snap, minimize animations) and DwmExtendFrameIntoClientArea keeps
//     the shadow. A Wayland window also has client-side decorations when
//     the compositor has no server-side ones (no xdg-decoration, or it
//     chose client-side); then the app draws its title bar too.
//   - Resize edges of a window with client-side decorations (not
//     maximized or fullscreen, not FixedSize) are the backend's: in a
//     band along the content edges ([ResizeEdge] on Wayland and X11: 5
//     logical pixels, corners 16 along each edge; Win32: the DPI's resize
//     frame width, answered from WM_NCHITTEST), presses start an OS resize
//     (xdg_toplevel.resize, _NET_WM_MOVERESIZE with the edge) and are not
//     delivered, and the backend shows the resize cursor there
//     ([Edge.Cursor]) over the app's SetCursor. The band takes precedence
//     over hit regions, with one exception the OS forces on Win32:
//     HitMinimize, HitMaximize and HitClose regions take the band except
//     the window's outermost pixel (one logical pixel, rounded to device
//     pixels), as the caption buttons of a native frame do, because
//     Windows 11 shows the snap layouts flyout from the hit test at the
//     pointer over the maximize button.
//
// # Window styling
//
// ADR-187 adds two requests for the look of the window's chrome, with
// WindowOptions fields of the same names ([WindowOptions].TitleBar and
// Backdrop): [Window.SetTitleBarColors] with [TitleBarColors], and
// [Window.SetBackdrop] with [Backdrop]. They are hints about decoration the
// OS draws, so they never produce events, and a backend that has no such
// thing ignores them (Wayland, X11, headless apart from recording them).
//
//   - Win32 on Windows 11: DwmSetWindowAttribute with DWMWA_CAPTION_COLOR,
//     DWMWA_TEXT_COLOR and DWMWA_BORDER_COLOR (build 22000; the border
//     also applies to frameless windows), DWMWA_SYSTEMBACKDROP_TYPE
//     (build 22621). Failures (older Windows) are ignored. The dark or
//     light title bar follows the color scheme, as before.
//   - A backdrop of Mica, MicaAlt or Acrylic also shows through the client
//     area where the app presents transparent pixels: the window extends
//     its frame into the client area (DwmExtendFrameIntoClientArea) and
//     presents the framebuffer with its alpha. Present's pixels are
//     premultiplied RGBA, so a transparent pixel is {0,0,0,0}. Without such
//     a backdrop the backend presents opaque pixels as always. A frameless
//     window may pay for the extended frame: Windows 11 may stop showing the
//     snap layouts flyout over its maximize button.
//
// # Custom title bars
//
// A window whose app draws the title bar declares it with
// [Window.SetHitRegions]: [HitRegion] rectangles in logical window
// coordinates, later ones on top ([HitTest]), sent again after every frame
// that changes them (the backend keeps a copy and ignores equal lists).
// They apply to every window, decorated or not (a drag area in a toolbar).
//
//   - A press of any button in a HitCaption region is the OS's: it is not
//     delivered as a PointerDown, and neither is the rest of that pointer
//     sequence up to the release (the OS move loop or the compositor's
//     grab owns it). A primary press starts an interactive move
//     (xdg_toplevel.move with the press serial; _NET_WM_MOVERESIZE_MOVE
//     after ungrabbing the pointer; HTCAPTION from WM_NCHITTEST). A primary
//     double click toggles maximize where that is the platform convention
//     (DefWindowProc does it on Win32; the Wayland and X11 backends detect
//     it with CaptionClicks, the double-click rule of content clicks:
//     SystemSettings.DoubleClickTime and DoubleClickSlop, ADR-192, and
//     toggle maximize themselves). A secondary click opens the window menu where one
//     exists (xdg_toplevel.show_window_menu; the system menu of
//     DefWindowProc on Win32; _GTK_SHOW_WINDOW_MENU on X11 when the window
//     manager lists it in _NET_SUPPORTED). Pointer motion without a
//     button over a caption region is delivered (hover effects work): on
//     Win32 the backend turns WM_NCMOUSEMOVE into PointerMove and reports
//     no leave when the pointer crosses between client and caption areas.
//   - Pointer events over HitMinimize, HitMaximize and HitClose regions are
//     delivered to the app as usual (presses, releases, moves): the app's
//     buttons perform the action. The regions only give the OS their
//     meaning: Win32 answers WM_NCHITTEST with HTMINBUTTON, HTMAXBUTTON
//     and HTCLOSE (Windows 11 shows snap layouts over the maximize
//     button), turns the WM_NC… mouse messages over them into pointer
//     events and does not pass the presses to DefWindowProc, which would
//     run its own button tracking. Other backends treat them as content.
//   - HitClient regions are content again, punched into earlier regions
//     (a search field inside the caption).
//
// # Parent windows and modality
//
// WindowOptions.Parent makes a window the child of another open window of
// the same backend (xdg_toplevel.set_parent, WM_TRANSIENT_FOR, the owner
// window of CreateWindowExW): it stays above its parent, follows it when
// minimized, and is placed over it by the OS. Close closes a window's
// children first, in creation order (Win32 destroys owned windows with
// their owner; every backend does the same), and no event is delivered for
// them afterwards.
//
// WindowOptions.Modal makes a child modal: while it is visible, its parent
// receives no input. Backends make the OS enforce it where they can:
// xdg_wm_dialog_v1.set_modal (xdg-dialog-v1) on Wayland, _NET_WM_STATE_MODAL
// with _NET_WM_WINDOW_TYPE_DIALOG on X11, EnableWindow(parent, FALSE) on
// Win32 (re-enabled before the modal child is hidden or destroyed, or
// Windows activates another application). Core also drops the input of a
// window with a visible modal child, so a backend that cannot enforce it
// (or a compositor without xdg-dialog-v1) is still correct; the headless
// backend records modality but delivers injected input anyway, so tests can
// check core.
//
// # Session restore
//
// The layer above saves each window's geometry under the app's own window
// key (size, maximized, fullscreen, and the position where
// State.HasPosition) and gives it back in WindowOptions at the next start.
// X11 and Win32 restore the position from that. Wayland clients cannot
// place windows; xdg-session-management-v1 lets the compositor restore
// them instead, which is what [Window.RestoreToken] carries: an opaque
// token saved with the geometry and passed back as
// WindowOptions.RestoreToken for the same window key. Rules:
//
//   - A backend that needs no token returns "" (X11, Win32, Wayland
//     without the protocol). A token may be "" until the OS answered
//     (Wayland's session id arrives with xdg_session_v1.created), so the
//     layer above reads it when it saves (state changes, close, quit).
//   - Backends ignore tokens they did not produce (another backend, a
//     changed protocol); they never fail NewWindow because of a token.
//   - Wayland: the token holds the session id and the toplevel's name in
//     it. The backend keeps one session per process, opened with the id of
//     the first token it is given (or a new one), calls restore_toplevel
//     for a window whose token belongs to that session before its initial
//     commit, and add_toplevel with a fresh unique name otherwise. When the
//     compositor restores a window, its ConfigureEvent carries the
//     restored size.
//
// # Desktop services
//
// [Backend.Desktop] returns the OS services ([Desktop]): opening URIs and
// paths in their default application, open-file, save-file and folder
// dialogs, message boxes, notifications and tray icons. Rules for every
// backend:
//
//   - UI thread only (except the secret methods, MoveToTrash and
//     Associations, see "Threading"). Arguments are checked with the shared helpers
//     ([CheckFileDialogOptions], [CheckMessageBoxOptions],
//     [CheckNotification], [CheckTrayOptions], [CheckURI]) before anything
//     else, so programmer errors panic on every platform, including where
//     the service is missing ([UnsupportedDesktop] checks too).
//   - A service the backend or the session lacks returns [ErrUnsupported]
//     (errors.Is(err, errors.ErrUnsupported) holds), synchronously. The
//     layers above handle it: package desktop draws its own message box and
//     reports a tray as unavailable.
//   - Dialogs and message boxes are asynchronous: done runs exactly once,
//     later, on the UI thread through the loop (internal/loop Post, like
//     Clipboard.ReadText), never from inside the call, unless the call
//     returned an error (then never) or Run returned first. Cancel is not
//     an error: nil paths (or "") and a nil error. The loop keeps
//     delivering events while a dialog is open (Win32: the dialog's modal
//     loop runs WndProc, which runs the shared delivery pass; the dialog is
//     never started from inside the handler, where that pass is blocked).
//   - Notifications and tray icons are identified by IDs the backend
//     allocates (from 1); the user's interactions arrive as app-scoped
//     [NotificationEvent] and [TrayEvent]. Tray icons go away when Run
//     returns. The backend never toggles a checkable menu item: the app
//     updates the menu.
//
// Linux (Wayland and X11 alike) uses D-Bus through
// internal/platform/internal/freedesktop (github.com/godbus/dbus/v5):
// org.freedesktop.portal.OpenURI (OpenURI; OpenFile with a file
// descriptor for OpenPath, folders included; xdg-open without a portal),
// org.freedesktop.portal.FileChooser (OpenFile, SaveFile, OpenFile with
// directory for folders; the Request's Response signal answers: 0 chosen,
// 1 canceled, other codes errors), with the parent window identifier
// "x11:<hex XID>" on X11 and none on Wayland (it needs an xdg-foreign-v2
// export, not done yet); no message box
// (ErrUnsupported); org.freedesktop.Notifications (the image-data hint
// for icons; ActionInvoked with "default" is NotificationActivated);
// StatusNotifierItem registered with org.kde.StatusNotifierWatcher and a
// com.canonical.dbusmenu menu (ErrUnsupported without a watcher). Win32
// uses ShellExecuteW, the common item dialogs over COM (IFileOpenDialog,
// IFileSaveDialog, FOS_PICKFOLDERS), MessageBoxW, and Shell_NotifyIconW
// for trays (menus with TrackPopupMenuEx after SetForegroundWindow, the
// documented quirk) and for notifications (NIF_INFO balloons, shown as
// toasts: no actions; a click is NotificationActivated, a timeout or
// dismissal NotificationClosed).
//
// Phase 13 (ADR-209, ADR-210) adds drag and drop, clipboard data, native
// menus, global shortcuts, system settings v2, secrets, single instance,
// the registration of schemes and file types and the trash (ADR-216), in
// the sections below.
// The rules above hold for them too: arguments are checked first with
// the shared helpers ([CheckData], [CheckMIME], [CheckSetDropEffect],
// [CheckDragOptions], [CheckMenu], [CheckPopupMenu], [CheckShortcut],
// [CheckSecret], [CheckAppID], [CheckHandlerApp], [CheckAssociation],
// [CheckTrashPath]),
// asynchronous answers go through the loop, and a missing service returns
// ErrUnsupported synchronously.
//
// # Drag and drop and the clipboard
//
// One data type, [Data], carries a drag and the clipboard: an ordered set
// of (MIME type, bytes) entries, best first. Four types are well known and
// mapped onto each OS's formats; any other MIME type is a custom type,
// carried by name (Wayland and X11 targets, Win32 formats registered with
// RegisterClipboardFormatW under the MIME type). Backends convert in both
// directions, so the app sees only MIME types:
//
//   - MIMEText ("text/plain;charset=utf-8"): Win32 CF_UNICODETEXT
//     (UTF-16, NUL-terminated; CRLF kept as is); Linux targets
//     "text/plain;charset=utf-8", UTF8_STRING, "text/plain", TEXT and
//     STRING (Latin-1) offered, any of them read ([SelectionTargets],
//     [SelectionType]).
//   - MIMEHTML ("text/html"): Win32 "HTML Format" (registered): written as
//     the fragment in a minimal document with the Version, StartHTML,
//     EndHTML, StartFragment and EndFragment header (byte offsets of the
//     UTF-8 text), read back as the fragment between StartFragment and
//     EndFragment; "text/html" on Linux.
//   - MIMEPNG ("image/png"): Win32 registered "PNG", plus CF_DIBV5 (and
//     CF_DIB by synthesis) converted from it for programs that read only
//     bitmaps; read from "PNG", else from CF_DIBV5 or CF_DIB converted to
//     PNG (image/png of the standard library); "image/png" on Linux.
//   - MIMEURIList ("text/uri-list", RFC 2483, CRLF): files as file: URIs
//     ([FileURI], [FilePath], [URIList], [ParseURIList]). Win32 CF_HDROP
//     (DROPFILES with a double-NUL-terminated UTF-16 path list; only
//     file: URIs of local paths, other URIs are dropped); Linux also
//     "x-special/gnome-copied-files" for file managers ([GnomeCopiedFiles],
//     "copy" when written), read as a URI list when it is the only one.
//
// Types are reported normalized (DragEvent.Types, Clipboard.ReadTypes):
// OS formats mapped to the well-known types, each type once at the
// position of its first format, meta targets (TARGETS, MULTIPLE,
// TIMESTAMP) and formats without a MIME name left out.
//
// The clipboard ([Clipboard]) keeps ReadText and WriteText, which text
// fields use, and adds ReadTypes, Read and Write for Data. Reads answer
// through the loop on every backend, even where the OS reads at once
// (Win32: OpenClipboard, retried while another process holds it). Write
// replaces the whole clipboard and the backend serves every entry for as
// long as it owns it: Wayland wl_data_source with SelectionTargets of
// each type and set_selection with the latest input serial, X11
// SetSelectionOwner of CLIPBOARD answering SelectionRequest (TARGETS,
// MULTIPLE, INCR for large data), Win32 EmptyClipboard and
// SetClipboardData per format (delayed rendering is not needed).
//
// Drag and drop follows HTML's model. Target side, for every window of
// every backend:
//
//   - A drag over a window is a session of [DragEvent]s: DragEnter, any
//     number of DragOver (only when the position, the modifiers or the
//     allowed effects change; backends coalesce bursts with internal/loop
//     QueueDrag), then exactly one DragLeave or DragDrop (no DragLeave
//     after a drop, whatever order the OS reports them in). A drag that
//     moves between windows of the app leaves one before entering the
//     other. While a drag is over a window, the window gets no pointer
//     events for the dragging pointer; a pointer that was over it gets a
//     PointerLeave first.
//   - Types, Allowed, Mods and FromApp are known from DragEnter on; only
//     the types, not the data, are known before the drop.
//   - The app answers with [Window.SetDropEffect]: an effect (DropNone
//     refuses; one outside Allowed counts as DropNone) and the types it
//     will read. The answer holds until changed or the session ends, and
//     a session starts refused. A release while the answer refuses ends
//     the session with DragLeave (the OS does not drop there).
//   - DragDrop carries Data, read by the backend before delivery: the
//     offered types the app named, else the well-known types offered
//     ([DropTypes]); a type the source fails to deliver within a few
//     seconds is left out (Wayland, X11; Win32's IDataObject::GetData is
//     synchronous and has no timeout, so a hung source blocks the UI
//     thread there, as it does in every Win32 app). The effect the window holds when the handler
//     returns is the outcome reported to the source; SetDropEffect(DropNone)
//     while handling the drop tells a moving source to keep its data.
//   - Answers reach the OS as each OS needs: Win32 must return the effect
//     from inside IDropTarget::DragEnter, DragOver and Drop, so the backend
//     delivers the event synchronously (internal/loop Deliver, as with
//     WM_SIZE) and returns the answer (when a pass is already running, it
//     returns the previous answer and the event waits); Wayland
//     (wl_data_offer.accept and set_actions) and X11 (XdndStatus) send the
//     answer after the handlers have run: after the delivery pass that
//     carried the event (a callback posted behind each DragOver would keep
//     QueueDrag from coalescing them), and from posted callbacks around a
//     drop.
//   - An app that accepts names the types it wants; when none of them is
//     offered, the backend still accepts the first offered type, so the
//     drop happens with empty Data (HTML drops in that case too).
//   - A drag whose Allowed holds only DropLink can only end canceled
//     where the OS has no link action (Wayland).
//
// Source side ([Window.StartDrag], [DragOptions], [DragDoneEvent]):
//
//   - StartDrag needs a pointer button pressed in the window and still
//     held (Wayland start_drag needs that press's serial; the implicit
//     grab), else ErrNoGrab; one drag at a time. The app calls it from the
//     handler of the sequence's PointerDown or PointerMove (core: when its
//     drag gesture starts on an h.Draggable element).
//   - The OS takes the pointer: the window gets a PointerCancel for it
//     before any drag event, and nothing else of that sequence.
//   - The drag may end over any window of any application, the app's own
//     included (then its windows get the target-side events with FromApp
//     set). One DragDoneEvent reports the outcome to the source window.
//   - Data and Image are copied before StartDrag returns. The image is a
//     hint (premultiplied RGBA in physical pixels, Hotspot under the
//     pointer).
//
// # Native menus
//
// [MenuItem] (shared with tray menus; IDs chosen by the app, Accelerator
// as display text) describes a window's menu bar ([Window.SetMenu]; a
// choice is a window-scoped [MenuEvent]) and popup menus
// ([Window.PopupMenu]; asynchronous like the dialogs, 0 for a dismissed
// menu). Where the OS has no native menu, both return ErrUnsupported and
// the layers above draw their own (kit's menus): Wayland and X11 always,
// frameless Win32 windows for the menu bar (their client area covers the
// frame), Cocoa until its desktop integration (whose menu bar is the
// app's NSMenu).
//
// # Global shortcuts
//
// [Desktop.RegisterShortcut] asks the OS for a system-wide [Shortcut]: an
// app-chosen stable ID, an [Accelerator] (modifiers and a logical key, as
// KeyEvent.Key reports it without Shift) and a description. The outcome
// is asynchronous on every backend: exactly one app-scoped
// [ShortcutStatusEvent] follows a request that returned nil (ShortcutBound
// with the trigger the OS bound, or Taken, Refused, Failed with a
// reason), even where the OS answers at once; later changes (the user
// rebinding or removing it) are further ShortcutStatusEvents. Presses and,
// where the OS reports them, releases are app-scoped [ShortcutEvent]s.
// Registering an ID again replaces it; shortcuts go away when Run
// returns.
//
// ShortcutBound with an empty Trigger means the desktop registered the
// shortcut without assigning keys, or without saying which:
// xdg-desktop-portal-hyprland ignores preferred_trigger and binds nothing,
// so the user adds "bind = MODS, KEY, global, <app ID>:<shortcut ID>" to
// the compositor's configuration, and the shortcut fires from then on.
//
// The app ID (ADR-217). The portal identifies the app by the ID that
// [Desktop.SetAppID] gives, which the layers above call before the first
// window with the app's own ID (backends start with [DefaultAppID], the
// executable's name, which has no desktop entry). xdg-desktop-portal 1.22
// accepts a host app's ID only when the desktop has an entry <ID>.desktop
// ([Associations.RegisterApp] writes one): RegisterShortcut then returns
// an error wrapping [ErrAppNotRegistered] synchronously, naming the entry,
// and no status follows; the X11 backend grabs the key instead, Wayland
// has no fallback. Under tela dev the child's SetAppID goes to the host's
// backend, which holds the shortcuts.
//
// # System settings
//
// [SystemSettings] reports dark mode, reduced motion, the double-click
// time, the accent color (HasAccentColor), high contrast and the text
// scale (0 unknown, TextScaleFactor treats it as 1; the layers above
// multiply rem by it). Backends read them when they start and deliver a
// [SettingsEvent] with the whole new value when one changes (an equal
// value is not reported).
//
// # Secrets
//
// [Desktop.SetSecret], Secret and DeleteSecret keep small secrets (keys,
// tokens; at most [MaxSecretSize] bytes, the Credential Manager limit) in
// the OS's secure storage, named by (app ID, name). Unlike the rest of
// Desktop they block (the OS may show an unlock prompt) and are safe from
// any goroutine. A missing name is [ErrSecretNotFound] (errors.Is with
// fs.ErrNotExist); a dismissed unlock prompt is [ErrSecretCanceled]
// (the Secret Service; Win32 has no prompt); no secure storage in the
// session is ErrUnsupported, and the layers above disable what needs it.
//
// # Trash
//
// [Desktop.MoveToTrash] moves a file or folder (an absolute path; a
// symbolic link itself, not its target) to the user's trash, where the
// user can restore it (ADR-216). It never deletes: a file the trash cannot
// take stays in place and the call returns ErrUnsupported, which the
// layers above report ("the file is still there"). A missing path wraps
// fs.ErrNotExist, a relative or empty one fs.ErrInvalid ([CheckTrash]).
// Like the secrets it blocks and is safe from any goroutine.
//
// # Single instance and launches
//
// [Desktop.ClaimInstance] makes the process the app's single instance,
// keyed by the app ID. It works before any window exists, so the layers
// above call it first and open nothing in a second process: there, the
// call forwards the process's [Launch] (arguments, working directory,
// activation token: [CurrentLaunch]) to the first process and returns
// false, and the caller exits with status 0. The first process receives
// the launch as an app-scoped [LaunchEvent] and raises a window with
// [Window.ActivateWithToken]. The claim ends when the process exits.
//
// # Registration of schemes and file types
//
// [Desktop.Associations] (and, without a running app, package
// internal/platform/assoc's Native) registers the app as a handler of URL
// schemes and file extensions ([Associations], [HandlerApp],
// [Association]) for the current user, under the owner's conditions
// (ADR-209): per user and never with administrator rights; it looks
// before writing and writes nothing when nothing changes; it never
// overwrites another application's association without
// RegisterOptions.Replace; Register makes the app a candidate, not the
// default, and becoming the default is the separate MakeDefault (Windows
// file types: the Default apps settings page); Status reports whether the
// app is registered, is the default, and which other application holds
// the association. [Associations.RegisterApp] makes the desktop know the
// app's ID with no association (ADR-217): on Linux the same desktop entry,
// with no MimeType and NoDisplay=true, which the GlobalShortcuts portal
// needs; it stays until UnregisterApp even when the last association goes,
// and UnregisterApp leaves the associations that still need it; Windows
// needs no entry and writes nothing. Package assoc documents what is
// written where; its tests run against temporary directories and an
// in-memory registry, never the real system.
//
// # Coordinates, scale and framebuffer size
//
// Everything above this layer works in logical pixels (float32). A window's
// [Metrics] are its logical [Size] and its scale (physical pixels per
// logical pixel, possibly fractional). The framebuffer is allocated in
// physical pixels, and its size is always [Metrics.Physical], that is,
// [PhysicalSize](size, scale): logical × scale per axis, rounded half away
// from zero. All backends use this one function; none computes a buffer size
// any other way. Backends with logical-native sizes (Wayland, Cocoa,
// headless) report the OS size and let the rule produce the buffer size.
// Backends with physical-native sizes (Win32, X11) report
// [LogicalSize](physical, scale), which maps back to the same physical size.
//
// Physical / scale may differ from Size by less than a physical pixel, so
// renderers must cover the whole framebuffer (clear it to the root
// background) rather than only Size × scale.
//
// Pointer positions are logical pixels relative to the top-left of the
// content area: the area Tela draws, excluding decorations drawn by the OS.
//
// # Present
//
// Present(fb, damage) shows a frame. fb is premultiplied RGBA8 in sRGB
// (Go's image.RGBA convention), physical pixels, with fb.Rect equal to
// image.Rect(0, 0, Metrics().Physical()) exactly; anything else is a
// programmer error and panics ([CheckFramebuffer]). damage lists the changed
// rectangles in fb coordinates since the previous Present to the same
// window; nil or empty means the whole frame ([ClipDamage]). Backends read
// only the damaged pixels and keep the rest from the previous frame, so the
// app may rasterize only the damage. The first Present after a
// ConfigureEvent is always full.
//
// Ownership: the backend copies what it needs before Present returns (into
// a wl_shm buffer, an XImage/SHM segment, a DIB section, a CGImage), and
// never retains fb. The caller may reuse fb for the next frame immediately.
// Because the backend keeps its own copy, it can answer OS repaint requests
// (Win32 WM_PAINT, X11 Expose) without involving the app.
//
// A window is not shown until its first Present, which avoids a flash of
// unpainted content (Wayland maps a surface only when a buffer is attached;
// the other backends delay ShowWindow / XMapWindow / orderFront until then),
// nor while it is hidden (see "Window control").
// Windows are opaque, except a Windows 11 window with a client-area
// backdrop ([AlphaPresenter], ADR-187), which shows the frame's alpha;
// other backends may ignore alpha.
//
// # Errors
//
// Platform and resource errors are returned as error: a missing display
// ([ErrUnavailable] from constructors), a lost connection (from Run), a
// failed buffer allocation (from Present). Programmer errors panic with an
// actionable message: invalid window options, a framebuffer of the wrong
// size, an invalid cursor, calling Run twice.
//
// # Mapping onto each OS
//
// Wayland (single fd). The UI thread loops: flush requests; dispatch queued
// events; deliver events to the handler; deliver due frames; then
// poll(display fd, eventfd) with no timeout. Wake writes the eventfd (an
// atomic flag avoids redundant writes); Quit sets a flag and wakes. Pacing
// uses wl_surface.frame: Present attaches the buffer, damages
// (wl_surface.damage_buffer, physical pixels), requests a frame callback and
// commits. A RequestFrame while a callback is outstanding waits for its done
// event; with none outstanding the FrameEvent is delivered on the next
// iteration if the window has not had one for an interval, otherwise the
// backend requests a frame callback with a commit that attaches no buffer
// and waits for it. Frame callbacks stop for hidden surfaces, which is the
// desired idle behavior. Scale: fractional via wp_fractional_scale_v1
// (scale = n/120 as float64) and wp_viewporter (destination = logical
// size, buffer = PhysicalSize, buffer scale 1) when the compositor has
// both; otherwise integer via wl_surface.preferred_buffer_scale (or
// wl_output.scale) and set_buffer_scale. xdg_surface.configure
// is acked right before the commit of the first buffer of the new size,
// which is the Present following the ConfigureEvent.
//
// X11 via a vendored, patched jezek/xgb (ADR-025; XInput2 and XKB requests
// are written by hand on its connection). xgb runs its own reader
// goroutine and exposes events through a blocking WaitForEvent, so the UI
// thread cannot poll the socket
// itself. A pump goroutine calls WaitForEvent and sends each event on a
// channel. The UI thread blocks in a select over that channel, a wake
// channel (buffer 1, non-blocking send from Wake), and a frame timer that is
// armed only while a frame is requested and due later (a fixed 60 Hz
// period, FrameInterval; X11 has no frame callbacks without the Present
// extension). Sizes are physical (ConfigureNotify); the scale comes
// from Xft.dpi / 96 when set, else 1; logical = LogicalSize(physical,
// scale). Present uses PutImage (MIT-SHM later), keeping a copy for Expose.
//
// Win32. GetMessageW / TranslateMessage / DispatchMessageW on the UI thread;
// WndProc translates messages into events. Wake posts a private message to a
// message-only window: PostMessageW to a window, not PostThreadMessageW,
// because thread messages are dropped while a modal loop (move/resize, menus)
// runs, a known Win32 quirk. Pacing: a frame request starts a goroutine
// that waits in DwmFlush (or sleeps out the refresh interval when it
// returns at once) and posts a private message, which is dispatched inside
// modal loops too, so animations continue during a live resize. WM_SIZE and
// WM_DPICHANGED become a ConfigureEvent followed by a synchronous FrameEvent,
// so the resized window is painted before DefWindowProc continues. WM_PAINT
// is answered from the retained copy (SetDIBitsToDevice). Sizes are physical
// (per-monitor DPI v2); scale = dpi / 96. Quit posts WM_QUIT. Implicit
// pointer capture needs SetCapture / ReleaseCapture.
//
// Cocoa via purego (ADR-222; the system bindings are the leaf package
// internal/darwin). New runs on the main thread; Run calls [NSApp run],
// which owns the loop; the backend is called back by the NSWindow delegate,
// the NSView subclass and the run loop, and calls the handler from there.
// Wake signals a CFRunLoopSource (CFRunLoopSourceSignal + CFRunLoopWakeUp,
// safe from any thread) added in kCFRunLoopCommonModes so it also fires
// during live resize (event tracking mode); its perform callback runs the
// delivery pass. A delegate callback that queues events (windowDidResize:,
// windowShouldClose:, key changes) runs a pass itself, so a live resize
// presents before AppKit continues. Pacing: a frame request starts the
// window's display link (NSView.displayLink on macOS 14+, a CADisplayLink
// on the main run loop in the common modes; on macOS 13, which Go 1.27
// still supports, a CVDisplayLink whose callback on its own thread flags
// the window and signals the same run loop source); each tick calls
// VsyncDone and runs a pass, and the link is stopped when no frame is
// requested. A Go timer that signals the run loop source paces what no
// link can (a window not on screen yet, no link at all) and stands in for
// a link that stops ticking (100 ms), and is stopped when no frame waits
// (a CFRunLoopTimer fired only every 100 ms or so on the CI runner). Sizes are points (logical); scale is backingScaleFactor. The content view hosts a
// CALayer (contentsScale = backing scale, top-left gravity); Present sets
// its contents to a CGImage made from the copied pixels (RGBA premultiplied
// is native: kCGImageAlphaPremultipliedLast; sRGB color space) inside a
// CATransaction without implicit animations. Quit calls [NSApp stop:] and
// posts an application-defined event so that run returns. Input: the
// content view's NSResponder methods; keys through the macOS half of
// internal/keyboard, with Command and Control key downs taken in
// performKeyEquivalent: and the releases AppKit swallows while Command is
// held forwarded by an NSApplication subclass (winit's approach); pointer
// enter, leave and moves from one tracking area; scrolls with AppKit's
// precise deltas or line steps and the touchpad's phases, momentum events
// dropped (the layers above fling). Implemented: windows, presentation,
// HiDPI, close requests, focus, hiding, child windows closed with their
// parent, keyboard, pointer and scroll input, cursors (cursor rectangles;
// CursorNone hides the cursor in the view), the display link, the
// clipboard with its Data types (NSPasteboard; see "Desktop integration on
// each OS") and the system settings: dark mode is the best match of
// NSApp.effectiveAppearance among Aqua and DarkAqua, the accent color
// NSColor.controlAccentColor in sRGB, reduced motion and high contrast
// NSWorkspace's accessibilityDisplayShouldReduceMotion and
// accessibilityDisplayShouldIncreaseContrast, the double-click time
// +[NSEvent doubleClickInterval], the text scale 1 (macOS has none);
// changes arrive through a registered observer (KVO of
// effectiveAppearance, NSSystemColorsDidChangeNotification,
// NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification, and
// NSApplicationDidBecomeActiveNotification for the double-click time),
// coalesced into one read per pass. The IME is NSTextInputClient on the
// content view (see "Text input and IME"). Window control, menus, drag
// and drop and the desktop services are the macOS desktop integration
// phase (validated no-ops and ErrUnsupported until then).
//
// Headless (subpackage headless). An in-memory backend with a controllable
// clock and an input injection API, used by tests, telatest and the CLI. It
// implements the same contract and also offers synchronous stepping. Its
// window manager grants every window control request deterministically,
// and its desktop services are answered by programmable responders and
// recorded for inspection (see the headless package).
//
// # Desktop integration on each OS
//
// What each backend calls for the phase-13 contract (ADR-210). Wayland and
// X11 share the D-Bus services in internal/platform/internal/freedesktop.
//
// Wayland:
//
//   - Drag and drop on wl_data_device (the clipboard's device). Target:
//     the wl_data_offer's offer events before enter give Types,
//     source_actions gives Allowed (copy and move; Wayland has no link,
//     and ask is ignored), enter and motion are DragEnter and DragOver
//     (surface-local coordinates), leave is DragLeave unless it follows a
//     drop; the answer is wl_data_offer.accept(serial, the first wanted
//     type offered, else the first offered type, or null when refused) and
//     set_actions(effect, effect) (version 3), sent after the delivery
//     pass, only when it changed; drop reads each wanted type
//     with receive and a pipe (read without blocking the loop, with a
//     timeout), then delivers DragDrop, then calls finish (if accepted)
//     and destroys the offer. Mods come from wl_keyboard.modifiers.
//     FromApp: the backend's own drag source is in progress. Source:
//     create_data_source, offer each type's SelectionTargets, set_actions
//     (Allowed), start_drag with the serial of the held press (the
//     backend keeps the serial of each pointer's latest button press) and
//     an icon surface holding the image at -Hotspot (buffer scale or
//     viewport for the window's scale); send events write the data
//     without blocking; dnd_finished reports the last action event,
//     cancelled DropNone. StartDrag queues the source window's
//     PointerCancel and PointerLeave at once and drops that window's
//     queued pointer events (compositors do not order the pointer's leave
//     against the data device's events, so the later leave is ignored); if
//     the compositor refuses the drag at once, a PointerEnter follows the
//     DragDoneEvent to give the pointer back.
//   - Clipboard data: the selection offer's types; receive per type.
//   - Menus: none (ErrUnsupported).
//   - Global shortcuts: org.freedesktop.portal.GlobalShortcuts only
//     (CreateSession, BindShortcuts with description and preferred_trigger
//     in the XDG shortcut format, "CTRL+SHIFT+k" with xkb keysym names,
//     an empty parent window; the Response's
//     shortcuts give trigger_description; Activated, Deactivated,
//     ShortcutsChanged). xdg-desktop-portal 1.19 and later first need the
//     app ID (SetAppID) registered
//     (org.freedesktop.host.portal.Registry.Register, the first call on
//     the shortcut connection, made by the first RegisterShortcut); 1.22
//     refuses an ID without a desktop entry, which is ErrAppNotRegistered
//     (no fallback). A sandboxed app (Flatpak, Snap) does not register. No
//     portal: ErrUnsupported.
//   - ActivateWithToken: xdg_activation_v1.activate(token, surface).
//
// X11:
//
//   - Drag and drop with XDND version 5. Target: every window has
//     XdndAware = 5; XdndEnter gives Types (in the message, or the source's
//     XdndTypeList when it has more than three); the first XdndPosition is
//     DragEnter (root coordinates translated to the window), later ones
//     DragOver; Allowed is the requested action, or XdndActionList when
//     the source sends XdndActionAsk; Mods from QueryPointer's mask; the
//     answer is XdndStatus (accept flag, XdndActionCopy/Move/Link, an
//     empty rectangle so every move is reported), sent after the handler;
//     XdndLeave is DragLeave; XdndDrop converts XdndSelection to each
//     wanted target with XdndDrop's timestamp (INCR for large data), then
//     delivers DragDrop, then sends XdndFinished (accepted, performed
//     action). Source: own XdndSelection, grab the pointer and the
//     keyboard (Escape cancels), find the XdndAware top-level under the
//     pointer, send XdndEnter, XdndPosition (the action
//     DefaultDropEffect picks among Allowed, never XdndActionAsk, since the
//     backend cannot ask the user; XdndActionList advertises Allowed),
//     XdndLeave and XdndDrop, answer SelectionRequest, finish at
//     XdndFinished or after a timeout; over its own windows it delivers
//     the target side directly. The image is not shown: a window under the
//     pointer would be hit by the walk that finds the target, unless the
//     Shape extension made it input-transparent (left for later).
//   - Clipboard data: CLIPBOARD with TARGETS and INCR (the text clipboard
//     of phase 5 comes with it). Like GTK and winit, the data is not handed
//     to a clipboard manager (SAVE_TARGETS) when Run returns.
//   - Menus: none (ErrUnsupported).
//   - Global shortcuts: the portal when the session has it and it accepts
//     the app ID; else (no portal, or ErrAppNotRegistered, and then for
//     every later shortcut) XGrabKey on the root window for the keycode of the key's keysym with the
//     modifier mask, once per combination of the Lock and NumLock masks;
//     KeyPress and KeyRelease (with detectable auto-repeat) on the root are
//     Pressed and Released; BadAccess is ShortcutTaken (as an event).
//   - ActivateWithToken: the token's "_TIME<n>" suffix as the timestamp of
//     _NET_ACTIVE_WINDOW, then a _NET_STARTUP_INFO "remove: ID=<token>"
//     message ending the launch feedback.
//
// Wayland and X11 (freedesktop):
//
//   - Settings: org.freedesktop.portal.Settings (ReadAll, SettingChanged):
//     org.freedesktop.appearance color-scheme (1: dark), accent-color
//     ((ddd) in 0..1, else unknown), contrast (1: high);
//     org.gnome.desktop.interface text-scaling-factor and enable-animations
//     where the portal exposes them. Wayland and X11 get dark mode this way
//     too.
//   - Secrets: the Secret Service (org.freedesktop.secrets) with the
//     encrypted session dh-ietf1024-sha256-aes128-cbc-pkcs7 (DH, HKDF and
//     AES-CBC from the standard library), items of the default collection
//     with the attributes xdg:schema "dev.telaui.Secret", app-id and name,
//     waiting for Unlock prompts. No secret service: ErrUnsupported.
//   - Single instance: RequestName of a well-known name derived from the
//     app ID (the ID itself when it is a valid bus name) with
//     DBUS_NAME_FLAG_DO_NOT_QUEUE; the primary exports a Launch method
//     (arguments, directory, token) on it; a second process calls it and
//     returns false. The token comes from XDG_ACTIVATION_TOKEN (Wayland)
//     or DESKTOP_STARTUP_ID (X11).
//   - Associations: package assoc (the app's .desktop file in
//     $XDG_DATA_HOME/applications; mimeapps.list only for MakeDefault).
//   - The app ID (SetAppID): the portal registration of the shortcuts,
//     the notifications' app_name and desktop-entry hint, the tray items'
//     Id and Title.
//   - Trash: org.freedesktop.portal.Trash.TrashFile with a descriptor of
//     the file (opened O_RDWR|O_NOFOLLOW, a folder O_PATH, as GIO does),
//     on a connection of its own; 1 means trashed. Without a portal, when
//     it answers 0 or an error, or for a path the portal cannot be handed
//     (a symbolic link, a device or a FIFO), the home trash of the
//     freedesktop Trash specification 1.0 is written in Go:
//     $XDG_DATA_HOME/Trash (else ~/.local/share/Trash) with files/ and
//     info/; the name is reserved by creating info/<name>.trashinfo with
//     O_EXCL ("[Trash Info]", Path percent-encoded, DeletionDate in local
//     time), then the file is renamed to files/<name> (renameat2 with
//     RENAME_NOREPLACE), "name.2", "name.3"… on a clash. Only a file on the
//     home trash's file system goes there: a rename, never a copy and a
//     delete; any other (another mount, whose $topdir/.Trash-$uid is not
//     used) is ErrUnsupported.
//
// Win32:
//
//   - Drag and drop over OLE: OleInitialize on the UI thread (it subsumes
//     the CoInitializeEx of the dialogs), RegisterDragDrop of an
//     IDropTarget for every window (RevokeDragDrop at close). Target:
//     DragEnter enumerates the IDataObject's formats (Types), takes
//     Allowed from *pdwEffect (DROPEFFECT_COPY, MOVE and LINK are the
//     DropEffect bits) and Mods from grfKeyState, delivers the event
//     synchronously and returns the answer; DragOver is called on a timer
//     too, so unchanged calls are answered without an event; Drop reads
//     the wanted formats with GetData, delivers DragDrop synchronously and
//     returns the final effect. FromApp: the IDataObject is the backend's
//     own. Source: DoDragDrop with an IDataObject (the Data, rendered per
//     format) and an IDropSource (Escape cancels, the release drops,
//     default cursors), started outside the handler like the dialogs (a
//     posted message; the loop keeps delivering through the modal loop's
//     WndProc); the image through IDragSourceHelper::InitializeFromBitmap
//     (SHDRAGIMAGE, ptOffset = Hotspot); DRAGDROP_S_DROP gives the
//     effect, DRAGDROP_S_CANCEL DropNone. The source window's
//     PointerCancel is delivered before DoDragDrop captures the mouse.
//   - Clipboard data: EnumClipboardFormats, GetClipboardData and
//     SetClipboardData with the formats above.
//   - Menus: a menu bar of CreateMenu/CreatePopupMenu with AppendMenuW
//     (labels with '&' doubled, the Accelerator after a tab; MF_CHECKED,
//     MF_GRAYED, MF_SEPARATOR) set with SetMenu on framed windows, the
//     window grown so the client area keeps its size
//     (AdjustWindowRectExForDpi with bMenu); WM_COMMAND from a menu is a
//     MenuEvent (command IDs are 16 bits: the backend maps MenuItemIDs).
//     Popup menus: TrackPopupMenuEx with TPM_RETURNCMD at the point
//     converted with ClientToScreen, after SetForegroundWindow, started
//     outside the handler like the dialogs.
//   - Global shortcuts: RegisterHotKey on the message-only window with
//     MOD_NOREPEAT and the virtual key of the key in the current layout
//     (VkKeyScanExW for characters), IDs 0x0000-0xBFFF mapped from the
//     string IDs; ERROR_HOTKEY_ALREADY_REGISTERED is ShortcutTaken (as an
//     event); WM_HOTKEY is ShortcutPressed (no releases).
//   - Settings: AppsUseLightTheme, the DWM AccentColor,
//     SPI_GETHIGHCONTRAST (HCF_HIGHCONTRASTON), the Accessibility
//     TextScaleFactor (percent), SPI_GETCLIENTAREAANIMATION; WM_SETTINGCHANGE
//     and WM_DWMCOLORIZATIONCOLORCHANGED report changes.
//   - Secrets: Credential Manager in advapi32 (CredWriteW, CredReadW,
//     CredDeleteW) with CRED_TYPE_GENERIC, TargetName "<appID>/<name>",
//     CRED_PERSIST_LOCAL_MACHINE; ERROR_NOT_FOUND is ErrSecretNotFound.
//   - Single instance: CreateMutexW("Local\\<derived from the app ID>"); with
//     ERROR_ALREADY_EXISTS the second process finds the first's
//     message-only window, whose title is derived from the app ID (all
//     Tela windows share one class; FindWindowExW with HWND_MESSAGE,
//     retried briefly while the first
//     starts), calls AllowSetForegroundWindow for its process and sends
//     the Launch with WM_COPYDATA (SendMessageTimeoutW). ActivateWithToken
//     ignores the token.
//   - Associations: package assoc (HKCU\Software\Classes).
//   - Trash: IFileOperation.DeleteItem with FOF_ALLOWUNDO, FOF_NO_UI,
//     FOFX_ADDUNDORECORD and FOFX_RECYCLEONDELETE, on a thread of its own
//     in a single-threaded apartment (the call is safe from any
//     goroutine), with an IFileOperationProgressSink whose PreDeleteItem
//     aborts an item without TSF_DELETE_RECYCLE_IF_POSSIBLE: the shell
//     would delete it for good (a network drive, a file larger than the
//     Recycle Bin), so the call returns ErrUnsupported instead, as
//     Chromium does. (SHFileOperationW deletes such a file silently with
//     FOF_NOCONFIRMATION, or asks the user with FOF_WANTNUKEWARNING.)
//
// Cocoa: clipboard data on NSPasteboard.generalPasteboard (phase 14),
// which copies what is written (nothing is served later; data outlives
// the app) and is read again for every request, our own data included:
// MIMEText is public.utf8-plain-text and MIMEHTML public.html, both
// through the string methods (which convert UTF-16 and legacy types);
// MIMEPNG is public.png, read from public.tiff converted to PNG when no
// PNG is offered; MIMEURIList is one pasteboard item per URI, as Finder
// writes files (public.file-url, else public.url; file reference URLs
// are resolved to paths); any other type is the UTType of the MIME type
// (+[UTType typeWithMIMEType:], declared or dynamic; a type with
// parameters gets an identifier naming it, "dev.telaui.mime." and the
// escaped type), reported through the UTType's preferred MIME type.
// Planned: NSDraggingDestination and NSDraggingSource,
// NSMenu (the app's menu bar; window menus return
// ErrUnsupported), the keychain, NSRunningApplication for single instance,
// the bundle's Info.plist for schemes and document types, NSFileManager's
// trashItemAtURL (until then MoveToTrash is ErrUnsupported).
//
// Remote (tela dev), protocol 5 (ADR-214): everything is forwarded to the
// host's backend, which owns the real windows: drags in both directions
// (the answer to a drag reaches the OS one round trip late, so a drag
// entering a window is refused until the child answered), menus, the
// clipboard's Data, global shortcuts and single instance held by the host
// across reloads, the app ID (the host's backend takes the child app's),
// secrets, associations and the trash (see the remote package).
//
// Headless: the clipboard holds Data; drags are driven by injected pointer
// events after Window.StartDrag or Backend.StartExternalDrag; menus,
// shortcuts, secrets, instances (a registry shared by the process's
// headless backends), associations (assoc.Fake) and the trash (recorded,
// and moved into a directory the test gives) are deterministic fakes with
// responders, injection and inspection (see the headless package).
package platform
