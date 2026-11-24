//go:build telagpu && linux

package vk

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DmabufLayout describes an image in somebody else's dmabuf.
type DmabufLayout struct {
	Format        Format
	Width, Height uint32
	Modifier      uint64
	// Planes are the memory planes, as many as the modifier has, all in
	// the one dmabuf FD.
	Planes []DmabufPlane
	// FD is the dmabuf. It stays the caller's: an import takes a
	// duplicate.
	FD int
	// Size is the size of the dmabuf in bytes.
	Size uint64
}

// ImportedMemory is an image or a buffer whose memory is a dmabuf.
type ImportedMemory struct {
	Image  Image  // of ImportDmabufImage
	Buffer Buffer // of ImportDmabufBuffer
	Memory DeviceMemory
	// Local reports that the memory is the device's own
	// ([MemoryPropertyDeviceLocal]): on a discrete GPU, not system memory
	// behind its bus.
	Local bool
}

// ImportDmabufImage creates a 2D image with one mip level and one layer
// over a dmabuf, at the offsets and strides of its planes. mem are the
// memory properties of the physical device. The image starts in
// [ImageLayoutUndefined]: a barrier from [ImageLayoutGeneral] that takes
// it from [QueueFamilyForeignEXT] keeps its pixels. The caller checks the
// layout against what it knows of the dmabuf. The driver's account of the
// image it made is checked here, before any memory is bound to it: a driver
// may make an image with other offsets or strides than the ones it was
// given (NVIDIA 615 rounds a stride up to its own minimum and says the
// offset is 0, and a submission that reads beyond the memory then loses the
// device), or read more than the dmabuf has. The device must have been
// created with the extensions of [DmabufExtensions].
func (d *Device) ImportDmabufImage(mem *PhysicalDeviceMemoryProperties, l *DmabufLayout, usage ImageUsageFlags) (*ImportedMemory, error) {
	if len(l.Planes) == 0 || len(l.Planes) > maxDmabufPlanes {
		return nil, fmt.Errorf("vk: importing a dmabuf of %d planes", len(l.Planes))
	}
	layouts := make([]SubresourceLayout, len(l.Planes))
	for i, p := range l.Planes {
		layouts[i] = SubresourceLayout{Offset: p.Offset, RowPitch: p.Stride}
	}
	explicit := &ImageDrmFormatModifierExplicitCreateInfoEXT{
		SType:                       StructureTypeImageDrmFormatModifierExplicitCreateInfoEXT,
		DrmFormatModifier:           l.Modifier,
		DrmFormatModifierPlaneCount: uint32(len(layouts)),
		PPlaneLayouts:               &layouts[0],
	}
	external := &ExternalMemoryImageCreateInfo{
		SType:       StructureTypeExternalMemoryImageCreateInfo,
		PNext:       unsafe.Pointer(explicit),
		HandleTypes: ExternalMemoryHandleTypeDmaBufEXT,
	}
	out := &ImportedMemory{}
	fail := func(what string, err error) (*ImportedMemory, error) {
		d.DestroyImported(out)
		return nil, fmt.Errorf("vk: importing a %dx%d dmabuf with the modifier %#x: %s: %w", l.Width, l.Height, l.Modifier, what, err)
	}
	var err error
	if out.Image, err = d.CreateImage(&ImageCreateInfo{
		SType:         StructureTypeImageCreateInfo,
		PNext:         unsafe.Pointer(external),
		ImageType:     ImageType2D,
		Format:        l.Format,
		Extent:        Extent3D{Width: l.Width, Height: l.Height, Depth: 1},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       SampleCount1,
		Tiling:        ImageTilingDrmFormatModifierEXT,
		Usage:         usage,
		SharingMode:   SharingModeExclusive,
		InitialLayout: ImageLayoutUndefined,
	}); err != nil {
		out.Image = 0
		return fail("creating the image", err)
	}
	// Where the driver finds the planes, and all it reads.
	req := d.GetImageMemoryRequirements(out.Image)
	if req.Size > l.Size {
		return fail("checking the layout", fmt.Errorf("the image needs %d bytes, the dmabuf has %d", req.Size, l.Size))
	}
	for i := range layouts {
		p := d.GetImageSubresourceLayout(out.Image, &ImageSubresource{AspectMask: ImageAspectMemoryPlane0EXT << i})
		if p.Offset != l.Planes[i].Offset || p.RowPitch != l.Planes[i].Stride {
			return fail("checking the layout", fmt.Errorf("plane %d is read with the stride %d at the offset %d, not with %d at %d", i, p.RowPitch, p.Offset, l.Planes[i].Stride, l.Planes[i].Offset))
		}
		if p.Offset > req.Size || p.Size > req.Size-p.Offset {
			return fail("checking the layout", fmt.Errorf("plane %d is %d bytes at %d, the image's memory is %d bytes", i, p.Size, p.Offset, req.Size))
		}
	}
	dedicated := &MemoryDedicatedAllocateInfo{SType: StructureTypeMemoryDedicatedAllocateInfo, Image: out.Image}
	if out.Memory, out.Local, err = d.importDmabuf(mem, l.FD, req, unsafe.Pointer(dedicated)); err != nil {
		return fail("importing the memory", err)
	}
	if err := d.BindImageMemory(out.Image, out.Memory, 0); err != nil {
		return fail("binding the memory", err)
	}
	return out, nil
}

// ImportDmabufBuffer creates a buffer over the first size bytes of a
// dmabuf of dmabufSize bytes. Its descriptor stays the caller's.
func (d *Device) ImportDmabufBuffer(mem *PhysicalDeviceMemoryProperties, fd int, size, dmabufSize uint64, usage BufferUsageFlags) (*ImportedMemory, error) {
	external := &ExternalMemoryBufferCreateInfo{
		SType:       StructureTypeExternalMemoryBufferCreateInfo,
		HandleTypes: ExternalMemoryHandleTypeDmaBufEXT,
	}
	out := &ImportedMemory{}
	fail := func(what string, err error) (*ImportedMemory, error) {
		d.DestroyImported(out)
		return nil, fmt.Errorf("vk: importing a dmabuf of %d bytes as a buffer: %s: %w", size, what, err)
	}
	var err error
	if out.Buffer, err = d.CreateBuffer(&BufferCreateInfo{
		SType:       StructureTypeBufferCreateInfo,
		PNext:       unsafe.Pointer(external),
		Size:        size,
		Usage:       usage,
		SharingMode: SharingModeExclusive,
	}); err != nil {
		out.Buffer = 0
		return fail("creating the buffer", err)
	}
	req := d.GetBufferMemoryRequirements(out.Buffer)
	if req.Size > dmabufSize {
		return fail("checking the size", fmt.Errorf("the buffer needs %d bytes, the dmabuf has %d", req.Size, dmabufSize))
	}
	dedicated := &MemoryDedicatedAllocateInfo{SType: StructureTypeMemoryDedicatedAllocateInfo, Buffer: out.Buffer}
	if out.Memory, out.Local, err = d.importDmabuf(mem, fd, req, unsafe.Pointer(dedicated)); err != nil {
		return fail("importing the memory", err)
	}
	if err := d.BindBufferMemory(out.Buffer, out.Memory, 0); err != nil {
		return fail("binding the memory", err)
	}
	return out, nil
}

// importDmabuf allocates memory that is a dmabuf, through a duplicate of
// its descriptor, and reports whether it is the device's own.
func (d *Device) importDmabuf(mem *PhysicalDeviceMemoryProperties, fd int, req MemoryRequirements, dedicated unsafe.Pointer) (DeviceMemory, bool, error) {
	bits, err := d.GetMemoryFdPropertiesKHR(ExternalMemoryHandleTypeDmaBufEXT, fd)
	if err != nil {
		return 0, false, fmt.Errorf("reading the memory types of the descriptor: %w", err)
	}
	typ, local := mem.FindMemoryType(req.MemoryTypeBits&bits, MemoryPropertyDeviceLocal)
	if !local {
		var ok bool
		if typ, ok = mem.FindMemoryType(req.MemoryTypeBits&bits, 0); !ok {
			return 0, false, fmt.Errorf("no memory type in common: the descriptor can be imported as %#x, the image or buffer needs %#x", bits, req.MemoryTypeBits)
		}
	}
	dup, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return 0, false, fmt.Errorf("duplicating the descriptor: %w", err)
	}
	imp := &ImportMemoryFdInfoKHR{
		SType:      StructureTypeImportMemoryFdInfoKHR,
		PNext:      dedicated,
		HandleType: ExternalMemoryHandleTypeDmaBufEXT,
		Fd:         int32(dup),
	}
	m, err := d.AllocateMemory(&MemoryAllocateInfo{
		SType:           StructureTypeMemoryAllocateInfo,
		PNext:           unsafe.Pointer(imp),
		AllocationSize:  req.Size,
		MemoryTypeIndex: typ,
	})
	if err != nil {
		unix.Close(dup) // a successful import owns the descriptor
		return 0, false, err
	}
	return m, local, nil
}

// DestroyImported destroys the image or the buffer and frees its memory.
// The device must have finished using them.
func (d *Device) DestroyImported(m *ImportedMemory) {
	if m.Image != 0 {
		d.DestroyImage(m.Image)
		m.Image = 0
	}
	if m.Buffer != 0 {
		d.DestroyBuffer(m.Buffer)
		m.Buffer = 0
	}
	if m.Memory != 0 {
		d.FreeMemory(m.Memory)
		m.Memory = 0
	}
}
