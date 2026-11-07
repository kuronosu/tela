# ADR-237: macOS parity: every drag seen, file promises, template tray icons, modeless boxes, hot keys that follow the layout

- **Date:** 2026-11-07
- **Status:** Accepted. Amends ADR-210, ADR-213, ADR-214 and ADR-224.

## Decision

- **Every drag is seen.** Views register the well-known and file promise types, `public.item`/`data`/`content`, and each new drag's types, learned from the drag pasteboard by a global monitor of left mouse drags. A type with no MIME type is the Data type `application/x-uti.<identifier>`, read and written under that identifier (`dyn.` and promise types are not data).
- **File promises are `MIMEURIList`.** A drop reading it receives them into `<TMPDIR>/tela-drops/<pid>-<random>` on a background queue; `performDragOperation:` returns at once and `DragDrop` follows when the files are in (30 s at most). The source gets the answer held at the release, and the window refuses drags meanwhile. The folders are removed when `Run` returns.
- **A drag without an image** shows each item's file or type icon.
- **`platform.TrayOptions.Template` and `desktop.Tray.Template`:** a template icon (`NSImage.template`); other backends ignore it. Remote protocol 7.
- **A parentless message box is modeless:** the NSAlert's panel with retargeted buttons, no `runModal`.
- **Hot keys follow the layout** (`kTISNotifySelectedKeyboardInputSourceChanged`): moved hot keys are registered again, and only status changes are reported.

## Why

An HTML drop target sees any drag; Photos and Mail drag only promises. Linux and Win32 boxes block no other windows.
