package style

import (
	"errors"
	"slices"
	"strings"
	"sync"
)

// Rule is one compiled class: the op it applies, under the variants that
// must hold.
type Rule struct {
	// Class is the class as written, with its variants.
	Class string
	Cond  Condition
	Op    Op
}

// Classes is a compiled class string: its rules in order of appearance.
// Classes are immutable and shared by the parser's cache.
type Classes struct {
	// Source is the class string that was compiled.
	Source string
	Rules  []Rule
	// Group and Peer report the marker classes "group" and "peer" (without
	// variants): the element is the group ancestor that group-hover: and
	// the like test, or a peer for peer-checked: and the like. They have
	// no style of their own.
	Group, Peer bool
	// Err holds an UnknownClassError, UnsupportedClassError or
	// InvalidClassError (inside a ClassErrors) for every class that could
	// not be compiled; the others are still in Rules.
	Err error
}

// DefaultCacheSize is the number of class strings a Parser interns before
// it starts discarding the least recently used generation.
const DefaultCacheSize = 4096

// Parser compiles class strings against a theme and interns the result:
// each distinct string is parsed once (ARCHITECTURE section 10). A Parser
// is safe for concurrent use.
type Parser struct {
	th *Theme

	mu       sync.RWMutex
	cur, old map[string]*Classes
	limit    int

	vocabOnce sync.Once
	vocab     []string
}

// NewParser returns a parser for the theme. The theme is copied: later
// changes to th do not affect the parser, so build a new parser when the
// theme changes.
func NewParser(th *Theme) *Parser {
	return &Parser{th: th.Clone(), cur: map[string]*Classes{}, limit: DefaultCacheSize}
}

var (
	defaultParserOnce sync.Once
	defaultParser     *Parser
)

// DefaultParser returns the process-wide parser of the default theme.
func DefaultParser() *Parser {
	defaultParserOnce.Do(func() { defaultParser = NewParser(DefaultTheme()) })
	return defaultParser
}

// Theme returns the parser's theme. It must not be modified.
func (p *Parser) Theme() *Theme { return p.th }

// Compile compiles a class string. The result is never nil; a string with
// invalid classes still yields the rules of the valid ones, with the
// problems in Classes.Err. Equal strings return the same *Classes (until
// the cache evicts it).
func (p *Parser) Compile(classes string) *Classes {
	p.mu.RLock()
	c, ok := p.cur[classes]
	p.mu.RUnlock()
	if ok {
		return c
	}
	p.mu.Lock()
	// A hit in the old generation moves to the current one, so a string in
	// use survives every rotation.
	if c, ok = p.cur[classes]; !ok {
		if c, ok = p.old[classes]; ok {
			p.store(classes, c)
		}
	}
	p.mu.Unlock()
	if ok {
		return c
	}
	c = p.parse(classes)
	p.mu.Lock()
	if prev, ok := p.cur[classes]; ok {
		c = prev
	} else if prev, ok := p.old[classes]; ok {
		c = prev
		p.store(classes, c)
	} else {
		p.store(classes, c)
	}
	p.mu.Unlock()
	return c
}

// store adds c to the current generation, rotating the generations when it
// is full. p.mu must be held for writing.
func (p *Parser) store(classes string, c *Classes) {
	if len(p.cur) >= p.limit {
		p.old, p.cur = p.cur, make(map[string]*Classes, p.limit)
	}
	p.cur[classes] = c
}

// Compile compiles a class string with the default parser.
func Compile(classes string) *Classes { return DefaultParser().Compile(classes) }

// Vocabulary returns every class the parser accepts that can be listed,
// sorted: the supported utilities expanded with the theme's keys, without
// the "DEFAULT" keys (ADR-037). Arbitrary values, fractions and the
// unbounded spacing scale are not enumerated beyond SpacingSuggestions.
func (p *Parser) Vocabulary() []string {
	p.vocabOnce.Do(func() { p.vocab = buildVocabulary(p.th) })
	return p.vocab
}

// Suggest returns the vocabulary class closest to class, or "".
func (p *Parser) Suggest(class string) string {
	return p.suggestToken(class)
}

// pctx is the context of parsing one class.
type pctx struct {
	th    *Theme
	class string // the whole token, for error messages
}

// parse compiles a whole class string.
func (p *Parser) parse(src string) *Classes {
	out := &Classes{Source: src}
	var errs ClassErrors
	for _, tok := range strings.Fields(src) {
		rule, err := p.parseToken(tok)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out.Rules = append(out.Rules, rule...)
		switch tok {
		case "group":
			out.Group = true
		case "peer":
			out.Peer = true
		}
	}
	if len(errs) > 0 {
		out.Err = errs
	}
	return out
}

// parseToken compiles one class (with variants) into rules.
func (p *Parser) parseToken(tok string) ([]Rule, error) {
	if strings.HasPrefix(tok, "!") || strings.HasSuffix(tok, "!") {
		return nil, &UnsupportedClassError{Class: tok,
			Reason: "!important is not supported (there is no cascade); later classes and Style props already win, so drop the \"!\""}
	}
	parts := splitTop(tok, ':')
	util := parts[len(parts)-1]
	var cond Condition
	for _, v := range parts[:len(parts)-1] {
		c, err := parseVariant(p.th, v)
		if err != nil {
			return nil, withVariantPrefix(err, tok)
		}
		cond = cond.and(c)
	}
	if util == "" {
		return nil, &InvalidClassError{Class: tok, Reason: "the class after the variants is empty"}
	}
	if strings.HasSuffix(util, "!") || strings.HasPrefix(util, "!") {
		return nil, &UnsupportedClassError{Class: tok, Reason: "!important is not supported; drop the \"!\""}
	}
	ops, err := p.resolveUtility(tok, util)
	if err != nil {
		return nil, err
	}
	rules := make([]Rule, len(ops))
	for i, o := range ops {
		rules[i] = Rule{Class: tok, Cond: cond, Op: o}
	}
	return rules, nil
}

// withVariantPrefix rewrites the class of a variant error to the whole token.
func withVariantPrefix(err error, tok string) error {
	var u *UnknownClassError
	if errors.As(err, &u) && u.Variant {
		// Report the variant name and keep the rest of the token as the
		// suggestion context.
		name := u.Class
		if u.Suggestion != "" {
			return &UnknownClassError{Class: name + ":", Suggestion: u.Suggestion + ":", Variant: true}
		}
		return &UnknownClassError{Class: name + ":", Variant: true}
	}
	var un *UnsupportedClassError
	if errors.As(err, &un) {
		return &UnsupportedClassError{Class: tok, Reason: un.Reason}
	}
	return err
}

// resolveUtility finds the utility of a class without variants and builds
// its op.
func (p *Parser) resolveUtility(tok, util string) ([]Op, error) {
	c := &pctx{th: p.th, class: tok}
	if strings.HasPrefix(util, "[") && strings.HasSuffix(util, "]") {
		return nil, &UnsupportedClassError{Class: tok,
			Reason: "arbitrary properties are not supported; use a utility class, for example w-[320px]"}
	}
	if alias, ok := reg.aliases[util]; ok {
		util = alias
	}
	neg := false
	if strings.HasPrefix(util, "-") && len(util) > 1 {
		neg, util = true, util[1:]
	}
	o, err := reg.resolve(c, util, neg)
	switch {
	case err == nil:
		return []Op{o}, nil
	case errors.Is(err, errNoMatch):
		return nil, p.unknown(tok, util, neg)
	}
	var us unsupportedErr
	if errors.As(err, &us) {
		return nil, &UnsupportedClassError{Class: tok, Reason: string(us)}
	}
	var ue *UnsupportedClassError
	var ie *InvalidClassError
	if errors.As(err, &ue) || errors.As(err, &ie) {
		return nil, err
	}
	return nil, &InvalidClassError{Class: tok, Reason: err.Error()}
}

// unknown builds the error of a class that matches nothing.
func (p *Parser) unknown(tok, util string, neg bool) error {
	if reason, ok := reg.unsupportedReason(util); ok {
		return &UnsupportedClassError{Class: tok, Reason: reason}
	}
	return &UnknownClassError{Class: tok, Suggestion: p.suggestToken(tok)}
}

// suggestToken suggests the closest vocabulary class for a token, keeping
// its variants and its negative sign.
func (p *Parser) suggestToken(tok string) string {
	parts := splitTop(tok, ':')
	util := parts[len(parts)-1]
	prefix := strings.Join(parts[:len(parts)-1], ":")
	if prefix != "" {
		prefix += ":"
	}
	neg := ""
	if strings.HasPrefix(util, "-") {
		neg, util = "-", util[1:]
	}
	vocab := p.Vocabulary()
	best := closest(neg+util, vocab)
	if best == "" && neg != "" {
		best = closest(util, vocab)
	}
	if best == "" {
		return ""
	}
	return prefix + best
}

// ---------------------------------------------------------------------
// Registry.

// errNoMatch is returned by a handler that does not recognize the value.
var errNoMatch = errors.New("no match")

// unsupportedErr is returned by a handler for a class that Tela does not
// implement; its text says what to use instead.
type unsupportedErr string

func (e unsupportedErr) Error() string { return string(e) }

// arg is the part of a class after the utility root.
type arg struct {
	root   string
	raw    string // everything after "root-": "red-500/50", "1/2", "[calc(1rem+2px)]"
	value  string // raw without the modifier
	mod    string // the text after the last top-level slash
	hasMod bool
	arb    bool   // value is [..]
	inner  string // arbitrary content, with underscores turned into spaces
	hint   string // the type hint of [length:..]
	neg    bool
}

var typeHints = map[string]bool{
	"length": true, "color": true, "number": true, "percentage": true, "url": true,
	"image": true, "position": true, "size": true, "family-name": true, "absolute-size": true,
	"relative-size": true, "line-width": true, "angle": true, "integer": true, "ratio": true,
	"bg-size": true, "shadow": true, "generic-name": true,
}

func newArg(root, raw string, neg bool) (*arg, error) {
	a := &arg{root: root, raw: raw, value: raw, neg: neg}
	if i := lastTopSlash(raw); i >= 0 {
		a.value, a.mod, a.hasMod = raw[:i], raw[i+1:], true
	}
	if strings.HasPrefix(a.value, "(") && strings.HasSuffix(a.value, ")") {
		return nil, unsupportedErr("CSS variables are not supported; use a theme token (bg-primary) or an arbitrary value")
	}
	if strings.HasPrefix(a.value, "[") && strings.HasSuffix(a.value, "]") {
		a.arb = true
		inner := a.value[1 : len(a.value)-1]
		if i := strings.IndexByte(inner, ':'); i > 0 && typeHints[inner[:i]] {
			a.hint, inner = inner[:i], inner[i+1:]
		}
		a.inner = arbitrarySpaces(inner)
		if strings.Contains(a.inner, "var(") {
			return nil, unsupportedErr("CSS variables are not supported; use a theme token or a literal value")
		}
	}
	return a, nil
}

// arbitrarySpaces turns underscores into spaces, except inside url(...)
// and after a backslash, as Tailwind does.
func arbitrarySpaces(s string) string {
	if !strings.ContainsRune(s, '_') {
		return s
	}
	var b strings.Builder
	depthURL := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && s[i+1] == '_':
			b.WriteByte('_')
			i++
		case c == '_' && depthURL == 0:
			b.WriteByte(' ')
		default:
			if strings.HasPrefix(s[i:], "url(") {
				depthURL++
			} else if c == ')' && depthURL > 0 {
				depthURL--
			}
			b.WriteByte(c)
		}
	}
	return b.String()
}

// lastTopSlash returns the index of the last '/' outside brackets and
// parentheses, or -1.
func lastTopSlash(s string) int {
	depth, idx := 0, -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case '/':
			if depth == 0 {
				idx = i
			}
		}
	}
	return idx
}

// handler turns the argument of a utility into an op, or returns
// errNoMatch when the value is not one of its values.
type handler func(c *pctx, a *arg) (Op, error)

type utility struct {
	root   string
	neg    bool
	parse  handler
	values func(th *Theme) []string // the suffixes after "root-"; "" is the bare root
}

type registry struct {
	byRoot      map[string][]*utility
	order       []*utility
	aliases     map[string]string
	unsupported map[string]string
}

func (r *registry) add(root string, neg bool, h handler, values func(*Theme) []string) {
	u := &utility{root: root, neg: neg, parse: h, values: values}
	r.byRoot[root] = append(r.byRoot[root], u)
	r.order = append(r.order, u)
}

// resolve finds the utility for util (a class without variants and sign).
func (r *registry) resolve(c *pctx, util string, neg bool) (Op, error) {
	try := func(root, raw string) (Op, bool, error) {
		us := r.byRoot[root]
		if len(us) == 0 {
			return Op{}, false, nil
		}
		for _, u := range us {
			if neg && !u.neg {
				continue
			}
			a, err := newArg(root, raw, neg)
			if err != nil {
				return Op{}, true, err
			}
			o, err := u.parse(c, a)
			if err == nil {
				return o, true, nil
			}
			if !errors.Is(err, errNoMatch) {
				return Op{}, true, err
			}
		}
		return Op{}, false, nil
	}
	if o, ok, err := try(util, ""); ok {
		return o, err
	}
	// Roots from the longest: "border-t-red-500" tries border-t-red,
	// then border-t, then border. Dashes inside brackets do not split.
	depth := 0
	var dashes []int
	for i := 0; i < len(util); i++ {
		switch util[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case '-':
			if depth == 0 {
				dashes = append(dashes, i)
			}
		}
	}
	for i := len(dashes) - 1; i >= 0; i-- {
		d := dashes[i]
		if o, ok, err := try(util[:d], util[d+1:]); ok {
			return o, err
		}
	}
	return Op{}, errNoMatch
}

func (r *registry) unsupportedReason(util string) (string, bool) {
	if strings.HasPrefix(util, "content-[") {
		return "generated content (content-[...]) is not supported; use regular nodes", true
	}
	best := ""
	for k := range r.unsupported {
		if (util == k || strings.HasPrefix(util, k+"-")) && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		return "", false
	}
	return r.unsupported[best], true
}

// static registers a class with no value ("flex", "truncate").
func (r *registry) static(name string, o Op) {
	r.add(name, false, func(c *pctx, a *arg) (Op, error) {
		if a.raw != "" {
			return Op{}, errNoMatch
		}
		return o, nil
	}, func(*Theme) []string { return []string{""} })
}

// enum registers root-key classes with fixed keys.
func (r *registry) enum(root string, neg bool, keys map[string]Op) {
	r.add(root, neg, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod || a.arb {
			return Op{}, errNoMatch
		}
		if o, ok := keys[a.value]; ok {
			return o, nil
		}
		return Op{}, errNoMatch
	}, func(*Theme) []string {
		out := make([]string, 0, len(keys))
		for k := range keys {
			out = append(out, k)
		}
		slices.Sort(out)
		return out
	})
}

// alias registers v3 spellings.
func (r *registry) alias(from, to string) { r.aliases[from] = to }

var reg = newRegistry()

func newRegistry() *registry {
	r := &registry{
		byRoot:      map[string][]*utility{},
		aliases:     map[string]string{},
		unsupported: map[string]string{},
	}
	registerLayout(r)
	registerFlexGrid(r)
	registerSizing(r)
	registerColors(r)
	registerBorders(r)
	registerEffects(r)
	registerTypography(r)
	registerMotion(r)
	registerInteraction(r)
	registerUnsupported(r)
	return r
}

// noNegative lists the values that have no negative form.
var noNegative = map[string]bool{"auto": true, "none": true, "first": true, "last": true}

// buildVocabulary expands the registry with the theme keys.
func buildVocabulary(th *Theme) []string {
	seen := map[string]struct{}{}
	for _, u := range reg.order {
		if u.values == nil {
			continue
		}
		for _, v := range u.values(th) {
			if v == "DEFAULT" {
				v = ""
			}
			name := u.root
			if v != "" {
				name += "-" + v
			}
			seen[name] = struct{}{}
			if u.neg && v != "" && !noNegative[v] {
				seen["-"+name] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
