// Package platform is the lowest layer of Tela: the contract between the
// framework and one OS windowing system (a backend). It knows windows,
// events and framebuffers; it does not know gfx. A backend receives a
// finished *image.RGBA framebuffer plus damage rectangles and shows them.
//
// Backends live in subpackages: wayland, x11, win32, cocoa and headless,
// plus remote, the IPC backend of the dev loop (ADR-112).
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
// KeyEvent, IMEEvent, IMESessionEvent, WindowStateEvent, and the app-scoped
// NotificationEvent and TrayEvent.
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
//     [Backend.Now].
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
// the layers above can start a kinetic fling. Wayland reports it from
// wl_pointer.axis_source (finger) and wl_pointer.axis_stop, as GTK 4 does;
// X11 cannot (XInput2 smooth scrolling has no stop signal, and the
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
//   - macOS, NSTextInputClient: setMarkedText:selectedRange:replacementRange:
//     is a Preedit (selectedRange gives the cursor), insertText:
//     replacementRange: a Commit with an empty Preedit, unmarkText a Commit
//     of the marked text; a replacement range next to the cursor becomes
//     DeleteBefore/DeleteAfter. firstRectForCharacterRange: answers the
//     cursor area in screen coordinates, selectedRange and
//     attributedSubstringForProposedRange: the surrounding text;
//     ResetTextInput is discardMarkedText.
//   - X11 has no IME yet (XIM is phase 8): SetTextInput and
//     ResetTextInput do nothing.
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
//   - SetVisible(false) hides a window without destroying anything; the
//     loop gets no FrameEvents for it (internal/loop SetHidden), requests
//     wait, and the focus goes as the OS reports. Showing it again shows
//     the last presented frame (Wayland: a new initial commit, then the
//     retained buffer), or nothing until the first Present for a window
//     never presented. Wayland unmaps by committing a null buffer, X11
//     unmaps (withdraws) and maps, Win32 uses SW_HIDE/SW_SHOW.
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
//     band a few logical pixels wide along the content edges (Win32: the
//     DPI's resize frame width, answered from WM_NCHITTEST), presses start
//     an OS resize (xdg_toplevel.resize, _NET_WM_MOVERESIZE with the edge)
//     and are not delivered, and the backend shows the resize cursor there
//     over the app's SetCursor. The band takes precedence over hit regions.
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
//     it with SystemSettings.DoubleClickInterval and toggle maximize
//     themselves). A secondary click opens the window menu where one
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
//   - UI thread only. Arguments are checked with the shared helpers
//     ([CheckFileDialogOptions], [CheckMessageBoxOptions],
//     [CheckNotification], [CheckTrayOptions], [CheckURI]) before anything
//     else, so programmer errors panic on every platform, including where
//     the service is missing ([UnsupportedDesktop] checks too).
//   - A service the backend or the session lacks returns [ErrUnsupported]
//     (errors.Is(err, errors.ErrUnsupported) holds), synchronously. The
//     layers above handle it: core draws its own message box, desktop
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
// descriptor and OpenDirectory for OpenPath),
// org.freedesktop.portal.FileChooser (OpenFile, SaveFile, OpenFile with
// directory for folders; the Request's Response signal answers: 0 chosen,
// 1 canceled, other codes errors), with the parent window identifier
// "wayland:<handle>" (xdg-foreign-v2 export) or "x11:<hex XID>"; no message box
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
// Windows are opaque in phase 1; backends may ignore alpha.
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
// armed only while a frame is requested and due later (the timer period
// follows the monitor refresh rate; X11 has no frame callbacks without the
// Present extension). Sizes are physical (ConfigureNotify); the scale comes
// from Xft.dpi / 96 when set, else 1; logical = LogicalSize(physical,
// scale). Present uses PutImage (MIT-SHM later), keeping a copy for Expose.
//
// Win32. GetMessageW / TranslateMessage / DispatchMessageW on the UI thread;
// WndProc translates messages into events. Wake posts a private message to a
// message-only window: PostMessageW to a window, not PostThreadMessageW,
// because thread messages are dropped while a modal loop (move/resize, menus)
// runs, a known Win32 quirk. Pacing: a frame request arms a timer or a
// DwmFlush-paced post of a private message; both are dispatched inside
// modal loops too, so animations continue during a live resize. WM_SIZE and
// WM_DPICHANGED become a ConfigureEvent followed by a synchronous FrameEvent,
// so the resized window is painted before DefWindowProc continues. WM_PAINT
// is answered from the retained copy (SetDIBitsToDevice). Sizes are physical
// (per-monitor DPI v2); scale = dpi / 96. Quit posts WM_QUIT. Implicit
// pointer capture needs SetCapture / ReleaseCapture.
//
// Cocoa via purego. Run calls [NSApp run], which owns the loop; the backend
// is called back by NSApplication/NSWindow delegates and the NSView
// subclass, and calls the handler from there. Wake signals a CFRunLoopSource
// (CFRunLoopSourceSignal + CFRunLoopWakeUp, safe from any thread) added in
// kCFRunLoopCommonModes so it also fires during live resize (event tracking
// mode). Pacing: a frame request starts a display link (CVDisplayLink on
// its own thread signalling the same run loop source, or NSView.displayLink
// on macOS 14+); the link is stopped when no frame is requested. Sizes are
// points (logical); scale is backingScaleFactor. Present sets the layer's
// contents to a CGImage made from the copied pixels (RGBA premultiplied is
// native: kCGImageAlphaPremultipliedLast). Quit calls [NSApp stop:] and posts
// a dummy event so that run returns.
//
// Headless (subpackage headless). An in-memory backend with a controllable
// clock and an input injection API, used by tests, telatest and the CLI. It
// implements the same contract and also offers synchronous stepping. Its
// window manager grants every window control request deterministically,
// and its desktop services are answered by programmable responders and
// recorded for inspection (see the headless package).
package platform
