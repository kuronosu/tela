//go:build telagpu && (linux || windows)

// The declarations are transcribed from the Vulkan headers
// (vulkan/vulkan_core.h), Copyright 2015-2026 The Khronos Group Inc.,
// SPDX-License-Identifier: Apache-2.0 OR MIT (see THIRD_PARTY.md).

package vk

import (
	"structs"
	"unsafe"
)

// The structs of the bindings, and only them: internal/layoutgen takes
// every type declared in this file as the Go form of the C type named Vk
// plus its name, and TestLayout checks size, alignment and every field
// against the real header (see the package documentation).
//
// A struct has the members of its C struct in the same order and under the
// same names with the first letter in upper case. Pointers are Go pointers
// (to the first element, for arrays), `const char*` is *byte (see
// [CString]), flags that Vulkan reserves or that the bindings have no
// constants for are uint32, and VkDeviceSize is uint64. The blank
// structs.HostLayout field asks the compiler for the C layout.

// Extent2D is VkExtent2D.
type Extent2D struct {
	_      structs.HostLayout
	Width  uint32
	Height uint32
}

// Extent3D is VkExtent3D.
type Extent3D struct {
	_      structs.HostLayout
	Width  uint32
	Height uint32
	Depth  uint32
}

// Offset2D is VkOffset2D.
type Offset2D struct {
	_ structs.HostLayout
	X int32
	Y int32
}

// Offset3D is VkOffset3D.
type Offset3D struct {
	_ structs.HostLayout
	X int32
	Y int32
	Z int32
}

// Rect2D is VkRect2D.
type Rect2D struct {
	_      structs.HostLayout
	Offset Offset2D
	Extent Extent2D
}

// Viewport is VkViewport.
type Viewport struct {
	_        structs.HostLayout
	X        float32
	Y        float32
	Width    float32
	Height   float32
	MinDepth float32
	MaxDepth float32
}

// ClearDepthStencilValue is VkClearDepthStencilValue.
type ClearDepthStencilValue struct {
	_       structs.HostLayout
	Depth   float32
	Stencil uint32
}

// ClearAttachment is VkClearAttachment.
type ClearAttachment struct {
	_               structs.HostLayout
	AspectMask      ImageAspectFlags
	ColorAttachment uint32
	ClearValue      ClearValue
}

// ClearRect is VkClearRect.
type ClearRect struct {
	_              structs.HostLayout
	Rect           Rect2D
	BaseArrayLayer uint32
	LayerCount     uint32
}

// ComponentMapping is VkComponentMapping.
type ComponentMapping struct {
	_ structs.HostLayout
	R ComponentSwizzle
	G ComponentSwizzle
	B ComponentSwizzle
	A ComponentSwizzle
}

// ImageSubresourceRange is VkImageSubresourceRange.
type ImageSubresourceRange struct {
	_              structs.HostLayout
	AspectMask     ImageAspectFlags
	BaseMipLevel   uint32
	LevelCount     uint32
	BaseArrayLayer uint32
	LayerCount     uint32
}

// ImageSubresourceLayers is VkImageSubresourceLayers.
type ImageSubresourceLayers struct {
	_              structs.HostLayout
	AspectMask     ImageAspectFlags
	MipLevel       uint32
	BaseArrayLayer uint32
	LayerCount     uint32
}

// ApplicationInfo is VkApplicationInfo.
type ApplicationInfo struct {
	_                  structs.HostLayout
	SType              StructureType
	PNext              unsafe.Pointer
	PApplicationName   *byte
	ApplicationVersion uint32
	PEngineName        *byte
	EngineVersion      uint32
	ApiVersion         uint32
}

// InstanceCreateInfo is VkInstanceCreateInfo.
type InstanceCreateInfo struct {
	_                       structs.HostLayout
	SType                   StructureType
	PNext                   unsafe.Pointer
	Flags                   uint32
	PApplicationInfo        *ApplicationInfo
	EnabledLayerCount       uint32
	PpEnabledLayerNames     **byte
	EnabledExtensionCount   uint32
	PpEnabledExtensionNames **byte
}

// ExtensionProperties is VkExtensionProperties.
type ExtensionProperties struct {
	_             structs.HostLayout
	ExtensionName [MaxExtensionNameSize]byte
	SpecVersion   uint32
}

// LayerProperties is VkLayerProperties.
type LayerProperties struct {
	_                     structs.HostLayout
	LayerName             [MaxExtensionNameSize]byte
	SpecVersion           uint32
	ImplementationVersion uint32
	Description           [MaxDescriptionSize]byte
}

// PhysicalDeviceLimits is VkPhysicalDeviceLimits.
type PhysicalDeviceLimits struct {
	_                                               structs.HostLayout
	MaxImageDimension1D                             uint32
	MaxImageDimension2D                             uint32
	MaxImageDimension3D                             uint32
	MaxImageDimensionCube                           uint32
	MaxImageArrayLayers                             uint32
	MaxTexelBufferElements                          uint32
	MaxUniformBufferRange                           uint32
	MaxStorageBufferRange                           uint32
	MaxPushConstantsSize                            uint32
	MaxMemoryAllocationCount                        uint32
	MaxSamplerAllocationCount                       uint32
	BufferImageGranularity                          uint64
	SparseAddressSpaceSize                          uint64
	MaxBoundDescriptorSets                          uint32
	MaxPerStageDescriptorSamplers                   uint32
	MaxPerStageDescriptorUniformBuffers             uint32
	MaxPerStageDescriptorStorageBuffers             uint32
	MaxPerStageDescriptorSampledImages              uint32
	MaxPerStageDescriptorStorageImages              uint32
	MaxPerStageDescriptorInputAttachments           uint32
	MaxPerStageResources                            uint32
	MaxDescriptorSetSamplers                        uint32
	MaxDescriptorSetUniformBuffers                  uint32
	MaxDescriptorSetUniformBuffersDynamic           uint32
	MaxDescriptorSetStorageBuffers                  uint32
	MaxDescriptorSetStorageBuffersDynamic           uint32
	MaxDescriptorSetSampledImages                   uint32
	MaxDescriptorSetStorageImages                   uint32
	MaxDescriptorSetInputAttachments                uint32
	MaxVertexInputAttributes                        uint32
	MaxVertexInputBindings                          uint32
	MaxVertexInputAttributeOffset                   uint32
	MaxVertexInputBindingStride                     uint32
	MaxVertexOutputComponents                       uint32
	MaxTessellationGenerationLevel                  uint32
	MaxTessellationPatchSize                        uint32
	MaxTessellationControlPerVertexInputComponents  uint32
	MaxTessellationControlPerVertexOutputComponents uint32
	MaxTessellationControlPerPatchOutputComponents  uint32
	MaxTessellationControlTotalOutputComponents     uint32
	MaxTessellationEvaluationInputComponents        uint32
	MaxTessellationEvaluationOutputComponents       uint32
	MaxGeometryShaderInvocations                    uint32
	MaxGeometryInputComponents                      uint32
	MaxGeometryOutputComponents                     uint32
	MaxGeometryOutputVertices                       uint32
	MaxGeometryTotalOutputComponents                uint32
	MaxFragmentInputComponents                      uint32
	MaxFragmentOutputAttachments                    uint32
	MaxFragmentDualSrcAttachments                   uint32
	MaxFragmentCombinedOutputResources              uint32
	MaxComputeSharedMemorySize                      uint32
	MaxComputeWorkGroupCount                        [3]uint32
	MaxComputeWorkGroupInvocations                  uint32
	MaxComputeWorkGroupSize                         [3]uint32
	SubPixelPrecisionBits                           uint32
	SubTexelPrecisionBits                           uint32
	MipmapPrecisionBits                             uint32
	MaxDrawIndexedIndexValue                        uint32
	MaxDrawIndirectCount                            uint32
	MaxSamplerLodBias                               float32
	MaxSamplerAnisotropy                            float32
	MaxViewports                                    uint32
	MaxViewportDimensions                           [2]uint32
	ViewportBoundsRange                             [2]float32
	ViewportSubPixelBits                            uint32
	MinMemoryMapAlignment                           uintptr
	MinTexelBufferOffsetAlignment                   uint64
	MinUniformBufferOffsetAlignment                 uint64
	MinStorageBufferOffsetAlignment                 uint64
	MinTexelOffset                                  int32
	MaxTexelOffset                                  uint32
	MinTexelGatherOffset                            int32
	MaxTexelGatherOffset                            uint32
	MinInterpolationOffset                          float32
	MaxInterpolationOffset                          float32
	SubPixelInterpolationOffsetBits                 uint32
	MaxFramebufferWidth                             uint32
	MaxFramebufferHeight                            uint32
	MaxFramebufferLayers                            uint32
	FramebufferColorSampleCounts                    SampleCountFlags
	FramebufferDepthSampleCounts                    SampleCountFlags
	FramebufferStencilSampleCounts                  SampleCountFlags
	FramebufferNoAttachmentsSampleCounts            SampleCountFlags
	MaxColorAttachments                             uint32
	SampledImageColorSampleCounts                   SampleCountFlags
	SampledImageIntegerSampleCounts                 SampleCountFlags
	SampledImageDepthSampleCounts                   SampleCountFlags
	SampledImageStencilSampleCounts                 SampleCountFlags
	StorageImageSampleCounts                        SampleCountFlags
	MaxSampleMaskWords                              uint32
	TimestampComputeAndGraphics                     Bool32
	TimestampPeriod                                 float32
	MaxClipDistances                                uint32
	MaxCullDistances                                uint32
	MaxCombinedClipAndCullDistances                 uint32
	DiscreteQueuePriorities                         uint32
	PointSizeRange                                  [2]float32
	LineWidthRange                                  [2]float32
	PointSizeGranularity                            float32
	LineWidthGranularity                            float32
	StrictLines                                     Bool32
	StandardSampleLocations                         Bool32
	OptimalBufferCopyOffsetAlignment                uint64
	OptimalBufferCopyRowPitchAlignment              uint64
	NonCoherentAtomSize                             uint64
}

// PhysicalDeviceSparseProperties is VkPhysicalDeviceSparseProperties.
type PhysicalDeviceSparseProperties struct {
	_                                        structs.HostLayout
	ResidencyStandard2DBlockShape            Bool32
	ResidencyStandard2DMultisampleBlockShape Bool32
	ResidencyStandard3DBlockShape            Bool32
	ResidencyAlignedMipSize                  Bool32
	ResidencyNonResidentStrict               Bool32
}

// PhysicalDeviceProperties is VkPhysicalDeviceProperties.
type PhysicalDeviceProperties struct {
	_                 structs.HostLayout
	ApiVersion        uint32
	DriverVersion     uint32
	VendorID          uint32
	DeviceID          uint32
	DeviceType        PhysicalDeviceType
	DeviceName        [MaxPhysicalDeviceNameSize]byte
	PipelineCacheUUID [UUIDSize]uint8
	Limits            PhysicalDeviceLimits
	SparseProperties  PhysicalDeviceSparseProperties
}

// PhysicalDeviceProperties2 is VkPhysicalDeviceProperties2.
type PhysicalDeviceProperties2 struct {
	_          structs.HostLayout
	SType      StructureType
	PNext      unsafe.Pointer
	Properties PhysicalDeviceProperties
}

// PhysicalDeviceDrmPropertiesEXT is VkPhysicalDeviceDrmPropertiesEXT.
type PhysicalDeviceDrmPropertiesEXT struct {
	_            structs.HostLayout
	SType        StructureType
	PNext        unsafe.Pointer
	HasPrimary   Bool32
	HasRender    Bool32
	PrimaryMajor int64
	PrimaryMinor int64
	RenderMajor  int64
	RenderMinor  int64
}

// PhysicalDeviceFeatures is VkPhysicalDeviceFeatures.
type PhysicalDeviceFeatures struct {
	_                                       structs.HostLayout
	RobustBufferAccess                      Bool32
	FullDrawIndexUint32                     Bool32
	ImageCubeArray                          Bool32
	IndependentBlend                        Bool32
	GeometryShader                          Bool32
	TessellationShader                      Bool32
	SampleRateShading                       Bool32
	DualSrcBlend                            Bool32
	LogicOp                                 Bool32
	MultiDrawIndirect                       Bool32
	DrawIndirectFirstInstance               Bool32
	DepthClamp                              Bool32
	DepthBiasClamp                          Bool32
	FillModeNonSolid                        Bool32
	DepthBounds                             Bool32
	WideLines                               Bool32
	LargePoints                             Bool32
	AlphaToOne                              Bool32
	MultiViewport                           Bool32
	SamplerAnisotropy                       Bool32
	TextureCompressionETC2                  Bool32
	TextureCompressionASTC_LDR              Bool32
	TextureCompressionBC                    Bool32
	OcclusionQueryPrecise                   Bool32
	PipelineStatisticsQuery                 Bool32
	VertexPipelineStoresAndAtomics          Bool32
	FragmentStoresAndAtomics                Bool32
	ShaderTessellationAndGeometryPointSize  Bool32
	ShaderImageGatherExtended               Bool32
	ShaderStorageImageExtendedFormats       Bool32
	ShaderStorageImageMultisample           Bool32
	ShaderStorageImageReadWithoutFormat     Bool32
	ShaderStorageImageWriteWithoutFormat    Bool32
	ShaderUniformBufferArrayDynamicIndexing Bool32
	ShaderSampledImageArrayDynamicIndexing  Bool32
	ShaderStorageBufferArrayDynamicIndexing Bool32
	ShaderStorageImageArrayDynamicIndexing  Bool32
	ShaderClipDistance                      Bool32
	ShaderCullDistance                      Bool32
	ShaderFloat64                           Bool32
	ShaderInt64                             Bool32
	ShaderInt16                             Bool32
	ShaderResourceResidency                 Bool32
	ShaderResourceMinLod                    Bool32
	SparseBinding                           Bool32
	SparseResidencyBuffer                   Bool32
	SparseResidencyImage2D                  Bool32
	SparseResidencyImage3D                  Bool32
	SparseResidency2Samples                 Bool32
	SparseResidency4Samples                 Bool32
	SparseResidency8Samples                 Bool32
	SparseResidency16Samples                Bool32
	SparseResidencyAliased                  Bool32
	VariableMultisampleRate                 Bool32
	InheritedQueries                        Bool32
}

// QueueFamilyProperties is VkQueueFamilyProperties.
type QueueFamilyProperties struct {
	_                           structs.HostLayout
	QueueFlags                  QueueFlags
	QueueCount                  uint32
	TimestampValidBits          uint32
	MinImageTransferGranularity Extent3D
}

// MemoryType is VkMemoryType.
type MemoryType struct {
	_             structs.HostLayout
	PropertyFlags MemoryPropertyFlags
	HeapIndex     uint32
}

// MemoryHeap is VkMemoryHeap.
type MemoryHeap struct {
	_     structs.HostLayout
	Size  uint64
	Flags MemoryHeapFlags
}

// PhysicalDeviceMemoryProperties is VkPhysicalDeviceMemoryProperties.
type PhysicalDeviceMemoryProperties struct {
	_               structs.HostLayout
	MemoryTypeCount uint32
	MemoryTypes     [MaxMemoryTypes]MemoryType
	MemoryHeapCount uint32
	MemoryHeaps     [MaxMemoryHeaps]MemoryHeap
}

// FormatProperties is VkFormatProperties.
type FormatProperties struct {
	_                     structs.HostLayout
	LinearTilingFeatures  FormatFeatureFlags
	OptimalTilingFeatures FormatFeatureFlags
	BufferFeatures        FormatFeatureFlags
}

// ImageFormatProperties is VkImageFormatProperties.
type ImageFormatProperties struct {
	_               structs.HostLayout
	MaxExtent       Extent3D
	MaxMipLevels    uint32
	MaxArrayLayers  uint32
	SampleCounts    SampleCountFlags
	MaxResourceSize uint64
}

// DeviceQueueCreateInfo is VkDeviceQueueCreateInfo.
type DeviceQueueCreateInfo struct {
	_                structs.HostLayout
	SType            StructureType
	PNext            unsafe.Pointer
	Flags            uint32
	QueueFamilyIndex uint32
	QueueCount       uint32
	PQueuePriorities *float32
}

// DeviceCreateInfo is VkDeviceCreateInfo.
type DeviceCreateInfo struct {
	_                       structs.HostLayout
	SType                   StructureType
	PNext                   unsafe.Pointer
	Flags                   uint32
	QueueCreateInfoCount    uint32
	PQueueCreateInfos       *DeviceQueueCreateInfo
	EnabledLayerCount       uint32
	PpEnabledLayerNames     **byte
	EnabledExtensionCount   uint32
	PpEnabledExtensionNames **byte
	PEnabledFeatures        *PhysicalDeviceFeatures
}

// CommandPoolCreateInfo is VkCommandPoolCreateInfo.
type CommandPoolCreateInfo struct {
	_                structs.HostLayout
	SType            StructureType
	PNext            unsafe.Pointer
	Flags            CommandPoolCreateFlags
	QueueFamilyIndex uint32
}

// CommandBufferAllocateInfo is VkCommandBufferAllocateInfo.
type CommandBufferAllocateInfo struct {
	_                  structs.HostLayout
	SType              StructureType
	PNext              unsafe.Pointer
	CommandPool        CommandPool
	Level              CommandBufferLevel
	CommandBufferCount uint32
}

// CommandBufferBeginInfo is VkCommandBufferBeginInfo.
type CommandBufferBeginInfo struct {
	_                structs.HostLayout
	SType            StructureType
	PNext            unsafe.Pointer
	Flags            CommandBufferUsageFlags
	PInheritanceInfo unsafe.Pointer // *VkCommandBufferInheritanceInfo, for secondary command buffers
}

// BufferCreateInfo is VkBufferCreateInfo.
type BufferCreateInfo struct {
	_                     structs.HostLayout
	SType                 StructureType
	PNext                 unsafe.Pointer
	Flags                 uint32
	Size                  uint64
	Usage                 BufferUsageFlags
	SharingMode           SharingMode
	QueueFamilyIndexCount uint32
	PQueueFamilyIndices   *uint32
}

// ImageCreateInfo is VkImageCreateInfo.
type ImageCreateInfo struct {
	_                     structs.HostLayout
	SType                 StructureType
	PNext                 unsafe.Pointer
	Flags                 ImageCreateFlags
	ImageType             ImageType
	Format                Format
	Extent                Extent3D
	MipLevels             uint32
	ArrayLayers           uint32
	Samples               SampleCountFlags
	Tiling                ImageTiling
	Usage                 ImageUsageFlags
	SharingMode           SharingMode
	QueueFamilyIndexCount uint32
	PQueueFamilyIndices   *uint32
	InitialLayout         ImageLayout
}

// ImageViewCreateInfo is VkImageViewCreateInfo.
type ImageViewCreateInfo struct {
	_                structs.HostLayout
	SType            StructureType
	PNext            unsafe.Pointer
	Flags            uint32
	Image            Image
	ViewType         ImageViewType
	Format           Format
	Components       ComponentMapping
	SubresourceRange ImageSubresourceRange
}

// SamplerCreateInfo is VkSamplerCreateInfo.
type SamplerCreateInfo struct {
	_                       structs.HostLayout
	SType                   StructureType
	PNext                   unsafe.Pointer
	Flags                   uint32
	MagFilter               Filter
	MinFilter               Filter
	MipmapMode              SamplerMipmapMode
	AddressModeU            SamplerAddressMode
	AddressModeV            SamplerAddressMode
	AddressModeW            SamplerAddressMode
	MipLodBias              float32
	AnisotropyEnable        Bool32
	MaxAnisotropy           float32
	CompareEnable           Bool32
	CompareOp               CompareOp
	MinLod                  float32
	MaxLod                  float32
	BorderColor             BorderColor
	UnnormalizedCoordinates Bool32
}

// MemoryRequirements is VkMemoryRequirements.
type MemoryRequirements struct {
	_              structs.HostLayout
	Size           uint64
	Alignment      uint64
	MemoryTypeBits uint32
}

// MemoryAllocateInfo is VkMemoryAllocateInfo.
type MemoryAllocateInfo struct {
	_               structs.HostLayout
	SType           StructureType
	PNext           unsafe.Pointer
	AllocationSize  uint64
	MemoryTypeIndex uint32
}

// MappedMemoryRange is VkMappedMemoryRange.
type MappedMemoryRange struct {
	_      structs.HostLayout
	SType  StructureType
	PNext  unsafe.Pointer
	Memory DeviceMemory
	Offset uint64
	Size   uint64
}

// AttachmentDescription is VkAttachmentDescription.
type AttachmentDescription struct {
	_              structs.HostLayout
	Flags          uint32
	Format         Format
	Samples        SampleCountFlags
	LoadOp         AttachmentLoadOp
	StoreOp        AttachmentStoreOp
	StencilLoadOp  AttachmentLoadOp
	StencilStoreOp AttachmentStoreOp
	InitialLayout  ImageLayout
	FinalLayout    ImageLayout
}

// AttachmentReference is VkAttachmentReference.
type AttachmentReference struct {
	_          structs.HostLayout
	Attachment uint32
	Layout     ImageLayout
}

// SubpassDescription is VkSubpassDescription.
type SubpassDescription struct {
	_                       structs.HostLayout
	Flags                   uint32
	PipelineBindPoint       PipelineBindPoint
	InputAttachmentCount    uint32
	PInputAttachments       *AttachmentReference
	ColorAttachmentCount    uint32
	PColorAttachments       *AttachmentReference
	PResolveAttachments     *AttachmentReference
	PDepthStencilAttachment *AttachmentReference
	PreserveAttachmentCount uint32
	PPreserveAttachments    *uint32
}

// SubpassDependency is VkSubpassDependency.
type SubpassDependency struct {
	_               structs.HostLayout
	SrcSubpass      uint32
	DstSubpass      uint32
	SrcStageMask    PipelineStageFlags
	DstStageMask    PipelineStageFlags
	SrcAccessMask   AccessFlags
	DstAccessMask   AccessFlags
	DependencyFlags DependencyFlags
}

// RenderPassCreateInfo is VkRenderPassCreateInfo.
type RenderPassCreateInfo struct {
	_               structs.HostLayout
	SType           StructureType
	PNext           unsafe.Pointer
	Flags           uint32
	AttachmentCount uint32
	PAttachments    *AttachmentDescription
	SubpassCount    uint32
	PSubpasses      *SubpassDescription
	DependencyCount uint32
	PDependencies   *SubpassDependency
}

// FramebufferCreateInfo is VkFramebufferCreateInfo.
type FramebufferCreateInfo struct {
	_               structs.HostLayout
	SType           StructureType
	PNext           unsafe.Pointer
	Flags           uint32
	RenderPass      RenderPass
	AttachmentCount uint32
	PAttachments    *ImageView
	Width           uint32
	Height          uint32
	Layers          uint32
}

// RenderPassBeginInfo is VkRenderPassBeginInfo.
type RenderPassBeginInfo struct {
	_               structs.HostLayout
	SType           StructureType
	PNext           unsafe.Pointer
	RenderPass      RenderPass
	Framebuffer     Framebuffer
	RenderArea      Rect2D
	ClearValueCount uint32
	PClearValues    *ClearValue
}

// ShaderModuleCreateInfo is VkShaderModuleCreateInfo.
type ShaderModuleCreateInfo struct {
	_        structs.HostLayout
	SType    StructureType
	PNext    unsafe.Pointer
	Flags    uint32
	CodeSize uintptr
	PCode    *uint32
}

// DescriptorSetLayoutBinding is VkDescriptorSetLayoutBinding.
type DescriptorSetLayoutBinding struct {
	_                  structs.HostLayout
	Binding            uint32
	DescriptorType     DescriptorType
	DescriptorCount    uint32
	StageFlags         ShaderStageFlags
	PImmutableSamplers *Sampler
}

// DescriptorSetLayoutCreateInfo is VkDescriptorSetLayoutCreateInfo.
type DescriptorSetLayoutCreateInfo struct {
	_            structs.HostLayout
	SType        StructureType
	PNext        unsafe.Pointer
	Flags        uint32
	BindingCount uint32
	PBindings    *DescriptorSetLayoutBinding
}

// DescriptorPoolSize is VkDescriptorPoolSize.
type DescriptorPoolSize struct {
	_               structs.HostLayout
	Type            DescriptorType
	DescriptorCount uint32
}

// DescriptorPoolCreateInfo is VkDescriptorPoolCreateInfo.
type DescriptorPoolCreateInfo struct {
	_             structs.HostLayout
	SType         StructureType
	PNext         unsafe.Pointer
	Flags         DescriptorPoolCreateFlags
	MaxSets       uint32
	PoolSizeCount uint32
	PPoolSizes    *DescriptorPoolSize
}

// DescriptorSetAllocateInfo is VkDescriptorSetAllocateInfo.
type DescriptorSetAllocateInfo struct {
	_                  structs.HostLayout
	SType              StructureType
	PNext              unsafe.Pointer
	DescriptorPool     DescriptorPool
	DescriptorSetCount uint32
	PSetLayouts        *DescriptorSetLayout
}

// DescriptorImageInfo is VkDescriptorImageInfo.
type DescriptorImageInfo struct {
	_           structs.HostLayout
	Sampler     Sampler
	ImageView   ImageView
	ImageLayout ImageLayout
}

// DescriptorBufferInfo is VkDescriptorBufferInfo.
type DescriptorBufferInfo struct {
	_      structs.HostLayout
	Buffer Buffer
	Offset uint64
	Range  uint64
}

// WriteDescriptorSet is VkWriteDescriptorSet.
type WriteDescriptorSet struct {
	_                structs.HostLayout
	SType            StructureType
	PNext            unsafe.Pointer
	DstSet           DescriptorSet
	DstBinding       uint32
	DstArrayElement  uint32
	DescriptorCount  uint32
	DescriptorType   DescriptorType
	PImageInfo       *DescriptorImageInfo
	PBufferInfo      *DescriptorBufferInfo
	PTexelBufferView *BufferView
}

// CopyDescriptorSet is VkCopyDescriptorSet.
type CopyDescriptorSet struct {
	_               structs.HostLayout
	SType           StructureType
	PNext           unsafe.Pointer
	SrcSet          DescriptorSet
	SrcBinding      uint32
	SrcArrayElement uint32
	DstSet          DescriptorSet
	DstBinding      uint32
	DstArrayElement uint32
	DescriptorCount uint32
}

// PushConstantRange is VkPushConstantRange.
type PushConstantRange struct {
	_          structs.HostLayout
	StageFlags ShaderStageFlags
	Offset     uint32
	Size       uint32
}

// PipelineLayoutCreateInfo is VkPipelineLayoutCreateInfo.
type PipelineLayoutCreateInfo struct {
	_                      structs.HostLayout
	SType                  StructureType
	PNext                  unsafe.Pointer
	Flags                  uint32
	SetLayoutCount         uint32
	PSetLayouts            *DescriptorSetLayout
	PushConstantRangeCount uint32
	PPushConstantRanges    *PushConstantRange
}

// SpecializationMapEntry is VkSpecializationMapEntry.
type SpecializationMapEntry struct {
	_          structs.HostLayout
	ConstantID uint32
	Offset     uint32
	Size       uintptr
}

// SpecializationInfo is VkSpecializationInfo.
type SpecializationInfo struct {
	_             structs.HostLayout
	MapEntryCount uint32
	PMapEntries   *SpecializationMapEntry
	DataSize      uintptr
	PData         unsafe.Pointer
}

// PipelineShaderStageCreateInfo is VkPipelineShaderStageCreateInfo.
type PipelineShaderStageCreateInfo struct {
	_                   structs.HostLayout
	SType               StructureType
	PNext               unsafe.Pointer
	Flags               uint32
	Stage               ShaderStageFlags
	Module              ShaderModule
	PName               *byte
	PSpecializationInfo *SpecializationInfo
}

// VertexInputBindingDescription is VkVertexInputBindingDescription.
type VertexInputBindingDescription struct {
	_         structs.HostLayout
	Binding   uint32
	Stride    uint32
	InputRate VertexInputRate
}

// VertexInputAttributeDescription is VkVertexInputAttributeDescription.
type VertexInputAttributeDescription struct {
	_        structs.HostLayout
	Location uint32
	Binding  uint32
	Format   Format
	Offset   uint32
}

// PipelineVertexInputStateCreateInfo is VkPipelineVertexInputStateCreateInfo.
type PipelineVertexInputStateCreateInfo struct {
	_                               structs.HostLayout
	SType                           StructureType
	PNext                           unsafe.Pointer
	Flags                           uint32
	VertexBindingDescriptionCount   uint32
	PVertexBindingDescriptions      *VertexInputBindingDescription
	VertexAttributeDescriptionCount uint32
	PVertexAttributeDescriptions    *VertexInputAttributeDescription
}

// PipelineInputAssemblyStateCreateInfo is VkPipelineInputAssemblyStateCreateInfo.
type PipelineInputAssemblyStateCreateInfo struct {
	_                      structs.HostLayout
	SType                  StructureType
	PNext                  unsafe.Pointer
	Flags                  uint32
	Topology               PrimitiveTopology
	PrimitiveRestartEnable Bool32
}

// PipelineViewportStateCreateInfo is VkPipelineViewportStateCreateInfo.
type PipelineViewportStateCreateInfo struct {
	_             structs.HostLayout
	SType         StructureType
	PNext         unsafe.Pointer
	Flags         uint32
	ViewportCount uint32
	PViewports    *Viewport
	ScissorCount  uint32
	PScissors     *Rect2D
}

// PipelineRasterizationStateCreateInfo is VkPipelineRasterizationStateCreateInfo.
type PipelineRasterizationStateCreateInfo struct {
	_                       structs.HostLayout
	SType                   StructureType
	PNext                   unsafe.Pointer
	Flags                   uint32
	DepthClampEnable        Bool32
	RasterizerDiscardEnable Bool32
	PolygonMode             PolygonMode
	CullMode                CullModeFlags
	FrontFace               FrontFace
	DepthBiasEnable         Bool32
	DepthBiasConstantFactor float32
	DepthBiasClamp          float32
	DepthBiasSlopeFactor    float32
	LineWidth               float32
}

// PipelineMultisampleStateCreateInfo is VkPipelineMultisampleStateCreateInfo.
type PipelineMultisampleStateCreateInfo struct {
	_                     structs.HostLayout
	SType                 StructureType
	PNext                 unsafe.Pointer
	Flags                 uint32
	RasterizationSamples  SampleCountFlags
	SampleShadingEnable   Bool32
	MinSampleShading      float32
	PSampleMask           *uint32
	AlphaToCoverageEnable Bool32
	AlphaToOneEnable      Bool32
}

// StencilOpState is VkStencilOpState.
type StencilOpState struct {
	_           structs.HostLayout
	FailOp      StencilOp
	PassOp      StencilOp
	DepthFailOp StencilOp
	CompareOp   CompareOp
	CompareMask uint32
	WriteMask   uint32
	Reference   uint32
}

// PipelineDepthStencilStateCreateInfo is VkPipelineDepthStencilStateCreateInfo.
type PipelineDepthStencilStateCreateInfo struct {
	_                     structs.HostLayout
	SType                 StructureType
	PNext                 unsafe.Pointer
	Flags                 uint32
	DepthTestEnable       Bool32
	DepthWriteEnable      Bool32
	DepthCompareOp        CompareOp
	DepthBoundsTestEnable Bool32
	StencilTestEnable     Bool32
	Front                 StencilOpState
	Back                  StencilOpState
	MinDepthBounds        float32
	MaxDepthBounds        float32
}

// PipelineColorBlendAttachmentState is VkPipelineColorBlendAttachmentState.
type PipelineColorBlendAttachmentState struct {
	_                   structs.HostLayout
	BlendEnable         Bool32
	SrcColorBlendFactor BlendFactor
	DstColorBlendFactor BlendFactor
	ColorBlendOp        BlendOp
	SrcAlphaBlendFactor BlendFactor
	DstAlphaBlendFactor BlendFactor
	AlphaBlendOp        BlendOp
	ColorWriteMask      ColorComponentFlags
}

// PipelineColorBlendStateCreateInfo is VkPipelineColorBlendStateCreateInfo.
type PipelineColorBlendStateCreateInfo struct {
	_               structs.HostLayout
	SType           StructureType
	PNext           unsafe.Pointer
	Flags           uint32
	LogicOpEnable   Bool32
	LogicOp         LogicOp
	AttachmentCount uint32
	PAttachments    *PipelineColorBlendAttachmentState
	BlendConstants  [4]float32
}

// PipelineDynamicStateCreateInfo is VkPipelineDynamicStateCreateInfo.
type PipelineDynamicStateCreateInfo struct {
	_                 structs.HostLayout
	SType             StructureType
	PNext             unsafe.Pointer
	Flags             uint32
	DynamicStateCount uint32
	PDynamicStates    *DynamicState
}

// GraphicsPipelineCreateInfo is VkGraphicsPipelineCreateInfo.
type GraphicsPipelineCreateInfo struct {
	_                   structs.HostLayout
	SType               StructureType
	PNext               unsafe.Pointer
	Flags               uint32
	StageCount          uint32
	PStages             *PipelineShaderStageCreateInfo
	PVertexInputState   *PipelineVertexInputStateCreateInfo
	PInputAssemblyState *PipelineInputAssemblyStateCreateInfo
	PTessellationState  unsafe.Pointer // *VkPipelineTessellationStateCreateInfo
	PViewportState      *PipelineViewportStateCreateInfo
	PRasterizationState *PipelineRasterizationStateCreateInfo
	PMultisampleState   *PipelineMultisampleStateCreateInfo
	PDepthStencilState  *PipelineDepthStencilStateCreateInfo
	PColorBlendState    *PipelineColorBlendStateCreateInfo
	PDynamicState       *PipelineDynamicStateCreateInfo
	Layout              PipelineLayout
	RenderPass          RenderPass
	Subpass             uint32
	BasePipelineHandle  Pipeline
	BasePipelineIndex   int32
}

// FenceCreateInfo is VkFenceCreateInfo.
type FenceCreateInfo struct {
	_     structs.HostLayout
	SType StructureType
	PNext unsafe.Pointer
	Flags FenceCreateFlags
}

// SemaphoreCreateInfo is VkSemaphoreCreateInfo.
type SemaphoreCreateInfo struct {
	_     structs.HostLayout
	SType StructureType
	PNext unsafe.Pointer
	Flags uint32
}

// SubmitInfo is VkSubmitInfo.
type SubmitInfo struct {
	_                    structs.HostLayout
	SType                StructureType
	PNext                unsafe.Pointer
	WaitSemaphoreCount   uint32
	PWaitSemaphores      *Semaphore
	PWaitDstStageMask    *PipelineStageFlags
	CommandBufferCount   uint32
	PCommandBuffers      *CommandBuffer
	SignalSemaphoreCount uint32
	PSignalSemaphores    *Semaphore
}

// BufferCopy is VkBufferCopy.
type BufferCopy struct {
	_         structs.HostLayout
	SrcOffset uint64
	DstOffset uint64
	Size      uint64
}

// BufferImageCopy is VkBufferImageCopy.
type BufferImageCopy struct {
	_                 structs.HostLayout
	BufferOffset      uint64
	BufferRowLength   uint32
	BufferImageHeight uint32
	ImageSubresource  ImageSubresourceLayers
	ImageOffset       Offset3D
	ImageExtent       Extent3D
}

// ImageCopy is VkImageCopy.
type ImageCopy struct {
	_              structs.HostLayout
	SrcSubresource ImageSubresourceLayers
	SrcOffset      Offset3D
	DstSubresource ImageSubresourceLayers
	DstOffset      Offset3D
	Extent         Extent3D
}

// ImageBlit is VkImageBlit.
type ImageBlit struct {
	_              structs.HostLayout
	SrcSubresource ImageSubresourceLayers
	SrcOffsets     [2]Offset3D
	DstSubresource ImageSubresourceLayers
	DstOffsets     [2]Offset3D
}

// MemoryBarrier is VkMemoryBarrier.
type MemoryBarrier struct {
	_             structs.HostLayout
	SType         StructureType
	PNext         unsafe.Pointer
	SrcAccessMask AccessFlags
	DstAccessMask AccessFlags
}

// BufferMemoryBarrier is VkBufferMemoryBarrier.
type BufferMemoryBarrier struct {
	_                   structs.HostLayout
	SType               StructureType
	PNext               unsafe.Pointer
	SrcAccessMask       AccessFlags
	DstAccessMask       AccessFlags
	SrcQueueFamilyIndex uint32
	DstQueueFamilyIndex uint32
	Buffer              Buffer
	Offset              uint64
	Size                uint64
}

// ImageMemoryBarrier is VkImageMemoryBarrier.
type ImageMemoryBarrier struct {
	_                   structs.HostLayout
	SType               StructureType
	PNext               unsafe.Pointer
	SrcAccessMask       AccessFlags
	DstAccessMask       AccessFlags
	OldLayout           ImageLayout
	NewLayout           ImageLayout
	SrcQueueFamilyIndex uint32
	DstQueueFamilyIndex uint32
	Image               Image
	SubresourceRange    ImageSubresourceRange
}

// DebugUtilsLabelEXT is VkDebugUtilsLabelEXT.
type DebugUtilsLabelEXT struct {
	_          structs.HostLayout
	SType      StructureType
	PNext      unsafe.Pointer
	PLabelName *byte
	Color      [4]float32
}

// DebugUtilsObjectNameInfoEXT is VkDebugUtilsObjectNameInfoEXT.
type DebugUtilsObjectNameInfoEXT struct {
	_            structs.HostLayout
	SType        StructureType
	PNext        unsafe.Pointer
	ObjectType   ObjectType
	ObjectHandle uint64
	PObjectName  *byte
}

// DebugUtilsMessengerCallbackDataEXT is VkDebugUtilsMessengerCallbackDataEXT.
type DebugUtilsMessengerCallbackDataEXT struct {
	_                structs.HostLayout
	SType            StructureType
	PNext            unsafe.Pointer
	Flags            uint32
	PMessageIdName   *byte
	MessageIdNumber  int32
	PMessage         *byte
	QueueLabelCount  uint32
	PQueueLabels     *DebugUtilsLabelEXT
	CmdBufLabelCount uint32
	PCmdBufLabels    *DebugUtilsLabelEXT
	ObjectCount      uint32
	PObjects         *DebugUtilsObjectNameInfoEXT
}

// DebugUtilsMessengerCreateInfoEXT is VkDebugUtilsMessengerCreateInfoEXT.
type DebugUtilsMessengerCreateInfoEXT struct {
	_               structs.HostLayout
	SType           StructureType
	PNext           unsafe.Pointer
	Flags           uint32
	MessageSeverity DebugUtilsMessageSeverityFlagsEXT
	MessageType     DebugUtilsMessageTypeFlagsEXT
	PfnUserCallback uintptr
	PUserData       unsafe.Pointer
}

// LayerSettingEXT is VkLayerSettingEXT.
type LayerSettingEXT struct {
	_            structs.HostLayout
	PLayerName   *byte
	PSettingName *byte
	Type         LayerSettingTypeEXT
	ValueCount   uint32
	PValues      unsafe.Pointer
}

// LayerSettingsCreateInfoEXT is VkLayerSettingsCreateInfoEXT.
type LayerSettingsCreateInfoEXT struct {
	_            structs.HostLayout
	SType        StructureType
	PNext        unsafe.Pointer
	SettingCount uint32
	PSettings    *LayerSettingEXT
}

// ClearColorValue is the union VkClearColorValue: the four components of a
// clear color as the bits of float32, int32 or uint32 values, whichever
// the format of the image takes. See [ClearColorFloat].
type ClearColorValue [4]uint32

// ClearValue is the union VkClearValue: a clear color, or a depth and a
// stencil value. See [ClearColor] and [ClearDepthStencil].
type ClearValue [4]uint32
