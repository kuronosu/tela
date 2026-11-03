# Tela website

The site at <https://telaui.dev>, built with [Astro](https://astro.build) and
[Starlight](https://starlight.astro.build) (ADR-206). Node >= 22.23.

```bash
npm install
npm run dev      # http://localhost:4321
npm run build    # to dist/, fails on a broken internal link or anchor
```

The documentation is `docs/wiki`. `scripts/sync-wiki.mjs` turns it into pages
at every `astro dev` and `astro build`; the generated files are not committed.
`TELA_DOCS_DIR` points to another `docs/` (it needs `wiki/` and `images/`), for
example the one of a release tag.

## Deploying (ADR-229)

Only tags deploy <https://telaui.dev> (`.github/workflows/site.yml`); pushes to
`main` and pull requests only build and check it.

- A release tag (`v0.13.3`) deploys the whole site from that tag.
- A site tag deploys the site code of its commit with the docs of the latest
  release tag, for a change to the site between releases. Name it after the
  day, with `-2`, `-3` for more that day, and put it on a commit no release tag
  points at (Pages keeps one deployment per commit):

  ```bash
  git tag -a site-2026-11-03 -m "Site: <what changed>"
  git push origin site-2026-11-03
  ```

- `gh workflow run site.yml` does the same from `main`'s head.

## The landing page

- `src/pages/index.astro` is the landing page (`/`), with `src/components/landing/`
  (the screenshot and comparison components) and `src/assets/landing/`, the PNGs
  that `tela snapshot` renders from the Go snippets. The hero counter is
  `../examples/counter/main.go` itself (comments and `Preview` stripped), and the
  example screenshots come from `../docs/images`.
- `snippets/` is a Go module (`go.mod` of its own, `replace` to this checkout)
  with the five programs behind the code and PNG tabs, plus `lintdemo`, the
  deliberate typo that `tela lint` catches on the page. The page imports the
  sources with `?raw`. `scripts/render-snippets.sh` renders the PNGs (light and
  dark, except `theming`, which pins its own schemes); `--check` renders into a temp dir and fails if they differ from the
  committed ones, which CI runs. It renders in parallel (one job per CPU) and
  names any render that fails. Re-run it after editing a snippet or after a
  change that moves pixels.
- The page shares its page, header and hairline colours with the docs
  (`src/styles/custom.css`); `.lp` aliases its tokens to Starlight's. It passes a
  `hero` in its frontmatter, and `src/components/Hero.astro` renders nothing for
  `/` and Starlight's own Hero for every other hero page (the 404).
- `src/data/compare.json` is the comparison table; `compare-sources.md` has the
  passages behind each cell.
- `src/assets/brand/` is the logo (see its README); `public/` has the favicons,
  `og.png` and `llms.txt`.
