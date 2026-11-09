package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// The numbering systems (ADR-251, ADR-266). golang.org/x/text formats
// numbers with CLDR 32, whose default numbering system per locale is out
// of date (ar-EG gives Latin digits). The generator reads the default
// numbering system of every CLDR locale and emits numbering_gen.go: the
// locales whose default differs from their parent's, the digits of every
// numeric system (a tag may ask for any with -u-nu-), and the script ICU
// gives the language-region locales it aliases (scriptAliases).

const (
	rawAvailable  = "https://raw.githubusercontent.com/unicode-org/cldr-json/%s/cldr-json/cldr-core/availableLocales.json"
	rawSystems    = "https://raw.githubusercontent.com/unicode-org/cldr-json/%s/cldr-json/cldr-core/supplemental/numberingSystems.json"
	rawLikely     = "https://raw.githubusercontent.com/unicode-org/cldr-json/%s/cldr-json/cldr-core/supplemental/likelySubtags.json"
	rawCurrencies = "https://raw.githubusercontent.com/unicode-org/cldr-json/%s/cldr-json/cldr-core/supplemental/currencyData.json"
	// rawRoot is CLDR's root.xml, for the arab and arabext symbols that
	// cldr-json leaves out of its root locale.
	rawRoot = "https://raw.githubusercontent.com/unicode-org/cldr/%s/common/main/root.xml"
	// rawICUConfig is the configuration of ICU's cldr-to-icu converter,
	// whose locale IDs say which language-region locales ICU aliases to a
	// script.
	rawICUConfig = "https://raw.githubusercontent.com/unicode-org/icu/%s/tools/cldr/cldr-to-icu/config.xml"

	// defaultSystemsFile is the vendored digest of main.<locale>.numbers.
	// defaultNumberingSystem for every locale of cldr-numbers-full, relative
	// to the cldr directory: {"<locale>": "<system>"}.
	defaultSystemsFile = "defaultNumberingSystems.json"
	// numberingSystemsFile is cldr-core's supplemental/numberingSystems.json
	// under its original path, kept for the numeric systems.
	numberingSystemsFile = "supplemental/numberingSystems.json"
	// likelySubtagsFile is cldr-core's supplemental/likelySubtags.json under
	// its original path, kept for the keys scriptAliases reads.
	likelySubtagsFile = "supplemental/likelySubtags.json"
	// icuLocalesFile has the locale IDs of ICU's converter configuration:
	// {"source": url, "localeIds": [...]}.
	icuLocalesFile = "icuLocaleIds.json"
)

// numberingSources are the tags -fetch-numbering reads.
type numberingSources struct {
	cldrJSON string // cldr-json tag ("48.0.0")
	cldr     string // cldr tag, for root.xml ("release-48")
	icu      string // icu tag, for the converter's config.xml ("release-78.1")
}

// fetchNumbering vendors the number data of every CLDR locale: the
// default numbering system, the symbols and standard patterns
// (numberFormats.json), the currency symbols (currencySymbols.json), and
// for the generated locales the compact patterns and the currency names
// (localeNumbers.json); plus the digits of the numeric systems, the
// currency digits and the regions' currencies, and what scriptAliases is
// made from.
func fetchNumbering(dir string, src numberingSources) error {
	b, err := get(fmt.Sprintf(rawAvailable, src.cldrJSON))
	if err != nil {
		return err
	}
	var avail struct {
		AvailableLocales struct {
			Full []string `json:"full"`
		} `json:"availableLocales"`
	}
	if err := json.Unmarshal(b, &avail); err != nil {
		return fmt.Errorf("availableLocales.json: %v", err)
	}
	locs := avail.AvailableLocales.Full

	current, err := fetchCurrencyData(dir, src.cldrJSON)
	if err != nil {
		return err
	}
	generated := map[string]bool{}
	for _, l := range locales {
		generated[l] = true
	}

	type result struct {
		loc, system string
		formats     numberDigest
		symbols     map[string][2]string
		local       *localeNumbers
		err         error
	}
	results := make([]result, len(locs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i, loc := range locs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := result{loc: loc}
			defer func() { results[i] = r }()
			doc, err := fetchJSON(src.cldrJSON, "cldr-numbers-full", loc, "numbers.json")
			if err != nil {
				r.err = err
				return
			}
			nums, err := dig(doc, "main", loc, "numbers")
			if err != nil {
				r.err = err
				return
			}
			r.system, _ = nums["defaultNumberingSystem"].(string)
			if r.system == "" {
				r.err = fmt.Errorf("%s numbers.json: no defaultNumberingSystem", loc)
				return
			}
			if r.formats, err = digestNumbers(nums); err != nil {
				r.err = fmt.Errorf("%s numbers.json: %v", loc, err)
				return
			}
			doc, err = fetchJSON(src.cldrJSON, "cldr-numbers-full", loc, "currencies.json")
			if err != nil {
				r.err = err
				return
			}
			curs, err := dig(doc, "main", loc, "numbers", "currencies")
			if err != nil {
				r.err = fmt.Errorf("%s currencies.json: %v", loc, err)
				return
			}
			r.symbols = currencySymbolsOf(curs, current)
			if generated[loc] || loc == "und" {
				ln, err := digestLocaleNumbers(nums, curs, current)
				if err != nil {
					r.err = fmt.Errorf("%s: %v", loc, err)
					return
				}
				r.local = &ln
			}
		}()
	}
	wg.Wait()
	defaults := map[string]string{}
	formats := map[string]numberDigest{}
	symbols := map[string]map[string][2]string{}
	local := map[string]localeNumbers{}
	for _, r := range results {
		if r.err != nil {
			return r.err
		}
		defaults[r.loc] = r.system
		formats[r.loc] = r.formats
		symbols[r.loc] = r.symbols
		if r.local != nil {
			local[r.loc] = *r.local
		}
	}
	root, ok := local["und"]
	if !ok {
		return fmt.Errorf("no root locale (und) in availableLocales.json")
	}
	delete(local, "und")
	raw, err := json.Marshal(local) // the data before dropRootLong, for the ancestors
	if err != nil {
		return err
	}
	var before map[string]localeNumbers
	if err := json.Unmarshal(raw, &before); err != nil {
		return err
	}
	set := map[string]bool{}
	for _, l := range locales {
		set[l] = true
	}
	for _, l := range locales {
		ln, ok := local[l]
		if !ok {
			return fmt.Errorf("generated locale %q is not a CLDR locale", l)
		}
		var parent *localeNumbers
		if chain := setChain(l, set); len(chain) > 1 {
			p := before[chain[1]]
			parent = &p
		}
		dropRootLong(&ln, root.Compact["latn"].Long, parent)
		local[l] = ln
	}
	if err := fetchRootSystems(formats, src.cldr); err != nil {
		return err
	}
	if err := writeLines(filepath.Join(dir, numberFormatsFile), formats); err != nil {
		return err
	}
	if err := writeLines(filepath.Join(dir, currencySymbolsFile), diffSymbols(symbols)); err != nil {
		return err
	}
	if err := writeLines(filepath.Join(dir, localeNumbersFile), diffLocaleNumbers(local)); err != nil {
		return err
	}
	out, err := json.MarshalIndent(defaults, "", " ") // keys sorted
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, defaultSystemsFile), append(out, '\n'), 0o644); err != nil {
		return err
	}
	if err := fetchSystems(dir, src.cldrJSON); err != nil {
		return err
	}
	if err := fetchAliasSources(dir, src, defaults); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "fetched", defaultSystemsFile+",", numberFormatsFile+",", currencySymbolsFile+",",
		localeNumbersFile+",", numberingSystemsFile+",", currencyDataFile+",", likelySubtagsFile, "and", icuLocalesFile)
	return nil
}

// fetchSystems vendors the numeric systems of supplemental
// numberingSystems.json (algorithmic ones, such as roman, are left out:
// the runtime writes only digits).
func fetchSystems(dir, version string) error {
	b, err := get(fmt.Sprintf(rawSystems, version))
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("numberingSystems.json: %v", err)
	}
	all, err := dig(doc, "supplemental", "numberingSystems")
	if err != nil {
		return fmt.Errorf("numberingSystems.json: %v", err)
	}
	kept := map[string]any{}
	for name, v := range all {
		sys, ok := v.(map[string]any)
		if !ok || sys["_type"] != "numeric" || name == "latn" {
			continue
		}
		kept[name] = pick(sys, "_type", "_digits")
	}
	return writeJSON(filepath.Join(dir, numberingSystemsFile), map[string]any{"supplemental": map[string]any{"numberingSystems": kept}})
}

// writeJSON writes v indented, creating the directory.
func writeJSON(path string, v any) error {
	out, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// parentOf returns the name a locale inherits from in the chain above:
// "" for a language.
func parentOf(name string) string {
	parts := strings.Split(name, "-")
	if len(parts) == 1 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "-")
}

// generateNumbering returns numbering_gen.go.
func generateNumbering(dir string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, defaultSystemsFile))
	if err != nil {
		return nil, err
	}
	var defaults map[string]string
	if err := json.Unmarshal(raw, &defaults); err != nil {
		return nil, fmt.Errorf("%s: %v", defaultSystemsFile, err)
	}
	// Parents first, so a locale is compared with what its parent resolves
	// to once the table is built.
	names := make([]string, 0, len(defaults))
	for loc := range defaults {
		names = append(names, loc)
	}
	sort.Slice(names, func(i, j int) bool {
		di, dj := strings.Count(names[i], "-"), strings.Count(names[j], "-")
		if di != dj {
			return di < dj
		}
		return names[i] < names[j]
	})
	table := map[string]string{}
	resolve := func(name string) string {
		for n := parentOf(name); ; n = parentOf(n) {
			if s, ok := table[n]; ok {
				return s
			}
			if n == "" {
				return "latn"
			}
		}
	}
	for _, loc := range names {
		if defaults[loc] != resolve(loc) {
			table[loc] = defaults[loc]
		}
	}

	var sysDoc struct {
		Supplemental struct {
			NumberingSystems map[string]struct {
				Type   string `json:"_type"`
				Digits string `json:"_digits"`
			} `json:"numberingSystems"`
		} `json:"supplemental"`
	}
	raw, err = os.ReadFile(filepath.Join(dir, numberingSystemsFile))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &sysDoc); err != nil {
		return nil, fmt.Errorf("%s: %v", numberingSystemsFile, err)
	}
	digits := map[string]string{}
	for s, d := range sysDoc.Supplemental.NumberingSystems {
		if d.Type != "numeric" || len([]rune(d.Digits)) != 10 {
			return nil, fmt.Errorf("numbering system %q: no ten digits in %s", s, numberingSystemsFile)
		}
		digits[s] = d.Digits
	}
	// Every default system and every system a locale has symbols for
	// must have digits.
	raw, err = os.ReadFile(filepath.Join(dir, numberFormatsFile))
	if err != nil {
		return nil, err
	}
	var formats map[string]numberDigest
	if err := json.Unmarshal(raw, &formats); err != nil {
		return nil, fmt.Errorf("%s: %v", numberFormatsFile, err)
	}
	for loc, f := range formats {
		for s := range f.Systems {
			if _, ok := digits[s]; !ok && s != "latn" {
				return nil, fmt.Errorf("%s has symbols for %q, which %s does not list", loc, s, numberingSystemsFile)
			}
		}
	}
	for loc, s := range table {
		if _, ok := digits[s]; !ok && s != "latn" {
			return nil, fmt.Errorf("%s defaults to %q, which %s does not list", loc, s, numberingSystemsFile)
		}
	}
	aliases, err := scriptAliases(dir, defaults)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	writeHeader(&b)
	b.WriteString("// numberingDefaults are the locales whose default numbering system differs from\n")
	b.WriteString("// their parent's (language-script-region, language-script, language; Latin at\n")
	b.WriteString("// the root).\n")
	b.WriteString("var numberingDefaults = map[string]string{\n")
	for _, k := range sortedKeys(table) {
		fmt.Fprintf(&b, "\t%q: %q,\n", k, table[k])
	}
	b.WriteString("}\n\n")
	b.WriteString("// numberingDigits are the ten digits, 0 to 9, of CLDR's numeric numbering\n")
	b.WriteString("// systems (other than latn).\n")
	b.WriteString("var numberingDigits = map[string]string{\n")
	for _, k := range sortedKeys(digits) {
		fmt.Fprintf(&b, "\t%q: %q,\n", k, digits[k])
	}
	b.WriteString("}\n\n")
	b.WriteString("// scriptAliases are the language-region locales that ICU's data aliases to\n")
	b.WriteString("// their likely script, as pa_PK to pa_Arab_PK and zh_TW to zh_Hant_TW: the\n")
	b.WriteString("// language-region IDs of the cldr-to-icu converter's config.xml that have no\n")
	b.WriteString("// CLDR data of their own and keep their subtags, with the script of their\n")
	b.WriteString("// CLDR likely subtags (golang.org/x/text's are CLDR 32's, where sd-IN is still\n")
	b.WriteString("// Arabic). Other tags are not given a script: ICU reads az-IR as az (Latin\n")
	b.WriteString("// digits), not az-Arab-IR.\n")
	b.WriteString("var scriptAliases = map[string]string{\n")
	for _, k := range sortedKeys(aliases) {
		fmt.Fprintf(&b, "\t%q: %q,\n", k, aliases[k])
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
