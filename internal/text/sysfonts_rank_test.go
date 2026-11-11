package text

import (
	"testing"

	"github.com/go-text/typesetting/language"
)

// defReq is the fallback request of a regular-weight query in the
// collection's default language.
func defReq(s *systemFonts) fbRequest { return fbRequest{weight: 400, prefs: s.prefs} }

// win11CJK reproduces the CJK fonts of a Windows 11 machine where the user
// installed a static Thin weight of Noto Sans JP (its family name is "Noto
// Sans JP Thin"): the narrowest family that has kana and the common kanji,
// so it drew the kana and Japanese kanji of a paragraph at weight 100 while
// Microsoft YaHei drew its Simplified Chinese characters (the owner's VM;
// TestSystemFontsLive failed there).
func win11CJK() []sysEntry {
	kana := runeRange(0x3041, 0x3096) + runeRange(0x30A1, 0x30FA) + "ー、。「」"
	latin := runeRange(0x20, 0x7E)
	// Common ideographs every CJK family has, so that each covers part of
	// the sample scriptFamilies probes.
	jis := runeRange(0x7000, 0x7AFF) + "日本語文章漢字単語間空白入行折返種類禁則処理考必要句読点閉括弧頭来基本的決世界"
	simplified := "中文简体们说这是一个句子"
	hangul := runeRange(0xAC00, 0xAC40)
	var es []sysEntry
	add := func(name string, runes string, weights ...float32) {
		for _, w := range weights {
			es = append(es, entry(name, StyleNormal, w, runes))
		}
	}
	add("notosansjpthin", latin+kana+jis, 100)
	add("yugothic", latin+kana+jis+"体是一个子", 300, 400, 500, 700)
	add("msgothic", latin+kana+jis, 400)
	add("microsoftyahei", latin+kana+jis+simplified+"体国", 300, 400, 700)
	add("simsun", latin+kana+jis+simplified+"体国", 400)
	add("microsoftjhenghei", latin+jis+"體國", 300, 400, 700)
	add("malgungothic", latin+hangul+"日本", 300, 400, 700)
	add("segoeui", latin, 300, 400, 600, 700)
	return es
}

// testCandidates returns the candidates of a query with no registered
// font, over the system fonts s.
func testCandidates(s *systemFonts, q Query) *candidates {
	c := NewCollection()
	return &candidates{c: c, q: q, sys: s, seen: map[string]bool{}, scripts: map[language.Script][]*famSlot{}}
}

func TestSystemFallbackWeight(t *testing.T) {
	s := synthetic("", win11CJK()...)
	for _, c := range []struct {
		cluster string
		weight  float32
		want    string
	}{
		// A family installed only in Thin loses to one with a Regular, for
		// kana and kanji alike.
		{"の", 400, "msgothic"},
		{"日", 400, "msgothic"},
		// For bold text a family with a Bold wins.
		{"の", 700, "yugothic"},
		{"日", 700, "yugothic"},
		// Thin text takes the Thin family.
		{"の", 100, "notosansjpthin"},
		// Simplified-only characters: a Chinese family with the weight.
		{"们", 400, "microsoftyahei"},
	} {
		got := s.fallbackFamilies([]rune(c.cluster), language.LookupScript([]rune(c.cluster)[0]), fbRequest{weight: c.weight, prefs: s.prefs})
		if len(got) == 0 || got[0] != c.want {
			t.Errorf("%q at %g: %v, want %s first", c.cluster, c.weight, got, c.want)
		}
	}
}

func TestSystemParagraphPick(t *testing.T) {
	s := synthetic("", win11CJK()...)
	pick := func(text string, weight float32) string {
		rs := []rune(text)
		scripts := make([]language.Script, len(rs))
		resolveScripts(rs, scripts)
		cs := testCandidates(s, Query{Weight: weight})
		scope := &paraScope{runes: rs, scripts: scripts, cands: func(int) *candidates { return cs }}
		return scope.pick(cs, language.Han, cs.sysRequest())
	}
	for _, c := range []struct {
		text   string
		weight float32
		want   string
	}{
		// The paragraph of TestSystemFontsLive: one family has every
		// character, kana included, so the whole CJK text is Microsoft
		// YaHei (not Noto Sans JP Thin for the Japanese and YaHei for the
		// Chinese).
		{"日本語の文章、こんにちは世界。 中文简体字们说这是一个句子。 Mixed with Latin text.", 400, "microsoftyahei"},
		// Japanese alone: the first Japanese family with a Regular.
		{"日本語の文章、こんにちは世界。", 400, "msgothic"},
		{"日本語の文章、こんにちは世界。", 700, "yugothic"},
		// Chinese characters that MS Gothic lacks and Yu Gothic has: the
		// first Japanese family that covers them all.
		{"日本語の文章、こんにちは世界。体是一个子", 400, "yugothic"},
		// Latin only: nothing to pick for the CJK group.
		{"Latin text", 400, ""},
	} {
		if got := pick(c.text, c.weight); got != c.want {
			t.Errorf("%q at %g: picked %q, want %q", c.text, c.weight, got, c.want)
		}
	}
}

func TestSystemRegionOf(t *testing.T) {
	for _, c := range []struct{ family, want string }{
		{"Noto Sans CJK JP", "jp"},
		{"Noto Sans JP Thin", "jp"},
		{"Noto Sans SC", "sc"},
		{"Source Han Serif KR Heavy", "kr"},
		{"PingFang HK", "hk"},
		{"Microsoft YaHei UI", "sc"},
		{"Microsoft JhengHei UI Light", "tc"},
		{"Yu Gothic UI Semibold", "jp"},
		{"Malgun Gothic", "kr"},
		{"MS PGothic", "jp"},
		{"Hiragino Sans GB", "sc"},
		{"Noto Sans Misc", ""},
		{"Noto Sans", ""},
		{"Segoe UI", ""},
		{"Fira Code Disc", ""},
	} {
		if got := regionOf(normalizeFamily(c.family)); got != c.want {
			t.Errorf("%s: region %q, want %q", c.family, got, c.want)
		}
	}
}

func TestSystemScriptRegion(t *testing.T) {
	// Kana are Japanese and Hangul Korean whatever the language; Han
	// follows the language.
	all := runeRange(0x3041, 0x3096) + runeRange(0xAC00, 0xAC40) + runeRange(0x4E00, 0x4FFF)
	for _, lang := range []string{"", "zh-Hans", "ja", "ko"} {
		s := synthetic(lang,
			entry("notosanscjkjp", StyleNormal, 400, all),
			entry("notosanscjkkr", StyleNormal, 400, all),
			entry("notosanscjksc", StyleNormal, 400, all),
		)
		for cluster, want := range map[string]string{"가": "notosanscjkkr", "の": "notosanscjkjp"} {
			if got := s.fallbackFamilies([]rune(cluster), language.LookupScript([]rune(cluster)[0]), defReq(s)); got[0] != want {
				t.Errorf("lang %q: %q in %v, want %s first", lang, cluster, got, want)
			}
		}
	}
}

func TestSystemRegionSupportsHan(t *testing.T) {
	// A Japanese family supports Han although it covers far less of it
	// than a Chinese family, so that it ranks among Han's families.
	han := runeRange(0x4E00, 0x9FFF)
	s := synthetic("ja",
		entry("microsoftyahei", StyleNormal, 400, han),
		entry("yugothic", StyleNormal, 400, han[:len([]rune(han))/8]),
		entry("somebigfont", StyleNormal, 400, han[:len([]rune(han))/8]),
	)
	got := s.scriptFamilies(language.Han, defReq(s))
	if !equalStrings(got, []string{"yugothic", "microsoftyahei"}) {
		t.Errorf("Han families in Japanese: %v", got)
	}
}
