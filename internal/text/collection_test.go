package text

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-text/typesetting/language"
)

// fakeFace returns a typeface record with only the matching properties.
func fakeFace(name string, style Style, wlo, whi, slo, shi float32) *Typeface {
	return &Typeface{name: name, style: style, weight: [3]float32{wlo, wlo, whi}, stretch: [3]float32{slo, slo, shi}}
}

func TestMatchWeight(t *testing.T) {
	static := func(ws ...float32) []*Typeface {
		var out []*Typeface
		for _, w := range ws {
			out = append(out, fakeFace(fmt.Sprint(w), StyleNormal, w, w, 100, 100))
		}
		return out
	}
	for _, c := range []struct {
		faces []*Typeface
		want  float32
		match string
	}{
		// CSS Fonts 4 §5.2: 400-500 look up to 500 first, then down, then up.
		{static(300, 500, 700), 400, "500"},
		{static(300, 600), 450, "300"},
		{static(300, 600), 400, "300"},
		{static(600, 700), 450, "600"},
		// Below 400: down first, then up.
		{static(200, 400), 300, "200"},
		{static(400, 500), 300, "400"},
		// Above 500: up first, then down.
		{static(300, 500, 800), 600, "800"},
		{static(300, 500), 600, "500"},
		{static(100, 900), 900, "900"},
		// Exact and variable ranges.
		{static(400, 700), 700, "700"},
		{[]*Typeface{fakeFace("var", StyleNormal, 100, 900, 100, 100), fakeFace("700", StyleNormal, 700, 700, 100, 100)}, 700, "var"},
		{[]*Typeface{fakeFace("light", StyleNormal, 100, 300, 100, 100), fakeFace("bold", StyleNormal, 600, 900, 100, 100)}, 450, "light"},
		{[]*Typeface{fakeFace("light", StyleNormal, 100, 300, 100, 100), fakeFace("bold", StyleNormal, 600, 900, 100, 100)}, 550, "bold"},
	} {
		if got := matchFace(c.faces, Query{Weight: c.want}); got.name != c.match {
			t.Errorf("weight %g among %v: matched %s, want %s", c.want, names(c.faces), got.name, c.match)
		}
	}
}

func TestMatchStretchAndStyle(t *testing.T) {
	faces := []*Typeface{
		fakeFace("condensed", StyleNormal, 400, 400, 75, 75),
		fakeFace("semi-expanded", StyleNormal, 400, 400, 112.5, 112.5),
		fakeFace("normal", StyleNormal, 400, 400, 100, 100),
		fakeFace("italic", StyleItalic, 400, 400, 100, 100),
		fakeFace("oblique", StyleOblique, 400, 400, 100, 100),
	}
	for _, c := range []struct {
		q     Query
		match string
	}{
		{Query{Stretch: 87.5}, "condensed"},     // narrower first when <= 100%
		{Query{Stretch: 125}, "semi-expanded"},  // wider first when > 100%
		{Query{Stretch: 150}, "semi-expanded"},  // then narrower
		{Query{Stretch: 50}, "condensed"},       // the narrowest available
		{Query{}, "normal"},                     // stretch narrows to the 100% faces first
		{Query{Style: StyleItalic}, "italic"},   // exact
		{Query{Style: StyleOblique}, "oblique"}, // exact
	} {
		if got := matchFace(faces, c.q); got.name != c.match {
			t.Errorf("%+v: matched %s, want %s", c.q, got.name, c.match)
		}
	}
	// Style fallbacks.
	noItalic := []*Typeface{faces[2], faces[4]}
	if got := matchFace(noItalic, Query{Style: StyleItalic}); got.name != "oblique" {
		t.Errorf("italic without an italic face: %s, want oblique", got.name)
	}
	onlyItalic := []*Typeface{faces[2], faces[3]}
	if got := matchFace(onlyItalic, Query{Style: StyleOblique}); got.name != "italic" {
		t.Errorf("oblique without an oblique face: %s, want italic", got.name)
	}
	uprightOnly := []*Typeface{faces[2]}
	if got := matchFace(uprightOnly, Query{Style: StyleItalic}); got.name != "normal" {
		t.Errorf("italic with only an upright face: %s, want normal", got.name)
	}
}

func names(tfs []*Typeface) []string {
	var out []string
	for _, tf := range tfs {
		out = append(out, tf.name)
	}
	return out
}

// TestCollectionMatch checks matching on the embedded families: Inter's
// variable upright and italic files cover every weight.
func TestCollectionMatch(t *testing.T) {
	c := NewDefaultCollection()
	if got := c.Match("inter", Query{Style: StyleItalic}); got == nil || got.Style() != StyleItalic {
		t.Errorf("Inter italic: %v", got)
	}
	if got := c.Match("INTER", Query{Style: StyleOblique}); got == nil || got.Style() != StyleItalic {
		t.Errorf("Inter oblique: %v (want the italic face)", got)
	}
	if got := c.Match(`"Inter"`, Query{Weight: 300}); got == nil || got.Style() != StyleNormal {
		t.Errorf("Inter 300: %v", got)
	}
	if lo, hi := c.Match("Inter", Query{}).WeightRange(); lo != 100 || hi != 900 {
		t.Errorf("Inter weight range %g-%g", lo, hi)
	}
	if lo, hi := c.Match("Noto Sans Arabic", Query{}).StretchRange(); lo != 62.5 || hi != 100 {
		t.Errorf("Noto Sans Arabic stretch range %g-%g", lo, hi)
	}
	if c.Match("Comic Sans", Query{}) != nil {
		t.Error("an unknown family matched")
	}
	if got := c.Face(Query{Families: []string{"Comic Sans", "monospace"}}, 16); got.Typeface().Family() != FamilyJetBrainsMono {
		t.Errorf("unknown family then monospace: %s", got.FontName())
	}
	if got := c.Face(Query{Families: []string{"Comic Sans"}}, 16); got.Typeface().Family() != FamilyInter {
		t.Errorf("unknown family: %s, want the first fallback (Inter)", got.FontName())
	}
	if got := NewCollection().Face(Query{}, 16); got != nil {
		t.Errorf("empty collection returned %v", got)
	}
}

// runsOf itemizes s and describes the runs as "text=Family" items.
func runsOf(c *Collection, s string, q Query) string {
	rs := []rune(s)
	var parts []string
	for _, r := range c.Itemize(rs, 0, len(rs), q, 16, language.Latin, nil) {
		parts = append(parts, string(rs[r.Start:r.End])+"="+r.Face.Typeface().Family())
	}
	return strings.Join(parts, "|")
}

func TestItemize(t *testing.T) {
	c := testCollection(t)
	sans := Query{Families: []string{"sans-serif"}}
	for _, tc := range []struct {
		text, want string
		q          Query
	}{
		{"Hello, world", "Hello, world=Inter", sans},
		// Spaces take the first family that has them, as in browsers.
		{"Hello مرحبا world", "Hello =Inter|مرحبا=Noto Sans Arabic| world=Inter", sans},
		{"مرحبا بكم", "مرحبا=Noto Sans Arabic| =Inter|بكم=Noto Sans Arabic", sans},
		{"東京は桜の花", "東京は桜の花=Noto Sans JP", sans},
		{"Tokyo 東京", "Tokyo =Inter|東京=Noto Sans JP", sans},
		{"カタカナ", "カタカナ=Noto Sans JP", sans},
		// U+2764 is text by default and Inter has it; VS16 asks for the
		// emoji font and VS15 for text.
		{"❤", "❤=Inter", sans},
		{"❤️", "❤️=Noto Color Emoji", sans},
		{"❤︎", "❤︎=Inter", sans},
		// Emoji_Presentation characters go to the emoji font even though a
		// later family could have them.
		{"a😀b", "a=Inter|😀=Noto Color Emoji|b=Inter", sans},
		// ZWJ sequences, skin tones, flags and keycaps are single clusters.
		{"👩‍👩‍👧", "👩‍👩‍👧=Noto Color Emoji", sans},
		{"x👍🏽y", "x=Inter|👍🏽=Noto Color Emoji|y=Inter", sans},
		{"🇯🇵🇧🇷", "🇯🇵🇧🇷=Noto Color Emoji", sans},
		{"1️⃣", "1️⃣=Noto Color Emoji", sans},
		// Spaces between emoji take the first family that has them.
		{"😀 😀", "😀=Noto Color Emoji| =Inter|😀=Noto Color Emoji", sans},
		// Zero-width characters stay with the face before them.
		{"東\u200b京", "東\u200b京=Noto Sans JP", sans},
		// The requested family comes first; then fallbacks.
		{"code: x", "code: x=JetBrains Mono", Query{Families: []string{"monospace"}}},
		{"x مرحبا", "x =JetBrains Mono|مرحبا=Noto Sans Arabic", Query{Families: []string{"JetBrains Mono", "Inter"}}},
		// A character nobody has: the first requested face (tofu).
		{"a\U000F0000", "a\U000F0000=Inter", sans},
	} {
		if got := runsOf(c, tc.text, tc.q); got != tc.want {
			t.Errorf("Itemize(%q):\n got %s\nwant %s", tc.text, got, tc.want)
		}
	}
}

// TestItemizeRange checks the start/end arguments and that runs are
// appended.
func TestItemizeRange(t *testing.T) {
	c := NewDefaultCollection()
	rs := []rune("ab😀cd")
	out := []Run{{Start: -1}}
	out = c.Itemize(rs, 1, 4, Query{}, 16, language.Latin, out)
	if len(out) != 4 || out[1].Start != 1 || out[1].End != 2 || out[2].Start != 2 || out[2].End != 3 || out[3].End != 4 {
		t.Errorf("runs %+v", out)
	}
	if got := c.Itemize(rs, 3, 3, Query{}, 16, language.Latin, nil); len(got) != 0 {
		t.Errorf("empty range gave %v", got)
	}
}

// TestScriptFallbackOrder checks that per-script families come before the
// general fallback order, and that user fonts come before embedded ones.
func TestScriptFallbackOrder(t *testing.T) {
	c := testCollection(t)
	// The CJK subset has ASCII too, but Latin text keeps the requested
	// family; with an unknown requested family the user font (registered
	// last) still comes before the embedded ones.
	if got := runsOf(c, "abc", Query{Families: []string{"sans-serif"}}); got != "abc=Inter" {
		t.Errorf("Latin: %s", got)
	}
	if got := runsOf(c, "abc", Query{Families: []string{"Unknown"}}); got != "abc=Noto Sans JP" {
		t.Errorf("Latin with an unknown family: %s (user fonts come before embedded ones)", got)
	}
	c.SetScriptFallback(language.Arabic) // none
	if got := runsOf(c, "مرحبا", Query{Families: []string{"sans-serif"}}); got != "مرحبا=Noto Sans Arabic" {
		t.Errorf("Arabic without a script fallback: %s (found by coverage)", got)
	}
}
