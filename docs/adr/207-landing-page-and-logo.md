# ADR-207: The landing page and the logo

- **Date:** 2026-10-31
- **Status:** Accepted

## Decision

**Landing page.** telaui.dev opens on a page built from the owner's design review (four prototypes and a survey of 42 framework sites). Its sections, in order, are:
- a first viewport with the version chip, the install steps with OS tabs (macOS shown as planned) and a prompt for coding agents that points at `https://telaui.dev/llms.txt`;
- the counter's code beside its window;
- tabs that swap Go code and its PNG;
- one terminal for the snapshot/inspect/lint/mcp/dev loop;
- a carousel of the example apps;
- a comparison with Fyne, Gio, Wails and Electron;
- a closing call to action.

Every image is a Tela render. The snippets behind the tabs are a nested module, `site/snippets/`, and the site's CI re-renders them and fails if a PNG differs. The comparison is data, `site/src/data/compare.json`, dated and with a source for every cell about another project, quoted in `compare-sources.md`. It shows where Tela loses (macOS, mobile, web, screen readers, maturity).

**Logo.** The mark is a punched card with a T. It is a manila card (#e3cf9c) with a thin graphite (#292524) keyline, and the clipped corner of an 80-column card.
- **Large (48 px and up):** a 5 × 5 matrix of square holes. The T is punched through and the other positions are printed.
- **32–47 px:** the same T without the printed positions.
- **Under 32 px:** a 3 × 3 T.
- **Pixel grid:** every small drawing is set on whole pixels. The docs header still shows the large drawing, because the owner preferred it there. It is redrawn on whole pixels at 32 and 40 px (`tela-mark-header-*`), and the `SiteTitle` override picks one by width.
- **One file for light and dark:** on light pages the keyline carries the edge (15:1), and on dark ones the manila card does (12:1). The T is hollow at every size, as the owner chose. On white it reads by its shape (1.5:1).
- **Square images:** a Tile (the card on a graphite square) is for avatars, `favicon.ico` and the touch icon.
- **Wordmark:** "tela" in Inter SemiBold.
- **Where the files live:** the assets are in `site/src/assets/brand/`, and the favicons and the social card in `site/public/`. They are not in `docs/`, because the deploy takes `docs/` from the latest release tag.

## Rationale

The owner judges by looking: options were rendered and picked, and the marks are drawn by Tela's own SVG engine. A framework mark lives alone as a favicon, an avatar or a README header, so the owner preferred it to a product-style brand. The manila card is the colour real punched cards had, and the owner wanted it everywhere; the small sizes are redrawn, not scaled, because the large grid turned to noise at 32 px. Comparisons and claims stay checkable, because the page sells a pre-1.0 project.

## What it changes

`site/` gains `src/pages/index.astro`, `components/landing/`, `assets/landing/`, `assets/brand/`, `snippets/` and `scripts/render-snippets.sh`. The landing page and the docs share one palette in `custom.css` (stone-50, and stone-950 in dark). The landing page uses Starlight's hero mechanism: a `Hero` override renders nothing on `/`, where the page's own h1 is `#_top`. The comparison sources open in one shared popover. Snippet PNGs are re-checked by `site.yml` and by a `site-snippets` job in `ci.yml`. The README shows the logo. The prototypes and the design history are kept outside the repository.
