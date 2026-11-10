package i18n

import (
	"fmt"
	"strings"
)

// ICU number skeletons (https://unicode-org.github.io/icu/userguide/format_parse/numbers/skeletons.html),
// the style of {n, number, ::...} in messages (ADR-266). A skeleton is
// stems separated by spaces, some with options after "/". Supported, in
// their long and concise forms:
//
//	notation      compact-short K, compact-long KK, scientific, engineering, E0, EE0, notation-simple
//	unit          percent %, %x100 (percent of a fraction), currency/EUR, base-unit
//	unit width    unit-width-short (symbol), unit-width-narrow, unit-width-iso-code, unit-width-full-name
//	precision     precision-integer ., .00 .0# .## .00+ .+ (fraction digits), @@@ @@# @+ (significant
//	              digits), precision-unlimited, precision-currency-standard
//	rounding      rounding-mode-half-even (the default), -half-up, -half-down, -half-ceiling,
//	              -half-floor, -ceiling, -floor, -up, -down (ICU has no concise forms)
//	scale         scale/100, scale/0.001... (powers of ten)
//	grouping      group-off ,_ group-min2 ,? group-auto group-on-aligned ,! group-thousands ,=
//	sign          sign-auto sign-always +! sign-never +_ sign-except-zero +? sign-negative +-
//	              sign-accounting () sign-accounting-always ()! sign-accounting-except-zero ()?
//	              sign-accounting-negative ()-
//	numbering     latin, numbering-system/arab
//	decimal       decimal-auto
//
// Other stems (units, permille, integer width, increments, the modes
// half-odd and unnecessary, options of scientific notation, decimal-always) fail
// when the catalog loads. As in Intl.NumberFormat, and unlike ICU's
// NumberFormatter, a skeleton without a precision rounds to 3 fraction
// digits as {n, number} does (ICU keeps 6).

// parseSkeleton parses the skeleton of a number style (after "::").
func parseSkeleton(s string) (numberConfig, error) {
	c := defaultNumberConfig()
	stems := strings.Fields(s)
	if len(stems) == 0 {
		return c, fmt.Errorf("empty skeleton after \"::\"")
	}
	for _, stem := range stems {
		if err := c.stem(stem); err != nil {
			return c, err
		}
	}
	return c, nil
}

var signStems = map[string]struct {
	sign       SignDisplayMode
	accounting bool
}{
	"sign-auto": {SignAuto, false}, "sign-always": {SignAlways, false}, "+!": {SignAlways, false},
	"sign-never": {SignNever, false}, "+_": {SignNever, false},
	"sign-except-zero": {SignExceptZero, false}, "+?": {SignExceptZero, false},
	"sign-negative": {SignNegative, false}, "+-": {SignNegative, false},
	"sign-accounting": {SignAuto, true}, "()": {SignAuto, true},
	"sign-accounting-always": {SignAlways, true}, "()!": {SignAlways, true},
	"sign-accounting-except-zero": {SignExceptZero, true}, "()?": {SignExceptZero, true},
	"sign-accounting-negative": {SignNegative, true}, "()-": {SignNegative, true},
}

// roundingStems are ICU's rounding modes other than half-even (the zero
// Rounding): up and down are away from and toward zero.
var roundingStems = map[string]Rounding{
	"rounding-mode-ceiling": RoundCeil, "rounding-mode-floor": RoundFloor,
	"rounding-mode-up": RoundExpand, "rounding-mode-down": RoundTrunc,
	"rounding-mode-half-up": RoundHalfExpand, "rounding-mode-half-down": RoundHalfTrunc,
	"rounding-mode-half-ceiling": RoundHalfCeil, "rounding-mode-half-floor": RoundHalfFloor,
}

var groupStems = map[string]groupingMode{
	"group-off": groupOff, ",_": groupOff, "group-min2": groupMin2, ",?": groupMin2,
	"group-auto": groupAuto, "group-on-aligned": groupAlways, ",!": groupAlways,
	"group-thousands": groupThousands, ",=": groupThousands,
}

var widthStems = map[string]CurrencyDisplayMode{
	"unit-width-short": CurrencySymbol, "unit-width-narrow": CurrencyNarrowSymbol,
	"unit-width-iso-code": CurrencyCode, "unit-width-full-name": CurrencyName,
}

// unlimited is the fraction digits of precision-unlimited: the value's
// shortest digits are never rounded.
const unlimited = 400

func (c *numberConfig) stem(stem string) error {
	name, opt, hasOpt := strings.Cut(stem, "/")
	if s, ok := signStems[stem]; ok {
		c.signDisplay = s.sign
		c.currencySign = CurrencySignStandard
		if s.accounting {
			c.currencySign = CurrencySignAccounting
		}
		return nil
	}
	if g, ok := groupStems[stem]; ok {
		c.grouping = g
		return nil
	}
	if w, ok := widthStems[stem]; ok {
		c.currencyDisplay = w
		return nil
	}
	switch {
	case stem == "compact-short" || stem == "K":
		c.notation, c.compactDisplay = NotationCompact, CompactShort
	case stem == "compact-long" || stem == "KK":
		c.notation, c.compactDisplay = NotationCompact, CompactLong
	case stem == "scientific" || stem == "E0":
		c.notation = NotationScientific
	case stem == "engineering" || stem == "EE0":
		c.notation = NotationEngineering
	case stem == "notation-simple":
		c.notation = NotationStandard
	case stem == "percent" || stem == "%":
		c.style, c.scale = stylePercent, 0
	case stem == "%x100":
		c.style, c.scale = stylePercent, 2
	case stem == "base-unit":
		c.style, c.scale = styleDecimal, 0
	case name == "currency" && hasOpt:
		if len(opt) != 3 || !isASCIILetters(opt) {
			return fmt.Errorf("currency/%s: not an ISO 4217 code; use three letters, as in currency/EUR", opt)
		}
		c.style, c.currency = styleCurrency, strings.ToUpper(opt)
	case stem == "precision-integer" || stem == ".":
		c.minFrac, c.maxFrac, c.minSig, c.maxSig = 0, 0, 0, 0
	case stem == "precision-unlimited" || stem == ".+":
		c.minFrac, c.maxFrac, c.minSig, c.maxSig = 0, unlimited, 0, 0
	case stem == "precision-currency-standard":
		c.minFrac, c.maxFrac, c.minSig, c.maxSig = -1, -1, 0, 0
	case roundingStems[stem] != 0 || stem == "rounding-mode-half-even":
		c.rounding = roundingStems[stem]
	case stem == "decimal-auto":
	case stem == "latin":
		c.numbering = "latn"
	case name == "numbering-system" && hasOpt:
		if _, ok := numberingDigits[opt]; !ok && opt != "latn" {
			return fmt.Errorf("numbering-system/%s: not a numeric numbering system (latn, arab, deva, thai...)", opt)
		}
		c.numbering = opt
	case name == "scale" && hasOpt:
		n, ok := powerOfTen(opt)
		if !ok {
			return fmt.Errorf("scale/%s: only powers of ten are supported (scale/100, scale/0.001)", opt)
		}
		c.scale += n
	case strings.HasPrefix(stem, "."):
		lo, hi, ok := digitStem(stem[1:], '0')
		if !ok {
			return fmt.Errorf("precision stem %q: use .00, .0#, .## or .00+ (options after / are not supported)", stem)
		}
		c.minFrac, c.maxFrac, c.minSig, c.maxSig = lo, hi, 0, 0
	case strings.HasPrefix(stem, "@"):
		lo, hi, ok := digitStem(stem, '@')
		if !ok || lo == 0 {
			return fmt.Errorf("precision stem %q: use @@@, @@# or @+ (options after / are not supported)", stem)
		}
		c.minFrac, c.maxFrac, c.minSig, c.maxSig = -1, -1, lo, hi
	default:
		return fmt.Errorf("unsupported skeleton stem %q; see the i18n package documentation for the supported ones", stem)
	}
	return nil
}

// digitStem reads a precision stem's digits: some required marks (0 or
// @), then # marks or a +: the minimum and maximum digits.
func digitStem(s string, mark byte) (lo, hi int, ok bool) {
	i := 0
	for i < len(s) && s[i] == mark {
		i++
	}
	lo = i
	switch {
	case i == len(s):
		return lo, lo, true
	case s[i:] == "+":
		return lo, unlimited, true
	}
	j := i
	for j < len(s) && s[j] == '#' {
		j++
	}
	if j != len(s) {
		return 0, 0, false
	}
	return lo, lo + j - i, true
}

// powerOfTen returns n when s is 10^n written as a plain decimal ("100",
// "0.01").
func powerOfTen(s string) (int, bool) {
	ip, fp, hasFrac := strings.Cut(s, ".")
	switch {
	case !hasFrac && len(ip) > 0 && ip[0] == '1' && strings.Trim(ip[1:], "0") == "":
		return len(ip) - 1, true
	case hasFrac && strings.Trim(ip, "0") == "" && len(fp) > 0 && fp[len(fp)-1] == '1' && strings.Trim(fp[:len(fp)-1], "0") == "":
		return -len(fp), true
	}
	return 0, false
}
