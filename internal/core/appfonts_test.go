package core

import (
	"strings"
	"testing"
	"testing/fstest"

	"os"

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
	f := FontData("A", []byte("x")).WithScripts("arab", "Hebr", "Arab")
	if got := strings.Join(f.scripts, ","); got != "Arab,Hebr" {
		t.Errorf("scripts = %q, want Arab,Hebr (normalized, no duplicates)", got)
	}
	panics := func(code string) (msg string) {
		defer func() { msg, _ = recover().(string) }()
		FontData("A", []byte("x")).WithScripts(code)
		return ""
	}
	for _, bad := range []string{"Arabic", "Arbc", "", "Ar"} {
		if m := panics(bad); !strings.Contains(m, "ISO 15924") || !strings.Contains(m, `"Arab"`) {
			t.Errorf("WithScripts(%q) panic = %q, want a message with the examples", bad, m)
		}
	}
	// The original is not modified.
	if len(FontData("A", []byte("x")).scripts) != 0 {
		t.Error("a new font has scripts")
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
		t.Error("two copies with the same roles have different sources: the font would be parsed twice")
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
