//go:build telagpu && linux

package gpu

import (
	"errors"
	"fmt"
	"image"
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/vk"
)

// The Vulkan device's images exported as dmabufs (export.go, ADR-281).

// exportSupported: this build exports images.
const exportSupported = true

// exportUsage is how an exported image is used: copied into, and read
// back by the tests. It is never drawn into, which leaves the device the
// modifiers it cannot render with.
const exportUsage = vk.ImageUsageTransferDst | vk.ImageUsageTransferSrc

// bgraFormat is the Vulkan format of the DRM formats XRGB8888 and
// ARGB8888: B, G, R, A in memory. A copy into it converts (a blit).
const bgraFormat = vk.FormatB8G8R8A8Unorm

// exportVkFormat returns the Vulkan format with the bytes of a DRM format.
func exportVkFormat(fourcc uint32) vk.Format {
	if fourcc == FourccXRGB8888 || fourcc == FourccARGB8888 {
		return bgraFormat
	}
	return targetFormat
}

// vkExport is an image exported as a dmabuf.
type vkExport struct {
	d      *vkDevice // nil once closed
	img    *vk.DmabufImage
	size   image.Point
	format DmabufFormat
	planes []DmabufPlane
	// convert: the image has another byte order than a target, so pixels
	// are copied into it by a blit.
	convert bool
	// written: the image holds pixels (a first copy was made); handed: it
	// was given to whoever else reads it (the general layout and, with
	// d.foreign, the foreign queue family).
	written, handed bool
}

var _ Exporter = (*vkDevice)(nil)

// askExports asks the driver, once, for the modifiers it exports images
// with.
func (d *vkDevice) askExports() {
	if d.exportAsk {
		return
	}
	d.exportAsk = true
	if d.exports == nil {
		return
	}
	pd := d.info.Device
	d.exportMods = map[vk.Format][]vk.DmabufModifier{}
	canBlit := d.inst.GetPhysicalDeviceFormatProperties(pd, targetFormat).OptimalTilingFeatures&vk.FormatFeatureBlitSrc != 0
	for _, format := range []vk.Format{targetFormat, bgraFormat} {
		if format != targetFormat && !canBlit {
			continue
		}
		for _, m := range d.inst.DmabufModifiers(pd, format, exportUsage) {
			if format != targetFormat && m.Features&vk.FormatFeatureBlitDst == 0 {
				continue
			}
			if m.Planes == 0 || m.Planes > 4 || m.Modifier == ModifierInvalid {
				continue
			}
			d.exportMods[format] = append(d.exportMods[format], m)
		}
	}
	for _, fourcc := range exportFourccs {
		for _, m := range d.exportMods[exportVkFormat(fourcc)] {
			d.exportable = append(d.exportable, DmabufFormat{Fourcc: fourcc, Modifier: m.Modifier})
		}
	}
}

func (d *vkDevice) DRMNodes() []DRMNode {
	var out []DRMNode
	drm := d.info.DRM
	if drm.HasRender {
		out = append(out, DRMNode{Major: uint32(drm.RenderMajor), Minor: uint32(drm.RenderMinor)})
	}
	if drm.HasPrimary {
		out = append(out, DRMNode{Major: uint32(drm.PrimaryMajor), Minor: uint32(drm.PrimaryMinor)})
	}
	return out
}

func (d *vkDevice) ExportFormats() []DmabufFormat {
	if d.usable() != nil {
		return nil
	}
	d.askExports()
	return d.exportable
}

func (d *vkDevice) NewExport(width, height int, accepted []DmabufFormat) (Export, error) {
	if err := d.usable(); err != nil {
		return nil, err
	}
	if width < 1 || height < 1 || width > d.maxSize || height > d.maxSize {
		return nil, fmt.Errorf("gpu: an exported image of %dx%d: the size must be between 1 and %d", width, height, d.maxSize)
	}
	d.askExports()
	fourcc, mods, ok := ChooseExport(accepted, d.exportable)
	if !ok {
		return nil, fmt.Errorf("%w (%s exports %d pairs of format and modifier, %d are accepted)", ErrNoExport, d.info.Name, len(d.exportable), len(accepted))
	}
	format := exportVkFormat(fourcc)
	var list []vk.DmabufModifier
	for _, m := range mods {
		for _, have := range d.exportMods[format] {
			if have.Modifier == m && int(have.MaxExtent.Width) >= width && int(have.MaxExtent.Height) >= height {
				list = append(list, have)
			}
		}
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%w at %dx%d", ErrNoExport, width, height)
	}
	img, err := d.dev.CreateDmabufImage(d.mem, format, uint32(width), uint32(height), exportUsage, list)
	if err != nil {
		return nil, d.check(fmt.Errorf("gpu: %w", err))
	}
	e := &vkExport{d: d, img: img, size: image.Pt(width, height), format: DmabufFormat{Fourcc: fourcc, Modifier: img.Modifier}, convert: format != targetFormat}
	for _, p := range img.Planes {
		if p.Offset > math.MaxUint32 || p.Stride > math.MaxUint32 {
			d.dev.DestroyDmabufImage(img)
			return nil, fmt.Errorf("gpu: a plane of an exported image at the offset %d with the stride %d: a dmabuf plane is described in 32 bits", p.Offset, p.Stride)
		}
		e.planes = append(e.planes, DmabufPlane{FD: img.FD, Offset: uint32(p.Offset), Stride: uint32(p.Stride)})
	}
	d.exports[e] = struct{}{}
	return e, nil
}

func (e *vkExport) Size() image.Point     { return e.size }
func (e *vkExport) Format() DmabufFormat  { return e.format }
func (e *vkExport) Planes() []DmabufPlane { return slices.Clone(e.planes) }

// Close destroys the image. It may be called more than once, and after
// the device was closed.
func (e *vkExport) Close() {
	if e.d == nil || e.d.closed {
		return
	}
	delete(e.d.exports, e)
	e.destroy()
}

func (e *vkExport) destroy() {
	d := e.d
	if d == nil {
		return
	}
	d.dev.DestroyDmabufImage(e.img) // nothing is in flight; closes the dmabuf
	e.d = nil
}

func (e *vkExport) usable() error {
	if e.d == nil || e.d.closed {
		return errors.New("gpu: the exported image is closed")
	}
	return e.d.usable()
}

// take records the barrier that takes the image from whoever else reads
// it, into a layout: from the general layout and the foreign queue
// family once it was handed over, and with its pixels kept unless it has
// none yet.
func (e *vkExport) take(cb vk.CommandBuffer, to vk.ImageLayout) {
	d := e.d
	stage, access := layoutSync(to)
	b := transition(e.img.Image, vk.ImageLayoutUndefined, to, 0, access)
	b.SubresourceRange = vk.ImageSubresourceRange{AspectMask: vk.ImageAspectColor, LevelCount: 1, LayerCount: 1}
	if e.written {
		b.OldLayout = vk.ImageLayoutGeneral
	}
	if e.handed && d.foreign {
		b.SrcQueueFamilyIndex, b.DstQueueFamilyIndex = vk.QueueFamilyForeignEXT, d.info.GraphicsQueueFamily
	}
	d.dev.CmdPipelineBarrier(cb, vk.PipelineStageTopOfPipe, stage, 0, nil, nil, []vk.ImageMemoryBarrier{b})
}

// give records the barrier that hands the image over, from a layout: the
// general layout, which other APIs read, and the foreign queue family.
func (e *vkExport) give(cb vk.CommandBuffer, from vk.ImageLayout) {
	d := e.d
	stage, access := layoutSync(from)
	b := transition(e.img.Image, from, vk.ImageLayoutGeneral, access, 0)
	b.SubresourceRange = vk.ImageSubresourceRange{AspectMask: vk.ImageAspectColor, LevelCount: 1, LayerCount: 1}
	if d.foreign {
		b.SrcQueueFamilyIndex, b.DstQueueFamilyIndex = d.info.GraphicsQueueFamily, vk.QueueFamilyForeignEXT
	}
	d.dev.CmdPipelineBarrier(cb, stage, vk.PipelineStageBottomOfPipe, 0, nil, nil, []vk.ImageMemoryBarrier{b})
}

func (e *vkExport) CopyFrom(t Target, rects ...image.Rectangle) error {
	if err := e.usable(); err != nil {
		return err
	}
	d := e.d
	vt, ok := t.(*vkTarget)
	if !ok || vt.d != d {
		return errors.New("gpu: an exported image copies from a target of its own device")
	}
	if vt.size != e.size {
		return fmt.Errorf("gpu: an exported image of %v copies from a target of %v", e.size, vt.size)
	}
	whole := image.Rectangle{Max: e.size}
	if !e.written {
		// The image has no pixels: the whole frame, whatever changed.
		rects = []image.Rectangle{whole}
	}
	var copies []vk.ImageCopy
	var blits []vk.ImageBlit
	for _, r := range rects {
		if r = r.Intersect(whole); r.Empty() {
			continue
		}
		min := vk.Offset3D{X: int32(r.Min.X), Y: int32(r.Min.Y)}
		if e.convert {
			box := [2]vk.Offset3D{min, {X: int32(r.Max.X), Y: int32(r.Max.Y), Z: 1}}
			blits = append(blits, vk.ImageBlit{SrcSubresource: colorLayers, SrcOffsets: box, DstSubresource: colorLayers, DstOffsets: box})
		} else {
			copies = append(copies, vk.ImageCopy{SrcSubresource: colorLayers, DstSubresource: colorLayers, SrcOffset: min, DstOffset: min,
				Extent: vk.Extent3D{Width: uint32(r.Dx()), Height: uint32(r.Dy()), Depth: 1}})
		}
	}
	if len(copies)+len(blits) == 0 {
		return nil // nothing changed: the image stays as it was handed over
	}
	dev := d.dev
	err := d.run(func(cb vk.CommandBuffer) {
		d.use(cb, vt.surf, vk.ImageLayoutTransferSrcOptimal)
		e.take(cb, vk.ImageLayoutTransferDstOptimal)
		if e.convert {
			// The same pixels in another byte order: a blit of equal
			// rectangles converts each channel and scales nothing.
			dev.CmdBlitImage(cb, vt.surf.tex.img, vk.ImageLayoutTransferSrcOptimal, e.img.Image, vk.ImageLayoutTransferDstOptimal, blits, vk.FilterNearest)
		} else {
			dev.CmdCopyImage(cb, vt.surf.tex.img, vk.ImageLayoutTransferSrcOptimal, e.img.Image, vk.ImageLayoutTransferDstOptimal, copies)
		}
		e.give(cb, vk.ImageLayoutTransferDstOptimal)
	})
	if err != nil {
		return d.check(err)
	}
	e.written, e.handed = true, true
	return nil
}

func (e *vkExport) ReadPixels(dst *image.RGBA) error {
	if err := e.usable(); err != nil {
		return err
	}
	if !e.written {
		return errors.New("gpu: an exported image has no pixels before its first copy")
	}
	d := e.d
	w, h := e.size.X, e.size.Y
	if err := d.growBuffer(&d.readback, 4*w*h, vk.BufferUsageTransferDst); err != nil {
		return d.check(fmt.Errorf("gpu: %w", err))
	}
	dev := d.dev
	err := d.run(func(cb vk.CommandBuffer) {
		e.take(cb, vk.ImageLayoutTransferSrcOptimal)
		dev.CmdCopyImageToBuffer(cb, e.img.Image, vk.ImageLayoutTransferSrcOptimal, d.readback.buf, []vk.BufferImageCopy{{
			ImageSubresource: colorLayers,
			ImageExtent:      vk.Extent3D{Width: uint32(w), Height: uint32(h), Depth: 1},
		}})
		dev.CmdPipelineBarrier(cb, vk.PipelineStageTransfer, vk.PipelineStageHost, 0, []vk.MemoryBarrier{{
			SType:         vk.StructureTypeMemoryBarrier,
			SrcAccessMask: vk.AccessTransferWrite,
			DstAccessMask: vk.AccessHostRead,
		}}, nil, nil)
		e.give(cb, vk.ImageLayoutTransferSrcOptimal)
	})
	if err != nil {
		return d.check(err)
	}
	e.handed = true
	r := image.Rectangle{Max: e.size}.Intersect(dst.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		src := d.readback.bytes[4*(y*w+r.Min.X) : 4*(y*w+r.Max.X)]
		o := dst.PixOffset(r.Min.X, y)
		row := dst.Pix[o : o+len(src)]
		copy(row, src)
		if e.convert {
			for i := 0; i < len(row); i += 4 {
				row[i], row[i+2] = row[i+2], row[i]
			}
		}
	}
	return nil
}
