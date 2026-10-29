// Package rawwin is the headless window that the tests of aria and kit
// drive with platform events when telatest has no action for what they
// test yet (touch and pen pointers, the secondary button, key repeats). It
// holds what those tests' own harnesses have in common: mounting, settling
// and moving the clock. Each test package keeps its own type around the
// backend, host and window with the actions it needs.
package rawwin

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// TouchAndPen are the pointer kinds that have no telatest action.
var TouchAndPen = []platform.PointerKind{platform.PointerTouch, platform.PointerPen}

// KindName is the name of a pointer kind of TouchAndPen, for subtests.
func KindName(k platform.PointerKind) string {
	if k == platform.PointerTouch {
		return "touch"
	}
	return "pen"
}

// Mount mounts root in a focused headless window of w by h at the display
// scale (0 is 1) with the dev-mode configuration cfg, and settles it. It
// sets cfg.Dev and, when cfg has none, the id prefix.
func Mount(t *testing.T, root tela.Component, w, h float32, scale float64, cfg core.Config) (*headless.Backend, *core.Host, *headless.Window) {
	t.Helper()
	return MountNode(t, core.NodeOf(root), w, h, scale, cfg)
}

// MountNode is Mount for a root that is already a node.
func MountNode(t *testing.T, root core.Node, w, h float32, scale float64, cfg core.Config) (*headless.Backend, *core.Host, *headless.Window) {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: w, H: h}, Scale: scale})
	cfg.Dev = true
	if cfg.IDPrefix == "" {
		cfg.IDPrefix = "tt-"
	}
	host, err := core.NewHost(b, core.Options{Config: cfg, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	win := host.Window().(*headless.Window)
	win.SetFocused(true)
	Settle(b, host)
	return b, host, win
}

// Settle runs the frames the app asks for.
func Settle(b *headless.Backend, host *core.Host) {
	for range 50 {
		b.Drain(host)
		if !b.Pending() && !host.Tree().Dirty() && !host.NeedsFrame() {
			return
		}
		b.Advance(b.FrameInterval())
	}
}

// Hold moves the clock forward by d, stopping at the frames and at the
// gesture deadlines (a long press) on the way, and settles.
func Hold(b *headless.Backend, host *core.Host, d time.Duration) {
	target := b.Now() + d
	b.Wake()
	for {
		b.Drain(host)
		now := b.Now()
		if now >= target {
			break
		}
		next := target
		if t, ok := b.NextFrameTime(); ok && t > now && t < next {
			next = t
		}
		if t, ok := host.Tree().NextDeadline(); ok && t > now && t < next {
			next = t
		}
		b.Advance(next - now)
		b.Wake()
	}
	Settle(b, host)
}

// Frames moves the clock forward by d a frame at a time (a transition runs
// its frames), and settles.
func Frames(b *headless.Backend, host *core.Host, d time.Duration) {
	for end := b.Now() + d; b.Now() < end; {
		b.Advance(b.FrameInterval())
		b.Wake()
		b.Drain(host)
	}
	Settle(b, host)
}
