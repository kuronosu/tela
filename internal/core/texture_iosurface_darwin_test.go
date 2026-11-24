package core

import (
	"errors"
	"fmt"
	"image/color"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/kuronosu/tela/internal/darwin"
)

// newTestSurface returns a real IOSurface (+1) of w x h pixels in format,
// filled with pixel.
func newTestSurface(t *testing.T, format uint32, w, h int, pixel func(x, y int) color.RGBA) uintptr {
	t.Helper()
	for _, load := range []func() error{darwin.Load, darwin.LoadIOSurface} {
		if err := load(); err != nil {
			t.Fatal(err)
		}
	}
	runtime.LockOSThread() // the pool is the thread's
	defer runtime.UnlockOSThread()
	defer darwin.PoolPop(darwin.PoolPush())
	s, err := darwin.NewIOSurface(w, h, format, 0)
	if err != nil {
		t.Fatal(err)
	}
	if kr := darwin.IOSurfaceLock(s, 0, nil); kr != 0 {
		t.Fatalf("IOSurfaceLock: %#x", uint32(kr))
	}
	stride := int(darwin.IOSurfaceGetBytesPerRow(s))
	pix := unsafe.Slice((*byte)(darwin.IOSurfaceGetBaseAddress(s)), stride*(h-1)+4*w)
	fillSurface(pix, format, w, h, stride, pixel)
	darwin.IOSurfaceUnlock(s, 0, nil)
	t.Logf("%s: %dx%d, %d bytes per row", osTypeString(format), w, h, stride)
	return s
}

// Frames of real surfaces, in both formats, reach the screen. The
// application's reference is given back after Import: Tela's keeps them.
func TestTextureIOSurface(t *testing.T) {
	const w, h = 13, 9 // rows of 52 bytes, which IOSurface aligns
	pixel := func(x, y int) color.RGBA {
		return color.RGBA{byte(10 + 15*x), byte(20 + 20*y), byte(200 - 3*x*y), 255}
	}
	for _, f := range []uint32{surfaceBGRA, surfaceRGBA} {
		t.Run(osTypeString(f), func(t *testing.T) {
			a := texApp{class: fmt.Sprintf("w-[%dpx] h-[%dpx] bg-black", w, h)}
			var rel texReleases
			x := newWinHarness(t, Options{Root: a.root()})
			s := newTestSurface(t, f, w, h, pixel)
			buf, err := a.tex.Import(IOSurface{Surface: s})
			darwin.CFRelease(s)
			if err != nil {
				t.Fatal(err)
			}
			for i := range 3 {
				if err := a.tex.Submit(buf, TextureFrame{Release: rel.fn("buf")}); err != nil {
					t.Fatal(err)
				}
				waitFor(t, "the copy", func() bool { return surfaceFrameReady(a.tex) })
				x.drain()
				if err := a.tex.SubmitImage(solid(1, 1, texRed), nil); err != nil {
					t.Fatal(err)
				}
				for y := range h {
					for px := range w {
						if g, want := x.pixel(16+px, 48+y), pixel(px, y); g != want {
							t.Fatalf("frame %d, pixel (%d, %d) is %v, want %v", i, px, y, g, want)
						}
					}
				}
				x.drain()
				if got := rel.got(t, a.tex); got != "buf" {
					t.Fatalf("released %q once replaced, want buf", got)
				}
			}
			if err := buf.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Other formats, and other objects than a surface, are refused.
func TestTextureIOSurfaceRefused(t *testing.T) {
	var a texApp
	newWinHarness(t, Options{Root: a.root()})
	s := newTestSurface(t, 'A'<<24|'B'<<16|'G'<<8|'R', 4, 4, func(int, int) color.RGBA { return texRed })
	defer darwin.CFRelease(s)
	if _, err := a.tex.Import(IOSurface{Surface: s}); !errors.Is(err, ErrTextureUnsupported) || !strings.Contains(err.Error(), "'ABGR'") {
		t.Errorf("ABGR: %v", err)
	}
	// A CFString.
	if _, err := a.tex.Import(IOSurface{Surface: darwin.CGColorSpaceSRGB}); err == nil || !strings.Contains(err.Error(), "not an IOSurfaceRef") {
		t.Errorf("a CFString: %v", err)
	}
}

// The registry ID is that of a device, or 0 on a Mac without a GPU or a
// window server session.
func TestTextureMetalRegistryID(t *testing.T) {
	id := darwin.MainDisplayMetalRegistryID()
	t.Logf("registry ID %#x", id)
	for range 100 {
		if got := darwin.MainDisplayMetalRegistryID(); got != id {
			t.Fatalf("then %#x", got)
		}
	}
}
