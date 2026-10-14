// Package base holds the types the fixture's public package aliases.
package base

// Event is something that happens.
type Event struct {
	// Name is what happened.
	Name string
	// At is when it happened, in milliseconds.
	At int64

	hidden bool
}

// Stop stops the event from going further.
func (e Event) Stop() { e.hidden = true }

// Stopped reports whether [Event.Stop] was called.
func (e *Event) Stopped() bool { return e.hidden }

// Box holds one value of any type.
type Box[T any] struct{ v T }

// Get returns the value in the box.
func (b Box[T]) Get() T { return b.v }
