//go:build telagpu && linux

package vk

import (
	"testing"

	"golang.org/x/sys/unix"
)

// dmabufSide is a device that takes part in an exchange of dmabufs.
type dmabufSide struct {
	*renderer
	inst  *Instance
	info  DeviceInfo
	queue Queue
	pool  CommandPool
	fence Fence
}

func newDmabufSide(t *testing.T, inst *Instance, info DeviceInfo) *dmabufSide {
	t.Helper()
	extensions, ok := DmabufExtensions(&info)
	if !ok || !info.HasExtension(ExtQueueFamilyForeignExtensionName) {
		t.Skipf("%s lacks an extension of the exchange of dmabufs", info.Name)
	}
	priority := float32(1)
	count, names := CStringArray(extensions)
	dev, err := inst.CreateDevice(info.Device, &DeviceCreateInfo{
		SType:                StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos: &DeviceQueueCreateInfo{
			SType:            StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: info.GraphicsQueueFamily,
			QueueCount:       1,
			PQueuePriorities: &priority,
		},
		EnabledExtensionCount:   count,
		PpEnabledExtensionNames: names,
		PEnabledFeatures:        &PhysicalDeviceFeatures{},
	})
	check(t, "CreateDevice", err)
	t.Cleanup(dev.Destroy)
	s := &dmabufSide{renderer: &renderer{t: t, dev: dev, mem: inst.GetPhysicalDeviceMemoryProperties(info.Device)},
		inst: inst, info: info, queue: dev.GetQueue(info.GraphicsQueueFamily, 0)}
	s.pool, err = dev.CreateCommandPool(&CommandPoolCreateInfo{
		SType:            StructureTypeCommandPoolCreateInfo,
		Flags:            CommandPoolCreateResetCommandBuffer,
		QueueFamilyIndex: info.GraphicsQueueFamily,
	})
	check(t, "CreateCommandPool", err)
	t.Cleanup(func() { dev.DestroyCommandPool(s.pool) })
	s.fence, err = dev.CreateFence(&FenceCreateInfo{SType: StructureTypeFenceCreateInfo})
	check(t, "CreateFence", err)
	t.Cleanup(func() { dev.DestroyFence(s.fence) })
	return s
}

// run records commands, submits them and waits for them.
func (s *dmabufSide) run(record func(cb CommandBuffer)) {
	s.t.Helper()
	dev := s.dev
	buffers, err := dev.AllocateCommandBuffers(&CommandBufferAllocateInfo{
		SType:              StructureTypeCommandBufferAllocateInfo,
		CommandPool:        s.pool,
		Level:              CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	})
	check(s.t, "AllocateCommandBuffers", err)
	defer dev.FreeCommandBuffers(s.pool, buffers)
	cb := buffers[0]
	check(s.t, "BeginCommandBuffer", dev.BeginCommandBuffer(cb, &CommandBufferBeginInfo{
		SType: StructureTypeCommandBufferBeginInfo,
		Flags: CommandBufferUsageOneTimeSubmit,
	}))
	record(cb)
	check(s.t, "EndCommandBuffer", dev.EndCommandBuffer(cb))
	check(s.t, "QueueSubmit", dev.QueueSubmit(s.queue, []SubmitInfo{{
		SType:              StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    &cb,
	}}, s.fence))
	check(s.t, "WaitForFences", dev.WaitForFences([]Fence{s.fence}, true, 10e9))
	check(s.t, "ResetFences", dev.ResetFences([]Fence{s.fence}))
}

// foreign returns the barrier that takes an image from the foreign queue
// family, or with back gives it to it.
func (s *dmabufSide) foreign(img Image, from, to ImageLayout, src, dst AccessFlags, back bool) ImageMemoryBarrier {
	b := transition(img, from, to, src, dst)
	b.SrcQueueFamilyIndex, b.DstQueueFamilyIndex = QueueFamilyForeignEXT, s.info.GraphicsQueueFamily
	if back {
		b.SrcQueueFamilyIndex, b.DstQueueFamilyIndex = b.DstQueueFamilyIndex, b.SrcQueueFamilyIndex
	}
	return b
}

// export makes an image in a dmabuf, filled with a color and handed over.
func (s *dmabufSide) export(m DmabufModifier, width, height uint32, color rgba) (*DmabufImage, uint64) {
	s.t.Helper()
	img, err := s.dev.CreateDmabufImage(s.mem, FormatR8G8B8A8Unorm, width, height, ImageUsageTransferDst, []DmabufModifier{m})
	check(s.t, "CreateDmabufImage", err)
	s.run(func(cb CommandBuffer) {
		s.dev.CmdPipelineBarrier(cb, PipelineStageTopOfPipe, PipelineStageTransfer, 0, nil, nil, []ImageMemoryBarrier{
			transition(img.Image, ImageLayoutUndefined, ImageLayoutTransferDstOptimal, 0, AccessTransferWrite),
		})
		c := unorm(color)
		fill := ClearColorFloat(c[0], c[1], c[2], c[3])
		s.dev.CmdClearColorImage(cb, img.Image, ImageLayoutTransferDstOptimal, &fill, []ImageSubresourceRange{colorRange})
		s.dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageBottomOfPipe, 0, nil, nil, []ImageMemoryBarrier{
			s.foreign(img.Image, ImageLayoutTransferDstOptimal, ImageLayoutGeneral, AccessTransferWrite, 0, true),
		})
	})
	size, err := unix.Seek(img.FD, 0, unix.SEEK_END)
	check(s.t, "lseek of a dmabuf", err)
	return img, uint64(size)
}

// read copies an imported image's pixels back.
func (s *dmabufSide) read(img Image, width, height uint32) []byte {
	s.t.Helper()
	buf := s.newBuffer(int(4*width*height), BufferUsageTransferDst)
	s.run(func(cb CommandBuffer) {
		s.dev.CmdPipelineBarrier(cb, PipelineStageTopOfPipe, PipelineStageTransfer, 0, nil, nil, []ImageMemoryBarrier{
			s.foreign(img, ImageLayoutGeneral, ImageLayoutTransferSrcOptimal, 0, AccessTransferRead, false),
		})
		s.dev.CmdCopyImageToBuffer(cb, img, ImageLayoutTransferSrcOptimal, buf.buf, []BufferImageCopy{{
			ImageSubresource: colorLayers,
			ImageExtent:      Extent3D{Width: width, Height: height, Depth: 1},
		}})
		s.dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageBottomOfPipe, 0, nil, nil, []ImageMemoryBarrier{
			s.foreign(img, ImageLayoutTransferSrcOptimal, ImageLayoutGeneral, 0, 0, true),
		})
		s.dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageHost, 0, []MemoryBarrier{{
			SType:         StructureTypeMemoryBarrier,
			SrcAccessMask: AccessTransferWrite,
			DstAccessMask: AccessHostRead,
		}}, nil, nil)
	})
	s.invalidate(buf)
	return buf.bytes
}

// TestDmabufImport imports, on every device, an image another device of the
// same GPU exported, with each modifier both do, as an image and the linear
// one as a buffer, and reads it back; and refuses layouts the driver would
// read another way or beyond the dmabuf. The exchange between GPUs is the
// renderer's test (internal/gfx/gpu).
func TestDmabufImport(t *testing.T) {
	inst, msgs := newTestInstance(t)
	const width, height = 256, 64
	const usage = ImageUsageSampled | ImageUsageTransferSrc
	for _, info := range testDevices(t, inst) {
		t.Run(info.Name, func(t *testing.T) {
			t.Cleanup(func() { msgs.report(t) })
			app, win := newDmabufSide(t, inst, info), newDmabufSide(t, inst, info)
			id, ok := inst.GetPhysicalDeviceID(info.Device)
			if !ok || id.DeviceUUID == ([16]byte{}) || id.DriverUUID == ([16]byte{}) {
				t.Errorf("the device has no UUIDs: %v, %+v", ok, id)
			}
			if !id.HasLUID && id.LUID != ([8]byte{}) {
				t.Errorf("an invalid LUID is not zero: %+v", id)
			}
			imports := inst.DmabufImportModifiers(info.Device, FormatR8G8B8A8Unorm, usage)
			exports := inst.DmabufModifiers(info.Device, FormatR8G8B8A8Unorm, ImageUsageTransferDst)
			count := 0
			for n, m := range imports {
				var exp *DmabufModifier
				for i := range exports {
					if exports[i].Modifier == m.Modifier {
						exp = &exports[i]
					}
				}
				if exp == nil {
					t.Logf("%#x is not exported", m.Modifier)
					continue
				}
				count++
				color := rgba{uint8(40 + 10*n), uint8(200 - 10*n), uint8(90 + n), 128}
				img, size := app.export(*exp, width, height, color)
				layout := func() *DmabufLayout {
					return &DmabufLayout{Format: FormatR8G8B8A8Unorm, Width: width, Height: height, Modifier: m.Modifier,
						Planes: append([]DmabufPlane(nil), img.Planes...), FD: img.FD, Size: size}
				}
				if bits, err := win.dev.GetMemoryFdPropertiesKHR(ExternalMemoryHandleTypeDmaBufEXT, img.FD); err != nil || bits == 0 {
					t.Errorf("%#x: the memory types of the descriptor: %#x, %v", m.Modifier, bits, err)
				}
				imp, err := win.dev.ImportDmabufImage(win.mem, layout(), usage)
				if err != nil {
					t.Errorf("%#x: %v", m.Modifier, err)
				} else {
					if x, y, got, ok := firstDifferent(win.read(imp.Image, width, height), 4*width, color); !ok {
						t.Errorf("%#x: the pixel (%d, %d) is %v, want %v", m.Modifier, x, y, got, color)
					}
					win.dev.DestroyImported(imp)
				}
				for what, edit := range map[string]func(l *DmabufLayout){
					"more than the dmabuf":         func(l *DmabufLayout) { l.Size = size / 2 },
					"a stride that is not its own": func(l *DmabufLayout) { l.Planes[0].Stride *= 4 },
					"an offset":                    func(l *DmabufLayout) { l.Planes[0].Offset = size &^ 0xffff },
					"no planes":                    func(l *DmabufLayout) { l.Planes = nil },
				} {
					l := layout()
					edit(l)
					if imp, err := win.dev.ImportDmabufImage(win.mem, l, usage); err == nil {
						t.Errorf("%#x, %s: imported", m.Modifier, what)
						win.dev.DestroyImported(imp)
					}
				}
				if m.Modifier == drmModLinear {
					if !inst.DmabufBufferImportable(info.Device, BufferUsageTransferSrc) {
						t.Log("no buffer is imported")
					} else if imp, err := win.dev.ImportDmabufBuffer(win.mem, img.FD, size, size, BufferUsageTransferSrc); err != nil {
						t.Errorf("as a buffer: %v", err)
					} else {
						buf := win.newBuffer(4*width, BufferUsageTransferDst)
						win.run(func(cb CommandBuffer) {
							win.dev.CmdCopyBuffer(cb, imp.Buffer, buf.buf, []BufferCopy{{SrcOffset: img.Planes[0].Offset, Size: 4 * width}})
							win.dev.CmdPipelineBarrier(cb, PipelineStageTransfer, PipelineStageHost, 0, []MemoryBarrier{{
								SType:         StructureTypeMemoryBarrier,
								SrcAccessMask: AccessTransferWrite,
								DstAccessMask: AccessHostRead,
							}}, nil, nil)
						})
						win.invalidate(buf)
						if x, y, got, ok := firstDifferent(buf.bytes, 4*width, color); !ok {
							t.Errorf("as a buffer: the pixel (%d, %d) is %v, want %v", x, y, got, color)
						}
						win.dev.DestroyImported(imp)
					}
					if imp, err := win.dev.ImportDmabufBuffer(win.mem, img.FD, 2*size, size, BufferUsageTransferSrc); err == nil {
						t.Error("a buffer larger than its dmabuf was imported")
						win.dev.DestroyImported(imp)
					}
				}
				app.dev.DestroyDmabufImage(img)
			}
			if count == 0 {
				t.Skipf("%s imports nothing it exports (%d modifiers imported, %d exported)", info.Name, len(imports), len(exports))
			}
			t.Logf("%d modifiers imported", count)

			// Without the extensions there is nothing to call.
			bare := newTestDevice(t, inst, info)
			if _, err := bare.GetMemoryFdPropertiesKHR(ExternalMemoryHandleTypeDmaBufEXT, 0); err != ErrorExtensionNotPresent {
				t.Errorf("a device without the extensions answers %v", err)
			}
		})
	}
}
