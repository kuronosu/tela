package text

import (
	"os"
	"strings"
)

// Ranking of the installed families that cover a cluster. fontconfig's
// answer depends on a large configuration; this is the small, predictable
// part of it that matters for fallback: the region of CJK families, a
// design that sits with a UI sans-serif, and specialization.

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

// regionPreference returns the family-name suffixes (Noto Sans CJK JP, ...)
// in order of preference for lang, or for the environment's language when
// lang is empty.
func regionPreference(lang string) []string {
	if lang == "" {
		for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
			if v := os.Getenv(name); v != "" && v != "C" && v != "POSIX" {
				lang = v
				break
			}
		}
	}
	parts := strings.FieldsFunc(strings.ToLower(lang), func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == '@' })
	first := ""
	if len(parts) > 0 {
		switch parts[0] {
		case "ja":
			first = "jp"
		case "ko":
			first = "kr"
		case "zh":
			first = "sc"
			for _, p := range parts[1:] {
				switch p {
				case "hant", "tw", "mo":
					first = "tc"
				case "hk":
					first = "hk"
				}
			}
		}
	}
	if first == "" {
		return defaultRegions
	}
	out := []string{first}
	for _, r := range defaultRegions {
		if r != first {
			out = append(out, r)
		}
	}
	return out
}

// regionRank is the position in prefs of the region suffix of the
// normalized family name, or len(prefs) for families without one.
func regionRank(family string, prefs []string) int {
	for i, suffix := range prefs {
		if strings.HasSuffix(family, suffix) {
			return i
		}
	}
	return len(prefs)
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
