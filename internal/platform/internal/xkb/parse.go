package xkb

import (
	"fmt"
	"strings"
)

// This file parses the XKB text format into a small syntax tree, following
// the grammar of libxkbcommon's parser.y. Sections are kept apart; the
// compiler (keymap.go) gives the statements their meaning.

type exprKind uint8

const (
	eIdent   exprKind = iota // name ("elem.field" for field references), optional index
	eNum                     // integer
	eFloat                   // decimal with a fraction (geometry, skipped)
	eString                  // string literal
	eKeyName                 // <NAME>
	eBinary                  // lhs op rhs (+ - * /)
	eUnary                   // op rhs (- + ! ~)
	eCall                    // name(args)
	eList                    // [args]
	eBraces                  // {args}: several keysyms on one level
	eAssign                  // lhs = rhs, inside a call's arguments
)

type expr struct {
	kind  exprKind
	op    byte
	name  string // identifier, string, key name, or call name
	num   int64
	index *expr
	lhs   *expr
	rhs   *expr
	args  []*expr
	line  int
}

type stmtKind uint8

const (
	sVar     stmtKind = iota // lhs [= rhs]; or !lhs; (key bodies: a bare list has no lhs)
	sVMods                   // virtual_modifiers a, b = expr
	sKeycode                 // <NAME> = number
	sAlias                   // alias <A> = <B>
	sType                    // type "NAME" { body }
	sInterp                  // interpret head { body }
	sKey                     // key <NAME> { body }
	sModMap                  // modifier_map Name { list }
	sIgnored                 // statements without meaning for a client (indicators, groups, geometry)
)

type stmt struct {
	kind stmtKind
	line int
	lhs  *expr // sVar: the field; sModMap: the modifier
	rhs  *expr // sVar: the value (nil for a bare flag); sKeycode: number; sInterp: head
	neg  bool  // sVar: !field
	name string
	body []stmt
	list []*expr // sModMap entries; sVMods values (eAssign or eIdent)
}

// section is one xkb_* section of a keymap.
type section struct {
	kind  string // "keycodes", "types", "compat", "symbols", "geometry"
	name  string
	stmts []stmt
}

type parser struct {
	lex  lexer
	tok  token // current token
	peek []token
}

func (p *parser) advance() error {
	if len(p.peek) > 0 {
		p.tok = p.peek[0]
		p.peek = p.peek[1:]
		return nil
	}
	t, err := p.lex.next()
	if err != nil {
		return err
	}
	p.tok = t
	return nil
}

// lookahead returns the token after the current one.
func (p *parser) lookahead() (token, error) {
	if len(p.peek) == 0 {
		t, err := p.lex.next()
		if err != nil {
			return token{}, err
		}
		p.peek = append(p.peek, t)
	}
	return p.peek[0], nil
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("xkb: line %d: %s", p.tok.line, fmt.Sprintf(format, args...))
}

func (p *parser) isPunct(s string) bool { return p.tok.kind == tPunct && p.tok.text == s }

func (p *parser) expect(s string) error {
	if !p.isPunct(s) {
		return p.errorf("expected %q, found %v", s, p.tok)
	}
	return p.advance()
}

// parseKeymap parses a whole keymap: "xkb_keymap { sections };" or bare
// sections.
func parseKeymap(src []byte) ([]section, error) {
	p := &parser{lex: lexer{src: src, line: 1}}
	if err := p.advance(); err != nil {
		return nil, err
	}
	var secs []section
	wrapped := false
	if err := p.skipFlags(); err != nil {
		return nil, err
	}
	if p.tok.kind == tIdent && strings.EqualFold(p.tok.text, "xkb_keymap") {
		wrapped = true
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.tok.kind == tString {
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
		if err := p.expect("{"); err != nil {
			return nil, err
		}
	}
	for {
		if wrapped && p.isPunct("}") {
			if err := p.advance(); err != nil {
				return nil, err
			}
			if p.isPunct(";") {
				if err := p.advance(); err != nil {
					return nil, err
				}
			}
			break
		}
		if p.tok.kind == tEOF {
			if wrapped {
				return nil, p.errorf("unterminated xkb_keymap block")
			}
			break
		}
		if p.isPunct(";") {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		s, err := p.parseSection()
		if err != nil {
			return nil, err
		}
		secs = append(secs, s)
	}
	return secs, nil
}

// skipFlags skips the flags that may precede a section or keymap
// (default, partial, hidden, alphanumeric_keys, …).
func (p *parser) skipFlags() error {
	for p.tok.kind == tIdent && !strings.HasPrefix(strings.ToLower(p.tok.text), "xkb_") {
		switch strings.ToLower(p.tok.text) {
		case "default", "partial", "hidden", "alphanumeric_keys", "modifier_keys", "keypad_keys",
			"function_keys", "alternate_group":
		default:
			return p.errorf("expected an xkb_* section, found %v", p.tok)
		}
		if err := p.advance(); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) parseSection() (section, error) {
	var s section
	if err := p.skipFlags(); err != nil {
		return s, err
	}
	if p.tok.kind != tIdent {
		return s, p.errorf("expected an xkb_* section, found %v", p.tok)
	}
	switch strings.ToLower(p.tok.text) {
	case "xkb_keycodes":
		s.kind = "keycodes"
	case "xkb_types":
		s.kind = "types"
	case "xkb_compatibility", "xkb_compat", "xkb_compatibility_map", "xkb_compat_map":
		s.kind = "compat"
	case "xkb_symbols":
		s.kind = "symbols"
	case "xkb_geometry":
		s.kind = "geometry"
	default:
		return s, p.errorf("unsupported section %v (a resolved keymap has xkb_keycodes, xkb_types, xkb_compatibility, xkb_symbols and optionally xkb_geometry)", p.tok)
	}
	if err := p.advance(); err != nil {
		return s, err
	}
	if p.tok.kind == tString {
		s.name = p.tok.text
		if err := p.advance(); err != nil {
			return s, err
		}
	}
	if err := p.expect("{"); err != nil {
		return s, err
	}
	if s.kind == "geometry" {
		// Geometry only describes the physical keyboard drawing.
		if err := p.skipBalanced(); err != nil {
			return s, err
		}
	} else {
		body, err := p.parseStmts()
		if err != nil {
			return s, err
		}
		s.stmts = body
	}
	if err := p.expect("}"); err != nil {
		return s, err
	}
	if p.isPunct(";") {
		return s, p.advance()
	}
	return s, nil
}

// skipBalanced skips tokens up to (not including) the "}" that closes the
// current block.
func (p *parser) skipBalanced() error {
	depth := 0
	for {
		switch {
		case p.tok.kind == tEOF:
			return p.errorf("unterminated block")
		case p.isPunct("{"):
			depth++
		case p.isPunct("}"):
			if depth == 0 {
				return nil
			}
			depth--
		}
		if err := p.advance(); err != nil {
			return err
		}
	}
}

// skipStmt skips one statement: tokens up to the ";" at depth 0 (or the
// closing "}" of the enclosing block, which is left in place).
func (p *parser) skipStmt() error {
	depth := 0
	for {
		switch {
		case p.tok.kind == tEOF:
			return p.errorf("unterminated statement")
		case p.isPunct("{"), p.isPunct("["), p.isPunct("("):
			depth++
		case p.isPunct("}"), p.isPunct("]"), p.isPunct(")"):
			if depth == 0 {
				return nil
			}
			depth--
		case p.isPunct(";") && depth == 0:
			return p.advance()
		}
		if err := p.advance(); err != nil {
			return err
		}
	}
}

// parseStmts parses statements up to (not including) a closing "}".
func (p *parser) parseStmts() ([]stmt, error) {
	var out []stmt
	for !p.isPunct("}") {
		if p.tok.kind == tEOF {
			return nil, p.errorf("unexpected end of keymap")
		}
		if p.isPunct(";") {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if s.kind != sIgnored {
			out = append(out, s)
		}
	}
	return out, nil
}

func (p *parser) endStmt() error {
	if p.isPunct("}") {
		return nil // libxkbcommon requires ";", but be lenient before "}"
	}
	return p.expect(";")
}

func (p *parser) parseStmt() (stmt, error) {
	s := stmt{line: p.tok.line}
	if p.tok.kind == tKeyName {
		// <NAME> = number;
		s.kind, s.name = sKeycode, p.tok.text
		if err := p.advance(); err != nil {
			return s, err
		}
		if err := p.expect("="); err != nil {
			return s, err
		}
		v, err := p.parseExpr()
		if err != nil {
			return s, err
		}
		s.rhs = v
		return s, p.endStmt()
	}
	if p.tok.kind != tIdent {
		if p.isPunct("!") || p.isPunct("~") {
			return p.parseVar()
		}
		return s, p.errorf("unexpected %v", p.tok)
	}
	kw := strings.ToLower(p.tok.text)
	next, err := p.lookahead()
	if err != nil {
		return s, err
	}
	switch kw {
	case "include", "augment", "override", "replace", "alternate":
		if next.kind == tString {
			return s, p.errorf("%s %q: includes are not supported; a Wayland keymap is fully resolved", kw, next.text)
		}
		if kw != "include" {
			// Merge mode prefix: the statement follows.
			if err := p.advance(); err != nil {
				return s, err
			}
			return p.parseStmt()
		}
	case "default":
		if next.kind == tIdent {
			if err := p.advance(); err != nil {
				return s, err
			}
			return p.parseStmt()
		}
	case "virtual_modifiers", "virtualmodifiers":
		return p.parseVMods()
	case "alias":
		if err := p.advance(); err != nil {
			return s, err
		}
		if p.tok.kind != tKeyName {
			return s, p.errorf("expected a key name after alias")
		}
		s.kind, s.name = sAlias, p.tok.text
		if err := p.advance(); err != nil {
			return s, err
		}
		if err := p.expect("="); err != nil {
			return s, err
		}
		if p.tok.kind != tKeyName {
			return s, p.errorf("expected a key name in alias")
		}
		s.rhs = &expr{kind: eKeyName, name: p.tok.text}
		if err := p.advance(); err != nil {
			return s, err
		}
		return s, p.endStmt()
	case "type":
		if next.kind == tString {
			if err := p.advance(); err != nil {
				return s, err
			}
			s.kind, s.name = sType, p.tok.text
			return p.parseBlock(s, false)
		}
	case "interpret":
		if !(next.kind == tPunct && next.text == ".") {
			if err := p.advance(); err != nil {
				return s, err
			}
			head, err := p.parseExpr()
			if err != nil {
				return s, err
			}
			s.kind, s.rhs = sInterp, head
			return p.parseBlockBody(s, false)
		}
	case "key":
		if next.kind == tKeyName {
			if err := p.advance(); err != nil {
				return s, err
			}
			s.kind, s.name = sKey, p.tok.text
			return p.parseBlock(s, true)
		}
	case "modifier_map", "modmap", "mod_map":
		if err := p.advance(); err != nil {
			return s, err
		}
		if p.tok.kind != tIdent {
			return s, p.errorf("expected a modifier name after modifier_map")
		}
		s.kind, s.lhs = sModMap, &expr{kind: eIdent, name: p.tok.text}
		if err := p.advance(); err != nil {
			return s, err
		}
		if err := p.expect("{"); err != nil {
			return s, err
		}
		list, err := p.parseExprList("}")
		if err != nil {
			return s, err
		}
		s.list = list
		return s, p.endStmt()
	case "indicator", "group", "virtual", "shape", "section", "doodad", "outline", "solid", "text",
		"logo", "overlay", "row", "keys":
		// Indicators (LEDs), compat group maps and geometry statements do
		// not affect what keys produce.
		if !(next.kind == tPunct && (next.text == "." || next.text == "=")) || kw == "indicator" || kw == "group" {
			s.kind = sIgnored
			if err := p.skipStmt(); err != nil {
				return s, err
			}
			return s, nil
		}
	}
	return p.parseVar()
}

// parseBlock parses "{ body };" for a type, interpret or key statement.
// Key bodies are comma-separated, the others semicolon-separated.
func (p *parser) parseBlock(s stmt, commas bool) (stmt, error) {
	if err := p.advance(); err != nil { // the name
		return s, err
	}
	return p.parseBlockBody(s, commas)
}

// parseBlockBody parses "{ body };" with the current token on "{".
func (p *parser) parseBlockBody(s stmt, commas bool) (stmt, error) {
	if err := p.expect("{"); err != nil {
		return s, err
	}
	if commas {
		for !p.isPunct("}") {
			if p.isPunct(",") || p.isPunct(";") {
				if err := p.advance(); err != nil {
					return s, err
				}
				continue
			}
			if p.isPunct("[") {
				v, err := p.parseExpr()
				if err != nil {
					return s, err
				}
				s.body = append(s.body, stmt{kind: sVar, line: v.line, rhs: v})
				continue
			}
			v, err := p.parseVarNoEnd()
			if err != nil {
				return s, err
			}
			s.body = append(s.body, v)
		}
	} else {
		body, err := p.parseStmts()
		if err != nil {
			return s, err
		}
		s.body = body
	}
	if err := p.expect("}"); err != nil {
		return s, err
	}
	return s, p.endStmt()
}

func (p *parser) parseVar() (stmt, error) {
	s, err := p.parseVarNoEnd()
	if err != nil {
		return s, err
	}
	return s, p.endStmt()
}

// parseVarNoEnd parses "lhs = expr", "lhs", "!lhs" or "~lhs".
func (p *parser) parseVarNoEnd() (stmt, error) {
	s := stmt{kind: sVar, line: p.tok.line}
	if p.isPunct("!") || p.isPunct("~") {
		s.neg = true
		if err := p.advance(); err != nil {
			return s, err
		}
	}
	lhs, err := p.parseLhs()
	if err != nil {
		return s, err
	}
	s.lhs = lhs
	if !s.neg && p.isPunct("=") {
		if err := p.advance(); err != nil {
			return s, err
		}
		v, err := p.parseExpr()
		if err != nil {
			return s, err
		}
		s.rhs = v
	}
	return s, nil
}

// parseLhs parses "ident", "ident.ident", optionally followed by "[expr]".
func (p *parser) parseLhs() (*expr, error) {
	if p.tok.kind != tIdent {
		return nil, p.errorf("expected a field name, found %v", p.tok)
	}
	e := &expr{kind: eIdent, name: p.tok.text, line: p.tok.line}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.isPunct(".") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.tok.kind != tIdent {
			return nil, p.errorf("expected a field name after %q.", e.name)
		}
		e.name += "." + p.tok.text
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	if p.isPunct("[") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		idx, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		e.index = idx
		if err := p.expect("]"); err != nil {
			return nil, err
		}
	}
	return e, nil
}

func (p *parser) parseVMods() (stmt, error) {
	s := stmt{kind: sVMods, line: p.tok.line}
	if err := p.advance(); err != nil {
		return s, err
	}
	for {
		if p.tok.kind != tIdent {
			return s, p.errorf("expected a virtual modifier name, found %v", p.tok)
		}
		e := &expr{kind: eIdent, name: p.tok.text, line: p.tok.line}
		if err := p.advance(); err != nil {
			return s, err
		}
		if p.isPunct("=") {
			if err := p.advance(); err != nil {
				return s, err
			}
			v, err := p.parseExpr()
			if err != nil {
				return s, err
			}
			e = &expr{kind: eAssign, lhs: e, rhs: v, line: e.line}
		}
		s.list = append(s.list, e)
		if !p.isPunct(",") {
			break
		}
		if err := p.advance(); err != nil {
			return s, err
		}
	}
	return s, p.endStmt()
}

// parseExprList parses comma-separated expressions up to the closing
// punctuation close, which it consumes.
func (p *parser) parseExprList(close string) ([]*expr, error) {
	var list []*expr
	for {
		if p.isPunct(close) {
			return list, p.advance()
		}
		var e *expr
		var err error
		if p.isPunct("{") {
			e, err = p.parseBraces()
		} else {
			e, err = p.parseExpr()
		}
		if err != nil {
			return nil, err
		}
		if close == ")" && p.isPunct("=") { // call argument: field=value
			if err := p.advance(); err != nil {
				return nil, err
			}
			v, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			e = &expr{kind: eAssign, lhs: e, rhs: v, line: e.line}
		}
		list = append(list, e)
		if p.isPunct(",") {
			if err := p.advance(); err != nil {
				return nil, err
			}
			continue
		}
		if !p.isPunct(close) {
			return nil, p.errorf("expected %q or \",\", found %v", close, p.tok)
		}
	}
}

func (p *parser) parseBraces() (*expr, error) {
	line := p.tok.line
	if err := p.advance(); err != nil {
		return nil, err
	}
	list, err := p.parseExprList("}")
	if err != nil {
		return nil, err
	}
	return &expr{kind: eBraces, args: list, line: line}, nil
}

func (p *parser) parseExpr() (*expr, error) {
	l, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for p.isPunct("+") || p.isPunct("-") {
		op := p.tok.text[0]
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		l = &expr{kind: eBinary, op: op, lhs: l, rhs: r, line: l.line}
	}
	return l, nil
}

func (p *parser) parseMul() (*expr, error) {
	l, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.isPunct("*") || p.isPunct("/") {
		op := p.tok.text[0]
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		l = &expr{kind: eBinary, op: op, lhs: l, rhs: r, line: l.line}
	}
	return l, nil
}

func (p *parser) parseUnary() (*expr, error) {
	if p.isPunct("-") || p.isPunct("+") || p.isPunct("!") || p.isPunct("~") {
		op, line := p.tok.text[0], p.tok.line
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &expr{kind: eUnary, op: op, rhs: r, line: line}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (*expr, error) {
	t := p.tok
	switch t.kind {
	case tNumber:
		return &expr{kind: eNum, num: t.num, name: t.text, line: t.line}, p.advance()
	case tFloat:
		return &expr{kind: eFloat, name: t.text, line: t.line}, p.advance()
	case tString:
		return &expr{kind: eString, name: t.text, line: t.line}, p.advance()
	case tKeyName:
		return &expr{kind: eKeyName, name: t.text, line: t.line}, p.advance()
	case tIdent:
		next, err := p.lookahead()
		if err != nil {
			return nil, err
		}
		if next.kind == tPunct && next.text == "(" {
			if err := p.advance(); err != nil {
				return nil, err
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
			args, err := p.parseExprList(")")
			if err != nil {
				return nil, err
			}
			return &expr{kind: eCall, name: t.text, args: args, line: t.line}, nil
		}
		return p.parseLhs()
	case tPunct:
		switch t.text {
		case "(":
			if err := p.advance(); err != nil {
				return nil, err
			}
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			return e, p.expect(")")
		case "[":
			if err := p.advance(); err != nil {
				return nil, err
			}
			args, err := p.parseExprList("]")
			if err != nil {
				return nil, err
			}
			return &expr{kind: eList, args: args, line: t.line}, nil
		case "{":
			return p.parseBraces()
		}
	}
	return nil, p.errorf("unexpected %v in an expression", t)
}
