package text

import (
	"os"
	"sync"
	"time"
)

// mapMinSize is the size from which readFontFile maps a font file instead
// of reading it: the CJK and emoji fonts (10 to 180 MB), not the Latin
// interface fonts (0.3 to 2 MB), whose bytes go-text mostly copies anyway
// (glyf, layout tables) and whose mapping would keep them locked on
// Windows (below).
const mapMinSize = 2 << 20

// readFontFile returns the bytes of the font file at path. A file of at
// least mapMinSize is mapped read-only into memory where the operating
// system allows it (mapFile); smaller files, and files that cannot be
// mapped, are read into the heap. A mapped file costs address space, not
// heap: its pages come from the page cache when they are touched, so a 180
// MB Apple Color Emoji or a 60 MB CJK collection costs only the pages of
// the tables parsed and of the glyphs drawn, and the kernel can drop them
// again under memory pressure.
//
// A mapping is never unmapped, and a file is mapped once per process
// (mappedFonts), so the mappings are bounded by the number of distinct
// large font files text has needed. The bytes are referenced by every
// typeface loaded from the file (sub-slices of its tables, ADR-243), by
// their glyph caches and by the faces, layouts, display lists, raster
// workers and the decoded-bitmap cache that hold those typefaces,
// possibly after their collection is gone; unmapping while any of them
// still holds a slice would turn a read into a crash, and tracking them
// all is not worth the address space it would give back. A file whose
// size or modification time changed is mapped again; the old mapping
// stays for the typefaces loaded from it.
//
// On Unix the file must not be truncated in place while it is mapped:
// reading a page past the new end faults (SIGBUS). Package managers
// replace font files with a new file (a rename), which leaves the old
// mapping valid; FreeType maps font files the same way. On Windows a
// mapped file cannot be deleted or replaced while the process runs, as
// with the fonts GDI has loaded.
func readFontFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() // a mapping stays valid once its file is closed
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() < mapMinSize || int64(int(fi.Size())) != fi.Size() {
		return os.ReadFile(path)
	}
	mappedFonts.Lock()
	defer mappedFonts.Unlock()
	if m, ok := mappedFonts.m[path]; ok && m.size == fi.Size() && m.mod.Equal(fi.ModTime()) {
		return m.data, nil
	}
	data, err := mapFile(f, int(fi.Size()))
	if err != nil {
		return os.ReadFile(path)
	}
	if mappedFonts.m == nil {
		mappedFonts.m = map[string]mappedFont{}
	}
	mappedFonts.m[path] = mappedFont{data: data, size: fi.Size(), mod: fi.ModTime()}
	return data, nil
}

// mappedFonts are the font files mapped by readFontFile, by path.
var mappedFonts struct {
	sync.Mutex
	m map[string]mappedFont
}

type mappedFont struct {
	data []byte
	size int64
	mod  time.Time
}
