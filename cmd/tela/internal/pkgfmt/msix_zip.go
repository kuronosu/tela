package pkgfmt

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
	"time"
)

// The zip of an MSIX package, written as the MSIX SDK's packer
// (github.com/microsoft/msix-packaging, src/msix/pack) writes it:
//
//   - Entries in order: the payload, AppxManifest.xml, AppxBlockMap.xml,
//     [Content_Types].xml. No folder entries (a package has parts, not
//     folders), names percent-encoded (opcName).
//   - Local file headers with the sizes and CRC (version 20, no flags, no
//     data descriptor, no extra field), so the block map's LfhSize is 30
//     plus the name's length; central directory headers made by version
//     45; and always the zip64 end of central directory record and its
//     locator, with the classic end record's fields at their maximum, as
//     the SDK writes it. Files of 4 GiB or more are refused.
//   - A deflated file is compressed in blocks of 64 KiB of its data, each
//     on its own (a new compressor per block, ending in a sync flush: the
//     SDK's Z_FULL_FLUSH), so each block's bytes inflate alone; the
//     stream ends with an empty final block (03 00) that no block counts,
//     like the SDK's Z_FINISH. The block map's Block Size is the
//     compressed size of each block; a stored file's blocks have none.
//   - AppxBlockMap.xml lists every payload file and the manifest, with the
//     SHA-256 of each 64 KiB block of its data in base64; the names use
//     '\' and are not encoded.

// blockSize is the block map's block: 64 KiB of a file's data.
const blockSize = 64 << 10

// opcPart is a file of the package.
type opcPart struct {
	name    string // slash-separated path in the package
	data    []byte
	deflate bool
}

// blockMapFile is a File of the block map.
type blockMapFile struct {
	name    string // the package path, slash-separated
	size    int
	lfhSize int
	blocks  []blockMapBlock
}

type blockMapBlock struct {
	hash string // base64 SHA-256 of the block's data
	size int    // compressed size; 0 for a stored file
}

// zipEntry is what the central directory needs of an entry.
type zipEntry struct {
	name              string
	method            uint16
	crc               uint32
	csize, usize, off uint64
}

type opcWriter struct {
	buf        bytes.Buffer
	time, date uint16
	entries    []zipEntry
}

// dosTime is t as MS-DOS time and date fields; 1980-01-01 at the
// earliest.
func dosTime(t time.Time) (uint16, uint16) {
	t = t.UTC()
	if t.Year() < 1980 {
		t = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return uint16(t.Hour()<<11 | t.Minute()<<5 | t.Second()/2),
		uint16((t.Year()-1980)<<9 | int(t.Month())<<5 | t.Day())
}

// deflateBlocks compresses data in independent blocks of blockSize and
// returns the stream and the compressed size of each block.
func deflateBlocks(data []byte) ([]byte, []int, error) {
	var out bytes.Buffer
	var sizes []int
	for off := 0; off < len(data); off += blockSize {
		end := min(off+blockSize, len(data))
		start := out.Len()
		w, err := flate.NewWriter(&out, flate.BestCompression)
		if err != nil {
			return nil, nil, err
		}
		if _, err := w.Write(data[off:end]); err != nil {
			return nil, nil, err
		}
		if err := w.Flush(); err != nil {
			return nil, nil, err
		}
		sizes = append(sizes, out.Len()-start)
	}
	// The final block: BFINAL set, fixed Huffman codes, no data.
	out.Write([]byte{0x03, 0x00})
	return out.Bytes(), sizes, nil
}

// add writes a file and returns its entry in the block map.
func (w *opcWriter) add(p opcPart) (blockMapFile, error) {
	name := opcName(p.name)
	if p.name == "[Content_Types].xml" {
		name = p.name // not a part: its name is written as it is
	}
	bf := blockMapFile{name: p.name, size: len(p.data), lfhSize: 30 + len(name)}
	stored := p.data
	method := uint16(0)
	var sizes []int
	if p.deflate {
		var err error
		if stored, sizes, err = deflateBlocks(p.data); err != nil {
			return bf, err
		}
		method = 8
	}
	for i, off := 0, 0; off < len(p.data); i, off = i+1, off+blockSize {
		sum := sha256.Sum256(p.data[off:min(off+blockSize, len(p.data))])
		b := blockMapBlock{hash: base64.StdEncoding.EncodeToString(sum[:])}
		if p.deflate {
			b.size = sizes[i]
		}
		bf.blocks = append(bf.blocks, b)
	}
	off := uint64(w.buf.Len())
	if len(p.data) >= 1<<32-1 || len(stored) >= 1<<32-1 || off >= 1<<32-1 {
		return bf, fmt.Errorf("%s: an MSIX package of 4 GiB or more is not supported", p.name)
	}
	e := zipEntry{name: name, method: method, crc: crc32.ChecksumIEEE(p.data), csize: uint64(len(stored)), usize: uint64(len(p.data)), off: off}
	le := binary.LittleEndian
	h := make([]byte, 30, 30+len(name))
	le.PutUint32(h[0:], 0x04034b50)
	le.PutUint16(h[4:], 20) // version needed
	le.PutUint16(h[6:], 0)  // flags
	le.PutUint16(h[8:], e.method)
	le.PutUint16(h[10:], w.time)
	le.PutUint16(h[12:], w.date)
	le.PutUint32(h[14:], e.crc)
	le.PutUint32(h[18:], uint32(e.csize))
	le.PutUint32(h[22:], uint32(e.usize))
	le.PutUint16(h[26:], uint16(len(name)))
	le.PutUint16(h[28:], 0) // extra field
	w.buf.Write(append(h, name...))
	w.buf.Write(stored)
	w.entries = append(w.entries, e)
	return bf, nil
}

// finish writes the central directory and the end records.
func (w *opcWriter) finish() []byte {
	le := binary.LittleEndian
	cdOff := uint64(w.buf.Len())
	for _, e := range w.entries {
		h := make([]byte, 46)
		le.PutUint32(h[0:], 0x02014b50)
		le.PutUint16(h[4:], 45) // made by (MS-DOS, 4.5)
		le.PutUint16(h[6:], 20) // version needed
		le.PutUint16(h[8:], 0)
		le.PutUint16(h[10:], e.method)
		le.PutUint16(h[12:], w.time)
		le.PutUint16(h[14:], w.date)
		le.PutUint32(h[16:], e.crc)
		le.PutUint32(h[20:], uint32(e.csize))
		le.PutUint32(h[24:], uint32(e.usize))
		le.PutUint16(h[28:], uint16(len(e.name)))
		// extra, comment, disk, internal and external attributes: 0.
		le.PutUint32(h[42:], uint32(e.off))
		w.buf.Write(h)
		w.buf.WriteString(e.name)
	}
	cdSize := uint64(w.buf.Len()) - cdOff
	n := uint64(len(w.entries))
	// The zip64 end of central directory record.
	z64 := uint64(w.buf.Len())
	r := make([]byte, 56)
	le.PutUint32(r[0:], 0x06064b50)
	le.PutUint64(r[4:], 44)
	le.PutUint16(r[12:], 45)
	le.PutUint16(r[14:], 45)
	le.PutUint64(r[24:], n)
	le.PutUint64(r[32:], n)
	le.PutUint64(r[40:], cdSize)
	le.PutUint64(r[48:], cdOff)
	w.buf.Write(r)
	// Its locator.
	l := make([]byte, 20)
	le.PutUint32(l[0:], 0x07064b50)
	le.PutUint64(l[8:], z64)
	le.PutUint32(l[16:], 1)
	w.buf.Write(l)
	// The end of central directory record, pointing to the zip64 one.
	e := make([]byte, 22)
	le.PutUint32(e[0:], 0x06054b50)
	le.PutUint16(e[8:], 0xFFFF)
	le.PutUint16(e[10:], 0xFFFF)
	le.PutUint32(e[12:], 0xFFFFFFFF)
	le.PutUint32(e[16:], 0xFFFFFFFF)
	w.buf.Write(e)
	return w.buf.Bytes()
}

// blockMap returns the AppxBlockMap.xml of files.
func blockMap(files []blockMapFile) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<BlockMap xmlns="http://schemas.microsoft.com/appx/2010/blockmap" HashMethod="http://www.w3.org/2001/04/xmlenc#sha256">` + "\n")
	for _, f := range files {
		fmt.Fprintf(&b, `<File Name="%s" Size="%d" LfhSize="%d">`, xmlText(winPath(f.name)), f.size, f.lfhSize)
		for _, bl := range f.blocks {
			if bl.size > 0 {
				fmt.Fprintf(&b, `<Block Hash="%s" Size="%d"/>`, bl.hash, bl.size)
			} else {
				fmt.Fprintf(&b, `<Block Hash="%s"/>`, bl.hash)
			}
		}
		b.WriteString("</File>\n")
	}
	b.WriteString("</BlockMap>\n")
	return []byte(b.String())
}

// writeMSIX returns the package of the payload parts and the manifest,
// every entry with the time when.
func writeMSIX(when time.Time, payload []opcPart, manifest []byte) ([]byte, error) {
	w := &opcWriter{}
	w.time, w.date = dosTime(when)
	var files []blockMapFile
	var names []string
	reserved := []string{"appxmanifest.xml", "appxblockmap.xml", "[content_types].xml", "appxsignature.p7x"}
	for _, p := range payload {
		if contains(reserved, strings.ToLower(p.name)) || strings.HasPrefix(strings.ToLower(p.name), "appxmetadata/") {
			return nil, fmt.Errorf("the package cannot hold a file named %s", p.name)
		}
		if p.name == "" || strings.HasPrefix(p.name, "/") || strings.HasSuffix(p.name, "/") || strings.Contains(p.name, "//") {
			return nil, errors.New("a package file has an empty name part")
		}
		f, err := w.add(p)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
		names = append(names, p.name)
	}
	f, err := w.add(opcPart{name: "AppxManifest.xml", data: manifest, deflate: true})
	if err != nil {
		return nil, err
	}
	files = append(files, f)
	if _, err := w.add(opcPart{name: "AppxBlockMap.xml", data: blockMap(files), deflate: true}); err != nil {
		return nil, err
	}
	if _, err := w.add(opcPart{name: "[Content_Types].xml", data: contentTypes(names), deflate: true}); err != nil {
		return nil, err
	}
	return w.finish(), nil
}
