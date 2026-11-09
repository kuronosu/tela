package i18n

import (
	"testing"
	"time"
)

// icuNumbers are the outputs of ICU 78.3 with CLDR 48 (Node 26's Intl) for
// new Intl.NumberFormat(locale, {maximumFractionDigits: 3}).format(1234567.891)
// and for the style "percent" with 0.256: the numbering system and symbols
// a locale writes by default (ADR-251). golang.org/x/text has CLDR 32,
// whose defaults lack ar-EG, ar-SA, ckb, sd...
var icuNumbers = []struct{ loc, number, percent string }{
	{"ar-EG", "١٬٢٣٤٬٥٦٧٫٨٩١", "٢٦٪\u061c"},
	{"ar-SA", "١٬٢٣٤٬٥٦٧٫٨٩١", "٢٦٪\u061c"},
	{"ar-IQ", "١٬٢٣٤٬٥٦٧٫٨٩١", "٢٦٪\u061c"},
	{"ar-AE", "1,234,567.891", "26\u200e%\u200e"},
	{"ar-MA", "1.234.567,891", "26\u200e%\u200e"},
	{"ar-DZ", "1.234.567,891", "26\u200e%\u200e"},
	{"fa", "۱٬۲۳۴٬۵۶۷٫۸۹۱", "۲۶٪"},
	{"fa-AF", "۱٬۲۳۴٬۵۶۷٫۸۹۱", "۲۶٪"},
	{"ur-IN", "۱٬۲۳۴٬۵۶۷٫۸۹۱", "۲۶%"},
	{"ur", "1,234,567.891", "26%"},
	{"ps", "۱٬۲۳۴٬۵۶۷٫۸۹۱", "۲۶٪"},
	{"mr", "१२,३४,५६७.८९१", "२६%"},
	{"bn", "১২,৩৪,৫৬৭.৮৯১", "২৬%"},
	{"as", "১২,৩৪,৫৬৭.৮৯১", "২৬%"},
	{"my", "၁,၂၃၄,၅၆၇.၈၉၁", "၂၆%"},
	{"dz", "༡༢,༣༤,༥༦༧.༨༩༡", "༢༦ %"},
	{"en", "1,234,567.891", "26%"},
	{"es", "1.234.567,891", "26 %"},
	{"hi", "12,34,567.891", "26%"},
}

func TestNumberingSystemsMatchICU(t *testing.T) {
	for _, c := range icuNumbers {
		f := For(c.loc)
		if got := f.Number(1234567.891); got != c.number {
			t.Errorf("For(%q).Number(1234567.891) = %q, want ICU's %q", c.loc, got, c.number)
		}
		if got := f.Number(0.256, Percent()); got != c.percent {
			t.Errorf("For(%q).Number(0.256, Percent()) = %q, want ICU's %q", c.loc, got, c.percent)
		}
	}
}

// ar-EG and the other Mashriq regions write Arabic-Indic digits, the
// Maghreb and the Gulf states that CLDR 48 lists write Latin ones, and the
// dates follow the numbers. Bare "ar" writes Latin digits, as in CLDR 48
// (ADR-258).
func TestArabicRegionsDigits(t *testing.T) {
	day := time.Date(2026, 11, 8, 15, 4, 5, 0, time.UTC)
	for _, c := range []struct {
		loc, number, date string
	}{
		{"ar", "1,234.5", "08\u200f/11\u200f/2026"},
		{"ar-EG", "١٬٢٣٤٫٥", "٠٨\u200f/١١\u200f/٢٠٢٦"},
		{"ar-SA", "١٬٢٣٤٫٥", "٠٨\u200f/١١\u200f/٢٠٢٦"},
		{"ar-AE", "1,234.5", "08\u200f/11\u200f/2026"},
		{"ar-MA", "1.234,5", "08\u200f/11\u200f/2026"},
		// The tag's own numbering system wins.
		{"ar-EG-u-nu-latn", "1,234.5", "08\u200f/11\u200f/2026"},
		{"en-u-nu-arab", "١٬٢٣٤٫٥", "Nov ٨, ٢٠٢٦"},
		// A script with a region, and a language that writes Latin.
		{"az-Arab-IR", "۱٬۲۳۴٫۵", ""},
		{"es-MX", "1,234.5", ""},
	} {
		f := For(c.loc)
		if got := f.Number(1234.5); got != c.number {
			t.Errorf("For(%q).Number(1234.5) = %q, want %q", c.loc, got, c.number)
		}
		if c.date != "" {
			if got := f.Date(day, Medium); got != c.date {
				t.Errorf("For(%q).Date = %q, want %q", c.loc, got, c.date)
			}
		}
	}
}

// A system golang.org/x/text does not know is written by replacing the
// ASCII digits of its output.
func TestNumberingSystemUnknownToXText(t *testing.T) {
	if got, want := For("hnj").Number(1234.5), "𞅁,𞅂𞅃𞅄.𞅅"; got != want {
		t.Errorf("For(hnj).Number(1234.5) = %q, want %q", got, want)
	}
}

// Numbers inside messages use the locale's digits too.
func TestNumberingInMessages(t *testing.T) {
	isolate(t)
	mustLoad(t, map[string]string{"ar-EG.json": `{"n": "{count, plural, other {# ملفات}}"}`, "ar-AE.json": `{"n": "{count, plural, other {# ملفات}}"}`})
	if got, want := For("ar-EG").T("n", "count", 1234), "١٬٢٣٤ ملفات"; got != want {
		t.Errorf("ar-EG: %q, want %q", got, want)
	}
	if got, want := For("ar-AE").T("n", "count", 1234), "1,234 ملفات"; got != want {
		t.Errorf("ar-AE: %q, want %q", got, want)
	}
}
