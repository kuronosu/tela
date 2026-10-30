package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// TestFieldRTLSelectionGolden draws selections in right-to-left and
// bidirectional text fields, the way a user makes them: a double click on
// an Arabic word of a right-to-left textarea, and a range that crosses
// from Latin digits into Arabic words in a field of automatic direction
// (its highlight splits at the bidi run boundaries). It began as the
// Arabic selection golden of the `text` example.
func TestFieldRTLSelectionGolden(t *testing.T) {
	const arabic = "النص العربي يُكتب من اليمين إلى اليسار، وتتصل حروفه ببعضها لتكوّن الكلمات."
	const mixed = "سعر الكتاب 1,250.50 ريال، رقم الطلب 4096 في 12 أكتوبر 2026."
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			field := "w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-slate-800 selection:bg-indigo-200 selection:text-indigo-950"
			root := div(Class("flex h-full flex-col gap-3 bg-slate-50 p-4"),
				textarea(Key("ar"), Dir("rtl"), Class(field+" text-base leading-7"), Rows(3), DefaultValue(arabic)),
				input(Key("mixed"), Dir("auto"), Class(field+" text-sm"), DefaultValue(mixed)),
			)
			b := headless.New(headless.Options{Size: platform.Size{W: 340, H: 200}, Scale: scale})
			host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: root})
			if err != nil {
				t.Fatal(err)
			}
			b.Drain(host)
			x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
			t.Cleanup(host.stopTimer)
			x.w.SetFocused(true)
			x.b.Drain(x.h)

			// A double click on the middle of "اليمين" selects the word.
			f := fieldOf(t, x.tree(), "ar")
			r := windowRect(f.bx)
			lay := f.layout()
			off := strings.Index(arabic, "اليمين") + len("اليم")
			c := lay.Caret(textCursor(off))
			px, py := r.X0+13+c.X, r.Y0+9+c.Top+c.Height/2
			x.click(px, py)
			x.click(px, py)
			if s, e := x.selection("ar"); arabic[s:e] != "اليمين" {
				t.Fatalf("double click selected %q (%d..%d), want the word اليمين", arabic[s:e], s, e)
			}
			x.settle()
			golden.Image(t, fmt.Sprintf("field_rtl_selection_word_x%g", scale), x.w.Frame())

			// A range from the digits of the price into the Arabic words
			// around them, as a drag would leave it.
			x.b.Advance(time.Second)
			x.tab()
			g := fieldOf(t, x.tree(), "mixed")
			from := strings.Index(mixed, "1,250")
			to := strings.Index(mixed, "الطلب") + len("الط")
			g.ed.Select(from, to)
			x.tree().caretReset(g)
			x.settle()
			golden.Image(t, fmt.Sprintf("field_rtl_selection_bidi_x%g", scale), x.w.Frame())
		})
	}
}
