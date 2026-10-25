package core

import "testing"

// TestGradientColorSpacePixels checks the interpolation color space of
// gradients against Chromium (ADR-158). The expected pixels were read from
// Chromium's rendering (--force-color-profile=srgb) of the same gradients in
// a 200x20 box, at the middle of the box:
//
//	linear-gradient(to right, #fb2c36, #2b7fff)           (146, 85, 155)  sRGB, CSS's default
//	linear-gradient(to right in oklab, #fb2c36, #2b7fff)  (170, 113, 166)
//	radial-gradient(circle at 0 10px, #f00, #00f 200px)   (127, 0, 128), at x=100
//	... with "in oklab"                                   (139, 83, 162)
//
// An arbitrary gradient without "in" is sRGB; Tailwind's named utilities
// (bg-linear-to-r, bg-radial, ...) add "in oklab" and must not change.
func TestGradientColorSpacePixels(t *testing.T) {
	const srgb, oklab = "srgb", "oklab"
	tests := []struct {
		name    string
		classes string
		want    [3]uint8
	}{
		{srgb + " function", "bg-[linear-gradient(to_right,#fb2c36,#2b7fff)]", [3]uint8{146, 85, 155}},
		{oklab + " function", "bg-[linear-gradient(to_right_in_oklab,#fb2c36,#2b7fff)]", [3]uint8{170, 113, 166}},
		{srgb + " arbitrary utility", "bg-linear-[to_right,#fb2c36,#2b7fff]", [3]uint8{146, 85, 155}},
		{oklab + " arbitrary utility", "bg-linear-[to_right_in_oklab,#fb2c36,#2b7fff]", [3]uint8{170, 113, 166}},
		{oklab + " arbitrary utility, modifier", "bg-linear-[to_right,#fb2c36,#2b7fff]/oklab", [3]uint8{170, 113, 166}},
		{oklab + " named utility", "bg-linear-to-r from-red-500 to-blue-500", [3]uint8{170, 113, 166}},
		{srgb + " named utility, modifier", "bg-linear-to-r/srgb from-red-500 to-blue-500", [3]uint8{146, 85, 155}},
		{srgb + " radial", "bg-[radial-gradient(circle_at_0px_10px,#f00,#00f_200px)]", [3]uint8{127, 0, 128}},
		{oklab + " radial", "bg-[radial-gradient(circle_in_oklab_at_0px_10px,#f00,#00f_200px)]", [3]uint8{139, 83, 162}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, img := Render(div(Class("h-[20px] w-[200px] "+tt.classes)), RenderOptions{
				Config: Config{Fonts: testFonts},
				Width:  200, Height: 20, Scale: 1,
			})
			c := img.RGBAAt(100, 10)
			for i, got := range [3]uint8{c.R, c.G, c.B} {
				if d := int(got) - int(tt.want[i]); d < -3 || d > 3 {
					t.Errorf("pixel (100,10) = (%d, %d, %d), want (%d, %d, %d) +-3", c.R, c.G, c.B, tt.want[0], tt.want[1], tt.want[2])
					break
				}
			}
		})
	}
}
