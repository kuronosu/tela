package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

func TestTextSelDragAndDrop(t *testing.T) {
	x := newKeys(t, twoParas(), 400, 200)
	tr := x.tree()
	x.drag(textAt(t, tr, "a", 6, 0), textAt(t, tr, "b", 6, 0))
	want := tr.SelectedText()
	// A click on the selection clears it.
	x.clickAt(textAt(t, tr, "a", 8, 0))
	if got := tr.SelectedText(); got != "" {
		t.Errorf("a click on the selection left %q", got)
	}
	x.drag(textAt(t, tr, "a", 6, 0), textAt(t, tr, "b", 6, 0))
	// A press on it that moves past the slop drags it.
	p := textAt(t, tr, "a", 8, 0)
	x.send(move(p.X, p.Y), down(p.X, p.Y), moveHeld(p.X+3, p.Y))
	if got := tr.SelectedText(); got != want {
		t.Fatalf("a press on the selection changed it to %q", got)
	}
	x.send(moveHeld(p.X+20, p.Y+10))
	info, ok := x.b.Drag()
	if !ok {
		t.Fatal("no drag and drop")
	}
	if s, _ := info.Data.Text(); s != want || info.Allowed != platform.DropCopy {
		t.Errorf("drag data %q allowed %v, want %q copy", s, info.Allowed, want)
	}
	if info.Image == nil {
		t.Error("no drag image")
	}
	if got := tr.SelectedText(); got != want {
		t.Errorf("the drag changed the selection to %q", got)
	}
}

func TestTextSelGestures(t *testing.T) {
	var drags int
	root := div(Class("p-4 w-80"),
		p(Key("a"), Text("Hello brave world")),
		div(Key("handle"), On("drag", func(DragEvent) { drags++ }), Text("Drag handle text")),
		div(Key("dnd"), Draggable(true), Text("Draggable text")),
		div(Key("press"), On("pointerdown", func(e PointerEvent) { NoTextSelection(e) }), Text("Pressable text")))
	x := newKeys(t, root, 400, 200)
	tr := x.tree()
	x.drag(textAt(t, tr, "a", 0, 0), textAt(t, tr, "a", 5, 0))
	for _, k := range []string{"handle", "press"} {
		x.drag(textAt(t, tr, k, 0, 0), textAt(t, tr, k, 10, 0))
		if got := tr.SelectedText(); got != "Hello" {
			t.Errorf("a drag on %s changed the selection to %q", k, got)
		}
	}
	if drags == 0 {
		t.Error("the drag handle did not drag")
	}
	x.clickAt(textAt(t, tr, "dnd", 2, 0))
	if got := tr.SelectedText(); got != "Hello" {
		t.Errorf("a click on a Draggable changed the selection to %q", got)
	}
}

func TestTextSelMacShiftClick(t *testing.T) {
	x := newFlavorKeys(t, twoParas(), platform.FlavorMac)
	tr := x.tree()
	x.drag(textAt(t, tr, "a", 6, 0), textAt(t, tr, "a", 17, 0)) // "brave world"
	shiftAt := func(p gfx.Point) {
		x.w.Inject(platform.PointerEvent{Action: platform.PointerDown, Pos: platform.Point{X: p.X, Y: p.Y}, Button: platform.ButtonPrimary,
			Buttons: platform.ButtonPrimary.Mask(), Mods: platform.ModShift})
		x.w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: p.X, Y: p.Y}, Button: platform.ButtonPrimary, Mods: platform.ModShift})
		x.b.Drain(x.h)
		x.b.Advance(time.Second)
	}
	// Inside, nearer the end: the start stays (non-directional).
	shiftAt(textAt(t, tr, "a", 14, 0))
	if got := tr.SelectedText(); got != "brave wo" {
		t.Errorf("mac shift+click near the end: %q", got)
	}
	// Before the start: the end stays.
	shiftAt(textAt(t, tr, "a", 0, 0))
	if got := tr.SelectedText(); got != "Hello brave wo" {
		t.Errorf("mac shift+click before: %q", got)
	}
	// Command+C copies, Command+A selects all.
	x.chord("c", platform.ModSuper)
	if got := x.readClip(); got != "Hello brave wo" {
		t.Errorf("Command+C copied %q", got)
	}
	x.chord("a", platform.ModSuper)
	if got := tr.SelectedText(); got != "Hello brave world\nSecond paragraph here" {
		t.Errorf("Command+A: %q", got)
	}
}

func TestTextSelRoots(t *testing.T) {
	root := div(Class("p-4 w-80"),
		p(Key("a"), Text("Main content text")),
		Portal(div(Class("fixed top-24 left-4 w-60 bg-white"), p(Key("o"), Text("Overlay text here")))))
	x := newKeys(t, root, 400, 300)
	tr := x.tree()
	// A drag from the content over the overlay stays in the content: the
	// point gives the position of the content under it (the end of the
	// line above).
	o := windowRect(keyed(tr, "o").box)
	x.drag(textAt(t, tr, "a", 5, 0), gfx.Point{X: o.X1 - 2, Y: o.Center().Y})
	if got := tr.SelectedText(); got != "content text" {
		t.Errorf("content over the overlay: %q", got)
	}
	x.drag(textAt(t, tr, "o", 0, 0), textAt(t, tr, "o", 7, 0))
	if got := tr.SelectedText(); got != "Overlay" {
		t.Errorf("in the overlay: %q", got)
	}
	// Ctrl+A with nothing focused selects the root of the selection.
	x.ctrl("a", 0)
	if got := tr.SelectedText(); got != "Overlay text here" {
		t.Errorf("Ctrl+A with the selection in the overlay: %q", got)
	}
}

func TestTextSelRemovedAndChanged(t *testing.T) {
	var set Setter[int]
	app := func() Node {
		n, s := UseState(0)
		set = s
		switch n {
		case 0:
			return div(Class("p-4 w-80"), p(Key("a"), Text("Hello brave world")), p(Key("b"), Text("Second")))
		case 1:
			return div(Class("p-4 w-80"), p(Key("a"), Text("Hi")), p(Key("b"), Text("Second")))
		}
		return div(Class("p-4 w-80"), p(Key("b"), Text("Second")))
	}
	x := newKeys(t, Component(app), 400, 200)
	tr := x.tree()
	x.drag(textAt(t, tr, "a", 6, 0), textAt(t, tr, "a", 17, 0))
	set.Set(1) // the text is shorter: the cursors are clamped
	x.settle()
	if got := tr.SelectedText(); got != "" {
		t.Errorf("after the text shrank: %q", got)
	}
	x.drag(textAt(t, tr, "a", 0, 0), textAt(t, tr, "b", 3, 0))
	if got := tr.SelectedText(); got != "Hi\nSec" {
		t.Fatalf("selected %q", got)
	}
	set.Set(2) // the anchor's paragraph unmounts
	x.settle()
	if got := tr.SelectedText(); got != "" || len(tr.tsel.hl) != 0 {
		t.Errorf("after the anchor unmounted: %q, %d highlights", got, len(tr.tsel.hl))
	}
}

func TestTextSelContextMenuKey(t *testing.T) {
	var at []gfx.Point
	var keyboard []bool
	root := div(Class("p-4 w-80"),
		div(Key("area"), On("contextmenu", func(e MouseEvent) {
			at = append(at, gfx.Point{X: e.X, Y: e.Y})
			keyboard = append(keyboard, KeyboardContextMenu(e))
		}),
			p(Key("a"), Text("Hello brave world"))),
		button(Key("btn"), Text("Other")))
	x := newKeys(t, root, 400, 200)
	tr := x.tree()
	x.drag(textAt(t, tr, "a", 6, 0), textAt(t, tr, "a", 11, 0))
	x.key(platform.KeyContextMenu, 0)
	start := textAt(t, tr, "a", 6, 0)
	if len(at) != 1 || !keyboard[0] || at[0].X < start.X-0.5 || at[0].X > start.X+0.5 || at[0].Y < start.Y-1 || at[0].Y > start.Y+1 {
		t.Fatalf("the ContextMenu key fired %v (keyboard %v), want one at the selection's start %v", at, keyboard, start)
	}
	x.key(platform.KeyF10, platform.ModShift)
	if len(at) != 2 {
		t.Errorf("Shift+F10 fired %d", len(at)-1)
	}
	// With the focus on an element outside the selection, the menu is
	// not the selection's.
	x.tab()
	if focusedName(tr) != "btn" {
		t.Fatalf("focus on %s", focusedName(tr))
	}
	x.key(platform.KeyContextMenu, 0)
	if len(at) != 2 {
		t.Errorf("the key with the focus elsewhere fired at the selection: %v", at)
	}
}

func TestTextSelAutoscroll(t *testing.T) {
	lines := []Child{Class("h-32 w-60 overflow-auto"), Key("sc")}
	for i := range 30 {
		lines = append(lines, p(Key(i), Textf("Line number %d", i)))
	}
	root := div(Class("p-4"), div(lines...))
	x := newKeys(t, root, 300, 300)
	tr := x.tree()
	s := keyed(tr, "sc").box.scr
	a := textAt(t, tr, 0, 2, 0)
	r := windowRect(keyed(tr, "sc").box)
	x.send(move(a.X, a.Y), down(a.X, a.Y), moveHeld(a.X, r.Y1+30))
	for range 5 {
		x.frame()
	}
	if s.pos.Y <= 0 {
		t.Fatalf("a drag below the scroll container did not scroll it: %v", s.pos)
	}
	x.send(up(a.X, r.Y1+30))
	got := tr.SelectedText()
	if !strings.HasPrefix(got, "ne number 0\nLine number 1\n") || strings.Count(got, "\n") < 8 {
		t.Errorf("the autoscrolled selection is %q", got)
	}
	x.settle() // frames stop once it is released
}

func TestTextSelGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			root := div(Class("flex h-full flex-col gap-3 bg-white p-5 text-slate-800"),
				h1(Key("h"), Class("text-xl font-semibold"), Text("Selectable text")),
				p(Key("a"), Class("text-sm leading-6"), Text("A selection crosses paragraphs: it starts here, "),
					code(Class("rounded bg-slate-100 px-1 font-mono"), Text("inline code")), Text(" keeps its highlight over decorations, and")),
				p(Key("b"), Class("text-sm leading-6"), Text("ends in the middle of the second paragraph, whose rest stays plain.")),
				p(Key("c"), Class("text-sm leading-6 selection:bg-amber-200 selection:text-amber-900"), Text("Classes set selection colors.")))
			b := headless.New(headless.Options{Size: platform.Size{W: 340, H: 240}, Scale: scale})
			host, err := NewHost(b, Options{Config: Config{Fonts: testFonts, Flavor: testFlavor}, Root: root})
			if err != nil {
				t.Fatal(err)
			}
			b.Drain(host)
			x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
			t.Cleanup(host.stopTimer)
			x.w.SetFocused(true)
			x.b.Drain(x.h)
			tr := x.tree()
			x.drag(textAt(t, tr, "a", 31, 0), textAt(t, tr, "b", 26, 0))
			x.settle()
			golden.Image(t, fmt.Sprintf("text_selection_x%g", scale), x.w.Frame())
			x.drag(textAt(t, tr, "c", 0, 0), textAt(t, tr, "c", 25, 0))
			x.settle()
			golden.Image(t, fmt.Sprintf("text_selection_classes_x%g", scale), x.w.Frame())
		})
	}
}

// A selection change only paints: no build, style or layout, and the
// damage covers the text whose highlight changed.
func TestTextSelPaintsOnly(t *testing.T) {
	rows := make([]Child, 0, 100)
	for i := range 100 {
		rows = append(rows, p(Key(i), Class("p-1"), Text(fmt.Sprintf("row %d of the list", i))))
	}
	x := newKeys(t, div(rows...), 400, 300)
	tr := x.tree()
	a, b := textAt(t, tr, 5, 0, 0), textAt(t, tr, 5, 3, 0)
	x.send(move(a.X, a.Y), down(a.X, a.Y), moveHeld(b.X, b.Y))
	x.settle()
	s0 := tr.Stats()
	c := textAt(t, tr, 5, 9, 0)
	x.send(moveHeld(c.X, c.Y))
	x.settle()
	s1 := tr.Stats()
	if s1.Layouts != s0.Layouts || s1.Builds != s0.Builds || s1.Styles != s0.Styles {
		t.Errorf("a selection drag: layouts +%d builds +%d styles +%d, want none",
			s1.Layouts-s0.Layouts, s1.Builds-s0.Builds, s1.Styles-s0.Styles)
	}
	row := windowRect(keyed(tr, 5).box)
	for _, d := range x.w.Damage() {
		if float32(d.Min.Y) < row.Y0-1 || float32(d.Max.Y) > row.Y1+1 {
			t.Errorf("damage %v outside the row %v", d, row)
		}
	}
	x.send(up(c.X, c.Y))
	if got := tr.SelectedText(); got != "row 5 of " {
		t.Errorf("selected %q", got)
	}
}

// user-select: all selects an element's content as a whole (Chromium's
// ExpandSelectionToRespectUserSelectAll).
func TestTextSelUserSelectAll(t *testing.T) {
	root := div(Class("p-4 w-96"),
		p(Key("a"), Text("Your key: "), span(Key("key"), Class("select-all"), Text("abc-123-xyz")), Text(" is shown once.")),
		div(Key("blk"), Class("select-all"), p(Key("b1"), Text("First line")), p(Key("b2"), Text("Second line"))))
	x := newKeys(t, root, 400, 200)
	tr := x.tree()
	x.clickAt(textAt(t, tr, "key", 14, 0))
	if got := tr.SelectedText(); got != "abc-123-xyz" {
		t.Errorf("a click in a select-all span selected %q", got)
	}
	x.clickAt(textAt(t, tr, "b2", 2, 0))
	if got := tr.SelectedText(); got != "First line\nSecond line" {
		t.Errorf("a click in a select-all block selected %q", got)
	}
	// A drag into it takes all of it.
	x.clickAt(textAt(t, tr, "a", 0, 0))
	x.drag(textAt(t, tr, "a", 0, 0), textAt(t, tr, "key", 13, 0))
	if got := tr.SelectedText(); got != "Your key: abc-123-xyz" {
		t.Errorf("a drag into a select-all span selected %q", got)
	}
}

// The drag image of a selection across two paragraphs: their selected
// text alone, at 75%, in its own colors.
func TestTextSelDragImageGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 200}, Scale: scale})
			host, err := NewHost(b, Options{Config: Config{Fonts: testFonts, Flavor: testFlavor}, Root: twoParas()})
			if err != nil {
				t.Fatal(err)
			}
			b.Drain(host)
			x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
			t.Cleanup(host.stopTimer)
			x.w.SetFocused(true)
			x.b.Drain(x.h)
			tr := x.tree()
			x.drag(textAt(t, tr, "a", 6, 0), textAt(t, tr, "b", 6, 0))
			p := textAt(t, tr, "a", 8, 0)
			x.send(move(p.X, p.Y), down(p.X, p.Y), moveHeld(p.X+20, p.Y+10))
			info, ok := x.b.Drag()
			if !ok || info.Image == nil {
				t.Fatal("no drag image")
			}
			for i := 0; i < len(info.Image.Pix); i += 4 {
				if px := info.Image.Pix[i : i+4]; px[3] > 192 || px[2] > px[0]+24 {
					t.Fatalf("pixel %v: not the text at 75%% (the highlight)", px)
				}
			}
			golden.Image(t, fmt.Sprintf("text_drag_image_x%g", scale), info.Image)
		})
	}
}
