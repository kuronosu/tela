package gfx

// External is the content of an Image whose pixels somebody else owns: a
// frame of an external texture (ADR-315). The pixels do not change while
// the Image is in use, as those of any Image.
type External interface {
	// Pixels returns the image's premultiplied RGBA8 pixels, row-major with
	// 4*width bytes per row, for a renderer that cannot use the content as
	// it is: the CPU rasterizer, for memory of the GPU. It may copy or wait;
	// the Image calls it at most once and keeps the answer. nil means the
	// pixels cannot be read: the Image then draws transparent pixels.
	Pixels() []byte
}

// NewImageNoCopy creates an Image over premultiplied RGBA8 pixels that it
// does not copy: pix must hold exactly 4*w*h bytes, which must not change
// for as long as something draws the Image. opaque tells that every alpha
// is 255, so that nobody looks; false when not known to be.
func NewImageNoCopy(w, h int, pix []byte, opaque bool) *Image {
	checkImageSize("NewImageNoCopy", w, h, len(pix))
	return &Image{id: nextImageID.Add(1), w: w, h: h, pix: pix, known: true, opaque: opaque}
}

// NewExternalImage creates an Image of w x h pixels whose content is ext. A
// renderer that understands ext's type uses it; others read Pix, which
// asks ext for the pixels. opaque is as in NewImageNoCopy.
func NewExternalImage(w, h int, opaque bool, ext External) *Image {
	if w <= 0 || h <= 0 || ext == nil {
		panic("gfx.NewExternalImage: an external image has a size of at least 1x1 and a content")
	}
	return &Image{id: nextImageID.Add(1), w: w, h: h, ext: ext, known: true, opaque: opaque}
}

// External returns the external content of the image, nil for an ordinary
// one.
func (i *Image) External() External { return i.ext }

// KnownOpaque reports whether the image was created with a statement about
// its opacity, and the statement. A known image must be trusted without a
// look at its pixels, which may be costly to read.
func (i *Image) KnownOpaque() (opaque, known bool) { return i.opaque, i.known }
