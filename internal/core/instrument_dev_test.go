//go:build teladev

package core

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// insReloaded is a component with state that tela dev keeps.
func insReloaded() Node {
	n, set := UseState(7)
	insReloadSet = set
	return Primitive(style.KindP, Textf("%d", n))
}

var insReloadSet Setter[int]

// A component mounted by a tela dev reload builds for CauseReload, not
// CauseMount (ADR-113, ADR-299).
func TestInstrumentReasonReload(t *testing.T) {
	old := devTestTree(t, Component(insReloaded), nil, nil)
	insReloadSet.Set(9)
	devFrames(old)
	data, _ := devSnap(t, old)
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}})
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.SeedDevState(data)
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	tr.Mount(Component(insReloaded))
	devFrames(tr)
	c := insOne(t, tr, "core.insReloaded")
	if got := causesOf(lastReasons(t, c)); !slices.Equal(got, []BuildCause{CauseReload}) {
		t.Errorf("reasons %v, want reload", got)
	}
	if c.Hooks[0].Value != "9" {
		t.Errorf("state %q", c.Hooks[0].Value)
	}
}
