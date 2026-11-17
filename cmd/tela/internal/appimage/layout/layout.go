// Package layout is the AppImage layout that tela package writes and
// Tela's AppImage runtime reads (ADR-295): an ELF runtime with the type 2
// magic, the sections that AppImage tools look for, a section of Tela's
// own that identifies the payload, and the squashfs image of the payload
// appended where the ELF file ends.
//
// The AppImage type 2 specification puts the magic "AI\x02" at offset 8
// (the ELF header's padding, which loaders ignore) and the squashfs image
// right after the ELF file, whose size is the end of the section header
// table or of the last section, whichever is further, as the upstream
// runtime's --appimage-offset computes it. Tools read the update
// information from the section .upd_info and the signature from
// .sha256_sig and .sig_key.
//
// The Go linker cannot add named sections, so AddSections rewrites a
// linked ELF file: the new sections' data is appended, then a section name
// table that adds their names, then a new section header table, which ends
// the file. Only 64-bit little-endian ELF files are handled (amd64, arm64,
// riscv64, loong64, ppc64le).
package layout

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// The sections of an AppImage.
const (
	// UpdateInfo holds the update information string (AppImageUpdate's
	// zsync format), NUL-padded.
	UpdateInfo     = ".upd_info"
	UpdateInfoSize = 1024
	// Signature and SigKey hold an embedded signature and its key; Tela
	// leaves them zeroed (unsigned) at the sizes the upstream runtime has.
	Signature     = ".sha256_sig"
	SignatureSize = 1024
	SigKey        = ".sig_key"
	SigKeySize    = 8192
	// Payload is Tela's own section: an [Info].
	Payload     = ".tela_payload"
	PayloadSize = 512
)

// Magic is what an AppImage of type 2 has at offset 8.
var Magic = [3]byte{'A', 'I', 2}

// Info identifies the payload: where its squashfs image is in the file,
// its size and SHA-256 (of the bytes as appended, the padding included),
// and the app's ID and version, which name its folder in the cache.
type Info struct {
	Offset, Size uint64
	SHA256       [32]byte
	AppID        string
	Version      string
}

var infoMagic = [8]byte{'T', 'E', 'L', 'A', 'A', 'I', 0, 1}

// Marshal returns the info as the Payload section's PayloadSize bytes.
func (in Info) Marshal() ([]byte, error) {
	if len(in.AppID) > 255 || len(in.Version) > 128 {
		return nil, errors.New("layout: the app ID or the version is too long")
	}
	b := make([]byte, 0, PayloadSize)
	b = append(b, infoMagic[:]...)
	b = binary.LittleEndian.AppendUint64(b, in.Offset)
	b = binary.LittleEndian.AppendUint64(b, in.Size)
	b = append(b, in.SHA256[:]...)
	b = binary.LittleEndian.AppendUint16(b, uint16(len(in.AppID)))
	b = append(b, in.AppID...)
	b = binary.LittleEndian.AppendUint16(b, uint16(len(in.Version)))
	b = append(b, in.Version...)
	return append(b, make([]byte, PayloadSize-len(b))...), nil
}

// UnmarshalInfo reads a Payload section.
func UnmarshalInfo(b []byte) (Info, error) {
	var in Info
	bad := errors.New("layout: the payload section is not Tela's")
	if len(b) < 8+8+8+32+2 || !bytes.Equal(b[:8], infoMagic[:]) {
		return in, bad
	}
	le := binary.LittleEndian
	in.Offset, in.Size = le.Uint64(b[8:]), le.Uint64(b[16:])
	copy(in.SHA256[:], b[24:56])
	rest := b[56:]
	str := func() (string, bool) {
		if len(rest) < 2 {
			return "", false
		}
		n := int(le.Uint16(rest))
		if len(rest) < 2+n {
			return "", false
		}
		s := string(rest[2 : 2+n])
		rest = rest[2+n:]
		return s, true
	}
	var ok1, ok2 bool
	in.AppID, ok1 = str()
	in.Version, ok2 = str()
	if !ok1 || !ok2 {
		return in, bad
	}
	return in, nil
}

// File is what Read finds in an ELF file.
type File struct {
	// End is where the ELF file ends: the AppImage's payload offset.
	End uint64
	// Sections are the contents of the sections asked for, by name.
	Sections map[string][]byte
	// Magic says the file has the AppImage type 2 magic.
	Magic bool
}

const (
	ehdrSize = 64
	shdrSize = 64
)

// header is what Read and AddSections use of the ELF header.
type header struct {
	shoff           uint64
	shnum, shstrndx uint16
	shentsize       uint16
}

func readHeader(b []byte) (header, error) {
	if len(b) < ehdrSize || string(b[:4]) != "\x7fELF" {
		return header{}, errors.New("layout: not an ELF file")
	}
	if b[4] != 2 || b[5] != 1 {
		return header{}, errors.New("layout: not a 64-bit little-endian ELF file")
	}
	le := binary.LittleEndian
	h := header{
		shoff:     le.Uint64(b[0x28:]),
		shentsize: le.Uint16(b[0x3a:]),
		shnum:     le.Uint16(b[0x3c:]),
		shstrndx:  le.Uint16(b[0x3e:]),
	}
	if h.shnum != 0 && h.shentsize != shdrSize {
		return header{}, fmt.Errorf("layout: section headers of %d bytes", h.shentsize)
	}
	if h.shnum >= 0xff00 || h.shstrndx >= 0xff00 {
		return header{}, errors.New("layout: extended section numbering is not handled")
	}
	return h, nil
}

// section is a section header.
type section struct {
	name               uint32
	typ                uint32
	flags, addr        uint64
	offset, size       uint64
	link, info         uint32
	addralign, entsize uint64
}

func readSection(b []byte) section {
	le := binary.LittleEndian
	return section{
		name: le.Uint32(b), typ: le.Uint32(b[4:]), flags: le.Uint64(b[8:]), addr: le.Uint64(b[16:]),
		offset: le.Uint64(b[24:]), size: le.Uint64(b[32:]), link: le.Uint32(b[40:]), info: le.Uint32(b[44:]),
		addralign: le.Uint64(b[48:]), entsize: le.Uint64(b[56:]),
	}
}

func (s section) append(b []byte) []byte {
	le := binary.LittleEndian
	b = le.AppendUint32(b, s.name)
	b = le.AppendUint32(b, s.typ)
	b = le.AppendUint64(b, s.flags)
	b = le.AppendUint64(b, s.addr)
	b = le.AppendUint64(b, s.offset)
	b = le.AppendUint64(b, s.size)
	b = le.AppendUint32(b, s.link)
	b = le.AppendUint32(b, s.info)
	b = le.AppendUint64(b, s.addralign)
	return le.AppendUint64(b, s.entsize)
}

const shtNobits = 8

// elfEnd is the size of the ELF file: the end of the section header
// table or of the last section in it, whichever is further (the upstream
// runtime's computation).
func elfEnd(h header, last section) uint64 {
	end := h.shoff + uint64(h.shentsize)*uint64(h.shnum)
	if last.typ != shtNobits && last.offset+last.size > end {
		end = last.offset + last.size
	}
	return end
}

// maxRead bounds the section contents Read reads.
const maxRead = 1 << 20

// Read reads the ELF header and section headers of r, which is size bytes
// long, and the contents of the named sections that it has.
func Read(r io.ReaderAt, size int64, names ...string) (*File, error) {
	var eh [ehdrSize]byte
	if _, err := r.ReadAt(eh[:], 0); err != nil {
		return nil, fmt.Errorf("layout: reading the ELF header: %v", err)
	}
	h, err := readHeader(eh[:])
	if err != nil {
		return nil, err
	}
	out := &File{Sections: map[string][]byte{}, Magic: eh[8] == Magic[0] && eh[9] == Magic[1] && eh[10] == Magic[2]}
	if h.shnum == 0 {
		return nil, errors.New("layout: the ELF file has no section headers")
	}
	tsize := uint64(h.shnum) * shdrSize
	if h.shoff > uint64(size) || tsize > uint64(size)-h.shoff {
		return nil, errors.New("layout: the section headers are outside the file")
	}
	table := make([]byte, tsize)
	if _, err := r.ReadAt(table, int64(h.shoff)); err != nil {
		return nil, err
	}
	secs := make([]section, h.shnum)
	for i := range secs {
		secs[i] = readSection(table[i*shdrSize:])
	}
	out.End = elfEnd(h, secs[len(secs)-1])
	if int(h.shstrndx) >= len(secs) {
		return nil, errors.New("layout: no section name table")
	}
	read := func(s section) ([]byte, error) {
		if s.typ == shtNobits {
			return nil, nil
		}
		if s.size > maxRead || s.offset > uint64(size) || s.size > uint64(size)-s.offset {
			return nil, errors.New("layout: a section is outside the file")
		}
		b := make([]byte, s.size)
		_, err := r.ReadAt(b, int64(s.offset))
		return b, err
	}
	strtab, err := read(secs[h.shstrndx])
	if err != nil {
		return nil, err
	}
	for _, s := range secs {
		name := cstring(strtab, s.name)
		for _, want := range names {
			if name == want {
				if out.Sections[name], err = read(s); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

// cstring returns the NUL-terminated string at off in tab.
func cstring(tab []byte, off uint32) string {
	if int(off) >= len(tab) {
		return ""
	}
	s := tab[off:]
	if i := bytes.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return string(s)
}

// Section is a section to add.
type Section struct {
	Name  string
	Data  []byte
	Align uint64
}

// AddSections returns elf with secs added (see the package comment) and
// the AppImage magic set, and the offset of each new section's data. The
// new sections are PROGBITS that are not loaded; the program is
// unchanged.
func AddSections(elf []byte, secs []Section) ([]byte, []uint64, error) {
	h, err := readHeader(elf)
	if err != nil {
		return nil, nil, err
	}
	if h.shnum == 0 {
		return nil, nil, errors.New("layout: the ELF file has no section headers")
	}
	tsize := uint64(h.shnum) * shdrSize
	if h.shoff > uint64(len(elf)) || tsize > uint64(len(elf))-h.shoff {
		return nil, nil, errors.New("layout: the section headers are outside the file")
	}
	old := make([]section, h.shnum)
	for i := range old {
		old[i] = readSection(elf[h.shoff+uint64(i)*shdrSize:])
	}
	if int(h.shstrndx) >= len(old) {
		return nil, nil, errors.New("layout: no section name table")
	}
	st := old[h.shstrndx]
	if st.offset > uint64(len(elf)) || st.size > uint64(len(elf))-st.offset {
		return nil, nil, errors.New("layout: the section name table is outside the file")
	}
	strtab := bytes.Clone(elf[st.offset : st.offset+st.size])
	for _, s := range secs {
		if bytes.Contains(strtab, append([]byte("\x00"+s.Name), 0)) {
			return nil, nil, fmt.Errorf("layout: the ELF file already has a section %s", s.Name)
		}
	}

	// The old table goes when it ends the file, as the linker writes it.
	body := elf
	if h.shoff+tsize == uint64(len(elf)) {
		body = elf[:h.shoff]
	}
	out := bytes.Clone(body)
	out[8], out[9], out[10] = Magic[0], Magic[1], Magic[2]
	var offsets []uint64
	var added []section
	for _, s := range secs {
		align := max(s.Align, 1)
		for uint64(len(out))%align != 0 {
			out = append(out, 0)
		}
		off := uint64(len(out))
		offsets = append(offsets, off)
		out = append(out, s.Data...)
		added = append(added, section{name: uint32(len(strtab)), typ: 1, offset: off, size: uint64(len(s.Data)), addralign: align})
		strtab = append(append(strtab, s.Name...), 0)
	}
	st.offset, st.size = uint64(len(out)), uint64(len(strtab))
	out = append(out, strtab...)
	for len(out)%8 != 0 {
		out = append(out, 0)
	}
	shoff := uint64(len(out))
	old[h.shstrndx] = st
	for _, s := range append(old, added...) {
		out = s.append(out)
	}
	le := binary.LittleEndian
	le.PutUint64(out[0x28:], shoff)
	le.PutUint16(out[0x3c:], uint16(len(old)+len(added)))
	return out, offsets, nil
}
