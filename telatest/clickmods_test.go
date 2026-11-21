package telatest_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// modsLog is a button that records the modifiers of its pointer press and
// its click.
func modsLog(rec *recorder) func() tela.Node {
	flags := func(shift, ctrl, alt, meta, cmd bool) string {
		var s []string
		for _, f := range []struct {
			on   bool
			name string
		}{{shift, "shift"}, {ctrl, "ctrl"}, {alt, "alt"}, {meta, "meta"}, {cmd, "cmd"}} {
			if f.on {
				s = append(s, f.name)
			}
		}
		return strings.Join(s, "+")
	}
	return func() tela.Node {
		return Button(Class("px-4 py-2"), Text("Target"),
			OnPointerDown(func(e tela.PointerEvent) {
				rec.add("down[%s]", flags(e.Shift, e.Ctrl, e.Alt, e.Meta, e.CmdOrCtrl()))
			}),
			OnClick(func(e tela.MouseEvent) {
				rec.add("click[%s]#%d", flags(e.Shift, e.Ctrl, e.Alt, e.Meta, e.CmdOrCtrl()), e.ClickCount)
			}))
	}
}

// Modifiers holds keys during a click, like Playwright's
// click({modifiers}): the pointer events and the click carry them, and
// they are released with it.
func TestClickModifiers(t *testing.T) {
	var rec recorder
	app := telatest.MountFunc(t, modsLog(&rec), telatest.Size(200, 100), telatest.PlatformKeys("linux"))
	btn := telatest.ByRole("button", "Target")
	app.Click(btn, telatest.Modifiers("Shift"))
	if got, want := rec.take(), "down[shift],click[shift]#1"; got != want {
		t.Errorf("Shift+click: %q, want %q", got, want)
	}
	app.Click(btn, telatest.Modifiers("Control", "Alt"))
	if got, want := rec.take(), "down[ctrl+alt+cmd],click[ctrl+alt+cmd]#1"; got != want {
		t.Errorf("Control+Alt+click: %q, want %q", got, want)
	}
	app.Click(btn, telatest.Modifiers("Meta"))
	if got, want := rec.take(), "down[meta],click[meta]#1"; got != want {
		t.Errorf("Meta+click: %q, want %q", got, want)
	}
	// Nothing is held afterwards.
	app.Click(btn)
	if got, want := rec.take(), "down[],click[]#1"; got != want {
		t.Errorf("plain click after them: %q, want %q", got, want)
	}
	app.ClickAt(10, 10, telatest.Modifiers("Shift"))
	if got, want := rec.take(), "down[shift],click[shift]#1"; got != want {
		t.Errorf("ClickAt with Shift: %q, want %q", got, want)
	}
	app.DoubleClick(btn, telatest.Modifiers("Shift"))
	if got, want := rec.take(), "down[shift],click[shift]#1,down[shift],click[shift]#2"; got != want {
		t.Errorf("DoubleClick with Shift: %q, want %q", got, want)
	}
}

// ControlOrMeta follows PlatformKeys, as it does for Press.
func TestClickModifiersControlOrMeta(t *testing.T) {
	for _, c := range []struct{ goos, want string }{
		{"darwin", "down[meta+cmd],click[meta+cmd]#1"},
		{"linux", "down[ctrl+cmd],click[ctrl+cmd]#1"},
		{"windows", "down[ctrl+cmd],click[ctrl+cmd]#1"},
	} {
		t.Run(c.goos, func(t *testing.T) {
			var rec recorder
			app := telatest.MountFunc(t, modsLog(&rec), telatest.Size(200, 100), telatest.PlatformKeys(c.goos))
			app.Click(telatest.ByRole("button", "Target"), telatest.Modifiers("ControlOrMeta"))
			if got := rec.take(); got != c.want {
				t.Errorf("ControlOrMeta+click: %q, want %q", got, c.want)
			}
		})
	}
}

// The same option holds keys for a drag.
func TestDragModifiers(t *testing.T) {
	var rec recorder
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("size-40"), TestID("area"),
			OnPointerDown(func(e tela.PointerEvent) { rec.add("down shift=%v", e.Shift) }),
			OnPointerUp(func(e tela.PointerEvent) { rec.add("up shift=%v", e.Shift) }))
	}, telatest.Size(200, 200))
	app.DragFromTo(tela.Point{X: 10, Y: 10}, tela.Point{X: 50, Y: 50}, telatest.Modifiers("Shift"))
	if got, want := rec.take(), "down shift=true,up shift=true"; got != want {
		t.Errorf("a drag with Shift: %q, want %q", got, want)
	}
}

func TestClickModifiersUnknown(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		var rec recorder
		app := telatest.MountFunc(tb, modsLog(&rec), telatest.Size(200, 100))
		app.Click(telatest.ByRole("button", "Target"), telatest.Modifiers("Ctrl"))
	}, `Modifiers("Ctrl")`, "ControlOrMeta")
}
