//go:build telagpu

package gputest

import (
	"image"
	"sync"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/vk"
)

// App is an application that renders frames with a device of its own, of the
// GPU that draws the windows or of another one.
type App struct {
	Dev gpu.Device
	// Same reports that its device is of the same GPU as the windows'.
	Same bool

	inst      *vk.Instance
	info, win vk.DeviceInfo
	mu        sync.Mutex // the device draws for one goroutine at a time
}

// Window opens a device that draws windows, which the caller may close, and
// which the test closes.
func (a *App) Window(t testing.TB) gpu.Device {
	t.Helper()
	dev, err := gpu.OpenVulkan(a.inst, a.win)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dev.Close)
	return dev
}

// EachPair runs f as a subtest for every ordered pair of physical devices,
// with an application on the first one, for windows drawn by the second one
// (App.Window); same asks for the pairs of one GPU only. What Vulkan reports
// is treated as EachDevice does. It skips the pairs of a device that cannot
// share dmabufs.
func EachPair(t *testing.T, same bool, f func(t *testing.T, app *App)) {
	t.Helper()
	if err := vk.Load(); err != nil {
		t.Skipf("Vulkan is not available: %v", err)
	}
	msgs := &messages{}
	inst, err := vk.NewInstance(vk.InstanceConfig{AppName: "tela gpu tests", Validation: Validating(), Debug: msgs.add})
	if err != nil {
		if Validating() {
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
	for i, a := range devices {
		for j, w := range devices {
			if same && i != j {
				continue
			}
			t.Run(Label(a.Name)+"_to_"+Label(w.Name), func(t *testing.T) {
				t.Cleanup(func() { msgs.report(t) }) // once the devices closed
				dev, err := gpu.OpenVulkan(inst, a)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(dev.Close)
				app := &App{Dev: dev, Same: i == j, inst: inst, info: a, win: w}
				win := app.Window(t)
				if imp, ok := win.(gpu.Importer); !ok {
					t.Skipf("%s imports no dmabufs", w.Name)
				} else if own, _ := imp.ImportFormats(); len(own) == 0 {
					t.Skipf("%s imports no dmabufs", w.Name)
				}
				win.Close()
				if exp, ok := dev.(gpu.Exporter); !ok || len(exp.ExportFormats()) == 0 {
					t.Skipf("%s exports no dmabufs", a.Name)
				}
				f(t, app)
			})
		}
	}
}

// Buffer is a buffer of an application's frames, in a dmabuf.
type Buffer struct {
	Width, Height int
	Format        gpu.DmabufFormat
	Planes        []gpu.DmabufPlane
	// Draw draws a picture of the buffer's size into it (premultiplied), and
	// returns once it is finished. Any goroutine.
	Draw func(pic *image.RGBA)
}

// NewBuffer allocates a buffer of a format that the application's device
// exports and draws.
func (a *App) NewBuffer(t testing.TB, w, h int, f gpu.DmabufFormat) *Buffer {
	t.Helper()
	target, err := a.Dev.NewTarget(w, h)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(target.Close)
	exp, err := a.Dev.(gpu.Exporter).NewExport(w, h, []gpu.DmabufFormat{f})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(exp.Close)
	var r gpu.Renderer
	return &Buffer{Width: w, Height: h, Format: exp.Format(), Planes: exp.Planes(), Draw: func(pic *image.RGBA) {
		a.mu.Lock()
		defer a.mu.Unlock()
		dl := gfxtest.Build(func(b *gfx.Builder) {
			b.DrawImage(gfx.NewImage(pic), gfx.Rect{}, gfx.RectXYWH(0, 0, float32(w), float32(h)), gfx.SampleNearest)
		})
		_, err := r.Render(target, dl, gpu.Options{Clear: &gfx.Color{}})
		if err == nil {
			err = exp.CopyFrom(target, pic.Rect)
		}
		if err != nil {
			t.Error(err)
		}
	}}
}

// Exports reports whether the application's device exports a format.
func (a *App) Exports(f gpu.DmabufFormat) bool {
	for _, e := range a.Dev.(gpu.Exporter).ExportFormats() {
		if e == f {
			return true
		}
	}
	return false
}

// NewSystemBuffer allocates a linear buffer in system memory, which every GPU
// reaches, with rows of stride bytes from an offset, and draws it with the
// CPU.
func (a *App) NewSystemBuffer(t testing.TB, w, h int, fourcc uint32, offset, stride int) *Buffer {
	t.Helper()
	const usage = vk.ImageUsageTransferDst
	extensions, ok := vk.DmabufExtensions(&a.info)
	if !ok {
		t.Skipf("%s lacks the dmabuf extensions", a.info.Name)
	}
	priority := float32(1)
	count, names := vk.CStringArray(extensions)
	dev, err := a.inst.CreateDevice(a.info.Device, &vk.DeviceCreateInfo{
		SType:                vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos: &vk.DeviceQueueCreateInfo{
			SType:            vk.StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: a.info.GraphicsQueueFamily,
			QueueCount:       1,
			PQueuePriorities: &priority,
		},
		EnabledExtensionCount:   count,
		PpEnabledExtensionNames: names,
		PEnabledFeatures:        &vk.PhysicalDeviceFeatures{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dev.Destroy)
	// The first memory type that is not the device's own, where it has one.
	mem := *a.inst.GetPhysicalDeviceMemoryProperties(a.info.Device)
	for i := range mem.MemoryTypes {
		mem.MemoryTypes[i].PropertyFlags &^= vk.MemoryPropertyDeviceLocal
	}
	var linear []vk.DmabufModifier
	for _, m := range a.inst.DmabufModifiers(a.info.Device, vk.FormatR8G8B8A8Unorm, usage) {
		if m.Modifier == gpu.ModifierLinear {
			linear = append(linear, m)
		}
	}
	// Enough rows of 4 KiB for the picture, wherever its rows lie.
	img, err := dev.CreateDmabufImage(&mem, vk.FormatR8G8B8A8Unorm, 1024, uint32((offset+stride*h)/4096+1), usage, linear)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dev.DestroyDmabufImage(img) })
	size, err := unix.Seek(img.FD, 0, unix.SEEK_END)
	if err != nil {
		t.Fatal(err)
	}
	pix, err := unix.Mmap(img.FD, 0, int(size), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		t.Skipf("the dmabuf cannot be mapped: %v", err)
	}
	t.Cleanup(func() { unix.Munmap(pix) })
	sync := func(flags uint64) {
		for {
			_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(img.FD), 0x40086200, uintptr(unsafe.Pointer(&flags))) // DMA_BUF_IOCTL_SYNC
			switch errno {
			case 0:
				return
			case unix.EINTR, unix.EAGAIN:
			default:
				t.Errorf("DMA_BUF_IOCTL_SYNC: %v", errno)
				return
			}
		}
	}
	swap := fourcc == gpu.FourccARGB8888 || fourcc == gpu.FourccXRGB8888
	return &Buffer{
		Width: w, Height: h,
		Format: gpu.DmabufFormat{Fourcc: fourcc},
		Planes: []gpu.DmabufPlane{{FD: img.FD, Offset: uint32(offset), Stride: uint32(stride)}},
		Draw: func(pic *image.RGBA) {
			sync(2) // DMA_BUF_SYNC_WRITE
			for y := range h {
				row := pix[offset+y*stride:][:4*w]
				copy(row, pic.Pix[y*pic.Stride:])
				if swap {
					for i := 0; i < len(row); i += 4 {
						row[i], row[i+2] = row[i+2], row[i]
					}
				}
			}
			sync(2 | 4) // DMA_BUF_SYNC_END
		},
	}
}
