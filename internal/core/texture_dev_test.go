//go:build teladev

package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// A texture holds nothing to keep across a reload of tela dev, and says
// nothing about it: the state beside it is kept, its handle is made again.
func TestTextureDevState(t *testing.T) {
	var x *Texture
	var n int
	var set Setter[int]
	root := func() Node {
		x = UseTexture()
		UseRef(x) // a handle resets silently
		n, set = UseState(0)
		return texture(x, Class("w-40 h-20"))
	}
	mount := func(data []byte) *Tree {
		tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(m string) { t.Errorf("warned: %s", m) }})
		tr.SetViewport(gfx.Size{W: 200, H: 100})
		if data != nil {
			tr.SeedDevState(data)
		}
		tr.Mount(Component(root))
		tr.Frame()
		return tr
	}
	tr := mount(nil)
	set.Set(7)
	if err := x.SubmitImage(solid(2, 2, texRed), nil); err != nil {
		t.Fatal(err)
	}
	tr.Frame()
	el := tr.root.children[0]
	if got := devSlotSig(el, 0); got != "UseTexture" {
		t.Errorf("signature %q", got)
	}
	el.hookDev = nil
	if got := devSlotKind(el, 0); got != hookTexture {
		t.Errorf("without dev records, the slot is %v", got)
	}
	_, rep, err := mount(nil).CaptureDevState()
	if err != nil || len(rep.Reset) != 1 || !rep.Reset[0].Silent || rep.Reset[0].Hook != "UseRef" {
		t.Errorf("%v, resets %+v: want the Ref alone, silently", err, rep.Reset)
	}
	tr2 := mount(nil)
	set.Set(9)
	tr2.Frame()
	data, _, err := tr2.CaptureDevState()
	if err != nil {
		t.Fatal(err)
	}
	old := x
	tr3 := mount(data)
	tr3.Frame()
	if n != 9 || x == old {
		t.Errorf("after a reload the state is %d, the texture is the old one: %v", n, x == old)
	}
}
