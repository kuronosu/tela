package text

import (
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
)

// 5.2).
type Query struct {
	// Families in order of preference: family names or CSS generic
	// families ("sans-serif", "monospace", "system-ui", "emoji"...). Names
	// are matched case-insensitively, ignoring spaces and quotes.
	Families []string
	// Weight is the CSS font-weight, 1 to 1000. Zero means 400.
	Weight float32
	// Style is the CSS font-style.
	Style Style
	// Stretch is the CSS font-stretch percentage, 50 to 200. Zero means 100.
	Stretch float32
}

func (q Query) weight() float32 {
	if q.Weight <= 0 || q.Weight != q.Weight {
		return 400
	}
	return min(q.Weight, 1000)
}

func (q Query) stretch() float32 {
	if q.Stretch <= 0 || q.Stretch != q.Stretch {
		return 100
	}
	return q.Stretch
}

// Collection is a registry of font families with CSS-like matching and
// fallback. It is safe for concurrent use.
//
// Families are registered as parsed typefaces (Register, AddTypeface) or as
// font sources that are parsed when text first needs them (AddSource,
// ADR-244). Fallback order for a cluster of text (see Itemize): the
// requested families, then the families registered for the cluster's
// script (SetScriptFallback), then every other registered family in
// registration order, then, when EnableSystemFonts was called, the
// installed system fonts. Emoji presentation clusters try the emoji
// families (generic "emoji") first. A family is parsed only when this walk
// reaches it, so a source that never draws anything is never parsed.
//
// Generic family names ("sans-serif", "system-ui", "monospace", "emoji"...)
// resolve to the families SetGeneric or a source's Generics mapped them to;
// with system fonts enabled, a generic the collection does not map
// resolves to the first installed family of the operating system's list
// for it (SystemFontOptions; macOS SF, Windows Segoe UI, fontconfig's
// preferences on Linux). Registered families win over installed ones of
// the same name.
//
// Variable faces are instantiated per optical size: CSS
// font-optical-sizing: auto sets opsz to the font size in CSS pixels, and
// the Collection rounds it to whole pixels (clamped to the axis), so a
// typeface has at most one instance per (weight, stretch, style, pixel
// size) used. Instances are interned for the life of the Collection's
// typefaces.
//
// No system fonts are used unless EnableSystemFonts is called (or the
// collection comes from NewSystemCollection); synthetic bold and oblique
// are not implemented: a request for a weight or style that no face covers
// gets the closest face as CSS matching defines it, unchanged. A
// collection with no font at all (none registered, none installed) lays
// text out with fallback metrics and no glyphs (SetNoFontHandler).
type Collection struct {
	mu       sync.RWMutex
	families map[string]*family // by normalized name
	order    []*family          // registration order
	generics map[string][]string
	scripts  map[language.Script][]string
	cache    map[string]*candidates
	sys      *systemFonts // nil unless EnableSystemFonts was called
	log      Logger       // nil: diagnostics are discarded

	noFont         atomic.Pointer[func()]
	noFontReported atomic.Bool
}

// family is a registered family: its parsed typefaces and its sources.
// The family of a system font has typefaces only.
type family struct {
	name  string
	faces []*Typeface
	srcs  []*FontSource
}

// Logger receives diagnostics: the font files that could not be read or
// parsed, the system font scan. A *log.Logger implements it.
type Logger interface {
	Printf(format string, args ...any)
}

// emptyCollection lays out text given a nil collection (NewLayout).
var emptyCollection = NewCollection()

// NewCollection returns an empty collection.
func NewCollection() *Collection {
	return &Collection{
		families: map[string]*family{},
		generics: map[string][]string{},
		scripts:  map[language.Script][]string{},
		cache:    map[string]*candidates{},
	}
}

func normalizeFamily(name string) string {
	return font.NormalizeFamily(strings.Trim(strings.TrimSpace(name), `"'`))
}

// Register parses a font file and adds its typefaces to family (or to the
// family named in the font when family is empty). It returns the
// typefaces. data must not be modified afterwards.
func (c *Collection) Register(data []byte, family string) ([]*Typeface, error) {
	tfs, err := ParseTypefaces(data, family)
	if err != nil {
		return nil, err
	}
	for _, tf := range tfs {
		c.AddTypeface(tf)
	}
	return tfs, nil
}

// AddTypeface adds a parsed typeface to its family.
func (c *Collection) AddTypeface(tf *Typeface) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f := c.familyLocked(tf.family)
	f.faces = append(f.faces, tf)
	clear(c.cache)
}

// familyLocked returns the registered family called name, adding it at
// the end of the order when it is new. c.mu must be held for writing.
func (c *Collection) familyLocked(name string) *family {
	key := normalizeFamily(name)
	f := c.families[key]
	if f == nil {
		f = &family{name: name}
		c.families[key] = f
		c.order = append(c.order, f)
	}
	return f
}

// AddSource registers the family of src without reading or parsing its
// files: they are parsed when text first needs the family (see
// FontSource). The family takes its place in the registration order (a
// family registered again keeps its first place and gains the source's
// fonts), and src.Generics and src.Scripts append the family to the lists
// of those generic names and scripts. A source may be added to several
// collections; it is parsed once for all of them. AddSource returns an
// error, and changes nothing, when src has no family name or no files.
func (c *Collection) AddSource(src *FontSource) error {
	switch {
	case src == nil:
		return errors.New("text: AddSource(nil)")
	case normalizeFamily(src.Family) == "":
		return errors.New("text: a FontSource needs a Family name")
	case len(src.Data) == 0 && src.Load == nil:
		return errors.New("text: the FontSource of " + src.Family + " has neither Data nor Load")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f := c.familyLocked(src.Family)
	if !slices.Contains(f.srcs, src) {
		f.srcs = append(f.srcs, src)
	}
	for _, g := range src.Generics {
		k := normalizeFamily(g)
		if !slices.Contains(c.generics[k], src.Family) {
			c.generics[k] = append(c.generics[k], src.Family)
		}
	}
	for _, s := range src.Scripts {
		if !slices.Contains(c.scripts[s], src.Family) {
			c.scripts[s] = append(c.scripts[s], src.Family)
		}
	}
	clear(c.cache)
	return nil
}

// SetGeneric maps a generic family name (such as "sans-serif") to a list of
// families, replacing the previous mapping (and, for that name, the
// operating system's list of the system fonts).
func (c *Collection) SetGeneric(generic string, families ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generics[normalizeFamily(generic)] = append([]string(nil), families...)
	clear(c.cache)
}

// SetScriptFallback sets the families tried, after the requested ones, for
// text in script.
func (c *Collection) SetScriptFallback(script language.Script, families ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scripts[script] = append([]string(nil), families...)
	clear(c.cache)
}

// Families returns the registered family names in registration order.
// Nothing is parsed.
func (c *Collection) Families() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.order))
	for i, f := range c.order {
		out[i] = f.name
	}
	return out
}

// SetLogger sets where the collection reports the font files of its
// sources that cannot be read or parsed (nil discards them).
func (c *Collection) SetLogger(l Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = l
	clear(c.cache)
}

// SetNoFontHandler sets a function that runs once, the first time the
// collection finds no font at all for text or for Face: nothing is
// registered (or nothing registered can be parsed) and no system font is
// installed or enabled. Text then lays out with fallback metrics (an
// ascent of 0.8 and a descent of 0.2 em) and draws nothing. f runs on the
// goroutine that laid the text out, possibly while text layout holds its
// shaping lock, so it must not use the collection; a nil f removes it.
func (c *Collection) SetNoFontHandler(f func()) {
	if f == nil {
		c.noFont.Store(nil)
		return
	}
	c.noFont.Store(&f)
}

// reportNoFont runs the no-font handler the first time.
func (c *Collection) reportNoFont() {
	if f := c.noFont.Load(); f != nil && c.noFontReported.CompareAndSwap(false, true) {
		(*f)()
	}
}

// famRef is a family as a name resolves to it, before anything is parsed:
// a registered family (its typefaces and sources as of the resolution) or
// an installed one.
type famRef struct {
	key   string // identity: "r" or "s" (registered, system) and the normalized name
	faces []*Typeface
	srcs  []*FontSource
	sys   *systemFonts // an installed family, key[1:] in its index
}

// typeface returns the typeface of the family that CSS matching selects
// for q, parsing it on first use, or nil when the family has none usable.
func (r *famRef) typeface(q Query, log Logger) *Typeface {
	if r.sys != nil {
		f := r.sys.familyByKey(r.key[1:], q)
		if f == nil {
			return nil
		}
		return f.faces[0]
	}
	return matchSources(r.faces, r.srcs, q, log)
}

// ref returns the reference of a registered family. c.mu must be held.
func (c *Collection) ref(f *family) famRef {
	return famRef{key: "r" + normalizeFamily(f.name), faces: slices.Clone(f.faces), srcs: slices.Clone(f.srcs)}
}

// famSlot is one entry of a fallback list: a family and the face it gives
// for the list's query, made on first need. A generic name the operating
// system resolves has its installed alternatives in order, and the first
// that can be loaded is used.
type famSlot struct {
	alts []famRef
	once sync.Once
	face *Face
}

// get returns the face of the slot for the query of cs, or nil when no
// alternative can be used.
func (s *famSlot) get(cs *candidates) *Face {
	s.once.Do(func() {
		for i := range s.alts {
			if tf := s.alts[i].typeface(cs.q, cs.log); tf != nil {
				s.face = instance(tf, cs.q, cs.size)
				return
			}
		}
	})
	return s.face
}

// resolveSlots expands names into the slots of their families, in order,
// skipping unknown names and the families whose key is in seen (and adding
// the new ones to it): a registered family; the families a generic name is
// mapped to; with system fonts, an unmapped generic name's family from the
// operating system's list, or an installed family of the name. c.mu must
// be held.
func (c *Collection) resolveSlots(names []string, seen map[string]bool, out []*famSlot) []*famSlot {
	var walk func(names []string, depth int)
	walk = func(names []string, depth int) {
		for _, n := range names {
			key := normalizeFamily(n)
			if f := c.families[key]; f != nil {
				r := c.ref(f)
				if !seen[r.key] {
					seen[r.key] = true
					out = append(out, &famSlot{alts: []famRef{r}})
				}
				continue
			}
			if g, ok := c.generics[key]; ok {
				if depth < 4 {
					walk(g, depth+1)
				}
				continue
			}
			if c.sys == nil || key == "" {
				continue
			}
			var alts []famRef
			if isGenericName(key) {
				for _, name := range c.sys.genericList(key) {
					if f := c.families[name]; f != nil {
						alts = append(alts, c.ref(f))
					} else if c.sys.has(name) {
						alts = append(alts, famRef{key: "s" + name, sys: c.sys})
					}
				}
			} else if c.sys.has(key) {
				alts = []famRef{{key: "s" + key, sys: c.sys}}
			}
			if len(alts) > 0 && !seen[alts[0].key] {
				seen[alts[0].key] = true
				out = append(out, &famSlot{alts: alts})
			}
		}
	}
	walk(names, 0)
	return out
}

// needsIndex reports whether resolving names reads the index of the system
// fonts, which is not loaded yet: a name that is neither registered nor a
// mapped generic. c.mu must be held.
func (c *Collection) needsIndex(names []string) bool {
	if c.sys == nil || c.sys.indexed.Load() {
		return false
	}
	found := false
	var walk func(names []string, depth int)
	walk = func(names []string, depth int) {
		for _, n := range names {
			key := normalizeFamily(n)
			g, mapped := c.generics[key]
			switch {
			case found || key == "" || c.families[key] != nil:
			case mapped:
				if depth < 4 {
					walk(g, depth+1)
				}
			case isGenericName(key):
				found = len(c.sys.genericList(key)) > 0
			default:
				found = true
			}
		}
	}
	walk(names, 0)
	return found
}

// lazySlots resolves names outside a cache build: it loads the system
// index first when the names need it, so that the scan (seconds, the first
// time) never runs under c.mu and never blocks the queries that need only
// registered fonts. seen is copied, not modified.
func (c *Collection) lazySlots(names func() []string, seen map[string]bool) []*famSlot {
	c.mu.RLock()
	sys, need := c.sys, c.needsIndex(names())
	c.mu.RUnlock()
	if need {
		sys.ensureIndex()
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.resolveSlots(names(), cloneSeen(seen), nil)
}

// cloneSeen copies a set of family keys (nil gives an empty set).
func cloneSeen(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Match returns the typeface of family that CSS font matching selects for
// q, or nil if the family (or generic name) is unknown or none of its
// fonts can be parsed. Only the selected font is parsed.
func (c *Collection) Match(familyName string, q Query) *Typeface {
	slots := c.lazySlots(func() []string { return []string{familyName} }, nil)
	c.mu.RLock()
	log := c.log
	c.mu.RUnlock()
	for _, s := range slots {
		for i := range s.alts {
			if tf := s.alts[i].typeface(q, log); tf != nil {
				return tf
			}
		}
	}
	return nil
}

// Face returns the face of the first available family of q, instantiated
// for size (CSS pixels, used for the optical size): the first requested
// family that has a usable font, else the first registered family, else
// (with system fonts) the operating system's sans-serif. It returns nil
// only when the collection has no font at all.
func (c *Collection) Face(q Query, size float32) *Face {
	return c.candidates(q, size).defaultFace()
}

// instance returns the face of tf for q at size.
func instance(tf *Typeface, q Query, size float32) *Face {
	inst := Instance{Weight: q.weight(), Stretch: q.stretch()}
	if size > 0 && size == size && !math.IsInf(float64(size), 0) {
		inst.OpticalSize = float32(math.Round(float64(size)))
	}
	switch q.Style {
	case StyleItalic:
		if tf.style == StyleNormal && tf.axisIndex(tagItal) >= 0 {
			inst.Italic = true
		} else if tf.style == StyleNormal && tf.axisIndex(tagSlnt) >= 0 {
			inst.Oblique = true
		}
	case StyleOblique:
		if tf.style == StyleNormal && tf.axisIndex(tagSlnt) >= 0 {
			inst.Oblique = true
		}
	}
	return tf.Face(inst)
}

// matchFace implements the CSS Fonts 4 font matching algorithm (section
// 5.2, step 4) over the faces of one family: font-stretch, then
// font-style, then font-weight narrow the set. Variable faces match every
// value of their axis ranges.
func matchFace(faces []*Typeface, q Query) *Typeface {
	if len(faces) == 0 {
		return nil
	}
	set := append([]*Typeface(nil), faces...)
	set = narrowRange(set, q.stretch(), 100, 100, func(tf *Typeface) (float32, float32) { return tf.StretchRange() })
	set = narrowStyle(set, q.Style)
	set = narrowRange(set, q.weight(), 400, 500, func(tf *Typeface) (float32, float32) { return tf.WeightRange() })
	return set[0]
}

// narrowRange keeps the faces whose range contains want, or else those
// closest to it in the CSS search order: when want is below lowSplit,
// smaller values first (descending) then larger ones (ascending); when it
// is above highSplit, larger first then smaller; in between (weights 400
// to 500) larger values up to highSplit first, then smaller values, then
// values above highSplit. For font-stretch both splits are 100%.
func narrowRange(set []*Typeface, want, lowSplit, highSplit float32, rng func(*Typeface) (float32, float32)) []*Typeface {
	// score: lower is better.
	score := func(tf *Typeface) float64 {
		lo, hi := rng(tf)
		if lo <= want && want <= hi {
			return 0
		}
		below := hi < want // the closest value of the face is hi
		v := hi
		if !below {
			v = lo
		}
		d := float64(math.Abs(float64(v - want)))
		switch {
		case want < lowSplit:
			if below {
				return 1 + d/1e4
			}
			return 2 + d/1e4
		case want > highSplit:
			if !below {
				return 1 + d/1e4
			}
			return 2 + d/1e4
		default:
			if !below && v <= highSplit {
				return 1 + d/1e4
			}
			if below {
				return 2 + d/1e4
			}
			return 3 + d/1e4
		}
	}
	best := math.Inf(1)
	for _, tf := range set {
		best = min(best, score(tf))
	}
	out := set[:0]
	for _, tf := range set {
		if score(tf) == best {
			out = append(out, tf)
		}
	}
	return out
}

// narrowStyle keeps the faces of the best available style: italic prefers
// italic, then oblique, then normal; oblique prefers oblique, then italic,
// then normal; normal prefers normal, then oblique, then italic. A face
// with an ital axis also covers italic, one with a slnt axis also oblique.
func narrowStyle(set []*Typeface, want Style) []*Typeface {
	order := map[Style][3]Style{
		StyleNormal:  {StyleNormal, StyleOblique, StyleItalic},
		StyleItalic:  {StyleItalic, StyleOblique, StyleNormal},
		StyleOblique: {StyleOblique, StyleItalic, StyleNormal},
	}[want]
	covers := func(tf *Typeface, s Style) bool {
		switch {
		case tf.style == s:
			return true
		case s == StyleItalic:
			return tf.style == StyleNormal && tf.axisIndex(tagItal) >= 0
		case s == StyleOblique:
			return tf.style == StyleNormal && tf.axisIndex(tagSlnt) >= 0
		}
		return false
	}
	for _, s := range order {
		var out []*Typeface
		for _, tf := range set {
			if covers(tf, s) {
				out = append(out, tf)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return set
}

// maxCachedQueries bounds the cache of resolved queries.
const maxCachedQueries = 1024

// candidates are the families tried for a query, in fallback order. They
// are resolved by name when the query is first seen, without parsing
// anything; each family's face is made (and its font parsed) when the
// fallback walk first reaches it. The emoji and per-script lists are
// resolved on their first use. A candidates value is shared by every
// goroutine that lays out text with the query.
type candidates struct {
	c    *Collection
	q    Query
	size float32
	log  Logger

	primary []*famSlot      // the requested families
	rest    []*famSlot      // every other registered family, in order
	seen    map[string]bool // the keys of primary; read-only once built

	emojiOnce sync.Once
	emoji     []*famSlot // the emoji families

	scriptMu sync.Mutex
	scripts  map[language.Script][]*famSlot // per-script fallbacks

	// sys is the system font fallback, tried after rest; nil when system
	// fonts are not enabled.
	sys *systemFonts

	defOnce sync.Once
	def     *Face
	sysOnce sync.Once
	sysDef  *Face
}

// queryKey is the cache key of a query at an optical size: the families,
// length-prefixed, then the style values.
func queryKey(q Query, opsz float32) string {
	n := 13
	for _, f := range q.Families {
		n += len(f) + binary.MaxVarintLen64
	}
	b := make([]byte, 0, n)
	for _, f := range q.Families {
		b = binary.AppendUvarint(b, uint64(len(f)))
		b = append(b, f...)
	}
	b = binary.LittleEndian.AppendUint32(b, math.Float32bits(q.weight()))
	b = append(b, byte(q.Style))
	b = binary.LittleEndian.AppendUint32(b, math.Float32bits(q.stretch()))
	b = binary.LittleEndian.AppendUint32(b, math.Float32bits(opsz))
	return string(b)
}

func (c *Collection) candidates(q Query, size float32) *candidates {
	opsz := float32(0)
	if size > 0 && size == size && !math.IsInf(float64(size), 0) {
		opsz = float32(math.Round(float64(size)))
	}
	key := queryKey(q, opsz)
	c.mu.RLock()
	cands := c.cache[key]
	sys := c.sys
	need := cands == nil && c.needsIndex(q.Families)
	c.mu.RUnlock()
	if cands != nil {
		return cands
	}
	if need {
		sys.ensureIndex()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if cands := c.cache[key]; cands != nil {
		return cands
	}
	cands = &candidates{c: c, q: q, size: size, log: c.log, sys: c.sys, seen: map[string]bool{}, scripts: map[language.Script][]*famSlot{}}
	cands.primary = c.resolveSlots(q.Families, cands.seen, nil)
	for _, f := range c.order {
		if r := c.ref(f); !cands.seen[r.key] {
			cands.rest = append(cands.rest, &famSlot{alts: []famRef{r}})
		}
	}
	if len(c.cache) >= maxCachedQueries {
		clear(c.cache)
	}
	c.cache[key] = cands
	return cands
}

// emojiSlots returns the families of the generic name "emoji", resolved on
// first use.
func (cs *candidates) emojiSlots() []*famSlot {
	cs.emojiOnce.Do(func() {
		cs.emoji = cs.c.lazySlots(func() []string { return []string{"emoji"} }, nil)
	})
	return cs.emoji
}

// scriptSlots returns the fallback families of script that are not among
// the requested ones, resolved on first use; Hiragana and Katakana use the
// Han list when they have none of their own.
func (cs *candidates) scriptSlots(script language.Script) []*famSlot {
	l := cs.scriptList(script)
	if len(l) == 0 && (script == language.Hiragana || script == language.Katakana || script == language.Katakana_Or_Hiragana) {
		l = cs.scriptList(language.Han)
	}
	return l
}

func (cs *candidates) scriptList(script language.Script) []*famSlot {
	cs.scriptMu.Lock()
	l, ok := cs.scripts[script]
	cs.scriptMu.Unlock()
	if ok {
		return l
	}
	c := cs.c
	l = c.lazySlots(func() []string { return c.scripts[script] }, cs.seen)
	cs.scriptMu.Lock()
	defer cs.scriptMu.Unlock()
	if prev, ok := cs.scripts[script]; ok {
		return prev
	}
	cs.scripts[script] = l
	return l
}

// first returns the first face of slots that ok accepts.
func (cs *candidates) first(slots []*famSlot, ok func(*Face) bool) *Face {
	for _, s := range slots {
		if f := s.get(cs); f != nil && ok(f) {
			return f
		}
	}
	return nil
}

// defaultFace is the face of Face and the last resort of Itemize: the
// first requested family that can be used, else the first registered one,
// else the system's default.
func (cs *candidates) defaultFace() *Face {
	cs.defOnce.Do(func() {
		always := func(*Face) bool { return true }
		if cs.def = cs.first(cs.primary, always); cs.def == nil {
			if cs.def = cs.first(cs.rest, always); cs.def == nil {
				cs.def = cs.systemDefault()
			}
		}
	})
	if cs.def == nil {
		cs.c.reportNoFont()
	}
	return cs.def
}

// systemDefault returns the face of the operating system's sans-serif
// family (its list, see systemGenerics) for the query, else of an
// installed family that has Latin letters, else nil (no system fonts, or
// none installed).
func (cs *candidates) systemDefault() *Face {
	cs.sysOnce.Do(func() {
		s := cs.sys
		if s == nil {
			return
		}
		s.ensureIndex()
		names := append(slices.Clone(s.genericList("sans-serif")), s.fallbackFamilies([]rune{'a'}, language.Latin, cs.sysRequest())...)
		for _, name := range names {
			if !s.has(name) {
				continue
			}
			if f := s.familyByKey(name, cs.q); f != nil {
				cs.sysDef = instance(f.faces[0], cs.q, cs.size)
				return
			}
		}
	})
	return cs.sysDef
}
