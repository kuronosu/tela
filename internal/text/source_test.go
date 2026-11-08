package text

import (
	"bytes"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/assets/fonts/inter"
)

// The parse of a family's files is shared by the sources over the same
// slices (ADR-244: copies of a Font share one parse, whatever their roles).

// cloneFiles copies font files, so that sources over the copies share no
// parse with sources over the originals.
func cloneFiles(files [][]byte) [][]byte {
	out := make([][]byte, len(files))
	for i, f := range files {
		out[i] = bytes.Clone(f)
	}
	return out
}

// parseCounter counts, through the parse hook, the descriptions and parses
// of the given files.
type parseCounter struct {
	mu        sync.Mutex
	files     [][]byte
	described map[int]int    // by file
	parsed    map[[2]int]int // by file and font
}

func countParses(t *testing.T, files [][]byte) *parseCounter {
	pc := &parseCounter{files: files, described: map[int]int{}, parsed: map[[2]int]int{}}
	t.Cleanup(SetParseHook(func(_ string, data []byte, font int) {
		pc.mu.Lock()
		defer pc.mu.Unlock()
		for i, f := range pc.files {
			if len(f) == len(data) && unsafe.SliceData(f) == unsafe.SliceData(data) {
				if font < 0 {
					pc.described[i]++
				} else {
					pc.parsed[[2]int{i, font}]++
				}
			}
		}
	}))
	return pc
}

// check fails t unless every file was described once and wantParsed fonts
// were parsed, each once.
func (pc *parseCounter) check(t *testing.T, step string, wantParsed int) {
	t.Helper()
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for i := range pc.files {
		if n := pc.described[i]; n != 1 {
			t.Errorf("%s: file %d described %d times, want once", step, i, n)
		}
	}
	for k, n := range pc.parsed {
		if n != 1 {
			t.Errorf("%s: font %d of file %d parsed %d times, want once", step, k[1], k[0], n)
		}
	}
	if len(pc.parsed) != wantParsed {
		t.Errorf("%s: %d fonts parsed, want %d", step, len(pc.parsed), wantParsed)
	}
}

func typefacesOf(t *testing.T, s *FontSource) []*Typeface {
	t.Helper()
	tfs, err := s.Typefaces()
	if err != nil {
		t.Fatalf("%s: %v", s.Family, err)
	}
	return tfs
}

// TestSourcesShareTheirFiles: sources that give one family different roles
// over the same slices, in Data or from Load, describe and parse the files
// once and give the same typefaces; equal copies of the bytes and another
// family name over the same bytes are other files.
func TestSourcesShareTheirFiles(t *testing.T) {
	files := cloneFiles([][]byte{inter.Upright, inter.Italic})
	pc := countParses(t, files)
	sans := &FontSource{Family: "Shared Files", Data: files, Generics: []string{"sans-serif"}}
	latin := &FontSource{Family: "Shared Files", Data: files, Scripts: []language.Script{language.Latin}}
	loaded := &FontSource{Family: "Shared Files", Load: func() ([][]byte, error) { return files, nil }, Generics: []string{"emoji"}}

	// Text needs the upright font through one role, then through another.
	c := collectionOf(t, sans, latin, loaded)
	if f := c.Face(Query{Families: []string{"sans-serif"}}, 16); f == nil || f.Typeface().Family() != "Shared Files" {
		t.Fatalf("sans-serif: %v", f)
	}
	c2 := collectionOf(t, latin)
	if f := c2.Face(Query{Families: []string{"Shared Files"}}, 16); f == nil {
		t.Fatal("no face through the Latin source")
	}
	pc.check(t, "upright through two roles", 1)
	if n := latin.parsedFonts(); n != 1 {
		t.Errorf("the Latin source sees %d parsed fonts, want the upright one the sans-serif source parsed", n)
	}

	a, b, l := typefacesOf(t, sans), typefacesOf(t, latin), typefacesOf(t, loaded)
	if len(a) != 2 {
		t.Fatalf("%d typefaces, want 2", len(a))
	}
	for i := range a {
		if a[i] != b[i] || a[i] != l[i] {
			t.Errorf("typeface %d differs between the roles: the files were parsed again", i)
		}
	}
	pc.check(t, "every font through three sources", 2)

	// The faces too: a collection of the three resolves the family to
	// the same face as a collection of one.
	q := Query{Families: []string{"Shared Files"}, Style: StyleItalic}
	if f, g := c.Face(q, 16), collectionOf(t, sans).Face(q, 16); f == nil || f != g {
		t.Errorf("italic faces %v and %v, want the same face", f, g)
	}

	// Equal bytes in other slices, and another family over the same
	// slices, are other files: their typefaces are their own.
	copied := &FontSource{Family: "Shared Files", Data: cloneFiles(files)}
	if typefacesOf(t, copied)[0] == a[0] {
		t.Error("a copy of the bytes shares the parse of the originals")
	}
	renamed := &FontSource{Family: "Renamed", Data: files}
	if tf := typefacesOf(t, renamed)[0]; tf == a[0] || tf.Family() != "Renamed" {
		t.Errorf("another family over the same bytes: %v (%q)", tf, tf.Family())
	}
}

// TestSharedFilesErrors: the errors of files are those of every source
// over them, and they are logged once.
func TestSharedFilesErrors(t *testing.T) {
	var log logRecorder
	junk := [][]byte{[]byte("not a font file")}
	a := &FontSource{Family: "Junk", Data: junk, Generics: []string{"serif"}}
	b := &FontSource{Family: "Junk", Data: junk, Generics: []string{"monospace"}}
	c := collectionOf(t, a, b)
	c.SetLogger(&log)
	for _, g := range []string{"serif", "monospace", "Junk"} {
		if f := c.Face(Query{Families: []string{g}}, 16); f != nil && f.Typeface().Family() == "Junk" {
			t.Errorf("%s: a face of junk", g)
		}
	}
	if a.Err() == nil || b.Err() == nil {
		t.Errorf("errors %v, %v: want the file's error on both sources", a.Err(), b.Err())
	}
	if len(log.msgs) != 1 {
		t.Errorf("logged %d times, want once:\n%s", len(log.msgs), log.all())
	}
}

// TestSharedFilesAreReleased: the files of sources nothing uses any more
// leave the registry, so an app's font bytes are not kept for the life of
// the process.
func TestSharedFilesAreReleased(t *testing.T) {
	entries := func() int {
		filesReg.mu.Lock()
		defer filesReg.mu.Unlock()
		n := 0
		for k := range filesReg.m {
			if k.family == "Released Files" {
				n++
			}
		}
		return n
	}
	func() {
		s := &FontSource{Family: "Released Files", Data: cloneFiles([][]byte{inter.Upright})}
		if len(s.fontsOf(nil)) != 1 {
			t.Fatal("not described")
		}
	}()
	if entries() != 1 {
		t.Fatalf("%d registry entries, want 1", entries())
	}
	deadline := time.Now().Add(2 * time.Second)
	for entries() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the files of a dropped source are still registered")
		}
		runtime.GC()
		time.Sleep(time.Millisecond) // the cleanups run on their own goroutine
	}
}
