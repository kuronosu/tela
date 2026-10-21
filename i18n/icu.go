package i18n

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ICU MessageFormat parser (https://unicode-org.github.io/icu/userguide/format_parse/messages/).
//
// A message is text with arguments in braces:
//
//	{name}                                     simple argument
//	{n, number}  {n, number, integer|percent}  number
//	{d, date, short|medium|long|full}          date (also: time)
//	{n, plural, offset:1 =0 {none} one {# file} other {# files}}
//	{n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}
//	{g, select, female {her} male {his} other {their}}
//
// Cases hold messages, which may nest arguments. In a plural, # stands for
// the number minus the offset, formatted for the locale; inside a select
// nested in a plural it still does. Apostrophes follow ICU's default
// "double optional" mode: '' is an apostrophe, and a single apostrophe
// starts quoted text only when it comes right before { or } (or # in a
// plural), up to the next single apostrophe; any other apostrophe is
// literal, so "don't" needs no escaping.

type nodeKind uint8

const (
	nodeText   nodeKind = iota // literal text
	nodeArg                    // {name}, {name, number|date|time[, style]}
	nodePlural                 // plural and selectordinal
	nodeSelect                 // select
	nodePound                  // # inside a plural
)

// argType is what an argument node formats.
type argType uint8

const (
	argSimple argType = iota
	argNumber
	argDate
	argTime
)

type node struct {
	kind  nodeKind
	text  string // nodeText
	name  string // argument name (nodeArg, nodePlural, nodeSelect)
	typ   argType
	style string // number, date and time style ("" = default)

	ordinal bool    // nodePlural: selectordinal
	offset  float64 // nodePlural
	cases   []icuCase
	other   int // index of the "other" case
}

// icuCase is one case of a plural or select.
type icuCase struct {
	selector string  // "one", "female"... ("=N" for explicit values: "")
	exact    bool    // an explicit value: =N
	value    float64 // =N
	body     []node
}

// ParseError describes a syntax error in a message.
type parseError struct {
	msg       string
	line, col int
}

func (e *parseError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.line, e.col, e.msg)
}

type parser struct {
	src string
	pos int
}

// parseMessage parses a whole message.
func parseMessage(src string) (nodes []node, err error) {
	p := &parser{src: src}
	defer func() {
		if r := recover(); r != nil {
			pe, ok := r.(*parseError)
			if !ok {
				panic(r)
			}
			nodes, err = nil, pe
		}
	}()
	nodes = p.parseBody(false, false)
	return nodes, nil
}

func (p *parser) fail(pos int, format string, args ...any) {
	line, col := 1, 1
	for i := 0; i < pos && i < len(p.src); {
		r, n := utf8.DecodeRuneInString(p.src[i:])
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
		i += n
	}
	panic(&parseError{msg: fmt.Sprintf(format, args...), line: line, col: col})
}

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) skipSpace() {
	for !p.eof() {
		r, n := utf8.DecodeRuneInString(p.src[p.pos:])
		if !unicode.IsSpace(r) {
			return
		}
		p.pos += n
	}
}

// parseBody parses text and arguments up to the closing brace of the
// enclosing case (nested) or the end of the source. inPlural is whether #
// is a number.
func (p *parser) parseBody(nested, inPlural bool) []node {
	var nodes []node
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			nodes = append(nodes, node{kind: nodeText, text: text.String()})
			text.Reset()
		}
	}
	for !p.eof() {
		c := p.src[p.pos]
		switch c {
		case '\'':
			p.apostrophe(&text, inPlural)
		case '{':
			flush()
			nodes = append(nodes, p.parseArg(inPlural))
		case '}':
			if nested {
				flush()
				return nodes
			}
			p.fail(p.pos, "unmatched '}'; to write a literal brace, quote it: '}'")
		case '#':
			if inPlural {
				flush()
				nodes = append(nodes, node{kind: nodePound})
			} else {
				text.WriteByte('#')
			}
			p.pos++
		default:
			text.WriteByte(c)
			p.pos++
		}
	}
	if nested {
		p.fail(p.pos, "missing '}' at the end of the message")
	}
	flush()
	return nodes
}

// apostrophe handles a ' at p.pos (ICU's DOUBLE_OPTIONAL mode).
func (p *parser) apostrophe(text *strings.Builder, inPlural bool) {
	next := byte(0)
	if p.pos+1 < len(p.src) {
		next = p.src[p.pos+1]
	}
	switch {
	case next == '\'':
		text.WriteByte('\'')
		p.pos += 2
	case next == '{' || next == '}' || next == '#' && inPlural:
		// Quoted text: up to the next single apostrophe.
		p.pos++
		for !p.eof() {
			c := p.src[p.pos]
			if c == '\'' {
				if p.pos+1 < len(p.src) && p.src[p.pos+1] == '\'' {
					text.WriteByte('\'')
					p.pos += 2
					continue
				}
				p.pos++
				return
			}
			text.WriteByte(c)
			p.pos++
		}
	default:
		text.WriteByte('\'')
		p.pos++
	}
}

func isNameRune(r rune) bool {
	return !unicode.IsSpace(r) && !strings.ContainsRune("{},#'", r)
}

func (p *parser) readName() string {
	start := p.pos
	for !p.eof() {
		r, n := utf8.DecodeRuneInString(p.src[p.pos:])
		if !isNameRune(r) {
			break
		}
		p.pos += n
	}
	return p.src[start:p.pos]
}

func (p *parser) expect(c byte, what string) {
	if p.eof() || p.src[p.pos] != c {
		got := "the end of the message"
		if !p.eof() {
			got = strconv.QuoteRune(rune(p.src[p.pos]))
		}
		p.fail(p.pos, "expected %s, found %s", what, got)
	}
	p.pos++
}

var argTypeNames = "number, date, time, plural, selectordinal and select"

// parseArg parses an argument; p.pos is at its '{'.
func (p *parser) parseArg(inPlural bool) node {
	open := p.pos
	p.pos++
	p.skipSpace()
	name := p.readName()
	if name == "" {
		p.fail(p.pos, "expected an argument name after '{'; to write a literal brace, quote it: '{'")
	}
	p.skipSpace()
	if p.eof() {
		p.fail(open, "argument {%s is never closed with '}'", name)
	}
	if p.src[p.pos] == '}' {
		p.pos++
		return node{kind: nodeArg, name: name}
	}
	p.expect(',', "',' or '}' after the argument name")
	p.skipSpace()
	typStart := p.pos
	typ := p.readName()
	p.skipSpace()
	switch typ {
	case "number", "date", "time":
		n := node{kind: nodeArg, name: name}
		switch typ {
		case "number":
			n.typ = argNumber
		case "date":
			n.typ = argDate
		default:
			n.typ = argTime
		}
		if !p.eof() && p.src[p.pos] == ',' {
			p.pos++
			n.style = p.readStyle()
			p.checkStyle(n, typStart)
		}
		p.expect('}', "'}' to close the argument")
		return n
	case "plural", "selectordinal":
		return p.parsePluralOrSelect(name, typ, inPlural)
	case "select":
		return p.parsePluralOrSelect(name, typ, inPlural)
	case "":
		p.fail(p.pos, "expected an argument type after ',' (%s)", argTypeNames)
	}
	p.fail(typStart, "unknown argument type %q (use %s)", typ, argTypeNames)
	return node{}
}

// readStyle reads a number, date or time style up to the closing brace.
func (p *parser) readStyle() string {
	start := p.pos
	for !p.eof() && p.src[p.pos] != '}' {
		p.pos++
	}
	return strings.TrimSpace(p.src[start:p.pos])
}

var (
	numberStyles = []string{"integer", "percent"}
	dateStyles   = []string{"short", "medium", "long", "full"}
)

func (p *parser) checkStyle(n node, at int) {
	var ok []string
	switch n.typ {
	case argNumber:
		ok = numberStyles
	default:
		ok = dateStyles
	}
	for _, s := range ok {
		if n.style == s {
			return
		}
	}
	kind := map[argType]string{argNumber: "number", argDate: "date", argTime: "time"}[n.typ]
	p.fail(at, "unsupported %s style %q in {%s, %s, ...}; use one of %s (skeletons and patterns are not supported)",
		kind, n.style, n.name, kind, strings.Join(ok, ", "))
}

var pluralKeywords = map[string]bool{"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true}

func (p *parser) parsePluralOrSelect(name, typ string, outerPlural bool) node {
	isSelect := typ == "select"
	n := node{kind: nodePlural, name: name, ordinal: typ == "selectordinal"}
	if isSelect {
		n.kind = nodeSelect
	}
	p.expect(',', "',' after "+typ)
	p.skipSpace()
	if !isSelect && strings.HasPrefix(p.src[p.pos:], "offset:") {
		p.pos += len("offset:")
		p.skipSpace()
		start := p.pos
		for !p.eof() && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.') {
			p.pos++
		}
		v, err := strconv.ParseFloat(p.src[start:p.pos], 64)
		if err != nil {
			p.fail(start, "offset: must be followed by a number")
		}
		n.offset = v
		p.skipSpace()
	}
	n.other = -1
	seen := map[string]bool{}
	for {
		p.skipSpace()
		if p.eof() {
			p.fail(p.pos, "{%s, %s, ...} is never closed with '}'", name, typ)
		}
		if p.src[p.pos] == '}' {
			p.pos++
			break
		}
		selStart := p.pos
		var c icuCase
		if !isSelect && p.src[p.pos] == '=' {
			p.pos++
			start := p.pos
			for !p.eof() && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.' || p.src[p.pos] == '-') {
				p.pos++
			}
			v, err := strconv.ParseFloat(p.src[start:p.pos], 64)
			if err != nil {
				p.fail(selStart, "'=' must be followed by a number, as in =0 {...}")
			}
			c.exact, c.value = true, v
			c.selector = p.src[selStart:p.pos]
		} else {
			c.selector = p.readName()
			if c.selector == "" {
				p.fail(p.pos, "expected a case selector (%s) or '}' in {%s, %s, ...}", selectorHint(isSelect), name, typ)
			}
			if !isSelect && !pluralKeywords[c.selector] {
				p.fail(selStart, "unknown plural category %q in {%s, %s, ...}; use =N, zero, one, two, few, many or other", c.selector, name, typ)
			}
		}
		if seen[c.selector] {
			p.fail(selStart, "duplicate case %q in {%s, %s, ...}", c.selector, name, typ)
		}
		seen[c.selector] = true
		p.skipSpace()
		if p.eof() || p.src[p.pos] != '{' {
			p.fail(p.pos, "expected '{' after the case selector %q", c.selector)
		}
		p.pos++
		c.body = p.parseBody(true, !isSelect || outerPlural)
		p.expect('}', "'}' to close the case")
		if c.selector == "other" {
			n.other = len(n.cases)
		}
		n.cases = append(n.cases, c)
	}
	if n.other < 0 {
		p.fail(p.pos, "{%s, %s, ...} needs an \"other\" case: other {...}", name, typ)
	}
	return n
}

func selectorHint(isSelect bool) string {
	if isSelect {
		return "a word, or other"
	}
	return "=N, zero, one, two, few, many or other"
}
