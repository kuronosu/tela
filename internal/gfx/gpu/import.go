package gpu

import (
	"errors"
	"image"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/gfx"
)

// Drawing an application's GPU memory (ADR-315, ADR-316): dmabufs an
// application rendered into with its own graphics API, imported by the
// device and drawn as images, without a copy through the CPU.
//
// A buffer is imported once (Importer.Import) and drawn many times: each
// frame the application renders into it is a new gfx.Image whose content
// (gfx.External) is the *Imported. The Encoder draws such an image of its
// own device from the imported memory: no pixels are read, nothing is
// uploaded and the memory does not count in the Encoder's budgets. An image
// of another device's *Imported, or of a closed one, is an ordinary image
// whose pixels come from Pix.
//
// # Who may touch the buffer when
//
// The application has finished drawing a frame before it reaches the
// Encoder (its fence was waited for, off the UI thread). Every frame the
// device draws from the buffer (Target.Draw) takes it from the foreign
// queue family (VK_QUEUE_FAMILY_FOREIGN_EXT, or EXTERNAL without the
// extension) in the general layout, reads it, and gives it back the same
// way, in one submission that is waited for. So:
//
//   - once Target.Draw has returned, the GPU does not read the buffer, and
//     nothing in flight will: the application may be given it back. The one
//     exception is a Draw that returns ErrDeviceLost, whose submission may
//     still run until Device.Close has returned;
//   - between two Draws the buffer must keep its pixels, as any image does,
//     for as long as an image of it can be drawn.
//
// Two ways lead a buffer's pixels to the shaders. A layout the device
// samples is an image over the memory itself. A linear one that the device
// cannot sample as it is (NVIDIA rounds a stride up to its own pitch, and
// ignores an offset), or whose memory is behind the bus of a discrete GPU,
// is a buffer over the memory, copied on the GPU into a texture of Tela's
// once per frame: that texture is an image texture like those of uploaded
// pixels, with their budget and their reuse (a frame of a stream takes the
// texture of the one before it).
//
// # Goroutines
//
// A Device is used by one goroutine at a time, but an application imports and
// closes buffers where it likes: Import, ImportFormats, Lost, Imported.Close
// and Imported.Pixels may be called from any goroutine, also while the
// device's own draws. Import and Close wait for the Draw in progress, and for
// nothing else.
//
// # Formats
//
// 32-bit RGB in sRGB, premultiplied, first row at the top: the fourccs of
// FourccXRGB8888 and the others. An X format's fourth byte is not read: its
// alpha is 1.
//
// Only the Vulkan device on Linux implements Importer.

// ErrImportUnsupported is wrapped by the errors of Importer.Import for a
// dmabuf that is what its descriptor says, as far as can be told, and that
// the device does not take: its format, its modifier, its size, rows it
// cannot read, memory it cannot reach. A linear one can be read by the CPU.
// An error without it is a wrong descriptor, or a device that is lost or
// closed.
var ErrImportUnsupported = errors.New("the device does not take it")

// DmabufDescriptor describes an image in a dmabuf, as an application hands
// it over.
type DmabufDescriptor struct {
	// Width and Height are the size in pixels.
	Width, Height int
	// Fourcc is the DRM format and Modifier the DRM format modifier, which
	// must be one of ImportFormats' pairs: an implicit layout
	// (ModifierInvalid) cannot be imported.
	Fourcc   uint32
	Modifier uint64
	// Planes are the memory planes, as many as the modifier has. They are
	// all in one dmabuf (their descriptors may be duplicates of each
	// other). The descriptors stay the caller's, who may close them after
	// the import.
	Planes []DmabufPlane
	// Stream names the stream the buffer's frames belong to, as
	// gfx.NewStreamImage's, among the images of which a new one replaces
	// the ones before it; 0 for a stream of the buffer's own. Only a buffer
	// whose frames are copied on the GPU uses it.
	Stream uint64
}

// DeviceID says which device a Device is, to an application that renders
// on the same one with another API.
type DeviceID struct {
	// Name is the device's name, VendorID and DeviceID its PCI IDs.
	Name               string
	VendorID, DeviceID uint32
	// UUID and DriverUUID are Vulkan's deviceUUID and driverUUID, which
	// other APIs know too (GL_DEVICE_UUID_EXT).
	UUID, DriverUUID [16]byte
	// LUID is the device's locally unique identifier, when HasLUID
	// (Windows: the adapter's).
	LUID    [8]byte
	HasLUID bool
	// Render is the DRM render node (/dev/dri/renderDN), when HasRender
	// (Linux).
	Render    DRMNode
	HasRender bool
}

// Identified is a Device that says which device it is. A caller asks a
// Device for it with a type assertion.
type Identified interface {
	DeviceID() DeviceID
}

// An Importer is a Device that draws dmabufs. A caller asks a Device for it
// with a type assertion. Its methods may be called from any goroutine.
type Importer interface {
	Identified
	// Lost reports that the device is lost (ErrDeviceLost): it draws and
	// imports nothing more, and until Close has returned it may still read
	// what it was drawing.
	Lost() bool
	// ImportFormats returns the pairs of format and modifier Import takes:
	// own for memory of the same device, the layouts it draws fastest
	// first, and foreign for memory of another one, which is linear and
	// must be memory the other device can reach (system memory; not the
	// video memory NVIDIA exports). Both are empty when the device imports
	// nothing. The slices are shared: do not modify them.
	ImportFormats() (own, foreign []DmabufFormat)
	// Import makes an image of a dmabuf. Whatever a descriptor says, a
	// wrong one is an error: a size, an offset or a stride that does not
	// fit the dmabuf, planes the modifier does not have, a descriptor that
	// is no dmabuf. So is, with ErrImportUnsupported, memory the device
	// cannot reach and a layout the device or the driver does not take.
	Import(desc DmabufDescriptor) (*Imported, error)
}

// Imported is a dmabuf imported by a device: the memory of an application's
// frames, and the gfx.External of the images that show them.
type Imported struct {
	id     uint64 // importedID and a number: what batches name
	dev    Device
	size   image.Point
	format DmabufFormat
	stream uint64
	// copied: the device does not sample the memory; a frame's pixels are
	// copied into a texture, as an image's are uploaded.
	copied bool
	mem    importedMemory
}

// importedMemory is the device's half of an Imported.
type importedMemory interface {
	// open reports whether the memory was not closed, by itself or with
	// its device.
	open() bool
	pixels() []byte
	close()
}

// ImportedContent is a gfx.External the Encoder draws from imported memory:
// *Imported itself, or a caller's type around one, whose Pixels may answer
// in another way when the memory cannot be read through the device.
type ImportedContent interface {
	gfx.External
	// Imported returns the memory of the content; nil for none.
	Imported() *Imported
}

var _ ImportedContent = (*Imported)(nil)

// importedID marks the numbers of imported memory in Batch.Image, which are
// not those of gfx.Images.
const importedID = 1 << 63

var nextImported atomic.Uint64

// newImported returns the Imported of a device's memory.
func newImported(dev Device, desc *DmabufDescriptor, copied bool, mem importedMemory) *Imported {
	m := &Imported{
		id:     importedID | nextImported.Add(1),
		dev:    dev,
		size:   image.Pt(desc.Width, desc.Height),
		format: DmabufFormat{Fourcc: desc.Fourcc, Modifier: desc.Modifier},
		stream: desc.Stream,
		copied: copied,
		mem:    mem,
	}
	if m.stream == 0 {
		m.stream = m.id
	}
	return m
}

// Size returns the size in pixels.
func (m *Imported) Size() image.Point { return m.size }

// Format returns the pair of format and modifier.
func (m *Imported) Format() DmabufFormat { return m.format }

// Opaque reports that every pixel is opaque, because the format has no
// alpha.
func (m *Imported) Opaque() bool { return m.format.opaque() }

// Copied reports that a frame's pixels are copied on the GPU before they
// are drawn, because the device cannot sample the memory as it lies, or
// not well.
func (m *Imported) Copied() bool { return m.copied }

// Frame returns the content of an image that shows what the buffer holds
// now (gfx.NewExternalImage, with the size of the buffer): it is the same
// for every frame, and each frame is an image of its own, with its own ID.
func (m *Imported) Frame() gfx.External { return m }

// Imported returns m.
func (m *Imported) Imported() *Imported { return m }

// Pixels reads the buffer's pixels back through the GPU, as premultiplied
// RGBA8: what the CPU rasterizer draws when a window of the device falls
// back to it. It waits for the GPU. It may be called by several goroutines
// at a time, as the rasterizer's tiles do; it is nil when the memory or the
// device is closed, or the device is lost.
func (m *Imported) Pixels() []byte { return m.mem.pixels() }

// Close gives up the memory: the device forgets the buffer, whose
// descriptors were never its own. It returns once no submitted work of the
// GPU reads it; on a lost device, whose work may still run, the memory is
// held until the device is closed. Images of it are then ordinary images
// without pixels. Close may be called more than once, and after the device
// was closed.
func (m *Imported) Close() { m.mem.close() }

// opaque reports whether a format has no alpha.
func (f DmabufFormat) opaque() bool {
	return f.Fourcc == FourccXRGB8888 || f.Fourcc == FourccXBGR8888
}

// imported returns the memory an image is drawn from, if the Encoder's
// device has imported it; nil for an ordinary image.
func (e *Encoder) imported(img *gfx.Image) *Imported {
	c, ok := img.External().(ImportedContent)
	if !ok {
		return nil
	}
	m := c.Imported()
	if m == nil || m.dev != e.dev || m.size != image.Pt(img.Width(), img.Height()) || !m.mem.open() {
		return nil
	}
	return m
}
