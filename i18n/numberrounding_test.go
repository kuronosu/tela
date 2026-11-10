package i18n

import "testing"

// roundingModes are the modes in the order of intlRounding's columns,
// Intl's ceil, floor, expand, trunc, halfCeil, halfFloor, halfExpand,
// halfTrunc and halfEven, with the ICU skeleton stem of each.
var roundingModes = [9]struct {
	mode Rounding
	stem string
}{
	{RoundCeil, "rounding-mode-ceiling"}, {RoundFloor, "rounding-mode-floor"},
	{RoundExpand, "rounding-mode-up"}, {RoundTrunc, "rounding-mode-down"},
	{RoundHalfCeil, "rounding-mode-half-ceiling"}, {RoundHalfFloor, "rounding-mode-half-floor"},
	{RoundHalfExpand, "rounding-mode-half-up"}, {RoundHalfTrunc, "rounding-mode-half-down"},
	{RoundHalfEven, "rounding-mode-half-even"},
}

// intlRounding are Node 26.12's Intl.NumberFormat outputs (ICU 78.3,
// CLDR 48) for new Intl.NumberFormat(loc, {...options, roundingMode}),
// options as the specs of intlOptions, one column per mode of
// roundingModes: positive and negative numbers, ties (1.25, 1.005 is a tie
// in its shortest digits), amounts, compact and scientific notation and
// percentages.
var intlRounding = []struct {
	loc, spec string
	v         float64
	want      [9]string
}{
	{"en", "maxfrac=1", 1.21, [9]string{"1.3", "1.2", "1.3", "1.2", "1.2", "1.2", "1.2", "1.2", "1.2"}},
	{"en", "maxfrac=1", 1.25, [9]string{"1.3", "1.2", "1.3", "1.2", "1.3", "1.2", "1.3", "1.2", "1.2"}},
	{"en", "maxfrac=1", 1.29, [9]string{"1.3", "1.2", "1.3", "1.2", "1.3", "1.3", "1.3", "1.3", "1.3"}},
	{"en", "maxfrac=1", 1.35, [9]string{"1.4", "1.3", "1.4", "1.3", "1.4", "1.3", "1.4", "1.3", "1.4"}},
	{"en", "maxfrac=1", -1.21, [9]string{"-1.2", "-1.3", "-1.3", "-1.2", "-1.2", "-1.2", "-1.2", "-1.2", "-1.2"}},
	{"en", "maxfrac=1", -1.25, [9]string{"-1.2", "-1.3", "-1.3", "-1.2", "-1.2", "-1.3", "-1.3", "-1.2", "-1.2"}},
	{"en", "maxfrac=1", -1.29, [9]string{"-1.2", "-1.3", "-1.3", "-1.2", "-1.3", "-1.3", "-1.3", "-1.3", "-1.3"}},
	{"en", "maxfrac=1", -1.35, [9]string{"-1.3", "-1.4", "-1.4", "-1.3", "-1.3", "-1.4", "-1.4", "-1.3", "-1.4"}},
	{"en", "maxfrac=1", 0.04, [9]string{"0.1", "0", "0.1", "0", "0", "0", "0", "0", "0"}},
	{"en", "maxfrac=1", -0.04, [9]string{"-0", "-0.1", "-0.1", "-0", "-0", "-0", "-0", "-0", "-0"}},
	{"en", "currency=USD", 1.005, [9]string{"$1.01", "$1.00", "$1.01", "$1.00", "$1.01", "$1.00", "$1.01", "$1.00", "$1.00"}},
	{"en", "currency=USD", -1.005, [9]string{"-$1.00", "-$1.01", "-$1.01", "-$1.00", "-$1.00", "-$1.01", "-$1.01", "-$1.00", "-$1.00"}},
	{"en", "currency=USD", 2.345, [9]string{"$2.35", "$2.34", "$2.35", "$2.34", "$2.35", "$2.34", "$2.35", "$2.34", "$2.34"}},
	{"en", "currency=USD", -2.345, [9]string{"-$2.34", "-$2.35", "-$2.35", "-$2.34", "-$2.34", "-$2.35", "-$2.35", "-$2.34", "-$2.34"}},
	{"de", "currency=EUR", 1234.565, [9]string{"1.234,57\u00a0€", "1.234,56\u00a0€", "1.234,57\u00a0€", "1.234,56\u00a0€", "1.234,57\u00a0€", "1.234,56\u00a0€", "1.234,57\u00a0€", "1.234,56\u00a0€", "1.234,56\u00a0€"}},
	{"de", "currency=EUR", -1234.565, [9]string{"-1.234,56\u00a0€", "-1.234,57\u00a0€", "-1.234,57\u00a0€", "-1.234,56\u00a0€", "-1.234,56\u00a0€", "-1.234,57\u00a0€", "-1.234,57\u00a0€", "-1.234,56\u00a0€", "-1.234,56\u00a0€"}},
	{"ja", "currency=JPY", 1234.5, [9]string{"￥1,235", "￥1,234", "￥1,235", "￥1,234", "￥1,235", "￥1,234", "￥1,235", "￥1,234", "￥1,234"}},
	{"ja", "currency=JPY", -1234.5, [9]string{"-￥1,234", "-￥1,235", "-￥1,235", "-￥1,234", "-￥1,234", "-￥1,235", "-￥1,235", "-￥1,234", "-￥1,234"}},
	{"en", "compact", 1250, [9]string{"1.3K", "1.2K", "1.3K", "1.2K", "1.3K", "1.2K", "1.3K", "1.2K", "1.2K"}},
	{"en", "compact", -1250, [9]string{"-1.2K", "-1.3K", "-1.3K", "-1.2K", "-1.2K", "-1.3K", "-1.3K", "-1.2K", "-1.2K"}},
	{"en", "compact", 1999, [9]string{"2K", "1.9K", "2K", "1.9K", "2K", "2K", "2K", "2K", "2K"}},
	{"en", "compact", -1999, [9]string{"-1.9K", "-2K", "-2K", "-1.9K", "-2K", "-2K", "-2K", "-2K", "-2K"}},
	{"en", "compact", 999950, [9]string{"1M", "999K", "1M", "999K", "1M", "1M", "1M", "1M", "1M"}},
	{"fr", "compact=long", 2500000, [9]string{"2,5 millions", "2,5 millions", "2,5 millions", "2,5 millions", "2,5 millions", "2,5 millions", "2,5 millions", "2,5 millions", "2,5 millions"}},
	{"fr", "compact=long", -2500000, [9]string{"-2,5 millions", "-2,5 millions", "-2,5 millions", "-2,5 millions", "-2,5 millions", "-2,5 millions", "-2,5 millions", "-2,5 millions", "-2,5 millions"}},
	{"en", "scientific", 12345, [9]string{"1.235E4", "1.234E4", "1.235E4", "1.234E4", "1.235E4", "1.234E4", "1.235E4", "1.234E4", "1.234E4"}},
	{"en", "scientific", -12345, [9]string{"-1.234E4", "-1.235E4", "-1.235E4", "-1.234E4", "-1.234E4", "-1.235E4", "-1.235E4", "-1.234E4", "-1.234E4"}},
	{"ar-EG", "percent", 0.125, [9]string{"١٣٪\u061c", "١٢٪\u061c", "١٣٪\u061c", "١٢٪\u061c", "١٣٪\u061c", "١٢٪\u061c", "١٣٪\u061c", "١٢٪\u061c", "١٢٪\u061c"}},
	{"ar-EG", "percent", -0.125, [9]string{"\u061c-١٢٪\u061c", "\u061c-١٣٪\u061c", "\u061c-١٣٪\u061c", "\u061c-١٢٪\u061c", "\u061c-١٢٪\u061c", "\u061c-١٣٪\u061c", "\u061c-١٣٪\u061c", "\u061c-١٢٪\u061c", "\u061c-١٢٪\u061c"}},
}

func TestRoundingModesMatchIntl(t *testing.T) {
	for _, c := range intlRounding {
		for i, m := range roundingModes {
			opts := append(intlOptions(t, c.spec), RoundingMode(m.mode))
			if got := For(c.loc).Number(c.v, opts...); got != c.want[i] {
				t.Errorf("For(%q).Number(%v, %s, %s) = %q, want Intl's %q", c.loc, c.v, c.spec, m.stem, got, c.want[i])
			}
		}
	}
}

// The skeleton stems give the same texts in messages.
func TestRoundingModeSkeletons(t *testing.T) {
	for _, c := range intlRounding {
		if c.spec != "maxfrac=1" && c.spec != "currency=USD" {
			continue
		}
		style := "::.#"
		if c.spec == "currency=USD" {
			style = "::currency/USD"
		}
		for i, m := range roundingModes {
			if got := msg(t, c.loc, "{n, number, "+style+" "+m.stem+"}", "n", c.v); got != c.want[i] {
				t.Errorf("{n, number, %s %s} with %v = %q, want %q", style, m.stem, c.v, got, c.want[i])
			}
		}
	}
}

// The zero Rounding is half to even, the default.
func TestRoundingDefault(t *testing.T) {
	var zero Rounding
	if zero != RoundHalfEven {
		t.Fatal("the zero Rounding is not RoundHalfEven")
	}
	en := For("en")
	if got, want := en.Number(2.5, MaxFractionDigits(0)), en.Number(2.5, MaxFractionDigits(0), RoundingMode(RoundHalfEven)); got != want || got != "2" {
		t.Errorf("default %q, half-even %q", got, want)
	}
	if got := en.Number(2.5, MaxFractionDigits(0), RoundingMode(RoundHalfExpand)); got != "3" {
		t.Errorf("half-expand: %q", got)
	}
}
