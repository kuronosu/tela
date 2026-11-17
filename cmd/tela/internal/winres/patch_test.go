package winres

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSection(t *testing.T) {
	info := Info{ProductName: "Notes", CompanyName: "Example Inc.", FileDescription: "Take notes", Version: "1.2.3-rc.1", FileName: "notes.exe"}
	res := Resources(testIcons(), info)
	if len(res) != 4 { // two icons, the group, the version
		t.Fatalf("%d resources, want 4", len(res))
	}
	const rva = 0x5000
	sec, err := Section(res, rva)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Section(res, rva)
	if !bytes.Equal(sec, again) {
		t.Error("the same resources gave different bytes")
	}
	got, err := Read(sec, rva)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(res) {
		t.Fatalf("read %d resources, want %d", len(got), len(res))
	}
	byKey := func(rs []Resource) map[[3]uint32][]byte {
		m := map[[3]uint32][]byte{}
		for _, r := range rs {
			m[[3]uint32{r.Type, r.Name, r.Lang}] = r.Data
		}
		return m
	}
	if !reflect.DeepEqual(byKey(got), byKey(res)) {
		t.Errorf("the resources read back differ from the ones written")
	}
	// The directory is sorted by type, then name.
	var order [][2]uint32
	for _, r := range got {
		order = append(order, [2]uint32{r.Type, r.Name})
	}
	if !reflect.DeepEqual(order, [][2]uint32{{RTIcon, 1}, {RTIcon, 2}, {RTGroupIcon, 1}, {RTVersion, 1}}) {
		t.Errorf("order %v", order)
	}
	if _, err := Section(append(res, res[0]), rva); err == nil || !strings.Contains(err.Error(), "two resources") {
		t.Errorf("duplicate resource: %v", err)
	}
	// Reading at the wrong address finds data outside the section.
	if _, err := Read(sec, 0); err == nil {
		t.Error("Read at the wrong address succeeded")
	}
}

// fakePE writes a PE executable like the Go linker's: a header with room
// for sixteen section headers, and nsect sections of 0x200 bytes at
// 0x1000-byte steps.
func fakePE(nsect int, pe32plus bool) []byte {
	le := binary.LittleEndian
	optSize := 224
	if pe32plus {
		optSize = 240
	}
	const headers = 0x600
	out := make([]byte, headers+0x200*nsect)
	out[0], out[1] = 'M', 'Z'
	le.PutUint32(out[0x3c:], 0x80)
	copy(out[0x80:], "PE\x00\x00")
	coff := 0x84
	le.PutUint16(out[coff:], 0x8664)
	le.PutUint16(out[coff+2:], uint16(nsect))
	le.PutUint16(out[coff+16:], uint16(optSize))
	opt := coff + 20
	if pe32plus {
		le.PutUint16(out[opt:], 0x20b)
		le.PutUint32(out[opt+108:], 16)
	} else {
		le.PutUint16(out[opt:], 0x10b)
		le.PutUint32(out[opt+92:], 16)
	}
	le.PutUint32(out[opt+32:], 0x1000)
	le.PutUint32(out[opt+36:], 0x200)
	le.PutUint32(out[opt+8:], 0x200*uint32(nsect))
	le.PutUint32(out[opt+56:], 0x1000*uint32(nsect+1))
	le.PutUint32(out[opt+60:], headers)
	table := opt + optSize
	for i := 0; i < nsect; i++ {
		h := out[table+40*i:]
		copy(h, ".text")
		le.PutUint32(h[8:], 0x180)
		le.PutUint32(h[12:], 0x1000*uint32(i+1))
		le.PutUint32(h[16:], 0x200)
		le.PutUint32(h[20:], headers+0x200*uint32(i))
		le.PutUint32(h[36:], 0x60000020)
		copy(out[headers+0x200*i:], "section data")
	}
	return out
}

func TestPatch(t *testing.T) {
	res := Resources(testIcons(), Info{ProductName: "Notes", Version: "1.0.0", FileName: "notes.exe"})
	for _, plus := range []bool{true, false} {
		for _, nsect := range []int{1, 16} {
			exe := fakePE(nsect, plus)
			out, err := Patch(exe, res)
			if err != nil {
				t.Fatalf("pe32+ %v, %d sections: %v", plus, nsect, err)
			}
			again, _ := Patch(exe, res)
			if !bytes.Equal(out, again) {
				t.Error("the same inputs gave different bytes")
			}
			if !bytes.Equal(out[0x600:len(exe)], exe[0x600:]) {
				t.Error("Patch changed the sections' data")
			}
			f, err := pe.NewFile(bytes.NewReader(out))
			if err != nil {
				t.Fatalf("debug/pe: %v", err)
			}
			if len(f.Sections) != nsect+1 {
				t.Fatalf("%d sections, want %d", len(f.Sections), nsect+1)
			}
			sec := f.Sections[nsect]
			if sec.Name != ".rsrc" || sec.VirtualAddress != 0x1000*uint32(nsect+1) || sec.Characteristics != 0x40000040 || sec.Offset%0x200 != 0 || sec.Size%0x200 != 0 {
				t.Errorf("new section %+v", sec.SectionHeader)
			}
			var dir pe.DataDirectory
			var sizeOfImage uint32
			switch oh := f.OptionalHeader.(type) {
			case *pe.OptionalHeader64:
				dir, sizeOfImage = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE], oh.SizeOfImage
			case *pe.OptionalHeader32:
				dir, sizeOfImage = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE], oh.SizeOfImage
			}
			if dir.VirtualAddress != sec.VirtualAddress || dir.Size != sec.VirtualSize {
				t.Errorf("resource directory %+v, want the section's %#x and %#x", dir, sec.VirtualAddress, sec.VirtualSize)
			}
			if sizeOfImage != alignUp(sec.VirtualAddress+sec.VirtualSize, 0x1000) {
				t.Errorf("SizeOfImage %#x", sizeOfImage)
			}
			data, err := sec.Data()
			if err != nil {
				t.Fatal(err)
			}
			got, err := Read(data[:sec.VirtualSize], sec.VirtualAddress)
			if err != nil || len(got) != len(res) {
				t.Errorf("reading the resources back: %d, %v", len(got), err)
			}
		}
	}
}

func TestPatchRefusals(t *testing.T) {
	res := Resources(nil, Info{ProductName: "Notes", FileName: "notes.exe"})
	le := binary.LittleEndian
	const dirs = 0x84 + 20 + 112 // the data directories of fakePE(_, true)
	withDir := fakePE(2, true)
	le.PutUint32(withDir[dirs+8*2+4:], 0x100)
	signed := fakePE(2, true)
	le.PutUint32(signed[dirs+8*4+4:], 0x100)
	named := fakePE(2, true)
	copy(named[0x84+20+240+40:], ".rsrc\x00\x00\x00")
	full := fakePE(2, true)
	le.PutUint32(full[0x84+20+240+20:], 0x84+20+240+40*2+20) // the first section's data starts inside the next header
	for _, tc := range []struct {
		name string
		exe  []byte
		want string
	}{
		{"resources", withDir, "already has resources"},
		{"named", named, "already has resources"},
		{"signed", signed, "is signed"},
		{"no room", full, "no room for another section"},
		{"not a PE", []byte("hello world, this is not an executable at all"), "no MZ header"},
		{"cut short", fakePE(2, true)[:0x70], "no PE signature"},
	} {
		if _, err := Patch(tc.exe, res); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
	if _, err := Patch(withDir, res); !errors.Is(err, ErrHasResources) {
		t.Errorf("an executable with resources: %v, want ErrHasResources", err)
	}
}
