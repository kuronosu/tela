package xim

import "testing"

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
