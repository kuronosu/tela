package i18n

import (
	"strings"
	"testing"
	"time"
)

func TestICUBasics(t *testing.T) {
	tests := []struct {
		loc, src string
		args     []any
		want     string
	}{
		{"en", "Hello, {name}!", []any{"name", "Ana"}, "Hello, Ana!"},
		{"en", "{ name }", []any{"name", "Ana"}, "Ana"},
		{"en", "no arguments", nil, "no arguments"},
		{"en", "{n} items", []any{"n", 1234}, "1,234 items"},
		{"es", "{n} artículos", []any{"n", 1234.5}, "1.234,5 artículos"},
		{"en", "# is literal outside a plural", nil, "# is literal outside a plural"},
		{"en", "{a}{b}", []any{"a", 1, "b", "x"}, "1x"},
		{"en", "{0}", []any{"0", "zero"}, "zero"},
		{"en", "{n, number}", []any{"n", 1234567}, "1,234,567"},
		{"en", "{n, number, integer}", []any{"n", 2.5}, "2"},
		{"en", "{n, number, percent}", []any{"n", 0.256}, "26%"},
		{"es", "{n, number}", []any{"n", 0.5}, "0,5"},
		{"en", "{d, date, long}", []any{"d", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)}, "January 5, 2026"},
		{"es", "{d, date, short}", []any{"d", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)}, "5/1/26"},
		{"en", "{d, date}", []any{"d", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)}, "Jan 5, 2026"},
		{"de", "{d, time, short}", []any{"d", time.Date(2026, 1, 5, 15, 4, 0, 0, time.UTC)}, "15:04"},
	}
	for _, tc := range tests {
		if got := msg(t, tc.loc, tc.src, tc.args...); got != tc.want {
			t.Errorf("%s %q = %q, want %q", tc.loc, tc.src, got, tc.want)
		}
	}
}

func TestICUApostrophes(t *testing.T) {
	tests := []struct{ src, want string }{
		{"don't", "don't"},
		{"It''s", "It's"},
		{"'{'name'}'", "{name}"},
		{"'{name}'", "{name}"},
		{"a '{' b", "a { b"},
		{"''{name}''", "'Ana'"},
		{"'}", "}"},
		{"rock 'n' roll", "rock 'n' roll"},
		{"'{''}'", "{'}"},
		{"unterminated '{quote", "unterminated {quote"},
		{"{name}'s", "Ana's"},
		{"'#' outside", "'#' outside"},
	}
	for _, tc := range tests {
		if got := msg(t, "en", tc.src, "name", "Ana"); got != tc.want {
			t.Errorf("%q = %q, want %q", tc.src, got, tc.want)
		}
	}
	// In a plural, ' before # quotes it.
	if got := msg(t, "en", "{n, plural, other {'#' is # and don't}}", "n", 3); got != "# is 3 and don't" {
		t.Errorf("plural quote = %q", got)
	}
}

func TestICUPlural(t *testing.T) {
	files := "{count, plural, =0 {No files} one {# file} other {# files}}"
	for _, tc := range []struct {
		n    any
		want string
	}{{0, "No files"}, {1, "1 file"}, {2, "2 files"}, {1000, "1,000 files"}, {1.5, "1.5 files"}, {uint8(1), "1 file"}} {
		if got := msg(t, "en", files, "count", tc.n); got != tc.want {
			t.Errorf("en %v = %q, want %q", tc.n, got, tc.want)
		}
	}
	// =N compares the number itself; keywords come after the offset.
	off := "{n, plural, offset:1 =0 {Nobody} =1 {Just {host}} one {{host} and # other} other {{host} and # others}}"
	for _, tc := range []struct {
		n    int
		want string
	}{{0, "Nobody"}, {1, "Just Ana"}, {2, "Ana and 1 other"}, {5, "Ana and 4 others"}} {
		if got := msg(t, "en", off, "n", tc.n, "host", "Ana"); got != tc.want {
			t.Errorf("offset %d = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestICULayout(t *testing.T) {
	// No spaces around selectors and braces, and newlines inside.
	for _, tc := range []struct {
		n    int
		want string
	}{{0, "none"}, {1, "1 item"}, {4, "4 items"}} {
		if got := msg(t, "en", "{n,plural,=0{none} one{# item}other{# items}}", "n", tc.n); got != tc.want {
			t.Errorf("compact plural %d = %q, want %q", tc.n, got, tc.want)
		}
	}
	const multi = "{n, plural,\n  one {One\n  line}\n  other {Many}\n}"
	if got := msg(t, "en", multi, "n", 1); got != "One\n  line" {
		t.Errorf("multi-line = %q", got)
	}
	if got := msg(t, "en", "日本語 {name}さん", "name", "Ana"); got != "日本語 Anaさん" {
		t.Errorf("unicode text = %q", got)
	}
}

// Plural categories per language (CLDR cardinal rules).
func TestICUPluralRules(t *testing.T) {
	const all = "{n, plural, zero {zero} one {one} two {two} few {few} many {many} other {other}}"
	tests := []struct {
		loc  string
		n    any
		want string
	}{
		{"en", 1, "one"}, {"en", 0, "other"}, {"en", 2, "other"}, {"en", 1.5, "other"},
		{"es", 1, "one"}, {"es", 0, "other"}, {"es", 2, "other"},
		{"fr", 0, "one"}, {"fr", 1, "one"}, {"fr", 1.5, "one"}, {"fr", 2, "other"},
		{"ru", 1, "one"}, {"ru", 2, "few"}, {"ru", 4, "few"}, {"ru", 5, "many"}, {"ru", 11, "many"},
		{"ru", 21, "one"}, {"ru", 22, "few"}, {"ru", 25, "many"}, {"ru", 1.5, "other"}, {"ru", 0, "many"},
		{"ar", 0, "zero"}, {"ar", 1, "one"}, {"ar", 2, "two"}, {"ar", 3, "few"}, {"ar", 10, "few"},
		{"ar", 11, "many"}, {"ar", 99, "many"}, {"ar", 100, "other"}, {"ar", 102, "other"}, {"ar", 103, "few"},
		{"ja", 1, "other"}, {"ja", 0, "other"}, {"ja", 2, "other"},
		{"pl", 1, "one"}, {"pl", 2, "few"}, {"pl", 4, "few"}, {"pl", 5, "many"}, {"pl", 12, "many"},
		{"pl", 22, "few"}, {"pl", 1.5, "other"}, {"pl", 0, "many"},
		{"es-MX", 1, "one"}, {"pt-BR", 0, "one"}, {"pt-BR", 2, "other"},
	}
	for _, tc := range tests {
		if got := msg(t, tc.loc, all, "n", tc.n); got != tc.want {
			t.Errorf("%s %v = %q, want %q", tc.loc, tc.n, got, tc.want)
		}
	}
}

func TestICUSelectOrdinal(t *testing.T) {
	const src = "{n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}"
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 23: "23rd", 101: "101st", 111: "111th"} {
		if got := msg(t, "en", src, "n", n); got != want {
			t.Errorf("selectordinal %d = %q, want %q", n, got, want)
		}
	}
	// Spanish has a single ordinal category.
	if got := msg(t, "es", src, "n", 2); got != "2th" {
		t.Errorf("es ordinal = %q", got)
	}
}

func TestICUSelectAndNesting(t *testing.T) {
	const gender = "{g, select, female {She} male {He} other {They}} liked it"
	for g, want := range map[string]string{"female": "She liked it", "male": "He liked it", "x": "They liked it", "": "They liked it"} {
		if got := msg(t, "en", gender, "g", g); got != want {
			t.Errorf("select %q = %q, want %q", g, got, want)
		}
	}
	// A plural inside a select; # is the plural's.
	const nested = "{g, select, female {{n, plural, one {She has # file} other {She has # files}}} other {{n, plural, one {They have # file} other {They have # files}}}}"
	for _, tc := range []struct {
		g    string
		n    int
		want string
	}{{"female", 1, "She has 1 file"}, {"female", 3, "She has 3 files"}, {"x", 1, "They have 1 file"}, {"x", 12, "They have 12 files"}} {
		if got := msg(t, "en", nested, "g", tc.g, "n", tc.n); got != tc.want {
			t.Errorf("nested %v = %q, want %q", tc, got, tc.want)
		}
	}
	// # inside a select refers to the enclosing plural.
	const poundInSelect = "{n, plural, other {{g, select, a {A has #} other {B has #}}}}"
	if got := msg(t, "en", poundInSelect, "n", 7, "g", "a"); got != "A has 7" {
		t.Errorf("pound in select = %q", got)
	}
	// A plural inside a plural: # is the inner one.
	const twice = "{a, plural, other {{b, plural, other {#}} of #}}"
	if got := msg(t, "en", twice, "a", 5, "b", 2); got != "2 of 5" {
		t.Errorf("plural in plural = %q", got)
	}
	// A select value can be a bool.
	if got := msg(t, "en", "{ok, select, true {yes} other {no}}", "ok", true); got != "yes" {
		t.Errorf("bool select = %q", got)
	}
}

func TestICUPoundUsesLocaleDigits(t *testing.T) {
	if got := msg(t, "ar", "{n, plural, other {# ملفات}}", "n", 1234); got != "١٬٢٣٤ ملفات" {
		t.Errorf("ar # = %q", got)
	}
	if got := msg(t, "de", "{n, plural, one {# Datei} other {# Dateien}}", "n", 1234); got != "1.234 Dateien" {
		t.Errorf("de # = %q", got)
	}
}

func TestICUErrors(t *testing.T) {
	tests := []struct{ src, want string }{
		{"hello {name", `1:7: argument {name is never closed`},
		{"hello }", `1:7: unmatched '}'`},
		{"{}", `1:2: expected an argument name`},
		{"{n, plural, one {# file}}", `needs an "other" case`},
		{"{n, plural, one {# file} one {x} other {y}}", `duplicate case "one"`},
		{"{n, plural, uno {# file} other {y}}", `unknown plural category "uno"`},
		{"{n, bogus, x {y}}", `unknown argument type "bogus"`},
		{"{n, number, #,##0.00}", `unsupported number style "#,##0.00"`},
		{"{n, date, yyyy}", `unsupported date style "yyyy"`},
		{"{n, plural, one {# file other {y}}", `never closed`},
		{"{n, plural, =x {y} other {z}}", `'=' must be followed by a number`},
		{"line one\nline {two", `2:6: argument {two is never closed`},
		{"{n, select}", `expected`},
		{"{n, select, a b}", `expected '{' after the case selector "a"`},
	}
	for _, tc := range tests {
		_, err := parseMessage(tc.src)
		if err == nil {
			t.Errorf("%q parsed without an error; want %q", tc.src, tc.want)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: error %q does not contain %q", tc.src, err, tc.want)
		}
	}
}

func TestICUArgumentProblems(t *testing.T) {
	t.Setenv("TELA_DEV", "1")
	log := isolate(t)
	// A missing argument keeps its placeholder.
	if got := msg(t, "en", "Hi {name}", "other", "x"); got != "Hi {name}" {
		t.Errorf("missing arg = %q", got)
	}
	if got := msg(t, "en", "{n, plural, one {a} other {b}}", "n", "three"); got != "b" {
		t.Errorf("non-number plural = %q", got)
	}
	if len(*log) != 2 || !strings.Contains((*log)[0], `argument "name" is not passed`) || !strings.Contains((*log)[1], "must be a number") {
		t.Errorf("dev log = %q", *log)
	}
	// The same problem is reported once.
	msg(t, "en", "Hi {name}", "other", "x")
	if len(*log) != 2 {
		t.Errorf("repeated problem logged again: %q", *log)
	}
}
