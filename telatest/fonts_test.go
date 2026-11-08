package telatest_test

import (
	"sync"
	"testing"
	"unsafe"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/fonts/inter"
	"github.com/kuronosu/tela/fonts/standard"
	"github.com/kuronosu/tela/h"
	asinter "github.com/kuronosu/tela/internal/assets/fonts/inter"
	"github.com/kuronosu/tela/internal/assets/fonts/jetbrainsmono"
	"github.com/kuronosu/tela/internal/assets/fonts/notocoloremoji"
	"github.com/kuronosu/tela/internal/assets/fonts/notosansarabic"
	"github.com/kuronosu/tela/internal/assets/fonts/sourceserif4"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/text"
	"github.com/kuronosu/tela/telatest"
)

// telaFontFiles are the files of Tela's fonts, which fonts/standard and the
// deterministic set of telatest (internal/stdfonts) both embed.
var telaFontFiles = [][]byte{
	asinter.Upright, asinter.Italic, jetbrainsmono.Upright, jetbrainsmono.Italic,
	notosansarabic.Data, notocoloremoji.Data, sourceserif4.Upright, sourceserif4.Italic,
}

// telaParses counts, from the start of the test binary, the descriptions
// (font -1) and parses of Tela's font files, by family, file and font.
var telaParses = struct {
	sync.Mutex
	n map[parseKey]int
}{n: map[parseKey]int{}}

type parseKey struct {
	family     string
	file, font int
}

// The hook is set before any test runs, so the counts are those of the
// whole process: each font must be parsed once in it.
func init() {
	text.SetParseHook(func(family string, data []byte, font int) {
		for i, f := range telaFontFiles {
			if len(f) == len(data) && unsafe.SliceData(f) == unsafe.SliceData(data) {
				telaParses.Lock()
				telaParses.n[parseKey{family, i, font}]++
				telaParses.Unlock()
			}
		}
	})
}

// TestStandardFontsParseOnce: an app that brings fonts/standard draws with
// the same files as the deterministic set telatest adds after the app's
// fonts (as tela snapshot and tela mcp do): each font is described and
// parsed once, and both give the same typefaces.
func TestStandardFontsParseOnce(t *testing.T) {
	root := tela.C(func() tela.Node {
		return h.Div(h.Class("flex flex-col p-4"),
			h.Span(h.Class("font-sans"), h.Text("Sans")),
			h.Span(h.Class("font-sans italic"), h.Text("Italic")),
			h.Span(h.Class("font-mono"), h.Text("Mono")),
			h.Span(h.Class("font-serif"), h.Text("Serif")),
			h.Span(h.Text("مرحبا 😀")),
		)
	})
	app := telatest.Mount(t, root, telatest.FromApp(tela.App{Fonts: standard.Fonts}))
	app.Screenshot()

	// Every font through both: the app's sources and the set's.
	var appFaces, setFaces []*text.Typeface
	for _, f := range standard.Fonts {
		src, err := core.Font(f).Source()
		if err != nil {
			t.Fatal(err)
		}
		tfs, err := src.Typefaces()
		if err != nil {
			t.Fatal(err)
		}
		appFaces = append(appFaces, tfs...)
	}
	for _, src := range stdfonts.Sources() {
		tfs, err := src.Typefaces()
		if err != nil {
			t.Fatal(err)
		}
		setFaces = append(setFaces, tfs...)
	}
	if len(appFaces) != len(telaFontFiles) || len(setFaces) != len(telaFontFiles) {
		t.Fatalf("%d and %d typefaces, want %d", len(appFaces), len(setFaces), len(telaFontFiles))
	}
	shared := map[*text.Typeface]bool{}
	for _, tf := range setFaces {
		shared[tf] = true
	}
	for _, tf := range appFaces {
		if !shared[tf] {
			t.Errorf("%s %v: fonts/standard and the deterministic set have different typefaces (the file was parsed twice)", tf.Family(), tf.Style())
		}
	}

	telaParses.Lock()
	defer telaParses.Unlock()
	for k, n := range telaParses.n {
		if n != 1 {
			t.Errorf("%s, file %d, font %d (-1: the description): %d times in the process, want once", k.family, k.file, k.font, n)
		}
	}
	// The counts saw the parses (the hook works): every file was parsed.
	for i := range telaFontFiles {
		parsed := false
		for k := range telaParses.n {
			parsed = parsed || (k.file == i && k.font >= 0)
		}
		if !parsed {
			t.Errorf("file %d: no parse counted", i)
		}
	}
}

// TestFromAppInterParsesOnce: inter.Sans, which an app imports alone, is
// the Inter of the deterministic set.
func TestFromAppInterParsesOnce(t *testing.T) {
	telatest.Mount(t, tela.C(func() tela.Node { return h.Span(h.Text("Hi")) }), telatest.FromApp(tela.App{Fonts: []tela.Font{inter.Sans}}))
	src, err := core.Font(inter.Sans).Source()
	if err != nil {
		t.Fatal(err)
	}
	a, _ := src.Typefaces()
	b, _ := stdfonts.Sources()[0].Typefaces()
	if len(a) != 2 || len(b) != 2 || a[0] != b[0] || a[1] != b[1] {
		t.Errorf("inter.Sans %v and the set's Inter %v are other typefaces", a, b)
	}
}
