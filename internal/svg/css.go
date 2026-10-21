package svg

import (
	"slices"
	"strings"
)

// Style sheets: a minimal subset of CSS for the <style> elements of a
// document (ADR-098).
//
// Supported selectors are compounds of a type selector or *, .class and
// #id (path.cls, #a.b, *), in selector lists separated by commas.
// Selectors with combinators, attribute selectors, pseudo-classes or
// pseudo-elements are skipped (they match nothing here, while a browser
// would match some elements); a rule whose selector list has no
// supported selector is dropped, and so is one with a syntax error. At
// rules (@media, @import, @font-face...) are skipped with their blocks.
// Declarations are limited to the properties of props.go.
//
// The cascade of one element: its presentation attributes, then the
// declarations of the rules that match it by specificity (ids, classes,
// types) and source order, then its style attribute; important
// declarations of the rules, then of the style attribute, come last
// (CSS Cascade 4 for author origins; presentation attributes count as
// author rules of specificity 0 before every sheet).

// maxRules bounds the rules of a document's style sheets; the rest are
// ignored.
const maxRules = 4096

// cssRule is one rule of a style sheet, for one selector of its list.
type cssRule struct {
	sel   selector
	order int // source order of the rule
	decls []cssDecl
}

type cssDecl struct {
	name, value string
	important   bool
}

// selector is a compound selector: an optional type (or *), classes and
// ids.
type selector struct {
	tag     string // "" for * or no type selector
	classes []string
	ids     []string
}

// specificity returns the selector's specificity as one comparable
// number (ids, classes, types; each count is small).
func (s *selector) specificity() int {
	t := 0
	if s.tag != "" {
		t = 1
	}
	return len(s.ids)<<20 | len(s.classes)<<10 | t
}

// matches reports whether the selector matches e.
func (s *selector) matches(e *element) bool {
	if s.tag != "" && s.tag != e.tag {
		return false
	}
	if len(s.ids) > 0 {
		id, ok := e.attr("id")
		if !ok {
			return false
		}
		for _, x := range s.ids {
			if x != id {
				return false
			}
		}
	}
	if len(s.classes) > 0 {
		cls, ok := e.attr("class")
		if !ok {
			return false
		}
		have := strings.Fields(cls)
		for _, c := range s.classes {
			if !slices.Contains(have, c) {
				return false
			}
		}
	}
	return true
}

// parseSheet appends the rules of the CSS text s to rules.
func parseSheet(s string, rules []cssRule) []cssRule {
	s = stripComments(s)
	order := len(rules)
	for len(s) > 0 && len(rules) < maxRules {
		s = strings.TrimLeft(s, " \t\r\n\f")
		if s == "" {
			break
		}
		if s[0] == '@' {
			s = skipAtRule(s)
			continue
		}
		open := strings.IndexByte(s, '{')
		if open < 0 {
			break
		}
		end := blockEnd(s, open)
		prelude, body := s[:open], s[open+1:end]
		if end < len(s) {
			s = s[end+1:]
		} else {
			s = ""
		}
		sels, ok := parseSelectorList(prelude)
		if !ok || len(sels) == 0 {
			continue
		}
		var decls []cssDecl
		forEachDeclaration(body, func(name, value string, important bool) {
			if _, ok := propNames[strings.ToLower(name)]; ok {
				decls = append(decls, cssDecl{strings.ToLower(name), value, important})
			}
		})
		if len(decls) == 0 {
			continue
		}
		for _, sel := range sels {
			rules = append(rules, cssRule{sel: sel, order: order, decls: decls})
		}
		order++
	}
	return rules
}

// stripComments removes /* */ comments; an unclosed one runs to the end.
func stripComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + " " + s[i+2+j+2:]
	}
}

// blockEnd returns the index of the brace that closes the block opened at
// s[open], or len(s) when it is not closed (CSS closes it at the end).
func blockEnd(s string, open int) int {
	depth := 0
	var quote byte
	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '{':
			depth++
		case c == '}':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return len(s)
}

// skipAtRule skips an at-rule: up to its semicolon, or its block.
func skipAtRule(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ';':
			return s[i+1:]
		case '{':
			end := blockEnd(s, i)
			if end < len(s) {
				return s[end+1:]
			}
			return ""
		}
	}
	return ""
}

// parseSelectorList parses a selector list. ok is false for a syntax
// error, which drops the rule; supported selectors are returned, the
// others skipped.
func parseSelectorList(s string) ([]selector, bool) {
	var out []selector
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		sel, supported, ok := parseCompound(part)
		if !ok {
			return nil, false
		}
		if supported {
			out = append(out, sel)
		}
	}
	return out, true
}

// parseCompound parses a compound selector. supported is false for valid
// selectors outside the subset (combinators, attributes, pseudo-classes).
func parseCompound(s string) (sel selector, supported, ok bool) {
	i := 0
	if s[0] == '*' {
		i = 1
	} else if isIdentStart(s[0]) {
		j := identEnd(s, 0)
		sel.tag, i = s[:j], j
	}
	for i < len(s) {
		c := s[i]
		switch c {
		case '.', '#':
			j := identEnd(s, i+1)
			if j == i+1 {
				return selector{}, false, false
			}
			if c == '.' {
				sel.classes = append(sel.classes, s[i+1:j])
			} else {
				sel.ids = append(sel.ids, s[i+1:j])
			}
			i = j
		case ' ', '\t', '\n', '\r', '\f', '>', '+', '~', '[', ':':
			// A combinator, an attribute selector or a pseudo-class:
			// valid CSS that this subset does not match.
			return selector{}, false, true
		default:
			return selector{}, false, false
		}
	}
	return sel, true, true
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '-' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// identEnd returns the end of the identifier starting at s[i].
func identEnd(s string, i int) int {
	for i < len(s) {
		c := s[i]
		if isIdentStart(c) || c >= '0' && c <= '9' {
			i++
			continue
		}
		if c == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		break
	}
	return i
}

// sheetIndex buckets rules by the most selective part of their selector,
// so that matching an element only tests the rules that can match it.
type sheetIndex struct {
	byID, byClass, byTag map[string][]*cssRule
	universal            []*cssRule
	tests                int // selector tests left (maxSelectorTests)
}

// maxSelectorTests bounds the selector tests of one document (elements ×
// candidate rules); beyond it Parse returns ErrTooComplex.
const maxSelectorTests = 4 << 20

func indexRules(rules []cssRule) *sheetIndex {
	ix := &sheetIndex{byID: map[string][]*cssRule{}, byClass: map[string][]*cssRule{}, byTag: map[string][]*cssRule{},
		tests: maxSelectorTests}
	for i := range rules {
		r := &rules[i]
		switch {
		case len(r.sel.ids) > 0:
			ix.byID[r.sel.ids[0]] = append(ix.byID[r.sel.ids[0]], r)
		case len(r.sel.classes) > 0:
			ix.byClass[r.sel.classes[0]] = append(ix.byClass[r.sel.classes[0]], r)
		case r.sel.tag != "":
			ix.byTag[r.sel.tag] = append(ix.byTag[r.sel.tag], r)
		default:
			ix.universal = append(ix.universal, r)
		}
	}
	return ix
}

// match returns the declarations of the rules matching e, in cascade
// order: by specificity, then source order.
func (ix *sheetIndex) match(e *element) []cssDecl {
	var hits []*cssRule
	try := func(rs []*cssRule) {
		ix.tests -= len(rs)
		if ix.tests < 0 {
			return
		}
		for _, r := range rs {
			if r.sel.matches(e) && !slices.Contains(hits, r) {
				hits = append(hits, r)
			}
		}
	}
	try(ix.universal)
	try(ix.byTag[e.tag])
	if id, ok := e.attr("id"); ok {
		try(ix.byID[id])
	}
	if cls, ok := e.attr("class"); ok {
		for _, c := range strings.Fields(cls) {
			try(ix.byClass[c])
		}
	}
	if len(hits) == 0 {
		return nil
	}
	slices.SortStableFunc(hits, func(a, b *cssRule) int {
		if sa, sb := a.sel.specificity(), b.sel.specificity(); sa != sb {
			return sa - sb
		}
		return a.order - b.order
	})
	var out []cssDecl
	for _, h := range hits {
		if ix.tests -= len(h.decls); ix.tests < 0 {
			return nil
		}
		out = append(out, h.decls...)
	}
	return out
}

// applySheet stores in every element of the tree rooted at e the
// declarations of the rules that match it. It returns ErrTooComplex when
// the document needs more than maxSelectorTests tests.
func applySheet(ix *sheetIndex, e *element) error {
	e.sheet = ix.match(e)
	if ix.tests < 0 {
		return ErrTooComplex
	}
	for _, ch := range e.children {
		if err := applySheet(ix, ch); err != nil {
			return err
		}
	}
	return nil
}
