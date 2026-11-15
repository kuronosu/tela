//go:build telagpu && linux

package vk

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DmabufPlane is one memory plane of an exported image: where it starts in
// the dmabuf and the bytes from one row to the next.
type DmabufPlane struct {
	Offset uint64
	Stride uint64
}

// DmabufImage is an image whose memory is exported as a dmabuf.
type DmabufImage struct {
	Image  Image
	Memory DeviceMemory
	Format Format
	Width  uint32
	Height uint32
	// Modifier is the DRM format modifier the driver chose among the ones
	// offered.
	Modifier uint64
	// Planes are the memory planes, all in the one dmabuf.
	Planes []DmabufPlane
	// FD is the dmabuf. The image owns it: [Device.DestroyDmabufImage]
	// closes it. Whoever is given the dmabuf (a compositor) gets a
	// duplicate, and keeps the memory alive as long as it holds it.
	FD int
}

// maxDmabufPlanes is the largest number of memory planes of an image
// (VK_IMAGE_ASPECT_MEMORY_PLANE_0_BIT_EXT to 3).
const maxDmabufPlanes = 4

// CreateDmabufImage creates a 2D image with one mip level and one layer
// whose memory is a dmabuf, with a modifier the driver picks among
// modifiers (those of [Instance.DmabufModifiers] for the same format and
// usage, narrowed to what the other side accepts), binds a dedicated allocation of memory to it
// and exports that. mem are the memory properties of the physical device.
// The image starts in [ImageLayoutUndefined] and is exclusive to one
// queue family. The device must have been created with the extensions of
// [DmabufExtensions].
func (d *Device) CreateDmabufImage(mem *PhysicalDeviceMemoryProperties, format Format, width, height uint32, usage ImageUsageFlags, modifiers []DmabufModifier) (*DmabufImage, error) {
	if len(modifiers) == 0 {
		return nil, fmt.Errorf("vk: CreateDmabufImage needs at least one DRM format modifier")
	}
	values := make([]uint64, len(modifiers))
	for i, m := range modifiers {
		values[i] = m.Modifier
	}
	list := &ImageDrmFormatModifierListCreateInfoEXT{
		SType:                  StructureTypeImageDrmFormatModifierListCreateInfoEXT,
		DrmFormatModifierCount: uint32(len(values)),
		PDrmFormatModifiers:    &values[0],
	}
	external := &ExternalMemoryImageCreateInfo{
		SType:       StructureTypeExternalMemoryImageCreateInfo,
		PNext:       unsafe.Pointer(list),
		HandleTypes: ExternalMemoryHandleTypeDmaBufEXT,
	}
	img, err := d.CreateImage(&ImageCreateInfo{
		SType:         StructureTypeImageCreateInfo,
		PNext:         unsafe.Pointer(external),
		ImageType:     ImageType2D,
		Format:        format,
		Extent:        Extent3D{Width: width, Height: height, Depth: 1},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       SampleCount1,
		Tiling:        ImageTilingDrmFormatModifierEXT,
		Usage:         usage,
		SharingMode:   SharingModeExclusive,
		InitialLayout: ImageLayoutUndefined,
	})
	if err != nil {
		return nil, fmt.Errorf("vk: creating a %dx%d image with a DRM format modifier: %w", width, height, err)
	}
	out := &DmabufImage{Image: img, Format: format, Width: width, Height: height, FD: -1}
	fail := func(what string, err error) (*DmabufImage, error) {
		d.DestroyDmabufImage(out)
		return nil, fmt.Errorf("vk: %s of a %dx%d dmabuf image: %w", what, width, height, err)
	}

	req := d.GetImageMemoryRequirements(img)
	// Any type the image allows: the memory of a dmabuf is where the
	// driver puts it, and device-local when there is a choice.
	typ, ok := mem.FindMemoryType(req.MemoryTypeBits, MemoryPropertyDeviceLocal)
	if !ok {
		if typ, ok = mem.FindMemoryType(req.MemoryTypeBits, 0); !ok {
			return fail("the memory", fmt.Errorf("no memory type among %#x", req.MemoryTypeBits))
		}
	}
	// A dedicated allocation: required by the drivers that say so
	// (DmabufModifier.DedicatedOnly, NVIDIA) and allowed by all, and it
	// makes the dmabuf hold this image and nothing else.
	dedicated := &MemoryDedicatedAllocateInfo{SType: StructureTypeMemoryDedicatedAllocateInfo, Image: img}
	export := &ExportMemoryAllocateInfo{
		SType:       StructureTypeExportMemoryAllocateInfo,
		PNext:       unsafe.Pointer(dedicated),
		HandleTypes: ExternalMemoryHandleTypeDmaBufEXT,
	}
	out.Memory, err = d.AllocateMemory(&MemoryAllocateInfo{
		SType:           StructureTypeMemoryAllocateInfo,
		PNext:           unsafe.Pointer(export),
		AllocationSize:  req.Size,
		MemoryTypeIndex: typ,
	})
	if err != nil {
		return fail("allocating the memory", err)
	}
	if err := d.BindImageMemory(img, out.Memory, 0); err != nil {
		return fail("binding the memory", err)
	}

	if out.Modifier, err = d.GetImageDrmFormatModifierPropertiesEXT(img); err != nil {
		return fail("reading the modifier", err)
	}
	// The number of memory planes is a property of the format and the
	// modifier; a plane the image lacks must not be asked for.
	planes := uint32(0)
	for _, m := range modifiers {
		if m.Modifier == out.Modifier {
			planes = m.Planes
		}
	}
	if planes == 0 || planes > maxDmabufPlanes {
		return fail("reading the modifier", fmt.Errorf("the driver chose the modifier %#x with %d planes, which is not one of the %d offered", out.Modifier, planes, len(modifiers)))
	}
	for p := range planes {
		layout := d.GetImageSubresourceLayout(img, &ImageSubresource{AspectMask: ImageAspectMemoryPlane0EXT << p})
		out.Planes = append(out.Planes, DmabufPlane{Offset: layout.Offset, Stride: layout.RowPitch})
	}

	out.FD, err = d.GetMemoryFdKHR(&MemoryGetFdInfoKHR{
		SType:      StructureTypeMemoryGetFdInfoKHR,
		Memory:     out.Memory,
		HandleType: ExternalMemoryHandleTypeDmaBufEXT,
	})
	if err != nil {
		return fail("exporting the memory", err)
	}
	return out, nil
}

// DestroyDmabufImage destroys the image, frees its memory and closes its
// dmabuf. Whoever holds a duplicate of the dmabuf keeps the memory alive.
// The device must have finished using the image.
func (d *Device) DestroyDmabufImage(img *DmabufImage) {
	if img.FD >= 0 {
		unix.Close(img.FD)
		img.FD = -1
	}
	if img.Image != 0 {
		d.DestroyImage(img.Image)
		img.Image = 0
	}
	if img.Memory != 0 {
		d.FreeMemory(img.Memory)
		img.Memory = 0
	}
}
