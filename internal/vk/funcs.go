//go:build telagpu && (linux || windows)

package vk

import (
	"fmt"
	"reflect"
)

// The entry points of the bindings. A field is the address of the command
// named vk plus the field's name; `vk:"optional"` marks the commands of
// extensions and of later versions, which stay zero when the loader, the
// instance or the device does not have them. Add a command here and its
// wrapper in instance.go, device.go or commands.go.

// globalFuncs are the commands that need no instance.
type globalFuncs struct {
	CreateInstance                       uintptr
	EnumerateInstanceExtensionProperties uintptr
	EnumerateInstanceLayerProperties     uintptr
	EnumerateInstanceVersion             uintptr `vk:"optional"` // Vulkan 1.1
}

// instanceFuncs are the commands that dispatch on an instance or a
// physical device.
type instanceFuncs struct {
	DestroyInstance                        uintptr
	EnumeratePhysicalDevices               uintptr
	GetPhysicalDeviceProperties            uintptr
	GetPhysicalDeviceProperties2           uintptr `vk:"optional"` // Vulkan 1.1
	GetPhysicalDeviceFeatures              uintptr
	GetPhysicalDeviceQueueFamilyProperties uintptr
	GetPhysicalDeviceMemoryProperties      uintptr
	GetPhysicalDeviceFormatProperties      uintptr
	GetPhysicalDeviceImageFormatProperties uintptr
	EnumerateDeviceExtensionProperties     uintptr
	CreateDevice                           uintptr
	GetDeviceProcAddr                      uintptr

	CreateDebugUtilsMessengerEXT  uintptr `vk:"optional"` // VK_EXT_debug_utils
	DestroyDebugUtilsMessengerEXT uintptr `vk:"optional"`

	// Exporting images as dmabufs (dmabuf.go).
	GetPhysicalDeviceFormatProperties2      uintptr `vk:"optional"` // Vulkan 1.1
	GetPhysicalDeviceImageFormatProperties2 uintptr `vk:"optional"` // Vulkan 1.1
}

// deviceFuncs are the commands that dispatch on a device, a queue or a
// command buffer.
type deviceFuncs struct {
	DestroyDevice  uintptr
	DeviceWaitIdle uintptr
	GetDeviceQueue uintptr
	QueueSubmit    uintptr
	QueueWaitIdle  uintptr

	CreateCommandPool      uintptr
	DestroyCommandPool     uintptr
	ResetCommandPool       uintptr
	AllocateCommandBuffers uintptr
	FreeCommandBuffers     uintptr
	BeginCommandBuffer     uintptr
	EndCommandBuffer       uintptr
	ResetCommandBuffer     uintptr

	AllocateMemory               uintptr
	FreeMemory                   uintptr
	MapMemory                    uintptr
	UnmapMemory                  uintptr
	FlushMappedMemoryRanges      uintptr
	InvalidateMappedMemoryRanges uintptr

	CreateBuffer                uintptr
	DestroyBuffer               uintptr
	GetBufferMemoryRequirements uintptr
	BindBufferMemory            uintptr
	CreateImage                 uintptr
	DestroyImage                uintptr
	GetImageMemoryRequirements  uintptr
	BindImageMemory             uintptr
	CreateImageView             uintptr
	DestroyImageView            uintptr
	CreateSampler               uintptr
	DestroySampler              uintptr

	CreateRenderPass   uintptr
	DestroyRenderPass  uintptr
	CreateFramebuffer  uintptr
	DestroyFramebuffer uintptr

	CreateShaderModule         uintptr
	DestroyShaderModule        uintptr
	CreateDescriptorSetLayout  uintptr
	DestroyDescriptorSetLayout uintptr
	CreateDescriptorPool       uintptr
	DestroyDescriptorPool      uintptr
	ResetDescriptorPool        uintptr
	AllocateDescriptorSets     uintptr
	FreeDescriptorSets         uintptr
	UpdateDescriptorSets       uintptr
	CreatePipelineLayout       uintptr
	DestroyPipelineLayout      uintptr
	CreateGraphicsPipelines    uintptr
	DestroyPipeline            uintptr

	CreateFence      uintptr
	DestroyFence     uintptr
	ResetFences      uintptr
	GetFenceStatus   uintptr
	WaitForFences    uintptr
	CreateSemaphore  uintptr
	DestroySemaphore uintptr

	CmdBeginRenderPass    uintptr
	CmdEndRenderPass      uintptr
	CmdBindPipeline       uintptr
	CmdSetViewport        uintptr
	CmdSetScissor         uintptr
	CmdSetBlendConstants  uintptr
	CmdBindDescriptorSets uintptr
	CmdBindVertexBuffers  uintptr
	CmdBindIndexBuffer    uintptr
	CmdPushConstants      uintptr
	CmdDraw               uintptr
	CmdDrawIndexed        uintptr
	CmdClearAttachments   uintptr
	CmdClearColorImage    uintptr
	CmdCopyBuffer         uintptr
	CmdCopyImage          uintptr
	CmdBlitImage          uintptr
	CmdCopyBufferToImage  uintptr
	CmdCopyImageToBuffer  uintptr
	CmdPipelineBarrier    uintptr

	SetDebugUtilsObjectNameEXT uintptr `vk:"optional"` // VK_EXT_debug_utils

	// Exporting images as dmabufs (dmabuf.go).
	GetImageSubresourceLayout              uintptr
	GetMemoryFdKHR                         uintptr `vk:"optional"` // VK_KHR_external_memory_fd
	GetImageDrmFormatModifierPropertiesEXT uintptr `vk:"optional"` // VK_EXT_image_drm_format_modifier
}

// loadFuncs fills table, a pointer to one of the structs above, with what
// lookup returns for each command. A command that is not optional and is
// missing is an error.
func loadFuncs(table any, lookup func(name string) uintptr) error {
	v := reflect.ValueOf(table).Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		addr := lookup("vk" + f.Name)
		if addr == 0 && f.Tag.Get("vk") != "optional" {
			return fmt.Errorf("vk: the Vulkan implementation has no vk%s", f.Name)
		}
		v.Field(i).SetUint(uint64(addr))
	}
	return nil
}
