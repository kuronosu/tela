package svg

import (
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// colorVal is a parsed <color>: a color, or the currentColor keyword.
type colorVal struct {
	c       gfx.Color
	current bool
}

// parseColor parses a CSS Color 4 color as SVG uses it: a named color,
// transparent, currentColor, #rgb, #rgba, #rrggbb, #rrggbbaa, rgb(),
// rgba(), hsl() and hsla() (legacy comma syntax and modern space syntax
// with an optional "/ alpha"). Keywords and function names are
// case-insensitive.
func parseColor(s string) (colorVal, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return colorVal{}, false
	}
	if s[0] == '#' {
		c, err := gfx.ParseHex(s)
		return colorVal{c: c}, err == nil
	}
	if p := strings.IndexByte(s, '('); p > 0 {
		name := strings.ToLower(strings.TrimSpace(s[:p]))
		if !strings.HasSuffix(s, ")") {
			return colorVal{}, false
		}
		args := s[p+1 : len(s)-1]
		switch name {
		case "rgb", "rgba":
			c, ok := parseRGB(args)
			return colorVal{c: c}, ok
		case "hsl", "hsla":
			c, ok := parseHSL(args)
			return colorVal{c: c}, ok
		}
		return colorVal{}, false
	}
	low := strings.ToLower(s)
	switch low {
	case "currentcolor":
		return colorVal{current: true}, true
	case "transparent":
		return colorVal{}, true
	}
	if v, ok := namedColors[low]; ok {
		return colorVal{c: gfx.RGB8(uint8(v>>16), uint8(v>>8), uint8(v))}, true
	}
	return colorVal{}, false
}

// colorArgs splits the arguments of a color function into its components
// and the optional alpha. legacy reports the comma syntax.
func colorArgs(s string) (comps []string, alpha string, legacy, ok bool) {
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
			if parts[i] == "" {
				return nil, "", false, false
			}
		}
		switch len(parts) {
		case 3:
			return parts, "", true, true
		case 4:
			return parts[:3], parts[3], true, true
		}
		return nil, "", false, false
	}
	main, a, slash := strings.Cut(s, "/")
	comps = strings.Fields(main)
	if len(comps) != 3 {
		return nil, "", false, false
	}
	if slash {
		a = strings.TrimSpace(a)
		if a == "" || strings.ContainsAny(a, " \t\n") {
			return nil, "", false, false
		}
	}
	return comps, a, false, true
}

// component parses a number or percentage; pct reports which. "none" (the
// modern syntax only) is 0.
func component(s string, legacy bool) (v float64, pct, ok bool) {
	if !legacy && strings.EqualFold(s, "none") {
		return 0, false, true
	}
	if strings.HasSuffix(s, "%") {
		v, ok = parseNumber(s[:len(s)-1])
		return v, true, ok
	}
	v, ok = parseNumber(s)
	return v, false, ok
}

func parseAlphaArg(s string, legacy bool) (float32, bool) {
	if s == "" {
		return 1, true
	}
	if !legacy && strings.EqualFold(s, "none") {
		return 0, true
	}
	return parseAlpha(s)
}

func parseRGB(s string) (gfx.Color, bool) {
	comps, a, legacy, ok := colorArgs(s)
	if !ok {
		return gfx.Color{}, false
	}
	var ch [3]float32
	var pcts int
	for i, c := range comps {
		v, pct, ok := component(c, legacy)
		if !ok {
			return gfx.Color{}, false
		}
		if pct {
			pcts++
			v = v * 255 / 100
		}
		ch[i] = float32(min(max(v, 0), 255) / 255)
	}
	if legacy && pcts != 0 && pcts != 3 { // legacy rgb() cannot mix
		return gfx.Color{}, false
	}
	alpha, ok := parseAlphaArg(a, legacy)
	if !ok {
		return gfx.Color{}, false
	}
	return gfx.Color{R: ch[0], G: ch[1], B: ch[2], A: alpha}, true
}

func parseHSL(s string) (gfx.Color, bool) {
	comps, a, legacy, ok := colorArgs(s)
	if !ok {
		return gfx.Color{}, false
	}
	h, ok := parseHue(comps[0], legacy)
	if !ok {
		return gfx.Color{}, false
	}
	var sl [2]float64
	for i, c := range comps[1:] {
		v, pct, ok := component(c, legacy)
		if !ok || (legacy && !pct) {
			return gfx.Color{}, false
		}
		sl[i] = min(max(v, 0), 100) / 100
	}
	alpha, ok := parseAlphaArg(a, legacy)
	if !ok {
		return gfx.Color{}, false
	}
	r, g, b := hslToRGB(h, sl[0], sl[1])
	return gfx.Color{R: float32(r), G: float32(g), B: float32(b), A: alpha}, true
}

// parseHue parses a hue in degrees (unitless) or with deg, grad, rad or
// turn, and returns it in sextants of the color wheel, in [0, 6).
func parseHue(s string, legacy bool) (float64, bool) {
	if !legacy && strings.EqualFold(s, "none") {
		return 0, true
	}
	ls := strings.ToLower(s)
	scale := 1.0
	for _, u := range []struct {
		suffix string
		deg    float64
	}{{"deg", 1}, {"grad", 0.9}, {"rad", 180 / math.Pi}, {"turn", 360}} {
		if strings.HasSuffix(ls, u.suffix) {
			ls, scale = ls[:len(ls)-len(u.suffix)], u.deg
			break
		}
	}
	v, ok := parseNumber(ls)
	if !ok {
		return 0, false
	}
	deg := math.Mod(float64(v*scale), 360)
	if deg < 0 {
		deg += 360
	}
	return deg / 60, true
}

// hslToRGB converts HSL (hue in sextants, saturation and lightness in
// [0, 1]) to sRGB with the algorithm of CSS Color 4, fusion-safe.
func hslToRGB(h, s, l float64) (r, g, b float64) {
	var t2 float64
	if l <= 0.5 {
		t2 = float64(l * (s + 1))
	} else {
		t2 = l + s - float64(l*s)
	}
	t1 := float64(l*2) - t2
	return hueToRGB(t1, t2, h+2), hueToRGB(t1, t2, h), hueToRGB(t1, t2, h-2)
}

func hueToRGB(t1, t2, h float64) float64 {
	if h < 0 {
		h += 6
	}
	if h >= 6 {
		h -= 6
	}
	switch {
	case h < 1:
		return float64((t2-t1)*h) + t1
	case h < 3:
		return t2
	case h < 4:
		return float64((t2-t1)*(4-h)) + t1
	}
	return t1
}

// namedColors holds the 148 CSS named colors (CSS Color 4, section 6.1)
// as 0xRRGGBB.
var namedColors = map[string]uint32{
	"aliceblue": 0xf0f8ff, "antiquewhite": 0xfaebd7, "aqua": 0x00ffff, "aquamarine": 0x7fffd4,
	"azure": 0xf0ffff, "beige": 0xf5f5dc, "bisque": 0xffe4c4, "black": 0x000000,
	"blanchedalmond": 0xffebcd, "blue": 0x0000ff, "blueviolet": 0x8a2be2, "brown": 0xa52a2a,
	"burlywood": 0xdeb887, "cadetblue": 0x5f9ea0, "chartreuse": 0x7fff00, "chocolate": 0xd2691e,
	"coral": 0xff7f50, "cornflowerblue": 0x6495ed, "cornsilk": 0xfff8dc, "crimson": 0xdc143c,
	"cyan": 0x00ffff, "darkblue": 0x00008b, "darkcyan": 0x008b8b, "darkgoldenrod": 0xb8860b,
	"darkgray": 0xa9a9a9, "darkgreen": 0x006400, "darkgrey": 0xa9a9a9, "darkkhaki": 0xbdb76b,
	"darkmagenta": 0x8b008b, "darkolivegreen": 0x556b2f, "darkorange": 0xff8c00, "darkorchid": 0x9932cc,
	"darkred": 0x8b0000, "darksalmon": 0xe9967a, "darkseagreen": 0x8fbc8f, "darkslateblue": 0x483d8b,
	"darkslategray": 0x2f4f4f, "darkslategrey": 0x2f4f4f, "darkturquoise": 0x00ced1, "darkviolet": 0x9400d3,
	"deeppink": 0xff1493, "deepskyblue": 0x00bfff, "dimgray": 0x696969, "dimgrey": 0x696969,
	"dodgerblue": 0x1e90ff, "firebrick": 0xb22222, "floralwhite": 0xfffaf0, "forestgreen": 0x228b22,
	"fuchsia": 0xff00ff, "gainsboro": 0xdcdcdc, "ghostwhite": 0xf8f8ff, "gold": 0xffd700,
	"goldenrod": 0xdaa520, "gray": 0x808080, "green": 0x008000, "greenyellow": 0xadff2f,
	"grey": 0x808080, "honeydew": 0xf0fff0, "hotpink": 0xff69b4, "indianred": 0xcd5c5c,
	"indigo": 0x4b0082, "ivory": 0xfffff0, "khaki": 0xf0e68c, "lavender": 0xe6e6fa,
	"lavenderblush": 0xfff0f5, "lawngreen": 0x7cfc00, "lemonchiffon": 0xfffacd, "lightblue": 0xadd8e6,
	"lightcoral": 0xf08080, "lightcyan": 0xe0ffff, "lightgoldenrodyellow": 0xfafad2, "lightgray": 0xd3d3d3,
	"lightgreen": 0x90ee90, "lightgrey": 0xd3d3d3, "lightpink": 0xffb6c1, "lightsalmon": 0xffa07a,
	"lightseagreen": 0x20b2aa, "lightskyblue": 0x87cefa, "lightslategray": 0x778899, "lightslategrey": 0x778899,
	"lightsteelblue": 0xb0c4de, "lightyellow": 0xffffe0, "lime": 0x00ff00, "limegreen": 0x32cd32,
	"linen": 0xfaf0e6, "magenta": 0xff00ff, "maroon": 0x800000, "mediumaquamarine": 0x66cdaa,
	"mediumblue": 0x0000cd, "mediumorchid": 0xba55d3, "mediumpurple": 0x9370db, "mediumseagreen": 0x3cb371,
	"mediumslateblue": 0x7b68ee, "mediumspringgreen": 0x00fa9a, "mediumturquoise": 0x48d1cc, "mediumvioletred": 0xc71585,
	"midnightblue": 0x191970, "mintcream": 0xf5fffa, "mistyrose": 0xffe4e1, "moccasin": 0xffe4b5,
	"navajowhite": 0xffdead, "navy": 0x000080, "oldlace": 0xfdf5e6, "olive": 0x808000,
	"olivedrab": 0x6b8e23, "orange": 0xffa500, "orangered": 0xff4500, "orchid": 0xda70d6,
	"palegoldenrod": 0xeee8aa, "palegreen": 0x98fb98, "paleturquoise": 0xafeeee, "palevioletred": 0xdb7093,
	"papayawhip": 0xffefd5, "peachpuff": 0xffdab9, "peru": 0xcd853f, "pink": 0xffc0cb,
	"plum": 0xdda0dd, "powderblue": 0xb0e0e6, "purple": 0x800080, "rebeccapurple": 0x663399,
	"red": 0xff0000, "rosybrown": 0xbc8f8f, "royalblue": 0x4169e1, "saddlebrown": 0x8b4513,
	"salmon": 0xfa8072, "sandybrown": 0xf4a460, "seagreen": 0x2e8b57, "seashell": 0xfff5ee,
	"sienna": 0xa0522d, "silver": 0xc0c0c0, "skyblue": 0x87ceeb, "slateblue": 0x6a5acd,
	"slategray": 0x708090, "slategrey": 0x708090, "snow": 0xfffafa, "springgreen": 0x00ff7f,
	"steelblue": 0x4682b4, "tan": 0xd2b48c, "teal": 0x008080, "thistle": 0xd8bfd8,
	"tomato": 0xff6347, "turquoise": 0x40e0d0, "violet": 0xee82ee, "wheat": 0xf5deb3,
	"white": 0xffffff, "whitesmoke": 0xf5f5f5, "yellow": 0xffff00, "yellowgreen": 0x9acd32,
}
