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

// The numbering systems (ADR-251). golang.org/x/text formats numbers with
// CLDR 32, whose default numbering system per locale is out of date (ar-EG
// gives Latin digits). The generator
// reads the default numbering system of every CLDR locale and emits
// numbering_gen.go: the locales whose default differs from their parent's,
// and the digits of the systems they name.

const (
	rawAvailable = "https://raw.githubusercontent.com/unicode-org/cldr-json/%s/cldr-json/cldr-core/availableLocales.json"
	rawSystems   = "https://raw.githubusercontent.com/unicode-org/cldr-json/%s/cldr-json/cldr-core/supplemental/numberingSystems.json"
	rawNumbers   = "https://raw.githubusercontent.com/unicode-org/cldr-json/%s/cldr-json/cldr-numbers-full/main/%s/numbers.json"

	// defaultSystemsFile is the vendored digest of main.<locale>.numbers.
	// defaultNumberingSystem for every locale of cldr-numbers-full, relative
	// to the cldr directory: {"<locale>": "<system>"}.
	defaultSystemsFile = "defaultNumberingSystems.json"
	// numberingSystemsFile is cldr-core's supplemental/numberingSystems.json
	// under its original path, kept for the systems some locale defaults to.
	numberingSystemsFile = "supplemental/numberingSystems.json"
)

// fetchNumbering vendors the default numbering system of every CLDR locale
// and the digits of the systems that are some locale's default.
func fetchNumbering(dir, version string) error {
	b, err := get(fmt.Sprintf(rawAvailable, version))
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
	type result struct {
		loc, system string
		formats     numberDigest
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
			doc, err := fetchJSON(version, "cldr-numbers-full", loc, "numbers.json")
			if err != nil {
				results[i] = result{loc: loc, err: err}
				return
			}
			nums, err := dig(doc, "main", loc, "numbers")
			if err != nil {
				results[i] = result{loc: loc, err: err}
				return
			}
			s, _ := nums["defaultNumberingSystem"].(string)
			if s == "" {
				results[i] = result{loc: loc, err: fmt.Errorf("%s numbers.json: no defaultNumberingSystem", loc)}
				return
			}
			nd, err := digestNumbers(nums)
			if err != nil {
				results[i] = result{loc: loc, err: fmt.Errorf("%s numbers.json: %v", loc, err)}
				return
			}
			results[i] = result{loc: loc, system: s, formats: nd}
		}()
	}
	wg.Wait()
	defaults := map[string]string{}
	formats := map[string]numberDigest{}
	used := map[string]bool{}
	for _, r := range results {
		if r.err != nil {
			return r.err
		}
		defaults[r.loc] = r.system
		formats[r.loc] = r.formats
		used[r.system] = true
	}
	if err := writeNumberFormats(filepath.Join(dir, numberFormatsFile), formats); err != nil {
		return err
	}
	out, err := json.MarshalIndent(defaults, "", " ") // keys sorted
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, defaultSystemsFile), append(out, '\n'), 0o644); err != nil {
		return err
	}

	b, err = get(fmt.Sprintf(rawSystems, version))
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
	for name := range used {
		if name == "latn" {
			continue
		}
		sys, ok := all[name].(map[string]any)
		if !ok {
			return fmt.Errorf("numberingSystems.json: no system %q", name)
		}
		kept[name] = pick(sys, "_type", "_digits")
	}
	out, err = json.MarshalIndent(map[string]any{"supplemental": map[string]any{"numberingSystems": kept}}, "", " ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, numberingSystemsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "fetched", defaultSystemsFile+",", numberFormatsFile, "and", numberingSystemsFile)
	return nil
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
	for _, s := range table {
		if s == "latn" {
			continue
		}
		d, ok := sysDoc.Supplemental.NumberingSystems[s]
		if !ok || d.Type != "numeric" || len([]rune(d.Digits)) != 10 {
			return nil, fmt.Errorf("numbering system %q: no ten digits in %s", s, numberingSystemsFile)
		}
		digits[s] = d.Digits
	}

	var b bytes.Buffer
	b.WriteString("// Code generated by internal/gen from the Unicode CLDR JSON data in\n")
	b.WriteString("// internal/gen/cldr; DO NOT EDIT.\n")
	b.WriteString("// Data files copyright (c) 1991-2025 Unicode, Inc. (Unicode License v3,\n")
	b.WriteString("// see internal/gen/cldr/LICENSE and THIRD_PARTY.md).\n\n")
	b.WriteString("package i18n\n\n")
	b.WriteString("// numberingDefaults are the locales whose default numbering system differs from\n")
	b.WriteString("// their parent's (language-script-region, language-script, language; Latin at\n")
	b.WriteString("// the root).\n")
	b.WriteString("var numberingDefaults = map[string]string{\n")
	keys := sortedKeys(table)
	for _, k := range keys {
		fmt.Fprintf(&b, "\t%q: %q,\n", k, table[k])
	}
	b.WriteString("}\n\n")
	b.WriteString("// numberingDigits are the ten digits, 0 to 9, of the numbering systems\n")
	b.WriteString("// numberingDefaults names (other than latn).\n")
	b.WriteString("var numberingDigits = map[string]string{\n")
	for _, k := range sortedKeys(digits) {
		fmt.Fprintf(&b, "\t%q: %q,\n", k, digits[k])
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
