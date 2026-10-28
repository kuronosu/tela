package win32

import (
	"image"
	"image/color"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the window styling (ADR-187); the DWM calls are tested
// live (live_style_test.go).

func TestColorRef(t *testing.T) {
	for _, tc := range []struct {
		in   platform.WindowColor
		want uint32
	}{
		{platform.WindowColor{}, 0xFFFFFFFF},                                     // the system's: DWMWA_COLOR_DEFAULT
		{platform.WindowColor{R: 0x11, G: 0x22, B: 0x33, Set: true}, 0x00332211}, // COLORREF is 0x00BBGGRR
		{platform.WindowColor{Set: true}, 0},                                     // black is a color, not the default
		{platform.WindowColor{R: 255, G: 255, B: 255, Set: true}, 0x00FFFFFF},
		{platform.WindowColor{R: 0x11, G: 0x22, B: 0x33}, 0xFFFFFFFF}, // not Set: components ignored
	} {
		if got := colorRef(tc.in); got != tc.want {
			t.Errorf("colorRef(%+v) = %#x, want %#x", tc.in, got, tc.want)
		}
	}
}

func TestBackdropType(t *testing.T) {
	for b, want := range map[platform.Backdrop]int32{
		platform.BackdropDefault: 0, // DWMSBT_AUTO
		platform.BackdropNone:    1, // DWMSBT_NONE
		platform.BackdropMica:    2, // DWMSBT_MAINWINDOW
		platform.BackdropAcrylic: 3, // DWMSBT_TRANSIENTWINDOW
		platform.BackdropMicaAlt: 4, // DWMSBT_TABBEDWINDOW
	} {
		if got := backdropType(b); got != want {
			t.Errorf("backdropType(%v) = %d, want %d", b, got, want)
		}
	}
}

func TestClientBackdrop(t *testing.T) {
	for _, tc := range []struct {
		b         platform.Backdrop
		build     uint32
		composing bool
		want      bool
	}{
		{platform.BackdropMica, 22621, true, true},
		{platform.BackdropMicaAlt, 26100, true, true},
		{platform.BackdropAcrylic, 22621, true, true},
		{platform.BackdropMica, 22000, true, false},  // Windows 11 21H2 has no system backdrop attribute
		{platform.BackdropMica, 19045, true, false},  // Windows 10
		{platform.BackdropMica, 22621, false, false}, // no composition: no glass
		{platform.BackdropNone, 22621, true, false},  // the title bar only
		{platform.BackdropDefault, 22621, true, false},
	} {
		if got := clientBackdrop(tc.b, tc.build, tc.composing); got != tc.want {
			t.Errorf("clientBackdrop(%v, %d, %v) = %v, want %v", tc.b, tc.build, tc.composing, got, tc.want)
		}
	}
}

// An alpha copy keeps the premultiplied alpha of the frame; the default copy
// makes every pixel opaque (a transparent window after a session reconnect
// is worse than an opaque one).
func TestDIBAlpha(t *testing.T) {
	fb := image.NewRGBA(image.Rect(0, 0, 2, 1))
	fb.SetRGBA(0, 0, color.RGBA{R: 0, G: 0, B: 0, A: 0})      // transparent
	fb.SetRGBA(1, 0, color.RGBA{R: 40, G: 20, B: 10, A: 128}) // half-transparent, premultiplied

	var opaque dib
	opaque.resize(image.Pt(2, 1))
	opaque.update(fb, []image.Rectangle{fb.Rect})
	if got, want := opaque.pix, []byte{0, 0, 0, 0xFF, 10, 20, 40, 0xFF}; string(got) != string(want) {
		t.Errorf("opaque copy = %v, want %v", got, want)
	}

	var glass dib
	if !glass.setAlpha(true) {
		t.Fatal("setAlpha(true) reported no change")
	}
	glass.resize(image.Pt(2, 1))
	if glass.pix[3] != 0 {
		t.Errorf("a new alpha copy starts opaque (alpha %#x); it must start transparent", glass.pix[3])
	}
	glass.update(fb, []image.Rectangle{fb.Rect})
	if got, want := glass.pix, []byte{0, 0, 0, 0, 10, 20, 40, 128}; string(got) != string(want) {
		t.Errorf("alpha copy = %v, want %v (BGRA, premultiplied)", got, want)
	}
	if glass.setAlpha(true) {
		t.Error("setAlpha(true) twice reported a change")
	}
	glass.release()
	if !glass.alpha {
		t.Error("release forgot the alpha mode")
	}
	if !glass.setAlpha(false) {
		t.Fatal("setAlpha(false) reported no change")
	}
	glass.resize(image.Pt(2, 1))
	glass.update(fb, []image.Rectangle{fb.Rect})
	if glass.pix[3] != 0xFF || glass.pix[7] != 0xFF {
		t.Errorf("leaving alpha mode left alpha bytes %v", glass.pix)
	}
}
