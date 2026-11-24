//go:build linux && (amd64 || arm64)

package vulkan

import (
	"errors"
	"fmt"
	"slices"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Format is a layout of pixels in shared memory: a DRM fourcc and a DRM
// format modifier.
type Format struct {
	Fourcc   uint32
	Modifier uint64
}

// Linear is DRM_FORMAT_MOD_LINEAR: rows of pixels, one after the other.
const Linear uint64 = 0

// vkFormats are the Vulkan formats of the fourccs, the opaque ones first: the
// scene has no transparency, so what shows it need not blend it.
var vkFormats = []struct {
	fourcc, format uint32
}{
	{0x34325258, formatB8g8r8a8Unorm}, // XR24
	{0x34324258, formatR8g8b8a8Unorm}, // XB24
	{0x34325241, formatB8g8r8a8Unorm}, // AR24
	{0x34324241, formatR8g8b8a8Unorm}, // AB24
}

// Memory says who reads the buffers, which decides where they lie.
type Memory int

const (
	// Own: the GPU that draws them.
	Own Memory = iota
	// Shared: another GPU. The buffers lie in system memory, as no GPU is
	// sure to reach the memory of another one.
	Shared
	// Mapped: the CPU, through a mapping. They lie in system memory that it
	// reads through its caches.
	Mapped
)

// waitLimit bounds the wait for a frame the GPU draws.
const waitLimit = 5e9 // ns

// Renderer draws on a device.
type Renderer struct {
	Device Device
	// Fourcc is the format of its buffers. Copies reports that the device
	// cannot draw into buffers that the reader takes (NVIDIA's draw into no
	// linear ones): it draws each frame beside them and copies it.
	Fourcc uint32
	Copies bool

	i       *Instance
	h       uintptr // VkDevice
	queue   uintptr
	family  uint32
	fn      deviceFuncs
	memory  physicalDeviceMemoryProperties
	readBy  Memory
	format  uint32
	mods    []uint64       // the modifiers to choose from
	planes  map[uint64]int // and their memory planes
	layout  uint64         // VkPipelineLayout
	pipe    uint64         // VkPipeline
	push    uint32         // bytes of push constants
	targets int
}

// NewRenderer makes a renderer of a device. formats are what the reader of
// the buffers takes, the preferred ones first; vert and frag are the SPIR-V of
// the shaders, and push the bytes of push constants the fragment shader gets.
func (i *Instance) NewRenderer(d Device, formats []Format, readBy Memory, vert, frag []byte, push int) (*Renderer, error) {
	if err := i.check(&d); err != nil {
		return nil, err
	}
	family, err := i.graphicsFamily(&d)
	if err != nil {
		return nil, err
	}
	r := &Renderer{Device: d, i: i, family: family, readBy: readBy, push: uint32(push)}
	if err := r.chooseFormat(formats); err != nil {
		return nil, err
	}
	call(i.fn.GetPhysicalDeviceMemoryProperties, d.h, uintptr(unsafe.Pointer(&r.memory)))

	priority := float32(1)
	n, names := cStrings(deviceExtensions)
	info := &deviceCreateInfo{
		SType:                   structureTypeDeviceCreateInfo,
		PNext:                   unsafe.Pointer(&physicalDeviceDynamicRenderingFeatures{SType: structureTypePhysicalDeviceDynamicRenderingFeatures, DynamicRendering: 1}),
		QueueCreateInfoCount:    1,
		PQueueCreateInfos:       &deviceQueueCreateInfo{SType: structureTypeDeviceQueueCreateInfo, QueueFamilyIndex: family, QueueCount: 1, PQueuePriorities: &priority},
		EnabledExtensionCount:   n,
		PpEnabledExtensionNames: names,
	}
	if err := do("vkCreateDevice", i.fn.CreateDevice, d.h, uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&r.h))); err != nil {
		return nil, err
	}
	err = load(&r.fn, func(name string) uintptr {
		return call(i.fn.GetDeviceProcAddr, r.h, uintptr(unsafe.Pointer(cString(name))))
	})
	if err == nil {
		call(r.fn.GetDeviceQueue, r.h, uintptr(family), 0, uintptr(unsafe.Pointer(&r.queue)))
		err = r.makePipeline(vert, frag)
	}
	if err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

// chooseFormat picks the format of the buffers: a layout the device draws
// into, tiled before linear, else linear rows it copies into.
func (r *Renderer) chooseFormat(formats []Format) error {
	for _, try := range []struct {
		linear bool
		usage  uint32
	}{
		{false, imageUsageColorAttachmentBit}, {true, imageUsageColorAttachmentBit}, {true, imageUsageTransferDstBit},
	} {
		for _, f := range vkFormats {
			var own map[uint64]int
			var mods []uint64
			for _, want := range formats {
				if want.Fourcc != f.fourcc || (want.Modifier == Linear) != try.linear {
					continue
				}
				if own == nil {
					own = r.i.modifiers(&r.Device, f.format, try.usage)
				}
				if own[want.Modifier] > 0 && !slices.Contains(mods, want.Modifier) {
					mods = append(mods, want.Modifier)
				}
			}
			if len(mods) > 0 {
				r.Fourcc, r.format, r.mods, r.planes = f.fourcc, f.format, mods, own
				r.Copies = try.usage == imageUsageTransferDstBit
				return nil
			}
		}
	}
	return fmt.Errorf("%s shares images of none of the %d formats taken", r.Device.Name, len(formats))
}

// makePipeline makes the pipeline that draws one triangle with the shaders.
func (r *Renderer) makePipeline(vert, frag []byte) error {
	var modules [2]uint64
	defer func() {
		for _, m := range modules {
			call(r.fn.DestroyShaderModule, r.h, uintptr(m), 0)
		}
	}()
	for k, code := range [][]byte{vert, frag} {
		words := make([]uint32, len(code)/4) // aligned, unlike embedded bytes
		copy(unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(code)), code)
		info := &shaderModuleCreateInfo{SType: structureTypeShaderModuleCreateInfo, CodeSize: uintptr(4 * len(words)), PCode: &words[0]}
		if err := do("vkCreateShaderModule", r.fn.CreateShaderModule, r.h, uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&modules[k]))); err != nil {
			return err
		}
	}
	layout := &pipelineLayoutCreateInfo{
		SType:                  structureTypePipelineLayoutCreateInfo,
		PushConstantRangeCount: 1,
		PPushConstantRanges:    &pushConstantRange{StageFlags: shaderStageFragmentBit, Size: r.push},
	}
	if err := do("vkCreatePipelineLayout", r.fn.CreatePipelineLayout, r.h, uintptr(unsafe.Pointer(layout)), 0, uintptr(unsafe.Pointer(&r.layout))); err != nil {
		return err
	}
	main := cString("main")
	stages := &[2]pipelineShaderStageCreateInfo{
		{SType: structureTypePipelineShaderStageCreateInfo, Stage: shaderStageVertexBit, Module: modules[0], PName: main},
		{SType: structureTypePipelineShaderStageCreateInfo, Stage: shaderStageFragmentBit, Module: modules[1], PName: main},
	}
	dynamic := &[2]uint32{dynamicStateViewport, dynamicStateScissor}
	info := &graphicsPipelineCreateInfo{
		SType:               structureTypeGraphicsPipelineCreateInfo,
		PNext:               unsafe.Pointer(&pipelineRenderingCreateInfo{SType: structureTypePipelineRenderingCreateInfo, ColorAttachmentCount: 1, PColorAttachmentFormats: &r.format}),
		StageCount:          2,
		PStages:             &stages[0],
		PVertexInputState:   &pipelineVertexInputStateCreateInfo{SType: structureTypePipelineVertexInputStateCreateInfo},
		PInputAssemblyState: &pipelineInputAssemblyStateCreateInfo{SType: structureTypePipelineInputAssemblyStateCreateInfo, Topology: primitiveTopologyTriangleList},
		PViewportState:      &pipelineViewportStateCreateInfo{SType: structureTypePipelineViewportStateCreateInfo, ViewportCount: 1, ScissorCount: 1},
		PRasterizationState: &pipelineRasterizationStateCreateInfo{SType: structureTypePipelineRasterizationStateCreateInfo, PolygonMode: polygonModeFill, CullMode: cullModeNone, LineWidth: 1},
		PMultisampleState:   &pipelineMultisampleStateCreateInfo{SType: structureTypePipelineMultisampleStateCreateInfo, RasterizationSamples: sampleCount1Bit},
		PColorBlendState:    &pipelineColorBlendStateCreateInfo{SType: structureTypePipelineColorBlendStateCreateInfo, AttachmentCount: 1, PAttachments: &pipelineColorBlendAttachmentState{ColorWriteMask: 0xf}},
		PDynamicState:       &pipelineDynamicStateCreateInfo{SType: structureTypePipelineDynamicStateCreateInfo, DynamicStateCount: 2, PDynamicStates: &dynamic[0]},
		Layout:              r.layout,
	}
	return do("vkCreateGraphicsPipelines", r.fn.CreateGraphicsPipelines, r.h, 0, 1, uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&r.pipe)))
}

// Close frees the renderer; its targets are closed before.
func (r *Renderer) Close() {
	if r.fn.DestroyDevice == 0 {
		return
	}
	call(r.fn.DeviceWaitIdle, r.h)
	call(r.fn.DestroyPipeline, r.h, uintptr(r.pipe), 0)
	call(r.fn.DestroyPipelineLayout, r.h, uintptr(r.layout), 0)
	call(r.fn.DestroyDevice, r.h, 0)
	r.fn.DestroyDevice = 0
}

// Targets is how many of its targets are open.
func (r *Renderer) Targets() int { return r.targets }

// Target is a buffer to draw frames into.
type Target struct {
	Width, Height int
	// FD is the dmabuf of its memory, which Close closes, with the layout of
	// the pixels in it.
	FD       int
	Modifier uint64
	Planes   []Plane

	r             *Renderer
	shared, drawn gpuImage // the exported image; what is drawn into, when it is copied
	view          uint64   // VkImageView of the one drawn into
	pool          uint64   // VkCommandPool
	cmd           uintptr  // VkCommandBuffer
	fence, done   uint64   // VkFence for Wait, VkSemaphore for the sync file
	pending       bool     // a frame was submitted that Wait did not wait for
}

// Plane is a memory plane of a target's layout.
type Plane struct{ Offset, Stride uint32 }

// gpuImage is an image with its memory.
type gpuImage struct{ img, mem uint64 }

// NewTarget allocates a buffer of w by h pixels.
func (r *Renderer) NewTarget(w, h int) (_ *Target, err error) {
	if w < 1 || h < 1 || w > r.Device.maxSize || h > r.Device.maxSize {
		return nil, fmt.Errorf("%s draws no buffer of %dx%d pixels", r.Device.Name, w, h)
	}
	t := &Target{Width: w, Height: h, FD: -1, r: r}
	r.targets++
	defer func() {
		if err != nil {
			t.Close()
		}
	}()
	if err := t.makeShared(); err != nil {
		return nil, err
	}
	drawn := t.shared.img
	if r.Copies {
		info := t.imageInfo(imageTilingOptimal, imageUsageColorAttachmentBit|imageUsageTransferSrcBit)
		if err := t.makeImage(&t.drawn, info, nil, Own); err != nil {
			return nil, err
		}
		drawn = t.drawn.img
	}
	view := &imageViewCreateInfo{SType: structureTypeImageViewCreateInfo, Image: drawn, ViewType: imageViewType2D, Format: r.format, SubresourceRange: colorRange}
	if err := do("vkCreateImageView", r.fn.CreateImageView, r.h, uintptr(unsafe.Pointer(view)), 0, uintptr(unsafe.Pointer(&t.view))); err != nil {
		return nil, err
	}
	pool := &commandPoolCreateInfo{SType: structureTypeCommandPoolCreateInfo, Flags: commandPoolCreateTransientBit, QueueFamilyIndex: r.family}
	if err := do("vkCreateCommandPool", r.fn.CreateCommandPool, r.h, uintptr(unsafe.Pointer(pool)), 0, uintptr(unsafe.Pointer(&t.pool))); err != nil {
		return nil, err
	}
	cmd := &commandBufferAllocateInfo{SType: structureTypeCommandBufferAllocateInfo, CommandPool: t.pool, Level: commandBufferLevelPrimary, CommandBufferCount: 1}
	if err := do("vkAllocateCommandBuffers", r.fn.AllocateCommandBuffers, r.h, uintptr(unsafe.Pointer(cmd)), uintptr(unsafe.Pointer(&t.cmd))); err != nil {
		return nil, err
	}
	fence := &fenceCreateInfo{SType: structureTypeFenceCreateInfo}
	if err := do("vkCreateFence", r.fn.CreateFence, r.h, uintptr(unsafe.Pointer(fence)), 0, uintptr(unsafe.Pointer(&t.fence))); err != nil {
		return nil, err
	}
	sem := &semaphoreCreateInfo{
		SType: structureTypeSemaphoreCreateInfo,
		PNext: unsafe.Pointer(&exportSemaphoreCreateInfo{SType: structureTypeExportSemaphoreCreateInfo, HandleTypes: externalSemaphoreHandleTypeSyncFdBit}),
	}
	if err := do("vkCreateSemaphore", r.fn.CreateSemaphore, r.h, uintptr(unsafe.Pointer(sem)), 0, uintptr(unsafe.Pointer(&t.done))); err != nil {
		return nil, err
	}
	return t, nil
}

// makeShared makes the exported image.
func (t *Target) makeShared() error {
	r := t.r
	usage := uint32(imageUsageColorAttachmentBit)
	if r.Copies {
		usage = imageUsageTransferDstBit
	}
	info := t.imageInfo(imageTilingDrmFormatModifierEXT, usage)
	mods := slices.Clone(r.mods)
	info.PNext = unsafe.Pointer(&externalMemoryImageCreateInfo{
		SType:       structureTypeExternalMemoryImageCreateInfo,
		PNext:       unsafe.Pointer(&imageDrmFormatModifierListCreateInfoEXT{SType: structureTypeImageDrmFormatModifierListCreateInfoEXT, DrmFormatModifierCount: uint32(len(mods)), PDrmFormatModifiers: &mods[0]}),
		HandleTypes: externalMemoryHandleTypeDmaBufBitEXT,
	})
	export := &exportMemoryAllocateInfo{SType: structureTypeExportMemoryAllocateInfo, HandleTypes: externalMemoryHandleTypeDmaBufBitEXT}
	if err := t.makeImage(&t.shared, info, export, r.readBy); err != nil {
		return err
	}
	fd := int32(-1)
	get := &memoryGetFdInfoKHR{SType: structureTypeMemoryGetFdInfoKHR, Memory: t.shared.mem, HandleType: externalMemoryHandleTypeDmaBufBitEXT}
	if err := do("vkGetMemoryFdKHR", r.fn.GetMemoryFdKHR, r.h, uintptr(unsafe.Pointer(get)), uintptr(unsafe.Pointer(&fd))); err != nil {
		return err
	}
	t.FD = int(fd)
	unix.CloseOnExec(t.FD)

	// Which of the modifiers the driver chose, and where its planes lie.
	mod := &imageDrmFormatModifierPropertiesEXT{SType: structureTypeImageDrmFormatModifierPropertiesEXT}
	if err := do("vkGetImageDrmFormatModifierPropertiesEXT", r.fn.GetImageDrmFormatModifierPropertiesEXT, r.h, uintptr(t.shared.img), uintptr(unsafe.Pointer(mod))); err != nil {
		return err
	}
	t.Modifier = mod.DrmFormatModifier
	for p := range r.planes[t.Modifier] {
		sub := &imageSubresource{AspectMask: imageAspectMemoryPlane0BitEXT << p}
		var l subresourceLayout
		call(r.fn.GetImageSubresourceLayout, r.h, uintptr(t.shared.img), uintptr(unsafe.Pointer(sub)), uintptr(unsafe.Pointer(&l)))
		t.Planes = append(t.Planes, Plane{Offset: uint32(l.Offset), Stride: uint32(l.RowPitch)})
	}
	return nil
}

func (t *Target) imageInfo(tiling, usage uint32) *imageCreateInfo {
	return &imageCreateInfo{
		SType:         structureTypeImageCreateInfo,
		ImageType:     imageType2D,
		Format:        t.r.format,
		Extent:        extent3D{uint32(t.Width), uint32(t.Height), 1},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       sampleCount1Bit,
		Tiling:        tiling,
		Usage:         usage,
		SharingMode:   sharingModeExclusive,
		InitialLayout: imageLayoutUndefined,
	}
}

// makeImage makes an image with memory of its own.
func (t *Target) makeImage(g *gpuImage, info *imageCreateInfo, export *exportMemoryAllocateInfo, readBy Memory) error {
	r := t.r
	if err := do("vkCreateImage", r.fn.CreateImage, r.h, uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&g.img))); err != nil {
		return err
	}
	var req memoryRequirements
	call(r.fn.GetImageMemoryRequirements, r.h, uintptr(g.img), uintptr(unsafe.Pointer(&req)))
	alloc := &memoryAllocateInfo{
		SType:           structureTypeMemoryAllocateInfo,
		PNext:           unsafe.Pointer(&memoryDedicatedAllocateInfo{SType: structureTypeMemoryDedicatedAllocateInfo, PNext: unsafe.Pointer(export), Image: g.img}),
		AllocationSize:  req.Size,
		MemoryTypeIndex: r.memoryType(req.MemoryTypeBits, readBy),
	}
	if err := do("vkAllocateMemory", r.fn.AllocateMemory, r.h, uintptr(unsafe.Pointer(alloc)), 0, uintptr(unsafe.Pointer(&g.mem))); err != nil {
		return err
	}
	return do("vkBindImageMemory", r.fn.BindImageMemory, r.h, uintptr(g.img), uintptr(g.mem), 0)
}

// memoryType returns the best memory type of those in bits for a reader.
func (r *Renderer) memoryType(bits uint32, readBy Memory) uint32 {
	best, most := uint32(0), -1
	for k := range r.memory.MemoryTypeCount {
		flags := r.memory.MemoryTypes[k].PropertyFlags
		if bits&(1<<k) == 0 || flags&(memoryPropertyProtectedBit|memoryPropertyLazilyAllocatedBit) != 0 {
			continue
		}
		score := 0
		if (flags&memoryPropertyDeviceLocalBit != 0) == (readBy == Own) {
			score += 2
		}
		if readBy == Mapped && flags&memoryPropertyHostCachedBit != 0 {
			score++
		}
		if score > most {
			best, most = k, score
		}
	}
	return best
}

// Close frees the buffer, once the GPU is done with it. Memory that others
// imported stays theirs.
func (t *Target) Close() {
	t.Wait() // if it failed, the device is lost: nothing runs
	r := t.r
	call(r.fn.DestroySemaphore, r.h, uintptr(t.done), 0)
	call(r.fn.DestroyFence, r.h, uintptr(t.fence), 0)
	call(r.fn.DestroyCommandPool, r.h, uintptr(t.pool), 0)
	call(r.fn.DestroyImageView, r.h, uintptr(t.view), 0)
	for _, g := range []gpuImage{t.shared, t.drawn} {
		call(r.fn.DestroyImage, r.h, uintptr(g.img), 0)
		call(r.fn.FreeMemory, r.h, uintptr(g.mem), 0)
	}
	if t.FD >= 0 {
		unix.Close(t.FD)
	}
	r.targets--
}

// Wait waits for the GPU to finish the frame drawn last.
func (t *Target) Wait() error {
	if !t.pending {
		return nil
	}
	if err := do("vkWaitForFences", t.r.fn.WaitForFences, t.r.h, 1, uintptr(unsafe.Pointer(&t.fence)), 1, waitLimit); err != nil {
		return err
	}
	t.pending = false
	return do("vkResetFences", t.r.fn.ResetFences, t.r.h, 1, uintptr(unsafe.Pointer(&t.fence)))
}

var colorRange = imageSubresourceRange{AspectMask: imageAspectColorBit, LevelCount: 1, LayerCount: 1}

// Draw draws a frame with the push constants, without waiting for the GPU. It
// returns a sync file that signals when the frame is drawn, -1 if it already
// is, which the caller closes.
//
// Nobody reads the buffer by then (its last frame was released), and the
// pixels of the last frame are not kept: no ownership is taken back.
func (t *Target) Draw(push []byte) (syncFile int, err error) {
	r := t.r
	if len(push) != int(r.push) {
		return -1, errors.New("the push constants are not those of the pipeline")
	}
	if err := t.Wait(); err != nil {
		return -1, err
	}
	if err := do("vkResetCommandPool", r.fn.ResetCommandPool, r.h, uintptr(t.pool), 0); err != nil {
		return -1, err
	}
	begin := &commandBufferBeginInfo{SType: structureTypeCommandBufferBeginInfo, Flags: commandBufferUsageOneTimeSubmitBit}
	if err := do("vkBeginCommandBuffer", r.fn.BeginCommandBuffer, t.cmd, uintptr(unsafe.Pointer(begin))); err != nil {
		return -1, err
	}
	t.record(push)
	if err := do("vkEndCommandBuffer", r.fn.EndCommandBuffer, t.cmd); err != nil {
		return -1, err
	}
	submit := &submitInfo{SType: structureTypeSubmitInfo, CommandBufferCount: 1, PCommandBuffers: &t.cmd, SignalSemaphoreCount: 1, PSignalSemaphores: &t.done}
	if err := do("vkQueueSubmit", r.fn.QueueSubmit, r.queue, 1, uintptr(unsafe.Pointer(submit)), uintptr(t.fence)); err != nil {
		return -1, err
	}
	t.pending = true
	// The export takes the signal out of the semaphore, which is free again
	// once the GPU is done.
	fd := int32(-1)
	get := &semaphoreGetFdInfoKHR{SType: structureTypeSemaphoreGetFdInfoKHR, Semaphore: t.done, HandleType: externalSemaphoreHandleTypeSyncFdBit}
	if err := do("vkGetSemaphoreFdKHR", r.fn.GetSemaphoreFdKHR, r.h, uintptr(unsafe.Pointer(get)), uintptr(unsafe.Pointer(&fd))); err != nil {
		return -1, err
	}
	if fd >= 0 {
		unix.CloseOnExec(int(fd))
	}
	return int(fd), nil
}

// record records the commands of a frame.
func (t *Target) record(push []byte) {
	r := t.r
	area := rect2D{Extent: extent2D{uint32(t.Width), uint32(t.Height)}}
	drawn := t.shared.img
	if r.Copies {
		drawn = t.drawn.img
	}

	t.barrier(drawn, pipelineStageColorAttachmentOutputBit, pipelineStageColorAttachmentOutputBit,
		0, accessColorAttachmentWriteBit, imageLayoutUndefined, imageLayoutColorAttachmentOptimal, queueFamilyIgnored)
	info := &renderingInfo{
		SType:                structureTypeRenderingInfo,
		RenderArea:           area,
		LayerCount:           1,
		ColorAttachmentCount: 1,
		PColorAttachments: &renderingAttachmentInfo{
			SType:       structureTypeRenderingAttachmentInfo,
			ImageView:   t.view,
			ImageLayout: imageLayoutColorAttachmentOptimal,
			LoadOp:      attachmentLoadOpDontCare, // every pixel is drawn
			StoreOp:     attachmentStoreOpStore,
		},
	}
	call(r.fn.CmdBeginRendering, t.cmd, uintptr(unsafe.Pointer(info)))
	call(r.fn.CmdBindPipeline, t.cmd, pipelineBindPointGraphics, uintptr(r.pipe))
	view := &viewport{Width: float32(t.Width), Height: float32(t.Height), MaxDepth: 1}
	call(r.fn.CmdSetViewport, t.cmd, 0, 1, uintptr(unsafe.Pointer(view)))
	call(r.fn.CmdSetScissor, t.cmd, 0, 1, uintptr(unsafe.Pointer(&area)))
	call(r.fn.CmdPushConstants, t.cmd, uintptr(r.layout), shaderStageFragmentBit, 0, uintptr(len(push)), uintptr(unsafe.Pointer(&push[0])))
	call(r.fn.CmdDraw, t.cmd, 3, 1, 0, 0)
	call(r.fn.CmdEndRendering, t.cmd)

	// Whoever shows the frame finds it in the general layout, given to the
	// foreign queue family.
	if !r.Copies {
		t.barrier(drawn, pipelineStageColorAttachmentOutputBit, pipelineStageBottomOfPipeBit,
			accessColorAttachmentWriteBit, 0, imageLayoutColorAttachmentOptimal, imageLayoutGeneral, queueFamilyForeignEXT)
		return
	}
	t.barrier(drawn, pipelineStageColorAttachmentOutputBit, pipelineStageTransferBit,
		accessColorAttachmentWriteBit, accessTransferReadBit, imageLayoutColorAttachmentOptimal, imageLayoutTransferSrcOptimal, queueFamilyIgnored)
	t.barrier(t.shared.img, pipelineStageTopOfPipeBit, pipelineStageTransferBit,
		0, accessTransferWriteBit, imageLayoutUndefined, imageLayoutTransferDstOptimal, queueFamilyIgnored)
	layers := imageSubresourceLayers{AspectMask: imageAspectColorBit, LayerCount: 1}
	region := &imageCopy{SrcSubresource: layers, DstSubresource: layers, Extent: extent3D{uint32(t.Width), uint32(t.Height), 1}}
	call(r.fn.CmdCopyImage, t.cmd, uintptr(drawn), imageLayoutTransferSrcOptimal, uintptr(t.shared.img), imageLayoutTransferDstOptimal, 1, uintptr(unsafe.Pointer(region)))
	t.barrier(t.shared.img, pipelineStageTransferBit, pipelineStageBottomOfPipeBit,
		accessTransferWriteBit, 0, imageLayoutTransferDstOptimal, imageLayoutGeneral, queueFamilyForeignEXT)
}

// barrier records a barrier of an image, which gives it to the queue family
// dst unless that is queueFamilyIgnored.
func (t *Target) barrier(img uint64, srcStage, dstStage, srcAccess, dstAccess, from, to uint32, dst uint32) {
	src := t.r.family
	if dst == queueFamilyIgnored {
		src = queueFamilyIgnored
	}
	b := &imageMemoryBarrier{
		SType:               structureTypeImageMemoryBarrier,
		SrcAccessMask:       srcAccess,
		DstAccessMask:       dstAccess,
		OldLayout:           from,
		NewLayout:           to,
		SrcQueueFamilyIndex: src,
		DstQueueFamilyIndex: dst,
		Image:               img,
		SubresourceRange:    colorRange,
	}
	call(t.r.fn.CmdPipelineBarrier, t.cmd, uintptr(srcStage), uintptr(dstStage), 0, 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(b)))
}
