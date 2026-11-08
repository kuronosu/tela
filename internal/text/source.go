package text

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/language"
)

// FontSource is a font family that a Collection registers without parsing
// it (Collection.AddSource): its files are read and parsed when text first
// needs the family, when it is requested by name or through a generic name
// it stands for, when it is the fallback of a script that text uses, or
// when fallback walks the registered families to draw a character that the
// families before it lack (ADR-244).
//
// Files are parsed in two steps, each at most once for the life of the
// files, whichever goroutine, Collection and source need them first (the
// others wait): the first need of the family reads the files (Load) and
// describes their fonts from their small tables (name, OS/2, head, fvar)
// without parsing them; font matching (CSS Fonts 4, as for any family)
// then picks the font of the request among them, and only that font is
// parsed. So a family of an upright and an italic file that draws only
// upright text never parses the italic one. The typefaces are immutable
// and shared by every Collection the source is added to, as are their
// faces.
//
// The parse belongs to the files, not to the source: every source with the
// same Family over the same files (the same byte slices, not equal copies,
// in the same order, whether in Data or returned by Load) shares one
// description and one parse, and the same typefaces, for as long as one of
// them is in use. So sources that give one family different roles
// (Generics, Scripts), or that two packages build over the same embedded
// bytes, parse each font once per process.
//
// A file that cannot be read or described, and a font that cannot be
// parsed, are left out and reported (Err, and the Collection's Logger);
// a family none of whose fonts can be used is skipped as if it were not
// registered. Nothing here panics.
//
// The fields must be set before the source is added to a Collection and
// not modified afterwards, and a FontSource must not be copied after its
// first use (it holds the state of its Load); use it by pointer.
type FontSource struct {
	// Family is the name the faces are registered under, whatever the
	// files call themselves. It is required.
	Family string
	// Data are the font files (TrueType, OpenType, WOFF or collections),
	// one slice per file. The slices must not be modified afterwards: the
	// typefaces keep references into them.
	Data [][]byte
	// Load reads the files on first need when Data is nil (an app's files
	// on disk or in an fs.FS). It runs at most once per source; an error
	// drops the family. Sources whose Load returns the same slices share
	// their parse.
	Load func() ([][]byte, error)
	// Generics are the generic family names the family stands for
	// ("sans-serif", "system-ui", "ui-sans-serif", "monospace", "emoji"...):
	// AddSource appends the family to the list of each one (as
	// SetGeneric would), and a generic the collection maps wins over the
	// operating system's family for that name.
	Generics []string
	// Scripts are the scripts the family is a fallback for: AddSource
	// appends it to their lists (see SetScriptFallback).
	Scripts []language.Script

	once    sync.Once
	files   atomic.Pointer[fontFiles] // after once: the files of the source, nil when Load failed
	loadErr atomic.Pointer[error]     // the error of Load
}

// fontFiles are the files of a family and their state: their described
// fonts, each parsed when matching first selects it. The sources over the
// same files share them (filesFor).
type fontFiles struct {
	family string
	data   [][]byte

	once  sync.Once
	fonts []*sourceFont // after once: the described fonts of every file

	errMu sync.Mutex
	errs  []error
}

// sourceFont is one font of a family's file: described (enough to match
// it), and parsed when matching first selects it.
type sourceFont struct {
	files *fontFiles
	ld    *ot.Loader
	data  []byte
	file  int // index of the file in the family, for messages
	index int // index of the font in the file
	stub  *Typeface

	once   sync.Once
	tf     *Typeface // nil until parsed, or when parsing failed
	parsed atomic.Bool
}

// The files of the process, by family and identity of the slices, while
// some source uses them: an entry goes when its files are collected.
var filesReg struct {
	mu sync.Mutex
	m  map[filesKey]weak.Pointer[fontFiles]
}

// filesKey identifies files: the family and, per file, the address and
// length of its bytes.
type filesKey struct {
	family string
	ids    string
}

// filesEntry is what the cleanup of collected files needs to remove their
// entry (and not a newer one under the same key).
type filesEntry struct {
	key filesKey
	ptr weak.Pointer[fontFiles]
}

// filesFor returns the files of family made of data, shared with every
// source over the same slices: an address and a length name the same bytes
// while the slices are alive, and the files keep them alive.
func filesFor(family string, data [][]byte) *fontFiles {
	ids := make([]byte, 0, 16*len(data))
	for _, d := range data {
		ids = binary.LittleEndian.AppendUint64(ids, uint64(uintptr(unsafe.Pointer(unsafe.SliceData(d)))))
		ids = binary.LittleEndian.AppendUint64(ids, uint64(len(d)))
	}
	key := filesKey{family, string(ids)}
	filesReg.mu.Lock()
	defer filesReg.mu.Unlock()
	if f := filesReg.m[key].Value(); f != nil {
		return f
	}
	f := &fontFiles{family: family, data: slices.Clone(data)}
	ptr := weak.Make(f)
	if filesReg.m == nil {
		filesReg.m = map[filesKey]weak.Pointer[fontFiles]{}
	}
	filesReg.m[key] = ptr
	runtime.AddCleanup(f, forgetFiles, filesEntry{key, ptr})
	return f
}

// forgetFiles removes the entry of collected files.
func forgetFiles(e filesEntry) {
	filesReg.mu.Lock()
	defer filesReg.mu.Unlock()
	if filesReg.m[e.key] == e.ptr {
		delete(filesReg.m, e.key)
	}
}

// parseHook is the function SetParseHook set, or nil.
var parseHook atomic.Pointer[func(family string, data []byte, font int)]

// SetParseHook makes f run each time the files of a FontSource are
// described (font is -1, once per file) and each time one of their fonts
// is parsed (font is its index in the file), until restore is called:
// family is the source's family and data the file. It is for tests, which
// count this work: the files of a family are described and their fonts
// parsed once per process, however many sources, roles and collections use
// them. f may run on any goroutine, several at once; nil removes the hook.
// The tests that set it must not run in parallel.
func SetParseHook(f func(family string, data []byte, font int)) (restore func()) {
	var p *func(string, []byte, int)
	if f != nil {
		p = &f
	}
	old := parseHook.Swap(p)
	return func() { parseHook.Store(old) }
}

// hookParse tells the parse hook of work on a file.
func hookParse(family string, data []byte, font int) {
	if h := parseHook.Load(); h != nil {
		(*h)(family, data, font)
	}
}

// fontsOf returns the described fonts of the source's files, reading and
// describing them on first use; log receives what cannot be used.
func (s *FontSource) fontsOf(log Logger) []*sourceFont {
	s.once.Do(func() {
		files := s.Data
		if files == nil && s.Load != nil {
			var err error
			if files, err = s.Load(); err != nil {
				err = fmt.Errorf("text: font family %q: %w", s.Family, err)
				s.loadErr.Store(&err)
				report(log, err)
				return
			}
		}
		s.files.Store(filesFor(s.Family, files))
	})
	if f := s.files.Load(); f != nil {
		return f.fontsOf(log)
	}
	return nil
}

// fontsOf returns the described fonts of the files, describing them on
// first use.
func (ff *fontFiles) fontsOf(log Logger) []*sourceFont {
	ff.once.Do(func() { ff.fonts = ff.describe(log) })
	return ff.fonts
}

func (ff *fontFiles) describe(log Logger) []*sourceFont {
	var out []*sourceFont
	for i, data := range ff.data {
		hookParse(ff.family, data, -1)
		lds, err := ot.NewLoaders(bytes.NewReader(data))
		if err != nil {
			ff.fail(log, fmt.Errorf("text: font family %q, file %d: %w", ff.family, i, err))
			continue
		}
		for j, ld := range lds {
			stub, err := describeTypeface(newTableSource(ld, data, j), ff.family)
			if err != nil {
				ff.fail(log, fmt.Errorf("text: font family %q, file %d, font %d: %w", ff.family, i, j, err))
				continue
			}
			out = append(out, &sourceFont{files: ff, ld: ld, data: data, file: i, index: j, stub: stub})
		}
	}
	if len(out) == 0 && ff.err() == nil {
		ff.fail(log, fmt.Errorf("text: font family %q: no font file", ff.family))
	}
	return out
}

// typeface returns the parsed font, parsing it on first use, or nil when
// it cannot be parsed (reported once to log).
//
// perf: the first need of a font parses it on that goroutine, which may be
// laying text out under shapeMu (Layout.shape): other goroutines' shaping
// waits for the parse (tens of milliseconds for Noto Color Emoji). Warming
// the fonts an app declares off the UI thread would hide it.
func (f *sourceFont) typeface(log Logger) *Typeface {
	f.once.Do(func() {
		hookParse(f.files.family, f.data, f.index)
		tf, err := newTypeface(newTableSource(f.ld, f.data, f.index), f.files.family)
		if err != nil {
			f.files.fail(log, fmt.Errorf("text: font family %q, file %d, font %d: %w", f.files.family, f.file, f.index, err))
		} else {
			f.tf = tf
		}
		f.parsed.Store(true)
	})
	return f.tf
}

// fail records err and reports it to log.
func (ff *fontFiles) fail(log Logger, err error) {
	ff.errMu.Lock()
	ff.errs = append(ff.errs, err)
	ff.errMu.Unlock()
	report(log, err)
}

// report tells log of a font left out.
func report(log Logger, err error) {
	if log != nil {
		log.Printf("%v (the font is left out)", err)
	}
}

// err returns the errors of the files and fonts so far.
func (ff *fontFiles) err() error {
	ff.errMu.Lock()
	defer ff.errMu.Unlock()
	return errors.Join(ff.errs...)
}

// Err returns the errors of the files and fonts of the source that could
// not be read, described or parsed so far, or nil. A source is only read
// when text needs its family, so nil can also mean not read yet. The
// errors of files are those of every source over them.
func (s *FontSource) Err() error {
	if err := s.loadErr.Load(); err != nil {
		return *err
	}
	if f := s.files.Load(); f != nil {
		return f.err()
	}
	return nil
}

// Typefaces reads and parses every font of the source now (a family is
// otherwise parsed font by font, as matching selects them) and returns the
// ones that could be parsed, in file order, with the errors of the others.
func (s *FontSource) Typefaces() ([]*Typeface, error) {
	var out []*Typeface
	for _, f := range s.fontsOf(nil) {
		if tf := f.typeface(nil); tf != nil {
			out = append(out, tf)
		}
	}
	return out, s.Err()
}

// parsedFonts counts the fonts of the source's files parsed so far (for
// tests).
func (s *FontSource) parsedFonts() int {
	ff := s.files.Load()
	if ff == nil {
		return 0
	}
	n := 0
	for _, f := range ff.fontsOf(nil) {
		if f.parsed.Load() && f.tf != nil {
			n++
		}
	}
	return n
}

// matchSources returns the typeface CSS font matching selects for q among
// a family's parsed typefaces and the fonts of its sources, parsing the
// selected source font on first use. A font that cannot be parsed is left
// out and matching runs again over the others; nil means none is usable.
func matchSources(faces []*Typeface, srcs []*FontSource, q Query, log Logger) *Typeface {
	if len(srcs) == 0 {
		return matchFace(faces, q)
	}
	set := append([]*Typeface(nil), faces...)
	owners := make([]*sourceFont, len(set), len(set)+4)
	var seen []*fontFiles
	for _, s := range srcs {
		fonts := s.fontsOf(log)
		if ff := s.files.Load(); ff != nil {
			if slices.Contains(seen, ff) {
				continue // another source over the same files: another role of the family
			}
			seen = append(seen, ff)
		}
		for _, f := range fonts {
			if f.parsed.Load() {
				if f.tf == nil {
					continue
				}
				set = append(set, f.tf) // the parsed typeface: same matching properties as the stub
			} else {
				set = append(set, f.stub)
			}
			owners = append(owners, f)
		}
	}
	for len(set) > 0 {
		best := matchFace(set, q)
		i := 0
		for set[i] != best {
			i++
		}
		f := owners[i]
		if f == nil {
			return best // a parsed typeface of the family
		}
		if tf := f.typeface(log); tf != nil {
			return tf
		}
		set = append(set[:i], set[i+1:]...)
		owners = append(owners[:i], owners[i+1:]...)
	}
	return nil
}

// describeTypeface reads what font matching needs of the font that src
// reads (family, name, style, weight, stretch and variation axes) from its
// small tables, without go-text's parse of the font (font.NewFont, which is
// most of the cost of a typeface). The values are the ones newTypeface
// sets; the stub has nothing else and is never drawn.
func describeTypeface(src tableSource, family string) (*Typeface, error) {
	ld := src.ld
	raw := func(tag string) []byte { return src.table(ot.MustNewTag(tag)) }
	tf := &Typeface{}
	var err error
	tf.head, _, err = font.LoadHeadTable(ld, nil)
	if err != nil {
		return nil, err
	}
	if os2, _, err := tables.ParseOs2(raw("OS/2")); err == nil {
		tf.os2, tf.hasOS2 = os2, true
	}
	if fvar, _, err := tables.ParseFvar(raw("fvar")); err == nil {
		for _, a := range fvar.FvarRecords.Axis {
			tf.axes = append(tf.axes, Axis{Tag: a.Tag, Min: a.Minimum, Default: a.Default, Max: a.Maximum})
		}
	}
	names, _, _ := tables.ParseName(raw("name"))
	desc, _ := font.Describe(ld, nil)
	tf.describe(family, names, desc)
	return tf, nil
}
