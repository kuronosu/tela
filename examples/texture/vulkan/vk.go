//go:build linux && (amd64 || arm64)

package vulkan

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Calling Vulkan: the loader opened with dlopen, and the commands called by
// their addresses through purego, without cgo. A command that takes more than
// this renderer passes is not here.

// vkError is a VkResult that is not a success.
type vkError int32

func (e vkError) Error() string {
	switch e {
	case errorDeviceLost:
		return "the device was lost (VK_ERROR_DEVICE_LOST)"
	case timeout:
		return "the GPU did not finish in time (VK_TIMEOUT)"
	}
	return fmt.Sprintf("VkResult %d", int32(e))
}

// call calls a command and returns its result. A pointer converted to
// uintptr in its arguments is kept alive, and on the heap, until it returns;
// so is what the structs behind it point to.
//
//go:uintptrescapes
func call(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, args...)
	return r
}

// do calls a command that returns a VkResult: what names it in the error.
//
//go:uintptrescapes
func do(what string, fn uintptr, args ...uintptr) error {
	r, _, _ := purego.SyscallN(fn, args...)
	if res := int32(r); res != 0 {
		return fmt.Errorf("%s: %w", what, vkError(res))
	}
	return nil
}

// The commands, by the names without vk: load fills in the fields.

type globalFuncs struct {
	CreateInstance uintptr
}

type instanceFuncs struct {
	DestroyInstance                              uintptr
	EnumeratePhysicalDevices                     uintptr
	EnumerateDeviceExtensionProperties           uintptr
	GetPhysicalDeviceProperties2                 uintptr
	GetPhysicalDeviceQueueFamilyProperties       uintptr
	GetPhysicalDeviceMemoryProperties            uintptr
	GetPhysicalDeviceFormatProperties2           uintptr
	GetPhysicalDeviceImageFormatProperties2      uintptr
	GetPhysicalDeviceExternalSemaphoreProperties uintptr
	GetDeviceProcAddr                            uintptr
	CreateDevice                                 uintptr
}

type deviceFuncs struct {
	DestroyDevice  uintptr
	GetDeviceQueue uintptr
	QueueSubmit    uintptr
	DeviceWaitIdle uintptr

	CreateImage                            uintptr
	DestroyImage                           uintptr
	GetImageMemoryRequirements             uintptr
	GetImageSubresourceLayout              uintptr
	GetImageDrmFormatModifierPropertiesEXT uintptr
	CreateImageView                        uintptr
	DestroyImageView                       uintptr
	AllocateMemory                         uintptr
	FreeMemory                             uintptr
	BindImageMemory                        uintptr
	GetMemoryFdKHR                         uintptr

	CreateShaderModule      uintptr
	DestroyShaderModule     uintptr
	CreatePipelineLayout    uintptr
	DestroyPipelineLayout   uintptr
	CreateGraphicsPipelines uintptr
	DestroyPipeline         uintptr

	CreateFence        uintptr
	DestroyFence       uintptr
	WaitForFences      uintptr
	ResetFences        uintptr
	CreateSemaphore    uintptr
	DestroySemaphore   uintptr
	GetSemaphoreFdKHR  uintptr
	CreateCommandPool  uintptr
	DestroyCommandPool uintptr
	ResetCommandPool   uintptr

	AllocateCommandBuffers uintptr
	BeginCommandBuffer     uintptr
	EndCommandBuffer       uintptr
	CmdPipelineBarrier     uintptr
	CmdBeginRendering      uintptr
	CmdEndRendering        uintptr
	CmdBindPipeline        uintptr
	CmdSetViewport         uintptr
	CmdSetScissor          uintptr
	CmdPushConstants       uintptr
	CmdDraw                uintptr
	CmdCopyImage           uintptr
}

// load fills the fields of the struct fns points to with the addresses that
// get returns for their commands.
func load(fns any, get func(name string) uintptr) error {
	v := reflect.ValueOf(fns).Elem()
	for i := range v.NumField() {
		name := "vk" + v.Type().Field(i).Name
		addr := get(name)
		if addr == 0 {
			return fmt.Errorf("the Vulkan driver has no %s", name)
		}
		v.Field(i).SetUint(uint64(addr))
	}
	return nil
}

// loader is the Vulkan loader of the system, opened once.
var loader = sync.OnceValues(func() (uintptr, error) {
	for _, name := range []string{"libvulkan.so.1", "libvulkan.so"} {
		if lib, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL); err == nil {
			return purego.Dlsym(lib, "vkGetInstanceProcAddr")
		}
	}
	return 0, errors.New("no Vulkan loader (libvulkan.so.1) is installed")
})

// cString returns s as C takes it, and cStrings a list of them.
func cString(s string) *byte { return &append([]byte(s), 0)[0] }

func cStrings(list []string) (uint32, **byte) {
	if len(list) == 0 {
		return 0, nil
	}
	out := make([]*byte, len(list))
	for i, s := range list {
		out[i] = cString(s)
	}
	return uint32(len(out)), &out[0]
}

func goString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// Instance is a connection to Vulkan.
type Instance struct {
	h  uintptr
	fn instanceFuncs

	// With OpenValidated: the messenger of the debug messages, and the
	// command that ends it.
	messenger        uint64
	destroyMessenger uintptr
}

const version13 = 1<<22 | 3<<12

// Open connects to Vulkan.
func Open() (*Instance, error) { return open(nil, nil) }

// open connects to Vulkan as edit, unless it is nil, says, and with the
// messenger that debug describes, if any.
func open(edit func(*instanceCreateInfo), debug *debugUtilsMessengerCreateInfoEXT) (*Instance, error) {
	get, err := loader()
	if err != nil {
		return nil, err
	}
	var global globalFuncs
	if err := load(&global, func(name string) uintptr { return call(get, 0, uintptr(unsafe.Pointer(cString(name)))) }); err != nil {
		return nil, err
	}
	info := &instanceCreateInfo{
		SType: structureTypeInstanceCreateInfo,
		PApplicationInfo: &applicationInfo{
			SType:            structureTypeApplicationInfo,
			PApplicationName: cString("tela examples/texture"),
			ApiVersion:       version13,
		},
	}
	if edit != nil {
		edit(info)
	}
	i := &Instance{}
	if err := do("vkCreateInstance", global.CreateInstance, uintptr(unsafe.Pointer(info)), 0, uintptr(unsafe.Pointer(&i.h))); err != nil {
		return nil, err
	}
	proc := func(name string) uintptr { return call(get, i.h, uintptr(unsafe.Pointer(cString(name)))) }
	err = load(&i.fn, proc)
	if err == nil && debug != nil {
		i.destroyMessenger = proc("vkDestroyDebugUtilsMessengerEXT")
		err = do("vkCreateDebugUtilsMessengerEXT", proc("vkCreateDebugUtilsMessengerEXT"), i.h, uintptr(unsafe.Pointer(debug)), 0, uintptr(unsafe.Pointer(&i.messenger)))
	}
	if err != nil {
		i.Close()
		return nil, err
	}
	return i, nil
}

// Close ends the connection; its renderers are closed before.
func (i *Instance) Close() {
	if i.messenger != 0 {
		call(i.destroyMessenger, i.h, uintptr(i.messenger), 0)
	}
	if i.fn.DestroyInstance != 0 {
		call(i.fn.DestroyInstance, i.h, 0)
	}
}

// Device is a GPU.
type Device struct {
	Name string
	// UUID is its Vulkan device UUID, and Major and Minor the numbers of its
	// DRM render node, zero when it has none.
	UUID         [16]byte
	Major, Minor int64

	h       uintptr
	version uint32
	exts    map[string]bool
	maxSize int
}

// Devices returns the GPUs of the system (no software renderer).
func (i *Instance) Devices() ([]Device, error) {
	const most = 16
	var handles [most]uintptr
	n := uint32(most)
	if err := do("vkEnumeratePhysicalDevices", i.fn.EnumeratePhysicalDevices, i.h, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&handles))); err != nil && !errors.Is(err, vkError(incomplete)) {
		return nil, err
	}
	var out []Device
	for _, h := range handles[:n] {
		d := Device{h: h, exts: i.extensions(h)}
		drm := &physicalDeviceDrmPropertiesEXT{SType: structureTypePhysicalDeviceDrmPropertiesEXT}
		id := &physicalDeviceIDProperties{SType: structureTypePhysicalDeviceIdProperties}
		if d.exts["VK_EXT_physical_device_drm"] {
			id.PNext = unsafe.Pointer(drm)
		}
		p := &physicalDeviceProperties2{SType: structureTypePhysicalDeviceProperties2, PNext: unsafe.Pointer(id)}
		call(i.fn.GetPhysicalDeviceProperties2, h, uintptr(unsafe.Pointer(p)))
		if p.Properties.DeviceType == physicalDeviceTypeCpu {
			continue
		}
		d.Name, d.UUID = goString(p.Properties.DeviceName[:]), id.DeviceUUID
		d.version, d.maxSize = p.Properties.ApiVersion, int(p.Properties.Limits.MaxImageDimension2D)
		if drm.HasRender != 0 {
			d.Major, d.Minor = drm.RenderMajor, drm.RenderMinor
		}
		out = append(out, d)
	}
	return out, nil
}

// extensions returns the device extensions of a GPU.
func (i *Instance) extensions(h uintptr) map[string]bool {
	var n uint32
	call(i.fn.EnumerateDeviceExtensionProperties, h, 0, uintptr(unsafe.Pointer(&n)), 0)
	if n == 0 {
		return nil
	}
	list := make([]extensionProperties, n)
	call(i.fn.EnumerateDeviceExtensionProperties, h, 0, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&list[0])))
	out := map[string]bool{}
	for k := range list[:n] {
		out[goString(list[k].ExtensionName[:])] = true
	}
	return out
}

// deviceExtensions are what the renderer enables: dmabufs of images with a
// DRM format modifier, sync files, and the queue family of whoever reads
// them outside.
var deviceExtensions = []string{
	"VK_KHR_external_memory_fd",
	"VK_EXT_external_memory_dma_buf",
	"VK_EXT_image_drm_format_modifier",
	"VK_KHR_external_semaphore_fd",
	"VK_EXT_queue_family_foreign",
}

// check returns why the renderer cannot draw with the device.
func (i *Instance) check(d *Device) error {
	if d.version < version13 {
		return fmt.Errorf("%s has Vulkan %d.%d; the scene asks for 1.3", d.Name, d.version>>22, d.version>>12&0x3ff)
	}
	for _, e := range deviceExtensions {
		if !d.exts[e] {
			return fmt.Errorf("%s has no %s", d.Name, e)
		}
	}
	info := &physicalDeviceExternalSemaphoreInfo{SType: structureTypePhysicalDeviceExternalSemaphoreInfo, HandleType: externalSemaphoreHandleTypeSyncFdBit}
	props := &externalSemaphoreProperties{SType: structureTypeExternalSemaphoreProperties}
	call(i.fn.GetPhysicalDeviceExternalSemaphoreProperties, d.h, uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(props)))
	if props.ExternalSemaphoreFeatures&externalSemaphoreFeatureExportableBit == 0 {
		return fmt.Errorf("%s exports no sync files", d.Name)
	}
	return nil
}

// graphicsFamily returns a queue family that draws.
func (i *Instance) graphicsFamily(d *Device) (uint32, error) {
	const most = 32
	var list [most]queueFamilyProperties
	n := uint32(most)
	call(i.fn.GetPhysicalDeviceQueueFamilyProperties, d.h, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&list)))
	for k := range list[:n] {
		if list[k].QueueFlags&queueGraphicsBit != 0 {
			return uint32(k), nil
		}
	}
	return 0, fmt.Errorf("%s has no graphics queue", d.Name)
}

// Formats returns the formats of which the device draws frames that others
// can read.
func (i *Instance) Formats(d Device) []Format {
	var out []Format
	if i.check(&d) != nil {
		return nil
	}
	for _, f := range vkFormats {
		for m := range i.modifiers(&d, f.format, imageUsageColorAttachmentBit) {
			out = append(out, Format{Fourcc: f.fourcc, Modifier: m})
		}
	}
	return out
}

// modifiers returns the DRM format modifiers with which the device makes
// images of a format that it can use so and export as a dmabuf, and how many
// memory planes each one has.
func (i *Instance) modifiers(d *Device, format, usage uint32) map[uint64]int {
	list := &drmFormatModifierPropertiesListEXT{SType: structureTypeDrmFormatModifierPropertiesListEXT}
	props := &formatProperties2{SType: structureTypeFormatProperties2, PNext: unsafe.Pointer(list)}
	call(i.fn.GetPhysicalDeviceFormatProperties2, d.h, uintptr(format), uintptr(unsafe.Pointer(props)))
	if list.DrmFormatModifierCount == 0 {
		return nil
	}
	mods := make([]drmFormatModifierPropertiesEXT, list.DrmFormatModifierCount)
	list.PDrmFormatModifierProperties = &mods[0]
	call(i.fn.GetPhysicalDeviceFormatProperties2, d.h, uintptr(format), uintptr(unsafe.Pointer(props)))

	// The image format query below does not say all of it: NVIDIA answers
	// it for a linear color attachment, which its features lack.
	need := uint32(formatFeatureColorAttachmentBit)
	if usage&imageUsageTransferDstBit != 0 {
		need = formatFeatureTransferDstBit
	}
	out := map[uint64]int{}
	for _, m := range mods[:list.DrmFormatModifierCount] {
		if m.DrmFormatModifierTilingFeatures&need == 0 {
			continue
		}
		mod := &physicalDeviceImageDrmFormatModifierInfoEXT{
			SType:             structureTypePhysicalDeviceImageDrmFormatModifierInfoEXT,
			DrmFormatModifier: m.DrmFormatModifier,
			SharingMode:       sharingModeExclusive,
		}
		ext := &physicalDeviceExternalImageFormatInfo{
			SType:      structureTypePhysicalDeviceExternalImageFormatInfo,
			PNext:      unsafe.Pointer(mod),
			HandleType: externalMemoryHandleTypeDmaBufBitEXT,
		}
		info := &physicalDeviceImageFormatInfo2{
			SType:  structureTypePhysicalDeviceImageFormatInfo2,
			PNext:  unsafe.Pointer(ext),
			Format: format,
			Type:   imageType2D,
			Tiling: imageTilingDrmFormatModifierEXT,
			Usage:  usage,
		}
		extProps := &externalImageFormatProperties{SType: structureTypeExternalImageFormatProperties}
		imgProps := &imageFormatProperties2{SType: structureTypeImageFormatProperties2, PNext: unsafe.Pointer(extProps)}
		err := do("", i.fn.GetPhysicalDeviceImageFormatProperties2, d.h, uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(imgProps)))
		if err == nil && extProps.ExternalMemoryProperties.ExternalMemoryFeatures&externalMemoryFeatureExportableBit != 0 {
			out[m.DrmFormatModifier] = int(m.DrmFormatModifierPlaneCount)
		}
	}
	return out
}
