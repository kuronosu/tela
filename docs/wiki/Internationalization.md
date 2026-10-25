# Internationalization

Package [`i18n`](https://pkg.go.dev/github.com/kuronosu/tela/i18n)
translates an app and formats numbers, dates and lists for the user's
locale, and switches the language while the app runs.

```go
import "github.com/kuronosu/tela/i18n"
```

| You want | Use |
|---|---|
| Messages in several languages | One JSON file per locale, loaded with [`i18n.Load`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#Load) |
| To translate in a component | [`i18n.UseT`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#UseT) |
| To translate in a handler or a goroutine | [`i18n.T`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#T) |
| To switch the language | [`i18n.SetLocale`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#SetLocale) |
| Numbers, dates, times and lists | [`FormatNumber`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#FormatNumber), [`FormatDate`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#FormatDate), [`FormatTime`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#FormatTime), [`FormatList`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#FormatList), or the methods of [`UseFormatter`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#UseFormatter) |
| Calendar data | [`Formatter.MonthName`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#Formatter.MonthName), [`Formatter.WeekdayName`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#Formatter.WeekdayName), [`FirstDayOfWeek`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#FirstDayOfWeek) |
| Whether to lay out right to left | [`i18n.Dir`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#Dir) |
| A fixed locale in a test or a preview | [`i18n.Provider`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#Provider) |

## Catalogs

A catalog is a JSON object of messages in
[ICU MessageFormat](https://unicode-org.github.io/icu/userguide/format_parse/messages/),
one file per locale, named after it: `locales/en.json`, `locales/es.json`,
`locales/pt-BR.json`. Nested objects give dotted keys (`nav.home`).

```json
{
  "hello": "Hello, {name}!",
  "files": "{count, plural, =0 {No files} one {# file} other {# files}}",
  "nav": { "home": "Home" }
}
```

Embed the files and load them once, from `init` or `main`:

```go
//go:embed locales/*.json
var locales embed.FS

func init() {
	if err := i18n.Load(locales, "locales/*.json"); err != nil {
		log.Fatal(err)
	}
}
```

Every message is parsed when it is loaded, so a mistake is reported at
startup with its file, key and position, for example
`i18n: locales/en.json: message "files": 1:30: {count, plural, ...} needs an "other" case: other {...}`
for `"{count, plural, one {# file}}"`, and nothing is loaded. Loading may be repeated: a locale loaded again adds
its messages.

### Message syntax

| Syntax | Example |
|---|---|
| Argument | `Hello, {name}!` |
| Number | `{n, number}`, `{n, number, integer}`, `{n, number, percent}`; a plain `{n}` formats numbers for the locale too |
| Date and time | `{when, date, long}`, `{when, time, short}` (styles `short`, `medium`, `long`, `full`; the argument is a `time.Time`) |
| Plural | `{n, plural, =0 {none} one {# item} other {# items}}`; categories `zero one two few many other` (as the language has them), `=N` for exact values, `offset:1`, and `#` for the number, formatted for the locale |
| Ordinal | `{n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}` |
| Select | `{gender, select, female {her} male {his} other {their}}` |
| Nesting | Cases hold messages, which may hold more arguments: a plural inside a select inside a plural; `#` is always the innermost enclosing plural's |
| Quoting | `''` is an apostrophe; an apostrophe quotes only when it comes right before `{`, `}` (or `#` in a plural), up to the next apostrophe: `'{'name'}'` is `{name}`; `don't` needs nothing |

A plural needs an `other` case. Number skeletons and patterns
(`{n, number, ::currency/USD}`), currencies, units, spellout and `choice`
are not supported and fail at load time.

## Translating

```go
func Files(n int) tela.Node {
	t := i18n.UseT()
	return Text(t("files", "count", n)) // "3 files", "3 archivos"
}
```

Arguments are name/value pairs, like `log/slog`. A number argument is
formatted for the locale and picks the plural; a `time.Time` is a date; a
string is itself. A key that is in no catalog is returned as is, and in dev
mode (on unless the program is a release build, the app's mode under
`App.Dev`, `TELA_DEV=0` or `1` overriding) reported once on stderr; an argument that the message uses but the
call does not pass keeps its `{name}` and is reported in dev mode too.

Messages are looked up in the locale's catalog, then its parents (es-MX
uses es) and the fallback locale, which is `en` when loaded, else the first
catalog loaded, unless [`SetFallback`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#SetFallback)
says otherwise. A message that comes from another language (the fallback's)
uses its own language's plural rules.

`UseT` is a hook, so call it at the top of a component. In a handler or a
goroutine use `i18n.T`, which does not subscribe anything.

## The locale

Until you call `SetLocale`, the locale is the system's (`LC_ALL`,
`LC_MESSAGES` or `LANG` on Linux; the user's default locale on Windows;
macOS reads the environment for now) when a loaded catalog covers its
language, else the fallback. `i18n.Locale()` returns it as a BCP 47 string
("es-MX").

```go
Button(OnClick(func(tela.MouseEvent) { i18n.SetLocale("es") }), Text("Español"))
```

`SetLocale` is safe from any goroutine. Only the components that called
`UseT`, `UseLocale` or `UseFormatter` build again, and only when the locale
really changes; the rest of the tree is untouched. `SetLocale("")` follows
the system again. A locale without a catalog still formats numbers, dates and
lists for itself (`SetLocale("fr")` with only English messages shows English
text and French numbers). [`i18n.Locales()`](https://pkg.go.dev/github.com/kuronosu/tela/i18n#Locales)
lists the loaded catalogs, for a language picker.

## Numbers, dates, times and lists

```go
f := i18n.UseFormatter() // rebuilds the component when the locale changes

f.Number(1234.5)                       // "1,234.5", "1.234,5" (es), "١٬٢٣٤٫٥" (ar)
f.Number(0.12, i18n.Percent())         // "12%"
f.Number(2, i18n.MinFractionDigits(2)) // "2.00"
f.Date(t, i18n.Long)                   // "January 5, 2026", "5 de enero de 2026"
f.Date(t, i18n.Full)                   // "Monday, January 5, 2026"
f.Time(t, i18n.Short)                  // "3:04 PM", "15:04"
f.List([]string{"a", "b", "c"})        // "a, b, and c", "a, b y c", "a、b、c"
```

`i18n.FormatNumber`, `FormatDate`, `FormatTime`, `FormatDateTime` and
`FormatList` do the same for the current locale without a hook: pair them
with `UseLocale` in a component so that it rebuilds. `i18n.For("de")` is the
formatter of any locale.

For calendars and date pickers, a Formatter also has the names of months and
weekdays on their own (CLDR's stand-alone form, which differs from the one in
a date in languages such as Russian: "октябрь", "5 октября") in four widths,
and the first day of the week:

```go
f.MonthName(time.October, i18n.NameWide)        // "October", "octubre", "октябрь"
f.MonthName(time.October, i18n.NameAbbreviated) // "Oct"
f.WeekdayName(time.Monday, i18n.NameShort)      // "Mo", "LU" (es), "月" (ja)
f.WeekdayName(time.Monday, i18n.NameNarrow)     // "M"
f.MonthYear(t)                                  // "October 2026", "octubre de 2026", "2026年10月", "2026. október"
f.FirstDayOfWeek()                              // time.Sunday for en-US, time.Monday for de or es-ES
i18n.FirstDayOfWeek("ar-EG")                    // time.Saturday
```

`MonthYear` is the heading of a calendar (CLDR's `yMMMM` format): the month
on its own and the year, in the locale's order and with its words
("октябрь 2026 г." in Russian, where a date says "5 октября").

The first day goes by region: the tag's own, else its language's most likely
one (`en` is the United States, `ar` Egypt), Monday when CLDR has no data;
`-u-fw-mon` and `-u-rg-gbzzzz` in the tag override it.

Numbers and plural rules come from `golang.org/x/text` (CLDR 32); dates,
lists and week data from CLDR 48 tables generated into the package for 113 locales. Other
locales use the closest one (en-SG uses en-001, es-PE uses es-419). Dates are
Gregorian and in the time's own zone, with zone abbreviations rather than
localized names.

## Right to left

`i18n.Dir("ar")` is `i18n.RTL`, `Dir("en")` is `i18n.LTR`: the direction of
the locale's script (Arabic, Hebrew, Persian, Urdu and the other
right-to-left scripts). Give it to [`h.Dir`](https://pkg.go.dev/github.com/kuronosu/tela/h#Dir),
the `dir` attribute of HTML, at the top of the screen:

```go
func App() tela.Node {
	f := i18n.UseFormatter() // rebuilds when the locale changes
	return Div(Dir(f.Dir().String()), Class("min-h-screen p-6"), // "RTL" or "LTR"
		Header(), Content(),
	)
}
```

Direction is inherited, and under `Dir("rtl")` the layout mirrors like a
browser's:

- flex rows (and `row-reverse`) start at the right; `justify-start`,
  `justify-end`, `gap-*`, `order-*` and auto margins follow;
- grid column 1 is the rightmost; explicit and automatic placement, spans
  and `justify-*` follow;
- paragraphs are right to left: they align right by default (`text-start`
  and `text-end` follow, `text-left` and `text-right` do not), and Latin
  words, numbers and punctuation inside Arabic text are ordered by the
  Unicode bidi algorithm with a right-to-left base;
- the logical classes use the right as the start: `ms-*`, `me-*`, `ps-*`,
  `pe-*`, `start-*`, `end-*`, `inset-s-*`, `inset-e-*`, `border-s-*`,
  `border-e-*`, `rounded-s-*`, `rounded-e-*`, `rounded-ss-*` and friends,
  and `space-x-*`; physical ones (`ml-*`, `left-*`, `border-l-*`,
  `rounded-tl-*`) stay where they are;
- an absolutely positioned box without horizontal insets starts at the
  right of where it would be in flow;
- scroll containers start scrolled to the right, content overflows to the
  left, and the vertical scrollbar is on the left; `ScrollOffset`,
  `ScrollTo`, `UseScroll` and `OnScroll` use the DOM's convention (x is 0
  at the right and negative toward the left);
- `Input` and `Textarea` type right to left and align right.

`rtl:` and `ltr:` classes apply under each direction, for what has no
logical class: `Div(Class("rtl:rotate-180"), Text("→"))`. `Dir("ltr")`
inside a right-to-left screen goes back to left to right for its subtree
(a code sample, a phone number field).

`Dir("auto")` takes the direction of the first strong letter of the
element's text (of its value in an `Input` or `Textarea`, as you type),
like HTML's `dir="auto"`, for user content in either direction:

```go
P(Dir("auto"), Text(comment.Body)) // an Arabic comment is right to left
```

The direction is not part of `i18n.Provider`: a Provider is a context
without a box of its own, so it cannot lay anything out. Put the `Dir` on
the element under it, as above.

Not mirrored: `Dir` on an inline element inside a paragraph (`Span`,
`Strong`, `A`) does not isolate its text from the paragraph's bidi
ordering (write the Unicode isolates U+2067 RLI / U+2066 LRI ... U+2069 PDI
in the text instead), and there is no `divide-x-*` (Tela has no
`divide-*` classes).

[`examples/i18n`](https://github.com/kuronosu/tela/tree/main/examples/i18n)
is an order summary in English, Spanish and Arabic with a language switcher
that mirrors the screen.

## Testing

`i18n.Provider` fixes the locale of its subtree, so a test does not touch the
process-wide locale:

```go
func TestSettingsInSpanish(t *testing.T) {
	app := telatest.Mount(t, i18n.Provider("es", Settings()))
	app.ExpectText("Ajustes")
}
```

The catalogs themselves are loaded globally; tests that call `SetLocale` or
`Load` should not run in parallel with each other.
