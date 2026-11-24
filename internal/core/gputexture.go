//go:build telagpu

package core

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/gfx/gpu"
)

// The memory of textures imported by the GPU that draws the windows
// (ADR-315, ADR-316): the Host's textureImporter, over gpu.Importer.
//
// # Goroutines
//
// Texture.Import and TextureBuffer.Close are the application's, from any
// goroutine. They call the device where they are called: it imports and
// closes beside the UI thread's frames (gpu/import.go, "Goroutines"), and
// makes them wait for a Draw in progress at most. Nothing is handed to the UI
// thread, so nothing waits for the code of the application that runs there
// (an OnResize that waits for the goroutine that imports, an effect's
// cleanup, a Release), for a loop that ended or for one that quits. Once the
// GPU closed, an import is answered as without one.
//
// # When a frame is let go
//
// A frame the GPU draws is let go as any other (texture.go): by the paint
// that composed a list without it, on the UI thread, before that list is
// drawn. The GPU does not read it by then: every Draw that did was waited for
// (gpu.Target.Draw), and a later one reads what the list it draws names, and
// no other memory of the application's. Layers, cached targets, filtered
// backdrops, shifted pixels, the textures that frames are copied into, and
// the buffers the display server shows are the renderer's own memory, written
// by earlier Draws.
//
// A lost device is the exception: a submission whose wait failed may run
// until the device is closed. Device.Close waits for it, so a Host closes a
// lost GPU before it lets go of any frame a window may have drawn
// (Host.endLostGPU), and a buffer closed meanwhile waits for it in the device.
//
// # When the device goes away
//
// A window that falls back to the CPU rasterizer draws the images of
// imported memory by their pixels, read back through the device
// (gpu.Imported.Pixels). Once the device is lost or closed (another one may
// have taken its place), its imports draw nothing, and linear memory is
// mapped: a frame on its way to the screen is read when it is rasterized
// (gpuTexture.Pixels), and the ones submitted from then on are copied as
// those of a mapped buffer, on the goroutine that waited for them. What only
// the device could read, a tiled layout, draws nothing, and Submit says so:
// the application asks Texture.Device again and imports other buffers.

// gpuTextures is the textureImporter of a GPU.
type gpuTextures struct {
	id  gpu.Identified
	imp gpu.Importer // nil for a device that imports nothing (Windows)
	// closed: the GPU closed, or is about to.
	closed atomic.Bool

	mu   sync.Mutex
	info *TextureDevice // what textureDevice answers, nil until it was asked
}

// textures returns what imports the memory of textures into g's device, nil
// for a device that does not even say which one it is. UI thread.
func (g *GPU) textures() *gpuTextures {
	if g.tex == nil && g.dev != nil {
		if id, ok := g.dev.(gpu.Identified); ok {
			imp, _ := g.dev.(gpu.Importer)
			g.tex = &gpuTextures{id: id, imp: imp}
		}
	}
	return g.tex
}

// syncTextures makes the GPU what imports the memory of the app's textures,
// while it draws a window of the app; nothing does otherwise, so that memory
// imported from then on is mapped, not read back through a GPU that draws
// nothing.
func (h *Host) syncTextures() {
	var imp textureImporter
	if g := h.gpu.g; g != nil {
		if t := g.textures(); t != nil && slices.ContainsFunc(h.views, func(v *winView) bool { return !v.gpu.off }) {
			imp = t
		}
	}
	h.setTextureImporter(imp)
}

// gone reports that the device draws and imports nothing more.
func (x *gpuTextures) gone() bool {
	return x.closed.Load() || x.imp != nil && x.imp.Lost()
}

// textureDevice implements textureImporter.
func (x *gpuTextures) textureDevice() TextureDevice {
	d := copyTextureDevice()
	if x.gone() {
		return d
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.info == nil {
		id := x.id.DeviceID()
		d.Name, d.VendorID, d.DeviceID, d.UUID = id.Name, id.VendorID, id.DeviceID, id.UUID
		if id.HasLUID {
			d.LUID = id.LUID
		}
		if id.HasRender {
			d.RenderMajor, d.RenderMinor = id.Render.Major, id.Render.Minor
			d.RenderNode = renderNode(id.Render.Major, id.Render.Minor)
		}
		if x.imp != nil {
			own, foreign := x.imp.ImportFormats()
			d.GPU = len(own) > 0
			// What the device does not take and is mapped comes last.
			d.Formats = textureFormats(own, d.Formats)
			d.ForeignFormats = textureFormats(foreign, d.ForeignFormats)
		}
		if x.gone() {
			return copyTextureDevice() // meanwhile
		}
		x.info = new(TextureDevice)
		*x.info = d
	}
	d = *x.info
	d.Formats, d.ForeignFormats = slices.Clone(d.Formats), slices.Clone(d.ForeignFormats)
	return d
}

// textureFormats returns the pairs of a device, and those of more that are
// not among them.
func textureFormats(pairs []gpu.DmabufFormat, more []TextureFormat) []TextureFormat {
	out := make([]TextureFormat, 0, len(pairs)+len(more))
	for _, p := range pairs {
		out = append(out, TextureFormat{Fourcc: p.Fourcc, Modifier: p.Modifier})
	}
	for _, f := range more {
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// importTexture implements textureImporter.
func (x *gpuTextures) importTexture(m TextureMemory, stream uint64) (textureContent, error) {
	d, ok := m.(Dmabuf)
	if !ok || x.imp == nil || x.gone() {
		return nil, nil
	}
	desc := gpu.DmabufDescriptor{Width: d.Width, Height: d.Height, Fourcc: d.Fourcc, Modifier: d.Modifier, Stream: stream}
	for _, p := range d.Planes {
		desc.Planes = append(desc.Planes, gpu.DmabufPlane{FD: p.FD, Offset: p.Offset, Stride: p.Stride})
	}
	im, err := x.imp.Import(desc)
	switch {
	case err == nil:
	case x.gone():
		return nil, nil // meanwhile
	case errors.Is(err, gpu.ErrImportUnsupported):
		return nil, fmt.Errorf("tela: Texture.Import: %s: %w: %w", d.describe(), err, ErrTextureUnsupported)
	default:
		return nil, fmt.Errorf("tela: Texture.Import: %s: %w", d.describe(), err)
	}
	c := &gpuTexture{x: x, im: im, modifier: d.Modifier}
	c.d, c.kept = keepDmabuf(d)
	return c, nil
}

// gpuTexture is a dmabuf imported by a GPU: the content of the images that
// draw its frames.
type gpuTexture struct {
	x        *gpuTextures
	im       *gpu.Imported
	modifier uint64
	// reads counts the calls of Pixels (tests: frames the GPU draws have
	// none).
	reads atomic.Int32

	mu sync.Mutex
	// d, when kept, is the dmabuf with a descriptor of the content's own: it
	// can be mapped (mem) once the device is gone.
	d      Dmabuf
	kept   bool
	mem    mappedTexture
	closed bool
	// memErr is why the memory cannot be mapped, once that was tried.
	memErr atomic.Pointer[error]
}

var (
	_ gpu.ImportedContent = (*gpuTexture)(nil)
	_ textureContent      = (*gpuTexture)(nil)
	_ deviceTexture       = (*gpuTexture)(nil)
)

// Imported implements gpu.ImportedContent.
func (c *gpuTexture) Imported() *gpu.Imported { return c.im }

// Pixels implements gfx.External: the pixels for the CPU rasterizer, and for
// another device. The device that imported the memory reads it best, whatever
// its layout; without it the mapping does.
func (c *gpuTexture) Pixels() []byte {
	c.reads.Add(1)
	if !c.x.gone() {
		if pix := c.im.Pixels(); pix != nil {
			return pix
		}
	}
	size := c.im.Size()
	pix := make([]byte, 4*size.X*size.Y)
	if c.read(pix) != nil {
		return nil
	}
	return pix
}

// deviceGone implements deviceTexture.
func (c *gpuTexture) deviceGone() bool { return c.x.gone() }

// readable implements deviceTexture.
func (c *gpuTexture) readable() error {
	const again = "ask Texture.Device again and import buffers of a format it has"
	if !c.kept {
		return fmt.Errorf("the GPU that imported the buffer is gone, and nothing else reads its layout (modifier 0x%x): %s: %w", c.modifier, again, ErrTextureUnsupported)
	}
	if err := c.memErr.Load(); err != nil {
		return fmt.Errorf("the GPU that imported the buffer is gone, and it cannot be mapped: %s: %w: %w", again, *err, ErrTextureUnsupported)
	}
	return nil
}

// read implements deviceTexture.
func (c *gpuTexture) read(dst []byte) error {
	if err := c.readable(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("the buffer is closed")
	}
	if c.mem == nil {
		mem, err := mapDmabuf(c.d)
		if err != nil {
			c.memErr.Store(&err)
			return err
		}
		c.mem = mem
	}
	return c.mem.read(dst)
}

// closeTexture implements textureContent.
func (c *gpuTexture) closeTexture() {
	c.im.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	if c.mem != nil {
		c.mem.close()
	}
	if c.kept {
		closeDmabuf(c.d)
	}
}
