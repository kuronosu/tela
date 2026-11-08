package memprobe

// DefaultRules attribute the live heap to subsystems; see [Classify] for how
// they are applied. The order matters: a rule that names a cache or a
// buffer goes before the rule of the package that fills it. Function names
// are matched by prefix after "github.com/kuronosu/tela/" is removed.
//
// The rules were chosen by reading the stacks of the heap profile of the
// examples (go run ./internal/testutil/memreport -v shows the largest
// sites with their category); rules_test.go checks the attribution on
// synthetic stacks.
var DefaultRules = []Rule{
	{CatBackdrop, []string{
		"internal/gfx/raster.(*BackdropCache)",
		"internal/gfx/raster.NewBackdropCache",
	}},
	{CatFonts, []string{
		"github.com/go-text/typesetting/font",
		"github.com/go-text/typesetting/opentype",
		"internal/text.ParseTypefaces",
		"internal/text.newTypeface",
		"internal/text.describeTypeface",
		"internal/text.(*FontSource)",
		"internal/text.(*sourceFont)",
		"internal/text.sys",
		"internal/assets/fonts", // every family's package: internal/assets/fonts/inter...
		"internal/stdfonts",
	}},
	// The renderer's buffers and growable scratch come before the glyph cache:
	// a glyph is rendered through them, and what they grow to while it is
	// rendered is theirs; only the mask and the entry are the cache's.
	{CatRasterBuf, []string{
		"internal/gfx/raster.(*Renderer).getBuf",
		"internal/gfx/raster.(*pathFiller)",
		"internal/gfx/raster.(*blurScratch)",
		"internal/gfx/raster.(*shadowScratch)",
		"internal/gfx/raster.(*imageShader)",
		"internal/gfx/raster.(*workPool)",
		"internal/gfx/raster.grow",
	}},
	{CatGlyphCache, []string{
		"internal/gfx/raster.(*GlyphCache)",
		"internal/gfx/raster.(*Renderer).glyphCached",
		"internal/gfx/raster.NewGlyphCache",
		"internal/gfx/raster.(*Renderer).glyphMask",
		"internal/gfx/raster.(*Renderer).glyphSprite",
		"internal/text.(*Face).",
		"internal/text.(*Typeface).Face",
	}},
	{CatFramebuffer, []string{
		"internal/core.resizeFramebuffer",
		"internal/platform/headless.(*Window).Present",
		"internal/platform/headless.(*Window).Frame",
		"internal/platform.CopyRGBAToBGRA",
		"examples/dashboard.newRig", // the dashboard's rig stands in for a Host and a backend
	}},
	{CatRasterBuf, []string{"internal/gfx/raster."}},
	{CatImages, []string{
		"internal/svg.",
		"internal/gfx.NewImage",
		"internal/gfx.(*Image)",
		"image/png.", "image/jpeg.", "image/gif.", "golang.org/x/image/",
	}},
	{CatTextLayout, []string{
		"internal/text.",
		"github.com/go-text/typesetting/",
	}},
	{CatDisplayList, []string{"internal/gfx."}},
	{CatTrees, []string{
		"internal/core.", "internal/style.", "internal/layout.", "internal/semantics.",
		"internal/anim.", "internal/gesture.",
		"github.com/kuronosu/tela.", "h.", "aria.", "kit.", "i18n.", "desktop.",
	}},
}
