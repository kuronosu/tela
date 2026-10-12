package text

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/fontscan"
)

// SystemFontOptions configures Collection.EnableSystemFonts. The zero value
// uses the operating system's font directories and an on-disk index in the
// user cache directory.
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
	// Lang chooses among the regional variants of a CJK family (Noto Sans
	// CJK JP, KR, SC, TC and HK). The default comes from the LC_ALL,
	// LC_MESSAGES and LANG environment variables, else Japanese forms are
	// preferred, then Simplified Chinese, Korean, Traditional Chinese.
	Lang string
	// Logger receives diagnostics of the scan (directories used, files that
	// could not be read, cache problems). The default discards them.
	Logger SystemFontLogger
}

// SystemFontLogger receives the diagnostics of the system font scan; a *log.Logger implements it.
type SystemFontLogger interface {
	Printf(format string, args ...any)
}

// EnableSystemFonts lets the collection use the fonts installed on the
// machine, found with go-text's fontscan. It is off by default and never
// enabled implicitly: NewDefaultCollection uses the embedded fonts only, so
// that snapshots and tests are the same on every machine (ADR-008). Do not
// call it from tests; under "go test" it returns an error unless
// TELA_SYSTEM_FONTS=1 is set or the options skip the system directories.
//
// System fonts serve two purposes:
//
//   - Named families. A requested family that is not registered (and is not
//     a generic name such as "serif") resolves to the installed family of
//     that name when there is one: "Segoe UI", "DejaVu Serif", "Noto Sans CJK
//     JP". Registered families always win over installed ones of the same
//     name. CSS matching (stretch, style, weight) is the collection's own;
//     only the face that matches is read from disk. Fonts that have no
//     outlines (bitmap-only color fonts such as Apple Color Emoji) are
//     ignored.
//   - Fallback. A grapheme cluster that no requested, script, registered or
//     embedded family covers (CJK, Hebrew, Devanagari, Thai...) is drawn with
//     an installed family that covers all of it. System fonts come after
//     everything else, so the embedded fonts still draw Latin, Arabic and
//     emoji exactly as without them, and the installed fonts only fill the
//     gaps. Among the installed families that cover a cluster the preference
//     is: the region of Lang for CJK families, then sans-serif designs over
//     serif, unknown and symbol or monospace families, then the family that
//     covers the fewest characters (a script-specific font over a pan-Unicode
//     one), then the name.
//
// Nothing is scanned or read until it is needed: the first unknown family
// name or the first cluster the other fonts cannot draw loads the index (a
// full scan the first time, then the cached index checks file times);
// parsing a font file waits until a face of it is used. Loaded fonts stay in
// memory with the collection (a TrueType collection is read once). The
// methods that use the fonts, Face, Match and Itemize, stay safe for
// concurrent use; the first one to need the index waits for it while others
// that need it too wait with it.
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
	logger SystemFontLogger
	prefs  []string // region suffixes in order of preference

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
	clusters  map[string][]string // cluster -> covering families, best first
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
	}
	if s.logger == nil {
		s.logger = nopLogger{}
	}
	return s
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

// family returns the installed family called name, with the face that
// matches q, or nil. Generic names never name a font.
func (s *systemFonts) family(name string, q Query) *family {
	key := normalizeFamily(name)
	if key == "" || isGenericName(key) {
		return nil
	}
	s.ensureIndex()
	if len(s.byFamily[key]) == 0 {
		return nil
	}
	return s.familyByKey(key, q)
}

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

// loadTypeface parses the font at e. It is ParseTypefaces for one font of a
// file: the same newTypeface, without parsing the other fonts of a
// collection (a Noto CJK collection holds five).
func (s *systemFonts) loadTypeface(e *sysEntry) (*Typeface, error) {
	data, ok := s.data[e.file]
	if !ok {
		var err error
		data, err = os.ReadFile(e.file)
		if err != nil {
			return nil, err
		}
		s.data[e.file] = data
	}
	lds, err := ot.NewLoaders(bytes.NewReader(data))
	if err == nil && e.index >= len(lds) {
		err = errors.New("the file changed since it was indexed")
	}
	if err == nil && !lds[e.index].HasTable(tagGlyf) && !lds[e.index].HasTable(tagCFF) && !lds[e.index].HasTable(tagCFF2) {
		err = errors.New("no outlines (bitmap-only font)")
	}
	var tf *Typeface
	if err == nil {
		tf, err = newTypeface(lds[e.index], "")
	}
	if err != nil {
		// Do not keep a file nothing was loaded from (a bitmap-only font).
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
// nil. It is the last stop of resolve.
func (cs *candidates) system(cluster []rune, avoidColor bool) *Face {
	s := cs.sys
	if s == nil {
		return nil
	}
	for _, key := range s.fallbackFamilies(cluster) {
		f := s.familyByKey(key, cs.q)
		if f == nil {
			continue
		}
		if avoidColor && f.faces[0].IsColor() {
			continue
		}
		if face := instance(f.faces[0], cs.q, cs.size); covers(face, cluster) {
			return face
		}
	}
	return nil
}

// fallbackFamilies returns the normalized names of the installed families
// that cover every character of cluster that needs a glyph, best first.
func (s *systemFonts) fallbackFamilies(cluster []rune) []string {
	key := string(cluster)
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
		type group struct {
			name   string
			nRunes int
			region int
			tier   int
		}
		var groups []group
		at := map[string]int{}
	entries:
		for i := range s.entries {
			e := &s.entries[i]
			for _, r := range need {
				if !e.runes.Contains(r) {
					continue entries
				}
			}
			if gi, ok := at[e.family]; ok {
				groups[gi].nRunes = min(groups[gi].nRunes, e.nRunes)
				continue
			}
			at[e.family] = len(groups)
			groups = append(groups, group{e.family, e.nRunes, regionRank(e.family, s.prefs), familyTier(e.family)})
		}
		sort.SliceStable(groups, func(i, j int) bool {
			a, b := groups[i], groups[j]
			switch {
			case a.tier != b.tier:
				return a.tier < b.tier
			case a.region != b.region:
				return a.region < b.region
			case a.nRunes != b.nRunes:
				return a.nRunes < b.nRunes
			}
			return a.name < b.name
		})
		for i := 0; i < len(groups) && i < maxFallbackFamilies; i++ {
			keys = append(keys, groups[i].name)
		}
	}

	s.mu.Lock()
	if len(s.clusters) >= maxClusterCache {
		clear(s.clusters)
	}
	s.clusters[key] = keys
	s.mu.Unlock()
	return keys
}
