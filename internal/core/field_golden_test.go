package core

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// Goldens of text fields in interactive states: a caret, a composition
// underlined with the IME's highlighted clause, a selection across the
// lines of a textarea, a focus outline, and placeholder, caret and
// selection colors set by classes.
func TestFieldStatesGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			field := "w-full rounded-md border border-slate-300 px-3 py-2 text-slate-900 focus:outline-2 focus:outline-indigo-500"
			root := div(Class("flex h-full flex-col gap-3 bg-slate-50 p-4"),
				input(Key("caret"), Class(field), DefaultValue("Caret at the end")),
				input(Key("ime"), Class(field), DefaultValue("IME ")),
				textarea(Key("sel"), Class(field+" selection:bg-amber-200 selection:text-amber-900"), Rows(3),
					DefaultValue("A selection that spans\nseveral lines of a textarea")),
				input(Key("ph"), Class(field+" placeholder:text-indigo-400 placeholder:italic caret-pink-500"), Placeholder("Styled placeholder")),
			)
			b := headless.New(headless.Options{Size: platform.Size{W: 320, H: 280}, Scale: scale})
			host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: root})
			if err != nil {
				t.Fatal(err)
			}
			b.Drain(host)
			x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
			t.Cleanup(host.stopTimer)
			x.w.SetFocused(true)
			x.b.Drain(x.h)
			tr := x.tree()
			// A composition, underlined, with the IME's highlighted clause.
			x.tab()
			x.key(platform.KeyEnd, 0)
			x.tab()
			x.key(platform.KeyEnd, 0)
			x.w.Inject(platform.IMEEvent{Preedit: "kana kanji", CursorStart: 5, CursorEnd: 10})
			x.b.Drain(x.h)
			x.settle()
			golden.Image(t, fmt.Sprintf("field_states_ime_x%g", scale), x.w.Frame())
			// A selection across the lines of the textarea (the blur
			// committed the composition).
			x.tab()
			f := fieldOf(t, tr, "sel")
			f.ed.Select(2, 40)
			tr.caretReset(f)
			x.settle()
			golden.Image(t, fmt.Sprintf("field_states_sel_x%g", scale), x.w.Frame())
			// The placeholder field focused: its pink caret.
			x.tab()
			x.settle()
			golden.Image(t, fmt.Sprintf("field_states_caret_x%g", scale), x.w.Frame())
		})
	}
}
