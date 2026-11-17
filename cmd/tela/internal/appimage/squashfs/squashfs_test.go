package squashfs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var epoch = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// sample returns entries that exercise the format: nested folders, a
// folder of more than 256 entries (several listing runs and metadata
// blocks), a file of several blocks, one that does not compress, an empty
// file, an empty folder, links and special permission bits.
func sample() []Entry {
	rnd := rand.New(rand.NewPCG(1, 2))
	noise := make([]byte, 70_000)
	for i := range noise {
		noise[i] = byte(rnd.Uint32())
	}
	big := bytes.Repeat([]byte("tela squashfs block "), 20_000) // 400 kB
	es := []Entry{
		{Path: ".", Mode: fs.ModeDir | 0o755},
		{Path: "AppRun", Mode: fs.ModeSymlink | 0o777, Target: "usr/bin/notes"},
		{Path: "usr/bin/notes", Mode: 0o755, Data: big},
		{Path: "usr/share/noise.bin", Mode: 0o644, Data: noise},
		{Path: "usr/share/empty", Mode: 0o600},
		{Path: "usr/share/void", Mode: fs.ModeDir | 0o700},
		{Path: "usr/share/setuid", Mode: 0o755 | fs.ModeSetuid | fs.ModeSticky, Data: []byte("#!/bin/sh\n")},
		{Path: "usr/lib/link", Mode: fs.ModeSymlink | 0o777, Target: "../share/noise.bin"},
		{Path: "a.desktop", Mode: 0o644, Data: []byte("[Desktop Entry]\nName=Notes\n")},
	}
	for i := range 600 {
		es = append(es, Entry{Path: fmt.Sprintf("many/file-%04d.txt", i), Mode: 0o644, Data: []byte(strings.Repeat("x", i))})
	}
	return es
}

func build(t *testing.T, es []Entry) []byte {
	t.Helper()
	img, err := Build(es, Options{ModTime: epoch})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// readAll reads every entry of img: path -> entry, with contents.
func readAll(t *testing.T, img []byte) (map[string]File, map[string][]byte, []string) {
	t.Helper()
	rd, err := NewReader(bytes.NewReader(img), int64(len(img)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]File{}
	data := map[string][]byte{}
	var order []string
	err = rd.Walk(func(f File) error {
		files[f.Path] = f
		order = append(order, f.Path)
		if f.Mode.IsRegular() {
			var b bytes.Buffer
			if _, err := rd.WriteTo(&b, f); err != nil {
				return err
			}
			data[f.Path] = b.Bytes()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files, data, order
}

func TestRoundTrip(t *testing.T) {
	es := sample()
	img := build(t, es)
	if len(img)%4096 != 0 {
		t.Errorf("the image is %d bytes, not padded to 4 KiB", len(img))
	}
	files, data, order := readAll(t, img)
	for _, e := range es {
		if e.Path == "." {
			continue
		}
		f, ok := files[e.Path]
		if !ok {
			t.Errorf("%s is missing", e.Path)
			continue
		}
		if f.Mode != e.Mode {
			t.Errorf("%s: mode %v, want %v", e.Path, f.Mode, e.Mode)
		}
		if !f.ModTime.Equal(epoch) {
			t.Errorf("%s: time %v", e.Path, f.ModTime)
		}
		if f.UID != 0 || f.GID != 0 {
			t.Errorf("%s: owner %d:%d", e.Path, f.UID, f.GID)
		}
		switch {
		case e.Mode&fs.ModeSymlink != 0:
			if f.Target != e.Target {
				t.Errorf("%s: target %q, want %q", e.Path, f.Target, e.Target)
			}
		case e.Mode.IsRegular():
			if !bytes.Equal(data[e.Path], e.Data) || f.Size != int64(len(e.Data)) {
				t.Errorf("%s: %d bytes read back, want %d", e.Path, len(data[e.Path]), len(e.Data))
			}
		}
	}
	// Implied folders and the order: each folder before its entries, by name.
	for _, d := range []string{"usr", "usr/bin", "usr/lib", "usr/share", "many"} {
		if f := files[d]; !f.Mode.IsDir() || f.Mode.Perm() != 0o755 {
			t.Errorf("%s: %v, want an implied folder 0755", d, f.Mode)
		}
	}
	idx := map[string]int{}
	for i, p := range order {
		idx[p] = i
	}
	if !(idx["AppRun"] < idx["a.desktop"] && idx["usr"] < idx["usr/bin"] && idx["usr/bin"] < idx["usr/bin/notes"]) {
		t.Errorf("order: %v", order[:8])
	}
	if want := len(es) - 1 + 5; len(order) != want {
		t.Errorf("%d entries, want %d", len(order), want)
	}
}

func TestDeterministic(t *testing.T) {
	a := build(t, sample())
	es := sample()
	// Another order of the same entries gives the same bytes.
	for i, j := 0, len(es)-1; i < j; i, j = i+1, j-1 {
		es[i], es[j] = es[j], es[i]
	}
	if b := build(t, es); !bytes.Equal(a, b) {
		t.Errorf("two images of the same entries differ")
	}
}

func TestSuperblock(t *testing.T) {
	img := build(t, sample())
	le := binary.LittleEndian
	if string(img[:4]) != "hsqs" || le.Uint16(img[28:]) != 4 || le.Uint16(img[20:]) != 1 {
		t.Errorf("magic, version or compressor wrong: % x", img[:32])
	}
	if got := le.Uint32(img[8:]); got != uint32(epoch.Unix()) {
		t.Errorf("mkfs time %d", got)
	}
	used := le.Uint64(img[40:])
	if used > uint64(len(img)) || uint64(len(img))-used >= 4096 {
		t.Errorf("bytes used %d of %d", used, len(img))
	}
	// unsquashfs's checks: the ID index ends the file system, and with no
	// fragments the fragment table starts at the ID table's block.
	idStart := le.Uint64(img[48:])
	if idStart+8 != used {
		t.Errorf("the ID index (%d) does not end the file system (%d)", idStart, used)
	}
	if frag, idBlock := le.Uint64(img[80:]), le.Uint64(img[idStart:]); frag != idBlock {
		t.Errorf("fragment table start %d, ID block %d", frag, idBlock)
	}
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		es   []Entry
		want string
	}{
		{[]Entry{{Path: "/abs", Mode: 0o644}}, "clean relative"},
		{[]Entry{{Path: "a/../b", Mode: 0o644}}, "clean relative"},
		{[]Entry{{Path: "../b", Mode: 0o644}}, "clean relative"},
		{[]Entry{{Path: "a", Mode: 0o644}, {Path: "a", Mode: 0o644}}, "twice"},
		{[]Entry{{Path: "a", Mode: 0o644}, {Path: "a/b", Mode: 0o644}}, "not a folder"},
		{[]Entry{{Path: "dev", Mode: fs.ModeDevice | 0o644}}, "not a regular file"},
		{[]Entry{{Path: "l", Mode: fs.ModeSymlink}}, "link target"},
		{[]Entry{{Path: ".", Mode: 0o644}}, "must be a folder"},
	} {
		if _, err := Build(tc.es, Options{ModTime: epoch}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: %v, want %q", tc.es, err, tc.want)
		}
	}
	if _, err := Build(nil, Options{ModTime: time.Unix(-1, 0)}); err == nil {
		t.Errorf("a time before 1970 was accepted")
	}
	if _, err := Build(nil, Options{ModTime: epoch, BlockSize: 3000}); err == nil {
		t.Errorf("block size 3000 was accepted")
	}
	// An empty image is a root folder.
	files, _, _ := readAll(t, build(t, nil))
	if len(files) != 0 {
		t.Errorf("empty image: %v", files)
	}
}

// TestDamaged reads images with bytes changed or cut: every result is an
// error or a successful read, never a panic or a hang.
func TestDamaged(t *testing.T) {
	img := build(t, sample()[:12])
	rnd := rand.New(rand.NewPCG(3, 4))
	try := func(b []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic: %v", r)
			}
		}()
		rd, err := NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return
		}
		rd.Walk(func(f File) error {
			if f.Mode.IsRegular() {
				var sink bytes.Buffer
				rd.WriteTo(&sink, f)
			}
			return nil
		})
	}
	for i := range 3000 {
		b := bytes.Clone(img)
		switch i % 3 {
		case 0:
			for range 1 + rnd.IntN(8) {
				b[rnd.IntN(len(b))] = byte(rnd.Uint32())
			}
		case 1:
			b = b[:rnd.IntN(len(b))]
		default:
			p := rnd.IntN(superSize - 8)
			binary.LittleEndian.PutUint64(b[p:], rnd.Uint64()%uint64(len(b)*2))
		}
		try(b)
	}
}

// TestUnsquashfs compares the image with what squashfs-tools reads, when
// unsquashfs is installed.
func TestUnsquashfs(t *testing.T) {
	bin, err := exec.LookPath("unsquashfs")
	if err != nil {
		t.Skip("unsquashfs is not installed")
	}
	dir := t.TempDir()
	es := sample()
	img := filepath.Join(dir, "img.squashfs")
	if err := os.WriteFile(img, build(t, es), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if b, err := exec.Command(bin, "-no-xattrs", "-d", out, img).CombinedOutput(); err != nil {
		t.Fatalf("unsquashfs: %v\n%s", err, b)
	}
	for _, e := range es {
		if e.Path == "." {
			continue
		}
		p := filepath.Join(out, filepath.FromSlash(e.Path))
		st, err := os.Lstat(p)
		if err != nil {
			t.Errorf("%s: %v", e.Path, err)
			continue
		}
		if e.Mode&fs.ModeSymlink != 0 {
			if got, _ := os.Readlink(p); got != e.Target {
				t.Errorf("%s: target %q", e.Path, got)
			}
			continue
		}
		if st.Mode().Perm() != e.Mode.Perm() || st.IsDir() != e.Mode.IsDir() {
			t.Errorf("%s: mode %v, want %v", e.Path, st.Mode(), e.Mode)
		}
		if e.Mode.IsRegular() {
			if got, _ := os.ReadFile(p); !bytes.Equal(got, e.Data) {
				t.Errorf("%s: content differs", e.Path)
			}
		}
	}
}
