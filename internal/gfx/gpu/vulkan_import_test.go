//go:build telagpu && linux

package gpu

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/vk"
)

// The tests play the application with a device of their own: it draws a
// picture, exports it (export.go) and gives it back when the picture has
// been drawn; the importer is another device, of the same GPU or of the
// other.

// Which pairs of an application's GPU and the renderer's a test is for.
const (
	anyPair   = iota
	onGPU     // one GPU, two devices
	acrossGPU // two GPUs
)

// eachPair runs f for every ordered pair of physical devices of a kind,
// with a device for the application and another for the renderer.
func eachPair(t *testing.T, kind int, f func(t *testing.T, app, win *vkDevice)) {
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
	open := func(t *testing.T, info vk.DeviceInfo) *vkDevice {
		dev, err := OpenVulkan(inst, info)
		if err != nil {
			t.Fatal(err)
		}
		return dev.(*vkDevice)
	}
	pairs := 0
	for i, a := range devices {
		for j, w := range devices {
			if kind == onGPU && i != j || kind == acrossGPU && i == j {
				continue
			}
			pairs++
			t.Run(deviceLabel(a.Name)+"_to_"+deviceLabel(w.Name), func(t *testing.T) {
				app, win := open(t, a), open(t, w)
				defer func() {
					win.Close()
					app.Close()
					msgs.report(t)
				}()
				if app.imports == nil || win.imports == nil {
					t.Skip("a device lacks the dmabuf extensions")
				}
				f(t, app, win)
			})
		}
	}
	if pairs == 0 {
		t.Skipf("%d Vulkan devices, not enough for the pair", len(devices))
	}
}

// sameGPU reports whether two devices are of the same GPU.
func sameGPU(a, b *vkDevice) bool { return a.id.UUID == b.id.UUID }

// appPicture is a picture of w x h pixels with every channel its own
// pattern, hard edges and translucency: premultiplied.
func appPicture(w, h, n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			a := 255
			if (x/16+y/16)%3 == 0 {
				a = 40 + (x*5+y*3)%200
			}
			r, g, b := (x*3+n*40)%256, (y*5+n*70)%256, (x*7+y*11)%256
			if x/24%2 == y/24%2 {
				b = 255 - b
			}
			o := img.PixOffset(x, y)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = uint8(r*a/255), uint8(g*a/255), uint8(b*a/255), uint8(a)
		}
	}
	return img
}

// appBuffer is a buffer of an application: an exported image of the device.
type appBuffer struct {
	t      testing.TB
	d      *vkDevice
	exp    *vkExport
	target Target
	r      Renderer
}

// newAppBuffer allocates a buffer of the format on app, in system memory
// when shared (what another GPU can reach), where the device has any.
func newAppBuffer(t testing.TB, app *vkDevice, w, h int, f DmabufFormat, shared bool) *appBuffer {
	t.Helper()
	b := &appBuffer{t: t, d: app}
	var err error
	if b.target, err = app.NewTarget(w, h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.target.Close)
	if !shared {
		exp, err := app.NewExport(w, h, []DmabufFormat{f})
		if err != nil {
			t.Fatalf("%s %#x: NewExport: %v", f.fourccString(), f.Modifier, err)
		}
		b.exp = exp.(*vkExport)
	} else {
		// The memory types as if none were the device's: the first one is
		// then taken that is not, where the device has one (NVIDIA lists its
		// system memory first).
		app.askExports()
		mem := *app.mem
		for i := range mem.MemoryTypes {
			mem.MemoryTypes[i].PropertyFlags &^= vk.MemoryPropertyDeviceLocal
		}
		format := exportVkFormat(f.Fourcc)
		var mods []vk.DmabufModifier
		for _, m := range app.exportMods[format] {
			if m.Modifier == f.Modifier {
				mods = append(mods, m)
			}
		}
		img, err := app.dev.CreateDmabufImage(&mem, format, uint32(w), uint32(h), exportUsage, mods)
		if err != nil {
			t.Fatalf("%s %#x: exporting system memory: %v", f.fourccString(), f.Modifier, err)
		}
		b.exp = &vkExport{d: app, img: img, size: image.Pt(w, h), format: f, convert: format != targetFormat}
		for _, p := range img.Planes {
			b.exp.planes = append(b.exp.planes, DmabufPlane{FD: img.FD, Offset: uint32(p.Offset), Stride: uint32(p.Stride)})
		}
		app.exports[b.exp] = struct{}{}
	}
	t.Cleanup(b.exp.Close)
	return b
}

// draw draws a picture into the buffer, and returns once the GPU has.
func (b *appBuffer) draw(pic *image.RGBA) {
	b.t.Helper()
	dl := gfxtest.Build(func(bl *gfx.Builder) {
		bl.DrawImage(gfx.NewImage(pic), gfx.Rect{}, gfx.RectXYWH(0, 0, float32(pic.Rect.Dx()), float32(pic.Rect.Dy())), gfx.SampleNearest)
	})
	if _, err := b.r.Render(b.target, dl, Options{Clear: &gfx.Color{}}); err != nil {
		b.t.Fatal(err)
	}
	if got := readAll(b.t, b.target); !bytes.Equal(got.Pix, pic.Pix) {
		b.t.Fatal("the application's target does not hold the picture it drew")
	}
	if err := b.exp.CopyFrom(b.target, pic.Rect); err != nil {
		b.t.Fatal(err)
	}
}

func (b *appBuffer) desc() DmabufDescriptor {
	return DmabufDescriptor{Width: b.exp.size.X, Height: b.exp.size.Y, Fourcc: b.exp.format.Fourcc, Modifier: b.exp.format.Modifier, Planes: b.exp.Planes()}
}

// asImported returns the pixels an importer reads of a picture: an X format
// has no alpha.
func asImported(pic *image.RGBA, f DmabufFormat) *image.RGBA {
	out := &image.RGBA{Pix: bytes.Clone(pic.Pix), Stride: pic.Stride, Rect: pic.Rect}
	if f.opaque() {
		for i := 3; i < len(out.Pix); i += 4 {
			out.Pix[i] = 255
		}
	}
	return out
}

// importScene draws an image in the ways a texture can be drawn, in
// 420x300.
func importScene(img *gfx.Image) *gfx.DisplayList {
	w, h := float32(img.Width()), float32(img.Height())
	return gfxtest.Build(func(b *gfx.Builder) {
		gfxtest.Checker(b, gfx.RectXYWH(0, 0, 420, 300), 10)
		// As it is, at a whole and at a fractional position.
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(4, 4, w/4, h/4), gfx.SampleLinear)
		b.DrawImage(img, gfx.Rect{X0: 8, Y0: 8, X1: min(40, w), Y1: min(40, h)}, gfx.RectXYWH(70.3, 4.6, 32, 32), gfx.SampleNearest)
		// Inside a rounded clip, larger.
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(110, 4, 140, 80), 18))
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(110, 4, 1.3*w/2, 1.3*h/2), gfx.SampleLinear)
		b.PopClip()
		// In an opacity layer.
		b.PushLayer(0.6, gfx.SrcOver)
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(260, 4, w/2, h/2), gfx.SampleLinear)
		b.FillRect(gfx.RectXYWH(270, 14, 30, 20), rose)
		b.PopLayer()
		// Under a rotation.
		b.PushTransform(gfx.Translate(70, 150).Mul(gfx.Rotate(0.35)).Mul(gfx.Scale(0.5, 0.45)))
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(-w/2, -h/2, w, h), gfx.SampleLinear)
		b.PopTransform()
		// In a blur layer.
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 3})
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(150, 110, w/2, h/2), gfx.SampleLinear)
		b.PopLayer()
		// With a blend mode, and under a glass panel.
		b.PushLayer(1, gfx.Multiply)
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(290, 100, w/2, h/2), gfx.SampleLinear)
		b.PopLayer()
		b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(20, 215, w, h/3), gfx.SampleLinear)
		glass(b, gfx.NewRRect(gfx.RectXYWH(40, 225, 120, 50), 12), 6, gfx.White.WithAlpha(0.2))
		// Cropped and stretched, in a path clip.
		b.PushClipPath(gfx.CirclePath(gfx.Pt(340, 240), 45), gfx.NonZero)
		b.DrawImage(img, gfx.Rect{X0: w / 4, Y0: h / 4, X1: 3 * w / 4, Y1: 3 * h / 4}, gfx.RectXYWH(290, 190, 110, 110), gfx.SampleLinear)
		b.PopClip()
	})
}

// compareImported draws the scene with an image of imported memory and
// compares it with the rasterizer's of the same pixels in main memory, at
// scale 1 and 2. It returns the largest difference.
func compareImported(t *testing.T, win *vkDevice, r *Renderer, m *Imported, pic *image.RGBA, what string) int {
	t.Helper()
	worst := 0
	frame := gfx.NewExternalImage(pic.Rect.Dx(), pic.Rect.Dy(), false, m.Frame())
	ordinary := gfx.NewImage(pic)
	for _, scale := range []float32{1, 2} {
		w, h := int(420*scale), int(300*scale)
		target, err := win.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		before := r.Memory().Images
		res, err := r.Render(target, importScene(frame), Options{Scale: scale, Clear: &gfx.White})
		if err != nil || len(res.Unsupported) > 0 {
			t.Fatalf("%s x%g: %v, not drawn: %v", what, scale, err, res.Unsupported)
		}
		if !m.Copied() && (res.UploadBytes > 1<<16 || r.Memory().Images != before) {
			t.Errorf("%s x%g: %d bytes uploaded, %d bytes of images more: an imported buffer is neither", what, scale, res.UploadBytes, r.Memory().Images-before)
		}
		gpu := readAll(t, target)
		target.Close()
		cpu := renderCPU(w, h, importScene(ordinary), scale, gfx.White)
		dump(t, strings.NewReplacer(" ", "_", "/", "_").Replace(fmt.Sprintf("import_%s_x%g", what, scale)), gpu, cpu)
		d := compare(gpu, cpu)
		if !d.WithinGPU() {
			t.Errorf("%s x%g: largest difference %d at %v, %.3f%% of the pixels over 2 levels", what, scale, d.Max, d.Worst, 100*d.Over2)
		}
		worst = max(worst, d.Max)
	}
	return worst
}

// TestImport draws, on every device, frames an application drew on every
// device: on the same one in every pair of format and modifier it imports
// and exports, on the other in the linear ones, whose memory must be the
// system's, at sizes whose rows an exporter pads and does not. The pixels
// are the rasterizer's of the same picture, within the tolerance of ADR-282.
func TestImport(t *testing.T) {
	sizes := []image.Point{{160, 100}, {131, 77}, {256, 64}}
	eachPair(t, anyPair, func(t *testing.T, app, win *vkDevice) {
		own, foreign := win.ImportFormats()
		formats := foreign
		if sameGPU(app, win) {
			formats = own
		}
		t.Logf("%s: %d pairs of its own, %d of another device's; %+v", win.Name(), len(own), len(foreign), win.DeviceID())
		exportable := app.ExportFormats()
		closedTarget, err := win.NewTarget(64, 64)
		if err != nil {
			t.Fatal(err)
		}
		defer closedTarget.Close()
		var r Renderer
		direct, copied, worst := 0, 0, 0
		for _, f := range formats {
			found := false
			for _, e := range exportable {
				found = found || e == f
			}
			if !found {
				t.Logf("%s %#x: the application's device does not export it", f.fourccString(), f.Modifier)
				continue
			}
			for i, size := range sizes {
				if i > 0 && f.Modifier != ModifierLinear {
					continue
				}
				what := fmt.Sprintf("%s %#x %dx%d", f.fourccString(), f.Modifier, size.X, size.Y)
				b := newAppBuffer(t, app, size.X, size.Y, f, !sameGPU(app, win))
				pic := appPicture(size.X, size.Y, i)
				b.draw(pic)
				fds := openFDs(t)
				m, err := win.Import(b.desc())
				if err != nil {
					t.Errorf("%s: %v", what, err)
					continue
				}
				if m.Copied() {
					copied++
				} else {
					direct++
				}
				want := asImported(pic, f)
				if m.Size() != size || m.Format() != f || m.Opaque() != f.opaque() {
					t.Errorf("%s: the import is %v %v, opaque %v", what, m.Size(), m.Format(), m.Opaque())
				}
				if got := m.Pixels(); !bytes.Equal(got, want.Pix) {
					t.Errorf("%s (copied %v): Pixels are not the picture's", what, m.Copied())
				}
				worst = max(worst, compareImported(t, win, &r, m, want, what))
				// The application draws the next frame into the same buffer.
				pic = appPicture(size.X, size.Y, i+10)
				b.draw(pic)
				worst = max(worst, compareImported(t, win, &r, m, asImported(pic, f), what+" second frame"))
				m.Close()
				m.Close()
				if m.Pixels() != nil {
					t.Errorf("%s: a closed buffer has pixels", what)
				}
				if _, err := r.Render(closedTarget, drawImages(gfx.NewExternalImage(size.X, size.Y, false, m.Frame())), Options{}); err != nil {
					t.Errorf("%s: drawing an image of closed memory: %v", what, err)
				}
				if got := openFDs(t); got != fds {
					t.Errorf("%s: %d descriptors open after the import was closed, %d before", what, got, fds)
				}
				b.exp.Close()
			}
		}
		t.Logf("%d buffers sampled where they lie, %d copied on the GPU; largest difference %d", direct, copied, worst)
		if len(win.imports) != 0 {
			t.Errorf("%d imports left", len(win.imports))
		}
	})
}

// TestImportForeignVideoMemory hands the renderer another device's own
// memory, which it may not reach: an error, at once, and the device goes
// on. (NVIDIA's video memory on i915: without the check, Mesa tries for 17
// seconds and loses the device.)
func TestImportForeignVideoMemory(t *testing.T) {
	eachPair(t, acrossGPU, func(t *testing.T, app, win *vkDevice) {
		b := newAppBuffer(t, app, 256, 64, DmabufFormat{Fourcc: FourccABGR8888}, false)
		pic := appPicture(256, 64, 0)
		b.draw(pic)
		t0 := time.Now()
		m, err := win.Import(b.desc())
		d := time.Since(t0)
		t.Logf("%s -> %s: %v in %v", app.Name(), win.Name(), err, d)
		if d > 2*time.Second {
			t.Errorf("the import took %v", d)
		}
		if err == nil {
			var r Renderer
			compareImported(t, win, &r, m, pic, "foreign video memory")
			m.Close()
		}
		if err := win.usable(); err != nil {
			t.Fatal(err)
		}
	})
}

// cpuBuffer maps a linear dmabuf of app's for the CPU, as an application
// does that draws with it.
func cpuBuffer(t *testing.T, app *vkDevice, size int) (fd int, pix []byte, sync func(end bool)) {
	t.Helper()
	b := newAppBuffer(t, app, 1024, size/4096, DmabufFormat{Fourcc: FourccABGR8888}, true)
	fd = b.exp.img.FD
	n, err := unix.Seek(fd, 0, unix.SEEK_END)
	if err != nil {
		t.Fatal(err)
	}
	pix, err = unix.Mmap(fd, 0, int(n), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		t.Skipf("the dmabuf cannot be mapped: %v", err)
	}
	t.Cleanup(func() { unix.Munmap(pix) })
	return fd, pix, func(end bool) {
		const dmaBufIoctlSync = 0x40086200 // DMA_BUF_IOCTL_SYNC
		arg := uint64(2)                   // DMA_BUF_SYNC_WRITE
		if end {
			arg |= 4 // DMA_BUF_SYNC_END
		}
		if err := ioctl(fd, dmaBufIoctlSync, unsafe.Pointer(&arg)); err != nil {
			t.Fatal(err)
		}
	}
}

// TestImportStrides draws linear buffers at every kind of stride and
// offset, which a device samples or copies: the pixels are right however
// the rows lie.
func TestImportStrides(t *testing.T) {
	eachPair(t, anyPair, func(t *testing.T, app, win *vkDevice) {
		fd, pix, sync := cpuBuffer(t, app, 75*4096)
		blank := bytes.Repeat([]byte{0xee}, len(pix))
		var r Renderer
		direct, copied := 0, 0
		for _, w := range []int{250, 256, 33, 1} {
			const h = 70
			row := 4 * w
			target, err := win.NewTarget(w, h)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[int]bool{}
			for _, stride := range []int{row, row + 4, row + 8, row + 16, (row + 31) &^ 31, (row + 63) &^ 63, (row + 127) &^ 127, (row + 255) &^ 255, (row + 4095) &^ 4095} {
				for _, off := range []int{0, 4, 64, 256, 4096} {
					if seen[stride<<16|off] {
						continue
					}
					seen[stride<<16|off] = true
					pic := appPicture(w, h, stride+off)
					sync(false)
					copy(pix, blank)
					for y := range h {
						copy(pix[off+y*stride:], pic.Pix[y*row:(y+1)*row])
					}
					sync(true)
					m, err := win.Import(DmabufDescriptor{Width: w, Height: h, Fourcc: FourccABGR8888, Planes: []DmabufPlane{{FD: fd, Offset: uint32(off), Stride: uint32(stride)}}})
					if err != nil {
						t.Fatalf("width %d, stride %d, offset %d: %v", w, stride, off, err)
					}
					if m.Copied() {
						copied++
					} else {
						direct++
					}
					dl := gfxtest.Build(func(b *gfx.Builder) {
						b.DrawImage(gfx.NewExternalImage(w, h, false, m.Frame()), gfx.Rect{}, gfx.RectXYWH(0, 0, float32(w), h), gfx.SampleNearest)
					})
					if _, err := r.Render(target, dl, Options{Clear: &gfx.Color{}}); err != nil {
						t.Fatal(err)
					}
					if got := readAll(t, target); !bytes.Equal(got.Pix, pic.Pix) {
						t.Errorf("width %d, stride %d, offset %d (copied %v): the pixels drawn are not the buffer's", w, stride, off, m.Copied())
					}
					if !bytes.Equal(m.Pixels(), pic.Pix) {
						t.Errorf("width %d, stride %d, offset %d (copied %v): the pixels read back are not the buffer's", w, stride, off, m.Copied())
					}
					m.Close()
				}
			}
			target.Close()
		}
		t.Logf("%d layouts sampled where they lie, %d copied on the GPU", direct, copied)
	})
}

// TestImportMalformed hands the device descriptors that are wrong in every
// way: each is an error, and the device draws on.
func TestImportMalformed(t *testing.T) {
	eachPair(t, onGPU, func(t *testing.T, app, win *vkDevice) {
		const w, h = 160, 100
		var r Renderer
		own, _ := win.ImportFormats()
		file, err := os.CreateTemp(t.TempDir(), "file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := file.Truncate(1 << 20); err != nil {
			t.Fatal(err)
		}
		memfd, err := unix.MemfdCreate("tela test", unix.MFD_CLOEXEC)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(memfd)
		unix.Ftruncate(memfd, 1<<20)
		// Every layout of the first format, the linear one among them.
		for _, f := range own {
			if f.Fourcc != FourccARGB8888 {
				continue
			}
			b := newAppBuffer(t, app, w, h, f, false)
			pic := appPicture(w, h, 3)
			b.draw(pic)
			other := newAppBuffer(t, app, w, h, f, false)
			good := b.desc()
			size, err := unix.Seek(good.Planes[0].FD, 0, unix.SEEK_END)
			if err != nil {
				t.Fatal(err)
			}
			closed := func() int {
				fd, err := unix.FcntlInt(uintptr(good.Planes[0].FD), unix.F_DUPFD_CLOEXEC, 0)
				if err != nil {
					t.Fatal(err)
				}
				unix.Close(fd)
				return fd
			}()
			fds := openFDs(t)
			for _, c := range []struct {
				name string
				edit func(d *DmabufDescriptor)
			}{
				{"no size", func(d *DmabufDescriptor) { d.Width, d.Height = 0, 0 }},
				{"a negative size", func(d *DmabufDescriptor) { d.Width = -w }},
				{"wider than its stride", func(d *DmabufDescriptor) { d.Width = int(d.Planes[0].Stride)/4 + 1 }},
				{"twice as high", func(d *DmabufDescriptor) { d.Height = 100 * h }},
				{"larger than any texture", func(d *DmabufDescriptor) { d.Width, d.Height = 1<<20, 1<<20 }},
				{"an overflowing size", func(d *DmabufDescriptor) { d.Width, d.Height = 1<<62, 1<<62 }},
				{"no format", func(d *DmabufDescriptor) { d.Fourcc = 0 }},
				{"NV12", func(d *DmabufDescriptor) { d.Fourcc = 0x3231564e }},
				{"an implicit modifier", func(d *DmabufDescriptor) { d.Modifier = ModifierInvalid }},
				{"an unknown modifier", func(d *DmabufDescriptor) { d.Modifier = 0x7f00000000000123 }},
				{"no planes", func(d *DmabufDescriptor) { d.Planes = nil }},
				{"a plane more", func(d *DmabufDescriptor) { d.Planes = append(d.Planes, d.Planes[0]) }},
				{"five planes", func(d *DmabufDescriptor) {
					d.Planes = []DmabufPlane{d.Planes[0], d.Planes[0], d.Planes[0], d.Planes[0], d.Planes[0]}
				}},
				{"no descriptor", func(d *DmabufDescriptor) { d.Planes[0].FD = -1 }},
				{"a closed descriptor", func(d *DmabufDescriptor) { d.Planes[0].FD = closed }},
				{"a file", func(d *DmabufDescriptor) { d.Planes[0].FD = int(file.Fd()) }},
				{"a memfd", func(d *DmabufDescriptor) { d.Planes[0].FD = memfd }},
				{"no stride", func(d *DmabufDescriptor) { d.Planes[0].Stride = 0 }},
				{"a stride shorter than a row", func(d *DmabufDescriptor) { d.Planes[0].Stride = 4*w - 4 }},
				{"a stride of no whole pixels", func(d *DmabufDescriptor) { d.Planes[0].Stride += 2 }},
				{"a huge stride", func(d *DmabufDescriptor) { d.Planes[0].Stride = 1<<32 - 4 }},
				{"a stride that does not fit", func(d *DmabufDescriptor) { d.Planes[0].Stride *= 4 }},
				{"an offset at the end", func(d *DmabufDescriptor) { d.Planes[0].Offset = uint32(size) }},
				{"an offset near the end", func(d *DmabufDescriptor) { d.Planes[0].Offset = uint32(size) - 4096 }},
				{"a huge offset", func(d *DmabufDescriptor) { d.Planes[0].Offset = 1<<32 - 4 }},
				{"an odd offset", func(d *DmabufDescriptor) { d.Planes[0].Offset = 3 }},
				{"the second plane's offset", func(d *DmabufDescriptor) {
					if len(d.Planes) < 2 {
						d.Planes = nil
						return
					}
					d.Planes[1].Offset += 100 << 20
				}},
				{"the second plane's stride", func(d *DmabufDescriptor) {
					if len(d.Planes) < 2 || validating() { // Mesa writes a warning
						d.Planes = nil
						return
					}
					d.Planes[1].Stride = 1
				}},
				{"planes of two dmabufs", func(d *DmabufDescriptor) {
					if len(d.Planes) < 2 {
						d.Planes = nil
						return
					}
					d.Planes[1].FD = other.exp.img.FD
				}},
			} {
				d := good
				d.Planes = append([]DmabufPlane(nil), good.Planes...)
				c.edit(&d)
				m, err := win.Import(d)
				if err == nil {
					t.Errorf("%#x, %s: no error", f.Modifier, c.name)
					m.Close()
					continue
				}
				if errors.Is(err, ErrDeviceLost) || win.usable() != nil {
					t.Fatalf("%#x, %s: the device is lost: %v", f.Modifier, c.name, err)
				}
				t.Logf("%#x, %s: %v", f.Modifier, c.name, err)
			}
			if got := openFDs(t); got != fds {
				t.Errorf("%#x: %d descriptors open after the refusals, %d before", f.Modifier, got, fds)
			}
			// The good one still goes.
			m, err := win.Import(good)
			if err != nil {
				t.Fatal(err)
			}
			compareImported(t, win, &r, m, pic, fmt.Sprintf("after the malformed ones %#x", f.Modifier))
			m.Close()
		}
	})
}

// TestImportLeak imports and closes a thousand buffers, drawing some: no
// descriptor, import or descriptor pool is left.
func TestImportLeak(t *testing.T) {
	eachPair(t, onGPU, func(t *testing.T, app, win *vkDevice) {
		own, _ := win.ImportFormats()
		var r Renderer
		target, err := win.NewTarget(200, 100)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		b := newAppBuffer(t, app, 160, 100, own[0], false)
		// And one the device copies, if there is such a layout.
		odd := newAppBuffer(t, app, 160, 100, DmabufFormat{Fourcc: FourccARGB8888}, false)
		oddDesc := odd.desc()
		oddDesc.Height, oddDesc.Planes[0].Offset = 90, 4
		b.draw(appPicture(160, 100, 0))
		odd.draw(appPicture(160, 100, 0))
		cycle := func(i int) {
			desc := b.desc()
			if i%2 == 1 {
				desc = oddDesc
			}
			m, err := win.Import(desc)
			if err != nil {
				t.Fatal(err)
			}
			if i%5 == 0 {
				img := gfx.NewExternalImage(desc.Width, desc.Height, true, m.Frame())
				if _, err := r.Render(target, drawImages(img), Options{Clear: &gfx.White}); err != nil {
					t.Fatal(err)
				}
				r.EndFrame()
			}
			m.Close()
		}
		for i := range 2 * idleFrames * 5 {
			cycle(i)
		}
		fds, pools, images := openFDs(t), len(win.imagePools), len(win.images)
		for i := range 1000 {
			cycle(i)
		}
		if got := openFDs(t); got != fds {
			t.Errorf("%d descriptors open after 1000 imports, %d before", got, fds)
		}
		if len(win.imports) != 0 || len(win.imagePools) != pools || len(win.images) > images {
			t.Errorf("%d imports, %d descriptor pools (%d before), %d images (%d before)", len(win.imports), len(win.imagePools), pools, len(win.images), images)
		}
		// An import left open goes with the device (the validation layer
		// reports what does not).
		if _, err := win.Import(b.desc()); err != nil {
			t.Fatal(err)
		}
		if _, err := win.Import(oddDesc); err != nil {
			t.Fatal(err)
		}
	})
}

// TestImportFrames draws the frames of three buffers as a window does, by
// regions, some twice: the device makes nothing per frame, and what is not
// drawn again keeps its pixels.
func TestImportFrames(t *testing.T) {
	eachPair(t, anyPair, func(t *testing.T, app, win *vkDevice) {
		const w, h = 131, 77 // a stride some devices copy
		f := DmabufFormat{Fourcc: FourccXRGB8888}
		if sameGPU(app, win) {
			own, _ := win.ImportFormats()
			for _, o := range own {
				if o.Fourcc == f.Fourcc {
					f = o // the first: tiled
					break
				}
			}
		}
		var bufs []*appBuffer
		var ims []*Imported
		for range 3 {
			b := newAppBuffer(t, app, w, h, f, !sameGPU(app, win))
			b.draw(appPicture(w, h, 0))
			desc := b.desc()
			desc.Stream = 5
			m, err := win.Import(desc)
			if err != nil {
				t.Fatal(err)
			}
			bufs, ims = append(bufs, b), append(ims, m)
		}
		target, err := win.NewTarget(200, 120)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		made := 0
		for n := range 12 {
			if n == 4 {
				made = win.made
			}
			b, m := bufs[n%3], ims[n%3]
			pic := appPicture(w, h, n)
			b.draw(pic)
			img := gfx.NewExternalImage(w, h, true, m.Frame())
			scene := func(img *gfx.Image) *gfx.DisplayList {
				return gfxtest.Build(func(b *gfx.Builder) {
					b.FillRect(gfx.RectXYWH(0, 0, 200, 120), sky)
					b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(20, 20, w, h), 14))
					b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(20, 20, w, h), gfx.SampleLinear)
					b.PopClip()
				})
			}
			dl := scene(img)
			// The texture's damage in two rectangles, then one of them again.
			for _, rc := range []image.Rectangle{image.Rect(0, 0, 100, 120), image.Rect(100, 0, 200, 120), image.Rect(30, 30, 60, 50)} {
				if _, err := r.Render(target, dl, Options{Region: rc, Clear: &gfx.White}); err != nil {
					t.Fatal(err)
				}
			}
			r.EndFrame()
			want := renderCPU(200, 120, scene(gfx.NewImage(asImported(pic, f))), 1, gfx.White)
			within(t, fmt.Sprintf("frame %d (copied %v)", n, m.Copied()), readAll(t, target), want)
			if n == 3 {
				// Measured, a frame is several submissions.
				win.SetProfiling(true)
				if _, err := r.Render(target, dl, Options{Clear: &gfx.White}); err != nil {
					t.Fatal(err)
				}
				win.SetProfiling(false)
				within(t, "a frame measured", readAll(t, target), want)
			}
			if images := r.Memory().Images; m.Copied() && images != 4*w*h || !m.Copied() && images != 0 {
				t.Errorf("frame %d (copied %v): %d bytes of images", n, m.Copied(), images)
			}
		}
		if win.made != made {
			t.Errorf("%d images created by the frames after the first ones", win.made-made)
		}
		// A rest keeps what is on screen, and needs nothing of the imports.
		r.Rest()
		for _, m := range ims {
			m.Close()
		}
	})
}

// TestImportPixels reads buffers back from several goroutines while the
// device draws, with a buffer another device imported drawn by its pixels,
// and on a device that is lost or closed.
func TestImportPixels(t *testing.T) {
	eachPair(t, onGPU, func(t *testing.T, app, win *vkDevice) {
		own, _ := win.ImportFormats()
		const w, h = 131, 77
		b := newAppBuffer(t, app, w, h, own[0], false)
		pic := appPicture(w, h, 1)
		b.draw(pic)
		m, err := win.Import(b.desc())
		if err != nil {
			t.Fatal(err)
		}
		want := asImported(pic, own[0])
		target, err := win.NewTarget(200, 120)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				for range 20 {
					if !bytes.Equal(m.Pixels(), want.Pix) {
						t.Error("the pixels read back are not the buffer's")
						return
					}
				}
			})
		}
		var r Renderer
		dl := func(img *gfx.Image) *gfx.DisplayList {
			return gfxtest.Build(func(b *gfx.Builder) {
				b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(20, 20, w, h), gfx.SampleLinear)
			})
		}
		for range 40 {
			if _, err := r.Render(target, dl(gfx.NewExternalImage(w, h, false, m.Frame())), Options{Clear: &gfx.White}); err != nil {
				t.Fatal(err)
			}
		}
		wg.Wait()
		cpu := renderCPU(200, 120, dl(gfx.NewImage(want)), 1, gfx.White)
		within(t, "drawn while read", readAll(t, target), cpu)
		// The rasterizer draws it too, as a window does that fell back.
		if d := compare(renderCPU(200, 120, dl(gfx.NewExternalImage(w, h, false, m.Frame())), 1, gfx.White), cpu); d.Max != 0 {
			t.Errorf("the rasterizer draws the buffer with a difference of %d", d.Max)
		}
		// On another device, it is an image like any.
		at, err := app.NewTarget(200, 120)
		if err != nil {
			t.Fatal(err)
		}
		defer at.Close()
		var ar Renderer
		res, err := ar.Render(at, dl(gfx.NewExternalImage(w, h, false, m.Frame())), Options{Clear: &gfx.White})
		if err != nil || res.UploadBytes != 4*w*h {
			t.Errorf("drawn by another device: %v, %d bytes uploaded", err, res.UploadBytes)
		}
		within(t, "drawn by another device", readAll(t, at), cpu)
		// A frame encoded, and the buffer closed before it is drawn.
		var e Encoder
		e.dev = win
		f := e.Encode(dl(gfx.NewExternalImage(w, h, false, m.Frame())), target.Size(), Options{})
		m.Close()
		if err := target.Draw(f); err == nil {
			t.Error("a frame that reads a closed buffer was drawn")
		}
		if _, err := win.Import(b.desc()); err != nil {
			t.Error(err)
		}
		// A lost device.
		m, err = win.Import(b.desc())
		if err != nil {
			t.Fatal(err)
		}
		win.lost.Store(true)
		if m.Pixels() != nil {
			t.Error("a lost device gave pixels")
		}
		if _, err := win.Import(b.desc()); !errors.Is(err, ErrDeviceLost) {
			t.Errorf("an import on a lost device: %v", err)
		}
		if own, foreign := win.ImportFormats(); own != nil || foreign != nil {
			t.Error("a lost device imports formats")
		}
		m.Close()
		if m.Pixels() != nil || len(win.held) != 1 {
			t.Errorf("a buffer closed on a lost device: %d held for its Close", len(win.held))
		}
		left, err := app.Import(b.desc())
		if err != nil {
			t.Fatal(err)
		}
		win.Close()
		app.Close()
		if left.Pixels() != nil {
			t.Error("a closed device gave pixels")
		}
		left.Close()
		if _, err := app.Import(b.desc()); err == nil {
			t.Error("an import on a closed device returned no error")
		}
	})
}

// TestStreamTextures draws a stream of CPU frames: no image is created per
// frame, and the stream holds one texture.
func TestStreamTextures(t *testing.T) {
	const w, h = 640, 480 // more than a small upload
	eachDevice(t, func(t *testing.T, dev Device) {
		d := dev.(*vkDevice)
		target, err := dev.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		made, staging := 0, vk.Buffer(0)
		for n := range 2 * idleFrames {
			if n == 2 {
				made, staging = d.made, d.staging.buf
			}
			pic := appPicture(w, h, n)
			dl := func(img *gfx.Image) *gfx.DisplayList {
				return gfxtest.Build(func(b *gfx.Builder) {
					b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(0, 0, w, h), gfx.SampleNearest)
				})
			}
			list := dl(gfx.NewStreamImage(w, h, pic.Pix, false, 3))
			for range 2 {
				if _, err := r.Render(target, list, Options{Clear: &gfx.Color{}}); err != nil {
					t.Fatal(err)
				}
			}
			r.EndFrame()
			if got := readAll(t, target); !bytes.Equal(got.Pix, pic.Pix) {
				t.Fatalf("frame %d: the pixels are not its", n)
			}
			if got := r.Memory().Images; got != 4*w*h {
				t.Fatalf("frame %d: %d bytes of images, want one texture's", n, got)
			}
		}
		if d.made != made || d.staging.buf != staging {
			t.Errorf("after the second frame %d images were created, and the buffer of uploads made again: %v", d.made-made, d.staging.buf != staging)
		}
		// A smaller frame takes none of another size, and the spare goes.
		small := appPicture(100, 50, 1)
		if _, err := r.Render(target, drawImages(gfx.NewStreamImage(100, 50, small.Pix, false, 3)), Options{}); err != nil {
			t.Fatal(err)
		}
		if d.made != made+1 || len(d.spare) != 0 || len(d.images) != 1 {
			t.Errorf("a frame of another size: %d images created, %d spare, %d held", d.made-made, len(d.spare), len(d.images))
		}
		// The buffer of uploads goes when they stop.
		for range idleFrames + 2 {
			if _, err := r.Render(target, nil, Options{Clear: &gfx.White}); err != nil {
				t.Fatal(err)
			}
		}
		if n := len(d.staging.bytes); n != 0 {
			t.Errorf("%d bytes kept for uploads after frames that upload nothing", n)
		}
	})
}

// BenchmarkImport measures a window frame that shows a new 1280x720 frame of
// an application's GPU memory through a rounded clip, as BenchmarkStream
// does with pixels: memory of the same GPU, of the other one as its rows
// lie, and of the other one with rows the device has to copy.
func BenchmarkImport(b *testing.B) {
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
	for _, a := range devices {
		for _, w := range devices {
			for _, off := range []uint32{0, 4} {
				ad, err := OpenVulkan(inst, a)
				if err != nil {
					b.Fatal(err)
				}
				wd, err := OpenVulkan(inst, w)
				if err != nil {
					b.Fatal(err)
				}
				app, win := ad.(*vkDevice), wd.(*vkDevice)
				name := fmt.Sprintf("%s_to_%s/offset_%d", deviceLabel(a.Name), deviceLabel(w.Name), off)
				b.Run(name, func(b *testing.B) { benchImport(b, app, win, off) })
				win.Close()
				app.Close()
			}
		}
	}
}

func benchImport(b *testing.B, app, win *vkDevice, off uint32) {
	const w, h = 1280, 720
	f := DmabufFormat{Fourcc: FourccABGR8888}
	if own, _ := win.ImportFormats(); sameGPU(app, win) && off == 0 {
		f = own[0]
	}
	buf := newAppBuffer(b, app, w, h+1, f, !sameGPU(app, win))
	if err := buf.exp.CopyFrom(buf.target); err != nil {
		b.Fatal(err)
	}
	desc := buf.desc()
	desc.Height, desc.Planes[0].Offset = h, off
	m, err := win.Import(desc)
	if err != nil {
		b.Skip(err)
	}
	defer m.Close()
	target, err := win.NewTarget(1920, 1080)
	if err != nil {
		b.Fatal(err)
	}
	defer target.Close()
	var r Renderer
	for b.Loop() {
		img := gfx.NewExternalImage(w, h, true, m.Frame())
		dl := gfxtest.Build(func(b *gfx.Builder) {
			b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(100, 100, w, h), 24))
			b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(100, 100, w, h), gfx.SampleLinear)
			b.PopClip()
		})
		if _, err := r.Render(target, dl, Options{Region: image.Rect(100, 100, 100+w, 100+h), Clear: &gfx.White}); err != nil {
			b.Fatal(err)
		}
		r.EndFrame()
	}
	copied := 0.0
	if m.Copied() {
		copied = 1
	}
	b.ReportMetric(copied, "copied")
}
