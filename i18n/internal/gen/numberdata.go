package main

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Currencies and compact numbers (ADR-266). The generator emits
// currencies_gen.go (the digits of each currency, the currency of each
// region, the currency symbols of every CLDR locale) and
// localenumbers_gen.go (for the generated locales: the compact decimal and
// currency patterns and the currency names with their plural forms).
// Only the currencies some region uses today (CLDR's currencyData, no end
// date, legal tender) have symbols and names; others are written with
// their ISO code, as ICU does for a code it has no data for.

const (
	// currencySymbolsFile has, per CLDR locale, the symbol and the narrow
	// symbol of the current currencies where they differ from the closest
	// ancestor by truncation (es-MX from es, the root "und" in full):
	// {"<locale>": {"<code>": ["<symbol>", "<narrow>"]}}.
	currencySymbolsFile = "currencySymbols.json"
	// localeNumbersFile has, per generated locale, its currency unit
	// patterns, compact patterns and currency names (localeNumbers).
	localeNumbersFile = "localeNumbers.json"
	// currencyDataFile is cldr-core's supplemental/currencyData.json under
	// its original path: the fractions, and the current tender currencies
	// of each region.
	currencyDataFile = "supplemental/currencyData.json"
)

// pluralCategories are CLDR's plural categories, "other" last.
var pluralCategories = []string{"zero", "one", "two", "few", "many", "other"}

// localeNumbers is what is kept of a generated locale's numbers.json and
// currencies.json. Plural forms equal to the "other" form are left out
// (the runtime falls back to it). A regional variant (es-MX) keeps only
// what differs from its closest generated ancestor by truncation (es).
type localeNumbers struct {
	// UnitPattern is currencyFormats' unitPattern-count-*, by category:
	// "{0} {1}", the number and the currency's name.
	UnitPattern map[string]string `json:"unitPattern,omitempty"`
	// Compact are the compact patterns by numbering system; a system
	// whose patterns equal latn's is left out.
	Compact map[string]compactDigest `json:"compact,omitempty"`
	// Currencies are the currencies' displayName-count-* by category.
	Currencies map[string]map[string]string `json:"currencies,omitempty"`
}

// compactDigest are the compact patterns of one numbering system: for
// each magnitude from 10^3, the patterns by plural category (CLDR's
// "1000-count-one" is Short[0]["one"]; "1000-count-1", the pattern of
// exactly 1, is Short[0]["1"]). A kind the system lacks is nil, and so is
// a magnitude without patterns.
type compactDigest struct {
	Short    []map[string]string `json:"short,omitempty"`
	Long     []map[string]string `json:"long,omitempty"`
	Currency []map[string]string `json:"currency,omitempty"`
}

var compactKey = regexp.MustCompile(`^1(0{3,})-count-(zero|one|two|few|many|other|0|1)$`)

// formsOf drops the plural forms equal to the "other" form.
func formsOf(m map[string]string) map[string]string {
	out := map[string]string{}
	for c, s := range m {
		if c == "other" || s != m["other"] {
			out[c] = s
		}
	}
	return out
}

// compactOf keeps the compact patterns of a CLDR format map, by magnitude.
func compactOf(v any) ([]map[string]string, error) {
	m, _ := v.(map[string]any)
	var out []map[string]string
	for k, p := range m {
		s, ok := p.(string)
		match := compactKey.FindStringSubmatch(k)
		if !ok || match == nil {
			continue
		}
		i := len(match[1]) - 3
		for len(out) <= i {
			out = append(out, nil)
		}
		if out[i] == nil {
			out[i] = map[string]string{}
		}
		out[i][match[2]] = s
	}
	for i, forms := range out {
		if forms != nil && forms["other"] == "" {
			return nil, fmt.Errorf("no pattern for 10^%d", i+3)
		}
	}
	return out, nil
}

// dropRootLong removes the long compact patterns a locale inherits from
// the root in cldr-json: ICU's root has no long patterns, so ICU writes
// those magnitudes in full (ps: "۱۰۰۰", but "۱٫۲G" from its own data,
// which has a "one" form where the root has only "other"), and a locale
// left with none uses its short ones. A pattern equal to the root's is
// the locale's own when its ancestor (parent, the raw data of the closest
// generated ancestor by truncation) has another one: zh-Hant-HK's "0K"
// replaces zh-Hant's "0". Then the plural forms equal to the "other"
// form are dropped (formsOf).
func dropRootLong(ln *localeNumbers, root []map[string]string, parent *localeNumbers) {
	defer func() {
		for sys, cd := range ln.Compact {
			for _, kind := range [][]map[string]string{cd.Short, cd.Long, cd.Currency} {
				for i, forms := range kind {
					if forms != nil {
						kind[i] = formsOf(forms)
					}
				}
			}
			ln.Compact[sys] = cd
		}
	}()
	for sys, cd := range ln.Compact {
		kept := false
		var up []map[string]string
		if parent != nil {
			up = parent.Compact[sys].Long
			if up == nil {
				up = parent.Compact["latn"].Long
			}
		}
		for i, forms := range cd.Long {
			inherited := i < len(root) && forms != nil && equalForms(forms, root[i])
			if inherited && parent != nil && i < len(up) && !equalForms(up[i], root[i]) {
				inherited = false // the locale's own, over its parent's
			}
			if inherited {
				cd.Long[i] = nil
			} else if forms != nil {
				kept = true
			}
		}
		if !kept {
			cd.Long = nil
		}
		for len(cd.Long) > 0 && cd.Long[len(cd.Long)-1] == nil {
			cd.Long = cd.Long[:len(cd.Long)-1]
		}
		ln.Compact[sys] = cd
	}
}

// digestLocaleNumbers extracts a generated locale's localeNumbers.
func digestLocaleNumbers(nums, curs map[string]any, current []string) (localeNumbers, error) {
	ln := localeNumbers{UnitPattern: map[string]string{}, Compact: map[string]compactDigest{}, Currencies: map[string]map[string]string{}}
	sys, _ := nums["defaultNumberingSystem"].(string)
	cf, ok := nums["currencyFormats-numberSystem-"+sys].(map[string]any)
	if !ok {
		return ln, fmt.Errorf("no currencyFormats for %s", sys)
	}
	for _, c := range pluralCategories {
		if p, ok := cf["unitPattern-count-"+c].(string); ok {
			ln.UnitPattern[c] = p
		}
	}
	if ln.UnitPattern["other"] == "" {
		return ln, fmt.Errorf("no unitPattern-count-other")
	}
	ln.UnitPattern = formsOf(ln.UnitPattern)
	for key, v := range nums {
		sys, ok := strings.CutPrefix(key, "decimalFormats-numberSystem-")
		if !ok {
			continue
		}
		df, _ := v.(map[string]any)
		short, _ := df["short"].(map[string]any)
		long, _ := df["long"].(map[string]any)
		cf, _ := nums["currencyFormats-numberSystem-"+sys].(map[string]any)
		cshort, _ := cf["short"].(map[string]any)
		// A system may lack a kind (ar's arab system has no currency
		// patterns): ICU then uses the Latin ones.
		var cd compactDigest
		var err error
		for _, k := range []struct {
			dst *[]map[string]string
			src any
		}{{&cd.Short, short["decimalFormat"]}, {&cd.Long, long["decimalFormat"]}, {&cd.Currency, cshort["standard"]}} {
			if *k.dst, err = compactOf(k.src); err != nil {
				return ln, fmt.Errorf("%s: %v", key, err)
			}
		}
		ln.Compact[sys] = cd
	}
	latn, ok := ln.Compact["latn"]
	if !ok || len(latn.Short) == 0 || len(latn.Currency) == 0 {
		return ln, fmt.Errorf("missing latn compact patterns")
	}
	for sys, cd := range ln.Compact {
		if sys != "latn" && equalCompact(cd, latn) {
			delete(ln.Compact, sys)
		}
	}
	for _, code := range current {
		e, ok := curs[code].(map[string]any)
		if !ok {
			continue
		}
		names := map[string]string{}
		for _, c := range pluralCategories {
			if s, ok := e["displayName-count-"+c].(string); ok {
				names[c] = s
			}
		}
		if names["other"] == "" {
			if s, ok := e["displayName"].(string); ok {
				names["other"] = s
			}
		}
		if names["other"] != "" {
			ln.Currencies[code] = formsOf(names)
		}
	}
	return ln, nil
}

func equalForms(x, y map[string]string) bool {
	if len(x) != len(y) {
		return false
	}
	for k, v := range x {
		if s, ok := y[k]; !ok || s != v {
			return false
		}
	}
	return true
}

func equalMagnitudes(a, b []map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalForms(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalCompact(a, b compactDigest) bool {
	return equalMagnitudes(a.Short, b.Short) && equalMagnitudes(a.Long, b.Long) && equalMagnitudes(a.Currency, b.Currency)
}

// diffLocaleNumbers keeps, for each generated locale with a generated
// ancestor by truncation, what differs from the ancestor's data.
func diffLocaleNumbers(full map[string]localeNumbers) map[string]localeNumbers {
	set := map[string]bool{}
	for l := range full {
		set[l] = true
	}
	out := map[string]localeNumbers{}
	for loc, ln := range full {
		chain := setChain(loc, set)
		if len(chain) == 1 {
			out[loc] = ln
			continue
		}
		a := full[chain[1]]
		var d localeNumbers
		if !equalForms(ln.UnitPattern, a.UnitPattern) {
			d.UnitPattern = ln.UnitPattern
		}
		for sys, cd := range ln.Compact {
			ac := a.Compact[sys]
			var dc compactDigest
			if !equalMagnitudes(cd.Short, ac.Short) {
				dc.Short = cd.Short
			}
			if !equalMagnitudes(cd.Long, ac.Long) {
				dc.Long = cd.Long
			}
			if !equalMagnitudes(cd.Currency, ac.Currency) {
				dc.Currency = cd.Currency
			}
			if dc.Short != nil || dc.Long != nil || dc.Currency != nil {
				if d.Compact == nil {
					d.Compact = map[string]compactDigest{}
				}
				d.Compact[sys] = dc
			}
		}
		for code, names := range ln.Currencies {
			if !equalForms(names, a.Currencies[code]) {
				if d.Currencies == nil {
					d.Currencies = map[string]map[string]string{}
				}
				d.Currencies[code] = names
			}
		}
		out[loc] = d
	}
	return out
}

// undoLocaleNumbersDiffs rebuilds the full data of the generated locales,
// ancestors first.
func undoLocaleNumbersDiffs(diffs map[string]localeNumbers, names []string, set map[string]bool) map[string]localeNumbers {
	full := map[string]localeNumbers{}
	for _, loc := range names {
		d := diffs[loc]
		chain := setChain(loc, set)
		if len(chain) == 1 {
			full[loc] = d
			continue
		}
		a := full[chain[1]]
		ln := localeNumbers{UnitPattern: a.UnitPattern, Compact: map[string]compactDigest{}, Currencies: map[string]map[string]string{}}
		if d.UnitPattern != nil {
			ln.UnitPattern = d.UnitPattern
		}
		for sys, cd := range a.Compact {
			ln.Compact[sys] = cd
		}
		for sys, dc := range d.Compact {
			cd := ln.Compact[sys]
			if dc.Short != nil {
				cd.Short = dc.Short
			}
			if dc.Long != nil {
				cd.Long = dc.Long
			}
			if dc.Currency != nil {
				cd.Currency = dc.Currency
			}
			ln.Compact[sys] = cd
		}
		for code, n := range a.Currencies {
			ln.Currencies[code] = n
		}
		for code, n := range d.Currencies {
			ln.Currencies[code] = n
		}
		full[loc] = ln
	}
	return full
}

// currencySymbolsOf returns the symbol and the narrow symbol of each
// current currency of a locale's currencies object; a currency without a
// symbol has its code.
func currencySymbolsOf(curs map[string]any, current []string) map[string][2]string {
	out := map[string][2]string{}
	for _, code := range current {
		e, _ := curs[code].(map[string]any)
		sym, _ := e["symbol"].(string)
		if sym == "" {
			sym = code
		}
		narrow, _ := e["symbol-alt-narrow"].(string)
		if narrow == "" {
			narrow = sym
		}
		out[code] = [2]string{sym, narrow}
	}
	return out
}

// truncationAncestor returns the closest ancestor of a locale by
// truncation that the table has, else "und".
func truncationAncestor[V any](table map[string]V, loc string) string {
	for p := parentOf(loc); p != ""; p = parentOf(p) {
		if _, ok := table[p]; ok {
			return p
		}
	}
	return "und"
}

// diffSymbols keeps, per locale, the symbols that differ from its
// truncation ancestor's (the root in full).
func diffSymbols(full map[string]map[string][2]string) map[string]map[string][2]string {
	out := map[string]map[string][2]string{}
	for loc, syms := range full {
		if loc == "und" {
			out[loc] = syms
			continue
		}
		parent := full[truncationAncestor(full, loc)]
		d := map[string][2]string{}
		for code, s := range syms {
			if parent[code] != s {
				d[code] = s
			}
		}
		if len(d) > 0 {
			out[loc] = d
		}
	}
	return out
}

// undoSymbolDiffs rebuilds every locale's full symbols from the vendored
// differences.
func undoSymbolDiffs(diffs map[string]map[string][2]string, locs []string) (map[string]map[string][2]string, error) {
	und, ok := diffs["und"]
	if !ok {
		return nil, fmt.Errorf("%s: no \"und\"", currencySymbolsFile)
	}
	full := map[string]map[string][2]string{"und": und}
	sort.Slice(locs, func(i, j int) bool {
		di, dj := strings.Count(locs[i], "-"), strings.Count(locs[j], "-")
		if di != dj {
			return di < dj
		}
		return locs[i] < locs[j]
	})
	known := map[string]bool{}
	for _, l := range locs {
		known[l] = true
	}
	for _, loc := range locs {
		if loc == "und" {
			continue
		}
		parent := "und"
		for p := parentOf(loc); p != ""; p = parentOf(p) {
			if known[p] {
				parent = p
				break
			}
		}
		m := map[string][2]string{}
		for k, v := range full[parent] {
			m[k] = v
		}
		for k, v := range diffs[loc] {
			m[k] = v
		}
		full[loc] = m
	}
	return full, nil
}

// fetchCurrencyData vendors supplemental currencyData (the fractions, and
// for each region its current tender currencies, in CLDR's order) and
// returns the current currencies, sorted.
func fetchCurrencyData(dir, version string) ([]string, error) {
	b, err := get(fmt.Sprintf(rawCurrencies, version))
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("currencyData.json: %v", err)
	}
	cd, err := dig(doc, "supplemental", "currencyData")
	if err != nil {
		return nil, fmt.Errorf("currencyData.json: %v", err)
	}
	fractions, err := dig(cd, "fractions")
	if err != nil {
		return nil, fmt.Errorf("currencyData.json: %v", err)
	}
	regions, err := dig(cd, "region")
	if err != nil {
		return nil, fmt.Errorf("currencyData.json: %v", err)
	}
	kept := map[string]any{}
	seen := map[string]bool{}
	for region, v := range regions {
		list, _ := v.([]any)
		var cur []any
		for _, e := range list {
			m, _ := e.(map[string]any)
			for code, info := range m {
				im, _ := info.(map[string]any)
				if _, ended := im["_to"]; ended || im["_tender"] == "false" {
					continue
				}
				cur = append(cur, map[string]any{code: pick(im, "_from")})
				seen[code] = true
			}
		}
		if len(cur) > 0 {
			kept[region] = cur
		}
	}
	out := map[string]any{"supplemental": map[string]any{"currencyData": map[string]any{"fractions": fractions, "region": kept}}}
	if err := writeJSON(filepath.Join(dir, currencyDataFile), out); err != nil {
		return nil, err
	}
	var current []string
	for c := range seen {
		current = append(current, c)
	}
	sort.Strings(current)
	return current, nil
}

// rootNumbers is the part of root.xml fetchRootSystems reads.
type rootNumbers struct {
	Numbers struct {
		Symbols []struct {
			System      string    `xml:"numberSystem,attr"`
			Alias       *struct{} `xml:"alias"`
			Decimal     string    `xml:"decimal"`
			Group       string    `xml:"group"`
			PercentSign string    `xml:"percentSign"`
			PlusSign    string    `xml:"plusSign"`
			MinusSign   string    `xml:"minusSign"`
			Exponential string    `xml:"exponential"`
			Infinity    string    `xml:"infinity"`
			NaN         string    `xml:"nan"`
		} `xml:"symbols"`
		Decimal  []rootFormats `xml:"decimalFormats"`
		Percent  []rootFormats `xml:"percentFormats"`
		Currency []rootFormats `xml:"currencyFormats"`
	} `xml:"numbers"`
}

// rootFormats is one *Formats element of root.xml: an alias, or lengths
// of formats of patterns.
type rootFormats struct {
	System  string    `xml:"numberSystem,attr"`
	Alias   *struct{} `xml:"alias"`
	Spacing *struct {
		Alias *struct{} `xml:"alias"`
	} `xml:"currencySpacing"`
	Lengths []struct {
		XMLName xml.Name
		Type    string `xml:"type,attr"`
		Formats []struct {
			XMLName  xml.Name
			Type     string    `xml:"type,attr"`
			Alias    *struct{} `xml:"alias"`
			Patterns []struct {
				Alt  string `xml:"alt,attr"`
				Text string `xml:",chardata"`
			} `xml:"pattern"`
		} `xml:",any"`
	} `xml:",any"`
}

// standard returns the pattern of the format type (""/"standard"/
// "accounting") of the default length; "" for an alias.
func (f rootFormats) standard(typ string) (string, error) {
	if f.Alias != nil {
		return "", nil
	}
	for _, l := range f.Lengths {
		if l.Type != "" || !strings.HasSuffix(l.XMLName.Local, "FormatLength") {
			continue
		}
		for _, fm := range l.Formats {
			if fm.Type != typ && !(typ == "standard" && fm.Type == "") {
				continue
			}
			if fm.Alias != nil {
				return "", errAliasToStandard
			}
			for _, p := range fm.Patterns {
				if p.Alt == "" {
					return p.Text, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no %s pattern", typ)
}

var errAliasToStandard = fmt.Errorf("alias to the standard format")

// fetchRootSystems adds to the root locale ("und") the numbering systems
// root.xml has symbols for besides latn (arab and arabext in CLDR 48),
// which cldr-json leaves out: ICU uses them for a tag that asks for such
// a system in a locale without data for it (en-u-nu-arab). An alias of
// root.xml to the Latin data of the locale is an empty field.
func fetchRootSystems(formats map[string]numberDigest, tag string) error {
	b, err := get(fmt.Sprintf(rawRoot, tag))
	if err != nil {
		return err
	}
	var root rootNumbers
	if err := xml.Unmarshal(b, &root); err != nil {
		return fmt.Errorf("root.xml: %v", err)
	}
	und := formats["und"]
	find := func(list []rootFormats, sys string) (rootFormats, error) {
		for _, f := range list {
			if f.System == sys {
				return f, nil
			}
		}
		return rootFormats{}, fmt.Errorf("root.xml: no formats for %s", sys)
	}
	for _, s := range root.Numbers.Symbols {
		if s.Alias != nil || s.System == "latn" {
			continue
		}
		sd := numberSystemDigest{Decimal: s.Decimal, Group: s.Group, MinusSign: s.MinusSign, PlusSign: s.PlusSign,
			PercentSign: s.PercentSign, Exponential: s.Exponential, Infinity: s.Infinity, NaN: s.NaN}
		var err error
		for _, f := range []struct {
			dst  *string
			list []rootFormats
			typ  string
		}{
			{&sd.DecimalFormat, root.Numbers.Decimal, "standard"},
			{&sd.PercentFormat, root.Numbers.Percent, "standard"},
			{&sd.CurrencyFormat, root.Numbers.Currency, "standard"},
		} {
			fm, err := find(f.list, s.System)
			if err != nil {
				return err
			}
			if *f.dst, err = fm.standard(f.typ); err != nil {
				return fmt.Errorf("root.xml %s: %v", s.System, err)
			}
		}
		cur, _ := find(root.Numbers.Currency, s.System)
		if sd.AccountingFormat, err = cur.standard("accounting"); err == errAliasToStandard {
			sd.AccountingFormat, err = sd.CurrencyFormat, nil
		}
		if err != nil {
			return fmt.Errorf("root.xml %s: %v", s.System, err)
		}
		if cur.Alias == nil && (cur.Spacing == nil || cur.Spacing.Alias == nil) {
			return fmt.Errorf("root.xml %s: currencySpacing is not an alias to latn", s.System)
		}
		und.Systems[s.System] = sd
	}
	formats["und"] = und
	return nil
}

// ---------------------------------------------------------------------
// Generation.

// groupOf encodes the plural forms of a text: the "other" form, then
// "\x1f" and the category's letter (z, o, t, f, m; 0 and 1 for CLDR's
// explicit counts) for each other form that differs from it.
func groupOf(forms map[string]string) string {
	var b strings.Builder
	b.WriteString(forms["other"])
	for _, c := range append(pluralCategories[:5:5], "0", "1") {
		if s, ok := forms[c]; ok && s != forms["other"] {
			b.WriteString("\x1f")
			b.WriteByte(c[0])
			b.WriteString(s)
		}
	}
	return b.String()
}

// compactText encodes compact patterns: the groups of the magnitudes from
// 10^3, joined by "\x1e" ("" for a magnitude without patterns); no
// patterns is "".
func compactText(m []map[string]string) (string, error) {
	parts := make([]string, len(m))
	for i, forms := range m {
		if forms == nil {
			continue
		}
		if forms["other"] == "" {
			return "", fmt.Errorf("no pattern for 10^%d", i+3)
		}
		parts[i] = groupOf(forms)
	}
	return strings.Join(parts, "\x1e"), nil
}

// chainOf returns the names the runtime looks a CLDR locale up under
// (localeChain in numbering.go), or nil when its own name is not among
// them (a variant, as ca-ES-valencia).
func chainOf(name string) []string {
	parts := strings.Split(name, "-")
	base := parts[0]
	var script, region string
	rest := parts[1:]
	if len(rest) > 0 && len(rest[0]) == 4 {
		script, rest = rest[0], rest[1:]
	}
	if len(rest) > 0 && (len(rest[0]) == 2 || len(rest[0]) == 3) {
		region, rest = rest[0], rest[1:]
	}
	if len(rest) > 0 {
		return nil
	}
	var chain []string
	if script != "" && region != "" {
		chain = append(chain, base+"-"+script+"-"+region)
	}
	if script != "" {
		chain = append(chain, base+"-"+script)
	}
	if region != "" {
		chain = append(chain, base+"-"+region)
	}
	return append(chain, base)
}

// setChain returns a generated locale and its ancestors by truncation
// among the generated locales (es-MX, es; zh-Hant-HK, zh-Hant, zh).
func setChain(name string, set map[string]bool) []string {
	chain := []string{name}
	for p := parentOf(name); p != ""; p = parentOf(p) {
		if set[p] {
			chain = append(chain, p)
		}
	}
	return chain
}

// generateCurrencies returns currencies_gen.go.
func generateCurrencies(dir string) ([]byte, error) {
	var cd struct {
		Supplemental struct {
			CurrencyData struct {
				Fractions map[string]map[string]string `json:"fractions"`
				Region    map[string][]map[string]any  `json:"region"`
			} `json:"currencyData"`
		} `json:"supplemental"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, currencyDataFile))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &cd); err != nil {
		return nil, fmt.Errorf("%s: %v", currencyDataFile, err)
	}
	fr := cd.Supplemental.CurrencyData.Fractions
	def, ok := fr["DEFAULT"]
	if !ok || def["_digits"] != "2" {
		return nil, fmt.Errorf("%s: DEFAULT fractions are not 2 digits", currencyDataFile)
	}
	var digits strings.Builder
	for _, code := range sortedKeysOf(fr) {
		f := fr[code]
		if code == "DEFAULT" {
			continue
		}
		if f["_rounding"] != "0" {
			return nil, fmt.Errorf("%s: %s rounds to increments of %s; the runtime has no rounding increments", currencyDataFile, code, f["_rounding"])
		}
		d, err := strconv.Atoi(f["_digits"])
		if err != nil || d < 0 || d > 9 || len(code) != 3 {
			return nil, fmt.Errorf("%s: %s has digits %q", currencyDataFile, code, f["_digits"])
		}
		if d != 2 {
			fmt.Fprintf(&digits, "%s%d", code, d)
		}
	}
	var regions strings.Builder
	for _, region := range sortedKeysOf(cd.Supplemental.CurrencyData.Region) {
		list := cd.Supplemental.CurrencyData.Region[region]
		if len(region) != 2 || len(list) == 0 || len(list[0]) != 1 {
			continue // 001, 150...: no country
		}
		for code := range list[0] {
			regions.WriteString(region + code)
		}
	}

	raw, err = os.ReadFile(filepath.Join(dir, currencySymbolsFile))
	if err != nil {
		return nil, err
	}
	var diffs map[string]map[string][2]string
	if err := json.Unmarshal(raw, &diffs); err != nil {
		return nil, fmt.Errorf("%s: %v", currencySymbolsFile, err)
	}
	raw, err = os.ReadFile(filepath.Join(dir, defaultSystemsFile))
	if err != nil {
		return nil, err
	}
	var all map[string]string
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("%s: %v", defaultSystemsFile, err)
	}
	locs := []string{"und"}
	for l := range all {
		if l != "und" {
			locs = append(locs, l)
		}
	}
	full, err := undoSymbolDiffs(diffs, locs)
	if err != nil {
		return nil, err
	}
	// The rows: a locale's symbols that differ from what the runtime finds
	// under the rest of its names (chainOf) and the root.
	rows := map[string]map[string][2]string{"und": full["und"]}
	for _, loc := range parentsFirst(full) {
		chain := chainOf(loc)
		if loc == "und" || len(chain) == 0 || chain[0] != loc {
			continue
		}
		d := map[string][2]string{}
		for code, s := range full[loc] {
			inherited := rows["und"][code]
			for _, n := range chain[1:] {
				if v, ok := rows[n][code]; ok {
					inherited = v
					break
				}
			}
			if s != inherited {
				d[code] = s
			}
		}
		if len(d) > 0 {
			rows[loc] = d
		}
	}

	var b bytes.Buffer
	writeHeader(&b)
	b.WriteString("// currencyDigits lists the currencies whose minor unit is not 2 digits: the\n")
	b.WriteString("// code and the digits (CLDR's supplemental currencyData fractions), sorted.\n")
	fmt.Fprintf(&b, "const currencyDigits = %q\n\n", digits.String())
	b.WriteString("// regionCurrencies lists the current currency of each region: the region's\n")
	b.WriteString("// code and the currency's, sorted (the first current tender of currencyData).\n")
	b.WriteString("const regionCurrencies = \"\" +\n")
	rs := regions.String()
	for i := 0; i < len(rs); i += 80 {
		end := min(i+80, len(rs))
		sep := " +"
		if end == len(rs) {
			sep = ""
		}
		fmt.Fprintf(&b, "\t%q%s\n", rs[i:end], sep)
	}
	b.WriteString("\n// currencySymbols has, per locale, sorted, the symbols of the current\n")
	b.WriteString("// currencies that differ from what the locale's other names (localeChain) and\n")
	b.WriteString("// the root resolve to: entries \"\\x1e<code>\\x1f<symbol>[\\x1f<narrow>]\" (no\n")
	b.WriteString("// narrow symbol: the same as the symbol).\n")
	b.WriteString("var currencySymbols = [...]localeText{\n")
	for _, loc := range sortedKeysOf(rows) {
		var t strings.Builder
		for _, code := range sortedKeysOf(rows[loc]) {
			s := rows[loc][code]
			t.WriteString("\x1e" + code + "\x1f" + s[0])
			if s[1] != s[0] {
				t.WriteString("\x1f" + s[1])
			}
		}
		fmt.Fprintf(&b, "\t{%q, %q},\n", loc, t.String())
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// generateLocaleNumbers returns localenumbers_gen.go.
func generateLocaleNumbers(dir string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, localeNumbersFile))
	if err != nil {
		return nil, err
	}
	var diffs map[string]localeNumbers
	if err := json.Unmarshal(raw, &diffs); err != nil {
		return nil, fmt.Errorf("%s: %v", localeNumbersFile, err)
	}
	set := map[string]bool{}
	for _, l := range locales {
		if _, ok := diffs[l]; !ok {
			return nil, fmt.Errorf("%s: no data for %s", localeNumbersFile, l)
		}
		set[l] = true
	}
	names := append([]string(nil), locales...)
	sort.Slice(names, func(i, j int) bool {
		di, dj := strings.Count(names[i], "-"), strings.Count(names[j], "-")
		if di != dj {
			return di < dj
		}
		return names[i] < names[j]
	})
	data := undoLocaleNumbersDiffs(diffs, names, set)

	type compactRow struct{ short, long, currency string }
	compact := map[string]compactRow{} // "loc/sys"
	type nameRow struct {
		unit  string
		names map[string]string // code -> group
	}
	nameRows := map[string]nameRow{}
	for _, loc := range names {
		d := data[loc]
		chain := setChain(loc, set)
		// Compact: each kind of a system's patterns that differs from what
		// the runtime finds without it (the system's kind under the
		// ancestors, else latn's under the whole chain); a kind "" looks
		// further.
		resolve := func(sys string, kind func(compactRow) string) string {
			for _, s := range []string{sys, "latn"} {
				for _, n := range chain {
					if n == loc && s == sys {
						continue
					}
					if r, ok := compact[n+"/"+s]; ok && kind(r) != "" {
						return kind(r)
					}
				}
			}
			return ""
		}
		kinds := []func(*compactRow) *string{
			func(r *compactRow) *string { return &r.short },
			func(r *compactRow) *string { return &r.long },
			func(r *compactRow) *string { return &r.currency },
		}
		// latn first: the other systems fall back to it.
		systems := sortedKeysOf(d.Compact)
		sort.SliceStable(systems, func(i, j int) bool { return systems[i] == "latn" && systems[j] != "latn" })
		for _, sys := range systems {
			cd := d.Compact[sys]
			var r compactRow
			for i, m := range [][]map[string]string{cd.Short, cd.Long, cd.Currency} {
				dst := kinds[i](&r)
				if *dst, err = compactText(m); err != nil {
					return nil, fmt.Errorf("%s %s compact: %v", loc, sys, err)
				}
				if *dst == resolve(sys, func(r compactRow) string { return *kinds[i](&r) }) {
					*dst = ""
				}
			}
			if r != (compactRow{}) {
				compact[loc+"/"+sys] = r
			}
		}
		// Names: the unit pattern and the names that differ from the
		// ancestors'.
		row := nameRow{names: map[string]string{}}
		unit := groupOf(d.UnitPattern)
		inheritedUnit := ""
		for _, n := range chain[1:] {
			if r, ok := nameRows[n]; ok && r.unit != "" {
				inheritedUnit = r.unit
				break
			}
		}
		if unit != inheritedUnit {
			row.unit = unit
		}
		for code, forms := range d.Currencies {
			g := groupOf(forms)
			inherited := ""
			for _, n := range chain[1:] {
				if v, ok := nameRows[n].names[code]; ok {
					inherited = v
					break
				}
			}
			if g != inherited {
				row.names[code] = g
			}
		}
		nameRows[loc] = row
	}

	var b bytes.Buffer
	writeHeader(&b)
	b.WriteString("// compactPatterns has the compact patterns of the generated locales (dateTags)\n")
	b.WriteString("// by numbering system, keyed \"locale/system\", sorted, where they differ from\n")
	b.WriteString("// what the locale's ancestors among them (es-MX, es) resolve to; a system\n")
	b.WriteString("// without a row has the Latin patterns. Each text has the 12 magnitudes, 10^3\n")
	b.WriteString("// to 10^14, joined by \\x1e, each the pattern of the \"other\" plural form and,\n")
	b.WriteString("// after \\x1f and the category's letter (z, o, t, f, m), of the forms that differ.\n")
	b.WriteString("// The fields are the short and long decimal patterns and the short currency\n")
	b.WriteString("// patterns.\n")
	b.WriteString("var compactPatterns = [...]compactRow{\n")
	for _, k := range sortedKeysOf(compact) {
		r := compact[k]
		fmt.Fprintf(&b, "\t{%q, %q, %q, %q},\n", k, r.short, r.long, r.currency)
	}
	b.WriteString("}\n\n")
	b.WriteString("// currencyNames has, per generated locale, sorted, the pattern of a number and\n")
	b.WriteString("// a currency's name (\"{0} {1}\"; \"\": the ancestor's) and the names of the\n")
	b.WriteString("// current currencies, \"\\x1e<code>\\x1f<names>\", plural forms as in\n")
	b.WriteString("// compactPatterns, where they differ from the ancestors'. The names are\n")
	b.WriteString("// compressed (compress/flate, a third of their size), and inflated when the\n")
	b.WriteString("// locale first writes one.\n")
	b.WriteString("var currencyNames = [...]currencyNameRow{\n")
	for _, loc := range sortedKeysOf(nameRows) {
		r := nameRows[loc]
		var t strings.Builder
		for _, code := range sortedKeysOf(r.names) {
			t.WriteString("\x1e" + code + "\x1f" + r.names[code])
		}
		z, err := deflate(t.String())
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "\t{%q, %q, %q},\n", loc, r.unit, z)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// deflate compresses a text with compress/flate ("" stays "").
func deflate(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	var b bytes.Buffer
	w, err := flate.NewWriter(&b, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write([]byte(s)); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func sortedKeysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
