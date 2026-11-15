//go:build telagpu

package vk

import (
	"fmt"

	"github.com/ebitengine/purego"
)

// libraryNames are the names the Vulkan loader is installed under: the
// versioned one of every distribution, then the development symlink.
var libraryNames = []string{"libvulkan.so.1", "libvulkan.so"}

// openLibrary opens the Vulkan loader and returns the address of
// vkGetInstanceProcAddr.
func openLibrary() (uintptr, error) { return openLibraryNamed(libraryNames) }

// openLibraryNamed opens the first of the libraries that exists and
// returns the address of its vkGetInstanceProcAddr.
func openLibraryNamed(names []string) (uintptr, error) {
	var lib uintptr
	var firstErr error
	for _, name := range names {
		h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err == nil {
			lib = h
			break
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if lib == 0 {
		return 0, fmt.Errorf("vk: opening the Vulkan loader: %w", firstErr)
	}
	addr, err := purego.Dlsym(lib, "vkGetInstanceProcAddr")
	if err != nil {
		return 0, fmt.Errorf("vk: the Vulkan loader has no vkGetInstanceProcAddr: %w", err)
	}
	return addr, nil
}
