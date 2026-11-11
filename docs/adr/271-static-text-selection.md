# ADR-271: Selection of static text

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-042, ADR-069, ADR-260 and ADR-265.

## Decision

- **Selectable by default** (owner): text outside fields unless `user-select: none`. A press on a Button, a `Draggable` or drag-handler element, a link or an aria press (React Aria's usePress; `PressOptions.AllowTextSelectionOnPress` opts in) starts none and keeps the selection; Chromium does start one from a button's text, so this is the owner's rule.
- **Range:** anchor and focus in paragraphs, in box-tree order with an atomic inline's text at its slot, inside one root: the window's content or one portal. Fields are outside; focusing one clears it, as does an end leaving the tree.
- **Pointer** (Chromium): a press puts a caret, Shift extends (non-directional on Mac), double click a word, triple a paragraph with its break, a drag extends by that unit and autoscrolls containers; the click still fires; a won drag gesture collapses it. A press on it drags the text (copy) as fields do, or clears it on release.
- **Keys** outside fields: Ctrl+C, Ctrl+Insert, Cmd+C; Ctrl/Cmd+A (root of the focus, else the selection); Escape and arrows do nothing. Menu key and Shift+F10 open the context menu at the start's caret when the selection has the focus (SelectionHasFocus), an OnContextMenu where no area took the key.
- **Copy:** rendered text, `\n` between blocks, `\n\n` after a P or heading whose bottom margin is half its font size.
- **Paint:** fields' colors and `selection:`; the paragraph's version, no layer. `cursor: auto` is the I-beam over selectable text.
- `telatest.App.SelectedText`.

## Why

The web's behavior; Chromium's SelectionController, TextIterator and event_handler.cc.
