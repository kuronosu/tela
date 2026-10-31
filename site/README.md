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
example the one of a release tag. The landing page is `src/content/docs/index.mdx`.
