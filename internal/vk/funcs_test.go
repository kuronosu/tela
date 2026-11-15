//go:build telagpu && (linux || windows)

package vk

import (
	"strings"
	"testing"
)

func TestLoadFuncs(t *testing.T) {
	type table struct {
		CreateThing  uintptr
		DestroyThing uintptr
		ThingEXT     uintptr `vk:"optional"`
	}
	addrs := map[string]uintptr{"vkCreateThing": 0x10, "vkDestroyThing": 0x20, "vkThingEXT": 0x30}
	lookup := func(name string) uintptr { return addrs[name] }

	var all table
	if err := loadFuncs(&all, lookup); err != nil {
		t.Fatal(err)
	}
	if all != (table{0x10, 0x20, 0x30}) {
		t.Errorf("loaded %+v", all)
	}

	delete(addrs, "vkThingEXT")
	var withoutOptional table
	if err := loadFuncs(&withoutOptional, lookup); err != nil {
		t.Fatalf("a missing optional command: %v", err)
	}
	if withoutOptional != (table{0x10, 0x20, 0}) {
		t.Errorf("loaded %+v", withoutOptional)
	}

	delete(addrs, "vkDestroyThing")
	var missing table
	if err := loadFuncs(&missing, lookup); err == nil || !strings.Contains(err.Error(), "vkDestroyThing") {
		t.Errorf("a missing command: %v", err)
	}
}

// Every command in the tables is spelled as Vulkan spells it: the loader
// and the first device have all the required ones (they loaded), and the
// commands of the extensions the test instance enables are there too.
func TestCommandNames(t *testing.T) {
	inst, _ := newTestInstance(t)
	if global.EnumerateInstanceVersion == 0 || inst.fn.GetPhysicalDeviceProperties2 == 0 {
		t.Error("a Vulkan 1.1 loader lacks a Vulkan 1.1 command: its name is misspelled")
	}
	if !inst.hasDebugUtils {
		t.Skip("no VK_EXT_debug_utils: its commands cannot be checked")
	}
	if inst.fn.CreateDebugUtilsMessengerEXT == 0 || inst.fn.DestroyDebugUtilsMessengerEXT == 0 {
		t.Error("the instance lacks a command of VK_EXT_debug_utils: its name is misspelled")
	}
	info := testDevices(t, inst)[0]
	dev := newTestDevice(t, inst, info)
	if dev.fn.SetDebugUtilsObjectNameEXT == 0 {
		t.Error("the device lacks vkSetDebugUtilsObjectNameEXT: its name is misspelled")
	}
}
