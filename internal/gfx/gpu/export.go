package gpu

import (
	"errors"
	"image"
	"slices"
)

// Presenting a target without a copy (ADR-281): images a device exports
// as dmabufs, for a window that shows such memory as it is (the platform
// layer's DmabufPresenter, which this package does not know: it speaks in
// the same plain values, a DRM format code, a format modifier, planes of a
// file descriptor, an offset and a stride).
//
// A window's target stays the frame: it is drawn into, shifted and kept
// between frames as before. An Export is a second image of the same size
// whose memory the display server reads, and the frame reaches it by a
// copy on the device (Export.CopyFrom), which returns when the GPU has
// finished: no fence travels with a dmabuf, and not every driver attaches
// one to it (NVIDIA has no implicit synchronization), so the pixels are
// handed over finished. Around the copy the image is taken from, and
// given back to, whoever else reads it: the layout other APIs read
// (general) and, where the device has VK_EXT_queue_family_foreign, a
// transfer to the foreign queue family and back, which keeps the pixels.
//
// Only the Vulkan device on Linux implements Exporter; on Windows it is a
// WindowPresenter instead (swapchain.go), and elsewhere the caller reads
// the frame back.

// The DRM formats (fourcc codes of drm_fourcc.h) a device exports: 32-bit
// pixels named from the most significant byte of a little-endian word, so
// XRGB8888 is B, G, R, X in memory and XBGR8888 is R, G, B, X, the order
// of a target. The X formats have no alpha.
const (
	FourccXRGB8888 uint32 = 0x34325258 // "XR24"
	FourccARGB8888 uint32 = 0x34325241 // "AR24"
	FourccXBGR8888 uint32 = 0x34324258 // "XB24"
	FourccABGR8888 uint32 = 0x34324241 // "AB24"
)

// The DRM format modifiers with a meaning of their own.
const (
	// ModifierLinear (DRM_FORMAT_MOD_LINEAR) is rows of pixels one after
	// the other.
	ModifierLinear uint64 = 0
	// ModifierInvalid (DRM_FORMAT_MOD_INVALID) stands for no explicit
	// modifier. A device exports no image under it.
	ModifierInvalid uint64 = 0x00ffffffffffffff
)

// DmabufFormat is a pixel format with the layout of its pixels in memory:
// a DRM fourcc and a DRM format modifier.
type DmabufFormat struct {
	Fourcc   uint32
	Modifier uint64
}

// DmabufPlane is one memory plane of an exported image: a dmabuf, where
// the plane starts in it and the bytes from one row to the next.
type DmabufPlane struct {
	FD     int
	Offset uint32
	Stride uint32
}

// DRMNode is the device number of a DRM node of a device: its primary
// node (/dev/dri/cardN) or its render node (/dev/dri/renderDN).
type DRMNode struct{ Major, Minor uint32 }

// ErrNoExport is returned by Exporter.NewExport when the device exports
// no image in any of the formats accepted.
var ErrNoExport = errors.New("gpu: the device exports no image in an accepted format")

// An Exporter is a Device that exports images as dmabufs. A caller asks a
// Device for it with a type assertion.
type Exporter interface {
	// DRMNodes returns the device numbers of the device's DRM nodes, the
	// ones it has: a display server names its device by one of them.
	DRMNodes() []DRMNode
	// ExportFormats returns every pair of format and modifier the device
	// exports images in. The slice is shared: do not modify it.
	ExportFormats() []DmabufFormat
	// NewExport creates an image of the given size in device pixels whose
	// memory is exported, in the format ChooseExport picks among the
	// pairs accepted (a window's, in its order of preference) and with
	// the modifier the driver prefers among that format's. Its pixels are
	// undefined until the first CopyFrom. It returns ErrNoExport when the
	// device exports none of the pairs.
	NewExport(width, height int, accepted []DmabufFormat) (Export, error)
}

// An Export is an image of a device whose memory is a dmabuf: a buffer of
// a window's swapchain.
type Export interface {
	// Size returns the size in device pixels.
	Size() image.Point
	// Format returns the format and the modifier the driver chose.
	Format() DmabufFormat
	// Planes returns the memory planes of the image, in order. Their
	// descriptors are the Export's, closed by Close: whoever is given the
	// memory duplicates them.
	Planes() []DmabufPlane
	// CopyFrom copies the pixels of the rectangles of a target of the
	// same device and size into the image, at the same coordinates, and
	// returns when the GPU has finished: the image can be handed over.
	// The pixels outside the rectangles are kept, except by the first
	// copy into the image, which copies the whole target. The image must
	// not be on screen: whoever shows it must have released it.
	CopyFrom(t Target, rects ...image.Rectangle) error
	// ReadPixels copies the image's pixels into dst, at the same
	// coordinates, in the order of a target's (R, G, B, A; the fourth
	// byte of a format without alpha is whatever was copied into it). For
	// tests and diagnosis: what a window is given is checked against the
	// target it was copied from.
	ReadPixels(dst *image.RGBA) error
	// Close destroys the image and closes its descriptors. Whoever holds
	// a duplicate keeps the memory alive. It may be called more than once,
	// and after the device was closed.
	Close()
}

// exportFourccs are the formats a device exports: the two without alpha,
// then the two with it (a window's buffers are opaque, and a display
// server may blend with the alpha of a format that has one).
var exportFourccs = [...]uint32{FourccXBGR8888, FourccXRGB8888, FourccABGR8888, FourccARGB8888}

// ChooseExport picks the format of a window's buffers: among the pairs the
// window accepts, in its order of preference, and the pairs a device
// exports, a fourcc and the modifiers to offer the driver. A format
// without alpha wins over one with it; among those, the one the window
// lists first. Only explicit modifiers count (ModifierInvalid names no
// layout), and the linear one is offered only when it is the only one in
// common: a tiled layout is what a device draws and reads fastest. ok is
// false when nothing is common.
func ChooseExport(accepted, exportable []DmabufFormat) (fourcc uint32, modifiers []uint64, ok bool) {
	for _, alpha := range []bool{false, true} {
		for _, a := range accepted {
			if i := slices.Index(exportFourccs[:], a.Fourcc); i < 0 || (i >= 2) != alpha {
				continue
			}
			var common []uint64
			for _, p := range accepted {
				if p.Fourcc == a.Fourcc && p.Modifier != ModifierInvalid && slices.Contains(exportable, p) && !slices.Contains(common, p.Modifier) {
					common = append(common, p.Modifier)
				}
			}
			if len(common) > 1 {
				common = slices.DeleteFunc(common, func(m uint64) bool { return m == ModifierLinear })
			}
			if len(common) > 0 {
				return a.Fourcc, common, true
			}
		}
	}
	return 0, nil, false
}
