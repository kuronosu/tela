package telatest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// devCard is a struct component, which h.If rejects in dev mode.
type devCard struct{}

func (devCard) Build() tela.Node { return Text("card") }

func recoverMessage(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprint(r)
		}
	}()
	f()
	return ""
}

// The dev checks of node constructors called while a component builds
// (h.If with a struct component, unknown roles, input types and
// directions) follow the mounted app's dev mode, not the process default,
// which is on in a test binary: NoDev and App.Dev = false test what a
// release build does.
func TestNoDevTurnsOffConstructorChecks(t *testing.T) {
	bad := func() tela.Node {
		return Div(If(true, devCard{}), Role("buton"), Dir("sideways"), Input(Type("emial")))
	}
	off := false
	for name, mount := range map[string]func(){
		"NoDev":        func() { telatest.MountFunc(t, bad, telatest.NoDev()) },
		"MountApp Dev": func() { telatest.MountApp(t, tela.App{Root: bad, Dev: &off}) },
		"FromApp Dev":  func() { telatest.Mount(t, tela.C(bad), telatest.FromApp(tela.App{Dev: &off})) },
		"Render NoDev": func() {
			if _, err := telatest.Render(tela.C(bad), telatest.Size(100, 100), telatest.NoDev()); err != nil {
				panic(err)
			}
		},
		"one If, NoDev": func() { telatest.MountFunc(t, func() tela.Node { return Div(If(true, devCard{})) }, telatest.NoDev()) },
	} {
		if msg := recoverMessage(mount); msg != "" {
			t.Errorf("%s: panicked: %s", name, msg)
		}
	}
	// In dev mode the same code panics with the fix.
	msg := recoverMessage(func() { telatest.MountFunc(t, bad) })
	if !strings.Contains(msg, "If(cond, devCard{...})") {
		t.Errorf("dev mode: panic %q, want h.If's check", msg)
	}
	msg = recoverMessage(func() { telatest.MountFunc(t, func() tela.Node { return Div(Role("buton")) }) })
	if !strings.Contains(msg, `unknown ARIA role "buton"`) {
		t.Errorf("dev mode: panic %q, want Role's check", msg)
	}
}
