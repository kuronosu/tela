//go:build telagpu && (linux || windows)

package gpu

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/vk"
)

// benchScenes are the scenes of the benchmarks: every one is drawn whole.
var benchScenes = []string{"tailwind_shadows", "linear", "strokes", "gpu_shadows", "gpu_rrects", "gpu_transforms",
	"gpu_text", "gpu_paths", "gpu_images", "gpu_clips_nested"}

// benchText lays out the page of raster's text benchmarks: 2,000 glyphs
// of 14 px text in 20 lines of 100 glyphs on a 1920x1080 page, at
// fractional origins so that every subpixel offset occurs; or more lines
// of it. With sizes above 1 the lines cycle through that many sizes from
// 11 px up, half a pixel apart, so the page has that many times more
// distinct glyphs (the test font has five letters).
func benchText(lines, sizes int) *gfx.DisplayList {
	b := gfx.NewBuilder()
	const line = "olAS iolS loo Ail SoAl iS lo AoS lil oSA Sol iAo lSi oAl S" // 59 glyphs
	for i := 0; i < lines; i++ {
		s := line + line
		size := float32(14)
		if sizes > 1 {
			size = 11 + float32(i%sizes)*0.5
		}
		b.DrawGlyphRun(font.Run(s[:100], size, 40.37+float32(i%20)*3.1+float32(i/20)*0.13, 60+float32(i%20)*48.6+float32(i/20)*2.3, slate))
	}
	return b.Finish()
}

// textPages are the pages of the text benchmarks.
var textPages = []struct {
	name         string
	lines, sizes int
}{{"2000_glyphs", 20, 1}, {"10000_glyphs", 100, 1}, {"10000_glyphs_25_sizes", 100, 25}}

// BenchmarkText measures frames of text at 1920x1080 on every device,
// without reading back: 2,000 and 10,000 glyphs, drawn from atlases that
// have them (steady: nothing is uploaded) and from empty ones (uploads:
// every mask is copied from the glyph cache into the frame and uploaded;
// the cache is warm, as after a Trim). BenchmarkTextRaster is the
// rasterizer on the same pages, from a warm cache, on one goroutine and on
// all.
func BenchmarkText(b *testing.B) {
	if err := vk.Load(); err != nil {
		b.Skipf("Vulkan is not available: %v", err)
	}
	inst, err := vk.NewInstance(vk.InstanceConfig{AppName: "tela gpu benchmarks"})
	if err != nil {
		b.Skipf("no Vulkan instance: %v", err)
	}
	defer inst.Destroy()
	devices, err := inst.Devices()
	if err != nil {
		b.Fatal(err)
	}
	for _, info := range devices {
		dev, err := OpenVulkan(inst, info)
		if err != nil {
			b.Fatal(err)
		}
		target, err := dev.NewTarget(1920, 1080)
		if err != nil {
			b.Fatal(err)
		}
		for _, page := range textPages {
			dl := benchText(page.lines, page.sizes)
			for _, cold := range []bool{false, true} {
				name := deviceLabel(info.Name) + "/" + page.name + "/steady"
				if cold {
					name = deviceLabel(info.Name) + "/" + page.name + "/uploads"
				}
				b.Run(name, func(b *testing.B) {
					var r Renderer
					r.enc.GlyphCache = raster.NewGlyphCache(raster.DefaultGlyphCacheBytes)
					res, err := r.Render(target, dl, Options{Clear: &gfx.White})
					if err != nil || res.Unsupported != nil {
						b.Fatal(err, res.Unsupported)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if cold {
							r.enc.Trim()
						}
						if res, err = r.Render(target, dl, Options{Clear: &gfx.White}); err != nil {
							b.Fatal(err)
						}
					}
					b.ReportMetric(float64(res.UploadBytes), "upload-bytes/op")
					b.ReportMetric(float64(res.Uploads), "uploads/op")
				})
			}
		}
		target.Close()
		dev.Close()
	}
}

func BenchmarkTextRaster(b *testing.B) {
	for _, page := range textPages {
		dl := benchText(page.lines, page.sizes)
		img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
		for _, workers := range []int{1, 0} {
			b.Run(fmt.Sprintf("%s/workers_%d", page.name, workers), func(b *testing.B) {
				r := raster.NewRenderer()
				opts := raster.Options{Clear: &gfx.White, Workers: workers, GlyphCache: raster.NewGlyphCache(raster.DefaultGlyphCacheBytes)}
				r.Render(img, dl, opts)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					r.Render(img, dl, opts)
				}
			})
		}
	}
}

// BenchmarkEncodeText measures the encoder alone on the pages of text,
// steady and with uploads.
func BenchmarkEncodeText(b *testing.B) {
	for _, page := range textPages {
		dl := benchText(page.lines, page.sizes)
		for _, cold := range []bool{false, true} {
			name := page.name + "/steady"
			if cold {
				name = page.name + "/uploads"
			}
			b.Run(name, func(b *testing.B) {
				e := Encoder{GlyphCache: raster.NewGlyphCache(raster.DefaultGlyphCacheBytes)}
				e.Encode(dl, image.Pt(1920, 1080), Options{Clear: &gfx.White})
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if cold {
						e.Trim()
					}
					e.Encode(dl, image.Pt(1920, 1080), Options{Clear: &gfx.White})
				}
			})
		}
	}
}

// BenchmarkRender measures a frame at scale 2 on every device: encoding,
// drawing and waiting for the GPU, without reading back. BenchmarkRaster
// is the rasterizer on the same frames.
func BenchmarkRender(b *testing.B) {
	if err := vk.Load(); err != nil {
		b.Skipf("Vulkan is not available: %v", err)
	}
	inst, err := vk.NewInstance(vk.InstanceConfig{AppName: "tela gpu benchmarks"})
	if err != nil {
		b.Skipf("no Vulkan instance: %v", err)
	}
	defer inst.Destroy()
	devices, err := inst.Devices()
	if err != nil {
		b.Fatal(err)
	}
	for _, info := range devices {
		dev, err := OpenVulkan(inst, info)
		if err != nil {
			b.Fatal(err)
		}
		for _, s := range allScenes() {
			if !benchScene(s.Name) {
				continue
			}
			dl := s.displayList()
			w, h := sceneSize(&s, 2)
			target, err := dev.NewTarget(w, h)
			if err != nil {
				b.Fatal(err)
			}
			b.Run(deviceLabel(info.Name)+"/"+s.Name, func(b *testing.B) {
				var r Renderer
				for i := 0; i < b.N; i++ {
					if _, err := r.Render(target, dl, Options{Scale: 2, Clear: &s.clear}); err != nil {
						b.Fatal(err)
					}
				}
			})
			target.Close()
		}
		dev.Close()
	}
}

func BenchmarkRaster(b *testing.B) {
	for _, s := range allScenes() {
		if !benchScene(s.Name) {
			continue
		}
		dl := s.displayList()
		w, h := sceneSize(&s, 2)
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		b.Run(s.Name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				raster.Render(img, dl, raster.Options{Scale: 2, Clear: &s.clear})
			}
		})
	}
}

// BenchmarkEncode measures the encoder alone.
func BenchmarkEncode(b *testing.B) {
	for _, s := range allScenes() {
		if !benchScene(s.Name) {
			continue
		}
		dl := s.displayList()
		w, h := sceneSize(&s, 2)
		b.Run(s.Name, func(b *testing.B) {
			var e Encoder
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				e.Encode(dl, image.Pt(w, h), Options{Scale: 2, Clear: &gfx.White})
			}
		})
	}
}

func benchScene(name string) bool {
	for _, n := range benchScenes {
		if n == name {
			return true
		}
	}
	return false
}
