package text

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// largeFontFile returns Inter followed by zeros up to mapMinSize, so that
// readFontFile maps it (the zeros are not part of any table).
func largeFontFile(extra int) []byte {
	data := append([]byte(nil), fontInter.data()...)
	return append(data, make([]byte, mapMinSize-len(data)+extra)...)
}

// forgetMappings unmaps the mappings readFontFile made of the files in
// dir, so that the temporary directory can be removed (Windows does not
// delete a mapped file). Nothing may use their bytes afterwards.
func forgetMappings(t *testing.T, dir string, extra ...[]byte) {
	mappedFonts.Lock()
	defer mappedFonts.Unlock()
	seen := map[*byte]bool{}
	unmap := func(b []byte) {
		if len(b) == 0 || seen[&b[0]] {
			return
		}
		seen[&b[0]] = true
		if err := unmapFile(b); err != nil {
			t.Error(err)
		}
	}
	for path, m := range mappedFonts.m {
		if filepath.Dir(path) == dir {
			unmap(m.data)
			delete(mappedFonts.m, path)
		}
	}
	for _, b := range extra {
		unmap(b)
	}
}

// TestReadFontFile checks that a large font file reads the same mapped as
// read, is mapped once while it does not change, and again when it is
// replaced; a small or empty file is read.
func TestReadFontFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "font.ttf")
	want := largeFontFile(0)
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	var old [][]byte // mappings no longer in mappedFonts
	defer func() { forgetMappings(t, dir, old...) }()

	a, err := readFontFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, want) {
		t.Fatal("the file reads other bytes")
	}
	mappedFonts.Lock()
	_, mapped := mappedFonts.m[path]
	mappedFonts.Unlock()
	if !mapped && runtime.GOOS != "js" && runtime.GOOS != "wasip1" && runtime.GOOS != "plan9" {
		t.Error("a large file was not mapped")
	}
	b, err := readFontFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if &a[0] != &b[0] {
		t.Error("an unchanged file was mapped twice")
	}
	if _, err := ParseTypefaces(a, ""); err != nil {
		t.Errorf("parsing the mapped file: %v", err)
	}

	// A new file in its place, as a package update writes it: the old
	// bytes stay readable, the new ones are read. (Windows does not
	// replace a mapped file.)
	if runtime.GOOS != "windows" {
		next := largeFontFile(4096)
		tmp := path + ".new"
		if err := os.WriteFile(tmp, next, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(tmp, time.Now(), time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, path); err != nil {
			t.Fatal(err)
		}
		c, err := readFontFile(path)
		if err != nil {
			t.Fatal(err)
		}
		old = append(old, a)
		if !bytes.Equal(c, next) {
			t.Error("the replaced file reads other bytes")
		}
		if !bytes.Equal(a, want) {
			t.Error("the old mapping changed")
		}
	}

	small := filepath.Join(dir, "small.ttf")
	if err := os.WriteFile(small, fontJetBrainsMono.data(), 0o644); err != nil {
		t.Fatal(err)
	}
	if d, err := readFontFile(small); err != nil || !bytes.Equal(d, fontJetBrainsMono.data()) {
		t.Errorf("small file: %d bytes, %v", len(d), err)
	}
	mappedFonts.Lock()
	_, mapped = mappedFonts.m[small]
	mappedFonts.Unlock()
	if mapped {
		t.Error("a small file was mapped")
	}
	empty := filepath.Join(dir, "empty.ttf")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if d, err := readFontFile(empty); err != nil || len(d) != 0 {
		t.Errorf("empty file: %d bytes, %v", len(d), err)
	}
	if _, err := readFontFile(filepath.Join(dir, "missing.ttf")); err == nil {
		t.Error("a missing file has no error")
	}
}
