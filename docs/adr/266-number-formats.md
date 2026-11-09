# ADR-266: Number formats

- **Date:** 2026-11-09
- **Status:** Accepted. Amends ADR-087, ADR-251 and ADR-259.

## Decision

- **Options in Intl.NumberFormat's style** (owner): `SignDisplay` (auto, always, exceptZero, negative, never), `Currency(code)` with `CurrencyDisplay` (symbol, narrow, code, name) and `CurrencySign` (standard, accounting), `Notation` (standard, scientific, engineering, compact) with `CompactDisplay` (short, long). Intl's defaults (currency digits, two significant digits in compact notation); ties still go to the even digit (Intl rounds them away from zero).
- **Tela formats every number itself**: shortest float64 digits (strconv, as ICU's double-conversion), its own rounding and patterns. x/text keeps only tags and plural rules (CLDR 32).
- **Data from CLDR 48**: symbols and decimal, percent, currency and accounting patterns of every locale, root.xml's arab and arabext symbols, currency symbols of every locale (current tender only), currency digits and region currencies; compact patterns and currency names (flate-compressed) for the 113 generated locales.
- **Messages**: `{n, number, currency}` and ICU skeletons (`::currency/EUR`, `::compact-short`, `::sign-always`, precision, grouping, scale, numbering-system); other stems fail at load.
- **`-u-nu-`**: any numeric CLDR system, with ICU's symbol fallback; unknown systems give the default.
- **`scriptAliases` is generated** from ICU 78.1's converter `config.xml` and CLDR likely subtags.

## Why

One engine keeps percent, currency, compact and scientific text consistent. Compressed names keep the hello-world growth to about 0.3 MB.
