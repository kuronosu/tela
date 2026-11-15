//go:build telagpu && (linux || windows)

package vk

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unsafe"
)

// DeviceEnv is the environment variable that chooses the physical device:
// an index into the list of [Instance.Devices], or a part of a device's
// name in any case (ADR-281).
const DeviceEnv = "TELA_GPU_DEVICE"

// DeviceInfo describes a physical device Tela can render with.
type DeviceInfo struct {
	Device PhysicalDevice

	Name          string
	Type          PhysicalDeviceType
	VendorID      uint32 // the PCI vendor: 0x10de NVIDIA, 0x8086 Intel, 0x1002 AMD
	DeviceID      uint32
	APIVersion    uint32 // packed, see [VersionString]
	DriverVersion uint32 // in the vendor's own encoding

	// GraphicsQueueFamily is the first queue family with [QueueGraphics]
	// (such a family also transfers).
	GraphicsQueueFamily uint32

	// Extensions are the names of the device's extensions.
	Extensions []string

	// DRM are the device's DRM nodes, on Linux, when it has
	// VK_EXT_physical_device_drm; the zero value otherwise.
	DRM DRMNodes
}

// DRMNodes are the device numbers of a physical device's DRM nodes
// (VkPhysicalDeviceDrmPropertiesEXT): the primary node is /dev/dri/cardN
// and the render node /dev/dri/renderDN. A compositor names its device by
// such a number, which is how a renderer finds the same one.
type DRMNodes struct {
	HasPrimary, HasRender      bool
	PrimaryMajor, PrimaryMinor int64
	RenderMajor, RenderMinor   int64
}

// HasExtension reports whether the device has the extension.
func (d *DeviceInfo) HasExtension(name string) bool {
	for _, e := range d.Extensions {
		if e == name {
			return true
		}
	}
	return false
}

// String describes the device on one line.
func (d DeviceInfo) String() string {
	s := fmt.Sprintf("%s (%s, %04x:%04x, Vulkan %s", d.Name, d.Type, d.VendorID, d.DeviceID, VersionString(d.APIVersion))
	if d.DRM.HasRender {
		s += fmt.Sprintf(", render node %d:%d", d.DRM.RenderMajor, d.DRM.RenderMinor)
	}
	if d.DRM.HasPrimary {
		s += fmt.Sprintf(", primary node %d:%d", d.DRM.PrimaryMajor, d.DRM.PrimaryMinor)
	}
	return s + ")"
}

// String names the type as Vulkan's tools do.
func (t PhysicalDeviceType) String() string {
	switch t {
	case PhysicalDeviceTypeIntegratedGPU:
		return "integrated"
	case PhysicalDeviceTypeDiscreteGPU:
		return "discrete"
	case PhysicalDeviceTypeVirtualGPU:
		return "virtual"
	case PhysicalDeviceTypeCPU:
		return "cpu"
	}
	return "other"
}

// Devices lists the physical devices Tela can use, in the loader's order:
// those that support Vulkan 1.1 and have a graphics queue family. The
// list may be empty.
func (i *Instance) Devices() ([]DeviceInfo, error) {
	pds, err := i.EnumeratePhysicalDevices()
	if err != nil {
		return nil, fmt.Errorf("vk: listing the physical devices: %w", err)
	}
	var out []DeviceInfo
	for _, pd := range pds {
		props := i.GetPhysicalDeviceProperties(pd)
		if props.ApiVersion < APIVersion1_1 {
			continue
		}
		family, ok := graphicsQueueFamily(i.GetPhysicalDeviceQueueFamilyProperties(pd))
		if !ok {
			continue
		}
		info := DeviceInfo{
			Device:              pd,
			Name:                props.Name(),
			Type:                props.DeviceType,
			VendorID:            props.VendorID,
			DeviceID:            props.DeviceID,
			APIVersion:          props.ApiVersion,
			DriverVersion:       props.DriverVersion,
			GraphicsQueueFamily: family,
		}
		exts, err := i.EnumerateDeviceExtensionProperties(pd)
		if err != nil {
			return nil, fmt.Errorf("vk: listing the extensions of %s: %w", info.Name, err)
		}
		for e := range exts {
			info.Extensions = append(info.Extensions, exts[e].Name())
		}
		if info.HasExtension(ExtPhysicalDeviceDrmExtensionName) {
			drm := &PhysicalDeviceDrmPropertiesEXT{SType: StructureTypePhysicalDeviceDrmPropertiesEXT}
			props2 := &PhysicalDeviceProperties2{SType: StructureTypePhysicalDeviceProperties2, PNext: unsafe.Pointer(drm)}
			if i.GetPhysicalDeviceProperties2(pd, props2) {
				info.DRM = DRMNodes{
					HasPrimary:   drm.HasPrimary != False,
					HasRender:    drm.HasRender != False,
					PrimaryMajor: drm.PrimaryMajor,
					PrimaryMinor: drm.PrimaryMinor,
					RenderMajor:  drm.RenderMajor,
					RenderMinor:  drm.RenderMinor,
				}
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// graphicsQueueFamily returns the index of the first family that draws.
func graphicsQueueFamily(families []QueueFamilyProperties) (uint32, bool) {
	for i := range families {
		if families[i].QueueFlags&QueueGraphics != 0 && families[i].QueueCount > 0 {
			return uint32(i), true
		}
	}
	return 0, false
}

// SelectDevice returns the index of the device to use among devices. A
// non-empty override (the value of [DeviceEnv]) wins: an index into the
// list when it is a number, else a part of a device's name, compared
// without case, where the first match is taken; an override that names no
// device is an error that lists them. Otherwise it is the first discrete device, else the first
// one.
func SelectDevice(devices []DeviceInfo, override string) (int, error) {
	if len(devices) == 0 {
		return 0, fmt.Errorf("vk: no usable Vulkan device (Vulkan 1.1 with a graphics queue)")
	}
	if override = strings.TrimSpace(override); override != "" {
		// A number is always an index, also when it is out of range and
		// some name has it ("2" in "GT2"): a count that silently turns
		// into a name would pick the wrong device.
		if n, err := strconv.Atoi(override); err == nil {
			if n >= 0 && n < len(devices) {
				return n, nil
			}
		} else {
			want := strings.ToLower(override)
			for i := range devices {
				if strings.Contains(strings.ToLower(devices[i].Name), want) {
					return i, nil
				}
			}
		}
		var list strings.Builder
		for i := range devices {
			fmt.Fprintf(&list, "\n  %d: %s", i, devices[i])
		}
		return 0, fmt.Errorf("vk: %s=%q names none of the Vulkan devices:%s", DeviceEnv, override, list.String())
	}
	for i := range devices {
		if devices[i].Type == PhysicalDeviceTypeDiscreteGPU {
			return i, nil
		}
	}
	return 0, nil
}

// PickDevice lists the usable devices and selects one as [SelectDevice]
// does, with the override of the environment variable [DeviceEnv].
func (i *Instance) PickDevice() (DeviceInfo, error) {
	devices, err := i.Devices()
	if err != nil {
		return DeviceInfo{}, err
	}
	n, err := SelectDevice(devices, os.Getenv(DeviceEnv))
	if err != nil {
		return DeviceInfo{}, err
	}
	return devices[n], nil
}
