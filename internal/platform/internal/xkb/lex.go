package xkb

import (
	"fmt"
	"strconv"
)

// tokKind is the kind of a lexical token of the XKB text format.
type tokKind uint8

const (
	tEOF     tokKind = iota
	tIdent           // identifiers and keywords: key, Shift, dead_acute, …
	tNumber          // integer literal (decimal or 0x hex)
	tFloat           // decimal literal with a fraction (geometry only)
	tString          // "…", unescaped
	tKeyName         // <AE01>, without the brackets
	tPunct           // one of { } [ ] ( ) ; , = + - * / ! ~ .
)

type token struct {
	kind tokKind
	text string // identifier, string, key name or punctuation
	num  int64
	line int
}

func (t token) String() string {
	switch t.kind {
	case tEOF:
		return "end of keymap"
	case tString:
		return strconv.Quote(t.text)
	case tKeyName:
		return "<" + t.text + ">"
	case tNumber:
		return strconv.FormatInt(t.num, 10)
	}
	return strconv.Quote(t.text)
}

// lexer splits a keymap into tokens. It follows libxkbcommon's scanner:
// comments start with // or # and run to the end of the line, or are
// /* … */; strings support the C escapes \\ \" \n \t \r \b \f \v \e and
// octal \NNN.
type lexer struct {
	src  []byte
	pos  int
	line int
}

func isIdentStart(c byte) bool { return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
func isIdentChar(c byte) bool  { return isIdentStart(c) || '0' <= c && c <= '9' }

func (l *lexer) errorf(format string, args ...any) error {
	return fmt.Errorf("xkb: line %d: %s", l.line, fmt.Sprintf(format, args...))
}

// next returns the next token.
func (l *lexer) next() (token, error) {
	src := l.src
	for l.pos < len(src) {
		c := src[l.pos]
		switch {
		case c == '\n':
			l.line++
			l.pos++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v' || c == 0:
			l.pos++
		case c == '#' || (c == '/' && l.pos+1 < len(src) && src[l.pos+1] == '/'):
			for l.pos < len(src) && src[l.pos] != '\n' {
				l.pos++
			}
		case c == '/' && l.pos+1 < len(src) && src[l.pos+1] == '*':
			l.pos += 2
			for {
				if l.pos+1 >= len(src) {
					return token{}, l.errorf("unterminated comment")
				}
				if src[l.pos] == '\n' {
					l.line++
				}
				if src[l.pos] == '*' && src[l.pos+1] == '/' {
					l.pos += 2
					break
				}
				l.pos++
			}
		default:
			return l.scan()
		}
	}
	return token{kind: tEOF, line: l.line}, nil
}

func (l *lexer) scan() (token, error) {
	src := l.src
	start := l.pos
	c := src[start]
	tok := token{line: l.line}
	switch {
	case isIdentStart(c):
		for l.pos < len(src) && isIdentChar(src[l.pos]) {
			l.pos++
		}
		tok.kind, tok.text = tIdent, string(src[start:l.pos])
		return tok, nil
	case '0' <= c && c <= '9':
		// Numbers, or identifiers that start with a digit (keysym names
		// such as 3270_Attn).
		for l.pos < len(src) && (isIdentChar(src[l.pos]) || src[l.pos] == '.') {
			l.pos++
		}
		s := string(src[start:l.pos])
		if v, err := strconv.ParseInt(s, 0, 64); err == nil && !(len(s) > 1 && s[0] == '0' && s[1] != 'x' && s[1] != 'X') {
			tok.kind, tok.num, tok.text = tNumber, v, s
			return tok, nil
		}
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			tok.kind, tok.num, tok.text = tNumber, v, s // decimal with leading zeros
			return tok, nil
		}
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			tok.kind, tok.text = tFloat, s
			return tok, nil
		}
		tok.kind, tok.text = tIdent, s
		return tok, nil
	case c == '"':
		l.pos++
		var buf []byte
		for {
			if l.pos >= len(src) || src[l.pos] == '\n' {
				return token{}, l.errorf("unterminated string")
			}
			c := src[l.pos]
			l.pos++
			if c == '"' {
				break
			}
			if c != '\\' || l.pos >= len(src) {
				buf = append(buf, c)
				continue
			}
			e := src[l.pos]
			l.pos++
			switch e {
			case 'n':
				buf = append(buf, '\n')
			case 't':
				buf = append(buf, '\t')
			case 'r':
				buf = append(buf, '\r')
			case 'b':
				buf = append(buf, '\b')
			case 'f':
				buf = append(buf, '\f')
			case 'v':
				buf = append(buf, '\v')
			case 'e':
				buf = append(buf, 0x1b)
			case '0', '1', '2', '3', '4', '5', '6', '7':
				v := int(e - '0')
				for n := 1; n < 3 && l.pos < len(src) && src[l.pos] >= '0' && src[l.pos] <= '7'; n++ {
					v = v*8 + int(src[l.pos]-'0')
					l.pos++
				}
				buf = append(buf, byte(v))
			default:
				buf = append(buf, e)
			}
		}
		tok.kind, tok.text = tString, string(buf)
		return tok, nil
	case c == '<':
		end := l.pos + 1
		for end < len(src) && src[end] != '>' && src[end] != '\n' && end-l.pos < 64 {
			end++
		}
		if end >= len(src) || src[end] != '>' {
			return token{}, l.errorf("malformed key name")
		}
		tok.kind, tok.text = tKeyName, string(src[l.pos+1:end])
		l.pos = end + 1
		return tok, nil
	}
	switch c {
	case '{', '}', '[', ']', '(', ')', ';', ',', '=', '+', '-', '*', '/', '!', '~', '.':
		l.pos++
		tok.kind, tok.text = tPunct, string(c)
		return tok, nil
	}
	return token{}, l.errorf("unexpected character %q", c)
}
