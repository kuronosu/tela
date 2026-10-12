package style

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// This file parses the CSS values that can appear in arbitrary-value
// classes: numbers, lengths with calc(), min(), max() and clamp(), angles,
// times and the comma and space separated lists built from them.

// parseFloat parses a plain decimal number such as "12", "-0.5" or ".25".
// It rejects hexadecimal, exponent, NaN and Inf spellings.
func parseFloat(s string) (float32, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && c != '.' && !(i == 0 && (c == '-' || c == '+')) {
			return 0, false
		}
	}
	v, err := strconv.ParseFloat(s, 32)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return float32(v), true
}

// parseUint parses a non-negative decimal integer.
func parseUint(s string) (int, bool) {
	if s == "" || len(s) > 9 {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

// isNumber reports whether s is a plain decimal number.
func isNumber(s string) bool { _, ok := parseFloat(s); return ok }

// splitTop splits s at the byte sep, ignoring separators nested in
// parentheses, brackets or quotes. Empty parts are kept.
func splitTop(s string, sep byte) []string {
	var parts []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case c == sep && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// fieldsTop splits s at runs of spaces, ignoring spaces nested in
// parentheses or brackets.
func fieldsTop(s string) []string {
	var parts []string
	depth, start := 0, -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case (c == ' ' || c == '\t') && depth == 0:
			if start >= 0 {
				parts = append(parts, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		parts = append(parts, s[start:])
	}
	return parts
}

// unitSuffix splits "12.5px" into its number and unit.
func unitSuffix(s string) (num float32, unit string, ok bool) {
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	v, ok := parseFloat(s[:i])
	if !ok {
		return 0, "", false
	}
	return v, s[i:], true
}

var errNotLength = errors.New("not a length")

// lengthUnit converts a number with a unit into a length expression.
func lengthUnit(v float32, unit string) (lenExpr, error) {
	switch unit {
	case "px":
		return exprPx(v), nil
	case "rem":
		return exprRem(v), nil
	case "em":
		return exprEm(v), nil
	case "%":
		return exprPct(v), nil
	case "vw", "svw", "lvw", "dvw":
		return exprVw(v), nil
	case "vh", "svh", "lvh", "dvh":
		return exprVh(v), nil
	case "ch":
		return exprCh(v), nil
	case "":
		if v == 0 {
			return exprZero(), nil
		}
		return lenExpr{}, errNotLength
	case "cm", "mm", "in", "pt", "pc", "ex", "q", "Q", "vmin", "vmax", "lh", "rlh", "cap", "ic":
		return lenExpr{}, fmt.Errorf("the unit %q is not supported; use px, rem, em, %%, vw, vh or ch", unit)
	}
	return lenExpr{}, errNotLength
}

// cssToken is one lexical token of a calc() expression.
type cssToken struct {
	kind byte // 'n' number with optional unit, 'i' identifier, or the operator byte itself
	num  float32
	unit string
	text string
}

func lexCSS(s string) ([]cssToken, error) {
	var toks []cssToken
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '(' || c == ')' || c == ',' || c == '*' || c == '/':
			toks = append(toks, cssToken{kind: c})
			i++
		case c == '+' || c == '-':
			// A sign when the previous token is not a value; otherwise a
			// binary operator, even without surrounding spaces.
			prevValue := len(toks) > 0 && (toks[len(toks)-1].kind == 'n' || toks[len(toks)-1].kind == ')' || toks[len(toks)-1].kind == 'i')
			next := byte(0)
			if i+1 < len(s) {
				next = s[i+1]
			}
			if prevValue || !(next >= '0' && next <= '9' || next == '.') {
				toks = append(toks, cssToken{kind: c})
				i++
				continue
			}
			fallthrough
		case c >= '0' && c <= '9' || c == '.':
			j := i
			if s[j] == '-' || s[j] == '+' {
				j++
			}
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j++
			}
			v, ok := parseFloat(s[i:j])
			if !ok {
				return nil, fmt.Errorf("invalid number %q", s[i:j])
			}
			k := j
			if k < len(s) && s[k] == '%' {
				k++
			} else {
				for k < len(s) && (s[k] >= 'a' && s[k] <= 'z' || s[k] >= 'A' && s[k] <= 'Z') {
					k++
				}
			}
			toks = append(toks, cssToken{kind: 'n', num: v, unit: s[j:k]})
			i = k
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_':
			j := i
			for j < len(s) && (s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9' || s[j] == '-' || s[j] == '_') {
				j++
			}
			toks = append(toks, cssToken{kind: 'i', text: s[i:j]})
			i = j
		default:
			return nil, fmt.Errorf("unexpected character %q", string(c))
		}
	}
	return toks, nil
}

// calcVal is a value inside a calc() expression: a length or a plain number.
type calcVal struct {
	e      lenExpr
	scalar bool
	n      float32
}

type calcParser struct {
	toks     []cssToken
	pos      int
	allowPct bool
}

func (p *calcParser) peek() byte {
	if p.pos < len(p.toks) {
		return p.toks[p.pos].kind
	}
	return 0
}

func (p *calcParser) sum() (calcVal, error) {
	v, err := p.product()
	if err != nil {
		return v, err
	}
	for p.peek() == '+' || p.peek() == '-' {
		op := p.toks[p.pos].kind
		p.pos++
		w, err := p.product()
		if err != nil {
			return v, err
		}
		if v.scalar != w.scalar {
			return v, errors.New("cannot add or subtract a number and a length")
		}
		if op == '-' {
			w.e, w.n = w.e.scaled(-1), -w.n
		}
		v.e, v.n = v.e.add(w.e), v.n+w.n
	}
	return v, nil
}

func (p *calcParser) product() (calcVal, error) {
	v, err := p.factor()
	if err != nil {
		return v, err
	}
	for p.peek() == '*' || p.peek() == '/' {
		op := p.toks[p.pos].kind
		p.pos++
		w, err := p.factor()
		if err != nil {
			return v, err
		}
		if op == '/' {
			if !w.scalar || w.n == 0 {
				return v, errors.New("a length can only be divided by a non-zero number")
			}
			if v.scalar {
				v.n /= w.n
			} else {
				v.e = v.e.scaled(1 / w.n)
			}
			continue
		}
		switch {
		case v.scalar && w.scalar:
			v.n = float32(v.n * w.n)
		case v.scalar:
			v = calcVal{e: w.e.scaled(v.n)}
		case w.scalar:
			v.e = v.e.scaled(w.n)
		default:
			return v, errors.New("cannot multiply two lengths")
		}
	}
	return v, nil
}

func (p *calcParser) factor() (calcVal, error) {
	if p.pos >= len(p.toks) {
		return calcVal{}, errors.New("the expression ends early")
	}
	t := p.toks[p.pos]
	p.pos++
	switch t.kind {
	case 'n':
		if t.unit == "" {
			if t.num == 0 {
				return calcVal{e: exprZero(), scalar: true, n: 0}, nil
			}
			return calcVal{scalar: true, n: t.num}, nil
		}
		e, err := lengthUnit(t.num, t.unit)
		if err != nil {
			if err == errNotLength {
				return calcVal{}, fmt.Errorf("unknown unit %q", t.unit)
			}
			return calcVal{}, err
		}
		if e.pct != 0 && !p.allowPct {
			return calcVal{}, errors.New("a percentage is not allowed here")
		}
		return calcVal{e: e}, nil
	case '(':
		v, err := p.sum()
		if err != nil {
			return v, err
		}
		if p.peek() != ')' {
			return v, errors.New("missing ')'")
		}
		p.pos++
		return v, nil
	case 'i':
		if p.peek() != '(' {
			return calcVal{}, fmt.Errorf("unexpected %q", t.text)
		}
		p.pos++
		switch strings.ToLower(t.text) {
		case "calc":
			v, err := p.sum()
			if err != nil {
				return v, err
			}
			if p.peek() != ')' {
				return v, errors.New("missing ')'")
			}
			p.pos++
			return v, nil
		case "min", "max", "clamp":
			return p.function(strings.ToLower(t.text))
		case "var":
			return calcVal{}, errors.New("CSS variables are not supported; use a theme token or a literal value")
		}
		return calcVal{}, fmt.Errorf("the function %s() is not supported; use calc(), min(), max() or clamp()", t.text)
	}
	return calcVal{}, fmt.Errorf("unexpected %q", string(t.kind))
}

func (p *calcParser) function(name string) (calcVal, error) {
	var args []lenExpr
	for {
		// Percentages cannot be compared with lengths until layout.
		sub := calcParser{toks: p.toks, pos: p.pos, allowPct: false}
		v, err := sub.sum()
		if err != nil {
			if strings.Contains(err.Error(), "percentage") {
				return calcVal{}, fmt.Errorf("%s() with a percentage is not supported yet; use calc() or fixed lengths", name)
			}
			return calcVal{}, err
		}
		if v.scalar && v.n != 0 {
			return calcVal{}, fmt.Errorf("%s() needs lengths, not plain numbers", name)
		}
		p.pos = sub.pos
		args = append(args, v.e)
		if p.peek() == ',' {
			p.pos++
			continue
		}
		break
	}
	if p.peek() != ')' {
		return calcVal{}, errors.New("missing ')'")
	}
	p.pos++
	kind := fnMin
	switch name {
	case "max":
		kind = fnMax
	case "clamp":
		kind = fnClamp
		if len(args) != 3 {
			return calcVal{}, errors.New("clamp() takes three arguments: clamp(min, value, max)")
		}
	}
	if kind != fnClamp && len(args) < 2 {
		return calcVal{}, fmt.Errorf("%s() takes at least two arguments", name)
	}
	return calcVal{e: lenExpr{fn: &lenFn{kind: kind, args: args}}}, nil
}

// parseLengthExpr parses a length: a number with a unit, a percentage
// (when allowPct), 0, calc(), min(), max() or clamp().
func parseLengthExpr(s string, allowPct bool) (lenExpr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return lenExpr{}, errNotLength
	}
	// Fast path: a single number with a unit.
	if v, unit, ok := unitSuffix(s); ok {
		e, err := lengthUnit(v, unit)
		if err != nil {
			return lenExpr{}, err
		}
		if e.pct != 0 && !allowPct {
			return lenExpr{}, errors.New("a percentage is not allowed here")
		}
		return e, nil
	}
	if !strings.ContainsAny(s, "(") {
		return lenExpr{}, errNotLength
	}
	toks, err := lexCSS(s)
	if err != nil {
		return lenExpr{}, err
	}
	p := calcParser{toks: toks, allowPct: allowPct}
	v, err := p.sum()
	if err != nil {
		return lenExpr{}, err
	}
	if p.pos != len(toks) {
		return lenExpr{}, errors.New("unexpected text after the expression")
	}
	if v.scalar && v.n != 0 {
		return lenExpr{}, errors.New("the expression is a number, not a length")
	}
	return v.e, nil
}

// looksLikeLength reports whether s starts like a CSS length value, for
// the type inference of arbitrary values (text-[14px] versus text-[#fff]).
func looksLikeLength(s string) bool {
	if v, unit, ok := unitSuffix(s); ok {
		_ = v
		switch unit {
		case "px", "rem", "em", "%", "vw", "vh", "svw", "lvw", "dvw", "svh", "lvh", "dvh", "ch",
			"cm", "mm", "in", "pt", "pc", "ex", "vmin", "vmax":
			return true
		}
		return unit == "" && v == 0
	}
	for _, f := range []string{"calc(", "min(", "max(", "clamp("} {
		if strings.HasPrefix(s, f) {
			return true
		}
	}
	return false
}

// parseAngle parses an angle with a deg, rad, grad or turn unit (a bare 0
// is allowed) and returns it in degrees.
func parseAngle(s string) (float32, bool) {
	v, unit, ok := unitSuffix(strings.TrimSpace(s))
	if !ok {
		return 0, false
	}
	switch unit {
	case "deg":
		return v, true
	case "rad":
		return float32(float64(v) * 180 / math.Pi), true
	case "grad":
		return float32(v * 0.9), true
	case "turn":
		return float32(v * 360), true
	case "":
		return v, v == 0
	}
	return 0, false
}

// parseTime parses "150ms" or "0.2s".
func parseTime(s string) (time.Duration, bool) {
	v, unit, ok := unitSuffix(strings.TrimSpace(s))
	if !ok {
		return 0, false
	}
	switch unit {
	case "ms":
		return time.Duration(float64(v) * float64(time.Millisecond)), true
	case "s":
		return time.Duration(float64(v) * float64(time.Second)), true
	}
	return 0, false
}
