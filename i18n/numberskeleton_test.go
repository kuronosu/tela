package i18n

import (
	"strings"
	"testing"
)

// skeletonCases are messages {n, number, <style>}: the wanted text is
// Node 26.12's Intl.NumberFormat output (ICU 78.3, CLDR 48) for the
// options ICU's skeleton stands for, in the comment.
var skeletonCases = []struct {
	loc, style string
	v          float64
	want       string
}{
	{"en", "::currency/EUR", 1234.56, "€1,234.56"},                                           // {style: "currency", currency: "EUR"}
	{"de", "::currency/EUR", 1234.56, "1.234,56\u00a0€"},                                     // {style: "currency", currency: "EUR"}
	{"en", "::currency/usd", 1234.56, "$1,234.56"},                                           // {style: "currency", currency: "USD"}
	{"en", "::currency/USD unit-width-full-name", 1, "1.00 US dollars"},                      // currencyDisplay: "name"
	{"en", "::currency/USD unit-width-iso-code", 1, "USD\u00a01.00"},                         // currencyDisplay: "code"
	{"en", "::currency/CAD unit-width-narrow", 1, "$1.00"},                                   // currencyDisplay: "narrowSymbol"
	{"en", "::currency/CAD unit-width-short", 1, "CA$1.00"},                                  // currencyDisplay: "symbol"
	{"en", "::currency/USD sign-accounting", -5, "($5.00)"},                                  // currencySign: "accounting"
	{"en", "::currency/USD ()", -5, "($5.00)"},                                               // currencySign: "accounting"
	{"en", "::currency/USD ()!", 5, "+$5.00"},                                                // currencySign: "accounting", signDisplay: "always"
	{"en", "::compact-short", 1234567, "1.2M"},                                               // {notation: "compact"}
	{"en", "::K", 1234567, "1.2M"},                                                           // {notation: "compact"}
	{"en", "::compact-long", 1234567, "1.2 million"},                                         // compactDisplay: "long"
	{"es", "::KK", 2000000, "2 millones"},                                                    // compactDisplay: "long"
	{"de", "::compact-short currency/EUR", 1234567, "1,2\u00a0Mio.\u00a0€"},                  // {notation: "compact", style: "currency", currency: "EUR"}
	{"en", "::scientific", 123456, "1.235E5"},                                                // {notation: "scientific"}
	{"en", "::E0", 123456, "1.235E5"},                                                        // {notation: "scientific"}
	{"en", "::engineering", 123456, "123.456E3"},                                             // {notation: "engineering"}
	{"en", "::EE0", 123456, "123.456E3"},                                                     // {notation: "engineering"}
	{"en", "::sign-always", 5, "+5"},                                                         // {signDisplay: "always"}
	{"en", "::+!", 5, "+5"},                                                                  // {signDisplay: "always"}
	{"en", "::+_", -5, "5"},                                                                  // {signDisplay: "never"}
	{"en", "::sign-except-zero", 0, "0"},                                                     // {signDisplay: "exceptZero"}
	{"en", "::+?", -5, "-5"},                                                                 // {signDisplay: "exceptZero"}
	{"en", "::sign-negative", -0.0001, "0"},                                                  // {signDisplay: "negative"}
	{"en", "::%x100", 0.5, "50%"},                                                            // {style: "percent"}
	{"en", "::percent scale/100", 0.5, "50%"},                                                // {style: "percent"}
	{"en", "::.00", 2, "2.00"},                                                               // {minimumFractionDigits: 2, maximumFractionDigits: 2}
	{"en", "::.0#", 2.346, "2.35"},                                                           // {minimumFractionDigits: 1, maximumFractionDigits: 2}
	{"en", "::.##", 2.3, "2.3"},                                                              // {maximumFractionDigits: 2}
	{"en", "::precision-integer", 2.6, "3"},                                                  // {maximumFractionDigits: 0}
	{"en", "::@@@", 1234, "1,230"},                                                           // {minimumSignificantDigits: 3, maximumSignificantDigits: 3}
	{"en", "::@@#", 1, "1.0"},                                                                // {minimumSignificantDigits: 2, maximumSignificantDigits: 3}
	{"en", "::@@@", 0.012346, "0.0123"},                                                      // {minimumSignificantDigits: 3, maximumSignificantDigits: 3}
	{"en", "::group-off", 1234567, "1234567"},                                                // {useGrouping: false}
	{"en", "::,_", 1234567, "1234567"},                                                       // {useGrouping: false}
	{"es", "::group-on-aligned", 1234, "1.234"},                                              // {useGrouping: "always"}
	{"es", "::group-min2", 1234, "1234"},                                                     // {useGrouping: "min2"}
	{"ar-EG", "::latin", 1234.5, "1,234.5"},                                                  // {numberingSystem: "latn"}
	{"en", "::numbering-system/arab", 1234.5, "١٬٢٣٤٫٥"},                                     // {numberingSystem: "arab"}
	{"en", "::sign-always compact-short", 1234, "+1.2K"},                                     // {signDisplay: "always", notation: "compact"}
	{"en", "::currency/JPY precision-currency-standard", 1234.6, "¥1,235"},                   // {style: "currency", currency: "JPY"}
	{"en", "::rounding-mode-half-even decimal-auto notation-simple", 1234.5678, "1,234.568"}, // {}
}

func TestNumberSkeletons(t *testing.T) {
	for _, c := range skeletonCases {
		if got := msg(t, c.loc, "{n, number, "+c.style+"}", "n", c.v); got != c.want {
			t.Errorf("%s {n, number, %s} with %v = %q, want %q", c.loc, c.style, c.v, got, c.want)
		}
	}
	// An integer argument is exact.
	if got := msg(t, "en", "{n, number, ::compact-short}", "n", int64(1234567)); got != "1.2M" {
		t.Errorf("int64 compact: %q", got)
	}
	// A skeleton's percent is a unit: no scaling (ICU's ::percent).
	if got := msg(t, "en", "{n, number, ::percent}", "n", 50); got != "50%" {
		t.Errorf("::percent: %q", got)
	}
}

// {n, number, currency} is the locale's currency: -u-cu-, else its
// region's (ICU's argument style; Intl has no default currency).
func TestNumberCurrencyStyle(t *testing.T) {
	for _, c := range []struct{ loc, want string }{
		{"en", "$1,234.56"},
		{"es", "1234,56\u00a0€"},
		{"ja", "￥1,235"},
		{"hi", "₹1,234.56"},
		{"pt-BR", "R$\u00a01.234,56"},
		{"es-u-cu-usd", "1234,56\u00a0US$"},
	} {
		if got := msg(t, c.loc, "{n, number, currency}", "n", 1234.56); got != c.want {
			t.Errorf("%s {n, number, currency} = %q, want %q", c.loc, got, c.want)
		}
	}
}

func TestNumberSkeletonErrors(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"{n, number, ::measure-unit/length-meter}", `unsupported skeleton stem "measure-unit/length-meter"`},
		{"{n, number, ::currency/EURO}", "not an ISO 4217 code"},
		{"{n, number, ::.0/@@}", `precision stem ".0/@@"`},
		{"{n, number, ::@#@}", `precision stem "@#@"`},
		{"{n, number, ::scale/2}", "only powers of ten"},
		{"{n, number, ::numbering-system/roman}", "not a numeric numbering system"},
		{"{n, number, ::}", "empty skeleton"},
		{"{n, number, ::scientific/+ee}", `unsupported skeleton stem "scientific/+ee"`},
		{"{n, number, ::rounding-mode-ceiling}", `unsupported skeleton stem "rounding-mode-ceiling"`},
		{"{n, number, money}", "or a skeleton after ::"},
	} {
		_, err := parseMessage(c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: error %v, want one with %q", c.src, err, c.want)
		}
	}
}

func TestSkeletonDigits(t *testing.T) {
	for _, c := range []struct {
		stem   string
		lo, hi int
		ok     bool
	}{
		{"00", 2, 2, true}, {"0#", 1, 2, true}, {"##", 0, 2, true}, {"00+", 2, unlimited, true},
		{"", 0, 0, true}, {"#0", 0, 0, false}, {"0x", 0, 0, false},
	} {
		lo, hi, ok := digitStem(c.stem, '0')
		if lo != c.lo || hi != c.hi || ok != c.ok {
			t.Errorf("digitStem(%q) = %d, %d, %v; want %d, %d, %v", c.stem, lo, hi, ok, c.lo, c.hi, c.ok)
		}
	}
	for s, want := range map[string]int{"100": 2, "1": 0, "0.01": -2, "1000": 3, ".1": -1} {
		if n, ok := powerOfTen(s); !ok || n != want {
			t.Errorf("powerOfTen(%q) = %d, %v; want %d", s, n, ok, want)
		}
	}
	for _, s := range []string{"2", "10.5", "0.02", ""} {
		if _, ok := powerOfTen(s); ok {
			t.Errorf("powerOfTen(%q) is ok", s)
		}
	}
}
