//go:build telagpu && (linux || windows)

package gpu

import (
	"image"
	"testing"
)

// TestPlain checks that the plain pipeline draws the pixels the quad
// pipeline draws for the instances it takes (the insides of solid fills,
// of layers and of filtered backdrops), bit for bit, on every scene.
func TestPlain(t *testing.T) {
	scenes := allScenes()
	eachDevice(t, func(t *testing.T, dev Device) {
		plains, backdrops := 0, 0
		for i := range scenes {
			s := &scenes[i]
			dl := s.displayList()
			for _, scale := range []float32{1, 1.5} {
				w, h := sceneSize(s, scale)
				var imgs [2]*image.RGBA
				for k, off := range []bool{true, false} {
					target, err := dev.NewTarget(w, h)
					if err != nil {
						t.Fatal(err)
					}
					var r Renderer
					r.enc.noPlain = off
					// With backdrop filters, the second render paints them from
					// the cache, opaque ones without blending.
					var bc *BackdropCache
					renders := 1
					if dl.NumBackdrops() > 0 {
						bc, renders = new(BackdropCache), 2
					}
					for n := range renders {
						if n > 0 {
							same := make([]int, dl.NumBackdrops())
							for j := range same {
								same[j] = j
							}
							bc.Retain(dl, scale, same, nil)
						}
						if _, err := r.Render(target, dl, Options{Scale: scale, Clear: &s.clear, Backdrops: bc}); err != nil {
							t.Fatal(err)
						}
					}
					if !off {
						for _, p := range r.enc.frame.Passes {
							for _, b := range p.Batches {
								switch h := head(&r.enc.frame, b.First); {
								case b.Pipeline == PipelinePlain:
									plains++
								case b.Pipeline == PipelineOpaque && h[0]&FlagLayer != 0 && h[1]&(1<<25) == 0:
									backdrops++ // an opaque backdrop from the cache
								}
							}
						}
					}
					imgs[k] = readAll(t, target)
					target.Close()
					r.Trim()
				}
				if m := compare(imgs[0], imgs[1]); m.Max != 0 {
					t.Errorf("%s, scale %g: the plain pipeline differs from the quad's in %d pixels, the first at %v", s.Name, scale, m.Over0, m.Worst)
				}
			}
		}
		if plains == 0 || backdrops == 0 {
			t.Errorf("%d batches of the plain pipeline, %d opaque backdrops painted without blending", plains, backdrops)
		}
	})
}
