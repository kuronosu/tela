//go:build telagpu

package vk

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// libraryName is the Vulkan loader the graphics drivers install in the
// system directory.
const libraryName = "vulkan-1.dll"

// openLibrary opens the Vulkan loader and returns the address of
// vkGetInstanceProcAddr. The library is looked for in the system directory
// only, so a vulkan-1.dll beside the program or in the working directory
// is never loaded in its place.
func openLibrary() (uintptr, error) {
	lib, err := windows.LoadLibraryEx(libraryName, 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return 0, fmt.Errorf("vk: opening the Vulkan loader %s: %w", libraryName, err)
	}
	addr, err := windows.GetProcAddress(lib, "vkGetInstanceProcAddr")
	if err != nil {
		return 0, fmt.Errorf("vk: %s has no vkGetInstanceProcAddr: %w", libraryName, err)
	}
	return addr, nil
}
