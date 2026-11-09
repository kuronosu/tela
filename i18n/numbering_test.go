package i18n

import (
	"math"
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

// icuAliases are ICU 78.3's outputs (Node 26's Intl, CLDR 48) for
// new Intl.NumberFormat(locale, {maximumFractionDigits: 3}).format(-1234567.891)
// and for NaN, for the language-region locales that ICU's data aliases to
// their likely script (pa_PK to pa_Arab_PK, zh_TW to zh_Hant_TW), for two
// that it does not (az-IR, ha-SD: the language's data), and for tags that
// ask for a system no locale uses by default (its digits).
var icuAliases = []struct{ loc, number, nan string }{
	{"pa-PK", "\u200e-\u200e۱٬۲۳۴٬۵۶۷٫۸۹۱", "NaN"},  // arabext
	{"pa-IN", "-12,34,567.891", "NaN"},              // latn
	{"uz-AF", "\u200e-\u200e۱٬۲۳۴٬۵۶۷٫۸۹۱", "NaN"},  // arabext
	{"uz-UZ", "-1 234 567,891", "son emas"},         // latn
	{"zh-TW-u-nu-hanidec", "-一,二三四,五六七.八九一", "非數值"}, // hanidec
	{"zh-TW", "-1,234,567.891", "非數值"},              // latn
	{"zh-HK", "-1,234,567.891", "非數值"},              // latn
	{"zh-MO", "-1,234,567.891", "非數值"},              // latn
	{"zh-SG", "-1,234,567.891", "NaN"},              // latn
	{"zh-CN", "-1,234,567.891", "NaN"},              // latn
	{"sr-ME", "-1.234.567,891", "NaN"},              // latn
	{"sr-RS", "-1.234.567,891", "NaN"},              // latn
	{"sr-BA", "-1.234.567,891", "NaN"},              // latn
	{"sr-XK", "-1.234.567,891", "NaN"},              // latn
	{"sd-PK", "\u061c-١٬٢٣٤٬٥٦٧.٨٩١", "NaN"},        // arab
	{"sd-IN", "-1,234,567.891", "NaN"},              // latn
	{"mni-IN", "-১,২৩৪,৫৬৭.৮৯১", "NaN"},             // beng
	{"sat-IN", "-᱑,᱒᱓᱔,᱕᱖᱗.᱘᱙᱑", "NaN"},             // olck
	{"ks-IN", "\u200e-\u200e۱٬۲۳۴٬۵۶۷٫۸۹۱", "NaN"},  // arabext
	{"kxv-IN", "-12,34,567.891", "NaN"},             // latn
	{"ff-GN", "-1 234 567,891", "NaN"},              // latn
	{"ff-SN", "-1 234 567,891", "NaN"},              // latn
	{"shi-MA", "-1 234 567,891", "NaN"},             // latn
	{"vai-LR", "-1,234,567.891", "NaN"},             // latn
	{"yue-CN", "-1,234,567.891", "非数值"},             // latn
	{"yue-HK", "-1,234,567.891", "非數值"},             // latn
	{"su-ID", "-1.234.567,891", "NaN"},              // latn
	{"bs-BA", "-1.234.567,891", "NaN"},              // latn
	{"az-AZ", "-1.234.567,891", "NaN"},              // latn
	{"az-IR", "-1.234.567,891", "NaN"},              // latn
	{"ku-SY", "-1.234.567,891", "NaN"},              // latn
	{"ha-SD", "-1,234,567.891", "NaN"},              // latn
	{"th-u-nu-thai", "-๑,๒๓๔,๕๖๗.๘๙๑", "NaN"},       // digits of a system no locale uses by default
	{"ta-u-nu-tamldec", "-௧௨,௩௪,௫௬௭.௮௯௧", "NaN"},    // digits of a system no locale uses by default
	{"en-u-nu-hanidec", "-一,二三四,五六七.八九一", "NaN"},    // digits of a system no locale uses by default
}

func TestNumbersOfScriptAliasesMatchICU(t *testing.T) {
	for _, c := range icuAliases {
		f := For(c.loc)
		if got := f.Number(-1234567.891); got != c.number {
			t.Errorf("For(%q).Number(-1234567.891) = %q, want ICU's %q", c.loc, got, c.number)
		}
		if got := f.Number(math.NaN()); got != c.nan {
			t.Errorf("For(%q).Number(NaN) = %q, want ICU's %q", c.loc, got, c.nan)
		}
	}
}
