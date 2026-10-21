// Package i18n translates an app and formats numbers, dates and lists for
// the user's locale, and switches the language at run time.
//
// Messages live in one JSON file per locale, written in ICU MessageFormat
// and loaded from an fs.FS (usually embedded):
//
//	locales/en.json   {"hello": "Hello, {name}!",
//	                   "files": "{count, plural, =0 {No files} one {# file} other {# files}}"}
//	locales/es.json   {"hello": "¡Hola, {name}!",
//	                   "files": "{count, plural, =0 {Sin archivos} one {# archivo} other {# archivos}}"}
//
//	//go:embed locales/*.json
//	var locales embed.FS
//
//	func init() { must(i18n.Load(locales, "locales/*.json")) }
//
// Components translate with the [UseT] hook, which also rebuilds them when
// the locale changes; handlers and goroutines use [T]:
//
//	func Files(n int) tela.Node {
//	    t := i18n.UseT()
//	    return Text(t("files", "count", n)) // "3 files", "3 archivos"
//	}
//
//	Button(OnClick(func(tela.MouseEvent) { i18n.SetLocale("es") }), Text("Español"))
//
// # Locale
//
// The current locale is the one given to [SetLocale]; until then, the
// system's (LC_ALL, LC_MESSAGES or LANG, and the user's default locale on
// Windows) when a loaded catalog covers its language, else the fallback
// locale ("en", else the first catalog loaded, or [SetFallback]). A message is looked
// up in the locale's catalog, then its parents' (es-MX, es) and the
// fallback's; a key found nowhere is returned as is, and reported once when
// TELA_DEV=1.
//
// SetLocale builds again only the components that called [UseLocale],
// [UseT] or [UseFormatter]. In tests, [Provider] fixes the locale of one
// tree without touching the process-wide one.
//
// # ICU MessageFormat
//
// Supported: arguments ({name}), {n, number[, integer|percent]},
// {d, date|time[, short|medium|long|full]}, plural and selectordinal
// (=N, zero, one, two, few, many, other, offset:N and # for the number),
// select, nesting, and ICU's apostrophe quoting (” is an apostrophe; a
// single apostrophe quotes only before a brace, or # in a plural). Plural
// rules are CLDR 32's, from golang.org/x/text. Not supported: number
// skeletons and patterns, currencies, units, spellout, and choice.
//
// # Formatting
//
// [FormatNumber], [FormatDate], [FormatTime], [FormatDateTime] and
// [FormatList] format for the current locale; [Formatter] (from [For] or
// [UseFormatter]) does the same for a given one. Numbers come from
// golang.org/x/text/number; dates and lists from CLDR 48 tables generated
// into this package (go generate), for the locales listed in
// internal/gen: other locales use the closest one. Dates are Gregorian and
// in the time.Time's own zone.
//
// [Dir] tells the direction of a locale ([LTR] or [RTL]); h.Dir with its
// String lays a subtree out right to left.
package i18n
