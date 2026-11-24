package canvas

import "github.com/kuronosu/tela/internal/gfx"

// CompositeOperation is how what is drawn meets what the canvas has:
// [Canvas.GlobalCompositeOperation].
type CompositeOperation uint8

// The operations, in the order of the HTML Standard's compositing and
// blending specification.
const (
	SourceOver CompositeOperation = iota
	SourceIn
	SourceOut
	SourceAtop
	DestinationOver
	DestinationIn
	DestinationOut
	DestinationAtop
	Lighter
	Copy
	Xor
	Multiply
	Screen
	Overlay
	Darken
	Lighten
	ColorDodge
	ColorBurn
	HardLight
	SoftLight
	Difference
	Exclusion
	Hue
	Saturation
	ColorOp
	Luminosity
)

var operations = [...]struct {
	name  string
	blend gfx.BlendMode
}{
	SourceOver:      {"source-over", gfx.SrcOver},
	SourceIn:        {"source-in", gfx.SrcIn},
	SourceOut:       {"source-out", gfx.SrcOut},
	SourceAtop:      {"source-atop", gfx.SrcAtop},
	DestinationOver: {"destination-over", gfx.DstOver},
	DestinationIn:   {"destination-in", gfx.DstIn},
	DestinationOut:  {"destination-out", gfx.DstOut},
	DestinationAtop: {"destination-atop", gfx.DstAtop},
	Lighter:         {"lighter", gfx.PlusLighter},
	Copy:            {"copy", gfx.Copy},
	Xor:             {"xor", gfx.Xor},
	Multiply:        {"multiply", gfx.Multiply},
	Screen:          {"screen", gfx.Screen},
	Overlay:         {"overlay", gfx.Overlay},
	Darken:          {"darken", gfx.Darken},
	Lighten:         {"lighten", gfx.Lighten},
	ColorDodge:      {"color-dodge", gfx.ColorDodge},
	ColorBurn:       {"color-burn", gfx.ColorBurn},
	HardLight:       {"hard-light", gfx.HardLight},
	SoftLight:       {"soft-light", gfx.SoftLight},
	Difference:      {"difference", gfx.Difference},
	Exclusion:       {"exclusion", gfx.Exclusion},
	Hue:             {"hue", gfx.Hue},
	Saturation:      {"saturation", gfx.Saturation},
	ColorOp:         {"color", gfx.ColorBlend},
	Luminosity:      {"luminosity", gfx.Luminosity},
}

// String returns the browser's name of the operation, such as
// "destination-out".
func (o CompositeOperation) String() string {
	if int(o) < len(operations) {
		return operations[o].name
	}
	return "CompositeOperation(?)"
}

// blend returns the blend mode of o: source-over for an unknown one.
func (o CompositeOperation) blend() gfx.BlendMode {
	if int(o) < len(operations) {
		return operations[o].blend
	}
	return gfx.SrcOver
}
