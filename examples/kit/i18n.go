package main

import (
	"embed"

	"github.com/kuronosu/tela/i18n"
)

// The gallery speaks three languages: English, Spanish and Arabic (which
// also lays the window out right to left). Their catalogs are
// locales/*.json, ICU MessageFormat embedded here and loaded at startup.
// They hold the Internationalization page's messages and the text that
// kit's components default to English: the date pickers' placeholders and
// month buttons, the dialogs' close button and the alert dialog's Cancel
// and Continue. The gallery's own titles, notes and captions stay English.

//go:embed locales/*.json
var locales embed.FS

func init() {
	if err := i18n.Load(locales, "locales/*.json"); err != nil {
		panic(err)
	}
}

// languages are the locales of the header's Language menu, in its order.
// Each is named in its own language by the catalog's "language" key.
var languages = []string{"en", "es", "ar"}

// kitText is kit's default English text in one language, from the catalog.
// Pages pass it to the fields that kit has for it (Placeholder,
// PreviousLabel, CloseLabel...).
type kitText struct {
	PickDate, PickDateRange string
	Previous, Next          string
	Close, Cancel, Continue string
	// Narrow asks the calendars for one-letter weekday names: Arabic's short
	// ones are as long as words and do not fit a 32 px cell.
	Narrow bool
}

// kitTextFor reads the kit.* keys of the formatter's language.
func kitTextFor(f i18n.Formatter) kitText {
	return kitText{
		PickDate:      f.T("kit.pickDate"),
		PickDateRange: f.T("kit.pickDateRange"),
		Previous:      f.T("kit.previous"),
		Next:          f.T("kit.next"),
		Close:         f.T("kit.close"),
		Cancel:        f.T("kit.cancel"),
		Continue:      f.T("kit.continue"),
		Narrow:        f.Locale() == "ar",
	}
}

// useKitText is kitTextFor for the language of the window; the component
// builds again when it changes.
func useKitText() kitText { return kitTextFor(i18n.UseFormatter()) }
