package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The number symbols and patterns (ADR-259). golang.org/x/text formats
// numbers with CLDR 32, whose symbols differ from CLDR 48's (the minus sign
// of Persian and Urdu, the bidi marks around negatives, Nepali grouping,
// minimum grouping digits). The generator reads the symbols and the
// standard decimal and percent patterns of every CLDR locale and every
// numbering system the locale has data for, vendored as numberFormats.json,
// and emits numberformats_gen.go: the (locale, system) pairs whose data
// differs from what their parent resolves to, or that no parent has.

// numberFormatsFile is the vendored digest of main.<locale>.numbers, relative
// to the cldr directory: {"<locale>": numberDigest}, one locale per line.
const numberFormatsFile = "numberFormats.json"

// numberSystemDigest is what is kept of the symbols and the standard
// patterns of one numbering system of a locale, under CLDR's own names.
type numberSystemDigest struct {
	Decimal       string `json:"decimal"`
	Group         string `json:"group"`
	MinusSign     string `json:"minusSign"`
	PercentSign   string `json:"percentSign"`
	Infinity      string `json:"infinity"`
	NaN           string `json:"nan"`
	DecimalFormat string `json:"decimalFormat"`
	PercentFormat string `json:"percentFormat"`
}

// numberDigest is what is kept of a locale's main.<locale>.numbers.
type numberDigest struct {
	MinimumGroupingDigits string                        `json:"minimumGroupingDigits"`
	Systems               map[string]numberSystemDigest `json:"systems"`
}

// digestNumbers extracts the numberDigest from a locale's numbers object.
func digestNumbers(nums map[string]any) (numberDigest, error) {
	d := numberDigest{Systems: map[string]numberSystemDigest{}}
	d.MinimumGroupingDigits, _ = nums["minimumGroupingDigits"].(string)
	if d.MinimumGroupingDigits == "" {
		d.MinimumGroupingDigits = "1"
	}
	const symbolsPrefix = "symbols-numberSystem-"
	for key, v := range nums {
		sys, ok := strings.CutPrefix(key, symbolsPrefix)
		if !ok {
			continue
		}
		syms, ok := v.(map[string]any)
		if !ok {
			return d, fmt.Errorf("%s is not an object", key)
		}
		str := func(m map[string]any, k string) (string, error) {
			s, ok := m[k].(string)
			if !ok {
				return "", fmt.Errorf("%s: no %q", key, k)
			}
			return s, nil
		}
		var sd numberSystemDigest
		var err error
		for _, f := range []struct {
			dst *string
			key string
		}{
			{&sd.Decimal, "decimal"}, {&sd.Group, "group"}, {&sd.MinusSign, "minusSign"},
			{&sd.PercentSign, "percentSign"}, {&sd.Infinity, "infinity"}, {&sd.NaN, "nan"},
		} {
			if *f.dst, err = str(syms, f.key); err != nil {
				return d, err
			}
		}
		standard := func(kind string) (string, error) {
			k := kind + "-numberSystem-" + sys
			m, ok := nums[k].(map[string]any)
			if !ok {
				return "", fmt.Errorf("symbols for %s but no %s", sys, k)
			}
			return str(m, "standard")
		}
		if sd.DecimalFormat, err = standard("decimalFormats"); err != nil {
			return d, err
		}
		if sd.PercentFormat, err = standard("percentFormats"); err != nil {
			return d, err
		}
		d.Systems[sys] = sd
	}
	if len(d.Systems) == 0 {
		return d, fmt.Errorf("no symbols")
	}
	return d, nil
}

// writeNumberFormats writes the digest sorted, one locale per line.
func writeNumberFormats(path string, formats map[string]numberDigest) error {
	locs := make([]string, 0, len(formats))
	for loc := range formats {
		locs = append(locs, loc)
	}
	sort.Strings(locs)
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, loc := range locs {
		line, err := marshalNoEscape(formats[loc])
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, " %q: %s", loc, line)
		if i < len(locs)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// marshalNoEscape is json.Marshal without HTML escaping (the patterns
// have no <, > or &, but keeping the text as is costs nothing).
func marshalNoEscape(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// numberRec is a row of the generated table; it mirrors i18n's
// numberSymbols.
type numberRec struct {
	decimal, group, minus, infinity, nan string
	pctPrefix, pctSuffix                 string
	primary, secondary, minGroup         int
}

// pctMark stands for the percent sign in an affix of a pattern while it is
// parsed.
const pctMark = "\x00"

// parsePattern reads the positive subpattern of a CLDR decimal or percent
// pattern ("#,##0.###", "#,##,##0%", "% #,##0"): its prefix and
// suffix (the percent sign as pctMark) and the primary and secondary
// grouping sizes (0: no grouping). Anything else in an affix fails.
func parsePattern(p string) (prefix, suffix string, primary, secondary int, err error) {
	pos, _, _ := strings.Cut(p, ";")
	var pre, post, num strings.Builder
	state := 0 // 0 prefix, 1 number, 2 suffix
	add := func(s string) {
		switch state {
		case 0:
			pre.WriteString(s)
		default:
			state = 2
			post.WriteString(s)
		}
	}
	rs := []rune(pos)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\'':
			if i+1 < len(rs) && rs[i+1] == '\'' {
				add("'")
				i++
				continue
			}
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			if j == len(rs) {
				return "", "", 0, 0, fmt.Errorf("pattern %q: unterminated quote", p)
			}
			add(string(rs[i+1 : j]))
			i = j
		case state < 2 && strings.ContainsRune("#0,.", c):
			state = 1
			num.WriteRune(c)
		case c == '%':
			add(pctMark)
		case strings.ContainsRune("‰¤*@E+-", c):
			return "", "", 0, 0, fmt.Errorf("pattern %q: unsupported %q", p, c)
		default:
			add(string(c))
		}
	}
	if num.Len() == 0 {
		return "", "", 0, 0, fmt.Errorf("pattern %q: no number", p)
	}
	intPart, _, _ := strings.Cut(num.String(), ".")
	last := strings.LastIndex(intPart, ",")
	if last >= 0 {
		primary = len(intPart) - last - 1
		secondary = primary
		if prev := strings.LastIndex(intPart[:last], ","); prev >= 0 {
			secondary = last - prev - 1
		}
	}
	return pre.String(), post.String(), primary, secondary, nil
}

// numberRecord makes the table row of a locale's numbering system.
func numberRecord(minGroup string, sd numberSystemDigest) (numberRec, error) {
	r := numberRec{decimal: sd.Decimal, group: sd.Group, minus: sd.MinusSign, infinity: sd.Infinity, nan: sd.NaN}
	n, err := strconv.Atoi(minGroup)
	if err != nil || n < 1 || n > 9 {
		return r, fmt.Errorf("minimumGroupingDigits %q", minGroup)
	}
	r.minGroup = n
	dp, ds, primary, secondary, err := parsePattern(sd.DecimalFormat)
	if err != nil {
		return r, err
	}
	if dp != "" || ds != "" {
		return r, fmt.Errorf("decimal pattern %q has a prefix or suffix", sd.DecimalFormat)
	}
	r.primary, r.secondary = primary, secondary
	pp, ps, _, _, err := parsePattern(sd.PercentFormat)
	if err != nil {
		return r, err
	}
	r.pctPrefix = strings.ReplaceAll(pp, pctMark, sd.PercentSign)
	r.pctSuffix = strings.ReplaceAll(ps, pctMark, sd.PercentSign)
	return r, nil
}

// generateNumberFormats returns numberformats_gen.go.
func generateNumberFormats(dir string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, numberFormatsFile))
	if err != nil {
		return nil, err
	}
	var digests map[string]numberDigest
	if err := json.Unmarshal(raw, &digests); err != nil {
		return nil, fmt.Errorf("%s: %v", numberFormatsFile, err)
	}
	if _, ok := digests["und"]; !ok {
		return nil, fmt.Errorf("%s: no \"und\" (the root locale)", numberFormatsFile)
	}
	// Parents first (the root before all), so a locale is compared with
	// what its parent resolves to once the table is built.
	names := make([]string, 0, len(digests))
	for loc := range digests {
		names = append(names, loc)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "und") != (names[j] == "und") {
			return names[i] == "und"
		}
		di, dj := strings.Count(names[i], "-"), strings.Count(names[j], "-")
		if di != dj {
			return di < dj
		}
		return names[i] < names[j]
	})
	table := map[string]numberRec{}
	// resolve is what the runtime lookup finds for a locale and a system:
	// the locale, its parents and the root, if one of them has data for
	// the system (the runtime tells a system with data from one without,
	// which it formats as Latin, so a row equal to the Latin one stays).
	resolve := func(name, sys string) (numberRec, bool) {
		for n := name; n != ""; n = parentOf(n) {
			if r, ok := table[n+"/"+sys]; ok {
				return r, true
			}
		}
		r, ok := table["und/"+sys]
		return r, ok
	}
	for _, loc := range names {
		d := digests[loc]
		systems := make([]string, 0, len(d.Systems))
		for sys := range d.Systems {
			systems = append(systems, sys)
		}
		sort.Strings(systems)
		for _, sys := range systems {
			rec, err := numberRecord(d.MinimumGroupingDigits, d.Systems[sys])
			if err != nil {
				return nil, fmt.Errorf("%s (%s): %v", loc, sys, err)
			}
			if inherited, ok := resolve(parentOf(loc), sys); ok && loc != "und" && rec == inherited {
				continue
			}
			table[loc+"/"+sys] = rec
		}
	}

	var b bytes.Buffer
	b.WriteString("// Code generated by internal/gen from the Unicode CLDR JSON data in\n")
	b.WriteString("// internal/gen/cldr; DO NOT EDIT.\n")
	b.WriteString("// Data files copyright (c) 1991-2025 Unicode, Inc. (Unicode License v3,\n")
	b.WriteString("// see internal/gen/cldr/LICENSE and THIRD_PARTY.md).\n\n")
	b.WriteString("package i18n\n\n")
	b.WriteString("// numberTable has the number symbols and patterns of the (locale, numbering\n")
	b.WriteString("// system) pairs, keyed \"locale/system\", whose data differs from what their\n")
	b.WriteString("// parent (language-script-region, language-script, language, und) resolves to,\n")
	b.WriteString("// or that no parent has data for.\n")
	b.WriteString("// The fields are numberSymbols': decimal, group, minus, infinity, NaN, the\n")
	b.WriteString("// percent affixes, the primary and secondary grouping sizes and the minimum\n")
	b.WriteString("// grouping digits.\n")
	b.WriteString("var numberTable = map[string]numberSymbols{\n")
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r := table[k]
		fmt.Fprintf(&b, "\t%q: {%q, %q, %q, %q, %q, %q, %q, %d, %d, %d},\n",
			k, r.decimal, r.group, r.minus, r.infinity, r.nan, r.pctPrefix, r.pctSuffix, r.primary, r.secondary, r.minGroup)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}
