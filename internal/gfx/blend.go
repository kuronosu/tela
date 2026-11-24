package gfx

// BlendMode is how a layer is composited onto what is already painted. The
// values mirror CSS mix-blend-mode (Compositing and Blending Level 1, plus
// plus-lighter from Level 2). The separable and non-separable formulas
// operate on sRGB-encoded premultiplied values, like browsers (ADR-007,
// ADR-074): with Cs, Cb the unpremultiplied source and backdrop colors,
// co = cs·(1 - αb) + cb·(1 - αs) + αs·αb·B(Cb, Cs) and
// αo = αs + αb·(1 - αs); plus-lighter is min(1, s + d) on every channel.
// These modes leave the backdrop unchanged where the source is transparent
// and give the source where the backdrop is transparent.
//
// The Porter-Duff operators follow (Compositing Level 1, section 9.1; the
// values of Canvas's globalCompositeOperation that CSS does not have, ADR-319).
// On premultiplied values, with s, d the source and the backdrop and αs, αb
// their alphas, each is co = s·Fa + d·Fb. Canvas's "lighter" is PlusLighter:
// the same formula. They composite with the enclosing group only. Some
// change the backdrop where the source is transparent: see Unbounded.
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

	SrcIn   // s·αb
	SrcOut  // s·(1 - αb)
	SrcAtop // s·αb + d·(1 - αs)
	DstOver // s·(1 - αb) + d
	DstIn   // d·αs
	DstOut  // d·(1 - αs)
	DstAtop // s·(1 - αb) + d·αs
	Copy    // s
	Xor     // s·(1 - αb) + d·(1 - αs)
)

var blendNames = [...]string{
	SrcOver: "normal", Multiply: "multiply", Screen: "screen", Overlay: "overlay",
	Darken: "darken", Lighten: "lighten", ColorDodge: "color-dodge", ColorBurn: "color-burn",
	HardLight: "hard-light", SoftLight: "soft-light", Difference: "difference",
	Exclusion: "exclusion", Hue: "hue", Saturation: "saturation", ColorBlend: "color",
	Luminosity: "luminosity", PlusLighter: "plus-lighter",
	SrcIn: "source-in", SrcOut: "source-out", SrcAtop: "source-atop",
	DstOver: "destination-over", DstIn: "destination-in", DstOut: "destination-out",
	DstAtop: "destination-atop", Copy: "copy", Xor: "xor",
}

// String returns the CSS mix-blend-mode keyword ("normal" for SrcOver), or
// the globalCompositeOperation keyword of a Porter-Duff operator.
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
	return b < Hue || b >= PlusLighter
}

// Unbounded reports whether the mode changes the backdrop where the source
// is transparent (it clears it there): SrcIn, SrcOut, DstIn, DstAtop and
// Copy. A layer with such a mode affects the whole clip current at its
// PushLayer, not only the bounds of its content, and one that shows no
// content (empty, at opacity 0, under an empty mask) still clears that
// clip. The other modes leave the pixels outside the content alone.
func (b BlendMode) Unbounded() bool {
	switch b {
	case SrcIn, SrcOut, DstIn, DstAtop, Copy:
		return true
	}
	return false
}
