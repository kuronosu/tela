package telatest_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// notice shows a message and hides it 3 s after it appears, restarting when
// the message changes (tela.UseTimeout, ADR-171).
func notice() tela.Node {
	message, setMessage := tela.UseState("Saved")
	visible, setVisible := tela.UseState(true)
	tela.UseTimeout(3*time.Second, func() { setVisible.Set(false) }, message)
	return Div(
		If(visible, P(Text(message))),
		Button(OnClick(func() { setMessage.Set("Saved again"); setVisible.Set(true) }), Text("Save")),
	)
}

func TestUseTimeoutOnTheAppClock(t *testing.T) {
	app := telatest.MountFunc(t, notice, telatest.Size(300, 200))
	shown := func(text string) bool { return app.Count(telatest.ByText(text)) > 0 }
	if !shown("Saved") {
		t.Fatal("the notice is not shown")
	}
	app.Advance(2 * time.Second)
	if !shown("Saved") {
		t.Fatal("the notice was hidden before 3 s")
	}
	app.Advance(1100 * time.Millisecond)
	if shown("Saved") {
		t.Fatal("the notice is still shown 3 s after it appeared")
	}
	// A new message restarts the timer: shown again for 3 s.
	app.Click(telatest.ByRole("button", "Save"))
	app.ExpectText("Saved again")
	app.Advance(2 * time.Second)
	if !shown("Saved again") {
		t.Fatal("the restarted notice was hidden before 3 s")
	}
	app.Advance(1200 * time.Millisecond)
	if shown("Saved again") {
		t.Error("the restarted notice is still shown after 3 s")
	}
}
