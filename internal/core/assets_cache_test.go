package core

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"runtime"
	"testing"
	"testing/fstest"
	"time"
	"weak"

	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/style"
)

// The release of decoded images that nothing shows (assets.go, "Releasing
// images", ADR-242).

// bigPNG encodes a w x h opaque image, larger than keepBytes when w*h*4
// is, with a pattern that depends on seed.
func bigPNG(t *testing.T, w, h int, seed uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			o := img.PixOffset(x, y)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = uint8(x)^seed, uint8(y)+seed, uint8(x+y), 255
		}
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// cachedSlot returns the slot of key k, or nil.
func cachedSlot(k any) *assetSlot {
	assets.mu.Lock()
	defer assets.mu.Unlock()
	return assets.slots[k]
}

// totalDecodes returns the lookups that missed the cache.
func totalDecodes() int {
	assets.mu.Lock()
	defer assets.mu.Unlock()
	return assets.decodes
}

// collectUntil runs the garbage collector, and lets the cleanups run,
// until cond holds; it reports whether it did within a second.
func collectUntil(cond func() bool) bool {
	deadline := time.Now().Add(time.Second)
	for {
		runtime.GC()
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond) // the cleanups run on their own goroutine
	}
}

// gone reports whether the cache no longer has key k.
func gone(k any) func() bool { return func() bool { return cachedSlot(k) == nil } }

// frameRig renders a tree into a framebuffer it keeps, incrementally, as
// the host does.
type frameRig struct {
	t  *testing.T
	tr *Tree
	fb *image.RGBA
	r  raster.Renderer
}

func newFrameRig(t *testing.T, w, h int, root Node) *frameRig {
	tr := newTestTree(t, float32(w), float32(h), root, nil)
	return &frameRig{t: t, tr: tr, fb: image.NewRGBA(image.Rect(0, 0, w, h))}
}

// frame runs a frame and returns a copy of its pixels, checking that the
// incremental rasterization equals a full one.
func (x *frameRig) frame() []byte {
	x.t.Helper()
	x.tr.Frame()
	x.tr.RasterizeFrame(x.fb, &x.r, false)
	full := image.NewRGBA(x.fb.Rect)
	raster.Clear(full, image.Rectangle{}, x.tr.clearColor())
	var r raster.Renderer
	r.Render(full, x.tr.LastFrame(), raster.Options{Scale: x.tr.lay.cur.scale})
	if !bytes.Equal(full.Pix, x.fb.Pix) {
		x.t.Fatal("the incremental frame differs from a full rasterization")
	}
	return bytes.Clone(x.fb.Pix)
}

// TestUnshownBackgroundReleased: a background image stays cached while a
// layer shows it, even through collections and recordings that look it
// up again; once no layer shows it, it is released, and when it comes
// back it is decoded again into the same pixels.
func TestUnshownBackgroundReleased(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	const w, h = 1100, 1000 // 4.4 MB: more than the keep list holds
	if w*h*4 <= keepBytes {
		t.Fatal("the test image must be larger than keepBytes")
	}
	RegisterAssets(fstest.MapFS{"a.png": {Data: bigPNG(t, w, h, 0)}, "b.png": {Data: bigPNG(t, w, h, 90)}})
	var setB, setRed Setter[bool]
	app := Component(func() Node {
		b, sb := UseState(false)
		red, sr := UseState(false)
		setB, setRed = sb, sr
		bg := "bg-[url(a.png)]"
		if b {
			bg = "bg-[url(b.png)]"
		}
		dot := "bg-blue-500"
		if red {
			dot = "bg-red-500"
		}
		return div(Class("size-full bg-cover "+bg), div(Class("size-4 "+dot)))
	})
	x := newFrameRig(t, 64, 48, app)
	first := x.frame()
	a := cachedSlot(pathKey("a.png"))
	if a == nil || a.weak.Value() == nil {
		t.Fatal("a.png is not cached")
	}
	wa := a.weak

	// Shown: collections keep it, and recordings find it.
	decodes := totalDecodes()
	for i := range 3 {
		runtime.GC()
		setRed.Set(i%2 == 0) // the root layer is recorded again
		x.frame()
	}
	if totalDecodes() != decodes || wa.Value() == nil {
		t.Fatalf("a shown image was released: %d decodes (was %d)", totalDecodes(), decodes)
	}

	// Not shown: released.
	setB.Set(true)
	second := x.frame()
	if bytes.Equal(first, second) {
		t.Fatal("b.png looks like a.png")
	}
	if !collectUntil(gone(pathKey("a.png"))) {
		t.Fatal("a.png is still cached when nothing shows it")
	}
	if wa.Value() != nil {
		t.Error("a.png is still alive")
	}
	if cachedSlot(pathKey("b.png")) == nil {
		t.Error("b.png, shown, was released")
	}

	// Back: decoded again, the same pixels.
	decodes = totalDecodes()
	setB.Set(false)
	setRed.Set(false)
	if again := x.frame(); !bytes.Equal(again, first) {
		t.Error("a.png shown again has other pixels")
	}
	if totalDecodes() != decodes+1 {
		t.Errorf("a.png shown again: %d decodes, want 1", totalDecodes()-decodes)
	}
}

// TestUnmountedImgReleased: the decoded source of an Img is released once
// no mounted box has it, and is the same image when it is mounted again
// while another box still shows it.
func TestUnmountedImgReleased(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	data := bigPNG(t, 1100, 1000, 7)
	key := sha256.Sum256(data)
	var setN Setter[int]
	app := Component(func() Node {
		n, s := UseState(2)
		setN = s
		var kids []Child
		for i := range n {
			kids = append(kids, img(Key(string(rune('a'+i))), Class("size-8"), SrcBytes(data)))
		}
		return div(kids...)
	})
	x := newFrameRig(t, 64, 48, app)
	first := x.frame()
	s := cachedSlot(key)
	if s == nil {
		t.Fatal("the image is not cached")
	}
	wa := s.weak
	decodes := totalDecodes()
	setN.Set(1) // one Img left: it still holds the image
	x.frame()
	runtime.GC()
	setN.Set(2)
	if again := x.frame(); !bytes.Equal(again, first) || totalDecodes() != decodes {
		t.Errorf("remounted beside a shown copy: %d decodes, same pixels %v", totalDecodes()-decodes, bytes.Equal(again, first))
	}
	setN.Set(0)
	x.frame()
	if !collectUntil(gone(key)) || wa.Value() != nil {
		t.Fatal("the image of unmounted Imgs is still cached")
	}
	setN.Set(2)
	if again := x.frame(); !bytes.Equal(again, first) {
		t.Error("the image mounted again has other pixels")
	}
	if totalDecodes() != decodes+1 {
		t.Errorf("mounted again: %d decodes, want 1", totalDecodes()-decodes)
	}
}

// TestDeferredMaskKept: a mask image painted when a deferred layer is
// composited (not recorded) is held by the frame: frames that only
// composite do not decode it again after a collection.
func TestDeferredMaskKept(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	RegisterAssets(fstest.MapFS{"m.png": {Data: bigPNG(t, 1100, 1000, 3)}})
	var pos *Value[float32]
	app := Component(func() Node {
		pos = UseValue(float32(0))
		return div(div(Class("size-10 bg-black mask-[url(m.png)] mask-cover"),
			Style(style.TranslateFn(func() (float32, float32) { return pos.Get(), 0 }))))
	})
	x := newFrameRig(t, 64, 48, app)
	x.frame()
	if cachedSlot(pathKey("m.png")) == nil {
		t.Fatal("the mask is not cached")
	}
	decodes := totalDecodes()
	for i := range 3 {
		runtime.GC()
		pos.Set(float32(i + 1)) // the deferred layer moves: composited again
		x.frame()
	}
	if totalDecodes() != decodes {
		t.Errorf("a shown mask was decoded %d times again", totalDecodes()-decodes)
	}
}

// TestKeepList: the images looked up last stay cached without holders up
// to keepBytes, least recently used out first; one larger than that is
// never held by the list.
func TestKeepList(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	// Icons: parsed once, though nothing holds them between the parses.
	if _, err := ParseIcon(checkSVG); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	decodes := totalDecodes()
	if _, err := ParseIcon(checkSVG); err != nil || totalDecodes() != decodes {
		t.Errorf("a recent icon was parsed again: %v, %d parses", err, totalDecodes()-decodes)
	}

	// Raster images of a quarter of the budget: the fifth pushes the
	// first out of the list, and the collector releases it.
	conv := func(seed uint8) *imageAsset {
		src := image.NewRGBA(image.Rect(0, 0, 512, 512)) // 1 MiB
		src.Pix[0] = seed
		return convertBgImage(src)
	}
	first := weak.Make(conv(1))
	for i := range 3 {
		conv(uint8(2 + i))
	}
	// 4 MiB and the icon exceed the budget: the icon, least recent, left.
	assets.mu.Lock()
	n, cost := 0, assets.keepBytes
	for a := assets.keepHead; a != nil; a = a.next {
		n++
	}
	assets.mu.Unlock()
	if n != 4 || cost != 4<<20 {
		t.Errorf("keep list: %d images, %d bytes; want 4, %d", n, cost, 4<<20)
	}
	runtime.GC()
	if first.Value() == nil {
		t.Error("an image in the keep list was released")
	}
	conv(9)
	if !collectUntil(func() bool { return first.Value() == nil }) {
		t.Error("the least recently used image is still alive")
	}

	// Larger than the budget: never in the list.
	big := convertBgImage(image.NewRGBA(image.Rect(0, 0, 1100, 1000)))
	if big.kept {
		t.Error("an image larger than keepBytes is in the keep list")
	}
	runtime.KeepAlive(big)
	assets.mu.Lock()
	if assets.keepBytes > keepBytes {
		t.Errorf("the keep list holds %d bytes, more than %d", assets.keepBytes, keepBytes)
	}
	assets.mu.Unlock()
}

// TestFailuresBounded: the cache keeps at most maxFailures failures, so an
// app that previews a document at every keystroke (most versions invalid)
// does not grow it without bound; the oldest failure goes first.
func TestFailuresBounded(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	bad := func(i int) string { return fmt.Sprintf("<svg %d", i) }
	for i := range maxFailures + 10 {
		if _, err := ParseIcon(bad(i)); err == nil {
			t.Fatalf("document %d parsed", i)
		}
	}
	assets.mu.Lock()
	n := len(assets.slots)
	assets.mu.Unlock()
	if n != maxFailures {
		t.Errorf("the cache holds %d slots; want %d", n, maxFailures)
	}
	if cachedSlot(iconKey(bad(0))) != nil {
		t.Error("the oldest failure is still cached")
	}
	if s := cachedSlot(iconKey(bad(maxFailures + 9))); s == nil || s.err == nil {
		t.Error("the newest failure is not cached")
	}
}
