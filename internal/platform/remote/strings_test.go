package remote

import (
	"reflect"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Protocol 12 (ADR-251): the child app's texts reach the host's backend,
// which shows the native message boxes and menus; a later set replaces the
// earlier one.
func TestStringsForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	want := platform.Strings{platform.StringMessageOK: "Aceptar", platform.StringMenuQuit: "Salir de {name}", "tela.toast.close": "Cerrar aviso"}
	a.ui(func() { a.b.Desktop().SetStrings(want) })
	if got := p.hb.DesktopStrings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("the host's strings %v, want %v", got, want)
	}
	next := platform.Strings{platform.StringMessageOK: "Accepter"}
	a.ui(func() { a.b.Desktop().SetStrings(next) })
	if got := p.hb.DesktopStrings(); !reflect.DeepEqual(got, next) {
		t.Errorf("the host's strings after a new language %v, want %v", got, next)
	}
	a.ui(func() { a.b.Desktop().SetStrings(nil) })
	if got := p.hb.DesktopStrings(); len(got) != 0 {
		t.Errorf("the host's strings after none %v", got)
	}
}

func TestStringMapRoundTrip(t *testing.T) {
	in := platform.Strings{"b": "2", "a": "1", "": "empty id"}
	e := newMsg(msgCtlNotify)
	e.stringMap(in)
	d := &dec{b: e.b[5:]}
	got := d.stringMap()
	if err := d.end(); err != nil || !reflect.DeepEqual(got, in) {
		t.Errorf("round trip = %v, %v; want %v", got, err, in)
	}
	e = newMsg(msgCtlNotify)
	e.u32(maxItems + 1)
	d = &dec{b: e.b[5:]}
	if d.stringMap(); d.err == nil {
		t.Error("a corrupt count decoded without an error")
	}
}
