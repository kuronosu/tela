package text

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/assets/fonts"
)

// The tests of system fonts never read the machine's fonts: they point the
// scanner at temporary directories (SkipSystemDirs).

func cjkFontData(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/fonts/NotoSansJP-Subset.otf")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// fontDir writes the files into a fresh temporary directory.
func fontDir(t testing.TB, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// withSystemDir enables system fonts on c with only dir as their source.
func withSystemDir(t testing.TB, c *Collection, dir string) *Collection {
	t.Helper()
	if err := c.EnableSystemFonts(SystemFontOptions{Dirs: []string{dir}, SkipSystemDirs: true}); err != nil {
		t.Fatal(err)
	}
	return c
}

// interOnly returns a collection with Inter alone, so that the other
// embedded families are only available as system fonts.
func interOnly(t testing.TB) *Collection {
	t.Helper()
	c := NewCollection()
	if _, err := c.Register(fonts.Data(fonts.InterVariable), "Inter"); err != nil {
		t.Fatal(err)
	}
	return c
}

// makeTTC packs sfnt fonts into a TrueType collection, rebasing the table
// offsets of each font.
func makeTTC(fonts ...[]byte) []byte {
	be := binary.BigEndian
	out := make([]byte, 12+4*len(fonts))
	copy(out, "ttcf")
	be.PutUint32(out[4:], 0x00010000)
	be.PutUint32(out[8:], uint32(len(fonts)))
	for i, f := range fonts {
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		base := len(out)
		be.PutUint32(out[12+4*i:], uint32(base))
		f = append([]byte(nil), f...)
		for k := 0; k < int(be.Uint16(f[4:])); k++ {
			rec := f[12+16*k:]
			be.PutUint32(rec[8:], be.Uint32(rec[8:])+uint32(base))
		}
		out = append(out, f...)
	}
	return out
}

// itemize returns the runs of s as "text:family" strings.
func itemize(c *Collection, s string, families ...string) []string {
	rs := []rune(s)
	runs := c.Itemize(rs, 0, len(rs), Query{Families: families}, 16, language.Latin, nil)
	var out []string
	for _, r := range runs {
		out = append(out, string(rs[r.Start:r.End])+":"+r.Face.Typeface().Family())
	}
	return out
}

func TestSystemFontsLazy(t *testing.T) {
	dir := fontDir(t, map[string][]byte{"cjk.otf": cjkFontData(t), "mono.ttf": fonts.Data(fonts.JetBrainsMono)})

	c := withSystemDir(t, NewDefaultCollection(), dir)
	s := c.sys
	if s.indexed.Load() {
		t.Fatal("EnableSystemFonts scanned the directories")
	}
	// Registered families, generic names, and text the embedded fonts cover
	// never need the system fonts.
	for _, fam := range [][]string{{"Inter"}, {"sans-serif", "system-ui"}, {"serif", "ui-serif", "ui-sans-serif"}, {"monospace"}, {"Noto Color Emoji"}} {
		c.Itemize([]rune("Hello مرحبا 😀 world"), 0, 16, Query{Families: fam}, 16, language.Latin, nil)
		c.Face(Query{Families: fam}, 16)
		c.Match(fam[0], Query{})
	}
	if s.indexed.Load() {
		t.Fatal("the index was loaded without an unknown family or an uncovered cluster")
	}
	// An unknown family loads it.
	if f := c.Face(Query{Families: []string{"No Such Family"}}, 16); f == nil || f.Typeface().Family() != FamilyInter {
		t.Fatalf("unknown family: got %v, want the default fallback", f)
	}
	if !s.indexed.Load() {
		t.Fatal("an unknown family did not load the index")
	}

	// An uncovered cluster loads it too.
	c = withSystemDir(t, NewDefaultCollection(), dir)
	if c.sys.indexed.Load() {
		t.Fatal("re-enabling scanned")
	}
	got := itemize(c, "ab日本", "Inter")
	if want := []string{"ab:Inter", "日本:Noto Sans CJK JP"}; !equalStrings(got, want) {
		t.Fatalf("runs %q, want %q", got, want)
	}
	if !c.sys.indexed.Load() {
		t.Fatal("an uncovered cluster did not load the index")
	}
	if c.SystemFontsErr() != nil {
		t.Fatalf("SystemFontsErr: %v", c.SystemFontsErr())
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSystemFontsOffByDefault(t *testing.T) {
	c := NewDefaultCollection()
	if c.sys != nil {
		t.Fatal("NewDefaultCollection enables system fonts")
	}
	// Without them the CJK cluster stays on the first requested face.
	got := itemize(c, "ab日本", "Inter")
	if len(got) != 1 || !strings.HasSuffix(got[0], ":Inter") {
		t.Fatalf("runs %q: CJK must stay on Inter (.notdef) without system fonts", got)
	}
	// The guard: the real system directories are refused under go test.
	t.Setenv("TELA_SYSTEM_FONTS", "")
	if err := c.EnableSystemFonts(SystemFontOptions{}); err == nil || !strings.Contains(err.Error(), "TELA_SYSTEM_FONTS") {
		t.Fatalf("EnableSystemFonts under go test: error %v, want one naming TELA_SYSTEM_FONTS", err)
	}
	if c.sys != nil {
		t.Fatal("a refused EnableSystemFonts changed the collection")
	}
	if err := c.EnableSystemFonts(SystemFontOptions{Dirs: []string{""}, SkipSystemDirs: true}); err == nil {
		t.Fatal("an empty directory name was accepted")
	}
}

func TestSystemNamedFamily(t *testing.T) {
	dir := fontDir(t, map[string][]byte{"mono.ttf": fonts.Data(fonts.JetBrainsMono), "cjk.otf": cjkFontData(t)})
	// A collection that has Inter only.
	c := interOnly(t)
	if c.Match("JetBrains Mono", Query{}) != nil {
		t.Fatal("JetBrains Mono found without system fonts")
	}
	withSystemDir(t, c, dir)

	for _, name := range []string{"JetBrains Mono", "jetbrains mono", `"JetBrainsMono"`} {
		tf := c.Match(name, Query{Weight: 700})
		if tf == nil || tf.Family() != "JetBrains Mono" {
			t.Fatalf("Match(%q) = %v, want JetBrains Mono", name, tf)
		}
	}
	// The registered family wins over an installed one with the same name,
	// and system families are not listed as registered.
	if got := c.Families(); len(got) != 1 || got[0] != "Inter" {
		t.Fatalf("Families() = %v, want [Inter]", got)
	}
	// Itemize with the named family draws Latin with it; the CJK subset (a
	// system family too) covers the rest, and Inter is not consulted for
	// what the first family has.
	got := itemize(c, "mono 日本", "JetBrains Mono", "Inter")
	if want := []string{"mono :JetBrains Mono", "日本:Noto Sans CJK JP"}; !equalStrings(got, want) {
		t.Fatalf("runs %q, want %q", got, want)
	}
	// Requesting the CJK family by name.
	got = itemize(c, "ab日本", "Noto Sans CJK JP")
	if len(got) != 1 || got[0] != "ab日本:Noto Sans CJK JP" {
		t.Fatalf("runs %q, want one run of the named family", got)
	}
	// Unknown names stay unknown (and do not break the lookup of known ones).
	if c.Match("Comic Nope", Query{}) != nil {
		t.Fatal("unknown family matched")
	}
	// Query styles reach the loaded face.
	f := c.Face(Query{Families: []string{"JetBrains Mono"}, Weight: 700}, 14)
	if f == nil || f.Typeface().Family() != "JetBrains Mono" {
		t.Fatalf("Face = %v", f)
	}
	if lo, hi := f.Typeface().WeightRange(); lo > 400 || hi < 700 {
		t.Fatalf("variable JetBrains Mono weight range %g-%g does not cover 700", lo, hi)
	}
}

func TestSystemRegisteredFamiliesWin(t *testing.T) {
	// The directory holds copies of every embedded font. Nothing the
	// embedded fonts cover may come from the directory, so default
	// rendering is the same with and without system fonts.
	files := map[string][]byte{}
	for _, f := range fonts.All() {
		files[fonts.Name(f)+".ttf"] = fonts.Data(f)
	}
	files["cjk.otf"] = cjkFontData(t)
	dir := fontDir(t, files)

	plain := NewDefaultCollection()
	sys := withSystemDir(t, NewDefaultCollection(), dir)
	text := "Hello, مرحبا بكم 😀🎉 ❤️ 1234 ‰ → é"
	for _, fam := range [][]string{{"Inter"}, {"Nope"}, {"monospace"}, {"JetBrains Mono", "Noto Sans Arabic"}, nil} {
		rs := []rune(text)
		q := Query{Families: fam}
		a := plain.Itemize(rs, 0, len(rs), q, 16, language.Latin, nil)
		b := sys.Itemize(rs, 0, len(rs), q, 16, language.Latin, nil)
		if len(a) != len(b) {
			t.Fatalf("families %q: %d runs without system fonts, %d with", fam, len(a), len(b))
		}
		for i := range a {
			if a[i].Start != b[i].Start || a[i].End != b[i].End || a[i].Face != b[i].Face {
				t.Errorf("families %q run %d: %+v without system fonts, %+v with", fam, i, a[i], b[i])
			}
		}
	}
	if tf := sys.Match("Inter", Query{}); tf == nil || tf != plain.Match("Inter", Query{}) {
		t.Error("Match(Inter) is not the embedded typeface")
	}
}

func TestSystemTrueTypeCollection(t *testing.T) {
	ttc := makeTTC(fonts.Data(fonts.JetBrainsMono), cjkFontData(t))
	dir := fontDir(t, map[string][]byte{"both.ttc": ttc, "notes.txt": []byte("not a font"), "junk.ttf": []byte("garbage garbage garbage")})
	c := withSystemDir(t, interOnly(t), dir)

	if tf := c.Match("JetBrains Mono", Query{}); tf == nil || tf.Family() != "JetBrains Mono" {
		t.Fatalf("Match from collection index 0: %v", tf)
	}
	got := itemize(c, "ab日本", "Inter")
	if want := []string{"ab:Inter", "日本:Noto Sans CJK JP"}; !equalStrings(got, want) {
		t.Fatalf("runs %q, want %q", got, want)
	}
	s := c.sys
	if len(s.entries) != 2 {
		t.Fatalf("indexed %d fonts, want the 2 of the collection (junk and text files skipped)", len(s.entries))
	}
	if len(s.data) != 1 {
		t.Errorf("%d file contents held, want the collection read once", len(s.data))
	}
	if len(s.typefaces) != 2 {
		t.Errorf("%d typefaces parsed, want 2", len(s.typefaces))
	}
}

// loadedTypefaces counts the typefaces parsed so far.
func (s *systemFonts) loadedTypefaces() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, tf := range s.typefaces {
		if tf != nil {
			n++
		}
	}
	return n
}

func TestSystemParsesOnlyWhatIsUsed(t *testing.T) {
	// Two fonts of a collection; asking for one parses one.
	dir := fontDir(t, map[string][]byte{"both.ttc": makeTTC(fonts.Data(fonts.JetBrainsMono), cjkFontData(t))})
	c := withSystemDir(t, interOnly(t), dir)
	if c.Match("JetBrains Mono", Query{}) == nil {
		t.Fatal("no match")
	}
	if n := c.sys.loadedTypefaces(); n != 1 {
		t.Fatalf("%d typefaces parsed for one family, want 1", n)
	}
}

func TestSystemSameTypefaceAsParse(t *testing.T) {
	// A system font is parsed by the same code as Register: same metadata,
	// same glyphs and advances.
	data := fonts.Data(fonts.JetBrainsMono)
	dir := fontDir(t, map[string][]byte{"mono.ttf": data})
	c := withSystemDir(t, NewCollection(), dir)
	got := c.Match("JetBrains Mono", Query{})
	want, err := ParseTypefaces(data, "")
	if err != nil || len(want) != 1 {
		t.Fatalf("ParseTypefaces: %v, %d typefaces", err, len(want))
	}
	if got == nil {
		t.Fatal("no system typeface")
	}
	if got.Family() != want[0].Family() || got.Name() != want[0].Name() || got.UnitsPerEm() != want[0].UnitsPerEm() || got.NumGlyphs() != want[0].NumGlyphs() {
		t.Fatalf("metadata differs: %q %q %d %d vs %q %q %d %d", got.Family(), got.Name(), got.UnitsPerEm(), got.NumGlyphs(), want[0].Family(), want[0].Name(), want[0].UnitsPerEm(), want[0].NumGlyphs())
	}
	fg := got.Face(Instance{Weight: 650})
	fw := want[0].Face(Instance{Weight: 650})
	for _, r := range "Aag@" {
		g1, ok1 := fg.GlyphIndex(r)
		g2, ok2 := fw.GlyphIndex(r)
		if g1 != g2 || ok1 != ok2 || fg.Advance(g1) != fw.Advance(g2) || fg.GlyphBounds(g1) != fw.GlyphBounds(g2) {
			t.Fatalf("rune %q: glyph %d/%v advance %g vs glyph %d/%v advance %g", r, g1, ok1, fg.Advance(g1), g2, ok2, fw.Advance(g2))
		}
	}
}

func TestSystemMissingFileAfterIndex(t *testing.T) {
	dir := fontDir(t, map[string][]byte{"cjk.otf": cjkFontData(t)})
	c := withSystemDir(t, NewDefaultCollection(), dir)
	c.sys.ensureIndex()
	if err := os.Remove(filepath.Join(dir, "cjk.otf")); err != nil {
		t.Fatal(err)
	}
	// The indexed font cannot be read any more: no panic, no face.
	if tf := c.Match("Noto Sans CJK JP", Query{}); tf != nil {
		t.Fatalf("Match found %v in a removed file", tf)
	}
	got := itemize(c, "日本", "Inter")
	if len(got) != 1 || !strings.HasSuffix(got[0], ":Inter") {
		t.Fatalf("runs %q: want the first requested face (.notdef)", got)
	}
}

func TestSystemDisable(t *testing.T) {
	dir := fontDir(t, map[string][]byte{"cjk.otf": cjkFontData(t)})
	c := withSystemDir(t, NewDefaultCollection(), dir)
	if got := itemize(c, "日", "Inter"); got[0] != "日:Noto Sans CJK JP" {
		t.Fatalf("enabled: %q", got)
	}
	c.DisableSystemFonts()
	if got := itemize(c, "日", "Inter"); got[0] != "日:Inter" {
		t.Fatalf("disabled: %q", got)
	}
	withSystemDir(t, c, dir)
	if got := itemize(c, "日", "Inter"); got[0] != "日:Noto Sans CJK JP" {
		t.Fatalf("re-enabled: %q", got)
	}
}

func TestSystemScriptFallbackNamesSystemFamily(t *testing.T) {
	// A per-script list may name an installed family.
	dir := fontDir(t, map[string][]byte{"cjk.otf": cjkFontData(t)})
	c := withSystemDir(t, NewDefaultCollection(), dir)
	c.SetScriptFallback(language.Han, "Noto Sans CJK JP")
	if got := itemize(c, "ab日本", "Inter"); !equalStrings(got, []string{"ab:Inter", "日本:Noto Sans CJK JP"}) {
		t.Fatalf("runs %q", got)
	}
}

func TestSystemConcurrent(t *testing.T) {
	dir := fontDir(t, map[string][]byte{
		"both.ttc": makeTTC(fonts.Data(fonts.JetBrainsMono), cjkFontData(t)),
	})
	text := "Hello 日本語の文章、中文简体字们说 mono مرحبا 😀 ab日"
	want := itemize(withSystemDir(t, interOnly(t), dir), text, "JetBrains Mono", "Inter")

	c := withSystemDir(t, interOnly(t), dir)
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				switch g % 4 {
				case 0:
					if got := itemize(c, text, "JetBrains Mono", "Inter"); !equalStrings(got, want) {
						errs <- strings.Join(got, "|")
						return
					}
				case 1:
					c.Face(Query{Families: []string{"Missing", "JetBrains Mono"}, Weight: float32(100 + 100*(i%9))}, float32(12+i))
				case 2:
					c.Match("Noto Sans CJK JP", Query{})
				default:
					itemize(c, "日本語 and 中文", "Inter")
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Errorf("concurrent runs differ: %s", e)
	}
	if n := c.sys.loadedTypefaces(); n != 2 {
		t.Errorf("%d typefaces parsed, want 2", n)
	}
}

// synthetic builds a systemFonts over made-up entries.
func synthetic(lang string, entries ...sysEntry) *systemFonts {
	s := newSystemFonts(SystemFontOptions{Lang: lang})
	s.indexOnce.Do(func() {})
	s.indexed.Store(true)
	s.byFamily = map[string][]int{}
	for i := range entries {
		e := &entries[i]
		e.nRunes = e.runes.Len()
		s.byFamily[e.family] = append(s.byFamily[e.family], i)
	}
	s.entries = entries
	return s
}

func entry(family string, style Style, weight float32, runes string) sysEntry {
	e := sysEntry{family: family, style: style, weight: weight, stretch: 100, file: family}
	for _, r := range runes {
		e.runes.Add(r)
	}
	return e
}

func TestSystemFallbackRanking(t *testing.T) {
	han := "日本語中文们说"
	entries := []sysEntry{
		entry("notosanscjkhk", StyleNormal, 400, han),
		entry("notosanscjkjp", StyleNormal, 400, han),
		entry("notosanscjkkr", StyleNormal, 400, han),
		entry("notosanscjksc", StyleNormal, 400, han),
		entry("notosanscjktc", StyleNormal, 400, han),
		entry("notoserifcjkjp", StyleNormal, 400, han),
		entry("notosansmonocjkjp", StyleNormal, 400, han),
		entry("unifont", StyleNormal, 400, han+"אבג"),
		entry("somehanfont", StyleNormal, 400, han),
		entry("notosanshebrew", StyleNormal, 400, "אבג"),
		entry("notoserifhebrew", StyleNormal, 400, "אבג"),
		entry("dejavusans", StyleNormal, 400, "אבגabcdefghij"),
		entry("notosansmath", StyleNormal, 400, "∑"),
	}
	for _, c := range []struct {
		lang    string
		cluster string
		want    []string
	}{
		// No language: Japanese forms, then Simplified Chinese, Korean...
		{"", "日", []string{"notosanscjkjp", "notosanscjksc", "notosanscjkkr", "notosanscjktc", "notosanscjkhk", "somehanfont", "notoserifcjkjp", "notosansmonocjkjp"}},
		{"ja_JP.UTF-8", "日", []string{"notosanscjkjp"}},
		{"zh-Hans", "日", []string{"notosanscjksc", "notosanscjkjp"}},
		{"zh-TW", "日", []string{"notosanscjktc", "notosanscjkjp"}},
		{"zh-HK", "日", []string{"notosanscjkhk", "notosanscjkjp"}},
		{"ko", "日", []string{"notosanscjkkr", "notosanscjkjp"}},
		// A script-specific sans over a serif and over a wide font.
		{"", "א", []string{"notosanshebrew", "dejavusans", "notoserifhebrew", "unifont"}},
		// Every character of the cluster must be covered.
		{"", "אa", []string{"dejavusans"}},
		{"", "∑", []string{"notosansmath"}},
		{"", "ก", nil},
	} {
		s := synthetic(c.lang, entries...)
		got := s.fallbackFamilies([]rune(c.cluster), language.LookupScript([]rune(c.cluster)[0]))
		n := min(len(got), len(c.want))
		if len(c.want) == 0 && len(got) != 0 || !equalStrings(got[:n], c.want[:n]) {
			t.Errorf("lang %q cluster %q: %v, want prefix %v", c.lang, c.cluster, got, c.want)
		}
	}
}

func TestSystemRegionPreference(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ko_KR.UTF-8")
	if got := regionPreference(""); got[0] != "kr" {
		t.Errorf("LANG=ko_KR: %v", got)
	}
	t.Setenv("LC_ALL", "zh_TW.UTF-8")
	if got := regionPreference(""); got[0] != "tc" {
		t.Errorf("LC_ALL=zh_TW: %v", got)
	}
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "")
	if got := regionPreference(""); got[0] != "jp" {
		t.Errorf("no language: %v", got)
	}
	if got := regionPreference("zh-Hant-TW"); got[0] != "tc" || len(got) != 5 {
		t.Errorf("zh-Hant-TW: %v", got)
	}
	if got := regionPreference("en-US"); got[0] != "jp" {
		t.Errorf("en-US: %v", got)
	}
}

func TestSystemRegionRank(t *testing.T) {
	prefs := []string{"kr", "jp", "sc", "tc", "hk"}
	for _, tc := range []struct {
		family string
		want   int
	}{
		{"Noto Sans CJK KR", 0},
		{"Noto Serif CJK JP", 1},
		{"Noto Sans Mono CJK SC", 2},
		{"Noto Sans TC", 3},
		{"Source Han Sans HK", 4},
		{"PingFang SC", 2},
		{"Noto Sans Misc", 5},
		{"Fira Code Disc", 5},
		{"Noto Sans", 5},
	} {
		if got := regionRank(normalizeFamily(tc.family), prefs); got != tc.want {
			t.Errorf("%s: rank %d, want %d", tc.family, got, tc.want)
		}
	}
}

func TestSystemMatchEntry(t *testing.T) {
	s := synthetic("",
		entry("fam", StyleNormal, 300, "a"),
		entry("fam", StyleNormal, 400, "a"),
		entry("fam", StyleNormal, 700, "a"),
		entry("fam", StyleItalic, 400, "a"),
		entry("fam", StyleItalic, 700, "a"),
	)
	idxs := s.byFamily["fam"]
	for _, c := range []struct {
		q    Query
		want int
	}{
		{Query{}, 1},
		{Query{Weight: 700}, 2},
		{Query{Weight: 600}, 2},
		{Query{Weight: 200}, 0},
		{Query{Style: StyleItalic}, 3},
		{Query{Style: StyleItalic, Weight: 800}, 4},
		{Query{Style: StyleOblique, Weight: 700}, 4},
	} {
		if got := s.matchEntry(idxs, c.q); got != c.want {
			t.Errorf("%+v: entry %d, want %d", c.q, got, c.want)
		}
	}
}

func TestSystemCacheDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := cacheDirectory(filepath.Join(dir, "a", "b"))
	if err != nil || got != filepath.Join(dir, "a", "b") {
		t.Fatalf("new directory: %q, %v", got, err)
	}
	if st, err := os.Stat(got); err != nil || !st.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
	if entries, _ := os.ReadDir(got); len(entries) != 0 {
		t.Fatalf("the writability probe left %d files", len(entries))
	}

	// A path below a regular file cannot be a directory: the caller falls
	// back to scanning without a cache.
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := cacheDirectory(filepath.Join(file, "sub")); err == nil {
		t.Fatal("a directory below a file was accepted")
	}

	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CACHE_HOME", dir)
		got, err := cacheDirectory("")
		if want := filepath.Join(dir, "tela", "fonts"); err != nil || got != want {
			t.Fatalf("default directory %q, %v, want %q", got, err, want)
		}
	}
}

func TestSystemUnusableCacheScansWithoutIt(t *testing.T) {
	// With a cache directory below a file, systemFootprints must not call
	// fontscan (whose process-wide state it would poison) but scan the
	// default directories itself.
	dir := fontDir(t, map[string][]byte{"cjk.otf": cjkFontData(t)})
	defer func(f func(fontscan.Logger) ([]string, error)) { defaultFontDirectories = f }(defaultFontDirectories)
	defaultFontDirectories = func(fontscan.Logger) ([]string, error) { return []string{dir}, nil }

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, opts := range []SystemFontOptions{{CacheDir: filepath.Join(file, "sub")}, {NoCache: true}} {
		s := newSystemFonts(opts)
		fps, err := s.systemFootprints()
		if err != nil {
			t.Fatal(err)
		}
		if len(fps) != 1 || fps[0].Family != "notosanscjkjp" {
			t.Fatalf("options %+v: footprints %v", opts, fps)
		}
	}
}

func TestSystemDirectoryScan(t *testing.T) {
	dir := fontDir(t, map[string][]byte{"cjk.otf": cjkFontData(t)})
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "mono.TTF"), fonts.Data(fonts.JetBrainsMono), 0o644); err != nil {
		t.Fatal(err)
	}
	fps := scanDirectory(dir, nopLogger{})
	byFamily := map[string]fontscan.Footprint{}
	for _, fp := range fps {
		byFamily[fp.Family] = fp
	}
	cjk, mono := byFamily["notosanscjkjp"], byFamily["jetbrainsmono"]
	if len(fps) != 2 || cjk.Family == "" || mono.Family == "" {
		t.Fatalf("footprints %v", byFamily)
	}
	if !cjk.Runes.Contains('日') || !cjk.Runes.Contains('a') || cjk.Runes.Contains('א') {
		t.Error("CJK coverage wrong")
	}
	if !mono.Runes.Contains('a') || mono.Runes.Contains('日') {
		t.Error("mono coverage wrong")
	}
	if cjk.Aspect.Weight != 400 || mono.Location.File != filepath.Join(sub, "mono.TTF") {
		t.Errorf("aspect %+v location %+v", cjk.Aspect, mono.Location)
	}
}

// A malformed cmap can declare ranges far outside Unicode; the scan must
// clamp them instead of looping until the rune overflows.
func TestSystemMalformedRuneRanges(t *testing.T) {
	var rs fontscan.RuneSet
	addRuneRanges(&rs, [][2]rune{{-5, 1}, {0x10FFFE, 0x7FFFFFFF}, {0x7FFFFFF0, 0x7FFFFFFF}, {9, 3}})
	for _, r := range []rune{0, 1, 0x10FFFE, 0x10FFFF} {
		if !rs.Contains(r) {
			t.Errorf("rune %#x missing", r)
		}
	}
	for _, r := range []rune{2, 5, 0x110000, 0x7FFFFFFF} {
		if rs.Contains(r) {
			t.Errorf("rune %#x present", r)
		}
	}
}

// blockingLogger holds the system font scan at its last message until
// release is closed.
type blockingLogger struct {
	scanning chan struct{}
	release  chan struct{}
}

func (l *blockingLogger) Printf(format string, _ ...any) {
	if strings.HasPrefix(format, "indexed ") {
		close(l.scanning)
		<-l.release
	}
}

// A scan started by an unknown family name must not block the queries that
// need only registered fonts.
func TestSystemScanDoesNotBlockRegistered(t *testing.T) {
	c := interOnly(t)
	log := &blockingLogger{scanning: make(chan struct{}), release: make(chan struct{})}
	err := c.EnableSystemFonts(SystemFontOptions{Dirs: []string{t.TempDir()}, SkipSystemDirs: true, Logger: log})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Face(Query{Families: []string{"Missing", "Inter"}}, 16)
	}()
	<-log.scanning

	got := make(chan *Face)
	go func() { got <- c.Face(Query{Families: []string{"Inter"}, Weight: 700}, 16) }()
	go func() { c.Match("Inter", Query{}) }()
	select {
	case f := <-got:
		if f == nil || f.Typeface().Family() != "Inter" {
			t.Errorf("got %v, want Inter", f)
		}
	case <-time.After(5 * time.Second):
		t.Error("a query for a registered family waited for the system font scan")
	}
	close(log.release)
	<-done
}
