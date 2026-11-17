# ADR-296: Auto-update, the `update` package

- **Date:** 2026-11-17
- **Status:** Accepted. Amends §5 (layers) and §17.

## Decision

- **Public package `update`** (above `desktop`): `update.Config{URL, PublicKeys, Channel, Policy}`, `Check(ctx)`, `Download(ctx)` with progress, `Apply`, `Restart`, `MarkHealthy`, and a hook form for components. `tela update keygen|sign|manifest` make ed25519 keys, signatures and manifests.
- **Manifest** (JSON, `schema: 1`): the app ID, channels (`stable`, `beta`, any name), releases with version, date, notes, `rollout` (a share of installs, from a hash of a random install ID and the version), `activation` (`next-start`, `app`, `now`), `minVersion`, `endOfSupport` (message and URL for versions that can no longer update), `published` and `expires`, and per target (`os`, `arch`, `format`) artifacts with URL, size, SHA-256, and a `patches` list (`from` version and its SHA-256, `kind`, URL, size, SHA-256). Clients ignore unknown fields and patch kinds, so delta updates arrive later without a new schema (the owner, 2026-11-17); this phase downloads full artifacts only.
- **Trust:** a detached ed25519 signature over the manifest's bytes (`manifest.json.sig`, with key IDs for rotation); an expired manifest, or one published before the newest one seen, is refused; every download is checked against its hash before use.
- **Applying:** formats that update themselves (`appimage`, Windows `zip`, macOS `app`) are staged in the cache, swapped in by renames, the old copy kept; a new version not marked healthy within its first starts is rolled back. Managed installs (`deb`, `flatpak`, `msix`) report `ErrManaged` with their manager (MSIX updates through its `.appinstaller`).

## Why

Standard library only (crypto/ed25519, net/http); a TUF-like minimum against freeze and rollback attacks; patches designed in from the start.
