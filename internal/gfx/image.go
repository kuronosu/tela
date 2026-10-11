package gfx

import (
	"fmt"
	"image"
	"image/draw"
	"sync/atomic"
)

// Sampling selects how DrawImage reads source pixels when scaling or
// transforming.
type Sampling uint8

const (
	// SampleLinear uses bilinear filtering. It is the zero value.
	SampleLinear Sampling = iota
	// SampleNearest picks the nearest source pixel (pixel art, icons at
	// integer scales).
	SampleNearest
)

// String returns "linear" or "nearest".
func (s Sampling) String() string {
	if s == SampleNearest {
		return "nearest"
	}
	return "linear"
}

var nextImageID atomic.Uint64

// Image is an immutable bitmap of premultiplied RGBA8 pixels in sRGB. It is
// created with NewImage, which copies the pixels, so an Image can be shared
// freely between goroutines and DisplayLists. Its ID identifies it in
// rasterizer caches (uploaded textures, mip chains).
type Image struct {
	id   uint64
	w, h int
	pix  []byte // 4*w*h bytes, premultiplied RGBA, row-major, stride 4*w
}

// NewImage copies src into a new Image. An *image.RGBA is copied as is (it
// is already premultiplied); any other image type is converted. NewImage
// panics if src is nil. A zero-size image is valid but draws nothing.
func NewImage(src image.Image) *Image {
	if src == nil {
		panic("gfx.NewImage: src is nil; pass a decoded image")
	}
	b := src.Bounds()
	img := &Image{id: nextImageID.Add(1), w: b.Dx(), h: b.Dy()}
	img.pix = make([]byte, 4*img.w*img.h)
	if rgba, ok := src.(*image.RGBA); ok {
		for y := 0; y < img.h; y++ {
			off := rgba.PixOffset(b.Min.X, b.Min.Y+y)
			copy(img.pix[y*4*img.w:(y+1)*4*img.w], rgba.Pix[off:off+4*img.w])
		}
		return img
	}
	dst := &image.RGBA{Pix: img.pix, Stride: 4 * img.w, Rect: image.Rect(0, 0, img.w, img.h)}
	draw.Draw(dst, dst.Rect, src, b.Min, draw.Src)
	return img
}

// NewImagePremul creates an Image from premultiplied RGBA8 pixels, copying
// them. pix must hold exactly 4*w*h bytes.
func NewImagePremul(w, h int, pix []byte) *Image {
	if w < 0 || h < 0 || len(pix) != 4*w*h {
		panic(fmt.Sprintf("gfx.NewImagePremul: %dx%d image needs %d bytes of RGBA data, got %d", w, h, 4*w*h, len(pix)))
	}
	return &Image{id: nextImageID.Add(1), w: w, h: h, pix: append([]byte(nil), pix...)}
}

// ID returns a process-unique identifier, stable for the life of the Image.
func (i *Image) ID() uint64 { return i.id }

// Width returns the width in pixels.
func (i *Image) Width() int { return i.w }

// Height returns the height in pixels.
func (i *Image) Height() int { return i.h }

// Bounds returns the pixel bounds as a Rect starting at the origin.
func (i *Image) Bounds() Rect { return Rect{0, 0, float32(i.w), float32(i.h)} }

// Stride returns the number of bytes per row (4*Width).
func (i *Image) Stride() int { return 4 * i.w }

// Pix returns the premultiplied RGBA8 pixels, row-major with Stride bytes per
// row. The slice is shared with the Image and must not be modified.
func (i *Image) Pix() []byte { return i.pix }
