//go:build telagpu && linux

package gpu

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/vk"
)

// The dmabufs the Vulkan device imports (import.go, ADR-316).

// importCache is what a device asked its driver of the imports, once: the
// modifiers it samples images with, by Vulkan format, whether it copies
// from linear buffers, and the pairs of ImportFormats.
type importCache struct {
	importMods    map[vk.Format][]vk.DmabufModifier
	importBuffers bool
	importOwn     []DmabufFormat
	importForeign []DmabufFormat
	importAsk     bool
}

// forget drops what the driver said, when the device closes.
func (c *importCache) forget() { *c = importCache{} }

// importUsage is how an imported image is used: sampled by the shaders, and
// copied from to read it back.
const importUsage = vk.ImageUsageSampled | vk.ImageUsageTransferSrc

// importFourccs are the formats imported.
var importFourccs = [...]uint32{FourccARGB8888, FourccXRGB8888, FourccABGR8888, FourccXBGR8888}

// fourccString spells the format's fourcc.
func (f DmabufFormat) fourccString() string {
	c := f.Fourcc
	return string([]byte{byte(c), byte(c >> 8), byte(c >> 16), byte(c >> 24)})
}

// vkImported is an imported dmabuf.
type vkImported struct {
	d    *vkDevice
	mem  *vk.ImportedMemory // an image, or for a copied one a buffer
	kind imageKind
	// view and set are the image's. A copied one has none.
	view vk.ImageView
	set  vk.DescriptorSet
	pool *imagePool
	// off and stride place the rows of a copied one in its buffer, and size
	// is the buffer's.
	off, stride, size int
	closed            atomic.Bool // set under d.mu
}

var _ Importer = (*vkDevice)(nil)

// askImports asks the driver, once, for the layouts it imports.
func (d *vkDevice) askImports() {
	if d.importAsk {
		return
	}
	d.importAsk = true
	if d.imports == nil {
		return
	}
	pd := d.info.Device
	d.importMods = map[vk.Format][]vk.DmabufModifier{}
	d.importBuffers = d.inst.DmabufBufferImportable(pd, vk.BufferUsageTransferSrc)
	const copyInto = vk.FormatFeatureSampledImage | vk.FormatFeatureTransferDst
	for _, fourcc := range importFourccs {
		format := exportVkFormat(fourcc)
		mods, ok := d.importMods[format]
		if !ok {
			for _, m := range d.inst.DmabufImportModifiers(pd, format, importUsage) {
				if m.Planes >= 1 && m.Planes <= 4 && m.Modifier != ModifierInvalid {
					mods = append(mods, m)
				}
			}
			// A tiled layout is what a device draws from fastest.
			slices.SortStableFunc(mods, func(a, b vk.DmabufModifier) int {
				return linearLast(a.Modifier) - linearLast(b.Modifier)
			})
			d.importMods[format] = mods
		}
		linear := false
		for _, m := range mods {
			d.importOwn = append(d.importOwn, DmabufFormat{Fourcc: fourcc, Modifier: m.Modifier})
			linear = linear || m.Modifier == ModifierLinear
		}
		if d.importBuffers && !linear && d.inst.GetPhysicalDeviceFormatProperties(pd, format).OptimalTilingFeatures&copyInto == copyInto {
			d.importOwn = append(d.importOwn, DmabufFormat{Fourcc: fourcc, Modifier: ModifierLinear})
			linear = true
		}
		if linear {
			d.importForeign = append(d.importForeign, DmabufFormat{Fourcc: fourcc, Modifier: ModifierLinear})
		}
	}
}

func linearLast(m uint64) int {
	if m == ModifierLinear {
		return 1
	}
	return 0
}

func (d *vkDevice) ImportFormats() (own, foreign []DmabufFormat) {
	d.resMu.Lock()
	defer d.resMu.Unlock()
	if d.usable() != nil {
		return nil, nil
	}
	d.askImports()
	return d.importOwn, d.importForeign
}

func (d *vkDevice) Import(desc DmabufDescriptor) (*Imported, error) {
	d.resMu.Lock()
	defer d.resMu.Unlock()
	if err := d.usable(); err != nil {
		return nil, err
	}
	d.askImports()
	l, err := d.importLayout(&desc)
	if err != nil {
		return nil, fmt.Errorf("gpu: a dmabuf cannot be imported: %w", err)
	}
	if err := d.reach(l.FD); err != nil {
		return nil, fmt.Errorf("gpu: a dmabuf cannot be imported: %w", err)
	}
	m, err := d.importImage(l)
	if (err != nil || !m.mem.Local) && desc.Modifier == ModifierLinear && d.importBuffers {
		// The device does not sample the rows where they lie, or would
		// through its bus, each time it draws them: one copy costs less
		// (measured: a frame that samples 1280x720 pixels of system memory
		// takes the RTX 3060 Ti 1.0 ms, one that copies them first 0.5).
		switch buf, bufErr := d.importBuffer(l); {
		case bufErr == nil:
			if err == nil {
				m.destroy()
			}
			m, err = buf, nil
		case err != nil:
			err = fmt.Errorf("%w; %w", err, bufErr)
		}
	}
	if err != nil {
		// The descriptor is right, as far as importLayout can tell: what a
		// linear one means is known, and a tiled one is what the driver
		// makes of it.
		if desc.Modifier == ModifierLinear || !errors.Is(err, vk.ErrDmabufLayout) && !errors.Is(err, vk.ErrorInvalidDrmFormatModifierPlaneLayoutEXT) {
			err = unsupported(err)
		}
		return nil, d.check(fmt.Errorf("gpu: a dmabuf cannot be imported: %w", err))
	}
	m.kind.opaque = DmabufFormat{Fourcc: desc.Fourcc}.opaque()
	if m.mem.Image != 0 {
		if m.view, err = d.newView(m.mem.Image, vk.ImageViewType2D, l.Format, m.kind.opaque); err == nil {
			m.set, m.pool, err = d.newImageSet(m.view)
		}
		if err != nil {
			m.destroy()
			return nil, d.check(fmt.Errorf("gpu: %w", err))
		}
	}
	d.imports[m] = struct{}{}
	return newImported(d, &desc, m.mem.Image == 0, m), nil
}

// unsupported marks an error as one of what the device does not take.
func unsupported(err error) error { return fmt.Errorf("%w: %w", ErrImportUnsupported, err) }

// importLayout checks a descriptor and returns what it says to the driver.
// What it says is never trusted: a driver is handed only a layout that lies
// inside the dmabuf.
func (d *vkDevice) importLayout(desc *DmabufDescriptor) (*vk.DmabufLayout, error) {
	if !slices.Contains(importFourccs[:], desc.Fourcc) {
		return nil, unsupported(fmt.Errorf("the format %#08x %q (AR24, XR24, AB24 and XB24 are)", desc.Fourcc, DmabufFormat{Fourcc: desc.Fourcc}.fourccString()))
	}
	if desc.Width < 1 || desc.Height < 1 {
		return nil, fmt.Errorf("the size %dx%d", desc.Width, desc.Height)
	}
	if desc.Width > d.maxSize || desc.Height > d.maxSize {
		return nil, unsupported(fmt.Errorf("the size %dx%d: it must be between 1 and %d", desc.Width, desc.Height, d.maxSize))
	}
	if desc.Modifier == ModifierInvalid {
		return nil, unsupported(errors.New("a layout without an explicit modifier (DRM_FORMAT_MOD_INVALID)"))
	}
	if !slices.Contains(d.importOwn, DmabufFormat{Fourcc: desc.Fourcc, Modifier: desc.Modifier}) {
		return nil, unsupported(fmt.Errorf("%s does not import the format %q with the modifier %#x", d.info.Name, DmabufFormat{Fourcc: desc.Fourcc}.fourccString(), desc.Modifier))
	}
	format := exportVkFormat(desc.Fourcc)
	planes := 1 // of a linear layout only copied
	for _, m := range d.importMods[format] {
		if m.Modifier == desc.Modifier {
			planes = int(m.Planes)
			if desc.Width > int(m.MaxExtent.Width) || desc.Height > int(m.MaxExtent.Height) {
				return nil, unsupported(fmt.Errorf("the size %dx%d: with the modifier %#x it is at most %dx%d", desc.Width, desc.Height, desc.Modifier, m.MaxExtent.Width, m.MaxExtent.Height))
			}
		}
	}
	if len(desc.Planes) != planes {
		return nil, fmt.Errorf("%d planes: the modifier %#x has %d", len(desc.Planes), desc.Modifier, planes)
	}
	l := &vk.DmabufLayout{Format: format, Width: uint32(desc.Width), Height: uint32(desc.Height), Modifier: desc.Modifier, FD: desc.Planes[0].FD}
	// The dmabuf, and that it is the one of every plane.
	var first unix.Stat_t
	for i, p := range desc.Planes {
		var st unix.Stat_t
		if err := unix.Fstat(p.FD, &st); err != nil {
			return nil, fmt.Errorf("the descriptor %d of plane %d: %w", p.FD, i, err)
		}
		var fs unix.Statfs_t
		if err := unix.Fstatfs(p.FD, &fs); err != nil {
			return nil, fmt.Errorf("the descriptor %d of plane %d: %w", p.FD, i, err)
		}
		if uint32(fs.Type) != unix.DMA_BUF_MAGIC {
			return nil, fmt.Errorf("the descriptor %d of plane %d is not a dmabuf", p.FD, i)
		}
		if i == 0 {
			first = st
		} else if st.Dev != first.Dev || st.Ino != first.Ino {
			return nil, unsupported(fmt.Errorf("plane %d is in another dmabuf than plane 0", i))
		}
		if i == 0 {
			size, err := unix.Seek(p.FD, 0, unix.SEEK_END)
			if err != nil || size <= 0 {
				return nil, fmt.Errorf("the size of the dmabuf: %d, %w", size, err)
			}
			l.Size = uint64(size)
		}
		l.Planes = append(l.Planes, vk.DmabufPlane{Offset: uint64(p.Offset), Stride: uint64(p.Stride)})
		if uint64(p.Offset) >= l.Size || p.Stride == 0 {
			return nil, fmt.Errorf("plane %d at the offset %d with the stride %d, in a dmabuf of %d bytes", i, p.Offset, p.Stride, l.Size)
		}
	}
	// The first plane holds the pixels: whatever the layout, at least rows
	// of its stride.
	row, p := 4*uint64(desc.Width), l.Planes[0]
	if p.Stride < row {
		return nil, fmt.Errorf("rows of %d bytes with the stride %d", row, p.Stride)
	}
	if need := p.Offset + p.Stride*uint64(desc.Height-1) + row; need > l.Size { // of 32-bit values: no overflow
		return nil, fmt.Errorf("%d rows with the stride %d at the offset %d need %d bytes, the dmabuf has %d", desc.Height, p.Stride, p.Offset, need, l.Size)
	}
	if p.Stride%4 != 0 || p.Offset%4 != 0 {
		return nil, unsupported(fmt.Errorf("the stride %d and the offset %d, which must be multiples of 4", p.Stride, p.Offset))
	}
	return l, nil
}

// sampleStride is what the stride of a linear image must be a multiple of
// for a device to sample it as it lies. The drivers do not say: NVIDIA's
// (615) takes a stride of 1028 for 256 pixels, copies from it right, and
// samples other pixels than the ones there; multiples of 32 are right, and
// its own strides are multiples of 128. Intel samples any.
func (d *vkDevice) sampleStride() uint64 {
	if d.info.VendorID == 0x8086 {
		return 4
	}
	return 128
}

// importImage makes an image over the dmabuf.
func (d *vkDevice) importImage(l *vk.DmabufLayout) (*vkImported, error) {
	if !slices.ContainsFunc(d.importMods[l.Format], func(m vk.DmabufModifier) bool { return m.Modifier == l.Modifier }) {
		return nil, fmt.Errorf("%s samples no image with the modifier %#x", d.info.Name, l.Modifier)
	}
	if n := d.sampleStride(); l.Modifier == ModifierLinear && l.Planes[0].Stride%n != 0 {
		return nil, fmt.Errorf("%s samples a linear image with a stride that is a multiple of %d, not %d", d.info.Name, n, l.Planes[0].Stride)
	}
	mem, err := d.dev.ImportDmabufImage(d.mem, l, importUsage)
	if err != nil {
		return nil, err
	}
	return &vkImported{d: d, mem: mem, kind: imageKind{w: int(l.Width), h: int(l.Height), format: l.Format}}, nil
}

// importBuffer makes a buffer over the rows of a linear dmabuf.
func (d *vkDevice) importBuffer(l *vk.DmabufLayout) (*vkImported, error) {
	p := l.Planes[0]
	if p.Stride > math.MaxInt32 {
		return nil, fmt.Errorf("the stride %d", p.Stride)
	}
	size := p.Offset + p.Stride*uint64(l.Height-1) + 4*uint64(l.Width)
	mem, err := d.dev.ImportDmabufBuffer(d.mem, l.FD, size, l.Size, vk.BufferUsageTransferSrc)
	if err != nil {
		return nil, err
	}
	return &vkImported{d: d, mem: mem, kind: imageKind{w: int(l.Width), h: int(l.Height), format: l.Format},
		off: int(p.Offset), stride: int(p.Stride), size: int(size)}, nil
}

// ---------------------------------------------------------------------------
// Memory the device cannot reach
// ---------------------------------------------------------------------------

// The DRM ioctls of reach.
const (
	drmIoctlPrimeFDToHandle  = 0xc00c642e // DRM_IOCTL_PRIME_FD_TO_HANDLE
	drmIoctlI915GemSetDomain = 0x400c645f // DRM_IOCTL_I915_GEM_SET_DOMAIN
	i915GemDomainGTT         = 0x40       // I915_GEM_DOMAIN_GTT
)

// drmPrimeHandle is struct drm_prime_handle.
type drmPrimeHandle struct {
	handle, flags uint32
	fd            int32
}

// i915GemSetDomain is struct drm_i915_gem_set_domain.
type i915GemSetDomain struct{ handle, readDomains, writeDomain uint32 }

// reach returns an error when the device cannot reach the memory of a
// dmabuf that another device exported. The kernel's answer depends on the
// driver: NVIDIA's asks for the memory when it is imported, which is then a
// plain error. i915 takes any dmabuf, and asks for its pages when a
// submission uses it: the one of memory it cannot have (the video memory of
// an NVIDIA GPU) makes Mesa try for 17 seconds and lose the device. So for
// i915 the pages are asked for here, with the one request of its interface
// that does nothing else with them. Other drivers are trusted: amdgpu's and
// xe's are not known to do either, and no such device was there to ask.
func (d *vkDevice) reach(fd int) error {
	drm := d.info.DRM
	if !drm.HasRender || drmDriver(drm.RenderMajor, drm.RenderMinor) != "i915" || dmabufExporter(fd) == "i915" {
		return nil // another driver, or its own memory
	}
	node, err := openDRM(uint64(unix.Mkdev(uint32(drm.RenderMajor), uint32(drm.RenderMinor))))
	if err != nil {
		return nil // no node to ask with
	}
	defer unix.Close(node) // and with it the handle
	prime := &drmPrimeHandle{fd: int32(fd)}
	if err := ioctl(node, drmIoctlPrimeFDToHandle, unsafe.Pointer(prime)); err != nil {
		return unsupported(fmt.Errorf("%s does not take the descriptor: %w", d.info.Name, err))
	}
	domain := &i915GemSetDomain{handle: prime.handle, readDomains: i915GemDomainGTT}
	if err := ioctl(node, drmIoctlI915GemSetDomain, unsafe.Pointer(domain)); err != nil && err != unix.ENODEV {
		return unsupported(fmt.Errorf("%s cannot reach the memory, which another device holds (frames for another GPU need system memory): %w", d.info.Name, err))
	}
	return nil
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	for {
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
		switch errno {
		case 0:
			return nil
		case unix.EINTR, unix.EAGAIN: // as libdrm's drmIoctl
		default:
			return errno
		}
	}
}

// dmabufExporter returns the name of the driver that exported a dmabuf, as
// the kernel tells it ("i915"; "drm" for the ones that use the common
// code), or "".
func dmabufExporter(fd int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/self/fdinfo/%d", fd))
	if err != nil {
		return ""
	}
	for line := range strings.Lines(string(data)) {
		if name, ok := strings.CutPrefix(line, "exp_name:"); ok {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// drmDriver returns the name of the kernel driver of a DRM node, or "".
func drmDriver(major, minor int64) string {
	link, err := os.Readlink(fmt.Sprintf("/sys/dev/char/%d:%d/device/driver", major, minor))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

// openDRM opens the DRM node with a device number.
func openDRM(dev uint64) (int, error) {
	nodes, err := filepath.Glob("/dev/dri/renderD*")
	if err != nil {
		return -1, err
	}
	for _, name := range nodes {
		var st unix.Stat_t
		if unix.Stat(name, &st) == nil && uint64(st.Rdev) == dev {
			return unix.Open(name, unix.O_RDWR|unix.O_CLOEXEC, 0)
		}
	}
	return -1, unix.ENOENT
}

// ---------------------------------------------------------------------------
// Closing
// ---------------------------------------------------------------------------

func (m *vkImported) open() bool { return !m.closed.Load() && !m.d.closed }

func (m *vkImported) close() {
	d := m.d
	// Every submission is waited for under the locks: none reads the memory
	// once they are taken, and no frame that does is about to be submitted.
	d.resMu.Lock()
	defer d.resMu.Unlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	if m.closed.Load() || d.closed {
		return
	}
	m.closed.Store(true)
	delete(d.imports, m)
	if d.lost.Load() {
		// Work may still run: the memory goes when the device is closed,
		// which waits for it.
		d.held = append(d.held, m.destroy)
		return
	}
	m.destroy()
}

// destroy destroys the buffer's objects. Nothing that reads them is in
// flight.
func (m *vkImported) destroy() {
	d := m.d
	if m.pool != nil {
		d.freeImageSet(m.set, m.pool)
		m.set, m.pool = 0, nil
	}
	if m.view != 0 {
		d.dev.DestroyImageView(m.view)
		m.view = 0
	}
	d.dev.DestroyImported(m.mem)
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

// checkImports returns an error when a frame reads an imported buffer the
// device does not have.
func (d *vkDevice) checkImports(f *Frame) error {
	check := func(im *Imported, copied bool) error {
		m, ok := im.mem.(*vkImported)
		if !ok || m.d != d || im.copied != copied {
			return errors.New("gpu: a frame reads a buffer imported by another device")
		}
		if !m.open() {
			return errors.New("gpu: a frame reads an imported buffer that was closed")
		}
		return nil
	}
	for _, im := range f.Imports {
		if err := check(im, false); err != nil {
			return err
		}
	}
	for i := range f.Images {
		if im := f.Images[i].From; im != nil {
			if err := check(im, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// importFamily is the queue family of whoever else uses imported memory.
func (d *vkDevice) importFamily() uint32 {
	if d.foreign {
		return vk.QueueFamilyForeignEXT
	}
	return vk.QueueFamilyExternal
}

// take records the barrier that takes an imported image from whoever drew
// it, into a layout: from the general layout, the one other APIs leave and
// read, with its pixels kept.
func (m *vkImported) take(cb vk.CommandBuffer, to vk.ImageLayout) {
	d := m.d
	stage, access := layoutSync(to)
	b := transition(m.mem.Image, vk.ImageLayoutGeneral, to, 0, access)
	b.SubresourceRange = planeRange
	b.SrcQueueFamilyIndex, b.DstQueueFamilyIndex = d.importFamily(), d.info.GraphicsQueueFamily
	d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTopOfPipe, stage, 0, nil, nil, []vk.ImageMemoryBarrier{b})
}

// give records the barrier that gives it back.
func (m *vkImported) give(cb vk.CommandBuffer, from vk.ImageLayout) {
	d := m.d
	stage, _ := layoutSync(from)
	b := transition(m.mem.Image, from, vk.ImageLayoutGeneral, 0, 0)
	b.SubresourceRange = planeRange
	b.SrcQueueFamilyIndex, b.DstQueueFamilyIndex = d.info.GraphicsQueueFamily, d.importFamily()
	d.dev.CmdPipelineBarrier(cb, stage, vk.PipelineStageBottomOfPipe, 0, nil, nil, []vk.ImageMemoryBarrier{b})
}

// planeRange is the one subresource of an image with a modifier.
var planeRange = vk.ImageSubresourceRange{AspectMask: vk.ImageAspectColor, LevelCount: 1, LayerCount: 1}

// takeImports records what takes the buffers a frame samples.
func (d *vkDevice) takeImports(cb vk.CommandBuffer, f *Frame) {
	for _, im := range f.Imports {
		im.mem.(*vkImported).take(cb, vk.ImageLayoutShaderReadOnlyOptimal)
	}
}

// giveImports records what gives them back.
func (d *vkDevice) giveImports(cb vk.CommandBuffer, f *Frame) {
	for _, im := range f.Imports {
		im.mem.(*vkImported).give(cb, vk.ImageLayoutShaderReadOnlyOptimal)
	}
}

// bufferBarriers returns what takes a copied buffer from whoever drew it,
// and what gives it back.
func (m *vkImported) bufferBarriers() (take, give vk.BufferMemoryBarrier) {
	d := m.d
	take = vk.BufferMemoryBarrier{
		SType:               vk.StructureTypeBufferMemoryBarrier,
		DstAccessMask:       vk.AccessTransferRead,
		SrcQueueFamilyIndex: d.importFamily(),
		DstQueueFamilyIndex: d.info.GraphicsQueueFamily,
		Buffer:              m.mem.Buffer,
		Size:                vk.WholeSize,
	}
	give = take
	give.DstAccessMask = 0
	give.SrcQueueFamilyIndex, give.DstQueueFamilyIndex = take.DstQueueFamilyIndex, take.SrcQueueFamilyIndex
	return take, give
}

// copyImport records the copy of a copied buffer's pixels into the texture
// of an image, which is in the layout of a copy's destination.
func (d *vkDevice) copyImport(cb vk.CommandBuffer, m *vkImported, into vk.Image) {
	take, give := m.bufferBarriers()
	d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTopOfPipe, vk.PipelineStageTransfer, 0, nil, []vk.BufferMemoryBarrier{take}, nil)
	d.dev.CmdCopyBufferToImage(cb, m.mem.Buffer, into, vk.ImageLayoutTransferDstOptimal, []vk.BufferImageCopy{{
		BufferOffset:     uint64(m.off),
		BufferRowLength:  uint32(m.stride / 4),
		ImageSubresource: colorLayers,
		ImageExtent:      vk.Extent3D{Width: uint32(m.kind.w), Height: uint32(m.kind.h), Depth: 1},
	}})
	d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageBottomOfPipe, 0, nil, []vk.BufferMemoryBarrier{give}, nil)
}

// ---------------------------------------------------------------------------
// Reading back
// ---------------------------------------------------------------------------

// pixels reads the buffer back, with objects of its own: it may run while
// the device's goroutine records or draws.
func (m *vkImported) pixels() []byte {
	d := m.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if m.closed.Load() || d.closed || d.lost.Load() {
		return nil
	}
	out, err := m.read()
	if err != nil {
		_ = d.check(err) // a lost device stays lost; there is no other use for the error
		return nil
	}
	return out
}

// read is pixels, under the lock.
//
// perf: every read makes a command buffer and a buffer of the size of the
// image: a window that stays on the CPU rasterizer pays that per frame.
func (m *vkImported) read() ([]byte, error) {
	d := m.d
	dev := d.dev
	w, h := m.kind.w, m.kind.h
	copied := m.mem.Image == 0
	n := 4 * w * h
	if copied {
		n = m.size - m.off
	}
	var buf hostBuffer
	pool, err := dev.CreateCommandPool(&vk.CommandPoolCreateInfo{
		SType:            vk.StructureTypeCommandPoolCreateInfo,
		QueueFamilyIndex: d.info.GraphicsQueueFamily,
	})
	if err != nil {
		return nil, err
	}
	fence := vk.Fence(0)
	free := func() {
		d.destroyBuffer(&buf)
		if fence != 0 {
			dev.DestroyFence(fence)
		}
		dev.DestroyCommandPool(pool) // frees the command buffer
	}
	defer func() {
		if free != nil {
			free()
		}
	}()
	if fence, err = dev.CreateFence(&vk.FenceCreateInfo{SType: vk.StructureTypeFenceCreateInfo}); err != nil {
		fence = 0
		return nil, err
	}
	if err := d.newBuffer(&buf, n, vk.BufferUsageTransferDst); err != nil {
		return nil, err
	}
	cbs, err := dev.AllocateCommandBuffers(&vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        pool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	})
	if err != nil {
		return nil, err
	}
	cb := cbs[0]
	if err := dev.BeginCommandBuffer(cb, &vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageOneTimeSubmit,
	}); err != nil {
		return nil, err
	}
	if copied {
		take, give := m.bufferBarriers()
		dev.CmdPipelineBarrier(cb, vk.PipelineStageTopOfPipe, vk.PipelineStageTransfer, 0, nil, []vk.BufferMemoryBarrier{take}, nil)
		dev.CmdCopyBuffer(cb, m.mem.Buffer, buf.buf, []vk.BufferCopy{{SrcOffset: uint64(m.off), Size: uint64(n)}})
		dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageBottomOfPipe, 0, nil, []vk.BufferMemoryBarrier{give}, nil)
	} else {
		m.take(cb, vk.ImageLayoutTransferSrcOptimal)
		dev.CmdCopyImageToBuffer(cb, m.mem.Image, vk.ImageLayoutTransferSrcOptimal, buf.buf, []vk.BufferImageCopy{{
			ImageSubresource: colorLayers,
			ImageExtent:      vk.Extent3D{Width: uint32(w), Height: uint32(h), Depth: 1},
		}})
		m.give(cb, vk.ImageLayoutTransferSrcOptimal)
	}
	dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageHost, 0, []vk.MemoryBarrier{{
		SType:         vk.StructureTypeMemoryBarrier,
		SrcAccessMask: vk.AccessTransferWrite,
		DstAccessMask: vk.AccessHostRead,
	}}, nil, nil)
	if err := dev.EndCommandBuffer(cb); err != nil {
		return nil, err
	}
	if err := dev.QueueSubmit(d.queue, []vk.SubmitInfo{{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    &cb,
	}}, fence); err != nil {
		return nil, err
	}
	if err := d.wait(fence); err != nil {
		// As in submitWith: the work may still run.
		d.lost.Store(true)
		d.held = append(d.held, free)
		free = nil
		return nil, err
	}
	stride := 4 * w
	if copied {
		stride = m.stride
	}
	out := make([]byte, 4*w*h)
	for y := range h {
		copy(out[4*w*y:4*w*(y+1)], buf.bytes[stride*y:])
	}
	if m.kind.format == bgraFormat {
		for i := 0; i < len(out); i += 4 {
			out[i], out[i+2] = out[i+2], out[i]
		}
	}
	if m.kind.opaque {
		for i := 3; i < len(out); i += 4 {
			out[i] = 255
		}
	}
	return out, nil
}
