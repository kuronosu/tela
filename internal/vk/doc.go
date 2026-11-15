// Package vk holds hand-written Vulkan bindings for the GPU backend
// (ADR-281): the subset of Vulkan 1.1 that a 2D renderer drawing into
// images it owns needs, called without cgo through
// github.com/ebitengine/purego.
//
// The bindings compile only with the build tag telagpu, on Linux and
// Windows (amd64 and arm64). Without the tag, and on every other system,
// the package is empty: purego makes a Linux binary dynamically linked, and
// Tela's default is one static binary. It is a leaf package (ADR-189): it
// imports only purego, golang.org/x/sys and the standard library, so every
// layer may use it.
//
// # Use
//
// [Load] opens the Vulkan loader (libvulkan.so.1, vulkan-1.dll) once and
// returns an error, never a panic, when Vulkan is missing. [CreateInstance]
// returns an [Instance], whose methods are the instance-level commands, and
// [Instance.CreateDevice] a [Device], whose methods are the device-level
// ones, each with its own table of entry points (vkGetInstanceProcAddr,
// vkGetDeviceProcAddr). [Instance.Devices] lists the physical devices Tela
// can use and [Instance.PickDevice] chooses one (TELA_GPU_DEVICE).
//
// A method has the name of its command without the vk prefix and the handle
// it dispatches on (Device.CreateBuffer is vkCreateBuffer). Commands that
// return a VkResult return an error: nil for VK_SUCCESS, else a [Result],
// also for the codes that are not failures ([Timeout], [NotReady]), which
// callers compare with ==. Arrays are slices, the two-call enumerations
// return slices, and allocation callbacks are always NULL. Structs, enums
// and flags mirror the C ones: a struct has the fields of its C struct in
// the same order, under the same names with the first letter in upper case,
// and the caller sets SType itself.
//
// What is here: instances ([NewInstance] makes the one Tela wants, with
// the Khronos validation layer and its synchronization checks on
// request), the debug messenger of VK_EXT_debug_utils, physical devices
// (properties, features, queue families, memory, formats, extensions, the
// DRM nodes of VK_EXT_physical_device_drm), devices and queues, command
// pools and buffers, buffers, images, views, samplers, memory, render
// passes and framebuffers, shader modules, descriptors, pipeline layouts
// and graphics pipelines, fences and semaphores, and the commands to draw,
// copy, blit and synchronize. There is no window system integration: on
// Linux the renderer presents by reading back or through dmabufs
// (ADR-281). Rendering uses render passes and framebuffers, not the
// dynamic rendering of Vulkan 1.3, so that drivers that stop at Vulkan 1.1
// or 1.2 work (older Windows drivers, older Mesa); 1.1 is required for
// vkGetPhysicalDeviceProperties2 and for the external memory the dmabuf
// path builds on.
//
// The export of images as dmabufs is in dmabuf.go (the wrappers of
// VK_KHR_external_memory_fd, VK_EXT_external_memory_dma_buf and
// VK_EXT_image_drm_format_modifier, [DmabufExtensions] and
// [Instance.DmabufModifiers]) and, on Linux, dmabuf_linux.go
// ([Device.CreateDmabufImage]); TestDmabufExport is its worked example.
//
// # Calls and pointers
//
// Every command goes through the call layer of call.go, over
// purego.SyscallN: no reflection, and on Windows a plain
// syscall.SyscallN. Its functions are marked uintptrescapes, so a pointer
// converted to uintptr in the argument list of a call is kept alive for
// the call and its object is moved to the heap, where the garbage
// collector never moves it; a uintptr computed earlier would point into a
// stack that may move before Vulkan reads it. That is why every wrapper
// converts its pointers in the argument list itself, and why the pointer
// fields of the structs are Go pointers (*T, unsafe.Pointer), never
// uintptr: what a struct points to is then on the heap and alive as long
// as the struct is. Vulkan reads and writes this memory only during the
// call.
//
// The consequence for callers: a struct or slice handed to a command is a
// heap allocation where it is created, so code that records many commands
// per frame keeps them in buffers it reuses; the call itself allocates
// nothing. Measured on an i5-10400 with both drivers (BenchmarkCall), a
// recorded command costs about 110 ns and no allocation, the same call
// through purego.SyscallN directly 110 ns and one allocation (its
// variadic arguments), and through a function of purego.RegisterFunc
// about 380 ns and four or five allocations.
//
// The call layer does not pass floating-point arguments by value; no
// bound command has one (vkCmdSetViewport and vkCmdSetBlendConstants take
// pointers), and one that does (vkCmdSetLineWidth, vkCmdSetDepthBias)
// must not be added this way.
//
// Vulkan handles have no thread affinity, but most objects need external
// synchronization: the bindings add none.
//
// # Adding to the bindings
//
// One place per thing: structs in structs.go, constants in enums.go (each
// with its C name in a trailing comment), handle types in handles.go,
// entry points in the tables of funcs.go, and the wrappers in instance.go,
// device.go and commands.go (dmabuf.go for the dmabuf export, whose
// structs, constants and entry points are a marked block at the end of
// their files). Struct layouts and constant values are not
// trusted: internal/layoutgen writes a C program (internal/layoutgen/c)
// that prints sizeof, alignof and offsetof of every struct of structs.go
// with the member list of the real header, and the value of every
// constant of enums.go; its output is layout_table_test.go, which
// TestLayout and TestConstants compare with the Go side, and which the
// compiler checks too when the tests are built for another target.
// Regenerate it after adding a struct or a constant (it needs the Vulkan
// headers and a C compiler, so it is a manual step; the checked-in table
// is what CI checks, and TestLayoutTableCurrent fails when it is stale):
//
//	go generate ./internal/vk
//
// The same command compiles the GLSL sources of the tests under
// testdata/shaders to SPIR-V with glslangValidator; the .spv files are
// checked in. `go run ./internal/layoutgen -cross`, from the package's
// directory, also checks with clang that the C layouts are the same on
// the four targets.
//
// # Tests
//
// The tests need the tag and skip without Vulkan or a device; TestRender
// draws on every physical device and compares the pixels read back, and
// is the worked example of the bindings. With TELA_VK_VALIDATE=1 they
// enable VK_LAYER_KHRONOS_validation, with synchronization validation,
// and fail on every warning or error it reports (TestValidationReports
// checks that one would arrive). To check that no command is handed a
// pointer into a stack, run them with a stack move forced at every
// function call:
//
//	go test -tags telagpu -gcflags=all=-d=maymorestack=runtime.mayMoreStackMove ./internal/vk/
package vk

//go:generate go run ./internal/layoutgen
//go:generate glslangValidator --quiet -V -o testdata/shaders/quad.vert.spv testdata/shaders/quad.vert
//go:generate glslangValidator --quiet -V -o testdata/shaders/quad.frag.spv testdata/shaders/quad.frag
