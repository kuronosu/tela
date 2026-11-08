package tela_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

const testFontDir = "internal/text/testdata/fonts"

// cjk is a font with ASCII glyphs whose widths differ from Inter's, so the
// width of a text tells which font drew it.
func cjk(t *testing.T) tela.Font {
	t.Helper()
	data, err := os.ReadFile(testFontDir + "/NotoSansJP-Subset.otf")
	if err != nil {
		t.Fatal(err)
	}
	return tela.FontData("Test Face", data)
}

func textWidth(t *testing.T, class string, opts ...telatest.Option) float32 {
	t.Helper()
	root := tela.C(func() tela.Node {
		return h.Div(h.Class("p-4"), h.Span(h.Class(class), h.TestID("t"), h.Text("Hamburgefonstiv")))
	})
	app := telatest.Mount(t, root, opts...)
	return app.Bounds(telatest.ByTestID("t")).Width
}

func TestFontsDrawTheText(t *testing.T) {
	base := textWidth(t, "font-sans")
	with := textWidth(t, "font-sans", telatest.Fonts(cjk(t).Sans()))
	if with == base {
		t.Errorf("font-sans is %gpx wide with and without a Sans() app font", base)
	}
	// Without a role the font is used by name only.
	if got := textWidth(t, "font-sans", telatest.Fonts(cjk(t))); got != base {
		t.Errorf("font-sans with a font that has no role: %g, want %g", got, base)
	}
	if got := textWidth(t, "font-['Test_Face']", telatest.Fonts(cjk(t))); got == base {
		t.Errorf("font-['Test_Face'] is %gpx wide, as Inter", got)
	}
	// The serif of the deterministic set is not Inter's width; an app
	// serif replaces it.
	serif := textWidth(t, "font-serif")
	if serif == base {
		t.Errorf("font-serif draws as font-sans (%gpx)", base)
	}
	if got := textWidth(t, "font-serif", telatest.Fonts(cjk(t).Serif())); got == serif {
		t.Errorf("font-serif with a Serif() app font: %g, same as without", got)
	}
	// FromApp takes App.Fonts.
	if got := textWidth(t, "font-sans", telatest.FromApp(tela.App{Fonts: []tela.Font{cjk(t).Sans()}})); got != with {
		t.Errorf("FromApp(App{Fonts}) width %g, want %g", got, with)
	}
}

func TestFontFSInTests(t *testing.T) {
	f := tela.FontFS(os.DirFS(testFontDir), "Test Face", "NotoSansJP*.otf").Sans()
	if got, want := textWidth(t, "font-sans", telatest.Fonts(f)), textWidth(t, "font-sans", telatest.Fonts(cjk(t).Sans())); got != want {
		t.Errorf("FontFS width %g, FontData %g", got, want)
	}
}

// TestRunChecksFonts: Run returns the error of a font that cannot be used
// before it opens a window.
func TestRunChecksFonts(t *testing.T) {
	root := tela.C(func() tela.Node { return nil })
	err := tela.Run(tela.App{Root: root, Fonts: []tela.Font{
		tela.FontFS(fstest.MapFS{"fonts/a.ttf": {Data: []byte("x")}}, "A", "fonts/*.otf"),
	}})
	if err == nil || !strings.Contains(err.Error(), `pattern "fonts/*.otf" matches no file`) {
		t.Errorf("Run error = %v", err)
	}
	err = tela.Run(tela.App{Root: root, Fonts: []tela.Font{{}}})
	if err == nil || !strings.Contains(err.Error(), "zero tela.Font") {
		t.Errorf("Run error with a zero Font = %v", err)
	}
}

func TestFontScriptsPanicInDevMode(t *testing.T) {
	defer func() {
		msg, _ := recover().(string)
		if !strings.Contains(msg, `"Arabic"`) || !strings.Contains(msg, `"Arab"`) {
			t.Errorf("panic = %q, want a message naming the code and examples", msg)
		}
	}()
	tela.FontData("A", []byte("x")).Scripts("Arabic")
}

func TestFontMountFails(t *testing.T) {
	ft := &fakeTB{TB: t}
	var msg string
	func() {
		defer func() { msg, _ = recover().(string) }()
		telatest.Mount(ft, tela.C(func() tela.Node { return nil }),
			telatest.Fonts(tela.FontFS(fstest.MapFS{}, "A", "*.ttf")))
	}()
	if !strings.Contains(msg, `pattern "*.ttf" matches no file`) {
		t.Errorf("Mount error = %q", msg)
	}
}
