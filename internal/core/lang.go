package core

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/text"
)

// Language (ADR-269). An element's language comes from its Lang prop,
// HTML's lang attribute, and is inherited like CSS's direction:
// style.Compute resolves it into ComputedStyle.Text.Language (set by
// style.Input.Lang, at the root by style.Context.Lang). Text shapes with
// it (OpenType locl: the Japanese, Simplified or Traditional Chinese or
// Korean forms of a pan-CJK font) and falls back to the CJK fonts of its
// region. Text with no Lang above it has the app's default language
// (DefaultLang): the one SetDefaultLang sets, which package i18n keeps
// equal to its current locale, else the user's (text.SystemLanguage). A
// LangProvider (i18n.Provider) gives a language to its children without a
// box: the first primitives below it take it.

// Lang sets the language of a primitive and its subtree, like HTML's lang
// attribute: a BCP 47 tag such as "ja", "zh-Hans", "zh-TW" or "pt-BR". ""
// has no effect (the element inherits its parent's language). In dev mode
// a tag that is not well formed panics; otherwise it is ignored.
func Lang(tag string) Prop {
	ok := tag == "" || validLangTag(tag)
	if !ok && devNow() {
		panic(langMessage("Lang", tag))
	}
	if !ok {
		tag = ""
	}
	return Prop{name: "Lang", apply: func(d *desc) { d.lang = tag }}
}

// langMessage is the dev-mode message of a tag that is not well formed.
func langMessage(fn, tag string) string {
	hint := ""
	if fixed := strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"); fixed != tag && validLangTag(fixed) {
		hint = fmt.Sprintf("; did you mean %q?", fixed)
	}
	return fmt.Sprintf("tela: %s(%q): the language is a BCP 47 tag, such as \"en\", \"ja\", \"zh-Hans\" or \"pt-BR\" (subtags separated by hyphens)%s", fn, tag, hint)
}

// CheckLang returns the message of Lang(tag) for a tag that is not a well
// formed BCP 47 tag, or "".
func CheckLang(tag string) string {
	if tag == "" || validLangTag(tag) {
		return ""
	}
	return langMessage("Lang", tag)
}

// validLangTag reports whether tag is a well-formed BCP 47 language tag
// (RFC 5646, section 2.1): language (with extlangs), script, region,
// variants, extensions and private use in that order, ASCII
// case-insensitive, or a private-use or grandfathered tag. Whether the
// subtags are registered is not checked, as HTML does not.
func validLangTag(tag string) bool {
	if irregularTags[strings.ToLower(tag)] {
		return true
	}
	parts := strings.Split(tag, "-")
	for _, p := range parts {
		if len(p) == 0 || len(p) > 8 || !isTagAlnum(p) {
			return false
		}
	}
	i := 0
	if isPrivate(parts[0]) {
		return len(parts) > 1
	}
	// language: 2-3 letters with up to 3 extlangs, or 4-8 letters.
	switch l := len(parts[0]); {
	case !isTagAlpha(parts[0]):
		return false
	case l <= 3:
		if l < 2 {
			return false
		}
		i = 1
		for n := 0; n < 3 && i < len(parts) && len(parts[i]) == 3 && isTagAlpha(parts[i]); n++ {
			i++
		}
	default:
		i = 1
	}
	if i < len(parts) && len(parts[i]) == 4 && isTagAlpha(parts[i]) { // script
		i++
	}
	if i < len(parts) && (len(parts[i]) == 2 && isTagAlpha(parts[i]) || len(parts[i]) == 3 && isTagDigit(parts[i])) { // region
		i++
	}
	for i < len(parts) && (len(parts[i]) >= 5 || len(parts[i]) == 4 && isTagDigit(parts[i][:1])) { // variants
		i++
	}
	for i < len(parts) && len(parts[i]) == 1 && !isPrivate(parts[i]) { // extensions
		i++
		n := 0
		for i < len(parts) && len(parts[i]) >= 2 {
			i, n = i+1, n+1
		}
		if n == 0 {
			return false
		}
	}
	if i < len(parts) && isPrivate(parts[i]) {
		return i+1 < len(parts)
	}
	return i == len(parts)
}

// irregularTags are RFC 5646's grandfathered tags that the syntax does not
// cover.
var irregularTags = map[string]bool{
	"en-gb-oed": true, "i-ami": true, "i-bnn": true, "i-default": true, "i-enochian": true,
	"i-hak": true, "i-klingon": true, "i-lux": true, "i-mingo": true, "i-navajo": true,
	"i-pwn": true, "i-tao": true, "i-tay": true, "i-tsu": true, "sgn-be-fr": true,
	"sgn-be-nl": true, "sgn-ch-de": true,
}

func isPrivate(s string) bool { return s == "x" || s == "X" }

func isTagAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i] | 0x20; !(c >= 'a' && c <= 'z' || s[i] >= '0' && s[i] <= '9') {
			return false
		}
	}
	return true
}

func isTagAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i] | 0x20; c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

func isTagDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------
// The app's default language.

var (
	defaultLang atomic.Pointer[string] // SetDefaultLang's; nil follows the system

	sysLangOnce sync.Once
	sysLang     string
)

// systemLang returns the user's language, read once: text.SystemLanguage,
// or "" in a binary that links the deterministic font set (tests,
// telatest, the tela command), so that their output does not depend on
// the machine.
var systemLang = func() string {
	sysLangOnce.Do(func() {
		if text.HasDefaultFonts() {
			return
		}
		if l := text.SystemLanguage(); validLangTag(l) {
			sysLang = l
		}
	})
	return sysLang
}

// PlatformLanguage is text.PlatformLanguage: the operating system's own
// locale name (Windows), which package i18n reads for its system locale.
func PlatformLanguage() string { return text.PlatformLanguage() }

// SetDefaultLang sets the language of the text that has no Lang above it,
// a BCP 47 tag; "" follows the user's language again. Package i18n calls
// it with its current locale. The trees restyle when it changes. It is
// safe from any goroutine; a tag that is not well formed panics.
func SetDefaultLang(tag string) {
	if tag != "" && !validLangTag(tag) {
		panic(langMessage("SetDefaultLang", tag))
	}
	old := defaultLang.Load()
	if old == nil && tag == "" || old != nil && *old == tag {
		return
	}
	if tag == "" {
		defaultLang.Store(nil)
	} else {
		defaultLang.Store(&tag)
	}
	postUpdate(update{postTree: func(t *Tree) { t.syncLang() }})
}

// DefaultLang returns the language of the text that has no Lang above it:
// SetDefaultLang's, else the user's language, else "" (unknown).
func DefaultLang() string {
	if p := defaultLang.Load(); p != nil {
		return *p
	}
	return systemLang()
}

// syncLang brings the root language of t and of its child windows' trees
// to DefaultLang, restyling everything when it changed.
func (t *Tree) syncLang() {
	if l := DefaultLang(); l != t.cx.Lang {
		t.cx.Lang = l
		t.restyleAll()
	}
	for _, w := range t.wins {
		if v := w.view; v != nil {
			v.tree.syncLang()
		}
	}
}

// ---------------------------------------------------------------------
// Languages given without a box.

// langContext carries the language of a LangProvider.
var langContext = CreateContext("")

// LangProvider gives the language tag to the text of its children, like a
// Lang on each of the first primitives below it, without a box of its own
// (package i18n's Provider uses it). "" has no effect. A tag that is not
// well formed panics in dev mode and is ignored otherwise.
func LangProvider(tag string, children ...Child) Node {
	if tag != "" && !validLangTag(tag) {
		if devNow() {
			panic(langMessage("LangProvider", tag))
		}
		tag = ""
	}
	return langContext.Provider(tag, children...)
}

// styleLang returns the language primitive el sets for its style
// computation: its Lang prop, else the tag of the nearest LangProvider
// between it and its parent primitive, else "" (it inherits).
func styleLang(el *Element) string {
	if l := el.node.d.lang; l != "" {
		return l
	}
	for p := el.parent; p != nil && p.kind != elemPrimitive; p = p.parent {
		if p.prov != nil && p.comp.ctx == langContext {
			if l, _ := p.prov.value.(string); l != "" {
				return l
			}
		}
	}
	return ""
}

// langProviderChanged restyles the first primitives below the
// LangProvider el, whose tag changed.
func (t *Tree) langProviderChanged(el *Element) {
	var walk func(e *Element)
	walk = func(e *Element) {
		for _, c := range e.children {
			if c.kind == elemPrimitive {
				t.markStyle(c)
				continue
			}
			walk(c)
		}
	}
	walk(el)
}
