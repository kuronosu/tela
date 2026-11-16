//go:build telagpu && (linux || windows)

package gputest

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/vk"
)

// Validating reports whether the tests run under the validation layer
// (TELA_VK_VALIDATE=1).
func Validating() bool { return os.Getenv("TELA_VK_VALIDATE") == "1" }

// messages collects what the validation layer and the drivers report.
type messages struct {
	mu   sync.Mutex
	list []vk.DebugMessage
}

func (m *messages) add(msg vk.DebugMessage) {
	m.mu.Lock()
	m.list = append(m.list, msg)
	m.mu.Unlock()
}

// report fails the test for every message when validating, and logs them
// otherwise.
func (m *messages) report(t testing.TB) {
	t.Helper()
	m.mu.Lock()
	list := m.list
	m.list = nil
	m.mu.Unlock()
	for _, msg := range list {
		if Validating() {
			t.Errorf("%s", msg)
		} else {
			t.Logf("%s", msg)
		}
	}
}

// Need skips the test without Vulkan or without a usable device, as
// EachDevice does: for a test that opens its device another way.
func Need(t testing.TB) {
	t.Helper()
	if err := vk.Load(); err != nil {
		t.Skipf("Vulkan is not available: %v", err)
	}
	inst, err := vk.NewInstance(vk.InstanceConfig{AppName: "tela gpu tests"})
	if err != nil {
		t.Skipf("no Vulkan instance: %v", err)
	}
	defer inst.Destroy()
	if devices, err := inst.Devices(); err != nil || len(devices) == 0 {
		t.Skipf("Vulkan has no usable physical device (%v)", err)
	}
}

// EachDevice runs f as a subtest for every physical device, named by
// Label, with a device that is closed when f returns. What Vulkan reports
// until then, the objects left alive at the close included, fails the
// subtest under TELA_VK_VALIDATE=1 (and is logged otherwise). It skips the
// test without Vulkan or without a usable device.
func EachDevice(t *testing.T, f func(t *testing.T, dev gpu.Device)) {
	t.Helper()
	if err := vk.Load(); err != nil {
		t.Skipf("Vulkan is not available: %v", err)
	}
	msgs := &messages{}
	inst, err := vk.NewInstance(vk.InstanceConfig{AppName: "tela gpu tests", Validation: Validating(), Debug: msgs.add})
	if err != nil {
		if Validating() {
			t.Fatalf("TELA_VK_VALIDATE=1: %v", err)
		}
		t.Skipf("no Vulkan instance: %v", err)
	}
	defer func() {
		inst.Destroy()
		msgs.report(t)
	}()
	devices, err := inst.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) == 0 {
		t.Skip("Vulkan has no usable physical device (Vulkan 1.1 with a graphics queue)")
	}
	for _, info := range devices {
		t.Run(Label(info.Name), func(t *testing.T) {
			dev, err := gpu.OpenVulkan(inst, info)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				dev.Close()
				msgs.report(t)
			}()
			f(t, dev)
		})
	}
}

// Label shortens a device name to a word for test and file names.
func Label(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "_"):
			b.WriteByte('_')
		}
	}
	return strings.TrimSuffix(b.String(), "_")
}
