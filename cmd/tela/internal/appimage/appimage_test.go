package appimage

import (
	"bufio"
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/cmd/tela/internal/appimage/layout"
)

// TestGoDirective keeps the runtime module's go line the tela module's.
func TestGoDirective(t *testing.T) {
	dir, _ := os.Getwd()
	for {
		b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && bytes.HasPrefix(b, []byte("module github.com/kuronosu/tela\n")) {
			sc := bufio.NewScanner(bytes.NewReader(b))
			for sc.Scan() {
				if v, ok := strings.CutPrefix(sc.Text(), "go "); ok && v != GoDirective {
					t.Errorf("go.mod says go %s, GoDirective %s", v, GoDirective)
				}
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no tela go.mod above the test")
		}
		dir = parent
	}
}

// tinyELF returns a minimal 64-bit little-endian ELF file with a null
// section, a .text section and a section name table, the table last.
func tinyELF() []byte {
	strtab := []byte("\x00.text\x00.shstrtab\x00")
	text := []byte{0xc3, 0xc3, 0xc3, 0xc3}
	b := make([]byte, 64)
	copy(b, "\x7fELF\x02\x01\x01")
	le := binary.LittleEndian
	le.PutUint16(b[16:], 2)    // ET_EXEC
	le.PutUint16(b[18:], 0x3e) // x86-64
	le.PutUint32(b[20:], 1)
	le.PutUint16(b[52:], 64) // ehsize
	textOff := uint64(len(b))
	b = append(b, text...)
	strOff := uint64(len(b))
	b = append(b, strtab...)
	for len(b)%8 != 0 {
		b = append(b, 0)
	}
	shoff := uint64(len(b))
	sh := func(name, typ uint32, off, size uint64) {
		b = le.AppendUint32(b, name)
		b = le.AppendUint32(b, typ)
		b = le.AppendUint64(b, 0)
		b = le.AppendUint64(b, 0)
		b = le.AppendUint64(b, off)
		b = le.AppendUint64(b, size)
		b = le.AppendUint32(b, 0)
		b = le.AppendUint32(b, 0)
		b = le.AppendUint64(b, 1)
		b = le.AppendUint64(b, 0)
	}
	sh(0, 0, 0, 0)
	sh(1, 1, textOff, uint64(len(text)))
	sh(7, 3, strOff, uint64(len(strtab)))
	le.PutUint64(b[0x28:], shoff)
	le.PutUint16(b[0x3a:], 64)
	le.PutUint16(b[0x3c:], 3)
	le.PutUint16(b[0x3e:], 2)
	return b
}

func TestAddSections(t *testing.T) {
	in := tinyELF()
	out, offs, err := layout.AddSections(in, []layout.Section{
		{Name: layout.UpdateInfo, Data: bytes.Repeat([]byte{'u'}, 10), Align: 1},
		{Name: layout.Payload, Data: []byte("payload!"), Align: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out[8:11], []byte("AI\x02")) {
		t.Errorf("bytes 8 to 10: % x", out[8:11])
	}
	f, err := elf.NewFile(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{".text": "\xc3\xc3\xc3\xc3", layout.UpdateInfo: "uuuuuuuuuu", layout.Payload: "payload!"} {
		s := f.Section(name)
		if s == nil {
			t.Errorf("no section %s", name)
			continue
		}
		got, _ := s.Data()
		if string(got) != want {
			t.Errorf("%s: %q", name, got)
		}
	}
	if offs[1]%8 != 0 {
		t.Errorf("the payload section is at %d, not aligned", offs[1])
	}
	lf, err := layout.Read(bytes.NewReader(out), int64(len(out)), layout.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if lf.End != uint64(len(out)) || !lf.Magic || string(lf.Sections[layout.Payload]) != "payload!" {
		t.Errorf("Read: end %d of %d, magic %v, %q", lf.End, len(out), lf.Magic, lf.Sections[layout.Payload])
	}
	if _, _, err := layout.AddSections(out, []layout.Section{{Name: layout.Payload}}); err == nil {
		t.Errorf("a second %s was added", layout.Payload)
	}
	if _, _, err := layout.AddSections([]byte("not an elf"), nil); err == nil {
		t.Errorf("not an ELF file was accepted")
	}
}

func TestInfo(t *testing.T) {
	in := layout.Info{Offset: 123, Size: 456, AppID: "dev.example.notes", Version: "1.2.3-rc.1"}
	in.SHA256[0], in.SHA256[31] = 1, 2
	b, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != layout.PayloadSize {
		t.Errorf("%d bytes", len(b))
	}
	got, err := layout.UnmarshalInfo(b)
	if err != nil || got != in {
		t.Errorf("%+v, %v", got, err)
	}
	if _, err := layout.UnmarshalInfo(make([]byte, 512)); err == nil {
		t.Errorf("zeros were read")
	}
}
