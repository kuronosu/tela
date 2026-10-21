package core

import (
	"fmt"
	"testing"
)

// The theme gallery: a shadcn/ui-styled card (title, description, a field,
// outline and primary buttons, badges, a focus ring, the chart colors)
// written with theme tokens, in a light panel and in a dark one (a Scheme
// prop here, the class "dark" in the page). Chromium renders the page with
// Tailwind v4 and shadcn/ui's v4 theme CSS: its neutral variables in :root
// and .dark, mapped by @theme inline, with --radius 0.625rem (Theme.Radius
// 10 here) and its dark variant (&:is(.dark *)). The field is a div styled
// like shadcn's Input showing its placeholder, so that both sides draw the
// same text.

// shadcnCSS is shadcn/ui's v4 theme for the neutral base color (the
// theming docs, apps/v4), without the sidebar variables.
const shadcnCSS = `@custom-variant dark (&:is(.dark *));
@theme inline {
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --color-card: var(--card);
  --color-card-foreground: var(--card-foreground);
  --color-popover: var(--popover);
  --color-popover-foreground: var(--popover-foreground);
  --color-primary: var(--primary);
  --color-primary-foreground: var(--primary-foreground);
  --color-secondary: var(--secondary);
  --color-secondary-foreground: var(--secondary-foreground);
  --color-muted: var(--muted);
  --color-muted-foreground: var(--muted-foreground);
  --color-accent: var(--accent);
  --color-accent-foreground: var(--accent-foreground);
  --color-destructive: var(--destructive);
  --color-border: var(--border);
  --color-input: var(--input);
  --color-ring: var(--ring);
  --color-chart-1: var(--chart-1);
  --color-chart-2: var(--chart-2);
  --color-chart-3: var(--chart-3);
  --color-chart-4: var(--chart-4);
  --color-chart-5: var(--chart-5);
  --radius-sm: calc(var(--radius) * 0.6);
  --radius-md: calc(var(--radius) * 0.8);
  --radius-lg: var(--radius);
  --radius-xl: calc(var(--radius) * 1.4);
  --radius-2xl: calc(var(--radius) * 1.8);
  --radius-3xl: calc(var(--radius) * 2.2);
  --radius-4xl: calc(var(--radius) * 2.6);
}
:root {
  --radius: 0.625rem;
  --background: oklch(1 0 0);
  --foreground: oklch(0.145 0 0);
  --card: oklch(1 0 0);
  --card-foreground: oklch(0.145 0 0);
  --popover: oklch(1 0 0);
  --popover-foreground: oklch(0.145 0 0);
  --primary: oklch(0.205 0 0);
  --primary-foreground: oklch(0.985 0 0);
  --secondary: oklch(0.97 0 0);
  --secondary-foreground: oklch(0.205 0 0);
  --muted: oklch(0.97 0 0);
  --muted-foreground: oklch(0.556 0 0);
  --accent: oklch(0.97 0 0);
  --accent-foreground: oklch(0.205 0 0);
  --destructive: oklch(0.577 0.245 27.325);
  --border: oklch(0.922 0 0);
  --input: oklch(0.922 0 0);
  --ring: oklch(0.708 0 0);
  --chart-1: oklch(0.646 0.222 41.116);
  --chart-2: oklch(0.6 0.118 184.704);
  --chart-3: oklch(0.398 0.07 227.392);
  --chart-4: oklch(0.828 0.189 84.429);
  --chart-5: oklch(0.769 0.188 70.08);
}
.dark {
  --background: oklch(0.145 0 0);
  --foreground: oklch(0.985 0 0);
  --card: oklch(0.205 0 0);
  --card-foreground: oklch(0.985 0 0);
  --popover: oklch(0.205 0 0);
  --popover-foreground: oklch(0.985 0 0);
  --primary: oklch(0.922 0 0);
  --primary-foreground: oklch(0.205 0 0);
  --secondary: oklch(0.269 0 0);
  --secondary-foreground: oklch(0.985 0 0);
  --muted: oklch(0.269 0 0);
  --muted-foreground: oklch(0.708 0 0);
  --accent: oklch(0.269 0 0);
  --accent-foreground: oklch(0.985 0 0);
  --destructive: oklch(0.704 0.191 22.216);
  --border: oklch(1 0 0 / 10%);
  --input: oklch(1 0 0 / 15%);
  --ring: oklch(0.556 0 0);
  --chart-1: oklch(0.488 0.243 264.376);
  --chart-2: oklch(0.696 0.17 162.48);
  --chart-3: oklch(0.769 0.188 70.08);
  --chart-4: oklch(0.627 0.265 303.9);
  --chart-5: oklch(0.645 0.246 16.439);
}
`

// shadcnCard is the card of the theme gallery.
func shadcnCard() inode {
	var chart []inode
	for i := 1; i <= 5; i++ {
		chart = append(chart, iel("div", fmt.Sprintf("flex-1 rounded-sm bg-chart-%d", i)))
	}
	return iel("div", "flex w-[300px] flex-col gap-4 rounded-xl border border-border bg-card p-5 text-card-foreground shadow-sm",
		iel("div", "flex flex-col gap-1.5",
			iel("div", "text-base font-semibold leading-none", itext("Create project")),
			iel("p", "text-sm text-muted-foreground", itext("Deploy your new project in one click.")),
		),
		iel("div", "flex flex-col gap-2",
			iel("div", "text-sm font-medium leading-none", itext("Name")),
			iel("div", "flex h-9 items-center rounded-md border border-input px-3 text-sm text-muted-foreground shadow-xs dark:bg-input/30",
				itext("Name of your project")),
		),
		iel("div", "flex justify-end gap-2",
			iel("button", "inline-flex h-9 items-center justify-center rounded-md border border-border bg-background px-4 text-sm font-medium shadow-xs dark:border-input dark:bg-input/30",
				itext("Cancel")),
			iel("button", "inline-flex h-9 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground shadow-xs",
				itext("Deploy")),
		),
		iel("div", "flex gap-2",
			iel("span", "rounded-md bg-secondary px-2 py-0.5 text-xs font-medium text-secondary-foreground", itext("Draft")),
			iel("span", "rounded-md bg-destructive px-2 py-0.5 text-xs font-medium text-white", itext("Failed")),
			iel("span", "rounded-md border border-ring px-2 py-0.5 text-xs font-medium ring-[3px] ring-ring/50", itext("Focus")),
			iel("span", "rounded-md bg-accent/60 px-2 py-0.5 text-xs text-accent-foreground", itext("Accent")),
		),
		iel("div", "flex h-4 gap-1", chart...),
	)
}

// themePanel is a row-filling panel with the card, dark or light.
func themePanel(dark bool) inode {
	p := iel("div", "-mx-4 flex h-full w-[340px] shrink-0 items-center justify-center bg-background text-foreground", shadcnCard())
	p.dark = dark
	return p
}

var themeGallery = &cssGallery{
	name:    "theme",
	width:   340,
	fontCSS: "@theme { --font-sans: \"Inter\"; }\n" + cssGalleryFonts + shadcnCSS,
	theme:   &Theme{Radius: 10},
	// Text anti-aliasing (Skia's glyph coverage differs from ours) and
	// the rounded corners dominate; flat areas agree within a level.
	mean: [2]float64{1.0, 1.1},
	geom: [2]float64{1.01, 1.01},
	rows: []cssRow{
		{name: "light", height: 330, kids: []inode{themePanel(false)}},
		{name: "dark", height: 330, kids: []inode{themePanel(true)}},
	},
}

// TestChromiumTheme compares the theme gallery with Chromium.
func TestChromiumTheme(t *testing.T) { themeGallery.compare(t) }

// TestThemeGallery is the gallery's golden: a light panel and a dark
// Scheme panel in a light app, at scales 1 and 2.
func TestThemeGallery(t *testing.T) { themeGallery.golden(t) }
