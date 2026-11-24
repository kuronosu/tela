//go:build telagpu && (linux || windows)

package vk

import "unsafe"

// Exporting images as dmabufs (ADR-281, ADR-283).
//
// On Linux the renderer presents without a Vulkan surface: it draws into
// images whose memory it exports as dmabuf file descriptors, and the
// window shows them (zwp_linux_dmabuf_v1 on Wayland, DRI3 on X11). What
// the other side needs to know about such an image is its DRM format
// modifier (the layout of the pixels in the memory: linear, tiled,
// compressed, with auxiliary planes) and, for every memory plane, its
// offset and row pitch in the dmabuf. That takes three extensions, and
// their wrappers and helpers are here, apart from the rest of the
// bindings:
//
//   - VK_KHR_external_memory_fd: vkGetMemoryFdKHR, a file descriptor of a
//     memory allocation ([Device.GetMemoryFdKHR]).
//   - VK_EXT_external_memory_dma_buf: the dmabuf kind of such a
//     descriptor ([ExternalMemoryHandleTypeDmaBufEXT]).
//   - VK_EXT_image_drm_format_modifier: the modifiers a device has for a
//     format ([Instance.DrmFormatModifiers]), images created with one of a
//     list of them ([ImageTilingDrmFormatModifierEXT],
//     [ImageDrmFormatModifierListCreateInfoEXT]), the one the driver chose
//     ([Device.GetImageDrmFormatModifierPropertiesEXT]) and the layout of
//     its memory planes ([Device.GetImageSubresourceLayout] with
//     [ImageAspectMemoryPlane0EXT] and the next ones).
//
// [DmabufExtensions] lists what a device must enable,
// [Instance.DmabufModifiers] the modifiers it can export for a format and
// a usage, and [Device.CreateDmabufImage] creates and exports an image.
// VK_EXT_queue_family_foreign ([QueueFamilyForeignEXT]) is enabled too when
// the device has it: a barrier that transfers the image to that family
// after rendering, and back before the next rendering, is what tells the
// driver that somebody else reads the pixels.

// DmabufExtensions returns the names of the device extensions the export
// of dmabufs needs, to enable at vkCreateDevice, and whether the device
// has them all. VK_KHR_image_format_list, which the modifiers extension
// requires and Vulkan 1.2 includes, is listed for the devices that have it
// as an extension, and VK_EXT_queue_family_foreign when the device has it.
func DmabufExtensions(info *DeviceInfo) ([]string, bool) {
	names := []string{KhrExternalMemoryFdExtensionName, ExtExternalMemoryDmaBufExtensionName, ExtImageDrmFormatModifierExtensionName}
	for _, name := range names {
		if !info.HasExtension(name) {
			return nil, false
		}
	}
	for _, name := range []string{KhrImageFormatListExtensionName, ExtQueueFamilyForeignExtensionName} {
		if info.HasExtension(name) {
			names = append(names, name)
		}
	}
	if !info.HasExtension(KhrImageFormatListExtensionName) && info.APIVersion < APIVersion1_2 {
		return nil, false
	}
	return names, true
}

// forgetDisabled clears the entry points of the device extensions that
// info does not enable: some drivers return one for a command whose
// extension is off, and calling it is an error (as in CreateInstance).
func (d *Device) forgetDisabled(info *DeviceCreateInfo) {
	enabled := map[string]bool{}
	for _, name := range unsafe.Slice(info.PpEnabledExtensionNames, info.EnabledExtensionCount) {
		enabled[GoString(name)] = true
	}
	if !enabled[KhrExternalMemoryFdExtensionName] {
		d.fn.GetMemoryFdKHR = 0
		d.fn.GetMemoryFdPropertiesKHR = 0
	}
	if !enabled[ExtImageDrmFormatModifierExtensionName] {
		d.fn.GetImageDrmFormatModifierPropertiesEXT = 0
	}
}

// GetPhysicalDeviceFormatProperties2 is vkGetPhysicalDeviceFormatProperties2:
// it fills p and the structs chained to p.PNext, whose STypes the caller
// has set. It reports false, and leaves p alone, when the instance does
// not have the command (Vulkan 1.0).
func (i *Instance) GetPhysicalDeviceFormatProperties2(pd PhysicalDevice, format Format, p *FormatProperties2) bool {
	if i.fn.GetPhysicalDeviceFormatProperties2 == 0 {
		return false
	}
	call3(i.fn.GetPhysicalDeviceFormatProperties2, uintptr(pd), uintptr(format), uintptr(unsafe.Pointer(p)))
	return true
}

// GetPhysicalDeviceImageFormatProperties2 is
// vkGetPhysicalDeviceImageFormatProperties2: the limits of the images that
// info and the structs chained to it describe, in p and the structs
// chained to it, or [ErrorFormatNotSupported]. It returns
// [ErrorExtensionNotPresent] when the instance does not have the command
// (Vulkan 1.0).
func (i *Instance) GetPhysicalDeviceImageFormatProperties2(pd PhysicalDevice, info *PhysicalDeviceImageFormatInfo2, p *ImageFormatProperties2) error {
	if i.fn.GetPhysicalDeviceImageFormatProperties2 == 0 {
		return ErrorExtensionNotPresent
	}
	r := call3(i.fn.GetPhysicalDeviceImageFormatProperties2, uintptr(pd), uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(p)))
	return result(r)
}

// DrmFormatModifiers returns the DRM format modifiers the device has for a
// format, with the number of memory planes and the format features of
// each (VkDrmFormatModifierPropertiesListEXT through
// vkGetPhysicalDeviceFormatProperties2). The device must have
// VK_EXT_image_drm_format_modifier; the list is empty for a format it has
// no modifiers for.
func (i *Instance) DrmFormatModifiers(pd PhysicalDevice, format Format) []DrmFormatModifierPropertiesEXT {
	list := &DrmFormatModifierPropertiesListEXT{SType: StructureTypeDrmFormatModifierPropertiesListEXT}
	props := &FormatProperties2{SType: StructureTypeFormatProperties2, PNext: unsafe.Pointer(list)}
	if !i.GetPhysicalDeviceFormatProperties2(pd, format, props) || list.DrmFormatModifierCount == 0 {
		return nil
	}
	out := make([]DrmFormatModifierPropertiesEXT, list.DrmFormatModifierCount)
	list.PDrmFormatModifierProperties = &out[0]
	i.GetPhysicalDeviceFormatProperties2(pd, format, props)
	return out[:min(list.DrmFormatModifierCount, uint32(len(out)))]
}

// GetImageSubresourceLayout is vkGetImageSubresourceLayout: where a
// subresource of an image lies in its memory. For an image with a DRM
// format modifier the aspect is one of its memory planes
// ([ImageAspectMemoryPlane0EXT] and the next ones), and the answer is the
// plane's offset and row pitch (stride) in the dmabuf; other images must
// have linear tiling.
func (d *Device) GetImageSubresourceLayout(img Image, sub *ImageSubresource) SubresourceLayout {
	p := new(SubresourceLayout)
	call4(d.fn.GetImageSubresourceLayout, uintptr(d.h), uintptr(img), uintptr(unsafe.Pointer(sub)), uintptr(unsafe.Pointer(p)))
	return *p
}

// GetMemoryFdKHR is vkGetMemoryFdKHR: a new file descriptor of the memory,
// which the caller owns and closes. The memory must have been allocated
// with an [ExportMemoryAllocateInfo] that names info.HandleType. It
// returns [ErrorExtensionNotPresent] when the device was created without
// VK_KHR_external_memory_fd.
func (d *Device) GetMemoryFdKHR(info *MemoryGetFdInfoKHR) (int, error) {
	if d.fn.GetMemoryFdKHR == 0 {
		return -1, ErrorExtensionNotPresent
	}
	fd := new(int32) // a C int
	*fd = -1
	r := call3(d.fn.GetMemoryFdKHR, uintptr(d.h), uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(fd)))
	if err := result(r); err != nil {
		return -1, err
	}
	return int(*fd), nil
}

// GetImageDrmFormatModifierPropertiesEXT is
// vkGetImageDrmFormatModifierPropertiesEXT: the modifier the driver chose
// for an image created with [ImageTilingDrmFormatModifierEXT]. It returns
// [ErrorExtensionNotPresent] when the device was created without
// VK_EXT_image_drm_format_modifier.
func (d *Device) GetImageDrmFormatModifierPropertiesEXT(img Image) (uint64, error) {
	if d.fn.GetImageDrmFormatModifierPropertiesEXT == 0 {
		return 0, ErrorExtensionNotPresent
	}
	p := &ImageDrmFormatModifierPropertiesEXT{SType: StructureTypeImageDrmFormatModifierPropertiesEXT}
	r := call3(d.fn.GetImageDrmFormatModifierPropertiesEXT, uintptr(d.h), uintptr(img), uintptr(unsafe.Pointer(p)))
	return p.DrmFormatModifier, result(r)
}

// DmabufModifier is a DRM format modifier a device can export or import 2D
// images of one format and usage with.
type DmabufModifier struct {
	// Modifier is the DRM format modifier (0 is DRM_FORMAT_MOD_LINEAR).
	Modifier uint64
	// Planes is the number of memory planes of such an image: one for the
	// pixels and, with some modifiers, auxiliary ones (compression
	// metadata). They are all in the one dmabuf of the image's memory.
	Planes uint32
	// Features are what the format can do with this modifier.
	Features FormatFeatureFlags
	// MaxExtent is the largest image.
	MaxExtent Extent3D
	// DedicatedOnly reports that the image needs a memory allocation of
	// its own ([MemoryDedicatedAllocateInfo]);
	// [Device.CreateDmabufImage] always makes one.
	DedicatedOnly bool
}

// DmabufModifiers returns the modifiers with which the device can create a
// 2D image of the format and usage and export its memory as a dmabuf, in
// the driver's order: those whose features cover the usage and whose
// image format query reports exportable dmabuf memory. The device must
// have the extensions of [DmabufExtensions]. The list is empty when it can
// export none.
func (i *Instance) DmabufModifiers(pd PhysicalDevice, format Format, usage ImageUsageFlags) []DmabufModifier {
	return i.dmabufModifiers(pd, format, usage, ExternalMemoryFeatureExportable)
}

// DmabufImportModifiers is [Instance.DmabufModifiers] for images whose
// memory is a dmabuf made by somebody else: the modifiers whose image
// format query reports importable dmabuf memory.
func (i *Instance) DmabufImportModifiers(pd PhysicalDevice, format Format, usage ImageUsageFlags) []DmabufModifier {
	return i.dmabufModifiers(pd, format, usage, ExternalMemoryFeatureImportable)
}

// dmabufModifiers returns the modifiers whose external memory has a
// feature.
func (i *Instance) dmabufModifiers(pd PhysicalDevice, format Format, usage ImageUsageFlags, feature ExternalMemoryFeatureFlags) []DmabufModifier {
	// What the usage needs of the format. The image format query below
	// does not always say it: NVIDIA (615) answers it for a linear color
	// attachment, whose modifier lacks the color attachment feature.
	var need FormatFeatureFlags
	for _, u := range []struct {
		usage   ImageUsageFlags
		feature FormatFeatureFlags
	}{
		{ImageUsageColorAttachment, FormatFeatureColorAttachment},
		{ImageUsageSampled, FormatFeatureSampledImage},
		{ImageUsageStorage, FormatFeatureStorageImage},
		{ImageUsageTransferSrc, FormatFeatureTransferSrc},
		{ImageUsageTransferDst, FormatFeatureTransferDst},
	} {
		if usage&u.usage != 0 {
			need |= u.feature
		}
	}
	var out []DmabufModifier
	for _, m := range i.DrmFormatModifiers(pd, format) {
		if m.DrmFormatModifierTilingFeatures&need != need {
			continue
		}
		mod := &PhysicalDeviceImageDrmFormatModifierInfoEXT{
			SType:             StructureTypePhysicalDeviceImageDrmFormatModifierInfoEXT,
			DrmFormatModifier: m.DrmFormatModifier,
			SharingMode:       SharingModeExclusive,
		}
		ext := &PhysicalDeviceExternalImageFormatInfo{
			SType:      StructureTypePhysicalDeviceExternalImageFormatInfo,
			PNext:      unsafe.Pointer(mod),
			HandleType: ExternalMemoryHandleTypeDmaBufEXT,
		}
		info := &PhysicalDeviceImageFormatInfo2{
			SType:  StructureTypePhysicalDeviceImageFormatInfo2,
			PNext:  unsafe.Pointer(ext),
			Format: format,
			Type:   ImageType2D,
			Tiling: ImageTilingDrmFormatModifierEXT,
			Usage:  usage,
		}
		extProps := &ExternalImageFormatProperties{SType: StructureTypeExternalImageFormatProperties}
		props := &ImageFormatProperties2{SType: StructureTypeImageFormatProperties2, PNext: unsafe.Pointer(extProps)}
		if err := i.GetPhysicalDeviceImageFormatProperties2(pd, info, props); err != nil {
			continue // this usage is not possible with this modifier
		}
		features := extProps.ExternalMemoryProperties.ExternalMemoryFeatures
		if features&feature == 0 {
			continue
		}
		out = append(out, DmabufModifier{
			Modifier:      m.DrmFormatModifier,
			Planes:        m.DrmFormatModifierPlaneCount,
			Features:      m.DrmFormatModifierTilingFeatures,
			MaxExtent:     props.ImageFormatProperties.MaxExtent,
			DedicatedOnly: features&ExternalMemoryFeatureDedicatedOnly != 0,
		})
	}
	return out
}
