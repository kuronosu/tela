package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestVerifyLanguage: a package with two languages is verified in the
// other one, and a label that only the Spanish text overflows is found
// there.
func TestVerifyLanguage(t *testing.T) {
	skipShort(t)
	var out, errOut bytes.Buffer
	code := run([]string{"verify", "-json", "./testdata/checki18n"}, &out, &errOut)
	var rep checkReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || code != 1 {
		t.Fatalf("exit %d: %v\n%s\n%s", code, err, out.String(), errOut.String())
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Rule != "text-clipped" || strings.Join(rep.Findings[0].Variants, ",") != "lang es" {
		t.Errorf("findings %+v", rep.Findings)
	}
	if rep.Variants[len(rep.Variants)-1] != "lang es" || len(rep.Skipped) != 0 {
		t.Errorf("variants %v, skipped %v", rep.Variants, rep.Skipped)
	}
	// --lang names another locale; the screen fits in English.
	out.Reset()
	if code := run([]string{"verify", "--lang", "en", "./testdata/checki18n"}, &out, &errOut); code != 0 {
		t.Errorf("--lang en: exit %d\n%s", code, out.String())
	}
}
