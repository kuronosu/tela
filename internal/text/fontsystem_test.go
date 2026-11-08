package text

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/gfx"
)

// The tests of ADR-244: font sources parsed on first need, the generic
// families of each operating system, and text without any font.

// countedSources returns new standard sources (not shared with other
// tests, so nothing is parsed yet) whose files are read through Load,
// counting the reads of each family.
func countedSources(loads map[string]*atomic.Int32, delay time.Duration) []*FontSource {
	var out []*FontSource
	for _, s := range stdSources() {
		n := &atomic.Int32{}
		loads[s.Family] = n
		data := s.Data
		out = append(out, &FontSource{
			Family:   s.Family,
			Generics: s.Generics,
			Scripts:  s.Scripts,
			Load: func() ([][]byte, error) {
				n.Add(1)
				time.Sleep(delay)
				return data, nil
			},
		})
	}
	return out
}

func collectionOf(t testing.TB, srcs ...*FontSource) *Collection {
	t.Helper()
	c := NewCollection()
	for _, s := range srcs {
		if err := c.AddSource(s); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

// sourcesByFamily indexes sources by family name.
func sourcesByFamily(srcs []*FontSource) map[string]*FontSource {
	m := map[string]*FontSource{}
	for _, s := range srcs {
		m[s.Family] = s
	}
	return m
}

var sansStack = []string{"ui-sans-serif", "system-ui", "sans-serif"}

func layoutText(c *Collection, s string, st TextStyle) *Layout {
	if st.Size == 0 {
		st.Size = 16
	}
	if st.Families == nil {
		st.Families = sansStack
	}
	return NewLayout(c, &Paragraph{Text: s, Default: st})
}

// TestSourceParsedOnFirstNeed checks that registering a source reads
// nothing, and that each family (each font of it) is read and parsed only
// when text first needs it: by name or generic, as a script fallback, as
// the emoji family, or when fallback walks the registered families.
func TestSourceParsedOnFirstNeed(t *testing.T) {
	loads := map[string]*atomic.Int32{}
	srcs := countedSources(loads, 0)
	by := sourcesByFamily(srcs)
	c := collectionOf(t, srcs...)
	want := func(step string, wantLoads, wantParsed map[string]int) {
		t.Helper()
		for fam, n := range wantLoads {
			if got := int(loads[fam].Load()); got != n {
				t.Errorf("%s: %s read %d times, want %d", step, fam, got, n)
			}
		}
		for fam, n := range wantParsed {
			if got := by[fam].parsedFonts(); got != n {
				t.Errorf("%s: %d fonts of %s parsed, want %d", step, got, fam, n)
			}
		}
	}
	none := map[string]int{FamilyInter: 0, FamilyJetBrainsMono: 0, FamilyNotoSansArabic: 0, FamilyNotoColorEmoji: 0}
	want("registered", none, none)
	if got := c.Families(); !reflect.DeepEqual(got, []string{FamilyInter, FamilyJetBrainsMono, FamilyNotoSansArabic, FamilyNotoColorEmoji}) {
		t.Errorf("Families() = %v", got)
	}
	want("Families", none, none)

	// Latin text in the sans stack: Inter upright only (its italic file
	// is described, not parsed); the emoji, Arabic and mono sources are
	// not even read.
	l := layoutText(c, "Hello, world 123", TextStyle{})
	if l.Width() <= 0 {
		t.Fatal("no width")
	}
	want("Latin", map[string]int{FamilyInter: 1, FamilyJetBrainsMono: 0, FamilyNotoSansArabic: 0, FamilyNotoColorEmoji: 0},
		map[string]int{FamilyInter: 1, FamilyJetBrainsMono: 0, FamilyNotoSansArabic: 0, FamilyNotoColorEmoji: 0})
	layoutText(c, "Italic", TextStyle{Style: StyleItalic})
	want("italic", map[string]int{FamilyInter: 1}, map[string]int{FamilyInter: 2})
	// Arabic: the script fallback.
	layoutText(c, "مرحبا", TextStyle{})
	want("Arabic", map[string]int{FamilyNotoSansArabic: 1, FamilyNotoColorEmoji: 0, FamilyJetBrainsMono: 0}, map[string]int{FamilyNotoSansArabic: 1})
	// Emoji: the emoji generic.
	layoutText(c, "😀", TextStyle{})
	want("emoji", map[string]int{FamilyNotoColorEmoji: 1, FamilyJetBrainsMono: 0}, map[string]int{FamilyNotoColorEmoji: 1, FamilyJetBrainsMono: 0})
	// A character nobody has walks every registered family: JetBrains
	// Mono is parsed now (its upright font, for an upright request).
	layoutText(c, "\U000F0000", TextStyle{})
	want("uncovered", map[string]int{FamilyJetBrainsMono: 1}, map[string]int{FamilyJetBrainsMono: 1})
	// Nothing is read twice.
	want("end", map[string]int{FamilyInter: 1, FamilyJetBrainsMono: 1, FamilyNotoSansArabic: 1, FamilyNotoColorEmoji: 1}, nil)

	// The faces are those of an eager parse: same glyphs and advances.
	f := c.Face(Query{Families: []string{"Inter"}, Weight: 650}, 20)
	ref := NewDefaultCollection().Face(Query{Families: []string{"Inter"}, Weight: 650}, 20)
	for _, r := range "Hamburgefonstiv" {
		g1, _ := f.GlyphIndex(r)
		g2, _ := ref.GlyphIndex(r)
		if g1 != g2 || f.Advance(g1) != ref.Advance(g2) || f.GlyphBounds(g1) != ref.GlyphBounds(g2) {
			t.Fatalf("%q differs from the standard collection's face", r)
		}
	}
}

// TestSourceConcurrentFirstUse lays text out from many goroutines and two
// collections that share the sources, all at once: each file is read once,
// each font parsed once, and every goroutine gets the same faces.
func TestSourceConcurrentFirstUse(t *testing.T) {
	loads := map[string]*atomic.Int32{}
	srcs := countedSources(loads, 20*time.Millisecond)
	cs := []*Collection{collectionOf(t, srcs...), collectionOf(t, srcs...)}
	text := []rune("Hello مرحبا 😀 ok")
	const n = 16
	runs := make([][]Run, n)
	var start, wg sync.WaitGroup
	start.Add(1)
	for g := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start.Wait()
			c := cs[g%2]
			if g%3 == 0 {
				layoutText(c, string(text), TextStyle{})
			}
			runs[g] = c.Itemize(text, 0, len(text), Query{Families: sansStack}, 16, language.Latin, nil)
		}()
	}
	start.Done()
	wg.Wait()
	for g := 1; g < n; g++ {
		if !reflect.DeepEqual(runs[g], runs[0]) {
			t.Fatalf("goroutine %d: runs %v, want %v", g, runs[g], runs[0])
		}
	}
	if len(runs[0]) < 4 {
		t.Fatalf("runs %v: want Latin, Arabic and emoji runs", runs[0])
	}
	by := sourcesByFamily(srcs)
	for fam, want := range map[string]int{FamilyInter: 1, FamilyNotoSansArabic: 1, FamilyNotoColorEmoji: 1, FamilyJetBrainsMono: 0} {
		if got := int(loads[fam].Load()); got != min(want, 1) {
			t.Errorf("%s read %d times, want %d", fam, got, min(want, 1))
		}
		if got := by[fam].parsedFonts(); got != want {
			t.Errorf("%s: %d fonts parsed, want %d", fam, got, want)
		}
	}
}

// logRecorder collects what a collection logs.
type logRecorder struct {
	mu   sync.Mutex
	msgs []string
}

func (l *logRecorder) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, fmt.Sprintf(format, args...))
}

func (l *logRecorder) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.msgs, "\n")
}

// TestSourceErrors checks that files that cannot be used drop what they
// hold and are reported, without panics, and that fallback goes on.
func TestSourceErrors(t *testing.T) {
	var log logRecorder
	broken := &FontSource{Family: "Broken", Data: [][]byte{[]byte("not a font file")}}
	failing := &FontSource{Family: "Failing", Load: func() ([][]byte, error) { return nil, errors.New("disk on fire") }}
	mixed := &FontSource{Family: "Mixed", Data: [][]byte{[]byte("junk"), fontJetBrainsMono.data()}}
	// The upright file describes but cannot be parsed (no maxp): matching
	// selects it, fails, and falls back to the italic one.
	noMaxp := rebuildSfnt(t, fontInter.data(), nil, "maxp")
	partial := &FontSource{Family: "Partial", Data: [][]byte{noMaxp, fontInterItalic.data()}}
	c := collectionOf(t, broken, failing, mixed, partial, stdSources()[0])
	c.SetLogger(&log)

	if f := c.Face(Query{Families: []string{"Broken", "Failing", "sans-serif"}}, 16); f == nil || f.Typeface().Family() != FamilyInter {
		t.Fatalf("Broken, Failing, sans-serif: %v, want Inter", f)
	}
	if c.Match("Broken", Query{}) != nil || c.Match("Failing", Query{}) != nil {
		t.Error("a family without a usable font matched")
	}
	if broken.Err() == nil || failing.Err() == nil || !strings.Contains(failing.Err().Error(), "disk on fire") {
		t.Errorf("errors: %v; %v", broken.Err(), failing.Err())
	}
	if tf := c.Match("Mixed", Query{}); tf == nil || tf.Family() != "Mixed" || tf.NumGlyphs() == 0 {
		t.Errorf("Mixed: %v, want the JetBrains Mono font under the name Mixed", tf)
	}
	if mixed.Err() == nil {
		t.Error("Mixed: the junk file was not reported")
	}
	if tf := c.Match("Partial", Query{}); tf == nil || tf.Style() != StyleItalic {
		t.Errorf("Partial: %v, want the italic font (the upright one cannot be parsed)", tf)
	}
	if err := partial.Err(); err == nil || !strings.Contains(err.Error(), "file 0") {
		t.Errorf("Partial: error %v, want one about file 0", err)
	}
	got := log.all()
	for _, fam := range []string{"Broken", "Failing", "Mixed", "Partial"} {
		if !strings.Contains(got, `"`+fam+`"`) {
			t.Errorf("the log does not name %s:\n%s", fam, got)
		}
	}
	// Each problem is logged once, however often the family is asked for.
	before := len(log.msgs)
	c.Match("Broken", Query{})
	c.Match("Partial", Query{Weight: 700})
	if len(log.msgs) != before {
		t.Errorf("logged again:\n%s", log.all())
	}
	// Typefaces reports the same.
	tfs, err := partial.Typefaces()
	if len(tfs) != 1 || err == nil {
		t.Errorf("Typefaces: %d, %v", len(tfs), err)
	}
}

func TestAddSourceInvalid(t *testing.T) {
	c := NewCollection()
	for _, s := range []*FontSource{nil, {Data: [][]byte{fontInter.data()}}, {Family: "  "}, {Family: "X"}} {
		if err := c.AddSource(s); err == nil {
			t.Errorf("AddSource(%+v) accepted", s)
		}
	}
	if len(c.Families()) != 0 {
		t.Errorf("a refused source registered %v", c.Families())
	}
	// Adding a source twice registers it once; its generics and scripts
	// are listed once.
	s := &FontSource{Family: "Twice", Data: [][]byte{fontInter.data()}, Generics: []string{"sans-serif"}, Scripts: []language.Script{language.Latin}}
	for range 2 {
		if err := c.AddSource(s); err != nil {
			t.Fatal(err)
		}
	}
	if f := c.families["twice"]; len(f.srcs) != 1 || len(c.generics["sans-serif"]) != 1 || len(c.scripts[language.Latin]) != 1 {
		t.Errorf("sources %d, generics %v, scripts %v", len(f.srcs), c.generics, c.scripts)
	}
}

// TestSourceDescriptionMatchesTypeface checks that the description of a
// font (what matching reads before parsing) equals the parsed typeface.
func TestSourceDescriptionMatchesTypeface(t *testing.T) {
	files := map[string][]byte{"cjk": cjkFontData(t)}
	for _, f := range allStdFonts {
		files[f.String()] = f.data()
	}
	for name, data := range files {
		src := &FontSource{Family: "", Data: [][]byte{data}}
		fonts := src.fontsOf(nil)
		tfs, err := ParseTypefaces(data, "")
		if err != nil || len(fonts) != len(tfs) {
			t.Fatalf("%s: %d described, %d parsed (%v)", name, len(fonts), len(tfs), err)
		}
		for i, f := range fonts {
			d, p := f.stub, tfs[i]
			if d.family != p.family || d.name != p.name || d.style != p.style || d.weight != p.weight || d.stretch != p.stretch || !reflect.DeepEqual(d.axes, p.axes) {
				t.Errorf("%s: described %q %q %v %v %v %v, parsed %q %q %v %v %v %v", name,
					d.family, d.name, d.style, d.weight, d.stretch, d.axes, p.family, p.name, p.style, p.weight, p.stretch, p.axes)
			}
		}
	}
}

// rebuildSfnt returns a copy of a single-font sfnt file with the tables of
// replace substituted (or added) and the tables of drop removed. Checksums
// are not recomputed; nothing here checks them.
func rebuildSfnt(t testing.TB, data []byte, replace map[string][]byte, drop ...string) []byte {
	t.Helper()
	be := binary.BigEndian
	if len(data) < 12 {
		t.Fatal("not an sfnt")
	}
	type table struct {
		tag  string
		data []byte
	}
	var tabs []table
	n := int(be.Uint16(data[4:]))
	seen := map[string]bool{}
	for i := range n {
		rec := data[12+16*i:]
		tag := string(rec[:4])
		off, ln := be.Uint32(rec[8:]), be.Uint32(rec[12:])
		seen[tag] = true
		if strings.Contains(strings.Join(drop, ","), tag) {
			continue
		}
		b := data[off : off+ln]
		if r, ok := replace[tag]; ok {
			b = r
		}
		tabs = append(tabs, table{tag, b})
	}
	for tag, b := range replace {
		if !seen[tag] {
			tabs = append(tabs, table{tag, b})
		}
	}
	out := make([]byte, 12+16*len(tabs))
	copy(out, data[:4])
	be.PutUint16(out[4:], uint16(len(tabs)))
	for i, tb := range tabs {
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		rec := out[12+16*i:]
		copy(rec, tb.tag)
		be.PutUint32(rec[8:], uint32(len(out)))
		be.PutUint32(rec[12:], uint32(len(tb.data)))
		out = append(out, tb.data...)
	}
	return out
}

// nameTable returns a name table (format 0, Windows Unicode English) that
// calls the font family, in the regular style.
func nameTable(family string) []byte {
	be := binary.BigEndian
	recs := []struct {
		id uint16
		s  string
	}{{1, family}, {2, "Regular"}, {4, family}, {6, strings.ReplaceAll(family, " ", "")}, {16, family}, {17, "Regular"}}
	var str []byte
	out := make([]byte, 6+12*len(recs))
	be.PutUint16(out[2:], uint16(len(recs)))
	be.PutUint16(out[4:], uint16(len(out)))
	for i, r := range recs {
		var s []byte
		for _, u := range utf16.Encode([]rune(r.s)) {
			s = be.AppendUint16(s, u)
		}
		rec := out[6+12*i:]
		be.PutUint16(rec[0:], 3)     // Windows
		be.PutUint16(rec[2:], 1)     // Unicode BMP
		be.PutUint16(rec[4:], 0x409) // English
		be.PutUint16(rec[6:], r.id)
		be.PutUint16(rec[8:], uint16(len(s)))
		be.PutUint16(rec[10:], uint16(len(str)))
		str = append(str, s...)
	}
	return append(out, str...)
}

// renamedFonts writes copies of JetBrains Mono called each of families
// into a new directory.
func renamedFonts(t testing.TB, families ...string) string {
	t.Helper()
	dir := t.TempDir()
	for i, fam := range families {
		data := rebuildSfnt(t, fontJetBrainsMono.data(), map[string][]byte{"name": nameTable(fam)})
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("font%02d.ttf", i)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// systemOn returns c with the fonts of dir as the system fonts, the
// generic lists of goos.
func systemOn(t testing.TB, c *Collection, dir, goos string, opts SystemFontOptions) *Collection {
	t.Helper()
	opts.Dirs, opts.SkipSystemDirs, opts.goos = []string{dir}, true, goos
	if err := c.EnableSystemFonts(opts); err != nil {
		t.Fatal(err)
	}
	return c
}

// genericFamily returns the family a generic name resolves to.
func genericFamily(c *Collection, generic string) string {
	f := c.Face(Query{Families: []string{generic}}, 16)
	if f == nil {
		return "<none>"
	}
	return f.Typeface().Family()
}

// installed is one machine for TestSystemGenericsPerOS: the families of
// the three systems' lists, installed together, so that each system's list
// decides.
var installed = []string{
	// macOS
	".SF NS", "Helvetica", "New York", "Times", ".SF NS Mono", "Menlo", "Apple Color Emoji",
	// Windows
	"Segoe UI", "Cambria", "Times New Roman", "Consolas", "Courier New", "Segoe UI Emoji",
	// Linux
	"DejaVu Sans", "Noto Sans", "Liberation Serif", "DejaVu Sans Mono", "Liberation Mono", "Noto Color Emoji",
}

func TestSystemGenericsPerOS(t *testing.T) {
	dir := renamedFonts(t, installed...)
	for _, tc := range []struct {
		goos string
		want map[string]string
	}{
		{"darwin", map[string]string{
			"system-ui": ".SF NS", "ui-sans-serif": ".SF NS", "sans-serif": "Helvetica",
			"ui-serif": "New York", "serif": "Times", "ui-monospace": ".SF NS Mono", "monospace": "Menlo",
			"emoji": "Apple Color Emoji", "ui-rounded": ".SF NS",
		}},
		{"windows", map[string]string{
			"system-ui": "Segoe UI", "ui-sans-serif": "Segoe UI", "sans-serif": "Segoe UI",
			"ui-serif": "Cambria", "serif": "Cambria", "ui-monospace": "Consolas", "monospace": "Consolas",
			"emoji": "Segoe UI Emoji",
		}},
		{"linux", map[string]string{
			"system-ui": "Noto Sans", "ui-sans-serif": "Noto Sans", "sans-serif": "Noto Sans",
			"ui-serif": "Liberation Serif", "serif": "Liberation Serif",
			"ui-monospace": "DejaVu Sans Mono", "monospace": "DejaVu Sans Mono", "emoji": "Noto Color Emoji",
		}},
		{"freebsd", map[string]string{"sans-serif": "Noto Sans", "monospace": "DejaVu Sans Mono"}},
	} {
		c := systemOn(t, NewCollection(), dir, tc.goos, SystemFontOptions{})
		for g, want := range tc.want {
			if got := genericFamily(c, g); got != want {
				t.Errorf("%s: %s resolves to %s, want %s", tc.goos, g, got, want)
			}
		}
		// A generic with nothing installed falls back to the system's
		// default face; an unknown generic keyword (fangsong) too.
		if got := genericFamily(c, "fangsong"); got == "<none>" {
			t.Errorf("%s: fangsong: no face", tc.goos)
		}
		// The theme's stack draws with the system's face.
		l := layoutText(c, "Hello", TextStyle{})
		if got := l.Line(0); got.Width <= 0 {
			t.Errorf("%s: the sans stack laid out nothing", tc.goos)
		}
	}
	// The lists are the running system's by default.
	if got, want := systemGenerics("", "", ""), systemGenerics(runtime.GOOS, "", ""); !reflect.DeepEqual(got, want) {
		t.Error("the default lists are not runtime.GOOS's")
	}
}

// TestSystemGenericsDesktopFamilies checks that the desktop's interface
// and monospace families lead their generic names when installed.
func TestSystemGenericsDesktopFamilies(t *testing.T) {
	dir := renamedFonts(t, installed...)
	c := systemOn(t, NewCollection(), dir, "linux", SystemFontOptions{UIFamily: "DejaVu Sans", MonoFamily: "Liberation Mono"})
	for g, want := range map[string]string{
		"system-ui": "DejaVu Sans", "ui-sans-serif": "DejaVu Sans", "sans-serif": "DejaVu Sans",
		"ui-monospace": "Liberation Mono", "monospace": "Liberation Mono", "serif": "Liberation Serif",
	} {
		if got := genericFamily(c, g); got != want {
			t.Errorf("%s resolves to %s, want %s", g, got, want)
		}
	}
	// Families that are not installed are skipped.
	c = systemOn(t, NewCollection(), dir, "linux", SystemFontOptions{UIFamily: "Not Installed Sans", MonoFamily: "Nope Mono"})
	if got := genericFamily(c, "system-ui"); got != "Noto Sans" {
		t.Errorf("system-ui with an uninstalled UIFamily: %s", got)
	}
	if got := genericFamily(c, "monospace"); got != "DejaVu Sans Mono" {
		t.Errorf("monospace with an uninstalled MonoFamily: %s", got)
	}
}

// TestSetDesktopFamilies checks that new desktop families replace the
// old ones for the generic names, drop the resolved queries and keep what
// was loaded.
func TestSetDesktopFamilies(t *testing.T) {
	dir := renamedFonts(t, installed...)
	c := systemOn(t, NewCollection(), dir, "linux", SystemFontOptions{UIFamily: "DejaVu Sans"})
	if got := genericFamily(c, "system-ui"); got != "DejaVu Sans" {
		t.Fatalf("system-ui: %s", got)
	}
	parsed := c.sys.loadedTypefaces()
	if !c.SetDesktopFamilies("Segoe UI", "Liberation Mono") {
		t.Error("SetDesktopFamilies reports no change")
	}
	if c.SetDesktopFamilies("Segoe UI", "Liberation Mono") {
		t.Error("the same families again report a change")
	}
	for g, want := range map[string]string{
		"system-ui": "Segoe UI", "ui-sans-serif": "Segoe UI", "sans-serif": "Segoe UI",
		"ui-monospace": "Liberation Mono", "monospace": "Liberation Mono", "serif": "Liberation Serif",
	} {
		if got := genericFamily(c, g); got != want {
			t.Errorf("after the change, %s resolves to %s, want %s", g, got, want)
		}
	}
	if !c.sys.indexed.Load() || c.sys.loadedTypefaces() < parsed {
		t.Error("the change dropped the index or the parsed faces")
	}
	// Empty families give the operating system's lists back.
	c.SetDesktopFamilies("", "")
	if got := genericFamily(c, "system-ui"); got != "Noto Sans" {
		t.Errorf("system-ui without desktop families: %s", got)
	}
	if got := genericFamily(c, "monospace"); got != "DejaVu Sans Mono" {
		t.Errorf("monospace without desktop families: %s", got)
	}
	// The app's mapping still wins.
	c.SetGeneric("sans-serif", "Liberation Serif")
	c.SetDesktopFamilies("DejaVu Sans", "")
	if got := genericFamily(c, "sans-serif"); got != "Liberation Serif" {
		t.Errorf("sans-serif mapped by the app: %s", got)
	}
	// Without system fonts nothing changes.
	if NewCollection().SetDesktopFamilies("DejaVu Sans", "") {
		t.Error("a collection without system fonts reports a change")
	}
}

// TestSetDesktopFamiliesConcurrent changes the desktop families while
// other goroutines lay text out (run with -race): every face is of one of
// the families system-ui can resolve to, and once the last change
// returned it decides.
func TestSetDesktopFamiliesConcurrent(t *testing.T) {
	dir := renamedFonts(t, installed...)
	c := systemOn(t, NewCollection(), dir, "linux", SystemFontOptions{})
	var wg sync.WaitGroup
	var stop atomic.Bool
	errs := make(chan string, 64)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				var f *Face
				switch g % 3 {
				case 0:
					f = c.Face(Query{Families: sansStack, Weight: float32(100 * (1 + i%9))}, float32(10+i%20))
				case 1:
					if tf := c.Match("system-ui", Query{}); tf != nil {
						f = tf.Face(Instance{})
					}
				default:
					if l := layoutText(c, "Hello", TextStyle{}); l.Line(0).Width <= 0 {
						errs <- "empty layout"
						return
					}
					continue
				}
				if f == nil {
					errs <- "no face"
					return
				}
				if fam := f.Typeface().Family(); fam != "DejaVu Sans" && fam != "Segoe UI" && fam != "Noto Sans" {
					errs <- "face of " + fam
					return
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		ui := "DejaVu Sans"
		if i%2 == 1 {
			ui = "Segoe UI"
		}
		c.SetDesktopFamilies(ui, "")
	}
	stop.Store(true)
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	c.SetDesktopFamilies("Segoe UI", "")
	if got := genericFamily(c, "system-ui"); got != "Segoe UI" {
		t.Errorf("after the last change, system-ui is %s", got)
	}
}

// TestPreload checks that Preload loads the index without parsing a face,
// alone or while text that needs the index is laid out (run with -race),
// and does nothing without system fonts.
func TestPreload(t *testing.T) {
	dir := renamedFonts(t, installed...)
	c := systemOn(t, NewCollection(), dir, "linux", SystemFontOptions{UIFamily: "DejaVu Sans"})
	c.Preload()
	if !c.sys.indexed.Load() {
		t.Fatal("Preload did not load the index")
	}
	if n := c.sys.loadedTypefaces(); n != 0 {
		t.Errorf("Preload parsed %d faces", n)
	}
	c.Preload() // loaded: nothing to do

	c = systemOn(t, NewCollection(), dir, "linux", SystemFontOptions{UIFamily: "DejaVu Sans"})
	var wg sync.WaitGroup
	fams := make([]string, 4)
	wg.Add(1 + len(fams))
	go func() { defer wg.Done(); c.Preload() }()
	for i := range fams {
		go func() { defer wg.Done(); fams[i] = genericFamily(c, "system-ui") }()
	}
	wg.Wait()
	for _, f := range fams {
		if f != "DejaVu Sans" {
			t.Errorf("system-ui laid out during Preload: %s", f)
		}
	}
	NewCollection().Preload() // no system fonts: nothing to do
}

// TestSystemGenericsAppWins checks that the app's mapping of a generic
// wins over the system's list, and that a family of the list the app
// registered wins over the installed one of the same name.
func TestSystemGenericsAppWins(t *testing.T) {
	dir := renamedFonts(t, installed...)
	app := &FontSource{Family: "Liberation Serif", Data: [][]byte{fontInter.data()}}
	c := systemOn(t, collectionOf(t, stdSources()[0], app), dir, "linux", SystemFontOptions{})
	if got := genericFamily(c, "sans-serif"); got != FamilyInter {
		t.Errorf("sans-serif, mapped by the app: %s", got)
	}
	if got := genericFamily(c, "monospace"); got != "DejaVu Sans Mono" {
		t.Errorf("monospace, not mapped: %s", got)
	}
	tf := c.Match("serif", Query{})
	if tf == nil || tf.Family() != "Liberation Serif" || tf.axisIndex(tagOpsz) < 0 {
		t.Errorf("serif: %v, want the app's Liberation Serif (Inter's file, with an opsz axis)", tf)
	}
	// SetGeneric replaces the system's list for the name.
	c.SetGeneric("monospace", FamilyInter)
	if got := genericFamily(c, "monospace"); got != FamilyInter {
		t.Errorf("monospace after SetGeneric: %s", got)
	}
}

// TestNewSystemCollectionUnderTest checks the guard of ADR-035: under go
// test the machine's fonts are refused, and the collection is empty.
func TestNewSystemCollectionUnderTest(t *testing.T) {
	t.Setenv("TELA_SYSTEM_FONTS", "")
	c, err := NewSystemCollection(SystemFontOptions{})
	if err == nil || c == nil {
		t.Fatalf("NewSystemCollection under go test: %v, %v", c, err)
	}
	if c.Face(Query{Families: sansStack}, 16) != nil {
		t.Error("a refused system collection has a face")
	}
	dir := renamedFonts(t, "Noto Sans")
	c, err = NewSystemCollection(SystemFontOptions{Dirs: []string{dir}, SkipSystemDirs: true, goos: "linux"})
	if err != nil || genericFamily(c, "system-ui") != "Noto Sans" {
		t.Fatalf("explicit directories: %v, %s", err, genericFamily(c, "system-ui"))
	}
}

// TestNoFonts checks text without any font: it lays out with fallback
// metrics and no glyphs, draws nothing, and the collection reports it once.
func TestNoFonts(t *testing.T) {
	broken := collectionOf(t, &FontSource{Family: "Broken", Data: [][]byte{[]byte("junk")}, Generics: []string{"sans-serif"}})
	for name, c := range map[string]*Collection{"nil": nil, "empty": NewCollection(), "broken": broken} {
		calls := 0
		if c != nil {
			c.SetNoFontHandler(func() { calls++ })
		}
		for _, lh := range []LineHeight{{}, LineHeightMultiple(1.5)} {
			st := TextStyle{Families: sansStack, Size: 20, LineHeight: lh, Decoration: TextDecoration{Lines: DecorationUnderline}}
			l := NewLayout(c, &Paragraph{Text: "Hello wörld 😀 مرحبا\n\tsecond line", Default: st, Style: ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}})
			l.BreakLines(50)
			lineH := float32(20) // ascent 16 + descent 4
			if lh.Kind == LineHeightScale {
				lineH = 30
			}
			if l.NumLines() != 2 || l.Width() != 0 || math.Abs(float64(l.Height()-2*lineH)) > 1e-3 {
				t.Errorf("%s, line height %v: %d lines, %gx%g, want 2 lines of %g", name, lh, l.NumLines(), l.Width(), l.Height(), lineH)
			}
			if l.NumClusters() == 0 {
				t.Errorf("%s: no clusters", name)
			}
			b := gfx.NewBuilder()
			l.PaintScaled(b, gfx.Pt(0, 0), 2)
			if dl := b.Finish(); dl.Len() != 0 {
				t.Errorf("%s: %d commands painted, want none", name, dl.Len())
			}
			// Editing geometry works on it.
			l.HitTest(10, 25)
			l.SelectionRects(Selection{Anchor: Cursor{Offset: 0}, Focus: Cursor{Offset: 5}}, nil)
			l.Caret(Cursor{Offset: 3})
		}
		if c == nil {
			continue
		}
		if calls != 1 {
			t.Errorf("%s: the no-font handler ran %d times, want once", name, calls)
		}
		if f := c.Face(Query{}, 16); f != nil {
			t.Errorf("%s: Face = %v", name, f)
		}
		rs := []rune("ab c")
		if runs := c.Itemize(rs, 0, len(rs), Query{}, 16, language.Latin, nil); len(runs) != 1 || runs[0].Face != nil || runs[0].End != len(rs) {
			t.Errorf("%s: runs %+v, want one run without a face", name, runs)
		}
	}
	// A collection with fonts never calls the handler.
	c := NewDefaultCollection()
	c.SetNoFontHandler(func() { t.Error("the handler ran for a collection with fonts") })
	layoutText(c, "Hello \U000F0000", TextStyle{})
}
