package xim

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestDecodeCompoundText(t *testing.T) {
	gr := func(s string) string { // a GL string in its GR form
		b := []byte(s)
		for i := range b {
			b[i] |= 0x80
		}
		return string(b)
	}
	for _, c := range []struct{ name, in, want string }{
		{"ascii", "hello, world", "hello, world"},
		{"latin-1 in the default GR", "caf\xe9 \xfcber", "café über"},
		{"utf-8 segment", "a\x1b%Gこんにちは\x1b%@b", "aこんにちはb"},
		{"utf-8 segment to the end", "\x1b%G日本", "日本"},
		// こんにちは in JIS X 0208: 2433 2473 244B 2441 244F.
		{"jis x 0208 in GL", "\x1b$(B$3$s$K$A$O\x1b(B!", "こんにちは!"},
		{"jis x 0208 in GR", "x\x1b$)B" + gr("$3$s"), "xこん"},
		{"jis x 0208 kanji", "\x1b$)B" + gr("F|K\\8l"), "日本語"},
		{"jis x 0212", "\x1b$(D\"/", "˘"},
		{"jis x 0201 kana in GR", "\x1b)I\xb1\xb2", "ｱｲ"},
		{"jis x 0201 roman", "\x1b(J\\~a", "¥‾a"},
		{"ks c 5601", "\x1b$)C\xc7\xd1\xb1\xdb", "한글"},
		{"gb 2312", "\x1b$)A\xd6\xd0\xce\xc4", "中文"},
		{"iso 8859-2", "\x1b-B\xb1", "ą"},
		{"iso 8859-5", "\x1b-L\xb0", "А"},
		{"iso 8859-11", "\x1b-T\xa1", "ก"},
		{"back to latin-1", "\x1b-B\xb1\x1b-A\xb1", "ą±"},
		{"space in a 94x94 GL", "\x1b$(B$3 $s", "こ ん"},
		{"odd byte of a 94x94 set", "\x1b$)B\xa4", "�"},
		{"invalid pair inside a 94x94 run", "\x1b$)B\xa4\xb3\xa0\xa0\xa4\xf3", "こ�ん"},
		{"odd byte after a 94x94 run", "\x1b$)B\xa4\xb3\xa4", "こ�"},
		{"unknown 94 set", "\x1b(Zab", "��"},
		{"cns 11643", "\x1b$(Gab", "�"},
		{"tab, newline, other controls", "a\tb\nc\x01d\x7f\x85", "a\tb\ncd"},
		{"direction CSI", "\x9b1]ab\x9b]", "ab"},
		{"extended segment", "\x1b%/1\x80\x8ciso8859-15\x02\xa4", "€"},
		{"extended utf-8 name", "\x1b%/2\x80\x87utf-8\x02\xc3", "�"},
		{"unknown extended segment", "\x1b%/2\x80\x89big5-0\x02\xa4\x40x", "�x"},
		{"truncated escape", "a\x1b$", "a"},
		{"invalid utf-8 in a segment", "\x1b%G\xff\x1b%@", "�"},
	} {
		if got := DecodeCompoundText([]byte(c.in)); got != c.want {
			t.Errorf("%s: DecodeCompoundText(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestCtWideRun checks that decoding a 94x94 run at once gives what
// decoding each character alone gives, over every byte pair of each set.
func TestCtWideRun(t *testing.T) {
	alone := func(cs int, b1, b2 byte) string {
		var enc encoding.Encoding
		switch cs {
		case csJIS0208, csJIS0212:
			enc = japanese.EUCJP
		case csKSC5601:
			enc = korean.EUCKR
		case csGB2312:
			enc = simplifiedchinese.GBK
		}
		b1, b2 = b1|0x80, b2|0x80
		if b1 < 0xa1 || b1 > 0xfe || b2 < 0xa1 || b2 > 0xfe {
			return string(utf8.RuneError)
		}
		euc := []byte{b1, b2}
		if cs == csJIS0212 {
			euc = []byte{0x8f, b1, b2}
		}
		s, err := enc.NewDecoder().Bytes(euc)
		if err != nil || len(s) == 0 {
			return string(utf8.RuneError)
		}
		return strings.ToValidUTF8(string(s), "\uFFFD")
	}
	for _, cs := range []int{csJIS0208, csJIS0212, csKSC5601, csGB2312} {
		var run []byte
		var want strings.Builder
		for b1 := 0x20; b1 <= 0x7f; b1++ {
			for b2 := 0x20; b2 <= 0x7f; b2++ {
				run = append(run, byte(b1), byte(b2))
				want.WriteString(alone(cs, byte(b1), byte(b2)))
			}
		}
		var got strings.Builder
		ctWide(&got, cs, run)
		if got.String() != want.String() {
			g, w := []rune(got.String()), []rune(want.String())
			for i := range min(len(g), len(w)) {
				if g[i] != w[i] {
					t.Errorf("set %d: rune %d = %q, decoded alone %q (lengths %d, %d)", cs, i, g[i], w[i], len(g), len(w))
					break
				}
			}
			if len(g) != len(w) {
				t.Errorf("set %d: %d runes, %d decoded alone", cs, len(g), len(w))
			}
		}
	}
}
