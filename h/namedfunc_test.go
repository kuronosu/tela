package h_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Named func types, which v0.6.0's handlers took (their parameter was a
// func type, assignable from a named one), satisfy the handler and Bind
// constraints (~func() | ~func(E), ~func(string)).
type (
	clickHandler func(tela.MouseEvent)
	action       func()
	keyHandler   func(tela.KeyboardEvent)
	textSink     func(string)
)

func TestNamedFuncHandlers(t *testing.T) {
	var got []string
	var onClick clickHandler = func(e tela.MouseEvent) { got = append(got, "click") }
	var onDouble action = func() { got = append(got, "dblclick") }
	var onKey keyHandler = func(e tela.KeyboardEvent) { got = append(got, "key "+string(e.Key)) }
	var sink textSink = func(s string) { got = append(got, "text "+s) }
	page := func() tela.Node {
		text, setText := tela.UseState("")
		return Div(
			Button(TestID("b"), OnClick(onClick), OnDoubleClick(onDouble), Text("b")),
			Input(AriaLabel("field"), OnKeyDown(onKey), Bind(text, textSink(func(s string) { sink(s); setText.Set(s) }))),
			// A plain literal still infers F.
			Button(TestID("plain"), OnClick(func() { got = append(got, "plain") }), Text("p")),
		)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.Click(telatest.ByTestID("b"))
	app.DoubleClick(telatest.ByTestID("b"))
	app.Click(telatest.ByTestID("plain"))
	app.Focus(telatest.ByRole("textbox", "field"))
	app.Type("x")
	app.ExpectValue(telatest.ByRole("textbox", "field"), "x")
	all := strings.Join(got, ",")
	for _, want := range []string{"click", "dblclick", "plain", "key x", "text x"} {
		if !strings.Contains(all, want) {
			t.Errorf("events %q lack %q", all, want)
		}
	}
	// A nil value of a named type panics like a nil func.
	if msg := panicText(func() { OnClick(clickHandler(nil)) }); !strings.Contains(msg, "OnClick(nil)") {
		t.Errorf("OnClick(clickHandler(nil)): %q", msg)
	}
	if msg := panicText(func() { OnDoubleClick(action(nil)) }); !strings.Contains(msg, "OnDoubleClick(nil)") {
		t.Errorf("OnDoubleClick(action(nil)): %q", msg)
	}
	if msg := panicText(func() { Bind("x", textSink(nil)) }); !strings.Contains(msg, "Bind(text, nil)") {
		t.Errorf("Bind(textSink(nil)): %q", msg)
	}
}
