package idle_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/testutil/idle"
)

func mount(t *testing.T, root func() tela.Node) *idle.Host {
	t.Helper()
	return idle.Mount(t, tela.C(root), 200, 100, core.Config{})
}

// TestOnlyEndless: what never ends is a spinner and a canvas that asks for a
// frame in every draw; a still screen is not "only endless", it is idle.
func TestOnlyEndless(t *testing.T) {
	t.Run("a spinner", func(t *testing.T) {
		host := mount(t, func() tela.Node { return h.Div(h.Class("size-8 animate-spin rounded-full border-4")) })
		host.SettleEndless()
		if !host.OnlyEndless() {
			t.Errorf("a spinner is endless: %v", host.Busy())
		}
	})
	t.Run("a canvas that asks for frames", func(t *testing.T) {
		host := mount(t, func() tela.Node {
			return h.Canvas(func(c *tela.Canvas) {
				c.FillRect(0, 0, c.Width*float64(c.Time.Milliseconds()%1000)/1000, c.Height)
				c.RequestFrame()
			}, h.Class("h-8 w-32"))
		})
		host.SettleEndless()
		if !host.OnlyEndless() {
			t.Errorf("a canvas that calls RequestFrame in every draw is endless: %v", host.Busy())
		}
		if len(host.Busy()) == 0 {
			t.Errorf("it asks for frames, so it is busy")
		}
	})
	t.Run("a canvas that rests", func(t *testing.T) {
		host := mount(t, func() tela.Node {
			return h.Canvas(func(c *tela.Canvas) { c.FillRect(0, 0, c.Width, c.Height) }, h.Class("h-8 w-32"))
		})
		host.Settle()
		if host.OnlyEndless() || len(host.Busy()) != 0 {
			t.Errorf("a canvas that does not ask for frames is idle: endless %v, busy %v", host.OnlyEndless(), host.Busy())
		}
	})
}
