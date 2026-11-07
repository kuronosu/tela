//go:build linux

package freedesktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
)

// Global shortcuts (ADR-209, ADR-210) through
// org.freedesktop.portal.GlobalShortcuts.
//
// The portal call sequence runs on a worker goroutine with its own
// connection, so the UI thread never waits for a dialog:
//
//  1. org.freedesktop.host.portal.Registry.Register(app ID, {}), the
//     first call on the connection, made by the first RegisterShortcut
//     on the UI thread (a refusal is its error, so the X11 backend can
//     grab the key instead without a Failed status first) and again by
//     the worker when it dials a new connection. xdg-desktop-portal 1.19
//     and later identify a host (unsandboxed) app only this way, and
//     refuse it once another portal call looked the caller up; 1.22
//     refuses an ID with no desktop entry ("Could not register app ID:
//     App info not found for '<id>'"), and then CreateSession fails ("An
//     app id is required"). Any refusal is ErrAppNotRegistered, naming
//     the entry to write (ADR-217), and the connection is closed so a
//     later call can try again. The app ID is SetAppID's (appid.go).
//     Older portals answer UnknownMethod, which is fine; a sandboxed app
//     (Flatpak, Snap) is identified by its sandbox and does not
//     register.
//  2. CreateSession, then BindShortcuts with every shortcut (description,
//     preferred_trigger in the XDG shortcuts format: "CTRL+SHIFT+k", the
//     key an xkb keysym name). Each is a Request whose Response signal
//     answers, subscribed to before the call at the predictable handle
//     path.
//  3. The Response's shortcuts, with their trigger_description, are
//     ShortcutBound; a requested shortcut missing from it is
//     ShortcutRefused, and so is every shortcut when the user cancels the
//     dialog (response 1); other responses and errors are ShortcutFailed.
//  4. Activated and Deactivated of the session are ShortcutEvents (pressed
//     and released, stamped with Options.Now); ShortcutsChanged and the
//     session's Closed signal report the bindings the user changed
//     (ShortcutBound with the new trigger, ShortcutRevoked).
//
// Requests made in the same loop iteration go out as one batch. A batch
// that changes the set binds the whole set in a new session and closes the
// old one: version 1 of the portal has no call that removes a shortcut,
// and backends differ in what a second BindShortcuts on a session does
// (KDE replaces the set, xdg-desktop-portal-hyprland adds to it). Portals
// remember what the user approved per app ID and shortcut ID, so binding
// the same shortcuts again does not ask the user again on GNOME or KDE.
// Unregistering the last shortcut only closes the session.

const (
	ifaceGlobalShortcuts = "org.freedesktop.portal.GlobalShortcuts"
	ifaceSession         = "org.freedesktop.portal.Session"
	ifaceRegistry        = "org.freedesktop.host.portal.Registry"
)

// shortcutService is the global shortcut state of a Desktop.
type shortcutService struct {
	dialMu sync.Mutex // one dial at a time (UI thread and worker)

	mu      sync.Mutex
	started bool          // the worker runs
	closed  bool          // Close was called
	ops     []shortcutOp  // requests the worker has not taken yet
	kick    chan struct{} // wakes the worker (buffer 1)
	done    chan struct{} // closed by Close
	conn    *dbus.Conn    // the worker's connection; nil until dialed
	connID  string        // the app ID conn registered
	tokens  uint64        // request and session handle tokens
	// preparing: kickShortcuts waits for the parent identifier, which
	// BindShortcuts then takes from parent.
	preparing bool
	parent    string

	waiters map[dbus.ObjectPath]chan portalResponse // requests waiting for their Response
	session dbus.ObjectPath                         // the current session; "" when none
	set     []platform.Shortcut                     // the shortcuts bound in it (or being bound)
	state   map[string]shortcutState                // the last status given to the app, per ID
}

type shortcutOp struct {
	register bool
	rebind   bool              // the app ID changed: bind the set again
	s        platform.Shortcut // register
	id       string            // unregister
}

type shortcutState struct {
	status  platform.ShortcutStatus
	trigger string
}

type portalResponse struct {
	code    uint32
	results map[string]dbus.Variant
}

// portalShortcut is a GlobalShortcuts shortcut: (sa{sv}), an ID and its
// properties.
type portalShortcut struct {
	ID    string
	Props map[string]dbus.Variant
}

// RegisterShortcut asks the GlobalShortcuts portal for s. It returns an
// ErrUnsupported error, synchronously, when the session has no such
// portal, and an ErrAppNotRegistered one when the portal refuses the app
// ID (the X11 backend then grabs the key itself); otherwise the outcome
// arrives as a ShortcutStatusEvent through the loop. The first call
// registers the app ID on the shortcut connection, a round trip on the
// UI thread like the portal check before it.
func (d *Desktop) RegisterShortcut(s platform.Shortcut) error {
	platform.CheckShortcut(s)
	if _, ok := shortcutTrigger(s.Accelerator); !ok {
		return fmt.Errorf("freedesktop: RegisterShortcut %q: the key %q has no keysym", s.ID, s.Accelerator.Key)
	}
	if d.isClosed() {
		return errClosed
	}
	if _, _, err := d.portal(ifaceGlobalShortcuts); err != nil {
		return err
	}
	// The app ID before anything is promised: a refusal is the caller's
	// (no status event follows).
	if _, err := d.shortcutConn(); err != nil {
		return err
	}
	d.shortcutOp(shortcutOp{register: true, s: s})
	return nil
}

// UnregisterShortcut removes a shortcut: the others are bound again in a
// new session, or the session is closed when none is left. Unknown IDs
// are ignored. Before any RegisterShortcut it returns ErrUnsupported when
// the session has no GlobalShortcuts portal, like RegisterShortcut.
func (d *Desktop) UnregisterShortcut(id string) error {
	platform.CheckShortcutID("UnregisterShortcut", id)
	if d.isClosed() {
		return errClosed
	}
	sc := &d.shortcuts
	sc.mu.Lock()
	started := sc.started
	sc.mu.Unlock()
	if !started {
		_, _, err := d.portal(ifaceGlobalShortcuts)
		return err
	}
	d.shortcutOp(shortcutOp{id: id})
	return nil
}

// shortcutOp queues a request for the worker, starting it on first use.
// The worker takes the requests at the loop's next Dispatch
// (kickShortcuts), so the requests of one handler call (a mount that
// registers several shortcuts) make one batch: one session and, where the
// portal asks the user, one dialog.
func (d *Desktop) shortcutOp(op shortcutOp) {
	sc := &d.shortcuts
	sc.mu.Lock()
	if !sc.started {
		sc.started = true
		sc.kick = make(chan struct{}, 1)
		sc.done = make(chan struct{})
		sc.initMaps()
		go d.shortcutWorker()
	}
	sc.ops = append(sc.ops, op)
	sc.mu.Unlock()
	d.opts.Wake() // the loop runs Dispatch again soon
}

// kickShortcuts hands the queued shortcut requests to the worker, once
// the parent window of their dialog is known: the app's focused window
// (Options.Focused), whose identifier may take a round trip (Wayland
// exports it). Requests queued meanwhile join the batch. UI thread, from
// Dispatch.
func (d *Desktop) kickShortcuts() {
	sc := &d.shortcuts
	sc.mu.Lock()
	queued := len(sc.ops) > 0 && !sc.preparing
	sc.preparing = queued
	sc.mu.Unlock()
	if !queued {
		return
	}
	var w platform.Window
	if d.opts.Focused != nil {
		w = d.opts.Focused()
	}
	d.prepare(w, false, func(parent, _ string) {
		sc.mu.Lock()
		sc.preparing, sc.parent = false, parent
		sc.mu.Unlock()
		select {
		case sc.kick <- struct{}{}:
		default:
		}
	})
}

// closeShortcuts stops the worker and closes its connection, which ends
// the session (the portal closes the sessions of a peer that leaves).
// UI thread, from Close.
func (d *Desktop) closeShortcuts() {
	sc := &d.shortcuts
	sc.mu.Lock()
	if sc.closed {
		sc.mu.Unlock()
		return
	}
	sc.closed = true
	if sc.started {
		close(sc.done)
	}
	conn := sc.conn
	sc.conn = nil
	sc.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
}

// shortcutTrigger formats an accelerator in the XDG shortcuts format:
// the modifiers CTRL, ALT, SHIFT and LOGO, then the xkb keysym name of
// the key, joined by '+' ("CTRL+SHIFT+k", "LOGO+F1", "ALT+space").
func shortcutTrigger(a platform.Accelerator) (string, bool) {
	ks, ok := xkb.AcceleratorKeysym(a.Key)
	if !ok {
		return "", false
	}
	var parts []string
	for _, m := range [...]struct {
		m platform.Modifiers
		n string
	}{{platform.ModCtrl, "CTRL"}, {platform.ModAlt, "ALT"}, {platform.ModShift, "SHIFT"}, {platform.ModSuper, "LOGO"}} {
		if a.Mods&m.m != 0 {
			parts = append(parts, m.n)
		}
	}
	return strings.Join(append(parts, ks.String()), "+"), true
}

// shortcutWorker runs the portal requests, one batch at a time, until
// Close.
func (d *Desktop) shortcutWorker() {
	sc := &d.shortcuts
	var set []platform.Shortcut // what the app has registered
	for {
		select {
		case <-sc.kick:
		case <-sc.done:
			return
		}
		sc.mu.Lock()
		ops := sc.ops
		sc.ops = nil
		sc.mu.Unlock()

		pending := map[string]int{} // outcomes owed, per ID
		changed := false
		for _, op := range ops {
			if op.rebind {
				changed = changed || len(set) > 0
				continue
			}
			if !op.register {
				if i := indexShortcut(set, op.id); i >= 0 {
					set = slices.Delete(set, i, i+1)
					changed = true
				}
				continue
			}
			pending[op.s.ID]++
			if i := indexShortcut(set, op.s.ID); i >= 0 {
				changed = changed || set[i] != op.s
				set[i] = op.s
			} else {
				set = append(set, op.s)
				changed = true
			}
		}
		if len(pending) == 0 && !changed {
			continue
		}
		if !changed && d.allBound(pending) {
			// Registered again unchanged: the binding stands.
			d.reportShortcuts(set, pending, nil)
			continue
		}
		d.bindShortcuts(slices.Clone(set), pending)
	}
}

func indexShortcut(set []platform.Shortcut, id string) int {
	return slices.IndexFunc(set, func(s platform.Shortcut) bool { return s.ID == id })
}

// allBound reports whether every pending ID is bound now.
func (d *Desktop) allBound(pending map[string]int) bool {
	sc := &d.shortcuts
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for id := range pending {
		if st, ok := sc.state[id]; !ok || st.status != platform.ShortcutBound {
			return false
		}
	}
	return true
}

// bindShortcuts closes the current session and binds set in a new one.
// Worker goroutine.
func (d *Desktop) bindShortcuts(set []platform.Shortcut, pending map[string]int) {
	sc := &d.shortcuts
	sc.mu.Lock()
	old, conn := sc.session, sc.conn
	sc.session, sc.set = "", nil
	sc.mu.Unlock()
	if old != "" && conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		conn.Object(portalName, old).CallWithContext(ctx, ifaceSession+".Close", 0)
		cancel()
	}
	if len(set) == 0 {
		d.reportShortcuts(nil, pending, nil)
		return
	}
	outcome := func(st platform.ShortcutStatus, reason string) map[string]shortcutResult {
		m := map[string]shortcutResult{}
		for _, s := range set {
			m[s.ID] = shortcutResult{status: st, reason: reason}
		}
		return m
	}

	conn, err := d.shortcutConn()
	if err != nil {
		d.reportShortcuts(set, pending, outcome(platform.ShortcutFailed, err.Error()))
		return
	}
	session, err := d.createShortcutSession(conn)
	if err != nil {
		d.reportShortcuts(set, pending, outcome(platform.ShortcutFailed, err.Error()))
		return
	}
	sc.mu.Lock()
	sc.session, sc.set = session, set
	sc.tokens++
	token := fmt.Sprintf("tela_bind%d", sc.tokens)
	parent := sc.parent
	sc.mu.Unlock()

	list := make([]portalShortcut, len(set))
	for i, s := range set {
		trigger, _ := shortcutTrigger(s.Accelerator)
		list[i] = portalShortcut{s.ID, map[string]dbus.Variant{
			"description":       dbus.MakeVariant(s.Description),
			"preferred_trigger": dbus.MakeVariant(trigger),
		}}
	}
	resp, err := d.portalRequest(conn, token, ifaceGlobalShortcuts+".BindShortcuts",
		session, list, parent, map[string]dbus.Variant{"handle_token": dbus.MakeVariant(token)})
	switch {
	case errors.Is(err, errClosed):
		return
	case err != nil:
		d.reportShortcuts(set, pending, outcome(platform.ShortcutFailed, fmt.Sprintf("BindShortcuts failed: %v", err)))
	case resp.code == 1:
		d.reportShortcuts(set, pending, outcome(platform.ShortcutRefused, "the user declined the shortcuts"))
	case resp.code != 0:
		d.reportShortcuts(set, pending, outcome(platform.ShortcutFailed, fmt.Sprintf("the portal ended the request (response %d)", resp.code)))
	default:
		bound, err := responseShortcuts(resp.results)
		if err != nil {
			d.reportShortcuts(set, pending, outcome(platform.ShortcutFailed, err.Error()))
			return
		}
		d.reportShortcuts(set, pending, boundResults(set, bound, "the portal did not bind it"))
	}
}

// shortcutResult is the new state of one shortcut.
type shortcutResult struct {
	status  platform.ShortcutStatus
	trigger string
	reason  string
}

// boundResults maps the shortcuts the portal reported (ID to
// trigger_description) onto set: present is bound, absent is not (status
// ShortcutRefused, with reason).
func boundResults(set []platform.Shortcut, bound map[string]string, reason string) map[string]shortcutResult {
	m := map[string]shortcutResult{}
	for _, s := range set {
		if trigger, ok := bound[s.ID]; ok {
			m[s.ID] = shortcutResult{status: platform.ShortcutBound, trigger: trigger}
		} else {
			m[s.ID] = shortcutResult{status: platform.ShortcutRefused, reason: reason}
		}
	}
	return m
}

// responseShortcuts reads the shortcuts of a BindShortcuts Response (or of
// a ShortcutsChanged signal): ID to trigger_description ("" when the
// portal gives none).
func responseShortcuts(results map[string]dbus.Variant) (map[string]string, error) {
	v, ok := results["shortcuts"]
	if !ok {
		return map[string]string{}, nil
	}
	var list []portalShortcut
	if err := v.Store(&list); err != nil {
		return nil, errors.New("the portal sent invalid shortcuts")
	}
	return shortcutTriggers(list), nil
}

func shortcutTriggers(list []portalShortcut) map[string]string {
	m := make(map[string]string, len(list))
	for _, s := range list {
		trigger := ""
		if v, ok := s.Props["trigger_description"]; ok {
			trigger, _ = v.Value().(string)
		}
		m[s.ID] = trigger
	}
	return m
}

// reportShortcuts gives the app the outcome of a batch: one
// ShortcutStatusEvent per registration in pending (the contract's
// exactly-one, even for a shortcut unregistered in the same batch, which
// is ShortcutFailed), and one for every other shortcut of set whose state
// changed (a bound one that is not bound any more is ShortcutRevoked).
// results nil means the shortcuts keep their state. Forgotten IDs leave
// the state.
func (d *Desktop) reportShortcuts(set []platform.Shortcut, pending map[string]int, results map[string]shortcutResult) {
	sc := &d.shortcuts
	sc.mu.Lock()
	defer sc.mu.Unlock()
	var events []platform.Event
	for _, s := range set {
		prev, had := sc.state[s.ID]
		r, ok := results[s.ID]
		if !ok {
			r = shortcutResult{status: prev.status, trigger: prev.trigger}
		}
		if n := pending[s.ID]; n > 0 {
			for range n {
				events = append(events, platform.ShortcutStatusEvent{ID: s.ID, Status: r.status, Trigger: r.trigger, Reason: r.reason})
			}
			sc.state[s.ID] = shortcutState{r.status, r.trigger}
			continue
		}
		if ev, ok := stateChange(s.ID, prev, had, r); ok {
			events = append(events, ev)
			sc.state[s.ID] = shortcutState{ev.Status, ev.Trigger}
		}
	}
	for id, n := range pending {
		if indexShortcut(set, id) >= 0 {
			continue
		}
		for range n {
			events = append(events, platform.ShortcutStatusEvent{ID: id, Status: platform.ShortcutFailed, Reason: "unregistered before the portal answered"})
		}
	}
	for id := range sc.state {
		if indexShortcut(set, id) < 0 {
			delete(sc.state, id)
		}
	}
	d.pushEvents(events)
}

// stateChange returns the event that reports a shortcut's new result r
// after prev (had: there was one), and whether there is one: a bound
// shortcut that is not bound any more is revoked; one that stays bound
// is reported again only with a new trigger; changes between the unbound
// statuses are not reported.
func stateChange(id string, prev shortcutState, had bool, r shortcutResult) (platform.ShortcutStatusEvent, bool) {
	wasBound := had && prev.status == platform.ShortcutBound
	switch {
	case r.status == platform.ShortcutBound:
		if wasBound && prev.trigger == r.trigger {
			return platform.ShortcutStatusEvent{}, false
		}
		return platform.ShortcutStatusEvent{ID: id, Status: platform.ShortcutBound, Trigger: r.trigger}, true
	case wasBound:
		return platform.ShortcutStatusEvent{ID: id, Status: platform.ShortcutRevoked, Reason: r.reason}, true
	}
	return platform.ShortcutStatusEvent{}, false
}

// pushEvents queues app events in order and wakes the loop once. Safe
// from any goroutine; dropped after Close.
func (d *Desktop) pushEvents(events []platform.Event) {
	if len(events) == 0 {
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	for _, e := range events {
		d.inbox = append(d.inbox, item{ev: e})
	}
	d.mu.Unlock()
	d.opts.Wake()
}

// shortcutConn returns the worker's connection, dialing it (and
// registering the app ID with the portal) on first use, after the bus
// went away, or when the app ID changed. A refusal of the app ID is an
// error wrapping platform.ErrAppNotRegistered, and the new connection is
// closed. UI thread (RegisterShortcut) or worker goroutine.
func (d *Desktop) shortcutConn() (*dbus.Conn, error) {
	sc := &d.shortcuts
	sc.dialMu.Lock()
	defer sc.dialMu.Unlock()
	id := d.appID()
	sc.mu.Lock()
	conn, connID, closed := sc.conn, sc.connID, sc.closed
	sc.mu.Unlock()
	if closed {
		return nil, errClosed
	}
	if conn != nil && conn.Connected() && connID == id {
		return conn, nil
	}
	conn, err := d.dial()
	if err != nil {
		return nil, err
	}
	// The Registry first: no portal call may precede it on the connection.
	if err := registerHostApp(conn, id); err != nil {
		conn.Close()
		return nil, err
	}

	ch := make(chan *dbus.Signal, 64)
	conn.Signal(ch)
	go d.shortcutSignals(conn, ch)
	for _, m := range [][]dbus.MatchOption{
		{dbus.WithMatchInterface(ifaceRequest), dbus.WithMatchMember("Response")},
		{dbus.WithMatchObjectPath(portalPath), dbus.WithMatchInterface(ifaceGlobalShortcuts)},
		{dbus.WithMatchInterface(ifaceSession), dbus.WithMatchMember("Closed")},
	} {
		if err := addMatch(conn, m...); err != nil {
			conn.Close()
			return nil, fmt.Errorf("cannot subscribe to the portal's signals: %v", err)
		}
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.closed {
		conn.Close()
		return nil, errClosed
	}
	if sc.conn != nil {
		// Lost, or registered under the previous app ID: its session
		// goes with it.
		sc.conn.Close()
	}
	sc.initMaps()
	sc.conn, sc.connID = conn, id
	return conn, nil
}

// initMaps makes the maps the worker and the signal goroutine share; sc.mu
// is held.
func (sc *shortcutService) initMaps() {
	if sc.waiters == nil {
		sc.waiters = map[dbus.ObjectPath]chan portalResponse{}
		sc.state = map[string]shortcutState{}
	}
}

// inSandbox reports whether the process runs in a Flatpak or Snap
// sandbox, whose app ID the portal takes from the sandbox: such an app
// does not register (the Registry refuses it).
var inSandbox = func() bool {
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		return true
	}
	return os.Getenv("SNAP") != ""
}

// registerHostApp registers the app ID with the portal's host Registry,
// the first portal call on conn. A portal without the Registry (older
// than 1.19) needs none; any refusal is ErrAppNotRegistered.
func registerHostApp(conn *dbus.Conn, id string) error {
	if inSandbox() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	err := conn.Object(portalName, portalPath).CallWithContext(ctx, ifaceRegistry+".Register", 0,
		id, map[string]dbus.Variant{}).Err
	cancel()
	if err == nil {
		return nil
	}
	var de dbus.Error
	if !errors.As(err, &de) {
		return fmt.Errorf("freedesktop: registering the app ID %q with the portal: %v", id, err)
	}
	switch de.Name {
	case "org.freedesktop.DBus.Error.UnknownMethod", "org.freedesktop.DBus.Error.UnknownInterface",
		"org.freedesktop.DBus.Error.UnknownObject":
		return nil // an older portal identifies host apps without it
	}
	msg := de.Name
	if len(de.Body) > 0 {
		if s, ok := de.Body[0].(string); ok && s != "" {
			msg = s
		}
	}
	return fmt.Errorf("%w: xdg-desktop-portal refused the app ID %q (%s); global shortcuts need a desktop entry %s.desktop: call desktop.RegisterApp in the installed program, or install one",
		platform.ErrAppNotRegistered, id, msg, id)
}

// createShortcutSession creates a GlobalShortcuts session and returns its
// handle. Worker goroutine.
func (d *Desktop) createShortcutSession(conn *dbus.Conn) (dbus.ObjectPath, error) {
	sc := &d.shortcuts
	sc.mu.Lock()
	sc.tokens++
	token := fmt.Sprintf("tela_session%d", sc.tokens)
	sc.mu.Unlock()
	resp, err := d.portalRequest(conn, token, ifaceGlobalShortcuts+".CreateSession", map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant(token),
		"session_handle_token": dbus.MakeVariant(token),
	})
	switch {
	case err != nil:
		return "", fmt.Errorf("CreateSession failed: %w", err)
	case resp.code != 0:
		return "", fmt.Errorf("the portal did not create a session (response %d)", resp.code)
	}
	// session_handle is a string in the specification, an object path in
	// some backends.
	switch h := unwrapVariant(resp.results["session_handle"].Value()).(type) {
	case string:
		if dbus.ObjectPath(h).IsValid() {
			return dbus.ObjectPath(h), nil
		}
	case dbus.ObjectPath:
		if h.IsValid() {
			return h, nil
		}
	}
	return "", errors.New("the portal sent no session handle")
}

// portalRequest calls a portal method that answers with a Request and
// waits for its Response: subscribed at the handle path predicted from
// token before the call, moved to the returned handle when an old portal
// picks another one. It returns errClosed when Close is called meanwhile,
// and an error when the connection closes. Worker goroutine.
func (d *Desktop) portalRequest(conn *dbus.Conn, token, method string, args ...any) (portalResponse, error) {
	sc := &d.shortcuts
	path := requestPath(conn.Names()[0], token)
	ch := make(chan portalResponse, 1)
	sc.mu.Lock()
	sc.waiters[path] = ch
	sc.mu.Unlock()
	defer func() {
		sc.mu.Lock()
		delete(sc.waiters, path)
		sc.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	var handle dbus.ObjectPath
	err := conn.Object(portalName, portalPath).CallWithContext(ctx, method, 0, args...).Store(&handle)
	cancel()
	if err != nil {
		select {
		case <-sc.done:
			return portalResponse{}, errClosed
		default:
		}
		return portalResponse{}, err
	}
	if handle != path && handle.IsValid() {
		sc.mu.Lock()
		delete(sc.waiters, path)
		path = handle
		sc.waiters[path] = ch
		sc.mu.Unlock()
	}
	select {
	case r := <-ch:
		return r, nil
	case <-sc.done:
		return portalResponse{}, errClosed
	case <-conn.Context().Done():
		return portalResponse{}, errors.New("the D-Bus connection closed")
	}
}

// shortcutSignals handles the signals of the worker's connection until it
// closes.
func (d *Desktop) shortcutSignals(conn *dbus.Conn, ch chan *dbus.Signal) {
	sc := &d.shortcuts
	for s := range ch {
		switch s.Name {
		case ifaceRequest + ".Response":
			var r portalResponse
			if dbus.Store(s.Body, &r.code, &r.results) != nil {
				r = portalResponse{code: 2}
			}
			sc.mu.Lock()
			w := sc.waiters[s.Path]
			delete(sc.waiters, s.Path)
			sc.mu.Unlock()
			if w != nil {
				w <- r // buffered; one Response per request
			}
		case ifaceGlobalShortcuts + ".Activated", ifaceGlobalShortcuts + ".Deactivated":
			var (
				session dbus.ObjectPath
				id      string
			)
			if len(s.Body) < 2 || dbus.Store(s.Body[:2], &session, &id) != nil {
				continue
			}
			action := platform.ShortcutPressed
			if s.Name == ifaceGlobalShortcuts+".Deactivated" {
				action = platform.ShortcutReleased
			}
			d.shortcutActivated(session, id, action)
		case ifaceGlobalShortcuts + ".ShortcutsChanged":
			var (
				session dbus.ObjectPath
				list    []portalShortcut
			)
			if dbus.Store(s.Body, &session, &list) != nil {
				continue
			}
			d.shortcutsChanged(session, shortcutTriggers(list), "the user removed it in the system settings")
		case ifaceSession + ".Closed":
			d.shortcutsChanged(s.Path, nil, "the portal closed the session")
		}
	}
}

// shortcutActivated delivers a press or release of a bound shortcut of
// the current session. Signal goroutine.
func (d *Desktop) shortcutActivated(session dbus.ObjectPath, id string, action platform.ShortcutAction) {
	sc := &d.shortcuts
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if session == "" || session != sc.session || sc.state[id].status != platform.ShortcutBound || indexShortcut(sc.set, id) < 0 {
		return
	}
	t := d.now()
	d.pushEvents([]platform.Event{platform.ShortcutEvent{ID: id, Action: action, Time: t}})
}

// shortcutsChanged reports the bindings of the current session after the
// user or the portal changed them: bound (ID to trigger) holds what is
// bound now; nil with a closed session. Signal goroutine.
func (d *Desktop) shortcutsChanged(session dbus.ObjectPath, bound map[string]string, reason string) {
	sc := &d.shortcuts
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if session == "" || session != sc.session {
		return
	}
	if bound == nil {
		sc.session = ""
	}
	var events []platform.Event
	for id, r := range boundResults(sc.set, bound, reason) {
		if r.status != platform.ShortcutBound {
			r.status = platform.ShortcutRevoked
		}
		prev, had := sc.state[id]
		if ev, ok := stateChange(id, prev, had, r); ok {
			events = append(events, ev)
			sc.state[id] = shortcutState{ev.Status, ev.Trigger}
		}
	}
	// Map order is random: report in registration order.
	slices.SortFunc(events, func(a, b platform.Event) int {
		return indexShortcut(sc.set, a.(platform.ShortcutStatusEvent).ID) - indexShortcut(sc.set, b.(platform.ShortcutStatusEvent).ID)
	})
	d.pushEvents(events)
}

// now returns the backend clock (Options.Now), or 0.
func (d *Desktop) now() (t time.Duration) {
	if d.opts.Now != nil {
		t = d.opts.Now()
	}
	return t
}
