# ADR-055: Pointer input follows the DOM for targets and propagation

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Pointer input follows the DOM for targets and propagation: events are passed by value and `StopPropagation`/`PreventDefault` act on a shared dispatch state; bubbling to the root, enter/leave on every element entered or left (not bubbling), pointer events for every pointer and mouse events as the compatibility view of the primary one (Chromium's boundary order), implicit capture for touch and pen, click on the nearest common ancestor of the press and release targets even after movement. A Flutter-style arena in `internal/gesture` decides between tap (no movement slop), drag (`OnDragStart`/`OnDrag`/`OnDragEnd`; 4 px mouse, 18 px touch and pen) and long press (500 ms); a drag or long press winning cancels the click; drag and long press go to the innermost element with the handler and do not bubble. Double click is a click count (500 ms / 5 px; 300 ms / 100 px for touch) that never delays clicks. Gesture timeouts use the backend clock, with one Host timer only while a press waits. After each discrete event the update queue is applied and components are rebuilt, so the next event's handlers see fresh state. `group-*`/`peer-*` combine every `group` ancestor and every preceding `peer` sibling (Tailwind's selectors); `Disabled` makes an element's subtree inert while its ancestors still receive bubbled events (React); interaction state changes restyle only elements whose classes test it. Hit-testing mirrors the paint order and geometry (clips, rounded clips, transforms including deferred ones) and is culled by per-box hit bounds

## Rationale

Owner's API decisions (value events with methods, Motion-style drag names, DOM click, pointer and mouse events); DOM semantics are what users and LLMs expect, Flutter's arena resolves competing gestures; flushing builds between discrete events fixes stale closures (two clicks before a frame count 2)
