package core

import (
	"os"
	"strconv"
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// floatScene: a scroll container with an anchor moved by a TranslateFn,
// and a popover in a Portal placed under it by the float pass, with a size
// limit (it scrolls), an arrow published to a TranslateFn and items with
// hover colors; the popover opens and closes with the state.
func floatScene(st *sceneState) Node {
	comp := func() Node {
		st.refs[0] = UseElementRef()
		anchor, arrow := UseElementRef(), UseElementRef()
		st.values[0] = UseValue[float32](0)
		arrowX := UseValue[float32](0)
		open, set := UseState(1)
		st.setters[0] = set
		x := st.values[0]
		place := func(in FloatInput) FloatResult {
			y := in.Anchor.Bottom() + 6
			fx := min(max(in.Anchor.X-20, in.Clip.X+4), in.Clip.Right()-in.Floating.Width-4)
			PublishValue(arrowX, in.Anchor.X+in.Anchor.Width/2-fx-in.Arrow.Width/2)
			return FloatResult{X: fx, Y: y, MaxHeight: max(in.Clip.Bottom()-y-6, 24), HasMaxHeight: true}
		}
		rows := []Child{AttachRef(st.refs[0]), Key("list"), Class("h-[220px] overflow-y-auto rounded-xl bg-white")}
		for i := range 20 {
			if i == 5 {
				rows = append(rows, div(Key("anchor"), AttachRef(anchor), Class("relative mx-6 h-8 w-24 rounded bg-indigo-500 px-2 text-sm text-white"),
					Style(style.TranslateFn(func() (float32, float32) { return x.Get(), 0 })), Text("Anchor")))
			}
			rows = append(rows, div(Key(i), Class("h-8 px-2 text-sm odd:bg-slate-50"), Textf("Row %d", i)))
		}
		items := []Child{Key("pop"), Floating(FloatOptions{Anchor: anchor, Arrow: arrow, Place: place}),
			Class("fixed w-40 overflow-y-auto rounded-lg bg-white p-2 shadow-lg ring-1 ring-black/10"),
			div(Key("arrow"), AttachRef(arrow), Class("absolute top-0 left-0 size-2 rotate-45 bg-indigo-300"),
				Style(style.TranslateFn(func() (float32, float32) { return arrowX.Get(), 0 })))}
		for i := range 8 {
			items = append(items, div(Key(i), Class("h-7 rounded px-2 text-sm hover:bg-sky-100"), Textf("Item %d", i)))
		}
		return div(Class("relative h-screen bg-slate-100 p-3"),
			div(rows...),
			If(open%3 != 0, Portal(div(items...))),
		)
	}
	return Component(comp)
}

// TestDamageFloating: incremental frames equal full ones with a floating
// element that follows its anchor through scrolls, translations, size
// limits, resizes, hovers and open/close (ADR-096), at every scale.
func TestDamageFloating(t *testing.T) {
	steps := 160
	if testing.Short() {
		steps = 40
	}
	if n, err := strconv.Atoi(os.Getenv("TELA_DAMAGE_STEPS")); err == nil {
		steps = n
	}
	runScene(t, "floating", floatScene, commonOps, Config{InstantScroll: true}, steps)
	runScene(t, "floating_smooth", floatScene, commonOps, Config{}, steps/2)
}
