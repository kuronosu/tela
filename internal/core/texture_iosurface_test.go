package core

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

// fakeSurface is memory that stands for an IOSurface: the system's calls are
// texture_iosurface_darwin_test.go's.
type fakeSurface struct {
	l   surfaceLayout
	pix []byte

	mu       sync.Mutex
	lockErr  error
	noBase   bool
	calls    []string
	released int
}

func (f *fakeSurface) layout() surfaceLayout { return f.l }

func (f *fakeSurface) lock() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "lock")
	return f.lockErr
}

func (f *fakeSurface) base() unsafe.Pointer {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.noBase {
		return nil
	}
	return unsafe.Pointer(&f.pix[0])
}

func (f *fakeSurface) unlock() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "unlock")
}

func (f *fakeSurface) release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released++
}

// state returns the calls since the last one, and how many times it was
// released.
func (f *fakeSurface) state() (calls string, released int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls = strings.Join(f.calls, " ")
	f.calls = nil
	return calls, f.released
}

// newFakeSurface is a surface of w x h pixels, with 0xEE in the padding of
// its rows, whose memory ends at its last pixel.
func newFakeSurface(format uint32, w, h, stride int, pixel func(x, y int) color.RGBA) *fakeSurface {
	f := &fakeSurface{pix: make([]byte, stride*(h-1)+4*w)}
	f.l = surfaceLayout{format: format, width: uint(w), height: uint(h), element: 4, stride: uint(stride), alloc: uint(len(f.pix))}
	fillSurface(f.pix, format, w, h, stride, pixel)
	return f
}

// fillSurface writes a picture to the memory of a surface.
func fillSurface(pix []byte, format uint32, w, h, stride int, pixel func(x, y int) color.RGBA) {
	for i := range pix {
		pix[i] = 0xEE
	}
	for y := range h {
		for x := range w {
			c := pixel(x, y)
			p := [4]byte{c.R, c.G, c.B, c.A}
			if format == surfaceBGRA {
				p[0], p[2] = c.B, c.R
			}
			copy(pix[y*stride+4*x:], p[:])
		}
	}
}

// useFakeSurfaces makes the references of IOSurfaces stand for fs: 1 is the
// first one.
func useFakeSurfaces(t *testing.T, fs ...*fakeSurface) {
	t.Helper()
	was := mapIOSurface
	mapIOSurface = func(s IOSurface) (mappedTexture, int, int, error) {
		m, err := newMappedSurface(fs[s.Surface-1], s.Opaque)
		if err != nil {
			return nil, 0, 0, err
		}
		return m, int(m.l.width), int(m.l.height), nil
	}
	t.Cleanup(func() { mapIOSurface = was })
}

// The values the headers of CoreVideo have.
func TestTextureSurfaceFormats(t *testing.T) {
	want := []TextureFormat{{Fourcc: 0x42475241}, {Fourcc: 0x52474241}}
	if got := surfaceFormats(); !slices.Equal(got, want) {
		t.Errorf("formats %x, want %x", got, want)
	}
	for v, want := range map[uint32]string{surfaceBGRA: "'BGRA'", 0x34323076: "'420v'", 32: "0x00000020", 0: "0x00000000"} {
		if got := osTypeString(v); got != want {
			t.Errorf("%#x is %s, want %s", v, got, want)
		}
	}
}

func TestTextureSurfaceCheck(t *testing.T) {
	ok := surfaceLayout{format: surfaceBGRA, width: 100, height: 50, element: 4, stride: 448, alloc: 448 * 50}
	for _, tc := range []struct {
		name        string
		change      func(l *surfaceLayout)
		want        string // "" for none
		unsupported bool
	}{
		{"BGRA", func(*surfaceLayout) {}, "", false},
		{"RGBA", func(l *surfaceLayout) { l.format = surfaceRGBA }, "", false},
		{"tight", func(l *surfaceLayout) { l.stride, l.alloc = 400, 400*50 }, "", false},
		{"ends at its last pixel", func(l *surfaceLayout) { l.alloc = 448*49 + 400 }, "", false},
		{"one row", func(l *surfaceLayout) { l.height, l.alloc = 1, 400 }, "", false},
		{"largest", func(l *surfaceLayout) { *l = surfaceLayout{surfaceBGRA, 0, 1 << 14, 1 << 14, 4, 4 << 14, 4 << 28} }, "", false},
		{"420v", func(l *surfaceLayout) { l.format, l.planes = '4'<<24|'2'<<16|'0'<<8|'v', 2 }, "'420v'", true},
		{"10 bits", func(l *surfaceLayout) { l.format = 'l'<<24 | '1'<<16 | '0'<<8 | 'r' }, "'l10r'", true},
		{"half float", func(l *surfaceLayout) { l.format = 'R'<<24 | 'G'<<16 | 'h'<<8 | 'A' }, "'RGhA'", true},
		{"ARGB", func(l *surfaceLayout) { l.format = 0x20 }, "0x00000020", true},
		{"compressed", func(l *surfaceLayout) { l.format = '&'<<24 | 'B'<<16 | 'G'<<8 | 'A' }, "'&BGA'", true},
		{"no format", func(l *surfaceLayout) { l.format = 0 }, "0x00000000", true},
		{"planar", func(l *surfaceLayout) { l.planes = 1 }, "planes", true},
		{"elements", func(l *surfaceLayout) { l.element = 8 }, "8 bytes per element", true},
		{"no width", func(l *surfaceLayout) { l.width = 0 }, "width and height", false},
		{"no height", func(l *surfaceLayout) { l.height = 0 }, "width and height", false},
		{"too wide", func(l *surfaceLayout) { l.width = 1<<14 + 1 }, "width and height", false},
		{"too high", func(l *surfaceLayout) { l.height = math.MaxUint }, "width and height", false},
		{"short rows", func(l *surfaceLayout) { l.stride = 399 }, "399 bytes per row for rows of 400", false},
		{"no stride", func(l *surfaceLayout) { l.stride = 0 }, "bytes per row", false},
		{"short memory", func(l *surfaceLayout) { l.alloc = 448*49 + 399 }, "beyond its memory", false},
		{"no memory", func(l *surfaceLayout) { l.alloc = 0 }, "beyond its memory", false},
		{"a wrapping stride", func(l *surfaceLayout) { l.stride = math.MaxUint / 49 }, "beyond its memory", false},
		{"more memory than there is", func(l *surfaceLayout) { l.alloc = math.MaxUint }, "beyond its memory", false},
	} {
		l := ok
		tc.change(&l)
		err := l.check()
		switch {
		case tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: %v, want %q", tc.name, err, tc.want)
		case errors.Is(err, ErrTextureUnsupported) != tc.unsupported:
			t.Errorf("%s: %v; wraps ErrTextureUnsupported: %v", tc.name, err, !tc.unsupported)
		case err == nil && uint(l.extent()) > l.alloc:
			t.Errorf("%s: reads %d bytes of %d", tc.name, l.extent(), l.alloc)
		}
	}
}

// Both formats, with padded rows, reach the screen; each copy locks and
// unlocks once.
func TestTextureSurfaceCopy(t *testing.T) {
	const w, h, stride = 13, 9, 13*4 + 12
	pixel := func(x, y int) color.RGBA {
		return color.RGBA{byte(10 + 15*x), byte(20 + 20*y), byte(200 - 3*x*y), 255}
	}
	for _, opaque := range []bool{false, true} {
		for _, f := range []uint32{surfaceBGRA, surfaceRGBA} {
			t.Run(fmt.Sprintf("%s opaque=%v", osTypeString(f), opaque), func(t *testing.T) {
				mem := newFakeSurface(f, w, h, stride, func(x, y int) color.RGBA {
					c := pixel(x, y)
					if opaque {
						c.A = byte(x) // means nothing
					}
					return c
				})
				useFakeSurfaces(t, mem)
				a := texApp{class: fmt.Sprintf("w-[%dpx] h-[%dpx] bg-black", w, h)}
				var rel texReleases
				x := newWinHarness(t, Options{Root: a.root()})
				buf, err := a.tex.Import(IOSurface{Surface: 1, Opaque: opaque})
				if err != nil {
					t.Fatal(err)
				}
				if calls, _ := mem.state(); calls != "" {
					t.Errorf("Import did %s", calls)
				}
				for i := range 3 { // the copies use spare pixels again
					if err := a.tex.Submit(buf, TextureFrame{Release: rel.fn("buf")}); err != nil {
						t.Fatal(err)
					}
					waitFor(t, "the copy", func() bool { return surfaceFrameReady(a.tex) })
					if calls, _ := mem.state(); calls != "lock unlock" {
						t.Errorf("a copy did %s", calls)
					}
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
				buf.Close()
				if _, n := mem.state(); n != 1 {
					t.Errorf("released %d times, want once", n)
				}
			})
		}
	}
}

// surfaceFrameReady reports that the only pending frame of x is ready.
func surfaceFrameReady(x *Texture) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.mb.pending() == 1 && x.mb.ready != nil
}

// Premultiplied alpha blends over the background, and the pixels are kept as
// they are.
func TestTextureSurfaceAlpha(t *testing.T) {
	mem := newFakeSurface(surfaceBGRA, 4, 4, 16, func(int, int) color.RGBA { return color.RGBA{128, 0, 0, 128} })
	useFakeSurfaces(t, mem)
	a := texApp{class: "w-10 h-10 bg-white"}
	x := newWinHarness(t, Options{Root: a.root()})
	buf, err := a.tex.Import(&IOSurface{Surface: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.tex.Submit(buf, TextureFrame{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the copy", func() bool { return surfaceFrameReady(a.tex) })
	x.drain()
	if got, want := x.pixel(36, 68), (color.RGBA{255, 127, 127, 255}); got != want {
		t.Errorf("over white: %v, want %v", got, want)
	}
}

// A surface that cannot be locked, or has no address, drops its frame; what
// was locked is unlocked, also when the memory is gone.
func TestTextureSurfaceReadFails(t *testing.T) {
	newSurface := func() *fakeSurface {
		return newFakeSurface(surfaceRGBA, 2, 2, 8, func(int, int) color.RGBA { return texRed })
	}
	locked, noBase, short := newSurface(), newSurface(), newSurface()
	locked.lockErr = errors.New("IOSurfaceLock failed (kern_return_t 0xe00002bc)")
	noBase.noBase = true
	useFakeSurfaces(t, locked, noBase)
	var mu sync.Mutex
	var warned []string
	var a texApp
	var rel texReleases
	x := newWinHarness(t, Options{Config: Config{Dev: true, Warn: func(m string) {
		mu.Lock()
		warned = append(warned, m)
		mu.Unlock()
	}}, Root: a.root()})
	for i, tc := range []struct {
		mem         *fakeSurface
		calls, want string
	}{
		{locked, "lock", "0xe00002bc"},
		{noBase, "lock unlock", "no base address"},
	} {
		buf, err := a.tex.Import(IOSurface{Surface: uintptr(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.tex.Submit(buf, TextureFrame{Release: rel.fn("buf")}); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the frame to be dropped", func() bool { return a.tex.Stats().Dropped == uint64(i+1) })
		if got := rel.got(t, a.tex); got != "buf" {
			t.Errorf("released %q", got)
		}
		if calls, _ := tc.mem.state(); calls != tc.calls {
			t.Errorf("did %s, want %s", calls, tc.calls)
		}
		waitFor(t, "the warning", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(warned) == i+1 && strings.Contains(warned[i], tc.want)
		})
	}
	x.drain()
	// A panic of the copy (a destination too short) leaves nothing locked.
	m, err := newMappedSurface(short, false)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() { _ = recover() }()
		_ = m.read(make([]byte, 4))
	}()
	if calls, _ := short.state(); calls != "lock unlock" {
		t.Errorf("a copy that panics did %s", calls)
	}
}

func TestTextureSurfaceImport(t *testing.T) {
	newSurface := func(f uint32) *fakeSurface {
		return newFakeSurface(f, 2, 2, 8, func(int, int) color.RGBA { return texRed })
	}
	refused, held, idle := newSurface(0x20), newSurface(surfaceBGRA), newSurface(surfaceBGRA)
	useFakeSurfaces(t, refused, held, idle)
	var a texApp
	x := newWinHarness(t, Options{Root: a.root()})
	for _, tc := range []struct {
		name, want  string
		m           TextureMemory
		unsupported bool
	}{
		{"nil pointer", "nil *IOSurface", (*IOSurface)(nil), false},
		{"NULL", "NULL", IOSurface{}, false},
		{"ARGB", "0x00000020", IOSurface{Surface: 1}, true},
	} {
		_, err := a.tex.Import(tc.m)
		if err == nil || !strings.Contains(err.Error(), tc.want) || errors.Is(err, ErrTextureUnsupported) != tc.unsupported {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	if _, n := refused.state(); n != 1 {
		t.Errorf("the reference of a surface refused was released %d times", n)
	}
	tex := a.tex
	buf, err := tex.Import(IOSurface{Surface: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tex.Import(IOSurface{Surface: 3}); err != nil {
		t.Fatal(err)
	}
	if err := tex.Submit(buf, TextureFrame{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the copy", func() bool { return surfaceFrameReady(tex) })
	x.drain()
	buf.Close()
	if _, n := held.state(); n != 0 {
		t.Error("the surface of the frame shown was released")
	}
	a.closed.Set(true) // the texture closes, with its buffers
	x.drain()
	for name, mem := range map[string]*fakeSurface{"shown": held, "idle": idle} {
		waitFor(t, name+" to be released", func() bool { _, n := mem.state(); return n == 1 })
	}
	if _, err := tex.Import(IOSurface{Surface: 3}); !errors.Is(err, ErrTextureClosed) {
		t.Errorf("after the unmount: %v", err)
	}
}

// Elsewhere an IOSurface is an error that says what to do.
func TestTextureSurfaceElsewhere(t *testing.T) {
	if runtime.GOOS == "darwin" {
		return // texture_iosurface_darwin_test.go
	}
	var a texApp
	newWinHarness(t, Options{Root: a.root()})
	_, err := a.tex.Import(IOSurface{Surface: 1})
	if !errors.Is(err, ErrTextureUnsupported) || !strings.Contains(err.Error(), "macOS") {
		t.Errorf("Import: %v", err)
	}
	if id := a.tex.Device().MetalRegistryID; id != 0 {
		t.Errorf("MetalRegistryID %d", id)
	}
}
