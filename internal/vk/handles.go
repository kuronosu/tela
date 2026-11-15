//go:build telagpu && (linux || windows)

package vk

import "unsafe"

// The bindings assume a 64-bit target (ARCHITECTURE §18 builds no other):
// handles and VkDeviceSize are 8 bytes and every struct has one layout.
// This fails to compile on a 32-bit one.
var _ [unsafe.Sizeof(uintptr(0)) - 8]struct{}

// Dispatchable handles are pointers to the loader's objects. An instance
// and a device are wrapped by [Instance] and [Device], which hold their
// entry points.
type (
	// InstanceHandle is VkInstance.
	InstanceHandle uintptr
	// PhysicalDevice is VkPhysicalDevice.
	PhysicalDevice uintptr
	// DeviceHandle is VkDevice.
	DeviceHandle uintptr
	// Queue is VkQueue.
	Queue uintptr
	// CommandBuffer is VkCommandBuffer.
	CommandBuffer uintptr
)

// Non-dispatchable handles are 64-bit values that only their device (or
// instance) understands. Zero is VK_NULL_HANDLE.
type (
	// Semaphore is VkSemaphore.
	Semaphore uint64
	// Fence is VkFence.
	Fence uint64
	// DeviceMemory is VkDeviceMemory.
	DeviceMemory uint64
	// Buffer is VkBuffer.
	Buffer uint64
	// BufferView is VkBufferView.
	BufferView uint64
	// Image is VkImage.
	Image uint64
	// ImageView is VkImageView.
	ImageView uint64
	// Sampler is VkSampler.
	Sampler uint64
	// ShaderModule is VkShaderModule.
	ShaderModule uint64
	// PipelineCache is VkPipelineCache.
	PipelineCache uint64
	// PipelineLayout is VkPipelineLayout.
	PipelineLayout uint64
	// Pipeline is VkPipeline.
	Pipeline uint64
	// RenderPass is VkRenderPass.
	RenderPass uint64
	// Framebuffer is VkFramebuffer.
	Framebuffer uint64
	// DescriptorSetLayout is VkDescriptorSetLayout.
	DescriptorSetLayout uint64
	// DescriptorPool is VkDescriptorPool.
	DescriptorPool uint64
	// DescriptorSet is VkDescriptorSet.
	DescriptorSet uint64
	// CommandPool is VkCommandPool.
	CommandPool uint64
	// DebugUtilsMessengerEXT is VkDebugUtilsMessengerEXT.
	DebugUtilsMessengerEXT uint64
)
