//go:build telagpu && (linux || windows)

package vk

import "strconv"

// resultNames are the C names of the result codes, for messages.
var resultNames = map[Result]string{
	Success:                    "VK_SUCCESS",
	NotReady:                   "VK_NOT_READY",
	Timeout:                    "VK_TIMEOUT",
	EventSet:                   "VK_EVENT_SET",
	EventReset:                 "VK_EVENT_RESET",
	Incomplete:                 "VK_INCOMPLETE",
	ErrorOutOfHostMemory:       "VK_ERROR_OUT_OF_HOST_MEMORY",
	ErrorOutOfDeviceMemory:     "VK_ERROR_OUT_OF_DEVICE_MEMORY",
	ErrorInitializationFailed:  "VK_ERROR_INITIALIZATION_FAILED",
	ErrorDeviceLost:            "VK_ERROR_DEVICE_LOST",
	ErrorMemoryMapFailed:       "VK_ERROR_MEMORY_MAP_FAILED",
	ErrorLayerNotPresent:       "VK_ERROR_LAYER_NOT_PRESENT",
	ErrorExtensionNotPresent:   "VK_ERROR_EXTENSION_NOT_PRESENT",
	ErrorFeatureNotPresent:     "VK_ERROR_FEATURE_NOT_PRESENT",
	ErrorIncompatibleDriver:    "VK_ERROR_INCOMPATIBLE_DRIVER",
	ErrorTooManyObjects:        "VK_ERROR_TOO_MANY_OBJECTS",
	ErrorFormatNotSupported:    "VK_ERROR_FORMAT_NOT_SUPPORTED",
	ErrorFragmentedPool:        "VK_ERROR_FRAGMENTED_POOL",
	ErrorUnknown:               "VK_ERROR_UNKNOWN",
	ErrorOutOfPoolMemory:       "VK_ERROR_OUT_OF_POOL_MEMORY",
	ErrorInvalidExternalHandle: "VK_ERROR_INVALID_EXTERNAL_HANDLE",
	ErrorFragmentation:         "VK_ERROR_FRAGMENTATION",
	ErrorSurfaceLostKHR:        "VK_ERROR_SURFACE_LOST_KHR",
	ErrorNativeWindowInUseKHR:  "VK_ERROR_NATIVE_WINDOW_IN_USE_KHR",
	SuboptimalKHR:              "VK_SUBOPTIMAL_KHR",
	ErrorOutOfDateKHR:          "VK_ERROR_OUT_OF_DATE_KHR",
	ErrorValidationFailedEXT:   "VK_ERROR_VALIDATION_FAILED_EXT",

	ErrorInvalidDrmFormatModifierPlaneLayoutEXT: "VK_ERROR_INVALID_DRM_FORMAT_MODIFIER_PLANE_LAYOUT_EXT",
}

// String returns the C name of the code, or its number when the bindings
// do not know it.
func (r Result) String() string {
	if s, ok := resultNames[r]; ok {
		return s
	}
	return "VkResult(" + strconv.Itoa(int(r)) + ")"
}

// Error makes a Result an error. The wrappers never return [Success] as
// one.
func (r Result) Error() string { return "vulkan: " + r.String() }

// result turns the integer result register of a call into the error of a
// wrapper. A VkResult is 32 bits wide and the upper half of the register
// is undefined.
func result(r1 uintptr) error {
	if r := Result(int32(r1)); r != Success {
		return r
	}
	return nil
}
