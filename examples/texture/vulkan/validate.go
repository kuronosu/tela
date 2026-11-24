//go:build linux && (amd64 || arm64)

package vulkan

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// messages is who hears of the debug messages of the process: one callback
// serves all, as purego never frees one.
var messages struct {
	mu   sync.Mutex
	once sync.Once
	addr uintptr
	f    func(string)
}

// OpenValidated is [Open] under the Khronos validation layer, which must be
// installed (or in VK_LAYER_PATH): report gets its warnings and errors until
// the instance is closed, those of objects left alive included. It is for
// tests, which use one such instance at a time.
func OpenValidated(report func(message string)) (*Instance, error) {
	messages.once.Do(func() { messages.addr = purego.NewCallback(debugCallback) })
	messages.mu.Lock()
	messages.f = report
	messages.mu.Unlock()
	debug := &debugUtilsMessengerCreateInfoEXT{
		SType:           structureTypeDebugUtilsMessengerCreateInfoEXT,
		MessageSeverity: debugUtilsMessageSeverityWarningBitEXT | debugUtilsMessageSeverityErrorBitEXT,
		MessageType:     debugUtilsMessageTypeGeneralBitEXT | debugUtilsMessageTypeValidationBitEXT,
		PfnUserCallback: messages.addr,
	}
	return open(func(info *instanceCreateInfo) {
		info.PNext = unsafe.Pointer(debug) // for the instance's own making and end
		info.EnabledLayerCount, info.PpEnabledLayerNames = cStrings([]string{"VK_LAYER_KHRONOS_validation"})
		info.EnabledExtensionCount, info.PpEnabledExtensionNames = cStrings([]string{"VK_EXT_debug_utils"})
	}, debug)
}

// debugCallback is PFN_vkDebugUtilsMessengerCallbackEXT. It returns false: the
// command that caused the message goes on.
func debugCallback(severity, types uintptr, data *debugUtilsMessengerCallbackDataEXT, user uintptr) uintptr {
	messages.mu.Lock()
	f := messages.f
	messages.mu.Unlock()
	if f != nil && data != nil {
		f(cGoString(data.PMessageIdName) + ": " + cGoString(data.PMessage))
	}
	return 0
}

// cGoString returns the string that C has at p.
func cGoString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}
