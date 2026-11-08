package text

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

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
// A source is parsed in two steps, each at most once for the life of the
// source, whichever goroutine and Collection need it first (the others
// wait): the first need of the family reads the files (Load) and describes
// their fonts from their small tables (name, OS/2, head, fvar) without
// parsing them; font matching (CSS Fonts 4, as for any family) then picks
// the font of the request among them, and only that font is parsed. So a
// family of an upright and an italic file that draws only upright text
// never parses the italic one. The typefaces are immutable and shared by
// every Collection the source is added to, as are their faces.
//
// A file that cannot be read or described, and a font that cannot be
// parsed, are left out and reported (Err, and the Collection's Logger);
// a family none of whose fonts can be used is skipped as if it were not
// registered. Nothing here panics.
//
// The fields must be set before the source is added to a Collection and
// not modified afterwards, and a FontSource must not be copied after its
// first use (it holds the parse state); use it by pointer.
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
	// drops the family.
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

	once  sync.Once
	fonts []*sourceFont // after once: the described fonts of every file

	errMu sync.Mutex
	errs  []error
}

// sourceFont is one font of a source's file: described (enough to match
// it), and parsed when matching first selects it.
type sourceFont struct {
	src   *FontSource
	ld    *ot.Loader
	data  []byte
	file  int // index of the file in the source, for messages
	index int // index of the font in the file
	stub  *Typeface

	once   sync.Once
	tf     *Typeface // nil until parsed, or when parsing failed
	parsed atomic.Bool
}

// fontsOf returns the described fonts of the source's files, reading and
// describing them on first use; log receives what cannot be used.
func (s *FontSource) fontsOf(log Logger) []*sourceFont {
	s.once.Do(func() { s.fonts = s.describeFiles(log) })
	return s.fonts
}

func (s *FontSource) describeFiles(log Logger) []*sourceFont {
	files := s.Data
	if files == nil && s.Load != nil {
		var err error
		files, err = s.Load()
		if err != nil {
			s.fail(log, fmt.Errorf("text: font family %q: %w", s.Family, err))
			return nil
		}
	}
	var out []*sourceFont
	for i, data := range files {
		lds, err := ot.NewLoaders(bytes.NewReader(data))
		if err != nil {
			s.fail(log, fmt.Errorf("text: font family %q, file %d: %w", s.Family, i, err))
			continue
		}
		for j, ld := range lds {
			stub, err := describeTypeface(newTableSource(ld, data, j), s.Family)
			if err != nil {
				s.fail(log, fmt.Errorf("text: font family %q, file %d, font %d: %w", s.Family, i, j, err))
				continue
			}
			out = append(out, &sourceFont{src: s, ld: ld, data: data, file: i, index: j, stub: stub})
		}
	}
	if len(out) == 0 && s.Err() == nil {
		s.fail(log, fmt.Errorf("text: font family %q: no font file", s.Family))
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
		tf, err := newTypeface(newTableSource(f.ld, f.data, f.index), f.src.Family)
		if err != nil {
			f.src.fail(log, fmt.Errorf("text: font family %q, file %d, font %d: %w", f.src.Family, f.file, f.index, err))
		} else {
			f.tf = tf
		}
		f.parsed.Store(true)
	})
	return f.tf
}

// fail records err and reports it to log.
func (s *FontSource) fail(log Logger, err error) {
	s.errMu.Lock()
	s.errs = append(s.errs, err)
	s.errMu.Unlock()
	if log != nil {
		log.Printf("%v (the font is left out)", err)
	}
}

// Err returns the errors of the files and fonts of the source that could
// not be read, described or parsed so far, or nil. A source is only read
// when text needs its family, so nil can also mean not read yet.
func (s *FontSource) Err() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return errors.Join(s.errs...)
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

// parsedFonts counts the fonts of the source parsed so far (for tests).
func (s *FontSource) parsedFonts() int {
	n := 0
	for _, f := range s.fonts {
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
	for _, s := range srcs {
		for _, f := range s.fontsOf(log) {
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
