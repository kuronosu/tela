//go:build telagpu && linux

package gpu

import (
	"errors"
	"image"
	"os"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/vk"
)

// openFDs counts the file descriptors the process has open.
func openFDs(t *testing.T) int {
	t.Helper()
	list, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("no /proc/self/fd: %v", err)
	}
	return len(list)
}

// TestExport copies a target into images exported as dmabufs, on every
// device and with every pair of format and modifier it exports: the whole
// frame at first, then rectangles of frames that differ (the pixels
// outside them are kept), one image behind the other by a frame as the
// buffers of a swapchain are, and reads each image back, which must give
// the target's pixels bit for bit: it is a copy, in either byte order.
// No file descriptor is left open.
func TestExport(t *testing.T) {
	const w, h = 257, 131
	eachDevice(t, func(t *testing.T, dev Device) {
		exp, ok := dev.(Exporter)
		if !ok {
			t.Skip("the device is not an Exporter")
		}
		formats := exp.ExportFormats()
		if len(formats) == 0 {
			t.Skipf("%s exports no image", dev.Name())
		}
		if nodes := exp.DRMNodes(); len(nodes) == 0 {
			t.Logf("%s has no DRM node", dev.Name())
		}
		fds := openFDs(t)
		target, err := dev.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		draw := func(c gfx.Color, region image.Rectangle) {
			t.Helper()
			dl := gfxtest.Build(func(b *gfx.Builder) {
				b.FillRect(gfx.RectXYWH(0, 0, w, h), c)
				b.FillRect(gfx.RectXYWH(30, 20, 90, 50), gfx.RGBA8(230, 50, 25, 255))
				b.FillRect(gfx.RectXYWH(150, 60, 70, 60), gfx.RGBA8(13, 38, 121, 128))
			})
			if _, err := r.Render(target, dl, Options{Region: region, Clear: &gfx.Color{}}); err != nil {
				t.Fatal(err)
			}
		}
		same := func(e Export, what string) {
			t.Helper()
			want := readAll(t, target)
			got := image.NewRGBA(want.Rect)
			if err := e.ReadPixels(got); err != nil {
				t.Fatalf("%s: ReadPixels: %v", what, err)
			}
			if m := compare(got, want); m.Max != 0 {
				t.Fatalf("%s: %d pixels of the exported image differ from the target, the first at %v: %v, want %v", what, m.Over0, m.Worst,
					got.RGBAAt(m.Worst.X, m.Worst.Y), want.RGBAAt(m.Worst.X, m.Worst.Y))
			}
		}
		for _, f := range formats {
			name := (DmabufFormat{Fourcc: f.Fourcc}).fourccString()
			draw(gfx.RGBA8(51, 153, 102, 255), image.Rectangle{})
			a, err := exp.NewExport(w, h, []DmabufFormat{f})
			if err != nil {
				t.Fatalf("%s %#x: NewExport: %v", name, f.Modifier, err)
			}
			if a.Format() != f || a.Size() != image.Pt(w, h) || len(a.Planes()) == 0 {
				t.Fatalf("%s %#x: the image is %v %+v with %d planes", name, f.Modifier, a.Size(), a.Format(), len(a.Planes()))
			}
			for i, p := range a.Planes() {
				if p.FD < 0 || p.Stride == 0 {
					t.Errorf("%s %#x: plane %d is %+v", name, f.Modifier, i, p)
				}
			}
			if err := a.ReadPixels(image.NewRGBA(image.Rect(0, 0, w, h))); err == nil {
				t.Errorf("%s %#x: ReadPixels before the first copy returned no error", name, f.Modifier)
			}
			// The first copy is whole, whatever rectangles are named.
			if err := a.CopyFrom(target, image.Rect(0, 0, 4, 4)); err != nil {
				t.Fatalf("%s %#x: CopyFrom: %v", name, f.Modifier, err)
			}
			same(a, name+" first copy")
			b, err := exp.NewExport(w, h, []DmabufFormat{f})
			if err != nil {
				t.Fatal(err)
			}
			if err := b.CopyFrom(target); err != nil {
				t.Fatal(err)
			}
			// Frames that change rectangles; each image gets the damage
			// of the frames since its last copy.
			r1, r2, r3 := image.Rect(20, 10, 130, 80), image.Rect(140, 50, 257, 131), image.Rect(0, 100, 60, 131)
			draw(gfx.RGBA8(180, 180, 25, 255), r1)
			if err := a.CopyFrom(target, r1); err != nil {
				t.Fatal(err)
			}
			same(a, name+" one rectangle")
			draw(gfx.RGBA8(25, 25, 76, 255), r2)
			if err := b.CopyFrom(target, r1, r2, image.Rect(-10, -10, 3, 3), image.Rectangle{}); err != nil {
				t.Fatal(err)
			}
			same(b, name+" two frames behind")
			draw(gfx.RGBA8(32, 58, 58, 64), r3) // translucent over the clear: alpha below 255
			if err := a.CopyFrom(target, r2, r3); err != nil {
				t.Fatal(err)
			}
			same(a, name+" the other buffer")
			if err := a.CopyFrom(target); err != nil { // nothing changed
				t.Fatal(err)
			}
			same(a, name+" nothing to copy")
			// A target of another size, or of no device, is refused.
			other, err := dev.NewTarget(w+1, h)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.CopyFrom(other, image.Rect(0, 0, 8, 8)); err == nil {
				t.Errorf("%s: a copy from a target of another size returned no error", name)
			}
			other.Close()
			a.Close()
			a.Close()
			if err := a.CopyFrom(target); err == nil {
				t.Errorf("%s: a copy into a closed image returned no error", name)
			}
			b.Close()
		}
		// A size beyond the device's, and formats it does not export.
		if _, err := exp.NewExport(dev.MaxTargetSize()+1, 8, formats); err == nil {
			t.Error("an image larger than the device's textures returned no error")
		}
		if _, err := exp.NewExport(w, h, []DmabufFormat{{Fourcc: FourccXRGB8888, Modifier: ModifierInvalid}, {Fourcc: 0x30335258, Modifier: 0}}); !errors.Is(err, ErrNoExport) {
			t.Errorf("an image in formats the device does not export: %v, want ErrNoExport", err)
		}
		// An image left open is destroyed with the device (the validation
		// layer reports what is not); its descriptor is closed then.
		if _, err := exp.NewExport(w, h, formats); err != nil {
			t.Fatal(err)
		}
		if got := openFDs(t); got != fds+1 {
			t.Errorf("%d file descriptors open with one image alive, %d before any", got, fds)
		}
		t.Cleanup(func() {
			if got := openFDs(t); got > fds {
				t.Errorf("%d file descriptors open after the device closed, %d before the first image", got, fds)
			}
		})
		t.Logf("%s: %d pairs of format and modifier exported and copied", dev.Name(), len(formats))
	})
}

// TestSelectDevice checks the device Open opens: a name wins over the
// display server's device, which wins over the first discrete one, which
// wins over the first integrated one, and a software device is refused
// when only a hardware one is wanted.
func TestSelectDevice(t *testing.T) {
	devices := []vk.DeviceInfo{
		{Name: "Intel UHD", Type: vk.PhysicalDeviceTypeIntegratedGPU, DRM: vk.DRMNodes{HasPrimary: true, PrimaryMajor: 226, PrimaryMinor: 0, HasRender: true, RenderMajor: 226, RenderMinor: 128}},
		{Name: "NVIDIA RTX", Type: vk.PhysicalDeviceTypeDiscreteGPU, DRM: vk.DRMNodes{HasPrimary: true, PrimaryMajor: 226, PrimaryMinor: 1, HasRender: true, RenderMajor: 226, RenderMinor: 129}},
		{Name: "llvmpipe", Type: vk.PhysicalDeviceTypeCPU},
	}
	node := func(minor uint32) Config { return Config{DRM: DRMNode{Major: 226, Minor: minor}, HasDRM: true} }
	hardware := func(c Config) Config { c.HardwareOnly = true; return c }
	integrated := []vk.DeviceInfo{devices[2], devices[0]}
	for _, c := range []struct {
		name string
		cfg  Config
		env  string
		want int
		err  error
	}{
		{"default: the first discrete", Config{}, "", 1, nil},
		{"the server's render node", node(128), "", 0, nil},
		{"the server's primary node", node(1), "", 1, nil},
		{"a node of no device", node(7), "", 1, nil},
		{"the environment wins over the server", node(128), "llvm", 2, nil},
		{"the configuration wins over both", Config{Device: "nvidia", DRM: DRMNode{Major: 226, Minor: 128}, HasDRM: true}, "llvm", 1, nil},
		{"hardware only: the discrete one", hardware(Config{}), "", 1, nil},
		{"hardware only: the server's integrated device", hardware(node(0)), "", 0, nil},
		{"hardware only: an integrated device by name", hardware(Config{}), "intel", 0, nil},
		{"hardware only: a software device by name", hardware(Config{}), "2", 0, ErrSoftwareDevice},
	} {
		n, err := selectDevice(devices, c.cfg, c.env)
		if !errors.Is(err, c.err) || err == nil && n != c.want || c.err == nil && err != nil {
			t.Errorf("%s: selectDevice = %d, %v; want %d, %v", c.name, n, err, c.want, c.err)
		}
	}
	if _, err := selectDevice(devices, Config{}, "no such device"); err == nil {
		t.Error("a name of no device returned no error")
	}
	// Without a discrete device, the first integrated one, before a
	// software one listed first.
	if n, err := selectDevice(integrated, hardware(Config{}), ""); n != 1 || err != nil {
		t.Errorf("software and integrated devices: selectDevice = %d, %v; want 1 (the integrated one)", n, err)
	}
}
