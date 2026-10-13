package xkb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ComposeTable is a compiled Compose file (the libX11 Compose(5) format
// that libxkbcommon's compose module reads): a trie of keysym sequences
// with their results. It is immutable and safe to share.
type ComposeTable struct {
	nodes []composeNode // nodes[0] is the root
	edges []composeEdge // the edges of node n are edges[n.first : n.first+n.count], sorted by keysym
}

type composeNode struct {
	first, count int32
	leaf         bool
	text         string // result of a leaf
}

type composeEdge struct {
	sym  Keysym
	next int32
}

// composeBuilder accumulates productions before the table is frozen into
// sorted edge arrays.
type composeBuilder struct {
	nodes []builderNode
}

type builderNode struct {
	children map[Keysym]int32
	leaf     bool
	text     string
}

// ComposeOptions says where Compose files and their includes are looked
// up. Every field may be empty; the backends fill them from the
// environment (see ComposeOptionsFromEnv).
type ComposeOptions struct {
	// Locale is the LC_CTYPE locale ("en_US.UTF-8"); "%L" includes and
	// the system file are resolved from it.
	Locale string
	// Home is the user's home directory ("%H").
	Home string
	// LocaleDir is the X11 locale directory ("%S"), by default
	// /usr/share/X11/locale.
	LocaleDir string
	// ReadFile reads a file; nil means os.ReadFile.
	ReadFile func(path string) ([]byte, error)
}

const defaultLocaleDir = "/usr/share/X11/locale"

func (o *ComposeOptions) read(path string) ([]byte, error) {
	if o.ReadFile != nil {
		return o.ReadFile(path)
	}
	return os.ReadFile(path)
}

func (o *ComposeOptions) localeDir() string {
	if o.LocaleDir != "" {
		return o.LocaleDir
	}
	return defaultLocaleDir
}

// ComposeOptionsFromEnv fills ComposeOptions from the environment like
// libxkbcommon and libX11: the locale from LC_ALL, LC_CTYPE, then LANG
// ("C" when none is set), HOME, and XLOCALEDIR.
func ComposeOptionsFromEnv(getenv func(string) string) ComposeOptions {
	o := ComposeOptions{Home: getenv("HOME"), LocaleDir: getenv("XLOCALEDIR")}
	for _, v := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if l := getenv(v); l != "" {
			o.Locale = l
			break
		}
	}
	if o.Locale == "" {
		o.Locale = "C"
	}
	return o
}

// ErrNoComposeFile is returned by LoadComposeTable when no Compose file
// exists for the user and the locale.
var ErrNoComposeFile = errors.New("xkb: no Compose file for this user and locale")

// LoadComposeTable finds and compiles the Compose file in effect, in
// libxkbcommon's order: $XCOMPOSEFILE, $XDG_CONFIG_HOME/XCompose (or
// ~/.config/XCompose), ~/.XCompose, then the system file of the locale
// (compose.dir under the locale directory, after locale.alias). Like
// libX11, a user file replaces the system file; it can pull it in with
// include "%L".
func LoadComposeTable(getenv func(string) string, o ComposeOptions) (*ComposeTable, string, error) {
	var candidates []string
	if p := getenv("XCOMPOSEFILE"); p != "" {
		candidates = append(candidates, p)
	}
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		candidates = append(candidates, filepath.Join(x, "XCompose"))
	} else if o.Home != "" {
		candidates = append(candidates, filepath.Join(o.Home, ".config", "XCompose"))
	}
	if o.Home != "" {
		candidates = append(candidates, filepath.Join(o.Home, ".XCompose"))
	}
	for _, path := range candidates {
		data, err := o.read(path)
		if err != nil {
			continue
		}
		t, err := ParseCompose(data, path, o)
		return t, path, err
	}
	if path, ok := o.localeComposePath(o.Locale); ok {
		if data, err := o.read(path); err == nil {
			t, err := ParseCompose(data, path, o)
			return t, path, err
		}
	}
	return nil, "", ErrNoComposeFile
}

// localeComposePath returns the system Compose file of a locale.
func (o *ComposeOptions) localeComposePath(locale string) (string, bool) {
	dir := o.localeDir()
	if locale == "" {
		locale = "C"
	}
	if alias, err := o.read(filepath.Join(dir, "locale.alias")); err == nil {
		if r, ok := resolveName(alias, locale, false); ok {
			locale = r
		}
	}
	var rel string
	if locale == "C" || locale == "POSIX" {
		// libxkbcommon's special case: compose.dir points C at the
		// ISO 8859-1 file, which is not UTF-8.
		rel = "en_US.UTF-8/Compose"
	} else {
		dirFile, err := o.read(filepath.Join(dir, "compose.dir"))
		if err != nil {
			return "", false
		}
		r, ok := resolveName(dirFile, locale, true)
		if !ok {
			return "", false
		}
		rel = r
	}
	if filepath.IsAbs(rel) {
		return rel, true
	}
	return filepath.Join(dir, rel), true
}

// resolveName looks a name up in a two-column file (locale.alias or
// compose.dir): with reverse false it matches the left column and returns
// the right one; with reverse true the other way round. A trailing ":"
// of the left column is ignored.
func resolveName(data []byte, name string, reverse bool) (string, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		left, right := strings.TrimSuffix(f[0], ":"), f[1]
		if reverse {
			if right == name {
				return left, true
			}
		} else if left == name {
			return right, true
		}
	}
	return "", false
}

// maxIncludeDepth bounds nested includes (libxkbcommon uses 5).
const maxIncludeDepth = 5

// ParseCompose compiles a Compose file. path is used for messages and
// relative includes; o resolves %H, %L and %S in includes.
func ParseCompose(data []byte, path string, o ComposeOptions) (*ComposeTable, error) {
	b := &composeBuilder{nodes: []builderNode{{}}}
	if err := b.parse(data, path, &o, 0); err != nil {
		return nil, err
	}
	return b.freeze(), nil
}

func (b *composeBuilder) parse(data []byte, path string, o *ComposeOptions, depth int) error {
	line := 0
	for len(data) > 0 {
		line++
		var l []byte
		if i := indexByte(data, '\n'); i >= 0 {
			l, data = data[:i], data[i+1:]
		} else {
			l, data = data, nil
		}
		if err := b.parseLine(l, path, line, o, depth); err != nil {
			return err
		}
	}
	return nil
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

func (b *composeBuilder) parseLine(l []byte, path string, line int, o *ComposeOptions, depth int) error {
	i := 0
	skipSpace := func() {
		for i < len(l) && (l[i] == ' ' || l[i] == '\t' || l[i] == '\r') {
			i++
		}
	}
	skipSpace()
	if i == len(l) || l[i] == '#' {
		return nil
	}
	errf := func(format string, args ...any) error {
		return fmt.Errorf("xkb: %s:%d: %s", path, line, fmt.Sprintf(format, args...))
	}
	if hasWord(l[i:], "include") {
		i += len("include")
		skipSpace()
		s, _, ok := composeString(l[i:])
		if !ok {
			return errf("include needs a quoted path")
		}
		if depth+1 > maxIncludeDepth {
			return errf("includes nested more than %d deep", maxIncludeDepth)
		}
		inc, err := o.expandInclude(s)
		if err != nil {
			return errf("%v", err)
		}
		data, err := o.read(inc)
		if err != nil {
			// libxkbcommon fails the whole table on a missing include.
			return errf("cannot read include %q: %v", inc, err)
		}
		return b.parse(data, inc, o, depth+1)
	}
	// Modifier prefixes (!, ~, None, Shift, Ctrl, …) are accepted and
	// ignored, like libxkbcommon does.
	var seq []Keysym
	for {
		skipSpace()
		if i >= len(l) {
			return errf("missing ':'")
		}
		c := l[i]
		if c == ':' {
			i++
			break
		}
		if c == '<' {
			end := i + 1
			for end < len(l) && l[end] != '>' {
				end++
			}
			if end >= len(l) {
				return errf("unterminated keysym")
			}
			name := string(l[i+1 : end])
			ks, ok := KeysymFromName(name)
			if !ok {
				// Unknown keysyms skip the line (libxkbcommon warns).
				return nil
			}
			seq = append(seq, ks)
			i = end + 1
			continue
		}
		// A modifier word or "!"/"~".
		for i < len(l) && l[i] != ' ' && l[i] != '\t' && l[i] != '<' && l[i] != ':' {
			i++
		}
	}
	if len(seq) == 0 {
		return errf("a sequence needs at least one keysym")
	}
	skipSpace()
	var text string
	if i < len(l) && l[i] == '"' {
		s, n, ok := composeString(l[i:])
		if !ok {
			return errf("malformed string")
		}
		text, i = s, i+n
		skipSpace()
	}
	if i < len(l) && l[i] != '#' {
		start := i
		for i < len(l) && l[i] != ' ' && l[i] != '\t' && l[i] != '#' && l[i] != '\r' {
			i++
		}
		ks, ok := KeysymFromName(string(l[start:i]))
		if ok && text == "" {
			text = keysymText(ks)
		}
	}
	if text == "" {
		return nil // nothing to produce
	}
	b.add(seq, text)
	return nil
}

func hasWord(l []byte, w string) bool {
	return len(l) > len(w) && string(l[:len(w)]) == w && (l[len(w)] == ' ' || l[len(w)] == '\t' || l[len(w)] == '"')
}

// composeString parses a quoted string with the escapes libxkbcommon
// accepts: \\, \", octal \NNN and hexadecimal \xHH; any other escaped
// character stands for itself ("\n" is "n"). It returns the string and
// the number of bytes consumed.
func composeString(l []byte) (string, int, bool) {
	if len(l) == 0 || l[0] != '"' {
		return "", 0, false
	}
	var out []byte
	i := 1
	for i < len(l) {
		c := l[i]
		i++
		switch {
		case c == '"':
			return string(out), i, true
		case c != '\\' || i >= len(l):
			out = append(out, c)
		default:
			e := l[i]
			i++
			switch {
			case e >= '0' && e <= '7':
				v := int(e - '0')
				for n := 1; n < 3 && i < len(l) && l[i] >= '0' && l[i] <= '7'; n++ {
					v = v*8 + int(l[i]-'0')
					i++
				}
				out = append(out, byte(v))
			case e == 'x' || e == 'X':
				v, n := 0, 0
				for ; n < 2 && i < len(l) && isHex(string(l[i])); n++ {
					v = v*16 + hexVal(l[i])
					i++
				}
				if n == 0 {
					out = append(out, '\\', e)
				} else {
					out = append(out, byte(v))
				}
			default:
				out = append(out, e)
			}
		}
	}
	return "", 0, false
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	}
	return int(c-'A') + 10
}

// expandInclude expands %H (home), %L (the locale's Compose file), %S
// (the locale directory) and %% in an include path.
func (o *ComposeOptions) expandInclude(s string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+1 >= len(s) {
			out.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case '%':
			out.WriteByte('%')
		case 'H':
			if o.Home == "" {
				return "", errors.New("%H in an include, but HOME is not set")
			}
			out.WriteString(o.Home)
		case 'L':
			p, ok := o.localeComposePath(o.Locale)
			if !ok {
				return "", fmt.Errorf("%%L in an include, but locale %q has no Compose file", o.Locale)
			}
			out.WriteString(p)
		case 'S':
			out.WriteString(o.localeDir())
		default:
			return "", fmt.Errorf("unknown %%%c in an include", s[i])
		}
	}
	return out.String(), nil
}

// add inserts a production with libxkbcommon's conflict rule: the later
// line wins. A new sequence that extends an existing one turns the shorter
// one into a prefix (it stops producing); a new sequence that is a prefix
// of existing ones drops them; an identical sequence replaces the result.
// Control characters are removed from results (Text never has them,
// ADR-050).
func (b *composeBuilder) add(seq []Keysym, text string) {
	text = stripControlChars(text)
	if text == "" {
		return
	}
	n := int32(0)
	for _, ks := range seq {
		node := &b.nodes[n]
		if node.leaf {
			node.leaf, node.text = false, ""
		}
		next, ok := node.children[ks]
		if !ok {
			if node.children == nil {
				node.children = map[Keysym]int32{}
			}
			next = int32(len(b.nodes))
			node.children[ks] = next
			b.nodes = append(b.nodes, builderNode{})
		}
		n = next
	}
	last := &b.nodes[n]
	last.children = nil // longer sequences through it are dropped
	last.leaf, last.text = true, text
}

func stripControlChars(s string) string {
	ctrl := func(r rune) bool { return r < 0x20 || (r >= 0x7f && r < 0xa0) }
	if strings.IndexFunc(s, ctrl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if ctrl(r) {
			return -1
		}
		return r
	}, s)
}

// freeze turns the builder into the compact table: nodes in creation
// order, each with its edges sorted by keysym.
func (b *composeBuilder) freeze() *ComposeTable {
	t := &ComposeTable{nodes: make([]composeNode, len(b.nodes))}
	total := 0
	for i := range b.nodes {
		total += len(b.nodes[i].children)
	}
	t.edges = make([]composeEdge, 0, total)
	for i := range b.nodes {
		bn := &b.nodes[i]
		first := int32(len(t.edges))
		for ks, next := range bn.children {
			t.edges = append(t.edges, composeEdge{ks, next})
		}
		es := t.edges[first:]
		slices.SortFunc(es, func(a, b composeEdge) int {
			switch {
			case a.sym < b.sym:
				return -1
			case a.sym > b.sym:
				return 1
			}
			return 0
		})
		t.nodes[i] = composeNode{first: first, count: int32(len(es)), leaf: bn.leaf, text: bn.text}
	}
	return t
}

// child returns the node reached from n with ks, or -1.
func (t *ComposeTable) child(n int32, ks Keysym) int32 {
	node := &t.nodes[n]
	es := t.edges[node.first : node.first+node.count]
	lo, hi := 0, len(es)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		if es[m].sym < ks {
			lo = m + 1
		} else {
			hi = m
		}
	}
	if lo < len(es) && es[lo].sym == ks {
		return es[lo].next
	}
	return -1
}

// ComposeStatus is the state of a compose sequence after a keysym.
type ComposeStatus uint8

const (
	// ComposeNothing: the keysym is not part of a sequence; it types
	// normally.
	ComposeNothing ComposeStatus = iota
	// ComposeComposing: a sequence is in progress; the keysym types
	// nothing.
	ComposeComposing
	// ComposeComposed: the keysym completed a sequence; its text is the
	// result.
	ComposeComposed
	// ComposeIgnored: a modifier keysym, which does not affect the
	// sequence.
	ComposeIgnored
)

// ComposeState is the progress of one keyboard through a ComposeTable.
// The zero value with a nil table composes nothing.
type ComposeState struct {
	t    *ComposeTable
	node int32
}

// NewComposeState returns a state at the start of a sequence.
func NewComposeState(t *ComposeTable) *ComposeState { return &ComposeState{t: t} }

// Reset abandons a sequence in progress (focus loss, keymap change).
func (s *ComposeState) Reset() { s.node = 0 }

// Composing reports whether a sequence is in progress.
func (s *ComposeState) Composing() bool { return s.node != 0 }

// Feed advances the sequence with the keysym of a key press and returns
// the status and, for ComposeComposed, the result.
//
// A keysym that does not continue the sequence in progress cancels it,
// and is then fed again from the start: it can start a new sequence (a
// second dead key), or it types normally (ComposeNothing). libxkbcommon
// reports such a key as cancelled and leaves the choice to the
// application; Qt and GTK let the key through as well.
func (s *ComposeState) Feed(ks Keysym) (ComposeStatus, string) {
	if s == nil || s.t == nil || ks == NoSymbol {
		return ComposeNothing, ""
	}
	if ks.IsModifier() {
		return ComposeIgnored, ""
	}
	next := s.t.child(s.node, ks)
	if next < 0 && s.node != 0 {
		s.node = 0
		next = s.t.child(0, ks)
	}
	if next < 0 {
		return ComposeNothing, ""
	}
	if n := &s.t.nodes[next]; n.leaf {
		s.node = 0
		return ComposeComposed, n.text
	}
	s.node = next
	return ComposeComposing, ""
}
