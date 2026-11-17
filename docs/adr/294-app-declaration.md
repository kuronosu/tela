# ADR-294: One declaration of the app

- **Date:** 2026-11-17
- **Status:** Accepted. Amends ADR-227 (what `tela build` reads).

## Decision

- **`tela.App` is the declaration** (the owner, 2026-11-17). New fields, used by the packages and ignored by `Run` unless noted: `Publisher` (a person or company; also the MSIX publisher name), `Description`, `Category` (freedesktop main category, mapped to `LSApplicationCategoryType` and MSIX), `Schemes []string`, `FileTypes []tela.FileType` (`Ext`, `MIME`, `Description`, `Role` as `desktop`'s roles), and `Services tela.Services`: usage strings for camera, microphone, location, contacts, calendars, photos, Bluetooth (empty is not used; `Info.plist` usage keys, macOS entitlements, MSIX capabilities, flatpak permissions) and booleans for network, notifications, secrets, global shortcuts, files outside the user's choices.
- **Read statically**, as `AppID` and `Title` are (ADR-227): constant strings, slice and struct literals of constants; anything else is an error naming the field and the way out.
- **Icon:** `icon.svg` or `icon.png` (at least 512 px) in the main package's directory, or `-icon`; `tela package` renders every size and format from it (`.ico`, `.icns`, hicolor PNGs, MSIX tiles), and the Windows executable gets it and its version info as a resource through the build overlay.
- **Version:** the main module's version as the go command stamps it (a VCS tag), or `-version`; `tela build` and `tela package` write it into the binary, which `update` reads.

## Why

One source, already read at build time and visible at run time; `tela` cannot import `desktop`, so the types are `tela`'s.
