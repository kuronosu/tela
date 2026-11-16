//go:build telagpu && (linux || windows)

package vk

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestSelectDevice(t *testing.T) {
	intel := DeviceInfo{Name: "Intel(R) UHD Graphics 630 (CFL GT2)", Type: PhysicalDeviceTypeIntegratedGPU}
	nvidia := DeviceInfo{Name: "NVIDIA GeForce RTX 3060 Ti", Type: PhysicalDeviceTypeDiscreteGPU}
	amd := DeviceInfo{Name: "AMD Radeon RX 7600 (RADV NAVI33)", Type: PhysicalDeviceTypeDiscreteGPU}
	soft := DeviceInfo{Name: "llvmpipe (LLVM 21.1.4, 256 bits)", Type: PhysicalDeviceTypeCPU}

	tests := []struct {
		name     string
		devices  []DeviceInfo
		override string
		want     int
		wantErr  string
	}{
		{name: "no devices", wantErr: "no usable Vulkan device"},
		{name: "no devices with an override", override: "0", wantErr: "no usable Vulkan device"},
		{name: "single", devices: []DeviceInfo{intel}, want: 0},
		{name: "first discrete", devices: []DeviceInfo{intel, nvidia, amd}, want: 1},
		{name: "discrete already first", devices: []DeviceInfo{nvidia, intel}, want: 0},
		{name: "no discrete takes the first integrated", devices: []DeviceInfo{soft, intel}, want: 1},
		{name: "neither takes the first", devices: []DeviceInfo{soft, {Name: "virtio", Type: PhysicalDeviceTypeVirtualGPU}}, want: 0},
		{name: "index", devices: []DeviceInfo{intel, nvidia}, override: "0", want: 0},
		{name: "index with spaces", devices: []DeviceInfo{nvidia, intel}, override: " 1 ", want: 1},
		{name: "index out of range", devices: []DeviceInfo{intel, nvidia}, override: "2", wantErr: `TELA_GPU_DEVICE="2" names none`},
		{name: "negative index", devices: []DeviceInfo{intel, nvidia}, override: "-1", wantErr: "names none"},
		{name: "name part", devices: []DeviceInfo{nvidia, intel}, override: "uhd", want: 1},
		{name: "name part in upper case", devices: []DeviceInfo{nvidia, intel}, override: "INTEL", want: 1},
		{name: "first match of a name part", devices: []DeviceInfo{intel, nvidia, amd}, override: "r", want: 0},
		{name: "a number is never a name", devices: []DeviceInfo{intel, nvidia}, override: "3060", wantErr: "names none"},
		{name: "name part with digits", devices: []DeviceInfo{intel, nvidia}, override: "rtx 3060", want: 1},
		{name: "unknown name", devices: []DeviceInfo{intel, nvidia}, override: "radeon", wantErr: "1: NVIDIA GeForce RTX 3060 Ti"},
		{name: "blank override", devices: []DeviceInfo{intel, nvidia}, override: "  ", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectDevice(tt.devices, tt.override)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %v, want one with %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("selected %d, want %d", got, tt.want)
			}
		})
	}
}

// TestDevices lists the real devices and checks what can be checked
// without knowing the machine: the fields are filled, the DRM nodes exist
// and the selection picks one of them.
func TestDevices(t *testing.T) {
	inst, _ := newTestInstance(t)
	version, err := InstanceVersion()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Vulkan loader %s", VersionString(version))
	devices := testDevices(t, inst)
	for i, d := range devices {
		t.Logf("%d: %s, graphics queue family %d, %d extensions", i, d, d.GraphicsQueueFamily, len(d.Extensions))
		if d.Device == 0 || d.Name == "" || d.VendorID == 0 || d.APIVersion < APIVersion1_1 || len(d.Extensions) == 0 {
			t.Errorf("device %d is not filled in: %+v", i, d)
		}
		families := inst.GetPhysicalDeviceQueueFamilyProperties(d.Device)
		if int(d.GraphicsQueueFamily) >= len(families) || families[d.GraphicsQueueFamily].QueueFlags&QueueGraphics == 0 {
			t.Errorf("%s: queue family %d of %d does not draw", d.Name, d.GraphicsQueueFamily, len(families))
		}
		mem := inst.GetPhysicalDeviceMemoryProperties(d.Device)
		if mem.MemoryTypeCount == 0 || mem.MemoryTypeCount > MaxMemoryTypes || mem.MemoryHeapCount == 0 || mem.MemoryHeapCount > MaxMemoryHeaps {
			t.Errorf("%s: %d memory types in %d heaps", d.Name, mem.MemoryTypeCount, mem.MemoryHeapCount)
		}
		if _, ok := mem.FindMemoryType(^uint32(0), MemoryPropertyHostVisible|MemoryPropertyHostCoherent); !ok {
			t.Errorf("%s: no host-visible, coherent memory type, which Vulkan requires", d.Name)
		}
		limits := &inst.GetPhysicalDeviceProperties(d.Device).Limits
		// The minimums Vulkan requires of every implementation, from
		// both ends of the struct.
		if limits.MaxImageDimension2D < 4096 || limits.MaxPushConstantsSize < 128 || limits.MaxBoundDescriptorSets < 4 || limits.NonCoherentAtomSize == 0 || limits.NonCoherentAtomSize > 256 {
			t.Errorf("%s: limits read wrong: %+v", d.Name, limits)
		}
		if d.HasExtension(ExtPhysicalDeviceDrmExtensionName) != (d.DRM.HasPrimary || d.DRM.HasRender) {
			t.Errorf("%s: the DRM extension and the DRM nodes disagree: %+v", d.Name, d.DRM)
		}
		if runtime.GOOS == "linux" {
			// A character device with these numbers exists.
			for _, node := range []struct {
				has          bool
				major, minor int64
			}{{d.DRM.HasPrimary, d.DRM.PrimaryMajor, d.DRM.PrimaryMinor}, {d.DRM.HasRender, d.DRM.RenderMajor, d.DRM.RenderMinor}} {
				if !node.has {
					continue
				}
				if _, err := os.Stat(fmt.Sprintf("/sys/dev/char/%d:%d", node.major, node.minor)); err != nil {
					t.Errorf("%s: DRM node %d:%d: %v", d.Name, node.major, node.minor, err)
				}
			}
		}
	}

	picked, err := inst.PickDevice()
	if err != nil {
		t.Fatalf("PickDevice with %s=%q: %v", DeviceEnv, os.Getenv(DeviceEnv), err)
	}
	t.Logf("picked: %s", picked.Name)
	if os.Getenv(DeviceEnv) == "" {
		for _, d := range devices {
			if d.Type == PhysicalDeviceTypeDiscreteGPU && picked.Type != PhysicalDeviceTypeDiscreteGPU {
				t.Errorf("picked %s although %s is discrete", picked.Name, d.Name)
			}
		}
	}
}

func TestResult(t *testing.T) {
	if err := result(0); err != nil {
		t.Errorf("VK_SUCCESS is the error %v", err)
	}
	// The upper half of the result register is not part of a VkResult.
	if err := result(uintptr(0xdeadbeef_00000000)); err != nil {
		t.Errorf("VK_SUCCESS under garbage is the error %v", err)
	}
	lost := uint32(0xfffffffc) // -4
	if err := result(uintptr(lost)); err != ErrorDeviceLost {
		t.Errorf("-4 is %v, want %v", err, ErrorDeviceLost)
	}
	if err := result(2); err != Timeout {
		t.Errorf("2 is %v, want %v", err, Timeout)
	}
	if got := ErrorDeviceLost.Error(); got != "vulkan: VK_ERROR_DEVICE_LOST" {
		t.Errorf("message %q", got)
	}
	if got := Result(-77).Error(); got != "vulkan: VkResult(-77)" {
		t.Errorf("message of an unknown code %q", got)
	}
}

func TestStrings(t *testing.T) {
	if got := GoString(CString("héllo")); got != "héllo" {
		t.Errorf("round trip %q", got)
	}
	if got := GoString(CString("")); got != "" {
		t.Errorf("empty round trip %q", got)
	}
	if got := GoString(nil); got != "" {
		t.Errorf("nil is %q", got)
	}
	n, p := CStringArray(nil)
	if n != 0 || p != nil {
		t.Errorf("no names give %d, %v", n, p)
	}
	n, p = CStringArray([]string{"a", "bc"})
	if n != 2 || GoString(*p) != "a" {
		t.Errorf("two names give %d, %q", n, GoString(*p))
	}
	if got := fixedString([]byte{'a', 'b', 0, 'c'}); got != "ab" {
		t.Errorf("fixed string %q", got)
	}
	if got := fixedString([]byte{'a', 'b'}); got != "ab" {
		t.Errorf("fixed string without NUL %q", got)
	}
	if got := VersionString(MakeVersion(1, 4, 329)); got != "1.4.329" {
		t.Errorf("version %q", got)
	}
	words := SPIRV([]byte{0x03, 0x02, 0x23, 0x07, 1, 0, 0, 0})
	if len(words) != 2 || words[0] != 0x07230203 || words[1] != 1 {
		t.Errorf("SPIR-V words %#x", words)
	}
}
