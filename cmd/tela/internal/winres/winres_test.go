package winres

import (
	"encoding/binary"
	"reflect"
	"testing"
	"unicode/utf16"

	"github.com/kuronosu/tela/cmd/tela/internal/appicon"
)

func testIcons() []appicon.Image {
	return []appicon.Image{
		{Size: 16, PNG: []byte("\x89PNG-sixteen")},
		{Size: 256, PNG: []byte("\x89PNG-two-hundred-fifty-six, an odd length")},
	}
}

func TestGroupIcon(t *testing.T) {
	var group []byte
	for _, r := range Resources(testIcons(), Info{}) {
		if r.Type == RTGroupIcon {
			group = r.Data
		}
	}
	le := binary.LittleEndian
	if le.Uint16(group[2:]) != 1 || le.Uint16(group[4:]) != 2 || len(group) != 6+2*14 {
		t.Fatalf("group % x", group)
	}
	for i, size := range []int{16, 256} {
		e := group[6+14*i:]
		dim := int(e[0])
		if dim == 0 {
			dim = 256
		}
		if dim != size || le.Uint16(e[4:]) != 1 || le.Uint16(e[6:]) != 32 || le.Uint16(e[12:]) != uint16(i+1) {
			t.Errorf("entry %d: % x", i, e[:14])
		}
	}
	if le.Uint32(group[6+8:]) != uint32(len(testIcons()[0].PNG)) {
		t.Errorf("first entry's size %d", le.Uint32(group[14:]))
	}
}

// versionNode is a node of a VS_VERSIONINFO tree.
type versionNode struct {
	key      string
	typ      uint16
	value    []byte
	children []versionNode
}

// parseNode reads the node at the start of b and its length (without the
// padding after it).
func parseNode(t *testing.T, b []byte) (versionNode, int) {
	t.Helper()
	le := binary.LittleEndian
	length, vlen, typ := int(le.Uint16(b)), int(le.Uint16(b[2:])), le.Uint16(b[4:])
	if length > len(b) || length < 6 {
		t.Fatalf("node length %d in %d bytes", length, len(b))
	}
	var key []uint16
	p := 6
	for ; ; p += 2 {
		c := le.Uint16(b[p:])
		if c == 0 {
			p += 2
			break
		}
		key = append(key, c)
	}
	n := versionNode{key: string(utf16.Decode(key)), typ: typ}
	p = (p + 3) &^ 3
	vbytes := vlen
	if typ == 1 {
		vbytes = 2 * vlen
	}
	n.value = b[p : p+vbytes]
	p += vbytes
	for p < length {
		p = (p + 3) &^ 3
		c, l := parseNode(t, b[p:length])
		n.children = append(n.children, c)
		p += l
	}
	return n, length
}

func TestVersionInfo(t *testing.T) {
	info := Info{ProductName: "Notes", CompanyName: "Example Inc.", FileDescription: "Take notes", Version: "1.2.3-rc.1", FileName: "notes.exe"}
	var data []byte
	for _, r := range Resources(nil, info) {
		if r.Type == RTVersion {
			data = r.Data
		}
	}
	root, n := parseNode(t, data)
	if n != len(data) || root.key != "VS_VERSION_INFO" || root.typ != 0 || len(root.value) != 52 {
		t.Fatalf("root %q type %d, %d value bytes, length %d of %d", root.key, root.typ, len(root.value), n, len(data))
	}
	le := binary.LittleEndian
	if sig := le.Uint32(root.value); sig != 0xFEEF04BD {
		t.Errorf("signature %#x", sig)
	}
	if ms, ls := le.Uint32(root.value[8:]), le.Uint32(root.value[12:]); ms != 1<<16|2 || ls != 3<<16|0 {
		t.Errorf("file version %#x %#x, want 1.2.3.0", ms, ls)
	}
	if ms, ls := le.Uint32(root.value[16:]), le.Uint32(root.value[20:]); ms != 1<<16|2 || ls != 3<<16|0 {
		t.Errorf("product version %#x %#x, want 1.2.3.0", ms, ls)
	}
	if len(root.children) != 2 || root.children[0].key != "StringFileInfo" || root.children[1].key != "VarFileInfo" {
		t.Fatalf("children %+v", root.children)
	}
	table := root.children[0].children
	if len(table) != 1 || table[0].key != "040904B0" {
		t.Fatalf("string table %+v", table)
	}
	got := map[string]string{}
	for _, s := range table[0].children {
		if s.typ != 1 {
			t.Errorf("%s has type %d", s.key, s.typ)
		}
		u := make([]uint16, len(s.value)/2)
		for i := range u {
			u[i] = le.Uint16(s.value[2*i:])
		}
		if len(u) == 0 || u[len(u)-1] != 0 {
			t.Errorf("%s is not NUL-terminated", s.key)
			continue
		}
		got[s.key] = string(utf16.Decode(u[:len(u)-1]))
	}
	want := map[string]string{
		"CompanyName": "Example Inc.", "FileDescription": "Take notes", "FileVersion": "1.2.3-rc.1",
		"InternalName": "notes", "OriginalFilename": "notes.exe", "ProductName": "Notes", "ProductVersion": "1.2.3-rc.1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("strings %v, want %v", got, want)
	}
	if tr := root.children[1].children; len(tr) != 1 || tr[0].key != "Translation" || le.Uint32(tr[0].value) != 0x04B00409 {
		t.Errorf("translation %+v", tr)
	}
}

func TestVersionInfoDefaults(t *testing.T) {
	// No publisher: no CompanyName; no description: the product name; no
	// version: 0.0.0.
	var data []byte
	for _, r := range Resources(nil, Info{ProductName: "Notes", FileName: "notes.exe"}) {
		if r.Type == RTVersion {
			data = r.Data
		}
	}
	root, _ := parseNode(t, data)
	keys := map[string]bool{}
	for _, s := range root.children[0].children[0].children {
		keys[s.key] = true
	}
	if keys["CompanyName"] || !keys["FileDescription"] || !keys["FileVersion"] {
		t.Errorf("strings %v", keys)
	}
	if ms := binary.LittleEndian.Uint32(root.value[8:]); ms != 0 {
		t.Errorf("file version %#x, want 0", ms)
	}
}

func TestNumericVersion(t *testing.T) {
	for in, want := range map[string][4]uint16{
		"1.2.3":            {1, 2, 3, 0},
		"v1.2.3":           {1, 2, 3, 0},
		"1.2.4-0.2026-abc": {1, 2, 4, 0},
		"1.2.3.4":          {1, 2, 3, 4},
		"2":                {2, 0, 0, 0},
		"dev":              {},
		"":                 {},
		"70000.1.1":        {0xffff, 1, 1, 0},
	} {
		if got := numericVersion(in); got != want {
			t.Errorf("numericVersion(%q) = %v, want %v", in, got, want)
		}
	}
}
