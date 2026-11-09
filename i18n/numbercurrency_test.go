package i18n

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
)

// intlEdges are Node 26.12's Intl.NumberFormat outputs (ICU 78.3, CLDR 48)
// for the corners of the notations, currencies and signs, in the specs of
// intlOptions.
var intlEdges = []struct {
	loc, spec string
	v         float64
	want      string
}{
	// Compact: a rounding that reaches the next magnitude takes its
	// pattern, numbers below the first pattern (or a "0" pattern, ja and
	// de at 10^3) are written in full with two-digit minimum grouping, the
	// largest pattern goes on (en's T, ja's 京 at 10^16).
	{"en", "compact", 999, "999"},
	{"en", "compact", 999999, "1M"},
	{"en", "compact", 9999.6, "10K"},
	{"en", "compact", 0, "0"},
	{"en", "compact", 0.0123, "0.012"},
	{"en", "compact", 1e20, "100,000,000T"},
	{"ja", "compact", 1234, "1234"},
	{"ja", "compact", 1e16, "1京"},
	{"de", "compact", 1234, "1234"},
	// CLDR's explicit "1" pattern, without a number (fr, it), for exactly
	// one once rounded; ps has long patterns only from 10^9 (ICU's root has
	// none); zh-Hant-HK's own long "0K" replaces zh-Hant's "0".
	{"fr", "compact=long", 1000, "mille"},
	{"fr", "compact=long", 1000.4, "mille"},
	{"fr", "compact=long", -1000, "-1 millier"},
	{"fr", "compact=long", 2000000, "2 millions"},
	{"it", "compact=long", 1000, "mille"},
	{"ps", "compact=long", 1000, "۱۰۰۰"},
	{"ps", "compact=long", 1.2e9, "۱٫۲G"},
	{"zh-Hant-HK", "compact=long", 1234, "1.2K"},
	// An explicit negative pattern (sw), Indian magnitudes, plural forms.
	{"sw", "compact", -1234, "elfu\u00a0-1.2"},
	{"hi", "compact", 1234567, "12\u00a0लाख"},
	{"en-IN", "compact", 1234567, "12L"},
	{"pl", "compact=long", 2000, "2 tysiące"},
	{"pl", "compact=long", 5000, "5 tysięcy"},
	{"en", "compact maxfrac=2", 1234567, "1.23M"},
	{"en", "compact currency=EUR display=name", 1234567, "1.2M euros"},
	{"en", "compact currency=USD accounting", -5, "($5)"},
	{"en", "compact currency=USD accounting", -1234, "-$1.2K"},
	{"en", "compact currency=BHD", 1234567, "BHD\u00a01.2M"},
	{"en", "compact percent", 12345, "1.2M%"},
	// Scientific and engineering notation.
	{"en", "engineering", 999999.9, "1E6"},
	{"en", "engineering", 0.00123, "1.23E-3"},
	{"en", "scientific", 0, "0E0"},
	{"en", "scientific", -999999, "-1E6"},
	{"en", "scientific percent", 12.5, "1E3%"},
	{"en", "scientific currency=USD", 12346, "$1.235E4"},
	{"fa", "scientific", 0.00123, "۱٫۲۳×۱۰^\u200e−۳"},
	{"tok", "engineering", 123456, "1\u00a023,456E3"},
	// NaN and infinities.
	{"en", "currency=USD", math.NaN(), "$NaN"},
	{"en", "currency=USD display=name", math.NaN(), "NaN US dollars"},
	{"en", "sign=always", math.NaN(), "+NaN"},
	{"en", "sign=always", math.Inf(-1), "-∞"},
	{"en", "compact", math.Inf(-1), "-∞"},
	{"en", "sign=never", math.Inf(-1), "∞"},
	// Currencies: no data (the code), digits, regional symbols, explicit
	// negative patterns (de-CH, nl), currency separators (de-AT groups
	// amounts with "."), bidi marks, spacing next to letters.
	{"en", "currency=XYZ", 1234.56, "XYZ\u00a01,234.56"},
	{"en", "currency=XYZ display=name", 1234.56, "1,234.56 XYZ"},
	{"en", "currency=BHD", 1234.56, "BHD\u00a01,234.560"},
	{"en", "currency=CAD", 1234.56, "CA$1,234.56"},
	{"en-CA", "currency=CAD", 1234.56, "$1,234.56"},
	{"es-PE", "currency=PEN", 1234.56, "S/\u00a01,234.56"},
	{"es-419", "currency=USD", 1234.56, "USD\u00a01,234.56"},
	{"es-MX", "currency=MXN", 1234.56, "$1,234.56"},
	{"de-CH", "currency=CHF sign=always", 1234.56, "CHF+1'234.56"},
	{"de-CH", "currency=CHF", -1234.56, "CHF-1'234.56"},
	{"nl", "currency=EUR", -1234.56, "€\u00a0-1.234,56"},
	{"nl", "currency=EUR accounting", -1234.56, "(€\u00a01.234,56)"},
	{"de-AT", "currency=EUR", -1234.56, "-€\u00a01.234,56"},
	{"de-AT", "currency=USD display=name", -1234.56, "-1.234,56 US-Dollar"},
	{"he", "currency=ILS", -1234.56, "\u200f\u200e-1,234.56\u00a0\u200f₪"},
	{"fa", "currency=IRR", -1234.56, "\u200e−\u200eریال\u00a0۱٬۲۳۵"},
	{"pl", "currency=PLN display=name", 2, "2,00 złotego polskiego"},
	{"pl", "currency=PLN display=name maxfrac=0", 2, "2 złote polskie"},
	{"ar", "currency=USD display=name maxfrac=0", 11, "11 دولار أمريكي"},
	{"si", "currency=EUR display=name", 3, "යුරෝ3.00"},
	{"en", "currency=USD minfrac=0", 1234, "$1,234"},
	{"en", "currency=USD minfrac=3", 1234, "$1,234.000"},
	{"en", "currency=USD sign=exceptZero", 0, "$0.00"},
	{"tr", "percent sign=always", 0.25, "+%25"},
	// The shortest digits of a float64 are what is rounded (2.675 is
	// 2.67499999... in binary).
	{"en", "maxfrac=20", 0.1, "0.1"},
	{"en", "maxfrac=2", 2.675, "2.68"},
	{"en", "", 1e21, "1,000,000,000,000,000,000,000"},
}

func TestNumberEdgesMatchIntl(t *testing.T) {
	for _, c := range intlEdges {
		if got := For(c.loc).Number(c.v, intlOptions(t, c.spec)...); got != c.want {
			t.Errorf("For(%q).Number(%v, %s) = %q, want Intl's %q", c.loc, c.v, c.spec, got, c.want)
		}
	}
}

// intlNumbering are Intl's outputs (Node 26.12, ICU 78.3) for tags that
// ask for a numbering system with -u-nu-: -1234567.891 with 3 fraction
// digits, 0.25 as a percentage and -1234.56 in USD. A system without data
// in the locale has the root's symbols (arab, arabext) or the locale's
// Latin ones, with its own digits (Tela's tables, not golang.org/x/text);
// an unknown or algorithmic one (roman) is the locale's default.
var intlNumbering = []struct{ loc, number, percent, currency string }{
	{"fr-u-nu-thai", "-๑\u202f๒๓๔\u202f๕๖๗,๘๙๑", "๒๕\u00a0%", "-๑\u202f๒๓๔,๕๖\u00a0$US"},
	{"de-u-nu-arab", "\u061c-١٬٢٣٤٬٥٦٧٫٨٩١", "٢٥٪\u061c", "\u061c-١٬٢٣٤٫٥٦\u00a0$"},
	{"de-u-nu-arabext", "\u200e-\u200e۱٬۲۳۴٬۵۶۷٫۸۹۱", "۲۵\u00a0٪", "\u200e-\u200e۱٬۲۳۴٫۵۶\u00a0$"},
	{"es-u-nu-deva", "-१.२३४.५६७,८९१", "२५\u00a0%", "-१२३४,५६\u00a0US$"},
	{"es-u-nu-beng", "-১.২৩৪.৫৬৭,৮৯১", "২৫\u00a0%", "-১২৩৪,৫৬\u00a0US$"},
	{"en-u-nu-tamldec", "-௧,௨௩௪,௫௬௭.௮௯௧", "௨௫%", "-$௧,௨௩௪.௫௬"},
	{"ja-u-nu-roman", "-1,234,567.891", "25%", "-$1,234.56"},
	{"en-u-nu-foo", "-1,234,567.891", "25%", "-$1,234.56"},
	{"ar-EG-u-nu-foo", "\u061c-١٬٢٣٤٬٥٦٧٫٨٩١", "٢٥٪\u061c", "\u061c-\u200f١٬٢٣٤٫٥٦\u00a0US$"},
	{"hi-u-nu-latn", "-12,34,567.891", "25%", "-$1,234.56"},
	{"fa-u-nu-latn", "\u200e−1,234,567.891", "25%", "\u200e−\u200e$\u00a01,234.56"},
	{"ar-u-nu-arab", "\u061c-١٬٢٣٤٬٥٦٧٫٨٩١", "٢٥٪\u061c", "\u061c-\u200f١٬٢٣٤٫٥٦\u00a0US$"},
}

func TestExplicitNumberingMatchesIntl(t *testing.T) {
	for _, c := range intlNumbering {
		f := For(c.loc)
		if got := f.Number(-1234567.891); got != c.number {
			t.Errorf("For(%q).Number(-1234567.891) = %q, want Intl's %q", c.loc, got, c.number)
		}
		if got := f.Number(0.25, Percent()); got != c.percent {
			t.Errorf("For(%q).Number(0.25, Percent()) = %q, want Intl's %q", c.loc, got, c.percent)
		}
		if got := f.Number(-1234.56, Currency("USD")); got != c.currency {
			t.Errorf("For(%q).Number(-1234.56, Currency(USD)) = %q, want Intl's %q", c.loc, got, c.currency)
		}
	}
}

// Ties go to the even digit (ICU4C's default, and Tela's before; Intl
// rounds them away from zero: 2.5 is "3" there).
func TestNumberRoundsHalfToEven(t *testing.T) {
	en := For("en")
	for _, c := range []struct {
		got, want string
	}{
		{en.Number(2.5, MaxFractionDigits(0)), "2"},
		{en.Number(3.5, MaxFractionDigits(0)), "4"},
		{en.Number(-2.5, MaxFractionDigits(0)), "-2"},
		{en.Number(1234.5, Currency("JPY")), "¥1,234"},
		{en.Number(1250, Notation(NotationCompact)), "1.2K"},
		{en.Number(1350, Notation(NotationCompact)), "1.4K"},
		{en.Number(0.125, MaxFractionDigits(2)), "0.12"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestCurrencyCode(t *testing.T) {
	if got, want := For("en").Number(5, Currency("eur")), "€5.00"; got != want {
		t.Errorf("Currency(\"eur\"): %q, want %q", got, want)
	}
	for _, code := range []string{"", "EURO", "E1R", "€"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Currency(%q) did not panic", code)
				}
			}()
			Currency(code)
		}()
	}
	// The last style wins.
	en := For("en")
	if got := en.Number(0.5, Currency("USD"), Percent()); got != "50%" {
		t.Errorf("Currency then Percent: %q", got)
	}
	if got := en.Number(0.5, Percent(), Currency("USD")); got != "$0.50" {
		t.Errorf("Percent then Currency: %q", got)
	}
}

// Every CLDR locale's currency symbols (the vendored differences, rebuilt
// along their truncation chain) are what the runtime finds for it.
func TestCurrencySymbolsMatchVendoredCLDR(t *testing.T) {
	raw, err := os.ReadFile("internal/gen/cldr/currencySymbols.json")
	if err != nil {
		t.Fatal(err)
	}
	var diffs map[string]map[string][2]string
	if err := json.Unmarshal(raw, &diffs); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("internal/gen/cldr/defaultNumberingSystems.json")
	if err != nil {
		t.Fatal(err)
	}
	var all map[string]string
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	locs := make([]string, 0, len(all))
	for l := range all {
		locs = append(locs, l)
	}
	sort.Slice(locs, func(i, j int) bool { return strings.Count(locs[i], "-") < strings.Count(locs[j], "-") })
	full := map[string]map[string][2]string{"und": diffs["und"]}
	for _, loc := range locs {
		if loc == "und" {
			continue
		}
		parent := "und"
		for p := parentName(loc); p != ""; p = parentName(p) {
			if _, ok := all[p]; ok {
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
	checked := 0
	for _, loc := range locs {
		if loc == "und" || strings.Count(loc, "-") > 2 || len(chainNames(loc)) == 0 {
			continue // variants: not reachable by localeChain
		}
		d, err := forTag(loc)
		if err != nil {
			t.Errorf("%s: %v", loc, err)
			continue
		}
		for code, want := range full[loc] {
			sym, narrow := d.currencyText(code, CurrencySymbol), d.currencyText(code, CurrencyNarrowSymbol)
			if sym != want[0] || narrow != want[1] {
				t.Errorf("%s %s: symbols %q, %q; CLDR has %q, %q", loc, code, sym, narrow, want[0], want[1])
			}
			checked++
		}
	}
	if checked < 100000 {
		t.Errorf("only %d (locale, currency) pairs checked", checked)
	}
}

// chainNames returns localeChain's names for a CLDR locale name, or nil
// when the name is not the first of them.
func chainNames(loc string) []string {
	tag, err := parseTag(loc)
	if err != nil {
		return nil
	}
	chain := localeChain(tag)
	if chain[0] != loc {
		return nil
	}
	return chain
}

// The lazily built tables (inflated names, compact tables, numbering
// systems of skeletons) are safe from several goroutines.
func TestNumberTablesConcurrently(t *testing.T) {
	locs := []string{"en", "es", "de", "ru", "ar-EG", "ja", "hi", "pt-BR", "zh-Hant-HK", "sw"}
	want := make([]string, len(locs))
	format := func(loc string) string {
		f := For(loc)
		return f.Number(2, Currency("EUR"), CurrencyDisplay(CurrencyName)) + "|" +
			f.Number(1234567, Notation(NotationCompact), CompactDisplay(CompactLong)) + "|" +
			f.loc().formatNumber(1234.5, numberConfig{minFrac: -1, maxFrac: -1, numbering: "thai"})
	}
	for i, loc := range locs {
		want[i] = format(loc)
	}
	inflatedNames = [len(currencyNames)]struct {
		once sync.Once
		text string
	}{}
	compactTables, numberLocales = sync.Map{}, sync.Map{}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i, loc := range locs {
				if got := format(loc); got != want[i] {
					t.Errorf("%s: %q, want %q", loc, got, want[i])
				}
			}
		}()
	}
	wg.Wait()
}

func TestRegionCurrency(t *testing.T) {
	for loc, want := range map[string]string{
		"en": "USD", "es": "EUR", "es-MX": "MXN", "ja": "JPY", "hi": "INR", "pt-BR": "BRL", "pt": "BRL",
		"ar": "EGP", "ar-SA": "SAR", "zh-TW": "TWD", "de-CH": "CHF", "es-u-cu-usd": "USD", "en-001": "XXX",
	} {
		if got := localeCurrency(For(loc).loc().tag); got != want {
			t.Errorf("localeCurrency(%s) = %s, want %s", loc, got, want)
		}
	}
	if got := currencyDigitsOf("JPY") + currencyDigitsOf("BHD")*10 + currencyDigitsOf("EUR")*100; got != 230 {
		t.Errorf("currency digits JPY, BHD, EUR: %d", got)
	}
}
