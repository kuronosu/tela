package main

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

// This file holds the words of the showcase: what each card says and how
// it is styled. Every paragraph is built (shaped) here, once.
//
// The CJK card only uses characters of internal/text/testdata/fonts/
// cjk_sample.txt, so that the test-only subset font covers them.

type tstyle = text.TextStyle

// seg is a piece of a paragraph: text in a style, or an inline box.
type seg struct {
	s   string
	st  tstyle
	box *text.InlineBox
}

func t(s string, st tstyle) seg { return seg{s: s, st: st} }

// sans is Inter at the given size, line height and weight.
func sans(size, lh, weight float32, c gfx.Color) tstyle {
	return tstyle{Size: size, Weight: weight, LineHeight: text.LineHeightPixels(lh), Paint: c}
}

// mono is JetBrains Mono (the monospace generic family).
func mono(size, lh float32, c gfx.Color) tstyle {
	return tstyle{Families: []string{"monospace"}, Size: size, Weight: 400, LineHeight: text.LineHeightPixels(lh), Paint: c}
}

func bold(st tstyle, weight float32, c gfx.Color) tstyle {
	st.Weight, st.Paint = weight, c
	return st
}

func tracking(st tstyle, px float32) tstyle {
	st.LetterSpacing = px
	return st
}

// build shapes a paragraph of segs; def is the block's style.
func build(c *text.Collection, ps text.ParagraphStyle, def tstyle, segs ...seg) *para {
	p := text.Paragraph{Style: ps, Default: def}
	for _, s := range segs {
		if s.box != nil {
			p.AddBox(*s.box)
		} else {
			p.AddText(s.s, s.st)
		}
	}
	return &para{lay: text.NewLayout(c, &p), boxes: len(p.Boxes)}
}

// one wraps a paragraph in a block of its own.
func one(mt float32, p *para) block { return block{mt: mt, row: []*para{p}} }

// label is the small uppercase caption at the top of a card.
func label(c *text.Collection, s string, dark bool) block {
	col := indigo600
	if dark {
		col = slate400
	}
	st := tracking(sans(11, 16, 600, col), 0.9)
	st.Transform = text.TransformUppercase
	return one(0, build(c, text.ParagraphStyle{WhiteSpace: text.WhiteSpaceNowrap}, st, t(s, st)))
}

func headerBlocks(c *text.Collection) []block {
	eyebrow := tracking(sans(12, 16, 600, indigo600), 1.2)
	eyebrow.Transform = text.TransformUppercase
	title := tracking(sans(30, 38, 700, slate900), -0.6)
	sub := sans(15, 24, 400, slate500)
	return []block{
		one(0, build(c, text.ParagraphStyle{}, eyebrow, t("Tela · phase 2", eyebrow))),
		one(4, build(c, text.ParagraphStyle{}, title, t("Every script, one paragraph", title))),
		one(6, build(c, text.ParagraphStyle{}, sub,
			t("Shaped once, re-broken at any width. Click, drag, double-click or triple-click a paragraph to select it.", sub))),
	}
}

func latinCard(c *text.Collection) *card {
	h := tracking(sans(20, 28, 600, slate900), -0.4)
	body := sans(14, 22, 400, slate600)
	key := bold(body, 600, slate900)

	spec := sans(26, 32, 700, slate900)
	cap := sans(11, 16, 500, slate400)
	off := spec
	off.Paint = slate400
	off.Features = []text.Feature{{Tag: "kern", Value: 0}}
	pre := text.ParagraphStyle{WhiteSpace: text.WhiteSpacePre} // never wraps: a specimen

	return &card{blocks: []block{
		label(c, "Latin · kerning and ligatures", false),
		one(6, build(c, text.ParagraphStyle{}, h, t("Shape once, break many times", h))),
		one(6, build(c, text.ParagraphStyle{}, body,
			t("Every paragraph is shaped a single time. Resizing the window only ", body),
			t("re-breaks the lines", key),
			t(" -> no reshaping, so ", body),
			t("kerning", key),
			t(", ligatures and optical sizes stay where the type designer put them.", body))),
		{mt: 12, row: []*para{
			build(c, pre, cap, t("AVATAR Toy\n", spec), t("kerning on", cap)),
			build(c, pre, cap, t("AVATAR Toy\n", off), t("kerning off", cap)),
		}},
	}}
}

func arabicCard(c *text.Collection) *card {
	ar := sans(16, 28, 400, slate800)
	small := sans(14, 24, 400, slate700)
	strong := bold(small, 600, slate900)
	rtl := text.ParagraphStyle{Direction: text.DirectionRTL, Align: text.AlignJustify}

	return &card{blocks: []block{
		label(c, "Arabic · right to left, justified, bidi", false),
		one(8, build(c, rtl, ar, t(
			"النص العربي يُكتب من اليمين إلى اليسار، "+
				"وتتصل حروفه ببعضها لتكوّن الكلمات. "+
				"تضبط Tela اتجاه كل مقطع تلقائيًا، "+
				"وتكسر الأسطر عند أنسب موضع.", ar))),
		// Arabic base direction with Latin words and numbers.
		one(10, build(c, text.ParagraphStyle{Direction: text.DirectionAuto}, small,
			t("سعر الكتاب ", small),
			t("1,250.50", strong),
			t(" ريال، رقم الطلب ", small),
			t("4096", strong),
			t(" في 12 أكتوبر 2026.", small))),
		// Left-to-right base direction with an Arabic word inside.
		one(4, build(c, text.ParagraphStyle{Direction: text.DirectionAuto}, small,
			t("The word ", small),
			t("كتاب", strong),
			t(" means book; it ships in 3 days.", small))),
	}}
}

func cjkCard(c *text.Collection) *card {
	cjk := sans(15, 25, 400, slate800)
	return &card{blocks: []block{
		label(c, "CJK · 日本語 · 中文", false),
		one(8, build(c, text.ParagraphStyle{}, cjk,
			t("吾輩は猫である。名前はまだ無い。どこで生まれたかとんと見当がつかぬ。", cjk))),
		one(6, build(c, text.ParagraphStyle{}, cjk,
			t("混合排版时，英文单词和数字也会出现在中文句子里，例如 ", cjk),
			t("Tela", bold(cjk, 600, slate900)),
			t(" 是一个用 Go 语言写成的界面库，版本号是 2.0。", cjk))),
	}}
}

func emojiCard(c *text.Collection) *card {
	em := sans(22, 34, 400, slate800)
	return &card{blocks: []block{
		label(c, "Emoji · color glyphs", false),
		one(6, build(c, text.ParagraphStyle{}, em, t(
			"Family \U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466 coder \U0001F469\u200D\U0001F4BB flags \U0001F1E8\U0001F1F4 \U0001F1EF\U0001F1F5 "+
				"tone \U0001F44B\U0001F3FD art \U0001F3A8\U0001F680\U0001F308\U0001F525\u2728 inside a line of text", em))),
	}}
}

// codeSeg is a token of the code sample.
type codeSeg struct {
	kind byte // k keyword, f function, s string, n number, c comment, 0 plain
	s    string
}

func codeCard(c *text.Collection) *card {
	base := mono(13, 20, slate300)
	styles := map[byte]tstyle{
		'k': bold(base, 400, violet300),
		'f': bold(base, 400, sky300),
		's': bold(base, 400, emerald3),
		'n': bold(base, 400, amber300),
		'c': func() tstyle { s := bold(base, 400, slate500); s.Style = text.StyleItalic; return s }(),
		'p': bold(base, 400, rose300),
		0:   base,
	}
	lines := [][]codeSeg{
		{{'c', "// shape once"}},
		{{0, "l := "}, {'f', "text.NewLayout"}, {0, "(c, &p)"}},
		{{'k', "for"}, {0, " _, w := "}, {'k', "range"}, {0, " widths {"}},
		{{0, "    l."}, {'f', "BreakLines"}, {0, "(w) "}, {'c', "// -> re-break"}},
		{{0, "    "}, {'k', "if"}, {0, " l."}, {'f', "NumLines"}, {0, "() >= "}, {'n', "3"}, {0, " && w != "}, {'n', "0"}, {0, " {"}},
		{{0, "        "}, {'f', "log"}, {0, "."}, {'f', "Println"}, {0, "("}, {'s', `"wrapped"`}, {0, ")"}},
		{{0, "    }"}},
		{{0, "}"}},
	}
	var nums, code []seg
	for i, ln := range lines {
		nl := "\n"
		if i == len(lines)-1 {
			nl = ""
		}
		nums = append(nums, t(string(rune('1'+i))+nl, mono(13, 20, slate600)))
		for _, sg := range ln {
			code = append(code, t(sg.s, styles[sg.kind]))
		}
		code = append(code, t(nl, base))
	}
	pre := text.ParagraphStyle{WhiteSpace: text.WhiteSpacePre}
	numPara := build(c, text.ParagraphStyle{WhiteSpace: text.WhiteSpacePre, Align: text.AlignRight}, mono(13, 20, slate600), nums...)
	numPara.fr = 0.07
	codePara := build(c, pre, base, code...)
	codePara.fr = 0.93
	codePara.clip = true // like overflow-x: hidden, when the card is narrower than the code
	return &card{dark: true, blocks: []block{
		label(c, "Monospace · JetBrains Mono ligatures", true),
		{mt: 10, row: []*para{numPara, codePara}},
	}}
}

func clampCard(c *text.Collection) *card {
	cap := mono(11, 16, slate400)
	line := sans(15, 22, 500, slate800)
	body := sans(14, 22, 400, slate600)
	return &card{blocks: []block{
		label(c, "Truncate and line-clamp", false),
		one(8, build(c, text.ParagraphStyle{}, cap, t("truncate", cap))),
		one(2, build(c, text.ParagraphStyle{WhiteSpace: text.WhiteSpaceNowrap, TextOverflow: text.TextOverflowEllipsis}, line,
			t("Quarterly report: revenue grew 18% year over year, led by the Asia-Pacific and Latin American markets", line))),
		one(10, build(c, text.ParagraphStyle{}, cap, t("line-clamp-3", cap))),
		one(2, build(c, text.ParagraphStyle{LineClamp: 3}, body,
			t("Text layout is the layer between shaping and painting. It breaks paragraphs into lines at UAX #14 opportunities, "+
				"aligns and justifies them, truncates what does not fit, and answers where the caret goes and what lies under a click. "+
				"None of it needs to shape the text again.", body))),
	}}
}

func detailsCard(c *text.Collection) *card {
	body := sans(15, 28, 400, slate700)
	strong := bold(body, 600, slate900)
	old := body
	old.Paint = slate400
	old.Decoration = text.TextDecoration{Lines: text.DecorationLineThrough}
	link := bold(body, 500, indigo600)
	link.Decoration = text.TextDecoration{Lines: text.DecorationUnderline, Paint: indigo300}

	cap := mono(11, 16, slate400)
	tab := sans(14, 20, 500, slate800)
	tab.Numeric = text.NumericTabular
	prop := tab
	prop.Numeric = text.NumericProportional
	right := text.ParagraphStyle{WhiteSpace: text.WhiteSpacePreLine, Align: text.AlignRight}
	nums := func(st tstyle, name string) *para {
		return build(c, right, st,
			t(name+"\n", cap),
			t("1,111.11\n8,888.88\n1,818.18", st))
	}

	doc := build(c, text.ParagraphStyle{}, body,
		t("Read the ", body),
		seg{box: &text.InlineBox{ID: 1, Width: 18, Height: 18, Descent: 3}},
		t(" docs, ", body),
		t("$49", old),
		t(" ", body),
		t("$29", strong),
		t(" today, or ", body),
		t("follow the link", link),
		t(".", body))
	doc.icon = func(b *gfx.Builder, r gfx.Rect, _ int) {
		b.FillRRect(gfx.NewRRect(r, 5), gfx.LinearGradient{Start: gfx.Pt(r.X0, r.Y0), End: gfx.Pt(r.X1, r.Y1), Stops: []gfx.GradientStop{gfx.AutoStop(indigo400), gfx.AutoStop(violet500)}})
		var p gfx.Path
		cx, cy := (r.X0+r.X1)/2, (r.Y0+r.Y1)/2
		p.MoveTo(gfx.Pt(cx, cy-5))
		p.LineTo(gfx.Pt(cx+3.5, cy))
		p.LineTo(gfx.Pt(cx, cy+5))
		p.LineTo(gfx.Pt(cx-3.5, cy))
		p.Close()
		b.FillPath(p, gfx.NonZero, gfx.White)
	}

	return &card{blocks: []block{
		label(c, "Decoration · inline box · tabular-nums", false),
		one(6, doc),
		{mt: 8, row: []*para{nums(tab, "tabular-nums"), nums(prop, "proportional")}},
	}}
}
