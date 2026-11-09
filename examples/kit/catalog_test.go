package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/i18n"
)

// trCall finds the calls tr("id", "English text", ...) of the gallery's
// sources: the stages' texts, which the catalogs translate.
var trCall = regexp.MustCompile(`\b(?:tr|name)\("([^"]+)",\s*"((?:[^"\\]|\\.)*)"`)

// placeholders lists the {names} of a message, sorted.
func placeholders(s string) string {
	var names []string
	for _, m := range regexp.MustCompile(`\{(\w+)\}`).FindAllStringSubmatch(s, -1) {
		names = append(names, m[1])
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// TestCatalogsHaveEveryText reads the sources of the gallery: every text a
// stage writes with tr has a Spanish and an Arabic message under
// "gallery.<id>" with the same {placeholders}, no catalog has a message that
// no code uses, and one id never has two English texts.
func TestCatalogsHaveEveryText(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	english := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range trCall.FindAllStringSubmatch(string(src), -1) {
			id, en := m[1], strings.ReplaceAll(m[2], `\"`, `"`)
			if prev, ok := english[id]; ok && prev != en {
				t.Errorf("%s: the id %q has two English texts: %q and %q", f, id, prev, en)
			}
			english[id] = en
		}
	}
	if len(english) < 200 {
		t.Fatalf("found only %d texts written with tr", len(english))
	}
	for _, tag := range []string{"es", "ar"} {
		data, err := locales.ReadFile("locales/" + tag + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var cat struct {
			Gallery map[string]string `json:"gallery"`
		}
		if err := json.Unmarshal(data, &cat); err != nil {
			t.Fatal(err)
		}
		for id, en := range english {
			msg, ok := cat.Gallery[id]
			switch {
			case !ok || msg == "":
				t.Errorf("%s.json has no \"gallery\" message %q (English: %q)", tag, id, en)
			case placeholders(msg) != placeholders(en):
				t.Errorf("%s.json %q has the placeholders {%s}, the English text has {%s}", tag, id, placeholders(msg), placeholders(en))
			}
		}
		for id := range cat.Gallery {
			if _, ok := english[id]; !ok {
				t.Errorf("%s.json has the \"gallery\" message %q, which no tr call uses", tag, id)
			}
		}
	}
}

// TestBuiltinTextsAreTranslated: the catalogs answer every label Tela
// shows itself (tela.Labels) in Spanish and Arabic, with the same
// {placeholders}; English is Tela's own default.
func TestBuiltinTextsAreTranslated(t *testing.T) {
	// The ones that Spanish spells as English does.
	same := map[string]bool{"tela.menu.zoom": true, "tela.messagebox.error": true, "tela.messagebox.no": true}
	for _, tag := range []string{"es", "ar"} {
		f := i18n.For(tag)
		for id, en := range tela.Labels() {
			got := f.Label(id, "name", "Notes")
			if got == strings.ReplaceAll(en, "{name}", "Notes") && !same[id] {
				t.Errorf("%s: the label %q is still the English %q: add it to the catalog's \"tela\" messages", tag, id, en)
			}
		}
	}
}
