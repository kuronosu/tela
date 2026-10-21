package h_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// tela.StopPropagation (ADR-097): the handler of every On… prop that stops
// the event before it calls f.

func TestStopPropagationClick(t *testing.T) {
	var parent, child, plain int
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("p-4"), OnClick(func() { parent++ }),
			Button(Class("block"), OnClick(tela.StopPropagation(func() { child++ })), Text("Stop")),
			Button(Class("block"), OnClick(func() { plain++ }), Text("Plain")))
	}, telatest.Size(300, 200))
	app.Click(telatest.ByRole("button", "Stop"))
	if parent != 0 || child != 1 {
		t.Errorf("click on the stopping button: parent %d, child %d, want 0 and 1", parent, child)
	}
	app.Click(telatest.ByRole("button", "Plain"))
	if parent != 1 || plain != 1 {
		t.Errorf("click on the plain button: parent %d, plain %d, want 1 and 1", parent, plain)
	}
}

func TestStopPropagationKey(t *testing.T) {
	var parent, child int
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(OnKeyDown(func() { parent++ }),
			Button(OnKeyDown(tela.StopPropagation(func() { child++ })), Text("A")),
			Button(Text("B")))
	}, telatest.Size(300, 200))
	app.Focus(telatest.ByRole("button", "A"))
	app.Press("x")
	if parent != 0 || child != 1 {
		t.Errorf("key on the stopping button: parent %d, child %d, want 0 and 1", parent, child)
	}
	app.Focus(telatest.ByRole("button", "B"))
	app.Press("x")
	if parent != 1 || child != 1 {
		t.Errorf("key on the plain button: parent %d, child %d, want 1 and 1", parent, child)
	}
}

func TestStopPropagationPointer(t *testing.T) {
	var parentDown, childDown, childUp, parentUp int
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(OnPointerDown(func() { parentDown++ }), OnMouseUp(func() { parentUp++ }),
			Button(OnPointerDown(tela.StopPropagation(func() { childDown++ })),
				OnMouseUp(tela.StopPropagation(func() { childUp++ })), Text("Press")))
	}, telatest.Size(300, 200))
	app.Click(telatest.ByRole("button", "Press"))
	if parentDown != 0 || childDown != 1 || parentUp != 0 || childUp != 1 {
		t.Errorf("pointer down/up: parent %d/%d, child %d/%d, want 0/0 and 1/1", parentDown, parentUp, childDown, childUp)
	}
}

// Every handler prop accepts the result, whatever its event type.
func TestStopPropagationCompilesInEveryHandler(t *testing.T) {
	f := tela.StopPropagation(func() {})
	n := Div(
		OnClick(f), OnDoubleClick(f), OnContextMenu(f), OnMouseDown(f), OnMouseUp(f), OnMouseMove(f), OnMouseEnter(f), OnMouseLeave(f),
		OnPointerDown(f), OnPointerMove(f), OnPointerUp(f), OnPointerCancel(f), OnPointerEnter(f), OnPointerLeave(f), OnLongPress(f),
		OnDragStart(f), OnDrag(f), OnDragEnd(f), OnWheel(f), OnScroll(f),
		OnKeyDown(f), OnKeyUp(f), OnFocus(f), OnBlur(f),
		OnChange(f), OnBeforeInput(f), OnCompositionStart(f), OnCompositionUpdate(f), OnCompositionEnd(f), OnSelect(f), OnSubmit(f))
	if n.IsZero() {
		t.Error("the node is zero")
	}
}

func TestStopPropagationNilPanics(t *testing.T) {
	msg := func(f func()) (m string) {
		defer func() {
			if r := recover(); r != nil {
				m, _ = r.(string)
			}
		}()
		f()
		return ""
	}
	if m := msg(func() { tela.StopPropagation(nil) }); !strings.Contains(m, "StopPropagation(nil)") || !strings.Contains(m, "OnClick(tela.StopPropagation(") {
		t.Errorf("StopPropagation(nil) panicked with %q", m)
	}
	var untyped func()
	if m := msg(func() { tela.StopPropagation(untyped) }); m == "" {
		t.Error("a nil func variable did not panic")
	}
	if m := msg(func() { OnClick(tela.StopHandler(nil)) }); !strings.Contains(m, "OnClick(nil)") {
		t.Errorf("a zero StopHandler panicked with %q", m)
	}
}
