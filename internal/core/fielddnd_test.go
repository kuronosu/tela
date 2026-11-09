package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// Drag and drop of text fields (fielddnd.go): the drop selects the text,
// the drop caret, and the selection as a drag source.

// fieldAt returns the window point of the caret at the value offset off
// of the single-line field f, at mid-height, moved right by dx.
func fieldAt(f *field, off int, dx float32) (float32, float32) {
	r := windowRect(f.bx)
	ox, _ := f.textOrigin()
	return r.X0 + ox + f.layout().Caret(textCursor(off)).X + dx, r.Center().Y
}

func selectionOf(f *field) [2]int {
	s, e, _ := f.ed.SelectionRange()
	return [2]int{s, e}
}

// textLog records the editing and drag source events of a field.
func textLog(log *[]string) []Child {
	add := func(s string) { *log = append(*log, s) }
	return []Child{
		On("beforeinput", func(e InputEvent) { add("beforeinput:" + e.InputType) }),
		On("change", func(e ChangeEvent) { add("change:" + e.InputType) }),
		On("dragstart", func(e DragEvent) { add("dragstart:" + e.DataTransfer.Text()) }),
		On("dragend", func(e DragEvent) { add("dragend:" + e.DataTransfer.DropEffect.String()) }),
	}
}

// A drop of text into a field selects the inserted text, as Chromium
// does, and the field takes the focus.
func TestTextDropSelectsTheText(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(Class("p-4"), input(Key("in"), Class("w-60"), DefaultValue("ab")))})
	w := x.windows()[0]
	x.b.StartExternalDrag(platform.Data{{Type: platform.MIMEText, Bytes: []byte("XY")}}, platform.DropCopy)
	w.Inject(move(200, 26))
	x.drain()
	w.Inject(up(200, 26))
	x.drain()
	f := fieldOf(t, x.h.Tree(), "in")
	if v, sel := f.ed.Value(), selectionOf(f); v != "abXY" || sel != [2]int{2, 4} {
		t.Errorf("value %q selection %v, want abXY with XY (2..4) selected", v, sel)
	}
	if x.h.Tree().foc.el != f.el {
		t.Error("the field is not focused after the drop")
	}
}

// While a drag of text is over an editable field, the field paints a
// drop caret where the drop would insert; its selection does not change;
// the caret goes away when the drag leaves the field, the window, or
// drops. A dragover handler that takes the drag over, and a read-only
// field, show none.
func TestDropCaret(t *testing.T) {
	var prevent bool
	root := div(Class("p-4 flex flex-col gap-4"),
		input(Key("in"), Class("w-64"), DefaultValue("hello brave new world"),
			On("dragover", func(e DragEvent) {
				if prevent {
					e.PreventDefault()
				}
			})),
		input(Key("ro"), Class("w-64"), DefaultValue("read only"), ReadOnly(true)))
	x := newWinHarness(t, Options{Root: root})
	w := x.windows()[0]
	tr := x.h.Tree()
	f, ro := fieldOf(t, tr, "in"), fieldOf(t, tr, "ro")
	px, py := fieldAt(f, 0, 1)
	x.click(w, px, py)
	tr.fieldChanged(f, f.ed.Select(0, 5))
	x.drain()
	caret := func() string {
		if c, ok := tr.dropCaret(f); ok {
			return fmt.Sprint("in:", c.Offset)
		}
		if c, ok := tr.dropCaret(ro); ok {
			return fmt.Sprint("ro:", c.Offset)
		}
		return "none"
	}
	x.b.StartExternalDrag(platform.Data{{Type: platform.MIMEText, Bytes: []byte("XY")}}, platform.DropCopy)
	cx, cy := fieldAt(f, 11, 1)
	w.Inject(move(cx, cy))
	x.drain()
	if got := caret(); got != "in:11" {
		t.Errorf("drop caret %s over the field, want at 11", got)
	}
	if sel := selectionOf(f); sel != [2]int{0, 5} {
		t.Errorf("the hover changed the selection to %v", sel)
	}
	cx2, cy2 := fieldAt(f, 16, 1)
	w.Inject(move(cx2, cy2))
	x.drain()
	if got := caret(); got != "in:16" {
		t.Errorf("drop caret %s after a move, want at 16", got)
	}
	w.Inject(move(390, 290)) // off the fields
	x.drain()
	if got := caret(); got != "none" {
		t.Errorf("drop caret %s off the field", got)
	}
	rx, ry := fieldAt(ro, 2, 1)
	w.Inject(move(rx, ry))
	x.drain()
	if got := caret(); got != "none" {
		t.Errorf("drop caret %s over a read-only field", got)
	}
	w.Inject(move(cx, cy))
	x.drain()
	w.Inject(ptr(platform.PointerLeave, cx, cy, 0, 0))
	x.drain()
	if got := caret(); got != "none" {
		t.Errorf("drop caret %s after the drag left the window", got)
	}
	prevent = true
	w.Inject(move(cx, cy))
	x.drain()
	if eff, _ := w.DropAnswer(); eff != platform.DropCopy {
		t.Errorf("answer %v with a dragover handler's PreventDefault", eff)
	}
	if got := caret(); got != "none" {
		t.Errorf("drop caret %s while a handler takes the drag over", got)
	}
	prevent = false
	w.Inject(move(cx+1, cy))
	x.drain()
	if got := caret(); got != "in:11" {
		t.Errorf("drop caret %s once the handler lets the field take it", got)
	}
	w.Inject(up(cx+1, cy))
	x.drain()
	if got := caret(); got != "none" {
		t.Errorf("drop caret %s after the drop", got)
	}
	if v := f.ed.Value(); v != "hello braveXY new world" {
		t.Errorf("value %q after the drop", v)
	}
}

// The drop caret as painted: a field that keeps its selection ("Drop",
// highlighted) and the caret of a drag of text before "here".
func TestDropCaretGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			cls := "w-full rounded-md border border-slate-300 px-3 py-2 text-slate-900"
			root := div(Class("flex h-full flex-col gap-3 bg-slate-50 p-4"),
				input(Key("in"), Class(cls), DefaultValue("Drop the text here")),
				textarea(Key("ta"), Class(cls+" caret-pink-600"), Rows(3), DefaultValue("A textarea with\nthe drop caret\non its second line")))
			x := newBackendHarness(t, headless.Options{Size: platform.Size{W: 280, H: 180}, Scale: scale},
				Options{Config: Config{Flavor: testFlavor}, Root: root})
			w := x.windows()[0]
			w.SetFocused(true)
			tr := x.h.Tree()
			f := fieldOf(t, tr, "in")
			px, py := fieldAt(f, 0, 1)
			x.click(w, px, py)
			tr.fieldChanged(f, f.ed.Select(0, 4))
			x.drain()
			x.b.StartExternalDrag(platform.Data{{Type: platform.MIMEText, Bytes: []byte("XY")}}, platform.DropCopy)
			cx, cy := fieldAt(f, 14, 1)
			w.Inject(move(cx, cy))
			x.drain()
			if c, ok := tr.dropCaret(f); !ok || c.Offset != 14 {
				t.Fatalf("drop caret %v %v, want before here (14)", c, ok)
			}
			golden.Image(t, fmt.Sprintf("field_drop_caret_x%g", scale), w.Frame())
			// In the textarea: its caret color, on the second line.
			ta := fieldOf(t, tr, "ta")
			r := windowRect(ta.bx)
			ox, oy := ta.textOrigin()
			c := ta.layout().Caret(textCursor(len("A textarea with\nthe ")))
			w.Inject(move(r.X0+ox+c.X+1, r.Y0+oy+c.Top+c.Height/2))
			x.drain()
			if c, ok := tr.dropCaret(ta); !ok || c.Offset != len("A textarea with\nthe ") {
				t.Fatalf("drop caret %v %v in the textarea", c, ok)
			}
			golden.Image(t, fmt.Sprintf("field_drop_caret_textarea_x%g", scale), w.Frame())
			x.b.CancelDrag()
			x.drain()
			if _, ok := tr.dropCaret(ta); ok {
				t.Error("the drop caret stays after the drag was cancelled")
			}
		})
	}
}

// selDrag sets up the field "a" of x focused with "brave " (6..12) of
// "hello brave new world" selected, presses on "brave" and moves past the
// drag slop with mods held.
func selDrag(t *testing.T, x *winHarness, w *headless.Window, mods platform.Modifiers) *field {
	t.Helper()
	tr := x.h.Tree()
	f := fieldOf(t, tr, "a")
	px, py := fieldAt(f, 0, 1)
	x.click(w, px, py)
	x.b.Advance(time.Second) // no double click
	tr.fieldChanged(f, f.ed.Select(6, 12))
	x.drain()
	px, py = fieldAt(f, 8, 1)
	w.Inject(move(px, py))
	w.Inject(down(px, py))
	x.drain()
	if sel := selectionOf(f); sel != [2]int{6, 12} {
		t.Fatalf("the press on the selection changed it to %v", sel)
	}
	w.Inject(withMods(moveHeld(px+10, py), mods))
	x.drain()
	return f
}

const selValue = "hello brave new world"

func selRoot(log *[]string, more ...Child) Node {
	kids := append([]Child{Class("p-4 flex flex-col gap-4"),
		input(append([]Child{Key("a"), Class("w-64"), DefaultValue(selValue)}, textLog(log)...)...)}, more...)
	return div(kids...)
}

// A press on a field's selection and a drag start an OS drag of the
// selected text; a drop in the same field moves the text (deleteByDrag,
// then insertFromDrop at the drop point, selected), and the copy
// modifier copies it instead: Ctrl, or Option under the Mac flavor. A
// drop on the dragged text itself is refused.
func TestSelectionDragInTheSameField(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flavor platform.Flavor
		mods   platform.Modifiers
		to     int
		value  string
		sel    [2]int
		events string
	}{
		{"move to the end", platform.FlavorLinux, 0, 21, "hello new worldbrave ", [2]int{15, 21},
			"dragstart:brave  beforeinput:deleteByDrag change:deleteByDrag beforeinput:insertFromDrop change:insertFromDrop dragend:move"},
		{"move to the start", platform.FlavorLinux, 0, 0, "brave hello new world", [2]int{0, 6},
			"dragstart:brave  beforeinput:deleteByDrag change:deleteByDrag beforeinput:insertFromDrop change:insertFromDrop dragend:move"},
		{"copy with Ctrl", platform.FlavorLinux, platform.ModCtrl, 21, "hello brave new worldbrave ", [2]int{21, 27},
			"dragstart:brave  beforeinput:insertFromDrop change:insertFromDrop dragend:copy"},
		{"copy with Option on a Mac", platform.FlavorMac, platform.ModAlt, 21, "hello brave new worldbrave ", [2]int{21, 27},
			"dragstart:brave  beforeinput:insertFromDrop change:insertFromDrop dragend:copy"},
		{"Ctrl moves on a Mac", platform.FlavorMac, platform.ModCtrl, 21, "hello new worldbrave ", [2]int{15, 21},
			"dragstart:brave  beforeinput:deleteByDrag change:deleteByDrag beforeinput:insertFromDrop change:insertFromDrop dragend:move"},
		{"on the dragged text", platform.FlavorLinux, 0, -1, selValue, [2]int{6, 12}, "dragstart:brave  dragend:none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log []string
			x := newWinHarness(t, Options{Config: Config{Flavor: tc.flavor}, Root: selRoot(&log)})
			w := x.windows()[0]
			f := selDrag(t, x, w, tc.mods)
			info, ok := x.b.Drag()
			if !ok {
				t.Fatalf("no OS drag; events %v", log)
			}
			if s, _ := info.Data.Text(); s != "brave " || info.Allowed != platform.DropCopy|platform.DropMove || info.Image == nil {
				t.Errorf("drag %q allowed %v image %v", s, info.Allowed, info.Image != nil)
			}
			ex, ey := fieldAt(f, 9, 0)
			if tc.to >= 0 {
				ex, ey = fieldAt(f, tc.to, 0)
			}
			w.Inject(withMods(moveHeld(ex, ey), tc.mods))
			x.drain()
			if sel := selectionOf(f); sel != [2]int{6, 12} {
				t.Errorf("the hover changed the selection to %v", sel)
			}
			w.Inject(withMods(up(ex, ey), tc.mods))
			x.drain()
			if v, sel := f.ed.Value(), selectionOf(f); v != tc.value || sel != tc.sel {
				t.Errorf("value %q selection %v, want %q %v", v, sel, tc.value, tc.sel)
			}
			if got := strings.Join(log, " "); got != tc.events {
				t.Errorf("events %q\nwant   %q", got, tc.events)
			}
		})
	}
}

// A drop into another field moves the text too: in the same window at
// the drop, in another window when the source learns the move
// (DragDoneEvent), as for another application's move; a copy keeps it.
func TestSelectionDragToAnotherField(t *testing.T) {
	t.Run("same window", func(t *testing.T) {
		for _, mods := range []platform.Modifiers{0, platform.ModCtrl} {
			var log, blog []string
			// Ctrl is the copy modifier of the Linux flavor (Option under
			// the Mac's, the host's on macOS).
			x := newWinHarness(t, Options{Config: Config{Flavor: platform.FlavorLinux}, Root: selRoot(&log,
				input(append([]Child{Key("b"), Class("w-64"), DefaultValue("xy")}, textLog(&blog)...)...))})
			w := x.windows()[0]
			f := selDrag(t, x, w, mods)
			g := fieldOf(t, x.h.Tree(), "b")
			ex, ey := fieldAt(g, 2, 5)
			w.Inject(withMods(moveHeld(ex, ey), mods))
			x.drain()
			w.Inject(withMods(up(ex, ey), mods))
			x.drain()
			wantA, wantEnd := "hello new world", "dragend:move"
			if mods != 0 {
				wantA, wantEnd = selValue, "dragend:copy"
			}
			if a, b := f.ed.Value(), g.ed.Value(); a != wantA || b != "xybrave " {
				t.Errorf("mods %v: values %q %q", mods, a, b)
			}
			if sel := selectionOf(g); sel != [2]int{2, 8} || x.h.Tree().foc.el != g.el {
				t.Errorf("mods %v: target selection %v, focused %v", mods, sel, x.h.Tree().foc.el == g.el)
			}
			if got := strings.Join(log, " "); !strings.HasSuffix(got, wantEnd) || (mods == 0) != strings.Contains(got, "deleteByDrag") {
				t.Errorf("mods %v: source events %q", mods, got)
			}
			if got := strings.Join(blog, " "); got != "beforeinput:insertFromDrop change:insertFromDrop" {
				t.Errorf("mods %v: target events %q", mods, got)
			}
		}
	})
	t.Run("another window", func(t *testing.T) {
		var log, blog []string
		root := Component(func() Node {
			return selRoot(&log, win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "Target", Width: 400, Height: 200}, OnClose: func() {},
				Root: div(Class("p-4"), input(append([]Child{Key("b"), Class("w-64"), DefaultValue("xy")}, textLog(&blog)...)...))}))
		})
		x := newWinHarness(t, Options{Root: root})
		main, c := x.windows()[0], x.child(0)
		f := selDrag(t, x, main, 0)
		var ct *Tree
		for _, v := range x.h.views {
			if v.win == c {
				ct = v.tree
			}
		}
		g := fieldOf(t, ct, "b")
		ex, ey := fieldAt(g, 2, 5)
		c.Inject(moveHeld(ex, ey))
		x.drain()
		if eff, _ := c.DropAnswer(); eff != platform.DropMove {
			t.Errorf("the other window answers %v, want move (the app's text drag)", eff)
		}
		c.Inject(up(ex, ey))
		x.drain()
		if a, b := f.ed.Value(), g.ed.Value(); a != "hello new world" || b != "xybrave " {
			t.Errorf("values %q %q", a, b)
		}
		if got := strings.Join(log, " "); got != "dragstart:brave  dragend:move beforeinput:deleteByDrag change:deleteByDrag" {
			t.Errorf("source events %q", got)
		}
	})
	t.Run("another application", func(t *testing.T) {
		for _, eff := range []platform.DropEffect{platform.DropMove, platform.DropCopy, platform.DropNone} {
			var log []string
			x := newWinHarness(t, Options{Root: selRoot(&log)})
			w := x.windows()[0]
			f := selDrag(t, x, w, 0)
			w.Inject(ptr(platform.PointerLeave, 390, 290, 0, 0))
			x.drain()
			x.b.DropOutside(eff)
			x.drain()
			want := selValue
			if eff == platform.DropMove {
				want = "hello new world"
			}
			if v := f.ed.Value(); v != want {
				t.Errorf("%v: value %q, want %q", eff, v, want)
			}
		}
	})
}

// The source deletes the dragged text only while it is still there: a
// value changed during the drag keeps its text.
func TestSelectionDragSourceChanged(t *testing.T) {
	var log []string
	x := newWinHarness(t, Options{Root: selRoot(&log)})
	w := x.windows()[0]
	f := selDrag(t, x, w, 0)
	tr := x.h.Tree()
	tr.setFieldValue(f, "changed")
	w.Inject(ptr(platform.PointerLeave, 390, 290, 0, 0))
	x.drain()
	x.b.DropOutside(platform.DropMove)
	x.drain()
	if v := f.ed.Value(); v != "changed" {
		t.Errorf("value %q", v)
	}
}

// A press on the selection that does not drag puts the caret at the
// release; OnDragStart's PreventDefault makes the drag select from the
// press instead; a read-only field's selection drags as a copy; a
// password field's selection does not drag; Shift extends as usual.
func TestSelectionPressWithoutDrag(t *testing.T) {
	t.Run("click", func(t *testing.T) {
		var log []string
		x := newWinHarness(t, Options{Root: selRoot(&log)})
		w := x.windows()[0]
		tr := x.h.Tree()
		f := fieldOf(t, tr, "a")
		px, py := fieldAt(f, 0, 1)
		x.click(w, px, py)
		x.b.Advance(time.Second)
		tr.fieldChanged(f, f.ed.Select(6, 12))
		x.drain()
		px, py = fieldAt(f, 8, 1)
		w.Inject(move(px, py))
		w.Inject(down(px, py))
		x.drain()
		if sel := selectionOf(f); sel != [2]int{6, 12} {
			t.Errorf("the press changed the selection to %v", sel)
		}
		w.Inject(moveHeld(px+2, py)) // within the slop
		x.drain()
		px2, _ := fieldAt(f, 9, 1)
		w.Inject(up(px2, py)) // the caret goes under the release
		x.drain()
		if sel := selectionOf(f); sel != [2]int{9, 9} {
			t.Errorf("selection %v after the release, want the caret at 9", sel)
		}
		if _, ok := x.b.Drag(); ok || len(log) != 0 {
			t.Errorf("a drag started; events %v", log)
		}
	})
	t.Run("prevented", func(t *testing.T) {
		var log []string
		x := newWinHarness(t, Options{Root: div(Class("p-4"), input(Key("a"), Class("w-64"), DefaultValue(selValue),
			On("dragstart", func(e DragEvent) { log = append(log, "dragstart"); e.PreventDefault() })))})
		w := x.windows()[0]
		f := selDrag(t, x, w, 0)
		if _, ok := x.b.Drag(); ok {
			t.Fatal("a drag started after PreventDefault")
		}
		ex, ey := fieldAt(f, 21, 2)
		w.Inject(moveHeld(ex, ey))
		w.Inject(up(ex, ey))
		x.drain()
		if sel := selectionOf(f); sel != [2]int{8, 21} {
			t.Errorf("selection %v, want from the press (8) to the end", sel)
		}
		if !slices.Equal(log, []string{"dragstart"}) {
			t.Errorf("events %v", log)
		}
	})
	for _, tc := range []struct {
		name  string
		props []Child
		mods  platform.Modifiers
		drag  bool
	}{
		{"read-only", []Child{ReadOnly(true)}, 0, true},
		{"password", []Child{FieldType("password")}, 0, false},
		{"shift", nil, platform.ModShift, false},
		{"not focused", nil, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := newWinHarness(t, Options{Root: div(Class("p-4"),
				input(append([]Child{Key("a"), Class("w-64"), DefaultValue(selValue)}, tc.props...)...))})
			w := x.windows()[0]
			tr := x.h.Tree()
			f := fieldOf(t, tr, "a")
			px, py := fieldAt(f, 0, 1)
			if tc.name != "not focused" {
				x.click(w, px, py)
			}
			x.b.Advance(time.Second)
			tr.fieldChanged(f, f.ed.Select(6, 12))
			x.drain()
			px, py = fieldAt(f, 8, 1)
			w.Inject(withMods(move(px, py), tc.mods))
			w.Inject(withMods(down(px, py), tc.mods))
			w.Inject(withMods(moveHeld(px+10, py), tc.mods))
			x.drain()
			info, ok := x.b.Drag()
			if ok != tc.drag {
				t.Fatalf("drag %v, want %v", ok, tc.drag)
			}
			if ok && info.Allowed != platform.DropCopy {
				t.Errorf("a read-only field's drag allows %v, want copy", info.Allowed)
			}
		})
	}
}

// The drag image of a selection is the selected text as painted: its
// selection rectangles, cut by the field.
func TestSelectionDragImage(t *testing.T) {
	var log []string
	x := newWinHarness(t, Options{Root: selRoot(&log)})
	w := x.windows()[0]
	w.SetFocused(true)
	f := selDrag(t, x, w, 0)
	info, ok := x.b.Drag()
	if !ok || info.Image == nil {
		t.Fatal("no drag image")
	}
	x0, _ := fieldAt(f, 6, 0)
	x1, _ := fieldAt(f, 12, 0)
	if dx := info.Image.Rect.Dx(); float32(dx) < x1-x0-1 || float32(dx) > x1-x0+2 {
		t.Errorf("image %v wide, want the selection's %v", dx, x1-x0)
	}
	// The press was at offset 8 + 1 px, inside the image.
	if hx := float32(info.Hotspot.X); hx < 1 || hx > x1-x0 {
		t.Errorf("hotspot %v", info.Hotspot)
	}
}

// A move inside one field whose deleteByDrag is cancelled (OnBeforeInput's
// PreventDefault) still inserts at the drop point, a copy then; it does
// not replace the dragged text with itself.
func TestSelectionDragCancelledDelete(t *testing.T) {
	cancel := On("beforeinput", func(e InputEvent) {
		if e.InputType == "deleteByDrag" {
			e.PreventDefault()
		}
	})
	for _, tc := range []struct {
		name  string
		props []Child
		want  string
	}{
		{"prevented", []Child{DefaultValue(selValue), cancel}, "hello brave new worldbrave "},
		// A controlled field whose OnChange stores nothing keeps its value
		// after each edit, the insertion included.
		{"controlled", []Child{FieldValue(selValue)}, selValue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log []string
			root := div(Class("p-4"), input(append(append([]Child{Key("a"), Class("w-64")}, tc.props...), textLog(&log)...)...))
			x := newWinHarness(t, Options{Root: root})
			w := x.windows()[0]
			f := selDrag(t, x, w, 0)
			ex, ey := fieldAt(f, 21, 0)
			w.Inject(moveHeld(ex, ey))
			x.drain()
			w.Inject(up(ex, ey))
			x.drain()
			if v := f.ed.Value(); v != tc.want {
				t.Errorf("value %q, want %q", v, tc.want)
			}
			if sel := selectionOf(f); tc.name == "prevented" && sel != [2]int{21, 27} {
				t.Errorf("selection %v, want the copy (21..27)", sel)
			}
			if got := strings.Join(log, " "); !strings.Contains(got, "beforeinput:insertFromDrop") {
				t.Errorf("events %q: no insertFromDrop", got)
			}
		})
	}
}
