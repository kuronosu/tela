//go:build linux

package freedesktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"github.com/kuronosu/tela/internal/platform"
)

// Single instance (ADR-209, ADR-210) over the session bus.
//
// The first process of an app owns a well-known bus name derived from
// the app ID (instanceBusName), requested with DBUS_NAME_FLAG_DO_NOT_QUEUE
// on a connection of its own, which it keeps until Close (when Run
// returns) or until it exits; the bus then releases the name. Before
// requesting the name it exports, at the name's object path ("/" and the
// name with '.' as '/' and '-' as '_', as org.freedesktop.Application
// specifies):
//
//   - dev.telaui.Instance.Launch(as args, s dir, s token): a launch
//     forwarded by a second process, delivered as a LaunchEvent.
//   - org.freedesktop.Application (Activate, Open, ActivateAction), so
//     the desktop's own launchers reach the running instance too: a
//     .desktop file with DBusActivatable=true, and tools such as
//     "gapplication launch". Activate is a LaunchEvent without
//     arguments, Open one whose Args are the URIs (file: URIs as local
//     paths, like a command line), both with the platform_data's
//     activation-token (or desktop-startup-id) as Token; the app has no
//     actions, so ActivateAction is an error. Tela does not install the
//     D-Bus service file that DBusActivatable needs (package assoc writes
//     .desktop files only); the interface answers when the app runs.
//
// A second process finds the name taken, calls Launch on its owner with
// its own platform.Launch (the caller gives CurrentLaunch, whose Token is
// XDG_ACTIVATION_TOKEN or DESKTOP_STARTUP_ID), closes its connection and
// returns false. When the owner exits between the two steps, the second
// process tries to take the name again.

const (
	ifaceInstance    = "dev.telaui.Instance"
	ifaceApplication = "org.freedesktop.Application"
)

// instanceState holds the claims of a Desktop. UI thread only.
type instanceState struct {
	conns map[string]*dbus.Conn // app ID to the connection owning its name
}

// ClaimInstance makes this process the single instance of appID (see the
// comment at the top of the file). Without a session bus it returns an
// ErrUnsupported error, and the app runs on its own.
func (d *Desktop) ClaimInstance(appID string, launch platform.Launch) (bool, error) {
	platform.CheckAppID("ClaimInstance", appID)
	if d.isClosed() {
		return false, errClosed
	}
	if d.instances.conns[appID] != nil {
		return true, nil
	}
	name := instanceBusName(appID)
	path := busNamePath(name)
	for range 3 {
		conn, err := d.dial()
		if err != nil {
			return false, err
		}
		if err := d.exportInstance(conn, path); err != nil {
			conn.Close()
			return false, fmt.Errorf("freedesktop: ClaimInstance: cannot export the launch method: %w", err)
		}
		reply, err := requestName(conn, name)
		if err != nil {
			conn.Close()
			return false, fmt.Errorf("freedesktop: ClaimInstance: cannot request the bus name %s: %w", name, err)
		}
		if reply == dbus.RequestNameReplyPrimaryOwner || reply == dbus.RequestNameReplyAlreadyOwner {
			if d.instances.conns == nil {
				d.instances.conns = map[string]*dbus.Conn{}
			}
			d.instances.conns[appID] = conn
			return true, nil
		}
		err = forwardLaunch(conn, name, path, launch)
		conn.Close()
		if err == nil {
			return false, nil
		}
		if !ownerGone(err) {
			return false, fmt.Errorf("freedesktop: ClaimInstance: the running instance of %s (bus name %s) did not take the launch: %w", appID, name, err)
		}
	}
	return false, fmt.Errorf("freedesktop: ClaimInstance: the owner of the bus name %s keeps going away", name)
}

// closeInstances releases the bus names. UI thread, from Close.
func (d *Desktop) closeInstances() {
	for id, conn := range d.instances.conns {
		conn.Close()
		delete(d.instances.conns, id)
	}
}

// requestName asks for name without queueing, with a timeout.
func requestName(conn *dbus.Conn, name string) (dbus.RequestNameReply, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var r uint32
	err := conn.BusObject().CallWithContext(ctx, busIface+".RequestName", 0, name, uint32(dbus.NameFlagDoNotQueue)).Store(&r)
	return dbus.RequestNameReply(r), err
}

// forwardLaunch calls Launch on the instance that owns name.
func forwardLaunch(conn *dbus.Conn, name string, path dbus.ObjectPath, l platform.Launch) error {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	args := l.Args
	if args == nil {
		args = []string{}
	}
	return conn.Object(name, path).CallWithContext(ctx, ifaceInstance+".Launch", 0, args, l.Dir, l.Token).Err
}

// ownerGone reports whether a call failed because nobody owns the name
// any more.
func ownerGone(err error) bool {
	var derr dbus.Error
	if errors.As(err, &derr) {
		return derr.Name == "org.freedesktop.DBus.Error.ServiceUnknown" || derr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner"
	}
	return false
}

// exportInstance exports the Launch method and org.freedesktop.Application
// at path; incoming calls run on godbus goroutines and queue LaunchEvents.
func (d *Desktop) exportInstance(conn *dbus.Conn, path dbus.ObjectPath) error {
	launch := func(l platform.Launch) {
		if len(l.Args) == 0 {
			l.Args = nil
		}
		d.push(item{ev: platform.LaunchEvent{Launch: l}})
	}
	if err := conn.ExportMethodTable(map[string]any{
		"Launch": func(args []string, dir, token string) *dbus.Error {
			launch(platform.Launch{Args: args, Dir: dir, Token: token})
			return nil
		},
	}, path, ifaceInstance); err != nil {
		return err
	}
	if err := conn.ExportMethodTable(map[string]any{
		"Activate": func(data map[string]dbus.Variant) *dbus.Error {
			launch(platform.Launch{Token: platformDataToken(data)})
			return nil
		},
		"Open": func(uris []string, data map[string]dbus.Variant) *dbus.Error {
			args := make([]string, len(uris))
			for i, u := range uris {
				args[i] = u
				if p, err := uriPath(u); err == nil {
					args[i] = p
				}
			}
			launch(platform.Launch{Args: args, Token: platformDataToken(data)})
			return nil
		},
		"ActivateAction": func(action string, _ []dbus.Variant, _ map[string]dbus.Variant) *dbus.Error {
			return dbus.NewError("org.freedesktop.DBus.Error.NotSupported", []any{fmt.Sprintf("the app has no action %q", action)})
		},
	}, path, ifaceApplication); err != nil {
		return err
	}
	node := &introspect.Node{
		Name: string(path),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{Name: ifaceInstance, Methods: []introspect.Method{{Name: "Launch", Args: []introspect.Arg{
				{Name: "args", Type: "as", Direction: "in"},
				{Name: "dir", Type: "s", Direction: "in"},
				{Name: "token", Type: "s", Direction: "in"},
			}}}},
			{Name: ifaceApplication, Methods: []introspect.Method{
				{Name: "Activate", Args: []introspect.Arg{{Name: "platform_data", Type: "a{sv}", Direction: "in"}}},
				{Name: "Open", Args: []introspect.Arg{
					{Name: "uris", Type: "as", Direction: "in"},
					{Name: "platform_data", Type: "a{sv}", Direction: "in"},
				}},
				{Name: "ActivateAction", Args: []introspect.Arg{
					{Name: "action_name", Type: "s", Direction: "in"},
					{Name: "parameter", Type: "av", Direction: "in"},
					{Name: "platform_data", Type: "a{sv}", Direction: "in"},
				}},
			}},
		},
	}
	return conn.Export(introspect.NewIntrospectable(node), path, "org.freedesktop.DBus.Introspectable")
}

// platformDataToken returns the activation token of an
// org.freedesktop.Application call's platform_data.
func platformDataToken(data map[string]dbus.Variant) string {
	for _, k := range [...]string{"activation-token", "desktop-startup-id"} {
		if t, ok := data[k].Value().(string); ok && t != "" {
			return t
		}
	}
	return ""
}

// instanceBusName returns the well-known bus name of an app ID: the ID
// itself when it is a valid bus name (reverse DNS, "com.example.Notes"),
// else a name derived from it deterministically:
//
//	dev.telaui.Instance.h<8 hex digits>_<sanitized ID>
//
// where the hex digits are the first four bytes of the SHA-256 of the ID
// (so IDs that sanitize alike still differ) and the sanitized ID is the
// ID's bytes with every byte other than an ASCII letter, digit or '_'
// replaced by '_', cut to 200 bytes ("notes" gives
// "dev.telaui.Instance.h<hash>_notes").
func instanceBusName(appID string) string {
	if validBusName(appID) {
		return appID
	}
	sum := sha256.Sum256([]byte(appID))
	var b strings.Builder
	b.WriteString("dev.telaui.Instance.h")
	b.WriteString(hex.EncodeToString(sum[:4]))
	b.WriteByte('_')
	for i := 0; i < len(appID) && i < 200; i++ {
		c := appID[i]
		if isBusNameChar(c) && c != '-' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// validBusName reports whether s is a valid well-known D-Bus name: at
// most 255 bytes, two or more '.'-separated elements of ASCII letters,
// digits, '_' and '-', none empty or starting with a digit.
func validBusName(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	elems := strings.Split(s, ".")
	if len(elems) < 2 {
		return false
	}
	for _, e := range elems {
		if e == "" || e[0] >= '0' && e[0] <= '9' {
			return false
		}
		for i := 0; i < len(e); i++ {
			if !isBusNameChar(e[i]) {
				return false
			}
		}
	}
	return true
}

func isBusNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// busNamePath is the object path of a bus name, as
// org.freedesktop.Application specifies: "/" and the name with '.' as
// '/' and '-' as '_' ("/com/example/Notes").
func busNamePath(name string) dbus.ObjectPath {
	return dbus.ObjectPath("/" + strings.NewReplacer(".", "/", "-", "_").Replace(name))
}

// closeIntegration ends the phase-13 services: the shortcut session and
// the instance claims. UI thread, from Close.
func (d *Desktop) closeIntegration() {
	d.closeShortcuts()
	d.closeInstances()
}
