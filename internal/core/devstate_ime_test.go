//go:build teladev

package core

import (
	"bytes"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// An IME composition in progress at a reload (ADR-309). The platform's IME
// keeps the preedit in the window's session, which tela dev's host keeps
// across the swap when the new build shows the same field
// (remote.Host.handoverIME): the dev state carries the composition, so the
// new build's first frame shows the preedit, its field reports the text
// input state the IME already has, and the IME's next update (more
// preedit, the commit) applies to it. When the session cannot go on, the
// host tells the new build it ended, which finishes the composition
// keeping its text, as a blur does.

// devIMEDisabled and devIMEReadOnly change the field of devIMEPage in the
// next build.
var devIMEDisabled, devIMEReadOnly bool

func devIMEPage() Node {
	return div(input(Key("in"), DefaultValue("abc"), Disabled(devIMEDisabled), ReadOnly(devIMEReadOnly)), input(Key("other")))
}

// devComposing returns a harness whose field "in" composes "にほ" over
// the selection "bc".
func devComposing(t *testing.T) *inputHarness {
	t.Helper()
	x := newKeys(t, Component(devIMEPage), 400, 200)
	x.tab()
	x.key(platform.KeyEnd, 0)
	x.key(platform.KeyArrowLeft, platform.ModShift)
	x.key(platform.KeyArrowLeft, platform.ModShift) // "bc" selected
	x.settle()
	if !x.w.IMEActive() {
		t.Fatal("no IME session")
	}
	x.w.Inject(platform.IMEEvent{Preedit: "にほ", CursorStart: 6, CursorEnd: 6})
	x.b.Drain(x.h)
	x.settle()
	if v := x.value("in"); v != "aにほ" {
		t.Fatalf("value during the composition %q", v)
	}
	return x
}

// devReloadKeys captures x's state and mounts the page again with it, as the
// next build of tela dev does (SetDevSeed before the Host).
func devReloadKeys(t *testing.T, x *inputHarness) *inputHarness {
	t.Helper()
	data, _ := devSnap(t, x.tree())
	SetDevSeed(data)
	y := newKeys(t, Component(devIMEPage), 400, 200)
	y.frame()
	y.settle()
	return y
}

func TestDevStateKeepsAComposition(t *testing.T) {
	a := devComposing(t)
	b := devReloadKeys(t, a)
	f := fieldOf(t, b.tree(), "in")
	if !f.ed.Composing() || b.value("in") != "aにほ" {
		t.Fatalf("after the reload: composing %v, value %q; want the composition of %q", f.ed.Composing(), b.value("in"), "aにほ")
	}
	if c, _ := f.ed.Composition(); c.Text != "にほ" || c.CursorStart != c.CursorEnd {
		t.Errorf("composition %+v", c)
	}
	// The new build reports the field the IME has: the session can go on.
	if got, want := b.w.TextInput(), a.w.TextInput(); !got.Equal(want) {
		t.Errorf("text input state %+v, want the previous build's %+v", got, want)
	}
	if ia, ib := a.w.Frame(), b.w.Frame(); ia == nil || ib == nil || !bytes.Equal(ia.Pix, ib.Pix) {
		t.Errorf("the first frame of the new build differs from the old one's: the preedit is not shown")
	}
	// The IME goes on: its commit replaces the preedit, and undo gives back
	// the selection the composition replaced.
	b.w.Inject(platform.IMEEvent{Commit: "日本"})
	b.b.Drain(b.h)
	if v := b.value("in"); v != "a日本" || f.ed.Composing() {
		t.Errorf("after the commit: %q, composing %v", v, f.ed.Composing())
	}
	b.ctrl("z", 0)
	if v := b.value("in"); v != "abc" {
		t.Errorf("undo of the composition: %q, want abc", v)
	}
}

// The session ended for the new build (another field, or none): its
// restored composition is finished, keeping the preedit as text.
func TestDevStateCompositionSessionEnds(t *testing.T) {
	b := devReloadKeys(t, devComposing(t))
	b.tree().HandleIMESession(true)
	b.tree().HandleIMESession(false)
	if v, f := b.value("in"), fieldOf(t, b.tree(), "in"); v != "aにほ" || f.ed.Composing() {
		t.Errorf("after the session ended: %q, composing %v; want the preedit committed", v, f.ed.Composing())
	}
}

// A field that the new build disables loses the focus in its first frame:
// the composition restored with the focus is finished as a blur finishes
// one, keeping the preedit as text.
func TestDevStateCompositionBlurred(t *testing.T) {
	a := devComposing(t)
	devIMEDisabled = true
	defer func() { devIMEDisabled = false }()
	b := devReloadKeys(t, a)
	if v, f := b.value("in"), fieldOf(t, b.tree(), "in"); v != "aにほ" || f.ed.Composing() || b.tree().foc.el != nil {
		t.Errorf("a disabled field: %q, composing %v, focused %v; want the preedit committed, no focus", v, f.ed.Composing(), b.tree().foc.el != nil)
	}
}

// A field that cannot compose in the new build (read-only) is not
// composing: the composition is canceled cleanly, the field shows the text
// it had before it began.
func TestDevStateCompositionCanceled(t *testing.T) {
	a := devComposing(t)
	devIMEReadOnly = true
	defer func() { devIMEReadOnly = false }()
	b := devReloadKeys(t, a)
	if v, f := b.value("in"), fieldOf(t, b.tree(), "in"); v != "abc" || f.ed.Composing() {
		t.Errorf("a read-only field: %q, composing %v; want abc", v, f.ed.Composing())
	}
}
