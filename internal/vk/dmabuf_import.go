//go:build telagpu && (linux || windows)

package vk

import "unsafe"

// Importing dmabufs (ADR-316): images whose memory somebody else
// allocated, an application's frames. The extensions are those of the
// export (dmabuf.go): VK_KHR_external_memory_fd has
// vkGetMemoryFdPropertiesKHR, the memory types a descriptor can be
// imported as ([Device.GetMemoryFdPropertiesKHR]), and
// [ImportMemoryFdInfoKHR]; VK_EXT_image_drm_format_modifier has the image
// whose layout is given instead of chosen
// ([ImageDrmFormatModifierExplicitCreateInfoEXT]).
//
// [Instance.DmabufImportModifiers] lists the modifiers a device imports,
// [Device.ImportDmabufImage] makes an image of a dmabuf and
// [Device.ImportDmabufBuffer] a buffer of one, for the linear layouts a
// device cannot read as an image. [Instance.GetPhysicalDeviceID] is how an
// application finds the same device with another API.

// GetMemoryFdPropertiesKHR is vkGetMemoryFdPropertiesKHR: the memory types
// (a bit per type) that a descriptor of a kind of handle can be imported
// as. It returns [ErrorExtensionNotPresent] when the device was created
// without VK_KHR_external_memory_fd.
func (d *Device) GetMemoryFdPropertiesKHR(handle ExternalMemoryHandleTypeFlags, fd int) (uint32, error) {
	if d.fn.GetMemoryFdPropertiesKHR == 0 {
		return 0, ErrorExtensionNotPresent
	}
	p := &MemoryFdPropertiesKHR{SType: StructureTypeMemoryFdPropertiesKHR}
	r := call4(d.fn.GetMemoryFdPropertiesKHR, uintptr(d.h), uintptr(handle), uintptr(int32(fd)), uintptr(unsafe.Pointer(p)))
	return p.MemoryTypeBits, result(r)
}

// DmabufBufferImportable reports whether the device can make a buffer of
// the usage from a dmabuf (vkGetPhysicalDeviceExternalBufferProperties).
func (i *Instance) DmabufBufferImportable(pd PhysicalDevice, usage BufferUsageFlags) bool {
	if i.fn.GetPhysicalDeviceExternalBufferProperties == 0 {
		return false
	}
	info := &PhysicalDeviceExternalBufferInfo{
		SType:      StructureTypePhysicalDeviceExternalBufferInfo,
		Usage:      usage,
		HandleType: ExternalMemoryHandleTypeDmaBufEXT,
	}
	props := &ExternalBufferProperties{SType: StructureTypeExternalBufferProperties}
	call3(i.fn.GetPhysicalDeviceExternalBufferProperties, uintptr(pd), uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(props)))
	return props.ExternalMemoryProperties.ExternalMemoryFeatures&ExternalMemoryFeatureImportable != 0
}

// DeviceID is what names a physical device to another API or another
// process: the identifiers of VkPhysicalDeviceIDProperties.
type DeviceID struct {
	// DeviceUUID and DriverUUID are the device's and its driver's. Memory
	// shared with an opaque handle needs both to be equal.
	DeviceUUID, DriverUUID [16]byte
	// LUID is the device's locally unique identifier, when HasLUID (on
	// Windows: the adapter's).
	LUID    [8]byte
	HasLUID bool
}

// GetPhysicalDeviceID returns the identifiers of a device
// (vkGetPhysicalDeviceProperties2), and false when the instance does not
// have the command (Vulkan 1.0).
func (i *Instance) GetPhysicalDeviceID(pd PhysicalDevice) (DeviceID, bool) {
	id := &PhysicalDeviceIDProperties{SType: StructureTypePhysicalDeviceIDProperties}
	props := &PhysicalDeviceProperties2{SType: StructureTypePhysicalDeviceProperties2, PNext: unsafe.Pointer(id)}
	if !i.GetPhysicalDeviceProperties2(pd, props) {
		return DeviceID{}, false
	}
	out := DeviceID{DeviceUUID: id.DeviceUUID, DriverUUID: id.DriverUUID, HasLUID: id.DeviceLUIDValid != False}
	if out.HasLUID {
		out.LUID = id.DeviceLUID
	}
	return out, true
}
