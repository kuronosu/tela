package text

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// TestSystemFontsLive uses the fonts installed on this machine. It is
// opt-in because its result depends on them:
//
//	TELA_SYSTEM_FONTS=1 go test ./internal/text -run TestSystemFontsLive -v
//
// TELA_SYSTEM_FONTS_CACHE names the directory of the font index (default: a
// temporary one, so the run measures a cold scan; run twice with the same
// directory to measure a warm one) and TELA_SYSTEM_FONTS_PNG the file the
// rendered paragraph is written to (default: a temporary file).
func TestSystemFontsLive(t *testing.T) {
	if os.Getenv("TELA_SYSTEM_FONTS") != "1" {
		t.Skip("set TELA_SYSTEM_FONTS=1 to run against the installed fonts")
	}
	cache := os.Getenv("TELA_SYSTEM_FONTS_CACHE")
	if cache == "" {
		cache = t.TempDir()
	}
	c := NewDefaultCollection()
	if err := c.EnableSystemFonts(SystemFontOptions{CacheDir: cache, Logger: testLogger{t}}); err != nil {
		t.Fatal(err)
	}

	// The embedded families keep drawing what they cover.
	start := time.Now()
	itemize(c, "Hello مرحبا 😀", "Inter")
	t.Logf("embedded-only text: %v (no scan expected)", time.Since(start))
	if c.sys.indexed.Load() {
		t.Error("the index was loaded for text the embedded fonts cover")
	}

	// First use: loads the index and one font.
	start = time.Now()
	got := itemize(c, "日本語", "Inter")
	t.Logf("first uncovered cluster (index + one font): %v -> %v", time.Since(start), got)
	if err := c.SystemFontsErr(); err != nil {
		t.Fatal(err)
	}
	t.Logf("index: %d fonts, %d families, scanned in %v (cache %s)", len(c.sys.entries), len(c.sys.byFamily), c.sys.scanTime, cache)

	// The scan without fontscan's cache (NoCache, or an unusable cache
	// directory) is this package's own scan of the same directories.
	nc := NewDefaultCollection()
	if err := nc.EnableSystemFonts(SystemFontOptions{NoCache: true}); err != nil {
		t.Fatal(err)
	}
	nc.sys.ensureIndex()
	t.Logf("uncached scan: %d fonts in %v", len(nc.sys.entries), nc.sys.scanTime)

	scripts := []struct{ name, text string }{
		{"Hebrew", "שלום עולם"},
		{"Devanagari", "नमस्ते दुनिया"},
		{"Thai", "สวัสดีชาวโลก"},
		{"Japanese", "日本語の文章、こんにちは"},
		{"Simplified Chinese", "中文简体字们说"},
		{"Korean", "안녕하세요 세계"},
		{"Greek", "Γειά σου Κόσμε"},
	}
	for _, sc := range scripts {
		start = time.Now()
		runs := itemize(c, sc.text, "Inter")
		t.Logf("%-18s %-26s %v (%v)", sc.name, sc.text, runs, time.Since(start))
		for _, r := range runs {
			if len(r) == 0 {
				t.Errorf("%s: empty run", sc.name)
			}
		}
	}
	for _, name := range []string{"DejaVu Sans", "Noto Sans CJK JP", "Noto Sans Hebrew", "Segoe UI", "Helvetica Neue"} {
		start = time.Now()
		tf := c.Match(name, Query{Weight: 700})
		if tf != nil {
			t.Logf("named %-18s -> %s (%v)", name, tf.Name(), time.Since(start))
		} else {
			t.Logf("named %-18s -> not installed (%v)", name, time.Since(start))
		}
	}

	// Render a mixed paragraph.
	const W = 640
	ink := gfx.RGB8(0x11, 0x18, 0x27)
	muted := gfx.RGB8(0x6b, 0x72, 0x80)
	b := gfx.NewBuilder()
	y := float32(16)
	add := func(label, text string, size float32) {
		lab := NewLayout(c, &Paragraph{Text: label, Default: TextStyle{Families: []string{"Inter"}, Size: 11, Paint: muted, LineHeight: LineHeightPixels(16)}})
		lab.BreakLines(W)
		lab.Paint(b, gfx.Pt(16, y))
		y += lab.Height()
		l := NewLayout(c, &Paragraph{Text: text, Default: TextStyle{Families: []string{"Inter"}, Size: size, Paint: ink, LineHeight: LineHeightPixels(size * 1.5)}})
		l.BreakLines(W)
		l.Paint(b, gfx.Pt(16, y))
		y += l.Height() + 10
	}
	add("Latin, Arabic and emoji (embedded fonts)", "Tela draws text: مرحبا بكم 😀🎉👨‍👩‍👧‍👦 and numbers 0123456789.", 18)
	add("Hebrew (system, right to left)", "שלום עולם, זהו משפט בעברית עם Latin באמצע ו-123 מספרים.", 18)
	add("Devanagari (system)", "नमस्ते दुनिया, यह देवनागरी में एक वाक्य है जिसमें संयुक्ताक्षर हैं: क्ष त्र ज्ञ।", 18)
	add("Thai (system, no spaces between words)", "สวัสดีชาวโลก นี่คือประโยคภาษาไทยที่ยาวพอที่จะตัดบรรทัดได้ในหลายตำแหน่ง", 18)
	add("Japanese and Simplified Chinese (system CJK)", "日本語の文章、こんにちは世界。 中文简体字们说这是一个句子。 Mixed with Latin text.", 18)
	add("Korean and Greek (system)", "안녕하세요 세계, 한국어 문장입니다. Γειά σου Κόσμε, ελληνικό κείμενο.", 18)
	add("One paragraph with every script", "Latin, العربية, עברית, हिन्दी, ไทย, 日本語, 中文, 한국어, Ελληνικά, Кириллица 😀", 18)
	H := int(y + 8)

	const scale = 2
	img := image.NewRGBA(image.Rect(0, 0, (W+32)*scale, H*scale))
	raster.Clear(img, image.Rectangle{}, gfx.RGB8(0xf9, 0xfa, 0xfb))
	raster.Render(img, b.Finish(), raster.Options{Scale: scale})
	out := os.Getenv("TELA_SYSTEM_FONTS_PNG")
	if out == "" {
		out = filepath.Join(t.TempDir(), "system_fonts.png")
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", out)
}

type testLogger struct{ t testing.TB }

func (l testLogger) Printf(format string, args ...any) { l.t.Logf("fontscan: "+format, args...) }
