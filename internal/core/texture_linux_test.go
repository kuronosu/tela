package core

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Memory of the tests that stands for a dma-buf: a memfd, which the kernel
// maps alike, and whose DMA_BUF_IOCTL_SYNC the tests answer (a real dma-buf
// needs a device).

// fakeDmabufs makes memfds pass for dma-bufs during a test, and records the
// sync calls.
type fakeDmabufs struct {
	mu    sync.Mutex
	flags []uint64
	// block, when not nil, is received from by each sync start of a read
	// after the import's.
	block chan struct{}
	begun int
}

func newFakeDmabufs(t *testing.T) *fakeDmabufs {
	t.Helper()
	f := &fakeDmabufs{}
	was := dmabufSync
	dmabufSync = func(_ int, flags uint64) error {
		f.mu.Lock()
		f.flags = append(f.flags, flags)
		block := f.block
		if flags&dmaBufSyncEnd == 0 {
			f.begun++
		}
		f.mu.Unlock()
		if block != nil && flags&dmaBufSyncEnd == 0 {
			<-block
		}
		return nil
	}
	t.Cleanup(func() { dmabufSync = was })
	return f
}

// memfd returns a descriptor of n bytes of memory.
func memfd(t *testing.T, n int) int {
	t.Helper()
	fd, err := unix.MemfdCreate("tela-texture-test", unix.MFD_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(fd) })
	if err := unix.Ftruncate(fd, int64(n)); err != nil {
		t.Fatal(err)
	}
	return fd
}

// fourccPixel is the bytes of color c in fourcc f (x is the ignored byte).
func fourccPixel(f uint32, c color.RGBA, x byte) [4]byte {
	switch f {
	case fourccAR24:
		return [4]byte{c.B, c.G, c.R, c.A}
	case fourccXR24:
		return [4]byte{c.B, c.G, c.R, x}
	case fourccAB24:
		return [4]byte{c.R, c.G, c.B, c.A}
	}
	return [4]byte{c.R, c.G, c.B, x}
}

// fillMemfd writes a picture of w x h pixels to fd, pixel(x, y) in fourcc f,
// and 0xEE in the padding of the rows.
func fillMemfd(t *testing.T, fd int, f uint32, w, h, offset, stride int, pixel func(x, y int) color.RGBA) {
	t.Helper()
	buf := make([]byte, stride*h)
	for i := range buf {
		buf[i] = 0xEE
	}
	for y := range h {
		for x := range w {
			p := fourccPixel(f, pixel(x, y), 0x5A)
			copy(buf[y*stride+4*x:], p[:])
		}
	}
	if _, err := unix.Pwrite(fd, buf, int64(offset)); err != nil {
		t.Fatal(err)
	}
}

// Every fourcc, with padded rows and an offset, reaches the screen.
func TestTextureDmabufCopy(t *testing.T) {
	const w, h, offset, stride = 13, 9, 64, 13*4 + 20
	pixel := func(x, y int) color.RGBA {
		return color.RGBA{byte(10 + 15*x), byte(20 + 20*y), byte(200 - 3*x*y), 255}
	}
	for _, f := range mappedFourccs {
		t.Run(fourccString(f), func(t *testing.T) {
			syncs := newFakeDmabufs(t)
			a := texApp{class: fmt.Sprintf("w-[%dpx] h-[%dpx] bg-black", w, h)}
			var rel texReleases
			x := newWinHarness(t, Options{Root: a.root()})
			fd := memfd(t, offset+stride*h)
			fillMemfd(t, fd, f, w, h, offset, stride, pixel)
			buf, err := a.tex.Import(Dmabuf{Width: w, Height: h, Fourcc: f, Planes: []DmabufPlane{{FD: fd, Offset: offset, Stride: stride}}})
			if err != nil {
				t.Fatal(err)
			}
			for i := range 3 { // the copies use spare pixels again
				if err := a.tex.Submit(buf, TextureFrame{Release: rel.fn("buf")}); err != nil {
					t.Fatal(err)
				}
				waitFor(t, "the copy", func() bool { return pendingFrames(a.tex) == 1 && readyFrame(a.tex) })
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
			syncs.mu.Lock()
			flags := slices.Clone(syncs.flags)
			syncs.mu.Unlock()
			var want []uint64 // the import asks once, each copy once
			for range 4 {
				want = append(want, dmaBufSyncStart|dmaBufSyncRead, dmaBufSyncEnd|dmaBufSyncRead)
			}
			if !slices.Equal(flags, want) {
				t.Errorf("DMA_BUF_IOCTL_SYNC with %v, want %v", flags, want)
			}
		})
	}
}

func pendingFrames(x *Texture) int {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.mb.pending()
}

func readyFrame(x *Texture) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.mb.ready != nil
}

// Premultiplied alpha blends over the background; an X format is opaque, and
// says so.
func TestTextureDmabufAlpha(t *testing.T) {
	newFakeDmabufs(t)
	a := texApp{class: "w-10 h-10 bg-white"}
	x := newWinHarness(t, Options{Root: a.root()})
	half := func(int, int) color.RGBA { return color.RGBA{128, 0, 0, 128} }
	for _, tc := range []struct {
		fourcc uint32
		want   color.RGBA
		opaque bool
	}{{fourccAR24, color.RGBA{255, 127, 127, 255}, false}, {fourccXB24, color.RGBA{128, 0, 0, 255}, true}} {
		fd := memfd(t, 4*4*4)
		fillMemfd(t, fd, tc.fourcc, 4, 4, 0, 16, half)
		buf, err := a.tex.Import(Dmabuf{Width: 4, Height: 4, Fourcc: tc.fourcc, Planes: []DmabufPlane{{FD: fd, Stride: 16}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.tex.Submit(buf, TextureFrame{}); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the copy", func() bool { return readyFrame(a.tex) })
		x.drain()
		if c := x.pixel(30, 60); c != tc.want {
			t.Errorf("%s: %v, want %v", fourccString(tc.fourcc), c, tc.want)
		}
		if o, known := a.tex.views[0].cur.img.KnownOpaque(); !known || o != tc.opaque {
			t.Errorf("%s: opaque %v, known %v", fourccString(tc.fourcc), o, known)
		}
	}
}

// What cannot be read is an error of Import, never a fault.
func TestTextureDmabufErrors(t *testing.T) {
	newFakeDmabufs(t)
	var a texApp
	newWinHarness(t, Options{Root: a.root()})
	const size = 64 * 64 * 4
	fd := memfd(t, size)
	closed := func() int {
		fds := make([]int, 2)
		if err := unix.Pipe2(fds, unix.O_CLOEXEC); err != nil {
			t.Fatal(err)
		}
		unix.Close(fds[0])
		unix.Close(fds[1])
		return fds[0]
	}()
	d := func(edit func(d *Dmabuf)) Dmabuf {
		out := Dmabuf{Width: 64, Height: 64, Fourcc: fourccAR24, Planes: []DmabufPlane{{FD: fd, Stride: 256}}}
		edit(&out)
		return out
	}
	for _, tc := range []struct {
		name, want  string
		m           Dmabuf
		unsupported bool
	}{
		{"one row too many", "its memory has 16384", d(func(d *Dmabuf) { d.Height = 65 }), false},
		{"an offset", "its memory has 16384", d(func(d *Dmabuf) { d.Planes[0].Offset = 1 }), false},
		{"a wide stride", "its memory has 16384", d(func(d *Dmabuf) { d.Planes[0].Stride = 260 }), false},
		{"a huge stride and offset", "its memory has 16384", d(func(d *Dmabuf) {
			d.Height, d.Planes[0].Stride, d.Planes[0].Offset = texMaxSide, math.MaxUint32, math.MaxUint32
		}), false},
		{"a short stride", "a stride of 255 bytes", d(func(d *Dmabuf) { d.Planes[0].Stride = 255 }), false},
		{"a modifier", "linear memory", d(func(d *Dmabuf) { d.Modifier = 1 << 56 }), true},
		{"two planes", "linear memory", d(func(d *Dmabuf) { d.Planes = append(d.Planes, d.Planes[0]) }), true},
		{"a closed descriptor", "duplicating", d(func(d *Dmabuf) { d.Planes[0].FD = closed }), false},
	} {
		b, err := a.tex.Import(tc.m)
		if b != nil || err == nil || !strings.Contains(err.Error(), tc.want) || errors.Is(err, ErrTextureUnsupported) != tc.unsupported {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	// What fits exactly does.
	if b, err := a.tex.Import(d(func(*Dmabuf) {})); err != nil {
		t.Error(err)
	} else if err := b.Close(); err != nil {
		t.Error(err)
	}
}

// A descriptor that is not a dma-buf is refused: the memory of a file can
// shrink under its mapping.
func TestTextureNotADmabuf(t *testing.T) {
	var a texApp
	newWinHarness(t, Options{Root: a.root()})
	file, err := os.Create(filepath.Join(t.TempDir(), "pixels"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := file.Truncate(4 * 64 * 64); err != nil {
		t.Fatal(err)
	}
	for _, fd := range []int{int(file.Fd()), memfd(t, 4*64*64)} {
		_, err := a.tex.Import(Dmabuf{Width: 64, Height: 64, Fourcc: fourccXR24, Planes: []DmabufPlane{{FD: fd, Stride: 256}}})
		if err == nil || !strings.Contains(err.Error(), "not a dma-buf") || errors.Is(err, ErrTextureUnsupported) {
			t.Errorf("imported: %v", err)
		}
	}
}

// Memory that goes away while it is copied drops the frame: no crash.
func TestTextureDmabufGone(t *testing.T) {
	newFakeDmabufs(t)
	var a texApp
	var rel texReleases
	var mu sync.Mutex
	var warned []string
	x := newWinHarness(t, Options{Config: Config{Dev: true, Warn: func(m string) {
		mu.Lock()
		warned = append(warned, m)
		mu.Unlock()
	}}, Root: a.root()})
	const size = 64 * 64 * 4
	fd := memfd(t, size)
	buf, err := a.tex.Import(Dmabuf{Width: 64, Height: 64, Fourcc: fourccAR24, Planes: []DmabufPlane{{FD: fd, Stride: 256}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Ftruncate(fd, 0); err != nil { // a dma-buf cannot
		t.Fatal(err)
	}
	if err := a.tex.Submit(buf, TextureFrame{Release: rel.fn("buf")}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the frame to be dropped", func() bool { return a.tex.Stats().Dropped == 1 })
	if got := rel.got(t, a.tex); got != "buf" {
		t.Errorf("released %q", got)
	}
	x.drain()
	mu.Lock()
	defer mu.Unlock()
	if len(warned) != 1 || !strings.Contains(warned[0], "memory of a dmabuf is gone") {
		t.Errorf("warned %q", warned)
	}
}

// A frame dropped while its memory is copied is released when the copy ends,
// not before; its buffer, closed meanwhile, is unmapped after it.
func TestTextureDroppedWhileCopied(t *testing.T) {
	syncs := newFakeDmabufs(t)
	var a texApp
	var rel texReleases
	x := newWinHarness(t, Options{Root: a.root()})
	tex := a.tex
	fd := memfd(t, 16)
	buf, err := tex.Import(Dmabuf{Width: 2, Height: 2, Fourcc: fourccAR24, Planes: []DmabufPlane{{FD: fd, Stride: 8}}})
	if err != nil {
		t.Fatal(err)
	}
	m := buf.mem.(*mappedDmabuf)
	syncs.mu.Lock()
	syncs.block, syncs.begun = make(chan struct{}), 0
	syncs.mu.Unlock()
	if err := tex.Submit(buf, TextureFrame{Release: rel.fn("copied")}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the copy to start", func() bool {
		syncs.mu.Lock()
		defer syncs.mu.Unlock()
		return syncs.begun == 1
	})
	if err := tex.SubmitImage(solid(2, 2, texGreen), rel.fn("pixels")); err != nil { // drops it
		t.Fatal(err)
	}
	if err := buf.Close(); err != nil {
		t.Fatal(err)
	}
	if st := tex.Stats(); st.Dropped != 1 {
		t.Errorf("stats %+v", st)
	}
	if got := rel.got(t, tex); got != "" || m.fd < 0 {
		t.Errorf("released %q while its memory is read (fd %d)", got, m.fd)
	}
	close(syncs.block)
	waitFor(t, "the buffer to be let go", func() bool {
		tex.mu.Lock()
		defer tex.mu.Unlock()
		return buf.freed
	})
	if got := rel.got(t, tex); got != "copied" {
		t.Errorf("released %q once the copy ended, want copied", got)
	}
	x.drain()
	if c := x.pixel(60, 80); c != texGreen {
		t.Errorf("shows %v", c)
	}
	if _, err := unix.FcntlInt(uintptr(m.fd), unix.F_GETFD, 0); m.fd != -1 || err == nil {
		t.Errorf("the buffer is closed and its descriptor is %d (%v)", m.fd, err)
	}
}

// A fence is waited for before the memory is read.
func TestTextureDmabufWaitsForFence(t *testing.T) {
	syncs := newFakeDmabufs(t)
	a := texApp{class: "w-[2px] h-[2px]"}
	x := newWinHarness(t, Options{Root: a.root()})
	fd := memfd(t, 16)
	fillMemfd(t, fd, fourccXR24, 2, 2, 0, 8, func(int, int) color.RGBA { return texRed })
	buf, err := a.tex.Import(Dmabuf{Width: 2, Height: 2, Fourcc: fourccXR24, Planes: []DmabufPlane{{FD: fd, Stride: 8}}})
	if err != nil {
		t.Fatal(err)
	}
	f := newChanFence()
	if err := a.tex.Submit(buf, TextureFrame{Fence: f}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the wait", func() bool { w, _, _ := f.state(); return w == 1 })
	syncs.mu.Lock()
	begun := syncs.begun
	syncs.mu.Unlock()
	if begun != 1 { // the import's
		t.Error("the memory was read before its fence signaled")
	}
	fillMemfd(t, fd, fourccXR24, 2, 2, 0, 8, func(int, int) color.RGBA { return texBlue })
	f.ch <- nil
	waitFor(t, "the copy", func() bool { return readyFrame(a.tex) })
	x.drain()
	if c := x.pixel(17, 49); c != texBlue {
		t.Errorf("shows %v, want what was drawn before the fence signaled", c)
	}
}

// With a device that imports, linear memory it refuses is copied.
func TestTextureDmabufFallback(t *testing.T) {
	newFakeDmabufs(t)
	var a texApp
	_, imp, _ := newTexImport(t, &a, 0)
	imp.err = fmt.Errorf("no linear memory: %w", ErrTextureUnsupported)
	b, err := a.tex.Import(&Dmabuf{Width: 2, Height: 2, Fourcc: fourccAR24, Planes: []DmabufPlane{{FD: memfd(t, 16), Stride: 8}}})
	if err != nil || b.mem == nil || b.content != nil {
		t.Errorf("%+v, %v", b, err)
	}
}

// ---------------------------------------------------------------------
// Sync files: a pipe's read end is readable when something was written, as
// a sync file is once it signaled.

func pipe(t *testing.T) (r, w int) {
	t.Helper()
	fds := make([]int, 2)
	if err := unix.Pipe2(fds, unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	return fds[0], fds[1]
}

// openFD reports whether fd is still the pipe end ino.
func fdIno(fd int) uint64 {
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil {
		return 0
	}
	return st.Ino
}

func TestSyncFile(t *testing.T) {
	// Signaled before and during the wait.
	for _, before := range []bool{true, false} {
		r, w := pipe(t)
		ino := fdIno(r)
		f := SyncFile(r)
		done := make(chan error, 1)
		if before {
			unix.Write(w, []byte{1})
		}
		go func() { done <- f.Wait(context.Background()) }()
		if !before {
			select {
			case err := <-done:
				t.Fatalf("returned %v before it signaled", err)
			case <-time.After(20 * time.Millisecond):
			}
			unix.Write(w, []byte{1})
		}
		if err := <-done; err != nil {
			t.Error(err)
		}
		if fdIno(r) == ino {
			t.Error("the descriptor is open after the wait")
		}
		// Waited for again, and freed: nothing is closed twice.
		again, w2 := pipe(t) // likely the same number
		ino = fdIno(again)
		if err := f.Wait(context.Background()); err != nil {
			t.Errorf("waited for again: %v", err)
		}
		closeFence(f)
		if fdIno(again) != ino {
			t.Error("a descriptor reused since was closed")
		}
		for _, fd := range []int{w, again, w2} {
			unix.Close(fd)
		}
	}

	// Cancelled: the fence is freed afterwards, once.
	r, w := pipe(t)
	defer unix.Close(w)
	ino := fdIno(r)
	f := SyncFile(r)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.Wait(ctx) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
	if err := f.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("a context already done: %v", err)
	}
	if fdIno(r) != ino {
		t.Error("closed by a wait that did not end")
	}
	closeFence(f)
	if fdIno(r) == ino {
		t.Error("open after the fence was freed")
	}
	if err := f.Wait(context.Background()); err == nil {
		t.Error("a freed fence signaled")
	}

	// Already signaled, as Vulkan exports it.
	if err := SyncFile(-1).Wait(context.Background()); err != nil {
		t.Errorf("-1: %v", err)
	}
	// Not open.
	if err := SyncFile(1 << 20).Wait(context.Background()); !errors.Is(err, unix.EBADF) {
		t.Errorf("a closed descriptor: %v", err)
	}
}

// A sync file is closed once whatever happens to its frame.
func TestSyncFileOfFrames(t *testing.T) {
	newFakeDmabufs(t)
	var a texApp
	x := newWinHarness(t, Options{Root: a.root()})
	tex := a.tex
	var bufs []*TextureBuffer
	for range 4 {
		b, err := tex.Import(Dmabuf{Width: 2, Height: 2, Fourcc: fourccAR24, Planes: []DmabufPlane{{FD: memfd(t, 16), Stride: 8}}})
		if err != nil {
			t.Fatal(err)
		}
		bufs = append(bufs, b)
	}
	var reads, inos []int
	var writes []int
	fence := func() Fence {
		r, w := pipe(t)
		reads, writes, inos = append(reads, r), append(writes, w), append(inos, int(fdIno(r)))
		return SyncFile(r)
	}
	closed := func(i int) bool { return int(fdIno(reads[i])) != inos[i] }
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(tex.Submit(bufs[0], TextureFrame{Fence: fence()})) // 0: shown
	must(tex.Submit(bufs[1], TextureFrame{Fence: fence()})) // 1: replaced by 2
	must(tex.Submit(bufs[2], TextureFrame{Fence: fence()})) // 2: waits at the unmount
	if err := tex.Submit(nil, TextureFrame{Fence: fence()}); err == nil {
		t.Error("no error")
	}
	if err := tex.Submit(bufs[2], TextureFrame{Fence: fence()}); err == nil { // 4: held
		t.Error("no error")
	}
	waitFor(t, "the replaced and the refused ones to close", func() bool { return closed(1) && closed(3) && closed(4) })
	if closed(0) || closed(2) {
		t.Error("a fence that is waited for was closed")
	}
	unix.Write(writes[0], []byte{1})
	waitFor(t, "the signaled one to close", func() bool { return closed(0) })
	a.closed.Set(true)
	x.drain()
	waitFor(t, "the one of the unmounted texture to close", func() bool { return closed(2) })
	if err := tex.Submit(bufs[3], TextureFrame{Fence: fence()}); !errors.Is(err, ErrTextureClosed) {
		t.Errorf("after the unmount: %v", err)
	}
	waitFor(t, "the one submitted after it to close", func() bool { return closed(5) })
	for _, w := range writes {
		unix.Close(w)
	}
}

// BenchmarkTextureDmabuf is the copy of a 1280x720 dmabuf frame.
func BenchmarkTextureDmabuf(b *testing.B) {
	const w, h = 1280, 720
	src := make([]byte, 4*w*h)
	m := &mappedDmabuf{data: src, stride: 4 * w, w: w, h: h}
	dst := image.NewRGBA(image.Rect(0, 0, w, h)).Pix
	was := dmabufSync
	dmabufSync = func(int, uint64) error { return nil }
	defer func() { dmabufSync = was }()
	for _, f := range mappedFourccs {
		b.Run(fourccString(f), func(b *testing.B) {
			m.swap, m.opaque, _ = textureFourcc(f)
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for b.Loop() {
				if err := m.read(dst); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
