// Command gen generates i18n/dates_gen.go, the date, time and list
// patterns of package i18n, from the Unicode CLDR JSON data vendored in
// cldr/ (ADR-087).
//
// It is run by "go generate ./i18n" (see generate.go there) from the i18n
// directory. The output is deterministic and gofmt'ed. golang.org/x/text
// has no date formatting (its date package keeps its tables private), so
// the patterns, month and weekday names, AM/PM markers, era names and
// list patterns of the locales in the locales variable come from CLDR.
//
// Flags:
//
//	-cldr dir      directory of the vendored locale files (default internal/gen/cldr)
//	-out path      output file (default dates_gen.go)
//	-fetch         download the locale files from the cldr-json repository first
//	-version tag   cldr-json tag that -fetch reads (default 48.0.0)
//
// -fetch keeps only the parts of CLDR's ca-gregorian.json and
// listPatterns.json that the generator reads, under their original paths
// (main.<locale>.dates.calendars.gregorian..., main.<locale>.listPatterns...),
// so the vendored files stay small (about 4 KB per locale instead of
// 20 KB). The data is licensed under the Unicode License v3 (cldr/LICENSE).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// locales are the locales whose data is generated: the languages of
// CLDR's modern coverage that golang.org/x/text has plural rules for,
// plus the regional variants whose date patterns differ from their
// language's. Other locales resolve to the closest one with language
// matching (en-NZ to en-001, es-PE to es-419, pt-BR to pt). Adding one is
// adding its name here and running "go run ./internal/gen -fetch" and
// "go generate".
var locales = []string{
	"af", "am", "ar", "as", "az", "be", "bg", "bn", "bs", "ca", "chr", "cs", "cy",
	"da", "de", "el", "en", "es", "et", "eu", "fa", "fi", "fil", "fr", "ga", "gd",
	"gl", "gu", "ha", "he", "hi", "hr", "hu", "hy", "id", "ig", "is", "it", "ja",
	"jv", "ka", "kk", "km", "kn", "ko", "ky", "lo", "lt", "lv", "mk", "ml", "mn",
	"mr", "ms", "my", "nb", "ne", "nl", "nn", "no", "or", "pa", "pl", "ps", "pt",
	"ro", "ru", "si", "sk", "sl", "so", "sq", "sr", "sv", "sw", "ta", "te", "th",
	"tk", "tr", "uk", "ur", "uz", "vi", "yo", "yue", "zh", "zu",
	// Regional variants.
	"de-AT", "de-CH", "en-001", "en-AU", "en-CA", "en-GB", "en-IE", "en-IN",
	"en-NZ", "en-ZA", "es-419", "es-AR", "es-CL", "es-CO", "es-MX", "es-US",
	"fr-BE", "fr-CA", "fr-CH", "it-CH", "nl-BE", "pt-PT", "sr-Latn", "zh-Hant",
	"zh-Hant-HK",
}

const rawBase = "https://raw.githubusercontent.com/unicode-org/cldr-json/%s/cldr-json/%s/main/%s/%s"

func main() {
	cldrDir := flag.String("cldr", "internal/gen/cldr", "directory of the vendored CLDR locale files")
	out := flag.String("out", "dates_gen.go", "Go file to write")
	fetch := flag.Bool("fetch", false, "download the locale files from the cldr-json repository first")
	version := flag.String("version", "48.0.0", "cldr-json tag that -fetch reads")
	flag.Parse()

	if *fetch {
		if err := fetchAll(*cldrDir, *version); err != nil {
			fatal(err)
		}
	}
	src, err := generate(*cldrDir)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "i18n gen:", err)
	os.Exit(1)
}

// ---------------------------------------------------------------------
// Fetching and trimming.

func get(url string) ([]byte, error) {
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// dig returns the nested object at path.
func dig(m map[string]any, path ...string) (map[string]any, error) {
	for _, p := range path {
		next, ok := m[p].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("missing %q (path %s)", p, strings.Join(path, "."))
		}
		m = next
	}
	return m, nil
}

func mustDig(m map[string]any, path ...string) map[string]any {
	r, err := dig(m, path...)
	if err != nil {
		panic(err)
	}
	return r
}

// pick copies the listed keys of src into a new map.
func pick(src map[string]any, keys ...string) map[string]any {
	dst := map[string]any{}
	for _, k := range keys {
		if v, ok := src[k]; ok {
			dst[k] = v
		}
	}
	return dst
}

func fetchJSON(version, pkg, loc, file string) (map[string]any, error) {
	b, err := get(fmt.Sprintf(rawBase, version, pkg, loc, file))
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s %s: %v", loc, file, err)
	}
	return doc, nil
}

func fetchAll(dir, version string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, loc := range locales {
		doc, err := fetchJSON(version, "cldr-dates-full", loc, "ca-gregorian.json")
		if err != nil {
			return err
		}
		greg, err := dig(doc, "main", loc, "dates", "calendars", "gregorian")
		if err != nil {
			return fmt.Errorf("%s ca-gregorian.json: %v", loc, err)
		}
		periods := mustDig(greg, "dayPeriods", "format")
		eras := mustDig(greg, "eras")
		trim := map[string]any{
			"months": map[string]any{"format": pick(mustDig(greg, "months", "format"), "abbreviated", "wide")},
			"days":   map[string]any{"format": pick(mustDig(greg, "days", "format"), "abbreviated", "wide")},
			"dayPeriods": map[string]any{"format": map[string]any{
				"abbreviated": pick(mustDig(periods, "abbreviated"), "am", "pm"),
				"wide":        pick(mustDig(periods, "wide"), "am", "pm"),
			}},
			"eras": map[string]any{
				"eraAbbr":  pick(mustDig(eras, "eraAbbr"), "0", "1"),
				"eraNames": pick(mustDig(eras, "eraNames"), "0", "1"),
			},
			"dateFormats":            pick(mustDig(greg, "dateFormats"), "full", "long", "medium", "short"),
			"timeFormats":            pick(mustDig(greg, "timeFormats"), "full", "long", "medium", "short"),
			"dateTimeFormats":        pick(mustDig(greg, "dateTimeFormats"), "full", "long", "medium", "short"),
			"dateTimeFormats-atTime": mustDig(greg, "dateTimeFormats-atTime"),
		}
		lp, err := fetchJSON(version, "cldr-misc-full", loc, "listPatterns.json")
		if err != nil {
			return err
		}
		lps, err := dig(lp, "main", loc, "listPatterns")
		if err != nil {
			return fmt.Errorf("%s listPatterns.json: %v", loc, err)
		}
		out := map[string]any{"main": map[string]any{loc: map[string]any{
			"dates":        map[string]any{"calendars": map[string]any{"gregorian": trim}},
			"listPatterns": pick(lps, "listPattern-type-standard"),
		}}}
		b, err := json.MarshalIndent(out, "", " ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, loc+".json"), append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "fetched", loc)
	}
	return nil
}

// ---------------------------------------------------------------------
// Reading.

type strMap map[string]string

type gregorian struct {
	Months struct {
		Format struct {
			Abbreviated strMap `json:"abbreviated"`
			Wide        strMap `json:"wide"`
		} `json:"format"`
	} `json:"months"`
	Days struct {
		Format struct {
			Abbreviated strMap `json:"abbreviated"`
			Wide        strMap `json:"wide"`
		} `json:"format"`
	} `json:"days"`
	DayPeriods struct {
		Format struct {
			Abbreviated strMap `json:"abbreviated"`
			Wide        strMap `json:"wide"`
		} `json:"format"`
	} `json:"dayPeriods"`
	Eras struct {
		Abbr  strMap `json:"eraAbbr"`
		Names strMap `json:"eraNames"`
	} `json:"eras"`
	DateFormats strMap `json:"dateFormats"`
	TimeFormats strMap `json:"timeFormats"`
	DateTime    strMap `json:"dateTimeFormats"`
	AtTime      struct {
		Standard strMap `json:"standard"`
	} `json:"dateTimeFormats-atTime"`
}

type localeFile struct {
	Main map[string]struct {
		Dates struct {
			Calendars struct {
				Gregorian gregorian `json:"gregorian"`
			} `json:"calendars"`
		} `json:"dates"`
		ListPatterns struct {
			Standard strMap `json:"listPattern-type-standard"`
		} `json:"listPatterns"`
	} `json:"main"`
}

// Field layout of a locale record (the order of fields in dateData rows).
const (
	fMonthsAbbr = 0  // 12
	fMonthsWide = 12 // 12
	fDaysAbbr   = 24 // 7, Sunday first
	fDaysWide   = 31 // 7
	fAM         = 38
	fPM         = 39
	fErasAbbr   = 40 // 2: BC, AD
	fErasWide   = 42 // 2
	fDate       = 44 // 4: full, long, medium, short
	fTime       = 48 // 4
	fDateTime   = 52 // 4
	fListStart  = 56
	fListMiddle = 57
	fListEnd    = 58
	fListTwo    = 59
	numFields   = 60
)

var dayKeys = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
var lengths = []string{"full", "long", "medium", "short"}

func record(loc string, f localeFile) ([numFields]string, error) {
	var r [numFields]string
	d, ok := f.Main[loc]
	if !ok {
		return r, fmt.Errorf("%s.json has no main.%s", loc, loc)
	}
	g := d.Dates.Calendars.Gregorian
	var err error
	set := func(i int, m strMap, key, what string) {
		if err != nil {
			return
		}
		v, ok := m[key]
		if !ok || v == "" {
			err = fmt.Errorf("%s: %s %q is missing", loc, what, key)
			return
		}
		r[i] = v
	}
	for i := 0; i < 12; i++ {
		k := strconv.Itoa(i + 1)
		set(fMonthsAbbr+i, g.Months.Format.Abbreviated, k, "abbreviated month")
		set(fMonthsWide+i, g.Months.Format.Wide, k, "wide month")
	}
	for i, k := range dayKeys {
		set(fDaysAbbr+i, g.Days.Format.Abbreviated, k, "abbreviated day")
		set(fDaysWide+i, g.Days.Format.Wide, k, "wide day")
	}
	set(fAM, g.DayPeriods.Format.Abbreviated, "am", "AM marker")
	set(fPM, g.DayPeriods.Format.Abbreviated, "pm", "PM marker")
	for i, k := range []string{"0", "1"} {
		set(fErasAbbr+i, g.Eras.Abbr, k, "abbreviated era")
		set(fErasWide+i, g.Eras.Names, k, "era name")
	}
	for i, k := range lengths {
		set(fDate+i, g.DateFormats, k, "date format")
		set(fTime+i, g.TimeFormats, k, "time format")
		// "{1} 'at' {0}" for full and long where CLDR has it (the glue
		// ICU uses for a date and a time together), the plain one
		// otherwise.
		if v := g.AtTime.Standard[k]; v != "" {
			set(fDateTime+i, strMap{k: v}, k, "date-time format")
		} else {
			set(fDateTime+i, g.DateTime, k, "date-time format")
		}
	}
	lp := d.ListPatterns.Standard
	set(fListStart, lp, "start", "list pattern")
	set(fListMiddle, lp, "middle", "list pattern")
	set(fListEnd, lp, "end", "list pattern")
	set(fListTwo, lp, "2", "list pattern")
	if err != nil {
		return r, err
	}
	// Patterns must only use letters the formatter implements.
	for _, p := range r[fDate:fListStart] {
		if bad := unsupportedLetter(p); bad != "" {
			return r, fmt.Errorf("%s: pattern %q uses %q, which the formatter does not implement", loc, p, bad)
		}
	}
	return r, nil
}

// supportedLetters are the pattern letters of datefmt.go.
const supportedLetters = "GyMLdEecaBbhHKkmszv"

// unsupportedLetter returns the first unquoted letter of a CLDR pattern
// that datefmt.go does not implement, and "" if there is none. The glue
// of date-time patterns ({0}, {1}) is skipped.
func unsupportedLetter(p string) string {
	inQuote := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '\'':
			if i+1 < len(p) && p[i+1] == '\'' {
				i++
				continue
			}
			inQuote = !inQuote
		case inQuote:
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			if !strings.ContainsRune(supportedLetters, rune(c)) {
				return string(c)
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------
// Writing.

func generate(dir string) ([]byte, error) {
	names := append([]string(nil), locales...)
	sort.Strings(names)
	recs := make([][numFields]string, len(names))
	for i, loc := range names {
		b, err := os.ReadFile(filepath.Join(dir, loc+".json"))
		if err != nil {
			return nil, fmt.Errorf("%v (run \"go run ./internal/gen -fetch\" to download the CLDR data)", err)
		}
		var f localeFile
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s.json: %v", loc, err)
		}
		if recs[i], err = record(loc, f); err != nil {
			return nil, err
		}
	}

	// A string pool: every distinct string once, in first-use order,
	// concatenated in dateText; dateEnds[i] is the end offset of string i.
	index := map[string]int{}
	var pool []string
	for _, r := range recs {
		for _, s := range r {
			if _, ok := index[s]; !ok {
				index[s] = len(pool)
				pool = append(pool, s)
			}
		}
	}
	if len(pool) > 1<<16 {
		return nil, fmt.Errorf("%d distinct strings do not fit uint16 indices", len(pool))
	}

	var b bytes.Buffer
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	w("// Code generated by internal/gen from the Unicode CLDR JSON data in\n")
	w("// internal/gen/cldr; DO NOT EDIT.\n")
	w("// Data files copyright (c) 1991-2025 Unicode, Inc. (Unicode License v3,\n")
	w("// see internal/gen/cldr/LICENSE and THIRD_PARTY.md).\n\n")
	w("package i18n\n\n")
	w("// Layout of a locale's record in dateData.\n")
	w("const (\n")
	for _, c := range []struct {
		n string
		v int
	}{
		{"dfMonthsAbbr", fMonthsAbbr}, {"dfMonthsWide", fMonthsWide}, {"dfDaysAbbr", fDaysAbbr},
		{"dfDaysWide", fDaysWide}, {"dfAM", fAM}, {"dfPM", fPM}, {"dfErasAbbr", fErasAbbr},
		{"dfErasWide", fErasWide}, {"dfDate", fDate}, {"dfTime", fTime}, {"dfDateTime", fDateTime},
		{"dfListStart", fListStart}, {"dfListMiddle", fListMiddle}, {"dfListEnd", fListEnd},
		{"dfListTwo", fListTwo}, {"dfNumFields", numFields},
	} {
		w("\t%s = %d\n", c.n, c.v)
	}
	w(")\n\n")
	w("// dateTags are the locales of dateData, sorted.\n")
	w("var dateTags = [...]string{\n")
	for _, n := range names {
		w("\t%q,\n", n)
	}
	w("}\n\n")
	w("// dateData holds, per locale of dateTags, the indices in the string pool\n")
	w("// (dateString) of its fields.\n")
	w("var dateData = [...][dfNumFields]uint16{\n")
	for _, r := range recs {
		w("\t{")
		for i, s := range r {
			if i > 0 {
				w(", ")
			}
			w("%d", index[s])
		}
		w("},\n")
	}
	w("}\n\n")
	w("// dateEnds[i] is the end offset of string i of the pool in dateText.\n")
	w("var dateEnds = [...]uint32{")
	off := 0
	for i, s := range pool {
		off += len(s)
		if i%16 == 0 {
			w("\n\t")
		} else {
			w(" ")
		}
		w("%d,", off)
	}
	w("\n}\n\n")
	w("const dateText = \"\" +\n")
	for i, s := range pool {
		sep := " +"
		if i == len(pool)-1 {
			sep = ""
		}
		w("\t%s%s\n", strconv.Quote(s), sep)
	}
	return format.Source(b.Bytes())
}
