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
// [Load] adds to the catalogs already loaded; [Replace] makes the files the
// only catalogs in one step (a reload in development, a language pack the
// user installs) and [Unload] removes the catalogs of some locales, or all.
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
// fallback's; a key found nowhere is returned as is, and reported once in
// dev mode (Tela's: on unless the program is a release build, App.Dev
// once Run starts, the mode of the app whose component calls T, TELA_DEV=0
// or 1 overriding).
//
// SetLocale builds again only the components that called [UseLocale],
// [UseT] or [UseFormatter]. In tests, [Provider] fixes the locale of one
// tree without touching the process-wide one.
//
// # ICU MessageFormat
//
// Supported: arguments ({name}), {n, number[, integer|percent|currency]}
// (currency is the locale's: its -u-cu-, else its region's), number
// skeletons after :: ({n, number, ::currency/EUR}, ::compact-short,
// ::scientific, ::sign-always, ::.00, ::@@#, ::group-off...; the list is in
// skeleton.go, and other stems fail when the catalog loads),
// {d, date|time[, short|medium|long|full]}, plural and selectordinal
// (=N, zero, one, two, few, many, other, offset:N and # for the number),
// select, nesting, and ICU's apostrophe quoting (two apostrophes are one; a
// single apostrophe quotes only before a brace, or # in a plural). Plural
// rules are CLDR 32's, from golang.org/x/text. Not supported: number
// patterns, units, spellout, and choice.
//
// # Formatting
//
// [FormatNumber], [FormatDate], [FormatTime], [FormatDateTime] and
// [FormatList] format for the current locale; [Formatter] (from [For] or
// [UseFormatter]) does the same for a given one. Numbers are written as
// ICU's NumberFormatter writes them with Intl.NumberFormat's options and
// defaults, from CLDR 48 tables generated into this package (go
// generate): the symbols, grouping and patterns of every CLDR locale (the
// minus sign and bidi marks of Persian, Urdu and Arabic, Nepali grouping,
// Spanish and Polish not grouping four digits), rounded half to even.
// Options ([NumberOption]) add percentages ([Percent]), amounts of money
// ([Currency], with [CurrencyDisplay] and [CurrencySign]), signs
// ([SignDisplay]) and scientific, engineering and compact notation
// ([Notation], [CompactDisplay]):
//
//	f.Number(-1234.5, i18n.Currency("EUR"))               // "-€1,234.50", "-1.234,50 €" (de)
//	f.Number(1234567, i18n.Notation(i18n.NotationCompact)) // "1.2M", "12 लाख" (hi), "123万" (ja)
//	f.Number(5, i18n.SignDisplay(i18n.SignAlways))         // "+5"
//	f.Number(2, i18n.Currency("PLN"), i18n.CurrencyDisplay(i18n.CurrencyName))
//	// "2,00 złotego polskiego" (pl)
//
// Currency symbols and patterns cover every CLDR locale; currency names
// and compact patterns, like dates and lists, the locales listed in
// internal/gen (other locales use the closest one). Only the currencies
// some region uses today have symbols and names; others are written with
// their code. Dates are Gregorian and
// in the time.Time's own zone. For calendars, [Formatter.MonthName] and
// [Formatter.WeekdayName] give the names on their own in four widths
// ([NameWidth]), and [FirstDayOfWeek] the day a week starts on, from
// CLDR's week data by region.
//
// Digits follow the locale's default numbering system in CLDR 48, as ICU's
// do: ar-EG, ar-SA, ckb and sd write Arabic-Indic digits, fa, ps and ur-IN
// extended ones, bn, mr, ne and my their own, ar-AE and ar-MA Latin ones; a
// tag's own -u-nu- (ar-EG-u-nu-latn, en-u-nu-thai: any numeric system of
// CLDR) wins, with the symbols of that system in the locale, else the
// root's (arab, arabext), else the locale's Latin ones, as ICU does; an
// unknown or algorithmic system (-u-nu-roman) is ignored. Bare "ar" writes
// Latin digits, as in CLDR 48 (ADR-258); say "ar-EG" or "ar-SA" for
// Arabic-Indic ones.
//
// [Dir] tells the direction of a locale ([LTR] or [RTL]); h.Dir with its
// String lays a subtree out right to left.
//
// # Tela's own texts
//
// Importing i18n makes the catalogs the source of the texts Tela shows by
// itself (ADR-251): the fallback title bar's buttons, message boxes, kit's
// toast viewport, the standard menu items, the macOS save panel. A message
// whose key is the label's ID translates it; tela.Labels lists the IDs
// with their English texts, and [Formatter.Label] gives one for a locale:
//
//	{"tela": {"messagebox": {"ok": "Aceptar", "cancel": "Cancelar"},
//	          "titlebar": {"close": "Cerrar"},
//	          "toast":    {"close": "Cerrar aviso", "region": "Notificaciones"}}}
//
// Labels the catalogs lack stay English. Tela ships no translations.
package i18n
