package gfx

// BlendMode is how a layer is composited onto what is already painted. The
// values mirror CSS mix-blend-mode (Compositing and Blending Level 1, plus
// plus-lighter from Level 2). The separable and non-separable formulas
// operate on sRGB-encoded premultiplied values, like browsers (ADR-007,
// ADR-074): with Cs, Cb the unpremultiplied source and backdrop colors,
// co = cs·(1 - αb) + cb·(1 - αs) + αs·αb·B(Cb, Cs) and
// αo = αs + αb·(1 - αs); plus-lighter is min(1, s + d) on every channel.
// Every mode leaves the backdrop unchanged where the source is transparent
// and gives the source where the backdrop is transparent.
type BlendMode uint8

const (
	// SrcOver is normal source-over compositing. It is the zero value.
	SrcOver BlendMode = iota
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
	ColorBlend // CSS "color"; named ColorBlend because Color is the type
	Luminosity
	PlusLighter
)

var blendNames = [...]string{
	SrcOver: "normal", Multiply: "multiply", Screen: "screen", Overlay: "overlay",
	Darken: "darken", Lighten: "lighten", ColorDodge: "color-dodge", ColorBurn: "color-burn",
	HardLight: "hard-light", SoftLight: "soft-light", Difference: "difference",
	Exclusion: "exclusion", Hue: "hue", Saturation: "saturation", ColorBlend: "color",
	Luminosity: "luminosity", PlusLighter: "plus-lighter",
}

// String returns the CSS mix-blend-mode keyword ("normal" for SrcOver).
func (b BlendMode) String() string {
	if int(b) < len(blendNames) {
		return blendNames[b]
	}
	return "BlendMode(?)"
}

// IsSeparable reports whether the mode operates on each channel
// independently (everything except Hue, Saturation, ColorBlend and
// Luminosity).
func (b BlendMode) IsSeparable() bool {
	return b < Hue || b == PlusLighter
}
