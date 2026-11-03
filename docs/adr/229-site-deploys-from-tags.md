# ADR-229: The site deploys only from tags

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-206 and ADR-208.

## Decision

- **Only tags deploy telaui.dev.** Pushes to `main` and pull requests that touch the site or the docs only check it (build, links, snippets).
- **A release tag (`v*`)** deploys the whole site from that tag: site code, snippets and docs.
- **A site tag (`site-YYYY-MM-DD`, then `-2`, `-3` the same day)** deploys the site code of its commit with the docs of the latest release tag, for a change to the site between releases. The owner pushes it, as release tags. A manual run of `site.yml` does the same from `main`'s head.
- ADR-208's exception (a release commit publishing its own docs before its tag) is gone. Pages keeps one deployment per commit, so a site tag and a release tag go on different commits.
- **CI runs on branch pushes only**, not tag pushes: a release tag points at a commit `main` already tested.

## Rationale

The site must show the docs of a released version, never of an untagged one. With `main` deploying, the release commit's docs went out before its tag existed. A site tag keeps site-only deploys explicit and recorded, and its date name has no meaning to Go, unlike `site/vX.Y.Z`, which would read as a version of the `site` module.
