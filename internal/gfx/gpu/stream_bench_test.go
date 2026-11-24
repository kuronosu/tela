//go:build telagpu && (linux || windows)

package gpu

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// BenchmarkStream measures a window frame that shows a new 1280x720 frame
// of CPU pixels (an external texture without the GPU path, a video): a new
// image every time, uploaded and drawn through a rounded clip. The images
// are ordinary ones, or frames of a stream (gfx.NewStreamImage).
func BenchmarkStream(b *testing.B) {
	const w, h = 1280, 720
	benchDevices(b, func(b *testing.B, label string, dev Device) {
		for _, stream := range []uint64{0, 1} {
			name := label + "/images"
			if stream != 0 {
				name = label + "/stream"
			}
			b.Run(name, func(b *testing.B) {
				target, err := dev.NewTarget(1920, 1080)
				if err != nil {
					b.Fatal(err)
				}
				defer target.Close()
				var r Renderer
				pix := make([]byte, 4*w*h)
				for i := range pix {
					pix[i] = uint8(i * 7)
				}
				frame := func() {
					img := gfx.NewStreamImage(w, h, pix, true, stream)
					dl := gfxtest.Build(func(b *gfx.Builder) {
						b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(100, 100, w, h), 24))
						b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(100, 100, w, h), gfx.SampleLinear)
						b.PopClip()
					})
					// A window frame of two damaged rectangles: the texture, and
					// something that uploads nothing.
					for _, rc := range []image.Rectangle{image.Rect(100, 100, 100+w, 100+h), image.Rect(0, 0, 50, 50)} {
						if _, err := r.Render(target, dl, Options{Region: rc, Clear: &gfx.White}); err != nil {
							b.Fatal(err)
						}
					}
					r.EndFrame()
				}
				for range 10 {
					frame()
				}
				made := dev.(*vkDevice).made
				b.SetBytes(4 * w * h)
				b.ResetTimer()
				peak := int64(0)
				for b.Loop() {
					frame()
					peak = max(peak, r.Memory().Images)
				}
				b.ReportMetric(float64(dev.(*vkDevice).made-made)/float64(b.N), "images/op")
				b.ReportMetric(float64(peak)/(1<<20), "peak-MiB")
			})
		}
	})
}
