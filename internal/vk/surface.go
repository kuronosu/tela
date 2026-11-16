//go:build telagpu && (linux || windows)

package vk

import "unsafe"

// Presenting to a window (ADR-281, ADR-289).
//
// On Windows the renderer presents through a swapchain on the window's
// HWND: VK_KHR_surface and VK_KHR_win32_surface make a surface of the
// window (instance extensions, which [InstanceConfig].Win32Surface
// enables), VK_KHR_swapchain the images the presentation engine shows
// (a device extension), and VK_KHR_incremental_present, where the driver
// has it, tells it the rectangles a present changed. Their wrappers and
// helpers are here, apart from the rest of the bindings, and compile on
// Linux too, where no instance enables them (Linux presents without a
// surface: dmabuf.go).
//
// The handles a surface is made from are the OS's: an HINSTANCE and an
// HWND, which the platform layer hands out (platform.Win32Surface) and
// which are never Go memory.

// win32SurfaceExtensions returns the instance extensions of a surface of
// a window on Windows, when the loader has both, else none.
func win32SurfaceExtensions() ([]string, error) {
	names := []string{KhrSurfaceExtensionName, KhrWin32SurfaceExtensionName}
	for _, name := range names {
		found, err := hasInstanceExtension(name, nil)
		if err != nil || !found {
			return nil, err
		}
	}
	return names, nil
}

// HasWin32Surface reports whether the instance was created with
// VK_KHR_surface and VK_KHR_win32_surface: it can make a surface of a
// window ([Instance.CreateWin32SurfaceKHR]).
func (i *Instance) HasWin32Surface() bool { return i.hasWin32Surface }

// forgetSurfaces clears the entry points of the surface extensions that
// info does not enable, as CreateInstance does for VK_EXT_debug_utils.
func (i *Instance) forgetSurfaces(info *InstanceCreateInfo) {
	enabled := map[string]bool{}
	for _, name := range unsafe.Slice(info.PpEnabledExtensionNames, info.EnabledExtensionCount) {
		enabled[GoString(name)] = true
	}
	if !enabled[KhrSurfaceExtensionName] {
		i.fn.DestroySurfaceKHR = 0
		i.fn.GetPhysicalDeviceSurfaceSupportKHR = 0
		i.fn.GetPhysicalDeviceSurfaceCapabilitiesKHR = 0
		i.fn.GetPhysicalDeviceSurfaceFormatsKHR = 0
		i.fn.GetPhysicalDeviceSurfacePresentModesKHR = 0
	}
	if !enabled[KhrSurfaceExtensionName] || !enabled[KhrWin32SurfaceExtensionName] {
		i.fn.CreateWin32SurfaceKHR = 0
		i.fn.GetPhysicalDeviceWin32PresentationSupportKHR = 0
	}
	i.hasWin32Surface = i.fn.CreateWin32SurfaceKHR != 0 && i.fn.DestroySurfaceKHR != 0 &&
		i.fn.GetPhysicalDeviceSurfaceCapabilitiesKHR != 0 && i.fn.GetPhysicalDeviceSurfaceFormatsKHR != 0
}

// SwapchainExtensions returns the names of the device extensions a
// swapchain needs, to enable at vkCreateDevice, and whether the device has
// them: VK_KHR_swapchain, and VK_KHR_incremental_present when the device
// has it (incremental reports that).
func SwapchainExtensions(info *DeviceInfo) (names []string, incremental, ok bool) {
	if !info.HasExtension(KhrSwapchainExtensionName) {
		return nil, false, false
	}
	names = []string{KhrSwapchainExtensionName}
	if info.HasExtension(KhrIncrementalPresentExtensionName) {
		names = append(names, KhrIncrementalPresentExtensionName)
		incremental = true
	}
	return names, incremental, true
}

// forgetSwapchain clears the entry points of VK_KHR_swapchain when info
// does not enable it (as forgetDisabled does for the dmabuf export).
func (d *Device) forgetSwapchain(info *DeviceCreateInfo) {
	for _, name := range unsafe.Slice(info.PpEnabledExtensionNames, info.EnabledExtensionCount) {
		if GoString(name) == KhrSwapchainExtensionName {
			return
		}
	}
	d.fn.CreateSwapchainKHR = 0
	d.fn.DestroySwapchainKHR = 0
	d.fn.GetSwapchainImagesKHR = 0
	d.fn.AcquireNextImageKHR = 0
	d.fn.QueuePresentKHR = 0
}

// HasSwapchain reports whether the device was created with
// VK_KHR_swapchain.
func (d *Device) HasSwapchain() bool { return d.fn.CreateSwapchainKHR != 0 }

// CreateWin32SurfaceKHR is vkCreateWin32SurfaceKHR. It returns
// [ErrorExtensionNotPresent] when the instance was created without the
// surface extensions.
func (i *Instance) CreateWin32SurfaceKHR(info *Win32SurfaceCreateInfoKHR) (SurfaceKHR, error) {
	if i.fn.CreateWin32SurfaceKHR == 0 {
		return 0, ErrorExtensionNotPresent
	}
	var h SurfaceKHR
	r := call4(i.fn.CreateWin32SurfaceKHR, uintptr(i.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroySurfaceKHR is vkDestroySurfaceKHR. Every swapchain of the surface
// must be destroyed before.
func (i *Instance) DestroySurfaceKHR(s SurfaceKHR) {
	if i.fn.DestroySurfaceKHR == 0 || s == 0 {
		return
	}
	call3(i.fn.DestroySurfaceKHR, uintptr(i.h), uintptr(s), 0)
}

// GetPhysicalDeviceWin32PresentationSupportKHR is
// vkGetPhysicalDeviceWin32PresentationSupportKHR: whether the queue family
// presents to windows on Windows. It reports false when the instance was
// created without the surface extensions.
func (i *Instance) GetPhysicalDeviceWin32PresentationSupportKHR(pd PhysicalDevice, family uint32) bool {
	if i.fn.GetPhysicalDeviceWin32PresentationSupportKHR == 0 {
		return false
	}
	return Bool32(call2(i.fn.GetPhysicalDeviceWin32PresentationSupportKHR, uintptr(pd), uintptr(family))) != False
}

// GetPhysicalDeviceSurfaceSupportKHR is
// vkGetPhysicalDeviceSurfaceSupportKHR: whether the queue family presents
// to the surface.
func (i *Instance) GetPhysicalDeviceSurfaceSupportKHR(pd PhysicalDevice, family uint32, s SurfaceKHR) (bool, error) {
	if i.fn.GetPhysicalDeviceSurfaceSupportKHR == 0 {
		return false, ErrorExtensionNotPresent
	}
	supported := new(Bool32)
	r := call4(i.fn.GetPhysicalDeviceSurfaceSupportKHR, uintptr(pd), uintptr(family), uintptr(s), uintptr(unsafe.Pointer(supported)))
	return *supported != False, result(r)
}

// GetPhysicalDeviceSurfaceCapabilitiesKHR is
// vkGetPhysicalDeviceSurfaceCapabilitiesKHR: the image counts, the extent
// (on Windows the window's client area, zero while it is minimized), the
// transforms, the alpha modes and the usages of a swapchain on the surface.
func (i *Instance) GetPhysicalDeviceSurfaceCapabilitiesKHR(pd PhysicalDevice, s SurfaceKHR) (SurfaceCapabilitiesKHR, error) {
	if i.fn.GetPhysicalDeviceSurfaceCapabilitiesKHR == 0 {
		return SurfaceCapabilitiesKHR{}, ErrorExtensionNotPresent
	}
	p := new(SurfaceCapabilitiesKHR)
	r := call3(i.fn.GetPhysicalDeviceSurfaceCapabilitiesKHR, uintptr(pd), uintptr(s), uintptr(unsafe.Pointer(p)))
	return *p, result(r)
}

// GetPhysicalDeviceSurfaceFormatsKHR is
// vkGetPhysicalDeviceSurfaceFormatsKHR: the formats and color spaces of a
// swapchain on the surface.
func (i *Instance) GetPhysicalDeviceSurfaceFormatsKHR(pd PhysicalDevice, s SurfaceKHR) ([]SurfaceFormatKHR, error) {
	if i.fn.GetPhysicalDeviceSurfaceFormatsKHR == 0 {
		return nil, ErrorExtensionNotPresent
	}
	for {
		var n uint32
		r := call4(i.fn.GetPhysicalDeviceSurfaceFormatsKHR, uintptr(pd), uintptr(s), uintptr(unsafe.Pointer(&n)), 0)
		if err := result(r); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, nil
		}
		out := make([]SurfaceFormatKHR, n)
		r = call4(i.fn.GetPhysicalDeviceSurfaceFormatsKHR, uintptr(pd), uintptr(s), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&out[0])))
		if err := result(r); err == Incomplete {
			continue
		} else if err != nil {
			return nil, err
		}
		return out[:n], nil
	}
}

// GetPhysicalDeviceSurfacePresentModesKHR is
// vkGetPhysicalDeviceSurfacePresentModesKHR. Every surface has
// [PresentModeFifoKHR].
func (i *Instance) GetPhysicalDeviceSurfacePresentModesKHR(pd PhysicalDevice, s SurfaceKHR) ([]PresentModeKHR, error) {
	if i.fn.GetPhysicalDeviceSurfacePresentModesKHR == 0 {
		return nil, ErrorExtensionNotPresent
	}
	for {
		var n uint32
		r := call4(i.fn.GetPhysicalDeviceSurfacePresentModesKHR, uintptr(pd), uintptr(s), uintptr(unsafe.Pointer(&n)), 0)
		if err := result(r); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, nil
		}
		out := make([]PresentModeKHR, n)
		r = call4(i.fn.GetPhysicalDeviceSurfacePresentModesKHR, uintptr(pd), uintptr(s), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&out[0])))
		if err := result(r); err == Incomplete {
			continue
		} else if err != nil {
			return nil, err
		}
		return out[:n], nil
	}
}

// CreateSwapchainKHR is vkCreateSwapchainKHR. It returns
// [ErrorExtensionNotPresent] when the device was created without
// VK_KHR_swapchain.
func (d *Device) CreateSwapchainKHR(info *SwapchainCreateInfoKHR) (SwapchainKHR, error) {
	if d.fn.CreateSwapchainKHR == 0 {
		return 0, ErrorExtensionNotPresent
	}
	var h SwapchainKHR
	r := call4(d.fn.CreateSwapchainKHR, uintptr(d.h), uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&h)))
	return h, result(r)
}

// DestroySwapchainKHR is vkDestroySwapchainKHR. Its images must not be in
// use by the device.
func (d *Device) DestroySwapchainKHR(sc SwapchainKHR) {
	if d.fn.DestroySwapchainKHR == 0 || sc == 0 {
		return
	}
	call3(d.fn.DestroySwapchainKHR, uintptr(d.h), uintptr(sc), 0)
}

// GetSwapchainImagesKHR is vkGetSwapchainImagesKHR: the images of the
// swapchain, which it owns.
func (d *Device) GetSwapchainImagesKHR(sc SwapchainKHR) ([]Image, error) {
	if d.fn.GetSwapchainImagesKHR == 0 {
		return nil, ErrorExtensionNotPresent
	}
	for {
		var n uint32
		r := call4(d.fn.GetSwapchainImagesKHR, uintptr(d.h), uintptr(sc), uintptr(unsafe.Pointer(&n)), 0)
		if err := result(r); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, nil
		}
		out := make([]Image, n)
		r = call4(d.fn.GetSwapchainImagesKHR, uintptr(d.h), uintptr(sc), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&out[0])))
		if err := result(r); err == Incomplete {
			continue
		} else if err != nil {
			return nil, err
		}
		return out[:n], nil
	}
}

// AcquireNextImageKHR is vkAcquireNextImageKHR: the index of an image the
// presentation engine gave back, whose use is to wait for semaphore (and
// fence; either may be zero). With a zero timeout it never waits and
// returns [NotReady] when no image is free. [SuboptimalKHR] comes with an
// index: the image is acquired, and the swapchain should be made again
// after it is presented. [ErrorOutOfDateKHR] comes without one.
func (d *Device) AcquireNextImageKHR(sc SwapchainKHR, timeout uint64, semaphore Semaphore, fence Fence) (uint32, error) {
	if d.fn.AcquireNextImageKHR == 0 {
		return 0, ErrorExtensionNotPresent
	}
	index := new(uint32)
	r := call6(d.fn.AcquireNextImageKHR, uintptr(d.h), uintptr(sc), uintptr(timeout), uintptr(semaphore), uintptr(fence), uintptr(unsafe.Pointer(index)))
	return *index, result(r)
}

// QueuePresentKHR is vkQueuePresentKHR. [SuboptimalKHR] means the image was
// presented and the swapchain should be made again; [ErrorOutOfDateKHR]
// that it must be.
func (d *Device) QueuePresentKHR(q Queue, info *PresentInfoKHR) error {
	if d.fn.QueuePresentKHR == 0 {
		return ErrorExtensionNotPresent
	}
	r := call2(d.fn.QueuePresentKHR, uintptr(q), uintptr(unsafe.Pointer(info)))
	return result(r)
}
