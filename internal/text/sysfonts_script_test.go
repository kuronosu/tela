package text

import (
	"strings"
	"testing"

	"github.com/go-text/typesetting/language"
)

func runeRange(from, to rune) string {
	var b strings.Builder
	for r := from; r <= to; r++ {
		b.WriteRune(r)
	}
	return b.String()
}

// TestSystemOneFamilyPerScript is the regression test of a script drawn by
// two families in one word: a narrow font that covers a few characters of
// the script (a symbol or extras font) took the clusters it has, the real
// family of the script took the others. A script is drawn by the family
// that supports it, whatever the cluster.
func TestSystemOneFamilyPerScript(t *testing.T) {
	thai := runeRange(0x0E01, 0x0E3A) + runeRange(0x0E40, 0x0E5B)
	latin := "abcdefghijklmnopqrstuvwxyz"
	entries := []sysEntry{
		// A sans with 10 Thai characters and few characters overall: the
		// narrowest family that covers "ก".
		entry("notosansextras", StyleNormal, 400, runeRange(0x0E01, 0x0E0A)+latin[:10]),
		entry("notosansthai", StyleNormal, 400, thai+latin),
		// The looped design covers more characters: a wider family.
		entry("notosansthailooped", StyleNormal, 400, thai+latin+runeRange(0x0E81, 0x0E8A)+"0123456789ABCDEF"),
	}
	s := synthetic("", entries...)
	if got := s.scriptFamilies(language.Thai); !equalStrings(got, []string{"notosansthai", "notosansthailooped"}) {
		t.Fatalf("Thai families %v, want the two that support the script, narrowest first", got)
	}
	for _, cluster := range []string{"ก", "ข", "ฯ", "กั", "๏"} {
		got := s.fallbackFamilies([]rune(cluster), language.Thai)
		if len(got) == 0 || got[0] != "notosansthai" {
			t.Errorf("cluster %q: %v, want notosansthai first", cluster, got)
		}
	}
	// A character only the wide family covers still gets it.
	if got := s.fallbackFamilies([]rune("ກ"), language.Thai); len(got) != 1 || got[0] != "notosansthailooped" {
		t.Errorf("Lao-range character: %v", got)
	}

	// The order the clusters come in does not matter.
	order := func(clusters ...string) []string {
		s := synthetic("", entries...)
		var out []string
		for _, c := range clusters {
			out = append(out, s.fallbackFamilies([]rune(c), language.Thai)[0])
		}
		return out
	}
	a := order("ก", "ฯ", "ข")
	b := order("ฯ", "ข", "ก")
	want := []string{"notosansthai", "notosansthai", "notosansthai"}
	if !equalStrings(a, want) || !equalStrings(b, want) {
		t.Errorf("first family per cluster depends on the order: %v and %v, want %v", a, b, want)
	}
}

// TestSystemScriptOfRunDecidesCommon checks that a cluster with no script of
// its own (punctuation inside Thai text) follows the family of the run's
// script, as the layout passes it.
func TestSystemScriptOfRunDecidesCommon(t *testing.T) {
	thai := runeRange(0x0E01, 0x0E3A) + runeRange(0x0E40, 0x0E5B)
	s := synthetic("",
		// Both have the guillemet; the narrow one is no Thai font.
		entry("notosansquotes", StyleNormal, 400, "«»"),
		entry("notosansthai", StyleNormal, 400, thai+"«»"+runeRange('a', 'z')),
	)
	if got := s.fallbackFamilies([]rune("«"), language.Thai); got[0] != "notosansthai" {
		t.Errorf("Common cluster in a Thai run: %v, want notosansthai first", got)
	}
	if got := s.fallbackFamilies([]rune("«"), language.Common); got[0] != "notosansquotes" {
		t.Errorf("Common cluster in a Common run: %v, want the narrowest first", got)
	}
}
