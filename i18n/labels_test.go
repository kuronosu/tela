package i18n_test

import (
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

// Tela's own labels (ADR-251) are catalog messages whose key is the
// label's ID.

func loadLabelCatalogs(t *testing.T) {
	t.Helper()
	load(t)
	err := i18n.Load(fstest.MapFS{
		"l/es.json": {Data: []byte(`{"tela": {"messagebox": {"ok": "Aceptar"}, "menu": {"quit": "Salir de {name}"}, "toast": {"close": "Cerrar aviso"}}}`)},
		"l/ar.json": {Data: []byte(`{"tela": {"messagebox": {"ok": "موافق"}}}`)},
	}, "l/*.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i18n.SetLocale("") })
}

func TestTelaLabelsFromTheCatalogs(t *testing.T) {
	loadLabelCatalogs(t)
	i18n.SetLocale("es")
	if got := tela.Label("tela.messagebox.ok"); got != "Aceptar" {
		t.Errorf("tela.Label in es = %q", got)
	}
	if got := tela.Label("tela.menu.quit", "name", "Notas"); got != "Salir de Notas" {
		t.Errorf("tela.Label with a name = %q", got)
	}
	// The catalogs lack it: Tela's English text.
	if got := tela.Label("tela.messagebox.cancel"); got != "Cancel" {
		t.Errorf("tela.Label of an untranslated ID = %q, want English", got)
	}
	i18n.SetLocale("ar")
	if got := tela.Label("tela.messagebox.ok"); got != "موافق" {
		t.Errorf("tela.Label in ar = %q", got)
	}
	// ar has no tela.toast.close; the fallback (en) has none either.
	if got := tela.Label("tela.toast.close"); got != "Close toast" {
		t.Errorf("tela.Label in ar without the key = %q, want English", got)
	}
	i18n.SetLocale("en")
	if got := tela.Label("tela.messagebox.ok"); got != "OK" {
		t.Errorf("tela.Label in en = %q", got)
	}
	// A missing label is not a dev report: they are optional.
}

func TestFormatterLabel(t *testing.T) {
	loadLabelCatalogs(t)
	i18n.SetLocale("en")
	if got := i18n.For("es").Label("tela.toast.close"); got != "Cerrar aviso" {
		t.Errorf("For(es).Label = %q", got)
	}
	if got := i18n.For("es").Label("tela.menu.about", "name", "Notas"); got != "About Notas" {
		t.Errorf("For(es).Label of an untranslated ID with a name = %q", got)
	}
	if got := i18n.T("tela.toast.close"); got != "Close toast" && got != "tela.toast.close" {
		t.Errorf("T in en = %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("Label of an unknown ID did not panic")
		}
	}()
	i18n.For("es").Label("tela.nothing")
}

// A component that calls tela.UseLabels builds again when the locale
// changes, and the platform is given the translated texts again.
func TestUseLabelsFollowsTheLocale(t *testing.T) {
	loadLabelCatalogs(t)
	i18n.SetLocale("en")
	var builds atomic.Int32
	app := telatest.MountFunc(t, func() tela.Node {
		label := tela.UseLabels()
		builds.Add(1)
		return h.Div(h.Button(h.Text(label("tela.messagebox.ok"))))
	})
	app.ExpectRole("button", "OK")
	if got := app.BackendStrings(); len(got) != 0 {
		t.Errorf("the backend has strings in en, where nothing is translated: %v", got)
	}
	before := builds.Load()
	i18n.SetLocale("es")
	app.ExpectRole("button", "Aceptar")
	if builds.Load() != before+1 {
		t.Errorf("builds = %d, want %d (one for the change)", builds.Load(), before+1)
	}
	strs := app.BackendStrings()
	if strs["tela.messagebox.ok"] != "Aceptar" || strs["tela.menu.quit"] != "Salir de {name}" {
		t.Errorf("the backend's strings after the change: %v", strs)
	}
}
