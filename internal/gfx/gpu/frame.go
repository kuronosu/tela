package gpu

import (
	"encoding/binary"
	"image"
	"math"
	"slices"
)

// A Frame is what the Encoder makes of a display list and what a Device
// executes: passes over targets, each a list of batches of instances, and
// the data the instances refer to. It names no graphics API. Its slices
// belong to the Encoder that made it and are valid until that Encoder
// encodes again.
//
// A frame also carries the changes to the textures its Encoder keeps on
// the device from frame to frame (see "Resources" in the package
// documentation): the pixels new in the atlases, the images to create and
// those to release. A device mirrors one Encoder's resources, so the frames
// of an Encoder must be drawn in the order they were encoded; Res lets the
// device tell when they were not.
type Frame struct {
	// Size is the size in device pixels of the target the frame was
	// encoded for.
	Size image.Point
	// Passes are executed in order.
	Passes []Pass
	// Instances holds the instances of every batch, InstanceSize bytes
	// each, in the layout the shaders read (see the Slot constants).
	Instances []byte
	// Ramps holds the gradient ramps: RampWidth premultiplied RGBA8 colors
	// (4*RampWidth bytes) per row. An instance names its first row.
	Ramps []byte
	// RampSegments holds RampSegmentSize float32 values per row of Ramps:
	// the offset of the segment's first stop and the factor from offsets
	// to ramp entries (the entry of t is int((t - offset)*factor + 0.5)).
	RampSegments []float32
	// Res identifies the resources the frame draws with and where it stands
	// in their history.
	Res ResourceSync
	// Atlases describes the atlases as the frame needs them: the device
	// gives each that many pages before it draws, keeping the pixels of
	// the pages it had.
	Atlases [NumAtlases]AtlasInfo
	// Data holds the pixels of the uploads of every pass.
	Data []byte
	// Images are the image textures the frame's batches use that the device
	// does not hold yet: it creates them before it draws.
	Images []ImageUpload
	// Releases are the image textures the device may destroy once the frame
	// is drawn (their IDs).
	Releases []uint64
	// Unsupported names what the display list holds that the renderer does
	// not draw yet, each once, in the order met: command names such as
	// "FillPath", sometimes with the case in parentheses. Such commands
	// are left out of the frame, with everything inside them when they
	// open a block; a frame with any is not the list's picture.
	Unsupported []string
}

// ResourceSync places a frame in the history of its Encoder's resources.
type ResourceSync struct {
	// ID names the resources: it changes when the Encoder starts over
	// (Encoder.Trim, a frame lost).
	ID uint64
	// Base is the number of changes the device must have applied before
	// this frame, and Next the number after it (Base, or Base+1 when the
	// frame changes the resources). A frame with Base 0 depends on nothing
	// the device holds: everything it draws with, it uploads.
	Base, Next uint64
}

// The atlases: array textures of square pages in which the Encoder packs
// small pictures that many instances read.
const (
	// AtlasCoverage holds 8-bit coverage, one byte per pixel: glyph masks,
	// filled paths, strokes and clip masks.
	AtlasCoverage = iota
	// AtlasColor holds premultiplied RGBA8: the sprites of color glyphs.
	AtlasColor
	// NumAtlases is the number of atlases.
	NumAtlases
)

// AtlasInfo is the size of an atlas.
type AtlasInfo struct {
	// PageSize is the width and height of a page in pixels.
	PageSize int
	// Pages is the number of pages; 0 when the frame uses none.
	Pages int
}

// An Upload copies pixels into an atlas page.
type Upload struct {
	// Atlas and Page select the page.
	Atlas, Page int
	// Rect is the rectangle of the page that is replaced.
	Rect image.Rectangle
	// Offset is where the pixels start in Frame.Data, row by row without
	// padding: Rect.Dx() bytes per row in the coverage atlas, four times
	// that in the color atlas. It is a multiple of 4.
	Offset int
}

// An ImageUpload creates the texture of a gfx.Image.
type ImageUpload struct {
	// ID is the image's ID, which batches name.
	ID uint64
	// Width and Height are the size in pixels, at least 1.
	Width, Height int
	// Pix holds the premultiplied RGBA8 pixels, 4*Width bytes per row. It
	// is the image's own memory.
	Pix []byte
}

// A Pass draws into one target.
type Pass struct {
	// Target is the surface drawn into: 0 is the destination. (Layers will
	// add offscreen targets.)
	Target int
	// Clear, when not nil, is done first: the pixels of Rect are replaced
	// by Color, premultiplied RGBA8.
	Clear *Clear
	// Uploads are applied before the batches are drawn, and after the
	// batches of the passes before: a pass may reuse the place of pixels
	// an earlier pass of the frame drew with.
	Uploads []Upload
	// Batches are drawn in order.
	Batches []Batch
}

// Clear replaces the pixels of a rectangle.
type Clear struct {
	Rect  image.Rectangle
	Color [4]uint8
}

// A Batch is a run of consecutive instances drawn by one pipeline under
// one scissor, with one image texture: one draw call.
type Batch struct {
	Pipeline Pipeline
	// Scissor is the device rectangle that may be written; it lies inside
	// the target and is not empty.
	Scissor image.Rectangle
	// First and Count select the instances.
	First, Count int
	// Image is the ID of the image texture the instances that paint an
	// image read (they all read the same one), or 0.
	Image uint64
}

// Pipeline names one of the fixed pipelines (ADR-282).
type Pipeline uint8

// PipelineQuad draws rectangles, rounded rectangles and their shadows,
// coverage from the atlas, images and sprites, with a solid or gradient
// paint, source-over.
const PipelineQuad Pipeline = iota

// The layout of an instance of PipelineQuad: InstanceSlots slots of four
// 32-bit values, little-endian, which is the std430 layout of the struct
// Instance of shaders/quad.frag. A slot holds float32 values unless noted.
//
// Coordinates are device pixels relative to the instance's origin
// (SlotOrigin), a whole pixel near its quad, so that the shader's float32
// arithmetic works on small numbers wherever the instance is.
const (
	// SlotBounds is the quad: x0, y0, x1, y1 in device pixels, whole
	// numbers inside the target. The shader runs for the pixels inside it
	// and the batch's scissor.
	SlotBounds = 0
	// SlotHead holds four uint32: the flags (Flag constants), the first
	// ramp row, and the colors before the first and from the last stop of
	// a gradient as premultiplied RGBA8 (R in the low byte); a solid paint
	// is the first color, and a sprite's alpha is the first color's.
	SlotHead = 1
	// SlotPaint are four slots of paint geometry. For a gradient: with
	// (X, Y) the pixel center, p = a*X + b*Y + c from slot 0 and q likewise
	// from slot 1 (see raster/gradient.go); slot 0 ends with a conic
	// gradient's start angle in turns. Slot 2 is a focal gradient's d.x,
	// d.y, dr and r0, and slot 3 its quadratic coefficient a, then the
	// offsets of the first and the last stop. For an image (see
	// raster/image.go): the image coordinates u = a*X + b*Y + c from slot 0
	// and v from slot 1, and in slot 2 the range that u - 0.5 and v - 0.5
	// are clamped to (bilinear) or the range of the pixel indices
	// (nearest): u low, u high, v low, v high.
	SlotPaint = 2
	// SlotShape is the shape filled, or the box of a shadow (ShapeSlots
	// slots).
	SlotShape = 6
	// SlotClip is the shape whose coverage multiplies the instance's, when
	// the flags say so.
	SlotClip = 11
	// SlotBlur is the shape a shadow blurs.
	SlotBlur = 16
	// SlotSigma is a shadow's standard deviations along x and y, and the
	// size of a pixel along them, in the frame of its Gaussian.
	SlotSigma = 21
	// SlotOrigin holds the origin of the instance's coordinates, x and y
	// (the top-left corner of the quad, before a clip mask split it), then
	// the atlas page of the instance's own pixels and the page of its clip
	// mask, as whole numbers.
	SlotOrigin = 22
	// SlotTex holds four int32: what to add to a device pixel's x and y to
	// get the texel of the instance's own pixels in their atlas page
	// (FlagCoverAtlas: the coverage atlas; PaintSprite: the color atlas),
	// and the same for the texel of the clip mask in the coverage atlas
	// (FlagMask).
	SlotTex = 23

	// InstanceSlots is the number of slots of an instance.
	InstanceSlots = 24
	// InstanceSize is the size of an instance in bytes.
	InstanceSize = 16 * InstanceSlots
)

// A shape is ShapeSlots slots: the rectangle (x0, y0, x1, y1), the x radii
// and the y radii of the corners (top-left, top-right, bottom-right,
// bottom-left), and for a local shape the map from a pixel center to the
// shape's space, local = (a*X + c*Y + e, b*X + d*Y + f): a slot a, b, c, d
// and a slot e, f.
//
// A device shape is in device pixels, snapped to 1/256 pixel like the
// rasterizer's, and its coverage is the exact area. A local shape is in
// the space of its command, under a transform that rotates or skews, and
// its coverage comes from the distance to its boundary.
const ShapeSlots = 5

// The flags of an instance (the first value of SlotHead).
const (
	// FlagKindMask selects where the coverage comes from: 0 fills the
	// shape, FlagOuterShadow and FlagInsetShadow paint a shadow outside or
	// inside it, FlagCoverAtlas reads it from the coverage atlas (SlotTex).
	FlagKindMask    = 3
	FlagOuterShadow = 1
	FlagInsetShadow = 2
	FlagCoverAtlas  = 3
	// FlagShapeLocal says the shape is a local shape.
	FlagShapeLocal = 1 << 2
	// FlagClipShift is where the clip's form is: 0 none, ClipDevice or
	// ClipLocal.
	FlagClipShift = 3
	ClipDevice    = 1
	ClipLocal     = 2
	// FlagPaintShift is where the paint's kind is (Paint constants).
	FlagPaintShift = 5
	// FlagSpreadShift is where the gfx.SpreadMode of a gradient is.
	FlagSpreadShift = 8
	// FlagBlurShift is where the way a shadow is evaluated is (Blur
	// constants).
	FlagBlurShift = 10
	// FlagMask says the coverage is multiplied by a clip mask read from
	// the coverage atlas (SlotTex).
	FlagMask = 1 << 12
	// FlagSegmentsShift is where the number of ramp rows of a gradient is
	// (16 bits).
	FlagSegmentsShift = 16
)

// The paint kinds.
const (
	PaintSolid = iota
	PaintLinear
	PaintRadial
	PaintConic
	PaintFocal
	// PaintImageLinear and PaintImageNearest sample the batch's image
	// texture, with the rasterizer's bilinear filter or its nearest pixel.
	PaintImageLinear
	PaintImageNearest
	// PaintSprite copies the pixels of the color atlas (SlotTex), faded by
	// the alpha of the first color.
	PaintSprite
)

// How a shadow's blurred shape is evaluated.
const (
	// BlurAnalytic is the closed form: SlotBlur is the shape in the frame
	// of the Gaussian, and its map takes a pixel center to that frame.
	BlurAnalytic = iota
	// BlurHardDevice and BlurHardLocal are a shadow too sharp to blur:
	// the coverage of SlotBlur, a device or a local shape.
	BlurHardDevice
	BlurHardLocal
	// BlurEmpty is an inset shadow whose inner shape is empty: the whole
	// box is shadowed.
	BlurEmpty
)

const (
	// RampWidth is the number of colors of a ramp row (raster.RampSize).
	RampWidth = 256
	// RampSegmentSize is the number of float32 values per ramp row in
	// Frame.RampSegments (the last two are unused).
	RampSegmentSize = 4
)

// maxSegments is the largest number of ramp rows one gradient may use.
const maxSegments = 1<<16 - 1

// instance is one instance being written. Until its flags are set, the
// first value of SlotHead says whether a value written with floats was not
// finite.
type instance [InstanceSlots][4]uint32

func (in *instance) floats(slot int, v0, v1, v2, v3 float64) {
	b := [4]uint32{f32bits(v0), f32bits(v1), f32bits(v2), f32bits(v3)}
	in[slot] = b
	// An exponent of all ones: an infinity or a NaN.
	const exp = 0x7f800000
	if b[0]&exp == exp || b[1]&exp == exp || b[2]&exp == exp || b[3]&exp == exp {
		in[SlotHead][0] = 1
	}
}

// finite reports whether every value written with floats was finite.
func (in *instance) finite() bool { return in[SlotHead][0] == 0 }

// f32bits rounds v to float32. Values beyond its range become infinities,
// which no shape or paint that reaches an instance has.
func f32bits(v float64) uint32 { return math.Float32bits(float32(v)) }

// appendTo appends the instance's bytes.
func (in *instance) appendTo(b []byte) []byte {
	n := len(b)
	b = slices.Grow(b, InstanceSize)[:n+InstanceSize]
	out := b[n : n+InstanceSize : n+InstanceSize]
	for s := range in {
		o := out[16*s : 16*s+16 : 16*s+16]
		binary.LittleEndian.PutUint32(o[0:4], in[s][0])
		binary.LittleEndian.PutUint32(o[4:8], in[s][1])
		binary.LittleEndian.PutUint32(o[8:12], in[s][2])
		binary.LittleEndian.PutUint32(o[12:16], in[s][3])
	}
	return b
}

// packRGBA8 packs a premultiplied color the way the shader unpacks it.
func packRGBA8(c [4]uint8) uint32 {
	return uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16 | uint32(c[3])<<24
}
