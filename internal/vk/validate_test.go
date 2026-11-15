//go:build telagpu && (linux || windows)

package vk

import (
	"strings"
	"testing"
)

// TestValidationReports checks that the tests would see a validation
// error: under TELA_VK_VALIDATE=1 it makes one on purpose and expects the
// layer's message to arrive through the debug callback. Without it, a
// clean run under validation would prove nothing.
//
// The error is a create info with the wrong sType: the layer reports it
// and the drivers, which do not read the field, create the object as
// usual.
func TestValidationReports(t *testing.T) {
	if !validating() {
		t.Skip("needs TELA_VK_VALIDATE=1 and the Khronos validation layer")
	}
	inst, msgs := newTestInstance(t)
	info := testDevices(t, inst)[0]
	dev := newTestDevice(t, inst, info)
	msgs.report(t) // nothing so far

	fence, err := dev.CreateFence(&FenceCreateInfo{SType: StructureTypeSemaphoreCreateInfo})
	check(t, "CreateFence", err)
	dev.DestroyFence(fence)

	msgs.mu.Lock()
	got := msgs.list
	msgs.list = nil
	msgs.mu.Unlock()
	if len(got) == 0 {
		t.Fatal("the validation layer reported nothing for a create info with the wrong sType")
	}
	for _, m := range got {
		t.Logf("received: %s", m)
		if m.Severity != DebugUtilsMessageSeverityErrorEXT || m.Types&DebugUtilsMessageTypeValidationEXT == 0 || !strings.Contains(m.IDName, "VkFenceCreateInfo-sType") || !strings.Contains(m.Text, "sType") {
			t.Errorf("unexpected message: %+v", m)
		}
	}
}
