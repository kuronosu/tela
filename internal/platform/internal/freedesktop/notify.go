//go:build linux

package freedesktop

import (
	"context"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// notificationIconSide is the side of the image sent with a notification:
// servers show icons at 32 to 64 logical pixels, and 128 covers a scale of
// 2 without a large message.
const notificationIconSide = 128

// note is a notification of this process. server is 0 until the Notify
// reply gives the server's ID; closing records a CloseNotification made
// before that.
type note struct {
	server  uint32
	closing bool
}

// Notify sends the notification to org.freedesktop.Notifications. The ID
// is allocated here, so the call does not wait for the server; the server's
// ID arrives with the reply and maps its signals to ours.
func (d *Desktop) Notify(n platform.Notification) (platform.NotificationID, error) {
	platform.CheckNotification(n)
	conn, err := d.bus()
	if err != nil {
		return 0, err
	}
	if !d.has(conn, notificationsName) {
		return 0, unsupported("no notification server (%s) on the session bus", notificationsName)
	}
	if !d.notifyMatched {
		if err := addMatch(conn,
			dbus.WithMatchSender(notificationsName),
			dbus.WithMatchObjectPath(notificationsPath),
			dbus.WithMatchInterface(notificationsIface)); err != nil {
			return 0, unsupported("cannot subscribe to the notification server's signals (%v)", err)
		}
		d.notifyMatched = true
	}
	caps := d.notificationCaps(conn)

	// "default" is the freedesktop name of a click on the notification
	// itself; servers do not show it as a button (GLib sends it the same
	// way, with an empty label).
	actions := []string{"default", ""}
	if caps["actions"] {
		for _, a := range n.Actions {
			actions = append(actions, a.ID, a.Label)
		}
	}
	hints := map[string]dbus.Variant{
		"desktop-entry": dbus.MakeVariant(d.opts.AppID),
	}
	if len(n.Icon) > 0 {
		hints["image-data"] = dbus.MakeVariant(notificationImage(platform.PickIcon(n.Icon, notificationIconSide)))
	}
	body := n.Body
	if caps["body-markup"] {
		body = escapeMarkup(body)
	}

	d.lastNote++
	id := d.lastNote
	d.mu.Lock()
	d.notes[id] = &note{}
	d.mu.Unlock()
	ch := make(chan *dbus.Call, 1)
	conn.Object(notificationsName, notificationsPath).Go(notificationsIface+".Notify", 0, ch,
		d.opts.AppID, uint32(0), "", n.Title, body, actions, hints, int32(-1))
	go d.awaitNotify(conn, id, ch)
	return id, nil
}

// notificationCaps reads the server's capabilities once per connection.
// A server that does not answer is treated as having none (no markup, no
// actions), and asked again next time.
func (d *Desktop) notificationCaps(conn *dbus.Conn) map[string]bool {
	if d.caps != nil {
		return d.caps
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var list []string
	if err := conn.Object(notificationsName, notificationsPath).CallWithContext(ctx, notificationsIface+".GetCapabilities", 0).Store(&list); err != nil {
		return map[string]bool{}
	}
	d.caps = map[string]bool{}
	for _, c := range list {
		d.caps[c] = true
	}
	return d.caps
}

// escapeMarkup escapes the body for servers that read it as markup (a
// small subset of HTML), so that plain text shows as written.
func escapeMarkup(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// awaitNotify records the server's ID of notification id (godbus
// goroutine). A failed Notify forgets the notification: it was never
// shown, so no event can come for it.
func (d *Desktop) awaitNotify(conn *dbus.Conn, id platform.NotificationID, ch chan *dbus.Call) {
	c := <-ch
	var server uint32
	ok := c.Err == nil && c.Store(&server) == nil && server != 0
	d.mu.Lock()
	nt := d.notes[id]
	if nt == nil || !ok {
		delete(d.notes, id)
		d.mu.Unlock()
		return
	}
	closing := nt.closing
	if closing {
		delete(d.notes, id)
	} else {
		nt.server = server
		d.byServer[server] = id
	}
	d.mu.Unlock()
	if closing {
		closeNotification(conn, server)
	}
}

func closeNotification(conn *dbus.Conn, server uint32) {
	conn.Object(notificationsName, notificationsPath).Go(notificationsIface+".CloseNotification", dbus.FlagNoReplyExpected, nil, server)
}

// CloseNotification withdraws a notification. No NotificationClosed event
// follows (contract): the notification is forgotten here.
func (d *Desktop) CloseNotification(id platform.NotificationID) error {
	if d.isClosed() {
		return errClosed
	}
	d.mu.Lock()
	nt := d.notes[id]
	if nt == nil {
		d.mu.Unlock()
		return nil
	}
	if nt.server == 0 {
		nt.closing = true // closed when the Notify reply arrives
		d.mu.Unlock()
		return nil
	}
	server := nt.server
	delete(d.notes, id)
	delete(d.byServer, server)
	d.mu.Unlock()
	if d.conn != nil {
		closeNotification(d.conn, server)
	}
	return nil
}

// onAction handles ActionInvoked (u id, s action_key).
func (d *Desktop) onAction(s *dbus.Signal) {
	var server uint32
	var key string
	if dbus.Store(s.Body, &server, &key) != nil {
		return
	}
	d.mu.Lock()
	id, ok := d.byServer[server]
	d.mu.Unlock()
	if !ok {
		return
	}
	ev := platform.NotificationEvent{ID: id, Kind: platform.NotificationActionInvoked, Action: key}
	if key == "default" {
		ev = platform.NotificationEvent{ID: id, Kind: platform.NotificationActivated}
	}
	d.push(item{ev: ev})
}

// onClosed handles NotificationClosed (u id, u reason). Reason 3 is a
// CloseNotification call, which the contract does not report (ours are
// already forgotten).
func (d *Desktop) onClosed(s *dbus.Signal) {
	var server, reason uint32
	if dbus.Store(s.Body, &server, &reason) != nil {
		return
	}
	d.mu.Lock()
	id, ok := d.byServer[server]
	if ok {
		delete(d.byServer, server)
		delete(d.notes, id)
	}
	d.mu.Unlock()
	if ok && reason != 3 {
		d.push(item{ev: platform.NotificationEvent{ID: id, Kind: platform.NotificationClosed}})
	}
}

// onActivationToken handles ActivationToken (u id, s token), which servers
// send just before ActionInvoked.
func (d *Desktop) onActivationToken(s *dbus.Signal) {
	var server uint32
	var token string
	if dbus.Store(s.Body, &server, &token) != nil {
		return
	}
	d.mu.Lock()
	_, ok := d.byServer[server]
	d.mu.Unlock()
	if ok {
		d.setActivationToken(token)
	}
}
