//go:build telagpu && (linux || windows)

package vk

import (
	_ "embed"
	"encoding/binary"
	"math"
	"testing"
	"unsafe"
)

// The shaders of TestRender, compiled by `go generate ./internal/vk`.
var (
	//go:embed testdata/shaders/quad.vert.spv
	quadVertSPV []byte
	//go:embed testdata/shaders/quad.frag.spv
	quadFragSPV []byte
)

// The scene of TestRender. Every color is chosen so that the result of
// blending is an exact 8-bit value, far from a rounding boundary, and
// every edge lies on a pixel boundary: the pixels are compared exactly.
const (
	targetSize = 64 // the render target, in pixels
	copySize   = 32 // the image the target is blitted and copied into
)

type rgba [4]uint8

var (
	// clearColor is what the render pass clears the target to. Under the
	// half-transparent red below, an odd channel c gives 127.5 + c/2,
	// whole, for the channel the red adds to, and an even one c/2 for
	// the others.
	clearColor = rgba{51, 102, 204, 255}
	// halfRed is premultiplied (0.5, 0, 0, 0.5); blended over clearColor
	// with ONE, ONE_MINUS_SRC_ALPHA it gives halfRedOver.
	halfRed     = [4]float32{0.5, 0, 0, 0.5}
	halfRedOver = rgba{153, 51, 102, 255}
	// attachmentClear is what vkCmdClearAttachments writes into a corner.
	attachmentClear = rgba{255, 255, 0, 255}
	// copyClear is what vkCmdClearColorImage fills the second image
	// with.
	copyClear = rgba{17, 34, 51, 68}
	// texels is the 2x2 texture of the instanced quads: three opaque
	// texels and a transparent one, which leaves the target as it was.
	texels = [4]rgba{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {0, 0, 0, 0}}
)

// rect is a rectangle of pixels: x, y, width, height.
type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

var (
	redRect      = rect{8, 8, 32, 32}  // the half-transparent quad
	redScissor   = rect{0, 0, 24, 64}  // cuts it at x = 24
	texRect      = rect{40, 8, 16, 16} // the first textured instance
	texStep      = 24                  // pixels down to the second one
	clearedRect  = rect{0, 48, 16, 16} // vkCmdClearAttachments
	blitSrc      = rect{4, 4, 8, 8}    // blitted to twice its size
	blitDst      = rect{0, 0, 16, 16}
	imageCopySrc = rect{40, 8, 16, 16} // the first textured instance again
	imageCopyDst = rect{16, 16, 16, 16}
)

// wantTarget is the pixel (x, y) the scene must leave in the target.
func wantTarget(x, y int) rgba {
	if clearedRect.contains(x, y) {
		return attachmentClear
	}
	if redRect.contains(x, y) && redScissor.contains(x, y) {
		return halfRedOver
	}
	for instance := range 2 {
		r := texRect
		r.y += instance * texStep
		if r.contains(x, y) {
			// Nearest sampling: each texel covers a quarter of the quad.
			texel := texels[(y-r.y)/(r.h/2)*2+(x-r.x)/(r.w/2)]
			if texel[3] == 0 {
				return clearColor
			}
			return texel
		}
	}
	return clearColor
}

// wantCopy is the pixel (x, y) of the second image: a clear, a part of
// the target blitted to twice its size, and a part copied as it is.
func wantCopy(x, y int) rgba {
	if imageCopyDst.contains(x, y) {
		return wantTarget(imageCopySrc.x+x-imageCopyDst.x, imageCopySrc.y+y-imageCopyDst.y)
	}
	if blitDst.contains(x, y) {
		return wantTarget(blitSrc.x+(x-blitDst.x)/2, blitSrc.y+(y-blitDst.y)/2)
	}
	return copyClear
}

// drawConstants is the push constant block of the shaders.
type drawConstants struct {
	Color        [4]float32
	Rect         [4]float32
	InstanceStep [2]float32
}

// TestRender is the smoke test of the bindings on real hardware. On every
// physical device it creates a device and draws offscreen into an
// R8G8B8A8_UNORM image: a clear, an indexed quad with a scissor and
// premultiplied source-over blending, two instances of a textured quad,
// and a clear of a corner, with a pipeline that has vertex and index
// buffers, a uniform buffer, a sampled texture and push constants. It
// then blits and copies the image into a second one, copies both to
// buffers, and one buffer to another, and compares every pixel with the
// expected value. All the commands of the bindings are called at least
// once, so with TELA_VK_VALIDATE=1 the validation layer sees every struct
// they take.
func TestRender(t *testing.T) {
	inst, msgs := newTestInstance(t)
	for _, info := range testDevices(t, inst) {
		t.Run(info.Name, func(t *testing.T) {
			// Registered first, so it runs after the device is destroyed
			// and sees what that reports (objects left alive).
			t.Cleanup(func() { msgs.report(t) })
			render(t, inst, info)
		})
	}
}

// renderer holds what the steps of TestRender share.
type renderer struct {
	t   *testing.T
	dev *Device
	mem *PhysicalDeviceMemoryProperties
}

// hostBuffer is a buffer in mapped memory.
type hostBuffer struct {
	buf   Buffer
	mem   DeviceMemory
	bytes []byte // the mapping, as long as the buffer
}

// newBuffer creates a buffer of size bytes in host-visible memory and
// maps it. Its cleanup is registered with the test.
func (r *renderer) newBuffer(size int, usage BufferUsageFlags) *hostBuffer {
	r.t.Helper()
	buf, err := r.dev.CreateBuffer(&BufferCreateInfo{
		SType:       StructureTypeBufferCreateInfo,
		Size:        uint64(size),
		Usage:       usage,
		SharingMode: SharingModeExclusive,
	})
	check(r.t, "CreateBuffer", err)
	r.t.Cleanup(func() { r.dev.DestroyBuffer(buf) })

	req := r.dev.GetBufferMemoryRequirements(buf)
	if req.Size < uint64(size) || req.Alignment == 0 || req.MemoryTypeBits == 0 {
		r.t.Fatalf("memory requirements of a %d byte buffer: %+v", size, req)
	}
	typ, ok := r.mem.FindMemoryType(req.MemoryTypeBits, MemoryPropertyHostVisible|MemoryPropertyHostCoherent)
	if !ok {
		r.t.Fatalf("no host-visible memory type among %#x", req.MemoryTypeBits)
	}
	mem, err := r.dev.AllocateMemory(&MemoryAllocateInfo{
		SType:           StructureTypeMemoryAllocateInfo,
		AllocationSize:  req.Size,
		MemoryTypeIndex: typ,
	})
	check(r.t, "AllocateMemory", err)
	r.t.Cleanup(func() { r.dev.FreeMemory(mem) })
	check(r.t, "BindBufferMemory", r.dev.BindBufferMemory(buf, mem, 0))

	p, err := r.dev.MapMemory(mem, 0, WholeSize, 0)
	check(r.t, "MapMemory", err)
	if p == nil {
		r.t.Fatal("MapMemory returned no pointer")
	}
	r.t.Cleanup(func() { r.dev.UnmapMemory(mem) })
	return &hostBuffer{buf: buf, mem: mem, bytes: unsafe.Slice((*byte)(p), size)}
}

// flush makes the bytes written to the buffers visible to the device.
// The memory is coherent, so this is only required of other memory; it is
// called to exercise the command.
func (r *renderer) flush(buffers ...*hostBuffer) {
	r.t.Helper()
	check(r.t, "FlushMappedMemoryRanges", r.dev.FlushMappedMemoryRanges(memoryRanges(buffers)))
}

// invalidate makes the bytes the device wrote visible in the mappings.
func (r *renderer) invalidate(buffers ...*hostBuffer) {
	r.t.Helper()
	check(r.t, "InvalidateMappedMemoryRanges", r.dev.InvalidateMappedMemoryRanges(memoryRanges(buffers)))
}

func memoryRanges(buffers []*hostBuffer) []MappedMemoryRange {
	ranges := make([]MappedMemoryRange, len(buffers))
	for i, b := range buffers {
		ranges[i] = MappedMemoryRange{SType: StructureTypeMappedMemoryRange, Memory: b.mem, Size: WholeSize}
	}
	return ranges
}

// newImage creates a 2D R8G8B8A8_UNORM image in device memory.
func (r *renderer) newImage(width, height int, usage ImageUsageFlags) Image {
	r.t.Helper()
	img, err := r.dev.CreateImage(&ImageCreateInfo{
		SType:         StructureTypeImageCreateInfo,
		ImageType:     ImageType2D,
		Format:        FormatR8G8B8A8Unorm,
		Extent:        Extent3D{Width: uint32(width), Height: uint32(height), Depth: 1},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       SampleCount1,
		Tiling:        ImageTilingOptimal,
		Usage:         usage,
		SharingMode:   SharingModeExclusive,
		InitialLayout: ImageLayoutUndefined,
	})
	check(r.t, "CreateImage", err)
	r.t.Cleanup(func() { r.dev.DestroyImage(img) })

	req := r.dev.GetImageMemoryRequirements(img)
	typ, ok := r.mem.FindMemoryType(req.MemoryTypeBits, MemoryPropertyDeviceLocal)
	if !ok {
		r.t.Fatalf("no device-local memory type among %#x", req.MemoryTypeBits)
	}
	mem, err := r.dev.AllocateMemory(&MemoryAllocateInfo{
		SType:           StructureTypeMemoryAllocateInfo,
		AllocationSize:  req.Size,
		MemoryTypeIndex: typ,
	})
	check(r.t, "AllocateMemory", err)
	r.t.Cleanup(func() { r.dev.FreeMemory(mem) })
	check(r.t, "BindImageMemory", r.dev.BindImageMemory(img, mem, 0))
	return img
}

// newView creates a view of the whole of an image of newImage.
func (r *renderer) newView(img Image) ImageView {
	r.t.Helper()
	view, err := r.dev.CreateImageView(&ImageViewCreateInfo{
		SType:    StructureTypeImageViewCreateInfo,
		Image:    img,
		ViewType: ImageViewType2D,
		Format:   FormatR8G8B8A8Unorm,
		Components: ComponentMapping{
			R: ComponentSwizzleIdentity, G: ComponentSwizzleIdentity, B: ComponentSwizzleIdentity, A: ComponentSwizzleIdentity,
		},
		SubresourceRange: colorRange,
	})
	check(r.t, "CreateImageView", err)
	r.t.Cleanup(func() { r.dev.DestroyImageView(view) })
	return view
}

var (
	colorRange  = ImageSubresourceRange{AspectMask: ImageAspectColor, LevelCount: 1, LayerCount: 1}
	colorLayers = ImageSubresourceLayers{AspectMask: ImageAspectColor, LayerCount: 1}
)

// transition returns the barrier that moves the whole of an image from
// one layout to another.
func transition(img Image, from, to ImageLayout, srcAccess, dstAccess AccessFlags) ImageMemoryBarrier {
	return ImageMemoryBarrier{
		SType:               StructureTypeImageMemoryBarrier,
		SrcAccessMask:       srcAccess,
		DstAccessMask:       dstAccess,
		OldLayout:           from,
		NewLayout:           to,
		SrcQueueFamilyIndex: QueueFamilyIgnored,
		DstQueueFamilyIndex: QueueFamilyIgnored,
		Image:               img,
		SubresourceRange:    colorRange,
	}
}

func unorm(c rgba) [4]float32 {
	return [4]float32{float32(c[0]) / 255, float32(c[1]) / 255, float32(c[2]) / 255, float32(c[3]) / 255}
}

func putFloats(b []byte, values ...float32) {
	for i, v := range values {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(v))
	}
}

func render(t *testing.T, inst *Instance, info DeviceInfo) {
	pd := info.Device

	// What the scene needs of the format, read through three structs.
	format := inst.GetPhysicalDeviceFormatProperties(pd, FormatR8G8B8A8Unorm)
	const needed = FormatFeatureColorAttachment | FormatFeatureColorAttachmentBlend | FormatFeatureSampledImage | FormatFeatureBlitSrc | FormatFeatureBlitDst | FormatFeatureTransferSrc | FormatFeatureTransferDst
	if format.OptimalTilingFeatures&needed != needed {
		t.Fatalf("R8G8B8A8_UNORM has the features %#x, without some of %#x that Vulkan requires", format.OptimalTilingFeatures, FormatFeatureFlags(needed))
	}
	const targetUsage = ImageUsageColorAttachment | ImageUsageTransferSrc
	limits, err := inst.GetPhysicalDeviceImageFormatProperties(pd, FormatR8G8B8A8Unorm, ImageType2D, ImageTilingOptimal, targetUsage, 0)
	check(t, "GetPhysicalDeviceImageFormatProperties", err)
	if limits.MaxExtent.Width < 4096 || limits.MaxExtent.Height < 4096 || limits.MaxExtent.Depth != 1 || limits.MaxMipLevels == 0 || limits.SampleCounts&SampleCount1 == 0 || limits.MaxResourceSize == 0 {
		t.Fatalf("limits of a 2D color target read wrong: %+v", limits)
	}
	if _, err := inst.GetPhysicalDeviceImageFormatProperties(pd, FormatR8G8B8A8Unorm, ImageType2D, ImageTilingOptimal, ImageUsageDepthStencilAttachment, 0); err != ErrorFormatNotSupported {
		t.Errorf("a color format as a depth attachment: %v, want %v", err, ErrorFormatNotSupported)
	}
	features := inst.GetPhysicalDeviceFeatures(pd)
	if features.RobustBufferAccess != True {
		t.Errorf("robustBufferAccess, the first feature and one every device has, is %d", features.RobustBufferAccess)
	}

	dev := newTestDevice(t, inst, info)
	queue := dev.GetQueue(info.GraphicsQueueFamily, 0)
	if queue == 0 {
		t.Fatal("GetQueue returned no queue")
	}
	r := &renderer{t: t, dev: dev, mem: inst.GetPhysicalDeviceMemoryProperties(pd)}

	// Buffers: geometry, the uniform block, the texels to upload and the
	// three the results are read from.
	//
	// Vertices are a corner of the unit square and its texture
	// coordinate. The first four are the corners, for the indexed draw;
	// the other six are the two triangles, for the plain one.
	corners := [][2]float32{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	indices := []uint16{0, 1, 2, 2, 1, 3}
	vertices := r.newBuffer((len(corners)+len(indices))*16, BufferUsageVertexBuffer)
	for i, c := range corners {
		putFloats(vertices.bytes[16*i:], c[0], c[1], c[0], c[1])
	}
	for i, index := range indices {
		c := corners[index]
		putFloats(vertices.bytes[16*(len(corners)+i):], c[0], c[1], c[0], c[1])
	}
	indexBuffer := r.newBuffer(len(indices)*2, BufferUsageIndexBuffer)
	for i, index := range indices {
		binary.LittleEndian.PutUint16(indexBuffer.bytes[2*i:], index)
	}
	uniforms := r.newBuffer(16, BufferUsageUniformBuffer)
	putFloats(uniforms.bytes, 2.0/targetSize, 2.0/targetSize, -1, -1)
	// The 2x2 texture, then the white texel of the untextured quad.
	staging := r.newBuffer(4*len(texels)+4, BufferUsageTransferSrc)
	for i, texel := range texels {
		copy(staging.bytes[4*i:], texel[:])
	}
	copy(staging.bytes[4*len(texels):], []byte{255, 255, 255, 255})
	r.flush(vertices, indexBuffer, uniforms, staging)

	targetBytes := targetSize * targetSize * 4
	targetOut := r.newBuffer(targetBytes, BufferUsageTransferSrc|BufferUsageTransferDst)
	targetOutCopy := r.newBuffer(targetBytes, BufferUsageTransferDst)
	copyOut := r.newBuffer(copySize*copySize*4, BufferUsageTransferDst)

	// Images.
	target := r.newImage(targetSize, targetSize, targetUsage)
	targetView := r.newView(target)
	check(t, "SetDebugUtilsObjectNameEXT", dev.SetDebugUtilsObjectNameEXT(ObjectTypeImage, uint64(target), "render target"))
	second := r.newImage(copySize, copySize, ImageUsageTransferSrc|ImageUsageTransferDst)
	texture := r.newImage(2, 2, ImageUsageSampled|ImageUsageTransferDst)
	textureView := r.newView(texture)
	white := r.newImage(1, 1, ImageUsageSampled|ImageUsageTransferDst)
	whiteView := r.newView(white)

	sampler, err := dev.CreateSampler(&SamplerCreateInfo{
		SType:        StructureTypeSamplerCreateInfo,
		MagFilter:    FilterNearest,
		MinFilter:    FilterNearest,
		MipmapMode:   SamplerMipmapModeNearest,
		AddressModeU: SamplerAddressModeClampToEdge,
		AddressModeV: SamplerAddressModeClampToEdge,
		AddressModeW: SamplerAddressModeClampToEdge,
		MaxLod:       0,
		BorderColor:  BorderColorFloatTransparentBlack,
	})
	check(t, "CreateSampler", err)
	t.Cleanup(func() { dev.DestroySampler(sampler) })

	// The render pass clears the target and leaves it ready to be copied
	// from.
	renderPass, err := dev.CreateRenderPass(&RenderPassCreateInfo{
		SType:           StructureTypeRenderPassCreateInfo,
		AttachmentCount: 1,
		PAttachments: &AttachmentDescription{
			Format:         FormatR8G8B8A8Unorm,
			Samples:        SampleCount1,
			LoadOp:         AttachmentLoadOpClear,
			StoreOp:        AttachmentStoreOpStore,
			StencilLoadOp:  AttachmentLoadOpDontCare,
			StencilStoreOp: AttachmentStoreOpDontCare,
			InitialLayout:  ImageLayoutUndefined,
			FinalLayout:    ImageLayoutTransferSrcOptimal,
		},
		SubpassCount: 1,
		PSubpasses: &SubpassDescription{
			PipelineBindPoint:    PipelineBindPointGraphics,
			ColorAttachmentCount: 1,
			PColorAttachments:    &AttachmentReference{Attachment: 0, Layout: ImageLayoutColorAttachmentOptimal},
		},
		DependencyCount: 2,
		PDependencies: unsafe.SliceData([]SubpassDependency{
			{
				SrcSubpass:    SubpassExternal,
				DstSubpass:    0,
				SrcStageMask:  PipelineStageColorAttachmentOutput,
				DstStageMask:  PipelineStageColorAttachmentOutput,
				DstAccessMask: AccessColorAttachmentWrite,
			},
			{
				SrcSubpass:    0,
				DstSubpass:    SubpassExternal,
				SrcStageMask:  PipelineStageColorAttachmentOutput,
				DstStageMask:  PipelineStageTransfer,
				SrcAccessMask: AccessColorAttachmentWrite,
				DstAccessMask: AccessTransferRead,
			},
		}),
	})
	check(t, "CreateRenderPass", err)
	t.Cleanup(func() { dev.DestroyRenderPass(renderPass) })

	framebuffer, err := dev.CreateFramebuffer(&FramebufferCreateInfo{
		SType:           StructureTypeFramebufferCreateInfo,
		RenderPass:      renderPass,
		AttachmentCount: 1,
		PAttachments:    &targetView,
		Width:           targetSize,
		Height:          targetSize,
		Layers:          1,
	})
	check(t, "CreateFramebuffer", err)
	t.Cleanup(func() { dev.DestroyFramebuffer(framebuffer) })

	// Descriptors: the uniform block and a texture, in two sets that
	// differ in the texture.
	setLayout, err := dev.CreateDescriptorSetLayout(&DescriptorSetLayoutCreateInfo{
		SType:        StructureTypeDescriptorSetLayoutCreateInfo,
		BindingCount: 2,
		PBindings: unsafe.SliceData([]DescriptorSetLayoutBinding{
			{Binding: 0, DescriptorType: DescriptorTypeUniformBuffer, DescriptorCount: 1, StageFlags: ShaderStageVertex},
			{Binding: 1, DescriptorType: DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: ShaderStageFragment},
		}),
	})
	check(t, "CreateDescriptorSetLayout", err)
	t.Cleanup(func() { dev.DestroyDescriptorSetLayout(setLayout) })

	descriptorPool, err := dev.CreateDescriptorPool(&DescriptorPoolCreateInfo{
		SType:         StructureTypeDescriptorPoolCreateInfo,
		Flags:         DescriptorPoolCreateFreeDescriptorSet,
		MaxSets:       3,
		PoolSizeCount: 2,
		PPoolSizes: unsafe.SliceData([]DescriptorPoolSize{
			{Type: DescriptorTypeUniformBuffer, DescriptorCount: 3},
			{Type: DescriptorTypeCombinedImageSampler, DescriptorCount: 3},
		}),
	})
	check(t, "CreateDescriptorPool", err)
	t.Cleanup(func() { dev.DestroyDescriptorPool(descriptorPool) })

	sets, err := dev.AllocateDescriptorSets(&DescriptorSetAllocateInfo{
		SType:              StructureTypeDescriptorSetAllocateInfo,
		DescriptorPool:     descriptorPool,
		DescriptorSetCount: 3,
		PSetLayouts:        unsafe.SliceData([]DescriptorSetLayout{setLayout, setLayout, setLayout}),
	})
	check(t, "AllocateDescriptorSets", err)
	if len(sets) != 3 || sets[0] == 0 || sets[1] == 0 || sets[2] == 0 || sets[0] == sets[1] {
		t.Fatalf("AllocateDescriptorSets returned %v", sets)
	}
	// The third set is only there to be freed.
	check(t, "FreeDescriptorSets", dev.FreeDescriptorSets(descriptorPool, sets[2:]))
	whiteSet, textureSet := sets[0], sets[1]

	uniformInfo := &DescriptorBufferInfo{Buffer: uniforms.buf, Offset: 0, Range: WholeSize}
	var writes []WriteDescriptorSet
	for _, s := range []struct {
		set  DescriptorSet
		view ImageView
	}{{whiteSet, whiteView}, {textureSet, textureView}} {
		writes = append(writes,
			WriteDescriptorSet{
				SType:           StructureTypeWriteDescriptorSet,
				DstSet:          s.set,
				DstBinding:      0,
				DescriptorCount: 1,
				DescriptorType:  DescriptorTypeUniformBuffer,
				PBufferInfo:     uniformInfo,
			},
			WriteDescriptorSet{
				SType:           StructureTypeWriteDescriptorSet,
				DstSet:          s.set,
				DstBinding:      1,
				DescriptorCount: 1,
				DescriptorType:  DescriptorTypeCombinedImageSampler,
				PImageInfo:      &DescriptorImageInfo{Sampler: sampler, ImageView: s.view, ImageLayout: ImageLayoutShaderReadOnlyOptimal},
			})
	}
	dev.UpdateDescriptorSets(writes, nil)

	// The pipeline.
	const drawStages = ShaderStageVertex | ShaderStageFragment
	pipelineLayout, err := dev.CreatePipelineLayout(&PipelineLayoutCreateInfo{
		SType:                  StructureTypePipelineLayoutCreateInfo,
		SetLayoutCount:         1,
		PSetLayouts:            &setLayout,
		PushConstantRangeCount: 1,
		PPushConstantRanges:    &PushConstantRange{StageFlags: drawStages, Offset: 0, Size: uint32(unsafe.Sizeof(drawConstants{}))},
	})
	check(t, "CreatePipelineLayout", err)
	t.Cleanup(func() { dev.DestroyPipelineLayout(pipelineLayout) })

	module := func(code []byte) ShaderModule {
		words := SPIRV(code)
		m, err := dev.CreateShaderModule(&ShaderModuleCreateInfo{
			SType:    StructureTypeShaderModuleCreateInfo,
			CodeSize: uintptr(len(code)),
			PCode:    &words[0],
		})
		check(t, "CreateShaderModule", err)
		t.Cleanup(func() { dev.DestroyShaderModule(m) })
		return m
	}
	entry := CString("main")
	pipelines, err := dev.CreateGraphicsPipelines(0, []GraphicsPipelineCreateInfo{{
		SType:      StructureTypeGraphicsPipelineCreateInfo,
		StageCount: 2,
		PStages: unsafe.SliceData([]PipelineShaderStageCreateInfo{
			{SType: StructureTypePipelineShaderStageCreateInfo, Stage: ShaderStageVertex, Module: module(quadVertSPV), PName: entry},
			{SType: StructureTypePipelineShaderStageCreateInfo, Stage: ShaderStageFragment, Module: module(quadFragSPV), PName: entry},
		}),
		PVertexInputState: &PipelineVertexInputStateCreateInfo{
			SType:                           StructureTypePipelineVertexInputStateCreateInfo,
			VertexBindingDescriptionCount:   1,
			PVertexBindingDescriptions:      &VertexInputBindingDescription{Binding: 0, Stride: 16, InputRate: VertexInputRateVertex},
			VertexAttributeDescriptionCount: 2,
			PVertexAttributeDescriptions: unsafe.SliceData([]VertexInputAttributeDescription{
				{Location: 0, Binding: 0, Format: FormatR32G32Sfloat, Offset: 0},
				{Location: 1, Binding: 0, Format: FormatR32G32Sfloat, Offset: 8},
			}),
		},
		PInputAssemblyState: &PipelineInputAssemblyStateCreateInfo{
			SType:    StructureTypePipelineInputAssemblyStateCreateInfo,
			Topology: PrimitiveTopologyTriangleList,
		},
		PViewportState: &PipelineViewportStateCreateInfo{
			SType:         StructureTypePipelineViewportStateCreateInfo,
			ViewportCount: 1, // set by commands
			ScissorCount:  1,
		},
		PRasterizationState: &PipelineRasterizationStateCreateInfo{
			SType:       StructureTypePipelineRasterizationStateCreateInfo,
			PolygonMode: PolygonModeFill,
			CullMode:    CullModeNone,
			FrontFace:   FrontFaceCounterClockwise,
			LineWidth:   1,
		},
		PMultisampleState: &PipelineMultisampleStateCreateInfo{
			SType:                StructureTypePipelineMultisampleStateCreateInfo,
			RasterizationSamples: SampleCount1,
		},
		PColorBlendState: &PipelineColorBlendStateCreateInfo{
			SType:           StructureTypePipelineColorBlendStateCreateInfo,
			AttachmentCount: 1,
			// Source-over for premultiplied colors.
			PAttachments: &PipelineColorBlendAttachmentState{
				BlendEnable:         True,
				SrcColorBlendFactor: BlendFactorOne,
				DstColorBlendFactor: BlendFactorOneMinusSrcAlpha,
				ColorBlendOp:        BlendOpAdd,
				SrcAlphaBlendFactor: BlendFactorOne,
				DstAlphaBlendFactor: BlendFactorOneMinusSrcAlpha,
				AlphaBlendOp:        BlendOpAdd,
				ColorWriteMask:      ColorComponentR | ColorComponentG | ColorComponentB | ColorComponentA,
			},
		},
		PDynamicState: &PipelineDynamicStateCreateInfo{
			SType:             StructureTypePipelineDynamicStateCreateInfo,
			DynamicStateCount: 3,
			PDynamicStates:    unsafe.SliceData([]DynamicState{DynamicStateViewport, DynamicStateScissor, DynamicStateBlendConstants}),
		},
		Layout:            pipelineLayout,
		RenderPass:        renderPass,
		Subpass:           0,
		BasePipelineIndex: -1,
	}})
	check(t, "CreateGraphicsPipelines", err)
	pipeline := pipelines[0]
	if pipeline == 0 {
		t.Fatal("CreateGraphicsPipelines returned no pipeline")
	}
	t.Cleanup(func() { dev.DestroyPipeline(pipeline) })

	// Commands.
	pool, err := dev.CreateCommandPool(&CommandPoolCreateInfo{
		SType:            StructureTypeCommandPoolCreateInfo,
		Flags:            CommandPoolCreateResetCommandBuffer,
		QueueFamilyIndex: info.GraphicsQueueFamily,
	})
	check(t, "CreateCommandPool", err)
	t.Cleanup(func() { dev.DestroyCommandPool(pool) })
	buffers, err := dev.AllocateCommandBuffers(&CommandBufferAllocateInfo{
		SType:              StructureTypeCommandBufferAllocateInfo,
		CommandPool:        pool,
		Level:              CommandBufferLevelPrimary,
		CommandBufferCount: 2,
	})
	check(t, "AllocateCommandBuffers", err)
	if len(buffers) != 2 || buffers[0] == 0 || buffers[1] == 0 {
		t.Fatalf("AllocateCommandBuffers returned %v", buffers)
	}
	dev.FreeCommandBuffers(pool, buffers[1:]) // only there to be freed
	cb := buffers[0]

	check(t, "BeginCommandBuffer", dev.BeginCommandBuffer(cb, &CommandBufferBeginInfo{
		SType: StructureTypeCommandBufferBeginInfo,
		Flags: CommandBufferUsageOneTimeSubmit,
	}))

	// Upload the two textures; the second image waits to be written.
	dev.CmdPipelineBarrier(cb, PipelineStageTopOfPipe, PipelineStageTransfer, 0, nil, nil, []ImageMemoryBarrier{
		transition(texture, ImageLayoutUndefined, ImageLayoutTransferDstOptimal, 0, AccessTransferWrite),
		transition(white, ImageLayoutUndefined, ImageLayoutTransferDstOptimal, 0, AccessTransferWrite),
		transition(second, ImageLayoutUndefined, ImageLayoutTransferDstOptimal, 0, AccessTransferWrite),
	})
	dev.CmdCopyBufferToImage(cb, staging.buf, texture, ImageLayoutTransferDstOptimal, []BufferImageCopy{{
		BufferOffset:     0,
		ImageSubresource: colorLayers,
		ImageExtent:      Extent3D{Width: 2, Height: 2, Depth: 1},
	}})
	dev.CmdCopyBufferToImage(cb, staging.buf, white, ImageLayoutTransferDstOptimal, []BufferImageCopy{{
		BufferOffset:     uint64(4 * len(texels)),
		ImageSubresource: colorLayers,
		ImageExtent:      Extent3D{Width: 1, Height: 1, Depth: 1},
	}})
	dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageFragmentShader, 0, nil, nil, []ImageMemoryBarrier{
		transition(texture, ImageLayoutTransferDstOptimal, ImageLayoutShaderReadOnlyOptimal, AccessTransferWrite, AccessShaderRead),
		transition(white, ImageLayoutTransferDstOptimal, ImageLayoutShaderReadOnlyOptimal, AccessTransferWrite, AccessShaderRead),
	})

	// Draw.
	clear := unorm(clearColor)
	whole := Rect2D{Extent: Extent2D{Width: targetSize, Height: targetSize}}
	dev.CmdBeginRenderPass(cb, &RenderPassBeginInfo{
		SType:           StructureTypeRenderPassBeginInfo,
		RenderPass:      renderPass,
		Framebuffer:     framebuffer,
		RenderArea:      whole,
		ClearValueCount: 1,
		PClearValues:    &[]ClearValue{ClearColor(clear[0], clear[1], clear[2], clear[3])}[0],
	}, SubpassContentsInline)
	dev.CmdBindPipeline(cb, PipelineBindPointGraphics, pipeline)
	dev.CmdSetViewport(cb, 0, []Viewport{{Width: targetSize, Height: targetSize, MinDepth: 0, MaxDepth: 1}})
	dev.CmdSetBlendConstants(cb, &[4]float32{0, 0, 0, 0})
	dev.CmdBindVertexBuffers(cb, 0, []Buffer{vertices.buf}, []uint64{0})
	dev.CmdBindIndexBuffer(cb, indexBuffer.buf, 0, IndexTypeUint16)

	// The half-transparent quad, indexed, cut by its scissor.
	draw := &drawConstants{
		Color: halfRed,
		Rect:  [4]float32{float32(redRect.x), float32(redRect.y), float32(redRect.w), float32(redRect.h)},
	}
	dev.CmdSetScissor(cb, 0, []Rect2D{{
		Offset: Offset2D{X: int32(redScissor.x), Y: int32(redScissor.y)},
		Extent: Extent2D{Width: uint32(redScissor.w), Height: uint32(redScissor.h)},
	}})
	dev.CmdBindDescriptorSets(cb, PipelineBindPointGraphics, pipelineLayout, 0, []DescriptorSet{whiteSet}, nil)
	dev.CmdPushConstants(cb, pipelineLayout, drawStages, 0, uint32(unsafe.Sizeof(*draw)), unsafe.Pointer(draw))
	dev.CmdDrawIndexed(cb, uint32(len(indices)), 1, 0, 0, 0)

	// The textured quad, twice, one below the other.
	draw = &drawConstants{
		Color:        [4]float32{1, 1, 1, 1},
		Rect:         [4]float32{float32(texRect.x), float32(texRect.y), float32(texRect.w), float32(texRect.h)},
		InstanceStep: [2]float32{0, float32(texStep)},
	}
	dev.CmdSetScissor(cb, 0, []Rect2D{whole})
	dev.CmdBindDescriptorSets(cb, PipelineBindPointGraphics, pipelineLayout, 0, []DescriptorSet{textureSet}, nil)
	dev.CmdPushConstants(cb, pipelineLayout, drawStages, 0, uint32(unsafe.Sizeof(*draw)), unsafe.Pointer(draw))
	dev.CmdDraw(cb, uint32(len(indices)), 2, uint32(len(corners)), 0)

	cleared := unorm(attachmentClear)
	dev.CmdClearAttachments(cb,
		[]ClearAttachment{{AspectMask: ImageAspectColor, ColorAttachment: 0, ClearValue: ClearColor(cleared[0], cleared[1], cleared[2], cleared[3])}},
		[]ClearRect{{
			Rect: Rect2D{
				Offset: Offset2D{X: int32(clearedRect.x), Y: int32(clearedRect.y)},
				Extent: Extent2D{Width: uint32(clearedRect.w), Height: uint32(clearedRect.h)},
			},
			LayerCount: 1,
		}})
	dev.CmdEndRenderPass(cb)

	// The second image: cleared, then a blit and a copy of the target.
	fill := unorm(copyClear)
	fillColor := ClearColorFloat(fill[0], fill[1], fill[2], fill[3])
	dev.CmdClearColorImage(cb, second, ImageLayoutTransferDstOptimal, &fillColor, []ImageSubresourceRange{colorRange})
	// The blit and the copy write over the clear: they wait for it.
	dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageTransfer, 0, nil, nil, []ImageMemoryBarrier{
		transition(second, ImageLayoutTransferDstOptimal, ImageLayoutTransferDstOptimal, AccessTransferWrite, AccessTransferWrite),
	})
	dev.CmdBlitImage(cb, target, ImageLayoutTransferSrcOptimal, second, ImageLayoutTransferDstOptimal, []ImageBlit{{
		SrcSubresource: colorLayers,
		SrcOffsets: [2]Offset3D{
			{X: int32(blitSrc.x), Y: int32(blitSrc.y)},
			{X: int32(blitSrc.x + blitSrc.w), Y: int32(blitSrc.y + blitSrc.h), Z: 1},
		},
		DstSubresource: colorLayers,
		DstOffsets: [2]Offset3D{
			{X: int32(blitDst.x), Y: int32(blitDst.y)},
			{X: int32(blitDst.x + blitDst.w), Y: int32(blitDst.y + blitDst.h), Z: 1},
		},
	}}, FilterNearest)
	dev.CmdCopyImage(cb, target, ImageLayoutTransferSrcOptimal, second, ImageLayoutTransferDstOptimal, []ImageCopy{{
		SrcSubresource: colorLayers,
		SrcOffset:      Offset3D{X: int32(imageCopySrc.x), Y: int32(imageCopySrc.y)},
		DstSubresource: colorLayers,
		DstOffset:      Offset3D{X: int32(imageCopyDst.x), Y: int32(imageCopyDst.y)},
		Extent:         Extent3D{Width: uint32(imageCopySrc.w), Height: uint32(imageCopySrc.h), Depth: 1},
	}})
	dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageTransfer, 0, nil, nil, []ImageMemoryBarrier{
		transition(second, ImageLayoutTransferDstOptimal, ImageLayoutTransferSrcOptimal, AccessTransferWrite, AccessTransferRead),
	})

	// Read both images back, and the first buffer through a second one.
	dev.CmdCopyImageToBuffer(cb, target, ImageLayoutTransferSrcOptimal, targetOut.buf, []BufferImageCopy{{
		ImageSubresource: colorLayers,
		ImageExtent:      Extent3D{Width: targetSize, Height: targetSize, Depth: 1},
	}})
	dev.CmdCopyImageToBuffer(cb, second, ImageLayoutTransferSrcOptimal, copyOut.buf, []BufferImageCopy{{
		ImageSubresource: colorLayers,
		ImageExtent:      Extent3D{Width: copySize, Height: copySize, Depth: 1},
	}})
	dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageTransfer, 0, nil, []BufferMemoryBarrier{{
		SType:               StructureTypeBufferMemoryBarrier,
		SrcAccessMask:       AccessTransferWrite,
		DstAccessMask:       AccessTransferRead,
		SrcQueueFamilyIndex: QueueFamilyIgnored,
		DstQueueFamilyIndex: QueueFamilyIgnored,
		Buffer:              targetOut.buf,
		Offset:              0,
		Size:                WholeSize,
	}}, nil)
	dev.CmdCopyBuffer(cb, targetOut.buf, targetOutCopy.buf, []BufferCopy{{Size: uint64(targetBytes)}})
	dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageHost, 0, []MemoryBarrier{{
		SType:         StructureTypeMemoryBarrier,
		SrcAccessMask: AccessTransferWrite,
		DstAccessMask: AccessHostRead,
	}}, nil, nil)
	check(t, "EndCommandBuffer", dev.EndCommandBuffer(cb))

	// Submit and wait.
	fence, err := dev.CreateFence(&FenceCreateInfo{SType: StructureTypeFenceCreateInfo})
	check(t, "CreateFence", err)
	t.Cleanup(func() { dev.DestroyFence(fence) })
	if err := dev.GetFenceStatus(fence); err != NotReady {
		t.Errorf("status of a new fence: %v, want %v", err, NotReady)
	}
	if err := dev.WaitForFences([]Fence{fence}, true, 0); err != Timeout {
		t.Errorf("waiting for a fence nothing signals: %v, want %v", err, Timeout)
	}
	check(t, "QueueSubmit", dev.QueueSubmit(queue, []SubmitInfo{{
		SType:              StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    &cb,
	}}, fence))
	check(t, "WaitForFences", dev.WaitForFences([]Fence{fence}, true, 10e9))
	check(t, "GetFenceStatus", dev.GetFenceStatus(fence))
	check(t, "ResetFences", dev.ResetFences([]Fence{fence}))
	if err := dev.GetFenceStatus(fence); err != NotReady {
		t.Errorf("status of a reset fence: %v, want %v", err, NotReady)
	}
	check(t, "QueueWaitIdle", dev.QueueWaitIdle(queue))

	// Compare.
	r.invalidate(targetOut, targetOutCopy, copyOut)
	comparePixels(t, "the render target", targetOut.bytes, targetSize, wantTarget)
	comparePixels(t, "the render target through a buffer copy", targetOutCopy.bytes, targetSize, wantTarget)
	comparePixels(t, "the blitted and copied image", copyOut.bytes, copySize, wantCopy)

	// What is left: semaphores, and the resets.
	semaphore, err := dev.CreateSemaphore(&SemaphoreCreateInfo{SType: StructureTypeSemaphoreCreateInfo})
	check(t, "CreateSemaphore", err)
	if semaphore == 0 {
		t.Error("CreateSemaphore returned no handle")
	}
	dev.DestroySemaphore(semaphore)
	check(t, "ResetCommandBuffer", dev.ResetCommandBuffer(cb, CommandBufferResetReleaseResources))
	check(t, "ResetCommandPool", dev.ResetCommandPool(pool, CommandPoolResetReleaseResources))
	check(t, "ResetDescriptorPool", dev.ResetDescriptorPool(descriptorPool))
	check(t, "WaitIdle", dev.WaitIdle())
}

// comparePixels compares a size x size RGBA image with the pixels want
// gives, exactly, and reports the first differences.
func comparePixels(t *testing.T, what string, got []byte, size int, want func(x, y int) rgba) {
	t.Helper()
	bad := 0
	for y := range size {
		for x := range size {
			g := rgba(got[4*(y*size+x):])
			if w := want(x, y); g != w {
				if bad < 8 {
					t.Errorf("%s: pixel (%d, %d) is %v, want %v", what, x, y, g, w)
				}
				bad++
			}
		}
	}
	if bad > 0 {
		t.Errorf("%s: %d of %d pixels differ", what, bad, size*size)
	}
}
