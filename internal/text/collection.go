package text

import (
	"encoding/binary"
	"math"
	"strings"
	"sync"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
)

// Query is a CSS-like font request: a family list and the style
// properties used to match a face within a family (CSS Fonts 4, section
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
// Fallback order for a cluster of text (see Itemize): the requested
// families, then the families registered for the cluster's script
// (SetScriptFallback), then every other family in registration order with
// the embedded families last, then, only when EnableSystemFonts was called,
// the installed system fonts. Emoji presentation clusters try the emoji
// families (generic "emoji") first.
//
// Variable faces are instantiated per optical size: CSS
// font-optical-sizing: auto sets opsz to the font size in CSS pixels, and
// the Collection rounds it to whole pixels (clamped to the axis), so a
// typeface has at most one instance per (weight, stretch, style, pixel
// size) used. Instances are interned for the life of the Collection's
// typefaces.
//
// No system fonts are used unless EnableSystemFonts is called (ADR-008);
// synthetic bold and oblique are not implemented: a request for a weight or
// style that no face covers gets the closest face as CSS matching defines
// it, unchanged.
type Collection struct {
	mu       sync.RWMutex
	families map[string]*family // by normalized name
	order    []*family          // registration order
	generics map[string][]string
	scripts  map[language.Script][]string
	cache    map[string]*candidates
	sys      *systemFonts // nil unless EnableSystemFonts was called
}

type family struct {
	name     string
	faces    []*Typeface
	embedded bool
}

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
func (c *Collection) AddTypeface(tf *Typeface) { c.add(tf, false) }

func (c *Collection) add(tf *Typeface, embedded bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := normalizeFamily(tf.family)
	f := c.families[key]
	if f == nil {
		f = &family{name: tf.family, embedded: embedded}
		c.families[key] = f
		c.order = append(c.order, f)
	}
	f.faces = append(f.faces, tf)
	clear(c.cache)
}

// SetGeneric maps a generic family name (such as "sans-serif") to a list of
// families, replacing the previous mapping.
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
func (c *Collection) Families() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.order))
	for i, f := range c.order {
		out[i] = f.name
	}
	return out
}

// resolveNames expands generic names into family records, skipping
// unknown names and duplicates. A name no registered family has is looked up
// among the system fonts when they are enabled (q picks the face of the
// system family). c.mu must be held.
func (c *Collection) resolveNames(names []string, q Query, seen map[*family]bool, out []*family) []*family {
	var walk func(names []string, depth int)
	walk = func(names []string, depth int) {
		for _, n := range names {
			key := normalizeFamily(n)
			if f := c.families[key]; f != nil {
				if !seen[f] {
					seen[f] = true
					out = append(out, f)
				}
				continue
			}
			if g, ok := c.generics[key]; ok {
				if depth < 4 {
					walk(g, depth+1)
				}
				continue
			}
			if c.sys != nil {
				if f := c.sys.family(n, q); f != nil && !seen[f] {
					seen[f] = true
					out = append(out, f)
				}
			}
		}
	}
	walk(names, 0)
	return out
}

// systemNames appends to out the names among names, and among the generic
// names they expand to, that resolveNames looks up in the system fonts.
// c.mu must be held.
func (c *Collection) systemNames(names []string, out []string) []string {
	var walk func(names []string, depth int)
	walk = func(names []string, depth int) {
		for _, n := range names {
			key := normalizeFamily(n)
			if c.families[key] != nil {
				continue
			}
			if g, ok := c.generics[key]; ok {
				if depth < 4 {
					walk(g, depth+1)
				}
				continue
			}
			out = append(out, n)
		}
	}
	walk(names, 0)
	return out
}

// loadSystemNames looks names up in sys before c.mu is taken. The first
// lookup scans the font directories and a new family parses its file, which
// can take seconds; done under c.mu, it would block the callers that need
// only registered fonts. resolveNames then finds the families cached.
func loadSystemNames(sys *systemFonts, names []string, q Query) {
	for _, n := range names {
		sys.family(n, q)
	}
}

// Match returns the typeface of family that CSS font matching selects for
// q, or nil if the family (or generic name) is unknown.
func (c *Collection) Match(familyName string, q Query) *Typeface {
	c.mu.RLock()
	sys, names := c.sys, []string{familyName}
	var sysNames []string
	if sys != nil {
		sysNames = c.systemNames(names, nil)
	}
	c.mu.RUnlock()
	loadSystemNames(sys, sysNames, q)

	c.mu.RLock()
	defer c.mu.RUnlock()
	fams := c.resolveNames(names, q, map[*family]bool{}, nil)
	if len(fams) == 0 {
		return nil
	}
	return matchFace(fams[0].faces, q)
}

// Face returns the face of the first available family of q, instantiated
// for size (CSS pixels, used for the optical size), or the first fallback
// family's face when none of q's families exists. It returns nil only for
// an empty collection.
func (c *Collection) Face(q Query, size float32) *Face {
	cands := c.candidates(q, size)
	if len(cands.primary) > 0 {
		return cands.primary[0]
	}
	if len(cands.rest) > 0 {
		return cands.rest[0]
	}
	return nil
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

// candidates are the faces tried for a query, in fallback order.
type candidates struct {
	primary []*Face                     // the requested families
	emoji   []*Face                     // the emoji families
	scripts map[language.Script][]*Face // per-script fallbacks
	rest    []*Face                     // every other family, embedded last

	// sys is the system font fallback, tried after rest; nil when system
	// fonts are not enabled. q and size select the face of a system family.
	sys  *systemFonts
	q    Query
	size float32
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
	var sysNames []string
	if cands == nil && sys != nil {
		sysNames = c.systemNames(q.Families, nil)
		sysNames = c.systemNames(c.generics["emoji"], sysNames)
		for _, names := range c.scripts {
			sysNames = c.systemNames(names, sysNames)
		}
	}
	c.mu.RUnlock()
	if cands != nil {
		return cands
	}
	loadSystemNames(sys, sysNames, q)

	c.mu.Lock()
	defer c.mu.Unlock()
	if cands := c.cache[key]; cands != nil {
		return cands
	}
	faceOf := func(f *family) *Face { return instance(matchFace(f.faces, q), q, size) }
	seen := map[*family]bool{}
	cands = &candidates{scripts: map[language.Script][]*Face{}, sys: c.sys, q: q, size: size}
	for _, f := range c.resolveNames(q.Families, q, seen, nil) {
		cands.primary = append(cands.primary, faceOf(f))
	}
	for _, f := range c.resolveNames(c.generics["emoji"], q, map[*family]bool{}, nil) {
		cands.emoji = append(cands.emoji, faceOf(f))
	}
	for s, names := range c.scripts {
		for _, f := range c.resolveNames(names, q, map[*family]bool{}, nil) {
			if !seen[f] {
				cands.scripts[s] = append(cands.scripts[s], faceOf(f))
			}
		}
	}
	var embedded []*Face
	for _, f := range c.order {
		if seen[f] {
			continue
		}
		if f.embedded {
			embedded = append(embedded, faceOf(f))
		} else {
			cands.rest = append(cands.rest, faceOf(f))
		}
	}
	cands.rest = append(cands.rest, embedded...)
	if len(c.cache) >= maxCachedQueries {
		clear(c.cache)
	}
	c.cache[key] = cands
	return cands
}
