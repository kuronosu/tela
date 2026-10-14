// Package fix is a small package to test the reference generator.
//
// It covers the shapes of a Go doc comment: `inline code`, *stars*, a
// snake_case_name, and a <tag> that must not become HTML.
//
// # Getting started
//
// Build a [Widget] with [NewWidget], then call [Widget.Draw]. The
// [Event] type is an alias of an internal type; [other.Helper] lives in
// another package and [time.Duration] in the standard library. A [Missing]
// symbol is left as text. See https://example.com/docs for more.
//
// The steps:
//
//  1. Create the widget.
//  2. Draw it.
//
// The options:
//
//   - [ModeFast] is quick.
//   - [ModeSlow] is careful, and its description
//     wraps onto a second line.
//
// A shell session:
//
//	go test ./...
//
// And Go:
//
//	w := fix.NewWidget("a")
//	w.Draw()
package fix

import (
	"time"

	"example.com/fix/internal/base"
	"example.com/fix/other"
)

var _ = other.Helper

// Event is an alias of a type in an internal package.
type Event = base.Event

// Box is a generic alias.
type Box[T any] = base.Box[T]

// Mode says how fast to draw.
type Mode int

// The modes. [Widget.Draw] honors them.
const (
	// ModeFast draws quickly.
	ModeFast Mode = iota
	// ModeSlow draws carefully.
	ModeSlow
)

// DefaultTimeout is how long to wait.
const DefaultTimeout = 5 * time.Second

// Registry names the widgets by id.
var Registry = map[string]*Widget{}

// Widget is a thing that can be drawn.
type Widget struct {
	// Name identifies the widget.
	Name string
	// Mode is how it is drawn; see [Mode].
	Mode Mode

	cache []byte
}

// NewWidget returns a widget called name, drawn in [ModeFast].
func NewWidget(name string) *Widget { return &Widget{Name: name} }

// Draw draws the widget and returns how long it took.
//
// Draw panics when the [Widget.Name] is empty.
func (w *Widget) Draw() time.Duration { return 0 }

// Reset clears the widget.
func (w Widget) Reset() {}

// Drawer is anything that can be drawn.
type Drawer interface {
	// Draw draws it.
	Draw() time.Duration
}

// Map returns the results of f applied to every item.
//
//	Map([]int{1, 2}, strconv.Itoa)
func Map[T, U any](items []T, f func(T) U) []U { return nil }

// Make is the old name of [NewWidget].
//
// Deprecated: use [NewWidget].
func Make(name string) *Widget { return NewWidget(name) }
