package main

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The committed dates_gen.go is what the generator makes of the vendored
// CLDR files.
func TestGeneratedFileIsCurrent(t *testing.T) {
	want, err := generate("cldr")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../dates_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("i18n/dates_gen.go is out of date; run \"go generate ./i18n\"")
	}
}

// The committed numbering_gen.go is what the generator makes of the
// vendored numbering data.
func TestNumberingFileIsCurrent(t *testing.T) {
	want, err := generateNumbering("cldr")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../numbering_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("i18n/numbering_gen.go is out of date; run \"go generate ./i18n\"")
	}
}

// The committed numberformats_gen.go is what the generator makes of the
// vendored number symbols.
func TestNumberFormatsFileIsCurrent(t *testing.T) {
	want, err := generateNumberFormats("cldr")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../numberformats_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("i18n/numberformats_gen.go is out of date; run \"go generate ./i18n\"")
	}
}

// The committed currencies_gen.go is what the generator makes of the
// vendored currency data.
func TestCurrenciesFileIsCurrent(t *testing.T) {
	want, err := generateCurrencies("cldr")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../currencies_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("i18n/currencies_gen.go is out of date; run \"go generate ./i18n\"")
	}
}

// The committed localenumbers_gen.go is what the generator makes of the
// vendored compact patterns and currency names. Its names are compressed:
// when the bytes differ, the files are compared with the names inflated,
// so that another compress/flate (a new Go release) is not a difference.
func TestLocaleNumbersFileIsCurrent(t *testing.T) {
	want, err := generateLocaleNumbers("cldr")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../localenumbers_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	w, err := inflateRows(string(want))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w, `\x1eUSD\x1fUS dollars\x1foUS dollar`) {
		t.Error("inflated rows lack en's USD")
	}
	if bytes.Equal(got, want) {
		return
	}
	g, err := inflateRows(string(got))
	if err != nil {
		t.Fatal(err)
	}
	if g != w {
		t.Error("i18n/localenumbers_gen.go is out of date; run \"go generate ./i18n\"")
	}
}

// inflateRows replaces the compressed names of currencyNames' rows
// ({"loc", "unit", "names"}) with their text.
func inflateRows(src string) (string, error) {
	start := strings.Index(src, "var currencyNames = ")
	if start < 0 {
		return "", fmt.Errorf("no currencyNames")
	}
	var b strings.Builder
	b.WriteString(src[:start])
	for _, line := range strings.Split(src[start:], "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "{\"") || !strings.HasSuffix(trimmed, "},") {
			b.WriteString(line + "\n")
			continue
		}
		fields, err := unquoteFields(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{"), "},"))
		if err != nil || len(fields) != 3 {
			return "", fmt.Errorf("row %q: %v", trimmed, err)
		}
		text := ""
		if fields[2] != "" {
			raw, err := io.ReadAll(flate.NewReader(strings.NewReader(fields[2])))
			if err != nil {
				return "", fmt.Errorf("row %q: %v", fields[0], err)
			}
			text = string(raw)
		}
		fmt.Fprintf(&b, "%q %q %q\n", fields[0], fields[1], text)
	}
	return b.String(), nil
}

// unquoteFields splits `"a", "b", "c"` into its unquoted strings.
func unquoteFields(s string) ([]string, error) {
	var out []string
	for s = strings.TrimSpace(s); s != ""; {
		q, err := strconv.QuotedPrefix(s)
		if err != nil {
			return nil, err
		}
		v, err := strconv.Unquote(q)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s[len(q):]), ","))
	}
	return out, nil
}

func TestParsePattern(t *testing.T) {
	s, c, p := markSign, markCurrency, markPercent
	for _, tc := range []struct {
		src  string
		want pattern
	}{
		{"#,##0.###", pattern{"", "", s, "", 3, 3}},
		{"#,##,##0.###", pattern{"", "", s, "", 3, 2}},
		{"#,##0%", pattern{"", p, s, p, 3, 3}},
		{"#,##0\u00a0%", pattern{"", "\u00a0" + p, s, "\u00a0" + p, 3, 3}},
		{"%\u00a0#,##0", pattern{p + "\u00a0", "", s + p + "\u00a0", "", 3, 3}},
		{"\u200e#,##0'%'\u200e", pattern{"\u200e", "%\u200e", s + "\u200e", "%\u200e", 3, 3}},
		{"0.###", pattern{"", "", s, "", 0, 0}},
		{"¤#,##0.00;(¤#,##0.00)", pattern{c, "", "(" + c, ")", 3, 3}},
		{"¤ #,##0.00;¤-#,##0.00", pattern{c + " ", "", c + s, "", 3, 3}},
		{"\u200f#,##0.00\u00a0¤;\u200f-#,##0.00\u00a0¤", pattern{"\u200f", "\u00a0" + c, "\u200f" + s, "\u00a0" + c, 3, 3}},
	} {
		got, err := parsePattern(tc.src)
		if err != nil || got != tc.want {
			t.Errorf("parsePattern(%q) = %#v, %v; want %#v", tc.src, got, err, tc.want)
		}
	}
	for _, src := range []string{"#,##0‰", "#,##0 'x", "abc", "-#,##0", "#,##0E0"} {
		if _, err := parsePattern(src); err == nil {
			t.Errorf("parsePattern(%q): no error", src)
		}
	}
}

func TestICULocaleIDs(t *testing.T) {
	config := `<config><convert>
        <localeIds>
            // A
            af, az, az_AZ, az_Cyrl
            // S
            sr_Latn_CS, sr_ME, sh_YU, zh_TW
        </localeIds>
        <directory dir="coll"><localeIds>zh_HK</localeIds></directory>`
	ids, err := icuLocaleIDs(config)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(ids, " "), "az-AZ sh-YU sr-ME zh-TW"; got != want {
		t.Errorf("icuLocaleIDs = %q, want %q", got, want)
	}
	if _, err := icuLocaleIDs("<config><directory/><localeIds>a_B</localeIds>"); err == nil {
		t.Error("a list inside a directory: no error")
	}
}

// The script aliases come from the vendored ICU and CLDR files: the
// language-region locales ICU aliases, with deprecated subtags left out.
func TestScriptAliases(t *testing.T) {
	raw, err := os.ReadFile("cldr/" + defaultSystemsFile)
	if err != nil {
		t.Fatal(err)
	}
	var defaults map[string]string
	if err := json.Unmarshal(raw, &defaults); err != nil {
		t.Fatal(err)
	}
	got, err := scriptAliases("cldr", defaults)
	if err != nil {
		t.Fatal(err)
	}
	for id, script := range map[string]string{"pa-PK": "Arab", "pa-IN": "Guru", "zh-TW": "Hant", "zh-CN": "Hans", "sd-IN": "Deva", "sr-ME": "Latn", "sr-RS": "Cyrl", "uz-AF": "Arab"} {
		if got[id] != script {
			t.Errorf("%s: %q, want %q", id, got[id], script)
		}
	}
	for _, id := range []string{"in-ID", "iw-IL", "tl-PH", "sh-BA", "az-IR", "en-NH"} {
		if s, ok := got[id]; ok {
			t.Errorf("%s has the alias %q", id, s)
		}
	}
}

func TestChainOf(t *testing.T) {
	for name, want := range map[string]string{
		"en": "en", "es-MX": "es-MX es", "sr-Latn": "sr-Latn sr", "sr-Latn-ME": "sr-Latn-ME sr-Latn sr-ME sr",
		"es-419": "es-419 es", "ca-ES-valencia": "",
	} {
		if got := strings.Join(chainOf(name), " "); got != want {
			t.Errorf("chainOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestParentOf(t *testing.T) {
	for name, want := range map[string]string{"ar": "", "ar-EG": "ar", "ff-Adlm-BF": "ff-Adlm"} {
		if got := parentOf(name); got != want {
			t.Errorf("parentOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestUnsupportedLetter(t *testing.T) {
	for p, want := range map[string]string{
		"EEEE, d 'de' MMMM 'de' y": "",
		"h:mm:ss a zzzz":           "",
		"{1} 'at' {0}":             "",
		"yyyy-QQQ":                 "Q",
		"y 'Q' w":                  "w",
		"y '' y":                   "",
	} {
		if got := unsupportedLetter(p); got != want {
			t.Errorf("unsupportedLetter(%q) = %q, want %q", p, got, want)
		}
	}
}
