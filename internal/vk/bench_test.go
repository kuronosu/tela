//go:build telagpu && (linux || windows)

package vk

import (
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The benchmarks measure what a command costs through the bindings, on
// the device the selection picks, beside the two ways purego itself offers
// to call a C function, which is what decided the call layer (call.go):
//
//   - "binding": the wrapper of the package, through call1 to call10;
//   - "SyscallN": purego.SyscallN called directly, whose variadic
//     arguments are a heap allocation per call;
//   - "RegisterFunc": a typed Go function made by purego.RegisterFunc,
//     which goes through reflection on every call.
//
// GetQueue is a command that does next to nothing (the driver returns a
// stored handle), so its time is the cost of a call. SetScissor, which
// takes a pointer, and BindIndexBuffer, which takes integers only, are
// commands a renderer records per draw; both are valid outside a render
// pass, so the benchmark needs no pipeline. Their time includes the
// driver's work of recording; the command buffer is reset every few
// thousand commands so that it does not grow without bound.
func BenchmarkCall(b *testing.B) {
	inst, _ := newTestInstance(b)
	info, err := inst.PickDevice()
	if err != nil {
		b.Skip(err)
	}
	dev := newTestDevice(b, inst, info)
	pool, err := dev.CreateCommandPool(&CommandPoolCreateInfo{
		SType:            StructureTypeCommandPoolCreateInfo,
		Flags:            CommandPoolCreateResetCommandBuffer,
		QueueFamilyIndex: info.GraphicsQueueFamily,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer dev.DestroyCommandPool(pool)
	buffers, err := dev.AllocateCommandBuffers(&CommandBufferAllocateInfo{
		SType:              StructureTypeCommandBufferAllocateInfo,
		CommandPool:        pool,
		Level:              CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	})
	if err != nil {
		b.Fatal(err)
	}
	cb := buffers[0]
	indexBuffer, err := dev.CreateBuffer(&BufferCreateInfo{
		SType:       StructureTypeBufferCreateInfo,
		Size:        64,
		Usage:       BufferUsageIndexBuffer,
		SharingMode: SharingModeExclusive,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer dev.DestroyBuffer(indexBuffer)
	req := dev.GetBufferMemoryRequirements(indexBuffer)
	typ, _ := inst.GetPhysicalDeviceMemoryProperties(info.Device).FindMemoryType(req.MemoryTypeBits, 0)
	mem, err := dev.AllocateMemory(&MemoryAllocateInfo{SType: StructureTypeMemoryAllocateInfo, AllocationSize: req.Size, MemoryTypeIndex: typ})
	if err != nil {
		b.Fatal(err)
	}
	defer dev.FreeMemory(mem)
	if err := dev.BindBufferMemory(indexBuffer, mem, 0); err != nil {
		b.Fatal(err)
	}

	begin := &CommandBufferBeginInfo{SType: StructureTypeCommandBufferBeginInfo}
	// record runs one command b.N times, in command buffers of 4096.
	record := func(b *testing.B, command func()) {
		b.Helper()
		b.ReportAllocs()
		n := 0
		for b.Loop() {
			if n%4096 == 0 {
				if n > 0 {
					dev.EndCommandBuffer(cb)
				}
				if err := dev.BeginCommandBuffer(cb, begin); err != nil { // resets it
					b.Fatal(err)
				}
			}
			n++
			command()
		}
		if err := dev.EndCommandBuffer(cb); err != nil {
			b.Fatal(err)
		}
	}

	family := info.GraphicsQueueFamily
	b.Run("GetQueue/binding", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if dev.GetQueue(family, 0) == 0 {
				b.Fatal("no queue")
			}
		}
	})
	b.Run("GetQueue/SyscallN", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var q Queue
			purego.SyscallN(dev.fn.GetDeviceQueue, uintptr(dev.h), uintptr(family), 0, uintptr(unsafe.Pointer(&q)))
			if q == 0 {
				b.Fatal("no queue")
			}
		}
	})
	b.Run("GetQueue/RegisterFunc", func(b *testing.B) {
		var getDeviceQueue func(d DeviceHandle, family, index uint32, q *Queue)
		purego.RegisterFunc(&getDeviceQueue, dev.fn.GetDeviceQueue)
		b.ReportAllocs()
		for b.Loop() {
			var q Queue
			getDeviceQueue(dev.h, family, 0, &q)
			if q == 0 {
				b.Fatal("no queue")
			}
		}
	})

	scissors := []Rect2D{{Extent: Extent2D{Width: 64, Height: 64}}} // reused, as a renderer would
	b.Run("SetScissor/binding", func(b *testing.B) {
		record(b, func() { dev.CmdSetScissor(cb, 0, scissors) })
	})
	b.Run("SetScissor/binding with a new slice", func(b *testing.B) {
		// What a caller pays that does not reuse its slice: the literal
		// is a heap allocation per call.
		record(b, func() { dev.CmdSetScissor(cb, 0, []Rect2D{{Extent: Extent2D{Width: 64, Height: 64}}}) })
	})
	b.Run("SetScissor/SyscallN", func(b *testing.B) {
		record(b, func() {
			purego.SyscallN(dev.fn.CmdSetScissor, uintptr(cb), 0, 1, uintptr(unsafe.Pointer(&scissors[0])))
		})
	})
	b.Run("SetScissor/RegisterFunc", func(b *testing.B) {
		var cmdSetScissor func(cb CommandBuffer, first, count uint32, scissors *Rect2D)
		purego.RegisterFunc(&cmdSetScissor, dev.fn.CmdSetScissor)
		record(b, func() { cmdSetScissor(cb, 0, 1, &scissors[0]) })
	})
	b.Run("BindIndexBuffer/binding", func(b *testing.B) {
		record(b, func() { dev.CmdBindIndexBuffer(cb, indexBuffer, 0, IndexTypeUint16) })
	})
	b.Run("BindIndexBuffer/SyscallN", func(b *testing.B) {
		record(b, func() {
			purego.SyscallN(dev.fn.CmdBindIndexBuffer, uintptr(cb), uintptr(indexBuffer), 0, uintptr(IndexTypeUint16))
		})
	})
	b.Run("BindIndexBuffer/RegisterFunc", func(b *testing.B) {
		var cmdBindIndexBuffer func(cb CommandBuffer, buffer Buffer, offset uint64, typ IndexType)
		purego.RegisterFunc(&cmdBindIndexBuffer, dev.fn.CmdBindIndexBuffer)
		record(b, func() { cmdBindIndexBuffer(cb, indexBuffer, 0, IndexTypeUint16) })
	})
}
