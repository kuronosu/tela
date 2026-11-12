//go:build linux

package freedesktop

import (
	"context"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// Reading the Settings portal before the first window (phase 24).
//
// The first frame needs the system settings (dark scheme, accent, text
// scale, fonts), and reading them costs a few D-Bus round trips: connect,
// NameHasOwner, the interface's version, AddMatch and ReadAll, 3 to 9 ms
// on a desktop with a portal, up to seconds while the bus starts the
// portal. PrefetchSettings makes them on another goroutine from the start
// of core.Run, so that they overlap the font index, the display
// connection and the window's creation instead of running after them on
// the UI thread.
//
// The first Desktop on the same bus takes the read over (New): its first
// SystemSettings waits for the read, at most settingsWait, and the read's
// connection becomes the Desktop's main one, with what it learned of the
// bus (activatable names, portal versions) and its SettingChanged
// subscription. Its first bus() waits for the read to end, without a
// bound of its own: every call of the read is bounded by callTimeout, as
// the UI thread's own calls are. A read still running after settingsWait
// leaves the first frame the zero values (as a portal that does not
// answer did before), and what it reads arrives as a SettingsEvent.

// settingsWait bounds the first SystemSettings' wait for a prefetched
// read: no longer than the one call of the synchronous read that it
// replaces, so a portal that does not answer stalls the startup no more
// than before (it was callTimeout per call, several calls). A variable
// for tests.
var settingsWait = callTimeout

// prefetch is a read of the Settings portal started by PrefetchSettings.
type prefetch struct {
	addr string        // the bus address
	done chan struct{} // closed when the read ended

	// Written by the read's goroutine before done is closed, then only
	// read, by the Desktop that took it.
	conn        *dbus.Conn // nil: no bus
	activatable map[string]bool
	versions    map[string]uint32
	subscribed  bool // the Settings interface is there: its changes are followed

	mu sync.Mutex
	// raw are the values read and the changes signaled since, until a
	// Desktop adopts them (adopted).
	raw     map[settingKey]any
	owner   *Desktop // the Desktop that took the read (New), told when it ends
	adopted *Desktop // the Desktop that holds the values: changes go to it
	ended   bool     // the goroutine finished
	dropped bool     // the owner closed before adopting: the read closes its connection
}

// pending is the read PrefetchSettings started, until a Desktop takes it.
var pending struct {
	mu sync.Mutex
	p  *prefetch
}

// PrefetchSettings starts reading the Settings portal of the session bus
// on another goroutine, for the first Desktop created afterwards (see the
// comment at the top of prefetch.go). It does nothing without a session
// bus, or while a read it started waits for its Desktop. Safe from any
// goroutine.
func PrefetchSettings() {
	if addr := sessionAddress(); addr != "" {
		prefetchSettings(addr)
	}
}

// prefetchSettings starts a read of the Settings portal on the bus at
// addr, unless one is pending.
func prefetchSettings(addr string) {
	pending.mu.Lock()
	defer pending.mu.Unlock()
	if pending.p != nil {
		return
	}
	p := &prefetch{addr: addr, done: make(chan struct{})}
	pending.p = p
	go p.run()
}

// takePrefetch gives d the pending read of its bus, if any.
func takePrefetch(d *Desktop) *prefetch {
	pending.mu.Lock()
	p := pending.p
	if p == nil || p.addr != d.address() {
		pending.mu.Unlock()
		return nil
	}
	pending.p = nil
	pending.mu.Unlock()
	// A read already over is adopted when d first needs it; one still
	// running tells d when it ends.
	p.mu.Lock()
	p.owner = d
	p.mu.Unlock()
	return p
}

// run is the read's goroutine. Its steps are those of Desktop.portal and
// readSettings.
func (p *prefetch) run() {
	defer p.end()
	conn, err := dbus.Connect(p.addr)
	if err != nil {
		return // no bus: the Desktop finds it so again
	}
	p.conn = conn
	if _, err := portalVersion(conn, ifaceSettings, &p.activatable, &p.versions); err != nil {
		return // no portal, or no Settings interface: zero values
	}
	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)
	go p.signals(ch)
	addMatch(conn,
		dbus.WithMatchObjectPath(portalPath),
		dbus.WithMatchInterface(ifaceSettings),
		dbus.WithMatchMember("SettingChanged"))
	p.subscribed = true
	all := readAll(conn)
	p.mu.Lock()
	p.raw = mergeRead(p.raw, all)
	p.mu.Unlock()
}

// end marks the read over and tells its owner, or closes its connection
// when the owner is gone.
func (p *prefetch) end() {
	p.mu.Lock()
	p.ended = true
	owner, dropped := p.owner, p.dropped
	close(p.done)
	p.mu.Unlock()
	switch {
	case dropped:
		if p.conn != nil {
			p.conn.Close()
		}
	case owner != nil:
		// The owner adopts it at its next delivery pass, unless its
		// SystemSettings or bus() did meanwhile.
		owner.push(item{fn: func() {
			if owner.pre == p {
				owner.adopt()
			}
		}})
	}
}

// wait waits for the read to end, at most d. UI thread.
func (p *prefetch) wait(d time.Duration) bool {
	select {
	case <-p.done:
		return true
	default:
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-p.done:
		return true
	case <-t.C:
		return false
	}
}

// abandon is the owner's Close before it adopted the read: the read's
// connection closes, now or when the read ends. UI thread.
func (p *prefetch) abandon() {
	p.mu.Lock()
	p.dropped = true
	ended := p.ended
	p.mu.Unlock()
	if ended && p.conn != nil {
		p.conn.Close()
	}
}

// signals handles the SettingChanged signals of the read's connection
// until it closes: they update the values until a Desktop adopts them,
// then the Desktop's.
func (p *prefetch) signals(ch chan *dbus.Signal) {
	for s := range ch {
		ns, key, v, ok := settingChange(s)
		if !ok {
			continue
		}
		p.mu.Lock()
		if d := p.adopted; d != nil {
			p.mu.Unlock()
			d.settingChanged(ns, key, v)
			continue
		}
		if p.raw == nil {
			p.raw = map[settingKey]any{}
		}
		p.raw[settingKey{ns, key}] = v
		p.mu.Unlock()
	}
}

// adopt takes over the ended read d.pre: its connection becomes the main
// one and its values the settings. When SystemSettings already answered
// without them (the read outlasted settingsWait), a SettingsEvent
// delivers them if they differ. UI thread.
func (d *Desktop) adopt() {
	p := d.pre
	d.pre = nil
	d.settings.taken = true
	if p.conn == nil {
		return // no bus: bus() dials again, SystemSettings stays zero
	}
	d.useConn(p.conn, p.activatable, p.versions)
	if !p.subscribed {
		return // no Settings interface: zero values, as readSettings finds
	}
	p.mu.Lock()
	d.mu.Lock()
	d.settings.raw = p.raw
	if d.settings.raw == nil {
		d.settings.raw = map[settingKey]any{}
	}
	p.raw = nil
	p.adopted = d
	s := settingsFrom(d.settings.raw, d.kdeFonts)
	d.settings.ready = true
	d.settings.reported = s
	late := d.settings.read && s != d.settings.ui
	if late {
		d.inbox = append(d.inbox,
			item{fn: func() { d.settings.ui = s }},
			item{ev: platform.SettingsEvent{Settings: s}})
	}
	d.mu.Unlock()
	p.mu.Unlock()
	if late {
		d.opts.Wake()
	}
}

// readAll reads every namespace the settings use, with a timeout; nil
// when the portal does not answer.
func readAll(conn *dbus.Conn) map[string]map[string]dbus.Variant {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var all map[string]map[string]dbus.Variant
	if err := conn.Object(portalName, portalPath).CallWithContext(ctx, ifaceSettings+".ReadAll", 0, settingsNamespaces).Store(&all); err != nil {
		return nil // the namespaces are unknown: zero values, changes still followed
	}
	return all
}

// mergeRead adds the values ReadAll returned to raw (allocated if nil)
// and returns it. A key raw already has keeps its value: it came from a
// change signaled during the read, which is newer.
func mergeRead(raw map[settingKey]any, all map[string]map[string]dbus.Variant) map[settingKey]any {
	if raw == nil {
		raw = map[settingKey]any{}
	}
	for ns, keys := range all {
		for key, v := range keys {
			k := settingKey{ns, key}
			if _, changed := raw[k]; !changed {
				raw[k] = unwrapVariant(v.Value())
			}
		}
	}
	return raw
}
