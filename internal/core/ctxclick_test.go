package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// Under the Mac flavor a Control+primary click is a context click
// (Chromium on the Mac): the press and release events of a primary
// press, then OnContextMenu with the primary button and Ctrl instead of
// the click, unless a drag took the press. The other flavors click.
// (Buttons are logged as numbers: 1 is the primary one.)
func TestControlClickPerFlavor(t *testing.T) {
	for _, c := range []struct {
		fl   platform.Flavor
		want string
	}{
		{platform.FlavorMac, "pointerdown mousedown pointerup mouseup contextmenu:1:ctrl"},
		{platform.FlavorLinux, "pointerdown mousedown pointerup mouseup click:1:ctrl"},
		{platform.FlavorWindows, "pointerdown mousedown pointerup mouseup click:1:ctrl"},
	} {
		t.Run(c.fl.String(), func(t *testing.T) {
			var r recorder
			mouse := func(name string) Prop {
				return On(name, func(e MouseEvent) {
					s := fmt.Sprintf("%s:%d", name, e.Button)
					if e.Ctrl {
						s += ":ctrl"
					}
					r.add(s)
				})
			}
			root := div(Class("p-10"),
				div(Key("d"), Class("size-20 active:bg-black"),
					On("pointerdown", func(PointerEvent) { r.add("pointerdown") }),
					On("pointerup", func(PointerEvent) { r.add("pointerup") }),
					On("mousedown", func(MouseEvent) { r.add("mousedown") }),
					On("mouseup", func(MouseEvent) { r.add("mouseup") }),
					mouse("click"), mouse("dblclick"), mouse("contextmenu"),
					On("dragstart", func(DragEvent) { r.add("dragstart") })))
			x := newFlavorKeys(t, root, c.fl)
			x.frame()
			x.send(move(50, 50), withMods(down(50, 50), platform.ModCtrl))
			if d := keyed(x.tree(), "d"); d.istate&style.StateActive == 0 {
				t.Error("a Control+press does not make its element active")
			}
			x.send(withMods(up(50, 50), platform.ModCtrl))
			if got := r.String(); got != c.want {
				t.Errorf("Control+click: %q, want %q", got, c.want)
			}
			if c.fl != platform.FlavorMac {
				return
			}

			// A plain click right after it is a single click.
			r.reset()
			x.send(down(50, 50), up(50, 50))
			if got := r.String(); got != "pointerdown mousedown pointerup mouseup click:1" {
				t.Errorf("click after a context click: %q", got)
			}

			// The press decides: Control let go before the button still
			// opens the menu.
			r.reset()
			x.b.Advance(time.Second)
			x.send(withMods(down(50, 50), platform.ModCtrl), up(50, 50))
			if got := r.String(); got != "pointerdown mousedown pointerup mouseup contextmenu:1" {
				t.Errorf("Control let go before the button: %q", got)
			}

			// A drag takes the press: no menu at its release.
			r.reset()
			x.b.Advance(time.Second)
			ctrl := platform.ModCtrl
			x.send(withMods(down(50, 50), ctrl), withMods(moveHeld(70, 50), ctrl), withMods(moveHeld(90, 50), ctrl), withMods(up(90, 50), ctrl))
			if got := r.String(); got != "pointerdown mousedown dragstart pointerup mouseup" {
				t.Errorf("Control+drag: %q", got)
			}

			// Control pressed after the button is a plain click.
			r.reset()
			x.b.Advance(time.Second)
			x.send(down(50, 50), withMods(up(50, 50), platform.ModCtrl))
			if got := r.String(); got != "pointerdown mousedown pointerup mouseup click:1:ctrl" {
				t.Errorf("Control after the press: %q", got)
			}
		})
	}
}
