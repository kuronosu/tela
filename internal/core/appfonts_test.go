package core

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"unsafe"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/text"
)

var sansNames = []string{"ui-sans-serif", "system-ui", "sans-serif"}

const testFontDir = "../text/testdata/fonts"

func readTestFont(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(testFontDir + "/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func familyOf(c *text.Collection, generic string) string {
	f := c.Face(text.Query{Families: []string{generic}}, 16)
	if f == nil {
		return ""
	}
	return f.Typeface().Family()
}

func TestFontFamilyFromFile(t *testing.T) {
	f := FontData("", readTestFont(t, "TelaSbixTest.ttf"))
	if err := f.Check(); err != nil {
		t.Fatal(err)
	}
	if got := f.Family(); got != "Tela Sbix Test" {
		t.Errorf("Family() = %q, want Tela Sbix Test", got)
	}
	named := FontData("Mine", readTestFont(t, "TelaSbixTest.ttf"))
	if got := named.Family(); got != "Mine" {
		t.Errorf("Family() = %q, want Mine", got)
	}
	// An FS font with no family reads its first file.
	fsys := os.DirFS(testFontDir)
	if got := FontFS(fsys, "", "TelaSbix*.ttf").Family(); got != "Tela Sbix Test" {
		t.Errorf("FS Family() = %q, want Tela Sbix Test", got)
	}
}

func TestFontCheck(t *testing.T) {
	fsys := fstest.MapFS{"fonts/a.ttf": {Data: readTestFont(t, "TelaSbixTest.ttf")}}
	for _, tc := range []struct {
		name string
		f    Font
		want string // substring of the error, "" for none
	}{
		{"zero", Font{}, "zero tela.Font"},
		{"no files", FontData("A"), "at least one font file"},
		{"empty file", FontData("A", nil), "empty"},
		{"nil fs", FontFS(nil, "A", "*.ttf"), "file system is nil"},
		{"no pattern", FontFS(fsys, "A"), "at least one pattern"},
		{"no match", FontFS(fsys, "A", "fonts/*.otf"), `pattern "fonts/*.otf" matches no file`},
		{"bad pattern", FontFS(fsys, "A", "fonts/[a"), `pattern "fonts/[a"`},
		{"not a font", FontData("", []byte("not a font at all")), "family"},
		{"ok", FontFS(fsys, "A", "fonts/*.ttf"), ""},
		{"ok data", FontData("A", readTestFont(t, "TelaSbixTest.ttf")), ""},
	} {
		err := tc.f.Check()
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: error %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
	if err := CheckFonts([]Font{FontData("A", readTestFont(t, "TelaSbixTest.ttf")), FontFS(fsys, "B", "x/*.ttf")}); err == nil || !strings.Contains(err.Error(), `"x/*.ttf"`) {
		t.Errorf("CheckFonts: %v", err)
	}
	// A file that is not a font passes Check (the family is given) and
	// fails Verify, as the collection would log it.
	if err := FontData("Bad", []byte("not a font at all")).Verify(); err == nil {
		t.Error("Verify accepted a file that is not a font")
	}
}

func TestFontScripts(t *testing.T) {
	f := FontData("A", []byte("x")).WithScripts("arab", "Hebr", "Arab", "LATN")
	if got := scriptCodes(f); got != "Arab,Hebr,Latn" {
		t.Errorf("scripts = %q, want Arab,Hebr,Latn (normalized, no duplicates)", got)
	}
	// The original is not modified.
	if len(FontData("A", []byte("x")).scripts) != 0 {
		t.Error("a new font has scripts")
	}
}

func scriptCodes(f Font) string {
	var codes []string
	for _, s := range f.scripts {
		codes = append(codes, s.String())
	}
	return strings.Join(codes, ",")
}

// TestFontScriptsUmbrellas: the umbrella codes stand for the scripts their
// text is itemized into, as in CLDR and browsers.
func TestFontScriptsUmbrellas(t *testing.T) {
	for code, want := range map[string]string{
		"Hans": "Hani", "Hant": "Hani", "hant": "Hani",
		"Jpan": "Hani,Hira,Kana",
		"Kore": "Hang,Hani",
		"Hrkt": "Hira,Kana",
	} {
		if got := scriptCodes(FontData("A", []byte("x")).WithScripts(code)); got != want {
			t.Errorf("Scripts(%q) = %s, want %s", code, got, want)
		}
	}
	if got := scriptCodes(FontData("A", []byte("x")).WithScripts("Jpan", "Kore", "Hani")); got != "Hani,Hira,Kana,Hang" {
		t.Errorf("Scripts(Jpan, Kore, Hani) = %s", got)
	}

	// End to end: Chinese and kana text uses the font of Jpan (Kore,
	// Hans) before the families registered before it, which also have
	// the characters.
	cjk := readTestFont(t, "NotoSansJP-Subset.otf")
	for _, code := range []string{"Jpan", "Hans", "Hant", "Kore", "Latn"} {
		c, _ := NewFontCollection([]Font{FontData("First", cjk), FontData("Umbrella", cjk).WithScripts(code)}, nil, nil)
		sample := "日本語"
		if code == "Jpan" {
			sample = "日本語かなカナ"
		}
		want := "Umbrella"
		if code == "Latn" {
			want = "First" // the control: a font for another script is not used
		}
		rs := []rune(sample)
		runs := c.Itemize(rs, 0, len(rs), text.Query{Families: []string{"unknown"}}, 16, language.Latin, nil)
		if len(runs) != 1 || runs[0].Face.Typeface().Family() != want {
			var got []string
			for _, r := range runs {
				got = append(got, string(rs[r.Start:r.End])+"="+r.Face.Typeface().Family())
			}
			t.Errorf("Scripts(%q): runs %v, want one run of %s", code, got, want)
		}
	}
}

// TestFontScriptsRejects: a code that is not one of a script of text
// panics in dev mode with the examples and the umbrella mapping, and is
// ignored otherwise.
func TestFontScriptsRejects(t *testing.T) {
	panics := func(code string) (msg string) {
		defer func() { msg, _ = recover().(string) }()
		FontData("A", []byte("x")).WithScripts(code)
		return ""
	}
	for bad, hint := range map[string]string{
		"Arabic": "", "Arbc": "", "": "", "Ar": "", "Ar1b": "", "Ärab": "",
		"Zyyy": "Common", "zinh": "Inherited", "Zzzz": "unknown", "Zxxx": "unwritten",
		"Zmth": "Common", "Zsym": "Common", "Zsye": "Emoji()",
	} {
		m := panics(bad)
		for _, want := range []string{fmt.Sprintf("Font.Scripts(%q)", bad), "ISO 15924", `"Arab"`, `"Jpan" for "Hani", "Hira" and "Kana"`, `"Kore" for "Hang" and "Hani"`, hint} {
			if !strings.Contains(m, want) {
				t.Errorf("WithScripts(%q) panic = %q, want it to contain %q", bad, m, want)
			}
		}
	}
	defer devAny.Store(devAny.Load())
	devAny.Store(false)
	if got := scriptCodes(FontData("A", []byte("x")).WithScripts("Zyyy", "Arab", "Nope")); got != "Arab" {
		t.Errorf("without dev mode: scripts %s, want the valid one", got)
	}
}

func TestFontCopiesShareTheSource(t *testing.T) {
	base := FontData("Mine", readTestFont(t, "TelaSbixTest.ttf"))
	a, err := base.WithGenerics(sansNames...).Source()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := base.WithGenerics(sansNames...).Source()
	c, _ := base.WithGenerics("ui-monospace", "monospace").Source()
	if a != b {
		t.Error("two copies with the same roles have different sources")
	}
	if a == c {
		t.Error("copies with other roles share a source")
	}
	if got := strings.Join(a.Generics, ","); got != "ui-sans-serif,system-ui,sans-serif" {
		t.Errorf("Sans generics = %q", got)
	}
	if base.generics != nil {
		t.Error("Sans modified the receiver")
	}
	arab, _ := base.WithScripts("Arab").Source()
	if len(arab.Scripts) != 1 || arab.Scripts[0] != language.Arabic {
		t.Errorf("scripts = %v", arab.Scripts)
	}
}

// parseCounts counts, with text's parse hook, the descriptions and parses
// of files (by the identity of their bytes).
type parseCounts struct {
	mu     sync.Mutex
	files  [][]byte
	counts map[[2]int]int // file, font (-1: described)
}

func countParses(t *testing.T, files [][]byte) *parseCounts {
	pc := &parseCounts{files: files, counts: map[[2]int]int{}}
	t.Cleanup(text.SetParseHook(func(_ string, data []byte, font int) {
		pc.mu.Lock()
		defer pc.mu.Unlock()
		for i, f := range pc.files {
			if len(f) == len(data) && unsafe.SliceData(f) == unsafe.SliceData(data) {
				pc.counts[[2]int{i, font}]++
			}
		}
	}))
	return pc
}

// once fails t unless each file was described once and each of its fonts
// parsed at most once, n fonts in all.
func (pc *parseCounts) once(t *testing.T, n int) {
	t.Helper()
	pc.mu.Lock()
	defer pc.mu.Unlock()
	parsed := 0
	for k, c := range pc.counts {
		if c != 1 {
			t.Errorf("file %d, font %d (-1: the description): %d times, want once", k[0], k[1], c)
		}
		if k[1] >= 0 {
			parsed++
		}
	}
	for i := range pc.files {
		if pc.counts[[2]int{i, -1}] == 0 {
			t.Errorf("file %d not described", i)
		}
	}
	if parsed != n {
		t.Errorf("%d fonts parsed, want %d", parsed, n)
	}
}

// TestFontRolesShareTheParse: the copies of a Font with different roles
// have their own sources, which read, describe and parse the files once
// and give the same typefaces (ADR-244: copies of a Font share one parse).
func TestFontRolesShareTheParse(t *testing.T) {
	data := readTestFont(t, "TelaSbixTest.ttf") // new bytes: parsed by no other test
	check := func(t *testing.T, base Font, files [][]byte, reads func() int) {
		pc := countParses(t, files)
		var first []*text.Typeface
		for _, f := range []Font{base, base.WithGenerics("ui-serif", "serif"), base.WithScripts("Latn"), base.WithGenerics(sansNames...).WithScripts("Arab")} {
			src, err := f.Source()
			if err != nil {
				t.Fatal(err)
			}
			tfs, err := src.Typefaces()
			if err != nil || len(tfs) == 0 {
				t.Fatalf("Typefaces: %d, %v", len(tfs), err)
			}
			if first == nil {
				first = tfs
			} else if !slices.Equal(tfs, first) {
				t.Errorf("the roles %v %v have other typefaces: the files were parsed again", f.generics, f.scripts)
			}
		}
		// Two roles in one collection: the family resolves through either.
		c, _ := NewFontCollection([]Font{base.WithGenerics("ui-serif", "serif"), base.WithScripts("Latn")}, nil, nil)
		if got := familyOf(c, "serif"); got != base.Family() {
			t.Errorf("serif = %q", got)
		}
		pc.once(t, len(first))
		if reads != nil && reads() != 1 {
			t.Errorf("the files were read %d times, want once", reads())
		}
	}
	t.Run("data", func(t *testing.T) {
		check(t, FontData("Roles", data), [][]byte{data}, nil)
	})
	t.Run("fs", func(t *testing.T) {
		reads := 0
		fsys := &fontCountFS{MapFS: fstest.MapFS{"f/a.ttf": {Data: data}}, reads: &reads}
		base := FontFS(fsys, "", "f/*.ttf") // the family comes from the file
		files, err := base.b.read()
		if err != nil {
			t.Fatal(err)
		}
		check(t, base, files, func() int { return reads })
	})
}

// TestFontFamilyConcurrent: goroutines that resolve the family of one Font
// given none (in parallel tests of an app, say) do not race (go test
// -race).
func TestFontFamilyConcurrent(t *testing.T) {
	data := readTestFont(t, "TelaSbixTest.ttf")
	for _, f := range []Font{
		FontFS(fstest.MapFS{"a.ttf": {Data: data}}, "", "*.ttf"),
		FontData("", data),
	} {
		var wg sync.WaitGroup
		names := make([]string, 8)
		for i := range names {
			wg.Add(1)
			go func() {
				defer wg.Done()
				switch i % 3 {
				case 0:
					names[i] = f.Family()
				case 1:
					if err := f.Check(); err != nil {
						t.Error(err)
					}
					names[i] = f.Family()
				default:
					src, err := f.WithGenerics("serif").Source()
					if err != nil {
						t.Error(err)
						return
					}
					names[i] = src.Family
				}
			}()
		}
		wg.Wait()
		for i, n := range names {
			if n != "Tela Sbix Test" {
				t.Errorf("goroutine %d: family %q", i, n)
			}
		}
	}
}

// TestRunFontsIgnoreAppFontsWithConfigFonts: Run checks the app's fonts
// only when it uses them, not when Config.Fonts is set.
func TestRunFontsIgnoreAppFontsWithConfigFonts(t *testing.T) {
	given := text.NewCollection()
	c, err := runCollection(Options{Config: Config{Fonts: given}, AppFonts: []Font{{}}})
	if err != nil || c != given {
		t.Errorf("with Config.Fonts: %v, %v; want the given collection and no error", c, err)
	}
	if _, err := runCollection(Options{AppFonts: []Font{{}}}); err == nil || !strings.Contains(err.Error(), "zero tela.Font") {
		t.Errorf("without Config.Fonts: error %v", err)
	}
}

func TestFontFSIsReadLazily(t *testing.T) {
	reads := 0
	fsys := &fontCountFS{MapFS: fstest.MapFS{"f/a.ttf": {Data: readTestFont(t, "TelaSbixTest.ttf")}}, reads: &reads}
	f := FontFS(fsys, "Lazy", "f/*.ttf").WithGenerics(sansNames...)
	if err := f.Check(); err != nil || reads != 0 {
		t.Fatalf("Check: %v, %d reads; Check must not read the files", err, reads)
	}
	c, _ := NewFontCollection([]Font{f}, nil, nil)
	if reads != 0 {
		t.Fatalf("%d reads before text needs the family", reads)
	}
	if got := familyOf(c, "Lazy"); got != "Lazy" {
		t.Fatalf("family %q", got)
	}
	if reads != 1 {
		t.Errorf("%d reads, want 1", reads)
	}
	// A second collection of the same font reads nothing more.
	c2, _ := NewFontCollection([]Font{f}, nil, nil)
	familyOf(c2, "sans-serif")
	if reads != 1 {
		t.Errorf("%d reads after a second collection, want 1", reads)
	}
}

type fontCountFS struct {
	fstest.MapFS
	reads *int
}

func (c *fontCountFS) ReadFile(name string) ([]byte, error) {
	*c.reads++
	return c.MapFS.ReadFile(name)
}

// TestAppFontsWinOverTheSystem: an app's Sans font is the sans-serif even
// when the system has fonts for it, and the system's still fill the rest.
func TestAppFontsWinOverTheSystem(t *testing.T) {
	// The "system": a directory of fonts, with the desktop's interface
	// family set to one of them, which leads sans-serif and system-ui.
	sys := &text.SystemFontOptions{SkipSystemDirs: true, Dirs: []string{testFontDir}, UIFamily: "Noto Sans CJK JP", NoCache: true}
	plain, err := NewFontCollection(nil, nil, sys)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{"sans-serif", "system-ui", "ui-sans-serif"} {
		if got := familyOf(plain, g); got != "Noto Sans CJK JP" {
			t.Fatalf("without app fonts %s = %q, want the system's Noto Sans CJK JP", g, got)
		}
	}
	mine := FontData("Mine", readTestFont(t, "TelaSbixTest.ttf")).WithGenerics(sansNames...)
	c, err := NewFontCollection([]Font{mine}, nil, sys)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{"sans-serif", "system-ui", "ui-sans-serif"} {
		if got := familyOf(c, g); got != "Mine" {
			t.Errorf("%s = %q, want the app's Mine", g, got)
		}
	}
	// A generic the app's font does not stand for still resolves to the
	// system's; a named installed family too.
	if got := familyOf(c, "Noto Sans CJK JP"); got != "Noto Sans CJK JP" {
		t.Errorf("named system family = %q", got)
	}
	// Fallback: a character Mine lacks and the system has comes from the
	// system, after the app's fonts.
	rs := []rune("a")
	runs := c.Itemize(rs, 0, 1, text.Query{Families: []string{"sans-serif"}}, 16, language.Latin, nil)
	if len(runs) != 1 || runs[0].Face.Typeface().Family() != "Noto Sans CJK JP" {
		t.Errorf("fallback for a: %v", runs)
	}
}

// TestAppFontsBeforeTheDefaultSet: telatest's collection, the app's fonts
// and then the deterministic set.
func TestAppFontsBeforeTheDefaultSet(t *testing.T) {
	mine := FontData("Mine", readTestFont(t, "TelaSbixTest.ttf")).WithGenerics(sansNames...).WithGenerics("serif")
	c, _ := NewFontCollection([]Font{mine}, stdfonts.Sources(), nil)
	if got := familyOf(c, "sans-serif"); got != "Mine" {
		t.Errorf("sans-serif = %q, want Mine", got)
	}
	if got := familyOf(c, "serif"); got != "Mine" {
		t.Errorf("serif = %q, want Mine", got)
	}
	if got := familyOf(c, "monospace"); got != stdfonts.JetBrainsMono {
		t.Errorf("monospace = %q, want %s", got, stdfonts.JetBrainsMono)
	}
	// The characters Mine lacks come from the set.
	rs := []rune("a")
	runs := c.Itemize(rs, 0, 1, text.Query{Families: []string{"sans-serif"}}, 16, language.Latin, nil)
	if len(runs) != 1 || runs[0].Face.Typeface().Family() != stdfonts.Inter {
		t.Errorf("fallback for a: %v", runs)
	}
}
