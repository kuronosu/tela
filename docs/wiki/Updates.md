# Updates

Package `update` keeps an installed app up to date from a manifest that you
sign and put on any web server (ADR-296). The app checks the manifest,
downloads the file for its platform, verifies it, swaps it in and runs the new
version at its next start, or at once when you ask. A new version that does
not start properly is undone on its own.

- [Publishing a release](#publishing-a-release)
- [Checking from the app](#checking-from-the-app)
- [Activation](#activation)
- [Channels and rollout](#channels-and-rollout)
- [Install formats](#install-formats)
- [Rollback](#rollback)
- [Trust](#trust)
- [The manifest](#the-manifest)
- [The signature file](#the-signature-file)
- [The Windows install marker](#the-windows-install-marker)

## Publishing a release

Make a signing key once. Keep the private key out of the repository: whoever
has it can publish updates for your app.

```bash
tela update keygen -o notes.key
```

It prints the public key in the form the app needs:

```text
Public key (key ID 7a966b65c41c995b), for update.Config:

	PublicKeys: []update.PublicKey{"ed25519:OWsplNJQAWg09GzQZtNSskYuS1sHWDMT4iSqU3RFSbo="},
```

For every release, package the app, add the release to the manifest and sign
it:

```bash
tela package -o dist .
tela update manifest -m dist/manifest.json -app-id com.example.notes \
    -version 1.2.0 -notes "Faster search." -key notes.key dist/notes-1.2.0-*
```

`tela update manifest` reads each file's target from its name as
`tela package` writes it (`<name>-<version>-<os>-<arch>.<ext>`), computes its
size and SHA-256 and lists it under the release; a universal macOS zip
(`-darwin-universal.zip`) is listed for both `darwin/amd64` and
`darwin/arm64`. Give an unusual name its target as
`file=linux/amd64/appimage`. Artifact URLs are relative to the
manifest unless you give `-base-url`. Upload `dist/` (the files,
`manifest.json` and `manifest.json.sig`) to your server.

The manifest expires (90 days by default, `-expires`), so a server cannot keep
an install on an old release forever. Extend it before then, even without a
release:

```bash
tela update manifest -m dist/manifest.json -key notes.key
```

`tela help update manifest` lists every flag: the channel, the rollout share,
the activation, the lowest version that may update directly (`-min-version`)
and the end of support for old versions.

## Checking from the app

`UseUpdate` checks when the component mounts, downloads and applies the update
on a goroutine of its own, and returns its state for a banner:

```go
var updates = update.Config{
    URL:        "https://example.com/notes/manifest.json",
    PublicKeys: []update.PublicKey{"ed25519:OWsplNJQAWg09GzQZtNSskYuS1sHWDMT4iSqU3RFSbo="},
}

func Root() tela.Node {
    up := update.UseUpdate(updates)
    return Div(Class("flex flex-col h-full"),
        If(up.Status == update.Ready && up.Update.Policy == update.ActivateByApp,
            Div(Class("flex items-center gap-3 px-4 py-2 bg-sky-100 text-sky-900"),
                Textf("Notes %s is ready.", up.Update.Version),
                Button(Class("ml-auto font-medium"), OnClick(up.Restart), Text("Restart now")),
            ),
        ),
        Content{},
    )
}
```

`up.Status` is `Checking`, `UpToDate`, `Downloading` (with `up.Done` and
`up.Total`), `Ready` or `Failed` (with `up.Err`). `up.Check()` checks again.
Without the hook, call the steps yourself:

```go
u, err := update.Check(ctx, updates)
switch {
case errors.Is(err, update.ErrUpToDate), errors.Is(err, update.ErrManaged), errors.Is(err, update.ErrNotInstalled):
    return
case err != nil:
    return err
}
if err := u.Download(ctx, func(done, total int64) { /* progress */ }); err != nil {
    return err
}
if err := u.Apply(); err != nil {
    return err
}
```

The running version is the one `tela build` and `tela package` write into the
binary (from the module's VCS tag or `-version`); a binary without one returns
`ErrNoVersion`. `go run` and `tela build` outputs are not installs:
`ErrNotInstalled`.

## Activation

Each release says when it takes effect (`-activation`), and `Config.Policy`
overrides it for every release:

| Activation | What happens |
|---|---|
| `next-start` (default) | The new version is put in place and runs the next time the user starts the app. |
| `app` | The same, and the app offers a restart (`up.Restart`, `update.Restart`). |
| `now` | `UseUpdate` restarts into the new version as soon as it is applied, for urgent fixes. |

`Restart` closes the app as `tela.Quit` does, so `desktop`'s settings are
written, and starts the new version: on Linux and macOS it replaces the process,
on Windows it starts the new one and exits.

## Channels and rollout

A manifest has any number of channels (`stable` by default, `beta`, any name;
`Config.Channel` picks one). An app reads only its channel, so list a release
in several channels to offer it in each. Pre-release versions such as
`1.3.0-beta.1` sort before `1.3.0`.

`-rollout 0.25` offers a release to a quarter of the installs. On its first
check an install makes a random ID, kept in `desktop.StateDir` and never sent;
its bucket for a version is the first 8 bytes of SHA-256 of the ID, a zero
byte and the version, scaled to [0, 1). The release is offered when the bucket
is below the share, so raising the share only adds installs, and the version
in the hash means a different quarter goes first each release. `-rollout 0`
pauses a release.

`-min-version 1.5.0` keeps older versions from updating straight to a release
(a data migration that needs 1.5); they get the newest release they may take,
a stepping stone. `-end-of-support 1.0.0` with a message and a URL makes
`Check` return an `*update.EndOfSupportError` to versions below 1.0.0, for the
app to show.

## Install formats

`Check` detects how the app was installed:

| Format | Detected by | Updated by |
|---|---|---|
| `appimage` | `$APPIMAGE` (with the executable inside `$APPDIR`) | `update`: the file is replaced by an atomic rename |
| `app` | the executable at `<Name>.app/Contents/MacOS/` | `update`: the bundle is swapped by renames in its folder |
| `zip` | a `tela-install.json` file next to the executable | `update`: the folder's files are moved aside and the new ones moved in |
| `deb` | an executable under `/usr` or `/opt` listed by dpkg | apt: `ErrManaged` |
| `flatpak` | `$FLATPAK_ID` or `/.flatpak-info` | flatpak: `ErrManaged` |
| `msix` | the process has a package identity | its `.appinstaller` file or the Store: `ErrManaged` |

`Config.Format` forces a format. Downloads go to the app's cache folder and
resume where they stopped. The new copy is unpacked there and renamed into
place, or copied next to the install first when the cache is on another
volume, so the last step is always a rename. The old copy stays next to the
install, in `.<name>.update` (the AppImage and the bundle) or in the folder's
`.update` (the zip), until the next update.

## Rollback

A new version is on trial until it calls `update.MarkHealthy()`. `UseUpdate`
calls it once its component's first frame is presented; an app with a stricter
test calls it itself, for example after it opened its database. `tela.Run`
counts the starts of a version on trial: at the third start without
`MarkHealthy` (`update.HealthyStarts` is 2), it puts the old copy back,
records why, starts the old version instead and does not offer that release to
the install again. `update.RolledBack()` returns the record, to tell the user.

## Trust

- The manifest's detached ed25519 signature (`manifest.json.sig`) is verified
  over its exact bytes with `Config.PublicKeys` before anything in it is read.
- A manifest of another app ID, an expired one and one published before the
  newest one the install has seen are refused (`ErrExpired`,
  `ErrOlderManifest`), so a server can neither freeze an install nor roll it
  back.
- Files come only from the URLs the signed manifest names, and their size and
  SHA-256 are checked before use (`ErrHashMismatch`).

To rotate keys, sign with the old and the new key
(`tela update sign -key old.key -key new.key manifest.json`), ship a release
whose `PublicKeys` has both, and sign with the new key only once installs have
it.

## The manifest

Schema 1, JSON. Clients ignore fields they do not know, so later versions add
fields without a new schema.

```json
{
  "schema": 1,
  "appID": "com.example.notes",
  "published": "2026-11-17T10:00:00Z",
  "expires": "2027-02-15T10:00:00Z",
  "endOfSupport": {"below": "1.0.0", "message": "Download Notes 2.", "url": "https://example.com/notes"},
  "channels": {
    "stable": [{
      "version": "1.2.0",
      "date": "2026-11-17T10:00:00Z",
      "notes": "Faster search.",
      "rollout": 0.25,
      "activation": "next-start",
      "minVersion": "1.0.0",
      "artifacts": [{
        "os": "linux", "arch": "amd64", "format": "appimage",
        "url": "notes-1.2.0-linux-amd64.AppImage",
        "size": 48213504,
        "sha256": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
        "patches": [{"from": "1.1.0", "fromSha256": "…", "kind": "…", "url": "…", "size": 812, "sha256": "…"}]
      }]
    }]
  }
}
```

`patches` lists deltas from older versions, for delta updates later. This
version of Tela downloads full files only and skips patch kinds it does not
know.

## The signature file

```json
{
  "signatures": [
    {"keyID": "7a966b65c41c995b", "sig": "base64 of the 64-byte ed25519 signature"}
  ]
}
```

A key ID is the first 8 bytes of the SHA-256 of the 32-byte public key, in
hex. One valid signature by a trusted key is enough; signatures by other keys
are ignored.

## The Windows install marker

The Windows `zip` that `tela package` writes holds `tela-install.json` next to
the executable. It marks the folder as an install that updates itself and names
the files that belong to the app, so files the user put in the folder survive
an update:

```json
{
  "schema": 1,
  "format": "zip",
  "appID": "com.example.notes",
  "version": "1.2.0",
  "exe": "Notes.exe",
  "files": ["Notes.exe", "LICENSE.txt", "tela-install.json"]
}
```

Windows lets a running executable be renamed but not replaced, so `Apply`
moves the listed files into `.update\old` and the new ones in; the new version
runs from the next start.
