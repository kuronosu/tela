package core

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/fs"
	"path"
	"reflect"
	"strings"
	"sync"

	"golang.org/x/image/webp"

	"github.com/kuronosu/tela/internal/gfx"
)

// Background images (ADR-079).
//
// The url() of a background (bg-[url(img/hero.jpg)]) names a file in the
// file systems registered with RegisterAssets (tela.Assets), searched in
// registration order: the first that has the file wins. Paths are
// relative to the root of each file system, as fs.FS paths are; a leading
// "/" and "./" are dropped, so url(/img/a.png), url(./img/a.png) and
// url(img/a.png) are the same file. Remote and data: URLs are not
// supported. Files are decoded with the standard library and
// golang.org/x/image: PNG, JPEG, GIF (its first frame) and WebP, chosen by
// their content, not by their extension.
//
// A decoded file is converted to a gfx.Image once and kept for the life
// of the process, keyed by its cleaned path (assets do not change while
// the app runs); a missing or undecodable file is remembered too, until
// another file system is registered. The paint phase reports it once per
// path to the dev log (Config.Log), with the registered file systems, and
// paints nothing for that layer.
//
// Decoded images given with tela.BgImage (style.BackgroundLayer.Image) are
// converted once per image.Image value and kept in a cache bounded by
// size (bgImageCacheBytes, least recently used first), so that passing
// the same image on every build converts it once; an image whose dynamic
// type is not comparable is converted on every paint.

// assetRegistry is the process-wide registry of asset file systems and
// the cache of decoded images.
type assetRegistry struct {
	mu  sync.Mutex
	fss []fs.FS
	// byPath caches the result of loading a cleaned path; failures are
	// stamped with gen and retried after a new registration.
	byPath map[string]assetEntry
	gen    uint64
	// conv caches the conversions of tela.BgImage images.
	conv convCache
}

type assetEntry struct {
	img *gfx.Image
	err error
	gen uint64
}

var assets = assetRegistry{byPath: map[string]assetEntry{}}

// RegisterAssets adds fsys to the file systems that background url()s
// are resolved in, after those registered before. It is safe to call from
// any goroutine, usually from an init function.
func RegisterAssets(fsys fs.FS) {
	if fsys == nil {
		panic("tela: Assets(nil): pass a file system such as an embed.FS or os.DirFS(\"assets\")")
	}
	assets.mu.Lock()
	defer assets.mu.Unlock()
	assets.fss = append(assets.fss, fsys)
	assets.gen++
}

// resetAssets forgets every registered file system and cached image
// (tests).
func resetAssets() {
	assets.mu.Lock()
	defer assets.mu.Unlock()
	assets.fss = nil
	assets.byPath = map[string]assetEntry{}
	assets.gen++
	assets.conv = convCache{}
}

// assetPath returns the fs.FS path of the url() u.
func assetPath(u string) (string, error) {
	if strings.Contains(u, "://") || strings.HasPrefix(u, "data:") {
		return "", fmt.Errorf("background image %q: remote and data: URLs are not supported; put the file in a file system registered with tela.Assets", u)
	}
	p := strings.TrimLeft(u, "/")
	p = path.Clean(p)
	if p == "." || !fs.ValidPath(p) {
		return "", fmt.Errorf("background image %q is not a valid path: write it relative to the root of a file system registered with tela.Assets, without \"..\"", u)
	}
	return p, nil
}

// loadAsset returns the decoded image of url u, from the cache or from
// the registered file systems.
func loadAsset(u string) (*gfx.Image, error) {
	p, err := assetPath(u)
	if err != nil {
		return nil, err
	}
	assets.mu.Lock()
	e, ok := assets.byPath[p]
	if ok && (e.err == nil || e.gen == assets.gen) {
		assets.mu.Unlock()
		return e.img, e.err
	}
	fss, gen := append([]fs.FS(nil), assets.fss...), assets.gen
	assets.mu.Unlock()
	img, err := findAsset(u, p, fss)
	assets.mu.Lock()
	assets.byPath[p] = assetEntry{img: img, err: err, gen: gen}
	assets.mu.Unlock()
	return img, err
}

// findAsset reads and decodes path p (url u) from the first of fss that
// has it.
func findAsset(u, p string, fss []fs.FS) (*gfx.Image, error) {
	if len(fss) == 0 {
		return nil, fmt.Errorf("background image %q: no asset file system is registered; call tela.Assets(fsys) from an init function with the embed.FS or directory that holds it", u)
	}
	for _, fsys := range fss {
		data, err := fs.ReadFile(fsys, p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("background image %q in %s: %v", u, describeFS(fsys), err)
		}
		src, err := decodeImage(data)
		if err != nil {
			return nil, fmt.Errorf("background image %q in %s cannot be decoded: %v", u, describeFS(fsys), err)
		}
		return gfx.NewImage(src), nil
	}
	names := make([]string, len(fss))
	for i, f := range fss {
		names[i] = describeFS(f)
	}
	return nil, fmt.Errorf("background image %q not found as %q in the registered asset file systems (tela.Assets, searched in order: %s); paths are relative to their roots", u, p, strings.Join(names, ", "))
}

// decodeImage decodes PNG, JPEG, GIF (first frame) and WebP data,
// recognized by their signatures.
func decodeImage(data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return png.Decode(r)
	case bytes.HasPrefix(data, []byte{0xff, 0xd8}):
		return jpeg.Decode(r)
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return gif.Decode(r)
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return webp.Decode(r)
	}
	return nil, errors.New("unknown format; Tela decodes PNG, JPEG, GIF and WebP")
}

// describeFS names a file system for error messages: its type, and its
// directory for os.DirFS.
func describeFS(f fs.FS) string {
	v := reflect.ValueOf(f)
	if v.Kind() == reflect.String {
		return fmt.Sprintf("%T(%q)", f, v.String())
	}
	if s, ok := f.(fmt.Stringer); ok {
		return fmt.Sprintf("%T(%s)", f, s)
	}
	return fmt.Sprintf("%T", f)
}

// ---------------------------------------------------------------------
// tela.BgImage conversions.

// bgImageCacheBytes bounds the converted pixels kept for tela.BgImage
// images. The most recent image is kept even when larger.
const bgImageCacheBytes = 256 << 20

// convCache is a least-recently-used cache of converted images.
type convCache struct {
	m     map[image.Image]*convEntry
	order []*convEntry // least recently used first
	bytes int
}

type convEntry struct {
	src image.Image
	img *gfx.Image
}

// convertBgImage returns the gfx.Image of a tela.BgImage image.
func convertBgImage(src image.Image) *gfx.Image {
	if src == nil {
		return nil
	}
	if t := reflect.TypeOf(src); !t.Comparable() {
		return gfx.NewImage(src)
	}
	assets.mu.Lock()
	c := &assets.conv
	if e, ok := c.m[src]; ok {
		c.touch(e)
		assets.mu.Unlock()
		return e.img
	}
	assets.mu.Unlock()
	img := gfx.NewImage(src)
	assets.mu.Lock()
	defer assets.mu.Unlock()
	if e, ok := c.m[src]; ok { // converted meanwhile by another tree
		return e.img
	}
	if c.m == nil {
		c.m = map[image.Image]*convEntry{}
	}
	c.add(&convEntry{src: src, img: img}, bgImageCacheBytes)
	return img
}

// add inserts e as the most recently used entry and evicts the least
// recently used ones while the cache holds more than limit bytes (the
// newest entry stays even when larger).
func (c *convCache) add(e *convEntry, limit int) {
	c.m[e.src] = e
	c.order = append(c.order, e)
	c.bytes += len(e.img.Pix())
	for c.bytes > limit && len(c.order) > 1 {
		old := c.order[0]
		// Clear the slot: the backing array outlives the reslice, and a
		// pointer left in it would keep the evicted pixels alive.
		c.order[0] = nil
		c.order = c.order[1:]
		delete(c.m, old.src)
		c.bytes -= len(old.img.Pix())
	}
}

// touch moves e to the most recently used end.
func (c *convCache) touch(e *convEntry) {
	for i, x := range c.order {
		if x == e {
			copy(c.order[i:], c.order[i+1:])
			c.order[len(c.order)-1] = e
			return
		}
	}
}
