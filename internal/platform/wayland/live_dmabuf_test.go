//go:build linux && telagpu

package wayland

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/vk"
)

// The live check of dmabuf.go, with the Vulkan bindings on the other side
// (ADR-281, ADR-283): what the unit tests cannot tell is whether a real
// compositor imports what a real driver exports. It needs TELA_LIVE=1 and
// the tag telagpu:
//
//	go test -c -tags telagpu -o bin/wayland.test ./internal/platform/wayland
//	TELA_LIVE=1 bin/wayland.test -test.run TestLiveDmabuf -test.v
//
// (a test binary in a sandboxed temporary directory is not visible to
// Hyprland). The windows open silently on the workspace nobody looks at,
// as the other live tests' do, and take no focus and no input.
//
// A window on a hidden workspace is suspended: the compositor sends it no
// frame callbacks, so the test presents from a timer, and nothing shows
// its pixels: grim reads outputs, and Hyprland 0.56 answers neither
// ext-image-copy-capture-v1 (with a toplevel source) nor its own
// hyprland_toplevel_export_manager_v1 for a window it does not draw (no
// frame within 12 s; tried on 2026-11-15). What the test checks then is
// the protocol: every buffer is taken, no protocol error, releases
// arrive, and no buffer is reused before its release. When the workspace
// is on a monitor it also compares the screen (grim) with the frame it
// presented, like TestLivePixels; that branch says in the log whether it
// ran.

const liveDmabufAppID = "dev.telaui.livedmabuf"

// placeDmabufWindow makes the test's windows open silently on the hidden
// workspace on Hyprland, and skips the test when it cannot: they must not
// take the focus on the workspace in use.
func placeDmabufWindow(t *testing.T) {
	t.Helper()
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return
	}
	ws := os.Getenv("TELA_LIVE_WORKSPACE")
	if ws == "" {
		ws = "7"
	}
	rule := `tela_live_dmabuf = hl.window_rule({ name = "tela-live-dmabuf", match = { class = "` + liveDmabufAppID + `" }, workspace = "` + ws + ` silent" })`
	if out, err := exec.Command("hyprctl", "eval", rule).CombinedOutput(); err != nil || strings.Contains(strings.ToLower(string(out)), "error") {
		t.Skipf("hyprctl eval (window rule): %v %s; not opening a window that would take the focus", err, out)
	}
	t.Cleanup(func() {
		exec.Command("hyprctl", "eval", `if tela_live_dmabuf then tela_live_dmabuf:set_enabled(false) end`).Run()
	})
}

// liveFormats are the formats the test can render, the opaque ones first:
// a DRM fourcc and the Vulkan format with the same bytes in memory.
var liveFormats = []struct {
	name   string
	fourcc uint32
	format vk.Format
}{
	{"XRGB8888", platform.DRMFormatXRGB8888, vk.FormatB8G8R8A8Unorm},
	{"XBGR8888", platform.DRMFormatXBGR8888, vk.FormatR8G8B8A8Unorm},
	{"ARGB8888", platform.DRMFormatARGB8888, vk.FormatB8G8R8A8Unorm},
	{"ABGR8888", platform.DRMFormatABGR8888, vk.FormatR8G8B8A8Unorm},
}

// liveUsage is how the renderer uses the images it presents.
const liveUsage = vk.ImageUsageColorAttachment | vk.ImageUsageTransferSrc | vk.ImageUsageTransferDst

// TestLiveDmabuf presents Vulkan images to the real compositor as
// dmabufs, once per physical device. On the device the compositor named
// as its main device every step must work. On another device the buffers
// cross from one driver to another, which needs a modifier both have, or
// a linear image that the compositor takes without a modifier, and a
// compositor that imports foreign memory: what happens there is logged
// and never a failure.
func TestLiveDmabuf(t *testing.T) {
	skipUnlessLive(t)
	if err := vk.Load(); err != nil {
		t.Skipf("Vulkan is not available: %v", err)
	}
	validate := os.Getenv("TELA_VK_VALIDATE") == "1"
	var messages []vk.DebugMessage
	inst, err := vk.NewInstance(vk.InstanceConfig{AppName: "tela live dmabuf", Validation: validate, Debug: func(m vk.DebugMessage) {
		messages = append(messages, m) // the test is single-threaded while Vulkan runs
	}})
	if err != nil {
		t.Skipf("no Vulkan instance: %v", err)
	}
	defer inst.Destroy()
	report := func(t *testing.T) {
		for _, m := range messages {
			if validate {
				t.Errorf("%s", m)
			} else {
				t.Logf("%s", m)
			}
		}
		messages = nil
	}
	devices, err := inst.Devices()
	if err != nil || len(devices) == 0 {
		t.Skipf("no usable Vulkan device: %v", err)
	}
	placeDmabufWindow(t)

	// The compositor's main device decides which run must pass.
	main := liveMainDevice(t, devices)
	for i, info := range devices {
		own := i == main
		name := info.Name
		if own {
			name += " (the compositor's device)"
		} else {
			name += " (another device)"
		}
		t.Run(name, func(t *testing.T) {
			liveDmabuf(t, inst, info, own)
			report(t)
		})
	}
}

// liveMainDevice opens a backend only to read the default feedback, logs
// it, and returns the index of the Vulkan device it names (-1 when it
// names none of them).
func liveMainDevice(t *testing.T, devices []vk.DeviceInfo) int {
	b := liveBackend(t)
	defer b.release()
	win, err := b.NewWindow(platform.WindowOptions{Title: "tela live dmabuf feedback", AppID: liveDmabufAppID, Hidden: true})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	fb, ok := w.DmabufFeedback()
	if !ok {
		t.Skip("the compositor reports no dmabuf feedback (no zwp_linux_dmabuf_v1, or nothing listed)")
	}
	t.Logf("zwp_linux_dmabuf_v1 version %d (the compositor has %d); %d format and modifier pairs", b.dmabuf.wl.Version(), b.dmabuf.version, len(fb.Formats))
	for _, f := range liveFormats {
		t.Logf("  %s: modifiers %#x", f.name, fb.Modifiers(f.fourcc))
	}
	if !fb.HasDevice {
		// The renderer's choice then (ADR-281): the first discrete one.
		main, _ := vk.SelectDevice(devices, "")
		t.Logf("the compositor names no main device; taking Vulkan device %d for it", main)
		return main
	}
	main := -1
	for i, d := range devices {
		if liveSameDevice(fb, d) {
			main = i
		}
		t.Logf("  Vulkan device %d: %s", i, d)
	}
	t.Logf("main device %d:%d is Vulkan device %d", fb.DeviceMajor, fb.DeviceMinor, main)
	return main
}

// liveSameDevice reports whether the feedback's main device is one of the
// DRM nodes of a Vulkan device. The compositor may name either node.
func liveSameDevice(fb platform.DmabufFeedback, d vk.DeviceInfo) bool {
	major, minor := int64(fb.DeviceMajor), int64(fb.DeviceMinor)
	return fb.HasDevice && (d.DRM.HasRender && d.DRM.RenderMajor == major && d.DRM.RenderMinor == minor ||
		d.DRM.HasPrimary && d.DRM.PrimaryMajor == major && d.DRM.PrimaryMinor == minor)
}

// liveScene is frame n of the test's animation for a buffer of a size:
// a background that changes every third frame, a square of a fixed color
// in each corner (so a flipped or channel-swapped image shows), and a
// patch in the middle that changes with every frame. Everything is
// opaque and lies on pixel boundaries.
type liveScene struct {
	size       image.Point
	background color.RGBA
	rects      []image.Rectangle
	colors     []color.RGBA
}

var (
	liveBackgrounds = []color.RGBA{{24, 32, 48, 255}, {200, 208, 216, 255}, {64, 24, 96, 255}}
	liveCorners     = [4]color.RGBA{{230, 30, 30, 255}, {30, 200, 60, 255}, {40, 60, 220, 255}, {240, 220, 40, 255}}
	livePatches     = []color.RGBA{{220, 40, 220, 255}, {40, 220, 230, 255}, {250, 250, 250, 255}, {250, 140, 20, 255}}
)

func newLiveScene(n int, size image.Point) liveScene {
	q := image.Pt(size.X/4, size.Y/4)
	s := liveScene{size: size, background: liveBackgrounds[n/3%len(liveBackgrounds)]}
	for i, at := range []image.Point{{0, 0}, {size.X - q.X, 0}, {0, size.Y - q.Y}, {size.X - q.X, size.Y - q.Y}} {
		s.rects = append(s.rects, image.Rectangle{Min: at, Max: at.Add(q)})
		s.colors = append(s.colors, liveCorners[i])
	}
	s.rects = append(s.rects, s.patch())
	s.colors = append(s.colors, livePatches[n%len(livePatches)])
	return s
}

// patch is the rectangle that changes with every frame.
func (s liveScene) patch() image.Rectangle {
	return image.Rect(3*s.size.X/8, 3*s.size.Y/8, 5*s.size.X/8, 5*s.size.Y/8)
}

// at is the color of a pixel.
func (s liveScene) at(p image.Point) color.RGBA {
	c := s.background
	for i, r := range s.rects {
		if p.In(r) {
			c = s.colors[i]
		}
	}
	return c
}

// draw paints the scene into a framebuffer, for the wl_shm presents.
func (s liveScene) draw(fb *image.RGBA) {
	fill(fb, fb.Rect, s.background)
	for i, r := range s.rects {
		fill(fb, r, s.colors[i])
	}
}

// liveGPU is the Vulkan side of one run.
type liveGPU struct {
	t       *testing.T
	inst    *vk.Instance
	info    vk.DeviceInfo
	dev     *vk.Device
	queue   vk.Queue
	mem     *vk.PhysicalDeviceMemoryProperties
	pool    vk.CommandPool
	cb      vk.CommandBuffer
	fence   vk.Fence
	foreign bool // VK_EXT_queue_family_foreign

	format     vk.Format
	modifiers  []vk.DmabufModifier
	renderPass vk.RenderPass
	// implicit: the buffers are registered without a modifier
	// (DRM_FORMAT_MOD_INVALID), see choose.
	implicit bool
}

// liveImage is one buffer of the test's swapchain.
type liveImage struct {
	img  *vk.DmabufImage
	view vk.ImageView
	fb   vk.Framebuffer
	buf  platform.DmabufBuffer
	// handed: the image was given to the foreign queue family (or just
	// left in the general layout) by a present.
	handed bool
	// held: presented, and its release not seen yet.
	held bool
}

func newLiveGPU(t *testing.T, inst *vk.Instance, info vk.DeviceInfo) *liveGPU {
	extensions, ok := vk.DmabufExtensions(&info)
	if !ok {
		t.Skipf("%s lacks an extension of the dmabuf export", info.Name)
	}
	priority := float32(1)
	count, names := vk.CStringArray(extensions)
	dev, err := inst.CreateDevice(info.Device, &vk.DeviceCreateInfo{
		SType:                vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos: &vk.DeviceQueueCreateInfo{
			SType:            vk.StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: info.GraphicsQueueFamily,
			QueueCount:       1,
			PQueuePriorities: &priority,
		},
		EnabledExtensionCount:   count,
		PpEnabledExtensionNames: names,
		PEnabledFeatures:        &vk.PhysicalDeviceFeatures{},
	})
	if err != nil {
		t.Fatalf("CreateDevice on %s: %v", info.Name, err)
	}
	g := &liveGPU{
		t: t, inst: inst, info: info, dev: dev,
		queue:   dev.GetQueue(info.GraphicsQueueFamily, 0),
		mem:     inst.GetPhysicalDeviceMemoryProperties(info.Device),
		foreign: info.HasExtension(vk.ExtQueueFamilyForeignExtensionName),
	}
	t.Cleanup(func() {
		dev.WaitIdle()
		if g.renderPass != 0 {
			dev.DestroyRenderPass(g.renderPass)
		}
		if g.fence != 0 {
			dev.DestroyFence(g.fence)
		}
		if g.pool != 0 {
			dev.DestroyCommandPool(g.pool)
		}
		dev.Destroy()
	})
	g.pool, err = dev.CreateCommandPool(&vk.CommandPoolCreateInfo{
		SType:            vk.StructureTypeCommandPoolCreateInfo,
		Flags:            vk.CommandPoolCreateResetCommandBuffer,
		QueueFamilyIndex: info.GraphicsQueueFamily,
	})
	g.check("CreateCommandPool", err)
	cbs, err := dev.AllocateCommandBuffers(&vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        g.pool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	})
	g.check("AllocateCommandBuffers", err)
	g.cb = cbs[0]
	g.fence, err = dev.CreateFence(&vk.FenceCreateInfo{SType: vk.StructureTypeFenceCreateInfo})
	g.check("CreateFence", err)
	return g
}

func (g *liveGPU) check(what string, err error) {
	g.t.Helper()
	if err != nil {
		g.t.Fatalf("%s on %s: %v", what, g.info.Name, err)
	}
}

// choose picks the format and the modifiers to render with: the first of
// liveFormats that the device can export with a modifier the compositor
// lists, with those modifiers, explicit ones only, and the tiled ones
// before the linear one. ok is false when there is no such format.
//
// With implicit set (a device that is not the compositor's), a format
// without a common modifier is still tried when the device exports it
// linear and the compositor lists it without a modifier
// (DRM_FORMAT_MOD_INVALID): the protocol's rule for a buffer of another
// device is a linear layout under the implicit modifier.
func (g *liveGPU) choose(fb platform.DmabufFeedback, implicit bool) (fourcc uint32, ok bool) {
	for _, pass := range []bool{false, true} {
		if fourcc, ok = g.chooseFormat(fb, pass); ok || !implicit {
			return fourcc, ok
		}
	}
	return 0, false
}

func (g *liveGPU) chooseFormat(fb platform.DmabufFeedback, implicit bool) (fourcc uint32, ok bool) {
	for _, f := range liveFormats {
		accepted := fb.Modifiers(f.fourcc)
		exportable := g.inst.DmabufModifiers(g.info.Device, f.format, liveUsage)
		var common []vk.DmabufModifier
		for _, m := range exportable {
			switch {
			case implicit:
				if m.Modifier == platform.DRMModifierLinear && slices.Contains(accepted, platform.DRMModifierInvalid) {
					common = append(common, m)
				}
			case m.Modifier != platform.DRMModifierInvalid && slices.Contains(accepted, m.Modifier):
				common = append(common, m)
			}
		}
		if implicit {
			if len(common) == 0 {
				continue
			}
			g.t.Logf("%s: no common modifier; trying a linear image under the implicit modifier", f.name)
			g.implicit = true
		} else {
			var exported []uint64
			for _, m := range exportable {
				exported = append(exported, m.Modifier)
			}
			g.t.Logf("%s: %s exports %#x; common with the compositor: %d", f.name, g.info.Name, exported, len(common))
			if len(common) == 0 {
				continue
			}
		}
		slices.SortStableFunc(common, func(a, b vk.DmabufModifier) int {
			switch {
			case a.Modifier == platform.DRMModifierLinear && b.Modifier != platform.DRMModifierLinear:
				return 1
			case b.Modifier == platform.DRMModifierLinear && a.Modifier != platform.DRMModifierLinear:
				return -1
			}
			return 0
		})
		g.format, g.modifiers = f.format, common
		var err error
		g.renderPass, err = g.dev.CreateRenderPass(&vk.RenderPassCreateInfo{
			SType:           vk.StructureTypeRenderPassCreateInfo,
			AttachmentCount: 1,
			PAttachments: &vk.AttachmentDescription{
				Format:         f.format,
				Samples:        vk.SampleCount1,
				LoadOp:         vk.AttachmentLoadOpClear,
				StoreOp:        vk.AttachmentStoreOpStore,
				StencilLoadOp:  vk.AttachmentLoadOpDontCare,
				StencilStoreOp: vk.AttachmentStoreOpDontCare,
				InitialLayout:  vk.ImageLayoutColorAttachmentOptimal,
				FinalLayout:    vk.ImageLayoutColorAttachmentOptimal,
			},
			SubpassCount: 1,
			PSubpasses: &vk.SubpassDescription{
				PipelineBindPoint:    vk.PipelineBindPointGraphics,
				ColorAttachmentCount: 1,
				PColorAttachments:    &vk.AttachmentReference{Attachment: 0, Layout: vk.ImageLayoutColorAttachmentOptimal},
			},
		})
		g.check("CreateRenderPass", err)
		return f.fourcc, true
	}
	return 0, false
}

// newImage creates an exported image of a size with its view and
// framebuffer.
func (g *liveGPU) newImage(size image.Point) (*liveImage, error) {
	img, err := g.dev.CreateDmabufImage(g.mem, g.format, uint32(size.X), uint32(size.Y), liveUsage, g.modifiers)
	if err != nil {
		return nil, err
	}
	li := &liveImage{img: img}
	li.view, err = g.dev.CreateImageView(&vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    img.Image,
		ViewType: vk.ImageViewType2D,
		Format:   g.format,
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask: vk.ImageAspectColor, LevelCount: 1, LayerCount: 1,
		},
	})
	g.check("CreateImageView", err)
	li.fb, err = g.dev.CreateFramebuffer(&vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass:      g.renderPass,
		AttachmentCount: 1,
		PAttachments:    &li.view,
		Width:           uint32(size.X),
		Height:          uint32(size.Y),
		Layers:          1,
	})
	g.check("CreateFramebuffer", err)
	return li, nil
}

// destroy destroys an image, after the device finished with it. Its
// platform buffer is destroyed first: the compositor keeps the memory of
// a frame it still shows.
func (g *liveGPU) destroy(li *liveImage) {
	if li.buf != nil {
		li.buf.Destroy()
	}
	g.dev.WaitIdle()
	g.dev.DestroyFramebuffer(li.fb)
	g.dev.DestroyImageView(li.view)
	g.dev.DestroyDmabufImage(li.img)
}

// render draws a scene into an image and waits until the device has
// finished: the buffer is handed over fully rendered (no fence travels
// with a dmabuf of a driver without implicit synchronization).
func (g *liveGPU) render(li *liveImage, s liveScene) {
	dev, cb := g.dev, g.cb
	g.check("BeginCommandBuffer", dev.BeginCommandBuffer(cb, &vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageOneTimeSubmit,
	}))
	whole := vk.ImageSubresourceRange{AspectMask: vk.ImageAspectColor, LevelCount: 1, LayerCount: 1}
	// Take the image back from whoever read it; its old pixels are not
	// needed, the scene covers it.
	acquire := vk.ImageMemoryBarrier{
		SType:               vk.StructureTypeImageMemoryBarrier,
		DstAccessMask:       vk.AccessColorAttachmentWrite,
		OldLayout:           vk.ImageLayoutUndefined,
		NewLayout:           vk.ImageLayoutColorAttachmentOptimal,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image:               li.img.Image,
		SubresourceRange:    whole,
	}
	if li.handed && g.foreign {
		acquire.OldLayout = vk.ImageLayoutGeneral
		acquire.SrcQueueFamilyIndex = vk.QueueFamilyForeignEXT
		acquire.DstQueueFamilyIndex = g.info.GraphicsQueueFamily
	}
	dev.CmdPipelineBarrier(cb, vk.PipelineStageTopOfPipe, vk.PipelineStageColorAttachmentOutput, 0, nil, nil, []vk.ImageMemoryBarrier{acquire})

	bg := s.background
	area := vk.Rect2D{Extent: vk.Extent2D{Width: uint32(s.size.X), Height: uint32(s.size.Y)}}
	clear := liveClear(bg)
	dev.CmdBeginRenderPass(cb, &vk.RenderPassBeginInfo{
		SType:           vk.StructureTypeRenderPassBeginInfo,
		RenderPass:      g.renderPass,
		Framebuffer:     li.fb,
		RenderArea:      area,
		ClearValueCount: 1,
		PClearValues:    &clear,
	}, vk.SubpassContentsInline)
	for i, r := range s.rects {
		dev.CmdClearAttachments(cb,
			[]vk.ClearAttachment{{AspectMask: vk.ImageAspectColor, ClearValue: liveClear(s.colors[i])}},
			[]vk.ClearRect{{
				Rect: vk.Rect2D{
					Offset: vk.Offset2D{X: int32(r.Min.X), Y: int32(r.Min.Y)},
					Extent: vk.Extent2D{Width: uint32(r.Dx()), Height: uint32(r.Dy())},
				},
				LayerCount: 1,
			}})
	}
	dev.CmdEndRenderPass(cb)

	// Hand it over: the layout other APIs read, and the foreign family.
	release := vk.ImageMemoryBarrier{
		SType:               vk.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       vk.AccessColorAttachmentWrite,
		OldLayout:           vk.ImageLayoutColorAttachmentOptimal,
		NewLayout:           vk.ImageLayoutGeneral,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image:               li.img.Image,
		SubresourceRange:    whole,
	}
	if g.foreign {
		release.SrcQueueFamilyIndex = g.info.GraphicsQueueFamily
		release.DstQueueFamilyIndex = vk.QueueFamilyForeignEXT
	}
	dev.CmdPipelineBarrier(cb, vk.PipelineStageColorAttachmentOutput, vk.PipelineStageBottomOfPipe, 0, nil, nil, []vk.ImageMemoryBarrier{release})
	g.check("EndCommandBuffer", dev.EndCommandBuffer(cb))
	g.check("QueueSubmit", dev.QueueSubmit(g.queue, []vk.SubmitInfo{{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    &cb,
	}}, g.fence))
	g.check("WaitForFences", dev.WaitForFences([]vk.Fence{g.fence}, true, 10e9))
	g.check("ResetFences", dev.ResetFences([]vk.Fence{g.fence}))
	li.handed = true
}

func liveClear(c color.RGBA) vk.ClearValue {
	return vk.ClearColor(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
}

// liveDmabuf is one run of the test with one rendering device. own says
// that it is the compositor's device, where every step must work.
func liveDmabuf(t *testing.T, inst *vk.Instance, info vk.DeviceInfo, own bool) {
	g := newLiveGPU(t, inst, info)
	b := liveBackend(t)
	title := "tela live dmabuf"
	win, err := b.NewWindow(platform.WindowOptions{Title: title, AppID: liveDmabufAppID, Size: platform.Size{W: 480, H: 320}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)

	// cross is how a failure of the dmabuf path is taken: a result to
	// report on another device, an error on the compositor's own.
	cross := func(format string, args ...any) {
		t.Helper()
		if own {
			t.Errorf(format, args...)
		} else {
			t.Logf("crossing devices: "+format, args...)
		}
	}

	var (
		fourcc   uint32
		modifier uint64
		images   []*liveImage
		second   platform.Window // the window whose opening resizes the first
		// onScreen: the window's workspace is on a monitor, so the screen
		// can be compared; asked once, at the first comparison.
		onScreen, asked bool

		frame      int // the animation's frame, shown by the latest present
		presents   int // dmabuf presents
		shm        int // wl_shm presents
		releases   int
		recreated  int
		callbacks  int // FrameEvents of the window after its first
		waits      int // ticks spent waiting for a free buffer
		checked    int // screenshots equal to the frame presented
		step       int
		stepSince  time.Time
		sizes      []image.Point
		started    bool
		finished   bool
		firstFrame bool
	)
	destroyImages := func() {
		for _, li := range images {
			g.destroy(li)
		}
		images = nil
	}
	defer destroyImages()

	// register creates and registers one more buffer of the window's
	// size.
	register := func() bool {
		size := w.Metrics().Physical()
		li, err := g.newImage(size)
		if err != nil {
			cross("creating an exportable %v image: %v", size, err)
			return false
		}
		if len(images) == 0 || images[0].img.Modifier != li.img.Modifier {
			t.Logf("%v image: modifier %#x, planes %+v", size, li.img.Modifier, li.img.Planes)
		}
		modifier = li.img.Modifier
		d := platform.DmabufDescriptor{Size: size, Fourcc: fourcc, Modifier: li.img.Modifier}
		if g.implicit {
			d.Modifier = platform.DRMModifierInvalid
		}
		for _, p := range li.img.Planes {
			d.Planes = append(d.Planes, platform.DmabufPlane{FD: li.img.FD, Offset: uint32(p.Offset), Stride: uint32(p.Stride)})
		}
		li.buf, err = w.CreateDmabuf(d)
		if err != nil {
			g.destroy(li)
			cross("the compositor did not take a %v buffer with modifier %#x: %v", size, li.img.Modifier, err)
			return false
		}
		images = append(images, li)
		return true
	}

	// presentGPU renders the next frame into a free buffer and presents
	// it; full says the whole buffer changed, else only the patch did. It
	// reports false while it waits for the compositor to release one.
	presentGPU := func(full bool) bool {
		size := w.Metrics().Physical()
		if len(images) > 0 && images[0].buf.Size() != size {
			// A ConfigureEvent changed the size: the swapchain is made
			// again. The old buffers go while the compositor may still
			// hold one.
			destroyImages()
			recreated++
			sizes = append(sizes, size)
		}
		var free *liveImage
		for _, li := range images {
			if !li.buf.Busy() {
				free = li
				break
			}
		}
		if free == nil {
			// Two buffers are the start; a third when the compositor
			// holds both. More than three means it releases nothing.
			if len(images) < 2 || len(images) < 3 && waits > 5 {
				if !register() {
					finished = true
					return false
				}
				free = images[len(images)-1]
				waits = 0
			} else {
				waits++
				return false
			}
		}
		waits = 0
		next := frame + 1
		if full {
			next = (frame/3 + 1) * 3 // a new background
		}
		scene := newLiveScene(next, size)
		g.render(free, scene)
		damage := []image.Rectangle{scene.patch()}
		if full {
			damage = nil
		}
		if free.buf.Busy() {
			t.Fatal("a buffer became busy while it was rendered into")
		}
		if err := w.PresentDmabuf(free.buf, damage); err != nil {
			t.Fatalf("PresentDmabuf: %v", err)
		}
		if !free.buf.Busy() {
			t.Fatalf("a presented buffer is not busy (window hidden? %v)", !w.showable())
		}
		free.held = true
		frame = next
		presents++
		return true
	}
	presentShm := func() bool {
		frame++
		scene := newLiveScene(frame, w.Metrics().Physical())
		fb := image.NewRGBA(image.Rectangle{Max: scene.size})
		scene.draw(fb)
		if err := w.Present(fb, nil); err != nil {
			t.Fatalf("Present: %v", err)
		}
		shm++
		return true
	}
	// verify compares the screen with the frame presented last, when the
	// window is on a monitor. The first comparison waits for the
	// compositor's open animation, the others for a frame or two.
	verify := func(what string) bool {
		if !asked {
			asked = true
			onScreen = liveOnScreen(t, title)
		}
		if !onScreen {
			return true
		}
		settle := 250 * time.Millisecond
		if checked == 0 {
			settle = 800 * time.Millisecond
		}
		if time.Since(stepSince) < settle {
			return false
		}
		c, err := findWindow(title)
		var img image.Image
		if err == nil {
			img, err = screenshot(c)
		}
		if err != nil {
			t.Errorf("%s: %v", what, err)
			return true
		}
		if bad := liveCompare(img, newLiveScene(frame, w.Metrics().Physical())); len(bad) > 0 {
			t.Errorf("%s (frame %d): the screen differs from the frame presented: %s", what, frame, strings.Join(bad, "; "))
		} else {
			checked++
		}
		return true
	}
	resized := func(what string) func() bool {
		return func() bool {
			if size := w.Metrics().Physical(); len(images) > 0 && images[0].buf.Size() != size {
				t.Logf("%s: the window is now %v", what, size)
				return true
			}
			if time.Since(stepSince) > 3*time.Second {
				t.Errorf("%s did not resize the window (a compositor that does not tile?)", what)
				return true
			}
			return false
		}
	}

	steps := []func() bool{
		// Frames in a row: whole ones and ones that change the patch.
		func() bool { return presentGPU(true) },
		func() bool { return verify("the first dmabuf") },
		func() bool { return presentGPU(false) },
		func() bool { return verify("a dmabuf with partial damage") },
		func() bool { return presentGPU(false) },
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		func() bool { return presentGPU(false) },
		func() bool { return verify("the sixth dmabuf") },
		// A resize: a second window of ours makes a tiling compositor
		// split the workspace.
		func() bool {
			var err error
			second, err = b.NewWindow(platform.WindowOptions{Title: title + " (second)", AppID: liveDmabufAppID, Size: platform.Size{W: 200, H: 150}})
			if err != nil {
				t.Fatal(err)
			}
			return true
		},
		resized("opening a second window"),
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		func() bool { return verify("a dmabuf after the resize") },
		// wl_shm and dmabufs take turns.
		presentShm,
		func() bool { return verify("wl_shm after dmabufs") },
		func() bool { return presentGPU(false) },
		func() bool { return verify("a dmabuf after wl_shm") },
		func() bool { return presentGPU(false) },
		presentShm,
		func() bool { return presentGPU(true) },
		func() bool { return verify("the last change of kind") },
		// The second window goes: back to the first size.
		func() bool { second.Close(); second = nil; return true },
		resized("closing the second window"),
		func() bool { return presentGPU(true) },
		func() bool { return presentGPU(false) },
		func() bool { return presentGPU(false) },
		func() bool { return verify("the last dmabuf") },
		// Time for the last releases.
		func() bool { return time.Since(stepSince) > 150*time.Millisecond },
	}

	tick := func() {
		// Releases arrive while the loop dispatches: seen here as buffers
		// that are no longer busy.
		for _, li := range images {
			if li.held && !li.buf.Busy() {
				li.held = false
				releases++
			}
		}
		if finished {
			return
		}
		// One step a tick: the compositor gets time to answer between two.
		if step < len(steps) {
			if stepSince.IsZero() {
				stepSince = time.Now()
			}
			if steps[step]() {
				step++
				stepSince = time.Time{}
			}
		}
		if step == len(steps) {
			finished = true
		}
	}

	stop := time.AfterFunc(40*time.Second, b.Quit) // safety net
	defer stop.Stop()
	var ticking atomic.Bool
	ticking.Store(true)
	defer ticking.Store(false)
	var ticker func()
	ticker = func() {
		if ticking.Load() {
			b.Wake()
			time.AfterFunc(8*time.Millisecond, ticker)
		}
	}
	runErr := b.Run(platform.HandlerFunc(func(pw platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ConfigureEvent:
			if pw == platform.Window(w) {
				t.Logf("configure %v (%v px)", e.Metrics, e.Metrics.Physical())
			}
		case platform.WindowStateEvent:
			if pw == platform.Window(w) {
				t.Logf("state %v", e.State)
			}
		case platform.FrameEvent:
			if second != nil && pw == second {
				// The second window only has to exist: one frame maps it.
				fb := image.NewRGBA(image.Rectangle{Max: second.Metrics().Physical()})
				quadrants(fb)
				if err := second.Present(fb, nil); err != nil {
					t.Fatalf("Present: %v", err)
				}
				return
			}
			if firstFrame {
				callbacks++
			}
			firstFrame = true
			if started {
				return
			}
			started = true
			fb, ok := w.DmabufFeedback()
			if !ok {
				t.Error("the window reports no dmabuf feedback")
				finished = true
				b.Quit()
				return
			}
			if got := liveSameDevice(fb, info); got != own {
				t.Logf("the window's feedback names the device %d:%d (this device: %v)", fb.DeviceMajor, fb.DeviceMinor, got)
			}
			if fourcc, ok = g.choose(fb, !own); !ok {
				finished = true
				b.Quit()
				return
			}
			sizes = append(sizes, w.Metrics().Physical())
			go ticker()
		case platform.WakeEvent:
			if !started {
				return
			}
			tick()
			if finished {
				// A last round trip: a protocol error for anything sent
				// would arrive before its answer.
				if err := b.conn.RoundtripTimeout(2 * time.Second); err != nil {
					t.Errorf("after the last present: %v", err)
				}
				logWorkspace(t)
				b.Quit()
			}
		}
	}))
	if runErr != nil {
		if !own {
			t.Skipf("crossing devices: the connection ended: %v", runErr)
		}
		t.Fatalf("Run: %v", runErr)
	}
	if !started {
		t.Fatal("the window never got its first frame")
	}
	if fourcc == 0 {
		// The lists of both sides are in the log (choose).
		if own {
			t.Skipf("%s exports no 32-bit color attachment with a modifier the compositor lists: nothing to present", info.Name)
		}
		t.Skipf("crossing devices: %s and the compositor have no explicit modifier in common: a renderer on this device presents with wl_shm", info.Name)
	}
	t.Logf("format %#x modifier %#x: %d dmabuf presents and %d wl_shm presents over the sizes %v; %d releases seen, %d buffers at the end, %d frame callbacks, %d screenshots equal to the frame",
		fourcc, modifier, presents, shm, sizes, releases, len(images), callbacks, checked)
	if step < len(steps) {
		cross("the sequence stopped at step %d of %d", step, len(steps))
		return
	}
	if recreated < 2 {
		t.Errorf("the swapchain was made again %d times, want 2 (a resize and back)", recreated)
	}
	// Every buffer but the ones on screen was released: the compositor
	// gives buffers back, and none was reused before that (PresentDmabuf
	// panics on a busy one).
	if releases == 0 || releases < presents-recreated*3-3 {
		t.Errorf("%d releases for %d presents", releases, presents)
	}
	if !onScreen {
		t.Log("the pixels were NOT looked at: the window's workspace is on no monitor, and the compositor captures no window it does not draw")
	} else if checked == 0 {
		t.Error("the window was on screen and no screenshot matched a frame")
	}
	if callbacks == 0 {
		t.Log("no frame callback after the first frame: the window is on a hidden workspace (suspended), so the presents were paced by a timer")
	}
}

// liveOnScreen reports whether the window of this process with the title
// is on a workspace some monitor shows (Hyprland only: elsewhere the
// answer is no, and the screen is not compared).
func liveOnScreen(t *testing.T, title string) bool {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return false
	}
	var clients []struct {
		Pid       int    `json:"pid"`
		Title     string `json:"title"`
		Workspace struct {
			ID int `json:"id"`
		} `json:"workspace"`
	}
	var monitors []struct {
		ActiveWorkspace struct {
			ID int `json:"id"`
		} `json:"activeWorkspace"`
	}
	for _, q := range []struct {
		what string
		into any
	}{{"clients", &clients}, {"monitors", &monitors}} {
		out, err := exec.Command("hyprctl", q.what, "-j").Output()
		if err == nil {
			err = json.Unmarshal(out, q.into)
		}
		if err != nil {
			t.Logf("hyprctl %s: %v", q.what, err)
			return false
		}
	}
	for _, c := range clients {
		if c.Pid != os.Getpid() || c.Title != title {
			continue
		}
		for _, m := range monitors {
			if m.ActiveWorkspace.ID == c.Workspace.ID {
				return true
			}
		}
	}
	return false
}

// liveCompare compares a screenshot of the window with a scene at the
// middle of each of its areas (the corner squares, the patch, and the
// background between them), with the tolerance of TestLivePixels: the
// desktop may dim or tint a window that is not focused. It returns what
// differs.
func liveCompare(img image.Image, s liveScene) []string {
	bounds := img.Bounds()
	var bad []string
	for _, at := range [][2]float64{
		{0.125, 0.125}, {0.875, 0.125}, {0.125, 0.875}, {0.875, 0.875}, // the corner squares
		{0.5, 0.5},                                                         // the patch
		{0.5, 0.125}, {0.125, 0.5}, {0.875, 0.5}, {0.5, 0.875}, {0.3, 0.3}, // the background
	} {
		x := bounds.Min.X + int(at[0]*float64(bounds.Dx()))
		y := bounds.Min.Y + int(at[1]*float64(bounds.Dy()))
		r, g, b, _ := img.At(x, y).RGBA()
		got := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
		want := s.at(image.Pt(int(at[0]*float64(s.size.X)), int(at[1]*float64(s.size.Y))))
		if !near(got, want, 24) {
			bad = append(bad, fmt.Sprintf("at (%.3f, %.3f) the screen has %v, the frame %v", at[0], at[1], got, want))
		}
	}
	return bad
}
