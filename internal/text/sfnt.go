package text

import (
	"encoding/binary"

	ot "github.com/go-text/typesetting/font/opentype"
)

// tableSource reads the tables of one font of a file for newTypeface.
//
// go-text's Loader copies every table it returns (RawTable reads it into a
// new buffer). The file is already in memory, though: the embedded fonts are
// embedded data (package fonts), and ParseTypefaces and the system fonts
// hold the bytes of the file. So when the file is a plain sfnt (a TrueType
// or OpenType font, or a collection of them), the tables are sub-slices of
// the file found through its table directory, which costs no heap; the
// tables Tela keeps unparsed (glyf, loca, gvar, hmtx) stay inside the file
// data. A WOFF file
// (compressed tables) or a dfont (offsets relative to its resource map) is
// read through the Loader's copies.
type tableSource struct {
	ld   *ot.Loader
	data []byte // the whole file; nil when the tables are read through ld
	dir  []byte // the table records of this font in data, 16 bytes each
}

// newTableSource returns the source of the index-th font of data, which
// ld is the Loader of.
func newTableSource(ld *ot.Loader, data []byte, index int) tableSource {
	s := tableSource{ld: ld}
	if dir := sfntDirectory(data, index); dir != nil {
		s.data, s.dir = data, dir
	}
	return s
}

// table returns the bytes of the table tag, or nil when the font has none
// or its record lies outside the file (where RawTable fails too). A
// sub-slice of the file has its capacity cut at its end, and its bytes must
// not be modified.
func (s tableSource) table(tag ot.Tag) []byte {
	if s.data == nil {
		b, _ := s.ld.RawTable(tag)
		return b
	}
	// The first record of a tag wins, as in go-text (parseOTF).
	for r := s.dir; len(r) >= 16; r = r[16:] {
		if ot.Tag(binary.BigEndian.Uint32(r)) != tag {
			continue
		}
		off, n := uint64(binary.BigEndian.Uint32(r[8:])), uint64(binary.BigEndian.Uint32(r[12:]))
		if off+n > uint64(len(s.data)) {
			return nil
		}
		return s.data[off : off+n : off+n]
	}
	return nil
}

// sfntDirectory returns the table records of the index-th font of data
// when data is a plain sfnt or a TrueType/OpenType collection, the formats
// whose table offsets are offsets into the file. It returns nil for any
// other format or a truncated header.
func sfntDirectory(data []byte, index int) []byte {
	u32 := func(off uint64) (uint32, bool) {
		if off+4 > uint64(len(data)) {
			return 0, false
		}
		return binary.BigEndian.Uint32(data[off:]), true
	}
	magic, ok := u32(0)
	if !ok {
		return nil
	}
	var font uint64 // offset of the font's offset table
	switch ot.Tag(magic) {
	case ot.TrueType, ot.OpenType, ot.AppleTrueType, ot.PostScript1:
		if index != 0 {
			return nil
		}
	case ot.MustNewTag("ttcf"):
		n, ok := u32(8)
		if !ok || index < 0 || uint64(index) >= uint64(n) {
			return nil
		}
		o, ok := u32(12 + 4*uint64(index))
		if !ok {
			return nil
		}
		font = uint64(o)
		if m, ok := u32(font); !ok || (ot.Tag(m) != ot.TrueType && ot.Tag(m) != ot.OpenType && ot.Tag(m) != ot.AppleTrueType && ot.Tag(m) != ot.PostScript1) {
			return nil
		}
	default: // WOFF, dfont
		return nil
	}
	if font+12 > uint64(len(data)) {
		return nil
	}
	numTables := uint64(binary.BigEndian.Uint16(data[font+4:]))
	start, end := font+12, font+12+16*numTables
	if end > uint64(len(data)) {
		return nil
	}
	return data[start:end:end]
}
