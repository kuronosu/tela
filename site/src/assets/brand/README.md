# Tela brand assets

The mark is the **manila card** (`#e3cf9c`) in every place: a Jacquard
punched card with a thin graphite (`#292524`) keyline. **One file serves
light and dark**: on light backgrounds the keyline carries the card's edge
(15.17:1 on white), on dark ones the manila card does (12.32:1 on GitHub
dark), and the keyline simply disappears.

The SVG sources and the PNG tiles are here, in `site/src/assets/brand/`; the
files the site serves as they are (`favicon.svg`, `favicon.ico`,
`apple-touch-icon.png`, `og.png`, the social card) are in `site/public/`. The
design history (other concepts, rounds, the filled-T variant) is not in the
repository (ADR-207).

## Three drawings, by size

| Size on screen | Files | Drawing |
|---|---|---|
| 48 px and up | `tela-mark.svg` | The large mark: 5 × 5 grid, the T punched through (it shows the page), the other positions printed |
| 32–47 px | `tela-mark-mid.svg` (32 grid), `tela-mark-40.svg` | The large mark's T (5 across, 4 down) without the printed positions, punched through |
| below 32 px | `tela-mark-16.svg`, `-20`, `-24` | The 3 × 3 T, punched through |

At every size the T is hollow: punched through the manila card and the
keyline card, so it shows the page. On dark pages it reads at 12.32:1; on
white it is white on manila, 1.54:1, a lighter T that reads by its shape.
Each small file is drawn on whole pixels at its own size (holes 2 px at 16,
3 px at 20, 4 px at 24 to 40): use the file whose number matches the size on
screen when you can (`-16`, `-20`, `-24`, `-mid` for 32, `-40`).

## Files

| File | Where it goes |
|---|---|
| `tela-mark.svg`, `-mid`, `-16`, `-20`, `-24`, `-40` | The mark, one file for light and dark |
| `tela-mark-light*.svg` | The same drawings under the name `final/` used |
| `tela-mark-dark*.svg` | The manila card without the keyline, for dark-only places (identical in look on dark) |
| `tela-mark-mono.svg` | One colour (`currentColor`), for print and monochrome contexts |
| `tela-lockup-light.svg` | Keyline manila card + graphite "tela" (Inter SemiBold, outlined) |
| `tela-lockup-dark.svg` | Manila card + stone-50 "tela" |
| `site/public/favicon.svg` | SVG favicon, the 16 px drawing; one file works on light and dark tabs |
| `site/public/favicon.ico` | 16, 32 and 48 px, the tile (full bleed, so no transparent corners are needed); its T is punched through the card and shows the graphite tile behind |
| `site/public/apple-touch-icon.png` | 180 × 180, the tile |
| `tela-tile.svg`, `-mid`, `-16`; `tela-tile-512/256/64.png` | The manila card on a graphite square: GitHub avatar, registry icons |
| `tela-social-card.svg` (PNG: `site/public/og.png`) | 1280 × 640, the large keyline mark and graphite "tela" on stone-50 |

The lockups colour their wordmark with `currentColor` and set the default with
the root `color` attribute; they render correctly without CSS.

## Starlight

Starlight's header shows the logo at 32 px, and at 40 px from 50rem. The site
uses the large mark there (the owner's choice over the small redrawn ones),
redrawn on whole pixels for those two sizes so its square holes stay square:
`tela-mark-header-32.svg` and `tela-mark-header-40.svg` (3 px holes, 2 px gaps,
1 px prints). `src/components/SiteTitle.astro` overrides Starlight's title to
pick one with `<picture>`; one file per size serves light and dark.

Favicons:

```html
<link rel="icon" href="/favicon.ico" sizes="16x16 32x32 48x48">
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<link rel="apple-touch-icon" href="/apple-touch-icon.png">
```

README:

```html
<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/src/assets/brand/tela-lockup-dark.svg">
    <img src="site/src/assets/brand/tela-lockup-light.svg" alt="Tela" width="200">
  </picture>
</p>
```

## Contrast (WCAG 2; graphics need 3:1)

| | #ffffff | #fafaf9 | #0d1117 | #1c1917 |
|---|---|---|---|---|
| Manila card | 1.54 | 1.47 | **12.32** | **11.39** |
| Graphite keyline | **15.17** | **14.52** | 1.25 | 1.15 |
| Card edge (the better of the two) | **15.17** | **14.52** | **12.32** | **11.39** |
| Large T (punched) | 1.54 | 1.47 | **12.32** | **11.39** |
| Small T (punched, hollow) | 1.54 | 1.47 | **12.32** | **11.39** |
| Wordmark | graphite **15.17** | **14.52** | stone-50 **18.12** | **16.74** |

The hollow T on white is carried by its shape and, at 48 px and up, the size
of its holes; that is the owner's choice (a filled T would read at 9.88:1). The printed positions (about 2:1) are decoration.
