# ADR-211: Drag and drop and settings v2 in core and the public API

- **Date:** 2026-11-01
- **Status:** Accepted

## Decision

Implements ADR-209 on ADR-210's contract (amends §10, §14).

- **One event type.** `tela.DragEvent` gains `DataTransfer *tela.DataTransfer` (nil in the plain gesture). `DataTransfer` wraps `platform.Data`: `Types`, `Has`, `Get`, `GetData`, `Text`, `HTML`, `Files`, `Image`; `Set`, `SetData`, `SetText`, `SetHTML`, `SetFiles`, `SetImage`, `ClearData`, `SetDragImage`; HTML's fields `DropEffect` and `EffectAllowed` (`tela.DropNone|DropCopy|DropMove|DropLink|DropAll`). HTML's names `text`, `url`, `Files` map to MIME types. The zero value is writable; the clipboard (step C) reuses it.
- **Targets:** `h.OnDragEnter`, `OnDragOver`, `OnDragLeave`, `OnDrop`, bubbling, hit-tested like pointers. HTML's order: enter on the new element, leave on the old (an unmounted one: its nearest mounted ancestor), then over. Before the drop only types are known. The latest dragover's `PreventDefault` accepts; `DropEffect` starts at `DefaultDropEffect`, a refused choice is none. The drop reads the well-known types plus the custom ones the handlers looked up. `OnDrop` needs no `PreventDefault`; its `DropEffect` is the outcome. A drag entering a window cancels its pointers (hover and `:active` end). A window under a modal child refuses drags.
- **Sources:** `h.Draggable(true)` adds a drag recognizer; its `OnDragStart` gets an empty `DataTransfer`. With data and no `PreventDefault`, core calls `StartDrag` with a crop of the last frame as the image. `OnDrag` does not run; `OnDragEnd` runs at `DragDoneEvent`, or at once with none if the drag cannot start (dev warning). Without data the press stays the gesture.
- **Settings:** `contrast-more:` (`contrast-less:` is an error); `tela.UseHighContrast`, `UseTextScale`, `UseAccentColor`. The text scale makes each tree compile with its theme's `RootFontSize` × scale, so every rem scales (type, spacing, breakpoints); `App.IgnoreTextScale` turns it off. `telatest.HighContrast`, `TextScale`, `AccentColor`.

## Rationale

Reusing `DragEvent` lets React code read as written; a scaled parser reuses the theme-change path.
