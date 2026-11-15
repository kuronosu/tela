package raster

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// Scenes of blend modes, mask layers and focal gradients, defined in package
// gfxtest. The first three
// (blend, masks, focal) are also written as HTML (blendHTML, masksHTML,
// focalHTML) and compared with Chromium (chromium4_test.go); the others
// cover what CSS cannot express (glyph masks, under rotations, nested in
// filter layers, non-isolated alpha).

var (
	oogGreen  = gfxtest.OogGreen
	oogYellow = gfxtest.OogYellow
)

var allBlendModes = gfxtest.AllBlendModes

type (
	blendCell = gfxtest.BlendCell
	focalCell = gfxtest.FocalCell
)

func blendCells() []blendCell { return gfxtest.BlendCells() }

var maskCells = gfxtest.MaskCells

func focalCells() []focalCell { return gfxtest.FocalCells() }

var scenes4 = fromGfxtest(gfxtest.Scenes4)

// ---------------------------------------------------------------------------
// HTML for Chromium
// ---------------------------------------------------------------------------

// cssGradientStops formats stops as CSS color stops.
func cssStops(cs []gfx.Color) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = cssColorOf(c)
	}
	return strings.Join(parts, ", ")
}

// cssColorOf is cssColor, except for the out-of-gamut test colors, which
// are written in oklch() so that Chromium converts them itself.
func cssColorOf(c gfx.Color) string {
	switch c {
	case oogGreen:
		return "oklch(0.75 0.3 150)"
	case oogYellow:
		return "oklch(0.9 0.25 100)"
	}
	return cssColor(c)
}

const htmlHead = `<!doctype html>
<html><head><meta charset="utf-8"><style>
html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
div { position: absolute; box-sizing: border-box; }
</style></head><body>
`

func px(v float32) string { return fmt.Sprintf("%gpx", v) }

func box(x, y, w, h float32) string {
	return fmt.Sprintf("left: %s; top: %s; width: %s; height: %s;", px(x), px(y), px(w), px(h))
}

// blendHTML is the blend scene as CSS: isolation, mix-blend-mode, opacity
// and gradients.
func blendHTML() string {
	var sb strings.Builder
	sb.WriteString(htmlHead)
	for _, c := range blendCells() {
		gx, gy := c.X+4, c.Y+4
		backdrop := fmt.Sprintf(`<div style="%s background: linear-gradient(to right, %s);"></div>
<div style="%s background: linear-gradient(to right, %s);"></div>
<div style="%s background: %s;"></div>
`, box(0, 0, 92, 60), cssStops([]gfx.Color{red, lime, sky}),
			box(0, 60, 92, 32), cssStops([]gfx.Color{indigo.WithAlpha(0.6), amber.WithAlpha(0.3)}),
			box(60, 0, 16, 92), cssColorOf(oogGreen))
		srcBg := "linear-gradient(in oklab to bottom, " + cssStops([]gfx.Color{oogYellow, fuchsia, sky.WithAlpha(0.4)}) + ")"
		if c.Clip {
			fmt.Fprintf(&sb, `<div title="clip %s" style="%s">
%s<div style="%s border-radius: 20px; overflow: hidden;"><div style="%s opacity: 0.9; mix-blend-mode: %s; background: %s;"></div></div>
</div>
`, c.Mode, box(gx, gy, 92, 92), backdrop, box(10, 10, 72, 72), box(-6, -6, 84, 84), c.Mode, srcBg)
			continue
		}
		fmt.Fprintf(&sb, `<div title="%s" style="%s isolation: isolate;">
%s<div style="%s opacity: 0.9; mix-blend-mode: %s; background: %s;"></div>
</div>
`, c.Mode, box(gx, gy, 92, 92), backdrop, box(14, 14, 64, 64), c.Mode, srcBg)
	}
	sb.WriteString("</body></html>\n")
	return sb.String()
}

// masksHTML is the masks scene as CSS mask-image.
func masksHTML() string {
	var sb strings.Builder
	sb.WriteString(htmlHead)
	content := fmt.Sprintf(`<div style="%s background: linear-gradient(to right, %s);"></div><div style="%s background: %s;"></div>`,
		box(0, 0, 140, 110), cssStops([]gfx.Color{indigo, rose}), box(30, 30, 80, 50), cssColor(sky.WithAlpha(0.7)))
	const radial = "radial-gradient(closest-side, black 60%, transparent)"
	for _, c := range maskCells {
		fmt.Fprintf(&sb, `<div title=%q style="%s background: linear-gradient(to bottom, %s);"></div>
`, c.Name, box(c.X+4, c.Y+4, 152, 122), cssStops([]gfx.Color{amber.WithAlpha(0.35), emerald.WithAlpha(0.35)}))
		e := box(c.X+10, c.Y+10, 140, 110)
		switch c.Name {
		case "alpha linear":
			fmt.Fprintf(&sb, `<div style="%s mask-image: linear-gradient(to bottom, black, transparent);">%s</div>
`, e, content)
		case "luminance":
			fmt.Fprintf(&sb, `<div style="%s mask-image: linear-gradient(to right, white, rgb(255 0 0), rgb(0 255 0 / 0.50196), black); mask-mode: luminance;">%s</div>
`, e, content)
		case "radial":
			fmt.Fprintf(&sb, `<div style="%s mask-image: %s;">%s</div>
`, e, radial, content)
		case "nested":
			fmt.Fprintf(&sb, `<div style="%s mask-image: linear-gradient(to right, black, transparent); background: linear-gradient(to bottom, %s);"><div style="%s mask-image: %s;"><div style="%s background: %s;"></div><div style="%s background: %s;"></div></div></div>
`, e, cssStops([]gfx.Color{emerald, slate}), box(20, 15, 100, 80), radial, box(0, 0, 100, 80), cssColor(rose), box(40, 0, 20, 80), cssColor(yellow))
		case "clip+rotate":
			fmt.Fprintf(&sb, `<div style="%s border-radius: 24px; overflow: hidden;"><div style="%s transform: rotate(12deg); mask-image: linear-gradient(to bottom, black, transparent);">%s</div></div>
`, e, box(0, 0, 140, 110), content)
		case "opacity+multiply":
			fmt.Fprintf(&sb, `<div style="%s background: %s;"></div><div style="%s mask-image: %s; opacity: 0.7; mix-blend-mode: multiply;">%s</div>
`, box(c.X+10, c.Y+50, 140, 30), cssColor(sky), e, radial, content)
		}
	}
	sb.WriteString("</body></html>\n")
	return sb.String()
}

// focalHTML is the focal scene as canvas createRadialGradient, the
// two-point conical gradient of the web platform (CSS radial gradients
// have no focal point).
func focalHTML() string {
	var items []string
	for _, c := range focalCells() {
		var cols []string
		for _, s := range c.Stops {
			cols = append(cols, fmt.Sprintf("%q", cssColor(s)))
		}
		sy := c.ScaleY
		if sy == 0 {
			sy = 1
		}
		items = append(items, fmt.Sprintf("[%g, %g, %g, %g, %g, %g, %g, %g, %g, [%s]]",
			c.X, c.Y, c.FX, c.FY, c.FR, c.CX, c.CY, c.R, sy, strings.Join(cols, ", ")))
	}
	return fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><style>
html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
canvas { display: block; width: 600px; height: 300px; }
</style></head><body><canvas id="c"></canvas>
<script>
const cells = [
%s
];
const dpr = window.devicePixelRatio, cv = document.getElementById("c");
cv.width = 600 * dpr; cv.height = 300 * dpr;
const ctx = cv.getContext("2d");
ctx.scale(dpr, dpr);
for (const [x, y, fx, fy, fr, cx, cy, r, sy, cols] of cells) {
  const ox = x + 75, oy = y + 75;
  ctx.save();
  ctx.beginPath(); ctx.rect(x, y, 150, 150); ctx.clip();
  ctx.translate(ox, oy); ctx.scale(1, sy); ctx.translate(-ox, -oy);
  const g = ctx.createRadialGradient(ox + fx, oy + fy, fr, ox + cx, oy + cy, r);
  cols.forEach((c, i) => g.addColorStop(i / (cols.length - 1), c));
  ctx.fillStyle = g;
  ctx.fillRect(x, oy - 75 / sy, 150, 150 / sy);
  ctx.restore();
}
</script></body></html>
`, strings.Join(items, ",\n"))
}
