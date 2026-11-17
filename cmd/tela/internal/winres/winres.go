// Package winres adds resources to a Windows executable (ADR-294): the icon
// (RT_GROUP_ICON and RT_ICON) and the version information (RT_VERSION) that
// Explorer, the task bar and Task Manager show.
//
// The Go linker takes resources from .syso files in the package's folder,
// which the go command opens by their real path (an -overlay does not reach
// them), so tela build, which must not touch that folder, adds the .rsrc
// section to the linked executable instead (Patch). Read is the inverse of
// the directory writer, for tests and checks.
//
// The bytes depend only on the inputs.
package winres

import (
	"bytes"
	"encoding/binary"
	"errors"
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

// Section returns the content of a .rsrc section for res: the resource
// directory (three levels: type, name, language, sorted), the data entries
// and the data, with the address of every data entry resolved for a section
// whose first byte is at rva. Two resources with the same type, name and
// language are an error.
func Section(res []Resource, rva uint32) ([]byte, error) {
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
	return directory(sorted, rva), nil
}

// directory lays out the three levels of the resource directory, the data
// entries and the data of res, sorted by type, name and language.
func directory(res []Resource, rva uint32) []byte {
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
		binary.Write(&b, le, rva+uint32(dataOff[i]))
		binary.Write(&b, le, uint32(len(r.Data)))
		binary.Write(&b, le, [2]uint32{0, 0}) // code page, reserved
	}
	for _, r := range res {
		b.Write(r.Data)
		for b.Len()%8 != 0 {
			b.WriteByte(0)
		}
	}
	return b.Bytes()
}

// ErrHasResources is what Patch returns for an executable that already has
// resources (a .rsrc section or a resource directory): it leaves them as
// they are.
var ErrHasResources = errors.New("the executable already has resources")

// Patch returns the PE executable exe (as the Go linker writes it, for
// 386, amd64 or arm64) with res added as a .rsrc section: a section header
// in the room the linker leaves for them, the section's data after the last
// one, the resource entry of the data directory, and the sizes of the image.
// Nothing else changes, so the bytes depend only on exe and res. It
// refuses (ErrHasResources) an executable with resources of its own, and an
// executable with a signature, which the change would invalidate.
func Patch(exe []byte, res []Resource) ([]byte, error) {
	le := binary.LittleEndian
	bad := func(what string) error { return fmt.Errorf("not a PE executable tela can add resources to: %s", what) }
	if len(exe) < 0x40 || exe[0] != 'M' || exe[1] != 'Z' {
		return nil, bad("no MZ header")
	}
	pe := int(le.Uint32(exe[0x3c:]))
	if pe < 0 || pe+24 > len(exe) || string(exe[pe:pe+4]) != "PE\x00\x00" {
		return nil, bad("no PE signature")
	}
	coff := pe + 4
	nsect := int(le.Uint16(exe[coff+2:]))
	optSize := int(le.Uint16(exe[coff+16:]))
	opt := coff + 20
	if opt+optSize > len(exe) || optSize < 96 {
		return nil, bad("the optional header is cut short")
	}
	dirs := opt + 96 // the data directories of PE32
	switch le.Uint16(exe[opt:]) {
	case 0x10b:
	case 0x20b:
		dirs = opt + 112
	default:
		return nil, bad("unknown optional header")
	}
	const (
		dirResource = 2
		dirSecurity = 4
	)
	ndirs := int(le.Uint32(exe[dirs-4:]))
	if ndirs <= dirSecurity || dirs+8*ndirs > opt+optSize {
		return nil, bad("no data directories for the resources")
	}
	if le.Uint32(exe[dirs+8*dirResource+4:]) != 0 {
		return nil, ErrHasResources
	}
	if le.Uint32(exe[dirs+8*dirSecurity+4:]) != 0 {
		return nil, errors.New("the executable is signed, and adding resources would break the signature; add them before signing")
	}
	secAlign, fileAlign := le.Uint32(exe[opt+32:]), le.Uint32(exe[opt+36:])
	if secAlign == 0 || fileAlign == 0 || secAlign&(secAlign-1) != 0 || fileAlign&(fileAlign-1) != 0 {
		return nil, bad("the alignments are not powers of two")
	}
	table := opt + optSize // the section headers
	if table+40*nsect > len(exe) {
		return nil, bad("the section table is cut short")
	}
	var vaEnd, rawEnd uint32
	firstRaw := uint32(len(exe))
	for i := range nsect {
		h := exe[table+40*i:]
		if string(bytes.TrimRight(h[:8], "\x00")) == ".rsrc" {
			return nil, ErrHasResources
		}
		vsize, va, rawSize, raw := le.Uint32(h[8:]), le.Uint32(h[12:]), le.Uint32(h[16:]), le.Uint32(h[20:])
		vaEnd = max(vaEnd, va+max(vsize, rawSize))
		if rawSize > 0 {
			rawEnd = max(rawEnd, raw+rawSize)
			firstRaw = min(firstRaw, raw)
		}
	}
	newHeader := table + 40*nsect
	if uint32(newHeader+40) > firstRaw || !allZero(exe[newHeader:newHeader+40]) {
		return nil, bad("the headers have no room for another section")
	}
	if uint32(len(exe)) < rawEnd {
		return nil, bad("a section is cut short")
	}

	rva := alignUp(vaEnd, secAlign)
	section, err := Section(res, rva)
	if err != nil {
		return nil, err
	}
	raw := alignUp(uint32(len(exe)), fileAlign)
	rawSize := alignUp(uint32(len(section)), fileAlign)
	out := make([]byte, int(raw)+int(rawSize))
	copy(out, exe)
	copy(out[raw:], section)

	h := out[newHeader:]
	copy(h, ".rsrc")
	le.PutUint32(h[8:], uint32(len(section)))
	le.PutUint32(h[12:], rva)
	le.PutUint32(h[16:], rawSize)
	le.PutUint32(h[20:], raw)
	le.PutUint32(h[36:], 0x40000040) // initialized data, readable
	le.PutUint16(out[coff+2:], uint16(nsect+1))
	le.PutUint32(out[opt+8:], le.Uint32(out[opt+8:])+rawSize) // SizeOfInitializedData
	le.PutUint32(out[opt+56:], alignUp(rva+uint32(len(section)), secAlign))
	le.PutUint32(out[dirs+8*dirResource:], rva)
	le.PutUint32(out[dirs+8*dirResource+4:], uint32(len(section)))
	return out, nil
}

func alignUp(v, a uint32) uint32 { return (v + a - 1) &^ (a - 1) }

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
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
