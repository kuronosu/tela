package tela

import "github.com/kuronosu/tela/internal/core"

// SetDefaultLang sets the language of the text that has no h.Lang above
// it, a BCP 47 tag such as "ja" or "pt-BR" (HTML's lang on the document):
// text shapes with its language (the Japanese, Chinese or Korean forms of
// a pan-CJK font) and falls back to the CJK fonts of its region. ""
// follows the user's language again (the system's locale; unknown in
// tests). Package i18n calls it with its current locale whenever that
// changes, so an app that uses i18n does not call it. It is safe from any
// goroutine; a tag that is not well formed panics.
func SetDefaultLang(tag string) { core.SetDefaultLang(tag) }

// DefaultLang returns the language of the text that has no h.Lang above
// it: the one SetDefaultLang set, else the user's language, else ""
// (unknown).
func DefaultLang() string { return core.DefaultLang() }
