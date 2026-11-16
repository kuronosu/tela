//go:build telagpu && (linux || windows)

package gpu

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"image"
	"os"
	"strings"
	"unsafe"

	"github.com/kuronosu/tela/internal/vk"
)

// The shaders, compiled by `go generate ./internal/gfx/gpu`.
var (
	//go:embed shaders/quad.vert.spv
	quadVertSPV []byte
	//go:embed shaders/quad.frag.spv
	quadFragSPV []byte
	//go:embed shaders/filter.frag.spv
	filterFragSPV []byte
	//go:embed shaders/plain.frag.spv
	plainFragSPV []byte
)

// targetFormat holds premultiplied sRGB-encoded values as they are: no
// sRGB format, so that blending happens on the encoded values (ADR-007).
// It is also the format of images, sprites and ramps.
const targetFormat = vk.FormatR8G8B8A8Unorm

// floatFormat is the format of FormatRGBA16F targets: what a blur keeps
// between its passes.
const floatFormat = vk.FormatR16G16B16A16Sfloat

// coverageFormat is the format of the coverage atlas.
const coverageFormat = vk.FormatR8Unorm

// sourceSetsPerPool is the number of descriptor sets one pool serves to
// the batches of a frame that read targets.
const sourceSetsPerPool = 128

// imageSetsPerPool is the number of image textures one descriptor pool
// serves.
const imageSetsPerPool = 64

// waitTimeout bounds the wait for one submission, in nanoseconds.
const waitTimeout = 30e9

// Open opens the Vulkan device ADR-281 chooses: cfg.Device, else
// TELA_GPU_DEVICE, else the device of the display server (cfg.DRM), else
// the first discrete one. It returns an error, and the caller renders with
// the CPU rasterizer, when Vulkan or a usable device is missing, and
// ErrNotDiscrete when cfg.DiscreteOnly refuses the device chosen.
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
	n, err := selectDevice(devices, cfg, os.Getenv(vk.DeviceEnv))
	if err != nil {
		inst.Destroy()
		return nil, err
	}
	d, err := OpenVulkan(inst, devices[n])
	if err != nil {
		inst.Destroy()
		return nil, err
	}
	d.(*vkDevice).ownInstance = true
	return d, nil
}

// selectDevice returns the index of the device Open opens among devices:
// the one cfg.Device names, else the one env (TELA_GPU_DEVICE) names, else
// the one that has the DRM node of cfg, else vk.SelectDevice's (the first
// discrete one, else the first). With cfg.DiscreteOnly a device of
// another type is ErrNotDiscrete.
func selectDevice(devices []vk.DeviceInfo, cfg Config, env string) (int, error) {
	override := cfg.Device
	if override == "" {
		override = env
	}
	n := -1
	if strings.TrimSpace(override) == "" && cfg.HasDRM {
		major, minor := int64(cfg.DRM.Major), int64(cfg.DRM.Minor)
		for i := range devices {
			if drm := devices[i].DRM; drm.HasRender && drm.RenderMajor == major && drm.RenderMinor == minor ||
				drm.HasPrimary && drm.PrimaryMajor == major && drm.PrimaryMinor == minor {
				n = i
				break
			}
		}
	}
	if n < 0 {
		var err error
		if n, err = vk.SelectDevice(devices, override); err != nil {
			return 0, fmt.Errorf("gpu: %w", err)
		}
	}
	if cfg.DiscreteOnly && devices[n].Type != vk.PhysicalDeviceTypeDiscreteGPU {
		return 0, fmt.Errorf("%w: %s", ErrNotDiscrete, devices[n])
	}
	return n, nil
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

	// The render passes of the two target formats, and the pipelines:
	// the quad's with and without the device's blending, and the filter's
	// for each format.
	renderPass     [2]vk.RenderPass
	sampler        vk.Sampler
	setLayout      vk.DescriptorSetLayout // set 0: the frame's data and the atlases
	imageLayout    vk.DescriptorSetLayout // set 1: the textures of a batch
	descPool       vk.DescriptorPool
	set            vk.DescriptorSet
	pipelineLayout vk.PipelineLayout
	vert, frag     vk.ShaderModule
	filterFrag     vk.ShaderModule
	plainFrag      vk.ShaderModule
	quad, blend    vk.Pipeline
	plain, opaque  vk.Pipeline
	filter         [2]vk.Pipeline

	instances hostBuffer // the instances of the frame (a storage buffer)
	segments  hostBuffer // Frame.RampSegments (a storage buffer)
	taps      hostBuffer // Frame.Taps (a storage buffer)
	staging   hostBuffer // pixels on their way to the textures
	readback  hostBuffer // pixels on their way back
	// scratch holds the pixels a Shift moves, between its two copies: a
	// copy of an image onto itself may not overlap. nil until a Shift.
	scratch  *surface
	ramps    texture // Frame.Ramps
	rampRows int     // rows of the ramp texture
	uploaded []byte  // the ramp bytes the texture holds
	setStale bool    // a buffer or a texture changed: write the set

	// The resources of the frames' Encoder (see Frame.Res): the atlases,
	// the image textures by ID, and whose they are.
	atlases    [NumAtlases]vkAtlas
	images     map[uint64]*vkImage
	noImage    *vkImage // bound while a batch reads no image
	imagePools []*imagePool
	resID      uint64
	resSerial  uint64
	// The offscreen targets of the frames' Encoder (Frame.Targets), by ID.
	offscreen map[int]*surface
	gen       uint64 // of the frame whose targets are being listed
	// The descriptor sets of the batches of the frame being drawn that
	// read targets: they live for the frame.
	sourcePools []vk.DescriptorPool
	sourceSets  map[[3]vk.ImageView]vk.DescriptorSet
	sourceUsed  int
	plan        []plannedBatch
	undo        []layoutChange

	targets map[*vkTarget]struct{}
	// exports are the images exported as dmabufs (vulkan_export_linux.go);
	// nil when the device was created without the extensions they need.
	// foreign: the device has VK_EXT_queue_family_foreign.
	exports map[*vkExport]struct{}
	foreign bool
	// exportable are the pairs of format and modifier the device exports,
	// asked once; exportMods the modifiers by Vulkan format.
	exportable []DmabufFormat
	exportMods map[vk.Format][]vk.DmabufModifier
	exportAsk  bool
	closed     bool
	// prof, when not nil, measures the frames instead of only drawing them
	// (vulkan_profile.go).
	prof *profiler
	// lost: a call returned VK_ERROR_DEVICE_LOST. Every call fails from
	// then on (ErrDeviceLost).
	lost bool
}

// check notes an error of a call into the device: a lost device stays
// lost, and its errors are ErrDeviceLost.
func (d *vkDevice) check(err error) error {
	if err != nil && !errors.Is(err, ErrDeviceLost) && errors.Is(err, vk.ErrorDeviceLost) {
		d.lost = true
		return fmt.Errorf("%w: %w", ErrDeviceLost, err)
	}
	return err
}

// usable returns the error of a call into a device that is closed or
// lost, or nil.
func (d *vkDevice) usable() error {
	switch {
	case d == nil || d.closed:
		return errors.New("gpu: the device is closed")
	case d.lost:
		return ErrDeviceLost
	}
	return nil
}

// surface is a color image the device draws into and its shaders read: a
// target's, or an offscreen one's.
type surface struct {
	tex         texture
	framebuffer vk.Framebuffer
	w, h        int
	format      TargetFormat
	// layout is the layout the commands recorded so far leave the image
	// in.
	layout vk.ImageLayout
	gen    uint64
}

// layoutChange is a change of a surface's layout by the commands being
// recorded, undone if they are not executed.
type layoutChange struct {
	s    *surface
	from vk.ImageLayout
}

// plannedBatch is what a batch is drawn with.
type plannedBatch struct {
	pipeline vk.Pipeline
	set      vk.DescriptorSet
	scissor  vk.Rect2D
	skip     bool
}

// vkAtlas is the array texture of an atlas: a 1x1 placeholder while the
// frames use no page.
type vkAtlas struct {
	tex      texture
	pageSize int
	pages    int
}

// vkImage is the texture of an image and the descriptor set that binds it.
type vkImage struct {
	tex  texture
	set  vk.DescriptorSet
	pool *imagePool
}

// imagePool allocates the descriptor sets of imageSetsPerPool images.
type imagePool struct {
	pool vk.DescriptorPool
	used int
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
	// colorRange is every layer of a color image.
	colorRange  = vk.ImageSubresourceRange{AspectMask: vk.ImageAspectColor, LevelCount: 1, LayerCount: vk.RemainingArrayLayers}
	colorLayers = vk.ImageSubresourceLayers{AspectMask: vk.ImageAspectColor, LayerCount: 1}
)

// OpenVulkan creates the device of one physical device of inst. The
// instance must outlive it; Close does not destroy the instance.
func OpenVulkan(inst *vk.Instance, info vk.DeviceInfo) (Device, error) {
	d := &vkDevice{inst: inst, info: info, targets: map[*vkTarget]struct{}{}, images: map[uint64]*vkImage{},
		offscreen: map[int]*surface{}, sourceSets: map[[3]vk.ImageView]vk.DescriptorSet{}}
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
	if got := d.inst.GetPhysicalDeviceFormatProperties(pd, floatFormat).OptimalTilingFeatures; got&(vk.FormatFeatureColorAttachment|vk.FormatFeatureSampledImage) != vk.FormatFeatureColorAttachment|vk.FormatFeatureSampledImage {
		return fmt.Errorf("R16G16B16A16_SFLOAT lacks features (has %#x)", got)
	}
	if got := d.inst.GetPhysicalDeviceFormatProperties(pd, coverageFormat).OptimalTilingFeatures; got&(vk.FormatFeatureSampledImage|vk.FormatFeatureTransferDst|vk.FormatFeatureTransferSrc) != vk.FormatFeatureSampledImage|vk.FormatFeatureTransferDst|vk.FormatFeatureTransferSrc {
		return fmt.Errorf("R8_UNORM lacks features (has %#x)", got)
	}
	d.maxSize = int(d.inst.GetPhysicalDeviceProperties(pd).Limits.MaxImageDimension2D)
	d.mem = d.inst.GetPhysicalDeviceMemoryProperties(pd)

	priority := float32(1)
	create := &vk.DeviceCreateInfo{
		SType:                vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos: &vk.DeviceQueueCreateInfo{
			SType:            vk.StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: d.info.GraphicsQueueFamily,
			QueueCount:       1,
			PQueuePriorities: &priority,
		},
		PEnabledFeatures: &vk.PhysicalDeviceFeatures{},
	}
	// The extensions of the dmabuf export, on the devices that have them
	// (Linux): what presenting without a copy needs (export.go).
	if names, ok := vk.DmabufExtensions(&d.info); ok && exportSupported {
		create.EnabledExtensionCount, create.PpEnabledExtensionNames = vk.CStringArray(names)
		d.exports = map[*vkExport]struct{}{}
		d.foreign = d.info.HasExtension(vk.ExtQueueFamilyForeignExtensionName)
	}
	dev, err := d.inst.CreateDevice(pd, create)
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

	// The render passes: they keep the pixels (a render of a region
	// leaves the rest, and a pass goes on where another stopped). The
	// layouts are changed by barriers around them (use).
	for i, format := range [2]vk.Format{FormatRGBA8: targetFormat, FormatRGBA16F: floatFormat} {
		if d.renderPass[i], err = dev.CreateRenderPass(&vk.RenderPassCreateInfo{
			SType:           vk.StructureTypeRenderPassCreateInfo,
			AttachmentCount: 1,
			PAttachments: &vk.AttachmentDescription{
				Format:         format,
				Samples:        vk.SampleCount1,
				LoadOp:         vk.AttachmentLoadOpLoad,
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
		}); err != nil {
			return fmt.Errorf("creating the render pass: %w", err)
		}
	}

	// Every texture is read texel by texel (texelFetch): the sampler's
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

	// Descriptors (the bindings of shaders/quad.frag and filter.frag). Set
	// 0: the instances, the ramp segments, the ramps, the two atlases and
	// the kernels. Set 1: the textures of a batch (an image or a layer,
	// its mask, the copy of its backdrop; the source of a filter and a
	// drop shadow's content).
	if d.setLayout, err = dev.CreateDescriptorSetLayout(&vk.DescriptorSetLayoutCreateInfo{
		SType:        vk.StructureTypeDescriptorSetLayoutCreateInfo,
		BindingCount: 6,
		PBindings: unsafe.SliceData([]vk.DescriptorSetLayoutBinding{
			{Binding: 0, DescriptorType: vk.DescriptorTypeStorageBuffer, DescriptorCount: 1, StageFlags: vk.ShaderStageVertex | vk.ShaderStageFragment},
			{Binding: 1, DescriptorType: vk.DescriptorTypeStorageBuffer, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
			{Binding: 2, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
			{Binding: 3, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
			{Binding: 4, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
			{Binding: 5, DescriptorType: vk.DescriptorTypeStorageBuffer, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
		}),
	}); err != nil {
		return fmt.Errorf("creating the descriptor set layout: %w", err)
	}
	if d.imageLayout, err = dev.CreateDescriptorSetLayout(&vk.DescriptorSetLayoutCreateInfo{
		SType:        vk.StructureTypeDescriptorSetLayoutCreateInfo,
		BindingCount: 3,
		PBindings: unsafe.SliceData([]vk.DescriptorSetLayoutBinding{
			{Binding: 0, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
			{Binding: 1, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
			{Binding: 2, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
		}),
	}); err != nil {
		return fmt.Errorf("creating the image descriptor set layout: %w", err)
	}
	if d.descPool, err = dev.CreateDescriptorPool(&vk.DescriptorPoolCreateInfo{
		SType:         vk.StructureTypeDescriptorPoolCreateInfo,
		MaxSets:       1,
		PoolSizeCount: 2,
		PPoolSizes: unsafe.SliceData([]vk.DescriptorPoolSize{
			{Type: vk.DescriptorTypeStorageBuffer, DescriptorCount: 3},
			{Type: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 3},
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
		SetLayoutCount:         2,
		PSetLayouts:            unsafe.SliceData([]vk.DescriptorSetLayout{d.setLayout, d.imageLayout}),
		PushConstantRangeCount: 1,
		PPushConstantRanges:    &vk.PushConstantRange{StageFlags: vk.ShaderStageVertex | vk.ShaderStageFragment, Offset: 0, Size: 16},
	}); err != nil {
		return fmt.Errorf("creating the pipeline layout: %w", err)
	}
	if d.vert, err = d.module(quadVertSPV); err != nil {
		return fmt.Errorf("creating the vertex shader: %w", err)
	}
	if d.frag, err = d.module(quadFragSPV); err != nil {
		return fmt.Errorf("creating the fragment shader: %w", err)
	}
	if d.filterFrag, err = d.module(filterFragSPV); err != nil {
		return fmt.Errorf("creating the filter shader: %w", err)
	}
	if d.plainFrag, err = d.module(plainFragSPV); err != nil {
		return fmt.Errorf("creating the plain shader: %w", err)
	}
	if d.quad, err = d.pipeline(d.frag, FormatRGBA8, true); err != nil {
		return fmt.Errorf("creating the quad pipeline: %w", err)
	}
	if d.blend, err = d.pipeline(d.frag, FormatRGBA8, false); err != nil {
		return fmt.Errorf("creating the blend pipeline: %w", err)
	}
	if d.plain, err = d.pipeline(d.plainFrag, FormatRGBA8, true); err != nil {
		return fmt.Errorf("creating the plain pipeline: %w", err)
	}
	if d.opaque, err = d.pipeline(d.plainFrag, FormatRGBA8, false); err != nil {
		return fmt.Errorf("creating the opaque pipeline: %w", err)
	}
	for i := range d.filter {
		if d.filter[i], err = d.pipeline(d.filterFrag, TargetFormat(i), false); err != nil {
			return fmt.Errorf("creating the filter pipeline: %w", err)
		}
	}

	// The buffers and the textures start small and grow with the frames;
	// the sets always name valid ones.
	if err := d.growBuffer(&d.instances, 64*InstanceSize, vk.BufferUsageStorageBuffer); err != nil {
		return err
	}
	if err := d.growBuffer(&d.segments, 64*4*RampSegmentSize, vk.BufferUsageStorageBuffer); err != nil {
		return err
	}
	if err := d.growBuffer(&d.taps, 64*8, vk.BufferUsageStorageBuffer); err != nil {
		return err
	}
	if err := d.growRamps(8); err != nil {
		return err
	}
	for i := range d.atlases {
		if err := d.sizeAtlas(i, AtlasInfo{}); err != nil {
			return err
		}
	}
	// A transparent pixel stands for the image while a batch reads none.
	if d.noImage, err = d.newImage(ImageUpload{Width: 1, Height: 1}); err != nil {
		return err
	}
	return d.run(func(cb vk.CommandBuffer) { d.clearTexture(cb, d.noImage.tex.img, 0) })
}

func (d *vkDevice) module(code []byte) (vk.ShaderModule, error) {
	words := vk.SPIRV(code)
	return d.dev.CreateShaderModule(&vk.ShaderModuleCreateInfo{
		SType:    vk.StructureTypeShaderModuleCreateInfo,
		CodeSize: uintptr(len(code)),
		PCode:    &words[0],
	})
}

// pipeline creates a pipeline of instanced quads for the targets of a
// format: four vertices per instance as a triangle strip, no vertex
// buffers, the given fragment shader, and either premultiplied source-over
// or no blending (the shader's output replaces the pixel).
func (d *vkDevice) pipeline(frag vk.ShaderModule, format TargetFormat, blend bool) (vk.Pipeline, error) {
	entry := vk.CString("main")
	enable := vk.Bool32(vk.False)
	if blend {
		enable = vk.True
	}
	pipelines, err := d.dev.CreateGraphicsPipelines(0, []vk.GraphicsPipelineCreateInfo{{
		SType:      vk.StructureTypeGraphicsPipelineCreateInfo,
		StageCount: 2,
		PStages: unsafe.SliceData([]vk.PipelineShaderStageCreateInfo{
			{SType: vk.StructureTypePipelineShaderStageCreateInfo, Stage: vk.ShaderStageVertex, Module: d.vert, PName: entry},
			{SType: vk.StructureTypePipelineShaderStageCreateInfo, Stage: vk.ShaderStageFragment, Module: frag, PName: entry},
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
				BlendEnable:         enable,
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
		RenderPass:        d.renderPass[format],
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
		for e := range d.exports {
			e.destroy()
		}
		d.exports = nil
		d.releaseResources()
		d.dropScratch()
		d.dropProfile()
		for _, p := range d.sourcePools {
			dev.DestroyDescriptorPool(p)
		}
		d.sourcePools = nil
		if d.noImage != nil {
			d.destroyImage(d.noImage)
			d.noImage = nil
		}
		for _, p := range d.imagePools {
			dev.DestroyDescriptorPool(p.pool)
		}
		d.imagePools = nil
		for i := range d.atlases {
			d.destroyTexture(&d.atlases[i].tex)
		}
		d.destroyTexture(&d.ramps)
		for _, b := range []*hostBuffer{&d.instances, &d.segments, &d.taps, &d.staging, &d.readback} {
			d.destroyBuffer(b)
		}
		for _, p := range []vk.Pipeline{d.quad, d.blend, d.plain, d.opaque, d.filter[0], d.filter[1]} {
			if p != 0 {
				dev.DestroyPipeline(p)
			}
		}
		for _, m := range []vk.ShaderModule{d.vert, d.frag, d.filterFrag, d.plainFrag} {
			if m != 0 {
				dev.DestroyShaderModule(m)
			}
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
		if d.imageLayout != 0 {
			dev.DestroyDescriptorSetLayout(d.imageLayout)
		}
		if d.sampler != 0 {
			dev.DestroySampler(d.sampler)
		}
		for _, p := range d.renderPass {
			if p != 0 {
				dev.DestroyRenderPass(p)
			}
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
	typ, ok := uint32(0), false
	if usage&vk.BufferUsageTransferDst != 0 {
		// A buffer the host reads: cached memory, where the device has it.
		// Measured: reading 4K back from the write-combined memory NVIDIA
		// lists first takes 95 ms, 350 MB/s; from cached memory, 7 ms.
		typ, ok = d.mem.FindMemoryType(req.MemoryTypeBits, vk.MemoryPropertyHostVisible|vk.MemoryPropertyHostCoherent|vk.MemoryPropertyHostCached)
	}
	if !ok {
		typ, ok = d.mem.FindMemoryType(req.MemoryTypeBits, vk.MemoryPropertyHostVisible|vk.MemoryPropertyHostCoherent)
	}
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

// newTexture creates a 2D R8G8B8A8_UNORM image in device memory, with its
// view.
func (d *vkDevice) newTexture(width, height int, usage vk.ImageUsageFlags) (texture, error) {
	return d.newTextureOf(targetFormat, width, height, 0, usage)
}

// newTextureOf creates a 2D image in device memory, with its view: an
// array of the given number of layers, or with 0 a plain image.
func (d *vkDevice) newTextureOf(format vk.Format, width, height, layers int, usage vk.ImageUsageFlags) (t texture, err error) {
	dev := d.dev
	viewType := vk.ImageViewType2DArray
	if layers == 0 {
		layers, viewType = 1, vk.ImageViewType2D
	}
	defer func() {
		if err != nil {
			d.destroyTexture(&t)
		}
	}()
	if t.img, err = dev.CreateImage(&vk.ImageCreateInfo{
		SType:         vk.StructureTypeImageCreateInfo,
		ImageType:     vk.ImageType2D,
		Format:        format,
		Extent:        vk.Extent3D{Width: uint32(width), Height: uint32(height), Depth: 1},
		MipLevels:     1,
		ArrayLayers:   uint32(layers),
		Samples:       vk.SampleCount1,
		Tiling:        vk.ImageTilingOptimal,
		Usage:         usage,
		SharingMode:   vk.SharingModeExclusive,
		InitialLayout: vk.ImageLayoutUndefined,
	}); err != nil {
		return t, fmt.Errorf("creating a %dx%d image of %d layers: %w", width, height, layers, err)
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
		ViewType: viewType,
		Format:   format,
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

// transition returns the barrier that moves the whole of an image, every
// layer, from one layout to another.
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
		{
			SType: vk.StructureTypeWriteDescriptorSet, DstSet: d.set, DstBinding: 3, DescriptorCount: 1,
			DescriptorType: vk.DescriptorTypeCombinedImageSampler,
			PImageInfo:     &vk.DescriptorImageInfo{Sampler: d.sampler, ImageView: d.atlases[AtlasCoverage].tex.view, ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal},
		},
		{
			SType: vk.StructureTypeWriteDescriptorSet, DstSet: d.set, DstBinding: 4, DescriptorCount: 1,
			DescriptorType: vk.DescriptorTypeCombinedImageSampler,
			PImageInfo:     &vk.DescriptorImageInfo{Sampler: d.sampler, ImageView: d.atlases[AtlasColor].tex.view, ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal},
		},
		{
			SType: vk.StructureTypeWriteDescriptorSet, DstSet: d.set, DstBinding: 5, DescriptorCount: 1,
			DescriptorType: vk.DescriptorTypeStorageBuffer,
			PBufferInfo:    &vk.DescriptorBufferInfo{Buffer: d.taps.buf, Range: vk.WholeSize},
		},
	}, nil)
}

// clearTexture records the commands that make a new texture transparent
// and ready to be read by the shader. With keep layers of old content to
// copy in afterwards, it leaves the texture ready for the copy instead.
func (d *vkDevice) clearTexture(cb vk.CommandBuffer, img vk.Image, keep int) {
	d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTopOfPipe, vk.PipelineStageTransfer, 0, nil, nil, []vk.ImageMemoryBarrier{
		transition(img, vk.ImageLayoutUndefined, vk.ImageLayoutTransferDstOptimal, 0, vk.AccessTransferWrite),
	})
	zero := vk.ClearColorFloat(0, 0, 0, 0)
	d.dev.CmdClearColorImage(cb, img, vk.ImageLayoutTransferDstOptimal, &zero, []vk.ImageSubresourceRange{colorRange})
	to := vk.ImageLayout(vk.ImageLayoutShaderReadOnlyOptimal)
	stage, access := vk.PipelineStageFlags(vk.PipelineStageFragmentShader), vk.AccessFlags(vk.AccessShaderRead)
	if keep > 0 {
		// The copy writes after the clear.
		to, stage, access = vk.ImageLayoutTransferDstOptimal, vk.PipelineStageTransfer, vk.AccessTransferWrite
	}
	d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, stage, 0, nil, nil, []vk.ImageMemoryBarrier{
		transition(img, vk.ImageLayoutTransferDstOptimal, to, vk.AccessTransferWrite, access),
	})
}

// sizeAtlas makes the texture of atlas i what a frame needs: pages of the
// given size, as many as the frame has, with the pixels of the pages it
// had before; a 1x1 placeholder for an atlas without pages. The texture
// has exactly the pages asked for, so an atlas that shrinks gives its
// memory back.
func (d *vkDevice) sizeAtlas(i int, want AtlasInfo) error {
	a := &d.atlases[i]
	size, pages := want.PageSize, want.Pages
	if pages == 0 {
		size, pages = 1, 1
	}
	if a.tex.img != 0 && a.pageSize == size && a.pages == pages {
		return nil
	}
	if size > d.maxSize {
		return fmt.Errorf("atlas pages of %d pixels: the device's textures are at most %d", size, d.maxSize)
	}
	format := vk.Format(coverageFormat)
	if i == AtlasColor {
		format = targetFormat
	}
	t, err := d.newTextureOf(format, size, size, pages, vk.ImageUsageSampled|vk.ImageUsageTransferDst|vk.ImageUsageTransferSrc)
	if err != nil {
		return err
	}
	// The pages both have keep their pixels.
	keep := 0
	if a.tex.img != 0 && a.pageSize == size {
		keep = min(a.pages, pages)
	}
	err = d.run(func(cb vk.CommandBuffer) {
		d.clearTexture(cb, t.img, keep)
		if keep == 0 {
			return
		}
		dev := d.dev
		dev.CmdPipelineBarrier(cb, vk.PipelineStageFragmentShader, vk.PipelineStageTransfer, 0, nil, nil, []vk.ImageMemoryBarrier{
			transition(a.tex.img, vk.ImageLayoutShaderReadOnlyOptimal, vk.ImageLayoutTransferSrcOptimal, vk.AccessShaderRead, vk.AccessTransferRead),
		})
		layers := vk.ImageSubresourceLayers{AspectMask: vk.ImageAspectColor, LayerCount: uint32(keep)}
		dev.CmdCopyImage(cb, a.tex.img, vk.ImageLayoutTransferSrcOptimal, t.img, vk.ImageLayoutTransferDstOptimal, []vk.ImageCopy{{
			SrcSubresource: layers, DstSubresource: layers,
			Extent: vk.Extent3D{Width: uint32(size), Height: uint32(size), Depth: 1},
		}})
		dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageFragmentShader, 0, nil, nil, []vk.ImageMemoryBarrier{
			transition(t.img, vk.ImageLayoutTransferDstOptimal, vk.ImageLayoutShaderReadOnlyOptimal, vk.AccessTransferWrite, vk.AccessShaderRead),
		})
	})
	if err != nil {
		d.destroyTexture(&t)
		return err
	}
	d.destroyTexture(&a.tex)
	*a = vkAtlas{tex: t, pageSize: size, pages: pages}
	d.setStale = true
	return nil
}

// newImage creates the texture of an image, without its pixels, and the
// descriptor set that binds it.
func (d *vkDevice) newImage(up ImageUpload) (*vkImage, error) {
	dev := d.dev
	im := &vkImage{}
	var err error
	if im.tex, err = d.newTexture(up.Width, up.Height, vk.ImageUsageSampled|vk.ImageUsageTransferDst); err != nil {
		return nil, err
	}
	for _, p := range d.imagePools {
		if p.used < imageSetsPerPool {
			im.pool = p
			break
		}
	}
	if im.pool == nil {
		pool, err := dev.CreateDescriptorPool(&vk.DescriptorPoolCreateInfo{
			SType:         vk.StructureTypeDescriptorPoolCreateInfo,
			Flags:         vk.DescriptorPoolCreateFreeDescriptorSet,
			MaxSets:       imageSetsPerPool,
			PoolSizeCount: 1,
			PPoolSizes:    &vk.DescriptorPoolSize{Type: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 3 * imageSetsPerPool},
		})
		if err != nil {
			d.destroyTexture(&im.tex)
			return nil, fmt.Errorf("creating a descriptor pool: %w", err)
		}
		im.pool = &imagePool{pool: pool}
		d.imagePools = append(d.imagePools, im.pool)
	}
	sets, err := dev.AllocateDescriptorSets(&vk.DescriptorSetAllocateInfo{
		SType:              vk.StructureTypeDescriptorSetAllocateInfo,
		DescriptorPool:     im.pool.pool,
		DescriptorSetCount: 1,
		PSetLayouts:        &d.imageLayout,
	})
	if err != nil {
		d.destroyTexture(&im.tex)
		return nil, fmt.Errorf("allocating a descriptor set: %w", err)
	}
	im.set = sets[0]
	im.pool.used++
	// The other textures of the set are the placeholder (which is this
	// image, for the placeholder itself).
	none := im.tex.view
	if d.noImage != nil {
		none = d.noImage.tex.view
	}
	d.writeTextures(im.set, [3]vk.ImageView{im.tex.view, none, none})
	return im, nil
}

// writeTextures points the three textures of a set of a batch at views.
func (d *vkDevice) writeTextures(set vk.DescriptorSet, views [3]vk.ImageView) {
	infos := make([]vk.DescriptorImageInfo, 3)
	writes := make([]vk.WriteDescriptorSet, 3)
	for i, v := range views {
		infos[i] = vk.DescriptorImageInfo{Sampler: d.sampler, ImageView: v, ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal}
		writes[i] = vk.WriteDescriptorSet{
			SType: vk.StructureTypeWriteDescriptorSet, DstSet: set, DstBinding: uint32(i), DescriptorCount: 1,
			DescriptorType: vk.DescriptorTypeCombinedImageSampler, PImageInfo: &infos[i],
		}
	}
	d.dev.UpdateDescriptorSets(writes, nil)
}

// destroyImage destroys the texture of an image and frees its set. Nothing
// is in flight when it is called.
func (d *vkDevice) destroyImage(im *vkImage) {
	// An error leaves the set to its pool, which is destroyed with it.
	_ = d.dev.FreeDescriptorSets(im.pool.pool, []vk.DescriptorSet{im.set})
	im.pool.used--
	d.destroyTexture(&im.tex)
}

// releaseResources destroys the image textures and the pools they leave
// empty, and forgets whose resources the device holds: the next frame
// must bring its own (Frame.Res.Base 0).
func (d *vkDevice) releaseResources() {
	for id, im := range d.images {
		d.destroyImage(im)
		delete(d.images, id)
	}
	kept := d.imagePools[:0]
	for _, p := range d.imagePools {
		if p.used == 0 {
			d.dev.DestroyDescriptorPool(p.pool)
		} else {
			kept = append(kept, p) // the placeholder's
		}
	}
	clear(d.imagePools[len(kept):])
	d.imagePools = kept
	d.resetSourceSets()
	for id, s := range d.offscreen {
		d.destroySurface(s)
		delete(d.offscreen, id)
	}
	d.resID, d.resSerial = 0, 0
}

// Trim releases the atlases, the image textures, the offscreen targets and
// the buffers of uploads and readbacks.
func (d *vkDevice) Trim() {
	if d.closed || d.dev == nil {
		return
	}
	d.releaseResources()
	for i := range d.atlases {
		if d.lost {
			break // nothing can be submitted; Close destroys the atlases
		}
		// An error keeps the atlas as it is: its pixels are not read again.
		_ = d.sizeAtlas(i, AtlasInfo{})
	}
	d.destroyBuffer(&d.staging)
	d.destroyBuffer(&d.readback)
	d.dropScratch()
}

// Rest applies a frame of Encoder.Rest and releases the device's own
// transient memory.
func (d *vkDevice) Rest(f *Frame) error {
	if err := d.usable(); err != nil {
		return err
	}
	if f == nil || len(f.Passes) > 0 || f.Res.ID != d.resID || f.Res.Base != d.resSerial {
		d.Trim()
		return ErrResourcesLost
	}
	for i := range f.Atlases {
		if err := d.sizeAtlas(i, f.Atlases[i]); err != nil {
			d.Trim()
			return d.check(fmt.Errorf("gpu: %w", err))
		}
	}
	d.resetSourceSets()
	if err := d.syncTargets(f.Targets); err != nil {
		d.Trim()
		return d.check(err)
	}
	d.release(f.Releases)
	d.resSerial = f.Res.Next
	d.destroyBuffer(&d.staging)
	d.destroyBuffer(&d.readback)
	d.dropScratch()
	return nil
}

// dropScratch destroys the surface of the shifts. Nothing is in flight.
func (d *vkDevice) dropScratch() {
	if d.scratch != nil {
		d.destroySurface(d.scratch)
		d.scratch = nil
	}
}

// submit records commands into the device's command buffer, submits them
// and waits until they are done (see run).
func (d *vkDevice) submit(record func(cb vk.CommandBuffer)) error {
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
// Surfaces
// ---------------------------------------------------------------------------

// newSurface creates a color image that is drawn into and read by the
// shaders, with its framebuffer. Its pixels are undefined.
func (d *vkDevice) newSurface(width, height int, format TargetFormat) (*surface, error) {
	if width < 1 || height < 1 || width > d.maxSize || height > d.maxSize || format > FormatRGBA16F {
		return nil, fmt.Errorf("a surface of %dx%d in the format %d: the size must be between 1 and %d", width, height, format, d.maxSize)
	}
	s := &surface{w: width, h: height, format: format, layout: vk.ImageLayoutUndefined}
	vf := vk.Format(targetFormat)
	if format == FormatRGBA16F {
		vf = floatFormat
	}
	var err error
	if s.tex, err = d.newTextureOf(vf, width, height, 0, vk.ImageUsageColorAttachment|vk.ImageUsageSampled|vk.ImageUsageTransferSrc|vk.ImageUsageTransferDst); err != nil {
		return nil, err
	}
	if s.framebuffer, err = d.dev.CreateFramebuffer(&vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass:      d.renderPass[format],
		AttachmentCount: 1,
		PAttachments:    &s.tex.view,
		Width:           uint32(width),
		Height:          uint32(height),
		Layers:          1,
	}); err != nil {
		d.destroySurface(s)
		return nil, fmt.Errorf("creating the framebuffer: %w", err)
	}
	return s, nil
}

func (d *vkDevice) destroySurface(s *surface) {
	if s.framebuffer != 0 {
		d.dev.DestroyFramebuffer(s.framebuffer)
		s.framebuffer = 0
	}
	d.destroyTexture(&s.tex)
}

// layoutSync returns the stage and the access of what uses an image in a
// layout.
func layoutSync(l vk.ImageLayout) (vk.PipelineStageFlags, vk.AccessFlags) {
	switch l {
	case vk.ImageLayoutColorAttachmentOptimal:
		return vk.PipelineStageColorAttachmentOutput, vk.AccessColorAttachmentRead | vk.AccessColorAttachmentWrite
	case vk.ImageLayoutShaderReadOnlyOptimal:
		return vk.PipelineStageFragmentShader, vk.AccessShaderRead
	case vk.ImageLayoutTransferSrcOptimal:
		return vk.PipelineStageTransfer, vk.AccessTransferRead
	case vk.ImageLayoutTransferDstOptimal:
		return vk.PipelineStageTransfer, vk.AccessTransferWrite
	}
	return vk.PipelineStageTopOfPipe, 0 // undefined: nothing to wait for
}

// use records the barrier that brings a surface to a layout: to be drawn
// into, read by a shader or copied from. It is not called inside a render
// pass.
func (d *vkDevice) use(cb vk.CommandBuffer, s *surface, to vk.ImageLayout) {
	if s.layout == to {
		return
	}
	srcStage, srcAccess := layoutSync(s.layout)
	dstStage, dstAccess := layoutSync(to)
	d.dev.CmdPipelineBarrier(cb, srcStage, dstStage, 0, nil, nil, []vk.ImageMemoryBarrier{
		transition(s.tex.img, s.layout, to, srcAccess, dstAccess),
	})
	d.undo = append(d.undo, layoutChange{s, s.layout})
	s.layout = to
}

// syncTargets makes the offscreen targets those of a frame's list: the
// ones missing are created, the ones not listed destroyed. Nothing is in
// flight.
func (d *vkDevice) syncTargets(list []TargetInfo) error {
	d.gen++
	for _, info := range list {
		s := d.offscreen[info.ID]
		if s != nil && (s.w != info.Width || s.h != info.Height || s.format != info.Format) {
			d.destroySurface(s)
			delete(d.offscreen, info.ID)
			s = nil
		}
		if s == nil {
			if info.ID <= 0 {
				return fmt.Errorf("gpu: an offscreen target with the ID %d", info.ID)
			}
			var err error
			if s, err = d.newSurface(info.Width, info.Height, info.Format); err != nil {
				return fmt.Errorf("gpu: %w", err)
			}
			d.offscreen[info.ID] = s
		}
		s.gen = d.gen
	}
	for id, s := range d.offscreen {
		if s.gen != d.gen {
			d.destroySurface(s)
			delete(d.offscreen, id)
		}
	}
	return nil
}

// resetSourceSets frees the descriptor sets of the batches of the last
// frame. Nothing is in flight.
func (d *vkDevice) resetSourceSets() {
	if len(d.sourceSets) == 0 {
		return
	}
	for _, p := range d.sourcePools {
		// An error leaves the sets allocated: the pool is then full, and
		// the next allocation fails with a message.
		_ = d.dev.ResetDescriptorPool(p)
	}
	clear(d.sourceSets)
	d.sourceUsed = 0
}

// sourceSet returns the descriptor set that binds three textures for a
// batch of the frame being drawn.
func (d *vkDevice) sourceSet(views [3]vk.ImageView) (vk.DescriptorSet, error) {
	if set, ok := d.sourceSets[views]; ok {
		return set, nil
	}
	pi := d.sourceUsed / sourceSetsPerPool
	if pi == len(d.sourcePools) {
		pool, err := d.dev.CreateDescriptorPool(&vk.DescriptorPoolCreateInfo{
			SType:         vk.StructureTypeDescriptorPoolCreateInfo,
			MaxSets:       sourceSetsPerPool,
			PoolSizeCount: 1,
			PPoolSizes:    &vk.DescriptorPoolSize{Type: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 3 * sourceSetsPerPool},
		})
		if err != nil {
			return 0, fmt.Errorf("gpu: creating a descriptor pool: %w", err)
		}
		d.sourcePools = append(d.sourcePools, pool)
	}
	sets, err := d.dev.AllocateDescriptorSets(&vk.DescriptorSetAllocateInfo{
		SType:              vk.StructureTypeDescriptorSetAllocateInfo,
		DescriptorPool:     d.sourcePools[pi],
		DescriptorSetCount: 1,
		PSetLayouts:        &d.imageLayout,
	})
	if err != nil {
		return 0, fmt.Errorf("gpu: allocating a descriptor set: %w", err)
	}
	d.sourceUsed++
	d.writeTextures(sets[0], views)
	d.sourceSets[views] = sets[0]
	return sets[0], nil
}

// ---------------------------------------------------------------------------
// Targets
// ---------------------------------------------------------------------------

type vkTarget struct {
	d     *vkDevice // nil once closed
	owner *vkDevice
	size  image.Point
	surf  *surface
}

func (d *vkDevice) NewTarget(width, height int) (Target, error) {
	if err := d.usable(); err != nil {
		return nil, err
	}
	if width < 1 || height < 1 || width > d.maxSize || height > d.maxSize {
		return nil, fmt.Errorf("gpu: a target of %dx%d: the size must be between 1 and %d", width, height, d.maxSize)
	}
	t := &vkTarget{d: d, owner: d, size: image.Pt(width, height)}
	var err error
	if t.surf, err = d.newSurface(width, height, FormatRGBA8); err != nil {
		return nil, d.check(fmt.Errorf("gpu: %w", err))
	}
	// Transparent, and in the layout it rests in between frames: ready to
	// be read back.
	err = d.run(func(cb vk.CommandBuffer) {
		d.use(cb, t.surf, vk.ImageLayoutTransferDstOptimal)
		zero := vk.ClearColorFloat(0, 0, 0, 0)
		d.dev.CmdClearColorImage(cb, t.surf.tex.img, vk.ImageLayoutTransferDstOptimal, &zero, []vk.ImageSubresourceRange{colorRange})
		d.use(cb, t.surf, vk.ImageLayoutTransferSrcOptimal)
	})
	if err != nil {
		t.destroy()
		return nil, d.check(err)
	}
	d.targets[t] = struct{}{}
	return t, nil
}

func (t *vkTarget) Size() image.Point { return t.size }
func (t *vkTarget) Device() Device    { return t.owner }

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
	if t.surf != nil {
		d.destroySurface(t.surf)
		t.surf = nil
	}
	t.d = nil
}

// usable returns the error of a call into a target that is closed, or
// whose device is closed or lost, or nil.
func (t *vkTarget) usable() error {
	if t.d == nil || t.d.closed {
		return errors.New("gpu: the target is closed")
	}
	return t.d.usable()
}

func (t *vkTarget) Draw(f *Frame) error {
	d := t.d
	if err := t.usable(); err != nil {
		return err
	}
	if f.Size != t.size {
		return fmt.Errorf("gpu: a frame encoded for %v drawn into a target of %v", f.Size, t.size)
	}
	// Whose resources the device holds, and how far it followed them.
	if f.Res.ID != d.resID || f.Res.Base != d.resSerial {
		if f.Res.Base != 0 {
			return ErrResourcesLost
		}
		// A frame that brings everything it draws with: start from it.
		d.releaseResources()
	}
	if err := d.draw(t, f); err != nil {
		// What the frame left on the device is not known.
		d.releaseResources()
		return d.check(err)
	}
	d.resID, d.resSerial = f.Res.ID, f.Res.Next
	return nil
}

// stagedImage is a new image on its way to its texture.
type stagedImage struct {
	im  *vkImage
	up  *ImageUpload
	off int // of its pixels in the staging buffer
}

// surfaceOf returns the surface of a target of a frame drawn into t: the
// destination's (0), or an offscreen one's.
func (d *vkDevice) surfaceOf(t *vkTarget, id int) *surface {
	if id == 0 {
		return t.surf
	}
	return d.offscreen[id]
}

// run records commands into the device's command buffer, submits them and
// waits until they are done. The layouts the commands leave the surfaces
// in are forgotten when they are not executed.
func (d *vkDevice) run(record func(cb vk.CommandBuffer)) error {
	d.undo = d.undo[:0]
	err := d.submit(record)
	if err != nil {
		for i := len(d.undo) - 1; i >= 0; i-- {
			d.undo[i].s.layout = d.undo[i].from
		}
	}
	clear(d.undo)
	d.undo = d.undo[:0]
	return err
}

// plan resolves what every batch of the frame is drawn with, into d.plan
// in the order of the passes, and checks the passes against the device's
// targets.
func (d *vkDevice) planFrame(t *vkTarget, f *Frame) error {
	d.plan = d.plan[:0]
	d.resetSourceSets()
	none := d.noImage.tex.view
	for pi := range f.Passes {
		pass := &f.Passes[pi]
		s := d.surfaceOf(t, pass.Target)
		if s == nil {
			return fmt.Errorf("gpu: a pass draws into the target %d, which the frame does not list", pass.Target)
		}
		whole := image.Rect(0, 0, s.w, s.h)
		for _, u := range pass.Uploads {
			if u.Atlas < 0 || u.Atlas >= NumAtlases {
				return fmt.Errorf("gpu: an upload to the atlas %d", u.Atlas)
			}
			a := &d.atlases[u.Atlas]
			if u.Page < 0 || u.Page >= a.pages || !u.Rect.In(image.Rect(0, 0, a.pageSize, a.pageSize)) || u.Rect.Empty() {
				return fmt.Errorf("gpu: an upload to %v of page %d of atlas %d, which has %d pages of %d pixels", u.Rect, u.Page, u.Atlas, a.pages, a.pageSize)
			}
		}
		for bi := range pass.Batches {
			b := &pass.Batches[bi]
			var pb plannedBatch
			switch {
			case b.Pipeline == PipelineQuad && s.format == FormatRGBA8:
				pb.pipeline = d.quad
			case b.Pipeline == PipelineBlend && s.format == FormatRGBA8:
				pb.pipeline = d.blend
			case b.Pipeline == PipelinePlain && s.format == FormatRGBA8:
				pb.pipeline = d.plain
			case b.Pipeline == PipelineOpaque && s.format == FormatRGBA8:
				pb.pipeline = d.opaque
			case b.Pipeline == PipelineFilter:
				pb.pipeline = d.filter[s.format]
			default:
				return fmt.Errorf("gpu: a batch of the pipeline %d in a target of the format %d", b.Pipeline, s.format)
			}
			if b.First < 0 || b.Count < 0 || (b.First+b.Count)*InstanceSize > len(f.Instances) {
				return fmt.Errorf("gpu: a batch of the instances %d to %d of %d", b.First, b.First+b.Count, len(f.Instances)/InstanceSize)
			}
			sc := b.Scissor.Sub(pass.Origin).Intersect(whole)
			pb.scissor, pb.skip = rect2D(sc), sc.Empty() || b.Count == 0
			views := [3]vk.ImageView{none, none, none}
			reads := false
			if b.Image != 0 {
				im := d.images[b.Image]
				if im == nil {
					return fmt.Errorf("gpu: a batch reads the image %d, which the device does not hold", b.Image)
				}
				if b.Sources[0] != 0 {
					return fmt.Errorf("gpu: a batch reads the image %d and the target %d as one texture", b.Image, b.Sources[0])
				}
				views[0] = im.tex.view
				pb.set = im.set
			}
			for i, src := range b.Sources {
				if src == 0 {
					continue
				}
				id := int(src)
				if src == SourceDest {
					id = 0
				}
				from := d.surfaceOf(t, id)
				if from == nil || id < 0 {
					return fmt.Errorf("gpu: a batch reads the target %d, which the frame does not list", src)
				}
				if from == s {
					return fmt.Errorf("gpu: a pass into the target %d reads it", pass.Target)
				}
				views[i] = from.tex.view
				reads = true
			}
			switch {
			case reads:
				var err error
				if pb.set, err = d.sourceSet(views); err != nil {
					return err
				}
			case b.Image == 0:
				pb.set = d.noImage.set
			}
			d.plan = append(d.plan, pb)
		}
	}
	return nil
}

// draw executes a frame whose resources follow the device's.
func (d *vkDevice) draw(t *vkTarget, f *Frame) error {
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
	if err := d.growBuffer(&d.taps, 4*len(f.Taps), vk.BufferUsageStorageBuffer); err != nil {
		return fmt.Errorf("gpu: %w", err)
	}
	if n := len(f.Taps); n > 0 {
		copy(d.taps.bytes, unsafe.Slice((*byte)(unsafe.Pointer(&f.Taps[0])), 4*n))
	}
	rampRows := len(f.Ramps) / (4 * RampWidth)
	// perf: the ramps are uploaded whole whenever they differ from the
	// previous frame's.
	rampUpload := rampRows > 0 && !bytes.Equal(f.Ramps, d.uploaded)
	if rampUpload {
		if err := d.growRamps(rampRows); err != nil {
			return fmt.Errorf("gpu: %w", err)
		}
	}
	for i := range f.Atlases {
		if err := d.sizeAtlas(i, f.Atlases[i]); err != nil {
			return fmt.Errorf("gpu: %w", err)
		}
	}
	// The sets of the last frame's batches go before the targets they
	// name do.
	d.resetSourceSets()
	if err := d.syncTargets(f.Targets); err != nil {
		return err
	}
	// The textures of the new images.
	var created []stagedImage
	for i := range f.Images {
		up := &f.Images[i]
		if d.images[up.ID] != nil {
			continue // an image never changes
		}
		if up.Width < 1 || up.Height < 1 || up.Width > d.maxSize || up.Height > d.maxSize || len(up.Pix) != 4*up.Width*up.Height {
			return fmt.Errorf("gpu: an image of %dx%d with %d bytes: the size must be between 1 and %d", up.Width, up.Height, len(up.Pix), d.maxSize)
		}
		im, err := d.newImage(*up)
		if err != nil {
			return fmt.Errorf("gpu: %w", err)
		}
		d.images[up.ID] = im
		created = append(created, stagedImage{im: im, up: up})
	}
	if err := d.planFrame(t, f); err != nil {
		return err
	}

	// Everything on its way to a texture goes through the staging buffer:
	// the ramps, the atlas uploads of every pass (Frame.Data, at the
	// offsets the uploads name) and the pixels of the new images.
	rampOff, dataOff := 0, 0
	if rampUpload {
		dataOff = len(f.Ramps)
	}
	staged := dataOff + (len(f.Data)+3)&^3
	for i := range created {
		created[i].off = staged
		staged += len(created[i].up.Pix)
	}
	if staged == 0 && len(d.staging.bytes) > 1<<20 {
		// What a large upload needed is not kept by frames that upload
		// nothing; small uploads keep their buffer.
		d.destroyBuffer(&d.staging)
	}
	if staged > 0 {
		if err := d.growBuffer(&d.staging, staged, vk.BufferUsageTransferSrc); err != nil {
			return fmt.Errorf("gpu: %w", err)
		}
		if rampUpload {
			copy(d.staging.bytes[rampOff:], f.Ramps)
		}
		copy(d.staging.bytes[dataOff:], f.Data)
		for _, c := range created {
			copy(d.staging.bytes[c.off:], c.up.Pix)
		}
	}
	d.writeSet()

	if len(f.Passes) == 0 && !rampUpload && len(created) == 0 {
		d.release(f.Releases)
		return nil // nothing to record
	}
	prelude := func(cb vk.CommandBuffer) {
		toDst := func(img vk.Image, from vk.ImageLayout) {
			stage, access := vk.PipelineStageFlags(vk.PipelineStageFragmentShader), vk.AccessFlags(vk.AccessShaderRead)
			if from == vk.ImageLayoutUndefined {
				stage, access = vk.PipelineStageTopOfPipe, 0
			}
			dev.CmdPipelineBarrier(cb, stage, vk.PipelineStageTransfer, 0, nil, nil, []vk.ImageMemoryBarrier{
				transition(img, from, vk.ImageLayoutTransferDstOptimal, access, vk.AccessTransferWrite),
			})
		}
		toRead := func(img vk.Image) {
			dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageFragmentShader, 0, nil, nil, []vk.ImageMemoryBarrier{
				transition(img, vk.ImageLayoutTransferDstOptimal, vk.ImageLayoutShaderReadOnlyOptimal, vk.AccessTransferWrite, vk.AccessShaderRead),
			})
		}
		if rampUpload {
			toDst(d.ramps.img, vk.ImageLayoutShaderReadOnlyOptimal)
			dev.CmdCopyBufferToImage(cb, d.staging.buf, d.ramps.img, vk.ImageLayoutTransferDstOptimal, []vk.BufferImageCopy{{
				BufferOffset:     uint64(rampOff),
				ImageSubresource: colorLayers,
				ImageExtent:      vk.Extent3D{Width: RampWidth, Height: uint32(rampRows), Depth: 1},
			}})
			toRead(d.ramps.img)
		}
		for _, c := range created {
			toDst(c.im.tex.img, vk.ImageLayoutUndefined)
			dev.CmdCopyBufferToImage(cb, d.staging.buf, c.im.tex.img, vk.ImageLayoutTransferDstOptimal, []vk.BufferImageCopy{{
				BufferOffset:     uint64(c.off),
				ImageSubresource: colorLayers,
				ImageExtent:      vk.Extent3D{Width: uint32(c.up.Width), Height: uint32(c.up.Height), Depth: 1},
			}})
			toRead(c.im.tex.img)
		}
	}
	var err error
	if d.prof != nil {
		// A measurement: a submission per pass, and more (vulkan_profile.go).
		err = d.drawProfiled(t, f, prelude, dataOff)
	} else {
		err = d.run(func(cb vk.CommandBuffer) {
			prelude(cb)
			next := 0 // in d.plan
			for pi := range f.Passes {
				pass := &f.Passes[pi]
				plan := d.plan[next : next+len(pass.Batches)]
				next += len(pass.Batches)
				d.recordPass(cb, t, pass, plan, dataOff)
			}
			// The destination rests ready to be read back.
			d.use(cb, t.surf, vk.ImageLayoutTransferSrcOptimal)
		})
	}
	if err != nil {
		return err
	}
	if rampUpload {
		d.uploaded = append(d.uploaded[:0], f.Ramps...)
	}
	// The frame is drawn: what it gave up can go.
	d.release(f.Releases)
	return nil
}

// recordPass records one pass of a frame drawn into t: its uploads, the
// barriers of the targets it reads and draws into, and its render pass.
// plan is what its batches are drawn with (planFrame), and dataOff where
// Frame.Data starts in the staging buffer.
func (d *vkDevice) recordPass(cb vk.CommandBuffer, t *vkTarget, pass *Pass, plan []plannedBatch, dataOff int) {
	dev := d.dev
	s := d.surfaceOf(t, pass.Target)
	toDst := func(img vk.Image) {
		dev.CmdPipelineBarrier(cb, vk.PipelineStageFragmentShader, vk.PipelineStageTransfer, 0, nil, nil, []vk.ImageMemoryBarrier{
			transition(img, vk.ImageLayoutShaderReadOnlyOptimal, vk.ImageLayoutTransferDstOptimal, vk.AccessShaderRead, vk.AccessTransferWrite),
		})
	}
	toRead := func(img vk.Image) {
		dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageFragmentShader, 0, nil, nil, []vk.ImageMemoryBarrier{
			transition(img, vk.ImageLayoutTransferDstOptimal, vk.ImageLayoutShaderReadOnlyOptimal, vk.AccessTransferWrite, vk.AccessShaderRead),
		})
	}
	// The pictures new in the atlases, after the draws of the
	// passes before, which may have read the same places.
	for ai := range d.atlases {
		var regions []vk.BufferImageCopy
		for _, u := range pass.Uploads {
			if u.Atlas != ai {
				continue
			}
			regions = append(regions, vk.BufferImageCopy{
				BufferOffset:     uint64(dataOff + u.Offset),
				ImageSubresource: vk.ImageSubresourceLayers{AspectMask: vk.ImageAspectColor, BaseArrayLayer: uint32(u.Page), LayerCount: 1},
				ImageOffset:      vk.Offset3D{X: int32(u.Rect.Min.X), Y: int32(u.Rect.Min.Y)},
				ImageExtent:      vk.Extent3D{Width: uint32(u.Rect.Dx()), Height: uint32(u.Rect.Dy()), Depth: 1},
			})
		}
		if len(regions) > 0 {
			img := d.atlases[ai].tex.img
			toDst(img)
			dev.CmdCopyBufferToImage(cb, d.staging.buf, img, vk.ImageLayoutTransferDstOptimal, regions)
			toRead(img)
		}
	}
	// The targets the pass reads, then the one it draws into.
	for bi := range pass.Batches {
		for _, src := range pass.Batches[bi].Sources {
			switch {
			case src == SourceDest:
				d.use(cb, t.surf, vk.ImageLayoutShaderReadOnlyOptimal)
			case src > 0:
				d.use(cb, d.offscreen[int(src)], vk.ImageLayoutShaderReadOnlyOptimal)
			}
		}
	}
	d.use(cb, s, vk.ImageLayoutColorAttachmentOptimal)
	whole := image.Rect(0, 0, s.w, s.h)
	dev.CmdBeginRenderPass(cb, &vk.RenderPassBeginInfo{
		SType:       vk.StructureTypeRenderPassBeginInfo,
		RenderPass:  d.renderPass[s.format],
		Framebuffer: s.framebuffer,
		RenderArea:  rect2D(whole),
	}, vk.SubpassContentsInline)
	if c := pass.Clear; c != nil {
		if r := c.Rect.Sub(pass.Origin).Intersect(whole); !r.Empty() {
			dev.CmdClearAttachments(cb,
				[]vk.ClearAttachment{{AspectMask: vk.ImageAspectColor, ColorAttachment: 0, ClearValue: vk.ClearColor(
					float32(c.Color[0])/255, float32(c.Color[1])/255, float32(c.Color[2])/255, float32(c.Color[3])/255)}},
				[]vk.ClearRect{{Rect: rect2D(r), LayerCount: 1}})
		}
	}
	if len(plan) > 0 {
		// Pixels of the pass to clip space, and to device pixels.
		constants := &[4]float32{2 / float32(s.w), 2 / float32(s.h), float32(pass.Origin.X), float32(pass.Origin.Y)}
		dev.CmdSetViewport(cb, 0, []vk.Viewport{{Width: float32(s.w), Height: float32(s.h), MinDepth: 0, MaxDepth: 1}})
		dev.CmdBindDescriptorSets(cb, vk.PipelineBindPointGraphics, d.pipelineLayout, 0, []vk.DescriptorSet{d.set}, nil)
		dev.CmdPushConstants(cb, d.pipelineLayout, vk.ShaderStageVertex|vk.ShaderStageFragment, 0, 16, unsafe.Pointer(constants))
		var pipeline vk.Pipeline
		var set vk.DescriptorSet
		for i := range plan {
			pb := &plan[i]
			if pb.skip {
				continue
			}
			if pb.pipeline != pipeline {
				pipeline = pb.pipeline
				dev.CmdBindPipeline(cb, vk.PipelineBindPointGraphics, pipeline)
			}
			if pb.set != set {
				set = pb.set
				dev.CmdBindDescriptorSets(cb, vk.PipelineBindPointGraphics, d.pipelineLayout, 1, []vk.DescriptorSet{set}, nil)
			}
			// perf: one scissor command per batch, each a small heap
			// allocation (see internal/vk's doc).
			dev.CmdSetScissor(cb, 0, []vk.Rect2D{pb.scissor})
			b := &pass.Batches[i]
			dev.CmdDraw(cb, 4, uint32(b.Count), 0, uint32(b.First))
		}
	}
	dev.CmdEndRenderPass(cb)
}

// release destroys the textures of the images a frame gave up.
func (d *vkDevice) release(ids []uint64) {
	for _, id := range ids {
		if im := d.images[id]; im != nil {
			d.destroyImage(im)
			delete(d.images, id)
		}
	}
}

func rect2D(r image.Rectangle) vk.Rect2D {
	return vk.Rect2D{
		Offset: vk.Offset2D{X: int32(r.Min.X), Y: int32(r.Min.Y)},
		Extent: vk.Extent2D{Width: uint32(r.Dx()), Height: uint32(r.Dy())},
	}
}

func (t *vkTarget) ReadPixels(dst *image.RGBA, rects ...image.Rectangle) error {
	d := t.d
	if err := t.usable(); err != nil {
		return err
	}
	// The rectangles, clipped, one after the other in the buffer.
	whole := image.Rectangle{Max: t.size}.Intersect(dst.Rect)
	var one [1]vk.BufferImageCopy
	regions := one[:0]
	n := 0
	for _, r := range rects {
		if r = r.Intersect(whole); r.Empty() {
			continue
		}
		regions = append(regions, vk.BufferImageCopy{
			BufferOffset:     uint64(n),
			ImageSubresource: colorLayers,
			ImageOffset:      vk.Offset3D{X: int32(r.Min.X), Y: int32(r.Min.Y)},
			ImageExtent:      vk.Extent3D{Width: uint32(r.Dx()), Height: uint32(r.Dy()), Depth: 1},
		})
		n += 4 * r.Dx() * r.Dy()
	}
	if len(regions) == 0 {
		return nil
	}
	if err := d.growBuffer(&d.readback, n, vk.BufferUsageTransferDst); err != nil {
		return d.check(fmt.Errorf("gpu: %w", err))
	}
	dev := d.dev
	err := d.run(func(cb vk.CommandBuffer) {
		d.use(cb, t.surf, vk.ImageLayoutTransferSrcOptimal)
		dev.CmdCopyImageToBuffer(cb, t.surf.tex.img, vk.ImageLayoutTransferSrcOptimal, d.readback.buf, regions)
		dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageHost, 0, []vk.MemoryBarrier{{
			SType:         vk.StructureTypeMemoryBarrier,
			SrcAccessMask: vk.AccessTransferWrite,
			DstAccessMask: vk.AccessHostRead,
		}}, nil, nil)
	})
	if err != nil {
		return d.check(err)
	}
	// perf: a second copy of every pixel, from the mapped buffer into the
	// image; a window that presents the device's images has neither
	// (ADR-281).
	for i := range regions {
		rg := &regions[i]
		x, y, w, h := int(rg.ImageOffset.X), int(rg.ImageOffset.Y), int(rg.ImageExtent.Width), int(rg.ImageExtent.Height)
		src := d.readback.bytes[rg.BufferOffset:]
		row := 4 * w
		for j := 0; j < h; j++ {
			o := dst.PixOffset(x, y+j)
			copy(dst.Pix[o:o+row], src[j*row:])
		}
	}
	return nil
}

// growScratch makes the surface of the shifts hold at least width by
// height pixels. Its pixels are undefined.
func (d *vkDevice) growScratch(width, height int) error {
	if s := d.scratch; s != nil {
		if s.w >= width && s.h >= height {
			return nil
		}
		width, height = max(width, s.w), max(height, s.h)
		d.dropScratch()
	}
	s, err := d.newSurface(width, height, FormatRGBA8)
	if err != nil {
		return err
	}
	d.scratch = s
	return nil
}

func (t *vkTarget) Shift(r image.Rectangle, dx, dy int) error {
	d := t.d
	if err := t.usable(); err != nil {
		return err
	}
	// The pixels whose source is in the target too.
	whole := image.Rectangle{Max: t.size}
	delta := image.Pt(dx, dy)
	r = r.Intersect(whole).Intersect(whole.Add(delta))
	if r.Empty() || delta == (image.Point{}) {
		return nil
	}
	if err := d.growScratch(r.Dx(), r.Dy()); err != nil {
		return d.check(fmt.Errorf("gpu: %w", err))
	}
	dev := d.dev
	from := r.Sub(delta)
	extent := vk.Extent3D{Width: uint32(r.Dx()), Height: uint32(r.Dy()), Depth: 1}
	err := d.run(func(cb vk.CommandBuffer) {
		// Through the scratch surface: the regions of one copy inside an
		// image may not overlap.
		d.use(cb, t.surf, vk.ImageLayoutTransferSrcOptimal)
		d.use(cb, d.scratch, vk.ImageLayoutTransferDstOptimal)
		dev.CmdCopyImage(cb, t.surf.tex.img, vk.ImageLayoutTransferSrcOptimal, d.scratch.tex.img, vk.ImageLayoutTransferDstOptimal, []vk.ImageCopy{{
			SrcSubresource: colorLayers, DstSubresource: colorLayers,
			SrcOffset: vk.Offset3D{X: int32(from.Min.X), Y: int32(from.Min.Y)},
			Extent:    extent,
		}})
		d.use(cb, d.scratch, vk.ImageLayoutTransferSrcOptimal)
		d.use(cb, t.surf, vk.ImageLayoutTransferDstOptimal)
		dev.CmdCopyImage(cb, d.scratch.tex.img, vk.ImageLayoutTransferSrcOptimal, t.surf.tex.img, vk.ImageLayoutTransferDstOptimal, []vk.ImageCopy{{
			SrcSubresource: colorLayers, DstSubresource: colorLayers,
			DstOffset: vk.Offset3D{X: int32(r.Min.X), Y: int32(r.Min.Y)},
			Extent:    extent,
		}})
		// The destination rests ready to be read back.
		d.use(cb, t.surf, vk.ImageLayoutTransferSrcOptimal)
	})
	return d.check(err)
}
