//go:build telagpu && (linux || windows)

package vk

import "unsafe"

// Device is a VkDevice with the entry points of its commands. Its methods
// are the device-level commands, the ones on a [Queue] and, in
// commands.go, the ones that record into a [CommandBuffer].
type Device struct {
	h  DeviceHandle
	fn deviceFuncs
}

// Handle returns the VkDevice.
func (d *Device) Handle() DeviceHandle { return d.h }

// Destroy is vkDestroyDevice. Every object of the device must be
// destroyed before, and the device idle.
func (d *Device) Destroy() {
	call2(d.fn.DestroyDevice, uintptr(d.h), 0)
	d.h = 0
}

// WaitIdle is vkDeviceWaitIdle.
func (d *Device) WaitIdle() error {
	r := call1(d.fn.DeviceWaitIdle, uintptr(d.h))
	return result(r)
}

// GetQueue is vkGetDeviceQueue: queue index of the family, which the
// device was created with.
func (d *Device) GetQueue(family, index uint32) Queue {
	var q Queue
	call4(d.fn.GetDeviceQueue, uintptr(d.h), uintptr(family), uintptr(index), uintptr(unsafe.Pointer(&q)))
	return q
}

// QueueSubmit is vkQueueSubmit. fence may be zero.
func (d *Device) QueueSubmit(q Queue, submits []SubmitInfo, fence Fence) error {
	r := call4(d.fn.QueueSubmit, uintptr(q), uintptr(len(submits)), uintptr(unsafe.Pointer(unsafe.SliceData(submits))), uintptr(fence))
	return result(r)
}

// QueueWaitIdle is vkQueueWaitIdle.
func (d *Device) QueueWaitIdle(q Queue) error {
	r := call1(d.fn.QueueWaitIdle, uintptr(q))
	return result(r)
}

// Command pools and buffers.

// CreateCommandPool is vkCreateCommandPool.
func (d *Device) CreateCommandPool(info *CommandPoolCreateInfo) (CommandPool, error) {
	var h CommandPool
	r := call4(d.fn.CreateCommandPool, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyCommandPool is vkDestroyCommandPool. It frees the pool's command
// buffers.
func (d *Device) DestroyCommandPool(pool CommandPool) {
	call3(d.fn.DestroyCommandPool, uintptr(d.h), uintptr(pool), 0)
}

// ResetCommandPool is vkResetCommandPool.
func (d *Device) ResetCommandPool(pool CommandPool, flags CommandPoolResetFlags) error {
	r := call3(d.fn.ResetCommandPool, uintptr(d.h), uintptr(pool), uintptr(flags))
	return result(r)
}

// AllocateCommandBuffers is vkAllocateCommandBuffers: it returns
// info.CommandBufferCount command buffers.
func (d *Device) AllocateCommandBuffers(info *CommandBufferAllocateInfo) ([]CommandBuffer, error) {
	if info.CommandBufferCount == 0 {
		return nil, nil
	}
	out := make([]CommandBuffer, info.CommandBufferCount)
	r := call3(d.fn.AllocateCommandBuffers, uintptr(d.h), uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(&out[0])))
	if err := result(r); err != nil {
		return nil, err
	}
	return out, nil
}

// FreeCommandBuffers is vkFreeCommandBuffers.
func (d *Device) FreeCommandBuffers(pool CommandPool, buffers []CommandBuffer) {
	call4(d.fn.FreeCommandBuffers, uintptr(d.h), uintptr(pool), uintptr(len(buffers)), uintptr(unsafe.Pointer(unsafe.SliceData(buffers))))
}

// BeginCommandBuffer is vkBeginCommandBuffer.
func (d *Device) BeginCommandBuffer(cb CommandBuffer, info *CommandBufferBeginInfo) error {
	r := call2(d.fn.BeginCommandBuffer, uintptr(cb), uintptr(unsafe.Pointer(info)))
	return result(r)
}

// EndCommandBuffer is vkEndCommandBuffer.
func (d *Device) EndCommandBuffer(cb CommandBuffer) error {
	r := call1(d.fn.EndCommandBuffer, uintptr(cb))
	return result(r)
}

// ResetCommandBuffer is vkResetCommandBuffer. The buffer's pool must have
// been created with [CommandPoolCreateResetCommandBuffer].
func (d *Device) ResetCommandBuffer(cb CommandBuffer, flags CommandBufferResetFlags) error {
	r := call2(d.fn.ResetCommandBuffer, uintptr(cb), uintptr(flags))
	return result(r)
}

// Memory.

// AllocateMemory is vkAllocateMemory.
func (d *Device) AllocateMemory(info *MemoryAllocateInfo) (DeviceMemory, error) {
	var h DeviceMemory
	r := call4(d.fn.AllocateMemory, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// FreeMemory is vkFreeMemory. It unmaps the memory.
func (d *Device) FreeMemory(mem DeviceMemory) {
	call3(d.fn.FreeMemory, uintptr(d.h), uintptr(mem), 0)
}

// MapMemory is vkMapMemory: a pointer to size bytes of the memory from
// offset on ([WholeSize] for the rest of it), valid until UnmapMemory or
// FreeMemory. The memory is not Go memory; view it with unsafe.Slice.
func (d *Device) MapMemory(mem DeviceMemory, offset, size uint64, flags uint32) (unsafe.Pointer, error) {
	var p unsafe.Pointer
	r := call6(d.fn.MapMemory, uintptr(d.h), uintptr(mem), uintptr(offset), uintptr(size), uintptr(flags), uintptr(unsafe.Pointer(&p)))
	if err := result(r); err != nil {
		return nil, err
	}
	return p, nil
}

// UnmapMemory is vkUnmapMemory.
func (d *Device) UnmapMemory(mem DeviceMemory) {
	call2(d.fn.UnmapMemory, uintptr(d.h), uintptr(mem))
}

// FlushMappedMemoryRanges is vkFlushMappedMemoryRanges: it makes writes
// through a mapping visible to the device. Memory without
// [MemoryPropertyHostCoherent] needs it.
func (d *Device) FlushMappedMemoryRanges(ranges []MappedMemoryRange) error {
	r := call3(d.fn.FlushMappedMemoryRanges, uintptr(d.h), uintptr(len(ranges)), uintptr(unsafe.Pointer(unsafe.SliceData(ranges))))
	return result(r)
}

// InvalidateMappedMemoryRanges is vkInvalidateMappedMemoryRanges: it makes
// writes of the device visible through a mapping. Memory without
// [MemoryPropertyHostCoherent] needs it.
func (d *Device) InvalidateMappedMemoryRanges(ranges []MappedMemoryRange) error {
	r := call3(d.fn.InvalidateMappedMemoryRanges, uintptr(d.h), uintptr(len(ranges)), uintptr(unsafe.Pointer(unsafe.SliceData(ranges))))
	return result(r)
}

// Buffers, images, views and samplers.

// CreateBuffer is vkCreateBuffer.
func (d *Device) CreateBuffer(info *BufferCreateInfo) (Buffer, error) {
	var h Buffer
	r := call4(d.fn.CreateBuffer, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyBuffer is vkDestroyBuffer.
func (d *Device) DestroyBuffer(b Buffer) {
	call3(d.fn.DestroyBuffer, uintptr(d.h), uintptr(b), 0)
}

// GetBufferMemoryRequirements is vkGetBufferMemoryRequirements.
func (d *Device) GetBufferMemoryRequirements(b Buffer) MemoryRequirements {
	p := new(MemoryRequirements)
	call3(d.fn.GetBufferMemoryRequirements, uintptr(d.h), uintptr(b), uintptr(unsafe.Pointer(p)))
	return *p
}

// BindBufferMemory is vkBindBufferMemory.
func (d *Device) BindBufferMemory(b Buffer, mem DeviceMemory, offset uint64) error {
	r := call4(d.fn.BindBufferMemory, uintptr(d.h), uintptr(b), uintptr(mem), uintptr(offset))
	return result(r)
}

// CreateImage is vkCreateImage.
func (d *Device) CreateImage(info *ImageCreateInfo) (Image, error) {
	var h Image
	r := call4(d.fn.CreateImage, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyImage is vkDestroyImage.
func (d *Device) DestroyImage(img Image) {
	call3(d.fn.DestroyImage, uintptr(d.h), uintptr(img), 0)
}

// GetImageMemoryRequirements is vkGetImageMemoryRequirements.
func (d *Device) GetImageMemoryRequirements(img Image) MemoryRequirements {
	p := new(MemoryRequirements)
	call3(d.fn.GetImageMemoryRequirements, uintptr(d.h), uintptr(img), uintptr(unsafe.Pointer(p)))
	return *p
}

// BindImageMemory is vkBindImageMemory.
func (d *Device) BindImageMemory(img Image, mem DeviceMemory, offset uint64) error {
	r := call4(d.fn.BindImageMemory, uintptr(d.h), uintptr(img), uintptr(mem), uintptr(offset))
	return result(r)
}

// CreateImageView is vkCreateImageView.
func (d *Device) CreateImageView(info *ImageViewCreateInfo) (ImageView, error) {
	var h ImageView
	r := call4(d.fn.CreateImageView, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyImageView is vkDestroyImageView.
func (d *Device) DestroyImageView(v ImageView) {
	call3(d.fn.DestroyImageView, uintptr(d.h), uintptr(v), 0)
}

// CreateSampler is vkCreateSampler.
func (d *Device) CreateSampler(info *SamplerCreateInfo) (Sampler, error) {
	var h Sampler
	r := call4(d.fn.CreateSampler, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroySampler is vkDestroySampler.
func (d *Device) DestroySampler(s Sampler) {
	call3(d.fn.DestroySampler, uintptr(d.h), uintptr(s), 0)
}

// Render passes and framebuffers.

// CreateRenderPass is vkCreateRenderPass.
func (d *Device) CreateRenderPass(info *RenderPassCreateInfo) (RenderPass, error) {
	var h RenderPass
	r := call4(d.fn.CreateRenderPass, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyRenderPass is vkDestroyRenderPass.
func (d *Device) DestroyRenderPass(p RenderPass) {
	call3(d.fn.DestroyRenderPass, uintptr(d.h), uintptr(p), 0)
}

// CreateFramebuffer is vkCreateFramebuffer.
func (d *Device) CreateFramebuffer(info *FramebufferCreateInfo) (Framebuffer, error) {
	var h Framebuffer
	r := call4(d.fn.CreateFramebuffer, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyFramebuffer is vkDestroyFramebuffer.
func (d *Device) DestroyFramebuffer(f Framebuffer) {
	call3(d.fn.DestroyFramebuffer, uintptr(d.h), uintptr(f), 0)
}

// Shaders, descriptors and pipelines.

// CreateShaderModule is vkCreateShaderModule. CodeSize is in bytes and
// PCode points to words (see [SPIRV]).
func (d *Device) CreateShaderModule(info *ShaderModuleCreateInfo) (ShaderModule, error) {
	var h ShaderModule
	r := call4(d.fn.CreateShaderModule, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyShaderModule is vkDestroyShaderModule.
func (d *Device) DestroyShaderModule(m ShaderModule) {
	call3(d.fn.DestroyShaderModule, uintptr(d.h), uintptr(m), 0)
}

// CreateDescriptorSetLayout is vkCreateDescriptorSetLayout.
func (d *Device) CreateDescriptorSetLayout(info *DescriptorSetLayoutCreateInfo) (DescriptorSetLayout, error) {
	var h DescriptorSetLayout
	r := call4(d.fn.CreateDescriptorSetLayout, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyDescriptorSetLayout is vkDestroyDescriptorSetLayout.
func (d *Device) DestroyDescriptorSetLayout(l DescriptorSetLayout) {
	call3(d.fn.DestroyDescriptorSetLayout, uintptr(d.h), uintptr(l), 0)
}

// CreateDescriptorPool is vkCreateDescriptorPool.
func (d *Device) CreateDescriptorPool(info *DescriptorPoolCreateInfo) (DescriptorPool, error) {
	var h DescriptorPool
	r := call4(d.fn.CreateDescriptorPool, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyDescriptorPool is vkDestroyDescriptorPool. It frees the pool's
// sets.
func (d *Device) DestroyDescriptorPool(p DescriptorPool) {
	call3(d.fn.DestroyDescriptorPool, uintptr(d.h), uintptr(p), 0)
}

// ResetDescriptorPool is vkResetDescriptorPool: it frees every set of the
// pool.
func (d *Device) ResetDescriptorPool(p DescriptorPool) error {
	r := call3(d.fn.ResetDescriptorPool, uintptr(d.h), uintptr(p), 0)
	return result(r)
}

// AllocateDescriptorSets is vkAllocateDescriptorSets: it returns
// info.DescriptorSetCount sets, one per layout of info.PSetLayouts.
func (d *Device) AllocateDescriptorSets(info *DescriptorSetAllocateInfo) ([]DescriptorSet, error) {
	if info.DescriptorSetCount == 0 {
		return nil, nil
	}
	out := make([]DescriptorSet, info.DescriptorSetCount)
	r := call3(d.fn.AllocateDescriptorSets, uintptr(d.h), uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(&out[0])))
	if err := result(r); err != nil {
		return nil, err
	}
	return out, nil
}

// FreeDescriptorSets is vkFreeDescriptorSets. The pool must have been
// created with [DescriptorPoolCreateFreeDescriptorSet].
func (d *Device) FreeDescriptorSets(p DescriptorPool, sets []DescriptorSet) error {
	r := call4(d.fn.FreeDescriptorSets, uintptr(d.h), uintptr(p), uintptr(len(sets)), uintptr(unsafe.Pointer(unsafe.SliceData(sets))))
	return result(r)
}

// UpdateDescriptorSets is vkUpdateDescriptorSets.
func (d *Device) UpdateDescriptorSets(writes []WriteDescriptorSet, copies []CopyDescriptorSet) {
	call5(d.fn.UpdateDescriptorSets, uintptr(d.h), uintptr(len(writes)), uintptr(unsafe.Pointer(unsafe.SliceData(writes))), uintptr(len(copies)), uintptr(unsafe.Pointer(unsafe.SliceData(copies))))
}

// CreatePipelineLayout is vkCreatePipelineLayout.
func (d *Device) CreatePipelineLayout(info *PipelineLayoutCreateInfo) (PipelineLayout, error) {
	var h PipelineLayout
	r := call4(d.fn.CreatePipelineLayout, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyPipelineLayout is vkDestroyPipelineLayout.
func (d *Device) DestroyPipelineLayout(l PipelineLayout) {
	call3(d.fn.DestroyPipelineLayout, uintptr(d.h), uintptr(l), 0)
}

// CreateGraphicsPipelines is vkCreateGraphicsPipelines: one pipeline per
// info. cache may be zero. On an error the pipelines that were created
// are destroyed and the result is nil.
func (d *Device) CreateGraphicsPipelines(cache PipelineCache, infos []GraphicsPipelineCreateInfo) ([]Pipeline, error) {
	if len(infos) == 0 {
		return nil, nil
	}
	out := make([]Pipeline, len(infos))
	r := call6(d.fn.CreateGraphicsPipelines, uintptr(d.h), uintptr(cache), uintptr(len(infos)), uintptr(unsafe.Pointer(&infos[0])), 0, uintptr(unsafe.Pointer(&out[0])))
	if err := result(r); err != nil {
		for _, p := range out {
			if p != 0 {
				d.DestroyPipeline(p)
			}
		}
		return nil, err
	}
	return out, nil
}

// DestroyPipeline is vkDestroyPipeline.
func (d *Device) DestroyPipeline(p Pipeline) {
	call3(d.fn.DestroyPipeline, uintptr(d.h), uintptr(p), 0)
}

// Fences and semaphores.

// CreateFence is vkCreateFence.
func (d *Device) CreateFence(info *FenceCreateInfo) (Fence, error) {
	var h Fence
	r := call4(d.fn.CreateFence, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyFence is vkDestroyFence.
func (d *Device) DestroyFence(f Fence) {
	call3(d.fn.DestroyFence, uintptr(d.h), uintptr(f), 0)
}

// ResetFences is vkResetFences.
func (d *Device) ResetFences(fences []Fence) error {
	r := call3(d.fn.ResetFences, uintptr(d.h), uintptr(len(fences)), uintptr(unsafe.Pointer(unsafe.SliceData(fences))))
	return result(r)
}

// GetFenceStatus is vkGetFenceStatus: nil when the fence is signaled,
// [NotReady] when it is not.
func (d *Device) GetFenceStatus(f Fence) error {
	r := call2(d.fn.GetFenceStatus, uintptr(d.h), uintptr(f))
	return result(r)
}

// WaitForFences is vkWaitForFences: it waits up to timeout nanoseconds for
// all of the fences, or for one of them, and returns nil, or [Timeout]
// when the time ran out.
func (d *Device) WaitForFences(fences []Fence, waitAll bool, timeout uint64) error {
	all := False
	if waitAll {
		all = True
	}
	r := call5(d.fn.WaitForFences, uintptr(d.h), uintptr(len(fences)), uintptr(unsafe.Pointer(unsafe.SliceData(fences))), uintptr(all), uintptr(timeout))
	return result(r)
}

// CreateSemaphore is vkCreateSemaphore.
func (d *Device) CreateSemaphore(info *SemaphoreCreateInfo) (Semaphore, error) {
	var h Semaphore
	r := call4(d.fn.CreateSemaphore, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroySemaphore is vkDestroySemaphore.
func (d *Device) DestroySemaphore(s Semaphore) {
	call3(d.fn.DestroySemaphore, uintptr(d.h), uintptr(s), 0)
}

// SetDebugUtilsObjectNameEXT is vkSetDebugUtilsObjectNameEXT: it names an
// object of the device for the messages of the validation layers and for
// debuggers. It does nothing when the instance was created without
// VK_EXT_debug_utils.
func (d *Device) SetDebugUtilsObjectNameEXT(typ ObjectType, handle uint64, name string) error {
	if d.fn.SetDebugUtilsObjectNameEXT == 0 {
		return nil
	}
	info := &DebugUtilsObjectNameInfoEXT{
		SType:        StructureTypeDebugUtilsObjectNameInfoEXT,
		ObjectType:   typ,
		ObjectHandle: handle,
		PObjectName:  CString(name),
	}
	r := call2(d.fn.SetDebugUtilsObjectNameEXT, uintptr(d.h), uintptr(unsafe.Pointer(info)))
	return result(r)
}
