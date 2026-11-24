//go:build amd64 || arm64

package main

import (
	"context"
	"fmt"
	"image"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/texture/vulkan"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The DRM fourccs a texture takes.
const (
	xr24 = 0x34325258
	xb24 = 0x34324258
	ar24 = 0x34325241
	ab24 = 0x34324241
)

func linear(fourccs ...uint32) (out []tela.TextureFormat) {
	for _, f := range fourccs {
		out = append(out, tela.TextureFormat{Fourcc: f})
	}
	return out
}

// openVulkan connects to Vulkan, with the validation layer under
// TELA_VK_VALIDATE=1 (as the tests of internal/vk): anything it reports fails
// the test, objects left alive included. It skips without Vulkan.
func openVulkan(t *testing.T) *vulkan.Instance {
	t.Helper()
	inst, err := vulkanOpener(t)()
	if err != nil {
		t.Skipf("no Vulkan: %v", err)
	}
	return inst
}

// vulkanOpener returns what openVulkan opens Vulkan with.
func vulkanOpener(t *testing.T) func() (*vulkan.Instance, error) {
	open := vulkan.Open
	if os.Getenv("TELA_VK_VALIDATE") == "1" {
		var mu sync.Mutex
		var said []string
		open = func() (*vulkan.Instance, error) {
			return vulkan.OpenValidated(func(m string) {
				mu.Lock()
				said = append(said, m)
				mu.Unlock()
			})
		}
		t.Cleanup(func() {
			mu.Lock()
			defer mu.Unlock()
			for _, m := range said {
				t.Errorf("validation: %s", m)
			}
		})
	}
	return open
}

// eachDevice runs a test on every GPU. It skips without any.
func eachDevice(t *testing.T, f func(t *testing.T, inst *vulkan.Instance, d vulkan.Device)) {
	inst := openVulkan(t)
	defer inst.Close()
	devices, err := inst.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) == 0 {
		t.Skip("Vulkan has no GPU")
	}
	for _, d := range devices {
		t.Run(d.Name, func(t *testing.T) { f(t, inst, d) })
	}
}

// renderer opens the scene's renderer. It skips on a device that lacks
// something it needs.
func renderer(t *testing.T, inst *vulkan.Instance, d vulkan.Device, formats []tela.TextureFormat, readBy vulkan.Memory) *vulkan.Renderer {
	t.Helper()
	r, err := openRenderer(inst, d, formats, readBy)
	if err != nil {
		t.Skip(err)
	}
	return r
}

// TestVulkanDrawsTheScene draws the scenes with every GPU into linear dmabufs,
// waits for the sync file of the frame, maps the dmabuf and compares its
// pixels with those of the software renderer.
func TestVulkanDrawsTheScene(t *testing.T) {
	eachDevice(t, func(t *testing.T, inst *vulkan.Instance, d vulkan.Device) {
		for _, readBy := range []vulkan.Memory{vulkan.Mapped, vulkan.Shared, vulkan.Own} {
			for _, fourcc := range []uint32{xr24, xb24, ar24, ab24} {
				t.Run(fmt.Sprintf("%c%c24 memory %d", fourcc&0xff, fourcc>>8&0xff, readBy), func(t *testing.T) {
					r := renderer(t, inst, d, linear(fourcc), readBy)
					defer r.Close()
					for _, size := range []image.Point{{640, 360}, {333, 187}} { // an odd row too
						drawAndCompare(t, r, size, readBy)
					}
					if n := r.Targets(); n != 0 {
						t.Errorf("%d targets left open", n)
					}
				})
			}
		}
	})
}

// TestVulkanAtTwiceTheSize is the largest the window draws at scale 2, 200%:
// there the edges are the sharpest, so the rounding of the GPU shows most.
func TestVulkanAtTwiceTheSize(t *testing.T) {
	eachDevice(t, func(t *testing.T, inst *vulkan.Instance, d vulkan.Device) {
		r := renderer(t, inst, d, linear(xb24), vulkan.Mapped)
		defer r.Close()
		drawAndCompare(t, r, image.Pt(3064, 1724), vulkan.Mapped)
	})
}

func drawAndCompare(t *testing.T, r *vulkan.Renderer, size image.Point, readBy vulkan.Memory) {
	t.Helper()
	target, err := r.NewTarget(size.X, size.Y)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	for i := range 3 * len(scenes) { // the buffer drawn into again
		s, at := Scene(i%len(scenes)), []time.Duration{4 * time.Second, 20 * time.Second, 2 * time.Hour}[i/len(scenes)]
		u := sceneAt(s, at, size.X, size.Y)
		fd, err := target.Draw(u.bytes())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		err = tela.SyncFile(fd).Wait(ctx)
		cancel()
		if err != nil {
			t.Fatalf("the sync file of frame %d: %v", i, err)
		}
		got, err := readDmabuf(target, r.Fourcc)
		if err != nil {
			if readBy == vulkan.Own {
				t.Skipf("the GPU's own memory cannot be mapped: %v", err)
			}
			t.Fatal(err)
		}
		want := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
		drawScene(want, &u)
		compare(t, scenes[s].name, got, want)
	}
}

// readDmabuf returns the pixels of a linear dmabuf, as RGBA.
func readDmabuf(target *vulkan.Target, fourcc uint32) (*image.RGBA, error) {
	if target.Modifier != vulkan.Linear || len(target.Planes) != 1 {
		return nil, fmt.Errorf("not linear: modifier %#x, %d planes", target.Modifier, len(target.Planes))
	}
	p := target.Planes[0]
	n := int(p.Offset) + int(p.Stride)*(target.Height-1) + 4*target.Width
	data, err := unix.Mmap(target.FD, 0, n, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mapping the dmabuf: %w", err)
	}
	defer unix.Munmap(data)
	sync := func(flags uint64) error { // DMA_BUF_IOCTL_SYNC
		if _, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(target.FD), 0x40086200, uintptr(unsafe.Pointer(&flags))); e != 0 {
			return fmt.Errorf("DMA_BUF_IOCTL_SYNC: %w", e)
		}
		return nil
	}
	if err := sync(1 | 0<<2); err != nil { // read, start
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, target.Width, target.Height))
	for y := range target.Height {
		copy(img.Pix[y*img.Stride:][:img.Stride], data[int(p.Offset)+y*int(p.Stride):])
	}
	if err := sync(1 | 1<<2); err != nil { // read, end
		return nil, err
	}
	if fourcc == xr24 || fourcc == ar24 {
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i]
		}
	}
	return img, nil
}

// compare fails the test unless the pictures are the same, but for the
// rounding of another processor.
func compare(t *testing.T, what string, got, want *image.RGBA) {
	t.Helper()
	if got.Rect.Size() != want.Rect.Size() {
		t.Fatalf("%s: a picture of %v, want %v", what, got.Rect.Size(), want.Rect.Size())
	}
	const most = 1 // level
	worst, at, differ := 0, image.Point{}, 0
	for i := range want.Pix {
		d := int(got.Pix[i]) - int(want.Pix[i])
		d = max(d, -d)
		if d > 0 {
			differ++
		}
		if d > worst {
			worst, at = d, image.Pt(i/4%want.Rect.Dx(), i/4/want.Rect.Dx())
		}
	}
	if worst > most {
		t.Errorf("%s, %v: %v, the software renderer drew %v (%d levels)", what, at, got.RGBAAt(at.X, at.Y), want.RGBAAt(at.X, at.Y), worst)
	}
	t.Logf("%s, %v: %d of %d components differ, by at most %d", what, want.Rect.Size(), differ, len(want.Pix), worst)
}

// shown mounts a texture of 320x180 alone in a window.
func shown(t *testing.T, opts ...telatest.Option) (*telatest.App, *tela.Texture) {
	t.Helper()
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Texture(tela.UseTexture(), Class("h-full w-full"), TestID("scene"))
	}), append(opts, telatest.Size(320, 180), telatest.Timeout(30*time.Second))...)
	return app, app.Texture(telatest.ByTestID("scene"))
}

// paintAndCompare paints a scene and compares what the window shows.
func paintAndCompare(t *testing.T, app *telatest.App, tex *tela.Texture, p painter, s Scene, at time.Duration) {
	t.Helper()
	size := tex.Size()
	u := sceneAt(s, at, size.Width, size.Height)
	before := tex.Stats().Shown
	if err := p.paint(t.Context(), &u); err != nil {
		t.Fatal(err)
	}
	app.WaitFor("the frame", func() bool { return tex.Stats().Shown > before })
	want := image.NewRGBA(image.Rect(0, 0, size.Width, size.Height))
	drawScene(want, &u)
	compare(t, scenes[s].name, app.Screenshot(), want)
}

// TestPainterOnTheWindow paints with every GPU into linear dmabufs that the
// texture maps and copies, and compares the window's pixels: many frames, a
// resize, a change of scale, then the end. It leaves no descriptor open.
func TestPainterOnTheWindow(t *testing.T) {
	eachDevice(t, func(t *testing.T, inst *vulkan.Instance, d vulkan.Device) {
		run := func() {
			app, tex := shown(t)
			p := newPainter(tex, inst, renderer(t, inst, d, tex.Device().ForeignFormats, vulkan.Mapped))
			p.inst = nil // the test's
			for i := range 12 {
				paintAndCompare(t, app, tex, p, Scene(i%len(scenes)), time.Duration(i)*time.Second)
			}
			app.Resize(400, 200)
			paintAndCompare(t, app, tex, p, Planet, 30*time.Second)
			app.SetScale(2)
			paintAndCompare(t, app, tex, p, Mountains, 31*time.Second)
			paintAndCompare(t, app, tex, p, Satin, 32*time.Second)
			app.Close() // the texture closes: Tela lets go of its frames
			p.close()
			if n := p.r.Targets(); n != 0 {
				t.Errorf("%d targets left open", n)
			}
		}
		run() // the driver opens what it keeps
		before := openFiles(t)
		run()
		if after := openFiles(t); len(after) > len(before) {
			t.Errorf("%d descriptors open, %d before:\n%s", len(after), len(before), strings.Join(after, "\n"))
		}
	})
}

// openFiles returns what the descriptors of the process are.
func openFiles(t *testing.T) (out []string) {
	t.Helper()
	list, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range list {
		if to, err := os.Readlink("/proc/self/fd/" + e.Name()); err == nil {
			out = append(out, e.Name()+" "+to)
		}
	}
	return out
}

// fakeGPU describes d as Tela would if it drew with it: the tiled formats d
// has for its own frames, linear ones for those of another GPU.
func fakeGPU(inst *vulkan.Instance, d vulkan.Device) tela.TextureDevice {
	dev := tela.TextureDevice{
		GPU: true, Name: d.Name, UUID: d.UUID, RenderMajor: uint32(d.Major), RenderMinor: uint32(d.Minor),
		ForeignFormats: linear(ar24, xr24, ab24, xb24),
	}
	for _, f := range inst.Formats(d) {
		if f.Modifier != vulkan.Linear {
			dev.Formats = append(dev.Formats, tela.TextureFormat(f))
		}
	}
	return dev
}

// TestPainterOnTelasGPU: the painter opens the GPU that Texture.Device names,
// by its UUID or by its render node, and gives it frames in a layout of that
// GPU's own, whose sync files signal; "another GPU" opens another one, and
// gives linear frames. Tela's GPU is a fake one here.
func TestPainterOnTelasGPU(t *testing.T) {
	eachDevice(t, func(t *testing.T, inst *vulkan.Instance, d vulkan.Device) {
		for _, c := range []struct {
			name  string
			other bool
			edit  func(*tela.TextureDevice)
		}{
			{"UUID", false, func(dev *tela.TextureDevice) { dev.RenderMajor, dev.RenderMinor = 0, 0 }},
			{"render node", false, func(dev *tela.TextureDevice) { dev.UUID = [16]byte{} }},
			{"another", true, func(*tela.TextureDevice) {}},
		} {
			t.Run(c.name, func(t *testing.T) {
				dev := fakeGPU(inst, d)
				c.edit(&dev)
				fake := telatest.NewFakeTextureDevice(dev)
				app, tex := shown(t, telatest.WithTextureDevice(fake))
				r, err := newRenderer(inst, tex.Device(), c.other)
				if err != nil {
					t.Skip(err)
				}
				p := newPainter(tex, inst, r)
				p.inst = nil // the test's
				if same := r.Device.UUID == d.UUID; same == c.other {
					t.Errorf("drawn on %s, Tela draws on %s", r.Device.Name, d.Name)
				}
				for i := range 8 {
					u := sceneAt(Scene(i%len(scenes)), time.Duration(i)*time.Second, 320, 180)
					if err := p.paint(t.Context(), &u); err != nil {
						t.Fatal(err)
					}
					app.WaitFor("the frame", func() bool { return tex.Stats().Shown == uint64(i+1) })
				}
				var mods []uint64
				for _, b := range fake.Imports() {
					mods = append(mods, b.Dmabuf().Modifier)
				}
				if n := len(mods); n == 0 || n > gpuBuffers {
					t.Errorf("%d buffers imported for 8 frames", n)
				}
				if slices.Contains(mods, vulkan.Linear) != c.other {
					t.Errorf("modifiers %#x", mods)
				}
				t.Logf("%s; %d buffers", p.about(), len(mods))
				app.Close()
				p.close()
				if n := r.Targets(); n != 0 {
					t.Errorf("%d targets left open", n)
				}
			})
		}
	})
}

// TestSizesOnAGPU: a change of the resolution, of the card's size, of both and
// of the window's size, on every GPU, paused and playing: the loop goes on with
// the same painter, draws frames of the new size, and frees the buffers of the
// old ones as Tela lets go of them. Tela's GPU is a fake one here.
func TestSizesOnAGPU(t *testing.T) {
	eachDevice(t, func(t *testing.T, inst *vulkan.Instance, d vulkan.Device) {
		dev := fakeGPU(inst, d)
		renderer(t, inst, d, dev.Formats, vulkan.Own).Close() // it skips where this does not work
		// The example opens Vulkan by itself, while the test has it open too.
		old, open := openInstance, vulkanOpener(t)
		opened := 0
		openInstance = func() (*vulkan.Instance, error) { opened++; return open() }
		t.Cleanup(func() { openInstance = old })
		closed := make(chan struct{}, 1)
		onClose = func() { closed <- struct{}{} }
		t.Cleanup(func() { onClose = func() {} })

		fake := telatest.NewFakeTextureDevice(dev)
		a := mount(t, Demo{Paused: true, At: 4 * time.Second}, telatest.WithTextureDevice(fake))
		a.ExpectText("The scene on " + d.Name)
		a.ExpectText("766×430 at scale 1")
		// only waits until the buffers of other sizes are freed.
		only := func(w, h int) {
			t.Helper()
			a.WaitFor(fmt.Sprintf("the buffers that are not of %dx%d to be freed", w, h), func() bool {
				n := 0
				for _, b := range fake.Imports() {
					if b.Closed() {
						continue
					}
					if m := b.Dmabuf(); m.Width != w || m.Height != h {
						return false
					}
					n++
				}
				if n > gpuBuffers {
					t.Fatalf("%d buffers of %dx%d", n, w, h)
				}
				return n > 0
			})
		}
		only(766, 430)
		tex := scene(Planet)
		oneFrame(t, a, tex, "half", func() { resolve(a, 50) })
		only(383, 215)
		oneFrame(t, a, tex, "full", func() { choose(a, "Full") })
		only(389, 219)
		oneFrame(t, a, tex, "satin", func() { choose(a, "Satin") })
		only(389, 219)
		oneFrame(t, a, scene(Satin), "twice", func() { resolve(a, 200) })
		only(1556, 876)

		a.Click(play)
		a.ExpectText("shown")
		resolve(a, 25)
		only(195, 110)
		choose(a, "Mountains")
		choose(a, "Small")
		only(112, 63)
		a.Resize(760, 600)
		only(90, 51)
		a.Click(pause)
		a.ExpectText("Paused")
		a.ExpectReleases(1)
		a.ExpectText("The scene on " + d.Name)
		if opened != 1 {
			t.Errorf("Vulkan was opened %d times", opened)
		}
		a.Close()
		select {
		case <-closed:
		case <-time.After(30 * time.Second):
			t.Fatal("the GPU was not closed with the window")
		}
		for _, b := range fake.Imports() {
			if !b.Closed() {
				t.Errorf("a buffer of %dx%d was left open", b.Dmabuf().Width, b.Dmabuf().Height)
			}
		}
	})
}

// TestStreamOnAnotherGPU plays the whole example on a GPU, where Tela draws
// with the CPU: its frames are shown, and its renderer closes when another
// choice replaced it.
func TestStreamOnAnotherGPU(t *testing.T) {
	inst := openVulkan(t)
	dev := tela.TextureDevice{ForeignFormats: linear(ar24, xr24, ab24, xb24)}
	r, err := newRenderer(inst, dev, true)
	if err != nil {
		t.Skip(err)
	}
	name := r.Device.Name
	r.Close()
	inst.Close()

	old := openInstance
	openInstance = vulkanOpener(t)
	t.Cleanup(func() { openInstance = old })
	closed := make(chan struct{}, 4)
	onClose = func() { closed <- struct{}{} }
	t.Cleanup(func() { onClose = func() {} })

	a := mount(t, Demo{})
	a.Click(telatest.ByRole("radio", "Another GPU"))
	a.ExpectText("The scene on " + name)
	first := a.PixelAt(480, 250)
	a.WaitFor("the planet to turn", func() bool { return a.PixelAt(480, 250) != first })
	a.Resize(800, 700)
	a.ExpectText("398×224 at scale 1")
	a.Click(telatest.ByRole("radio", "Software"))
	a.ExpectText("the CPU, as pixels")
	a.WaitFor("the GPU to be closed", func() bool { return len(closed) == 1 })

	a.Click(telatest.ByRole("radio", "Another GPU")) // and closed with the window
	a.ExpectText("The scene on " + name)
	a.Close()
	for range 2 {
		select {
		case <-closed:
		case <-time.After(30 * time.Second):
			t.Fatal("the GPU was not closed with the window")
		}
	}
}

// BenchmarkVulkan draws frames of the window's size at scale 2 and waits for
// each: the time of the GPU and of the submission.
func BenchmarkVulkan(b *testing.B) {
	inst, err := vulkan.Open()
	if err != nil {
		b.Skip(err)
	}
	defer inst.Close()
	devices, err := inst.Devices()
	if err != nil {
		b.Fatal(err)
	}
	const w, h = 1532, 862
	for _, d := range devices {
		dev := fakeGPU(inst, d)
		for _, c := range []struct {
			name    string
			formats []tela.TextureFormat
			readBy  vulkan.Memory
		}{{"tiled", dev.Formats, vulkan.Own}, {"shared", dev.ForeignFormats, vulkan.Shared}} {
			for s := range scenes {
				b.Run(d.Name+" "+c.name+" "+scenes[s].name, func(b *testing.B) {
					r, err := openRenderer(inst, d, c.formats, c.readBy)
					if err != nil {
						b.Skip(err)
					}
					defer r.Close()
					target, err := r.NewTarget(w, h)
					if err != nil {
						b.Fatal(err)
					}
					defer target.Close()
					for i := 0; b.Loop(); i++ {
						u := sceneAt(Scene(s), time.Duration(i)*time.Second/60, w, h)
						fd, err := target.Draw(u.bytes())
						if err != nil {
							b.Fatal(err)
						}
						if err := tela.SyncFile(fd).Wait(b.Context()); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}
