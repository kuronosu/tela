//go:build linux

package freedesktop

import (
	"strings"
	"unicode"
)

// The desktop's font settings name a font with its size and style; Tela
// needs the family only (SystemSettings.UIFontFamily, MonoFontFamily).

// maxFontFamily bounds the family name taken from a setting: a real one
// is a few words long.
const maxFontFamily = 256

// pangoFamily returns the first family of a Pango font description, the
// syntax of GNOME's font-name and monospace-font-name ("Adwaita Sans 11",
// "Cantarell Bold Italic 10.5", "DejaVu Sans, Noto Sans, 12"), or "" when
// it names none.
//
// pango_font_description_from_string reads the string from its end: font
// features (a last word starting with '#'), variations ('@'), a size (a
// decimal number of points, or "px" pixels), then style words (style,
// variant, weight, stretch and gravity names such as Italic, Small-Caps,
// Semi-Bold or 600, Condensed, Rotated-Left; case-insensitive, hyphens
// optional), as long as they are recognized; what remains is a
// comma-separated family list. Pango has no quoting, but a family is
// sometimes written in quotes ('"Fira Sans" 10', or a value copied from
// gsettings with its single quotes); they are removed.
func pangoFamily(desc string) string {
	s := strings.TrimSpace(desc)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		s = strings.TrimSpace(s[1 : len(s)-1]) // gsettings' GVariant quoting
	}
	// The trailing words: features, variations, size, style words.
	end := len(s)
	word := func(stop string) (start int, w string) {
		e := end
		for e > 0 && asciiSpace(s[e-1]) {
			e--
		}
		b := e
		for b > 0 && !asciiSpace(s[b-1]) && !strings.ContainsRune(stop, rune(s[b-1])) {
			b--
		}
		return b, s[b:e]
	}
	if b, w := word(""); w != "" && w[0] == '#' {
		end = b
	}
	if b, w := word(""); w != "" && w[0] == '@' {
		end = b
	}
	if b, w := word(""); w != "" && pangoSize(w) {
		end = b
	}
	for {
		b, w := word(",")
		if w == "" || !pangoStyleWord(w) {
			break
		}
		end = b
	}
	// The family list: its first entry.
	first, _, _ := strings.Cut(s[:end], ",")
	return cleanFamily(first)
}

// asciiSpace reports whether c is ASCII white space (g_ascii_isspace,
// which Pango splits words with; bytes of UTF-8 sequences never are).
func asciiSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// pangoSize reports whether w is the size of a font description: a
// positive decimal number of points, or of pixels with "px".
func pangoSize(w string) bool {
	w = strings.TrimSuffix(w, "px")
	digits, dot := 0, false
	for _, c := range w {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	return digits > 0
}

// pangoStyleWords are Pango's style, variant, weight, stretch and gravity
// names (pango/fonts.c), lowercase and without hyphens, which Pango
// ignores when it compares.
var pangoStyleWords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range []string{
		// Style.
		"Normal", "Roman", "Oblique", "Italic",
		// Variant.
		"Small-Caps", "All-Small-Caps", "Petite-Caps", "All-Petite-Caps", "Unicase", "Title-Caps",
		// Weight.
		"Thin", "Ultra-Light", "Extra-Light", "Light", "Semi-Light", "Demi-Light", "Book",
		"Regular", "Medium", "Semi-Bold", "Demi-Bold", "Bold", "Ultra-Bold", "Extra-Bold",
		"Heavy", "Black", "Ultra-Black", "Extra-Black", "Ultra-Heavy", "Extra-Heavy",
		// Stretch.
		"Ultra-Condensed", "Extra-Condensed", "Condensed", "Semi-Condensed",
		"Semi-Expanded", "Expanded", "Extra-Expanded", "Ultra-Expanded",
		// Gravity.
		"Not-Rotated", "South", "Upside-Down", "North", "Rotated-Left", "East", "Rotated-Right", "West",
	} {
		m[strings.ToLower(strings.ReplaceAll(w, "-", ""))] = true
	}
	return m
}()

// pangoStyleWord reports whether w is a style word of a font
// description: a name of pangoStyleWords or a numeric weight (1 to 1000).
func pangoStyleWord(w string) bool {
	if pangoStyleWords[strings.ToLower(strings.ReplaceAll(w, "-", ""))] {
		return true
	}
	n := 0
	for _, c := range w {
		if c < '0' || c > '9' || n > 1000 {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n >= 1 && n <= 1000
}

// qtFamily returns the family of a font as Qt writes it (QFont::toString,
// what KDE keeps in kdeglobals' font and fixed): the first of its
// comma-separated fields, "Noto Sans,10,-1,5,50,0,0,0,0,0" (Qt 5) or
// "Noto Sans,10,-1,5,400,0,0,0,0,0,0,0,0,0,0,1" (Qt 6). KDE's portal sends
// it as a string; a value split into a list of strings (KConfig's list
// syntax) gives its first entry.
func qtFamily(v any) string {
	switch s := v.(type) {
	case string:
		f, _, _ := strings.Cut(s, ",")
		return cleanFamily(f)
	case []string:
		if len(s) > 0 {
			return cleanFamily(s[0])
		}
	case []any:
		if len(s) > 0 {
			if f, ok := unwrapVariant(s[0]).(string); ok {
				return cleanFamily(f)
			}
		}
	}
	return ""
}

// cleanFamily trims a family name and the quotes around it; a name that
// is too long or holds control characters gives "".
func cleanFamily(f string) string {
	f = strings.TrimSpace(f)
	if len(f) >= 2 && (f[0] == '"' && f[len(f)-1] == '"' || f[0] == '\'' && f[len(f)-1] == '\'') {
		f = strings.TrimSpace(f[1 : len(f)-1])
	}
	if len(f) > maxFontFamily || strings.IndexFunc(f, unicode.IsControl) >= 0 {
		return ""
	}
	return f
}
