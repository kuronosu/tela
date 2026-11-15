//go:build telagpu && (linux || windows)

// The declarations are transcribed from the Vulkan headers
// (vulkan/vulkan_core.h), Copyright 2015-2026 The Khronos Group Inc.,
// SPDX-License-Identifier: Apache-2.0 OR MIT (see THIRD_PARTY.md).

package vk

// The constants of the bindings. Every constant carries the name of its C
// constant in a trailing comment: internal/layoutgen reads it and
// TestLayout compares the value with the one of the real header (see the
// package documentation). Add a constant with its comment and regenerate.

// Sizes of the fixed arrays in structs, and the special values of counts
// and indices.
const (
	MaxPhysicalDeviceNameSize = 256 // VK_MAX_PHYSICAL_DEVICE_NAME_SIZE
	UUIDSize                  = 16  // VK_UUID_SIZE
	MaxExtensionNameSize      = 256 // VK_MAX_EXTENSION_NAME_SIZE
	MaxDescriptionSize        = 256 // VK_MAX_DESCRIPTION_SIZE
	MaxMemoryTypes            = 32  // VK_MAX_MEMORY_TYPES
	MaxMemoryHeaps            = 16  // VK_MAX_MEMORY_HEAPS

	WholeSize            uint64 = 1<<64 - 1 // VK_WHOLE_SIZE
	RemainingMipLevels   uint32 = 1<<32 - 1 // VK_REMAINING_MIP_LEVELS
	RemainingArrayLayers uint32 = 1<<32 - 1 // VK_REMAINING_ARRAY_LAYERS
	QueueFamilyIgnored   uint32 = 1<<32 - 1 // VK_QUEUE_FAMILY_IGNORED
	AttachmentUnused     uint32 = 1<<32 - 1 // VK_ATTACHMENT_UNUSED
	SubpassExternal      uint32 = 1<<32 - 1 // VK_SUBPASS_EXTERNAL
)

// Bool32 is VkBool32: [True] or [False] in 32 bits.
type Bool32 uint32

const (
	False Bool32 = 0 // VK_FALSE
	True  Bool32 = 1 // VK_TRUE
)

// Versions of the API, as [MakeVersion] packs them.
const (
	APIVersion1_0 uint32 = 1 << 22       // VK_API_VERSION_1_0
	APIVersion1_1 uint32 = 1<<22 | 1<<12 // VK_API_VERSION_1_1
	APIVersion1_2 uint32 = 1<<22 | 2<<12 // VK_API_VERSION_1_2
	APIVersion1_3 uint32 = 1<<22 | 3<<12 // VK_API_VERSION_1_3
)

// Names of the extensions the bindings know.
const (
	ExtDebugUtilsExtensionName        = "VK_EXT_debug_utils"         // VK_EXT_DEBUG_UTILS_EXTENSION_NAME
	ExtPhysicalDeviceDrmExtensionName = "VK_EXT_physical_device_drm" // VK_EXT_PHYSICAL_DEVICE_DRM_EXTENSION_NAME
	ExtLayerSettingsExtensionName     = "VK_EXT_layer_settings"      // VK_EXT_LAYER_SETTINGS_EXTENSION_NAME
)

// Result is VkResult. It implements error; the wrappers return nil instead
// of [Success].
type Result int32

const (
	Success                    Result = 0           // VK_SUCCESS
	NotReady                   Result = 1           // VK_NOT_READY
	Timeout                    Result = 2           // VK_TIMEOUT
	EventSet                   Result = 3           // VK_EVENT_SET
	EventReset                 Result = 4           // VK_EVENT_RESET
	Incomplete                 Result = 5           // VK_INCOMPLETE
	ErrorOutOfHostMemory       Result = -1          // VK_ERROR_OUT_OF_HOST_MEMORY
	ErrorOutOfDeviceMemory     Result = -2          // VK_ERROR_OUT_OF_DEVICE_MEMORY
	ErrorInitializationFailed  Result = -3          // VK_ERROR_INITIALIZATION_FAILED
	ErrorDeviceLost            Result = -4          // VK_ERROR_DEVICE_LOST
	ErrorMemoryMapFailed       Result = -5          // VK_ERROR_MEMORY_MAP_FAILED
	ErrorLayerNotPresent       Result = -6          // VK_ERROR_LAYER_NOT_PRESENT
	ErrorExtensionNotPresent   Result = -7          // VK_ERROR_EXTENSION_NOT_PRESENT
	ErrorFeatureNotPresent     Result = -8          // VK_ERROR_FEATURE_NOT_PRESENT
	ErrorIncompatibleDriver    Result = -9          // VK_ERROR_INCOMPATIBLE_DRIVER
	ErrorTooManyObjects        Result = -10         // VK_ERROR_TOO_MANY_OBJECTS
	ErrorFormatNotSupported    Result = -11         // VK_ERROR_FORMAT_NOT_SUPPORTED
	ErrorFragmentedPool        Result = -12         // VK_ERROR_FRAGMENTED_POOL
	ErrorUnknown               Result = -13         // VK_ERROR_UNKNOWN
	ErrorOutOfPoolMemory       Result = -1000069000 // VK_ERROR_OUT_OF_POOL_MEMORY
	ErrorInvalidExternalHandle Result = -1000072003 // VK_ERROR_INVALID_EXTERNAL_HANDLE
	ErrorFragmentation         Result = -1000161000 // VK_ERROR_FRAGMENTATION
	ErrorSurfaceLostKHR        Result = -1000000000 // VK_ERROR_SURFACE_LOST_KHR
	ErrorNativeWindowInUseKHR  Result = -1000000001 // VK_ERROR_NATIVE_WINDOW_IN_USE_KHR
	SuboptimalKHR              Result = 1000001003  // VK_SUBOPTIMAL_KHR
	ErrorOutOfDateKHR          Result = -1000001004 // VK_ERROR_OUT_OF_DATE_KHR
	ErrorValidationFailedEXT   Result = -1000011001 // VK_ERROR_VALIDATION_FAILED_EXT
)

// StructureType identifies a struct to the functions that follow pNext chains (VkStructureType).
type StructureType int32

const (
	StructureTypeApplicationInfo                      StructureType = 0          // VK_STRUCTURE_TYPE_APPLICATION_INFO
	StructureTypeInstanceCreateInfo                   StructureType = 1          // VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO
	StructureTypeDeviceQueueCreateInfo                StructureType = 2          // VK_STRUCTURE_TYPE_DEVICE_QUEUE_CREATE_INFO
	StructureTypeDeviceCreateInfo                     StructureType = 3          // VK_STRUCTURE_TYPE_DEVICE_CREATE_INFO
	StructureTypeSubmitInfo                           StructureType = 4          // VK_STRUCTURE_TYPE_SUBMIT_INFO
	StructureTypeMemoryAllocateInfo                   StructureType = 5          // VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO
	StructureTypeMappedMemoryRange                    StructureType = 6          // VK_STRUCTURE_TYPE_MAPPED_MEMORY_RANGE
	StructureTypeFenceCreateInfo                      StructureType = 8          // VK_STRUCTURE_TYPE_FENCE_CREATE_INFO
	StructureTypeSemaphoreCreateInfo                  StructureType = 9          // VK_STRUCTURE_TYPE_SEMAPHORE_CREATE_INFO
	StructureTypeBufferCreateInfo                     StructureType = 12         // VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO
	StructureTypeImageCreateInfo                      StructureType = 14         // VK_STRUCTURE_TYPE_IMAGE_CREATE_INFO
	StructureTypeImageViewCreateInfo                  StructureType = 15         // VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO
	StructureTypeShaderModuleCreateInfo               StructureType = 16         // VK_STRUCTURE_TYPE_SHADER_MODULE_CREATE_INFO
	StructureTypePipelineShaderStageCreateInfo        StructureType = 18         // VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO
	StructureTypePipelineVertexInputStateCreateInfo   StructureType = 19         // VK_STRUCTURE_TYPE_PIPELINE_VERTEX_INPUT_STATE_CREATE_INFO
	StructureTypePipelineInputAssemblyStateCreateInfo StructureType = 20         // VK_STRUCTURE_TYPE_PIPELINE_INPUT_ASSEMBLY_STATE_CREATE_INFO
	StructureTypePipelineViewportStateCreateInfo      StructureType = 22         // VK_STRUCTURE_TYPE_PIPELINE_VIEWPORT_STATE_CREATE_INFO
	StructureTypePipelineRasterizationStateCreateInfo StructureType = 23         // VK_STRUCTURE_TYPE_PIPELINE_RASTERIZATION_STATE_CREATE_INFO
	StructureTypePipelineMultisampleStateCreateInfo   StructureType = 24         // VK_STRUCTURE_TYPE_PIPELINE_MULTISAMPLE_STATE_CREATE_INFO
	StructureTypePipelineDepthStencilStateCreateInfo  StructureType = 25         // VK_STRUCTURE_TYPE_PIPELINE_DEPTH_STENCIL_STATE_CREATE_INFO
	StructureTypePipelineColorBlendStateCreateInfo    StructureType = 26         // VK_STRUCTURE_TYPE_PIPELINE_COLOR_BLEND_STATE_CREATE_INFO
	StructureTypePipelineDynamicStateCreateInfo       StructureType = 27         // VK_STRUCTURE_TYPE_PIPELINE_DYNAMIC_STATE_CREATE_INFO
	StructureTypeGraphicsPipelineCreateInfo           StructureType = 28         // VK_STRUCTURE_TYPE_GRAPHICS_PIPELINE_CREATE_INFO
	StructureTypePipelineLayoutCreateInfo             StructureType = 30         // VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO
	StructureTypeSamplerCreateInfo                    StructureType = 31         // VK_STRUCTURE_TYPE_SAMPLER_CREATE_INFO
	StructureTypeDescriptorSetLayoutCreateInfo        StructureType = 32         // VK_STRUCTURE_TYPE_DESCRIPTOR_SET_LAYOUT_CREATE_INFO
	StructureTypeDescriptorPoolCreateInfo             StructureType = 33         // VK_STRUCTURE_TYPE_DESCRIPTOR_POOL_CREATE_INFO
	StructureTypeDescriptorSetAllocateInfo            StructureType = 34         // VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO
	StructureTypeWriteDescriptorSet                   StructureType = 35         // VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET
	StructureTypeCopyDescriptorSet                    StructureType = 36         // VK_STRUCTURE_TYPE_COPY_DESCRIPTOR_SET
	StructureTypeFramebufferCreateInfo                StructureType = 37         // VK_STRUCTURE_TYPE_FRAMEBUFFER_CREATE_INFO
	StructureTypeRenderPassCreateInfo                 StructureType = 38         // VK_STRUCTURE_TYPE_RENDER_PASS_CREATE_INFO
	StructureTypeCommandPoolCreateInfo                StructureType = 39         // VK_STRUCTURE_TYPE_COMMAND_POOL_CREATE_INFO
	StructureTypeCommandBufferAllocateInfo            StructureType = 40         // VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO
	StructureTypeCommandBufferBeginInfo               StructureType = 42         // VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO
	StructureTypeRenderPassBeginInfo                  StructureType = 43         // VK_STRUCTURE_TYPE_RENDER_PASS_BEGIN_INFO
	StructureTypeBufferMemoryBarrier                  StructureType = 44         // VK_STRUCTURE_TYPE_BUFFER_MEMORY_BARRIER
	StructureTypeImageMemoryBarrier                   StructureType = 45         // VK_STRUCTURE_TYPE_IMAGE_MEMORY_BARRIER
	StructureTypeMemoryBarrier                        StructureType = 46         // VK_STRUCTURE_TYPE_MEMORY_BARRIER
	StructureTypePhysicalDeviceProperties2            StructureType = 1000059001 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_PROPERTIES_2
	StructureTypePhysicalDeviceDrmPropertiesEXT       StructureType = 1000353000 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_DRM_PROPERTIES_EXT
	StructureTypeDebugUtilsObjectNameInfoEXT          StructureType = 1000128000 // VK_STRUCTURE_TYPE_DEBUG_UTILS_OBJECT_NAME_INFO_EXT
	StructureTypeDebugUtilsLabelEXT                   StructureType = 1000128002 // VK_STRUCTURE_TYPE_DEBUG_UTILS_LABEL_EXT
	StructureTypeDebugUtilsMessengerCallbackDataEXT   StructureType = 1000128003 // VK_STRUCTURE_TYPE_DEBUG_UTILS_MESSENGER_CALLBACK_DATA_EXT
	StructureTypeDebugUtilsMessengerCreateInfoEXT     StructureType = 1000128004 // VK_STRUCTURE_TYPE_DEBUG_UTILS_MESSENGER_CREATE_INFO_EXT
	StructureTypeLayerSettingsCreateInfoEXT           StructureType = 1000496000 // VK_STRUCTURE_TYPE_LAYER_SETTINGS_CREATE_INFO_EXT
)

// LayerSettingTypeEXT is the type of the values of a layer setting
// (VkLayerSettingTypeEXT).
type LayerSettingTypeEXT int32

const (
	LayerSettingTypeBool32EXT LayerSettingTypeEXT = 0 // VK_LAYER_SETTING_TYPE_BOOL32_EXT
	LayerSettingTypeInt32EXT  LayerSettingTypeEXT = 1 // VK_LAYER_SETTING_TYPE_INT32_EXT
	LayerSettingTypeUint32EXT LayerSettingTypeEXT = 3 // VK_LAYER_SETTING_TYPE_UINT32_EXT
	LayerSettingTypeStringEXT LayerSettingTypeEXT = 7 // VK_LAYER_SETTING_TYPE_STRING_EXT
)

// PhysicalDeviceType is the kind of a physical device (VkPhysicalDeviceType).
type PhysicalDeviceType int32

const (
	PhysicalDeviceTypeOther         PhysicalDeviceType = 0 // VK_PHYSICAL_DEVICE_TYPE_OTHER
	PhysicalDeviceTypeIntegratedGPU PhysicalDeviceType = 1 // VK_PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU
	PhysicalDeviceTypeDiscreteGPU   PhysicalDeviceType = 2 // VK_PHYSICAL_DEVICE_TYPE_DISCRETE_GPU
	PhysicalDeviceTypeVirtualGPU    PhysicalDeviceType = 3 // VK_PHYSICAL_DEVICE_TYPE_VIRTUAL_GPU
	PhysicalDeviceTypeCPU           PhysicalDeviceType = 4 // VK_PHYSICAL_DEVICE_TYPE_CPU
)

// Format is a pixel or vertex attribute format (VkFormat).
type Format int32

const (
	FormatUndefined              Format = 0   // VK_FORMAT_UNDEFINED
	FormatR8Unorm                Format = 9   // VK_FORMAT_R8_UNORM
	FormatR8G8Unorm              Format = 16  // VK_FORMAT_R8G8_UNORM
	FormatR8G8B8A8Unorm          Format = 37  // VK_FORMAT_R8G8B8A8_UNORM
	FormatR8G8B8A8Uint           Format = 41  // VK_FORMAT_R8G8B8A8_UINT
	FormatR8G8B8A8Srgb           Format = 43  // VK_FORMAT_R8G8B8A8_SRGB
	FormatB8G8R8A8Unorm          Format = 44  // VK_FORMAT_B8G8R8A8_UNORM
	FormatB8G8R8A8Srgb           Format = 50  // VK_FORMAT_B8G8R8A8_SRGB
	FormatA8B8G8R8UnormPack32    Format = 51  // VK_FORMAT_A8B8G8R8_UNORM_PACK32
	FormatA2R10G10B10UnormPack32 Format = 58  // VK_FORMAT_A2R10G10B10_UNORM_PACK32
	FormatA2B10G10R10UnormPack32 Format = 64  // VK_FORMAT_A2B10G10R10_UNORM_PACK32
	FormatR16Unorm               Format = 70  // VK_FORMAT_R16_UNORM
	FormatR16Uint                Format = 74  // VK_FORMAT_R16_UINT
	FormatR16Sfloat              Format = 76  // VK_FORMAT_R16_SFLOAT
	FormatR16G16Unorm            Format = 77  // VK_FORMAT_R16G16_UNORM
	FormatR16G16Uint             Format = 81  // VK_FORMAT_R16G16_UINT
	FormatR16G16Sint             Format = 82  // VK_FORMAT_R16G16_SINT
	FormatR16G16Sfloat           Format = 83  // VK_FORMAT_R16G16_SFLOAT
	FormatR16G16B16A16Unorm      Format = 91  // VK_FORMAT_R16G16B16A16_UNORM
	FormatR16G16B16A16Uint       Format = 95  // VK_FORMAT_R16G16B16A16_UINT
	FormatR16G16B16A16Sint       Format = 96  // VK_FORMAT_R16G16B16A16_SINT
	FormatR16G16B16A16Sfloat     Format = 97  // VK_FORMAT_R16G16B16A16_SFLOAT
	FormatR32Uint                Format = 98  // VK_FORMAT_R32_UINT
	FormatR32Sint                Format = 99  // VK_FORMAT_R32_SINT
	FormatR32Sfloat              Format = 100 // VK_FORMAT_R32_SFLOAT
	FormatR32G32Uint             Format = 101 // VK_FORMAT_R32G32_UINT
	FormatR32G32Sint             Format = 102 // VK_FORMAT_R32G32_SINT
	FormatR32G32Sfloat           Format = 103 // VK_FORMAT_R32G32_SFLOAT
	FormatR32G32B32Sfloat        Format = 106 // VK_FORMAT_R32G32B32_SFLOAT
	FormatR32G32B32A32Uint       Format = 107 // VK_FORMAT_R32G32B32A32_UINT
	FormatR32G32B32A32Sint       Format = 108 // VK_FORMAT_R32G32B32A32_SINT
	FormatR32G32B32A32Sfloat     Format = 109 // VK_FORMAT_R32G32B32A32_SFLOAT
	FormatD16Unorm               Format = 124 // VK_FORMAT_D16_UNORM
	FormatD32Sfloat              Format = 126 // VK_FORMAT_D32_SFLOAT
	FormatS8Uint                 Format = 127 // VK_FORMAT_S8_UINT
	FormatD24UnormS8Uint         Format = 129 // VK_FORMAT_D24_UNORM_S8_UINT
	FormatD32SfloatS8Uint        Format = 130 // VK_FORMAT_D32_SFLOAT_S8_UINT
)

// ImageType is the dimensionality of an image (VkImageType).
type ImageType int32

const (
	ImageType1D ImageType = 0 // VK_IMAGE_TYPE_1D
	ImageType2D ImageType = 1 // VK_IMAGE_TYPE_2D
	ImageType3D ImageType = 2 // VK_IMAGE_TYPE_3D
)

// ImageViewType is the kind of an image view (VkImageViewType).
type ImageViewType int32

const (
	ImageViewType1D      ImageViewType = 0 // VK_IMAGE_VIEW_TYPE_1D
	ImageViewType2D      ImageViewType = 1 // VK_IMAGE_VIEW_TYPE_2D
	ImageViewType3D      ImageViewType = 2 // VK_IMAGE_VIEW_TYPE_3D
	ImageViewTypeCube    ImageViewType = 3 // VK_IMAGE_VIEW_TYPE_CUBE
	ImageViewType1DArray ImageViewType = 4 // VK_IMAGE_VIEW_TYPE_1D_ARRAY
	ImageViewType2DArray ImageViewType = 5 // VK_IMAGE_VIEW_TYPE_2D_ARRAY
)

// ImageTiling is the arrangement of an image's texels in memory (VkImageTiling).
type ImageTiling int32

const (
	ImageTilingOptimal              ImageTiling = 0          // VK_IMAGE_TILING_OPTIMAL
	ImageTilingLinear               ImageTiling = 1          // VK_IMAGE_TILING_LINEAR
	ImageTilingDrmFormatModifierEXT ImageTiling = 1000158000 // VK_IMAGE_TILING_DRM_FORMAT_MODIFIER_EXT
)

// ImageLayout is the layout of an image subresource (VkImageLayout).
type ImageLayout int32

const (
	ImageLayoutUndefined                     ImageLayout = 0          // VK_IMAGE_LAYOUT_UNDEFINED
	ImageLayoutGeneral                       ImageLayout = 1          // VK_IMAGE_LAYOUT_GENERAL
	ImageLayoutColorAttachmentOptimal        ImageLayout = 2          // VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL
	ImageLayoutDepthStencilAttachmentOptimal ImageLayout = 3          // VK_IMAGE_LAYOUT_DEPTH_STENCIL_ATTACHMENT_OPTIMAL
	ImageLayoutDepthStencilReadOnlyOptimal   ImageLayout = 4          // VK_IMAGE_LAYOUT_DEPTH_STENCIL_READ_ONLY_OPTIMAL
	ImageLayoutShaderReadOnlyOptimal         ImageLayout = 5          // VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL
	ImageLayoutTransferSrcOptimal            ImageLayout = 6          // VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL
	ImageLayoutTransferDstOptimal            ImageLayout = 7          // VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL
	ImageLayoutPreinitialized                ImageLayout = 8          // VK_IMAGE_LAYOUT_PREINITIALIZED
	ImageLayoutPresentSrcKHR                 ImageLayout = 1000001002 // VK_IMAGE_LAYOUT_PRESENT_SRC_KHR
)

// SharingMode says whether queue families share a buffer or image (VkSharingMode).
type SharingMode int32

const (
	SharingModeExclusive  SharingMode = 0 // VK_SHARING_MODE_EXCLUSIVE
	SharingModeConcurrent SharingMode = 1 // VK_SHARING_MODE_CONCURRENT
)

// ComponentSwizzle remaps a color component of an image view (VkComponentSwizzle).
type ComponentSwizzle int32

const (
	ComponentSwizzleIdentity ComponentSwizzle = 0 // VK_COMPONENT_SWIZZLE_IDENTITY
	ComponentSwizzleZero     ComponentSwizzle = 1 // VK_COMPONENT_SWIZZLE_ZERO
	ComponentSwizzleOne      ComponentSwizzle = 2 // VK_COMPONENT_SWIZZLE_ONE
	ComponentSwizzleR        ComponentSwizzle = 3 // VK_COMPONENT_SWIZZLE_R
	ComponentSwizzleG        ComponentSwizzle = 4 // VK_COMPONENT_SWIZZLE_G
	ComponentSwizzleB        ComponentSwizzle = 5 // VK_COMPONENT_SWIZZLE_B
	ComponentSwizzleA        ComponentSwizzle = 6 // VK_COMPONENT_SWIZZLE_A
)

// Filter is a texture filter (VkFilter).
type Filter int32

const (
	FilterNearest Filter = 0 // VK_FILTER_NEAREST
	FilterLinear  Filter = 1 // VK_FILTER_LINEAR
)

// SamplerMipmapMode is the filter between mip levels (VkSamplerMipmapMode).
type SamplerMipmapMode int32

const (
	SamplerMipmapModeNearest SamplerMipmapMode = 0 // VK_SAMPLER_MIPMAP_MODE_NEAREST
	SamplerMipmapModeLinear  SamplerMipmapMode = 1 // VK_SAMPLER_MIPMAP_MODE_LINEAR
)

// SamplerAddressMode is the behavior of a sampler outside the image (VkSamplerAddressMode).
type SamplerAddressMode int32

const (
	SamplerAddressModeRepeat         SamplerAddressMode = 0 // VK_SAMPLER_ADDRESS_MODE_REPEAT
	SamplerAddressModeMirroredRepeat SamplerAddressMode = 1 // VK_SAMPLER_ADDRESS_MODE_MIRRORED_REPEAT
	SamplerAddressModeClampToEdge    SamplerAddressMode = 2 // VK_SAMPLER_ADDRESS_MODE_CLAMP_TO_EDGE
	SamplerAddressModeClampToBorder  SamplerAddressMode = 3 // VK_SAMPLER_ADDRESS_MODE_CLAMP_TO_BORDER
)

// BorderColor is the border color of a sampler (VkBorderColor).
type BorderColor int32

const (
	BorderColorFloatTransparentBlack BorderColor = 0 // VK_BORDER_COLOR_FLOAT_TRANSPARENT_BLACK
	BorderColorIntTransparentBlack   BorderColor = 1 // VK_BORDER_COLOR_INT_TRANSPARENT_BLACK
	BorderColorFloatOpaqueBlack      BorderColor = 2 // VK_BORDER_COLOR_FLOAT_OPAQUE_BLACK
	BorderColorIntOpaqueBlack        BorderColor = 3 // VK_BORDER_COLOR_INT_OPAQUE_BLACK
	BorderColorFloatOpaqueWhite      BorderColor = 4 // VK_BORDER_COLOR_FLOAT_OPAQUE_WHITE
	BorderColorIntOpaqueWhite        BorderColor = 5 // VK_BORDER_COLOR_INT_OPAQUE_WHITE
)

// CompareOp is a comparison of depth, stencil or sampler references (VkCompareOp).
type CompareOp int32

const (
	CompareOpNever          CompareOp = 0 // VK_COMPARE_OP_NEVER
	CompareOpLess           CompareOp = 1 // VK_COMPARE_OP_LESS
	CompareOpEqual          CompareOp = 2 // VK_COMPARE_OP_EQUAL
	CompareOpLessOrEqual    CompareOp = 3 // VK_COMPARE_OP_LESS_OR_EQUAL
	CompareOpGreater        CompareOp = 4 // VK_COMPARE_OP_GREATER
	CompareOpNotEqual       CompareOp = 5 // VK_COMPARE_OP_NOT_EQUAL
	CompareOpGreaterOrEqual CompareOp = 6 // VK_COMPARE_OP_GREATER_OR_EQUAL
	CompareOpAlways         CompareOp = 7 // VK_COMPARE_OP_ALWAYS
)

// StencilOp is what a stencil test does to the stored value (VkStencilOp).
type StencilOp int32

const (
	StencilOpKeep              StencilOp = 0 // VK_STENCIL_OP_KEEP
	StencilOpZero              StencilOp = 1 // VK_STENCIL_OP_ZERO
	StencilOpReplace           StencilOp = 2 // VK_STENCIL_OP_REPLACE
	StencilOpIncrementAndClamp StencilOp = 3 // VK_STENCIL_OP_INCREMENT_AND_CLAMP
	StencilOpDecrementAndClamp StencilOp = 4 // VK_STENCIL_OP_DECREMENT_AND_CLAMP
	StencilOpInvert            StencilOp = 5 // VK_STENCIL_OP_INVERT
	StencilOpIncrementAndWrap  StencilOp = 6 // VK_STENCIL_OP_INCREMENT_AND_WRAP
	StencilOpDecrementAndWrap  StencilOp = 7 // VK_STENCIL_OP_DECREMENT_AND_WRAP
)

// LogicOp is a framebuffer logical operation (VkLogicOp).
type LogicOp int32

const (
	LogicOpClear LogicOp = 0 // VK_LOGIC_OP_CLEAR
	LogicOpCopy  LogicOp = 3 // VK_LOGIC_OP_COPY
)

// BlendFactor is a factor of the blend equation (VkBlendFactor).
type BlendFactor int32

const (
	BlendFactorZero                  BlendFactor = 0  // VK_BLEND_FACTOR_ZERO
	BlendFactorOne                   BlendFactor = 1  // VK_BLEND_FACTOR_ONE
	BlendFactorSrcColor              BlendFactor = 2  // VK_BLEND_FACTOR_SRC_COLOR
	BlendFactorOneMinusSrcColor      BlendFactor = 3  // VK_BLEND_FACTOR_ONE_MINUS_SRC_COLOR
	BlendFactorDstColor              BlendFactor = 4  // VK_BLEND_FACTOR_DST_COLOR
	BlendFactorOneMinusDstColor      BlendFactor = 5  // VK_BLEND_FACTOR_ONE_MINUS_DST_COLOR
	BlendFactorSrcAlpha              BlendFactor = 6  // VK_BLEND_FACTOR_SRC_ALPHA
	BlendFactorOneMinusSrcAlpha      BlendFactor = 7  // VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA
	BlendFactorDstAlpha              BlendFactor = 8  // VK_BLEND_FACTOR_DST_ALPHA
	BlendFactorOneMinusDstAlpha      BlendFactor = 9  // VK_BLEND_FACTOR_ONE_MINUS_DST_ALPHA
	BlendFactorConstantColor         BlendFactor = 10 // VK_BLEND_FACTOR_CONSTANT_COLOR
	BlendFactorOneMinusConstantColor BlendFactor = 11 // VK_BLEND_FACTOR_ONE_MINUS_CONSTANT_COLOR
	BlendFactorConstantAlpha         BlendFactor = 12 // VK_BLEND_FACTOR_CONSTANT_ALPHA
	BlendFactorOneMinusConstantAlpha BlendFactor = 13 // VK_BLEND_FACTOR_ONE_MINUS_CONSTANT_ALPHA
	BlendFactorSrcAlphaSaturate      BlendFactor = 14 // VK_BLEND_FACTOR_SRC_ALPHA_SATURATE
	BlendFactorSrc1Color             BlendFactor = 15 // VK_BLEND_FACTOR_SRC1_COLOR
	BlendFactorOneMinusSrc1Color     BlendFactor = 16 // VK_BLEND_FACTOR_ONE_MINUS_SRC1_COLOR
	BlendFactorSrc1Alpha             BlendFactor = 17 // VK_BLEND_FACTOR_SRC1_ALPHA
	BlendFactorOneMinusSrc1Alpha     BlendFactor = 18 // VK_BLEND_FACTOR_ONE_MINUS_SRC1_ALPHA
)

// BlendOp combines the two terms of the blend equation (VkBlendOp).
type BlendOp int32

const (
	BlendOpAdd             BlendOp = 0 // VK_BLEND_OP_ADD
	BlendOpSubtract        BlendOp = 1 // VK_BLEND_OP_SUBTRACT
	BlendOpReverseSubtract BlendOp = 2 // VK_BLEND_OP_REVERSE_SUBTRACT
	BlendOpMin             BlendOp = 3 // VK_BLEND_OP_MIN
	BlendOpMax             BlendOp = 4 // VK_BLEND_OP_MAX
)

// AttachmentLoadOp is what a render pass does to an attachment when it begins (VkAttachmentLoadOp).
type AttachmentLoadOp int32

const (
	AttachmentLoadOpLoad     AttachmentLoadOp = 0 // VK_ATTACHMENT_LOAD_OP_LOAD
	AttachmentLoadOpClear    AttachmentLoadOp = 1 // VK_ATTACHMENT_LOAD_OP_CLEAR
	AttachmentLoadOpDontCare AttachmentLoadOp = 2 // VK_ATTACHMENT_LOAD_OP_DONT_CARE
)

// AttachmentStoreOp is what a render pass does to an attachment when it ends (VkAttachmentStoreOp).
type AttachmentStoreOp int32

const (
	AttachmentStoreOpStore    AttachmentStoreOp = 0 // VK_ATTACHMENT_STORE_OP_STORE
	AttachmentStoreOpDontCare AttachmentStoreOp = 1 // VK_ATTACHMENT_STORE_OP_DONT_CARE
)

// PipelineBindPoint is the kind of pipeline a binding is for (VkPipelineBindPoint).
type PipelineBindPoint int32

const (
	PipelineBindPointGraphics PipelineBindPoint = 0 // VK_PIPELINE_BIND_POINT_GRAPHICS
	PipelineBindPointCompute  PipelineBindPoint = 1 // VK_PIPELINE_BIND_POINT_COMPUTE
)

// PrimitiveTopology is how vertices form primitives (VkPrimitiveTopology).
type PrimitiveTopology int32

const (
	PrimitiveTopologyPointList     PrimitiveTopology = 0 // VK_PRIMITIVE_TOPOLOGY_POINT_LIST
	PrimitiveTopologyLineList      PrimitiveTopology = 1 // VK_PRIMITIVE_TOPOLOGY_LINE_LIST
	PrimitiveTopologyLineStrip     PrimitiveTopology = 2 // VK_PRIMITIVE_TOPOLOGY_LINE_STRIP
	PrimitiveTopologyTriangleList  PrimitiveTopology = 3 // VK_PRIMITIVE_TOPOLOGY_TRIANGLE_LIST
	PrimitiveTopologyTriangleStrip PrimitiveTopology = 4 // VK_PRIMITIVE_TOPOLOGY_TRIANGLE_STRIP
	PrimitiveTopologyTriangleFan   PrimitiveTopology = 5 // VK_PRIMITIVE_TOPOLOGY_TRIANGLE_FAN
)

// PolygonMode is how polygons are rasterized (VkPolygonMode).
type PolygonMode int32

const (
	PolygonModeFill  PolygonMode = 0 // VK_POLYGON_MODE_FILL
	PolygonModeLine  PolygonMode = 1 // VK_POLYGON_MODE_LINE
	PolygonModePoint PolygonMode = 2 // VK_POLYGON_MODE_POINT
)

// FrontFace is the winding of front-facing triangles (VkFrontFace).
type FrontFace int32

const (
	FrontFaceCounterClockwise FrontFace = 0 // VK_FRONT_FACE_COUNTER_CLOCKWISE
	FrontFaceClockwise        FrontFace = 1 // VK_FRONT_FACE_CLOCKWISE
)

// VertexInputRate says whether a vertex binding advances per vertex or per instance (VkVertexInputRate).
type VertexInputRate int32

const (
	VertexInputRateVertex   VertexInputRate = 0 // VK_VERTEX_INPUT_RATE_VERTEX
	VertexInputRateInstance VertexInputRate = 1 // VK_VERTEX_INPUT_RATE_INSTANCE
)

// DynamicState is a piece of pipeline state set by a command instead of the pipeline (VkDynamicState).
type DynamicState int32

const (
	DynamicStateViewport           DynamicState = 0 // VK_DYNAMIC_STATE_VIEWPORT
	DynamicStateScissor            DynamicState = 1 // VK_DYNAMIC_STATE_SCISSOR
	DynamicStateLineWidth          DynamicState = 2 // VK_DYNAMIC_STATE_LINE_WIDTH
	DynamicStateDepthBias          DynamicState = 3 // VK_DYNAMIC_STATE_DEPTH_BIAS
	DynamicStateBlendConstants     DynamicState = 4 // VK_DYNAMIC_STATE_BLEND_CONSTANTS
	DynamicStateDepthBounds        DynamicState = 5 // VK_DYNAMIC_STATE_DEPTH_BOUNDS
	DynamicStateStencilCompareMask DynamicState = 6 // VK_DYNAMIC_STATE_STENCIL_COMPARE_MASK
	DynamicStateStencilWriteMask   DynamicState = 7 // VK_DYNAMIC_STATE_STENCIL_WRITE_MASK
	DynamicStateStencilReference   DynamicState = 8 // VK_DYNAMIC_STATE_STENCIL_REFERENCE
)

// DescriptorType is the kind of resource a descriptor names (VkDescriptorType).
type DescriptorType int32

const (
	DescriptorTypeSampler              DescriptorType = 0  // VK_DESCRIPTOR_TYPE_SAMPLER
	DescriptorTypeCombinedImageSampler DescriptorType = 1  // VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER
	DescriptorTypeSampledImage         DescriptorType = 2  // VK_DESCRIPTOR_TYPE_SAMPLED_IMAGE
	DescriptorTypeStorageImage         DescriptorType = 3  // VK_DESCRIPTOR_TYPE_STORAGE_IMAGE
	DescriptorTypeUniformTexelBuffer   DescriptorType = 4  // VK_DESCRIPTOR_TYPE_UNIFORM_TEXEL_BUFFER
	DescriptorTypeStorageTexelBuffer   DescriptorType = 5  // VK_DESCRIPTOR_TYPE_STORAGE_TEXEL_BUFFER
	DescriptorTypeUniformBuffer        DescriptorType = 6  // VK_DESCRIPTOR_TYPE_UNIFORM_BUFFER
	DescriptorTypeStorageBuffer        DescriptorType = 7  // VK_DESCRIPTOR_TYPE_STORAGE_BUFFER
	DescriptorTypeUniformBufferDynamic DescriptorType = 8  // VK_DESCRIPTOR_TYPE_UNIFORM_BUFFER_DYNAMIC
	DescriptorTypeStorageBufferDynamic DescriptorType = 9  // VK_DESCRIPTOR_TYPE_STORAGE_BUFFER_DYNAMIC
	DescriptorTypeInputAttachment      DescriptorType = 10 // VK_DESCRIPTOR_TYPE_INPUT_ATTACHMENT
)

// IndexType is the width of the indices of an index buffer (VkIndexType).
type IndexType int32

const (
	IndexTypeUint16 IndexType = 0 // VK_INDEX_TYPE_UINT16
	IndexTypeUint32 IndexType = 1 // VK_INDEX_TYPE_UINT32
)

// SubpassContents says where the commands of a subpass are recorded (VkSubpassContents).
type SubpassContents int32

const (
	SubpassContentsInline                  SubpassContents = 0 // VK_SUBPASS_CONTENTS_INLINE
	SubpassContentsSecondaryCommandBuffers SubpassContents = 1 // VK_SUBPASS_CONTENTS_SECONDARY_COMMAND_BUFFERS
)

// CommandBufferLevel is primary or secondary (VkCommandBufferLevel).
type CommandBufferLevel int32

const (
	CommandBufferLevelPrimary   CommandBufferLevel = 0 // VK_COMMAND_BUFFER_LEVEL_PRIMARY
	CommandBufferLevelSecondary CommandBufferLevel = 1 // VK_COMMAND_BUFFER_LEVEL_SECONDARY
)

// ObjectType is the kind of a handle, for debug names (VkObjectType).
type ObjectType int32

const (
	ObjectTypeUnknown             ObjectType = 0  // VK_OBJECT_TYPE_UNKNOWN
	ObjectTypeInstance            ObjectType = 1  // VK_OBJECT_TYPE_INSTANCE
	ObjectTypePhysicalDevice      ObjectType = 2  // VK_OBJECT_TYPE_PHYSICAL_DEVICE
	ObjectTypeDevice              ObjectType = 3  // VK_OBJECT_TYPE_DEVICE
	ObjectTypeQueue               ObjectType = 4  // VK_OBJECT_TYPE_QUEUE
	ObjectTypeSemaphore           ObjectType = 5  // VK_OBJECT_TYPE_SEMAPHORE
	ObjectTypeCommandBuffer       ObjectType = 6  // VK_OBJECT_TYPE_COMMAND_BUFFER
	ObjectTypeFence               ObjectType = 7  // VK_OBJECT_TYPE_FENCE
	ObjectTypeDeviceMemory        ObjectType = 8  // VK_OBJECT_TYPE_DEVICE_MEMORY
	ObjectTypeBuffer              ObjectType = 9  // VK_OBJECT_TYPE_BUFFER
	ObjectTypeImage               ObjectType = 10 // VK_OBJECT_TYPE_IMAGE
	ObjectTypeImageView           ObjectType = 14 // VK_OBJECT_TYPE_IMAGE_VIEW
	ObjectTypeShaderModule        ObjectType = 15 // VK_OBJECT_TYPE_SHADER_MODULE
	ObjectTypePipelineLayout      ObjectType = 17 // VK_OBJECT_TYPE_PIPELINE_LAYOUT
	ObjectTypeRenderPass          ObjectType = 18 // VK_OBJECT_TYPE_RENDER_PASS
	ObjectTypePipeline            ObjectType = 19 // VK_OBJECT_TYPE_PIPELINE
	ObjectTypeDescriptorSetLayout ObjectType = 20 // VK_OBJECT_TYPE_DESCRIPTOR_SET_LAYOUT
	ObjectTypeSampler             ObjectType = 21 // VK_OBJECT_TYPE_SAMPLER
	ObjectTypeDescriptorPool      ObjectType = 22 // VK_OBJECT_TYPE_DESCRIPTOR_POOL
	ObjectTypeDescriptorSet       ObjectType = 23 // VK_OBJECT_TYPE_DESCRIPTOR_SET
	ObjectTypeFramebuffer         ObjectType = 24 // VK_OBJECT_TYPE_FRAMEBUFFER
	ObjectTypeCommandPool         ObjectType = 25 // VK_OBJECT_TYPE_COMMAND_POOL
)

// QueueFlags are the capabilities of a queue family (VkQueueFlags).
type QueueFlags uint32

const (
	QueueGraphics      QueueFlags = 0x1 // VK_QUEUE_GRAPHICS_BIT
	QueueCompute       QueueFlags = 0x2 // VK_QUEUE_COMPUTE_BIT
	QueueTransfer      QueueFlags = 0x4 // VK_QUEUE_TRANSFER_BIT
	QueueSparseBinding QueueFlags = 0x8 // VK_QUEUE_SPARSE_BINDING_BIT
)

// MemoryPropertyFlags are the properties of a memory type (VkMemoryPropertyFlags).
type MemoryPropertyFlags uint32

const (
	MemoryPropertyDeviceLocal     MemoryPropertyFlags = 0x1  // VK_MEMORY_PROPERTY_DEVICE_LOCAL_BIT
	MemoryPropertyHostVisible     MemoryPropertyFlags = 0x2  // VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT
	MemoryPropertyHostCoherent    MemoryPropertyFlags = 0x4  // VK_MEMORY_PROPERTY_HOST_COHERENT_BIT
	MemoryPropertyHostCached      MemoryPropertyFlags = 0x8  // VK_MEMORY_PROPERTY_HOST_CACHED_BIT
	MemoryPropertyLazilyAllocated MemoryPropertyFlags = 0x10 // VK_MEMORY_PROPERTY_LAZILY_ALLOCATED_BIT
)

// MemoryHeapFlags are the properties of a memory heap (VkMemoryHeapFlags).
type MemoryHeapFlags uint32

const (
	MemoryHeapDeviceLocal MemoryHeapFlags = 0x1 // VK_MEMORY_HEAP_DEVICE_LOCAL_BIT
)

// FormatFeatureFlags are the uses a format supports (VkFormatFeatureFlags).
type FormatFeatureFlags uint32

const (
	FormatFeatureSampledImage             FormatFeatureFlags = 0x1    // VK_FORMAT_FEATURE_SAMPLED_IMAGE_BIT
	FormatFeatureStorageImage             FormatFeatureFlags = 0x2    // VK_FORMAT_FEATURE_STORAGE_IMAGE_BIT
	FormatFeatureVertexBuffer             FormatFeatureFlags = 0x40   // VK_FORMAT_FEATURE_VERTEX_BUFFER_BIT
	FormatFeatureColorAttachment          FormatFeatureFlags = 0x80   // VK_FORMAT_FEATURE_COLOR_ATTACHMENT_BIT
	FormatFeatureColorAttachmentBlend     FormatFeatureFlags = 0x100  // VK_FORMAT_FEATURE_COLOR_ATTACHMENT_BLEND_BIT
	FormatFeatureDepthStencilAttachment   FormatFeatureFlags = 0x200  // VK_FORMAT_FEATURE_DEPTH_STENCIL_ATTACHMENT_BIT
	FormatFeatureBlitSrc                  FormatFeatureFlags = 0x400  // VK_FORMAT_FEATURE_BLIT_SRC_BIT
	FormatFeatureBlitDst                  FormatFeatureFlags = 0x800  // VK_FORMAT_FEATURE_BLIT_DST_BIT
	FormatFeatureSampledImageFilterLinear FormatFeatureFlags = 0x1000 // VK_FORMAT_FEATURE_SAMPLED_IMAGE_FILTER_LINEAR_BIT
	FormatFeatureTransferSrc              FormatFeatureFlags = 0x4000 // VK_FORMAT_FEATURE_TRANSFER_SRC_BIT
	FormatFeatureTransferDst              FormatFeatureFlags = 0x8000 // VK_FORMAT_FEATURE_TRANSFER_DST_BIT
)

// SampleCountFlags are sample counts; one bit is a count (VkSampleCountFlags, VkSampleCountFlagBits).
type SampleCountFlags uint32

const (
	SampleCount1  SampleCountFlags = 0x1  // VK_SAMPLE_COUNT_1_BIT
	SampleCount2  SampleCountFlags = 0x2  // VK_SAMPLE_COUNT_2_BIT
	SampleCount4  SampleCountFlags = 0x4  // VK_SAMPLE_COUNT_4_BIT
	SampleCount8  SampleCountFlags = 0x8  // VK_SAMPLE_COUNT_8_BIT
	SampleCount16 SampleCountFlags = 0x10 // VK_SAMPLE_COUNT_16_BIT
)

// ImageUsageFlags are the uses of an image (VkImageUsageFlags).
type ImageUsageFlags uint32

const (
	ImageUsageTransferSrc            ImageUsageFlags = 0x1  // VK_IMAGE_USAGE_TRANSFER_SRC_BIT
	ImageUsageTransferDst            ImageUsageFlags = 0x2  // VK_IMAGE_USAGE_TRANSFER_DST_BIT
	ImageUsageSampled                ImageUsageFlags = 0x4  // VK_IMAGE_USAGE_SAMPLED_BIT
	ImageUsageStorage                ImageUsageFlags = 0x8  // VK_IMAGE_USAGE_STORAGE_BIT
	ImageUsageColorAttachment        ImageUsageFlags = 0x10 // VK_IMAGE_USAGE_COLOR_ATTACHMENT_BIT
	ImageUsageDepthStencilAttachment ImageUsageFlags = 0x20 // VK_IMAGE_USAGE_DEPTH_STENCIL_ATTACHMENT_BIT
	ImageUsageTransientAttachment    ImageUsageFlags = 0x40 // VK_IMAGE_USAGE_TRANSIENT_ATTACHMENT_BIT
	ImageUsageInputAttachment        ImageUsageFlags = 0x80 // VK_IMAGE_USAGE_INPUT_ATTACHMENT_BIT
)

// ImageCreateFlags are creation options of an image (VkImageCreateFlags).
type ImageCreateFlags uint32

const (
	ImageCreateMutableFormat  ImageCreateFlags = 0x8  // VK_IMAGE_CREATE_MUTABLE_FORMAT_BIT
	ImageCreateCubeCompatible ImageCreateFlags = 0x10 // VK_IMAGE_CREATE_CUBE_COMPATIBLE_BIT
)

// ImageAspectFlags select the color, depth or stencil part of an image (VkImageAspectFlags).
type ImageAspectFlags uint32

const (
	ImageAspectColor   ImageAspectFlags = 0x1 // VK_IMAGE_ASPECT_COLOR_BIT
	ImageAspectDepth   ImageAspectFlags = 0x2 // VK_IMAGE_ASPECT_DEPTH_BIT
	ImageAspectStencil ImageAspectFlags = 0x4 // VK_IMAGE_ASPECT_STENCIL_BIT
)

// BufferUsageFlags are the uses of a buffer (VkBufferUsageFlags).
type BufferUsageFlags uint32

const (
	BufferUsageTransferSrc        BufferUsageFlags = 0x1   // VK_BUFFER_USAGE_TRANSFER_SRC_BIT
	BufferUsageTransferDst        BufferUsageFlags = 0x2   // VK_BUFFER_USAGE_TRANSFER_DST_BIT
	BufferUsageUniformTexelBuffer BufferUsageFlags = 0x4   // VK_BUFFER_USAGE_UNIFORM_TEXEL_BUFFER_BIT
	BufferUsageStorageTexelBuffer BufferUsageFlags = 0x8   // VK_BUFFER_USAGE_STORAGE_TEXEL_BUFFER_BIT
	BufferUsageUniformBuffer      BufferUsageFlags = 0x10  // VK_BUFFER_USAGE_UNIFORM_BUFFER_BIT
	BufferUsageStorageBuffer      BufferUsageFlags = 0x20  // VK_BUFFER_USAGE_STORAGE_BUFFER_BIT
	BufferUsageIndexBuffer        BufferUsageFlags = 0x40  // VK_BUFFER_USAGE_INDEX_BUFFER_BIT
	BufferUsageVertexBuffer       BufferUsageFlags = 0x80  // VK_BUFFER_USAGE_VERTEX_BUFFER_BIT
	BufferUsageIndirectBuffer     BufferUsageFlags = 0x100 // VK_BUFFER_USAGE_INDIRECT_BUFFER_BIT
)

// ShaderStageFlags are shader stages; one bit is a stage (VkShaderStageFlags, VkShaderStageFlagBits).
type ShaderStageFlags uint32

const (
	ShaderStageVertex      ShaderStageFlags = 0x1        // VK_SHADER_STAGE_VERTEX_BIT
	ShaderStageFragment    ShaderStageFlags = 0x10       // VK_SHADER_STAGE_FRAGMENT_BIT
	ShaderStageCompute     ShaderStageFlags = 0x20       // VK_SHADER_STAGE_COMPUTE_BIT
	ShaderStageAllGraphics ShaderStageFlags = 0x1f       // VK_SHADER_STAGE_ALL_GRAPHICS
	ShaderStageAll         ShaderStageFlags = 0x7fffffff // VK_SHADER_STAGE_ALL
)

// PipelineStageFlags are pipeline stages, for barriers and waits (VkPipelineStageFlags).
type PipelineStageFlags uint32

const (
	PipelineStageTopOfPipe             PipelineStageFlags = 0x1        // VK_PIPELINE_STAGE_TOP_OF_PIPE_BIT
	PipelineStageDrawIndirect          PipelineStageFlags = 0x2        // VK_PIPELINE_STAGE_DRAW_INDIRECT_BIT
	PipelineStageVertexInput           PipelineStageFlags = 0x4        // VK_PIPELINE_STAGE_VERTEX_INPUT_BIT
	PipelineStageVertexShader          PipelineStageFlags = 0x8        // VK_PIPELINE_STAGE_VERTEX_SHADER_BIT
	PipelineStageFragmentShader        PipelineStageFlags = 0x80       // VK_PIPELINE_STAGE_FRAGMENT_SHADER_BIT
	PipelineStageEarlyFragmentTests    PipelineStageFlags = 0x100      // VK_PIPELINE_STAGE_EARLY_FRAGMENT_TESTS_BIT
	PipelineStageLateFragmentTests     PipelineStageFlags = 0x200      // VK_PIPELINE_STAGE_LATE_FRAGMENT_TESTS_BIT
	PipelineStageColorAttachmentOutput PipelineStageFlags = 0x400      // VK_PIPELINE_STAGE_COLOR_ATTACHMENT_OUTPUT_BIT
	PipelineStageComputeShader         PipelineStageFlags = 0x800      // VK_PIPELINE_STAGE_COMPUTE_SHADER_BIT
	PipelineStageTransfer              PipelineStageFlags = 0x1000     // VK_PIPELINE_STAGE_TRANSFER_BIT
	PipelineStageBottomOfPipe          PipelineStageFlags = 0x2000     // VK_PIPELINE_STAGE_BOTTOM_OF_PIPE_BIT
	PipelineStageHost                  PipelineStageFlags = 0x4000     // VK_PIPELINE_STAGE_HOST_BIT
	PipelineStageAllGraphics           PipelineStageFlags = 0x8000     // VK_PIPELINE_STAGE_ALL_GRAPHICS_BIT
	PipelineStageAllCommands           PipelineStageFlags = 0x00010000 // VK_PIPELINE_STAGE_ALL_COMMANDS_BIT
)

// AccessFlags are kinds of memory access, for barriers (VkAccessFlags).
type AccessFlags uint32

const (
	AccessIndirectCommandRead         AccessFlags = 0x1        // VK_ACCESS_INDIRECT_COMMAND_READ_BIT
	AccessIndexRead                   AccessFlags = 0x2        // VK_ACCESS_INDEX_READ_BIT
	AccessVertexAttributeRead         AccessFlags = 0x4        // VK_ACCESS_VERTEX_ATTRIBUTE_READ_BIT
	AccessUniformRead                 AccessFlags = 0x8        // VK_ACCESS_UNIFORM_READ_BIT
	AccessInputAttachmentRead         AccessFlags = 0x10       // VK_ACCESS_INPUT_ATTACHMENT_READ_BIT
	AccessShaderRead                  AccessFlags = 0x20       // VK_ACCESS_SHADER_READ_BIT
	AccessShaderWrite                 AccessFlags = 0x40       // VK_ACCESS_SHADER_WRITE_BIT
	AccessColorAttachmentRead         AccessFlags = 0x80       // VK_ACCESS_COLOR_ATTACHMENT_READ_BIT
	AccessColorAttachmentWrite        AccessFlags = 0x100      // VK_ACCESS_COLOR_ATTACHMENT_WRITE_BIT
	AccessDepthStencilAttachmentRead  AccessFlags = 0x200      // VK_ACCESS_DEPTH_STENCIL_ATTACHMENT_READ_BIT
	AccessDepthStencilAttachmentWrite AccessFlags = 0x400      // VK_ACCESS_DEPTH_STENCIL_ATTACHMENT_WRITE_BIT
	AccessTransferRead                AccessFlags = 0x800      // VK_ACCESS_TRANSFER_READ_BIT
	AccessTransferWrite               AccessFlags = 0x1000     // VK_ACCESS_TRANSFER_WRITE_BIT
	AccessHostRead                    AccessFlags = 0x2000     // VK_ACCESS_HOST_READ_BIT
	AccessHostWrite                   AccessFlags = 0x4000     // VK_ACCESS_HOST_WRITE_BIT
	AccessMemoryRead                  AccessFlags = 0x8000     // VK_ACCESS_MEMORY_READ_BIT
	AccessMemoryWrite                 AccessFlags = 0x00010000 // VK_ACCESS_MEMORY_WRITE_BIT
)

// DependencyFlags are options of a dependency or barrier (VkDependencyFlags).
type DependencyFlags uint32

const (
	DependencyByRegion DependencyFlags = 0x1 // VK_DEPENDENCY_BY_REGION_BIT
)

// ColorComponentFlags select the color components a pipeline writes (VkColorComponentFlags).
type ColorComponentFlags uint32

const (
	ColorComponentR ColorComponentFlags = 0x1 // VK_COLOR_COMPONENT_R_BIT
	ColorComponentG ColorComponentFlags = 0x2 // VK_COLOR_COMPONENT_G_BIT
	ColorComponentB ColorComponentFlags = 0x4 // VK_COLOR_COMPONENT_B_BIT
	ColorComponentA ColorComponentFlags = 0x8 // VK_COLOR_COMPONENT_A_BIT
)

// CullModeFlags select the faces a pipeline culls (VkCullModeFlags).
type CullModeFlags uint32

const (
	CullModeNone  CullModeFlags = 0x0 // VK_CULL_MODE_NONE
	CullModeFront CullModeFlags = 0x1 // VK_CULL_MODE_FRONT_BIT
	CullModeBack  CullModeFlags = 0x2 // VK_CULL_MODE_BACK_BIT
)

// CommandPoolCreateFlags are creation options of a command pool (VkCommandPoolCreateFlags).
type CommandPoolCreateFlags uint32

const (
	CommandPoolCreateTransient          CommandPoolCreateFlags = 0x1 // VK_COMMAND_POOL_CREATE_TRANSIENT_BIT
	CommandPoolCreateResetCommandBuffer CommandPoolCreateFlags = 0x2 // VK_COMMAND_POOL_CREATE_RESET_COMMAND_BUFFER_BIT
)

// CommandPoolResetFlags are options of ResetCommandPool (VkCommandPoolResetFlags).
type CommandPoolResetFlags uint32

const (
	CommandPoolResetReleaseResources CommandPoolResetFlags = 0x1 // VK_COMMAND_POOL_RESET_RELEASE_RESOURCES_BIT
)

// CommandBufferUsageFlags say how a command buffer will be used (VkCommandBufferUsageFlags).
type CommandBufferUsageFlags uint32

const (
	CommandBufferUsageOneTimeSubmit CommandBufferUsageFlags = 0x1 // VK_COMMAND_BUFFER_USAGE_ONE_TIME_SUBMIT_BIT
)

// CommandBufferResetFlags are options of ResetCommandBuffer (VkCommandBufferResetFlags).
type CommandBufferResetFlags uint32

const (
	CommandBufferResetReleaseResources CommandBufferResetFlags = 0x1 // VK_COMMAND_BUFFER_RESET_RELEASE_RESOURCES_BIT
)

// DescriptorPoolCreateFlags are creation options of a descriptor pool (VkDescriptorPoolCreateFlags).
type DescriptorPoolCreateFlags uint32

const (
	DescriptorPoolCreateFreeDescriptorSet DescriptorPoolCreateFlags = 0x1 // VK_DESCRIPTOR_POOL_CREATE_FREE_DESCRIPTOR_SET_BIT
)

// FenceCreateFlags are creation options of a fence (VkFenceCreateFlags).
type FenceCreateFlags uint32

const (
	FenceCreateSignaled FenceCreateFlags = 0x1 // VK_FENCE_CREATE_SIGNALED_BIT
)

// DebugUtilsMessageSeverityFlagsEXT are severities of debug messages (VkDebugUtilsMessageSeverityFlagsEXT).
type DebugUtilsMessageSeverityFlagsEXT uint32

const (
	DebugUtilsMessageSeverityVerboseEXT DebugUtilsMessageSeverityFlagsEXT = 0x1    // VK_DEBUG_UTILS_MESSAGE_SEVERITY_VERBOSE_BIT_EXT
	DebugUtilsMessageSeverityInfoEXT    DebugUtilsMessageSeverityFlagsEXT = 0x10   // VK_DEBUG_UTILS_MESSAGE_SEVERITY_INFO_BIT_EXT
	DebugUtilsMessageSeverityWarningEXT DebugUtilsMessageSeverityFlagsEXT = 0x100  // VK_DEBUG_UTILS_MESSAGE_SEVERITY_WARNING_BIT_EXT
	DebugUtilsMessageSeverityErrorEXT   DebugUtilsMessageSeverityFlagsEXT = 0x1000 // VK_DEBUG_UTILS_MESSAGE_SEVERITY_ERROR_BIT_EXT
)

// DebugUtilsMessageTypeFlagsEXT are kinds of debug messages (VkDebugUtilsMessageTypeFlagsEXT).
type DebugUtilsMessageTypeFlagsEXT uint32

const (
	DebugUtilsMessageTypeGeneralEXT     DebugUtilsMessageTypeFlagsEXT = 0x1 // VK_DEBUG_UTILS_MESSAGE_TYPE_GENERAL_BIT_EXT
	DebugUtilsMessageTypeValidationEXT  DebugUtilsMessageTypeFlagsEXT = 0x2 // VK_DEBUG_UTILS_MESSAGE_TYPE_VALIDATION_BIT_EXT
	DebugUtilsMessageTypePerformanceEXT DebugUtilsMessageTypeFlagsEXT = 0x4 // VK_DEBUG_UTILS_MESSAGE_TYPE_PERFORMANCE_BIT_EXT
)

// --- Exporting images as dmabufs (ADR-281, ADR-283): external memory and
// dedicated allocations (Vulkan 1.1), VK_KHR_external_memory_fd,
// VK_EXT_external_memory_dma_buf, VK_EXT_image_drm_format_modifier and
// VK_EXT_queue_family_foreign. ---

// Names of the extensions of the dmabuf export.
const (
	KhrExternalMemoryFdExtensionName       = "VK_KHR_external_memory_fd"        // VK_KHR_EXTERNAL_MEMORY_FD_EXTENSION_NAME
	ExtExternalMemoryDmaBufExtensionName   = "VK_EXT_external_memory_dma_buf"   // VK_EXT_EXTERNAL_MEMORY_DMA_BUF_EXTENSION_NAME
	ExtImageDrmFormatModifierExtensionName = "VK_EXT_image_drm_format_modifier" // VK_EXT_IMAGE_DRM_FORMAT_MODIFIER_EXTENSION_NAME
	KhrImageFormatListExtensionName        = "VK_KHR_image_format_list"         // VK_KHR_IMAGE_FORMAT_LIST_EXTENSION_NAME
	ExtQueueFamilyForeignExtensionName     = "VK_EXT_queue_family_foreign"      // VK_EXT_QUEUE_FAMILY_FOREIGN_EXTENSION_NAME
)

// The queue families of the other side of a shared image, for the
// ownership transfers of barriers: another Vulkan instance or driver
// ([QueueFamilyExternal]), or anything else, such as a compositor
// ([QueueFamilyForeignEXT], VK_EXT_queue_family_foreign).
const (
	QueueFamilyExternal   uint32 = 1<<32 - 2 // VK_QUEUE_FAMILY_EXTERNAL
	QueueFamilyForeignEXT uint32 = 1<<32 - 3 // VK_QUEUE_FAMILY_FOREIGN_EXT
)

// The structure types of the dmabuf export.
const (
	StructureTypeFormatProperties2                           StructureType = 1000059002 // VK_STRUCTURE_TYPE_FORMAT_PROPERTIES_2
	StructureTypeImageFormatProperties2                      StructureType = 1000059003 // VK_STRUCTURE_TYPE_IMAGE_FORMAT_PROPERTIES_2
	StructureTypePhysicalDeviceImageFormatInfo2              StructureType = 1000059004 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_IMAGE_FORMAT_INFO_2
	StructureTypePhysicalDeviceExternalImageFormatInfo       StructureType = 1000071000 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_EXTERNAL_IMAGE_FORMAT_INFO
	StructureTypeExternalImageFormatProperties               StructureType = 1000071001 // VK_STRUCTURE_TYPE_EXTERNAL_IMAGE_FORMAT_PROPERTIES
	StructureTypeExternalMemoryImageCreateInfo               StructureType = 1000072001 // VK_STRUCTURE_TYPE_EXTERNAL_MEMORY_IMAGE_CREATE_INFO
	StructureTypeExportMemoryAllocateInfo                    StructureType = 1000072002 // VK_STRUCTURE_TYPE_EXPORT_MEMORY_ALLOCATE_INFO
	StructureTypeMemoryGetFdInfoKHR                          StructureType = 1000074002 // VK_STRUCTURE_TYPE_MEMORY_GET_FD_INFO_KHR
	StructureTypeMemoryDedicatedAllocateInfo                 StructureType = 1000127001 // VK_STRUCTURE_TYPE_MEMORY_DEDICATED_ALLOCATE_INFO
	StructureTypeDrmFormatModifierPropertiesListEXT          StructureType = 1000158000 // VK_STRUCTURE_TYPE_DRM_FORMAT_MODIFIER_PROPERTIES_LIST_EXT
	StructureTypePhysicalDeviceImageDrmFormatModifierInfoEXT StructureType = 1000158002 // VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_IMAGE_DRM_FORMAT_MODIFIER_INFO_EXT
	StructureTypeImageDrmFormatModifierListCreateInfoEXT     StructureType = 1000158003 // VK_STRUCTURE_TYPE_IMAGE_DRM_FORMAT_MODIFIER_LIST_CREATE_INFO_EXT
	StructureTypeImageDrmFormatModifierPropertiesEXT         StructureType = 1000158005 // VK_STRUCTURE_TYPE_IMAGE_DRM_FORMAT_MODIFIER_PROPERTIES_EXT
)

// The memory planes of an image with a DRM format modifier, for
// vkGetImageSubresourceLayout (VK_EXT_image_drm_format_modifier).
const (
	ImageAspectMemoryPlane0EXT ImageAspectFlags = 0x80  // VK_IMAGE_ASPECT_MEMORY_PLANE_0_BIT_EXT
	ImageAspectMemoryPlane1EXT ImageAspectFlags = 0x100 // VK_IMAGE_ASPECT_MEMORY_PLANE_1_BIT_EXT
	ImageAspectMemoryPlane2EXT ImageAspectFlags = 0x200 // VK_IMAGE_ASPECT_MEMORY_PLANE_2_BIT_EXT
	ImageAspectMemoryPlane3EXT ImageAspectFlags = 0x400 // VK_IMAGE_ASPECT_MEMORY_PLANE_3_BIT_EXT
)

// ExternalMemoryHandleTypeFlags are kinds of handles to memory shared with
// other APIs and processes; one bit is a kind
// (VkExternalMemoryHandleTypeFlags, VkExternalMemoryHandleTypeFlagBits).
type ExternalMemoryHandleTypeFlags uint32

const (
	ExternalMemoryHandleTypeOpaqueFd  ExternalMemoryHandleTypeFlags = 0x1   // VK_EXTERNAL_MEMORY_HANDLE_TYPE_OPAQUE_FD_BIT
	ExternalMemoryHandleTypeDmaBufEXT ExternalMemoryHandleTypeFlags = 0x200 // VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT_EXT
)

// ExternalMemoryFeatureFlags say what an image or buffer with a handle
// type can do (VkExternalMemoryFeatureFlags).
type ExternalMemoryFeatureFlags uint32

const (
	ExternalMemoryFeatureDedicatedOnly ExternalMemoryFeatureFlags = 0x1 // VK_EXTERNAL_MEMORY_FEATURE_DEDICATED_ONLY_BIT
	ExternalMemoryFeatureExportable    ExternalMemoryFeatureFlags = 0x2 // VK_EXTERNAL_MEMORY_FEATURE_EXPORTABLE_BIT
	ExternalMemoryFeatureImportable    ExternalMemoryFeatureFlags = 0x4 // VK_EXTERNAL_MEMORY_FEATURE_IMPORTABLE_BIT
)
