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
	"runtime"
	"strings"
	"sync"
	"weak"

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
// their content, not by their extension; SVG files are parsed by
// internal/svg and drawn as vectors (ADR-098). Img's SrcPath and
// mask-[url()] use the same registry and cache (media.go).
//
// A decoded file is converted to a gfx.Image once and cached by its
// cleaned path (assets do not change while the app runs); a missing or
// undecodable file is remembered too, until another file system is
// registered. The paint phase reports it once per path to the dev log
// (Config.Log), with the registered file systems, and paints nothing for
// that layer.
//
// Decoded images given with tela.BgImage (style.BackgroundLayer.Image) are
// converted once per image.Image value, so that passing the same image on
// every build converts it once; an image that cannot be a map key (its
// dynamic type is not comparable, or it holds a value that is not in an
// interface field: hashable) is converted on every paint.
//
// # Releasing images (ADR-242)
//
// The cache (assetRegistry.slots) holds its images weakly: an image stays
// while something that shows it holds it, and is released by the garbage
// collector once nothing does. The holders are the boxes of Img and Icon
// elements (boxMedia), the layers whose lists paint a background or mask
// image (layer.uses, painter.hold) and the composed frames (frame.uses: a
// mask composited with a deferred layer). A lookup while the image is
// alive returns the same image, so a layer recorded again draws the same
// gfx.Image (the damage sees no change); once it is gone, the next lookup
// decodes the source again, into the same pixels. To keep an image that
// comes back soon cheap (a row scrolled out and in, an icon in a menu
// opened again), the images looked up last are also held strongly, up to
// keepBytes in all (least recently used out first); a larger image is
// never held there, so a wallpaper the app stops showing is released at
// the next collections. Failures are cached strongly (they hold no
// pixels). A slot whose image was collected is removed by a cleanup
// (runtime.AddCleanup), so the keys (the tela.BgImage values among them)
// do not outlive their images.

// assetRegistry is the process-wide registry of asset file systems and
// the cache of decoded images.
type assetRegistry struct {
	mu  sync.Mutex
	fss []fs.FS
	// gen counts the registrations: failures to load a path are stamped
	// with it and retried after a new registration.
	gen uint64
	// slots caches decoded sources by key: pathKey (the cleaned path of
	// url() and SrcPath), [32]byte (the SHA-256 of SrcBytes data), fsKey
	// (SrcFS), iconKey (icon text) and convKey (tela.BgImage values).
	slots map[any]*assetSlot
	// keepHead and keepTail are the ends of the keep list (least recently
	// used first), keepBytes the cost of its images.
	keepHead, keepTail *imageAsset
	keepBytes          int
	// decodes counts the lookups that missed the cache (tests).
	decodes int
}

// pathKey keys the cache with the cleaned path of a url() or SrcPath;
// convKey with a tela.BgImage image.
type (
	pathKey string
	convKey struct{ img image.Image }
)

// assetSlot is a cached source: its decoded image, held weakly, or the
// error of its load.
type assetSlot struct {
	key  any
	weak weak.Pointer[imageAsset]
	err  error
	gen  uint64 // the registration generation of a failure
}

// keepBytes bounds the cost of the images the cache holds strongly
// because they were looked up last (the keep list).
const keepBytes = 4 << 20

var assets = assetRegistry{slots: map[any]*assetSlot{}}

// RegisterAssets adds fsys to the file systems that background url()s
// are resolved in, after those registered before. It is safe to call from
// any goroutine, usually from an init function.
func RegisterAssets(fsys fs.FS) {
	if fsys == nil {
		panic("tela: Assets(nil): pass a file system such as an embed.FS or os.DirFS(\"assets\")")
	}
	assets.mu.Lock()
	assets.fss = append(assets.fss, fsys)
	assets.gen++
	assets.mu.Unlock()
	if devStateEnabled {
		devAssetAdded(fsys) // tela dev: watch the directory (ADR-114)
	}
}

// resetAssets forgets every registered file system and cached image
// (tests).
func resetAssets() {
	assets.mu.Lock()
	defer assets.mu.Unlock()
	assets.fss = nil
	assets.slots = map[any]*assetSlot{}
	assets.gen++
	assets.clearKeep()
	assets.decodes = 0
}

// assetPath returns the fs.FS path of the url() u.
func assetPath(u string) (string, error) {
	if strings.Contains(u, "://") || strings.HasPrefix(u, "data:") {
		return "", fmt.Errorf("image %q: remote and data: URLs are not supported; put the file in a file system registered with tela.Assets", u)
	}
	p := strings.TrimLeft(u, "/")
	p = path.Clean(p)
	if p == "." || !fs.ValidPath(p) {
		return "", fmt.Errorf("image %q is not a valid path: write it relative to the root of a file system registered with tela.Assets, without \"..\"", u)
	}
	return p, nil
}

// loadAsset returns the decoded image of url u, from the cache or from
// the registered file systems.
func loadAsset(u string) (*imageAsset, error) {
	p, err := assetPath(u)
	if err != nil {
		return nil, err
	}
	k := pathKey(p)
	assets.mu.Lock()
	if a, ok, err := assets.lookup(k); ok {
		assets.mu.Unlock()
		return a, err
	}
	fss, gen := append([]fs.FS(nil), assets.fss...), assets.gen
	assets.mu.Unlock()
	a, err := findAsset(u, p, fss)
	assets.mu.Lock()
	defer assets.mu.Unlock()
	return assets.store(k, a, err, gen)
}

// lookup returns the cached image of key k (a hit also moves it to the
// recent end of the keep list) or the error of its load; ok is false when
// the cache has neither: the image was never loaded, was released, or is
// a failure to load a path made before the last registration. assets.mu
// must be held.
func (r *assetRegistry) lookup(k any) (a *imageAsset, ok bool, err error) {
	if s := r.slots[k]; s != nil {
		if s.err != nil {
			if _, isPath := k.(pathKey); !isPath || s.gen == r.gen {
				return nil, true, s.err
			}
		} else if a := s.weak.Value(); a != nil {
			r.touch(a)
			return a, true, nil
		}
	}
	r.decodes++
	return nil, false, nil
}

// store caches the result of loading key k (a, or the error err; gen is
// the registration generation it was loaded with) and returns what the
// cache holds for k: the image another tree stored meanwhile, if it is
// still alive. assets.mu must be held.
func (r *assetRegistry) store(k any, a *imageAsset, err error, gen uint64) (*imageAsset, error) {
	if s := r.slots[k]; s != nil && s.err == nil {
		if b := s.weak.Value(); b != nil { // loaded meanwhile by another tree
			r.touch(b)
			return b, nil
		}
	}
	if a == nil && err == nil {
		return nil, nil // nothing to cache
	}
	s := &assetSlot{key: k, err: err, gen: gen}
	if a != nil {
		s.weak = weak.Make(a)
		runtime.AddCleanup(a, forgetSlot, s)
		r.touch(a)
	}
	r.slots[k] = s
	return a, s.err
}

// forgetSlot removes slot s, whose image was collected, unless the cache
// holds another slot for its key by now.
func forgetSlot(s *assetSlot) {
	assets.mu.Lock()
	defer assets.mu.Unlock()
	if assets.slots[s.key] == s {
		delete(assets.slots, s.key)
	}
}

// touch moves a to the recent end of the keep list, adding it, and drops
// the least recently used images while the list costs more than
// keepBytes. An image that costs more than keepBytes is not kept.
// assets.mu must be held.
func (r *assetRegistry) touch(a *imageAsset) {
	if a.kept {
		if r.keepTail == a {
			return
		}
		r.unlink(a)
	}
	if a.cost > keepBytes {
		return
	}
	a.kept = true
	a.prev, a.next = r.keepTail, nil
	if r.keepTail != nil {
		r.keepTail.next = a
	} else {
		r.keepHead = a
	}
	r.keepTail = a
	r.keepBytes += a.cost
	for r.keepBytes > keepBytes {
		r.unlink(r.keepHead)
	}
}

// unlink removes a from the keep list.
func (r *assetRegistry) unlink(a *imageAsset) {
	if a.prev != nil {
		a.prev.next = a.next
	} else {
		r.keepHead = a.next
	}
	if a.next != nil {
		a.next.prev = a.prev
	} else {
		r.keepTail = a.prev
	}
	a.prev, a.next, a.kept = nil, nil, false
	r.keepBytes -= a.cost
}

// clearKeep empties the keep list.
func (r *assetRegistry) clearKeep() {
	for r.keepHead != nil {
		r.unlink(r.keepHead)
	}
}

// findAsset reads and decodes path p (url u) from the first of fss that
// has it.
func findAsset(u, p string, fss []fs.FS) (*imageAsset, error) {
	if len(fss) == 0 {
		return nil, fmt.Errorf("image %q: no asset file system is registered; call tela.Assets(fsys) from an init function with the embed.FS or directory that holds it", u)
	}
	for _, fsys := range fss {
		data, err := fs.ReadFile(fsys, p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("image %q in %s: %v", u, describeFS(fsys), err)
		}
		a, err := decodeAsset(data)
		if err != nil {
			return nil, fmt.Errorf("image %q in %s cannot be decoded: %v", u, describeFS(fsys), err)
		}
		return a, nil
	}
	names := make([]string, len(fss))
	for i, f := range fss {
		names[i] = describeFS(f)
	}
	return nil, fmt.Errorf("image %q not found as %q in the registered asset file systems (tela.Assets, searched in order: %s); paths are relative to their roots", u, p, strings.Join(names, ", "))
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
	return nil, errors.New("unknown format; Tela decodes PNG, JPEG, GIF, WebP and SVG")
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

// convertBgImage returns the image of a tela.BgImage image, converted
// once while it is in use (or kept: see "Releasing images").
func convertBgImage(src image.Image) *imageAsset {
	if src == nil {
		return nil
	}
	conv := func() (*imageAsset, error) { return rasterAsset(gfx.NewImage(src)), nil }
	if !hashable(reflect.ValueOf(src)) {
		// Not a map key: a type that is not comparable, or a comparable
		// one holding such a value in an interface (hashing it panics).
		a, _ := conv()
		return a
	}
	a, _ := cachedMedia(convKey{src}, conv)
	return a
}
