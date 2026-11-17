// Package update updates an app from a signed manifest (ADR-296).
//
// The publisher makes a key once, and for every release a manifest that
// lists the release's files with their sizes and SHA-256 sums, signed
// with the key:
//
//	tela update keygen -o notes.key            # prints the public key
//	tela package -version 1.2.0 ./cmd/notes    # writes dist/notes-1.2.0-…
//	tela update manifest -m dist/manifest.json -app-id com.example.notes -version 1.2.0 dist/notes-1.2.0-*
//	tela update sign -key notes.key dist/manifest.json   # writes manifest.json.sig
//
// and uploads dist/ to a web server. The app checks with [Check],
// downloads with [Update.Download] (verified against the manifest),
// puts the new version in place with [Update.Apply], and runs it at the
// next start or now with [Restart]; [UseUpdate] does all of it for a
// component. See the [Manifest] for the format and [SignatureFile] for the
// signature.
//
// # Trust
//
// Check verifies the manifest's detached ed25519 signature
// (<manifest>.sig) over its exact bytes with Config.PublicKeys before it
// reads anything in it. It refuses a manifest of another app, an expired
// one (a server cannot freeze an install on an old release forever) and
// one published before the newest one the install has seen (nor roll it
// back to an older manifest). Every download is checked against the
// size and SHA-256 the signed manifest gives before it is used, and
// comes only from a URL the manifest names.
//
// # Formats
//
// Check detects how the running program was installed. Three formats
// update themselves: an AppImage (appimage), the Windows zip folder
// (zip, marked by a [MarkerName] file that tela package writes) and a
// macOS bundle (app, from a zip of the bundle). Apply swaps them by
// renames, keeping the old copy. Installs that a package manager updates
// make Check return a [*ManagedError]: deb (apt), flatpak, and MSIX
// (through its .appinstaller file).
//
// # Rollback
//
// A new version is on trial until it calls [MarkHealthy] ([UseUpdate]
// does once its first frame is presented). tela.Run counts its starts;
// on the start after [HealthyStarts] starts without it, the old copy is
// put back and started instead, the rollback is recorded ([RolledBack])
// and Check stops offering that version.
//
// # Delta updates
//
// The manifest lists patches from older versions for each artifact
// ([Patch]), so delta updates can come later without a new schema; this
// version downloads full artifacts only, and ignores patches.
package update
