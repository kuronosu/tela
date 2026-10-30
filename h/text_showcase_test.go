package h_test

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
	"github.com/kuronosu/tela/internal/text"
)

// The text showcase is one static page of cards with the text features
// that elements and classes can express: Latin with kerning and the "->"
// ligature, justified right-to-left Arabic with Latin words and numbers
// (bidi), Japanese and Chinese, color emoji (ZWJ sequences, flags, skin
// tones), a monospace block with ligatures, truncate and line-clamp-3,
// underline and line-through, an inline box on the baseline and tabular
// against proportional figures. It is the page-level text golden of the
// framework: it began as the `text` example, the phase-2 showcase that
// drove internal/text by hand, whose paragraphs and features it keeps.
// The text layer has its own goldens (internal/text/testdata); this one
// checks the same content through `h` elements and Tailwind classes.

const textShowW, textShowH = 960, 920

var (
	textShowOnce  sync.Once
	textShowFonts *text.Collection
)

// textShowCollection is the embedded collection plus the CJK subset the
// text package's goldens use: the embedded fonts have no CJK glyphs
// (ADR-031), and system fonts are never used in tests.
func textShowCollection(tb testing.TB) *text.Collection {
	tb.Helper()
	textShowOnce.Do(func() {
		c := text.NewDefaultCollection()
		data, err := os.ReadFile("../internal/text/testdata/fonts/NotoSansJP-Subset.otf")
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := c.Register(data, "Noto Sans JP"); err != nil {
			tb.Fatal(err)
		}
		c.SetScriptFallback(language.Han, "Noto Sans JP")
		textShowFonts = c
	})
	return textShowFonts
}

// tsCard is a card: a small uppercase caption and its content.
func tsCard(caption, extra string, children ...tela.Child) tela.Node {
	captionColor := "text-indigo-600"
	if strings.Contains(extra, "bg-slate-900") {
		captionColor = "text-slate-400"
	}
	body := []tela.Child{
		Class("flex flex-col gap-2 rounded-2xl border p-5 shadow-sm", extra),
		Div(Class("text-[11px] leading-4 font-semibold tracking-[0.08em] uppercase", captionColor), Text(caption)),
	}
	return Div(append(body, children...)...)
}

// textShowcase is the page: two columns of cards.
func textShowcase() tela.Node {
	light := "border-slate-200 bg-white text-slate-600"
	left := []tela.Child{
		Class("flex w-0 flex-1 flex-col gap-4"),
		tsLatinCard(light), tsCodeCard(), tsClampCard(light),
	}
	right := []tela.Child{
		Class("flex w-0 flex-1 flex-col gap-4"),
		tsArabicCard(light), tsCJKCard(light), tsEmojiCard(light), tsDecorationCard(light),
	}
	return Div(Class("min-h-screen bg-slate-100 p-8 font-sans"),
		Div(Class("mb-6 flex flex-col gap-1"),
			Div(Class("text-xs leading-4 font-semibold tracking-[0.1em] text-indigo-600 uppercase"), Text("Tela · text")),
			H1(Class("text-3xl leading-[38px] font-bold tracking-tight text-slate-900"), Text("Every script, one paragraph")),
			P(Class("text-[15px] leading-6 text-slate-500"),
				Text("Shaped once, re-broken at any width.")),
		),
		Div(Class("flex items-start gap-4"), Div(left...), Div(right...)),
	)
}

func tsLatinCard(extra string) tela.Node {
	return tsCard("Latin · kerning and ligatures", extra,
		H2(Class("text-xl leading-7 font-semibold tracking-tight text-slate-900"), Text("Shape once, break many times")),
		P(Class("text-sm leading-[22px]"),
			Text("Every paragraph is shaped a single time. Resizing the window only "),
			Strong(Class("font-semibold text-slate-900"), Text("re-breaks the lines")),
			Text(" -> no reshaping, so "),
			Strong(Class("font-semibold text-slate-900"), Text("kerning")),
			Text(", ligatures and optical sizes stay where the type designer put them.")),
		Div(Class("mt-2 text-[26px] leading-8 font-bold text-slate-900"), Text("AVATAR Toy")),
	)
}

func tsArabicCard(extra string) tela.Node {
	return tsCard("Arabic · right to left, justified, bidi", extra,
		P(Class("text-base leading-7 text-slate-800 text-justify"), Dir("rtl"),
			Text("النص العربي يُكتب من اليمين إلى اليسار، "+
				"وتتصل حروفه ببعضها لتكوّن الكلمات. "+
				"تضبط Tela اتجاه كل مقطع تلقائيًا، "+
				"وتكسر الأسطر عند أنسب موضع.")),
		// Arabic base direction with Latin words and numbers.
		P(Class("text-sm leading-6 text-slate-700"), Dir("auto"),
			Text("سعر الكتاب "),
			Strong(Class("font-semibold text-slate-900"), Text("1,250.50")),
			Text(" ريال، رقم الطلب "),
			Strong(Class("font-semibold text-slate-900"), Text("4096")),
			Text(" في 12 أكتوبر 2026.")),
		// Left-to-right base direction with an Arabic word inside.
		P(Class("text-sm leading-6 text-slate-700"), Dir("auto"),
			Text("The word "),
			Strong(Class("font-semibold text-slate-900"), Text("كتاب")),
			Text(" means book; it ships in 3 days.")),
	)
}

func tsCJKCard(extra string) tela.Node {
	return tsCard("CJK · 日本語 · 中文", extra,
		P(Class("text-[15px] leading-[25px] text-slate-800"),
			Text("吾輩は猫である。名前はまだ無い。どこで生まれたかとんと見当がつかぬ。")),
		P(Class("text-[15px] leading-[25px] text-slate-800"),
			Text("混合排版时，英文单词和数字也会出现在中文句子里，例如 "),
			Strong(Class("font-semibold text-slate-900"), Text("Tela")),
			Text(" 是一个用 Go 语言写成的界面库，版本号是 2.0。")),
	)
}

func tsEmojiCard(extra string) tela.Node {
	return tsCard("Emoji · color glyphs", extra,
		P(Class("text-[22px] leading-[34px] text-slate-800"),
			Text("Family \U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466 coder \U0001F469\u200D\U0001F4BB "+
				"flags \U0001F1E8\U0001F1F4 \U0001F1EF\U0001F1F5 tone \U0001F44B\U0001F3FD "+
				"art \U0001F3A8\U0001F680\U0001F308\U0001F525\u2728 inside a line of text")),
	)
}

// tsCodeCard is the dark card: line numbers and highlighted monospace
// code, whose "->", ">=" and "!=" are JetBrains Mono ligatures.
func tsCodeCard() tela.Node {
	type seg struct{ class, s string }
	const (
		kw  = "text-violet-300"
		fn  = "text-sky-300"
		str = "text-emerald-300"
		num = "text-amber-300"
		com = "text-slate-500 italic"
	)
	lines := [][]seg{
		{{com, "// shape once"}},
		{{"", "l := "}, {fn, "text.NewLayout"}, {"", "(c, &p)"}},
		{{kw, "for"}, {"", " _, w := "}, {kw, "range"}, {"", " widths {"}},
		{{"", "    l."}, {fn, "BreakLines"}, {"", "(w) "}, {com, "// -> re-break"}},
		{{"", "    "}, {kw, "if"}, {"", " l."}, {fn, "NumLines"}, {"", "() >= "}, {num, "3"}, {"", " && w != "}, {num, "0"}, {"", " {"}},
		{{"", "        "}, {fn, "log"}, {"", "."}, {fn, "Println"}, {"", "("}, {str, `"wrapped"`}, {"", ")"}},
		{{"", "    }"}},
		{{"", "}"}},
	}
	var nums strings.Builder
	var code []tela.Child
	code = append(code, Class("min-w-0 flex-1 overflow-hidden whitespace-pre text-slate-300"))
	for i, ln := range lines {
		if i > 0 {
			nums.WriteString("\n")
			code = append(code, Text("\n"))
		}
		fmt.Fprintf(&nums, "%d", i+1)
		for _, s := range ln {
			if s.class == "" {
				code = append(code, Text(s.s))
			} else {
				code = append(code, Span(Class(s.class), Text(s.s)))
			}
		}
	}
	return tsCard("Monospace · JetBrains Mono ligatures", "border-slate-800 bg-slate-900 text-slate-400",
		Div(Class("mt-2 flex gap-4 font-mono text-[13px] leading-5"),
			Div(Class("w-6 text-right whitespace-pre text-slate-600"), Text(nums.String())),
			Div(code...),
		),
	)
}

func tsClampCard(extra string) tela.Node {
	return tsCard("Truncate and line-clamp", extra,
		Div(Class("mt-2 font-mono text-[11px] leading-4 text-slate-400"), Text("truncate")),
		P(Class("truncate text-[15px] leading-[22px] font-medium text-slate-800"),
			Text("Quarterly report: revenue grew 18% year over year, led by the Asia-Pacific and Latin American markets")),
		Div(Class("mt-2 font-mono text-[11px] leading-4 text-slate-400"), Text("line-clamp-3")),
		P(Class("line-clamp-3 text-sm leading-[22px]"),
			Text("Text layout is the layer between shaping and painting. It breaks paragraphs into lines at UAX #14 opportunities, "+
				"aligns and justifies them, truncates what does not fit, and answers where the caret goes and what lies under a click. "+
				"None of it needs to shape the text again.")),
	)
}

func tsDecorationCard(extra string) tela.Node {
	figures := func(name, class string) tela.Node {
		return Div(Class("flex-1 text-right text-sm leading-5 font-medium whitespace-pre-line text-slate-800 "+class),
			Div(Class("font-mono text-[11px] leading-4 font-normal text-slate-400"), Text(name)),
			Text("1,111.11\n8,888.88\n1,818.18"))
	}
	return tsCard("Decoration · inline box · tabular-nums", extra,
		P(Class("text-[15px] leading-7 text-slate-700"),
			Text("Read the "),
			Span(Class("inline-block size-[18px] rounded-md bg-linear-to-br from-indigo-400 to-violet-500 align-middle")),
			Text(" docs, "),
			Span(Class("text-slate-400 line-through"), Text("$49")),
			Text(" "),
			Strong(Class("font-semibold text-slate-900"), Text("$29")),
			Text(" today, or "),
			A(Class("font-medium text-indigo-600 underline decoration-indigo-300"), Text("follow the link")),
			Text(".")),
		Div(Class("mt-2 flex gap-4"),
			figures("tabular-nums", "tabular-nums"),
			figures("proportional", "proportional-nums")),
	)
}

// TestTextShowcaseGolden renders the page at 960x920, at scales 1 and 2,
// bit by bit. Dev mode is on: an unknown class or any other dev warning
// fails the test with core's message.
func TestTextShowcaseGolden(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			_, img := core.Render(core.Component(textShowcase), core.RenderOptions{
				Config: core.Config{Dev: true, Fonts: textShowCollection(t), Log: func(m string) { t.Error(m) }},
				Width:  textShowW, Height: textShowH, Scale: scale,
			})
			golden.Image(t, fmt.Sprintf("text_showcase_x%g", scale), img)
		})
	}
}

// TestTextShowcaseNarrow shows the page at the width of a phone: the same
// content reflowed, with no dev warning at any size.
func TestTextShowcaseNarrow(t *testing.T) {
	for _, s := range [][2]float32{{420, 1700}, {700, 1100}, {1280, 800}, {1, 1}} {
		core.Render(core.Component(textShowcase), core.RenderOptions{
			Config: core.Config{Dev: true, Fonts: textShowCollection(t), Log: func(m string) { t.Errorf("%vx%v: %s", s[0], s[1], m) }},
			Width:  s[0], Height: s[1], Scale: 1,
		})
	}
}

// TestTextShowcaseNoMissingGlyphs guards the content: every character of
// the page must be covered by the embedded fonts or the CJK subset, so
// that goldens show glyphs and never missing-glyph boxes.
func TestTextShowcaseNoMissingGlyphs(t *testing.T) {
	dl, _ := core.Render(core.Component(textShowcase), core.RenderOptions{
		Config: core.Config{Dev: true, Fonts: textShowCollection(t), Log: func(m string) { t.Error(m) }},
		Width:  textShowW, Height: textShowH, Scale: 1,
	})
	runs := 0
	for i := range dl.Len() {
		run, ok := dl.At(i).(gfx.DrawGlyphRun)
		if !ok {
			continue
		}
		runs++
		for _, g := range run.Glyphs {
			if g.ID == 0 {
				t.Errorf("a glyph run of %s at %v draws a missing glyph (.notdef)", run.Font.FontName(), run.Origin)
				break
			}
		}
	}
	if runs < 40 {
		t.Errorf("the page painted %d glyph runs, want at least 40", runs)
	}
}
