package i18n

import (
	"os"
	"strings"

	"golang.org/x/text/language"
)

// The system's locale: on Windows the user's default locale name
// (syslocale_windows.go); elsewhere, and on Windows when that is empty,
// the POSIX environment: LC_ALL, then LC_MESSAGES, then LANG. macOS reads
// the environment too for now (a GUI app launched from Finder usually has
// none, so it follows the fallback locale until SetLocale is called).

// osLocale returns the operating system's own locale name, or "".
var osLocale = platformLocale

// getenv is os.Getenv, replaceable by tests.
var getenv = os.Getenv

// systemLocale returns the system's locale and whether there is one.
func systemLocale() (language.Tag, bool) {
	if s := osLocale(); s != "" {
		if t, ok := parsePOSIXLocale(s); ok {
			return t, true
		}
	}
	// POSIX: the first of these that is set decides, even if it is "C".
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			return parsePOSIXLocale(v)
		}
	}
	return language.Und, false
}

// parsePOSIXLocale turns "es_MX.UTF-8@euro" (or a BCP 47 tag) into a tag.
// "C" and "POSIX" are not locales.
func parsePOSIXLocale(s string) (language.Tag, bool) {
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "_", "-")
	if s == "" || s == "C" || s == "POSIX" {
		return language.Und, false
	}
	t, err := language.Parse(s)
	if err != nil || t == language.Und {
		return language.Und, false
	}
	return t, true
}
