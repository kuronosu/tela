//go:build telagpu && windows

package gpu

import (
	"errors"
	"fmt"
	"image"
	"unsafe"

	"github.com/kuronosu/tela/internal/vk"
)

// The Vulkan device's swapchains on windows (swapchain.go, ADR-289): a
// VkSurfaceKHR of the HWND (VK_KHR_win32_surface) and a FIFO swapchain on
// it, whose images are only copied into from the window's target.
//
// The images are 8-bit BGRA (or RGBA) in the sRGB color space without an
// sRGB format: the target's values are sRGB-encoded already (ADR-007) and
// reach the screen as they are. BGRA is filled by a blit of equal
// rectangles, which converts the byte order and changes no value (as the
// Linux export's XRGB8888 buffers are, TestExport), RGBA by a copy.
//
// Every submission is waited for, as everywhere in this device: the copy
// waits for the semaphore of the acquire and signals one the present waits
// for. The acquire semaphores are a ring one longer than the images, so
// one is reused only after the copy that waited for it was done; the
// present semaphores are one per image, reused only once the image was
// acquired again, which the presentation engine allows only after that
// present's wait. Images are acquired without waiting (a timeout of 0):
// with no image free the frame is retried at the next one, and the UI
// thread never blocks in the presentation engine. FIFO is the present
// mode every surface has; the window's frames come once a refresh
// (DwmFlush pacing), so its queue never holds more than one present.
//
// winit, wgpu and Gio choose alike: FIFO, the surface's current extent and
// transform, opaque alpha where the surface offers it. Windows reports a
// minimized window's surface with an extent of 0x0, where no swapchain
// can be made: nothing is presented then (SwapchainHidden), and the
// swapchain is made again when the window comes back if the driver says
// it is out of date.

// surfaceSupported: this build makes surfaces of windows.
const surfaceSupported = true

var _ WindowPresenter = (*vkDevice)(nil)

// vkSwapchain is a window's surface and its swapchain: the SwapchainOps of
// a Swapchain.
type vkSwapchain struct {
	d       *vkDevice // nil once destroyed
	surface vk.SurfaceKHR
	format  vk.SurfaceFormatKHR
	alpha   vk.CompositeAlphaFlagsKHR
	// convert: the images have another byte order than a target (BGRA),
	// so pixels are copied into them by a blit.
	convert bool

	chain     vk.SwapchainKHR
	extent    image.Point
	transform vk.SurfaceTransformFlagsKHR
	images    []vk.Image
	// presented: the image was presented once, so it is in the layout of
	// presenting and holds the pixels it was given.
	presented []bool
	// acquire is the ring of the acquires' semaphores, next the one the
	// next acquire signals and waiting the one of the image acquired;
	// ready are the semaphores the presents wait for, one per image.
	acquire []vk.Semaphore
	next    int
	waiting vk.Semaphore
	ready   []vk.Semaphore
}

func (d *vkDevice) NewSwapchain(w NativeWindow) (*Swapchain, error) {
	if err := d.usable(); err != nil {
		return nil, err
	}
	if d.windows == nil {
		return nil, fmt.Errorf("gpu: %s presents to no window: the Vulkan instance has no %s or the device no %s", d.info.Name, vk.KhrWin32SurfaceExtensionName, vk.KhrSwapchainExtensionName)
	}
	if w.Window == 0 {
		return nil, errors.New("gpu: a swapchain on a window without a handle")
	}
	inst, pd := d.inst, d.info.Device
	surface, err := inst.CreateWin32SurfaceKHR(&vk.Win32SurfaceCreateInfoKHR{
		SType:     vk.StructureTypeWin32SurfaceCreateInfoKHR,
		Hinstance: w.Instance,
		Hwnd:      w.Window,
	})
	if err != nil {
		return nil, d.check(fmt.Errorf("gpu: creating the surface of a window: %w", err))
	}
	s := &vkSwapchain{d: d, surface: surface}
	fail := func(err error) (*Swapchain, error) {
		inst.DestroySurfaceKHR(surface)
		return nil, d.check(err)
	}
	if ok, err := inst.GetPhysicalDeviceSurfaceSupportKHR(pd, d.info.GraphicsQueueFamily, surface); err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("gpu: the graphics queue of %s does not present to the window", d.info.Name)
		}
		return fail(err)
	}
	formats, err := inst.GetPhysicalDeviceSurfaceFormatsKHR(pd, surface)
	if err != nil {
		return fail(fmt.Errorf("gpu: the formats of a window's surface: %w", err))
	}
	var ok bool
	if s.format, ok = chooseSurfaceFormat(formats); !ok {
		return fail(fmt.Errorf("gpu: a window's surface on %s has no 8-bit BGRA or RGBA format in the sRGB color space (%d formats)", d.info.Name, len(formats)))
	}
	caps, err := inst.GetPhysicalDeviceSurfaceCapabilitiesKHR(pd, surface)
	if err != nil {
		return fail(fmt.Errorf("gpu: the capabilities of a window's surface: %w", err))
	}
	if caps.SupportedUsageFlags&vk.ImageUsageTransferDst == 0 {
		return fail(fmt.Errorf("gpu: the images of a window's surface on %s cannot be copied into", d.info.Name))
	}
	if s.alpha, ok = chooseCompositeAlpha(caps.SupportedCompositeAlpha); !ok {
		return fail(fmt.Errorf("gpu: a window's surface on %s composites alpha in no way the renderer writes (%#x)", d.info.Name, caps.SupportedCompositeAlpha))
	}
	if s.convert = s.format.Format != targetFormat; s.convert {
		src := inst.GetPhysicalDeviceFormatProperties(pd, targetFormat).OptimalTilingFeatures
		dst := inst.GetPhysicalDeviceFormatProperties(pd, s.format.Format).OptimalTilingFeatures
		if src&vk.FormatFeatureBlitSrc == 0 || dst&vk.FormatFeatureBlitDst == 0 {
			return fail(fmt.Errorf("gpu: %s cannot blit into the BGRA images of a window's surface", d.info.Name))
		}
	}
	d.windows[s] = struct{}{}
	return NewSwapchainOf(s), nil
}

// usable returns the error of a call into a swapchain whose device is
// closed or lost, or that was destroyed.
func (s *vkSwapchain) usable() error {
	if s.d == nil || s.d.closed {
		return ErrSwapchainClosed
	}
	return s.d.usable()
}

func (s *vkSwapchain) Surface() (SurfaceInfo, error) {
	if err := s.usable(); err != nil {
		return SurfaceInfo{}, err
	}
	d := s.d
	caps, err := d.inst.GetPhysicalDeviceSurfaceCapabilitiesKHR(d.info.Device, s.surface)
	if err != nil {
		return SurfaceInfo{}, d.check(fmt.Errorf("gpu: the capabilities of a window's surface: %w", err))
	}
	s.transform = caps.CurrentTransform
	info := SurfaceInfo{
		MinImages:   int(caps.MinImageCount),
		MaxImages:   int(caps.MaxImageCount),
		Incremental: d.incremental,
	}
	if e := caps.CurrentExtent; e.Width == 0xFFFFFFFF && e.Height == 0xFFFFFFFF {
		info.AnyExtent = true
	} else {
		info.Extent = image.Pt(int(e.Width), int(e.Height))
	}
	return info, nil
}

func (s *vkSwapchain) Create(extent image.Point, images int) (int, error) {
	if err := s.usable(); err != nil {
		return 0, err
	}
	d := s.d
	dev := d.dev
	if extent.X < 1 || extent.Y < 1 || extent.X > d.maxSize || extent.Y > d.maxSize {
		return 0, fmt.Errorf("gpu: a swapchain of %v: the size must be between 1 and %d", extent, d.maxSize)
	}
	transform := s.transform
	if transform == 0 {
		transform = vk.SurfaceTransformIdentityKHR
	}
	old := s.chain
	chain, err := dev.CreateSwapchainKHR(&vk.SwapchainCreateInfoKHR{
		SType:            vk.StructureTypeSwapchainCreateInfoKHR,
		Surface:          s.surface,
		MinImageCount:    uint32(images),
		ImageFormat:      s.format.Format,
		ImageColorSpace:  s.format.ColorSpace,
		ImageExtent:      vk.Extent2D{Width: uint32(extent.X), Height: uint32(extent.Y)},
		ImageArrayLayers: 1,
		ImageUsage:       vk.ImageUsageTransferDst,
		ImageSharingMode: vk.SharingModeExclusive,
		PreTransform:     transform,
		CompositeAlpha:   s.alpha,
		PresentMode:      vk.PresentModeFifoKHR,
		Clipped:          vk.True,
		OldSwapchain:     old,
	})
	// The old swapchain is retired either way: destroyed once the queue
	// is done with its presents' semaphores.
	s.chain = 0
	s.dropChain(old)
	if err != nil {
		return 0, d.check(fmt.Errorf("gpu: creating a swapchain of %v: %w", extent, err))
	}
	s.chain = chain
	imgs, err := dev.GetSwapchainImagesKHR(chain)
	if err == nil && len(imgs) == 0 {
		err = errors.New("no images")
	}
	if err != nil {
		s.dropChain(chain)
		s.chain = 0
		return 0, d.check(fmt.Errorf("gpu: the images of a swapchain: %w", err))
	}
	s.images, s.extent = imgs, extent
	s.presented = make([]bool, len(imgs))
	for len(s.acquire) < len(imgs)+1 || len(s.ready) < len(imgs) {
		sem, err := dev.CreateSemaphore(&vk.SemaphoreCreateInfo{SType: vk.StructureTypeSemaphoreCreateInfo})
		if err != nil {
			s.dropChain(chain)
			s.chain = 0
			return 0, d.check(fmt.Errorf("gpu: creating a semaphore: %w", err))
		}
		if len(s.acquire) < len(imgs)+1 {
			s.acquire = append(s.acquire, sem)
		} else {
			s.ready = append(s.ready, sem)
		}
	}
	return len(imgs), nil
}

// dropChain destroys a swapchain and the semaphores beside it, once the
// queue is idle (their presents waited), and forgets its images.
func (s *vkSwapchain) dropChain(chain vk.SwapchainKHR) {
	d := s.d
	if d == nil {
		return
	}
	dev := d.dev
	if err := dev.QueueWaitIdle(d.queue); err != nil {
		// The queue may still use the semaphores: the device counts as
		// lost, as after a failed fence wait (submitWith), so that nothing
		// is submitted or reused any more and its Close waits for it.
		d.lost = true
	}
	if chain != 0 {
		dev.DestroySwapchainKHR(chain)
	}
	for _, sem := range append(s.acquire, s.ready...) {
		dev.DestroySemaphore(sem)
	}
	s.acquire, s.ready, s.next, s.waiting = nil, nil, 0, 0
	s.images, s.presented, s.extent = nil, nil, image.Point{}
}

func (s *vkSwapchain) Acquire() (int, SwapchainStatus, error) {
	if err := s.usable(); err != nil {
		return 0, 0, err
	}
	if s.chain == 0 {
		return 0, SwapchainOutOfDate, nil
	}
	sem := s.acquire[s.next]
	index, err := s.d.dev.AcquireNextImageKHR(s.chain, 0, sem, 0)
	status := SwapchainOK
	switch {
	case err == nil:
	case errors.Is(err, vk.SuboptimalKHR):
		status = SwapchainSuboptimal
	case errors.Is(err, vk.ErrorOutOfDateKHR):
		return 0, SwapchainOutOfDate, nil
	case errors.Is(err, vk.NotReady), errors.Is(err, vk.Timeout):
		return 0, SwapchainNotReady, nil
	default:
		return 0, 0, s.d.check(fmt.Errorf("gpu: acquiring an image of a swapchain: %w", err))
	}
	s.waiting, s.next = sem, (s.next+1)%len(s.acquire)
	return int(index), status, nil
}

func (s *vkSwapchain) Present(t Target, index int, copies, damage []image.Rectangle) (SwapchainStatus, error) {
	if err := s.usable(); err != nil {
		return 0, err
	}
	d := s.d
	vt, ok := t.(*vkTarget)
	if !ok || vt.d != d {
		return 0, errors.New("gpu: a swapchain presents a target of its own device")
	}
	if vt.size != s.extent {
		return 0, fmt.Errorf("gpu: a swapchain of %v presents a target of %v", s.extent, vt.size)
	}
	if index < 0 || index >= len(s.images) || s.waiting == 0 {
		return 0, fmt.Errorf("gpu: presenting the image %d of a swapchain, which was not acquired", index)
	}
	img := s.images[index]
	whole := image.Rectangle{Max: s.extent}
	var regions []vk.ImageCopy
	var blits []vk.ImageBlit
	for _, r := range copies {
		if r = r.Intersect(whole); r.Empty() {
			continue
		}
		min := vk.Offset3D{X: int32(r.Min.X), Y: int32(r.Min.Y)}
		if s.convert {
			box := [2]vk.Offset3D{min, {X: int32(r.Max.X), Y: int32(r.Max.Y), Z: 1}}
			blits = append(blits, vk.ImageBlit{SrcSubresource: colorLayers, SrcOffsets: box, DstSubresource: colorLayers, DstOffsets: box})
		} else {
			regions = append(regions, vk.ImageCopy{SrcSubresource: colorLayers, DstSubresource: colorLayers, SrcOffset: min, DstOffset: min,
				Extent: vk.Extent3D{Width: uint32(r.Dx()), Height: uint32(r.Dy()), Depth: 1}})
		}
	}
	from := vk.ImageLayoutUndefined // no pixels yet
	if s.presented[index] {
		from = vk.ImageLayoutPresentSrcKHR // keeps what it was given
	}
	dev := d.dev
	sub := vk.ImageSubresourceRange{AspectMask: vk.ImageAspectColor, LevelCount: 1, LayerCount: 1}
	wait := s.waiting
	s.waiting = 0
	err := d.runWith(wait, vk.PipelineStageTransfer, s.ready[index], func(cb vk.CommandBuffer) {
		d.use(cb, vt.surf, vk.ImageLayoutTransferSrcOptimal)
		take := transition(img, from, vk.ImageLayoutTransferDstOptimal, 0, vk.AccessTransferWrite)
		take.SubresourceRange = sub
		// The semaphore's wait is at the transfer stage: the barrier
		// starts there too, after the presentation engine let go.
		dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageTransfer, 0, nil, nil, []vk.ImageMemoryBarrier{take})
		switch {
		case len(blits) > 0:
			dev.CmdBlitImage(cb, vt.surf.tex.img, vk.ImageLayoutTransferSrcOptimal, img, vk.ImageLayoutTransferDstOptimal, blits, vk.FilterNearest)
		case len(regions) > 0:
			dev.CmdCopyImage(cb, vt.surf.tex.img, vk.ImageLayoutTransferSrcOptimal, img, vk.ImageLayoutTransferDstOptimal, regions)
		}
		give := transition(img, vk.ImageLayoutTransferDstOptimal, vk.ImageLayoutPresentSrcKHR, vk.AccessTransferWrite, 0)
		give.SubresourceRange = sub
		dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageBottomOfPipe, 0, nil, nil, []vk.ImageMemoryBarrier{give})
	})
	if err != nil {
		return 0, d.check(err)
	}
	s.presented[index] = true

	ready, chain, idx := s.ready[index], s.chain, uint32(index)
	info := &vk.PresentInfoKHR{
		SType:              vk.StructureTypePresentInfoKHR,
		WaitSemaphoreCount: 1,
		PWaitSemaphores:    &ready,
		SwapchainCount:     1,
		PSwapchains:        &chain,
		PImageIndices:      &idx,
	}
	if d.incremental && len(damage) > 0 {
		rects := make([]vk.RectLayerKHR, 0, len(damage))
		for _, r := range damage {
			if r = r.Intersect(whole); !r.Empty() {
				rects = append(rects, vk.RectLayerKHR{
					Offset: vk.Offset2D{X: int32(r.Min.X), Y: int32(r.Min.Y)},
					Extent: vk.Extent2D{Width: uint32(r.Dx()), Height: uint32(r.Dy())},
				})
			}
		}
		if len(rects) > 0 {
			info.PNext = unsafe.Pointer(&vk.PresentRegionsKHR{
				SType:          vk.StructureTypePresentRegionsKHR,
				SwapchainCount: 1,
				PRegions:       &vk.PresentRegionKHR{RectangleCount: uint32(len(rects)), PRectangles: &rects[0]},
			})
		}
	}
	switch err := dev.QueuePresentKHR(d.queue, info); {
	case err == nil:
		return SwapchainOK, nil
	case errors.Is(err, vk.SuboptimalKHR):
		return SwapchainSuboptimal, nil
	case errors.Is(err, vk.ErrorOutOfDateKHR):
		return SwapchainOutOfDate, nil
	default:
		return 0, d.check(fmt.Errorf("gpu: presenting to a window: %w", err))
	}
}

func (s *vkSwapchain) DestroySwapchain() {
	if s.d == nil || s.d.closed {
		return
	}
	chain := s.chain
	s.chain = 0
	s.dropChain(chain)
}

func (s *vkSwapchain) Close() {
	if s.d == nil || s.d.closed {
		return
	}
	delete(s.d.windows, s)
	s.destroy()
}

// destroy destroys the swapchain and the surface; the device may be
// closing.
func (s *vkSwapchain) destroy() {
	d := s.d
	if d == nil {
		return
	}
	chain := s.chain
	s.chain = 0
	s.dropChain(chain)
	d.inst.DestroySurfaceKHR(s.surface)
	s.surface = 0
	s.d = nil
}
