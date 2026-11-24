package gfx

import (
	"bytes"
	"testing"
)

type fakeExternal struct {
	pix   []byte
	calls int
}

func (f *fakeExternal) Pixels() []byte { f.calls++; return f.pix }

func TestExternalImage(t *testing.T) {
	ext := &fakeExternal{pix: bytes.Repeat([]byte{1, 2, 3, 255}, 4)}
	img := NewExternalImage(2, 2, true, ext)
	if img.External() != External(ext) || ext.calls != 0 {
		t.Fatalf("the content was read before anybody asked (%d calls)", ext.calls)
	}
	if o, known := img.KnownOpaque(); !o || !known {
		t.Errorf("KnownOpaque() = %v, %v, want true, true", o, known)
	}
	if !bytes.Equal(img.Pix(), ext.pix) || !bytes.Equal(img.Level(1).Pix(), []byte{1, 2, 3, 255}) {
		t.Errorf("pixels %v, level 1 %v", img.Pix(), img.Level(1).Pix())
	}
	if img.Pix(); ext.calls != 1 {
		t.Errorf("the pixels were read %d times, want once", ext.calls)
	}
	if got := NewExternalImage(2, 1, false, &fakeExternal{}).Pix(); !bytes.Equal(got, make([]byte, 8)) {
		t.Errorf("unreadable pixels are %v, want transparent ones", got)
	}
}

func TestImageNoCopy(t *testing.T) {
	pix := []byte{9, 9, 9, 255}
	img := NewImageNoCopy(1, 1, pix, false)
	if &img.Pix()[0] != &pix[0] {
		t.Error("the pixels were copied")
	}
	if o, known := img.KnownOpaque(); o || !known {
		t.Errorf("KnownOpaque() = %v, %v, want false, true", o, known)
	}
	if _, known := NewImagePremul(1, 1, pix).KnownOpaque(); known {
		t.Error("an ordinary image claims to be known")
	}
}
