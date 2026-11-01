//go:build linux

package freedesktop

import (
	"fmt"
	"maps"
	"strings"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakePortal13 is an XDG Desktop Portal with the phase-13 interfaces:
// Settings, GlobalShortcuts and the host Registry. Each may be left out.
type fakePortal13 struct {
	t    *testing.T
	conn *dbus.Conn

	mu       sync.Mutex
	settings map[string]map[string]dbus.Variant // nil: no Settings interface
	// bind answers BindShortcuts: the response code and the shortcuts
	// bound; nil binds every shortcut with its preferred trigger as the
	// trigger description.
	bind     func(shortcuts []portalShortcut) (uint32, []portalShortcut)
	calls    []string                 // methods called, in order ("Register tela-test", "CreateSession", …)
	binds    [][]portalShortcut       // BindShortcuts arguments
	sessions []dbus.ObjectPath        // sessions created
	closed   map[dbus.ObjectPath]bool // sessions the client closed
	senders  map[string]bool          // senders that made a portal call
	regErr   []string                 // Register calls made after another call (the portal refuses them)
	readAlls int
}

type portal13Options struct {
	settings  map[string]map[string]dbus.Variant // Settings interface when non-nil
	shortcuts bool                               // GlobalShortcuts interface
	registry  bool                               // Registry interface
}

func startPortal13(t *testing.T, addr string, o portal13Options) *fakePortal13 {
	t.Helper()
	p := &fakePortal13{t: t, conn: connect(t, addr), settings: o.settings,
		closed: map[dbus.ObjectPath]bool{}, senders: map[string]bool{}}
	versions := map[string]uint32{}
	if o.settings != nil {
		versions[ifaceSettings] = 2
		if err := p.conn.ExportMethodTable(map[string]any{
			"ReadAll": func(sender dbus.Sender, namespaces []string) (map[string]map[string]dbus.Variant, *dbus.Error) {
				p.mu.Lock()
				defer p.mu.Unlock()
				p.called(sender, "ReadAll")
				p.readAlls++
				out := map[string]map[string]dbus.Variant{}
				for _, ns := range namespaces {
					if keys, ok := p.settings[ns]; ok {
						// A copy: godbus encodes the reply after the
						// lock is released.
						out[ns] = maps.Clone(keys)
					}
				}
				return out, nil
			},
		}, portalPath, ifaceSettings); err != nil {
			t.Fatal(err)
		}
	}
	if o.shortcuts {
		versions[ifaceGlobalShortcuts] = 1
		if err := p.conn.ExportMethodTable(map[string]any{
			"CreateSession": func(sender dbus.Sender, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
				token, _ := options["handle_token"].Value().(string)
				stoken, _ := options["session_handle_token"].Value().(string)
				handle := requestPath(string(sender), token)
				session := sessionPath(string(sender), stoken)
				p.mu.Lock()
				p.called(sender, "CreateSession")
				p.sessions = append(p.sessions, session)
				p.mu.Unlock()
				if err := p.conn.ExportMethodTable(map[string]any{
					"Close": func() *dbus.Error {
						p.mu.Lock()
						defer p.mu.Unlock()
						p.closed[session] = true
						return nil
					},
				}, session, ifaceSession); err != nil {
					t.Error(err)
				}
				go p.conn.Emit(handle, ifaceRequest+".Response", uint32(0), map[string]dbus.Variant{
					"session_handle": dbus.MakeVariant(string(session)),
				})
				return handle, nil
			},
			"BindShortcuts": func(sender dbus.Sender, session dbus.ObjectPath, shortcuts []portalShortcut, parent string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
				token, _ := options["handle_token"].Value().(string)
				handle := requestPath(string(sender), token)
				p.mu.Lock()
				p.called(sender, "BindShortcuts")
				p.binds = append(p.binds, shortcuts)
				bind := p.bind
				p.mu.Unlock()
				code, bound := uint32(0), []portalShortcut(nil)
				if bind != nil {
					code, bound = bind(shortcuts)
				} else {
					for _, s := range shortcuts {
						trigger, _ := s.Props["preferred_trigger"].Value().(string)
						bound = append(bound, portalShortcut{s.ID, map[string]dbus.Variant{
							"description":         s.Props["description"],
							"trigger_description": dbus.MakeVariant(trigger),
						}})
					}
				}
				results := map[string]dbus.Variant{}
				if code == 0 {
					results["shortcuts"] = dbus.MakeVariant(bound)
				}
				go p.conn.Emit(handle, ifaceRequest+".Response", code, results)
				return handle, nil
			},
		}, portalPath, ifaceGlobalShortcuts); err != nil {
			t.Fatal(err)
		}
	}
	if o.registry {
		versions[ifaceRegistry] = 1
		if err := p.conn.ExportMethodTable(map[string]any{
			"Register": func(sender dbus.Sender, appID string, options map[string]dbus.Variant) *dbus.Error {
				p.mu.Lock()
				defer p.mu.Unlock()
				if p.senders[string(sender)] {
					// xdg-desktop-portal refuses a registration after the
					// caller was looked up by another call.
					p.regErr = append(p.regErr, appID)
					return dbus.NewError("org.freedesktop.portal.Error.Failed", []any{"already looked up"})
				}
				p.called(sender, "Register "+appID)
				return nil
			},
		}, portalPath, ifaceRegistry); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.conn.ExportMethodTable(map[string]any{
		"Get": func(iface, name string) (dbus.Variant, *dbus.Error) {
			if v, ok := versions[iface]; ok && name == "version" {
				return dbus.MakeVariant(v), nil
			}
			return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", []any{iface + "." + name})
		},
	}, portalPath, propsIface); err != nil {
		t.Fatal(err)
	}
	own(t, p.conn, portalName)
	return p
}

// called records a portal call; p.mu is held.
func (p *fakePortal13) called(sender dbus.Sender, what string) {
	p.senders[string(sender)] = true
	p.calls = append(p.calls, what)
}

// sessionPath is the handle path of a session:
// /org/freedesktop/portal/desktop/session/SENDER/TOKEN.
func sessionPath(unique, token string) dbus.ObjectPath {
	sender := strings.ReplaceAll(strings.TrimPrefix(unique, ":"), ".", "_")
	return dbus.ObjectPath("/org/freedesktop/portal/desktop/session/" + sender + "/" + token)
}

// setSetting changes a value and emits SettingChanged.
func (p *fakePortal13) setSetting(ns, key string, v any) {
	p.t.Helper()
	p.mu.Lock()
	if p.settings[ns] == nil {
		p.settings[ns] = map[string]dbus.Variant{}
	}
	p.settings[ns][key] = dbus.MakeVariant(v)
	p.mu.Unlock()
	if err := p.conn.Emit(portalPath, ifaceSettings+".SettingChanged", ns, key, dbus.MakeVariant(v)); err != nil {
		p.t.Fatal(err)
	}
}

func (p *fakePortal13) setBind(fn func([]portalShortcut) (uint32, []portalShortcut)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bind = fn
}

func (p *fakePortal13) snapshot() (calls []string, binds [][]portalShortcut, sessions []dbus.ObjectPath, closed map[dbus.ObjectPath]bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	closed = map[dbus.ObjectPath]bool{}
	for k, v := range p.closed {
		closed[k] = v
	}
	return append([]string(nil), p.calls...), append([][]portalShortcut(nil), p.binds...),
		append([]dbus.ObjectPath(nil), p.sessions...), closed
}

// emitShortcut emits a GlobalShortcuts signal (Activated, Deactivated).
func (p *fakePortal13) emitShortcut(member string, session dbus.ObjectPath, id string) {
	p.t.Helper()
	if err := p.conn.Emit(portalPath, ifaceGlobalShortcuts+"."+member, session, id, uint64(1234), map[string]dbus.Variant{}); err != nil {
		p.t.Fatal(err)
	}
}

// boundShortcuts builds a shortcut list with trigger descriptions, as the
// portal sends it (ID, trigger, ID, trigger…).
func boundShortcuts(pairs ...string) []portalShortcut {
	if len(pairs)%2 != 0 {
		panic(fmt.Sprintf("boundShortcuts: odd arguments %q", pairs))
	}
	var out []portalShortcut
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, portalShortcut{pairs[i], map[string]dbus.Variant{
			"trigger_description": dbus.MakeVariant(pairs[i+1]),
		}})
	}
	return out
}
