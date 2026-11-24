package core

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// Imported buffers of textures (ADR-315): memory the application allocated
// with its own graphics API and gives to a Texture once, to submit it as
// many frames.
//
// The Host's GPU imports it into its device (textureImporter; gputexture.go),
// and samples it without a copy. Without a GPU, or for memory it does not
// import, linear dmabufs and IOSurfaces (texture_iosurface.go) are mapped, and
// every frame that becomes ready is copied first, on the goroutine that waited
// for its fence (Texture.await): the copy is what the window draws, and the
// buffer is not read again. It is still held until its frame is replaced, as a
// buffer the device reads is, so that an application behaves the same with
// and without the GPU.

// TextureMemory is memory that Texture.Import imports: a Dmabuf or an
// IOSurface.
type TextureMemory interface{ textureMemory() }

// Dmabuf is memory shared by a Linux dma-buf (Texture.Import).
type Dmabuf struct {
	// Width and Height are the size of the picture in pixels.
	Width, Height int
	// Fourcc is the DRM fourcc of its pixels: AR24 or AB24 (premultiplied
	// alpha), or XR24 or XB24 (opaque), 8 bits per channel in sRGB.
	Fourcc uint32
	// Modifier is the DRM format modifier of its layout; 0 is linear.
	Modifier uint64
	// Planes are the memory planes of the layout, as the modifier defines
	// them: one for a linear picture.
	Planes []DmabufPlane
}

// DmabufPlane is a memory plane of a Dmabuf.
type DmabufPlane struct {
	// FD is the descriptor of the dma-buf that holds the plane. It stays
	// the application's: Import keeps a duplicate.
	FD int
	// Offset is where the plane starts in it, and Stride the bytes from a
	// row to the next one.
	Offset, Stride uint32
}

func (Dmabuf) textureMemory() {}

// The fourccs a texture takes.
const (
	fourccAR24 = platform.DRMFormatARGB8888 // bytes B G R A
	fourccXR24 = platform.DRMFormatXRGB8888 // bytes B G R X
	fourccAB24 = platform.DRMFormatABGR8888 // bytes R G B A
	fourccXB24 = platform.DRMFormatXBGR8888 // bytes R G B X
)

// textureFourcc describes fourcc f: whether its bytes are blue first and
// whether it is opaque; ok is false for one textures do not take.
func textureFourcc(f uint32) (swap, opaque, ok bool) {
	switch f {
	case fourccAR24:
		return true, false, true
	case fourccXR24:
		return true, true, true
	case fourccAB24:
		return false, false, true
	case fourccXB24:
		return false, true, true
	}
	return false, false, false
}

// fourccString returns the four characters of f.
func fourccString(f uint32) string {
	b := [4]byte{byte(f), byte(f >> 8), byte(f >> 16), byte(f >> 24)}
	for _, c := range b {
		if c < ' ' || c > '~' {
			return fmt.Sprintf("0x%08x", f)
		}
	}
	return string(b[:])
}

// describe names the picture in errors: "a 1280x720 dmabuf, AR24, modifier
// 0x0, 1 plane".
func (d Dmabuf) describe() string {
	planes := "1 plane"
	if n := len(d.Planes); n != 1 {
		planes = fmt.Sprintf("%d planes", n)
	}
	return fmt.Sprintf("a %dx%d dmabuf, %s, modifier 0x%x, %s", d.Width, d.Height, fourccString(d.Fourcc), d.Modifier, planes)
}

// texMaxSide bounds the width and the height of an imported picture.
const texMaxSide = 1 << 14

// check reports what is wrong with d whatever imports it.
func (d Dmabuf) check() error {
	switch {
	case d.Width <= 0 || d.Height <= 0 || d.Width > texMaxSide || d.Height > texMaxSide:
		return fmt.Errorf("tela: Texture.Import: %s: its width and height must be 1 to %d", d.describe(), texMaxSide)
	case len(d.Planes) == 0:
		return fmt.Errorf("tela: Texture.Import: %s: give its planes (one for a linear picture): the descriptor, the offset and the stride", d.describe())
	}
	for i, p := range d.Planes {
		if p.FD < 0 {
			return fmt.Errorf("tela: Texture.Import: %s: plane %d has the descriptor %d", d.describe(), i, p.FD)
		}
	}
	if _, _, ok := textureFourcc(d.Fourcc); !ok {
		return fmt.Errorf("tela: Texture.Import: %s: textures take 8-bit RGB in sRGB, the fourccs AR24, XR24, AB24 and XB24: %w", d.describe(), ErrTextureUnsupported)
	}
	return nil
}

// ---------------------------------------------------------------------
// The device.

// textureImporter is what the GPU that draws a Host's windows does for
// textures (Host.setTextureImporter). Its methods are called from any
// goroutine.
type textureImporter interface {
	// textureDevice describes the device.
	textureDevice() TextureDevice
	// importTexture imports m, whose descriptors stay the caller's, for the
	// texture whose images are of stream (gfx.Image.Stream). An error that
	// wraps ErrTextureUnsupported lets the memory be mapped and copied
	// instead, where that is possible; so does a nil content without an
	// error, of a device that imports nothing or is gone.
	importTexture(m TextureMemory, stream uint64) (textureContent, error)
}

// textureContent is imported memory: the content of the images that draw
// its frames (gfx.NewExternalImage), one image per frame.
type textureContent interface {
	gfx.External
	// closeTexture frees the import, once, from any goroutine, when the
	// buffer is closed and no frame of it is held: no display list that may
	// be drawn has it.
	closeTexture()
}

// deviceTexture is a textureContent whose device may go away before the
// buffer is closed: lost, or closed. Any goroutine.
type deviceTexture interface {
	textureContent
	// deviceGone reports that the device is gone. The frames submitted from
	// then on are copied (read), like those of mapped memory, or refused.
	deviceGone() bool
	// readable returns why the memory cannot be read without the device, for
	// Submit: an error that wraps ErrTextureUnsupported. It does not block.
	readable() error
	textureReader
}

// textureReader copies the pixels of a buffer.
type textureReader interface {
	// read copies the picture into dst, premultiplied RGBA8 with rows of 4
	// bytes per pixel. It may block while the memory is written.
	read(dst []byte) error
}

// setTextureImporter makes imp what imports the memory of the app's
// textures, nil for none. A texture's buffers imported before stay what they
// were.
func (h *Host) setTextureImporter(imp textureImporter) {
	h.texMu.Lock()
	h.texImp = imp
	h.texMu.Unlock()
}

// textureImporter returns what imports the memory of textures, nil for
// nothing. Any goroutine.
func (h *Host) textureImporter() textureImporter {
	h.texMu.Lock()
	defer h.texMu.Unlock()
	return h.texImp
}

// importer returns the importer of the texture's Host, nil for none.
func (x *Texture) importer() textureImporter {
	if x.host == nil {
		return nil
	}
	return x.host.textureImporter()
}

// ---------------------------------------------------------------------
// Buffers.

// mappedTexture is imported memory mapped into the process, to be copied.
type mappedTexture interface {
	textureReader
	close()
}

// TextureBuffer is memory of the application's that a Texture imported
// (Texture.Import), to be shown by Texture.Submit any number of times.
type TextureBuffer struct {
	x      *Texture
	w, h   int
	opaque bool
	// content is the memory imported into the device, or mem the mapped
	// one: one of them.
	content textureContent
	mem     mappedTexture

	// Under the texture's lock: held reports that a frame holds it, closed
	// that it takes no more frames, freed that its memory was given back (or
	// is about to be).
	held, closed, freed bool
}

// Import imports memory that the application allocated, to submit it as
// frames (Submit): once per buffer, not per frame. The memory stays the
// application's, and Import keeps its own reference to it until the buffer
// is closed.
//
// A Dmabuf is imported by the GPU that draws the windows when it takes its
// format and reaches its memory (Device), and its frames are drawn without
// the CPU. Otherwise a Dmabuf with one linear plane (modifier 0) is mapped,
// and copied by the CPU for every frame that becomes ready, on a goroutine of
// Tela's. The GPU opens with the first window: a buffer imported before, in
// the first build, stays a mapped one, so import in an effect or a goroutine.
// An IOSurface is always copied so: no GPU of Tela's draws on macOS.
//
// Import may be called from any goroutine, the UI thread included. It waits
// for the GPU's frame being drawn, if there is one, and for nothing else: not
// for the UI thread.
//
// A GPU may be lost while the app runs (its driver was reset). Nothing has to
// be imported again for linear buffers: they are mapped from then on. The
// others show nothing any more, and Submit returns an error that wraps
// ErrTextureUnsupported for them: ask Device again, and import buffers of a
// format it has.
//
// It returns ErrTextureClosed once the component unmounted, an error that
// wraps ErrTextureUnsupported for memory that cannot be shown, which says
// what is, and another one for a Dmabuf that is wrong: a plane that ends
// beyond its descriptor's memory, a stride shorter than a row, a descriptor
// that is not a dma-buf.
func (x *Texture) Import(m TextureMemory) (*TextureBuffer, error) {
	var d Dmabuf
	switch m := m.(type) {
	case Dmabuf:
		d = m
	case *Dmabuf:
		if m == nil {
			return nil, errors.New("tela: Texture.Import of a nil *Dmabuf; pass a tela.Dmabuf")
		}
		d = *m
	case IOSurface:
		return x.importSurface(m)
	case *IOSurface:
		if m == nil {
			return nil, errors.New("tela: Texture.Import of a nil *IOSurface; pass a tela.IOSurface")
		}
		return x.importSurface(*m)
	default:
		return nil, errors.New("tela: Texture.Import(nil): pass the memory to import, such as a tela.Dmabuf or a tela.IOSurface")
	}
	if x.isClosed() {
		return nil, ErrTextureClosed
	}
	if err := d.check(); err != nil {
		return nil, err
	}
	_, opaque, _ := textureFourcc(d.Fourcc)
	b := &TextureBuffer{x: x, w: d.Width, h: d.Height, opaque: opaque}
	var err error
	if imp := x.importer(); imp != nil {
		b.content, err = imp.importTexture(d, x.stream)
		if err != nil && !errors.Is(err, ErrTextureUnsupported) {
			return nil, err
		}
	}
	if b.content == nil {
		mem, merr := mapDmabuf(d)
		switch {
		case merr == nil:
		case x.isClosed():
			return nil, ErrTextureClosed // meanwhile: the GPU goes with the app
		case err != nil && errors.Is(merr, ErrTextureUnsupported):
			return nil, err // what the device says of it
		default:
			return nil, merr
		}
		b.mem = mem
	}
	return x.addBuffer(b)
}

// addBuffer makes b a buffer of the texture, or frees it when the texture is
// closed.
func (x *Texture) addBuffer(b *TextureBuffer) (*TextureBuffer, error) {
	x.mu.Lock()
	if x.closed {
		x.mu.Unlock()
		b.free()
		return nil, ErrTextureClosed
	}
	if x.bufs == nil {
		x.bufs = map[*TextureBuffer]struct{}{}
	}
	x.bufs[b] = struct{}{}
	x.mu.Unlock()
	return b, nil
}

// reader returns what copies the frames of the buffer, nil for memory the
// device reads itself, and an error when nothing reads it any more.
func (b *TextureBuffer) reader() (textureReader, error) {
	if b.mem != nil {
		return b.mem, nil
	}
	d, ok := b.content.(deviceTexture)
	if !ok || !d.deviceGone() {
		return nil, nil
	}
	if err := d.readable(); err != nil {
		return nil, fmt.Errorf("tela: Texture.Submit: %w", err)
	}
	return d, nil
}

// image returns an image of the buffer's current pixels, which the device
// reads.
func (b *TextureBuffer) image() *gfx.Image {
	return gfx.NewExternalImage(b.w, b.h, b.opaque, b.content)
}

// Close gives back what Tela holds of the buffer's memory. A buffer whose
// frame is still held (waiting, ready or shown) is closed when that frame is
// let go, right before its Release is called. Submit then returns an error.
// The buffers of a texture are closed with it. Closing twice is fine; Close
// returns nil.
func (b *TextureBuffer) Close() error {
	b.x.mu.Lock()
	free := b.x.closeBuffer(b)
	b.x.mu.Unlock()
	if free {
		b.free()
	}
	return nil
}

// closeBuffer closes b. It reports that its memory must be freed now; when
// the texture itself closes, the release goroutine frees it. x.mu is held.
func (x *Texture) closeBuffer(b *TextureBuffer) bool {
	if b.closed {
		return false
	}
	b.closed = true
	delete(x.bufs, b)
	if b.held {
		return false // letGo queues it
	}
	if x.closed {
		x.queue(texRelease{free: b})
		return false
	}
	return true
}

// free gives the memory back, once nothing reads it.
func (b *TextureBuffer) free() {
	b.x.mu.Lock()
	done := b.freed
	b.freed = true
	b.x.mu.Unlock()
	switch {
	case done:
	case b.mem != nil:
		b.mem.close()
	case b.content != nil:
		b.content.closeTexture()
	}
}
