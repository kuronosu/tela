package i18n

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// The expected strings come from the CLDR 48 symbols and patterns vendored
// in internal/gen/cldr/numberFormats.json (what ICU 78 formats with), not
// from golang.org/x/text, whose CLDR 32 data differs for some of them: sd
// writes "." as its Latin-script decimal separator, ckb puts a no-break
// space before the percent sign, ne groups 12,34,567, es and pl do not
// group four digits, fr groups with a narrow no-break space.
func TestNumberSymbolsCLDR48(t *testing.T) {
	for _, c := range []struct {
		loc                   string
		small, large, percent string // -1234.5, -1234567.891, -0.256 as a percentage
	}{
		{"fa", "\u200e−۱٬۲۳۴٫۵", "\u200e−۱٬۲۳۴٬۵۶۷٫۸۹۱", "\u200e−۲۶٪"},
		{"ur", "\u200e-1,234.5", "\u200e-1,234,567.891", "\u200e-26%"},
		{"ur-IN", "\u200e-\u200e۱٬۲۳۴٫۵", "\u200e-\u200e۱٬۲۳۴٬۵۶۷٫۸۹۱", "\u200e-\u200e۲۶%"},
		{"ne", "-१,२३४.५", "-१२,३४,५६७.८९१", "-२६%"},
		{"ckb", "\u200f-١٬٢٣٤٫٥", "\u200f-١٬٢٣٤٬٥٦٧٫٨٩١", "\u200f-٢٦ ٪"},
		{"sd", "\u061c-١٬٢٣٤.٥", "\u061c-١٬٢٣٤٬٥٦٧.٨٩١", "\u061c-٢٦٪\u061c"},
		{"ar-EG", "\u061c-١٬٢٣٤٫٥", "\u061c-١٬٢٣٤٬٥٦٧٫٨٩١", "\u061c-٢٦٪\u061c"},
		{"ar", "\u200e-1,234.5", "\u200e-1,234,567.891", "\u200e-26\u200e%\u200e"},
		{"sv", "−1 234,5", "−1 234 567,891", "−26 %"},
		{"tr", "-1.234,5", "-1.234.567,891", "-%26"},
		{"hi", "-1,234.5", "-12,34,567.891", "-26%"},
		{"en-IN", "-1,234.5", "-12,34,567.891", "-26%"},
		{"fr", "-1 234,5", "-1 234 567,891", "-26 %"},
		{"es", "-1234,5", "-1.234.567,891", "-26 %"},
		{"pl", "-1234,5", "-1 234 567,891", "-26%"},
		{"de-CH", "-1'234.5", "-1'234'567.891", "-26%"},
		{"en", "-1,234.5", "-1,234,567.891", "-26%"},
		// A tag's own numbering system keeps the locale's symbols.
		{"fa-u-nu-latn", "\u200e−1,234.5", "\u200e−1,234,567.891", "\u200e−26%"},
		{"ar-EG-u-nu-latn", "\u200e-1,234.5", "\u200e-1,234,567.891", "\u200e-26\u200e%\u200e"},
	} {
		f := For(c.loc)
		if got := f.Number(-1234.5); got != c.small {
			t.Errorf("For(%q).Number(-1234.5) = %q, want %q", c.loc, got, c.small)
		}
		if got := f.Number(-1234567.891); got != c.large {
			t.Errorf("For(%q).Number(-1234567.891) = %q, want %q", c.loc, got, c.large)
		}
		if got := f.Number(-0.256, Percent()); got != c.percent {
			t.Errorf("For(%q).Number(-0.256, Percent()) = %q, want %q", c.loc, got, c.percent)
		}
	}
}

// The first separator needs minimumGroupingDigits digits ahead of the last
// group (Spanish, Polish and Portuguese in Portugal group from five digits).
func TestNumberMinimumGrouping(t *testing.T) {
	for _, c := range []struct {
		loc  string
		v    float64
		want string
	}{
		{"es", 999, "999"},
		{"es", 1000, "1000"},
		{"es", 9999, "9999"},
		{"es", 10000, "10.000"},
		{"pt-PT", 1000, "1000"},
		{"pt-PT", 10000, "10 000"},
		{"pl", 10000, "10 000"},
		{"pt-BR", 1000, "1.000"},
		{"es-MX", 1000, "1,000"},
		{"hi", 1000, "1,000"},
		{"hi", 100000, "1,00,000"},
		{"hi", 10000000, "1,00,00,000"},
		{"ne", 100000, "१,००,०००"},
	} {
		if got := For(c.loc).Number(c.v); got != c.want {
			t.Errorf("For(%q).Number(%v) = %q, want %q", c.loc, c.v, got, c.want)
		}
	}
}

func TestNumberOptionsAndSpecialValues(t *testing.T) {
	fa, en := For("fa"), For("en")
	for _, c := range []struct{ got, want string }{
		{fa.Number(math.NaN()), "ناعدد"},
		{fa.Number(math.Inf(1)), "∞"},
		{fa.Number(math.Inf(-1)), "\u200e−∞"},
		{en.Number(math.NaN()), "NaN"},
		{en.Number(1234567.891, NoGrouping()), "1234567.891"},
		{fa.Number(1234567.891, NoGrouping()), "۱۲۳۴۵۶۷٫۸۹۱"},
		{fa.Number(-1234.5, MinFractionDigits(2), MaxFractionDigits(2)), "\u200e−۱٬۲۳۴٫۵۰"},
		{fa.Number(0.256, Percent(), MaxFractionDigits(1)), "۲۵٫۶٪"},
		{For("ne").Number(1234567, NoGrouping()), "१२३४५६७"},
		{For("tr").Number(0.5, Percent()), "%50"},
		{en.Number(0.5), "0.5"},
		{en.Number(-0.0004), "-0"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

// Numbers inside messages use the symbols too.
func TestNumberSymbolsInMessages(t *testing.T) {
	isolate(t)
	mustLoad(t, map[string]string{"ne.json": `{"n": "{count, number} / {pct, number, percent}"}`})
	if got, want := For("ne").T("n", "count", 1234567, "pct", 0.5), "१२,३४,५६७ / ५०%"; got != want {
		t.Errorf("ne: %q, want %q", got, want)
	}
}

// numberDigests is the vendored CLDR data: the generated table must answer
// for every CLDR locale what the locale's own data says.
func TestNumberTableMatchesVendoredCLDR(t *testing.T) {
	raw, err := os.ReadFile("internal/gen/cldr/numberFormats.json")
	if err != nil {
		t.Fatal(err)
	}
	var digests map[string]struct {
		Systems map[string]struct {
			Decimal     string `json:"decimal"`
			Group       string `json:"group"`
			MinusSign   string `json:"minusSign"`
			PercentSign string `json:"percentSign"`
			Infinity    string `json:"infinity"`
			NaN         string `json:"nan"`
		} `json:"systems"`
	}
	if err := json.Unmarshal(raw, &digests); err != nil {
		t.Fatal(err)
	}
	if len(digests) < 500 {
		t.Fatalf("only %d locales in numberFormats.json", len(digests))
	}
	checked := 0
	for loc, d := range digests {
		if loc == "und" {
			continue
		}
		tag, err := parseTag(loc)
		if err != nil {
			t.Errorf("CLDR locale %q: %v", loc, err)
			continue
		}
		for sys, want := range d.Systems {
			got, found := symbolsFor(tag, sys)
			if !found || got.decimal != want.Decimal || got.group != want.Group || got.minus != want.MinusSign ||
				got.infinity != want.Infinity || got.nan != want.NaN {
				t.Errorf("%s/%s: table has %+v (found %v), CLDR has %+v", loc, sys, got, found, want)
			}
			checked++
		}
	}
	if checked < 800 {
		t.Errorf("only %d (locale, system) pairs checked", checked)
	}
}
