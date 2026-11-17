// Package winres writes the resources of a Windows executable as a COFF
// object, a .syso file, which the Go linker adds to the .rsrc section of the
// program (ADR-294): the icon (RT_GROUP_ICON and RT_ICON) and the version
// information (RT_VERSION) that Explorer, the task bar and Task Manager
// show. It is a writer of the resource directory format for what tela build
// needs; Read is its inverse, which the tests use.
//
// The bytes depend only on the inputs.
package winres

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/kuronosu/tela/cmd/tela/internal/appicon"
)

// The resource types.
const (
	RTIcon      = 3
	RTGroupIcon = 14
	RTVersion   = 16
	RTManifest  = 24
)

// LangEnglishUS is the language of the resources (0x0409).
const LangEnglishUS = 0x0409

// Resource is one entry of the resource directory, by numeric type, name
// (ID) and language.
type Resource struct {
	Type, Name, Lang uint32
	Data             []byte
}

// Info is what the executable's version information says.
type Info struct {
	// ProductName is the app's name (App.Title).
	ProductName string
	// CompanyName is the publisher (App.Publisher); left out when empty.
	CompanyName string
	// FileDescription is what Task Manager shows (App.Description); the
	// product name when empty.
	FileDescription string
	// Version is the app's version, such as "1.2.3" or "1.2.3-rc.1"; its
	// numeric part is the file version, "0.0.0" when empty.
	Version string
	// FileName is the executable's name ("notes.exe").
	FileName string
}

// Resources returns the resources of an executable with the icon images
// (PNG-compressed, from appicon) and the version information; icons may be
// empty.
func Resources(icons []appicon.Image, info Info) []Resource {
	var res []Resource
	if len(icons) > 0 {
		var group bytes.Buffer
		le := binary.LittleEndian
		binary.Write(&group, le, [3]uint16{0, 1, uint16(len(icons))})
		for i, ic := range icons {
			id := uint32(i + 1)
			dim := byte(ic.Size) // 256 is 0
			group.Write([]byte{dim, dim, 0, 0})
			binary.Write(&group, le, [2]uint16{1, 32})
			binary.Write(&group, le, uint32(len(ic.PNG)))
			binary.Write(&group, le, uint16(id))
			res = append(res, Resource{Type: RTIcon, Name: id, Lang: LangEnglishUS, Data: ic.PNG})
		}
		res = append(res, Resource{Type: RTGroupIcon, Name: 1, Lang: LangEnglishUS, Data: group.Bytes()})
	}
	res = append(res, Resource{Type: RTVersion, Name: 1, Lang: LangEnglishUS, Data: versionInfo(info)})
	return res
}

// numericVersion returns the four numbers of the file version from the
// numeric start of v ("1.2.3-rc.1" is 1, 2, 3, 0), zeros when it has none.
func numericVersion(v string) [4]uint16 {
	var out [4]uint16
	v = strings.TrimPrefix(v, "v")
	for i := 0; i < 4 && v != ""; i++ {
		n := 0
		for n < len(v) && v[n] >= '0' && v[n] <= '9' {
			n++
		}
		if n == 0 {
			break
		}
		x, err := strconv.ParseUint(v[:n], 10, 16)
		if err != nil {
			x = 0xffff
		}
		out[i] = uint16(x)
		v = v[n:]
		if v == "" || v[0] != '.' {
			break
		}
		v = v[1:]
	}
	return out
}

// versionInfo encodes a VS_VERSIONINFO resource.
func versionInfo(info Info) []byte {
	ver := info.Version
	if ver == "" {
		ver = "0.0.0"
	}
	nums := numericVersion(ver)
	ms := uint32(nums[0])<<16 | uint32(nums[1])
	ls := uint32(nums[2])<<16 | uint32(nums[3])
	le := binary.LittleEndian
	fixed := make([]byte, 52)
	for i, v := range []uint32{
		0xFEEF04BD, 0x00010000, // signature, structure version
		ms, ls, ms, ls, // file and product version
		0x3F, 0, // flags mask, flags
		0x00040004, // VOS_NT_WINDOWS32
		1, 0,       // VFT_APP, no subtype
		0, 0, // date
	} {
		le.PutUint32(fixed[4*i:], v)
	}

	desc := info.FileDescription
	if desc == "" {
		desc = info.ProductName
	}
	name := strings.TrimSuffix(info.FileName, filepath.Ext(info.FileName))
	var strs [][]byte
	add := func(k, v string) {
		if v != "" {
			strs = append(strs, stringNode(k, v))
		}
	}
	add("CompanyName", info.CompanyName)
	add("FileDescription", desc)
	add("FileVersion", ver)
	add("InternalName", name)
	add("OriginalFilename", info.FileName)
	add("ProductName", info.ProductName)
	add("ProductVersion", ver)

	table := node("040904B0", 1, nil, 0, strs...) // en-US, Unicode
	stringInfo := node("StringFileInfo", 1, nil, 0, table)
	var translation [4]byte
	le.PutUint32(translation[:], 0x04B00409)
	vars := node("VarFileInfo", 1, nil, 0, node("Translation", 0, translation[:], 4))
	return node("VS_VERSION_INFO", 0, fixed, 52, stringInfo, vars)
}

func stringNode(key, value string) []byte {
	u := utf16z(value)
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return node(key, 1, b, len(u))
}

func utf16z(s string) []uint16 { return append(utf16.Encode([]rune(s)), 0) }

// node encodes a version resource node: wLength, wValueLength, wType, the
// key, the value and the children, each part aligned to 4 bytes from the
// start of the node (which is itself aligned).
func node(key string, typ uint16, value []byte, valueLen int, children ...[]byte) []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 6))
	for _, c := range utf16z(key) {
		binary.Write(&b, binary.LittleEndian, c)
	}
	pad := func() {
		for b.Len()%4 != 0 {
			b.WriteByte(0)
		}
	}
	pad()
	b.Write(value)
	for _, c := range children {
		pad()
		b.Write(c)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out[0:], uint16(len(out)))
	binary.LittleEndian.PutUint16(out[2:], uint16(valueLen))
	binary.LittleEndian.PutUint16(out[4:], typ)
	return out
}

// The machines of the COFF file header and their relocation for the
// address of a resource without the image base (RVA).
var archs = map[string]struct{ machine, rva uint16 }{
	"386":   {0x014c, 7}, // IMAGE_REL_I386_DIR32NB
	"amd64": {0x8664, 3}, // IMAGE_REL_AMD64_ADDR32NB
	"arm64": {0xAA64, 2}, // IMAGE_REL_ARM64_ADDR32NB
}

// Arch reports whether Object writes for the GOARCH goarch.
func Arch(goarch string) bool { _, ok := archs[goarch]; return ok }

// Object returns res as a COFF object file for the GOARCH goarch, the
// content of a .syso file: one .rsrc section with the resource directory,
// whose data entries carry a relocation to the section's start so the
// linker places them.
func Object(goarch string, res []Resource) ([]byte, error) {
	a, ok := archs[goarch]
	if !ok {
		return nil, fmt.Errorf("no Windows resources for GOARCH=%s; tela writes them for amd64, arm64 and 386", goarch)
	}
	sorted := append([]Resource(nil), res...)
	sort.SliceStable(sorted, func(i, j int) bool {
		x, y := sorted[i], sorted[j]
		if x.Type != y.Type {
			return x.Type < y.Type
		}
		if x.Name != y.Name {
			return x.Name < y.Name
		}
		return x.Lang < y.Lang
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Type == sorted[i-1].Type && sorted[i].Name == sorted[i-1].Name && sorted[i].Lang == sorted[i-1].Lang {
			return nil, fmt.Errorf("two resources have type %d, name %d and language %#x", sorted[i].Type, sorted[i].Name, sorted[i].Lang)
		}
	}
	data, relocs := directory(sorted)

	var out bytes.Buffer
	le := binary.LittleEndian
	const headers = 20 + 40
	relocAt := headers + len(data)
	symAt := relocAt + 10*len(relocs)
	// File header.
	binary.Write(&out, le, a.machine)
	binary.Write(&out, le, uint16(1))       // sections
	binary.Write(&out, le, uint32(0))       // time stamp: none, for the same bytes every time
	binary.Write(&out, le, uint32(symAt))   // symbol table
	binary.Write(&out, le, uint32(1))       // symbols
	binary.Write(&out, le, [2]uint16{0, 0}) // no optional header, no flags
	// Section header.
	out.WriteString(".rsrc\x00\x00\x00")
	binary.Write(&out, le, [2]uint32{0, 0}) // virtual size and address
	binary.Write(&out, le, uint32(len(data)))
	binary.Write(&out, le, uint32(headers))
	binary.Write(&out, le, uint32(relocAt))
	binary.Write(&out, le, uint32(0)) // line numbers
	binary.Write(&out, le, uint16(len(relocs)))
	binary.Write(&out, le, uint16(0))
	binary.Write(&out, le, uint32(0x40000040)) // initialized data, readable
	out.Write(data)
	for _, off := range relocs {
		binary.Write(&out, le, off)
		binary.Write(&out, le, uint32(0)) // the section's symbol
		binary.Write(&out, le, a.rva)
	}
	// The symbol of the section, and an empty string table.
	out.WriteString(".rsrc\x00\x00\x00")
	binary.Write(&out, le, uint32(0))
	binary.Write(&out, le, uint16(1)) // section 1
	binary.Write(&out, le, uint16(0))
	out.Write([]byte{3, 0}) // IMAGE_SYM_CLASS_STATIC, no auxiliary symbols
	binary.Write(&out, le, uint32(4))
	return out.Bytes(), nil
}

// directory lays out the three levels of the resource directory (type,
// name, language), the data entries and the data of the resources, sorted
// by type, name and language. It returns the section's content and the
// offsets of the data entries' RVA fields, which need a relocation.
func directory(res []Resource) (data []byte, relocs []uint32) {
	// group is a type or a name: its ID, and the resources under it.
	type group struct {
		id    uint32
		items []int
	}
	var types []group
	names := map[uint32][]group{} // the names of each type
	for i, r := range res {
		if len(types) == 0 || types[len(types)-1].id != r.Type {
			types = append(types, group{id: r.Type})
		}
		ns := names[r.Type]
		if len(ns) == 0 || ns[len(ns)-1].id != r.Name {
			ns = append(ns, group{id: r.Name})
		}
		ns[len(ns)-1].items = append(ns[len(ns)-1].items, i)
		names[r.Type] = ns
	}

	dirSize := func(n int) int { return 16 + 8*n }
	// Offsets: root, the type directories, the name directories, the data
	// entries, the data.
	off := dirSize(len(types))
	typeOff := map[uint32]int{}
	for _, t := range types {
		typeOff[t.id] = off
		off += dirSize(len(names[t.id]))
	}
	type key struct{ typ, name uint32 }
	nameOff := map[key]int{}
	for _, t := range types {
		for _, n := range names[t.id] {
			nameOff[key{t.id, n.id}] = off
			off += dirSize(len(n.items))
		}
	}
	entryOff := make([]int, len(res)) // by index of the resource
	for i := range res {
		entryOff[i] = off
		off += 16
	}
	dataOff := make([]int, len(res))
	for i, r := range res {
		dataOff[i] = off
		off += (len(r.Data) + 7) &^ 7
	}

	var b bytes.Buffer
	le := binary.LittleEndian
	dir := func(n int) {
		binary.Write(&b, le, [2]uint32{0, 0}) // characteristics, time stamp
		binary.Write(&b, le, [2]uint16{0, 0}) // version
		binary.Write(&b, le, uint16(0))       // named entries
		binary.Write(&b, le, uint16(n))       // ID entries
	}
	const sub = 0x80000000
	dir(len(types))
	for _, t := range types {
		binary.Write(&b, le, [2]uint32{t.id, sub | uint32(typeOff[t.id])})
	}
	for _, t := range types {
		dir(len(names[t.id]))
		for _, n := range names[t.id] {
			binary.Write(&b, le, [2]uint32{n.id, sub | uint32(nameOff[key{t.id, n.id}])})
		}
	}
	for _, t := range types {
		for _, n := range names[t.id] {
			dir(len(n.items))
			for _, i := range n.items {
				binary.Write(&b, le, [2]uint32{res[i].Lang, uint32(entryOff[i])})
			}
		}
	}
	for i, r := range res {
		relocs = append(relocs, uint32(b.Len()))
		binary.Write(&b, le, uint32(dataOff[i])) // the RVA, relative to the section until linked
		binary.Write(&b, le, uint32(len(r.Data)))
		binary.Write(&b, le, [2]uint32{0, 0}) // code page, reserved
	}
	for _, r := range res {
		b.Write(r.Data)
		for b.Len()%8 != 0 {
			b.WriteByte(0)
		}
	}
	return b.Bytes(), relocs
}

// Read parses a resource directory: the content of a .rsrc section whose
// first byte has the address rva (0 for the section of a .syso file, the
// section's VirtualAddress in an executable). Names and languages must be
// numeric.
func Read(section []byte, rva uint32) ([]Resource, error) {
	le := binary.LittleEndian
	u32 := func(off uint32) (uint32, error) {
		if uint64(off)+4 > uint64(len(section)) {
			return 0, fmt.Errorf("resource directory: offset %d is outside the %d-byte section", off, len(section))
		}
		return le.Uint32(section[off:]), nil
	}
	entries := func(dir uint32) (ids, offs []uint32, err error) {
		if uint64(dir)+16 > uint64(len(section)) {
			return nil, nil, fmt.Errorf("resource directory: offset %d is outside the %d-byte section", dir, len(section))
		}
		n := uint32(le.Uint16(section[dir+12:])) + uint32(le.Uint16(section[dir+14:]))
		if n > 1<<16 {
			return nil, nil, fmt.Errorf("resource directory: %d entries", n)
		}
		for i := uint32(0); i < n; i++ {
			id, err := u32(dir + 16 + 8*i)
			if err != nil {
				return nil, nil, err
			}
			off, err := u32(dir + 20 + 8*i)
			if err != nil {
				return nil, nil, err
			}
			if id&0x80000000 != 0 {
				return nil, nil, fmt.Errorf("resource directory: a named entry at %d; only numeric IDs are read", dir)
			}
			ids, offs = append(ids, id), append(offs, off)
		}
		return ids, offs, nil
	}
	const sub = 0x80000000
	var out []Resource
	tids, toffs, err := entries(0)
	if err != nil {
		return nil, err
	}
	for ti, tid := range tids {
		nids, noffs, err := entries(toffs[ti] &^ sub)
		if err != nil {
			return nil, err
		}
		for ni, nid := range nids {
			lids, loffs, err := entries(noffs[ni] &^ sub)
			if err != nil {
				return nil, err
			}
			for li, lid := range lids {
				e := loffs[li]
				rv, err := u32(e)
				if err != nil {
					return nil, err
				}
				size, err := u32(e + 4)
				if err != nil {
					return nil, err
				}
				start := rv - rva
				if rv < rva || uint64(start)+uint64(size) > uint64(len(section)) {
					return nil, fmt.Errorf("resource %d/%d/%d: data at %#x (%d bytes) is outside the section", tid, nid, lid, rv, size)
				}
				out = append(out, Resource{Type: tid, Name: nid, Lang: lid, Data: append([]byte(nil), section[start:start+size]...)})
			}
		}
	}
	return out, nil
}
