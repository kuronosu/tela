//go:build linux && telagpu

// Package dmabuftest is the Vulkan side of the live tests of
// platform.DmabufPresenter (ADR-281, ADR-283), shared by the Wayland and
// X11 backends: a device that renders a known scene into images it
// exports as dmabufs, as the GPU renderer will. What only a live test can
// tell is whether a real display server imports what a real driver
// exports, so both backends render the same way and check the result
// their own way (a screenshot, GetImage). It needs the build tag telagpu,
// like the Vulkan bindings.
package dmabuftest

import (
	"image"
	"image/color"
	"os"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/vk"
)

// Format is a format the tests can render: a DRM fourcc and the Vulkan
// format with the same bytes in memory.
type Format struct {
	Name   string
	Fourcc uint32
	VK     vk.Format
}

// Formats are the formats the tests can render, the opaque ones first.
var Formats = []Format{
	{"XRGB8888", platform.DRMFormatXRGB8888, vk.FormatB8G8R8A8Unorm},
	{"XBGR8888", platform.DRMFormatXBGR8888, vk.FormatR8G8B8A8Unorm},
	{"ARGB8888", platform.DRMFormatARGB8888, vk.FormatB8G8R8A8Unorm},
	{"ABGR8888", platform.DRMFormatABGR8888, vk.FormatR8G8B8A8Unorm},
}

// Usage is how the renderer uses the images it presents.
const Usage = vk.ImageUsageColorAttachment | vk.ImageUsageTransferSrc | vk.ImageUsageTransferDst

// NewInstance loads Vulkan and creates an instance, with the validation
// layers when TELA_VK_VALIDATE=1, or skips the test. report logs the
// debug messages received since its last call, as errors under
// validation.
func NewInstance(t *testing.T, name string) (inst *vk.Instance, report func(t *testing.T)) {
	t.Helper()
	if err := vk.Load(); err != nil {
		t.Skipf("Vulkan is not available: %v", err)
	}
	validate := os.Getenv("TELA_VK_VALIDATE") == "1"
	var messages []vk.DebugMessage
	inst, err := vk.NewInstance(vk.InstanceConfig{AppName: name, Validation: validate, Debug: func(m vk.DebugMessage) {
		messages = append(messages, m) // the tests are single-threaded while Vulkan runs
	}})
	if err != nil {
		t.Skipf("no Vulkan instance: %v", err)
	}
	t.Cleanup(inst.Destroy)
	return inst, func(t *testing.T) {
		for _, m := range messages {
			if validate {
				t.Errorf("%s", m)
			} else {
				t.Logf("%s", m)
			}
		}
		messages = nil
	}
}

// SameDevice reports whether the feedback's main device is one of the DRM
// nodes of a Vulkan device. The display server may name either node.
func SameDevice(fb platform.DmabufFeedback, d vk.DeviceInfo) bool {
	major, minor := int64(fb.DeviceMajor), int64(fb.DeviceMinor)
	return fb.HasDevice && (d.DRM.HasRender && d.DRM.RenderMajor == major && d.DRM.RenderMinor == minor ||
		d.DRM.HasPrimary && d.DRM.PrimaryMajor == major && d.DRM.PrimaryMinor == minor)
}

// Scene is frame n of the tests' animation for a buffer of a size: a
// background that changes every third frame, a square of a fixed color in
// each corner (so a flipped or channel-swapped image shows), and a patch
// in the middle that changes with every frame. Everything is opaque and
// lies on pixel boundaries.
type Scene struct {
	Size       image.Point
	Background color.RGBA
	Rects      []image.Rectangle
	Colors     []color.RGBA
}

var (
	backgrounds = []color.RGBA{{24, 32, 48, 255}, {200, 208, 216, 255}, {64, 24, 96, 255}}
	corners     = [4]color.RGBA{{230, 30, 30, 255}, {30, 200, 60, 255}, {40, 60, 220, 255}, {240, 220, 40, 255}}
	patches     = []color.RGBA{{220, 40, 220, 255}, {40, 220, 230, 255}, {250, 250, 250, 255}, {250, 140, 20, 255}}
)

// NewScene returns frame n for a buffer of a size.
func NewScene(n int, size image.Point) Scene {
	q := image.Pt(size.X/4, size.Y/4)
	s := Scene{Size: size, Background: backgrounds[n/3%len(backgrounds)]}
	for i, at := range []image.Point{{0, 0}, {size.X - q.X, 0}, {0, size.Y - q.Y}, {size.X - q.X, size.Y - q.Y}} {
		s.Rects = append(s.Rects, image.Rectangle{Min: at, Max: at.Add(q)})
		s.Colors = append(s.Colors, corners[i])
	}
	s.Rects = append(s.Rects, s.Patch())
	s.Colors = append(s.Colors, patches[n%len(patches)])
	return s
}

// Patch is the rectangle that changes with every frame.
func (s Scene) Patch() image.Rectangle {
	return image.Rect(3*s.Size.X/8, 3*s.Size.Y/8, 5*s.Size.X/8, 5*s.Size.Y/8)
}

// At is the color of a pixel.
func (s Scene) At(p image.Point) color.RGBA {
	c := s.Background
	for i, r := range s.Rects {
		if p.In(r) {
			c = s.Colors[i]
		}
	}
	return c
}

// Draw paints the scene into a framebuffer, for the presents that copy.
func (s Scene) Draw(fb *image.RGBA) {
	fill(fb, fb.Rect, s.Background)
	for i, r := range s.Rects {
		fill(fb, r, s.Colors[i])
	}
}

func fill(fb *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(fb.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := fb.PixOffset(x, y)
			fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
}

// GPU is the Vulkan side of one run: a device, and after Choose the
// format and the modifiers it renders with.
type GPU struct {
	Info vk.DeviceInfo
	// Format and Modifiers are what Choose picked; Implicit says that the
	// buffers are registered without a modifier (DRM_FORMAT_MOD_INVALID).
	Format    vk.Format
	Modifiers []vk.DmabufModifier
	Implicit  bool

	t       *testing.T
	inst    *vk.Instance
	dev     *vk.Device
	queue   vk.Queue
	mem     *vk.PhysicalDeviceMemoryProperties
	pool    vk.CommandPool
	cb      vk.CommandBuffer
	fence   vk.Fence
	foreign bool // VK_EXT_queue_family_foreign

	renderPass vk.RenderPass
}

// Image is one buffer of a test's swapchain.
type Image struct {
	Img *vk.DmabufImage
	// Buf is the platform's buffer, set by the test once registered.
	Buf platform.DmabufBuffer
	// Held: presented, and its release not seen yet (the test's own
	// bookkeeping).
	Held bool

	view vk.ImageView
	fb   vk.Framebuffer
	// handed: the image was given to the foreign queue family (or just
	// left in the general layout) by a present.
	handed bool
}

// NewGPU creates a device with the extensions of the dmabuf export, or
// skips the test when the physical device lacks one. The device is
// destroyed with the test.
func NewGPU(t *testing.T, inst *vk.Instance, info vk.DeviceInfo) *GPU {
	t.Helper()
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
	g := &GPU{
		Info: info, t: t, inst: inst, dev: dev,
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

func (g *GPU) check(what string, err error) {
	g.t.Helper()
	if err != nil {
		g.t.Fatalf("%s on %s: %v", what, g.Info.Name, err)
	}
}

// Choose picks the format and the modifiers to render with: the first of
// Formats that the device can export with a modifier the feedback lists,
// with those modifiers, explicit ones only, and the tiled ones before the
// linear one. ok is false when there is no such format.
//
// With implicit set (a device that is not the display server's), a format
// without a common modifier is still tried when the device exports it
// linear and the feedback lists it without a modifier
// (DRM_FORMAT_MOD_INVALID): the rule for a buffer of another device is a
// linear layout under the implicit modifier.
func (g *GPU) Choose(fb platform.DmabufFeedback, implicit bool) (fourcc uint32, ok bool) {
	for _, pass := range []bool{false, true} {
		if fourcc, ok = g.chooseFormat(fb, pass); ok || !implicit {
			return fourcc, ok
		}
	}
	return 0, false
}

func (g *GPU) chooseFormat(fb platform.DmabufFeedback, implicit bool) (fourcc uint32, ok bool) {
	for _, f := range Formats {
		accepted := fb.Modifiers(f.Fourcc)
		exportable := g.inst.DmabufModifiers(g.Info.Device, f.VK, Usage)
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
			g.t.Logf("%s: no common modifier; trying a linear image under the implicit modifier", f.Name)
			g.Implicit = true
		} else {
			if len(accepted) == 0 {
				continue // not a format of this display server
			}
			var exported []uint64
			for _, m := range exportable {
				exported = append(exported, m.Modifier)
			}
			g.t.Logf("%s: %s exports %#x; common with the display server: %d", f.Name, g.Info.Name, exported, len(common))
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
		g.Format, g.Modifiers = f.VK, common
		var err error
		g.renderPass, err = g.dev.CreateRenderPass(&vk.RenderPassCreateInfo{
			SType:           vk.StructureTypeRenderPassCreateInfo,
			AttachmentCount: 1,
			PAttachments: &vk.AttachmentDescription{
				Format:         f.VK,
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
		return f.Fourcc, true
	}
	return 0, false
}

// NewImage creates an exported image of a size with its view and
// framebuffer.
func (g *GPU) NewImage(size image.Point) (*Image, error) {
	img, err := g.dev.CreateDmabufImage(g.mem, g.Format, uint32(size.X), uint32(size.Y), Usage, g.Modifiers)
	if err != nil {
		return nil, err
	}
	li := &Image{Img: img}
	li.view, err = g.dev.CreateImageView(&vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    img.Image,
		ViewType: vk.ImageViewType2D,
		Format:   g.Format,
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

// Descriptor describes an image to CreateDmabuf: its planes, in its one
// dmabuf, with the fourcc Choose returned and the image's modifier (the
// implicit one after an implicit Choose).
func (g *GPU) Descriptor(li *Image, size image.Point, fourcc uint32) platform.DmabufDescriptor {
	d := platform.DmabufDescriptor{Size: size, Fourcc: fourcc, Modifier: li.Img.Modifier}
	if g.Implicit {
		d.Modifier = platform.DRMModifierInvalid
	}
	for _, p := range li.Img.Planes {
		d.Planes = append(d.Planes, platform.DmabufPlane{FD: li.Img.FD, Offset: uint32(p.Offset), Stride: uint32(p.Stride)})
	}
	return d
}

// Destroy destroys an image, after the device finished with it. Its
// platform buffer is destroyed first: the display server keeps the memory
// of a frame it still shows.
func (g *GPU) Destroy(li *Image) {
	if li.Buf != nil {
		li.Buf.Destroy()
	}
	g.dev.WaitIdle()
	g.dev.DestroyFramebuffer(li.fb)
	g.dev.DestroyImageView(li.view)
	g.dev.DestroyDmabufImage(li.Img)
}

// Render draws a scene into an image and waits until the device has
// finished: the buffer is handed over fully rendered (no fence travels
// with a dmabuf of a driver without implicit synchronization).
func (g *GPU) Render(li *Image, s Scene) {
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
		Image:               li.Img.Image,
		SubresourceRange:    whole,
	}
	if li.handed && g.foreign {
		acquire.OldLayout = vk.ImageLayoutGeneral
		acquire.SrcQueueFamilyIndex = vk.QueueFamilyForeignEXT
		acquire.DstQueueFamilyIndex = g.Info.GraphicsQueueFamily
	}
	dev.CmdPipelineBarrier(cb, vk.PipelineStageTopOfPipe, vk.PipelineStageColorAttachmentOutput, 0, nil, nil, []vk.ImageMemoryBarrier{acquire})

	area := vk.Rect2D{Extent: vk.Extent2D{Width: uint32(s.Size.X), Height: uint32(s.Size.Y)}}
	clear := clearColor(s.Background)
	dev.CmdBeginRenderPass(cb, &vk.RenderPassBeginInfo{
		SType:           vk.StructureTypeRenderPassBeginInfo,
		RenderPass:      g.renderPass,
		Framebuffer:     li.fb,
		RenderArea:      area,
		ClearValueCount: 1,
		PClearValues:    &clear,
	}, vk.SubpassContentsInline)
	for i, r := range s.Rects {
		dev.CmdClearAttachments(cb,
			[]vk.ClearAttachment{{AspectMask: vk.ImageAspectColor, ClearValue: clearColor(s.Colors[i])}},
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
		Image:               li.Img.Image,
		SubresourceRange:    whole,
	}
	if g.foreign {
		release.SrcQueueFamilyIndex = g.Info.GraphicsQueueFamily
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

func clearColor(c color.RGBA) vk.ClearValue {
	return vk.ClearColor(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
}
