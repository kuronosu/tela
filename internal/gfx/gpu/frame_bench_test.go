//go:build telagpu && (linux || windows)

package gpu

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/vk"
)

// benchDevices runs f for every physical device, with a device that is
// closed afterwards. It skips without Vulkan.
func benchDevices(b *testing.B, f func(b *testing.B, label string, dev Device)) {
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
		f(b, deviceLabel(info.Name), dev)
		dev.Close()
	}
}

// BenchmarkReadback measures Target.ReadPixels: the copy of a rectangle
// from the device and into an image, which is what a window presented
// through its framebuffer pays for every damaged pixel (ADR-281). The
// sizes are a hover's damage, a pane's, and full frames at 1440p and 4K.
func BenchmarkReadback(b *testing.B) {
	benchDevices(b, func(b *testing.B, label string, dev Device) {
		target, err := dev.NewTarget(3840, 2160)
		if err != nil {
			b.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		if _, err := r.Render(target, nil, Options{Clear: &gfx.White}); err != nil {
			b.Fatal(err)
		}
		dst := image.NewRGBA(image.Rect(0, 0, 3840, 2160))
		for _, size := range []image.Point{{64, 64}, {400, 300}, {1280, 720}, {2560, 1440}, {3840, 2160}} {
			rc := image.Rectangle{Max: size}
			b.Run(fmt.Sprintf("%s/%dx%d", label, size.X, size.Y), func(b *testing.B) {
				b.SetBytes(int64(4 * size.X * size.Y))
				for b.Loop() {
					if err := target.ReadPixels(dst, rc); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	})
}

// BenchmarkSubmit measures what a Render that draws one rectangle into a
// small region costs: mostly the submission and the wait for it, which
// every Render, every ReadPixels and every Shift pays once.
func BenchmarkSubmit(b *testing.B) {
	dl := func() *gfx.DisplayList {
		bl := gfx.NewBuilder()
		bl.FillRect(gfx.RectXYWH(10, 10, 100, 100), sky)
		return bl.Finish()
	}()
	benchDevices(b, func(b *testing.B, label string, dev Device) {
		target, err := dev.NewTarget(2560, 1440)
		if err != nil {
			b.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := r.Render(target, dl, Options{Region: image.Rect(0, 0, 128, 128), Clear: &gfx.White}); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

// BenchmarkFill measures what a pixel costs the quad pipeline: full frames
// at 3840x2160 that fill the target ten times over with opaque rectangles,
// with rounded rectangles, and with one shadow, without reading back. The
// frame's time over the megapixels it shades (83 for the fills) is the
// device's rate for the shader's simplest instances.
func BenchmarkFill(b *testing.B) {
	const w, h = 3840, 2160
	scene := func(draw func(bl *gfx.Builder, i int)) *gfx.DisplayList {
		bl := gfx.NewBuilder()
		for i := 0; i < 10; i++ {
			draw(bl, i)
		}
		return bl.Finish()
	}
	scenes := []struct {
		name string
		dl   *gfx.DisplayList
	}{
		{"rects_x10", scene(func(bl *gfx.Builder, i int) {
			bl.FillRect(gfx.RectXYWH(0, 0, w, h), gfx.Color{R: float32(i) / 10, G: 0.4, B: 0.6, A: 1})
		})},
		{"translucent_rects_x10", scene(func(bl *gfx.Builder, i int) {
			bl.FillRect(gfx.RectXYWH(0, 0, w, h), gfx.Color{R: float32(i) / 10, G: 0.4, B: 0.6, A: 0.5})
		})},
		{"rrects_x10", scene(func(bl *gfx.Builder, i int) {
			bl.FillRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, w, h), 40), gfx.Color{R: float32(i) / 10, G: 0.4, B: 0.6, A: 1})
		})},
		{"shadow_x1", scene(func(bl *gfx.Builder, i int) {
			if i == 0 {
				shadow(bl, gfx.NewRRect(gfx.RectXYWH(200, 200, w-400, h-400), 40), 0, 20, 60, 0, gfx.Black.WithAlpha(0.5), false)
			}
		})},
	}
	benchDevices(b, func(b *testing.B, label string, dev Device) {
		target, err := dev.NewTarget(w, h)
		if err != nil {
			b.Fatal(err)
		}
		defer target.Close()
		for _, s := range scenes {
			var r Renderer
			b.Run(label+"/"+s.name, func(b *testing.B) {
				for b.Loop() {
					if _, err := r.Render(target, s.dl, Options{Clear: &gfx.White}); err != nil {
						b.Fatal(err)
					}
				}
			})
			r.Trim()
		}
	})
}
