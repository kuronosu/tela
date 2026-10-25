package style

import (
	"errors"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// The interpolation color space of gradients (ADR-158): Tailwind's named
// utilities add "in oklab"; a gradient written as CSS text, an arbitrary
// bg-linear-[...] included, is sRGB unless the text says "in <space>".
func TestGradientInterpolationSpace(t *testing.T) {
	const (
		ok = gfx.InterpOklab
		sr = gfx.InterpSRGB
	)
	tests := []struct {
		classes string
		want    gfx.ColorSpace
		angle   float32
		corner  Corner
	}{
		// Named utilities: Oklab, with the modifiers.
		{"bg-linear-to-r from-red-500 to-blue-500", ok, 90, CornerNone},
		{"bg-linear-45 from-red-500 to-blue-500", ok, 45, CornerNone},
		{"bg-radial from-red-500 to-blue-500", ok, 180, CornerNone},
		{"bg-conic from-red-500 to-blue-500", ok, 180, CornerNone},
		{"bg-linear-to-r/srgb from-red-500 to-blue-500", sr, 90, CornerNone},
		{"bg-linear-to-r/oklab from-red-500 to-blue-500", ok, 90, CornerNone},

		// Arbitrary utilities: CSS's default, sRGB (what Tailwind emits).
		{"bg-linear-[to_right,#f00,#00f]", sr, 90, CornerNone},
		{"bg-linear-[25deg,#f00,#00f]", sr, 25, CornerNone},
		{"bg-linear-[#f00,#00f]", sr, 180, CornerNone},
		{"bg-linear-[to_right] from-red-500 to-blue-500", sr, 90, CornerNone},
		{"bg-radial-[at_25%_25%] from-red-500 to-blue-500", sr, 180, CornerNone},
		{"bg-radial-[circle,#f00,#00f]", sr, 180, CornerNone},
		{"bg-conic-[from_90deg,#f00,#00f]", sr, 90, CornerNone},
		// The prelude may name the space, before or after the rest.
		{"bg-linear-[to_right_in_oklab,#f00,#00f]", ok, 90, CornerNone},
		{"bg-linear-[in_oklab_to_right,#f00,#00f]", ok, 90, CornerNone},
		{"bg-linear-[in_oklab,#f00,#00f]", ok, 180, CornerNone},
		{"bg-linear-[to_bottom_right_in_oklab,#f00,#00f]", ok, 0, CornerBottomRight},
		{"bg-linear-[45deg_in_srgb,#f00,#00f]", sr, 45, CornerNone},
		{"bg-radial-[circle_in_oklab,#f00,#00f]", ok, 180, CornerNone},
		{"bg-radial-[in_oklab_circle_at_top,#f00,#00f]", ok, 180, CornerNone},
		{"bg-conic-[from_90deg_in_oklab,#f00,#00f]", ok, 90, CornerNone},
		{"bg-conic-[in_oklab,#f00,#00f]", ok, 180, CornerNone},
		// The Tela modifiers still work on the arbitrary forms.
		{"bg-linear-[to_right,#f00,#00f]/oklab", ok, 90, CornerNone},
		{"bg-linear-[to_right,#f00,#00f]/srgb", sr, 90, CornerNone},
		{"bg-linear-[to_right_in_oklab,#f00,#00f]/oklab", ok, 90, CornerNone},

		// CSS functions inside bg-[...].
		{"bg-[linear-gradient(to_right,#f00,#00f)]", sr, 90, CornerNone},
		{"bg-[linear-gradient(to_right_in_oklab,#f00,#00f)]", ok, 90, CornerNone},
		{"bg-[linear-gradient(in_srgb,#f00,#00f)]", sr, 180, CornerNone},
		{"bg-[radial-gradient(circle_in_oklab,#f00,#00f)]", ok, 180, CornerNone},
		{"bg-[radial-gradient(#f00,#00f)]", sr, 180, CornerNone},
		{"bg-[conic-gradient(from_90deg_in_srgb,#f00,#00f)]", sr, 90, CornerNone},
		{"bg-[conic-gradient(from_90deg_in_oklab,#f00,#00f)]", ok, 90, CornerNone},
		{"bg-[conic-gradient(#f00,#00f)]", sr, 180, CornerNone},
	}
	for _, tt := range tests {
		t.Run(tt.classes, func(t *testing.T) {
			s := style(t, tt.classes)
			if len(s.Backgrounds) != 1 || s.Backgrounds[0].Gradient == nil {
				t.Fatalf("Backgrounds = %+v", s.Backgrounds)
			}
			g := s.Backgrounds[0].Gradient
			if g.Interp != tt.want {
				t.Errorf("Interp = %v, want %v", g.Interp, tt.want)
			}
			if g.Kind == GradientLinear && (g.Corner != tt.corner || (tt.corner == CornerNone && g.Angle != tt.angle)) {
				t.Errorf("direction = %v deg, corner %v; want %v deg, corner %v", g.Angle, g.Corner, tt.angle, tt.corner)
			}
			if g.Kind == GradientConic && g.Angle != tt.angle && tt.angle != 180 {
				t.Errorf("Angle = %v, want %v", g.Angle, tt.angle)
			}
		})
	}
}

// Every gradient of a multi-layer background has its own space.
func TestGradientInterpolationLayers(t *testing.T) {
	s := style(t, "bg-[linear-gradient(to_right,#f00,#00f),linear-gradient(in_oklab,#f00,#00f),url(a.jpg)]")
	if len(s.Backgrounds) != 3 {
		t.Fatalf("Backgrounds = %d", len(s.Backgrounds))
	}
	eq(t, "layer 0", s.Backgrounds[0].Gradient.Interp, gfx.InterpSRGB)
	eq(t, "layer 1", s.Backgrounds[1].Gradient.Interp, gfx.InterpOklab)
	if s.Backgrounds[2].Gradient != nil || s.Backgrounds[2].URL != "a.jpg" {
		t.Errorf("layer 2 = %+v", s.Backgrounds[2])
	}
}

// A mask gradient is sRGB like any CSS gradient, and now respects an
// explicit "in oklab" instead of forcing sRGB.
func TestMaskGradientInterpolationSpace(t *testing.T) {
	s := style(t, "mask-[linear-gradient(black,transparent)]")
	if len(s.Mask.Layers) != 1 {
		t.Fatalf("Mask.Layers = %d", len(s.Mask.Layers))
	}
	eq(t, "default", s.Mask.Layers[0].Gradient.Interp, gfx.InterpSRGB)
	s = style(t, "mask-[linear-gradient(to_right_in_oklab,black,transparent)]")
	eq(t, "in oklab", s.Mask.Layers[0].Gradient.Interp, gfx.InterpOklab)
	s = style(t, "mask-[linear-gradient(in_srgb,black,transparent)]")
	eq(t, "in srgb", s.Mask.Layers[0].Gradient.Interp, gfx.InterpSRGB)
}

func TestGradientInterpolationErrors(t *testing.T) {
	tests := []struct {
		class       string
		unsupported bool
		mention     string
	}{
		{"bg-[linear-gradient(to_right_in_oklch,#f00,#00f)]", true, "in oklch"},
		{"bg-[linear-gradient(in_srgb-linear,#f00,#00f)]", true, "srgb-linear"},
		{"bg-[linear-gradient(in_hsl_longer_hue,#f00,#00f)]", true, "hsl"},
		{"bg-[radial-gradient(circle_in_display-p3,#f00,#00f)]", true, "display-p3"},
		{"bg-[conic-gradient(from_90deg_in_lab,#f00,#00f)]", true, "in srgb"},
		{"bg-linear-[to_right_in_oklch,#f00,#00f]", true, "in oklab"},
		{"bg-linear-[in_oklab_shorter_hue,#f00,#00f]", false, "polar"},
		{"bg-linear-[in,#f00,#00f]", false, "needs a color space"},
		{"bg-linear-[in_nowhere,#f00,#00f]", false, "nowhere"},
		{"bg-linear-[in_oklab_nonsense,#f00,#00f]", false, "nonsense"},
		{"bg-linear-[to_right_in_oklab,#f00,#00f]/srgb", false, "contradicts"},
		{"mask-[linear-gradient(in_oklch,black,transparent)]", true, "oklch"},
	}
	for _, tt := range tests {
		c := Compile(tt.class)
		if c.Err == nil {
			t.Errorf("%q: no error", tt.class)
			continue
		}
		var u *UnsupportedClassError
		var in *InvalidClassError
		var reason string
		switch {
		case tt.unsupported && errors.As(c.Err, &u):
			reason = u.Reason
		case !tt.unsupported && errors.As(c.Err, &in):
			reason = in.Reason
		default:
			t.Errorf("%q: Err = %v (%T)", tt.class, c.Err, c.Err)
			continue
		}
		if !strings.Contains(reason, tt.mention) {
			t.Errorf("%q: reason %q does not mention %q", tt.class, reason, tt.mention)
		}
	}
}
