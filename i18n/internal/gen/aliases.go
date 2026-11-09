package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/language"
)

// scriptAliases (ADR-266) replaces a table that was copied by hand from
// ICU: ICU's data has no language-region locale for a language that CLDR
// splits by script, so its converter aliases pa_PK to pa_Arab_PK and
// zh_TW to zh_Hant_TW, and a lookup of a tag without a script must do the
// same to find the data ICU finds. Which IDs get an alias is ICU's choice
// (its converter's config.xml lists them, and az_IR is not one), and the
// script is CLDR's likely subtags. Both are vendored, so a CLDR or ICU
// update that changes either changes the generated table.

// languageRegion matches an ICU locale ID made of a language and a region.
var languageRegion = regexp.MustCompile(`^[a-z]{2,3}_([A-Z]{2}|[0-9]{3})$`)

// fetchAliasSources vendors the language-region IDs of ICU's converter
// configuration and the CLDR likely subtags of them and of their
// languages.
func fetchAliasSources(dir string, src numberingSources, cldrLocales map[string]string) error {
	url := fmt.Sprintf(rawICUConfig, src.icu)
	b, err := get(url)
	if err != nil {
		return err
	}
	ids, err := icuLocaleIDs(string(b))
	if err != nil {
		return fmt.Errorf("%s: %v", url, err)
	}
	if err := writeJSON(filepath.Join(dir, icuLocalesFile), map[string]any{"source": url, "localeIds": ids}); err != nil {
		return err
	}
	b, err = get(fmt.Sprintf(rawLikely, src.cldrJSON))
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("likelySubtags.json: %v", err)
	}
	all, err := dig(doc, "supplemental", "likelySubtags")
	if err != nil {
		return fmt.Errorf("likelySubtags.json: %v", err)
	}
	kept := map[string]any{}
	for _, id := range ids {
		lang, _, _ := strings.Cut(id, "-")
		for _, k := range []string{id, lang} {
			if v, ok := all[k]; ok {
				kept[k] = v
			}
		}
	}
	return writeJSON(filepath.Join(dir, likelySubtagsFile), map[string]any{"supplemental": map[string]any{"likelySubtags": kept}})
}

// icuLocaleIDs returns the language-region IDs of the main <localeIds>
// list of the converter's config.xml (the first one, before any
// <directory>), in BCP 47 form ("pa-PK").
func icuLocaleIDs(config string) ([]string, error) {
	start := strings.Index(config, "<localeIds>")
	end := strings.Index(config, "</localeIds>")
	if dirAt := strings.Index(config, "<directory"); start < 0 || end < start || dirAt >= 0 && dirAt < start {
		return nil, fmt.Errorf("no main <localeIds> list")
	}
	var ids []string
	for _, line := range strings.Split(config[start+len("<localeIds>"):end], "\n") {
		line, _, _ = strings.Cut(line, "//")
		for _, f := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\r' }) {
			if languageRegion.MatchString(f) {
				ids = append(ids, strings.ReplaceAll(f, "_", "-"))
			}
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no language-region IDs in <localeIds>")
	}
	sort.Strings(ids)
	return ids, nil
}

// scriptAliases computes the table from the vendored files: each
// language-region ID of ICU's list that is not a CLDR locale (cldrLocales,
// the keys of defaultNumberingSystems.json) and whose subtags are current
// (ICU's list also has deprecated ones, such as in_ID and sh_BA, which
// tags never reach: golang.org/x/text canonicalizes them), with the script
// of its likely subtags, else its language's.
func scriptAliases(dir string, cldrLocales map[string]string) (map[string]string, error) {
	var icu struct {
		LocaleIDs []string `json:"localeIds"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, icuLocalesFile))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &icu); err != nil {
		return nil, fmt.Errorf("%s: %v", icuLocalesFile, err)
	}
	var likely struct {
		Supplemental struct {
			LikelySubtags map[string]string `json:"likelySubtags"`
		} `json:"supplemental"`
	}
	raw, err = os.ReadFile(filepath.Join(dir, likelySubtagsFile))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &likely); err != nil {
		return nil, fmt.Errorf("%s: %v", likelySubtagsFile, err)
	}
	out := map[string]string{}
	for _, id := range icu.LocaleIDs {
		if _, ok := cldrLocales[id]; ok {
			continue
		}
		t, err := language.Parse(id)
		if err != nil {
			return nil, fmt.Errorf("%s: %q: %v", icuLocalesFile, id, err)
		}
		if t.String() != id {
			continue // deprecated subtags
		}
		lang, _, _ := strings.Cut(id, "-")
		full, ok := likely.Supplemental.LikelySubtags[id]
		if !ok {
			full, ok = likely.Supplemental.LikelySubtags[lang]
		}
		if !ok {
			return nil, fmt.Errorf("%s: no likely subtags for %s or %s", likelySubtagsFile, id, lang)
		}
		parts := strings.Split(full, "-")
		if len(parts) != 3 || len(parts[1]) != 4 {
			return nil, fmt.Errorf("%s: likely subtags of %s are %q, not language-script-region", likelySubtagsFile, id, full)
		}
		out[id] = parts[1]
	}
	return out, nil
}
