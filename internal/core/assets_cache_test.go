package core

import (
	"image"
	"runtime"
	"testing"
	"weak"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestConvCacheEvictionReleases: an entry evicted from the BgImage cache
// is not kept alive by the backing array of the LRU order.
func TestConvCacheEvictionReleases(t *testing.T) {
	c := &convCache{m: map[image.Image]*convEntry{}}
	entry := func() *convEntry {
		src := image.NewRGBA(image.Rect(0, 0, 4, 4))
		return &convEntry{src: src, img: gfx.NewImage(src)}
	}
	size := len(entry().img.Pix())
	first := entry()
	w := weak.Make(first)
	c.add(first, 3*size)
	first = nil
	for range 3 { // the fourth entry evicts the first
		c.add(entry(), 3*size)
	}
	if len(c.order) != 3 || len(c.m) != 3 || c.bytes != 3*size {
		t.Fatalf("cache holds %d entries, %d keys, %d bytes", len(c.order), len(c.m), c.bytes)
	}
	runtime.GC()
	if w.Value() != nil {
		t.Error("the evicted entry is still reachable")
	}
	runtime.KeepAlive(c)
}
