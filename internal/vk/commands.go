//go:build telagpu && (linux || windows)

package vk

import "unsafe"

// The commands that record into a command buffer. They return nothing:
// Vulkan reports their errors when the buffer ends or is submitted.
//
// A call allocates nothing, but the slices and pointers given to it are
// heap memory (see the package documentation): a renderer keeps the ones
// of its per-draw commands in buffers it reuses.

// CmdBeginRenderPass is vkCmdBeginRenderPass.
func (d *Device) CmdBeginRenderPass(cb CommandBuffer, info *RenderPassBeginInfo, contents SubpassContents) {
	call3(d.fn.CmdBeginRenderPass, uintptr(cb), uintptr(unsafe.Pointer(info)), uintptr(contents))
}

// CmdEndRenderPass is vkCmdEndRenderPass.
func (d *Device) CmdEndRenderPass(cb CommandBuffer) {
	call1(d.fn.CmdEndRenderPass, uintptr(cb))
}

// CmdBindPipeline is vkCmdBindPipeline.
func (d *Device) CmdBindPipeline(cb CommandBuffer, bindPoint PipelineBindPoint, p Pipeline) {
	call3(d.fn.CmdBindPipeline, uintptr(cb), uintptr(bindPoint), uintptr(p))
}

// CmdSetViewport is vkCmdSetViewport, for pipelines with
// [DynamicStateViewport].
func (d *Device) CmdSetViewport(cb CommandBuffer, first uint32, viewports []Viewport) {
	call4(d.fn.CmdSetViewport, uintptr(cb), uintptr(first), uintptr(len(viewports)), uintptr(unsafe.Pointer(unsafe.SliceData(viewports))))
}

// CmdSetScissor is vkCmdSetScissor, for pipelines with
// [DynamicStateScissor].
func (d *Device) CmdSetScissor(cb CommandBuffer, first uint32, scissors []Rect2D) {
	call4(d.fn.CmdSetScissor, uintptr(cb), uintptr(first), uintptr(len(scissors)), uintptr(unsafe.Pointer(unsafe.SliceData(scissors))))
}

// CmdSetBlendConstants is vkCmdSetBlendConstants, for pipelines with
// [DynamicStateBlendConstants]. The C function takes the array by
// pointer.
func (d *Device) CmdSetBlendConstants(cb CommandBuffer, constants *[4]float32) {
	call2(d.fn.CmdSetBlendConstants, uintptr(cb), uintptr(unsafe.Pointer(constants)))
}

// CmdBindDescriptorSets is vkCmdBindDescriptorSets.
func (d *Device) CmdBindDescriptorSets(cb CommandBuffer, bindPoint PipelineBindPoint, layout PipelineLayout, firstSet uint32, sets []DescriptorSet, dynamicOffsets []uint32) {
	call8(d.fn.CmdBindDescriptorSets, uintptr(cb), uintptr(bindPoint), uintptr(layout), uintptr(firstSet),
		uintptr(len(sets)), uintptr(unsafe.Pointer(unsafe.SliceData(sets))),
		uintptr(len(dynamicOffsets)), uintptr(unsafe.Pointer(unsafe.SliceData(dynamicOffsets))))
}

// CmdBindVertexBuffers is vkCmdBindVertexBuffers. offsets has one offset
// per buffer; it panics otherwise, since Vulkan would read past the
// shorter slice.
func (d *Device) CmdBindVertexBuffers(cb CommandBuffer, first uint32, buffers []Buffer, offsets []uint64) {
	if len(buffers) != len(offsets) {
		panic("vk: CmdBindVertexBuffers needs one offset per buffer")
	}
	call5(d.fn.CmdBindVertexBuffers, uintptr(cb), uintptr(first), uintptr(len(buffers)), uintptr(unsafe.Pointer(unsafe.SliceData(buffers))), uintptr(unsafe.Pointer(unsafe.SliceData(offsets))))
}

// CmdBindIndexBuffer is vkCmdBindIndexBuffer.
func (d *Device) CmdBindIndexBuffer(cb CommandBuffer, b Buffer, offset uint64, typ IndexType) {
	call4(d.fn.CmdBindIndexBuffer, uintptr(cb), uintptr(b), uintptr(offset), uintptr(typ))
}

// CmdPushConstants is vkCmdPushConstants: size bytes at values become the
// push constants from offset on.
func (d *Device) CmdPushConstants(cb CommandBuffer, layout PipelineLayout, stages ShaderStageFlags, offset, size uint32, values unsafe.Pointer) {
	call6(d.fn.CmdPushConstants, uintptr(cb), uintptr(layout), uintptr(stages), uintptr(offset), uintptr(size), uintptr(values))
}

// CmdDraw is vkCmdDraw.
func (d *Device) CmdDraw(cb CommandBuffer, vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	call5(d.fn.CmdDraw, uintptr(cb), uintptr(vertexCount), uintptr(instanceCount), uintptr(firstVertex), uintptr(firstInstance))
}

// CmdDrawIndexed is vkCmdDrawIndexed.
func (d *Device) CmdDrawIndexed(cb CommandBuffer, indexCount, instanceCount, firstIndex uint32, vertexOffset int32, firstInstance uint32) {
	// A negative int32 is passed sign-extended; the callee reads 32 bits.
	call6(d.fn.CmdDrawIndexed, uintptr(cb), uintptr(indexCount), uintptr(instanceCount), uintptr(firstIndex), uintptr(vertexOffset), uintptr(firstInstance))
}

// CmdClearAttachments is vkCmdClearAttachments: it clears rectangles of
// attachments inside a render pass, ignoring the scissor.
func (d *Device) CmdClearAttachments(cb CommandBuffer, attachments []ClearAttachment, rects []ClearRect) {
	call5(d.fn.CmdClearAttachments, uintptr(cb), uintptr(len(attachments)), uintptr(unsafe.Pointer(unsafe.SliceData(attachments))), uintptr(len(rects)), uintptr(unsafe.Pointer(unsafe.SliceData(rects))))
}

// CmdClearColorImage is vkCmdClearColorImage, outside a render pass.
func (d *Device) CmdClearColorImage(cb CommandBuffer, img Image, layout ImageLayout, color *ClearColorValue, ranges []ImageSubresourceRange) {
	call6(d.fn.CmdClearColorImage, uintptr(cb), uintptr(img), uintptr(layout), uintptr(unsafe.Pointer(color)), uintptr(len(ranges)), uintptr(unsafe.Pointer(unsafe.SliceData(ranges))))
}

// CmdCopyBuffer is vkCmdCopyBuffer.
func (d *Device) CmdCopyBuffer(cb CommandBuffer, src, dst Buffer, regions []BufferCopy) {
	call5(d.fn.CmdCopyBuffer, uintptr(cb), uintptr(src), uintptr(dst), uintptr(len(regions)), uintptr(unsafe.Pointer(unsafe.SliceData(regions))))
}

// CmdCopyImage is vkCmdCopyImage.
func (d *Device) CmdCopyImage(cb CommandBuffer, src Image, srcLayout ImageLayout, dst Image, dstLayout ImageLayout, regions []ImageCopy) {
	call7(d.fn.CmdCopyImage, uintptr(cb), uintptr(src), uintptr(srcLayout), uintptr(dst), uintptr(dstLayout), uintptr(len(regions)), uintptr(unsafe.Pointer(unsafe.SliceData(regions))))
}

// CmdBlitImage is vkCmdBlitImage: a copy that scales and converts.
func (d *Device) CmdBlitImage(cb CommandBuffer, src Image, srcLayout ImageLayout, dst Image, dstLayout ImageLayout, regions []ImageBlit, filter Filter) {
	call8(d.fn.CmdBlitImage, uintptr(cb), uintptr(src), uintptr(srcLayout), uintptr(dst), uintptr(dstLayout), uintptr(len(regions)), uintptr(unsafe.Pointer(unsafe.SliceData(regions))), uintptr(filter))
}

// CmdCopyBufferToImage is vkCmdCopyBufferToImage.
func (d *Device) CmdCopyBufferToImage(cb CommandBuffer, src Buffer, dst Image, dstLayout ImageLayout, regions []BufferImageCopy) {
	call6(d.fn.CmdCopyBufferToImage, uintptr(cb), uintptr(src), uintptr(dst), uintptr(dstLayout), uintptr(len(regions)), uintptr(unsafe.Pointer(unsafe.SliceData(regions))))
}

// CmdCopyImageToBuffer is vkCmdCopyImageToBuffer.
func (d *Device) CmdCopyImageToBuffer(cb CommandBuffer, src Image, srcLayout ImageLayout, dst Buffer, regions []BufferImageCopy) {
	call6(d.fn.CmdCopyImageToBuffer, uintptr(cb), uintptr(src), uintptr(srcLayout), uintptr(dst), uintptr(len(regions)), uintptr(unsafe.Pointer(unsafe.SliceData(regions))))
}

// CmdPipelineBarrier is vkCmdPipelineBarrier.
func (d *Device) CmdPipelineBarrier(cb CommandBuffer, srcStages, dstStages PipelineStageFlags, flags DependencyFlags, memory []MemoryBarrier, buffers []BufferMemoryBarrier, images []ImageMemoryBarrier) {
	call10(d.fn.CmdPipelineBarrier, uintptr(cb), uintptr(srcStages), uintptr(dstStages), uintptr(flags),
		uintptr(len(memory)), uintptr(unsafe.Pointer(unsafe.SliceData(memory))),
		uintptr(len(buffers)), uintptr(unsafe.Pointer(unsafe.SliceData(buffers))),
		uintptr(len(images)), uintptr(unsafe.Pointer(unsafe.SliceData(images))))
}
