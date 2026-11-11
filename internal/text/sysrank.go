package text

import (
	"strings"

	"github.com/go-text/typesetting/language"
)

// Ranking of the installed families that cover a cluster. fontconfig's
// answer depends on a large configuration, and Chromium's on per-script and
// per-locale lists of each OS's own fonts; this is the small, predictable
// part of both that matters for fallback over whatever is installed: a
// design that sits with a UI sans-serif, a font near the requested weight,
// the region of CJK families, and specialization.

// genericNames are the CSS generic family keywords and the names this
// package maps itself. They never name an installed font.
var genericNames = map[string]bool{
	"serif": true, "sans-serif": true, "monospace": true, "cursive": true,
	"fantasy": true, "system-ui": true, "ui-serif": true, "ui-sans-serif": true,
	"ui-monospace": true, "ui-rounded": true, "emoji": true, "math": true,
	"fangsong": true,
}

// isGenericName reports whether the normalized family name is a generic
// family keyword.
func isGenericName(key string) bool { return genericNames[key] }

// defaultRegions is the order of the regional variants of CJK families when
// the language is unknown.
var defaultRegions = []string{"jp", "sc", "kr", "tc", "hk"}

// regionPreference returns the regions of CJK families (jp for Noto Sans
// CJK JP, Yu Gothic...) in order of preference for lang, or for the user's
// language (SystemLanguage) when lang is empty.
func regionPreference(lang string) []string {
	if lang == "" {
		lang = SystemLanguage()
	}
	first := cjkRegion(lang)
	if first == "" {
		return defaultRegions
	}
	return regionsFrom(first)
}

// regionOrders are the orders of the regions when the first one is
// known: the closest conventions next (Hong Kong and Taiwan share
// Traditional characters, and Simplified Chinese follows them), so that
// zh-HK text without a Hong Kong font takes a Traditional Chinese one.
var regionOrders = map[string][]string{
	"jp": defaultRegions,
	"sc": {"sc", "tc", "hk", "jp", "kr"},
	"tc": {"tc", "hk", "sc", "jp", "kr"},
	"hk": {"hk", "tc", "sc", "jp", "kr"},
	"kr": {"kr", "jp", "sc", "tc", "hk"},
}

// regionsFrom returns the regions in order of preference when first is
// the language's (regionOrders).
func regionsFrom(first string) []string {
	if o, ok := regionOrders[first]; ok {
		return o
	}
	return defaultRegions
}

// cjkRegion returns the region of the CJK families of a language (a BCP
// 47 tag or a POSIX locale name): jp for Japanese, kr for Korean, sc for
// Chinese, tc for Traditional Chinese (Hant, Taiwan, Macau), hk for Hong
// Kong and Cantonese; "" for any other language.
func cjkRegion(lang string) string {
	parts := strings.FieldsFunc(strings.ToLower(lang), func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == '@' })
	if len(parts) == 0 {
		return ""
	}
	switch parts[0] {
	case "ja":
		return "jp"
	case "ko":
		return "kr"
	case "yue":
		return "hk"
	case "zh":
		// The script decides (zh-Hans-HK is Simplified), then the region.
		var script, region string
		for _, p := range parts[1:] {
			switch p {
			case "hans", "hant":
				script = p
			case "tw", "mo", "hk":
				region = p
			}
		}
		switch {
		case script == "hans":
			return "sc"
		case region == "hk":
			return "hk"
		case script == "hant" || region != "":
			return "tc"
		}
		return "sc"
	}
	return ""
}

// regionalBases are the normalized names of the CJK families published in
// regional variants (Noto Sans JP, Source Han Serif KR, PingFang SC...),
// besides the ones whose base name ends in "cjk" (Noto Sans Mono CJK SC).
var regionalBases = map[string]bool{
	"notosans": true, "notoserif": true,
	"sourcehansans": true, "sourcehanserif": true, "sourcehansansmono": true,
	"sarasagothic": true, "sarasaui": true, "sarasamono": true,
	// macOS
	"pingfang": true, "heiti": true, "songti": true, "kaiti": true, "yuanti": true,
	"lantinghei": true, "hannotate": true, "hanzipen": true, "libian": true,
	"weibei": true, "wawati": true,
}

// regionRank is the position in prefs of the region of a CJK family
// (regionOf), or len(prefs) for families of no region.
func regionRank(family string, prefs []string) int {
	if r := regionOf(family); r != "" {
		for i, p := range prefs {
			if p == r {
				return i
			}
		}
	}
	return len(prefs)
}

// weightWords are the style words that end the family names of static
// fonts installed one weight per family ("Noto Sans JP Thin", "Yu Gothic
// UI Semibold", "Microsoft YaHei UI"), longest first.
var weightWords = []string{"extralight", "ultralight", "demilight", "semilight", "extrabold", "ultrabold", "semibold", "demibold",
	"regular", "medium", "normal", "light", "heavy", "black", "thin", "bold", "book", "ui"}

// regionalFamilies are installed CJK families whose names do not end in a
// region (Microsoft YaHei, Yu Gothic, Malgun Gothic), by normalized name:
// the families of Chromium's per-locale fallback lists on Windows, macOS
// and Linux, and their usual neighbors.
var regionalFamilies = map[string]string{
	// Japanese
	"yugothic": "jp", "yumincho": "jp", "meiryo": "jp", "msgothic": "jp", "mspgothic": "jp",
	"msuigothic": "jp", "msmincho": "jp", "mspmincho": "jp", "bizudgothic": "jp", "bizudpgothic": "jp",
	"bizudmincho": "jp", "bizudpmincho": "jp", "hiraginosans": "jp", "hiraginokakugothicpro": "jp",
	"hiraginokakugothicpron": "jp", "hiraginokakugothicstd": "jp", "hiraginokakugothicstdn": "jp",
	"hiraginominchopro": "jp", "hiraginominchopron": "jp", "hiraginomarugothicpro": "jp",
	"hiraginomarugothicpron": "jp", "osaka": "jp", "ipagothic": "jp", "ipapgothic": "jp",
	"ipaexgothic": "jp", "ipamincho": "jp", "ipapmincho": "jp", "ipaexmincho": "jp", "takaogothic": "jp",
	"takaopgothic": "jp", "takaomincho": "jp", "vlgothic": "jp", "vlpgothic": "jp", "umegothic": "jp",
	// Simplified Chinese
	"microsoftyahei": "sc", "dengxian": "sc", "simsun": "sc", "nsimsun": "sc", "simhei": "sc",
	"kaiti": "sc", "fangsong": "sc", "wenquanyimicrohei": "sc", "wenquanyimicroheimono": "sc",
	"wenquanyizenhei": "sc", "wenquanyizenheisharp": "sc", "arplumingcn": "sc", "arplukaicn": "sc",
	"stheiti": "sc", "stxihei": "sc", "stsong": "sc", "stkaiti": "sc", "stfangsong": "sc",
	"hiraginosansgb": "sc", "droidsansfallback": "sc",
	// Traditional Chinese
	"microsoftjhenghei": "tc", "mingliu": "tc", "pmingliu": "tc", "dfkaisb": "tc", "arplumingtw": "tc",
	"arplukaitw": "tc", "arplumingtwmbe": "tc", "arplukaitwmbe": "tc", "lihei": "tc", "lisong": "tc",
	"applelihei": "tc", "applelisung": "tc", "hiraginosanscns": "tc",
	// Hong Kong
	"mingliuhkscs": "hk", "mingliuhkscsextb": "hk", "arpluminghk": "hk",
	// Korean
	"malgungothic": "kr", "gulim": "kr", "gulimche": "kr", "dotum": "kr", "dotumche": "kr",
	"batang": "kr", "batangche": "kr", "gungsuh": "kr", "gungsuhche": "kr", "applesdgothicneo": "kr",
	"applemyungjo": "kr", "applegothic": "kr", "nanumgothic": "kr", "nanumbarungothic": "kr",
	"nanummyeongjo": "kr", "nanumsquare": "kr", "undotum": "kr", "unbatang": "kr",
	"baekmukgulim": "kr", "baekmukdotum": "kr", "baekmukbatang": "kr",
}

// regionOf returns the region ("jp", "sc", "tc", "hk", "kr") of the CJK
// family with the normalized name family, or "": a regional variant of a
// pan-CJK design (Noto Sans CJK JP, Source Han Serif KR, Noto Sans SC,
// PingFang TC), a family of regionalFamilies, or either followed by weight
// words ("Noto Sans JP Thin", "Yu Gothic UI Light"), as a font installed
// one weight per family is named. A family whose name just ends in the
// same letters (Noto Sans Misc) has no region.
func regionOf(family string) string {
	for name := family; name != ""; {
		for _, r := range defaultRegions {
			base, ok := strings.CutSuffix(name, r)
			if ok && (strings.HasSuffix(base, "cjk") || regionalBases[base]) {
				return r
			}
		}
		if r := regionalFamilies[name]; r != "" {
			return r
		}
		cut := false
		for _, w := range weightWords {
			if rest, ok := strings.CutSuffix(name, w); ok {
				name, cut = rest, true
				break
			}
		}
		if !cut {
			break
		}
	}
	return ""
}

// regionSupports reports whether a CJK family of region r is made for
// script, so that it supports the script however little of it its fonts
// cover: a Japanese font has far fewer ideographs than a Chinese one, but
// it is the font of Japanese text.
func regionSupports(r string, script language.Script) bool {
	switch script {
	case language.Han:
		return r != ""
	case language.Hiragana, language.Katakana, language.Katakana_Or_Hiragana:
		return r == "jp"
	case language.Hangul:
		return r == "kr"
	case language.Bopomofo:
		return r == "tc" || r == "hk"
	}
	return false
}

// isCJKScript reports the scripts whose fonts come in regional variants.
func isCJKScript(script language.Script) bool {
	switch script {
	case language.Han, language.Hiragana, language.Katakana, language.Katakana_Or_Hiragana, language.Hangul, language.Bopomofo:
		return true
	}
	return false
}

// weightRank ranks the distance between the requested weight and the
// closest weight of a family's fonts: 0 below 200 (a Regular for a Light
// or a Medium request), then one more per 200. So a family installed in
// one thin weight loses to one with a Regular for body text, and a family
// with a Bold wins for bold text, before region and coverage count.
// Variable fonts count at their default weight, as the index has it.
func weightRank(closest, want float32) int {
	d := closest - want
	if d < 0 {
		d = -d
	}
	return int(d / 200)
}

// scriptGroup returns the scripts that one family draws in a paragraph
// (paraScope): Han, kana and Bopomofo are one group, since Japanese and
// Chinese text mixes them; every other script is its own group; clusters
// of no script of their own have none (0).
func scriptGroup(script language.Script) language.Script {
	switch script {
	case language.Common, language.Inherited, language.Unknown:
		return 0
	case language.Hiragana, language.Katakana, language.Katakana_Or_Hiragana, language.Bopomofo:
		return language.Han
	}
	return script
}

var (
	tierSkip  = []string{"mono", "code", "symbol", "math", "emoji", "unifont", "lastresort", "braille", "dingbat", "wingding", "webding", "icon", "awesome"}
	tierSerif = []string{"serif", "mincho", "myungjo", "batang", "songti", "simsun", "ming"}
	tierSans  = []string{"sans", "gothic", "hei", "kaku", "segoe", "arial", "helvetica", "roboto", "tahoma", "verdana", "calibri", "pingfang", "malgun", "meiryo"}
)

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// familyTier ranks a normalized family name by how well it sits next to a
// UI sans-serif: 0 sans-serif designs, 1 unknown, 2 serif designs, 3
// symbol, monospace and pan-Unicode fonts.
func familyTier(family string) int {
	switch {
	case containsAny(family, tierSkip):
		return 3
	case strings.Contains(family, "sans"):
		return 0
	case containsAny(family, tierSerif):
		return 2
	case containsAny(family, tierSans):
		return 0
	}
	return 1
}
