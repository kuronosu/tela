package core

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// keyAt returns the key of the element hit at (x, y), or nil.
func keyAt(tr *Tree, x, y float32) any {
	el := tr.HitTest(x, y)
	if el == nil {
		return nil
	}
	return el.key
}

func TestHitTestClipPath(t *testing.T) {
	root := div(Key("root"), Class("relative h-screen"),
		// A circle of radius 50 centered at (60, 60).
		div(Key("circle"), Class("absolute left-[10px] top-[10px] size-[100px] bg-indigo-500 [clip-path:circle(50%)]"),
			div(Key("child"), Class("size-full"))),
		// A pentagram (evenodd: the center is a hole) at (150, 10).
		div(Key("star"), Class("absolute left-[150px] top-[10px] size-[100px] bg-violet-500 [clip-path:polygon(evenodd,50%_0,79%_91%,2%_35%,98%_35%,21%_91%)]")),
		// A rotated box clipped to its inset, at (10, 150).
		div(Key("rot"), Class("absolute left-[10px] top-[150px] size-[100px] rotate-45 bg-rose-500 [clip-path:inset(20px)]")),
	)
	var logs []string
	tr := newTestTree(t, 300, 300, root, &logs)
	tr.Frame()
	for _, c := range []struct {
		x, y float32
		want any
	}{
		{60, 60, "child"},  // the circle's center: the child, inside the clip
		{14, 14, "root"},   // the box's corner, outside the circle
		{105, 105, "root"}, // the opposite corner
		{105, 60, "child"}, // inside, near the right edge
		{200, 20, "star"},  // the star's top point
		{200, 55, "root"},  // the evenodd hole in the middle
		{160, 49, "star"},  // the left arm
		{160, 15, "root"},  // the bounding box's corner
		{60, 200, "rot"},   // the rotated box's center
		{60, 145, "root"},  // inside the rotated box's border box, outside its inset clip
		{22, 168, "root"},  // the unrotated inset's corner, outside after the rotation
		{60, 168, "rot"},   // up from the center, inside the rotated inset
	} {
		if got := keyAt(tr, c.x, c.y); got != c.want {
			t.Errorf("hit (%g, %g) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	if len(logs) != 0 {
		t.Errorf("dev messages: %v", logs)
	}
}

func TestPolygonContains(t *testing.T) {
	sq := []gfx.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	// Twice around: nonzero fills, evenodd does not.
	twice := append(append([]gfx.Point(nil), sq...), sq...)
	p := gfx.Point{X: 5, Y: 5}
	if !polygonContains(sq, gfx.NonZero, p) || !polygonContains(sq, gfx.EvenOdd, p) ||
		!polygonContains(twice, gfx.NonZero, p) || polygonContains(twice, gfx.EvenOdd, p) ||
		polygonContains(sq, gfx.NonZero, gfx.Point{X: 11, Y: 5}) || polygonContains(nil, gfx.NonZero, p) {
		t.Error("polygonContains")
	}
}

// TestBlendIsolation checks the layers: a stacking context whose
// descendants blend is a group; a blend layer carries its mode.
func TestBlendIsolation(t *testing.T) {
	root := div(Class("h-screen bg-white"),
		div(Key("sc"), Class("relative z-10 size-20"),
			div(Class("absolute inset-0 bg-sky-500")),
			div(Class("absolute inset-2 bg-amber-400 mix-blend-multiply"))),
		div(Class("relative size-20"),
			div(Class("absolute inset-2 bg-amber-400 mix-blend-screen"))),
	)
	tr := newTestTree(t, 200, 200, root, nil)
	dl, _ := tr.Frame()
	var layers []string
	for _, c := range dl.All() {
		if l, ok := c.(gfx.PushLayer); ok {
			layers = append(layers, fmt.Sprintf("%v/%g", l.Blend, l.Opacity))
		}
	}
	// The z-10 context isolates its multiply; the screen blends at the
	// top level, with the canvas: no group around it.
	if got := strings.Join(layers, " "); got != "screen/1 normal/1 multiply/1" {
		t.Errorf("layers: %s", got)
	}
}

// fxScene: a scroll container over an opaque background whose rows hold
// blended, clipped, image and clipped-text boxes that change with state,
// with a clipped and blended box moved by TranslateFn (a deferred layer).
func fxScene(st *sceneState) Node {
	photo := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for i := range 40 * 30 {
		photo.Set(i%40, i/40, color.RGBA{uint8(i % 40 * 6), uint8(i / 40 * 8), 160, 255})
	}
	row := func(i int) Node {
		v, set := UseState(0)
		st.setters[i] = set
		var kids []Child
		kids = append(kids, Class("relative flex h-[56px] items-center gap-3 px-3 hover:bg-slate-100"))
		switch i % 5 {
		case 0:
			blend := "mix-blend-multiply"
			if v%2 == 1 {
				blend = "mix-blend-difference"
			}
			kids = append(kids, div(Class("relative size-10 isolate bg-linear-to-r from-sky-400 to-pink-500"),
				div(Class("absolute inset-2 rounded-full bg-amber-400 "+blend))))
		case 1:
			clip := "[clip-path:circle(40%)] hover:[clip-path:circle(50%)]"
			if v%2 == 1 {
				clip = "[clip-path:polygon(50%_0,100%_100%,0_100%)]"
			}
			kids = append(kids, div(Class("size-10 bg-emerald-500 shadow-md "+clip)))
		case 2:
			cls := "h-10 w-16 bg-[url(img/photo.png)] bg-cover bg-center"
			if v%2 == 1 {
				cls = "h-10 w-16 bg-[url(img/tile.png)] bg-repeat-round rounded-lg"
			}
			kids = append(kids, div(Class(cls)))
			kids = append(kids, div(Class("h-10 w-12 bg-contain bg-no-repeat"), Style(style.BgImage(photo))))
		case 3:
			txt := fmt.Sprintf("Clip %d", v)
			kids = append(kids, div(Class("bg-linear-to-r from-indigo-500 to-amber-400 bg-clip-text text-xl font-bold text-transparent"), Text(txt)))
		case 4:
			kids = append(kids, div(Class("text-sm"), Text("Plain "),
				span(Class("bg-[url(img/photo.png)] bg-cover bg-clip-text font-black text-transparent"), Textf("img %d", v))))
		}
		return div(kids...)
	}
	comp := func() Node {
		st.refs[0] = UseElementRef()
		x := UseValue[float32](0)
		st.values[0] = x
		items := []Child{AttachRef(st.refs[0]), Key("list"), Class("grow overflow-y-auto bg-white")}
		for i := range 40 {
			items = append(items, WithKey(ComponentProps(row, i), i))
		}
		return div(Class("relative flex h-screen flex-col bg-slate-50"),
			div(Class("h-10 shrink-0 bg-slate-800 px-3 text-white"), Text("Effects")),
			div(items...),
			div(Class("absolute right-4 top-12 size-14 bg-rose-500 mix-blend-multiply [clip-path:inset(4px_round_10px)]"),
				Style(style.TranslateFn(func() (float32, float32) { return -x.Get(), x.Get() / 2 }))),
		)
	}
	return Component(comp)
}

func TestDamageEffects(t *testing.T) {
	registerFxAssets(t)
	steps := 120
	if testing.Short() {
		steps = 30
	}
	runScene(t, "effects", fxScene, commonOps, Config{InstantScroll: true}, steps)
}

// ---------------------------------------------------------------------
// Assets.

func pngBytes(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := range 6 {
		img.Set(i%3, i/3, c)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// countFS counts the files read from it.
type countFS struct {
	fs.FS
	reads atomic.Int32
}

func (c *countFS) Open(name string) (fs.File, error) {
	c.reads.Add(1)
	return c.FS.Open(name)
}

func TestAssetRegistry(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	if _, err := loadAsset("a.png"); err == nil || !strings.Contains(err.Error(), "no asset file system is registered") {
		t.Errorf("nothing registered: %v", err)
	}
	red, blue := pngBytes(t, color.RGBA{R: 255, A: 255}), pngBytes(t, color.RGBA{B: 255, A: 255})
	first := &countFS{FS: fstest.MapFS{"img/a.png": {Data: red}}}
	second := &countFS{FS: fstest.MapFS{"img/a.png": {Data: blue}, "img/b.png": {Data: blue}, "bad.png": {Data: []byte("not an image")}}}
	RegisterAssets(first)
	RegisterAssets(second)
	// The first file system that has the file wins; "/" and "./" are
	// dropped.
	for _, u := range []string{"img/a.png", "/img/a.png", "./img/a.png", "img/../img/a.png"} {
		img, err := loadAsset(u)
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		if p := img.Pix(); p[0] != 255 || p[2] != 0 {
			t.Errorf("%s: pixel %v, want the first file system's red", u, p[:4])
		}
	}
	if img, err := loadAsset("img/b.png"); err != nil || img.Pix()[2] != 255 {
		t.Errorf("img/b.png from the second file system: %v", err)
	}
	// Decoded once: the cache answers.
	reads := first.reads.Load() + second.reads.Load()
	a1, _ := loadAsset("img/a.png")
	a2, _ := loadAsset("/img/a.png")
	if a1 != a2 || first.reads.Load()+second.reads.Load() != reads {
		t.Error("an asset is read again")
	}
	_, err := loadAsset("img/missing.png")
	if err == nil {
		t.Fatal("a missing asset loads")
	}
	for _, s := range []string{`"img/missing.png"`, "*core.countFS", "searched in order"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("missing asset error %q does not mention %s", err, s)
		}
	}
	if _, err := loadAsset("bad.png"); err == nil || !strings.Contains(err.Error(), "cannot be decoded") {
		t.Errorf("undecodable: %v", err)
	}
	for _, u := range []string{"../x.png", "https://example.com/a.png", "data:image/png;base64,AAAA"} {
		if _, err := loadAsset(u); err == nil {
			t.Errorf("%s loads", u)
		}
	}
	// A failure is retried after another registration.
	RegisterAssets(fstest.MapFS{"img/missing.png": {Data: red}})
	if _, err := loadAsset("img/missing.png"); err != nil {
		t.Errorf("after registering it: %v", err)
	}
	if !strings.Contains(describeFS(os.DirFS("assets")), `"assets"`) {
		t.Errorf("describeFS(os.DirFS): %s", describeFS(os.DirFS("assets")))
	}
}

func TestAssetFormats(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	dir := filepath.Join("testdata", "chromium", "effects", "img")
	RegisterAssets(os.DirFS(dir))
	ref, err := loadAsset("tile.png")
	if err != nil {
		t.Fatal(err)
	}
	// Lossless WebP decodes to the PNG's pixels.
	webp, err := loadAsset("tile.webp")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(webp.Pix(), ref.Pix()) {
		t.Error("tile.webp differs from tile.png")
	}
	for _, name := range []string{"photo.jpg", "checker.gif", "zone.png"} {
		img, err := loadAsset(name)
		if err != nil || img.Width() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	gif, _ := loadAsset("checker.gif")
	if p := gif.Pix(); p[3] != 255 || p[7] != 0 {
		t.Errorf("checker.gif: first pixels %v", p[:8])
	}
	if _, err := decodeImage([]byte("BM....")); err == nil || !strings.Contains(err.Error(), "WebP") {
		t.Errorf("unknown format: %v", err)
	}
}

// TestMissingAssetReported checks the dev message of a missing background
// image: once per path, with the registered file systems, and nothing
// painted.
func TestMissingAssetReported(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	RegisterAssets(fstest.MapFS{"a.png": {Data: pngBytes(t, color.RGBA{R: 255, A: 255})}})
	var logs []string
	tr := newTestTree(t, 100, 100, div(Class("size-20 bg-[url(img/nope.png)] bg-cover")), &logs)
	dl, _ := tr.Frame()
	tr.SetScale(2) // paints again
	tr.Frame()
	if len(logs) != 1 || !strings.Contains(logs[0], `"img/nope.png"`) || !strings.Contains(logs[0], "fstest.MapFS") {
		t.Errorf("logs: %q", logs)
	}
	for _, c := range dl.All() {
		if _, ok := c.(gfx.DrawImage); ok {
			t.Error("a missing image is drawn")
		}
	}
}

// TestBgImageConverted checks that a tela.BgImage image is converted once
// across builds and frames.
func TestBgImageConverted(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	a := convertBgImage(img)
	if b := convertBgImage(img); a != b {
		t.Error("converted twice")
	}
	if c := convertBgImage(image.NewNRGBA(image.Rect(0, 0, 4, 4))); c == a {
		t.Error("another image shares the conversion")
	}
}

func TestImageMipLevels(t *testing.T) {
	img := gfx.NewImage(image.NewRGBA(image.Rect(0, 0, 9, 4)))
	for n, want := range [][2]int{{9, 4}, {4, 2}, {2, 1}, {1, 1}, {1, 1}} {
		l := img.Level(n)
		if l.Width() != want[0] || l.Height() != want[1] {
			t.Errorf("level %d: %dx%d, want %dx%d", n, l.Width(), l.Height(), want[0], want[1])
		}
	}
	if l1 := img.Level(1); l1 != img.Level(1) {
		t.Error("levels are not cached")
	}
	// A checkerboard averages to gray.
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range 16 {
		if (i%4+i/4)%2 == 0 {
			src.Set(i%4, i/4, color.White)
		} else {
			src.Set(i%4, i/4, color.Black)
		}
	}
	if p := gfx.NewImage(src).Level(1).Pix(); p[0] != 128 || p[3] != 255 {
		t.Errorf("box filter: %v", p[:4])
	}
}
