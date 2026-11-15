//go:build telagpu && (linux || windows)

package vk

import "unsafe"

// Instance is a VkInstance with the entry points of its instance-level
// commands. Its methods are those commands; the ones that take a
// [PhysicalDevice] belong here too.
type Instance struct {
	h  InstanceHandle
	fn instanceFuncs

	// debug is the messenger [NewInstance] made, destroyed with the
	// instance.
	debug         *DebugCallback
	debugHandle   DebugUtilsMessengerEXT
	hasDebugUtils bool
}

// CreateInstance is vkCreateInstance. It also binds the commands of the
// instance, so it fails when the implementation lacks one the bindings
// need. Most callers want [NewInstance].
func CreateInstance(info *InstanceCreateInfo) (*Instance, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	var h InstanceHandle
	r := call3(global.CreateInstance, uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	if err := result(r); err != nil {
		return nil, err
	}
	inst := &Instance{h: h}
	if err := loadFuncs(&inst.fn, func(name string) uintptr { return instanceProcAddr(h, name) }); err != nil {
		// vkDestroyInstance is the first field: when it is the one
		// missing there is nothing to destroy the instance with.
		if inst.fn.DestroyInstance != 0 {
			inst.Destroy()
		}
		return nil, err
	}
	// Some loaders return an entry point for the commands of an extension
	// that was not enabled; calling it is an error, so forget them.
	for _, name := range unsafe.Slice(info.PpEnabledExtensionNames, info.EnabledExtensionCount) {
		if GoString(name) == ExtDebugUtilsExtensionName {
			inst.hasDebugUtils = true
		}
	}
	if !inst.hasDebugUtils {
		inst.fn.CreateDebugUtilsMessengerEXT = 0
		inst.fn.DestroyDebugUtilsMessengerEXT = 0
	}
	return inst, nil
}

// Handle returns the VkInstance.
func (i *Instance) Handle() InstanceHandle { return i.h }

// Destroy is vkDestroyInstance, after destroying the debug messenger of
// [NewInstance]. Every device of the instance must be destroyed before.
func (i *Instance) Destroy() {
	if i.debugHandle != 0 {
		i.DestroyDebugUtilsMessengerEXT(i.debugHandle)
		i.debugHandle = 0
	}
	call2(i.fn.DestroyInstance, uintptr(i.h), 0)
	i.h = 0
	if i.debug != nil {
		// Released after the instance: destroying it may still report.
		i.debug.Release()
		i.debug = nil
	}
}

// EnumeratePhysicalDevices is vkEnumeratePhysicalDevices.
func (i *Instance) EnumeratePhysicalDevices() ([]PhysicalDevice, error) {
	for {
		var n uint32
		r := call3(i.fn.EnumeratePhysicalDevices, uintptr(i.h), uintptr(unsafe.Pointer(&n)), 0)
		if err := result(r); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, nil
		}
		out := make([]PhysicalDevice, n)
		r = call3(i.fn.EnumeratePhysicalDevices, uintptr(i.h), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&out[0])))
		if err := result(r); err == Incomplete {
			continue // a device appeared between the two calls
		} else if err != nil {
			return nil, err
		}
		return out[:n], nil
	}
}

// GetPhysicalDeviceProperties is vkGetPhysicalDeviceProperties.
func (i *Instance) GetPhysicalDeviceProperties(pd PhysicalDevice) *PhysicalDeviceProperties {
	p := new(PhysicalDeviceProperties)
	call2(i.fn.GetPhysicalDeviceProperties, uintptr(pd), uintptr(unsafe.Pointer(p)))
	return p
}

// GetPhysicalDeviceProperties2 is vkGetPhysicalDeviceProperties2: it fills
// p and the structs chained to p.PNext, whose STypes the caller has set.
// The device must support Vulkan 1.1 (see ApiVersion in
// [Instance.GetPhysicalDeviceProperties]); it reports false, and leaves p
// alone, when the instance does not have the command.
func (i *Instance) GetPhysicalDeviceProperties2(pd PhysicalDevice, p *PhysicalDeviceProperties2) bool {
	if i.fn.GetPhysicalDeviceProperties2 == 0 {
		return false
	}
	call2(i.fn.GetPhysicalDeviceProperties2, uintptr(pd), uintptr(unsafe.Pointer(p)))
	return true
}

// GetPhysicalDeviceFeatures is vkGetPhysicalDeviceFeatures.
func (i *Instance) GetPhysicalDeviceFeatures(pd PhysicalDevice) *PhysicalDeviceFeatures {
	p := new(PhysicalDeviceFeatures)
	call2(i.fn.GetPhysicalDeviceFeatures, uintptr(pd), uintptr(unsafe.Pointer(p)))
	return p
}

// GetPhysicalDeviceQueueFamilyProperties is
// vkGetPhysicalDeviceQueueFamilyProperties. The index of a family is its
// position in the result.
func (i *Instance) GetPhysicalDeviceQueueFamilyProperties(pd PhysicalDevice) []QueueFamilyProperties {
	var n uint32
	call3(i.fn.GetPhysicalDeviceQueueFamilyProperties, uintptr(pd), uintptr(unsafe.Pointer(&n)), 0)
	if n == 0 {
		return nil
	}
	out := make([]QueueFamilyProperties, n)
	call3(i.fn.GetPhysicalDeviceQueueFamilyProperties, uintptr(pd), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&out[0])))
	return out[:n]
}

// GetPhysicalDeviceMemoryProperties is vkGetPhysicalDeviceMemoryProperties.
func (i *Instance) GetPhysicalDeviceMemoryProperties(pd PhysicalDevice) *PhysicalDeviceMemoryProperties {
	p := new(PhysicalDeviceMemoryProperties)
	call2(i.fn.GetPhysicalDeviceMemoryProperties, uintptr(pd), uintptr(unsafe.Pointer(p)))
	return p
}

// GetPhysicalDeviceFormatProperties is vkGetPhysicalDeviceFormatProperties.
func (i *Instance) GetPhysicalDeviceFormatProperties(pd PhysicalDevice, format Format) FormatProperties {
	p := new(FormatProperties)
	call3(i.fn.GetPhysicalDeviceFormatProperties, uintptr(pd), uintptr(format), uintptr(unsafe.Pointer(p)))
	return *p
}

// GetPhysicalDeviceImageFormatProperties is
// vkGetPhysicalDeviceImageFormatProperties: the limits of images of this
// format, type, tiling, usage and flags, or [ErrorFormatNotSupported].
func (i *Instance) GetPhysicalDeviceImageFormatProperties(pd PhysicalDevice, format Format, typ ImageType, tiling ImageTiling, usage ImageUsageFlags, flags ImageCreateFlags) (ImageFormatProperties, error) {
	p := new(ImageFormatProperties)
	r := call7(i.fn.GetPhysicalDeviceImageFormatProperties, uintptr(pd), uintptr(format), uintptr(typ), uintptr(tiling), uintptr(usage), uintptr(flags), uintptr(unsafe.Pointer(p)))
	return *p, result(r)
}

// EnumerateDeviceExtensionProperties is
// vkEnumerateDeviceExtensionProperties for the implementation's own
// extensions (a NULL layer name).
func (i *Instance) EnumerateDeviceExtensionProperties(pd PhysicalDevice) ([]ExtensionProperties, error) {
	for {
		var n uint32
		r := call4(i.fn.EnumerateDeviceExtensionProperties, uintptr(pd), 0, uintptr(unsafe.Pointer(&n)), 0)
		if err := result(r); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, nil
		}
		out := make([]ExtensionProperties, n)
		r = call4(i.fn.EnumerateDeviceExtensionProperties, uintptr(pd), 0, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&out[0])))
		if err := result(r); err == Incomplete {
			continue
		} else if err != nil {
			return nil, err
		}
		return out[:n], nil
	}
}

// CreateDevice is vkCreateDevice. It also binds the commands of the
// device through vkGetDeviceProcAddr, so they skip the loader's dispatch.
func (i *Instance) CreateDevice(pd PhysicalDevice, info *DeviceCreateInfo) (*Device, error) {
	var h DeviceHandle
	r := call4(i.fn.CreateDevice, uintptr(pd), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	if err := result(r); err != nil {
		return nil, err
	}
	dev := &Device{h: h}
	err := loadFuncs(&dev.fn, func(name string) uintptr {
		return call2(i.fn.GetDeviceProcAddr, uintptr(h), uintptr(unsafe.Pointer(CString(name))))
	})
	if err != nil {
		if dev.fn.DestroyDevice != 0 {
			dev.Destroy()
		}
		return nil, err
	}
	if !i.hasDebugUtils {
		dev.fn.SetDebugUtilsObjectNameEXT = 0 // as in CreateInstance
	}
	dev.forgetDisabled(info) // the dmabuf export's extensions (dmabuf.go)
	return dev, nil
}

// CreateDebugUtilsMessengerEXT is vkCreateDebugUtilsMessengerEXT. It
// returns [ErrorExtensionNotPresent] when the instance was created without
// VK_EXT_debug_utils. See [DebugCallback] for the create info.
func (i *Instance) CreateDebugUtilsMessengerEXT(info *DebugUtilsMessengerCreateInfoEXT) (DebugUtilsMessengerEXT, error) {
	if i.fn.CreateDebugUtilsMessengerEXT == 0 {
		return 0, ErrorExtensionNotPresent
	}
	var h DebugUtilsMessengerEXT
	r := call4(i.fn.CreateDebugUtilsMessengerEXT, uintptr(i.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroyDebugUtilsMessengerEXT is vkDestroyDebugUtilsMessengerEXT.
func (i *Instance) DestroyDebugUtilsMessengerEXT(m DebugUtilsMessengerEXT) {
	if i.fn.DestroyDebugUtilsMessengerEXT == 0 {
		return
	}
	call3(i.fn.DestroyDebugUtilsMessengerEXT, uintptr(i.h), uintptr(m), 0)
}
