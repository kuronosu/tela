//go:build !teladev

package core

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestDevStateOff checks that a build without the teladev tag has no
// dev-loop state (ADR-111, ADR-113): the hook points are off, and neither
// this package nor its dependencies import encoding/gob, which only the
// tagged files use.
func TestDevStateOff(t *testing.T) {
	if devStateEnabled {
		t.Fatal("devStateEnabled is true without the teladev tag")
	}
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("no go command: %v", err)
	}
	deps := func(tags ...string) []string {
		t.Helper()
		args := append([]string{"list", "-deps"}, tags...)
		out, err := exec.Command(gobin, append(args, ".")...).Output()
		if err != nil {
			t.Skipf("go list: %v", err)
		}
		return strings.Fields(string(out))
	}
	if d := deps(); slices.Contains(d, "encoding/gob") {
		t.Error("internal/core depends on encoding/gob without the teladev tag")
	}
	// The check sees the tag's dependencies.
	if d := deps("-tags", "teladev"); !slices.Contains(d, "encoding/gob") {
		t.Error("internal/core does not depend on encoding/gob with the teladev tag: the check above proves nothing")
	}
}

// TestFixFocusUnstyledOff checks that the focus fix-up of a build without
// the teladev tag is the one before ADR-114: a focused element without a
// computed style is not focusable, so the fix-up blurs it. Only tela dev's
// restore (devFocus) focuses an element before its first style phase, and
// only builds with the tag leave it alone until the frame decides. The
// focus paths of a release build (ElementRef.Focus, Tab, a press, a Label,
// AutoFocus, the focus scopes) need a computed style, which an element
// never loses, so the state is made by hand here.
func TestFixFocusUnstyledOff(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: 200, H: 100})
	tr.Mount(input(Key("in")))
	el := keyed(tr, "in")
	if el == nil || el.box.cs != nil {
		t.Fatal("setup: want the input mounted and not styled yet")
	}
	tr.foc.el = el
	tr.fixFocus()
	if tr.foc.el != nil {
		t.Error("fixFocus kept the focus on an element without a computed style; release builds blur it (the ADR-114 exemption is for tela dev only)")
	}
}
