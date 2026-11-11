package tela_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The patterns of TeX's hyphen.tex that hyphenate "hyphenation"
// (TeXbook, appendix H): hy-phen-ation.
const texPatterns = ".hy3ph he2n hena4 hen5at 1na n2at 1tio 2io o2n"

func paragraphHeight(t *testing.T, class, lang, text string, opts ...telatest.Option) float32 {
	t.Helper()
	root := tela.C(func() tela.Node {
		return h.Div(h.Class("flex items-start p-4"),
			h.P(h.TestID("p"), h.Class("w-[60px] text-base leading-6", class), h.Lang(lang), h.Text(text)))
	})
	return telatest.Mount(t, root, opts...).Bounds(telatest.ByTestID("p")).Height
}

// TestHyphenationPatterns checks an app's own patterns: hyphens-auto
// breaks words of their language, and only with them.
func TestHyphenationPatterns(t *testing.T) {
	it := tela.HyphenationPatterns("it", texPatterns, "")
	// "hyphenation hyphenation": each word alone on its line overflows
	// 60 px without a dictionary, and breaks as hy-phen-ation with one.
	text := "hyphenation hyphenation"
	without := paragraphHeight(t, "hyphens-auto", "it", text)
	with := paragraphHeight(t, "hyphens-auto", "it", text, telatest.Hyphenation(it))
	if without != 48 || !(with > without) {
		t.Errorf("heights %g without the patterns and %g with them, want 48 and more", without, with)
	}
	if got := paragraphHeight(t, "hyphens-auto", "es", text, telatest.Hyphenation(it)); got != without {
		t.Errorf("Spanish text with Italian patterns: %g px high, want %g", got, without)
	}
	if got := paragraphHeight(t, "", "it", text, telatest.Hyphenation(it)); got != without {
		t.Errorf("hyphens-manual with patterns: %g px high, want %g", got, without)
	}
}

// TestHyphenationMistakes checks the errors of a dictionary that cannot
// be used.
func TestHyphenationMistakes(t *testing.T) {
	for _, c := range []struct {
		h    tela.Hyphenation
		want string
	}{
		{tela.Hyphenation{}, "zero tela.Hyphenation"},
		{tela.HyphenationPatterns("", texPatterns, ""), "give the language"},
		{tela.HyphenationPatterns("en_US!", texPatterns, ""), "BCP 47"},
		{tela.HyphenationPatterns("it", "  ", ""), "the patterns are empty"},
	} {
		err := tela.Run(tela.App{Hyphenation: []tela.Hyphenation{c.h}, Root: tela.C(func() tela.Node { return nil })})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Run: %v, want an error with %q", err, c.want)
		}
	}
	// Patterns that do not compile fail a test (Mount, Render).
	bad := tela.HyphenationPatterns("it", "a1-b", "")
	_, err := telatest.Render(tela.C(func() tela.Node { return nil }), telatest.Size(100, 100), telatest.Hyphenation(bad))
	if err == nil || !strings.Contains(err.Error(), "telatest.Hyphenation") || !strings.Contains(err.Error(), `unexpected '-'`) {
		t.Errorf("Render with malformed patterns: %v", err)
	}
}
