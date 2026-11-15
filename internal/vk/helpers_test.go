//go:build telagpu && (linux || windows)

package vk

import (
	"os"
	"sync"
	"testing"
)

// messages collects what the validation layers, the loader and the
// drivers report during a test.
type messages struct {
	mu   sync.Mutex
	list []DebugMessage
}

func (m *messages) add(msg DebugMessage) {
	m.mu.Lock()
	m.list = append(m.list, msg)
	m.mu.Unlock()
}

// report fails t for every message received since the last report, a
// warning too, when the tests run under validation; otherwise it only
// logs them, since a loader or a driver may warn about the system (a
// broken driver manifest) and that is not a failure of the bindings.
func (m *messages) report(t testing.TB) {
	t.Helper()
	m.mu.Lock()
	list := m.list
	m.list = nil
	m.mu.Unlock()
	for _, msg := range list {
		if validating() {
			t.Errorf("%s", msg)
		} else {
			t.Logf("%s", msg)
		}
	}
}

// validating reports whether the tests run under the validation layer.
func validating() bool { return os.Getenv("TELA_VK_VALIDATE") == "1" }

// newTestInstance returns an instance for a test, destroyed with it, and
// the messages it receives. It skips the test when this system has no
// Vulkan. With TELA_VK_VALIDATE=1 the instance has the Khronos validation
// layer, and its absence is a failure, not a skip: it was asked for.
// Messages still unreported when the test ends, the ones of destroying
// the instance among them, are reported then.
func newTestInstance(t testing.TB) (*Instance, *messages) {
	t.Helper()
	if err := Load(); err != nil {
		t.Skipf("Vulkan is not available: %v", err)
	}
	msgs := &messages{}
	inst, err := NewInstance(InstanceConfig{AppName: "tela vk tests", Validation: validating(), Debug: msgs.add})
	if err != nil {
		if validating() {
			t.Fatalf("TELA_VK_VALIDATE=1: %v", err)
		}
		t.Skipf("no Vulkan instance: %v", err)
	}
	t.Cleanup(func() {
		inst.Destroy()
		msgs.report(t)
	})
	return inst, msgs
}

// testDevices returns the usable physical devices and skips the test when
// there is none.
func testDevices(t testing.TB, inst *Instance) []DeviceInfo {
	t.Helper()
	devices, err := inst.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) == 0 {
		t.Skip("Vulkan has no usable physical device (Vulkan 1.1 with a graphics queue)")
	}
	return devices
}

// newTestDevice creates a device on a physical device, with one queue of
// its graphics family and no features or extensions, destroyed with the
// test: after the cleanups registered later, so after its objects.
func newTestDevice(t testing.TB, inst *Instance, info DeviceInfo) *Device {
	t.Helper()
	priority := float32(1)
	dev, err := inst.CreateDevice(info.Device, &DeviceCreateInfo{
		SType:                StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos: &DeviceQueueCreateInfo{
			SType:            StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: info.GraphicsQueueFamily,
			QueueCount:       1,
			PQueuePriorities: &priority,
		},
		PEnabledFeatures: &PhysicalDeviceFeatures{},
	})
	check(t, "CreateDevice", err)
	t.Cleanup(dev.Destroy)
	if dev.Handle() == 0 {
		t.Fatal("CreateDevice returned no handle")
	}
	return dev
}

// check stops the test on an error of a Vulkan command.
func check(t testing.TB, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}
