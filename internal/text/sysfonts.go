package text

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/language"
)

// SystemFontOptions configures Collection.EnableSystemFonts and
// NewSystemCollection. The zero value uses the operating system's font
// directories, an on-disk index in the user cache directory and the
// operating system's generic families.
type SystemFontOptions struct {
	// CacheDir is the directory of go-text's font index cache (the file
	// font_index_v*.cache). The default is os.UserCacheDir()/tela/fonts.
	//
	// The index is process-wide and loaded once (fontscan.SystemFonts): the
	// first Collection that reaches the system fonts decides the directory
	// for the whole process. When the directory cannot be created or written
	// the fonts are scanned without a cache instead (slower, every start),
	// and the problem is reported to Logger.
	CacheDir string
	// NoCache scans the font directories on every start instead of using an
	// on-disk index.
	NoCache bool
	// Dirs are extra directories scanned for fonts (recursively), besides
	// the operating system's. They are scanned when the fonts are first
	// needed, on every start (they have no on-disk index), so they should
	// stay small.
	Dirs []string
	// SkipSystemDirs ignores the operating system's font directories and
	// uses only Dirs. It makes the set of fonts independent of the machine,
	// which is what the tests use.
	SkipSystemDirs bool
	// Lang is the preferred language as a BCP 47 tag ("ja", "ko", "zh-Hans",
	// "zh-TW"...). Unified Han characters have no language of their own;
	// for text whose own language (Query.Lang) is not Chinese, Japanese or
	// Korean, Lang chooses among the CJK families of each region (Noto
	// Sans CJK JP, KR, SC, TC and HK; Yu Gothic, Malgun Gothic, Microsoft
	// YaHei, Microsoft JhengHei...). The default is the user's language
	// (SystemLanguage); when that is not CJK either, Japanese forms are
	// preferred, then Simplified Chinese, Korean, Traditional Chinese.
	Lang string
	// UIFamily and MonoFamily are the desktop's interface and monospace
	// font families, when the platform can tell them (on Linux, the
	// desktop's settings). When set and installed, UIFamily comes first
	// for system-ui, ui-sans-serif and sans-serif, and MonoFamily for
	// ui-monospace and monospace, before the operating system's lists
	// (systemGenerics).
	UIFamily, MonoFamily string
	// Logger receives diagnostics of the scan (directories used, files that
	// could not be read, cache problems). The default discards them.
	Logger Logger

	// goos selects the generic family lists; "" is runtime.GOOS. Tests set
	// it to check another operating system's lists.
	goos string
}

// EnableSystemFonts lets the collection use the fonts installed on the
// machine, found with go-text's fontscan. A collection made by
// NewCollection does not use them until this is called;
// NewSystemCollection, which internal/core uses for an app's text when the
// app embeds no font (ADR-244), calls it. Tests must not depend on the
// installed fonts (ADR-008): under "go test" it returns an error unless
// TELA_SYSTEM_FONTS=1 is set or the options skip the system directories.
//
// System fonts serve three purposes:
//
//   - Generic families. A generic name ("system-ui", "ui-sans-serif",
//     "sans-serif", "ui-serif", "serif", "ui-monospace", "monospace",
//     "emoji", "cursive", "fantasy", "math", "ui-rounded" on macOS) that
//     the collection does not map (SetGeneric, FontSource.Generics)
//     resolves to the first installed family of the operating system's
//     list for it: macOS SF (system-ui, ui-sans-serif), Helvetica, New
//     York, Times, SF Mono and Menlo, Apple Color Emoji; Windows Segoe UI,
//     Cambria and Times New Roman, Cascadia Mono, Consolas and Courier
//     New, Segoe UI Emoji; on Linux and the BSDs fontconfig's preferences
//     (Cantarell, Noto Sans, DejaVu Sans, Liberation Sans...). The
//     desktop's own families (SystemFontOptions.UIFamily, MonoFamily) come
//     first. See systemGenerics for the lists.
//   - Named families. A requested family that is not registered resolves
//     to the installed family of that name when there is one: "Segoe UI",
//     "DejaVu Serif", "Noto Sans CJK JP". Registered families always win
//     over installed ones of the same name. CSS matching (stretch, style,
//     weight) is the collection's own; only the face that matches is read
//     from disk. Fonts that have neither outlines nor color bitmaps
//     (CBDT, sbix: Noto Color Emoji, Apple Color Emoji) are ignored.
//   - Fallback. A grapheme cluster that no requested, script or registered
//     family covers (CJK, Hebrew, Devanagari, Thai...) is drawn with an
//     installed family that covers all of it. System fonts come after
//     everything else, so the registered fonts draw what they can and the
//     installed fonts fill the gaps. A script is drawn by one family: the
//     installed families that support the script (their fonts cover at
//     least half as much of it as the font that covers most) are ranked
//     once per script, and a cluster takes the first of them that covers
//     it, so Thai is Noto Sans Thai throughout and not partly a narrower
//     font that has some Thai characters. The rank is: sans-serif designs
//     over unknown, serif and symbol or monospace families, then the
//     distance from the requested weight to the family's closest font (in
//     steps of 200: a family installed only in Thin loses to one with a
//     Regular), then the region of the language for CJK families (Lang,
//     or the text's own language; the regional variants of pan-CJK designs
//     and the known Windows, macOS and Linux CJK families, such as Yu
//     Gothic, Microsoft YaHei, Malgun Gothic), then the family that covers
//     the fewest characters (Noto Sans Thai, 101, before its Looped design,
//     146; a script-specific font over a pan-Unicode one), then the name. A
//     CJK family of a region supports the scripts of its region however few
//     ideographs it has. Clusters with no script of their own (punctuation)
//     use the script of their run. Families that do not support the script
//     come after, by the same rank, for the clusters only they cover. In a
//     paragraph laid out by NewLayout a script takes one family (Han, kana
//     and Bopomofo together): the first in that order that has every
//     character of the script the registered fonts lack, else the one that
//     has most of them, so Japanese text with Chinese characters is not
//     drawn by two families.
//
// Nothing is scanned or read until it is needed: the first generic or
// unknown family name or the first cluster the other fonts cannot draw
// loads the index (a full scan the first time, then the cached index
// checks file times); parsing a font file waits until a face of it is
// used. Loaded fonts stay with the collection (a TrueType collection is
// read once); their files are mapped into memory where the operating
// system allows it, so only the pages that are read take memory (see
// readFontFile). The methods that use the fonts, Face, Match
// and Itemize, stay safe for concurrent use; the first one to need the
// index waits for it while others that need it too wait with it, and those
// that do not need it go on.
//
// Fonts load through the same code as ParseTypefaces, so they get the same
// instances, variations and determinism: TrueType outlines through this
// package's port (ADR-030), CFF and CFF2 through go-text (CFF2 outlines may
// differ in their last bits across architectures). Installed fonts are not
// reproducible across machines by nature.
//
// Calling it again replaces the options and drops what was loaded.
func (c *Collection) EnableSystemFonts(opts SystemFontOptions) error {
	if !opts.SkipSystemDirs && inTestBinary() && os.Getenv("TELA_SYSTEM_FONTS") == "" {
		return errors.New("text: system fonts are disabled under go test, because goldens must not depend on the installed fonts; set TELA_SYSTEM_FONTS=1 to allow them or use SystemFontOptions{Dirs: ..., SkipSystemDirs: true}")
	}
	for _, d := range opts.Dirs {
		if d == "" {
			return errors.New("text: SystemFontOptions.Dirs has an empty entry")
		}
	}
	s := newSystemFonts(opts)
	c.mu.Lock()
	c.sys = s
	clear(c.cache)
	c.mu.Unlock()
	return nil
}

// NewSystemCollection returns a collection of the fonts installed on the
// machine: nothing is registered, the generic families resolve to the
// operating system's (see EnableSystemFonts) and every other name to the
// installed family of that name. Families registered later (AddSource,
// Register) come before the installed ones. When the system fonts cannot
// be enabled (under go test, see EnableSystemFonts) it returns an empty
// collection, which lays text out with no glyphs, and the error.
func NewSystemCollection(opts SystemFontOptions) (*Collection, error) {
	c := NewCollection()
	return c, c.EnableSystemFonts(opts)
}

// SetDesktopFamilies replaces the desktop's interface and monospace
// families (SystemFontOptions.UIFamily and MonoFamily) of a collection
// that uses the system fonts, and reports whether they changed. A change
// drops the resolved queries, so the layouts made after it resolve
// system-ui, ui-sans-serif, sans-serif, ui-monospace and monospace with
// the new families; layouts made before keep the faces they have (the
// caller lays its text out again: internal/core does on a SettingsEvent).
// What was loaded stays: the index of the installed fonts and the parsed
// faces. Without system fonts it does nothing and returns false. Safe for
// concurrent use with every other method.
func (c *Collection) SetDesktopFamilies(ui, mono string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sys
	if s == nil {
		return false
	}
	if g := s.generics.Load(); g.ui == ui && g.mono == mono {
		return false
	}
	s.generics.Store(newGenericLists(s.opts.goos, ui, mono))
	clear(c.cache)
	return true
}

// Preload loads the index of the installed fonts on the calling
// goroutine, ahead of the first text that needs it: a scan of the font
// directories the first time, the on-disk index otherwise (about 20 ms
// for 800 fonts). internal/core calls it on a goroutine when an app
// starts, so that the index loads while the window opens instead of in
// the first frame. Without system fonts, or once the index is loaded, it
// does nothing. Safe for concurrent use: text laid out meanwhile waits
// for the same load instead of starting another.
//
// It parses no face: on Linux the desktop's families arrive with the
// platform's settings, after the index load starts, and parsing a face
// takes far less than the index.
func (c *Collection) Preload() {
	c.mu.RLock()
	s := c.sys
	c.mu.RUnlock()
	if s != nil {
		s.ensureIndex()
	}
}

// DisableSystemFonts turns system fonts off again and drops what was loaded.
func (c *Collection) DisableSystemFonts() {
	c.mu.Lock()
	c.sys = nil
	clear(c.cache)
	c.mu.Unlock()
}

// SystemFontsErr returns the error that kept the system fonts from loading
// (no font directory, unreadable index...), or nil if they are not enabled,
// not loaded yet or loaded. When it is not nil the collection keeps working
// with its own fonts.
func (c *Collection) SystemFontsErr() error {
	c.mu.RLock()
	s := c.sys
	c.mu.RUnlock()
	if s == nil || !s.indexed.Load() {
		return nil
	}
	return s.err
}

// inTestBinary reports whether the program is a test binary.
func inTestBinary() bool { return flag.Lookup("test.v") != nil }

type nopLogger struct{}

func (nopLogger) Printf(string, ...any) {}

// sysEntry is one font of the system index: a file and the index of the
// font in it, with what is needed to match and cover without parsing it.
type sysEntry struct {
	file    string
	index   int
	family  string // normalized
	style   Style
	weight  float32 // CSS weight
	stretch float32 // CSS percentage
	runes   fontscan.RuneSet
	nRunes  int
}

type entryKey struct {
	file  string
	index int
}

// famKey identifies a system family at the style values that selected its
// face.
type famKey struct {
	name    string
	weight  float32
	style   Style
	stretch float32
}

// maxClusterCache bounds the memo of fallback families per cluster.
const maxClusterCache = 8192

// maxFallbackFamilies is how many covering families a cluster remembers.
const maxFallbackFamilies = 8

// systemFonts is the lazily loaded view of the installed fonts of one
// Collection.
type systemFonts struct {
	opts   SystemFontOptions
	logger Logger
	prefs  []string // region suffixes in order of preference
	// generics maps normalized generic names to normalized families, in
	// order (systemGenerics); SetDesktopFamilies replaces it under the
	// collection's lock, and readers that do not hold it load it once.
	generics atomic.Pointer[genericLists]

	indexOnce sync.Once
	indexed   atomic.Bool
	entries   []sysEntry
	byFamily  map[string][]int
	err       error
	scanTime  time.Duration

	mu        sync.RWMutex
	data      map[string][]byte // file contents, kept by the typefaces
	typefaces map[entryKey]*Typeface
	fams      map[famKey]*family
	clusters  map[string][]string          // script, request and cluster -> covering families, best first
	scripts   map[scriptKey][]string       // script and request -> families that support it, best first
	support   map[language.Script][]string // script -> families that support it, by name
}

// scriptKey is a script and the key of a fallback request.
type scriptKey struct {
	script language.Script
	req    string
}

func newSystemFonts(opts SystemFontOptions) *systemFonts {
	s := &systemFonts{
		opts:      opts,
		logger:    opts.Logger,
		prefs:     regionPreference(opts.Lang),
		data:      map[string][]byte{},
		typefaces: map[entryKey]*Typeface{},
		fams:      map[famKey]*family{},
		clusters:  map[string][]string{},
		scripts:   map[scriptKey][]string{},
		support:   map[language.Script][]string{},
	}
	if s.logger == nil {
		s.logger = nopLogger{}
	}
	s.generics.Store(newGenericLists(opts.goos, opts.UIFamily, opts.MonoFamily))
	return s
}

// genericLists are the generic family lists of a systemFonts, with the
// desktop's families they were made with. They are not modified once
// made.
type genericLists struct {
	ui, mono string
	lists    map[string][]string
}

func newGenericLists(goos, ui, mono string) *genericLists {
	return &genericLists{ui: ui, mono: mono, lists: systemGenerics(goos, ui, mono)}
}

// ensureIndex loads the index of installed fonts once.
func (s *systemFonts) ensureIndex() {
	s.indexOnce.Do(func() {
		start := time.Now()
		var fps []fontscan.Footprint
		if !s.opts.SkipSystemDirs {
			var err error
			fps, err = s.systemFootprints()
			if err != nil {
				s.err = fmt.Errorf("text: system fonts: %w", err)
				s.logger.Printf("system fonts unavailable: %v", err)
			}
		}
		for _, dir := range s.opts.Dirs {
			fps = append(fps, scanDirectory(dir, s.logger)...)
		}
		s.entries = make([]sysEntry, 0, len(fps))
		s.byFamily = map[string][]int{}
		for _, fp := range fps {
			if fp.Family == "" {
				continue
			}
			e := sysEntry{
				file:    fp.Location.File,
				index:   int(fp.Location.Index),
				family:  fp.Family,
				weight:  float32(fp.Aspect.Weight),
				stretch: float32(fp.Aspect.Stretch) * 100,
				runes:   fp.Runes,
				nRunes:  fp.Runes.Len(),
			}
			if fp.Aspect.Style == font.StyleItalic {
				e.style = StyleItalic
			}
			s.byFamily[e.family] = append(s.byFamily[e.family], len(s.entries))
			s.entries = append(s.entries, e)
		}
		s.scanTime = time.Since(start)
		s.logger.Printf("indexed %d system fonts in %v", len(s.entries), s.scanTime)
		s.indexed.Store(true)
	})
}

// has reports whether an installed family has the normalized name key,
// loading the index on first use.
func (s *systemFonts) has(key string) bool {
	s.ensureIndex()
	return len(s.byFamily[key]) > 0
}

// genericList returns the normalized names of the installed families that
// the generic name key (normalized) resolves to, in order of preference;
// the families need not be installed.
func (s *systemFonts) genericList(key string) []string { return s.generics.Load().lists[key] }

// familyByKey returns the family with the face of the entry that best
// matches q among the entries of the normalized family name key, parsed on
// first use, or nil if none can be loaded. The index must be loaded.
func (s *systemFonts) familyByKey(key string, q Query) *family {
	fk := famKey{key, q.weight(), q.Style, q.stretch()}
	s.mu.RLock()
	f, ok := s.fams[fk]
	s.mu.RUnlock()
	if ok {
		return f
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.fams[fk]; ok {
		return f
	}
	idxs := append([]int(nil), s.byFamily[key]...)
	for len(idxs) > 0 {
		pos := s.matchEntry(idxs, q)
		if tf := s.typeface(&s.entries[idxs[pos]]); tf != nil {
			f = &family{name: tf.family, faces: []*Typeface{tf}}
			break
		}
		idxs = append(idxs[:pos], idxs[pos+1:]...)
	}
	s.fams[fk] = f
	return f
}

// matchEntry returns the position in idxs of the entry CSS font matching
// selects for q. It runs the collection's own matching (matchFace) over
// stand-in typefaces carrying the entries' style, weight and stretch, so
// that nothing is parsed. Variable fonts count at their default values, as
// in fontscan's index.
func (s *systemFonts) matchEntry(idxs []int, q Query) int {
	fakes := make([]*Typeface, len(idxs))
	for i, ix := range idxs {
		e := &s.entries[ix]
		fakes[i] = &Typeface{
			style:   e.style,
			weight:  [3]float32{e.weight, e.weight, e.weight},
			stretch: [3]float32{e.stretch, e.stretch, e.stretch},
		}
	}
	best := matchFace(fakes, q)
	for i, f := range fakes {
		if f == best {
			return i
		}
	}
	return 0
}

// typeface returns the parsed font of e, or nil if it cannot be read or
// parsed. s.mu must be held for writing.
func (s *systemFonts) typeface(e *sysEntry) *Typeface {
	k := entryKey{e.file, e.index}
	if tf, ok := s.typefaces[k]; ok {
		return tf
	}
	tf, err := s.loadTypeface(e)
	if err != nil {
		s.logger.Printf("skipping system font %s (%d): %v", e.file, e.index, err)
		tf = nil
	}
	s.typefaces[k] = tf
	return tf
}

var (
	tagGlyf = ot.MustNewTag("glyf")
	tagCFF  = ot.MustNewTag("CFF ")
	tagCFF2 = ot.MustNewTag("CFF2")
)

// hasGlyphs reports whether the font has outlines or color bitmaps.
func hasGlyphs(ld *ot.Loader) bool {
	return ld.HasTable(tagGlyf) || ld.HasTable(tagCFF) || ld.HasTable(tagCFF2) ||
		ld.HasTable(tagSbix) || (ld.HasTable(tagCBLC) && ld.HasTable(tagCBDT))
}

// loadTypeface parses the font at e. It is ParseTypefaces for one font of a
// file: the same newTypeface, without parsing the other fonts of a
// collection (a Noto CJK collection holds five).
func (s *systemFonts) loadTypeface(e *sysEntry) (*Typeface, error) {
	data, ok := s.data[e.file]
	if !ok {
		var err error
		data, err = readFontFile(e.file)
		if err != nil {
			return nil, err
		}
		s.data[e.file] = data
	}
	lds, err := ot.NewLoaders(bytes.NewReader(data))
	if err == nil && e.index >= len(lds) {
		err = errors.New("the file changed since it was indexed")
	}
	if err == nil && !hasGlyphs(lds[e.index]) {
		err = errors.New("no outlines and no color bitmaps (a grayscale bitmap font)")
	}
	var tf *Typeface
	if err == nil {
		tf, err = newTypeface(newTableSource(lds[e.index], data, e.index), "")
	}
	if err != nil {
		// Do not keep a file nothing was loaded from.
		keep := false
		for k, tf := range s.typefaces {
			if k.file == e.file && tf != nil {
				keep = true
			}
		}
		if !keep {
			delete(s.data, e.file)
		}
		return nil, err
	}
	return tf, nil
}

// system returns the face of an installed family that covers cluster, or
// nil. script is the cluster's script (the run's when the cluster has none
// of its own). scope, when not nil, is the paragraph's: the family it
// picked for the script's group comes first. It is the last stop of
// resolve.
func (cs *candidates) system(cluster []rune, script language.Script, avoidColor bool, scope *paraScope) *Face {
	s := cs.sys
	if s == nil {
		return nil
	}
	req := cs.sysRequest()
	try := func(key string) *Face {
		f := s.familyByKey(key, cs.q)
		if f == nil || avoidColor && f.faces[0].IsColor() {
			return nil
		}
		if face := instance(f.faces[0], cs.q, cs.size); covers(face, cluster) {
			return face
		}
		return nil
	}
	if scope != nil {
		if key := scope.pick(cs, scriptGroup(script), req); key != "" {
			if face := try(key); face != nil {
				return face
			}
		}
	}
	for _, key := range s.fallbackFamilies(cluster, script, req) {
		if face := try(key); face != nil {
			return face
		}
	}
	return nil
}

// sysRequest is what ranks the installed families for the query of cs:
// its weight, and the regions of its language when it is Chinese,
// Japanese or Korean, else the collection's (SystemFontOptions.Lang).
func (cs *candidates) sysRequest() fbRequest {
	prefs := cs.sys.prefs
	if r := cjkRegion(cs.q.Lang); r != "" {
		prefs = regionsFrom(r)
	}
	return fbRequest{weight: cs.q.weight(), prefs: prefs}
}

// fbRequest is what ranks the installed families for a fallback, besides
// the families themselves: the requested weight and the order of the CJK
// regions (regionPreference).
type fbRequest struct {
	weight float32
	prefs  []string
}

// forScript returns the request for the clusters of script: kana are
// Japanese, Hangul Korean and Bopomofo Traditional Chinese whatever the
// language, as in Chromium's per-script fonts, so their region comes
// first; Han and the other scripts keep the language's order.
func (r fbRequest) forScript(script language.Script) fbRequest {
	first := ""
	switch script {
	case language.Hiragana, language.Katakana, language.Katakana_Or_Hiragana:
		first = "jp"
	case language.Hangul:
		first = "kr"
	case language.Bopomofo:
		first = "tc"
	default:
		return r
	}
	if len(r.prefs) > 0 && r.prefs[0] == first {
		return r
	}
	r.prefs = regionsFrom(first)
	return r
}

// key identifies the order the request gives: the weight and the first
// region (the others follow defaultRegions).
func (r fbRequest) key() string {
	k := binary.BigEndian.AppendUint32(nil, math.Float32bits(r.weight))
	if len(r.prefs) > 0 {
		k = append(k, r.prefs[0]...)
	}
	return string(k)
}

// famRank is what ranks a family: lower is better, in this order.
type famRank struct {
	name   string
	tier   int // familyTier
	weight int // weightRank of its closest font
	region int // regionRank
	nRunes int // characters of its smallest font: specific before pan-Unicode
}

func (a famRank) less(b famRank) bool {
	switch {
	case a.tier != b.tier:
		return a.tier < b.tier
	case a.weight != b.weight:
		return a.weight < b.weight
	case a.region != b.region:
		return a.region < b.region
	case a.nRunes != b.nRunes:
		return a.nRunes < b.nRunes
	}
	return a.name < b.name
}

// rankOf returns the rank of the family called name for req. s.mu is not
// needed: the index is read-only.
func (s *systemFonts) rankOf(name string, req fbRequest) famRank {
	r := famRank{name: name, tier: familyTier(name), region: regionRank(name, req.prefs), nRunes: -1, weight: -1}
	for _, i := range s.byFamily[name] {
		e := &s.entries[i]
		if n := e.nRunes; r.nRunes < 0 || n < r.nRunes {
			r.nRunes = n
		}
		if w := weightRank(e.weight, req.weight); r.weight < 0 || w < r.weight {
			r.weight = w
		}
	}
	return r
}

// sortRanked returns names sorted by rankOf for req.
func (s *systemFonts) sortRanked(names []string, req fbRequest) []string {
	ranks := make([]famRank, len(names))
	for i, n := range names {
		ranks[i] = s.rankOf(n, req)
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i].less(ranks[j]) })
	out := make([]string, len(ranks))
	for i, r := range ranks {
		out[i] = r.name
	}
	return out
}

// maxScriptProbes bounds the characters sampled per script to score the
// support of every font.
const maxScriptProbes = 2048

// scriptFamilies returns the installed families that support script, best
// first (famRank for req). A family supports a script when its fonts cover
// at least half as much of it as the font that covers most, or when it is
// a CJK family made for the script (regionSupports: a Japanese font has
// far fewer ideographs than a Chinese one); coverage is counted on an even
// sample of the script's characters (Unicode's script ranges).
//
// The list depends only on the script, the request and the installed
// fonts, never on the text: it is what keeps every cluster of a script on
// one family (Noto Sans Thai, not Noto Sans Thai Looped for some words), as
// a browser does.
func (s *systemFonts) scriptFamilies(script language.Script, req fbRequest) []string {
	req = req.forScript(script)
	key := scriptKey{script, req.key()}
	s.mu.RLock()
	names, ok := s.scripts[key]
	s.mu.RUnlock()
	if ok {
		return names
	}
	names = s.sortRanked(s.scriptSupport(script), req)
	s.mu.Lock()
	s.scripts[key] = names
	s.mu.Unlock()
	return names
}

// scriptSupport returns the installed families that support script (see
// scriptFamilies), in no order.
func (s *systemFonts) scriptSupport(script language.Script) []string {
	s.mu.RLock()
	names, ok := s.support[script]
	s.mu.RUnlock()
	if ok {
		return names
	}
	var runes []rune
	for _, rg := range language.ScriptRanges {
		if rg.Script != script {
			continue
		}
		for r := rg.Start; r <= rg.End; r++ {
			runes = append(runes, r)
		}
	}
	if stride := (len(runes) + maxScriptProbes - 1) / maxScriptProbes; stride > 1 {
		for i := 0; i*stride < len(runes); i++ {
			runes[i] = runes[i*stride]
		}
		runes = runes[:(len(runes)+stride-1)/stride]
	}
	score := map[string]int{}
	best := 0
	for i := range s.entries {
		e := &s.entries[i]
		n := 0
		for _, r := range runes {
			if e.runes.Contains(r) {
				n++
			}
		}
		if n > score[e.family] {
			score[e.family] = n
		}
		best = max(best, n)
	}
	names = []string{}
	for name, n := range score {
		if n > 0 && (2*n >= best || regionSupports(regionOf(name), script)) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	s.mu.Lock()
	s.support[script] = names
	s.mu.Unlock()
	return names
}

// usesScriptList reports whether clusters of script share a family list.
func usesScriptList(script language.Script) bool {
	return script != language.Common && script != language.Inherited && script != language.Unknown
}

// fallbackFamilies returns the normalized names of the installed families
// that cover every character of cluster that needs a glyph, in order of
// preference for req: first the families that support the cluster's
// script, in the script's order (so that a script is drawn by one family
// wherever that family has the characters), then every other covering
// family by famRank.
func (s *systemFonts) fallbackFamilies(cluster []rune, script language.Script, req fbRequest) []string {
	req = req.forScript(script)
	key := string(binary.BigEndian.AppendUint32(nil, uint32(script))) + req.key() + "\x00" + string(cluster) // script tags are not valid runes
	s.mu.RLock()
	keys, ok := s.clusters[key]
	s.mu.RUnlock()
	if ok {
		return keys
	}
	s.ensureIndex()

	var need []rune
	for _, r := range cluster {
		if !isIgnorable(r) {
			need = append(need, r)
		}
	}
	if len(need) > 0 {
		has := func(name string) bool {
		entries:
			for _, i := range s.byFamily[name] {
				for _, r := range need {
					if !s.entries[i].runes.Contains(r) {
						continue entries
					}
				}
				return true
			}
			return false
		}
		seen := map[string]bool{}
		if usesScriptList(script) {
			for _, name := range s.scriptFamilies(script, req) {
				if has(name) {
					seen[name] = true
					keys = append(keys, name)
				}
			}
		}
		if len(keys) < maxFallbackFamilies {
			var rest []string
			for name := range s.byFamily {
				if !seen[name] && has(name) {
					rest = append(rest, name)
				}
			}
			keys = append(keys, s.sortRanked(rest, req)...)
		}
		keys = keys[:min(len(keys), maxFallbackFamilies)]
	}

	s.mu.Lock()
	if len(s.clusters) >= maxClusterCache {
		clear(s.clusters)
	}
	s.clusters[key] = keys
	s.mu.Unlock()
	return keys
}
