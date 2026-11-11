package text

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

// This file holds hyphenation dictionaries (CSS hyphens: auto, ADR-270):
// Liang's algorithm over TeX hyphenation patterns, with the rules Chromium
// applies around it on Linux and Windows (Blink's HyphenationMinikin over
// Android's Minikin hyphenator), and the choice of a dictionary for a
// content language (Blink's MapLocale). Line breaking (breaking.go) asks
// for the hyphenation points of the word that overflows a line.

// Hyphens is the CSS hyphens property of a span of text (CSS Text 3,
// section 5.4): how words may be broken at the end of a line.
type Hyphens uint8

const (
	// HyphensManual breaks words only at soft hyphens (U+00AD), which then
	// show a hyphen at the end of the line. It is the zero value, like
	// CSS's initial value (Tailwind hyphens-manual).
	HyphensManual Hyphens = iota
	// HyphensNone never breaks words with a hyphen: soft hyphens are not
	// break opportunities (Tailwind hyphens-none).
	HyphensNone
	// HyphensAuto also breaks words at the hyphenation points of the
	// dictionary of the text's language (Tailwind hyphens-auto), when the
	// collection has one (Collection.SetHyphenators); text whose language
	// is unknown is not hyphenated.
	HyphensAuto
)

const (
	// softHyphen is U+00AD SOFT HYPHEN.
	softHyphen = '\u00ad'

	// Blink's Hyphenation limits (hyphenate-limit-chars: auto, CSS Text 4's
	// suggested 5 2 2), and those of the Minikin hyphenator Chromium runs
	// on Linux and Windows (hyphenator_aosp.h: MIN_PREFIX 2, MIN_SUFFIX 3,
	// MAX_HYPHENATED_SIZE 64 UTF-16 units with the two word boundaries):
	// a word of at least 5 characters breaks with at least 2 characters
	// before the hyphen and 3 after it.
	hyphenMinWord   = 5
	hyphenMinPrefix = 2
	hyphenMinSuffix = 3
	hyphenMaxWord   = 62
)

// Hyphenator is the hyphenation dictionary of one language: TeX
// hyphenation patterns (Liang's algorithm) and exceptions. It is compiled
// when it is first used and is then safe for concurrent use.
type Hyphenator struct {
	lang       string // lowercase BCP 47
	patterns   string
	exceptions string
	noCaps     bool // Blink does not hyphenate capitalized English words

	once sync.Once
	dict *hyphDict
	err  error
}

// NewHyphenator returns the dictionary of the language lang (a BCP 47 tag
// such as "en-US", "en-GB", "es" or "de-1996": the dictionary Chromium
// names so) for the TeX hyphenation patterns and exceptions, in the
// format of hyph-utf8's .pat.txt and .hyp.txt files: patterns are letters
// with digits between them (".ach4", "hy3ph") and exceptions are words
// with hyphens where they break ("ta-ble"), separated by white space; text
// from a % to the end of a line is a comment. patterns may instead be the
// compact form EncodeHyphenPatterns returns (exceptions is then ignored).
// Nothing is parsed until the dictionary is first used (Load).
func NewHyphenator(lang, patterns, exceptions string) *Hyphenator {
	l := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(lang), "_", "-"))
	return &Hyphenator{lang: l, patterns: patterns, exceptions: exceptions, noCaps: strings.HasPrefix(l, "en")}
}

// Lang returns the language of the dictionary, in lowercase.
func (h *Hyphenator) Lang() string { return h.lang }

// Load compiles the dictionary, if that was not done yet, and returns why
// it cannot be used (malformed patterns); a dictionary that cannot be used
// hyphenates nothing.
func (h *Hyphenator) Load() error {
	h.once.Do(func() {
		pat, exc := h.patterns, h.exceptions
		if strings.HasPrefix(pat, hyphenMagic) {
			var err error
			if pat, exc, err = decodeHyphenPatterns(pat); err != nil {
				h.err = fmt.Errorf("hyphenation dictionary %q: %v", h.lang, err)
				return
			}
		}
		d, err := compileHyphenDict(pat, exc)
		if err != nil {
			h.err = fmt.Errorf("hyphenation dictionary %q: %v", h.lang, err)
			return
		}
		h.dict = d
	})
	return h.err
}

// Hyphenate returns where word may break with a hyphen, as rune offsets
// into word in increasing order, by the rules of hyphens: auto in
// Chromium: leading spaces, opening punctuation and quotes and trailing
// spaces, punctuation and controls are not part of the word; a word
// shorter than 5 letters, longer than 62, with a letter the patterns do
// not know, or capitalized in English, does not break; a break leaves at
// least 2 letters before it and 3 after it.
func (h *Hyphenator) Hyphenate(word string) []int {
	pts := h.points([]rune(word), nil)
	out := make([]int, len(pts))
	for i, p := range pts {
		out[i] = int(p)
	}
	return out
}

// points appends to dst the hyphenation points of text (rune offsets, in
// increasing order): Blink's HyphenationMinikin::HyphenLocations, with
// WordToHyphenate and ShouldHyphenateWord, over Minikin's hyphenate.
func (h *Hyphenator) points(text []rune, dst []int32) []int32 {
	if h == nil || h.Load() != nil {
		return dst
	}
	b, e := 0, len(text)
	for b < e && hyphenSkipLeading(text[b]) {
		b++
	}
	for b < e && hyphenSkipTrailing(text[e-1]) {
		e--
	}
	word := text[b:e]
	m := len(word)
	if m < hyphenMinWord || m > hyphenMaxWord || (h.noCaps && unicode.IsUpper(word[0])) {
		return dst
	}
	return h.dict.points(word, int32(b), dst)
}

// hyphenSkipLeading reports the characters that Blink strips from the
// start of a word before hyphenating it: spaces (TreatAsSpace) and the
// opening punctuation and quotation marks of UAX #14 (OP and QU).
func hyphenSkipLeading(r rune) bool {
	switch r {
	case ' ', '\t', '\n', 0xA0:
		return true
	}
	return unicode.Is(lineBreakOPQU, r)
}

// hyphenSkipTrailing reports the characters that Blink strips from the
// end of a word before hyphenating it: space separators, punctuation and
// controls.
func hyphenSkipTrailing(r rune) bool {
	return unicode.In(r, unicode.Zs, unicode.P, unicode.Cc)
}

// lineBreakOPQU holds the characters of the UAX #14 line break classes OP
// and QU (Unicode 16 LineBreak.txt).
var lineBreakOPQU = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x0022, 0x0022, 1}, {0x0027, 0x0028, 1}, {0x005B, 0x005B, 1}, {0x007B, 0x007B, 1},
		{0x00A1, 0x00A1, 1}, {0x00AB, 0x00AB, 1}, {0x00BB, 0x00BB, 1}, {0x00BF, 0x00BF, 1},
		{0x0F3A, 0x0F3C, 2}, {0x169B, 0x169B, 1},
		{0x2018, 0x2019, 1}, {0x201A, 0x201F, 1}, {0x2039, 0x203A, 1}, {0x2045, 0x2045, 1},
		{0x207D, 0x207D, 1}, {0x208D, 0x208D, 1}, {0x2308, 0x230A, 2}, {0x2329, 0x2329, 1},
		{0x275B, 0x2760, 1}, {0x2768, 0x2774, 2}, {0x27C5, 0x27C5, 1}, {0x27E6, 0x27EE, 2},
		{0x2983, 0x2997, 2}, {0x29D8, 0x29DA, 2}, {0x29FC, 0x29FC, 1},
		{0x2E00, 0x2E0D, 1}, {0x2E18, 0x2E18, 1}, {0x2E1C, 0x2E1D, 1}, {0x2E20, 0x2E22, 1},
		{0x2E24, 0x2E28, 2}, {0x2E42, 0x2E42, 1}, {0x2E55, 0x2E5B, 2},
		{0x3008, 0x3010, 2}, {0x3014, 0x301A, 2}, {0x301D, 0x301D, 1},
		{0xFD3F, 0xFD3F, 1}, {0xFE17, 0xFE17, 1}, {0xFE35, 0xFE43, 2}, {0xFE47, 0xFE47, 1},
		{0xFE59, 0xFE5D, 2}, {0xFF08, 0xFF08, 1}, {0xFF3B, 0xFF3B, 1}, {0xFF5B, 0xFF5B, 1},
		{0xFF5F, 0xFF5F, 1}, {0xFF62, 0xFF62, 1},
	},
	R32: []unicode.Range32{
		{0x13258, 0x1325A, 1}, {0x13286, 0x13288, 2}, {0x13379, 0x13379, 1}, {0x1342F, 0x1342F, 1},
		{0x13437, 0x13437, 1}, {0x1343C, 0x1343E, 2}, {0x145CE, 0x145CE, 1}, {0x1E95E, 0x1E95F, 1},
		{0x1F676, 0x1F678, 1},
	},
}

// hyphDict is a compiled dictionary: the patterns (exceptions included,
// as whole-word patterns of values 10 and 11, like Minikin's) sorted by
// their letters, for Liang's search. Keys are the UTF-8 of the lowercase
// letters, with '.' for the word boundaries.
//
// perf: about 2.3 bytes per pattern letter plus 8 per pattern (some 130
// KiB for American English); a packed trie would be smaller and faster.
type hyphDict struct {
	keys  string   // the keys, concatenated in sorted order
	koff  []uint32 // key i is keys[koff[i]:koff[i+1]]
	vals  []uint8  // values of key i: one per boundary, its runes plus one
	voff  []uint32
	alpha map[rune]rune // letter (either case) to the lowercase form of the patterns
}

// compileHyphenDict parses patterns and exceptions in TeX's format (see
// NewHyphenator).
func compileHyphenDict(patterns, exceptions string) (*hyphDict, error) {
	type entry struct {
		key  string
		vals []uint8
	}
	byKey := map[string]*entry{}
	letters := map[rune]bool{}
	add := func(key string, vals []uint8) {
		if e := byKey[key]; e != nil {
			for i := range e.vals {
				e.vals[i] = max(e.vals[i], vals[i])
			}
			return
		}
		byKey[key] = &entry{key, vals}
	}
	for _, tok := range texTokens(patterns) {
		var key strings.Builder
		vals := []uint8{0}
		for _, r := range tok {
			switch {
			case r >= '0' && r <= '9':
				vals[len(vals)-1] = uint8(r - '0')
			case r == '-' || unicode.IsSpace(r) || unicode.IsControl(r):
				return nil, fmt.Errorf("pattern %q: unexpected %q", tok, r)
			default:
				if r != '.' {
					letters[r] = true
				}
				key.WriteRune(r)
				vals = append(vals, 0)
			}
		}
		if key.Len() == 0 {
			return nil, fmt.Errorf("pattern %q has no letters", tok)
		}
		add(key.String(), vals)
	}
	for _, tok := range texTokens(exceptions) {
		var key strings.Builder
		key.WriteByte('.')
		// Boundaries of ".word.": none before and after the dots, 10 (no
		// break, above every pattern) between letters, 11 at a hyphen.
		vals := []uint8{0, 0}
		for _, r := range tok {
			switch {
			case r == '-':
				if len(vals) > 2 {
					vals[len(vals)-1] = 11
				}
			case r >= '0' && r <= '9' || r == '.' || unicode.IsSpace(r) || unicode.IsControl(r):
				return nil, fmt.Errorf("exception %q: unexpected %q", tok, r)
			default:
				letters[r] = true
				key.WriteRune(r)
				vals = append(vals, 10)
			}
		}
		if len(vals) == 2 {
			return nil, fmt.Errorf("exception %q has no letters", tok)
		}
		vals[len(vals)-1] = 0 // no break after the last letter
		key.WriteByte('.')
		vals = append(vals, 0)
		add(key.String(), vals)
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	d := &hyphDict{koff: make([]uint32, 0, len(keys)+1), voff: make([]uint32, 0, len(keys)+1), alpha: map[rune]rune{}}
	var kb strings.Builder
	for _, k := range keys {
		d.koff = append(d.koff, uint32(kb.Len()))
		d.voff = append(d.voff, uint32(len(d.vals)))
		kb.WriteString(k)
		d.vals = append(d.vals, byKey[k].vals...)
	}
	d.koff = append(d.koff, uint32(kb.Len()))
	d.voff = append(d.voff, uint32(len(d.vals)))
	d.keys = kb.String()
	// The alphabet: the letters of the patterns and their capitals (the
	// .chr.txt files of hyph-utf8 list them; Minikin's mk_hyb_file maps
	// the capital of ß, which Unicode spells SS, to ẞ).
	for r := range letters {
		d.alpha[r] = r
	}
	for r := range letters {
		up := unicode.ToUpper(r)
		if r == 'ß' {
			up = 'ẞ'
		}
		if _, ok := d.alpha[up]; !ok && up != r {
			d.alpha[up] = r
		}
	}
	return d, nil
}

// texTokens splits TeX patterns or exceptions into their tokens, without
// comments.
func texTokens(s string) []string {
	var out []string
	for line := range strings.Lines(s) {
		if i := strings.IndexByte(line, '%'); i >= 0 {
			line = line[:i]
		}
		out = append(out, strings.Fields(line)...)
	}
	return out
}

// points appends to dst the hyphenation points of word (rune offsets plus
// base) by Liang's algorithm, as Minikin's Hyphenator::hyphenate: none
// unless every letter is in the alphabet; at least hyphenMinPrefix
// letters before a point and hyphenMinSuffix after it.
func (d *hyphDict) points(word []rune, base int32, dst []int32) []int32 {
	m := len(word)
	var wbuf [hyphenMaxWord + 2]rune
	w := wbuf[:m+2]
	w[0], w[m+1] = '.', '.'
	for i, r := range word {
		lr, ok := d.alpha[r]
		if !ok || r > 0xFFFF { // Minikin looks up UTF-16 units
			return dst
		}
		w[i+1] = lr
	}
	var sbuf [hyphenMaxWord + 3]uint8
	slots := sbuf[:m+3] // slot s is the boundary before w[s]
	n := len(d.koff) - 1
	var ub [utf8.UTFMax]byte
	for i := range w {
		lo, hi, depth := 0, n, 0
		for j := i; j < len(w) && lo < hi; j++ {
			k := utf8.EncodeRune(ub[:], w[j])
			for _, c := range ub[:k] {
				lo, hi = d.narrow(lo, hi, depth, c)
				depth++
				if lo == hi {
					break
				}
			}
			if lo < hi && int(d.koff[lo+1]-d.koff[lo]) == depth {
				vs := d.vals[d.voff[lo]:d.voff[lo+1]]
				for t, v := range vs {
					slots[i+t] = max(slots[i+t], v)
				}
			}
		}
	}
	// A hyphen before word[p] is the boundary before w[p+1].
	for p := hyphenMinPrefix; p <= m-hyphenMinSuffix; p++ {
		if slots[p+1]%2 == 1 {
			dst = append(dst, base+int32(p))
		}
	}
	return dst
}

// narrow returns the keys of [lo, hi), which share their first depth
// bytes, whose byte at depth is c: shorter keys sort first, then keys by
// that byte.
func (d *hyphDict) narrow(lo, hi, depth int, c byte) (int, int) {
	at := func(i int) int { // the byte at depth of key i, -1 past its end
		s, e := int(d.koff[i]), int(d.koff[i+1])
		if s+depth >= e {
			return -1
		}
		return int(d.keys[s+depth])
	}
	a := lo + sort.Search(hi-lo, func(k int) bool { return at(lo+k) >= int(c) })
	b := a + sort.Search(hi-a, func(k int) bool { return at(a+k) > int(c) })
	return a, b
}

// The compact form of a dictionary: hyphenMagic, then the DEFLATE stream
// of the length of the patterns (uvarint), the patterns and the
// exceptions, each token on its own line, sorted.
const hyphenMagic = "\x00tela-hyphen-1\n"

// EncodeHyphenPatterns returns the compact form of TeX hyphenation
// patterns and exceptions (see NewHyphenator), which NewHyphenator
// accepts as its patterns: the tokens without comments, sorted, one per
// line, compressed. It reports patterns that do not compile.
func EncodeHyphenPatterns(patterns, exceptions string) (string, error) {
	if _, err := compileHyphenDict(patterns, exceptions); err != nil {
		return "", err
	}
	join := func(s string) string {
		t := texTokens(s)
		sort.Strings(t)
		if len(t) == 0 {
			return ""
		}
		return strings.Join(t, "\n") + "\n"
	}
	p, e := join(patterns), join(exceptions)
	var raw bytes.Buffer
	raw.Write(binary.AppendUvarint(nil, uint64(len(p))))
	raw.WriteString(p)
	raw.WriteString(e)
	var out bytes.Buffer
	out.WriteString(hyphenMagic)
	fw, err := flate.NewWriter(&out, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(raw.Bytes()); err != nil {
		return "", err
	}
	if err := fw.Close(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// decodeHyphenPatterns returns the patterns and exceptions of a compact
// form.
func decodeHyphenPatterns(s string) (patterns, exceptions string, err error) {
	raw, err := io.ReadAll(flate.NewReader(strings.NewReader(s[len(hyphenMagic):])))
	if err != nil {
		return "", "", err
	}
	n, k := binary.Uvarint(raw)
	if k <= 0 || n > uint64(len(raw)-k) {
		return "", "", errors.New("malformed compact form")
	}
	raw = raw[k:]
	return string(raw[:n]), string(raw[n:]), nil
}

// hyphenSet is the hyphenation dictionaries of a collection and the one
// chosen for each language.
type hyphenSet struct {
	list  []*Hyphenator
	mu    sync.Mutex
	cache map[string]*Hyphenator
}

// SetHyphenators sets the hyphenation dictionaries text in this collection
// may use for hyphens: auto (they replace the previous ones). A text's
// language picks one as Chromium does (hyphenLocale): "en" and "en-US"
// the dictionary of "en-US", "en-GB" and the other English regions that
// of "en-GB", "de" that of "de-1996", "es-MX" that of "es"; without that
// dictionary, the first one of the same primary language, else none. The
// layouts made before keep the dictionaries they had.
func (c *Collection) SetHyphenators(hs []*Hyphenator) {
	var set *hyphenSet
	if len(hs) > 0 {
		set = &hyphenSet{list: append([]*Hyphenator(nil), hs...), cache: map[string]*Hyphenator{}}
	}
	c.hyph.Store(set)
}

// Hyphenators returns the hyphenation dictionaries of the collection.
func (c *Collection) Hyphenators() []*Hyphenator {
	if s := c.hyph.Load(); s != nil {
		return append([]*Hyphenator(nil), s.list...)
	}
	return nil
}

// hyphenator returns the dictionary of the language lang (a BCP 47 tag),
// or nil.
func (c *Collection) hyphenator(lang string) *Hyphenator {
	set := c.hyph.Load()
	if set == nil || lang == "" {
		return nil
	}
	set.mu.Lock()
	defer set.mu.Unlock()
	if h, ok := set.cache[lang]; ok {
		return h
	}
	mapped := hyphenLocale(lang)
	var h *Hyphenator
	for _, d := range set.list {
		if d.lang == mapped {
			h = d
			break
		}
	}
	if h == nil {
		primary := primaryTag(mapped)
		for _, d := range set.list {
			if primaryTag(d.lang) == primary {
				h = d
				break
			}
		}
	}
	set.cache[lang] = h
	return h
}

// hyphenLocales is Blink's locale fallback map for hyphenation
// (HyphenationMinikin's CreateLocaleFallbackMap, CLDR data compiled by
// AOSP): the key's dictionary, and the one for an exact match.
var hyphenLocales = map[string][2]string{
	"en-as": {"en-us"}, "en-gu": {"en-us"}, "en-mh": {"en-us"}, "en-mp": {"en-us"},
	"en-pr": {"en-us"}, "en-um": {"en-us"}, "en-vi": {"en-us"},
	"en":         {"en-gb", "en-us"},
	"de":         {"de-1996"},
	"de-li-1901": {"de-ch-1901"},
	"no":         {"nb"},
	"mn":         {"mn-cyrl"},
	"am":         {"und-ethi"}, "byn": {"und-ethi"}, "gez": {"und-ethi"}, "ti": {"und-ethi"}, "wal": {"und-ethi"},
	"und-beng": {"bn"}, "und-deva": {"hi"}, "und-gujr": {"gu"}, "und-guru": {"pa"}, "und-knda": {"kn"},
	"und-mlym": {"ml"}, "und-orya": {"or"}, "und-taml": {"ta"}, "und-telu": {"te"},
	"de-1901": {"de-1901"}, "de-1996": {"de-1996"}, "de-ch-1901": {"de-ch-1901"},
	"en-gb": {"en-gb"}, "en-us": {"en-us"}, "mn-cyrl": {"mn-cyrl"}, "und-ethi": {"und-ethi"},
}

// hyphenLocale returns the dictionary locale of a content language, in
// lowercase: Blink's HyphenationMinikin::MapLocale, which walks the tag up
// its subtags to the first one in hyphenLocales, else returns the primary
// language.
func hyphenLocale(lang string) string {
	tag := strings.ToLower(strings.ReplaceAll(lang, "_", "-"))
	for m := tag; ; {
		if v, ok := hyphenLocales[m]; ok {
			if v[1] != "" && m == tag {
				return v[1]
			}
			return v[0]
		}
		i := strings.LastIndexByte(m, '-')
		if i <= 0 {
			return m
		}
		m = m[:i]
	}
}

// hyphenStore is the atomic pointer of Collection.hyph.
type hyphenStore = atomic.Pointer[hyphenSet]
