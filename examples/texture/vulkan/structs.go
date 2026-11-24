//go:build linux && (amd64 || arm64)

package vulkan

import "unsafe"

// The structs and constants of Vulkan the renderer uses, with the C names.
// Their layout was checked against vulkan_core.h 1.4.363 with a C compiler:
// TestLayout has what it said. The members the renderer never
// sets that point to other structs are unsafe.Pointer.

const (
	structureTypeApplicationInfo                             = 0          // VK_STRUCTURE_TYPE_APPLICATION_INFO
	structureTypeInstanceCreateInfo                          = 1          // VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO
	structureTypePhysicalDeviceProperties2                   = 1000059001 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_PROPERTIES_2
	structureTypePhysicalDeviceIdProperties                  = 1000071004 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_ID_PROPERTIES
	structureTypePhysicalDeviceDrmPropertiesEXT              = 1000353000 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_DRM_PROPERTIES_EXT
	structureTypeDeviceQueueCreateInfo                       = 2          // VK_STRUCTURE_TYPE_DEVICE_QUEUE_CREATE_INFO
	structureTypeDeviceCreateInfo                            = 3          // VK_STRUCTURE_TYPE_DEVICE_CREATE_INFO
	structureTypePhysicalDeviceDynamicRenderingFeatures      = 1000044003 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_DYNAMIC_RENDERING_FEATURES
	structureTypeFormatProperties2                           = 1000059002 // VK_STRUCTURE_TYPE_FORMAT_PROPERTIES_2
	structureTypeDrmFormatModifierPropertiesListEXT          = 1000158000 // VK_STRUCTURE_TYPE_DRM_FORMAT_MODIFIER_PROPERTIES_LIST_EXT
	structureTypePhysicalDeviceImageFormatInfo2              = 1000059004 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_IMAGE_FORMAT_INFO_2
	structureTypePhysicalDeviceExternalImageFormatInfo       = 1000071000 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_EXTERNAL_IMAGE_FORMAT_INFO
	structureTypePhysicalDeviceImageDrmFormatModifierInfoEXT = 1000158002 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_IMAGE_DRM_FORMAT_MODIFIER_INFO_EXT
	structureTypeImageFormatProperties2                      = 1000059003 // VK_STRUCTURE_TYPE_IMAGE_FORMAT_PROPERTIES_2
	structureTypeExternalImageFormatProperties               = 1000071001 // VK_STRUCTURE_TYPE_EXTERNAL_IMAGE_FORMAT_PROPERTIES
	structureTypePhysicalDeviceExternalSemaphoreInfo         = 1000076000 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_EXTERNAL_SEMAPHORE_INFO
	structureTypeExternalSemaphoreProperties                 = 1000076001 // VK_STRUCTURE_TYPE_EXTERNAL_SEMAPHORE_PROPERTIES
	structureTypeImageCreateInfo                             = 14         // VK_STRUCTURE_TYPE_IMAGE_CREATE_INFO
	structureTypeExternalMemoryImageCreateInfo               = 1000072001 // VK_STRUCTURE_TYPE_EXTERNAL_MEMORY_IMAGE_CREATE_INFO
	structureTypeImageDrmFormatModifierListCreateInfoEXT     = 1000158003 // VK_STRUCTURE_TYPE_IMAGE_DRM_FORMAT_MODIFIER_LIST_CREATE_INFO_EXT
	structureTypeImageDrmFormatModifierPropertiesEXT         = 1000158005 // VK_STRUCTURE_TYPE_IMAGE_DRM_FORMAT_MODIFIER_PROPERTIES_EXT
	structureTypeMemoryAllocateInfo                          = 5          // VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO
	structureTypeExportMemoryAllocateInfo                    = 1000072002 // VK_STRUCTURE_TYPE_EXPORT_MEMORY_ALLOCATE_INFO
	structureTypeMemoryDedicatedAllocateInfo                 = 1000127001 // VK_STRUCTURE_TYPE_MEMORY_DEDICATED_ALLOCATE_INFO
	structureTypeMemoryGetFdInfoKHR                          = 1000074002 // VK_STRUCTURE_TYPE_MEMORY_GET_FD_INFO_KHR
	structureTypeImageViewCreateInfo                         = 15         // VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO
	structureTypeShaderModuleCreateInfo                      = 16         // VK_STRUCTURE_TYPE_SHADER_MODULE_CREATE_INFO
	structureTypePipelineLayoutCreateInfo                    = 30         // VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO
	structureTypePipelineShaderStageCreateInfo               = 18         // VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO
	structureTypePipelineVertexInputStateCreateInfo          = 19         // VK_STRUCTURE_TYPE_PIPELINE_VERTEX_INPUT_STATE_CREATE_INFO
	structureTypePipelineInputAssemblyStateCreateInfo        = 20         // VK_STRUCTURE_TYPE_PIPELINE_INPUT_ASSEMBLY_STATE_CREATE_INFO
	structureTypePipelineViewportStateCreateInfo             = 22         // VK_STRUCTURE_TYPE_PIPELINE_VIEWPORT_STATE_CREATE_INFO
	structureTypePipelineRasterizationStateCreateInfo        = 23         // VK_STRUCTURE_TYPE_PIPELINE_RASTERIZATION_STATE_CREATE_INFO
	structureTypePipelineMultisampleStateCreateInfo          = 24         // VK_STRUCTURE_TYPE_PIPELINE_MULTISAMPLE_STATE_CREATE_INFO
	structureTypePipelineColorBlendStateCreateInfo           = 26         // VK_STRUCTURE_TYPE_PIPELINE_COLOR_BLEND_STATE_CREATE_INFO
	structureTypePipelineDynamicStateCreateInfo              = 27         // VK_STRUCTURE_TYPE_PIPELINE_DYNAMIC_STATE_CREATE_INFO
	structureTypePipelineRenderingCreateInfo                 = 1000044002 // VK_STRUCTURE_TYPE_PIPELINE_RENDERING_CREATE_INFO
	structureTypeGraphicsPipelineCreateInfo                  = 28         // VK_STRUCTURE_TYPE_GRAPHICS_PIPELINE_CREATE_INFO
	structureTypeCommandPoolCreateInfo                       = 39         // VK_STRUCTURE_TYPE_COMMAND_POOL_CREATE_INFO
	structureTypeCommandBufferAllocateInfo                   = 40         // VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO
	structureTypeCommandBufferBeginInfo                      = 42         // VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO
	structureTypeImageMemoryBarrier                          = 45         // VK_STRUCTURE_TYPE_IMAGE_MEMORY_BARRIER
	structureTypeRenderingAttachmentInfo                     = 1000044001 // VK_STRUCTURE_TYPE_RENDERING_ATTACHMENT_INFO
	structureTypeRenderingInfo                               = 1000044000 // VK_STRUCTURE_TYPE_RENDERING_INFO
	structureTypeSubmitInfo                                  = 4          // VK_STRUCTURE_TYPE_SUBMIT_INFO
	structureTypeFenceCreateInfo                             = 8          // VK_STRUCTURE_TYPE_FENCE_CREATE_INFO
	structureTypeSemaphoreCreateInfo                         = 9          // VK_STRUCTURE_TYPE_SEMAPHORE_CREATE_INFO
	structureTypeExportSemaphoreCreateInfo                   = 1000077000 // VK_STRUCTURE_TYPE_EXPORT_SEMAPHORE_CREATE_INFO
	structureTypeSemaphoreGetFdInfoKHR                       = 1000079001 // VK_STRUCTURE_TYPE_SEMAPHORE_GET_FD_INFO_KHR
	structureTypeDebugUtilsMessengerCreateInfoEXT            = 1000128004 // VK_STRUCTURE_TYPE_DEBUG_UTILS_MESSENGER_CREATE_INFO_EXT
	errorDeviceLost                                          = -4         // VK_ERROR_DEVICE_LOST
	incomplete                                               = 5          // VK_INCOMPLETE
	timeout                                                  = 2          // VK_TIMEOUT
	physicalDeviceTypeCpu                                    = 4          // VK_PHYSICAL_DEVICE_TYPE_CPU
	queueGraphicsBit                                         = 0x1        // VK_QUEUE_GRAPHICS_BIT
	queueFamilyIgnored                                       = 0xffffffff // VK_QUEUE_FAMILY_IGNORED
	queueFamilyForeignEXT                                    = 0xfffffffd // VK_QUEUE_FAMILY_FOREIGN_EXT
	formatB8g8r8a8Unorm                                      = 44         // VK_FORMAT_B8G8R8A8_UNORM
	formatR8g8b8a8Unorm                                      = 37         // VK_FORMAT_R8G8B8A8_UNORM
	formatFeatureColorAttachmentBit                          = 0x80       // VK_FORMAT_FEATURE_COLOR_ATTACHMENT_BIT
	formatFeatureTransferDstBit                              = 0x8000     // VK_FORMAT_FEATURE_TRANSFER_DST_BIT
	imageType2D                                              = 1          // VK_IMAGE_TYPE_2D
	imageViewType2D                                          = 1          // VK_IMAGE_VIEW_TYPE_2D
	imageTilingOptimal                                       = 0          // VK_IMAGE_TILING_OPTIMAL
	imageTilingDrmFormatModifierEXT                          = 1000158000 // VK_IMAGE_TILING_DRM_FORMAT_MODIFIER_EXT
	imageUsageColorAttachmentBit                             = 0x10       // VK_IMAGE_USAGE_COLOR_ATTACHMENT_BIT
	imageUsageTransferSrcBit                                 = 0x1        // VK_IMAGE_USAGE_TRANSFER_SRC_BIT
	imageUsageTransferDstBit                                 = 0x2        // VK_IMAGE_USAGE_TRANSFER_DST_BIT
	imageLayoutUndefined                                     = 0          // VK_IMAGE_LAYOUT_UNDEFINED
	imageLayoutGeneral                                       = 1          // VK_IMAGE_LAYOUT_GENERAL
	imageLayoutColorAttachmentOptimal                        = 2          // VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL
	imageLayoutTransferSrcOptimal                            = 6          // VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL
	imageLayoutTransferDstOptimal                            = 7          // VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL
	imageAspectColorBit                                      = 0x1        // VK_IMAGE_ASPECT_COLOR_BIT
	imageAspectMemoryPlane0BitEXT                            = 0x80       // VK_IMAGE_ASPECT_MEMORY_PLANE_0_BIT_EXT
	sampleCount1Bit                                          = 0x1        // VK_SAMPLE_COUNT_1_BIT
	sharingModeExclusive                                     = 0          // VK_SHARING_MODE_EXCLUSIVE
	externalMemoryHandleTypeDmaBufBitEXT                     = 0x200      // VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT_EXT
	externalMemoryFeatureExportableBit                       = 0x2        // VK_EXTERNAL_MEMORY_FEATURE_EXPORTABLE_BIT
	externalSemaphoreHandleTypeSyncFdBit                     = 0x10       // VK_EXTERNAL_SEMAPHORE_HANDLE_TYPE_SYNC_FD_BIT
	externalSemaphoreFeatureExportableBit                    = 0x1        // VK_EXTERNAL_SEMAPHORE_FEATURE_EXPORTABLE_BIT
	memoryPropertyDeviceLocalBit                             = 0x1        // VK_MEMORY_PROPERTY_DEVICE_LOCAL_BIT
	memoryPropertyHostCachedBit                              = 0x8        // VK_MEMORY_PROPERTY_HOST_CACHED_BIT
	memoryPropertyProtectedBit                               = 0x20       // VK_MEMORY_PROPERTY_PROTECTED_BIT
	memoryPropertyLazilyAllocatedBit                         = 0x10       // VK_MEMORY_PROPERTY_LAZILY_ALLOCATED_BIT
	shaderStageVertexBit                                     = 0x1        // VK_SHADER_STAGE_VERTEX_BIT
	shaderStageFragmentBit                                   = 0x10       // VK_SHADER_STAGE_FRAGMENT_BIT
	primitiveTopologyTriangleList                            = 3          // VK_PRIMITIVE_TOPOLOGY_TRIANGLE_LIST
	polygonModeFill                                          = 0          // VK_POLYGON_MODE_FILL
	cullModeNone                                             = 0          // VK_CULL_MODE_NONE
	dynamicStateViewport                                     = 0          // VK_DYNAMIC_STATE_VIEWPORT
	dynamicStateScissor                                      = 1          // VK_DYNAMIC_STATE_SCISSOR
	pipelineBindPointGraphics                                = 0          // VK_PIPELINE_BIND_POINT_GRAPHICS
	attachmentLoadOpDontCare                                 = 2          // VK_ATTACHMENT_LOAD_OP_DONT_CARE
	attachmentStoreOpStore                                   = 0          // VK_ATTACHMENT_STORE_OP_STORE
	pipelineStageColorAttachmentOutputBit                    = 0x400      // VK_PIPELINE_STAGE_COLOR_ATTACHMENT_OUTPUT_BIT
	pipelineStageTransferBit                                 = 0x1000     // VK_PIPELINE_STAGE_TRANSFER_BIT
	pipelineStageBottomOfPipeBit                             = 0x2000     // VK_PIPELINE_STAGE_BOTTOM_OF_PIPE_BIT
	pipelineStageTopOfPipeBit                                = 0x1        // VK_PIPELINE_STAGE_TOP_OF_PIPE_BIT
	accessColorAttachmentWriteBit                            = 0x100      // VK_ACCESS_COLOR_ATTACHMENT_WRITE_BIT
	accessTransferReadBit                                    = 0x800      // VK_ACCESS_TRANSFER_READ_BIT
	accessTransferWriteBit                                   = 0x1000     // VK_ACCESS_TRANSFER_WRITE_BIT
	commandBufferUsageOneTimeSubmitBit                       = 0x1        // VK_COMMAND_BUFFER_USAGE_ONE_TIME_SUBMIT_BIT
	commandBufferLevelPrimary                                = 0          // VK_COMMAND_BUFFER_LEVEL_PRIMARY
	commandPoolCreateTransientBit                            = 0x1        // VK_COMMAND_POOL_CREATE_TRANSIENT_BIT
	debugUtilsMessageSeverityWarningBitEXT                   = 0x100      // VK_DEBUG_UTILS_MESSAGE_SEVERITY_WARNING_BIT_EXT
	debugUtilsMessageSeverityErrorBitEXT                     = 0x1000     // VK_DEBUG_UTILS_MESSAGE_SEVERITY_ERROR_BIT_EXT
	debugUtilsMessageTypeGeneralBitEXT                       = 0x1        // VK_DEBUG_UTILS_MESSAGE_TYPE_GENERAL_BIT_EXT
	debugUtilsMessageTypeValidationBitEXT                    = 0x2        // VK_DEBUG_UTILS_MESSAGE_TYPE_VALIDATION_BIT_EXT
)

type applicationInfo struct {
	SType              uint32
	PNext              unsafe.Pointer
	PApplicationName   *byte
	ApplicationVersion uint32
	PEngineName        *byte
	EngineVersion      uint32
	ApiVersion         uint32
}

type instanceCreateInfo struct {
	SType                   uint32
	PNext                   unsafe.Pointer
	Flags                   uint32
	PApplicationInfo        *applicationInfo
	EnabledLayerCount       uint32
	PpEnabledLayerNames     **byte
	EnabledExtensionCount   uint32
	PpEnabledExtensionNames **byte
}

type physicalDeviceProperties2 struct {
	SType      uint32
	PNext      unsafe.Pointer
	Properties physicalDeviceProperties
}

type physicalDeviceProperties struct {
	ApiVersion        uint32
	DriverVersion     uint32
	VendorID          uint32
	DeviceID          uint32
	DeviceType        uint32
	DeviceName        [256]byte
	PipelineCacheUUID [16]byte
	Limits            physicalDeviceLimits
	SparseProperties  [5]uint32
}

type physicalDeviceLimits struct {
	MaxImageDimension1D, MaxImageDimension2D uint32
	Rest                                     [62]uint64 // not used
}

type physicalDeviceIDProperties struct {
	SType           uint32
	PNext           unsafe.Pointer
	DeviceUUID      [16]byte
	DriverUUID      [16]byte
	DeviceLUID      [8]byte
	DeviceNodeMask  uint32
	DeviceLUIDValid uint32
}

type physicalDeviceDrmPropertiesEXT struct {
	SType        uint32
	PNext        unsafe.Pointer
	HasPrimary   uint32
	HasRender    uint32
	PrimaryMajor int64
	PrimaryMinor int64
	RenderMajor  int64
	RenderMinor  int64
}

type queueFamilyProperties struct {
	QueueFlags                  uint32
	QueueCount                  uint32
	TimestampValidBits          uint32
	MinImageTransferGranularity extent3D
}

type extent3D struct {
	Width  uint32
	Height uint32
	Depth  uint32
}

type extent2D struct {
	Width  uint32
	Height uint32
}

type offset2D struct {
	X int32
	Y int32
}

type offset3D struct {
	X int32
	Y int32
	Z int32
}

type rect2D struct {
	Offset offset2D
	Extent extent2D
}

type viewport struct {
	X        float32
	Y        float32
	Width    float32
	Height   float32
	MinDepth float32
	MaxDepth float32
}

type physicalDeviceMemoryProperties struct {
	MemoryTypeCount uint32
	MemoryTypes     [32]memoryType
	MemoryHeapCount uint32
	MemoryHeaps     [16]memoryHeap
}

type memoryType struct {
	PropertyFlags uint32
	HeapIndex     uint32
}

type memoryHeap struct {
	Size  uint64
	Flags uint32
}

type extensionProperties struct {
	ExtensionName [256]byte
	SpecVersion   uint32
}

type deviceQueueCreateInfo struct {
	SType            uint32
	PNext            unsafe.Pointer
	Flags            uint32
	QueueFamilyIndex uint32
	QueueCount       uint32
	PQueuePriorities *float32
}

type deviceCreateInfo struct {
	SType                   uint32
	PNext                   unsafe.Pointer
	Flags                   uint32
	QueueCreateInfoCount    uint32
	PQueueCreateInfos       *deviceQueueCreateInfo
	EnabledLayerCount       uint32
	PpEnabledLayerNames     **byte
	EnabledExtensionCount   uint32
	PpEnabledExtensionNames **byte
	PEnabledFeatures        unsafe.Pointer
}

type physicalDeviceDynamicRenderingFeatures struct {
	SType            uint32
	PNext            unsafe.Pointer
	DynamicRendering uint32
}

type formatProperties struct {
	LinearTilingFeatures  uint32
	OptimalTilingFeatures uint32
	BufferFeatures        uint32
}

type formatProperties2 struct {
	SType            uint32
	PNext            unsafe.Pointer
	FormatProperties formatProperties
}

type drmFormatModifierPropertiesListEXT struct {
	SType                        uint32
	PNext                        unsafe.Pointer
	DrmFormatModifierCount       uint32
	PDrmFormatModifierProperties *drmFormatModifierPropertiesEXT
}

type drmFormatModifierPropertiesEXT struct {
	DrmFormatModifier               uint64
	DrmFormatModifierPlaneCount     uint32
	DrmFormatModifierTilingFeatures uint32
}

type physicalDeviceImageFormatInfo2 struct {
	SType  uint32
	PNext  unsafe.Pointer
	Format uint32
	Type   uint32
	Tiling uint32
	Usage  uint32
	Flags  uint32
}

type physicalDeviceExternalImageFormatInfo struct {
	SType      uint32
	PNext      unsafe.Pointer
	HandleType uint32
}

type physicalDeviceImageDrmFormatModifierInfoEXT struct {
	SType                 uint32
	PNext                 unsafe.Pointer
	DrmFormatModifier     uint64
	SharingMode           uint32
	QueueFamilyIndexCount uint32
	PQueueFamilyIndices   *uint32
}

type imageFormatProperties struct {
	MaxExtent       extent3D
	MaxMipLevels    uint32
	MaxArrayLayers  uint32
	SampleCounts    uint32
	MaxResourceSize uint64
}

type imageFormatProperties2 struct {
	SType                 uint32
	PNext                 unsafe.Pointer
	ImageFormatProperties imageFormatProperties
}

type externalMemoryProperties struct {
	ExternalMemoryFeatures        uint32
	ExportFromImportedHandleTypes uint32
	CompatibleHandleTypes         uint32
}

type externalImageFormatProperties struct {
	SType                    uint32
	PNext                    unsafe.Pointer
	ExternalMemoryProperties externalMemoryProperties
}

type physicalDeviceExternalSemaphoreInfo struct {
	SType      uint32
	PNext      unsafe.Pointer
	HandleType uint32
}

type externalSemaphoreProperties struct {
	SType                         uint32
	PNext                         unsafe.Pointer
	ExportFromImportedHandleTypes uint32
	CompatibleHandleTypes         uint32
	ExternalSemaphoreFeatures     uint32
}

type imageCreateInfo struct {
	SType                 uint32
	PNext                 unsafe.Pointer
	Flags                 uint32
	ImageType             uint32
	Format                uint32
	Extent                extent3D
	MipLevels             uint32
	ArrayLayers           uint32
	Samples               uint32
	Tiling                uint32
	Usage                 uint32
	SharingMode           uint32
	QueueFamilyIndexCount uint32
	PQueueFamilyIndices   *uint32
	InitialLayout         uint32
}

type externalMemoryImageCreateInfo struct {
	SType       uint32
	PNext       unsafe.Pointer
	HandleTypes uint32
}

type imageDrmFormatModifierListCreateInfoEXT struct {
	SType                  uint32
	PNext                  unsafe.Pointer
	DrmFormatModifierCount uint32
	PDrmFormatModifiers    *uint64
}

type imageDrmFormatModifierPropertiesEXT struct {
	SType             uint32
	PNext             unsafe.Pointer
	DrmFormatModifier uint64
}

type memoryRequirements struct {
	Size           uint64
	Alignment      uint64
	MemoryTypeBits uint32
}

type memoryAllocateInfo struct {
	SType           uint32
	PNext           unsafe.Pointer
	AllocationSize  uint64
	MemoryTypeIndex uint32
}

type exportMemoryAllocateInfo struct {
	SType       uint32
	PNext       unsafe.Pointer
	HandleTypes uint32
}

type memoryDedicatedAllocateInfo struct {
	SType  uint32
	PNext  unsafe.Pointer
	Image  uint64
	Buffer uint64
}

type memoryGetFdInfoKHR struct {
	SType      uint32
	PNext      unsafe.Pointer
	Memory     uint64
	HandleType uint32
}

type imageSubresource struct {
	AspectMask uint32
	MipLevel   uint32
	ArrayLayer uint32
}

type subresourceLayout struct {
	Offset     uint64
	Size       uint64
	RowPitch   uint64
	ArrayPitch uint64
	DepthPitch uint64
}

type imageSubresourceRange struct {
	AspectMask     uint32
	BaseMipLevel   uint32
	LevelCount     uint32
	BaseArrayLayer uint32
	LayerCount     uint32
}

type imageSubresourceLayers struct {
	AspectMask     uint32
	MipLevel       uint32
	BaseArrayLayer uint32
	LayerCount     uint32
}

type imageViewCreateInfo struct {
	SType            uint32
	PNext            unsafe.Pointer
	Flags            uint32
	Image            uint64
	ViewType         uint32
	Format           uint32
	Components       componentMapping
	SubresourceRange imageSubresourceRange
}

type componentMapping struct {
	R uint32
	G uint32
	B uint32
	A uint32
}

type shaderModuleCreateInfo struct {
	SType    uint32
	PNext    unsafe.Pointer
	Flags    uint32
	CodeSize uintptr
	PCode    *uint32
}

type pushConstantRange struct {
	StageFlags uint32
	Offset     uint32
	Size       uint32
}

type pipelineLayoutCreateInfo struct {
	SType                  uint32
	PNext                  unsafe.Pointer
	Flags                  uint32
	SetLayoutCount         uint32
	PSetLayouts            *uint64
	PushConstantRangeCount uint32
	PPushConstantRanges    *pushConstantRange
}

type pipelineShaderStageCreateInfo struct {
	SType               uint32
	PNext               unsafe.Pointer
	Flags               uint32
	Stage               uint32
	Module              uint64
	PName               *byte
	PSpecializationInfo unsafe.Pointer
}

type pipelineVertexInputStateCreateInfo struct {
	SType                           uint32
	PNext                           unsafe.Pointer
	Flags                           uint32
	VertexBindingDescriptionCount   uint32
	PVertexBindingDescriptions      unsafe.Pointer
	VertexAttributeDescriptionCount uint32
	PVertexAttributeDescriptions    unsafe.Pointer
}

type pipelineInputAssemblyStateCreateInfo struct {
	SType                  uint32
	PNext                  unsafe.Pointer
	Flags                  uint32
	Topology               uint32
	PrimitiveRestartEnable uint32
}

type pipelineViewportStateCreateInfo struct {
	SType         uint32
	PNext         unsafe.Pointer
	Flags         uint32
	ViewportCount uint32
	PViewports    *viewport
	ScissorCount  uint32
	PScissors     *rect2D
}

type pipelineRasterizationStateCreateInfo struct {
	SType                   uint32
	PNext                   unsafe.Pointer
	Flags                   uint32
	DepthClampEnable        uint32
	RasterizerDiscardEnable uint32
	PolygonMode             uint32
	CullMode                uint32
	FrontFace               uint32
	DepthBiasEnable         uint32
	DepthBiasConstantFactor float32
	DepthBiasClamp          float32
	DepthBiasSlopeFactor    float32
	LineWidth               float32
}

type pipelineMultisampleStateCreateInfo struct {
	SType                 uint32
	PNext                 unsafe.Pointer
	Flags                 uint32
	RasterizationSamples  uint32
	SampleShadingEnable   uint32
	MinSampleShading      float32
	PSampleMask           unsafe.Pointer
	AlphaToCoverageEnable uint32
	AlphaToOneEnable      uint32
}

type pipelineColorBlendAttachmentState struct {
	BlendEnable         uint32
	SrcColorBlendFactor uint32
	DstColorBlendFactor uint32
	ColorBlendOp        uint32
	SrcAlphaBlendFactor uint32
	DstAlphaBlendFactor uint32
	AlphaBlendOp        uint32
	ColorWriteMask      uint32
}

type pipelineColorBlendStateCreateInfo struct {
	SType           uint32
	PNext           unsafe.Pointer
	Flags           uint32
	LogicOpEnable   uint32
	LogicOp         uint32
	AttachmentCount uint32
	PAttachments    *pipelineColorBlendAttachmentState
	BlendConstants  [4]float32
}

type pipelineDynamicStateCreateInfo struct {
	SType             uint32
	PNext             unsafe.Pointer
	Flags             uint32
	DynamicStateCount uint32
	PDynamicStates    *uint32
}

type pipelineRenderingCreateInfo struct {
	SType                   uint32
	PNext                   unsafe.Pointer
	ViewMask                uint32
	ColorAttachmentCount    uint32
	PColorAttachmentFormats *uint32
	DepthAttachmentFormat   uint32
	StencilAttachmentFormat uint32
}

type graphicsPipelineCreateInfo struct {
	SType               uint32
	PNext               unsafe.Pointer
	Flags               uint32
	StageCount          uint32
	PStages             *pipelineShaderStageCreateInfo
	PVertexInputState   *pipelineVertexInputStateCreateInfo
	PInputAssemblyState *pipelineInputAssemblyStateCreateInfo
	PTessellationState  unsafe.Pointer
	PViewportState      *pipelineViewportStateCreateInfo
	PRasterizationState *pipelineRasterizationStateCreateInfo
	PMultisampleState   *pipelineMultisampleStateCreateInfo
	PDepthStencilState  unsafe.Pointer
	PColorBlendState    *pipelineColorBlendStateCreateInfo
	PDynamicState       *pipelineDynamicStateCreateInfo
	Layout              uint64
	RenderPass          uint64
	Subpass             uint32
	BasePipelineHandle  uint64
	BasePipelineIndex   int32
}

type commandPoolCreateInfo struct {
	SType            uint32
	PNext            unsafe.Pointer
	Flags            uint32
	QueueFamilyIndex uint32
}

type commandBufferAllocateInfo struct {
	SType              uint32
	PNext              unsafe.Pointer
	CommandPool        uint64
	Level              uint32
	CommandBufferCount uint32
}

type commandBufferBeginInfo struct {
	SType            uint32
	PNext            unsafe.Pointer
	Flags            uint32
	PInheritanceInfo unsafe.Pointer
}

type imageMemoryBarrier struct {
	SType               uint32
	PNext               unsafe.Pointer
	SrcAccessMask       uint32
	DstAccessMask       uint32
	OldLayout           uint32
	NewLayout           uint32
	SrcQueueFamilyIndex uint32
	DstQueueFamilyIndex uint32
	Image               uint64
	SubresourceRange    imageSubresourceRange
}

type renderingAttachmentInfo struct {
	SType              uint32
	PNext              unsafe.Pointer
	ImageView          uint64
	ImageLayout        uint32
	ResolveMode        uint32
	ResolveImageView   uint64
	ResolveImageLayout uint32
	LoadOp             uint32
	StoreOp            uint32
	ClearValue         [4]float32
}

type renderingInfo struct {
	SType                uint32
	PNext                unsafe.Pointer
	Flags                uint32
	RenderArea           rect2D
	LayerCount           uint32
	ViewMask             uint32
	ColorAttachmentCount uint32
	PColorAttachments    *renderingAttachmentInfo
	PDepthAttachment     *renderingAttachmentInfo
	PStencilAttachment   *renderingAttachmentInfo
}

type imageCopy struct {
	SrcSubresource imageSubresourceLayers
	SrcOffset      offset3D
	DstSubresource imageSubresourceLayers
	DstOffset      offset3D
	Extent         extent3D
}

type submitInfo struct {
	SType                uint32
	PNext                unsafe.Pointer
	WaitSemaphoreCount   uint32
	PWaitSemaphores      *uint64
	PWaitDstStageMask    *uint32
	CommandBufferCount   uint32
	PCommandBuffers      *uintptr
	SignalSemaphoreCount uint32
	PSignalSemaphores    *uint64
}

type fenceCreateInfo struct {
	SType uint32
	PNext unsafe.Pointer
	Flags uint32
}

type semaphoreCreateInfo struct {
	SType uint32
	PNext unsafe.Pointer
	Flags uint32
}

type exportSemaphoreCreateInfo struct {
	SType       uint32
	PNext       unsafe.Pointer
	HandleTypes uint32
}

type semaphoreGetFdInfoKHR struct {
	SType      uint32
	PNext      unsafe.Pointer
	Semaphore  uint64
	HandleType uint32
}

type debugUtilsMessengerCreateInfoEXT struct {
	SType           uint32
	PNext           unsafe.Pointer
	Flags           uint32
	MessageSeverity uint32
	MessageType     uint32
	PfnUserCallback uintptr
	PUserData       unsafe.Pointer
}

type debugUtilsMessengerCallbackDataEXT struct {
	SType            uint32
	PNext            unsafe.Pointer
	Flags            uint32
	PMessageIdName   *byte
	MessageIdNumber  int32
	PMessage         *byte
	QueueLabelCount  uint32
	PQueueLabels     unsafe.Pointer
	CmdBufLabelCount uint32
	PCmdBufLabels    unsafe.Pointer
	ObjectCount      uint32
	PObjects         unsafe.Pointer
}
