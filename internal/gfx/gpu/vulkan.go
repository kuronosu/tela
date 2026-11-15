//go:build telagpu && (linux || windows)

package gpu

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"image"
	"os"
	"unsafe"

	"github.com/kuronosu/tela/internal/vk"
)

// The shaders, compiled by `go generate ./internal/gfx/gpu`.
var (
	//go:embed shaders/quad.vert.spv
	quadVertSPV []byte
	//go:embed shaders/quad.frag.spv
	quadFragSPV []byte
)

// targetFormat holds premultiplied sRGB-encoded values as they are: no
// sRGB format, so that blending happens on the encoded values (ADR-007).
const targetFormat = vk.FormatR8G8B8A8Unorm

// waitTimeout bounds the wait for one submission, in nanoseconds.
const waitTimeout = 30e9

// Open opens the Vulkan device ADR-281 chooses (cfg.Device, else
// TELA_GPU_DEVICE, else the first discrete one). It returns an error, and
// the caller renders with the CPU rasterizer, when Vulkan or a usable
// device is missing.
func Open(cfg Config) (Device, error) {
	icfg := vk.InstanceConfig{AppName: "tela", Validation: cfg.Validate}
	if cfg.Log != nil {
		icfg.Debug = func(m vk.DebugMessage) { cfg.Log(m.String()) }
	}
	inst, err := vk.NewInstance(icfg)
	if err != nil {
		return nil, fmt.Errorf("gpu: %w", err)
	}
	devices, err := inst.Devices()
	if err != nil {
		inst.Destroy()
		return nil, fmt.Errorf("gpu: %w", err)
	}
	override := cfg.Device
	if override == "" {
		override = os.Getenv(vk.DeviceEnv)
	}
	n, err := vk.SelectDevice(devices, override)
	if err != nil {
		inst.Destroy()
		return nil, fmt.Errorf("gpu: %w", err)
	}
	d, err := OpenVulkan(inst, devices[n])
	if err != nil {
		inst.Destroy()
		return nil, err
	}
	d.(*vkDevice).ownInstance = true
	return d, nil
}

// vkDevice is the Vulkan Device. Every frame is recorded into one command
// buffer, submitted and waited for.
//
// perf: a frame waits for the GPU before Draw returns, and its instances
// are copied into one host-visible buffer; frames in flight, with a ring
// of buffers, come with presenting.
type vkDevice struct {
	inst        *vk.Instance
	ownInstance bool
	info        vk.DeviceInfo
	dev         *vk.Device
	queue       vk.Queue
	mem         *vk.PhysicalDeviceMemoryProperties
	maxSize     int

	pool  vk.CommandPool
	cb    vk.CommandBuffer
	fence vk.Fence

	renderPass     vk.RenderPass
	sampler        vk.Sampler
	setLayout      vk.DescriptorSetLayout
	descPool       vk.DescriptorPool
	set            vk.DescriptorSet
	pipelineLayout vk.PipelineLayout
	vert, frag     vk.ShaderModule
	quad           vk.Pipeline

	instances hostBuffer // the instances of the frame (a storage buffer)
	segments  hostBuffer // Frame.RampSegments (a storage buffer)
	staging   hostBuffer // ramp rows on their way to the texture
	readback  hostBuffer // pixels on their way back
	ramps     texture    // Frame.Ramps
	rampRows  int        // rows of the ramp texture
	uploaded  []byte     // the ramp bytes the texture holds
	setStale  bool       // a buffer or the texture changed: write the set

	targets map[*vkTarget]struct{}
	closed  bool
}

// hostBuffer is a buffer in host-visible memory, mapped for its lifetime.
type hostBuffer struct {
	buf   vk.Buffer
	mem   vk.DeviceMemory
	bytes []byte
}

// texture is a sampled image and its view.
type texture struct {
	img  vk.Image
	mem  vk.DeviceMemory
	view vk.ImageView
}

var (
	colorRange  = vk.ImageSubresourceRange{AspectMask: vk.ImageAspectColor, LevelCount: 1, LayerCount: 1}
	colorLayers = vk.ImageSubresourceLayers{AspectMask: vk.ImageAspectColor, LayerCount: 1}
)

// OpenVulkan creates the device of one physical device of inst. The
// instance must outlive it; Close does not destroy the instance.
func OpenVulkan(inst *vk.Instance, info vk.DeviceInfo) (Device, error) {
	d := &vkDevice{inst: inst, info: info, targets: map[*vkTarget]struct{}{}}
	if err := d.init(); err != nil {
		d.Close()
		return nil, fmt.Errorf("gpu: %s: %w", info.Name, err)
	}
	return d, nil
}

func (d *vkDevice) init() error {
	pd := d.info.Device
	const needed = vk.FormatFeatureColorAttachment | vk.FormatFeatureColorAttachmentBlend | vk.FormatFeatureSampledImage | vk.FormatFeatureTransferSrc | vk.FormatFeatureTransferDst
	if got := d.inst.GetPhysicalDeviceFormatProperties(pd, targetFormat).OptimalTilingFeatures; got&needed != needed {
		return fmt.Errorf("R8G8B8A8_UNORM lacks features (has %#x, needs %#x)", got, vk.FormatFeatureFlags(needed))
	}
	d.maxSize = int(d.inst.GetPhysicalDeviceProperties(pd).Limits.MaxImageDimension2D)
	d.mem = d.inst.GetPhysicalDeviceMemoryProperties(pd)

	priority := float32(1)
	dev, err := d.inst.CreateDevice(pd, &vk.DeviceCreateInfo{
		SType:                vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos: &vk.DeviceQueueCreateInfo{
			SType:            vk.StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: d.info.GraphicsQueueFamily,
			QueueCount:       1,
			PQueuePriorities: &priority,
		},
		PEnabledFeatures: &vk.PhysicalDeviceFeatures{},
	})
	if err != nil {
		return fmt.Errorf("creating the device: %w", err)
	}
	d.dev = dev
	d.queue = dev.GetQueue(d.info.GraphicsQueueFamily, 0)

	if d.pool, err = dev.CreateCommandPool(&vk.CommandPoolCreateInfo{
		SType:            vk.StructureTypeCommandPoolCreateInfo,
		Flags:            vk.CommandPoolCreateResetCommandBuffer,
		QueueFamilyIndex: d.info.GraphicsQueueFamily,
	}); err != nil {
		return fmt.Errorf("creating the command pool: %w", err)
	}
	cbs, err := dev.AllocateCommandBuffers(&vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        d.pool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	})
	if err != nil {
		return fmt.Errorf("allocating the command buffer: %w", err)
	}
	d.cb = cbs[0]
	if d.fence, err = dev.CreateFence(&vk.FenceCreateInfo{SType: vk.StructureTypeFenceCreateInfo}); err != nil {
		return fmt.Errorf("creating the fence: %w", err)
	}

	// The render pass of every target: it keeps the pixels (a render of a
	// region leaves the rest) and leaves the image ready to be read back.
	if d.renderPass, err = dev.CreateRenderPass(&vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 1,
		PAttachments: &vk.AttachmentDescription{
			Format:         targetFormat,
			Samples:        vk.SampleCount1,
			LoadOp:         vk.AttachmentLoadOpLoad,
			StoreOp:        vk.AttachmentStoreOpStore,
			StencilLoadOp:  vk.AttachmentLoadOpDontCare,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  vk.ImageLayoutTransferSrcOptimal,
			FinalLayout:    vk.ImageLayoutTransferSrcOptimal,
		},
		SubpassCount: 1,
		PSubpasses: &vk.SubpassDescription{
			PipelineBindPoint:    vk.PipelineBindPointGraphics,
			ColorAttachmentCount: 1,
			PColorAttachments:    &vk.AttachmentReference{Attachment: 0, Layout: vk.ImageLayoutColorAttachmentOptimal},
		},
		DependencyCount: 2,
		PDependencies: unsafe.SliceData([]vk.SubpassDependency{
			{
				SrcSubpass:    vk.SubpassExternal,
				DstSubpass:    0,
				SrcStageMask:  vk.PipelineStageColorAttachmentOutput | vk.PipelineStageTransfer,
				DstStageMask:  vk.PipelineStageColorAttachmentOutput,
				SrcAccessMask: vk.AccessTransferWrite,
				DstAccessMask: vk.AccessColorAttachmentRead | vk.AccessColorAttachmentWrite,
			},
			{
				SrcSubpass:    0,
				DstSubpass:    vk.SubpassExternal,
				SrcStageMask:  vk.PipelineStageColorAttachmentOutput,
				DstStageMask:  vk.PipelineStageTransfer,
				SrcAccessMask: vk.AccessColorAttachmentWrite,
				DstAccessMask: vk.AccessTransferRead,
			},
		}),
	}); err != nil {
		return fmt.Errorf("creating the render pass: %w", err)
	}

	// Ramps are looked up texel by texel (texelFetch): the sampler's
	// filters do not matter.
	if d.sampler, err = dev.CreateSampler(&vk.SamplerCreateInfo{
		SType:        vk.StructureTypeSamplerCreateInfo,
		MagFilter:    vk.FilterNearest,
		MinFilter:    vk.FilterNearest,
		MipmapMode:   vk.SamplerMipmapModeNearest,
		AddressModeU: vk.SamplerAddressModeClampToEdge,
		AddressModeV: vk.SamplerAddressModeClampToEdge,
		AddressModeW: vk.SamplerAddressModeClampToEdge,
		BorderColor:  vk.BorderColorFloatTransparentBlack,
	}); err != nil {
		return fmt.Errorf("creating the sampler: %w", err)
	}

	// Descriptors: the instances, the ramp segments and the ramps (the
	// bindings of shaders/quad.frag).
	if d.setLayout, err = dev.CreateDescriptorSetLayout(&vk.DescriptorSetLayoutCreateInfo{
		SType:        vk.StructureTypeDescriptorSetLayoutCreateInfo,
		BindingCount: 3,
		PBindings: unsafe.SliceData([]vk.DescriptorSetLayoutBinding{
			{Binding: 0, DescriptorType: vk.DescriptorTypeStorageBuffer, DescriptorCount: 1, StageFlags: vk.ShaderStageVertex | vk.ShaderStageFragment},
			{Binding: 1, DescriptorType: vk.DescriptorTypeStorageBuffer, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
			{Binding: 2, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
		}),
	}); err != nil {
		return fmt.Errorf("creating the descriptor set layout: %w", err)
	}
	if d.descPool, err = dev.CreateDescriptorPool(&vk.DescriptorPoolCreateInfo{
		SType:         vk.StructureTypeDescriptorPoolCreateInfo,
		MaxSets:       1,
		PoolSizeCount: 2,
		PPoolSizes: unsafe.SliceData([]vk.DescriptorPoolSize{
			{Type: vk.DescriptorTypeStorageBuffer, DescriptorCount: 2},
			{Type: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1},
		}),
	}); err != nil {
		return fmt.Errorf("creating the descriptor pool: %w", err)
	}
	sets, err := dev.AllocateDescriptorSets(&vk.DescriptorSetAllocateInfo{
		SType:              vk.StructureTypeDescriptorSetAllocateInfo,
		DescriptorPool:     d.descPool,
		DescriptorSetCount: 1,
		PSetLayouts:        &d.setLayout,
	})
	if err != nil {
		return fmt.Errorf("allocating the descriptor set: %w", err)
	}
	d.set = sets[0]

	if d.pipelineLayout, err = dev.CreatePipelineLayout(&vk.PipelineLayoutCreateInfo{
		SType:                  vk.StructureTypePipelineLayoutCreateInfo,
		SetLayoutCount:         1,
		PSetLayouts:            &d.setLayout,
		PushConstantRangeCount: 1,
		PPushConstantRanges:    &vk.PushConstantRange{StageFlags: vk.ShaderStageVertex, Offset: 0, Size: 8},
	}); err != nil {
		return fmt.Errorf("creating the pipeline layout: %w", err)
	}
	if d.vert, err = d.module(quadVertSPV); err != nil {
		return fmt.Errorf("creating the vertex shader: %w", err)
	}
	if d.frag, err = d.module(quadFragSPV); err != nil {
		return fmt.Errorf("creating the fragment shader: %w", err)
	}
	if d.quad, err = d.quadPipeline(); err != nil {
		return fmt.Errorf("creating the quad pipeline: %w", err)
	}

	// The buffers and the ramp texture start small and grow with the
	// frames; the set always names valid ones.
	if err := d.growBuffer(&d.instances, 64*InstanceSize, vk.BufferUsageStorageBuffer); err != nil {
		return err
	}
	if err := d.growBuffer(&d.segments, 64*4*RampSegmentSize, vk.BufferUsageStorageBuffer); err != nil {
		return err
	}
	return d.growRamps(8)
}

func (d *vkDevice) module(code []byte) (vk.ShaderModule, error) {
	words := vk.SPIRV(code)
	return d.dev.CreateShaderModule(&vk.ShaderModuleCreateInfo{
		SType:    vk.StructureTypeShaderModuleCreateInfo,
		CodeSize: uintptr(len(code)),
		PCode:    &words[0],
	})
}

// quadPipeline creates the quad pipeline: four vertices per instance as a
// triangle strip, no vertex buffers, premultiplied source-over.
func (d *vkDevice) quadPipeline() (vk.Pipeline, error) {
	entry := vk.CString("main")
	pipelines, err := d.dev.CreateGraphicsPipelines(0, []vk.GraphicsPipelineCreateInfo{{
		SType:      vk.StructureTypeGraphicsPipelineCreateInfo,
		StageCount: 2,
		PStages: unsafe.SliceData([]vk.PipelineShaderStageCreateInfo{
			{SType: vk.StructureTypePipelineShaderStageCreateInfo, Stage: vk.ShaderStageVertex, Module: d.vert, PName: entry},
			{SType: vk.StructureTypePipelineShaderStageCreateInfo, Stage: vk.ShaderStageFragment, Module: d.frag, PName: entry},
		}),
		PVertexInputState: &vk.PipelineVertexInputStateCreateInfo{
			SType: vk.StructureTypePipelineVertexInputStateCreateInfo,
		},
		PInputAssemblyState: &vk.PipelineInputAssemblyStateCreateInfo{
			SType:    vk.StructureTypePipelineInputAssemblyStateCreateInfo,
			Topology: vk.PrimitiveTopologyTriangleStrip,
		},
		PViewportState: &vk.PipelineViewportStateCreateInfo{
			SType:         vk.StructureTypePipelineViewportStateCreateInfo,
			ViewportCount: 1, // set by commands
			ScissorCount:  1,
		},
		PRasterizationState: &vk.PipelineRasterizationStateCreateInfo{
			SType:       vk.StructureTypePipelineRasterizationStateCreateInfo,
			PolygonMode: vk.PolygonModeFill,
			CullMode:    vk.CullModeNone,
			FrontFace:   vk.FrontFaceCounterClockwise,
			LineWidth:   1,
		},
		PMultisampleState: &vk.PipelineMultisampleStateCreateInfo{
			SType:                vk.StructureTypePipelineMultisampleStateCreateInfo,
			RasterizationSamples: vk.SampleCount1,
		},
		PColorBlendState: &vk.PipelineColorBlendStateCreateInfo{
			SType:           vk.StructureTypePipelineColorBlendStateCreateInfo,
			AttachmentCount: 1,
			PAttachments: &vk.PipelineColorBlendAttachmentState{
				BlendEnable:         vk.True,
				SrcColorBlendFactor: vk.BlendFactorOne,
				DstColorBlendFactor: vk.BlendFactorOneMinusSrcAlpha,
				ColorBlendOp:        vk.BlendOpAdd,
				SrcAlphaBlendFactor: vk.BlendFactorOne,
				DstAlphaBlendFactor: vk.BlendFactorOneMinusSrcAlpha,
				AlphaBlendOp:        vk.BlendOpAdd,
				ColorWriteMask:      vk.ColorComponentR | vk.ColorComponentG | vk.ColorComponentB | vk.ColorComponentA,
			},
		},
		PDynamicState: &vk.PipelineDynamicStateCreateInfo{
			SType:             vk.StructureTypePipelineDynamicStateCreateInfo,
			DynamicStateCount: 2,
			PDynamicStates:    unsafe.SliceData([]vk.DynamicState{vk.DynamicStateViewport, vk.DynamicStateScissor}),
		},
		Layout:            d.pipelineLayout,
		RenderPass:        d.renderPass,
		Subpass:           0,
		BasePipelineIndex: -1,
	}})
	if err != nil {
		return 0, err
	}
	return pipelines[0], nil
}

func (d *vkDevice) Name() string       { return d.info.Name }
func (d *vkDevice) MaxTargetSize() int { return d.maxSize }

// Close destroys the device's objects, the targets it still has, and the
// instance when Open created it. It may be called more than once.
func (d *vkDevice) Close() {
	if d.closed {
		return
	}
	d.closed = true
	dev := d.dev
	if dev != nil {
		dev.WaitIdle() // nothing is in flight; an error leaves nothing to do
		for t := range d.targets {
			t.destroy()
		}
		d.targets = nil
		d.destroyTexture(&d.ramps)
		for _, b := range []*hostBuffer{&d.instances, &d.segments, &d.staging, &d.readback} {
			d.destroyBuffer(b)
		}
		if d.quad != 0 {
			dev.DestroyPipeline(d.quad)
		}
		if d.vert != 0 {
			dev.DestroyShaderModule(d.vert)
		}
		if d.frag != 0 {
			dev.DestroyShaderModule(d.frag)
		}
		if d.pipelineLayout != 0 {
			dev.DestroyPipelineLayout(d.pipelineLayout)
		}
		if d.descPool != 0 {
			dev.DestroyDescriptorPool(d.descPool) // frees the set
		}
		if d.setLayout != 0 {
			dev.DestroyDescriptorSetLayout(d.setLayout)
		}
		if d.sampler != 0 {
			dev.DestroySampler(d.sampler)
		}
		if d.renderPass != 0 {
			dev.DestroyRenderPass(d.renderPass)
		}
		if d.fence != 0 {
			dev.DestroyFence(d.fence)
		}
		if d.pool != 0 {
			dev.DestroyCommandPool(d.pool) // frees the command buffer
		}
		dev.Destroy()
		d.dev = nil
	}
	if d.ownInstance {
		d.inst.Destroy()
	}
}

// ---------------------------------------------------------------------------
// Resources
// ---------------------------------------------------------------------------

// growBuffer makes b a mapped buffer of at least size bytes, replacing a
// smaller one (whose content is lost).
func (d *vkDevice) growBuffer(b *hostBuffer, size int, usage vk.BufferUsageFlags) error {
	if len(b.bytes) >= size {
		return nil
	}
	size = max(size, 2*len(b.bytes)) // amortized growth
	d.destroyBuffer(b)
	dev := d.dev
	buf, err := dev.CreateBuffer(&vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        uint64(size),
		Usage:       usage,
		SharingMode: vk.SharingModeExclusive,
	})
	if err != nil {
		return fmt.Errorf("creating a buffer of %d bytes: %w", size, err)
	}
	b.buf = buf
	req := dev.GetBufferMemoryRequirements(buf)
	typ, ok := d.mem.FindMemoryType(req.MemoryTypeBits, vk.MemoryPropertyHostVisible|vk.MemoryPropertyHostCoherent)
	if !ok {
		return errors.New("no host-visible coherent memory type for a buffer")
	}
	if b.mem, err = dev.AllocateMemory(&vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  req.Size,
		MemoryTypeIndex: typ,
	}); err != nil {
		return fmt.Errorf("allocating %d bytes of buffer memory: %w", req.Size, err)
	}
	if err := dev.BindBufferMemory(buf, b.mem, 0); err != nil {
		return fmt.Errorf("binding buffer memory: %w", err)
	}
	p, err := dev.MapMemory(b.mem, 0, vk.WholeSize, 0)
	if err != nil {
		return fmt.Errorf("mapping buffer memory: %w", err)
	}
	b.bytes = unsafe.Slice((*byte)(p), size)
	d.setStale = true
	return nil
}

func (d *vkDevice) destroyBuffer(b *hostBuffer) {
	if b.mem != 0 && b.bytes != nil {
		d.dev.UnmapMemory(b.mem)
	}
	if b.buf != 0 {
		d.dev.DestroyBuffer(b.buf)
	}
	if b.mem != 0 {
		d.dev.FreeMemory(b.mem)
	}
	*b = hostBuffer{}
}

// newTexture creates a 2D R8G8B8A8_UNORM image in device memory, with a
// view when it is sampled or drawn into.
func (d *vkDevice) newTexture(width, height int, usage vk.ImageUsageFlags) (t texture, err error) {
	dev := d.dev
	defer func() {
		if err != nil {
			d.destroyTexture(&t)
		}
	}()
	if t.img, err = dev.CreateImage(&vk.ImageCreateInfo{
		SType:         vk.StructureTypeImageCreateInfo,
		ImageType:     vk.ImageType2D,
		Format:        targetFormat,
		Extent:        vk.Extent3D{Width: uint32(width), Height: uint32(height), Depth: 1},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       vk.SampleCount1,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         usage,
		SharingMode:   vk.SharingModeExclusive,
		InitialLayout: vk.ImageLayoutUndefined,
	}); err != nil {
		return t, fmt.Errorf("creating a %dx%d image: %w", width, height, err)
	}
	req := dev.GetImageMemoryRequirements(t.img)
	typ, ok := d.mem.FindMemoryType(req.MemoryTypeBits, vk.MemoryPropertyDeviceLocal)
	if !ok {
		return t, errors.New("no device-local memory type for an image")
	}
	if t.mem, err = dev.AllocateMemory(&vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  req.Size,
		MemoryTypeIndex: typ,
	}); err != nil {
		return t, fmt.Errorf("allocating %d bytes of image memory: %w", req.Size, err)
	}
	if err = dev.BindImageMemory(t.img, t.mem, 0); err != nil {
		return t, fmt.Errorf("binding image memory: %w", err)
	}
	if t.view, err = dev.CreateImageView(&vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    t.img,
		ViewType: vk.ImageViewType2D,
		Format:   targetFormat,
		Components: vk.ComponentMapping{
			R: vk.ComponentSwizzleIdentity, G: vk.ComponentSwizzleIdentity, B: vk.ComponentSwizzleIdentity, A: vk.ComponentSwizzleIdentity,
		},
		SubresourceRange: colorRange,
	}); err != nil {
		return t, fmt.Errorf("creating an image view: %w", err)
	}
	return t, nil
}

func (d *vkDevice) destroyTexture(t *texture) {
	if t.view != 0 {
		d.dev.DestroyImageView(t.view)
	}
	if t.img != 0 {
		d.dev.DestroyImage(t.img)
	}
	if t.mem != 0 {
		d.dev.FreeMemory(t.mem)
	}
	*t = texture{}
}

// transition returns the barrier that moves the whole of an image from one
// layout to another.
func transition(img vk.Image, from, to vk.ImageLayout, srcAccess, dstAccess vk.AccessFlags) vk.ImageMemoryBarrier {
	return vk.ImageMemoryBarrier{
		SType:               vk.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       srcAccess,
		DstAccessMask:       dstAccess,
		OldLayout:           from,
		NewLayout:           to,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image:               img,
		SubresourceRange:    colorRange,
	}
}

// growRamps makes the ramp texture at least rows high. A new texture is
// cleared, so that the set names an image in the layout the shader reads
// even before a frame has ramps.
func (d *vkDevice) growRamps(rows int) error {
	if rows <= d.rampRows {
		return nil
	}
	if rows > d.maxSize {
		return fmt.Errorf("a frame with %d gradient ramp rows: the device's textures are at most %d high", rows, d.maxSize)
	}
	n := max(d.rampRows, 8)
	for n < rows {
		n *= 2
	}
	n = min(n, d.maxSize)
	t, err := d.newTexture(RampWidth, n, vk.ImageUsageSampled|vk.ImageUsageTransferDst)
	if err != nil {
		return err
	}
	err = d.run(func(cb vk.CommandBuffer) {
		d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTopOfPipe, vk.PipelineStageTransfer, 0, nil, nil, []vk.ImageMemoryBarrier{
			transition(t.img, vk.ImageLayoutUndefined, vk.ImageLayoutTransferDstOptimal, 0, vk.AccessTransferWrite),
		})
		zero := vk.ClearColorFloat(0, 0, 0, 0)
		d.dev.CmdClearColorImage(cb, t.img, vk.ImageLayoutTransferDstOptimal, &zero, []vk.ImageSubresourceRange{colorRange})
		d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageFragmentShader, 0, nil, nil, []vk.ImageMemoryBarrier{
			transition(t.img, vk.ImageLayoutTransferDstOptimal, vk.ImageLayoutShaderReadOnlyOptimal, vk.AccessTransferWrite, vk.AccessShaderRead),
		})
	})
	if err != nil {
		d.destroyTexture(&t)
		return err
	}
	d.destroyTexture(&d.ramps)
	d.ramps, d.rampRows, d.uploaded = t, n, d.uploaded[:0]
	d.setStale = true
	return nil
}

// writeSet points the descriptor set at the current buffers and texture.
// Nothing is in flight when it is called.
func (d *vkDevice) writeSet() {
	if !d.setStale {
		return
	}
	d.setStale = false
	d.dev.UpdateDescriptorSets([]vk.WriteDescriptorSet{
		{
			SType: vk.StructureTypeWriteDescriptorSet, DstSet: d.set, DstBinding: 0, DescriptorCount: 1,
			DescriptorType: vk.DescriptorTypeStorageBuffer,
			PBufferInfo:    &vk.DescriptorBufferInfo{Buffer: d.instances.buf, Range: vk.WholeSize},
		},
		{
			SType: vk.StructureTypeWriteDescriptorSet, DstSet: d.set, DstBinding: 1, DescriptorCount: 1,
			DescriptorType: vk.DescriptorTypeStorageBuffer,
			PBufferInfo:    &vk.DescriptorBufferInfo{Buffer: d.segments.buf, Range: vk.WholeSize},
		},
		{
			SType: vk.StructureTypeWriteDescriptorSet, DstSet: d.set, DstBinding: 2, DescriptorCount: 1,
			DescriptorType: vk.DescriptorTypeCombinedImageSampler,
			PImageInfo:     &vk.DescriptorImageInfo{Sampler: d.sampler, ImageView: d.ramps.view, ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal},
		},
	}, nil)
}

// run records commands into the device's command buffer, submits them and
// waits until they are done.
func (d *vkDevice) run(record func(cb vk.CommandBuffer)) error {
	dev := d.dev
	if err := dev.BeginCommandBuffer(d.cb, &vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageOneTimeSubmit,
	}); err != nil {
		return fmt.Errorf("gpu: beginning the command buffer: %w", err)
	}
	record(d.cb)
	if err := dev.EndCommandBuffer(d.cb); err != nil {
		return fmt.Errorf("gpu: ending the command buffer: %w", err)
	}
	if err := dev.QueueSubmit(d.queue, []vk.SubmitInfo{{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    &d.cb,
	}}, d.fence); err != nil {
		return fmt.Errorf("gpu: submitting: %w", err)
	}
	if err := dev.WaitForFences([]vk.Fence{d.fence}, true, waitTimeout); err != nil {
		return fmt.Errorf("gpu: waiting for the frame: %w", err)
	}
	if err := dev.ResetFences([]vk.Fence{d.fence}); err != nil {
		return fmt.Errorf("gpu: resetting the fence: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Targets
// ---------------------------------------------------------------------------

type vkTarget struct {
	d           *vkDevice
	size        image.Point
	tex         texture
	framebuffer vk.Framebuffer
}

func (d *vkDevice) NewTarget(width, height int) (Target, error) {
	if d.closed {
		return nil, errors.New("gpu: the device is closed")
	}
	if width < 1 || height < 1 || width > d.maxSize || height > d.maxSize {
		return nil, fmt.Errorf("gpu: a target of %dx%d: the size must be between 1 and %d", width, height, d.maxSize)
	}
	t := &vkTarget{d: d, size: image.Pt(width, height)}
	var err error
	if t.tex, err = d.newTexture(width, height, vk.ImageUsageColorAttachment|vk.ImageUsageTransferSrc|vk.ImageUsageTransferDst); err != nil {
		return nil, fmt.Errorf("gpu: %w", err)
	}
	if t.framebuffer, err = d.dev.CreateFramebuffer(&vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass:      d.renderPass,
		AttachmentCount: 1,
		PAttachments:    &t.tex.view,
		Width:           uint32(width),
		Height:          uint32(height),
		Layers:          1,
	}); err != nil {
		t.destroy()
		return nil, fmt.Errorf("gpu: creating the framebuffer: %w", err)
	}
	// Transparent, and in the layout the render pass starts from.
	err = d.run(func(cb vk.CommandBuffer) {
		d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTopOfPipe, vk.PipelineStageTransfer, 0, nil, nil, []vk.ImageMemoryBarrier{
			transition(t.tex.img, vk.ImageLayoutUndefined, vk.ImageLayoutTransferDstOptimal, 0, vk.AccessTransferWrite),
		})
		zero := vk.ClearColorFloat(0, 0, 0, 0)
		d.dev.CmdClearColorImage(cb, t.tex.img, vk.ImageLayoutTransferDstOptimal, &zero, []vk.ImageSubresourceRange{colorRange})
		d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageTransfer, 0, nil, nil, []vk.ImageMemoryBarrier{
			transition(t.tex.img, vk.ImageLayoutTransferDstOptimal, vk.ImageLayoutTransferSrcOptimal, vk.AccessTransferWrite, vk.AccessTransferRead),
		})
	})
	if err != nil {
		t.destroy()
		return nil, err
	}
	d.targets[t] = struct{}{}
	return t, nil
}

func (t *vkTarget) Size() image.Point { return t.size }

// Close destroys the target. It may be called more than once, and after
// the device was closed.
func (t *vkTarget) Close() {
	if t.d == nil || t.d.closed {
		return
	}
	delete(t.d.targets, t)
	t.destroy()
}

func (t *vkTarget) destroy() {
	d := t.d
	if d == nil {
		return
	}
	if t.framebuffer != 0 {
		d.dev.DestroyFramebuffer(t.framebuffer)
		t.framebuffer = 0
	}
	d.destroyTexture(&t.tex)
	t.d = nil
}

func (t *vkTarget) Draw(f *Frame) error {
	d := t.d
	if d == nil || d.closed {
		return errors.New("gpu: the target is closed")
	}
	if f.Size != t.size {
		return fmt.Errorf("gpu: a frame encoded for %v drawn into a target of %v", f.Size, t.size)
	}
	if len(f.Passes) == 0 {
		return nil
	}
	if len(f.Passes) != 1 || f.Passes[0].Target != 0 {
		return errors.New("gpu: the Vulkan device draws one pass, into the destination")
	}
	pass := &f.Passes[0]
	dev := d.dev

	// The frame's data.
	if err := d.growBuffer(&d.instances, len(f.Instances), vk.BufferUsageStorageBuffer); err != nil {
		return fmt.Errorf("gpu: %w", err)
	}
	copy(d.instances.bytes, f.Instances)
	if err := d.growBuffer(&d.segments, 4*len(f.RampSegments), vk.BufferUsageStorageBuffer); err != nil {
		return fmt.Errorf("gpu: %w", err)
	}
	if n := len(f.RampSegments); n > 0 {
		copy(d.segments.bytes, unsafe.Slice((*byte)(unsafe.Pointer(&f.RampSegments[0])), 4*n))
	}
	rampRows := len(f.Ramps) / (4 * RampWidth)
	// perf: the ramps are uploaded whole whenever they differ from the
	// previous frame's.
	upload := rampRows > 0 && !bytes.Equal(f.Ramps, d.uploaded)
	if upload {
		if err := d.growRamps(rampRows); err != nil {
			return fmt.Errorf("gpu: %w", err)
		}
		if err := d.growBuffer(&d.staging, len(f.Ramps), vk.BufferUsageTransferSrc); err != nil {
			return fmt.Errorf("gpu: %w", err)
		}
		copy(d.staging.bytes, f.Ramps)
	}
	d.writeSet()

	whole := vk.Rect2D{Extent: vk.Extent2D{Width: uint32(t.size.X), Height: uint32(t.size.Y)}}
	err := d.run(func(cb vk.CommandBuffer) {
		if upload {
			dev.CmdPipelineBarrier(cb, vk.PipelineStageFragmentShader, vk.PipelineStageTransfer, 0, nil, nil, []vk.ImageMemoryBarrier{
				transition(d.ramps.img, vk.ImageLayoutShaderReadOnlyOptimal, vk.ImageLayoutTransferDstOptimal, vk.AccessShaderRead, vk.AccessTransferWrite),
			})
			dev.CmdCopyBufferToImage(cb, d.staging.buf, d.ramps.img, vk.ImageLayoutTransferDstOptimal, []vk.BufferImageCopy{{
				ImageSubresource: colorLayers,
				ImageExtent:      vk.Extent3D{Width: RampWidth, Height: uint32(rampRows), Depth: 1},
			}})
			dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageFragmentShader, 0, nil, nil, []vk.ImageMemoryBarrier{
				transition(d.ramps.img, vk.ImageLayoutTransferDstOptimal, vk.ImageLayoutShaderReadOnlyOptimal, vk.AccessTransferWrite, vk.AccessShaderRead),
			})
		}
		dev.CmdBeginRenderPass(cb, &vk.RenderPassBeginInfo{
			SType:       vk.StructureTypeRenderPassBeginInfo,
			RenderPass:  d.renderPass,
			Framebuffer: t.framebuffer,
			RenderArea:  whole,
		}, vk.SubpassContentsInline)
		if c := pass.Clear; c != nil {
			if r := c.Rect.Intersect(image.Rectangle{Max: t.size}); !r.Empty() {
				dev.CmdClearAttachments(cb,
					[]vk.ClearAttachment{{AspectMask: vk.ImageAspectColor, ColorAttachment: 0, ClearValue: vk.ClearColor(
						float32(c.Color[0])/255, float32(c.Color[1])/255, float32(c.Color[2])/255, float32(c.Color[3])/255)}},
					[]vk.ClearRect{{Rect: rect2D(r), LayerCount: 1}})
			}
		}
		if len(pass.Batches) > 0 {
			dev.CmdBindPipeline(cb, vk.PipelineBindPointGraphics, d.quad)
			dev.CmdSetViewport(cb, 0, []vk.Viewport{{Width: float32(t.size.X), Height: float32(t.size.Y), MinDepth: 0, MaxDepth: 1}})
			dev.CmdBindDescriptorSets(cb, vk.PipelineBindPointGraphics, d.pipelineLayout, 0, []vk.DescriptorSet{d.set}, nil)
			scale := &[2]float32{2 / float32(t.size.X), 2 / float32(t.size.Y)}
			dev.CmdPushConstants(cb, d.pipelineLayout, vk.ShaderStageVertex, 0, 8, unsafe.Pointer(scale))
			for i := range pass.Batches {
				b := &pass.Batches[i]
				// perf: one scissor command per batch, each a small heap
				// allocation (see internal/vk's doc).
				dev.CmdSetScissor(cb, 0, []vk.Rect2D{rect2D(b.Scissor)})
				dev.CmdDraw(cb, 4, uint32(b.Count), 0, uint32(b.First))
			}
		}
		dev.CmdEndRenderPass(cb)
	})
	if err != nil {
		return err
	}
	if upload {
		d.uploaded = append(d.uploaded[:0], f.Ramps...)
	}
	return nil
}

func rect2D(r image.Rectangle) vk.Rect2D {
	return vk.Rect2D{
		Offset: vk.Offset2D{X: int32(r.Min.X), Y: int32(r.Min.Y)},
		Extent: vk.Extent2D{Width: uint32(r.Dx()), Height: uint32(r.Dy())},
	}
}

func (t *vkTarget) ReadPixels(dst *image.RGBA, r image.Rectangle) error {
	d := t.d
	if d == nil || d.closed {
		return errors.New("gpu: the target is closed")
	}
	r = r.Intersect(image.Rectangle{Max: t.size}).Intersect(dst.Rect)
	if r.Empty() {
		return nil
	}
	n := 4 * r.Dx() * r.Dy()
	if err := d.growBuffer(&d.readback, n, vk.BufferUsageTransferDst); err != nil {
		return fmt.Errorf("gpu: %w", err)
	}
	dev := d.dev
	err := d.run(func(cb vk.CommandBuffer) {
		dev.CmdCopyImageToBuffer(cb, t.tex.img, vk.ImageLayoutTransferSrcOptimal, d.readback.buf, []vk.BufferImageCopy{{
			ImageSubresource: colorLayers,
			ImageOffset:      vk.Offset3D{X: int32(r.Min.X), Y: int32(r.Min.Y)},
			ImageExtent:      vk.Extent3D{Width: uint32(r.Dx()), Height: uint32(r.Dy()), Depth: 1},
		}})
		dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageHost, 0, []vk.MemoryBarrier{{
			SType:         vk.StructureTypeMemoryBarrier,
			SrcAccessMask: vk.AccessTransferWrite,
			DstAccessMask: vk.AccessHostRead,
		}}, nil, nil)
	})
	if err != nil {
		return err
	}
	row := 4 * r.Dx()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		o := dst.PixOffset(r.Min.X, y)
		copy(dst.Pix[o:o+row], d.readback.bytes[(y-r.Min.Y)*row:])
	}
	return nil
}
