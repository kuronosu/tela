package h_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
)

// The drag and drop props of package h (ADR-211): a Draggable source and
// a target with every handler, with func() and event handlers and
// tela.StopPropagation.
func TestDragAndDropProps(t *testing.T) {
	var log []string
	add := func(s string) { log = append(log, s) }
	root := func() tela.Node {
		return Div(Class("relative w-full h-full"),
			OnDragEnter(func() { add("outer-enter") }),
			OnDragLeave(func() { add("outer-leave") }),
			OnDrop(func() { add("outer-drop") }),
			Div(Class("absolute left-[10px] top-[10px] size-[50px] bg-indigo-500"), Draggable(true),
				OnDragStart(func(e tela.DragEvent) {
					e.DataTransfer.SetData("application/x-todo", "42")
					e.DataTransfer.SetText("Buy milk")
					e.DataTransfer.EffectAllowed = tela.DropMove
				}),
				OnDrag(func() { add("drag") }),
				OnDragEnd(func(e tela.DragEvent) { add("end:" + e.DataTransfer.DropEffect.String()) }),
			),
			Div(Class("absolute left-[200px] top-[10px] size-[100px]"),
				OnDragEnter(tela.StopPropagation(func() { add("enter") })),
				OnDragOver(func(e tela.DragEvent) {
					if e.DataTransfer.Has("application/x-todo") {
						e.PreventDefault()
					}
				}),
				OnDragLeave(tela.StopPropagation(func() { add("leave") })),
				OnDrop(func(e tela.DragEvent) {
					e.StopPropagation()
					add("drop:" + e.DataTransfer.GetData("application/x-todo") + ":" + e.DataTransfer.Text())
				}),
			),
		)
	}
	a := open(t, root)
	a.send(pe(platform.PointerMove, 20, 20, 0, 0), pe(platform.PointerDown, 20, 20, platform.ButtonPrimary, primary))
	a.send(pe(platform.PointerMove, 30, 20, 0, primary))
	a.send(pe(platform.PointerMove, 250, 50, 0, primary))
	a.send(pe(platform.PointerUp, 250, 50, platform.ButtonPrimary, 0))
	// The drag enters the window over the source (the outer element),
	// moves to the target (whose enter stops), and drops there.
	if got := strings.Join(log, " "); got != "outer-enter enter outer-leave drop:42:Buy milk end:move" {
		t.Errorf("events %q", got)
	}
	if d := a.w.Drops(); !slices.Equal(d, []platform.DropEffect{platform.DropMove}) {
		t.Errorf("drops %v", d)
	}
}

// Files dropped from another application arrive as paths.
func TestDropFiles(t *testing.T) {
	var got []string
	root := func() tela.Node {
		return Div(Class("w-full h-full"),
			OnDragOver(func(e tela.DragEvent) {
				if e.DataTransfer.Has("Files") {
					e.PreventDefault()
				}
			}),
			OnDrop(func(e tela.DragEvent) { got = e.DataTransfer.Files() }))
	}
	a := open(t, root)
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b c.png")}
	data, err := platform.FileData(paths...)
	if err != nil {
		t.Fatal(err)
	}
	a.b.StartExternalDrag(data, platform.DropCopy|platform.DropLink)
	a.send(pe(platform.PointerMove, 100, 100, 0, 0))
	a.send(pe(platform.PointerUp, 100, 100, platform.ButtonPrimary, 0))
	if !slices.Equal(got, paths) {
		t.Errorf("dropped files %q, want %q", got, paths)
	}
	if d := a.w.Drops(); !slices.Equal(d, []platform.DropEffect{platform.DropCopy}) {
		t.Errorf("drops %v", d)
	}
}

func TestDropPropsPanicOnNil(t *testing.T) {
	for name, f := range map[string]func(){
		"OnDragEnter": func() { OnDragEnter[func()](nil) },
		"OnDragOver":  func() { OnDragOver[func()](nil) },
		"OnDragLeave": func() { OnDragLeave[func(tela.DragEvent)](nil) },
		"OnDrop":      func() { OnDrop[func()](nil) },
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(r.(string), name+"(nil)") {
					t.Errorf("%s(nil): panic %v", name, r)
				}
			}()
			f()
		}()
	}
}
