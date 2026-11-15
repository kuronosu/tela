//go:build telagpu && linux

package vk

import (
	"fmt"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// drmModLinear is DRM_FORMAT_MOD_LINEAR: rows of pixels, one after the
// other, which is the one layout this test can read through a mapping.
const drmModLinear = 0

// dmabufUsage is how the renderer uses the images it presents: it draws
// into them, and copies from and into them.
const dmabufUsage = ImageUsageColorAttachment | ImageUsageTransferSrc | ImageUsageTransferDst

// dmabufSize is the size of the test's images: not a power of two, so a
// row pitch larger than the row shows.
const (
	dmabufWidth  = 250
	dmabufHeight = 70
)

// TestDmabufExport creates, on every physical device that can, images
// whose memory is exported as a dmabuf, in the two 32-bit formats a
// compositor takes, once with every modifier the device exports and once
// per modifier, and checks what the other side of the dmabuf would be
// told: the modifier is one of those offered, the descriptor is a dmabuf
// large enough for the planes, and the planes lie inside it. It clears
// each image and reads it back through Vulkan, and, where the dmabuf can
// be mapped (not on NVIDIA) and the modifier is linear, through the
// descriptor too, at the offset and row pitch reported: the pixels are
// where the layout says.
func TestDmabufExport(t *testing.T) {
	inst, msgs := newTestInstance(t)
	for _, info := range testDevices(t, inst) {
		t.Run(info.Name, func(t *testing.T) {
			t.Cleanup(func() { msgs.report(t) })
			exportDmabufs(t, inst, info)
		})
	}
}

func exportDmabufs(t *testing.T, inst *Instance, info DeviceInfo) {
	extensions, ok := DmabufExtensions(&info)
	if !ok {
		t.Skipf("%s lacks an extension of the dmabuf export (%s, %s, %s)", info.Name,
			KhrExternalMemoryFdExtensionName, ExtExternalMemoryDmaBufExtensionName, ExtImageDrmFormatModifierExtensionName)
	}
	foreign := info.HasExtension(ExtQueueFamilyForeignExtensionName)
	priority := float32(1)
	count, names := CStringArray(extensions)
	dev, err := inst.CreateDevice(info.Device, &DeviceCreateInfo{
		SType:                StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos: &DeviceQueueCreateInfo{
			SType:            StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: info.GraphicsQueueFamily,
			QueueCount:       1,
			PQueuePriorities: &priority,
		},
		EnabledExtensionCount:   count,
		PpEnabledExtensionNames: names,
		PEnabledFeatures:        &PhysicalDeviceFeatures{},
	})
	check(t, "CreateDevice", err)
	t.Cleanup(dev.Destroy)
	queue := dev.GetQueue(info.GraphicsQueueFamily, 0)
	mem := inst.GetPhysicalDeviceMemoryProperties(info.Device)
	r := &renderer{t: t, dev: dev, mem: mem}

	pool, err := dev.CreateCommandPool(&CommandPoolCreateInfo{
		SType:            StructureTypeCommandPoolCreateInfo,
		Flags:            CommandPoolCreateResetCommandBuffer,
		QueueFamilyIndex: info.GraphicsQueueFamily,
	})
	check(t, "CreateCommandPool", err)
	t.Cleanup(func() { dev.DestroyCommandPool(pool) })
	fence, err := dev.CreateFence(&FenceCreateInfo{SType: StructureTypeFenceCreateInfo})
	check(t, "CreateFence", err)
	t.Cleanup(func() { dev.DestroyFence(fence) })
	readback := r.newBuffer(4*dmabufWidth*dmabufHeight, BufferUsageTransferDst)

	exported := 0
	for _, format := range []struct {
		name   string
		format Format
		// order is where the red, green, blue and alpha bytes of a pixel
		// are in memory.
		order [4]int
	}{
		{"B8G8R8A8_UNORM", FormatB8G8R8A8Unorm, [4]int{2, 1, 0, 3}},
		{"R8G8B8A8_UNORM", FormatR8G8B8A8Unorm, [4]int{0, 1, 2, 3}},
	} {
		// The modifiers of the format, and among them the ones an image
		// of this usage can be exported with.
		all := inst.DrmFormatModifiers(info.Device, format.format)
		modifiers := inst.DmabufModifiers(info.Device, format.format, dmabufUsage)
		t.Logf("%s: %d modifiers, %d exportable as a color attachment:", format.name, len(all), len(modifiers))
		for _, a := range all {
			if a.DrmFormatModifierTilingFeatures&FormatFeatureColorAttachment == 0 {
				t.Logf("  %#016x: %d plane(s), features %#x: not a color attachment", a.DrmFormatModifier, a.DrmFormatModifierPlaneCount, a.DrmFormatModifierTilingFeatures)
			}
		}
		for _, m := range modifiers {
			t.Logf("  %#016x: %d plane(s), features %#x, up to %dx%d, dedicated only %v",
				m.Modifier, m.Planes, m.Features, m.MaxExtent.Width, m.MaxExtent.Height, m.DedicatedOnly)
			if m.Planes == 0 || m.Planes > maxDmabufPlanes {
				t.Errorf("%s modifier %#x has %d memory planes", format.name, m.Modifier, m.Planes)
			}
			if m.Features&FormatFeatureColorAttachment == 0 {
				t.Errorf("%s modifier %#x is listed for a color attachment without the feature (%#x)", format.name, m.Modifier, m.Features)
			}
			if m.MaxExtent.Width < dmabufWidth || m.MaxExtent.Height < dmabufHeight {
				t.Errorf("%s modifier %#x: largest image %dx%d", format.name, m.Modifier, m.MaxExtent.Width, m.MaxExtent.Height)
			}
			found := false
			for _, a := range all {
				found = found || a.DrmFormatModifier == m.Modifier && a.DrmFormatModifierPlaneCount == m.Planes
			}
			if !found {
				t.Errorf("%s: the exportable modifier %#x is not among the format's modifiers", format.name, m.Modifier)
			}
		}
		if len(modifiers) == 0 {
			continue
		}

		// The driver's choice among all of them, then each one alone.
		offers := [][]DmabufModifier{modifiers}
		for i := range modifiers {
			offers = append(offers, modifiers[i:i+1])
		}
		for n, offer := range offers {
			what := fmt.Sprintf("%s, modifier %#x", format.name, offer[0].Modifier)
			if n == 0 {
				what = fmt.Sprintf("%s, any of %d modifiers", format.name, len(offer))
			}
			// A color of its own per image, with four different bytes.
			color := rgba{uint8(40 + 10*n), uint8(200 - 10*n), uint8(90 + n), 255}
			exportDmabuf(t, r, queue, pool, fence, readback, what, format.format, format.order, offer, color, info.GraphicsQueueFamily, foreign)
			exported++
		}
	}
	if exported == 0 {
		t.Skipf("%s exports no dmabuf of a 32-bit color attachment", info.Name)
	}
}

// exportDmabuf creates one exported image with a modifier of offer,
// checks its description, clears it to color and reads it back.
func exportDmabuf(t *testing.T, r *renderer, queue Queue, pool CommandPool, fence Fence, readback *hostBuffer, what string, format Format, order [4]int, offer []DmabufModifier, color rgba, family uint32, foreign bool) {
	dev := r.dev
	img, err := dev.CreateDmabufImage(r.mem, format, dmabufWidth, dmabufHeight, dmabufUsage, offer)
	if err != nil {
		t.Errorf("%s: %v", what, err)
		return
	}
	defer dev.DestroyDmabufImage(img)

	// The description.
	var chosen *DmabufModifier
	for i := range offer {
		if offer[i].Modifier == img.Modifier {
			chosen = &offer[i]
		}
	}
	if chosen == nil {
		t.Errorf("%s: the image has the modifier %#x, which was not offered", what, img.Modifier)
		return
	}
	if len(img.Planes) != int(chosen.Planes) {
		t.Errorf("%s: %d plane layouts, the modifier has %d planes", what, len(img.Planes), chosen.Planes)
	}
	var st unix.Stat_t
	if err := unix.Fstat(img.FD, &st); err != nil {
		t.Errorf("%s: the exported descriptor %d: %v", what, img.FD, err)
		return
	}
	// A dmabuf answers lseek(SEEK_END) with its size.
	size, err := unix.Seek(img.FD, 0, unix.SEEK_END)
	if err != nil {
		t.Errorf("%s: the exported descriptor is not a dmabuf: lseek: %v", what, err)
		return
	}
	unix.Seek(img.FD, 0, unix.SEEK_SET)
	t.Logf("%s: chose %#x, dmabuf of %d bytes, planes %v", what, img.Modifier, size, img.Planes)
	for i, p := range img.Planes {
		if p.Offset >= uint64(size) {
			t.Errorf("%s: plane %d starts at %d in a dmabuf of %d bytes", what, i, p.Offset, size)
		}
		if p.Stride == 0 {
			t.Errorf("%s: plane %d has no row pitch", what, i)
		}
	}
	main := img.Planes[0]
	if main.Stride < 4*dmabufWidth {
		t.Errorf("%s: row pitch %d of a %d pixel row of 4 bytes", what, main.Stride, dmabufWidth)
	}
	linear := img.Modifier == drmModLinear
	if linear && main.Offset+main.Stride*(dmabufHeight-1)+4*dmabufWidth > uint64(size) {
		t.Errorf("%s: a linear plane at %d with row pitch %d does not fit %d rows in %d bytes", what, main.Offset, main.Stride, dmabufHeight, size)
	}
	if uint64(size) < 4*dmabufWidth*dmabufHeight {
		t.Errorf("%s: a dmabuf of %d bytes for %dx%d pixels of 4 bytes", what, size, dmabufWidth, dmabufHeight)
	}

	// Clear it, read it back, and hand it to whoever holds the dmabuf.
	buffers, err := dev.AllocateCommandBuffers(&CommandBufferAllocateInfo{
		SType:              StructureTypeCommandBufferAllocateInfo,
		CommandPool:        pool,
		Level:              CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	})
	check(t, "AllocateCommandBuffers", err)
	defer dev.FreeCommandBuffers(pool, buffers)
	cb := buffers[0]
	check(t, "BeginCommandBuffer", dev.BeginCommandBuffer(cb, &CommandBufferBeginInfo{
		SType: StructureTypeCommandBufferBeginInfo,
		Flags: CommandBufferUsageOneTimeSubmit,
	}))
	dev.CmdPipelineBarrier(cb, PipelineStageTopOfPipe, PipelineStageTransfer, 0, nil, nil, []ImageMemoryBarrier{
		transition(img.Image, ImageLayoutUndefined, ImageLayoutTransferDstOptimal, 0, AccessTransferWrite),
	})
	c := unorm(color)
	fill := ClearColorFloat(c[0], c[1], c[2], c[3])
	dev.CmdClearColorImage(cb, img.Image, ImageLayoutTransferDstOptimal, &fill, []ImageSubresourceRange{colorRange})
	dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageTransfer, 0, nil, nil, []ImageMemoryBarrier{
		transition(img.Image, ImageLayoutTransferDstOptimal, ImageLayoutTransferSrcOptimal, AccessTransferWrite, AccessTransferRead),
	})
	dev.CmdCopyImageToBuffer(cb, img.Image, ImageLayoutTransferSrcOptimal, readback.buf, []BufferImageCopy{{
		ImageSubresource: colorLayers,
		ImageExtent:      Extent3D{Width: dmabufWidth, Height: dmabufHeight, Depth: 1},
	}})
	// What a renderer does before it presents the dmabuf: the image goes
	// to the layout other APIs read, and, with
	// VK_EXT_queue_family_foreign, to the foreign queue family.
	release := transition(img.Image, ImageLayoutTransferSrcOptimal, ImageLayoutGeneral, AccessTransferRead, 0)
	if foreign {
		release.SrcQueueFamilyIndex = family
		release.DstQueueFamilyIndex = QueueFamilyForeignEXT
	}
	dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageBottomOfPipe, 0, nil, nil, []ImageMemoryBarrier{release})
	dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageHost, 0, []MemoryBarrier{{
		SType:         StructureTypeMemoryBarrier,
		SrcAccessMask: AccessTransferWrite,
		DstAccessMask: AccessHostRead,
	}}, nil, nil)
	check(t, "EndCommandBuffer", dev.EndCommandBuffer(cb))
	check(t, "QueueSubmit", dev.QueueSubmit(queue, []SubmitInfo{{
		SType:              StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    &cb,
	}}, fence))
	check(t, "WaitForFences", dev.WaitForFences([]Fence{fence}, true, 10e9))
	check(t, "ResetFences", dev.ResetFences([]Fence{fence}))

	want := [4]uint8{}
	for channel, at := range order {
		want[at] = color[channel]
	}
	r.invalidate(readback)
	if x, y, got, ok := firstDifferent(readback.bytes, 4*dmabufWidth, want); !ok {
		t.Errorf("%s: read back through Vulkan, pixel (%d, %d) is %v, want %v", what, x, y, got, want)
	}

	// The same pixels through the dmabuf, where it can be mapped and the
	// layout is rows of pixels.
	if !linear {
		return
	}
	data, err := unix.Mmap(img.FD, 0, int(size), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		t.Logf("%s: the dmabuf cannot be mapped (%v): its pixels are not compared", what, err)
		return
	}
	defer unix.Munmap(data)
	rows := unsafe.Slice(unsafe.SliceData(data), len(data))[main.Offset:]
	for y := range dmabufHeight {
		row := rows[uint64(y)*main.Stride:][:4*dmabufWidth]
		if x, _, got, ok := firstDifferent(row, 4*dmabufWidth, want); !ok {
			t.Errorf("%s: through the mapped dmabuf, pixel (%d, %d) is %v, want %v", what, x, y, got, want)
			return
		}
	}
	t.Logf("%s: the mapped dmabuf has the pixels at offset %d with row pitch %d", what, main.Offset, main.Stride)
}

// firstDifferent returns the first pixel of rows of stride bytes that is
// not want, or ok.
func firstDifferent(pixels []byte, stride int, want [4]uint8) (x, y int, got [4]uint8, ok bool) {
	for i := 0; i+4 <= len(pixels); i += 4 {
		got = [4]uint8(pixels[i : i+4])
		if got != want {
			return i % stride / 4, i / stride, got, false
		}
	}
	return 0, 0, want, true
}
