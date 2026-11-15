//go:build telagpu && (linux || windows)

package gpu

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/vk"
)

// benchScenes are the scenes of the benchmarks: every one is drawn whole.
var benchScenes = []string{"tailwind_shadows", "linear", "gpu_shadows", "gpu_rrects", "gpu_transforms"}

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
