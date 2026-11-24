package gpu

import (
	"image"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// nullDevice is a Device that is only compared with others.
type nullDevice struct{ Device }

// fakeMemory is imported memory of no device.
type fakeMemory struct {
	pix    []byte
	closed bool
	reads  int
}

func (m *fakeMemory) open() bool { return !m.closed }
func (m *fakeMemory) pixels() []byte {
	m.reads++
	if m.closed {
		return nil
	}
	return m.pix
}
func (m *fakeMemory) close() { m.closed = true }

// content is a caller's External around imported memory.
type content struct {
	m     *Imported
	pix   []byte
	reads int
}

func (c *content) Imported() *Imported { return c.m }
func (c *content) Pixels() []byte      { c.reads++; return c.pix }

func fakeImport(dev Device, w, h int, copied bool) (*Imported, *fakeMemory) {
	mem := &fakeMemory{pix: make([]byte, 4*w*h)}
	return newImported(dev, &DmabufDescriptor{Width: w, Height: h, Fourcc: FourccARGB8888}, copied, mem), mem
}

func drawImages(imgs ...*gfx.Image) *gfx.DisplayList {
	return gfxtest.Build(func(b *gfx.Builder) {
		for i, img := range imgs {
			b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(float32(10+40*i), 10, 30, 30), gfx.SampleLinear)
		}
	})
}

// imagesOf returns the images the batches of a frame read, in order.
func imagesOf(f *Frame) []uint64 {
	var out []uint64
	for _, p := range f.Passes {
		for _, b := range p.Batches {
			if b.Image != 0 {
				out = append(out, b.Image)
			}
		}
	}
	return out
}

// TestEncodeImported checks what the Encoder does with an image of imported
// memory: its own device's is read where it lies, anything else by its
// pixels.
func TestEncodeImported(t *testing.T) {
	own, other := &nullDevice{}, &nullDevice{}
	size := image.Pt(400, 100)
	e := &Encoder{dev: own}

	m, mem := fakeImport(own, 20, 10, false)
	frame1 := gfx.NewExternalImage(20, 10, true, m.Frame())
	plain := gfx.NewImagePremul(1, 1, []byte{1, 2, 3, 255})
	f := e.Encode(drawImages(frame1, plain, frame1), size, Options{})
	if mem.reads != 0 {
		t.Fatalf("the pixels of imported memory were read %d times", mem.reads)
	}
	if len(f.Imports) != 1 || f.Imports[0] != m {
		t.Errorf("Imports = %v, want the one buffer once", f.Imports)
	}
	if len(f.Images) != 1 || f.Images[0].ID != plain.ID() {
		t.Errorf("%d images uploaded, want the ordinary one", len(f.Images))
	}
	if got := imagesOf(f); len(got) != 3 || got[0] != m.id || got[1] != plain.ID() || got[2] != m.id || m.id&importedID == 0 {
		t.Errorf("the batches read %v, want %d, %d, %d", got, m.id, plain.ID(), m.id)
	}
	if b := e.Memory().Images; b != 4 {
		t.Errorf("the Encoder counts %d bytes of images, want the ordinary image's 4", b)
	}
	// Another frame in the same buffer: nothing changes for the device, not
	// even the history of the resources. A frame without it names none.
	base := f.Res.Next
	f = e.Encode(drawImages(gfx.NewExternalImage(20, 10, true, m.Frame())), size, Options{})
	if len(f.Imports) != 1 || len(f.Images) != 0 || f.Res.Base != base || f.Res.Next != base {
		t.Errorf("a second frame: %d imports, %d uploads, history %v after %d", len(f.Imports), len(f.Images), f.Res, base)
	}
	if f = e.Encode(drawImages(plain), size, Options{}); len(f.Imports) != 0 {
		t.Errorf("a frame that draws no imported memory lists %d", len(f.Imports))
	}

	// An External of the caller's around the memory.
	c := &content{m: m, pix: make([]byte, 800)}
	f = e.Encode(drawImages(gfx.NewExternalImage(20, 10, true, c)), size, Options{})
	if len(f.Imports) != 1 || c.reads != 0 {
		t.Errorf("a caller's content: %d imports, %d reads", len(f.Imports), c.reads)
	}
	c.m = nil
	f = e.Encode(drawImages(gfx.NewExternalImage(20, 10, true, c)), size, Options{})
	if len(f.Imports) != 0 || len(f.Images) != 1 || c.reads != 1 {
		t.Errorf("a caller's content without memory: %d imports, %d uploads, %d reads", len(f.Imports), len(f.Images), c.reads)
	}

	// Another device's, one of another size, and a closed one are ordinary.
	foreign, foreignMem := fakeImport(other, 20, 10, false)
	for _, img := range []*gfx.Image{gfx.NewExternalImage(20, 10, true, foreign.Frame()), gfx.NewExternalImage(10, 20, true, m.Frame())} {
		f = e.Encode(drawImages(img), size, Options{})
		if len(f.Imports) != 0 || len(f.Images) != 1 || f.Images[0].ID != img.ID() || len(f.Images[0].Pix) != 800 {
			t.Errorf("an image that is not the device's: %d imports, uploads %v", len(f.Imports), f.Images)
		}
	}
	if foreignMem.reads != 1 || mem.reads != 1 {
		t.Errorf("the pixels were read %d and %d times, want once each", foreignMem.reads, mem.reads)
	}
	m.Close()
	f = e.Encode(drawImages(gfx.NewExternalImage(20, 10, true, m.Frame())), size, Options{})
	if len(f.Imports) != 0 || len(f.Images) != 1 || !bytesAllZero(f.Images[0].Pix) {
		t.Errorf("an image of closed memory: %d imports, %d uploads", len(f.Imports), len(f.Images))
	}
}

func bytesAllZero(b []byte) bool {
	return !slices.ContainsFunc(b, func(v byte) bool { return v != 0 })
}

// TestEncodeCopied checks memory the device cannot sample: each frame is an
// image uploaded on the device, and the buffer's frames follow each other
// in one texture.
func TestEncodeCopied(t *testing.T) {
	own := &nullDevice{}
	size := image.Pt(400, 100)
	e := &Encoder{dev: own}
	m, mem := fakeImport(own, 20, 10, true)
	var last uint64
	for i := range 4 {
		img := gfx.NewExternalImage(20, 10, true, m.Frame())
		dl := drawImages(img)
		f := e.Encode(dl, size, Options{})
		if len(f.Imports) != 0 || len(f.Images) != 1 || f.Images[0].From != m || f.Images[0].Pix != nil || f.Images[0].ID != img.ID() {
			t.Fatalf("frame %d: %d imports, uploads %+v", i, len(f.Imports), f.Images)
		}
		if got := imagesOf(f); !slices.Equal(got, []uint64{img.ID()}) {
			t.Errorf("frame %d: the batches read %v, want %d", i, got, img.ID())
		}
		if want := []uint64{last}; i > 0 && !slices.Equal(f.Releases, want) {
			t.Errorf("frame %d releases %v, want %v", i, f.Releases, want)
		}
		if b := e.Memory().Images; b != 800 {
			t.Errorf("frame %d: %d bytes of images, want one texture's", i, b)
		}
		// Drawn again, it is there.
		if f = e.Encode(dl, size, Options{}); len(f.Images) != 0 || len(f.Releases) != 0 {
			t.Errorf("frame %d drawn again: %d uploads, %d releases", i, len(f.Images), len(f.Releases))
		}
		last = img.ID()
	}
	if mem.reads != 0 {
		t.Errorf("the pixels of imported memory were read %d times", mem.reads)
	}
	// Buffers of one stream replace each other's frames.
	a := newImported(own, &DmabufDescriptor{Width: 20, Height: 10, Fourcc: FourccARGB8888, Stream: 9}, true, &fakeMemory{})
	b := newImported(own, &DmabufDescriptor{Width: 20, Height: 10, Fourcc: FourccARGB8888, Stream: 9}, true, &fakeMemory{})
	ia, ib := gfx.NewExternalImage(20, 10, true, a), gfx.NewExternalImage(20, 10, true, b)
	e.Encode(drawImages(ia), size, Options{})
	if f := e.Encode(drawImages(ib), size, Options{}); !slices.Equal(f.Releases, []uint64{ia.ID()}) {
		t.Errorf("a buffer of the same stream releases %v, want %d", f.Releases, ia.ID())
	}
}

// TestEncodeStream checks the textures of the frames of a stream: a new one
// takes the place of the ones before it, whatever else is drawn.
func TestEncodeStream(t *testing.T) {
	size := image.Pt(400, 100)
	e := &Encoder{}
	pix := make([]byte, 400)
	frame := func(stream uint64) *gfx.Image { return gfx.NewStreamImage(10, 10, pix, true, stream) }
	other := frame(2)
	still := gfx.NewImagePremul(10, 10, pix)
	e.Encode(drawImages(other, still), size, Options{})
	var prev *gfx.Image
	for i := range 3 * idleFrames {
		img := frame(1)
		f := e.Encode(drawImages(still, img, other), size, Options{})
		e.EndFrame()
		if len(f.Images) != 1 || f.Images[0].ID != img.ID() {
			t.Fatalf("frame %d uploads %d images", i, len(f.Images))
		}
		if prev != nil && !slices.Equal(f.Releases, []uint64{prev.ID()}) {
			t.Fatalf("frame %d releases %v, want %d", i, f.Releases, prev.ID())
		}
		if got := e.Memory().Images; got != 3*400 {
			t.Fatalf("frame %d: %d bytes of images, want three textures", i, got)
		}
		prev = img
	}
	// Both in one list, in any order: none is given up while the frame
	// draws it, and the older one goes with the next.
	a, b := frame(1), frame(1)
	f := e.Encode(drawImages(b, a), size, Options{})
	if len(f.Images) != 2 || !slices.Equal(f.Releases, []uint64{prev.ID()}) {
		t.Errorf("two frames of a stream in a list: %d uploads, releases %v (want %d)", len(f.Images), f.Releases, prev.ID())
	}
	c := frame(1)
	f = e.Encode(drawImages(a, c), size, Options{})
	if !slices.Equal(f.Releases, []uint64{b.ID()}) || len(f.Images) != 1 {
		t.Errorf("an older frame beside a new one: %d uploads, releases %v, want %d", len(f.Images), f.Releases, b.ID())
	}
	// An older frame drawn again does not take a newer one's texture, and
	// one given up in the same frame that draws it is kept.
	f = e.Encode(drawImages(b), size, Options{})
	if len(f.Images) != 1 || len(f.Releases) != 1 || f.Releases[0] != a.ID() {
		t.Errorf("an older frame drawn again: %d uploads, releases %v (want %d, older still)", len(f.Images), f.Releases, a.ID())
	}
	if f = e.Encode(drawImages(c), size, Options{}); len(f.Images) != 0 {
		t.Errorf("the newest frame was given up for an older one")
	}
	d := frame(1)
	f = e.Encode(drawImages(d, b), size, Options{})
	if !slices.Equal(f.Releases, []uint64{c.ID()}) {
		t.Errorf("a frame that draws what it gave up releases %v, want %d", f.Releases, c.ID())
	}
	for _, up := range f.Images {
		if up.ID == b.ID() {
			t.Errorf("a texture the device still has is uploaded again")
		}
	}
}
