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

// Phase is a step of an event. Its constants are documented one by one,
// below the type: an alias page prints them from this file.
type Phase uint8

const (
	// PhaseCapture goes from the root down to the target.
	PhaseCapture Phase = iota
	// PhaseBubble goes from the target back up
	// to the root.
	PhaseBubble
)
