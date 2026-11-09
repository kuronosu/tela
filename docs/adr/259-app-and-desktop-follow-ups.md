# ADR-259: App and desktop follow-ups

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-087, ADR-209, ADR-210, ADR-234, ADR-251, ADR-252 and ADR-253 (contract v11, protocol 15).

## Decision

- **Access keys:** `MenuItem.AccessKey` (HTML's `accesskey`), a letter or digit: marked where the label has it, else appended, "ファイル(&F)"; reserved before Windows' automatic keys. No `&` syntax: labels stay plain text.
- **Built-in labels:** Win32 widens renamed message box buttons and the box (CBT hook, `GetTextExtentPoint32W`); comctl32's TaskDialog is not an allowed DLL. Portal dialogs and OS-set tray items keep the OS's language; `Notify` actions are the app's text. Title bar, native boxes and menus follow the global label source, not `i18n.Provider`: they are window or app scoped.
- **Numbers:** x/text only rounds; `i18n` applies CLDR 48 symbols, percent affixes, grouping and minimumGroupingDigits (es writes 1234,5), generated from vendored `numberFormats.json`.
- **Text fields:** a drop selects the text; a steady drop caret while a field takes a drag; a focused field's selection drags (move, copy with Ctrl or Option; same-tree drops move at the drop, others at `DragDoneEvent`).
- **Stores:** a write re-reads and merges when the file changed before its rename; a missing folder is watched through its nearest ancestor; the watch opens off the caller; macOS uses kqueue.
- **App:** `WindowHandle.Destroy` runs a close's default without `UseCloseRequest` handlers. `Launch.AtStart` (contract `LaunchEvent.AtStart`, Cocoa only).
- **Associations:** `AssociationStatus.Role`, `Registration.Role`; Linux and Windows keep no roles. macOS has no API to clear a user's default, and developer entries keep their role.
- **Monitors:** X11 fits the frame (`_NET_REQUEST_FRAME_EXTENTS`, 100 ms) on `_GTK_WORKAREAS_D<n>`; Win32 the visible frame. Removed monitors are left to the OS.

## Why

Native conventions, and the smallest change that closes each gap.
