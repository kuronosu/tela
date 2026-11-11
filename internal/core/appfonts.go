package core

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/text"
)

// Font is a font family an app brings (tela.Font, ADR-244): font files in
// memory (FontData) or in a file system (FontFS), the generic families it
// stands for and the scripts it is a fallback for. Fonts are values; the
// methods that change one return a modified copy. The copies share the
// files and, per (generics, scripts), the text.FontSource that registers
// them; the sources of every role share one read and one parse (the
// sources over the same slices do, text.FontSource), so a family is read
// and parsed once per process however many apps, collections, copies and
// roles use it, and so are the files of a fonts/ package that
// internal/stdfonts registers too (both are over the same embedded bytes).
//
// The zero Font is invalid: Check reports it.
type Font struct {
	b        *fontBase
	generics []string
	scripts  []language.Script // without duplicates, in order
}

// fontBase is what the copies of a Font share.
type fontBase struct {
	given string // the family given; "" takes it from the first file
	data  [][]byte
	fsys  fs.FS
	pats  []string
	err   error // a construction error, reported by Check

	filesOnce sync.Once // reads the files of an FS font, once
	files     [][]byte  // the same slices every time, so the sources of every role share their parse
	filesErr  error

	famOnce sync.Once // resolves an empty family from the first file
	family  string    // after famOnce
	famErr  error

	mu      sync.Mutex
	sources map[string]*text.FontSource
}

// FontData returns the font of files already in memory.
func FontData(family string, files ...[]byte) Font {
	b := &fontBase{given: strings.TrimSpace(family), data: files}
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
	b := &fontBase{given: strings.TrimSpace(family), fsys: fsys, pats: slices.Clone(patterns)}
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

// scriptExamples are the codes the message of a bad script code suggests,
// and scriptUmbrellaHelp what it says of the umbrella codes.
const (
	scriptExamples     = `"Arab", "Hebr", "Deva", "Thai", "Hani", "Cyrl"`
	scriptUmbrellaHelp = `"Hans" and "Hant" stand for "Hani", "Jpan" for "Hani", "Hira" and "Kana", "Kore" for "Hang" and "Hani", "Hrkt" for "Hira" and "Kana"`
)

// scriptUmbrellas are the ISO 15924 codes of writing systems made of
// several scripts, mapped as CLDR and browsers do to the scripts their
// text is itemized into (text: UAX #24, where Chinese is Hani, kana Hira
// and Kana, Hangul Hang).
var scriptUmbrellas = map[string][]language.Script{
	"Hans": {language.Han},
	"Hant": {language.Han},
	"Jpan": {language.Han, language.Hiragana, language.Katakana},
	"Kore": {language.Hangul, language.Han},
	"Hrkt": {language.Hiragana, language.Katakana},
}

// scriptHints say why the ISO 15924 codes that are not scripts of text
// cannot be a fallback.
var scriptHints = map[string]string{
	"Zyyy": "Zyyy is Common: digits, punctuation and symbols take the script of the text around them",
	"Zinh": "Zinh is Inherited: combining marks take the script of the character before them",
	"Zzzz": "Zzzz is the code of unknown scripts",
	"Zxxx": "Zxxx is the code of unwritten languages",
	"Zmth": "math symbols are Common characters, which take the script of the text around them",
	"Zsym": "symbols are Common characters, which take the script of the text around them",
	"Zsye": "emoji use the emoji family: make the font Emoji()",
}

// textScripts are the scripts text runs can be in: those of Unicode's
// characters (go-text's table, which itemization reads), without Common
// and Inherited, whose characters take the script of the text around them.
var textScripts = sync.OnceValue(func() map[language.Script]bool {
	m := map[language.Script]bool{}
	for _, r := range language.ScriptRanges {
		if r.Script != language.Common && r.Script != language.Inherited {
			m[r.Script] = true
		}
	}
	return m
})

// parseScripts returns the scripts of an ISO 15924 code (any case): the
// script itself, or the scripts of an umbrella code; why says what is
// wrong with a code that is not one of a script of text.
func parseScripts(code string) (scripts []language.Script, why string) {
	if len(code) != 4 {
		return nil, ""
	}
	for i := range 4 {
		if c := code[i] | 0x20; c < 'a' || c > 'z' {
			return nil, ""
		}
	}
	code = strings.ToUpper(code[:1]) + strings.ToLower(code[1:])
	if l, ok := scriptUmbrellas[code]; ok {
		return l, ""
	}
	s, err := language.ParseScript(code)
	if err != nil || !textScripts()[s] {
		return nil, scriptHints[code]
	}
	return []language.Script{s}, ""
}

// WithScripts returns f also a fallback for the scripts, ISO 15924 codes
// of scripts of text, or umbrella codes, which stand for the scripts their
// text is made of (Hans and Hant: Hani; Jpan: Hani, Hira and Kana; Kore:
// Hang and Hani; Hrkt: Hira and Kana). Another code is a programmer error:
// it panics in dev mode and is ignored otherwise.
func (f Font) WithScripts(codes ...string) Font {
	out := slices.Clone(f.scripts)
	for _, c := range codes {
		l, why := parseScripts(c)
		if l == nil {
			if devNow() {
				if why != "" {
					why = " (" + why + ")"
				}
				panic(fmt.Sprintf("tela: Font.Scripts(%q): not the ISO 15924 code of a script of text%s; give four-letter codes such as %s, or an umbrella code: %s", c, why, scriptExamples, scriptUmbrellaHelp))
			}
			continue
		}
		for _, s := range l {
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
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
	name, _ := f.b.resolveFamily()
	return name
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
	_, err := b.resolveFamily()
	return err
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

// resolveFamily returns the family: the one given, else the one of the
// first file (reading it, for an FS font, once), with the error of the
// font or of taking it from the file.
func (b *fontBase) resolveFamily() (string, error) {
	if b.given != "" || b.err != nil {
		return b.given, b.err
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
	return b.family, b.famErr
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
// the same generics and scripts; the sources of the other roles share its
// files' read and parse. The family is resolved now, which reads the first
// file of an FS font that was given no family.
func (f Font) Source() (*text.FontSource, error) {
	if err := f.Check(); err != nil {
		return nil, err
	}
	b := f.b
	family, _ := b.resolveFamily() // no error: Check resolved it
	var key strings.Builder
	key.WriteString(strings.Join(f.generics, ","))
	key.WriteByte('|')
	for _, s := range f.scripts {
		key.WriteString(s.String())
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if s := b.sources[key.String()]; s != nil {
		return s, nil
	}
	s := &text.FontSource{Family: family, Generics: slices.Clone(f.generics), Scripts: slices.Clone(f.scripts)}
	if b.fsys == nil {
		s.Data = b.data
	} else {
		s.Load = b.read
	}
	if b.sources == nil {
		b.sources = map[string]*text.FontSource{}
	}
	b.sources[key.String()] = s
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

// runCollection returns the fonts of Run: Config.Fonts when set (the app's
// fonts are then ignored, unchecked), else the app's fonts (after
// CheckFonts) and the installed ones, else the installed ones or the
// default set (runFonts).
func runCollection(opts Options) (*text.Collection, error) {
	if err := CheckHyphenation(opts.AppHyphenation); err != nil {
		return nil, err
	}
	var c *text.Collection
	switch {
	case opts.Fonts != nil:
		c = opts.Fonts
	case len(opts.AppFonts) > 0:
		if err := CheckFonts(opts.AppFonts); err != nil {
			return nil, err
		}
		c = runAppFonts(opts.AppFonts, errorLog(opts.ErrorLog))
	default:
		c = runFonts(errorLog(opts.ErrorLog)) // fonts.go
	}
	if len(opts.AppHyphenation) > 0 {
		// The app's dictionaries (ADR-270); one app per process, so the
		// shared collection of the system fonts takes them too.
		c.SetHyphenators(Hyphenators(opts.AppHyphenation))
	}
	return c, nil
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
