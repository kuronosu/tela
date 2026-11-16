//go:build telagpu && (linux || windows)

package vk

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ValidationLayerName is the Khronos validation layer.
const ValidationLayerName = "VK_LAYER_KHRONOS_validation"

// DebugMessage is one message of VK_EXT_debug_utils: what a validation
// layer, the loader or a driver has to say.
type DebugMessage struct {
	Severity DebugUtilsMessageSeverityFlagsEXT // one bit
	Types    DebugUtilsMessageTypeFlagsEXT
	IDName   string // such as "VUID-vkCmdDraw-None-02699"; may be empty
	ID       int32
	Text     string
}

// String formats the message on one line, with its severity first.
func (m DebugMessage) String() string {
	sev := "verbose"
	switch {
	case m.Severity&DebugUtilsMessageSeverityErrorEXT != 0:
		sev = "error"
	case m.Severity&DebugUtilsMessageSeverityWarningEXT != 0:
		sev = "warning"
	case m.Severity&DebugUtilsMessageSeverityInfoEXT != 0:
		sev = "info"
	}
	if m.IDName == "" {
		return fmt.Sprintf("vulkan %s: %s", sev, m.Text)
	}
	return fmt.Sprintf("vulkan %s: [%s] %s", sev, m.IDName, m.Text)
}

// DebugCallback is a Go function that receives debug messages. Vulkan
// keeps a pointer to it, so it stays alive from [NewDebugCallback] until
// [DebugCallback.Release], which comes after the messenger and the
// instance that use it are destroyed.
//
// The function is called during the Vulkan command that causes the
// message, on the goroutine that makes it, and so possibly on several at
// once; it must not call Vulkan.
type DebugCallback struct {
	fn func(DebugMessage)
}

var (
	debugOnce sync.Once
	// debugTrampoline is the one C function pointer behind every
	// DebugCallback: purego's callbacks are never freed and limited in
	// number, so the Go function is found through the user data.
	debugTrampoline uintptr

	debugMu   sync.Mutex
	debugLive = map[*DebugCallback]struct{}{} // keeps the callbacks reachable
)

// NewDebugCallback makes fn a receiver of debug messages.
func NewDebugCallback(fn func(DebugMessage)) *DebugCallback {
	debugOnce.Do(func() { debugTrampoline = purego.NewCallback(debugMessengerCallback) })
	c := &DebugCallback{fn: fn}
	debugMu.Lock()
	debugLive[c] = struct{}{}
	debugMu.Unlock()
	return c
}

// Release lets the callback be collected. Vulkan must no longer use it.
func (c *DebugCallback) Release() {
	debugMu.Lock()
	delete(debugLive, c)
	debugMu.Unlock()
}

// Info returns the create info of a messenger that sends the messages of
// these severities and types to the callback: for
// [Instance.CreateDebugUtilsMessengerEXT], and for the PNext chain of an
// [InstanceCreateInfo], where it covers the creation and the destruction
// of the instance itself.
func (c *DebugCallback) Info(severity DebugUtilsMessageSeverityFlagsEXT, types DebugUtilsMessageTypeFlagsEXT) *DebugUtilsMessengerCreateInfoEXT {
	return &DebugUtilsMessengerCreateInfoEXT{
		SType:           StructureTypeDebugUtilsMessengerCreateInfoEXT,
		MessageSeverity: severity,
		MessageType:     types,
		PfnUserCallback: debugTrampoline,
		PUserData:       unsafe.Pointer(c),
	}
}

// debugMessengerCallback is PFN_vkDebugUtilsMessengerCallbackEXT. The two
// flag arguments are 32 bits wide in C and the rest of their registers is
// undefined. It returns VK_FALSE: the command that caused the message goes
// on.
func debugMessengerCallback(severity, types uintptr, data *DebugUtilsMessengerCallbackDataEXT, user *DebugCallback) uintptr {
	if data == nil || user == nil || user.fn == nil {
		return 0
	}
	user.fn(DebugMessage{
		Severity: DebugUtilsMessageSeverityFlagsEXT(uint32(severity)),
		Types:    DebugUtilsMessageTypeFlagsEXT(uint32(types)),
		IDName:   GoString(data.PMessageIdName),
		ID:       data.MessageIdNumber,
		Text:     GoString(data.PMessage),
	})
	return 0
}

// InstanceConfig says what [NewInstance] creates.
type InstanceConfig struct {
	// AppName names the application to the drivers; it may be empty.
	AppName string

	// Validation enables VK_LAYER_KHRONOS_validation, with its
	// synchronization validation (missing barriers and dependencies)
	// when the layer has VK_EXT_layer_settings to ask for it. NewInstance
	// fails when the layer is not installed.
	Validation bool

	// Debug, when not nil, receives the warnings and errors of the
	// validation layers, the loader and the drivers (see [DebugCallback]
	// for how it is called). It needs VK_EXT_debug_utils, which
	// NewInstance enables when the implementation or the validation layer
	// has it; without it Debug is never called, unless Validation is set,
	// which then fails.
	Debug func(DebugMessage)

	// Win32Surface enables VK_KHR_surface and VK_KHR_win32_surface when
	// the loader has both, so that the instance can make surfaces of
	// windows (surface.go, ADR-289); [Instance.HasWin32Surface] tells.
	// Without them the instance is made as without the option.
	Win32Surface bool
}

// NewInstance creates an instance for Vulkan 1.1, the version the
// bindings are written for, as Tela's renderer and its tests want it.
func NewInstance(cfg InstanceConfig) (*Instance, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	var layers, extensions []string
	if cfg.Validation {
		found, err := hasLayer(ValidationLayerName)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("vk: the layer %s is not installed (VK_ADD_LAYER_PATH names extra directories of layer manifests)", ValidationLayerName)
		}
		layers = append(layers, ValidationLayerName)
	}
	var debug *DebugCallback
	var debugInfo *DebugUtilsMessengerCreateInfoEXT
	if cfg.Debug != nil {
		found, err := hasInstanceExtension(ExtDebugUtilsExtensionName, layers)
		if err != nil {
			return nil, err
		}
		switch {
		case found:
			extensions = append(extensions, ExtDebugUtilsExtensionName)
			debug = NewDebugCallback(cfg.Debug)
			debugInfo = debug.Info(
				DebugUtilsMessageSeverityWarningEXT|DebugUtilsMessageSeverityErrorEXT,
				DebugUtilsMessageTypeGeneralEXT|DebugUtilsMessageTypeValidationEXT|DebugUtilsMessageTypePerformanceEXT)
		case cfg.Validation:
			return nil, fmt.Errorf("vk: validation was asked for and its messages cannot be received: no %s", ExtDebugUtilsExtensionName)
		}
	}
	if cfg.Win32Surface {
		names, err := win32SurfaceExtensions()
		if err != nil {
			return nil, err
		}
		extensions = append(extensions, names...)
	}

	info := &InstanceCreateInfo{
		SType: StructureTypeInstanceCreateInfo,
		PApplicationInfo: &ApplicationInfo{
			SType:            StructureTypeApplicationInfo,
			PApplicationName: CString(cfg.AppName),
			PEngineName:      CString("Tela"),
			ApiVersion:       APIVersion1_1,
		},
	}
	if debugInfo != nil {
		// A copy: the messenger below is created from debugInfo, whose
		// PNext must stay nil.
		chained := *debugInfo
		info.PNext = unsafe.Pointer(&chained)
	}
	if cfg.Validation {
		found, err := hasInstanceExtension(ExtLayerSettingsExtensionName, layers)
		if err != nil {
			if debug != nil {
				debug.Release()
			}
			return nil, err
		}
		if found {
			extensions = append(extensions, ExtLayerSettingsExtensionName)
			enabled := True
			info.PNext = unsafe.Pointer(&LayerSettingsCreateInfoEXT{
				SType:        StructureTypeLayerSettingsCreateInfoEXT,
				PNext:        info.PNext,
				SettingCount: 1,
				PSettings: &LayerSettingEXT{
					PLayerName:   CString(ValidationLayerName),
					PSettingName: CString("validate_sync"),
					Type:         LayerSettingTypeBool32EXT,
					ValueCount:   1,
					PValues:      unsafe.Pointer(&enabled),
				},
			})
		}
	}
	info.EnabledLayerCount, info.PpEnabledLayerNames = CStringArray(layers)
	info.EnabledExtensionCount, info.PpEnabledExtensionNames = CStringArray(extensions)
	inst, err := CreateInstance(info)
	if err != nil {
		if debug != nil {
			debug.Release()
		}
		return nil, fmt.Errorf("vk: creating the instance: %w", err)
	}
	inst.debug = debug
	if debugInfo != nil {
		inst.debugHandle, err = inst.CreateDebugUtilsMessengerEXT(debugInfo)
		if err != nil {
			inst.Destroy()
			return nil, fmt.Errorf("vk: creating the debug messenger: %w", err)
		}
	}
	return inst, nil
}

// hasLayer reports whether the loader finds the layer.
func hasLayer(name string) (bool, error) {
	layers, err := EnumerateInstanceLayerProperties()
	if err != nil {
		return false, fmt.Errorf("vk: listing the layers: %w", err)
	}
	for i := range layers {
		if layers[i].Name() == name {
			return true, nil
		}
	}
	return false, nil
}

// hasInstanceExtension reports whether the implementation or one of the
// layers provides the instance extension.
func hasInstanceExtension(name string, layers []string) (bool, error) {
	for _, layer := range append([]string{""}, layers...) {
		exts, err := EnumerateInstanceExtensionProperties(layer)
		if err != nil {
			return false, fmt.Errorf("vk: listing the instance extensions: %w", err)
		}
		for i := range exts {
			if exts[i].Name() == name {
				return true, nil
			}
		}
	}
	return false, nil
}
