//go:build linux

package wire

import "fmt"

// Object is implemented by every generated interface type. Generated types
// get Base from the embedded [Proxy] and define Descriptor and Dispatch.
type Object interface {
	// Base returns the embedded Proxy.
	Base() *Proxy
	// Descriptor returns the interface table of the object.
	Descriptor() *Interface
	// Dispatch decodes the event with the given opcode from r and calls the
	// registered handler. The runtime calls it from [Conn.Dispatch].
	Dispatch(opcode uint16, r *Reader) error
}

// Proxy is the client-side state of one Wayland object. Generated types embed
// it, so ID, Version, Conn and Destroyed are available on every proxy.
type Proxy struct {
	conn      *Conn
	id        uint32
	version   uint32
	iface     *Interface
	destroyed bool
	dropped   bool // the request that created it was never sent
}

// Base returns p. It makes every type that embeds a Proxy satisfy the Base
// method of [Object].
func (p *Proxy) Base() *Proxy { return p }

// ID returns the object ID, or 0 for a proxy that was never bound.
func (p *Proxy) ID() uint32 { return p.id }

// Version returns the version the object was created or bound with.
func (p *Proxy) Version() uint32 { return p.version }

// Conn returns the connection the proxy belongs to, or nil if unbound.
func (p *Proxy) Conn() *Conn { return p.conn }

// Destroyed reports whether the proxy was destroyed, either client-side by a
// destructor request or after wl_display.delete_id, or never existed on the
// server because the request that created it was dropped (see
// [Writer.Done]). Requests on a destroyed proxy panic, except on a dropped
// one, where they are dropped too.
func (p *Proxy) Destroyed() bool { return p.destroyed }

func (p *Proxy) String() string {
	name := "unbound"
	if p.iface != nil {
		name = p.iface.Name
	}
	return fmt.Sprintf("%s#%d", name, p.id)
}

// Begin starts encoding the request with the given opcode and returns the
// writer to append its arguments to. Generated request methods call it.
//
// It panics if the proxy is unbound or destroyed, or if the request is newer
// than the version the object was bound with. Those are programming errors:
// check [Proxy.Version] before calling a request that is not in version 1.
func (p *Proxy) Begin(opcode uint16) *Writer {
	if p.conn == nil {
		panic(fmt.Sprintf("wayland: request on a proxy that is not bound to a connection (opcode %d)", opcode))
	}
	m := &p.iface.Requests[opcode]
	if p.dropped {
		// Its creation was never sent and the connection is failing:
		// encode the request into the void (Done discards it).
		w := p.conn.begin(p.id, opcode)
		w.err = errDroppedProxy
		return w
	}
	if p.destroyed {
		panic(fmt.Sprintf("wayland: %s.%s called on destroyed object %s", p.iface.Name, m.Name, p))
	}
	if m.Since > p.version {
		panic(fmt.Sprintf("wayland: %s.%s requires version %d, but %s was bound at version %d; check Version() before calling it",
			p.iface.Name, m.Name, m.Since, p, p.version))
	}
	return p.conn.begin(p.id, opcode)
}

// MarkDestroyed records that a destructor request was sent. The object ID
// stays reserved until the compositor acknowledges it with wl_display.delete_id,
// and events still in flight for the object are dropped (their file
// descriptors are closed). Generated destructor methods call it after
// finishing the request.
func (p *Proxy) MarkDestroyed() {
	if p.conn != nil && !p.destroyed {
		p.conn.destroyProxy(p)
	}
}
