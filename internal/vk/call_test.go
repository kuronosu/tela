//go:build telagpu && (linux || windows)

package vk

import (
	"testing"
	"unsafe"
)

// TestCommandsDoNotAllocate guards the reason the call layer exists (see
// call.go): recording a command with arguments the caller reuses must not
// allocate, whether the command takes integers only or pointers.
func TestCommandsDoNotAllocate(t *testing.T) {
	inst, _ := newTestInstance(t)
	info := testDevices(t, inst)[0]
	dev := newTestDevice(t, inst, info)
	pool, err := dev.CreateCommandPool(&CommandPoolCreateInfo{
		SType:            StructureTypeCommandPoolCreateInfo,
		QueueFamilyIndex: info.GraphicsQueueFamily,
	})
	check(t, "CreateCommandPool", err)
	defer dev.DestroyCommandPool(pool)
	buffers, err := dev.AllocateCommandBuffers(&CommandBufferAllocateInfo{
		SType:              StructureTypeCommandBufferAllocateInfo,
		CommandPool:        pool,
		Level:              CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	})
	check(t, "AllocateCommandBuffers", err)
	cb := buffers[0]
	check(t, "BeginCommandBuffer", dev.BeginCommandBuffer(cb, &CommandBufferBeginInfo{SType: StructureTypeCommandBufferBeginInfo}))

	// The commands of dynamic state are valid in any recording command
	// buffer, with no pipeline bound.
	scissors := []Rect2D{{Extent: Extent2D{Width: 64, Height: 64}}}
	viewports := []Viewport{{Width: 64, Height: 64, MaxDepth: 1}}
	constants := &[4]float32{}
	queue := new(Queue)
	for _, tt := range []struct {
		name string
		call func()
	}{
		{"CmdSetScissor (a slice)", func() { dev.CmdSetScissor(cb, 0, scissors) }},
		{"CmdSetViewport (a slice)", func() { dev.CmdSetViewport(cb, 0, viewports) }},
		{"CmdSetBlendConstants (a pointer)", func() { dev.CmdSetBlendConstants(cb, constants) }},
		{"vkGetDeviceQueue through the call layer (integers and a pointer)", func() {
			call4(dev.fn.GetDeviceQueue, uintptr(dev.h), uintptr(info.GraphicsQueueFamily), 0, uintptr(unsafe.Pointer(queue)))
		}},
	} {
		if allocs := testing.AllocsPerRun(200, tt.call); allocs != 0 {
			t.Errorf("%s: %v allocations per call, want none", tt.name, allocs)
		}
	}
	if *queue == 0 {
		t.Error("vkGetDeviceQueue returned no queue")
	}
	check(t, "EndCommandBuffer", dev.EndCommandBuffer(cb))
}
