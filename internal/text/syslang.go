package text

import (
	"os"
	"strings"
)

// The user's language, for the text that has no language of its own
// (ADR-269): the default of an app's text when it does not use package
// i18n, and the order of the regional CJK families. Package i18n reads
// the same source for its system locale (core.PlatformLanguage).

// PlatformLanguage returns the operating system's own locale name, such
// as "es-MX": on Windows the user's default locale
// (GetUserDefaultLocaleName); "" elsewhere and when it is not set. macOS
// apps read the environment, which a GUI app started from Finder usually
// does not have.
func PlatformLanguage() string { return platformLang() }

// platformLang is platformLanguage, replaceable by tests.
var platformLang = platformLanguage

// getenv is os.Getenv, replaceable by tests.
var getenv = os.Getenv

// SystemLanguage returns the user's language as a BCP 47 tag ("es-MX"),
// or "" when it is unknown: PlatformLanguage, else the POSIX environment,
// where the first of LC_ALL, LC_MESSAGES and LANG that is set decides
// (C and POSIX name no language). A POSIX locale name such as
// "es_MX.UTF-8@euro" is written as its tag, "es-MX".
func SystemLanguage() string {
	if s := posixToTag(PlatformLanguage()); s != "" {
		return s
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			return posixToTag(v)
		}
	}
	return ""
}

// posixToTag turns a POSIX locale name ("es_MX.UTF-8@euro") or a BCP 47
// tag into a tag ("es-MX"); "C", "POSIX" and "" give "".
func posixToTag(s string) string {
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(strings.TrimSpace(s), "_", "-")
	if s == "C" || s == "POSIX" {
		return ""
	}
	return s
}
