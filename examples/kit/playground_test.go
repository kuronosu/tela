package main

import (
	"image"
	"runtime"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// openPlayground opens the demo window from the button of a page.
func openPlayground(t *testing.T, page, tag, button string) (*telatest.App, *telatest.App) {
	t.Helper()
	a := mount(t, Canvas{Page: page, Locale: tag}, telatest.Size(1280, 900), telatest.PlatformKeys("linux"))
	w := a.WaitForWindow(func() { a.Click(telatest.ByRole("button", button)) })
	return a, w
}

// The Title Bar and Menubar pages open a frameless, transparent window with
// a resizable card and its own menu bar, so h.WindowResize, F10 and a lone
// Alt can be tried in the gallery: a press on a card's edge is the OS's
// resize, and F10 or Alt focus the bar's first menu from anywhere.
func TestPlaygroundWindow(t *testing.T) {
	for _, page := range []string{"TitleBar", "Menubar"} {
		t.Run(page, func(t *testing.T) {
			a, w := openPlayground(t, page, "en", "Open a frameless window")
			st := w.WindowState()
			if !st.Transparent || !st.ClientSideDecorations {
				t.Fatalf("WindowState = %+v, want a transparent window that draws its own title bar", st)
			}
			w.ExpectText("Frameless window")

			// The card's edges are resize handles; the margin around it is not.
			if k := w.HitAt(26, 200); k != telatest.HitResizeLeft {
				t.Errorf("HitAt the card's left edge = %v, want resize-left", k)
			}
			if k := w.HitAt(528, 393); k != telatest.HitResizeBottomRight {
				t.Errorf("HitAt the card's bottom-right corner = %v, want resize-bottom-right", k)
			}
			if w.TakesInput(5, 200) {
				t.Error("the transparent margin takes input")
			}
			w.ClickAt(26, 200)
			if got := w.ResizeStarts(); len(got) != 1 || got[0] != tela.EdgeLeft {
				t.Errorf("ResizeStarts = %v, want [left]", got)
			}

			// F10 and a lone Alt focus the first menu; Escape gives it back.
			for _, key := range []string{"F10", "Alt"} {
				w.Press(key)
				w.ExpectFocused(telatest.ByRole("menuitem", "File"))
				w.Press("Escape")
				w.Expect(telatest.ByRole("menuitem", "File"), telatest.Focused(false))
			}
			w.Press("F10")
			w.Press("ArrowRight")
			w.Press("ArrowDown")
			w.Press("Enter") // View > Show the size
			w.ExpectCount(telatest.ByTestID("playground-size"), 0)

			// The title bar's Close asks the window to close.
			w.Click(telatest.ByRole("button", "Close"))
			a.ExpectWindowCount(1)
		})
	}
}

// The demo window speaks the gallery's language and direction.
func TestPlaygroundWindowLanguage(t *testing.T) {
	for _, c := range []struct{ tag, button, title, bar, size string }{
		{"es", "Abrir una ventana sin marco", "Ventana sin marco", "Archivo", "Tamaño: 512 × 372"},
		{"ar", "فتح نافذة بلا إطار", "نافذة بلا إطار", "ملف", "الحجم: 512 × 372"},
	} {
		t.Run(c.tag, func(t *testing.T) {
			_, w := openPlayground(t, "Menubar", c.tag, c.button)
			w.ExpectText(c.title)
			w.ExpectCount(telatest.ByRole("menuitem", c.bar), 1)
			w.ExpectText(c.size)
		})
	}
}

// ink returns the bounding box of the pixels of img inside r that differ
// from blank's.
func ink(img, blank *image.RGBA, r image.Rectangle) image.Rectangle {
	var box image.Rectangle
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.RGBAAt(x, y) != blank.RGBAAt(x, y) {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return box
}

// TestDateRangePickerReadingOrder: the button writes the start first and
// then the end in the direction of reading, so under right to left the
// start is at the right, as Chromium lays out the same string (checked
// against it: "2026/11/05 - 2026/10/20" is the start, Oct 20, on the right
// in an Arabic page). The start alone, set in the same button, lies exactly
// where the start lies in the pair; the end alone does not.
func TestDateRangePickerReadingOrder(t *testing.T) {
	r := aria.DateRange{Start: oct(20), End: aria.Date{Year: 2026, Month: 11, Day: 5}}
	for _, c := range []struct {
		tag, dir     string
		startAtRight bool
	}{{"en", "ltr", false}, {"ar", "rtl", true}} {
		t.Run(c.tag, func(t *testing.T) {
			shot := func(format func(i18n.Formatter, aria.DateRange) string) (*image.RGBA, image.Rectangle) {
				page := func() tela.Node {
					return h.Div(h.Dir(c.dir), i18n.Provider(c.tag, kit.DateRangePicker{Locale: c.tag, Direction: c.dir, Value: &r, Class: "w-72", AriaLabel: "Trip", Format: format}))
				}
				a := telatest.MountFunc(t, page, telatest.Size(320, 80), telatest.Theme(kit.Theme()))
				b := a.Bounds(telatest.ByRole("button", ""))
				// The text's room: inside the border and the icon's side.
				rect := image.Rect(int(b.X)+2, int(b.Y)+2, int(b.X+b.Width)-2, int(b.Y+b.Height)-2)
				return a.Screenshot(), rect
			}
			f := i18n.For(c.tag)
			date := func(d aria.Date) string { return f.Date(d.Time(nil), i18n.Medium) }
			pair, rect := shot(nil)
			blank, _ := shot(func(i18n.Formatter, aria.DateRange) string { return "" })
			start, _ := shot(func(_ i18n.Formatter, r aria.DateRange) string { return date(r.Start) })
			end, _ := shot(func(_ i18n.Formatter, r aria.DateRange) string { return date(r.End) })

			// Where the start alone has ink, the pair has the same pixels.
			box := ink(start, blank, rect)
			if box.Empty() {
				t.Fatal("the start alone draws nothing")
			}
			same := func(a, b *image.RGBA) bool {
				for y := box.Min.Y; y < box.Max.Y; y++ {
					for x := box.Min.X; x < box.Max.X; x++ {
						if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
							return false
						}
					}
				}
				return true
			}
			if !same(pair, start) {
				t.Errorf("the pair does not have the start where the start alone is (%v): the dates are in the wrong order", box)
			}
			if same(pair, end) {
				t.Errorf("the pair has the end where the start belongs (%v)", box)
			}
			// And the start is at the end of the line the direction reads from.
			mid := (rect.Min.X + rect.Max.X) / 2
			if got := box.Min.X+box.Dx()/2 > mid; got != c.startAtRight {
				t.Errorf("the start is at the right of the button = %v, want %v", got, c.startAtRight)
			}
		})
	}
}

// TestGoldenPlayground renders the demo window: the card over a transparent
// margin (the PNG has alpha), in English with a menu open, and in Arabic,
// dark, mirrored. TELA_UPDATE=1 writes the pictures, which must be looked
// at before they are committed.
func TestGoldenPlayground(t *testing.T) {
	// The demo window's kit.TitleBar has the Default variant, the host's
	// look (Windows' and macOS' arrangements elsewhere), and the pictures
	// are Linux's.
	if runtime.GOOS != "linux" {
		t.Skip("the title bar follows the host's look; the goldens are Linux's")
	}
	_, w := openPlayground(t, "Menubar", "en", "Open a frameless window")
	w.Press("F10")
	w.Press("ArrowDown")
	w.ExpectCount(telatest.ByRole("menu", "File"), 1)
	w.GoldenScales("kit_playground_menu_en", 1)

	a := mount(t, Canvas{Page: "TitleBar", Locale: "ar"}, telatest.Size(1280, 900), telatest.Dark(), telatest.PlatformKeys("linux"))
	w = a.WaitForWindow(func() { a.Click(telatest.ByRole("button", "فتح نافذة بلا إطار")) })
	w.GoldenScales("kit_playground_ar_dark", 1)
}
