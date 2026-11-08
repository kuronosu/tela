package text

import (
	"image"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// TestSystemEmojiHeapLive measures the Go heap that drawing a few emoji
// with a bitmap emoji font found as a system font costs: an installed one
// (TELA_EMOJI_FONT, default Noto Color Emoji where Arch, Fedora and Debian
// install it, or Apple Color Emoji) and a synthetic sbix font of about
// 100 MB, the size of Apple Color Emoji. It is opt-in, because it needs
// the installed font and writes the synthetic one:
//
//	TELA_SYSTEM_FONTS=1 go test ./internal/text -run TestSystemEmojiHeapLive -v
func TestSystemEmojiHeapLive(t *testing.T) {
	if os.Getenv("TELA_SYSTEM_FONTS") != "1" {
		t.Skip("set TELA_SYSTEM_FONTS=1 to run against an installed emoji font")
	}
	t.Run("installed", func(t *testing.T) {
		path := os.Getenv("TELA_EMOJI_FONT")
		if path == "" {
			for _, p := range systemBitmapFonts {
				if _, err := os.Stat(p); err == nil {
					path = p
					break
				}
			}
		}
		if path == "" {
			t.Skip("no emoji font found; set TELA_EMOJI_FONT")
		}
		// The font alone in a directory, so the index holds nothing else.
		dir := t.TempDir()
		copyFile(t, path, filepath.Join(dir, filepath.Base(path)))
		measureEmojiHeap(t, dir, "😀🎉👍❤️🚀🇯🇵👨‍👩‍👧‍👦")
	})
	t.Run("synthetic sbix", func(t *testing.T) {
		dir := t.TempDir()
		data := largeSbixFont(2000, []uint16{40, 64, 96, 160}, 12<<10)
		if err := os.WriteFile(filepath.Join(dir, "LargeSbix.ttf"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("synthetic sbix font: %.1f MiB", mib(uint64(len(data))))
		measureEmojiHeap(t, dir, "\U0001F601\U0001F602\U0001F603\U0001F604\U0001F605")
	})
}

func measureEmojiHeap(t *testing.T, dir, text string) {
	c, err := NewSystemCollection(SystemFontOptions{Dirs: []string{dir}, SkipSystemDirs: true, NoCache: true})
	if err != nil {
		t.Fatal(err)
	}
	c.Preload()
	before := heapInUse()

	l := NewLayout(c, &Paragraph{Text: text, Default: TextStyle{Families: []string{"emoji"}, Size: 32, Paint: gfx.RGB8(0, 0, 0)}})
	l.BreakLines(600)
	b := gfx.NewBuilder()
	l.Paint(b, gfx.Pt(0, 0))
	img := image.NewRGBA(image.Rect(0, 0, 1200, 160))
	raster.Render(img, b.Finish(), raster.Options{Scale: 2})
	var runs []LineRun
	for i := range l.NumLines() {
		for _, r := range l.LineRuns(i, runs[:0]) {
			if r.Face != nil {
				t.Logf("drawn with %s", r.Face.Typeface().Name())
			}
		}
	}
	after := heapInUse()
	n, cached := bitmapImages.len()
	t.Logf("heap in use %.1f MiB before the emoji, %.1f MiB after (+%.1f MiB; %d decoded images in the cache, %.1f MiB)",
		mib(before), mib(after), mib(after-before), n, mib(uint64(cached)))
	runtime.KeepAlive(c)
	runtime.KeepAlive(l)
	// Nothing draws with c any more: let the directory go (Windows does
	// not delete a mapped file).
	forgetMappings(t, dir)
}

// largeSbixFont writes an sbix font of n glyphs (U+1F600 onwards from
// glyph 1) with strikes of the given sizes, each glyph's image a small PNG
// padded after its end to pad bytes.
func largeSbixFont(n int, ppems []uint16, pad int) []byte {
	base := buildSbixTestFont()
	glyphs := make([][]byte, n)
	var sbixStrikes []sbixTestStrike
	for _, ppem := range ppems {
		p := pngOf(square2(int(ppem), int(ppem)))
		for g := 1; g < n; g++ {
			rec := append(be(uint16(0), uint16(0)), "png "...)
			rec = append(rec, p...)
			rec = append(rec, make([]byte, max(0, pad-len(rec)))...)
			glyphs[g] = rec
		}
		sbixStrikes = append(sbixStrikes, sbixTestStrike{ppem: ppem, glyphs: append([][]byte(nil), glyphs...)})
	}
	hhea := append([]byte(nil), tableOf(base, "hhea")...)
	hhea[34], hhea[35] = byte(n>>8), byte(n)
	var hmtx []byte
	cmap := map[rune]uint16{}
	for g := range n {
		hmtx = append(hmtx, be(uint16(1000), int16(0))...)
		if g > 0 {
			cmap[0x1F600+rune(g)-1] = uint16(g)
		}
	}
	return writeSFNT(map[string][]byte{
		"head": tableOf(base, "head"),
		"hhea": hhea,
		"hmtx": hmtx,
		"maxp": be(uint32(0x00005000), uint16(n)),
		"OS/2": tableOf(base, "OS/2"),
		"post": tableOf(base, "post"),
		"cmap": cmapTable(cmap),
		"name": bitmapNameTable("Large Sbix", "Written for Tela's tests."),
		"sbix": sbixTableN(n, 0, sbixStrikes),
	})
}

func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapInuse
}

func mib(n uint64) float64 { return float64(n) / (1 << 20) }

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
}
