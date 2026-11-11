package en_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/hyphen/en"
	"github.com/kuronosu/tela/hyphen/es"
	"github.com/kuronosu/tela/hyphen/fr"
	"github.com/kuronosu/tela/telatest"
)

// article lays out the same text with hyphens-auto in English (and an
// "en-GB" quote), with soft hyphens and with hyphens-none.
func article() tela.Node {
	p := func(id, class, lang, text string) tela.Node {
		return P(TestID(id), Class("w-[100px] text-base leading-6", class), Lang(lang), Text(text))
	}
	return Div(Class("flex items-start gap-6 p-4 bg-white text-gray-900"),
		p("auto", "hyphens-auto", "en", "Extraordinary internationalization of hyphenation"),
		p("word", "hyphens-auto", "en", "Extraordinary internationalization"),
		p("gb", "hyphens-auto underline decoration-indigo-600", "en-GB", "The internationalisation programme"),
		p("manual", "", "", "Ex\u00adtra\u00ador\u00addi\u00adnary in\u00adter\u00adna\u00adtion\u00adal\u00adiza\u00adtion"),
		p("none", "hyphens-none", "en", "Ex\u00adtra\u00ador\u00addi\u00adnary internationalization"),
	)
}

// TestHyphenation mounts paragraphs with and without the dictionaries:
// hyphens-auto breaks words only with a dictionary of the text's
// language, soft hyphens always break (but with hyphens-none).
func TestHyphenation(t *testing.T) {
	with := telatest.Mount(t, tela.C(article), telatest.Size(680, 160), telatest.Hyphenation(en.US, en.GB, es.Spanish, fr.French))
	without := telatest.Mount(t, tela.C(article), telatest.Size(680, 160))
	h := func(app *telatest.App, id string) float32 { return app.Bounds(telatest.ByTestID(id)).Height }
	// Chromium: "Extraordinary / international- / ization of / hyphenation".
	if got := h(with, "auto"); got != 96 {
		t.Errorf("hyphens-auto with en.US: %g px high, want 4 lines", got)
	}
	// The word that does not fit breaks only with a dictionary.
	if got := h(with, "word"); got != 72 {
		t.Errorf("a long word with en.US: %g px high, want 3 lines", got)
	}
	if got := h(without, "word"); got != 48 {
		t.Errorf("a long word without a dictionary: %g px high, want 2 lines", got)
	}
	if h(with, "manual") != h(without, "manual") || h(with, "manual") != 72 {
		t.Errorf("soft hyphens: %g and %g px high, want 3 lines", h(with, "manual"), h(without, "manual"))
	}
	if got := h(with, "none"); got != 48 {
		t.Errorf("hyphens-none: %g px high, want 2 lines", got)
	}
	with.GoldenScales("hyphens", 1, 2)
}

// TestFromApp checks that telatest.FromApp takes the app's dictionaries.
func TestFromApp(t *testing.T) {
	app := tela.App{Width: 680, Height: 160, Hyphenation: []tela.Hyphenation{en.US}, Root: tela.C(article)}
	a := telatest.MountApp(t, app)
	if got := a.Bounds(telatest.ByTestID("auto")).Height; got != 96 {
		t.Errorf("hyphens-auto through FromApp: %g px high, want 4 lines", got)
	}
}
