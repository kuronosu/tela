//go:build linux

package wire

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// callbackInterface is the minimal wl_callback used by Roundtrip, so the
// runtime does not depend on the generated bindings.
var callbackInterface = Interface{
	Name:    "wl_callback",
	Version: 1,
	Events:  []Message{{Name: "done", Args: []Arg{{Type: ArgUint, Name: "callback_data"}}}},
}

type syncCallback struct {
	Proxy
	done bool
}

func (cb *syncCallback) Descriptor() *Interface { return &callbackInterface }

func (cb *syncCallback) Dispatch(opcode uint16, r *Reader) error {
	if opcode != 0 {
		return UnknownEvent(cb, opcode)
	}
	r.Uint()
	cb.done = true
	return nil
}

// Roundtrip sends wl_display.sync and blocks until the compositor answers,
// dispatching every event that arrives meanwhile. When it returns, all
// requests sent before it have been processed and their events delivered.
// It is meant for startup (registry enumeration, initial configure); the
// steady-state loop belongs to the backend, which must not block here.
func (c *Conn) Roundtrip() error { return c.RoundtripTimeout(-1) }

// RoundtripTimeout is like [Conn.Roundtrip] but gives up with an error after
// timeout. A negative timeout waits forever.
func (c *Conn) RoundtripTimeout(timeout time.Duration) error {
	if c.err != nil {
		return c.err
	}
	var deadline time.Time
	if timeout >= 0 {
		deadline = time.Now().Add(timeout)
	}
	cb := &syncCallback{}
	w := c.begin(DisplayID, 0) // wl_display.sync
	w.NewObject(cb, 1)
	w.Done()

	for {
		// Send everything, waiting for POLLOUT when the socket is full.
		for {
			err := c.Flush()
			if err == nil {
				break
			}
			if err != ErrWouldBlock {
				return c.explain(err)
			}
			if err := c.wait(unix.POLLOUT, deadline); err != nil {
				return err
			}
		}
		if _, err := c.Dispatch(); err != nil {
			return err
		}
		if cb.done {
			return nil
		}
		if err := c.wait(unix.POLLIN, deadline); err != nil {
			return err
		}
		if _, err := c.Read(); err != nil {
			// Drain what arrived before the close; the callback may be in it.
			if _, derr := c.Dispatch(); derr != nil {
				return derr
			}
			if cb.done {
				return nil
			}
			return err
		}
	}
}

// wait blocks until the socket is ready for events or the deadline passes.
func (c *Conn) wait(events int16, deadline time.Time) error {
	for {
		timeout := -1
		if !deadline.IsZero() {
			left := time.Until(deadline)
			if left <= 0 {
				return errors.New("wayland: timed out waiting for the compositor")
			}
			timeout = int((left + time.Millisecond - 1) / time.Millisecond)
		}
		fds := []unix.PollFd{{Fd: int32(c.fd), Events: events}}
		n, err := unix.Poll(fds, timeout)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return c.fail(fmt.Errorf("wayland: poll failed: %w", err))
		}
		if n > 0 {
			// POLLHUP/POLLERR surface through Read or Flush.
			return nil
		}
	}
}

// explain improves a send failure: if the compositor sent a protocol error
// before hanging up, that is the real cause.
func (c *Conn) explain(sendErr error) error {
	if c.err != nil {
		return c.err
	}
	c.Read()
	if _, err := c.Dispatch(); err != nil {
		return err
	}
	return sendErr
}
