package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// The theme variables gallery (ADR-202): arbitrary values that use the
// theme's own CSS variables (var(--color-primary), var(--radius),
// var(--spacing)...), in a light panel and a dark Scheme panel, compared
// with Chromium and Tailwind v4, where they are the real variables. The
// page defines the tokens as theme variables in @theme, overridden under
// .dark (with shadcn/ui's @theme inline, var(--color-primary) would stay
// at its light value inside .dark), with colors of a made-up indigo and
// amber theme, which the Tela theme repeats.

// themeVarsColors are the tokens of the gallery, as light and dark hex
// colors: the page's CSS repeats them.
var themeVarsColors = map[string][2]uint32{
	"background": {0xffffff, 0x0a0a0a},
	"foreground": {0x0a0a0a, 0xfafafa},
	"card":       {0xfafaff, 0x1a1a24},
	"muted":      {0xf0f0f5, 0x262633},
	"border":     {0xc7c7d9, 0x3a3a4d},
	"primary":    {0x4f46e5, 0x818cf8},
	"accent":     {0xf59e0b, 0xd946ef},
	"ring":       {0x6366f1, 0xa5b4fc},
	"gold":       {0xca8a04, 0xfacc15},
}

const themeVarsCSS = `@custom-variant dark (&:is(.dark *));
@theme {
  --color-background: #ffffff;
  --color-foreground: #0a0a0a;
  --color-card: #fafaff;
  --color-muted: #f0f0f5;
  --color-border: #c7c7d9;
  --color-primary: #4f46e5;
  --color-accent: #f59e0b;
  --color-ring: #6366f1;
  --color-gold: #ca8a04;
  --radius-sm: calc(0.625rem * 0.6);
  --radius-md: calc(0.625rem * 0.8);
  --radius-lg: 0.625rem;
  --radius-xl: calc(0.625rem * 1.4);
}
:root { --radius: 0.625rem; }
.dark {
  --color-background: #0a0a0a;
  --color-foreground: #fafafa;
  --color-card: #1a1a24;
  --color-muted: #262633;
  --color-border: #3a3a4d;
  --color-primary: #818cf8;
  --color-accent: #d946ef;
  --color-ring: #a5b4fc;
  --color-gold: #facc15;
}
`

func themeVarsTheme() *Theme {
	colors := map[string]SchemeColor{}
	rgb := func(hex uint32) gfx.Color { return gfx.RGBA8(uint8(hex>>16), uint8(hex>>8), uint8(hex), 255) }
	for name, c := range themeVarsColors {
		colors[name] = SchemeColor{Light: rgb(c[0]), Dark: rgb(c[1])}
	}
	return &Theme{Radius: 10, Colors: colors}
}

// themeVarsPanel is a row-filling panel with the cases, dark or light.
func themeVarsPanel(dark bool) inode {
	p := iel("div", "-mx-4 flex h-full w-[360px] shrink-0 flex-col justify-center gap-4 bg-background p-5 text-foreground",
		iel("div", "flex items-center gap-4",
			// A meter: the ring is a conic gradient of two tokens.
			iel("div", "relative size-20 shrink-0",
				iel("div", "absolute inset-0 rounded-full bg-[conic-gradient(var(--color-primary)_72%,var(--color-muted)_0)]"),
				iel("div", "absolute inset-[calc(var(--spacing)*2)] flex items-center justify-center rounded-full bg-background text-sm font-medium", itext("72%"))),
			// A linear gradient from two tokens, with the theme's radius.
			iel("div", "h-20 grow rounded-[var(--radius)] bg-[linear-gradient(to_right,var(--color-primary),var(--color-accent))]")),
		iel("div", "flex items-center gap-4",
			// The theme's radius and spacing, a border and a ring from tokens.
			iel("div", "flex h-16 grow items-center rounded-[var(--radius-xl)] border border-[var(--color-border)] bg-[var(--color-card)] px-[calc(var(--spacing)*4)] text-sm shadow-[0_0_0_2px_var(--color-ring)]",
				itext("Card with a ring")),
			// A token with an opacity modifier, a custom token, and a fallback.
			iel("div", "size-16 shrink-0 rounded-[var(--radius-md)] bg-[var(--color-primary)]/40"),
			iel("div", "size-16 shrink-0 rounded-[var(--radius-sm)] bg-[var(--color-gold)]"),
			iel("div", "size-16 shrink-0 rounded-[var(--radius-nope,3px)] bg-[var(--color-nope,#16a34a)]")),
	)
	p.dark = dark
	return p
}

var themeVarsGallery = &cssGallery{
	name:    "themevars",
	width:   360,
	fontCSS: "@theme { --font-sans: \"Inter\"; }\n" + cssGalleryFonts + themeVarsCSS,
	theme:   themeVarsTheme(),
	// Text anti-aliasing and the antialiased edges of rounded corners and the ring dominate.
	mean: [2]float64{0.6, 0.4},
	geom: [2]float64{1.01, 1.01},
	rows: []cssRow{
		{name: "light", height: 240, kids: []inode{themeVarsPanel(false)}},
		{name: "dark", height: 240, kids: []inode{themeVarsPanel(true)}},
	},
}

// TestChromiumThemeVars compares the theme variables gallery with Chromium.
func TestChromiumThemeVars(t *testing.T) { themeVarsGallery.compare(t) }

// TestThemeVarsGallery is the gallery's golden, at scales 1 and 2.
func TestThemeVarsGallery(t *testing.T) { themeVarsGallery.golden(t) }
