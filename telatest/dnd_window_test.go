package telatest_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// twoWindows is a board whose zones live in a second window.
type twoWindows struct{ log *recorder }

func (w twoWindows) Build() tela.Node {
	return Div(Class("p-4"),
		Div(TestID("card"), Class("size-[60px] bg-indigo-500 text-white"), Draggable(true),
			OnDragStart(func(e tela.DragEvent) {
				e.DataTransfer.SetText("Card 7")
				w.log.add("start")
			}),
			OnDragEnd(func(e tela.DragEvent) { w.log.add("end:%v", e.DataTransfer.DropEffect) }),
			Text("Card 7")),
		tela.Component(tela.Window{
			Title: "Inbox", Width: 240, Height: 160,
			Root: dropZone{id: "inbox", log: w.log, accept: func(*tela.DataTransfer) bool { return true }},
		}),
	)
}

func TestDragToAnotherWindow(t *testing.T) {
	var rec recorder
	app := telatest.Mount(t, twoWindows{log: &rec}, telatest.Size(300, 200), telatest.Title("Main"), telatest.Screen(1000, 700))
	inbox := app.Window("Inbox")
	if got := app.DragToWindow(telatest.ByTestID("card"), inbox, telatest.ByTestID("inbox")); got != tela.DropCopy {
		t.Errorf("DragToWindow returned %v, want copy", got)
	}
	if got, want := rec.take(), `start,inbox:enter,inbox:drop card="" text="Card 7" html="" files=[],end:copy`; got != want {
		t.Errorf("events\n got %s\nwant %s", got, want)
	}

	// The same in steps: the drag moves between the windows, and the
	// windows see it come and go.
	app.StartDrag(telatest.ByTestID("card"))
	inbox.HoverDrag(telatest.ByTestID("inbox"))
	inbox.ExpectText("inbox ready")
	if i, _ := app.ActiveDrag(); i.Over != inbox || i.Source != app {
		t.Errorf("the drag is over %v from %v, want the inbox from the main window", i.Over, i.Source)
	}
	inbox.LeaveDrag()
	inbox.ExpectNoText("inbox ready")
	app.DropOutside(tela.DropCopy)
	if got := rec.take(); !strings.HasSuffix(got, "end:copy") {
		t.Errorf("events %s do not end with end:copy", got)
	}
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, twoWindows{log: &recorder{}}, telatest.Size(300, 200), telatest.Screen(1000, 700))
		app.DragToWindow(telatest.ByTestID("card"), nil, telatest.ByTestID("inbox"))
	}, "DragToWindow", "not a window of this app")
}

func TestDragHelpersFailClearly(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.StartDrag(telatest.ByTestID("plain"))
	}, "StartDrag", "did not start a drag", "h.Draggable(true)", "OnDragStart must put data")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.HoverDrag(telatest.ByTestID("zone-a"))
	}, "HoverDrag: no drag is in progress", "Carrying(payload)")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.CancelDrag()
	}, "CancelDrag: no drag is in progress")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.HoverDrag(telatest.ByTestID("zone-a"), telatest.Carrying(telatest.DragText("a")))
		app.HoverDrag(telatest.ByTestID("zone-a"), telatest.Carrying(telatest.DragText("b")))
	}, "Carrying(...) starts a drag, but one is already in progress")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.StartDrag(telatest.ByTestID("card"))
		app.StartDrag(telatest.ByTestID("card"))
	}, "StartDrag: a drag is already in progress")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.DropFiles(telatest.ByTestID("upload"), "relative.txt")
	}, "the payload is not valid", "absolute")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.DropData(telatest.ByTestID("upload"), telatest.DragPayload{})
	}, "the payload is empty")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.DropData(telatest.ByTestID("upload"), telatest.DragText("x"), telatest.Modifiers("Hyper"))
	}, `Modifiers("Hyper")`, "Shift")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.DropData(telatest.ByTestID("upload"), telatest.DragText("x"), telatest.Allowed())
	}, "Allowed(none) cannot start a drag")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.HoverDrag(telatest.ByTestID("upload"), telatest.Carrying(telatest.DragText("x")))
		app.DropOutside(tela.DropCopy)
	}, "DropOutside: the drag in progress is another application's")
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.StartDrag(telatest.ByTestID("card"))
		app.HoverDrag(telatest.ByTestID("zone-a"), telatest.Allowed(tela.DropCopy))
	}, "Allowed(...) applies to the drag Carrying(...) starts")
}

func TestDragPayloadAnd(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), `both payloads carry "text/plain;charset=utf-8"`) {
			t.Errorf("And with a repeated type panicked with %v", r)
		}
	}()
	telatest.DragText("a").And(telatest.DragText("b"))
}
