package xim

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// Compound Text (X Consortium standard, "Compound Text Encoding", version
// 1.1) is ISO 2022 with fixed rules: GL starts as ASCII, GR as the right
// half of ISO 8859-1, and escape sequences designate other sets into GL
// (ESC ( F, ESC $ ( F) or GR (ESC ) F, ESC - F, ESC $ ) F). Input
// methods that cannot send UTF-8 send it: ibus's XIM frontend always,
// fcitx5's when the client does not negotiate UTF8_STRING. They convert
// with libX11's (or xcb-imdkit's copy of its) UTF-8 converter, which picks
// for each character the first charset that has it in this order: ISO
// 8859-1..16, JIS X 0201, the 94x94 sets JIS X 0208, JIS X 0212, KS C 5601
// and GB 2312, and as a fallback the UTF-8 segment ESC % G ... ESC % @.
// DecodeCompoundText reads all of those (the CJK sets through the EUC
// decoders of golang.org/x/text, which take the GR form of their bytes);
// CNS 11643 and Big5 extended segments become U+FFFD.

// The charsets a designation can name.
const (
	csNone = iota
	csASCII
	csJISRoman    // JIS X 0201 Roman: ASCII with ¥ and ‾
	csJISKana     // JIS X 0201 Katakana
	csLatin       // a 96-character ISO 8859 right half (latin field)
	csTIS620      // ISO 8859-11
	csJIS0208     // 94x94
	csJIS0212     // 94x94
	csKSC5601     // 94x94
	csGB2312      // 94x94
	csUnsupported // a set we cannot decode: U+FFFD per character
	csUnsupported2
)

type ctSet struct {
	cs    int
	latin *charmap.Charmap // csLatin; nil is ISO 8859-1
}

func (s ctSet) wide() bool {
	return s.cs == csJIS0208 || s.cs == csJIS0212 || s.cs == csKSC5601 || s.cs == csGB2312 || s.cs == csUnsupported2
}

// latinSets are the 96-character sets of ESC - F.
var latinSets = map[byte]*charmap.Charmap{
	'A': nil, // ISO 8859-1, decoded directly
	'B': charmap.ISO8859_2, 'C': charmap.ISO8859_3, 'D': charmap.ISO8859_4,
	'L': charmap.ISO8859_5, 'G': charmap.ISO8859_6, 'F': charmap.ISO8859_7,
	'H': charmap.ISO8859_8, 'M': charmap.ISO8859_9, 'V': charmap.ISO8859_10,
	'Y': charmap.ISO8859_13, '_': charmap.ISO8859_14, 'b': charmap.ISO8859_15,
	'f': charmap.ISO8859_16,
}

// DecodeCompoundText returns the UTF-8 text of a COMPOUND_TEXT string.
// Bytes it cannot decode become U+FFFD; control characters other than
// tab and newline are dropped.
func DecodeCompoundText(b []byte) string {
	var out strings.Builder
	gl, gr := ctSet{cs: csASCII}, ctSet{cs: csLatin}
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0x1b:
			i = ctEscape(b, i, &gl, &gr, &out)
		case c == 0x9b: // CSI: direction and other controls, skipped
			i++
			for i < len(b) && (b[i] < 0x40 || b[i] > 0x7e) {
				i++
			}
			i++
		case c == '\t' || c == '\n' || c == ' ':
			out.WriteByte(c) // 0x20 is SPACE whatever GL holds
			i++
		case c < 0x20 || (c >= 0x7f && c < 0xa0):
			i++ // other controls (and DEL) carry no text
		default:
			// A run of graphic bytes of the same half: 0x21-0x7e in
			// GL, 0xa0-0xff in GR.
			gl0 := c < 0x80
			j := i + 1
			for j < len(b) && (gl0 && b[j] >= 0x21 && b[j] <= 0x7e || !gl0 && b[j] >= 0xa0) {
				j++
			}
			set := gl
			if !gl0 {
				set = gr
			}
			ctRun(&out, set, b[i:j])
			i = j
		}
	}
	return out.String()
}

// ctEscape handles the escape sequence at b[i] and returns the index after
// it (and after the segment it introduces, for UTF-8 and extended
// segments).
func ctEscape(b []byte, i int, gl, gr *ctSet, out *strings.Builder) int {
	rest := b[i+1:]
	switch {
	case len(rest) >= 2 && rest[0] == '(':
		*gl = set94(rest[1])
		return i + 3
	case len(rest) >= 2 && rest[0] == ')':
		*gr = set94(rest[1])
		return i + 3
	case len(rest) >= 2 && rest[0] == '-':
		*gr = set96(rest[1])
		return i + 3
	case len(rest) >= 3 && rest[0] == '$' && (rest[1] == '(' || rest[1] == ')'):
		s := set94x94(rest[2])
		if rest[1] == '(' {
			*gl = s
		} else {
			*gr = s
		}
		return i + 4
	case len(rest) >= 2 && rest[0] == '%' && rest[1] == 'G':
		// UTF-8 until ESC % @ (or the end).
		start := i + 3
		end := start
		for end < len(b) && !(b[end] == 0x1b && end+2 < len(b) && b[end+1] == '%' && b[end+2] == '@') {
			end++
		}
		out.WriteString(strings.ToValidUTF8(string(b[start:end]), "�"))
		if end < len(b) {
			return end + 3
		}
		return end
	case len(rest) >= 2 && rest[0] == '%' && rest[1] == '@':
		return i + 3 // end of a UTF-8 segment that was not open
	case len(rest) >= 5 && rest[0] == '%' && rest[1] == '/':
		// Extended segment: ESC % / F M L, then M L bytes: the encoding
		// name, STX, and the text.
		n := int(rest[3]&0x7f)*128 + int(rest[4]&0x7f)
		start := i + 6
		end := min(start+n, len(b))
		seg := b[start:end]
		name, text, ok := strings.Cut(string(seg), "\x02")
		if ok {
			ctExtended(out, strings.ToLower(name), []byte(text))
		}
		return end
	}
	// An unknown or truncated sequence: skip ESC and its intermediate
	// bytes (0x20-0x2f) up to the final byte.
	j := i + 1
	for j < len(b) && b[j] >= 0x20 && b[j] <= 0x2f {
		j++
	}
	if j < len(b) {
		j++
	}
	return j
}

func set94(f byte) ctSet {
	switch f {
	case 'B':
		return ctSet{cs: csASCII}
	case 'J':
		return ctSet{cs: csJISRoman}
	case 'I':
		return ctSet{cs: csJISKana}
	}
	return ctSet{cs: csUnsupported}
}

func set96(f byte) ctSet {
	if m, ok := latinSets[f]; ok {
		return ctSet{cs: csLatin, latin: m}
	}
	if f == 'T' {
		return ctSet{cs: csTIS620}
	}
	return ctSet{cs: csUnsupported}
}

func set94x94(f byte) ctSet {
	switch f {
	case 'A':
		return ctSet{cs: csGB2312}
	case 'B', '@':
		return ctSet{cs: csJIS0208}
	case 'C':
		return ctSet{cs: csKSC5601}
	case 'D':
		return ctSet{cs: csJIS0212}
	}
	return ctSet{cs: csUnsupported2}
}

// ctRun decodes a run of graphic bytes of one half (GL or GR) in set.
func ctRun(out *strings.Builder, set ctSet, run []byte) {
	if set.wide() {
		ctWide(out, set.cs, run)
		return
	}
	for _, c := range run {
		g := c & 0x7f // position in the 94/96 table
		switch set.cs {
		case csASCII:
			out.WriteByte(g)
		case csJISRoman:
			switch g {
			case 0x5c:
				out.WriteRune('¥')
			case 0x7e:
				out.WriteRune('‾')
			default:
				out.WriteByte(g)
			}
		case csJISKana:
			if g >= 0x21 && g <= 0x5f {
				out.WriteRune(0xff61 + rune(g-0x21))
			} else {
				out.WriteRune(utf8.RuneError)
			}
		case csLatin:
			hi := g | 0x80
			if set.latin == nil {
				out.WriteRune(rune(hi))
			} else {
				out.WriteRune(set.latin.DecodeByte(hi))
			}
		case csTIS620:
			hi := rune(g | 0x80)
			switch {
			case hi == 0xa0:
				out.WriteRune(0xa0)
			case hi >= 0xa1 && hi <= 0xda, hi >= 0xdf && hi <= 0xfb:
				out.WriteRune(hi - 0xa0 + 0x0e00)
			default:
				out.WriteRune(utf8.RuneError)
			}
		default:
			out.WriteRune(utf8.RuneError)
		}
	}
}

// ctWide decodes the bytes of a 94x94 set through its EUC form (both bytes
// with the high bit set, plus 0x8f before JIS X 0212).
func ctWide(out *strings.Builder, cs int, run []byte) {
	var dec encoding.Encoding
	switch cs {
	case csJIS0208, csJIS0212:
		dec = japanese.EUCJP
	case csKSC5601:
		dec = korean.EUCKR
	case csGB2312:
		dec = simplifiedchinese.GBK
	}
	for i := 0; i < len(run); i += 2 {
		if i+1 >= len(run) || dec == nil {
			out.WriteRune(utf8.RuneError)
			continue
		}
		b1, b2 := run[i]|0x80, run[i+1]|0x80
		if b1 < 0xa1 || b1 > 0xfe || b2 < 0xa1 || b2 > 0xfe {
			out.WriteRune(utf8.RuneError)
			continue
		}
		euc := []byte{b1, b2}
		if cs == csJIS0212 {
			euc = []byte{0x8f, b1, b2}
		}
		s, err := dec.NewDecoder().Bytes(euc)
		if err != nil || len(s) == 0 {
			out.WriteRune(utf8.RuneError)
			continue
		}
		out.WriteString(strings.ToValidUTF8(string(s), "�"))
	}
}

// ctExtended decodes the text of an extended segment by its encoding name.
func ctExtended(out *strings.Builder, name string, text []byte) {
	var dec encoding.Encoding
	switch name {
	case "iso8859-14":
		dec = charmap.ISO8859_14
	case "iso8859-15":
		dec = charmap.ISO8859_15
	case "gbk-0":
		dec = simplifiedchinese.GBK
	case "utf-8", "iso10646-1":
		out.WriteString(strings.ToValidUTF8(string(text), "�"))
		return
	}
	if dec == nil {
		if len(text) > 0 {
			out.WriteRune(utf8.RuneError)
		}
		return
	}
	s, err := dec.NewDecoder().Bytes(text)
	if err != nil {
		out.WriteRune(utf8.RuneError)
		return
	}
	out.WriteString(strings.ToValidUTF8(string(s), "�"))
}
