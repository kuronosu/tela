//go:build telagpu && (linux || windows)

package gpu

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/vk"
)

// hasBackend reports whether this build has a device backend.
const hasBackend = true

// The tolerance of GPU output against the CPU rasterizer's (ADR-282), on
// premultiplied 8-bit channels: all pixels but a small share differ by at
// most tolLevels, and none by more than tolHard. The numbers only tighten.
const (
	tolLevels = 2
	tolShare  = 0.005
	tolHard   = 24
)

// tolerance is the rule of one scene.
type tolerance struct {
	levels int
	share  float64
	hard   int
	// flatten, when not zero, adds a second comparison that must meet the
	// general rule: with curves flattened at this tolerance instead of
	// raster.FlattenTolerance, by the rasterizer and by what the renderer
	// rasterizes with its code (paths, strokes, glyphs, clip masks).
	flatten float32
}

// overrides are the scenes that do not meet the rule, each with its cause.
var overrides = map[string]tolerance{
	// Rounded rectangles under rotations and skews, a scene full of them.
	// The rasterizer fills them as paths, whose corners it flattens to
	// polygons within raster.FlattenTolerance (0.1 pixel) of the arcs:
	// the pixels along a corner lose up to a tenth of their coverage, 10
	// to 16 levels here, and at scale 1 the scene has more than 0.5% of
	// such pixels. The shader's arcs are not flattened. Against a
	// rasterizer flattening fifty times finer the scene meets the general
	// rule (within 2 levels but for a few pixels at the tip of a thin
	// rotated ellipse).
	"gpu_transforms": {levels: tolLevels, share: 0.01, hard: tolHard, flatten: 1.0 / 512},
	// Shadows under a skew, and under a rotation with different scales
	// per axis. The rasterizer does not evaluate these in closed form: it
	// rasterizes the shape and blurs it with a separable device-space
	// Gaussian of the two marginal standard deviations, which ignores the
	// correlation such a transform gives the two axes. The shader
	// evaluates the closed form in the command's local space, where the
	// Gaussian is round, which is the picture CSS defines (blur, then
	// transform). The two differ over the whole shadow, by up to 15
	// levels.
	"gpu_shadows_skewed": {levels: tolLevels, share: 0.25, hard: tolHard},
	// The shared scene has one card of that kind: a shadow under a skew
	// ("the mask-and-blur path"), up to 6 levels over its soft edge. The
	// rest of the scene is within 2.
	"shadows": {levels: tolLevels, share: 0.02, hard: tolHard},
	// Pixel centers exactly on a discontinuity of a paint (a hard stop of a
	// gradient, a conic seam, the edge of a cone, the wrap of a repeat,
	// the boundary of two pixels of an image sampled by its nearest
	// pixel), and hard stops in a focal gradient, whose parameter the
	// shader solves in float32: which side such a pixel takes depends on
	// the last bits of the coordinate, float64 in the rasterizer. The
	// pixels are few, and each takes the color of one side or the other.
	"gpu_discontinuities": {levels: tolLevels, share: tolShare, hard: 255},
	// Rounded clips under a rotation, a skew and a quarter turn, whose
	// corners the rasterizer flattens within raster.FlattenTolerance (see
	// gpu_transforms): the pixels along them differ by up to a tenth of
	// their coverage, of content with much contrast (23 levels). The
	// scenes meet the general rule; this adds the second comparison, with
	// curves flattened fifty times finer on both sides (the paths of the
	// scenes are flattened by the same code), where the corners agree.
	"gpu_clips_shapes": {levels: tolLevels, share: tolShare, hard: tolHard, flatten: 1.0 / 512},
	"gpu_clips_nested": {levels: tolLevels, share: tolShare, hard: tolHard, flatten: 1.0 / 512},
}

func toleranceOf(scene string) tolerance {
	if tol, ok := overrides[scene]; ok {
		return tol
	}
	return tolerance{levels: tolLevels, share: tolShare, hard: tolHard}
}

// messages collects what the validation layer and the drivers report.
type messages struct {
	mu   sync.Mutex
	list []vk.DebugMessage
}

func (m *messages) add(msg vk.DebugMessage) {
	m.mu.Lock()
	m.list = append(m.list, msg)
	m.mu.Unlock()
}

// report fails the test for every message when validating, and logs them
// otherwise.
func (m *messages) report(t testing.TB) {
	t.Helper()
	m.mu.Lock()
	list := m.list
	m.list = nil
	m.mu.Unlock()
	for _, msg := range list {
		if validating() {
			t.Errorf("%s", msg)
		} else {
			t.Logf("%s", msg)
		}
	}
}

func validating() bool { return os.Getenv("TELA_VK_VALIDATE") == "1" }

// eachDevice runs f as a subtest for every physical device, with a device
// that is closed afterwards; what Vulkan reports until then, objects left
// alive included, fails the test under TELA_VK_VALIDATE=1. It skips
// without Vulkan.
func eachDevice(t *testing.T, f func(t *testing.T, dev Device)) {
	t.Helper()
	if err := vk.Load(); err != nil {
		t.Skipf("Vulkan is not available: %v", err)
	}
	msgs := &messages{}
	inst, err := vk.NewInstance(vk.InstanceConfig{AppName: "tela gpu tests", Validation: validating(), Debug: msgs.add})
	if err != nil {
		if validating() {
			t.Fatalf("TELA_VK_VALIDATE=1: %v", err)
		}
		t.Skipf("no Vulkan instance: %v", err)
	}
	defer func() {
		inst.Destroy()
		msgs.report(t)
	}()
	devices, err := inst.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) == 0 {
		t.Skip("Vulkan has no usable physical device (Vulkan 1.1 with a graphics queue)")
	}
	for _, info := range devices {
		t.Run(deviceLabel(info.Name), func(t *testing.T) {
			dev, err := OpenVulkan(inst, info)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				dev.Close()
				msgs.report(t)
			}()
			f(t, dev)
		})
	}
}

// deviceLabel shortens a device name to a word for test and file names.
func deviceLabel(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "_"):
			b.WriteByte('_')
		}
	}
	return strings.TrimSuffix(b.String(), "_")
}

func sceneSize(s *testScene, scale float32) (int, int) {
	return int(math.Round(float64(float32(s.W) * scale))), int(math.Round(float64(float32(s.H) * scale)))
}

// renderCPU renders the list with the rasterizer onto an image cleared to
// a color, as its goldens are.
func renderCPU(w, h int, dl *gfx.DisplayList, scale float32, clear gfx.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	raster.Render(img, dl, raster.Options{Scale: scale, Clear: &clear})
	return img
}

// renderGPU renders the list into a new target and reads it back.
func renderGPU(t testing.TB, dev Device, w, h int, dl *gfx.DisplayList, opts Options) (*image.RGBA, Result) {
	t.Helper()
	target, err := dev.NewTarget(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	var r Renderer
	res, err := r.Render(target, dl, opts)
	if err != nil {
		t.Fatal(err)
	}
	return readAll(t, target), res
}

func readAll(t testing.TB, target Target) *image.RGBA {
	t.Helper()
	img := image.NewRGBA(image.Rectangle{Max: target.Size()})
	if err := target.ReadPixels(img, img.Rect); err != nil {
		t.Fatal(err)
	}
	return img
}

// metrics describes how two images of the same size differ: the largest
// difference of a channel, and the share of the pixels with a channel
// that differs by more than 1, 2 and 8 levels.
type metrics struct {
	max           int
	over1, over2  float64
	over8         float64
	worst         image.Point
	pixels, over0 int
}

func compare(a, b *image.RGBA) metrics {
	var m metrics
	var n1, n2, n8 int
	r := a.Rect
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i, j := a.PixOffset(x, y), b.PixOffset(x, y)
			d := 0
			for c := 0; c < 4; c++ {
				d = max(d, absInt(int(a.Pix[i+c])-int(b.Pix[j+c])))
			}
			if d > m.max {
				m.max, m.worst = d, image.Pt(x, y)
			}
			if d > 0 {
				m.over0++
			}
			if d > 1 {
				n1++
			}
			if d > 2 {
				n2++
			}
			if d > 8 {
				n8++
			}
		}
	}
	m.pixels = r.Dx() * r.Dy()
	if m.pixels > 0 {
		p := float64(m.pixels)
		m.over1, m.over2, m.over8 = float64(n1)/p, float64(n2)/p, float64(n8)/p
	}
	return m
}

// overShare returns the share of pixels of m that differ by more than n
// levels, for the n the metrics keep.
func (m metrics) overShare(a, b *image.RGBA, n int) float64 {
	switch n {
	case 1:
		return m.over1
	case 2:
		return m.over2
	case 8:
		return m.over8
	}
	count := 0
	for i := 0; i < len(a.Pix); i += 4 {
		d := 0
		for c := 0; c < 4; c++ {
			d = max(d, absInt(int(a.Pix[i+c])-int(b.Pix[i+c])))
		}
		if d > n {
			count++
		}
	}
	return float64(count) / float64(m.pixels)
}

// dump writes the two images and their difference when TELA_GPU_DUMP names
// a directory. In the difference a pixel is black where the images agree
// and brighter the more they differ: gray up to 2 levels, red beyond.
func dump(t testing.TB, name string, gpu, cpu *image.RGBA) {
	dir := os.Getenv("TELA_GPU_DUMP")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	diff := image.NewNRGBA(gpu.Rect)
	for y := gpu.Rect.Min.Y; y < gpu.Rect.Max.Y; y++ {
		for x := gpu.Rect.Min.X; x < gpu.Rect.Max.X; x++ {
			i := gpu.PixOffset(x, y)
			d := 0
			for c := 0; c < 4; c++ {
				d = max(d, absInt(int(gpu.Pix[i+c])-int(cpu.Pix[i+c])))
			}
			px := color.NRGBA{A: 255}
			switch {
			case d == 0:
			case d <= tolLevels:
				px.R, px.G, px.B = uint8(60*d), uint8(60*d), uint8(60*d)
			default:
				px.R, px.G = 255, uint8(min(255, 8*d))
			}
			diff.SetNRGBA(x, y, px)
		}
	}
	for suffix, img := range map[string]image.Image{"gpu": gpu, "cpu": cpu, "diff": diff} {
		f, err := os.Create(filepath.Join(dir, name+"_"+suffix+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestCompare renders every scene at three scales on every physical
// device and compares the pixels read back with the rasterizer's. A scene
// with anything the renderer does not draw fails. The table of
// differences is logged per device (go test -v).
func TestCompare(t *testing.T) {
	scenes := allScenes()
	eachDevice(t, func(t *testing.T, dev Device) {
		label := deviceLabel(dev.Name())
		var table strings.Builder
		fmt.Fprintf(&table, "\n%s\n%-26s %5s %4s %8s %8s %8s  %s\n", dev.Name(), "scene", "scale", "max", ">1", ">2", ">8", "result")
		for i := range scenes {
			s := &scenes[i]
			dl := s.displayList()
			for _, scale := range []float32{1, 1.5, 2} {
				name := fmt.Sprintf("%s_x%g", s.Name, scale)
				t.Run(name, func(t *testing.T) {
					w, h := sceneSize(s, scale)
					gpu, res := renderGPU(t, dev, w, h, dl, Options{Scale: scale, Clear: &s.clear})
					if len(res.Unsupported) > 0 {
						// Every command is drawn: nothing is left to skip.
						fmt.Fprintf(&table, "%-26s %5g %4s %8s %8s %8s  FAIL: not drawn: %s\n", s.Name, scale, "", "", "", "", strings.Join(res.Unsupported, ", "))
						t.Fatalf("not drawn: %s", strings.Join(res.Unsupported, ", "))
					}
					cpu := renderCPU(w, h, dl, scale, s.clear)
					m := compare(gpu, cpu)
					dump(t, fmt.Sprintf("%s_%g_%s", s.Name, scale, label), gpu, cpu)
					tol := toleranceOf(s.Name)
					share := m.overShare(gpu, cpu, tol.levels)
					result := "ok"
					if m.max > tol.hard || share > tol.share {
						result = "FAIL"
						t.Errorf("largest difference %d at %v (limit %d); %.3f%% of the pixels differ by more than %d levels (limit %.3f%%)",
							m.max, m.worst, tol.hard, 100*share, tol.levels, 100*tol.share)
					}
					if tol.flatten != 0 {
						// The same with curves flattened finer.
						old := raster.FlattenTolerance
						raster.FlattenTolerance = tol.flatten
						gpu, _ := renderGPU(t, dev, w, h, dl, Options{Scale: scale, Clear: &s.clear})
						fine := renderCPU(w, h, dl, scale, s.clear)
						raster.FlattenTolerance = old
						fm := compare(gpu, fine)
						dump(t, fmt.Sprintf("%s_%g_%s_fine", s.Name, scale, label), gpu, fine)
						if fm.max > tolHard || fm.over2 > tolShare {
							result = "FAIL"
							t.Errorf("flattening at %g: largest difference %d at %v; %.3f%% of the pixels differ by more than 2 levels",
								tol.flatten, fm.max, fm.worst, 100*fm.over2)
						}
						result += fmt.Sprintf(" (override; flattening at 1/%g px: max %d, >2 %.3f%%)", 1/tol.flatten, fm.max, 100*fm.over2)
					} else if _, ok := overrides[s.Name]; ok {
						result += " (override)"
					}
					fmt.Fprintf(&table, "%-26s %5g %4d %7.3f%% %7.3f%% %7.3f%%  %s\n", s.Name, scale, m.max, 100*m.over1, 100*m.over2, 100*m.over8, result)
				})
			}
		}
		t.Log(table.String())
	})
}

// regionScene is drawn by the tests of regions and kept pixels: every
// primitive, over the whole target.
func regionScene() *gfx.DisplayList {
	return gfxtest.Build(func(b *gfx.Builder) {
		for i := range gpuScenes {
			if n := gpuScenes[i].Name; n == "gpu_shadows" || n == "gpu_linear" || n == "gpu_clips" {
				b.PushTransform(gfx.Translate(float32(i)*8, float32(i)*4)) // whole pixels at 1.5 too
				gpuScenes[i].Draw(b)
				b.PopTransform()
			}
		}
		for i := range textureScenes {
			if n := textureScenes[i].Name; n != "gpu_textures_transparent" && n != "gpu_text_transformed" {
				b.PushTransform(gfx.Translate(float32(i)*6, float32(i)*2))
				textureScenes[i].Draw(b)
				b.PopTransform()
			}
		}
	})
}

// regionCase is a scene of TestRegion with the regions it is rendered in.
type regionCase struct {
	name    string
	dl      *gfx.DisplayList
	w, h    int // logical size
	regions []image.Rectangle
}

// regionCases returns the scenes of TestRegion: every primitive, then the
// scenes of layers, filters and backdrop filters, whose regions cross
// layers, clips, blurred edges and glass panels, inside them, at their
// edges and across several.
func regionCases() []regionCase {
	cases := []regionCase{{name: "primitives", dl: regionScene(), w: 421, h: 303}}
	for i := range layerScenes {
		s := &layerScenes[i]
		switch s.Name {
		case "gpu_blend_clips", "gpu_opacity", "gpu_masks", "gpu_filters", "gpu_blur_edges", "gpu_drop_shadows",
			"gpu_backdrop", "gpu_backdrop_layers", "gpu_glass":
			cases = append(cases, regionCase{name: s.Name, dl: s.displayList(), w: s.W, h: s.H})
		}
	}
	rng := rand.New(rand.NewSource(11))
	for i := range cases {
		c := &cases[i]
		w, h := c.w, c.h
		c.regions = []image.Rectangle{
			image.Rect(0, 0, 64, 64), image.Rect(37, 51, 190, 143), image.Rect(200, 1, 201, h-3),
			image.Rect(100, 100, 101, 101), image.Rect(w-121, h-103, w+79, h+97), image.Rect(-50, -50, 30, 30),
			image.Rect(119, 40, 262, h-13), image.Rect(0, h/2, w, h/2+9),
		}
		// Regions inside a backdrop filter, across its edges and its
		// corners, where two overlap, inside the layers that hold them and
		// across those, at the target's edges.
		c.regions = append(c.regions, map[string][]image.Rectangle{
			"gpu_backdrop": {image.Rect(5, 75, 40, 100), image.Rect(60, 100, 90, 130), image.Rect(125, 90, 150, 120), image.Rect(90, 230, 160, 280),
				image.Rect(500, 270, 520, 340), image.Rect(396, 196, 420, 214), image.Rect(130, 180, 190, 200), image.Rect(0, 300, 30, 340)},
			"gpu_backdrop_layers": {image.Rect(50, 40, 100, 70), image.Rect(130, 20, 160, 60), image.Rect(100, 180, 180, 230), image.Rect(330, 40, 350, 60),
				image.Rect(150, 130, 200, 150), image.Rect(240, 130, 270, 160)},
			"gpu_glass": {image.Rect(200, 40, 260, 70), image.Rect(430, 110, 470, 150), image.Rect(300, 330, 360, 345), image.Rect(10, 10, 30, 30),
				image.Rect(320, 150, 360, 170)},
			"gpu_masks": {image.Rect(260, 180, 300, 210)},
		}[c.name]...)
		// Small and middle-sized regions anywhere, as damage is.
		for k := 0; k < 8; k++ {
			x, y := rng.Intn(w), rng.Intn(h)
			c.regions = append(c.regions, image.Rect(x, y, x+3+rng.Intn(60+k*20), y+3+rng.Intn(40+k*12)))
		}
	}
	return cases
}

// TestRegion checks that a render of a region writes only the region, and
// there the pixels of a full render, bit for bit: also through layers,
// filters and backdrop filters, which read beyond the region.
func TestRegion(t *testing.T) {
	cases := regionCases()
	sentinel := gfx.RGB8(1, 2, 3)
	eachDevice(t, func(t *testing.T, dev Device) {
		for _, c := range cases {
			for _, scale := range []float32{1, 1.5} {
				w, h := int(float32(c.w)*scale), int(float32(c.h)*scale)
				full, res := renderGPU(t, dev, w, h, c.dl, Options{Scale: scale, Clear: &gfx.White})
				if res.Unsupported != nil {
					t.Fatalf("%s has commands that are not drawn: %v", c.name, res.Unsupported)
				}
				// One Renderer for every region: each finds in the atlases what
				// the regions before it drew with, and adds its own.
				var r Renderer
				target, err := dev.NewTarget(w, h)
				if err != nil {
					t.Fatal(err)
				}
				for _, region := range c.regions {
					region = image.Rect(int(float32(region.Min.X)*scale), int(float32(region.Min.Y)*scale), int(float32(region.Max.X)*scale), int(float32(region.Max.Y)*scale))
					if _, err := r.Render(target, nil, Options{Clear: &sentinel}); err != nil {
						t.Fatal(err)
					}
					res, err := r.Render(target, c.dl, Options{Scale: scale, Region: region, Clear: &gfx.White})
					if err != nil {
						t.Fatal(err)
					}
					if res.Unsupported != nil {
						t.Fatalf("%s has commands that are not drawn: %v", c.name, res.Unsupported)
					}
					got := readAll(t, target)
					bad := 0
					for y := 0; y < h; y++ {
						for x := 0; x < w; x++ {
							want := color.RGBA{1, 2, 3, 255}
							if image.Pt(x, y).In(region) {
								want = full.RGBAAt(x, y)
							}
							if g := got.RGBAAt(x, y); g != want {
								if bad < 3 {
									t.Errorf("%s, scale %g, region %v: pixel (%d, %d) is %v, want %v", c.name, scale, region, x, y, g, want)
								}
								bad++
							}
						}
					}
					if bad > 0 {
						t.Errorf("%s, scale %g, region %v: %d pixels differ", c.name, scale, region, bad)
					}
				}
				target.Close()
			}
		}
	})
}

// TestKeepsPixels checks that a target keeps its pixels: a second render
// without Clear composites over the first, as the rasterizer does over
// its destination image.
func TestKeepsPixels(t *testing.T) {
	first := gpuScenes[0].displayList()
	second := gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(10.5, 10.5, 150, 80), sky.WithAlpha(0.5))
		shadow(b, gfx.NewRRect(gfx.RectXYWH(40, 30, 80, 50), 10), 0, 6, 14, 0, gfx.Black.WithAlpha(0.6), false)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(40, 30, 80, 50), 10), gfx.LinearGradient{
			Start: gfx.Pt(40, 30), End: gfx.Pt(120, 80), Stops: stops(amber.WithAlpha(0.3), rose)})
		// What comes from textures, translucent too: text, a sprite, a
		// path, an image, through a clip mask.
		text(b, "SoA*", 28, 6.3, 100.6, indigo.WithAlpha(0.6))
		b.PushClipPath(gfx.CirclePath(gfx.Pt(150, 70), 44), gfx.NonZero)
		b.FillPath(gfxtest.StarPath(150, 70, 46), gfx.EvenOdd, emerald.WithAlpha(0.5))
		b.DrawImage(gfxtest.TestImage, gfx.Rect{}, gfx.RectXYWH(130, 20, 60, 40), gfx.SampleLinear)
		b.PopClip()
	})
	const w, h = 200, 120
	eachDevice(t, func(t *testing.T, dev Device) {
		target, err := dev.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		cpu := image.NewRGBA(image.Rect(0, 0, w, h))
		// A new target is transparent, like a new image.
		if m := compare(readAll(t, target), cpu); m.max != 0 {
			t.Fatalf("a new target is not transparent: largest channel %d at %v", m.max, m.worst)
		}
		for i, dl := range []*gfx.DisplayList{first, second, second} {
			if _, err := r.Render(target, dl, Options{}); err != nil {
				t.Fatal(err)
			}
			raster.Render(cpu, dl, raster.Options{})
			gpu := readAll(t, target)
			m := compare(gpu, cpu)
			dump(t, fmt.Sprintf("keeps_%d_%s", i, deviceLabel(dev.Name())), gpu, cpu)
			t.Logf("after render %d: largest difference %d, %.3f%% of the pixels over 1 level", i+1, m.max, 100*m.over1)
			// Differences of one level add up over renders: each render
			// rounds, on both sides.
			if m.max > tolHard || m.over2 > tolShare {
				t.Errorf("after render %d: largest difference %d at %v, %.3f%% of the pixels over 2 levels", i+1, m.max, m.worst, 100*m.over2)
			}
		}
	})
}

// TestSizes renders into targets of odd sizes, down to one pixel.
func TestSizes(t *testing.T) {
	eachDevice(t, func(t *testing.T, dev Device) {
		for _, s := range []*testScene{&gpuScenes[1], &textureScenes[0], &textureScenes[2]} { // gpu_rrects, gpu_text, gpu_paths
			dl := s.displayList()
			for _, size := range []image.Point{{1, 1}, {1, 7}, {3, 5}, {127, 61}, {255, 3}, {333, 257}} {
				gpu, _ := renderGPU(t, dev, size.X, size.Y, dl, Options{Scale: 1.25, Clear: &gfx.White})
				cpu := renderCPU(size.X, size.Y, dl, 1.25, gfx.White)
				// A single pixel over the limit is a large share of a
				// target of a few pixels: there, only the hard limit.
				if m := compare(gpu, cpu); m.max > tolHard || (m.over2 > tolShare && m.pixels > 1000) || (m.pixels <= 1000 && m.max > tolLevels+1) {
					t.Errorf("%s, size %v: largest difference %d at %v, %.3f%% of the pixels over 2 levels", s.Name, size, m.max, m.worst, 100*m.over2)
				}
			}
		}
		if _, err := dev.NewTarget(0, 10); err == nil {
			t.Error("a target of width 0 was created")
		}
		if _, err := dev.NewTarget(dev.MaxTargetSize()+1, 1); err == nil {
			t.Error("a target wider than the device allows was created")
		}
	})
}

// TestEmpty checks the renders that draw nothing, or only clear.
func TestEmpty(t *testing.T) {
	const w, h = 40, 30
	empty := gfx.NewBuilder().Finish()
	scene := gpuScenes[0].displayList()
	half := gfx.Color{R: 0.2, G: 0.4, B: 0.6, A: 0.5}
	eachDevice(t, func(t *testing.T, dev Device) {
		check := func(what string, gpu, cpu *image.RGBA) {
			t.Helper()
			if m := compare(gpu, cpu); m.max != 0 {
				t.Errorf("%s: largest difference %d at %v: GPU %v, CPU %v", what, m.max, m.worst, gpu.RGBAAt(m.worst.X, m.worst.Y), cpu.RGBAAt(m.worst.X, m.worst.Y))
			}
		}
		for _, dl := range []*gfx.DisplayList{nil, empty} {
			// Nothing at all: the target stays transparent.
			gpu, res := renderGPU(t, dev, w, h, dl, Options{})
			check("an empty list", gpu, image.NewRGBA(image.Rect(0, 0, w, h)))
			if res.Instances != 0 || res.Draws != 0 || res.Unsupported != nil {
				t.Errorf("an empty list: %+v", res)
			}
			// Only a clear, of the whole target and of a region, with a
			// translucent color (premultiplied).
			gpu, _ = renderGPU(t, dev, w, h, dl, Options{Clear: &half})
			check("a clear", gpu, renderCPU(w, h, dl, 1, half))
			region := image.Rect(5, 7, 22, 19)
			gpu, _ = renderGPU(t, dev, w, h, dl, Options{Clear: &half, Region: region})
			cpu := image.NewRGBA(image.Rect(0, 0, w, h))
			raster.Render(cpu, dl, raster.Options{Clear: &half, Region: region})
			check("a clear of a region", gpu, cpu)
		}
		// A region without area means the whole target, as for the
		// rasterizer; a region outside the target draws nothing.
		for _, region := range []image.Rectangle{{}, image.Rect(5, 5, 5, 20), image.Rect(100, 100, 200, 200), image.Rect(-10, -10, 0, 0)} {
			gpu, _ := renderGPU(t, dev, w, h, scene, Options{Clear: &gfx.White, Region: region})
			cpu := image.NewRGBA(image.Rect(0, 0, w, h))
			raster.Render(cpu, scene, raster.Options{Clear: &gfx.White, Region: region})
			if m := compare(gpu, cpu); m.max > 1 {
				t.Errorf("region %v: largest difference %d at %v", region, m.max, m.worst)
			}
			if region.Empty() == (cpu.RGBAAt(0, 0) == color.RGBA{}) {
				t.Errorf("region %v: the rasterizer's pixel (0, 0) is %v", region, cpu.RGBAAt(0, 0))
			}
		}
	})
}

// TestLifetime checks what happens around Close: targets closed twice,
// after their device, and used when closed.
func TestLifetime(t *testing.T) {
	eachDevice(t, func(t *testing.T, dev Device) {
		a, err := dev.NewTarget(16, 16)
		if err != nil {
			t.Fatal(err)
		}
		b, err := dev.NewTarget(8, 8)
		if err != nil {
			t.Fatal(err)
		}
		var r Renderer
		// A frame for another size is refused.
		if err := b.Draw(r.enc.Encode(nil, image.Pt(16, 16), Options{Clear: &gfx.White})); err == nil {
			t.Error("a frame for 16x16 was drawn into a target of 8x8")
		}
		a.Close()
		a.Close()
		if _, err := r.Render(a, nil, Options{Clear: &gfx.White}); err == nil {
			t.Error("a closed target was drawn into")
		}
		if err := a.ReadPixels(image.NewRGBA(image.Rect(0, 0, 16, 16)), image.Rect(0, 0, 16, 16)); err == nil {
			t.Error("a closed target was read")
		}
		// b is left to the device.
		dev.Close()
		dev.Close()
		b.Close()
		if _, err := dev.NewTarget(4, 4); err == nil {
			t.Error("a closed device created a target")
		}
	})
}

// TestBlend measures the device's blending against the rasterizer's exact
// rounding, which a blender need not match: it is only required to work
// at the precision of the target. Two cases, each every 8-bit destination
// value (256 opaque columns) under 256 sources (rows): a black source of
// every alpha, whose premultiplied values are whole 8-bit numbers; and a
// colored source at half coverage of every alpha, whose values are not.
// The test logs how many pixels differ, and by how much.
func TestBlend(t *testing.T) {
	scene := func(row func(b *gfx.Builder, i int)) *gfx.DisplayList {
		return gfxtest.Build(func(b *gfx.Builder) {
			for i := 0; i < 256; i++ {
				v := float32(i) / 255
				b.FillRect(gfx.RectXYWH(float32(i), 0, 1, 256), gfx.Color{R: v, G: v, B: v, A: 1})
			}
			for i := 0; i < 256; i++ {
				row(b, i)
			}
		})
	}
	whole := scene(func(b *gfx.Builder, i int) {
		b.FillRect(gfx.RectXYWH(0, float32(i), 256, 1), gfx.Black.WithAlpha(float32(i)/255))
	})
	fractional := scene(func(b *gfx.Builder, i int) {
		b.FillRect(gfx.RectXYWH(0, float32(i)+0.25, 256, 0.5), gfx.Color{R: 0.9, G: 0.5, B: 0.1, A: float32(i) / 255})
	})
	eachDevice(t, func(t *testing.T, dev Device) {
		for _, c := range []struct {
			name string
			dl   *gfx.DisplayList
		}{{"whole 8-bit sources", whole}, {"sources between 8-bit values", fractional}} {
			gpu, res := renderGPU(t, dev, 256, 256, c.dl, Options{Clear: &gfx.White})
			if res.Draws != 1 || res.Instances != 511 { // the transparent row is dropped
				t.Errorf("%s: %d instances in %d draws, want 511 in 1", c.name, res.Instances, res.Draws)
			}
			cpu := renderCPU(256, 256, c.dl, 1, gfx.White)
			m := compare(gpu, cpu)
			t.Logf("%s: largest difference %d, %d of %d pixels differ", c.name, m.max, m.over0, m.pixels)
			if m.max > 1 {
				t.Errorf("%s: largest difference %d at %v: GPU %v, CPU %v", c.name, m.max, m.worst, gpu.RGBAAt(m.worst.X, m.worst.Y), cpu.RGBAAt(m.worst.X, m.worst.Y))
			}
		}
	})
}
