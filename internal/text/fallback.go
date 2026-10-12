package text

import (
	"unicode"

	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/segmenter"
)

// Run is a range of text that one face draws.
type Run struct {
	Start, End int // rune indices, End exclusive
	Face       *Face
}

// Itemize splits text[start:end] into runs that each use one face, for
// shaping. Font selection works per grapheme cluster (UAX #29, so emoji ZWJ
// sequences, flags, keycaps and skin tones are single clusters): a cluster
// takes the first face, in the Collection's fallback order, whose cmap has
// every character of the cluster other than variation selectors, joiners
// and emoji tags. When no face covers the whole cluster, the first face
// that has its first character is used, and failing that the first
// requested face (which then shows .notdef).
//
// Emoji presentation: a cluster with VS16 (U+FE0F), a character with
// Emoji_Presentation, a ZWJ sequence or a skin tone modifier tries the
// emoji families first; a cluster with VS15 (U+FE0E) tries every other
// family before the color ones. So "❤" (U+2764, text by default) uses a
// text font when one has the character and "❤️" uses the emoji font.
//
// Clusters of one control, line or paragraph separator or default
// ignorable character keep the face of the preceding cluster. Spaces
// resolve like any other character, so a space between two Arabic words
// takes the first requested family that has it (Inter, say), as in
// browsers: Blink shapes with the first available font and falls back only
// for the characters it lacks.
//
// script is the script of the text (from itemization); it chooses the
// per-script fallback for clusters whose characters are all Common or
// Inherited. Runs are appended to out. size is the font size in CSS pixels
// (optical size). The text slice is not retained.
//
// The API takes runes rather than a go-text shaping.Fontmap because
// Fontmap resolves one rune at a time and cannot see variation selectors
// or emoji sequences; T4 can feed the runs to go-text's shaper with
// Face.ShapingFace.
func (c *Collection) Itemize(text []rune, start, end int, q Query, size float32, script language.Script, out []Run) []Run {
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	if start >= end {
		return out
	}
	cands := c.candidates(q, size)
	var seg segmenter.Segmenter
	seg.Init(text[start:end])
	it := seg.GraphemeIterator()
	var cur *Face
	runStart := start
	for it.Next() {
		g := it.Grapheme()
		cs := start + g.Offset
		cluster := g.Text
		var f *Face
		if cur != nil && keepsFace(cluster) {
			f = cur
		} else {
			f = cands.resolve(cluster, script)
		}
		if f != cur {
			if cur != nil {
				out = append(out, Run{runStart, cs, cur})
			}
			cur, runStart = f, cs
		}
	}
	if cur != nil {
		out = append(out, Run{runStart, end, cur})
	}
	return out
}

// keepsFace reports whether a one-character cluster that does not need a
// glyph of its own stays with the current face.
func keepsFace(cluster []rune) bool {
	if len(cluster) != 1 {
		return false
	}
	r := cluster[0]
	return unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || isIgnorable(r)
}

// isIgnorable reports the characters that never need a glyph of their own:
// variation selectors, joiners, emoji tags and other format characters
// that fonts commonly lack.
func isIgnorable(r rune) bool {
	switch {
	case r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF: // variation selectors
		return true
	case r == 0x200C, r == 0x200D, r == 0x200B, r == 0x2060, r == 0xFEFF, r == 0x034F: // ZWNJ, ZWJ, ZWSP, WJ, BOM, CGJ
		return true
	case r >= 0x200E && r <= 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069: // bidi controls
		return true
	case r >= 0xE0000 && r <= 0xE007F: // tags
		return true
	}
	return false
}

// presentation is the emoji presentation of a cluster.
type presentation uint8

const (
	presDefault presentation = iota // text
	presEmoji
	presText // explicit VS15
)

func clusterPresentation(cluster []rune) presentation {
	hasZWJ := false
	for _, r := range cluster {
		switch r {
		case 0xFE0F:
			return presEmoji
		case 0xFE0E:
			return presText
		case 0x200D:
			hasZWJ = true
		}
	}
	first := cluster[0]
	switch {
	case unicode.Is(emojiPresentation, first):
		return presEmoji
	case hasZWJ && unicode.Is(emojiProperty, first):
		return presEmoji
	case len(cluster) > 1 && unicode.Is(emojiModifierBase, first) && cluster[1] >= 0x1F3FB && cluster[1] <= 0x1F3FF:
		return presEmoji
	}
	return presDefault
}

// clusterScript returns the script of the first character of the cluster
// that is not Common or Inherited, or def.
func clusterScript(cluster []rune, def language.Script) language.Script {
	for _, r := range cluster {
		if s := language.LookupScript(r); s != language.Common && s != language.Inherited && s != language.Unknown {
			return s
		}
	}
	return def
}

// covers reports whether f has every character of the cluster that needs
// a glyph.
func covers(f *Face, cluster []rune) bool {
	any := false
	for _, r := range cluster {
		if isIgnorable(r) {
			continue
		}
		if !f.HasRune(r) {
			return false
		}
		any = true
	}
	return any
}

// resolve picks the face of one cluster.
func (cs *candidates) resolve(cluster []rune, script language.Script) *Face {
	pres := clusterPresentation(cluster)
	sc := clusterScript(cluster, script)
	// Kana and Hrkt fall back to the Han list when they have none of their own.
	scriptFaces := cs.scripts[sc]
	if len(scriptFaces) == 0 && (sc == language.Hiragana || sc == language.Katakana || sc == language.Katakana_Or_Hiragana) {
		scriptFaces = cs.scripts[language.Han]
	}

	var order [5][]*Face
	switch pres {
	case presEmoji:
		order = [5][]*Face{cs.emoji, cs.primary, scriptFaces, cs.rest}
	default:
		order = [5][]*Face{cs.primary, scriptFaces, cs.rest}
	}
	try := func(ok func(*Face) bool) *Face {
		for _, list := range order {
			for _, f := range list {
				if pres == presText && f.tf.IsColor() {
					continue
				}
				if ok(f) {
					return f
				}
			}
		}
		return nil
	}
	if f := try(func(f *Face) bool { return covers(f, cluster) }); f != nil {
		return f
	}
	if pres == presText {
		// No text font has it: a color font is better than nothing.
		pres = presDefault
		if f := try(func(f *Face) bool { return covers(f, cluster) }); f != nil {
			return f
		}
	}
	first := cluster[0]
	if f := try(func(f *Face) bool { return f.HasRune(first) }); f != nil {
		return f
	}
	for _, list := range order {
		if len(list) > 0 {
			return list[0]
		}
	}
	return nil
}
