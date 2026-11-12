# ADR-278: macOS presents through IOSurfaces

- **Date:** 2026-11-11
- **Status:** Accepted

## Decision

The cocoa backend opens `IOSurface.framework` (a system framework, like AppKit, QuartzCore and CoreGraphics; opened on first use by `internal/darwin`). Each window keeps its retained RGBA frame and a swap chain of two IOSurfaces of its physical size (BGRA, premultiplied, tagged `kCGColorSpaceSRGB` through `IOSurfaceColorSpace`). It adds a third only while the window server still uses the one it would write next (`IOSurfaceIsInUse`). A Present copies its damage into the retained frame. Inside `IOSurfaceLock`, it then writes into a surface the layer does not show the rectangles that surface lacks: this frame's damage and what it missed while the others were shown. It sets that surface as `CALayer.contents`. A size change replaces the chain. Without IOSurface, or when a surface cannot be created or locked, the window falls back to the CGImage of the whole frame.

## Rationale

Every Present copied the whole frame into a new `CFData` and `CGImage`, and CoreAnimation copied it again on upload. For a 2880x1800 Retina window that is about 41 MB per frame (2 × 20.7 MB), whatever the damage. Now a frame copies its damage about three times: into the retained frame, and into the surface with the damage of the frame before it. A caret blink copies under a kilobyte. The window server shows the surface without a copy. The surface written is never the one shown, because CoreAnimation does not notice a change to the surface it already shows. Firefox (NativeLayerCA, SurfacePoolCA) and Chromium present the same way.

## Changes

ARCHITECTURE §2 (the macOS line lists IOSurface) and §15 (the cocoa row: present via IOSurfaces). `internal/darwin/iosurface_darwin.go`, `internal/platform/cocoa/present_darwin.go`, `surfaces.go`.
