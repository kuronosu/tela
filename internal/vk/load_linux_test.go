//go:build telagpu

package vk

import (
	"strings"
	"testing"
)

// A system without Vulkan, or with something else under the loader's
// name, gives an error and no panic.
func TestOpenLibraryMissing(t *testing.T) {
	if _, err := openLibraryNamed([]string{"libtela-no-such-vulkan.so.1", "libtela-no-such-vulkan.so"}); err == nil || !strings.Contains(err.Error(), "opening the Vulkan loader") || !strings.Contains(err.Error(), "libtela-no-such-vulkan.so.1") {
		t.Errorf("a missing library: %v", err)
	}
	// The C library is there on every system the tests run on, and it is
	// not a Vulkan loader.
	if _, err := openLibraryNamed([]string{"libc.so.6"}); err == nil || !strings.Contains(err.Error(), "no vkGetInstanceProcAddr") {
		t.Errorf("a library without the entry point: %v", err)
	}
}
