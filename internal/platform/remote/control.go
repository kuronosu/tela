package remote

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// The control channel
//
// Besides the platform traffic, a connection carries opaque messages in
// both directions for the layers above (ADR-112): a request names a kind
// ("tela.save-state") and carries a payload, and gets one reply (a payload
// or an error text); a notification has a kind and a payload and no
// reply. This package gives the kinds no meaning. Handlers run on the UI
// thread of the receiving side (the child's loop, the host backend's
// loop), so they may touch the state of that side's app without locks.

// ErrNoHandler is wrapped by the error of a request whose kind has no
// handler on the other side.
var ErrNoHandler = errors.New("remote: no handler for this request kind")

// RemoteError is a request's failure reported by the other side's handler.
type RemoteError struct {
	Kind string // the request kind
	Msg  string // the handler's error text
}

func (e *RemoteError) Error() string { return fmt.Sprintf("remote: %s: %s", e.Kind, e.Msg) }

// Is makes errors.Is(err, ErrNoHandler) work across the connection.
func (e *RemoteError) Is(target error) bool {
	return target == ErrNoHandler && e.Msg == ErrNoHandler.Error()
}

type ctlReply struct {
	payload []byte
	err     error
}

type ctlPending struct {
	kind string
	ch   chan ctlReply
}

// ctl is the request bookkeeping of one connection.
type ctl struct {
	c *conn

	mu      sync.Mutex
	next    uint64
	pending map[uint64]ctlPending
	err     error // the connection failed
}

func newCtl(c *conn) *ctl { return &ctl{c: c, pending: map[uint64]ctlPending{}} }

// request sends a request and waits for its reply, ctx, or the end of the
// connection.
func (k *ctl) request(ctx context.Context, kind string, payload []byte) ([]byte, error) {
	k.mu.Lock()
	if k.err != nil {
		err := k.err
		k.mu.Unlock()
		return nil, err
	}
	k.next++
	id := k.next
	ch := make(chan ctlReply, 1)
	k.pending[id] = ctlPending{kind: kind, ch: ch}
	k.mu.Unlock()

	e := newMsg(msgCtlRequest)
	e.u64(id)
	e.str(kind)
	e.blob(payload)
	if err := k.c.send(e.bytes()); err != nil {
		k.forget(id)
		return nil, err
	}
	select {
	case r := <-ch:
		return r.payload, r.err
	case <-ctx.Done():
		k.forget(id)
		return nil, ctx.Err()
	}
}

func (k *ctl) forget(id uint64) {
	k.mu.Lock()
	delete(k.pending, id)
	k.mu.Unlock()
}

// notify sends a notification and waits until it is written to the
// socket, so a process may exit right after it.
func (k *ctl) notify(kind string, payload []byte) error {
	e := newMsg(msgCtlNotify)
	e.str(kind)
	e.blob(payload)
	return k.c.sendWait(e.bytes())
}

// post sends a notification without waiting for the socket (the host's UI
// thread, which never blocks on a child).
func (k *ctl) post(kind string, payload []byte) {
	e := newMsg(msgCtlNotify)
	e.str(kind)
	e.blob(payload)
	k.c.send(e.bytes())
}

// reply sends the reply to request id.
func (k *ctl) reply(id uint64, payload []byte, err error) {
	e := newMsg(msgCtlReply)
	e.u64(id)
	text := ""
	if err != nil {
		text = err.Error()
	}
	e.str(text)
	e.boolean(err != nil)
	e.blob(payload)
	k.c.send(e.bytes())
}

// replied delivers a reply read from the connection.
func (k *ctl) replied(d *dec) error {
	id := d.u64()
	text := d.str()
	failed := d.boolean()
	payload := d.blob()
	if err := d.end(); err != nil {
		return err
	}
	k.mu.Lock()
	p, ok := k.pending[id]
	delete(k.pending, id)
	k.mu.Unlock()
	if !ok {
		return nil // the requester gave up (context)
	}
	r := ctlReply{payload: payload}
	if failed {
		r.err = &RemoteError{Kind: p.kind, Msg: text}
	}
	p.ch <- r
	return nil
}

// fail ends every pending request with err.
func (k *ctl) fail(err error) {
	if err == nil {
		err = ErrClosed
	}
	k.mu.Lock()
	if k.err == nil {
		k.err = err
	}
	p := k.pending
	k.pending = map[uint64]ctlPending{}
	k.mu.Unlock()
	for _, x := range p {
		x.ch <- ctlReply{err: err}
	}
}

// incoming is a request or notification read from the connection.
type incoming struct {
	id      uint64 // 0 for a notification
	kind    string
	payload []byte
}

func decodeRequest(d *dec) (incoming, error) {
	in := incoming{id: d.u64(), kind: d.str(), payload: d.blob()}
	if in.id == 0 && d.err == nil {
		d.err = errors.New("remote: control request with ID 0")
	}
	return in, d.end()
}

func decodeNotify(d *dec) (incoming, error) {
	in := incoming{kind: d.str(), payload: d.blob()}
	return in, d.end()
}

// run calls fn (nil: no handler) for in and replies when in is a request.
// A panicking handler fails the request and re-panics on this side.
func (k *ctl) run(in incoming, fn func([]byte) ([]byte, error)) {
	if fn == nil {
		if in.id != 0 {
			k.reply(in.id, nil, ErrNoHandler)
		}
		return
	}
	replied := false
	defer func() {
		if !replied && in.id != 0 {
			k.reply(in.id, nil, fmt.Errorf("handler panicked"))
		}
	}()
	out, err := fn(in.payload)
	if in.id != 0 {
		k.reply(in.id, out, err)
	}
	replied = true
}
