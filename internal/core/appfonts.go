package core

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"

	gotextlang "github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/text"
	xlang "golang.org/x/text/language"
)

// Font is a font family an app brings (tela.Font, ADR-244): font files in
// memory (FontData) or in a file system (FontFS), the generic families it
// stands for and the scripts it is a fallback for. Fonts are values; the
// methods that change one return a modified copy, and the copies share the
// files and, per (generics, scripts), the text.FontSource that parses them,
// so a family is read and parsed once per process however many apps,
// collections and copies use it.
//
// The zero Font is invalid: Check reports it.
type Font struct {
	b        *fontBase
	generics []string
	scripts  []string // ISO 15924 codes, as xlang.Script.String
}

// fontBase is what the copies of a Font share.
type fontBase struct {
	family string // "" until known: taken from the first file
	data   [][]byte
	fsys   fs.FS
	pats   []string
	err    error // a construction error, reported by Check

	filesOnce sync.Once // reads the files of an FS font, once
	files     [][]byte
	filesErr  error

	famOnce sync.Once // resolves an empty family from the first file
	famErr  error

	mu      sync.Mutex
	sources map[string]*text.FontSource
}

// FontData returns the font of files already in memory.
func FontData(family string, files ...[]byte) Font {
	b := &fontBase{family: strings.TrimSpace(family), data: files}
	switch {
	case len(files) == 0:
		b.err = errors.New("tela.FontData: give at least one font file, such as tela.FontData(\"Lora\", loraTTF, loraItalicTTF) with the files embedded using //go:embed")
	default:
		for i, f := range files {
			if len(f) == 0 {
				b.err = fmt.Errorf("tela.FontData: file %d is empty; embed the font with //go:embed into a []byte variable", i)
				break
			}
		}
	}
	return Font{b: b}
}

// FontFS returns the font of the files of fsys that patterns (fs.Glob
// syntax) match, read when text first needs the family.
func FontFS(fsys fs.FS, family string, patterns ...string) Font {
	b := &fontBase{family: strings.TrimSpace(family), fsys: fsys, pats: slices.Clone(patterns)}
	switch {
	case fsys == nil:
		b.err = errors.New("tela.FontFS: the file system is nil; pass an embed.FS, os.DirFS(dir) or another fs.FS")
	case len(patterns) == 0:
		b.err = errors.New("tela.FontFS: give at least one pattern, such as tela.FontFS(fonts, \"Lora\", \"fonts/Lora*.ttf\")")
	}
	return Font{b: b}
}

// WithGenerics returns f also standing for the generic family names.
func (f Font) WithGenerics(names ...string) Font {
	f.generics = append(slices.Clone(f.generics), names...)
	return f
}

// scriptExamples are the codes the message of a bad script code suggests.
const scriptExamples = `"Arab", "Hebr", "Deva", "Thai", "Hani", "Cyrl"`

// WithScripts returns f also a fallback for the scripts, ISO 15924 codes.
// A code that is not one is a programmer error: it panics in dev mode and
// is ignored otherwise.
func (f Font) WithScripts(codes ...string) Font {
	out := slices.Clone(f.scripts)
	for _, c := range codes {
		s, err := xlang.ParseScript(c)
		if err != nil || len(c) != 4 {
			if devNow() {
				panic(fmt.Sprintf("tela: Font.Scripts(%q): not an ISO 15924 script code; give four-letter codes such as %s", c, scriptExamples))
			}
			continue
		}
		if code := s.String(); !slices.Contains(out, code) {
			out = append(out, code)
		}
	}
	f.scripts = out
	return f
}

// Family returns the family name: the one given, else the one the first
// file declares ("" while that cannot be read).
func (f Font) Family() string {
	if f.b == nil {
		return ""
	}
	_ = f.b.resolveFamily()
	return f.b.family
}

// Check reports why the font cannot be used, without reading the files of
// an FS font: a construction mistake, a pattern that matches no file, or
// (with no family given) a first file that declares none. Run calls it
// before it opens the window.
func (f Font) Check() error {
	b := f.b
	switch {
	case b == nil:
		return errors.New("tela: the zero tela.Font is not a font; make one with tela.FontData, tela.FontFS or a package of tela/fonts")
	case b.err != nil:
		return b.err
	}
	if b.fsys != nil {
		for _, p := range b.pats {
			m, err := fs.Glob(b.fsys, p)
			if err != nil {
				return fmt.Errorf("tela.FontFS: pattern %q: %v", p, err)
			}
			if len(m) == 0 {
				return fmt.Errorf("tela.FontFS: pattern %q matches no file; patterns are fs.Glob patterns relative to the root of the file system, such as \"fonts/*.ttf\" for an embed.FS holding a fonts directory", p)
			}
		}
	}
	return b.resolveFamily()
}

// Verify reads and parses every font of f now (a family is otherwise
// parsed when text needs it) and reports what cannot be used. Tests use
// it so that a broken font fails them instead of drawing fallback glyphs.
func (f Font) Verify() error {
	if err := f.Check(); err != nil {
		return err
	}
	src, err := f.Source()
	if err != nil {
		return err
	}
	_, err = src.Typefaces()
	return err
}

// resolveFamily sets an empty family from the first file (reading it,
// for an FS font) and returns the error of doing so.
func (b *fontBase) resolveFamily() error {
	if b.family != "" || b.err != nil {
		return b.err
	}
	b.famOnce.Do(func() {
		files, err := b.read()
		if err != nil {
			b.famErr = err
			return
		}
		name, err := text.FamilyName(files[0])
		if err != nil {
			b.famErr = fmt.Errorf("tela: the family of the font could not be taken from its file (%v); give it: tela.FontData(\"Lora\", ...)", err)
			return
		}
		b.family = name
	})
	return b.famErr
}

// read returns the font files, reading an FS font's once.
func (b *fontBase) read() ([][]byte, error) {
	if b.fsys == nil {
		return b.data, nil
	}
	b.filesOnce.Do(func() {
		seen := map[string]bool{}
		for _, p := range b.pats {
			m, err := fs.Glob(b.fsys, p)
			if err != nil {
				b.filesErr = fmt.Errorf("tela.FontFS: pattern %q: %v", p, err)
				return
			}
			if len(m) == 0 {
				b.filesErr = fmt.Errorf("tela.FontFS: pattern %q matches no file", p)
				return
			}
			for _, name := range m {
				if seen[name] {
					continue
				}
				seen[name] = true
				data, err := fs.ReadFile(b.fsys, name)
				if err != nil {
					b.filesErr = fmt.Errorf("tela.FontFS: %v", err)
					return
				}
				b.files = append(b.files, data)
			}
		}
	})
	return b.files, b.filesErr
}

// Source returns the text.FontSource of f, shared by every copy of f with
// the same generics and scripts. The family is resolved now, which reads
// the first file of an FS font that was given no family.
func (f Font) Source() (*text.FontSource, error) {
	if err := f.Check(); err != nil {
		return nil, err
	}
	b := f.b
	var scripts []gotextlang.Script
	for _, c := range f.scripts {
		if s, err := gotextlang.ParseScript(c); err == nil {
			scripts = append(scripts, s)
		}
	}
	key := strings.Join(f.generics, ",") + "|" + strings.Join(f.scripts, ",")
	b.mu.Lock()
	defer b.mu.Unlock()
	if s := b.sources[key]; s != nil {
		return s, nil
	}
	s := &text.FontSource{Family: b.family, Generics: slices.Clone(f.generics), Scripts: scripts}
	if b.fsys == nil {
		s.Data = b.data
	} else {
		s.Load = b.read
	}
	if b.sources == nil {
		b.sources = map[string]*text.FontSource{}
	}
	b.sources[key] = s
	return s, nil
}

// CheckFonts reports the first font of fonts that cannot be used (Check).
func CheckFonts(fonts []Font) error {
	for _, f := range fonts {
		if err := f.Check(); err != nil {
			return err
		}
	}
	return nil
}

// NewFontCollection returns the collection of an app's fonts: fonts first,
// in order, so they win the generic families they stand for and are tried
// first in fallback, then after (such as the deterministic set of tests),
// then, when system is not nil, the installed fonts. The error is why the
// system fonts are off (the collection works without them). Call
// CheckFonts first: a font that cannot be used is left out here.
func NewFontCollection(fonts []Font, after []*text.FontSource, system *text.SystemFontOptions) (*text.Collection, error) {
	c := text.NewCollection()
	for _, f := range fonts {
		if src, err := f.Source(); err == nil {
			_ = c.AddSource(src) // has a family and files
		}
	}
	for _, src := range after {
		_ = c.AddSource(src)
	}
	if system == nil {
		return c, nil
	}
	return c, c.EnableSystemFonts(*system)
}

// runAppFonts returns the fonts of Run with Options.AppFonts: the app's
// fonts and then the installed ones, reporting like runFonts. Under go
// test the system fonts are off (ADR-035) and only the app's remain.
func runAppFonts(fonts []Font, w io.Writer) *text.Collection {
	c, err := NewFontCollection(fonts, nil, &text.SystemFontOptions{})
	reportFonts(c, err, w)
	// As in runFonts: the system index loads while the window opens.
	go c.Preload()
	return c
}
