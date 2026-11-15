//go:build telagpu && (linux || windows)

package vk

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"
)

// MakeVersion packs a version the way VK_MAKE_API_VERSION does, with
// variant 0.
func MakeVersion(major, minor, patch uint32) uint32 {
	return major<<22 | minor<<12 | patch
}

// VersionMajor, VersionMinor and VersionPatch take a packed version apart.
func VersionMajor(v uint32) uint32 { return v >> 22 & 0x7f }

// VersionMinor is the minor number of a packed version.
func VersionMinor(v uint32) uint32 { return v >> 12 & 0x3ff }

// VersionPatch is the patch number of a packed version.
func VersionPatch(v uint32) uint32 { return v & 0xfff }

// VersionString formats a packed version as "1.4.329".
func VersionString(v uint32) string {
	return fmt.Sprintf("%d.%d.%d", VersionMajor(v), VersionMinor(v), VersionPatch(v))
}

// CString returns a pointer to a NUL-terminated copy of s, for the
// `const char*` fields of structs. The copy is Go memory: it lives as long
// as something points to it.
func CString(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// CStrings returns NUL-terminated copies of names as the array a
// `const char* const*` field points to (see [CStringArray]).
func CStrings(names []string) []*byte {
	out := make([]*byte, len(names))
	for i, s := range names {
		out[i] = CString(s)
	}
	return out
}

// CStringArray returns the count and the pointer for a pair of fields such
// as EnabledExtensionCount and PpEnabledExtensionNames. The pointer is nil
// for no names.
func CStringArray(names []string) (uint32, **byte) {
	if len(names) == 0 {
		return 0, nil
	}
	return uint32(len(names)), unsafe.SliceData(CStrings(names))
}

// GoString copies the NUL-terminated string at p. A nil p is "".
func GoString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// fixedString returns the string in a fixed array of chars, up to its
// first NUL.
func fixedString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// Name returns the extension's name.
func (p *ExtensionProperties) Name() string { return fixedString(p.ExtensionName[:]) }

// Name returns the layer's name.
func (p *LayerProperties) Name() string { return fixedString(p.LayerName[:]) }

// Name returns the device's name.
func (p *PhysicalDeviceProperties) Name() string { return fixedString(p.DeviceName[:]) }

// SPIRV converts a SPIR-V module from the bytes of its file (little
// endian, as glslang writes them and go:embed delivers them) to the words
// that [ShaderModuleCreateInfo] points to. It panics when the length is
// not a multiple of four: the modules are compiled in.
func SPIRV(code []byte) []uint32 {
	if len(code)%4 != 0 {
		panic("vk: a SPIR-V module is a sequence of 32-bit words")
	}
	words := make([]uint32, len(code)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(code[4*i:])
	}
	return words
}

// ClearColorFloat is a clear color for the normalized and float formats.
func ClearColorFloat(r, g, b, a float32) ClearColorValue {
	return ClearColorValue{math.Float32bits(r), math.Float32bits(g), math.Float32bits(b), math.Float32bits(a)}
}

// ClearColor is the clear value of a color attachment with a normalized or
// float format.
func ClearColor(r, g, b, a float32) ClearValue {
	return ClearValue(ClearColorFloat(r, g, b, a))
}

// ClearDepthStencil is the clear value of a depth and stencil attachment.
func ClearDepthStencil(depth float32, stencil uint32) ClearValue {
	return ClearValue{math.Float32bits(depth), stencil}
}

// FindMemoryType returns the index of the first memory type that is
// allowed by typeBits (the MemoryTypeBits of a [MemoryRequirements]) and
// has all of the wanted properties.
func (p *PhysicalDeviceMemoryProperties) FindMemoryType(typeBits uint32, want MemoryPropertyFlags) (uint32, bool) {
	for i := uint32(0); i < p.MemoryTypeCount && i < MaxMemoryTypes; i++ {
		if typeBits&(1<<i) != 0 && p.MemoryTypes[i].PropertyFlags&want == want {
			return i, true
		}
	}
	return 0, false
}
