//go:build linux

package freedesktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// Options configures a Desktop. Wake, Post and Event are required.
type Options struct {
	// AppID identifies the app to the services: the notifications' app
	// name and desktop-entry hint, the tray items' Id and Title. ""
	// means platform.DefaultAppID().
	AppID string
	// Wake unblocks the backend's wait so that its loop calls Dispatch.
	// It is called from godbus goroutines, must not block, and may
	// coalesce.
	Wake func()
	// Post queues a callback to run on the UI thread from the loop's next
	// delivery pass (internal/loop Loop.Post). Called from Dispatch only.
	Post func(func())
	// Event queues an app-scoped event (NotificationEvent, TrayEvent) for
	// the handler (internal/loop Loop.Enqueue with a nil window). Called
	// from Dispatch only.
	Event func(platform.Event)
	// Parent returns the XDG Desktop Portal parent window identifier of a
	// window: "x11:<hex XID>", "wayland:<exported handle>", or "" when
	// there is none. nil means "" for every window.
	Parent func(platform.Window) string
	// Address is the D-Bus address of the session bus. "" means the
	// session's: $DBUS_SESSION_BUS_ADDRESS, else $XDG_RUNTIME_DIR/bus (the
	// bus is never autolaunched). Tests point it at a private bus.
	Address string
	// Opener is the command OpenURI and OpenPath run with the target as
	// its only argument when there is no portal. "" means xdg-open.
	Opener string
}

// D-Bus names used by the services.
const (
	portalName = "org.freedesktop.portal.Desktop"
	portalPath = dbus.ObjectPath("/org/freedesktop/portal/desktop")

	ifaceOpenURI     = "org.freedesktop.portal.OpenURI"
	ifaceFileChooser = "org.freedesktop.portal.FileChooser"
	ifaceRequest     = "org.freedesktop.portal.Request"

	notificationsName  = "org.freedesktop.Notifications"
	notificationsPath  = dbus.ObjectPath("/org/freedesktop/Notifications")
	notificationsIface = "org.freedesktop.Notifications"

	watcherName  = "org.kde.StatusNotifierWatcher"
	watcherPath  = dbus.ObjectPath("/StatusNotifierWatcher")
	watcherIface = "org.kde.StatusNotifierWatcher"

	busIface = "org.freedesktop.DBus"
)

// callTimeout bounds the synchronous calls made on the UI thread
// (presence checks, adding match rules, reading a portal version). The
// first portal call may start the portal through D-Bus activation, which
// takes longer than a bus daemon round trip.
const callTimeout = 5 * time.Second

// Desktop is the platform.Desktop of the Linux backends. Create it with
// New; the backend calls Dispatch on every loop iteration and Close when
// Run returns.
type Desktop struct {
	opts Options

	// UI thread only.
	conn           *dbus.Conn        // the session bus; nil until first use
	activatable    map[string]bool   // ListActivatableNames, read once per connection
	versions       map[string]uint32 // portal interface versions (0: missing)
	caps           map[string]bool   // notification server capabilities; nil until read
	notifyMatched  bool              // the Notifications match rule is installed
	watcherMatched bool              // the watcher NameOwnerChanged rule is installed
	tokens         uint64            // portal request handle tokens
	lastNote       platform.NotificationID
	lastTray       platform.TrayID

	// mu guards what godbus goroutines touch too.
	mu       sync.Mutex
	closed   bool
	inbox    []item
	requests map[dbus.ObjectPath]*request
	notes    map[platform.NotificationID]*note
	byServer map[uint32]platform.NotificationID
	trays    map[platform.TrayID]*tray
	token    string // the latest activation token
}

// item is a unit of work for the UI thread: a callback or an app-scoped
// event.
type item struct {
	fn func()
	ev platform.Event
}

var _ platform.Desktop = (*Desktop)(nil)

// New returns the desktop services. It does no I/O: the session bus is
// connected on the first call that needs it.
func New(opts Options) *Desktop {
	if opts.Wake == nil || opts.Post == nil || opts.Event == nil {
		panic("freedesktop: Options.Wake, Post and Event are required")
	}
	if opts.AppID == "" {
		opts.AppID = platform.DefaultAppID()
	}
	return &Desktop{
		opts:     opts,
		requests: map[dbus.ObjectPath]*request{},
		notes:    map[platform.NotificationID]*note{},
		byServer: map[uint32]platform.NotificationID{},
		trays:    map[platform.TrayID]*tray{},
	}
}

// push queues work for the UI thread and wakes the loop. Safe from any
// goroutine; dropped after Close.
func (d *Desktop) push(it item) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.inbox = append(d.inbox, it)
	d.mu.Unlock()
	d.opts.Wake()
}

// Dispatch moves the answers and events that arrived since the last call to
// the loop (Options.Post and Options.Event), in arrival order. UI thread;
// the backend calls it on every iteration before its delivery pass.
func (d *Desktop) Dispatch() {
	d.mu.Lock()
	items := d.inbox
	d.inbox = nil
	d.mu.Unlock()
	for _, it := range items {
		if it.fn != nil {
			d.opts.Post(it.fn)
		} else {
			d.opts.Event(it.ev)
		}
	}
}

// Close removes the tray icons and closes the D-Bus connections. Pending
// dialog callbacks are never called. Later calls return
// platform.ErrUnsupported. UI thread; idempotent.
func (d *Desktop) Close() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	d.inbox = nil
	clear(d.requests)
	trays := d.trays
	d.trays = map[platform.TrayID]*tray{}
	d.mu.Unlock()
	for _, t := range trays {
		t.close()
	}
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
}

// TakeActivationToken returns the XDG activation token the notification
// server or the tray host sent last (with an action, or with an
// activation), and forgets it; "" if there is none. A Wayland backend can
// use it to activate a window in answer to the event. UI thread.
func (d *Desktop) TakeActivationToken() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	t := d.token
	d.token = ""
	return t
}

func (d *Desktop) setActivationToken(t string) {
	d.mu.Lock()
	d.token = t
	d.mu.Unlock()
}

func (d *Desktop) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

var errClosed = fmt.Errorf("freedesktop: the event loop has ended: %w", platform.ErrUnsupported)

// address returns the session bus address, or "" when there is none.
func (d *Desktop) address() string {
	if d.opts.Address != "" {
		return d.opts.Address
	}
	return sessionAddress()
}

// sessionAddress finds the session bus like godbus does, without its
// autolaunch fallback (which runs dbus-launch and starts a bus) and without
// setting the environment.
func sessionAddress() string {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" && a != "autolaunch:" {
		return a
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	p := filepath.Join(dir, "bus")
	if fi, err := os.Stat(p); err == nil && fi.Mode()&os.ModeSocket != 0 {
		return "unix:path=" + dbus.EscapeBusAddressValue(p)
	}
	return ""
}

// dial opens a new connection to the session bus (the main one or a
// tray's). A missing bus is ErrUnsupported.
func (d *Desktop) dial() (*dbus.Conn, error) {
	addr := d.address()
	if addr == "" {
		return nil, fmt.Errorf("freedesktop: no D-Bus session bus (DBUS_SESSION_BUS_ADDRESS is not set): %w", platform.ErrUnsupported)
	}
	conn, err := dbus.Connect(addr)
	if err != nil {
		return nil, fmt.Errorf("freedesktop: cannot connect to the D-Bus session bus (%v): %w", err, platform.ErrUnsupported)
	}
	return conn, nil
}

// bus returns the main connection, connecting on first use (and again if
// the bus went away). UI thread.
func (d *Desktop) bus() (*dbus.Conn, error) {
	if d.isClosed() {
		return nil, errClosed
	}
	if d.conn != nil && d.conn.Connected() {
		return d.conn, nil
	}
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
	conn, err := d.dial()
	if err != nil {
		return nil, err
	}
	d.conn = conn
	d.activatable, d.versions, d.caps = nil, nil, nil
	d.notifyMatched, d.watcherMatched = false, false
	ch := make(chan *dbus.Signal, 64)
	conn.Signal(ch)
	go d.signals(conn, ch)
	return conn, nil
}

// signals handles the signals of the main connection until it closes
// (godbus closes ch then).
func (d *Desktop) signals(conn *dbus.Conn, ch chan *dbus.Signal) {
	for s := range ch {
		switch s.Name {
		case ifaceRequest + ".Response":
			d.onResponse(conn, s)
		case notificationsIface + ".ActionInvoked":
			d.onAction(s)
		case notificationsIface + ".NotificationClosed":
			d.onClosed(s)
		case notificationsIface + ".ActivationToken":
			d.onActivationToken(s)
		case busIface + ".NameOwnerChanged":
			d.onNameOwnerChanged(s)
		}
	}
}

// has reports whether a service is on the bus or can be started by it.
// UI thread.
func (d *Desktop) has(conn *dbus.Conn, name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var owned bool
	if err := conn.BusObject().CallWithContext(ctx, busIface+".NameHasOwner", 0, name).Store(&owned); err == nil && owned {
		return true
	}
	if d.activatable == nil {
		var names []string
		if err := conn.BusObject().CallWithContext(ctx, busIface+".ListActivatableNames", 0).Store(&names); err != nil {
			return false // not cached: retried next time
		}
		d.activatable = map[string]bool{}
		for _, n := range names {
			d.activatable[n] = true
		}
	}
	return d.activatable[name]
}

// portal returns the main connection and the version of the portal
// interface iface, or an ErrUnsupported error when the portal or the
// interface is missing. A version is read once per connection. UI thread.
func (d *Desktop) portal(iface string) (*dbus.Conn, uint32, error) {
	conn, err := d.bus()
	if err != nil {
		return nil, 0, err
	}
	v, ok := d.versions[iface]
	if !ok {
		if !d.has(conn, portalName) {
			return nil, 0, fmt.Errorf("freedesktop: no XDG desktop portal (%s) on the session bus: %w", portalName, platform.ErrUnsupported)
		}
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		var variant dbus.Variant
		err := conn.Object(portalName, portalPath).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, iface, "version").Store(&variant)
		var derr dbus.Error
		switch {
		case err == nil:
			v, _ = variant.Value().(uint32)
		case errors.As(err, &derr) && missingInterface[derr.Name]:
			v = 0 // the portal answered: it has no such interface
		default: // no answer, or the portal is not running after all
			return nil, 0, fmt.Errorf("freedesktop: the XDG desktop portal does not answer (%v): %w", err, platform.ErrUnsupported)
		}
		if d.versions == nil {
			d.versions = map[string]uint32{}
		}
		d.versions[iface] = v
	}
	if v == 0 {
		return nil, 0, fmt.Errorf("freedesktop: the XDG desktop portal has no %s interface (install a portal backend that provides it, such as xdg-desktop-portal-gtk): %w", iface, platform.ErrUnsupported)
	}
	return conn, v, nil
}

// missingInterface lists the errors with which a portal answers a version
// read for an interface it does not implement (xdg-desktop-portal exports
// only the interfaces some installed backend provides).
var missingInterface = map[string]bool{
	"org.freedesktop.DBus.Error.UnknownInterface": true,
	"org.freedesktop.DBus.Error.UnknownProperty":  true,
	"org.freedesktop.DBus.Error.InvalidArgs":      true,
	"org.freedesktop.DBus.Error.UnknownMethod":    true,
	"org.freedesktop.DBus.Error.UnknownObject":    true,
}

// addMatch installs a match rule on conn, with a timeout.
func addMatch(conn *dbus.Conn, opts ...dbus.MatchOption) error {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	return conn.AddMatchSignalContext(ctx, opts...)
}

// parentID returns the portal parent window identifier of w.
func (d *Desktop) parentID(w platform.Window) string {
	if w == nil || d.opts.Parent == nil {
		return ""
	}
	return d.opts.Parent(w)
}

// unsupported returns an ErrUnsupported error with a message.
func unsupported(format string, args ...any) error {
	return fmt.Errorf("freedesktop: "+format+": %w", append(args, platform.ErrUnsupported)...)
}

// MessageBox checks its arguments and returns ErrUnsupported: the portal
// has no message box, so core draws one.
func (d *Desktop) MessageBox(opts platform.MessageBoxOptions, done func(platform.MessageAnswer, error)) error {
	return platform.UnsupportedDesktop{}.MessageBox(opts, done)
}
