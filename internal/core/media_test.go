package core

import (
	"image"
	"image/color"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

func icon(text string, c ...Child) Node {
	return Primitive(style.KindSVG, append([]Child{IconSrc(text)}, c...)...)
}

// loadAssetImage is loadAsset for raster files.
func loadAssetImage(u string) (*gfx.Image, error) {
	a, err := loadAsset(u)
	if err != nil {
		return nil, err
	}
	return a.raster, nil
}

const checkSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`

// wideSVG has a natural size of 48 x 24.
const wideSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="48" height="24" viewBox="0 0 48 24"><rect width="48" height="24" fill="#2563eb"/></svg>`

func mediaDecodes() int {
	assets.mu.Lock()
	defer assets.mu.Unlock()
	return assets.media.decodes
}

func TestIconCaching(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	var setBlue Setter[bool]
	app := Component(func() Node {
		blue, set := UseState(false)
		setBlue = set
		cls := "text-red-500"
		if blue {
			cls = "text-blue-500"
		}
		return div(Class(cls),
			icon(checkSVG, Key("a")), icon(checkSVG, Key("b"), Class("size-8")))
	})
	tr := newTestTree(t, 200, 100, app, nil)
	dl, _ := tr.Frame()
	if n := mediaDecodes(); n != 1 {
		t.Errorf("two icons with the same text: %d parses, want 1", n)
	}
	if !strings.Contains(dl.String(), "#fb2c36") {
		t.Errorf("the icon's stroke is not the text color:\n%s", dl)
	}
	recs := tr.svgRecords
	for range 3 {
		tr.Frame()
	}
	// Recolor: the text color changes, the document is recorded again
	// with the new currentColor but not parsed again.
	setBlue.Set(true)
	dl, _ = tr.Frame()
	if n := mediaDecodes(); n != 1 {
		t.Errorf("after a recolor: %d parses, want 1", n)
	}
	if !strings.Contains(dl.String(), "#2b7fff") || strings.Contains(dl.String(), "#fb2c36") {
		t.Errorf("recolored icon:\n%s", dl)
	}
	if tr.svgRecords != recs+2 {
		t.Errorf("records: %d, want %d (one per icon for the new color)", tr.svgRecords, recs+2)
	}
	// ParseIcon shares the cache.
	if _, err := ParseIcon(checkSVG); err != nil || mediaDecodes() != 1 {
		t.Errorf("ParseIcon: %v, %d parses", err, mediaDecodes())
	}
}

func TestImgSourceCaching(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	red := pngBytes(t, color.RGBA{R: 255, A: 255})
	fsys := &countFS{FS: fstest.MapFS{"img/a.png": {Data: red}, "logo.svg": {Data: []byte(wideSVG)}}}
	RegisterAssets(fsys)
	var setN Setter[int]
	app := Component(func() Node {
		_, set := UseState(0)
		setN = set
		return div(
			img(Key("bytes"), SrcBytes(red)),
			img(Key("copy"), SrcBytes(append([]byte(nil), red...))),
			img(Key("path"), SrcPath("/img/a.png")),
			img(Key("fs"), SrcFS(fsys, "logo.svg")),
		)
	})
	tr := newTestTree(t, 200, 200, app, nil)
	tr.Frame()
	decodes, reads := mediaDecodes(), fsys.reads.Load()
	if decodes != 2 { // the bytes (both slices: one content) and the fs file
		t.Errorf("decodes = %d, want 2", decodes)
	}
	for i := 1; i <= 3; i++ {
		setN.Set(i) // rebuild with the same sources
		tr.Frame()
	}
	if mediaDecodes() != decodes || fsys.reads.Load() != reads {
		t.Errorf("rebuilds decoded again: %d decodes (was %d), %d reads (was %d)", mediaDecodes(), decodes, fsys.reads.Load(), reads)
	}
	if b := mustFind(t, tr, "fs"); b.Rect.Width() != 48 || b.Rect.Height() != 24 {
		t.Errorf("SVG natural size: %v", b.Rect)
	}
	if b := mustFind(t, tr, "path"); b.Rect.Width() != 3 || b.Rect.Height() != 2 {
		t.Errorf("PNG natural size: %v", b.Rect)
	}
}

// wrapFS is a comparable file system type whose value may not be
// hashable: its interface field can hold a map (fstest.MapFS).
type wrapFS struct{ fs.FS }

// TestSrcFSUnhashable: a comparable fs.FS type whose value cannot be
// hashed (a struct holding a map in an interface field) never panics as
// a cache key: it loads without the cache, while one whose value can be
// hashed is cached.
func TestSrcFSUnhashable(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	unhashable := wrapFS{fstest.MapFS{"logo.svg": {Data: []byte(wideSVG)}}}
	counted := &countFS{FS: fstest.MapFS{"logo.svg": {Data: []byte(wideSVG)}}}
	wrapped := wrapFS{counted} // a pointer in the interface field
	tr := newTestTree(t, 200, 200, div(
		img(Key("a"), SrcFS(unhashable, "logo.svg")),
		img(Key("b"), SrcFS(wrapped, "logo.svg")),
		img(Key("c"), SrcFS(wrapped, "logo.svg")),
	), nil)
	tr.Frame()
	for _, k := range []string{"a", "b", "c"} {
		if b := mustFind(t, tr, k); b.Rect.Width() != 48 || b.Rect.Height() != 24 {
			t.Errorf("%s: natural size %v, want 48x24", k, b.Rect)
		}
	}
	if n := counted.reads.Load(); n != 1 {
		t.Errorf("a hashable wrapper read %d times, want 1 (cached)", n)
	}
}

// TestCachedMediaUnlocksOnPanic: a key whose hashing panics (loadSource
// guards against it with hashable; cachedMedia is called directly here)
// does not leave the asset lock held, so later loads still work instead
// of deadlocking.
func TestCachedMediaUnlocksOnPanic(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("hashing an unhashable key did not panic")
			}
		}()
		cachedMedia(fsKey{wrapFS{fstest.MapFS{}}, "logo.svg"}, 0, func() (*imageAsset, error) {
			t.Error("load called for a key that cannot be hashed")
			return nil, nil
		})
	}()
	if !assets.mu.TryLock() {
		assets.mu.Unlock() // so that the cleanup does not deadlock
		t.Fatal("the asset lock is still held after the panic")
	}
	assets.mu.Unlock()
	doc, err := ParseIcon(checkSVG)
	if err != nil || doc == nil {
		t.Fatalf("a load after the panic: %v, %v", doc, err)
	}
}

// TestBgImageUnhashable: a tela.BgImage whose dynamic type is comparable
// but whose value cannot be hashed (an image held in an interface field)
// is converted without the cache instead of panicking under the asset
// lock.
func TestBgImageUnhashable(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	type sliceImage struct {
		image.Image
		_ []byte // not comparable
	}
	type wrapImage struct{ image.Image }
	src := wrapImage{sliceImage{Image: image.NewNRGBA(image.Rect(0, 0, 2, 2))}}
	var g *gfx.Image
	func() {
		defer func() {
			if r := recover(); r != nil {
				if !assets.mu.TryLock() {
					t.Error("the panic left the asset lock held")
				}
				assets.mu.Unlock() // so that the cleanup does not deadlock
				t.Fatalf("convertBgImage panicked: %v", r)
			}
		}()
		g = convertBgImage(src)
	}()
	if g == nil {
		t.Fatal("no image")
	}
	if !assets.mu.TryLock() {
		assets.mu.Unlock()
		t.Fatal("the asset lock is held")
	}
	assets.mu.Unlock()
}

// TestHasInterfaceCached: hashable looks up whether a type holds
// interfaces once per type, not by walking it on every load.
func TestHasInterfaceCached(t *testing.T) {
	type nested struct {
		_ [2]struct{ f fs.FS }
		_ int
	}
	typ := reflect.TypeOf(nested{})
	if !hasInterface(typ) {
		t.Fatal("hasInterface(nested) = false")
	}
	if v, ok := interfaceTypes.Load(typ); !ok || v != true {
		t.Errorf("hasInterface(nested) not cached: %v, %v", v, ok)
	}
	if hasInterface(reflect.TypeOf(struct{ a [3]int }{})) {
		t.Error("hasInterface(struct{ a [3]int }) = true")
	}
}

func TestHashable(t *testing.T) {
	unhashable := wrapFS{fstest.MapFS{}}
	counted := &countFS{FS: fstest.MapFS{}}
	for _, c := range []struct {
		v    any
		want bool
	}{
		{unhashable, false}, {wrapFS{counted}, true}, {fstest.MapFS{}, false}, {counted, true},
		{wrapFS{}, true}, {[2]fs.FS{counted, unhashable}, false}, {[2]fs.FS{counted, nil}, true},
		{struct{ a [3]int }{}, true}, {struct{ f func() }{}, false},
	} {
		if got := hashable(reflect.ValueOf(c.v)); got != c.want {
			t.Errorf("hashable(%T %v) = %v, want %v", c.v, c.v, got, c.want)
		}
	}
}

func TestIconSizing(t *testing.T) {
	tr := newTestTree(t, 300, 200, div(Class("text-2xl"),
		icon(checkSVG, Key("em")),
		icon(checkSVG, Key("size"), Class("size-8")),
		icon(checkSVG, Key("h"), Class("h-10")),
		div(Class("text-sm"), icon(checkSVG, Key("sm"))),
		img(Key("ratio"), SrcBytes([]byte(wideSVG)), Class("h-8")),
		img(Key("ratio-w"), SrcBytes([]byte(wideSVG)), Class("w-24")),
	), nil)
	tr.Frame()
	for key, want := range map[string][2]float32{
		"em": {24, 24}, "size": {32, 32}, "h": {24, 40}, "sm": {14, 14}, "ratio": {64, 32}, "ratio-w": {96, 48},
	} {
		if r := mustFind(t, tr, key).Rect; r.Width() != want[0] || r.Height() != want[1] {
			t.Errorf("%s: %gx%g, want %gx%g", key, r.Width(), r.Height(), want[0], want[1])
		}
	}
}

func TestIconPaintInheritance(t *testing.T) {
	s := paintDump(t, 100, 100, 1, button(Class("flex fill-current stroke-[1.5] text-red-500"),
		icon(checkSVG, Class("size-6"))))
	if !strings.Contains(s, "width=1.5") {
		t.Errorf("stroke-[1.5] on the button does not reach the icon:\n%s", s)
	}
	if !strings.Contains(s, "FillPath") || !strings.Contains(s, "#fb2c36") {
		t.Errorf("fill-current on the button: the icon's fill=none is not overridden:\n%s", s)
	}
	// On the icon itself; stroke-none removes the stroke.
	s = paintDump(t, 100, 100, 1, icon(checkSVG, Class("size-6 fill-sky-500 stroke-none")))
	if strings.Contains(s, "StrokePath") || !strings.Contains(s, "#00a6f4") {
		t.Errorf("fill-sky-500 stroke-none:\n%s", s)
	}
	// The root's own color attribute wins over an inherited text color,
	// not over the icon's own text-* class.
	colored := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2" color="#00ff00"><rect width="2" height="2" fill="currentColor"/></svg>`
	if s := paintDump(t, 50, 50, 1, div(Class("text-red-500"), icon(colored))); !strings.Contains(s, "#00ff00") {
		t.Errorf("inherited color beat the root's attribute:\n%s", s)
	}
	if s := paintDump(t, 50, 50, 1, icon(colored, Class("text-red-500"))); !strings.Contains(s, "#fb2c36") {
		t.Errorf("the icon's own text color must beat the attribute:\n%s", s)
	}
}

func TestIconSemantics(t *testing.T) {
	tr := newTestTree(t, 200, 100, div(
		button(icon(checkSVG), Text("Save")),
		button(icon(checkSVG, AriaLabel("Close"))),
		icon(checkSVG, AriaHidden(false), Key("plain")),
	), nil)
	tr.Frame()
	got := tr.AriaSnapshot()
	for _, want := range []string{`button "Save"`, `button "Close"`, `img "Close"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	if strings.Count(got, "img") != 2 { // the labelled one and the one with AriaHidden(false)
		t.Errorf("decorative icons must be hidden:\n%s", got)
	}
}

func TestMediaErrors(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	RegisterAssets(fstest.MapFS{"bad.png": {Data: []byte("\x89PNG\r\n\x1a\nnope")}})
	var logs []string
	tr := newTestTree(t, 200, 100, div(
		img(SrcPath("missing.png"), Class("size-4")),
		img(SrcPath("bad.png"), Class("size-4")),
		img(SrcBytes([]byte("plain text")), Class("size-4")),
		img(SrcBytes([]byte("plain text")), Class("size-4")),
		icon(`<svg><path d="M0 0`),
		icon(`<g/>`),
	), &logs)
	dl, _ := tr.Frame()
	tr.Frame()
	all := strings.Join(logs, "\n")
	for _, want := range []string{`"missing.png" not found`, `"bad.png"`, "SrcBytes cannot be decoded", "Icon: the SVG cannot be parsed", "not <svg>"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in the log:\n%s", want, all)
		}
	}
	if strings.Count(all, "SrcBytes cannot be decoded") != 1 {
		t.Errorf("the same bad bytes must be reported once:\n%s", all)
	}
	if strings.Contains(dl.String(), "DrawImage") || strings.Contains(dl.String(), "Path") {
		t.Errorf("bad sources must paint nothing:\n%s", dl)
	}
}

func TestObjectFit(t *testing.T) {
	src := pngBytes(t, color.RGBA{B: 255, A: 255}) // 3 x 2
	cases := []struct {
		class string
		want  []string
	}{
		{"w-30 h-10", []string{"DrawImage", "dst=[0,0,120,40]"}},
		{"w-30 h-10 object-contain", []string{"DrawImage", "dst=[30,0,90,40]"}},
		{"w-30 h-10 object-contain object-left", []string{"dst=[0,0,60,40]"}},
		{"w-30 h-10 object-cover", []string{"PushClipRect rect=[0,0,120,40]", "dst=[0,-20,120,60]", "PopClip"}},
		{"w-30 h-10 object-none object-bottom-right", []string{"dst=[117,38,120,40]"}},
		{"w-30 h-10 object-scale-down", []string{"dst=[58.5,19,61.5,21]"}},
	}
	for _, c := range cases {
		s := paintDump(t, 200, 100, 1, img(SrcBytes(src), Class(c.class)))
		order(t, s, c.want...)
	}
}

func TestTint(t *testing.T) {
	src := pngBytes(t, color.RGBA{G: 255, A: 255})
	s := paintDump(t, 100, 100, 1, img(SrcBytes(src), Tint(gfx.MustParseHex("#d97757")), Class("size-5")))
	order(t, s, "mask=alpha", "FillRect rect=[0,0,20,20] paint=#d97757", "PopLayer")
	s = paintDump(t, 100, 100, 1, img(SrcBytes([]byte(wideSVG)), Tint(gfx.MustParseHex("#d97757")), Class("h-5")))
	order(t, s, "mask=alpha", "FillRect rect=[0,0,40,20] paint=#d97757", "PopLayer")
}

func TestMaskPainting(t *testing.T) {
	resetAssets()
	t.Cleanup(resetAssets)
	RegisterAssets(fstest.MapFS{"m.png": {Data: pngBytes(t, color.RGBA{A: 255})}, "m.svg": {Data: []byte(wideSVG)}})
	s := paintDump(t, 100, 100, 1, div(Class("size-5 bg-[#d97757] mask-[url(m.png)] mask-contain mask-no-repeat")))
	order(t, s, "mask=alpha", "FillRect rect=[0,0,20,20] paint=#d97757", "PopLayer")
	if s := paintDump(t, 100, 100, 1, div(Class("size-5 bg-[#d97757] mask-[url(m.svg)]"))); !strings.Contains(s, "mask=alpha") {
		t.Errorf("SVG mask:\n%s", s)
	}
	// Two gradient families intersect: nested masks inside the mask.
	tr := newTestTree(t, 100, 100, div(Class("size-20 bg-sky-500 mask-t-from-50% mask-radial-from-20%")), nil)
	dl, _ := tr.Frame()
	if n := strings.Count(dl.String(), "mask="); n < 1 {
		t.Errorf("gradient masks:\n%s", dl)
	}
	// An unloadable mask image hides the element (a transparent layer).
	var logs []string
	tr = newTestTree(t, 100, 100, div(Class("size-5 bg-sky-500 mask-[url(none.png)]")), &logs)
	dl, _ = tr.Frame()
	if strings.Contains(dl.String(), "#00a6f4") || len(logs) == 0 {
		t.Errorf("missing mask image: %v\n%s", logs, dl)
	}
}

func TestImgSourceLastWins(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 5, 7))
	if s := paintDump(t, 100, 100, 1, img(SrcBytes([]byte(wideSVG)), Src(src))); !strings.Contains(s, "image=5x7") {
		t.Errorf("Src after SrcBytes:\n%s", s)
	}
	if s := paintDump(t, 100, 100, 1, img(Src(src), SrcBytes([]byte(wideSVG)))); strings.Contains(s, "DrawImage") || !strings.Contains(s, "#2563eb") {
		t.Errorf("SrcBytes after Src:\n%s", s)
	}
}
