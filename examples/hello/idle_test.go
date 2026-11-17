package main

import (
	"testing"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/testutil/idle"
)

// TestIdleBudget: once the screen has been drawn, a window nobody touches
// does nothing (ARCHITECTURE §3.5, "no change, no frame"; ADR-290): no
// frame, build, style, layout or paint over a second of frames and five
// more seconds, and nothing pending, no frame request, animation or timer.
func TestIdleBudget(t *testing.T) {
	idle.Zero(t, "hello", idle.Watch(mount(t)))
	idle.Mount(t, Hello(), 800, 450, core.Config{}).Settle().Idle("hello")
}
