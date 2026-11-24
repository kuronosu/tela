//go:build linux && (amd64 || arm64)

package vulkan

import (
	"os"
	"strings"
	"testing"
	"unsafe"
)

// TestValidationReports checks that a test under TELA_VK_VALIDATE=1 would
// hear of an error, and of an object left alive: it makes both. Without it a
// clean run proves nothing.
func TestValidationReports(t *testing.T) {
	if os.Getenv("TELA_VK_VALIDATE") != "1" {
		t.Skip("needs TELA_VK_VALIDATE=1 and the Khronos validation layer")
	}
	var said []string
	inst, err := OpenValidated(func(m string) { said = append(said, m) })
	if err != nil {
		t.Fatal(err)
	}
	devices, err := inst.Devices()
	if err != nil || len(devices) == 0 {
		t.Skipf("no GPU: %v", err)
	}
	var dev uintptr
	priority := float32(1)
	info := &deviceCreateInfo{
		SType:                structureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos:    &deviceQueueCreateInfo{SType: structureTypeDeviceQueueCreateInfo, QueueCount: 1, PQueuePriorities: &priority},
	}
	if err := do("vkCreateDevice", inst.fn.CreateDevice, devices[0].h, uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&dev))); err != nil {
		t.Fatal(err)
	}
	proc := func(name string) uintptr {
		return call(inst.fn.GetDeviceProcAddr, dev, uintptr(unsafe.Pointer(cString(name))))
	}
	fence := &fenceCreateInfo{SType: structureTypeSemaphoreCreateInfo} // wrong, and never destroyed
	var h uint64
	if err := do("vkCreateFence", proc("vkCreateFence"), dev, uintptr(unsafe.Pointer(fence)), 0, uintptr(unsafe.Pointer(&h))); err != nil {
		t.Fatal(err)
	}
	call(proc("vkDestroyDevice"), dev, 0)
	inst.Close()
	for _, want := range []string{"VkFenceCreateInfo-sType", "leaked objects"} {
		if !strings.Contains(strings.Join(said, "\n"), want) {
			t.Errorf("no message with %q in %q", want, said)
		}
	}
}
