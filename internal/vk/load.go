//go:build telagpu && (linux || windows)

package vk

import (
	"fmt"
	"sync"
	"unsafe"
)

var (
	loadOnce sync.Once
	loadErr  error

	// getInstanceProcAddr is vkGetInstanceProcAddr, the one symbol taken
	// from the library itself; global holds the commands it returns for a
	// NULL instance. Both are zero until Load succeeds.
	getInstanceProcAddr uintptr
	global              globalFuncs
	loaderVersion       uint32
)

// Load opens the Vulkan loader and binds the commands that need no
// instance. It runs once; later calls return the first result. An error
// means that this system has no usable Vulkan: no loader library, a
// library without the entry point, or a loader older than Vulkan 1.1.
// Nothing here panics for a missing library or symbol.
//
// The functions of the package call Load themselves, so calling it first
// is only a way to ask whether Vulkan is there.
func Load() error {
	loadOnce.Do(func() { loadErr = load() })
	return loadErr
}

func load() error {
	addr, err := openLibrary()
	if err != nil {
		return err
	}
	getInstanceProcAddr = addr
	if err := loadFuncs(&global, func(name string) uintptr { return instanceProcAddr(0, name) }); err != nil {
		return err
	}
	// A loader without vkEnumerateInstanceVersion is a Vulkan 1.0 one.
	loaderVersion = APIVersion1_0
	if global.EnumerateInstanceVersion != 0 {
		var v uint32
		r := call1(global.EnumerateInstanceVersion, uintptr(unsafe.Pointer(&v)))
		if err := result(r); err != nil {
			return fmt.Errorf("vk: vkEnumerateInstanceVersion: %w", err)
		}
		loaderVersion = v
	}
	if loaderVersion < APIVersion1_1 {
		return fmt.Errorf("vk: the Vulkan loader is version %s and Tela needs 1.1", VersionString(loaderVersion))
	}
	return nil
}

// instanceProcAddr is vkGetInstanceProcAddr: the address of the command
// name for instance (zero for the global commands), or zero.
func instanceProcAddr(instance InstanceHandle, name string) uintptr {
	return call2(getInstanceProcAddr, uintptr(instance), uintptr(unsafe.Pointer(CString(name))))
}

// InstanceVersion returns the version of the instance-level API the loader
// supports (vkEnumerateInstanceVersion), packed as [MakeVersion] does.
func InstanceVersion() (uint32, error) {
	if err := Load(); err != nil {
		return 0, err
	}
	return loaderVersion, nil
}

// EnumerateInstanceExtensionProperties lists the instance extensions of
// the implementation, or of the layer named layer when it is not empty.
func EnumerateInstanceExtensionProperties(layer string) ([]ExtensionProperties, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	var layerName *byte
	if layer != "" {
		layerName = CString(layer)
	}
	for {
		var n uint32
		r := call3(global.EnumerateInstanceExtensionProperties, uintptr(unsafe.Pointer(layerName)), uintptr(unsafe.Pointer(&n)), 0)
		if err := result(r); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, nil
		}
		out := make([]ExtensionProperties, n)
		r = call3(global.EnumerateInstanceExtensionProperties, uintptr(unsafe.Pointer(layerName)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&out[0])))
		if err := result(r); err == Incomplete {
			continue // the list grew between the two calls
		} else if err != nil {
			return nil, err
		}
		return out[:n], nil
	}
}

// EnumerateInstanceLayerProperties lists the layers the loader finds.
func EnumerateInstanceLayerProperties() ([]LayerProperties, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	for {
		var n uint32
		r := call2(global.EnumerateInstanceLayerProperties, uintptr(unsafe.Pointer(&n)), 0)
		if err := result(r); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, nil
		}
		out := make([]LayerProperties, n)
		r = call2(global.EnumerateInstanceLayerProperties, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&out[0])))
		if err := result(r); err == Incomplete {
			continue
		} else if err != nil {
			return nil, err
		}
		return out[:n], nil
	}
}
