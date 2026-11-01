package telatest_test

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// board is an app with a draggable card, a zone that takes cards and
// shows its drag-over state, a zone that refuses everything, and an upload
// zone that takes any other application's data. Everything the handlers
// see goes to log.
type board struct{ log *recorder }

func (b board) Build() tela.Node {
	return Div(Class("flex gap-4 p-4 w-full"),
		Div(TestID("card"), Class("size-[60px] bg-indigo-500 text-white"), Draggable(true),
			OnDragStart(func(e tela.DragEvent) {
				e.DataTransfer.SetData("application/x-card", "7")
				e.DataTransfer.SetText("Card 7")
				e.DataTransfer.EffectAllowed = tela.DropCopy | tela.DropMove
				b.log.add("start")
			}),
			OnDragEnd(func(e tela.DragEvent) { b.log.add("end:%v", e.DataTransfer.DropEffect) }),
			Text("Card 7")),
		Div(TestID("plain"), Class("size-[60px] bg-slate-500"), Text("Plain")),
		dropZone{id: "zone-a", log: b.log, accept: func(dt *tela.DataTransfer) bool { return dt.Has("application/x-card") }},
		dropZone{id: "zone-b", log: b.log, accept: func(*tela.DataTransfer) bool { return false }},
		dropZone{id: "upload", log: b.log, accept: func(*tela.DataTransfer) bool { return true }},
	)
}

// dropZone shows "<id> ready" while a drag it accepts is over it.
type dropZone struct {
	id     string
	log    *recorder
	accept func(*tela.DataTransfer) bool
}

func (z dropZone) Build() tela.Node {
	over, setOver := tela.UseState(false)
	label := z.id
	if over {
		label += " ready"
	}
	return Div(TestID(z.id), Class("w-[90px] h-[90px] bg-emerald-200 text-black"),
		OnDragEnter(func() { setOver.Set(true); z.log.add("%s:enter", z.id) }),
		OnDragOver(func(e tela.DragEvent) {
			// A drop reads the custom types its handlers looked up.
			e.DataTransfer.Has("application/x-card")
			if z.accept(e.DataTransfer) {
				e.PreventDefault()
			}
		}),
		OnDragLeave(func() { setOver.Set(false); z.log.add("%s:leave", z.id) }),
		OnDrop(func(e tela.DragEvent) {
			setOver.Set(false)
			dt := e.DataTransfer
			z.log.add("%s:drop card=%q text=%q html=%q files=%v", z.id, dt.GetData("application/x-card"), dt.Text(), dt.HTML(), dt.Files())
			if img, ok := dt.Image(); ok {
				b := img.Bounds()
				z.log.add("%s:image %dx%d", z.id, b.Dx(), b.Dy())
			}
		}),
		Text(label))
}

func mountBoard(t testing.TB, rec *recorder, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(520, 200)}, opts...)
	return telatest.Mount(t, board{log: rec}, opts...)
}

func TestDragToDropsACard(t *testing.T) {
	var rec recorder
	app := mountBoard(t, &rec)
	// Without modifiers the platform's convention copies.
	if got := app.DragTo(telatest.ByTestID("card"), telatest.ByTestID("zone-a")); got != tela.DropCopy {
		t.Errorf("DragTo returned %v, want copy", got)
	}
	if got, want := rec.take(), `start,zone-a:enter,zone-a:drop card="7" text="Card 7" html="" files=[],end:copy`; got != want {
		t.Errorf("events\n got %s\nwant %s", got, want)
	}
	app.ExpectText("zone-a")
	// Shift moves.
	if got := app.DragTo(telatest.ByTestID("card"), telatest.ByTestID("zone-a"), telatest.Modifiers("Shift")); got != tela.DropMove {
		t.Errorf("DragTo with Shift returned %v, want move", got)
	}
	if got := rec.take(); !strings.HasSuffix(got, "end:move") {
		t.Errorf("events %s do not end with end:move", got)
	}
	// The pointer works again afterwards.
	app.Hover(telatest.ByTestID("plain"))
}

func TestDragToARefusingTarget(t *testing.T) {
	var rec recorder
	app := mountBoard(t, &rec)
	if got := app.DragTo(telatest.ByTestID("card"), telatest.ByTestID("zone-b")); got != tela.DropNone {
		t.Errorf("DragTo a refusing zone returned %v, want none", got)
	}
	// A refused drop is a leave, and the source learns it. The pointer
	// went through zone-a on the way, as a user's would.
	if got, want := rec.take(), "start,zone-a:enter,zone-b:enter,zone-a:leave,zone-b:leave,end:none"; got != want {
		t.Errorf("events\n got %s\nwant %s", got, want)
	}
	app.ExpectText("zone-b")
}

func TestStartDragInspectHoverLeaveCancel(t *testing.T) {
	var rec recorder
	app := mountBoard(t, &rec)
	if _, ok := app.ActiveDrag(); ok {
		t.Fatal("ActiveDrag reports a drag before any")
	}
	info := app.StartDrag(telatest.ByTestID("card"))
	if info.External || info.Source != app || info.Over != app {
		t.Errorf("drag info %+v: want an app drag from and over the main window", info)
	}
	if info.Allowed != tela.DropCopy|tela.DropMove || info.Data.EffectAllowed != info.Allowed {
		t.Errorf("allowed %v / %v, want copy|move", info.Allowed, info.Data.EffectAllowed)
	}
	if info.Data.GetData("application/x-card") != "7" || info.Data.Text() != "Card 7" {
		t.Errorf("data %v: want the card and its text", info.Data.Types())
	}
	again, ok := app.ActiveDrag()
	if !ok || again.Data.Text() != "Card 7" {
		t.Errorf("ActiveDrag = %v, %v", again, ok)
	}

	// Drag-over styling: the text changes with the drag, and the drag
	// leaving an element runs leave there and enter on the next.
	app.HoverDrag(telatest.ByTestID("zone-a"))
	app.ExpectText("zone-a ready")
	app.HoverDrag(telatest.ByTestID("zone-b"))
	app.ExpectNoText("zone-a ready")
	if got := rec.take(); !strings.Contains(got, "zone-a:enter") || !strings.Contains(got, "zone-a:leave,zone-b:enter") &&
		!strings.Contains(got, "zone-b:enter,zone-a:leave") {
		t.Errorf("events %s: want zone-a entered and left, zone-b entered", got)
	}
	app.HoverDrag(telatest.ByTestID("zone-a"))
	app.LeaveDrag()
	app.ExpectNoText("zone-a ready")
	if i, ok := app.ActiveDrag(); !ok || i.Over != nil {
		t.Errorf("after LeaveDrag the drag is %v, over %v", ok, i.Over)
	}
	rec.take()
	app.CancelDrag()
	if got := rec.take(); got != "end:none" {
		t.Errorf("CancelDrag events %q, want the source's end with none", got)
	}
	if _, ok := app.ActiveDrag(); ok {
		t.Error("the drag is still in progress after CancelDrag")
	}
}

func TestDropOutside(t *testing.T) {
	var rec recorder
	app := mountBoard(t, &rec)
	app.StartDrag(telatest.ByTestID("card"))
	app.HoverDrag(telatest.ByTestID("zone-a"))
	rec.take()
	app.DropOutside(tela.DropMove)
	if got := rec.take(); got != "zone-a:leave,end:move" {
		t.Errorf("events %q, want zone-a left and the end with move", got)
	}
	// A refusal by the other application.
	app.StartDrag(telatest.ByTestID("card"))
	rec.take()
	app.DropOutside(tela.DropNone)
	if got := rec.take(); got != "end:none" {
		t.Errorf("events %q, want the end with none", got)
	}
	mustFail(t, func(tb testing.TB) {
		app := mountBoard(tb, &recorder{})
		app.StartDrag(telatest.ByTestID("card"))
		app.DropOutside(tela.DropLink)
	}, "DropOutside(link)", "allows only copy|move")
}

func TestDropFilesAndData(t *testing.T) {
	var rec recorder
	app := mountBoard(t, &rec)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b c.png")
	if got := app.DropFiles(telatest.ByTestID("upload"), a, b); got != tela.DropCopy {
		t.Errorf("DropFiles returned %v, want copy", got)
	}
	want := `upload:enter,upload:drop card="" text="" html="" files=[` + a + ` ` + b + `]`
	if got := rec.take(); got != want {
		t.Errorf("events\n got %s\nwant %s", got, want)
	}

	// Several representations, a custom type and a PNG.
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	payload := telatest.DragHTML("<b>Hi</b>").And(telatest.DragText("Hi")).And(telatest.DragType("application/x-card", "9")).And(telatest.DragPNG(img))
	if got := app.DropData(telatest.ByTestID("upload"), payload); got != tela.DropCopy {
		t.Errorf("DropData returned %v, want copy", got)
	}
	want = `upload:enter,upload:drop card="9" text="Hi" html="<b>Hi</b>" files=[],upload:image 3x2`
	if got := rec.take(); got != want {
		t.Errorf("events\n got %s\nwant %s", got, want)
	}

	// The zone that refuses, and one that wants a type the drag lacks.
	if got := app.DropFiles(telatest.ByTestID("zone-b"), a); got != tela.DropNone {
		t.Errorf("a refusing zone returned %v, want none", got)
	}
	if got := app.DropData(telatest.ByTestID("zone-a"), telatest.DragText("x")); got != tela.DropNone {
		t.Errorf("a zone that wants cards took text: %v", got)
	}
	if got := rec.take(); got != "zone-b:enter,zone-b:leave,zone-a:enter,zone-a:leave" {
		t.Errorf("events %q", got)
	}
}

func TestDropAllowedAndModifiers(t *testing.T) {
	var rec recorder
	app := mountBoard(t, &rec)
	for _, tc := range []struct {
		name    string
		opts    []telatest.DragOption
		want    tela.DropEffect
		refused bool
	}{
		{"move only", []telatest.DragOption{telatest.Allowed(tela.DropMove)}, tela.DropMove, false},
		{"link only", []telatest.DragOption{telatest.Allowed(tela.DropLink)}, tela.DropLink, false},
		{"ctrl", []telatest.DragOption{telatest.Modifiers("Control")}, tela.DropCopy, false},
		{"shift", []telatest.DragOption{telatest.Modifiers("Shift")}, tela.DropMove, false},
		{"ctrl+shift", []telatest.DragOption{telatest.Modifiers("Control", "Shift")}, tela.DropLink, false},
		{"shift but move not allowed", []telatest.DragOption{telatest.Modifiers("Shift"), telatest.Allowed(tela.DropCopy)}, tela.DropCopy, false},
	} {
		got := app.DropData(telatest.ByTestID("upload"), telatest.DragText("x"), tc.opts...)
		if got != tc.want {
			t.Errorf("%s: effect %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestHoverDragExternalThenLeaveOrCancel(t *testing.T) {
	var rec recorder
	app := mountBoard(t, &rec)
	zone := telatest.ByTestID("upload")
	app.HoverDrag(zone, telatest.Carrying(telatest.DragFiles(filepath.Join(t.TempDir(), "x.txt"))), telatest.Allowed(tela.DropCopy))
	app.ExpectText("upload ready")
	info, ok := app.ActiveDrag()
	if !ok || !info.External || info.Source != nil || info.Over != app || info.Allowed != tela.DropCopy || len(info.Data.Files()) != 1 {
		t.Errorf("drag info %+v (%v)", info, ok)
	}
	app.LeaveDrag()
	app.ExpectNoText("upload ready")
	// It can come back, and then it is cancelled.
	app.HoverDrag(zone)
	app.ExpectText("upload ready")
	app.CancelDrag()
	app.ExpectNoText("upload ready")
	if _, ok := app.ActiveDrag(); ok {
		t.Error("the drag is still in progress after CancelDrag")
	}
	if got, want := rec.take(), "upload:enter,upload:leave,upload:enter,upload:leave"; got != want {
		t.Errorf("events\n got %s\nwant %s", got, want)
	}
}
