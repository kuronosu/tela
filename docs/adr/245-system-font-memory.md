# ADR-245: System font files are mapped, and go-text sees stubs of the bitmap tables

- **Date:** 2026-11-08
- **Status:** Accepted

## Decision

Refines ADR-243 and ADR-244.

- **Mapped files.** A system font file of 2 MiB or more is mapped read-only (`mmap` on Unix and macOS, `MapViewOfFile` on Windows) instead of read into the heap; smaller files, and any mapping that fails, are read. A file is mapped once per process, again only if its size or modification time changes, and never unmapped: typefaces, display lists, raster workers and the bitmap cache hold sub-slices of it.
- **Bitmap stubs for go-text.** go-text copies every table it loads and kept all of CBDT, EBDT or sbix only for glyph extents. Tela hands it a table directory where those tables are small stubs holding the strike go-text picks at 0 ppem (CBLC metrics without image data; sbix PNG headers of the same sizes), so extents, advances and shaping are identical; Tela draws from the file's own tables. sbix strikes are read lazily, glyph by glyph.

## Why

A macOS app drawing one emoji would have loaded Apple Color Emoji, about 180 MB, into the heap. The system's Noto Color Emoji CBDT went from 22 to 1.7 MiB of heap; a 94 MB synthetic sbix font from 188 to 0.4 MiB.

## What it changes

Windows cannot delete a mapped file while the process runs; the 2 MiB threshold keeps Latin UI fonts unmapped. Code that sets a go-text face's ppem must stop using the stubs.
