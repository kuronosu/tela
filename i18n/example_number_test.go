package i18n_test

import (
	"fmt"

	"github.com/kuronosu/tela/i18n"
)

// The texts are quoted (%q) to show their no-break spaces.
func ExampleFormatter_Number() {
	en, de, es := i18n.For("en"), i18n.For("de"), i18n.For("es")
	fmt.Printf("%q\n", en.Number(-1234.5, i18n.Currency("EUR")))
	fmt.Printf("%q\n", de.Number(-1234.5, i18n.Currency("EUR")))
	fmt.Printf("%q\n", es.Number(-1234.5, i18n.Currency("EUR")))
	fmt.Printf("%q\n", en.Number(-1234.5, i18n.Currency("USD"), i18n.CurrencySign(i18n.CurrencySignAccounting)))
	fmt.Printf("%q\n", en.Number(1234.5, i18n.Currency("USD"), i18n.CurrencyDisplay(i18n.CurrencyCode)))
	fmt.Printf("%q\n", en.Number(1, i18n.Currency("USD"), i18n.CurrencyDisplay(i18n.CurrencyName)))
	fmt.Printf("%q\n", i18n.For("ru").Number(5, i18n.Currency("RUB"), i18n.CurrencyDisplay(i18n.CurrencyName), i18n.MaxFractionDigits(0)))
	fmt.Printf("%q\n", i18n.For("pl").Number(2, i18n.Currency("PLN"), i18n.CurrencyDisplay(i18n.CurrencyName)))
	fmt.Printf("%q\n", en.Number(5, i18n.SignDisplay(i18n.SignAlways)))
	for _, loc := range []string{"en", "de", "ja", "hi"} {
		fmt.Printf("%q\n", i18n.For(loc).Number(1234567, i18n.Notation(i18n.NotationCompact)))
	}
	fmt.Printf("%q\n", en.Number(2000000, i18n.Notation(i18n.NotationCompact), i18n.CompactDisplay(i18n.CompactLong)))
	fmt.Printf("%q\n", es.Number(2000000, i18n.Notation(i18n.NotationCompact), i18n.CompactDisplay(i18n.CompactLong)))
	fmt.Printf("%q\n", en.Number(123456, i18n.Notation(i18n.NotationScientific)))
	fmt.Printf("%q\n", en.Number(123456, i18n.Notation(i18n.NotationEngineering)))
	fmt.Printf("%q\n", en.Number(2.5, i18n.MaxFractionDigits(0)))
	// Output:
	// "-€1,234.50"
	// "-1.234,50\u00a0€"
	// "-1234,50\u00a0€"
	// "($1,234.50)"
	// "USD\u00a01,234.50"
	// "1.00 US dollars"
	// "5 российских рублей"
	// "2,00 złotego polskiego"
	// "+5"
	// "1.2M"
	// "1,2\u00a0Mio."
	// "123万"
	// "12\u00a0लाख"
	// "2 million"
	// "2 millones"
	// "1.235E5"
	// "123.456E3"
	// "2"
}
