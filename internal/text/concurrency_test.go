package text

import (
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/assets/fonts"
	"github.com/kuronosu/tela/internal/gfx"
)

// glyphSummary is everything a Face returns for one glyph, digested.
func glyphSummary(f *Face, g gfx.GlyphID) uint64 {
	d := newDigest()
	p, ok := f.GlyphOutline(g)
	if ok {
		d.path(p)
	}
	b := f.GlyphBounds(g)
	d.f32(f.Advance(g), b.X0, b.Y0, b.X1, b.Y1)
	if l := f.ColorGlyph(g, gfx.Black); l != nil {
		d.list(l)
	}
	return d.h.Sum64()
}

// TestConcurrentFaces hammers cold faces from many goroutines (the raster
// workers' access pattern) and checks every result against a sequential
// reference computed on a separate copy of the font. Without -race (it
// needs cgo), this checks that concurrent first uses agree and do not
// crash.
func TestConcurrentFaces(t *testing.T) {
	type subject struct {
		font fonts.Font
		inst Instance
		max  int // glyphs to check
	}
	subjects := []subject{
		{fonts.InterVariable, Instance{Weight: 550, OpticalSize: 22}, 1200},
		{fonts.NotoColorEmoji, Instance{}, 600},
	}
	if testing.Short() {
		for i := range subjects {
			subjects[i].max /= 4
		}
	}
	for _, s := range subjects {
		fresh := func() *Face {
			tfs, err := ParseTypefaces(fonts.Data(s.font), "Test")
			if err != nil {
				t.Fatal(err)
			}
			return tfs[0].Face(s.inst)
		}
		ref := fresh()
		n := min(ref.tf.nGlyphs, s.max)
		want := make([]uint64, n)
		for g := range want {
			want[g] = glyphSummary(ref, gfx.GlyphID(g))
		}

		f := fresh()
		const workers = 12
		var wg sync.WaitGroup
		errs := make(chan string, workers)
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(seed uint64) {
				defer wg.Done()
				if got := f.tf.Face(s.inst); got != f {
					errs <- "Typeface.Face returned a second instance"
					return
				}
				for _, g := range rand.New(rand.NewPCG(seed, seed)).Perm(n) {
					if got := glyphSummary(f, gfx.GlyphID(g)); got != want[g] {
						errs <- fonts.Name(s.font) + ": a glyph differs from the sequential reference"
						return
					}
				}
			}(uint64(w))
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			t.Error(e)
		}
	}
}

// TestConcurrentCollection resolves faces and itemizes text from many
// goroutines while fonts are registered.
func TestConcurrentCollection(t *testing.T) {
	c := NewDefaultCollection()
	text := []rune("Hello مرحبا 😀 👩‍👩‍👧 ❤️ code")
	want := c.Itemize(text, 0, len(text), Query{Families: []string{"sans-serif"}}, 16, language.Latin, nil)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				got := c.Itemize(text, 0, len(text), Query{Families: []string{"sans-serif"}}, 16, language.Latin, nil)
				if len(got) != len(want) {
					t.Errorf("worker %d: %d runs, want %d", w, len(got), len(want))
					return
				}
				for k := range got {
					if got[k] != want[k] {
						t.Errorf("worker %d: run %d = %+v, want %+v", w, k, got[k], want[k])
						return
					}
				}
				c.Face(Query{Families: []string{"monospace"}, Weight: float32(100 + 100*(i%9))}, float32(10+i%30))
			}
		}(w)
	}
	// Registering (another family) concurrently clears the query cache.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			if _, err := c.Register(fonts.Data(fonts.JetBrainsMonoItalic), "Extra"); err != nil {
				t.Error(err)
			}
		}
	}()
	wg.Wait()
}
