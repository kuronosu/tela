package core

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/anim"
	"github.com/kuronosu/tela/internal/style"
)

// styleLeaves calls f with every leaf field of v (a settable struct),
// recursing into structs and arrays: the values sameStyle compares, by
// their path ("Text.Decoration.Color.R").
func styleLeaves(t *testing.T, v reflect.Value, path string, f func(path string, leaf reflect.Value)) {
	switch v.Kind() {
	case reflect.Struct:
		for i := range v.NumField() {
			fv := v.Field(i)
			name := v.Type().Field(i).Name
			if !fv.CanSet() {
				t.Fatalf("%s.%s is not settable: extend the test (and sameStyle)", path, name)
			}
			styleLeaves(t, fv, strings.TrimPrefix(path+"."+name, "."), f)
		}
	case reflect.Array:
		for i := range v.Len() {
			styleLeaves(t, v.Index(i), fmt.Sprintf("%s[%d]", path, i), f)
		}
	default:
		f(path, v)
	}
}

// mutate sets leaf, which holds its zero value, to another value.
func mutate(t *testing.T, path string, leaf reflect.Value) {
	switch leaf.Kind() {
	case reflect.Bool:
		leaf.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		leaf.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		leaf.SetUint(1)
	case reflect.Float32, reflect.Float64:
		leaf.SetFloat(1)
	case reflect.String:
		leaf.SetString("x")
	case reflect.Slice:
		leaf.Set(reflect.MakeSlice(leaf.Type(), 1, 1))
	case reflect.Pointer:
		leaf.Set(reflect.New(leaf.Type().Elem()))
	default:
		t.Fatalf("%s: a %v field: extend the test (and sameStyle)", path, leaf.Kind())
	}
}

// TestSameStyleCoversEveryField: a change of any field of a
// ComputedStyle, today's and any added later, is a change for the tracker
// of partial recording (sameStyle): what painting reads of a style can
// change without being noticed only where the compositor applies it (the
// opacity and transform of a deferred layer). A copy of a style, which
// shares its slices and pointers, is the same.
func TestSameStyleCoversEveryField(t *testing.T) {
	compositor := func(path string) bool {
		return path == "Opacity" || strings.HasPrefix(path, "Transform.")
	}
	var base style.ComputedStyle
	n := 0
	styleLeaves(t, reflect.ValueOf(&base).Elem(), "", func(path string, _ reflect.Value) {
		n++
		mut := base
		var leaf reflect.Value
		styleLeaves(t, reflect.ValueOf(&mut).Elem(), "", func(p string, v reflect.Value) {
			if p == path {
				leaf = v
			}
		})
		mutate(t, path, leaf)
		if sameStyle(&base, &mut, false) {
			t.Errorf("a change of %s is not seen", path)
		}
		if got, want := sameStyle(&base, &mut, true), compositor(path); got != want {
			t.Errorf("in a deferred layer, a change of %s: same %v, want %v", path, got, want)
		}
	})
	if n < 100 {
		t.Fatalf("%d leaf fields, want the whole ComputedStyle", n)
	}
	// Every field set, then copied: the copy shares the slices.
	var full style.ComputedStyle
	styleLeaves(t, reflect.ValueOf(&full).Elem(), "", func(path string, leaf reflect.Value) { mutate(t, path, leaf) })
	cp := full
	if !sameStyle(&full, &cp, false) {
		t.Error("a copy of a style is not the same")
	}
}

// styleStep is an animation-like path: on the next frame it installs, as
// an animation step does (animApply), a copy of the box's used style
// changed by change, which need not be an animatable property.
type styleStep struct {
	bx     *Box
	change func(*style.ComputedStyle)
}

func (s *styleStep) Advance(time.Duration) anim.State {
	old := s.bx.cs
	u := new(style.ComputedStyle)
	*u = *old
	s.change(u)
	s.bx.tree.animApply(s.bx, old, u)
	return anim.Done
}

// TestAnimationStyleSeen: a field that painting reads, changed in the
// used style by an animation-like path without being one of the
// animatable properties, records the layers that paint the box: a card in
// a scroll space's chunk, and a deferred layer (TranslateFn). Every frame
// shows what a full rendering shows, and with TELA_CHECK_LAYERS=1
// composes what a full recording composes.
func TestAnimationStyleSeen(t *testing.T) {
	scene := func() Node {
		cards := []Child{Class("grid grid-cols-3 gap-4 p-4")}
		for i := range 9 {
			cards = append(cards, div(Key(i), Class("relative h-24 rounded-lg border border-slate-300 bg-white p-2"), Textf("Card %d", i)))
		}
		return div(Class("h-screen overflow-y-auto bg-slate-100"),
			div(cards...),
			div(Key("moved"), Class("m-4 h-16 w-48 rounded-lg border border-slate-300 bg-white p-2"),
				Style(style.TranslateFn(func() (float32, float32) { return 6, 0 })), Text("Moved")),
			div(Class("h-[900px]")))
	}
	changes := []struct {
		name   string
		change func(*style.ComputedStyle)
	}{
		{"visibility", func(cs *style.ComputedStyle) { cs.Visibility = style.VisibilityHidden }},
		{"border style", func(cs *style.ComputedStyle) {
			cs.Border.Top.Style, cs.Border.Bottom.Style = style.BorderDashed, style.BorderDotted
		}},
		{"outline", func(cs *style.ComputedStyle) {
			// Its color (animatable) stays: the text color.
			cs.Outline.Width, cs.Outline.Offset, cs.Outline.Style = 3, 2, style.BorderSolid
		}},
		{"background clip", func(cs *style.ComputedStyle) {
			cs.BackgroundClip = style.BackgroundContentBox
		}},
	}
	for _, key := range []any{4, "moved"} {
		for _, c := range changes {
			t.Run(fmt.Sprintf("%v %s", key, c.name), func(t *testing.T) {
				x := newDamageHarness(t, Component(scene), 600, 420, 1.5, Config{}, 1)
				tr := x.tree()
				bx := keyed(tr, key).box
				if want := key == "moved"; deferredLayer(bx) != want {
					t.Fatalf("deferred layer %v, want %v", deferredLayer(bx), want)
				}
				tr.anim.tk.Add(&styleStep{bx: bx, change: c.change})
				tr.requestFrame()
				x.frame(c.name)
				if tr.Dirty() {
					x.frame(c.name + " (settled)")
				}
				found := false
				for _, l := range tr.lay.layers {
					if l != nil && l.paint == tr.lay.paints && l.kind != layerBars {
						found = true
					}
				}
				if !found {
					t.Errorf("no layer was recorded for the change of %s", c.name)
				}
			})
		}
	}
}
