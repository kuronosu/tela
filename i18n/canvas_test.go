package i18n_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

// A change of the locale in a draw function panics in dev mode, by its own
// name and at the application's line, and before it changes or locks
// anything.
func TestSetLocaleInADraw(t *testing.T) {
	t.Cleanup(func() { i18n.SetLocale("") })
	i18n.SetLocale("en")
	var got string
	telatest.Mount(t, tela.C(func() tela.Node {
		return Canvas(func(*tela.Canvas) {
			defer func() { got = fmt.Sprint(recover()) }()
			i18n.SetLocale("es")
		})
	}))
	if !strings.Contains(got, "i18n.SetLocale called in the draw function") || !strings.Contains(got, "canvas_test.go") {
		t.Errorf("recovered %q", got)
	}
	if l := i18n.Locale(); l != "en" {
		t.Errorf("the locale is %q", l)
	}
	i18n.SetLocale("es") // does not hang
	if l := i18n.Locale(); l != "es" {
		t.Errorf("after it, the locale is %q", l)
	}
}
