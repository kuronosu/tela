package kit_test

import (
	"testing"
	"testing/fstest"

	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The toast viewport's name and the close button's (ADR-251) come from a
// field, else from the built-in labels "tela.toast.region" and
// "tela.toast.close" of the component's locale.

func loadToastCatalogs(t *testing.T) {
	t.Helper()
	// Made-up locales, so that no machine's own language ever matches them,
	// and an English catalog with the built-in texts: catalogs cannot be
	// unloaded, so the fallback must give what the other tests expect.
	err := i18n.Load(fstest.MapFS{
		"l/en.json":  {Data: []byte(`{"tela": {"toast": {"region": "Notifications", "close": "Close toast"}}}`)},
		"l/tlh.json": {Data: []byte(`{"tela": {"toast": {"region": "Avisos", "close": "Cerrar aviso"}}}`)},
		"l/tlx.json": {Data: []byte(`{"tela": {"toast": {"region": "Notices", "close": "Dismiss notice"}}}`)},
	}, "l/*.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i18n.SetLocale("") })
	if err := i18n.SetFallback("en"); err != nil {
		t.Fatal(err)
	}
}

func TestToasterLabelFields(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{Duration: kit.ToastForever, CloseButton: true, AriaLabel: "Mensajes", CloseLabel: "Descartar"})
	handle.Show("Hecho")
	app.ExpectRole("region", "Mensajes")
	app.ExpectCount(telatest.ByRole("button", "Descartar"), 1)
	app.ExpectCount(telatest.ByRole("button", "Close toast"), 0)
}

func TestToasterLabelsFromTheCatalogs(t *testing.T) {
	loadToastCatalogs(t)
	var handle kit.ToastHandle
	app := telatest.Mount(t, i18n.Provider("tlh", toastApp{&handle, kit.Toaster{Duration: kit.ToastForever, CloseButton: true}}),
		telatest.Size(480, 360), telatest.Theme(kit.Theme()))
	handle.Show("Hecho")
	app.ExpectRole("region", "Avisos")
	app.ExpectCount(telatest.ByRole("button", "Cerrar aviso"), 1)

	// A field wins over the catalogs.
	var handle2 kit.ToastHandle
	app2 := telatest.Mount(t, i18n.Provider("tlh", toastApp{&handle2, kit.Toaster{Duration: kit.ToastForever, CloseButton: true, CloseLabel: "Quitar"}}),
		telatest.Size(480, 360), telatest.Theme(kit.Theme()))
	handle2.Show("Hecho")
	app2.ExpectRole("region", "Avisos")
	app2.ExpectCount(telatest.ByRole("button", "Quitar"), 1)
}

// Switching the locale at run time renames both, and a locale without the
// keys gets the fallback's texts.
func TestToasterLabelsFollowTheLocale(t *testing.T) {
	loadToastCatalogs(t)
	app, handle := newToastApp(t, kit.Toaster{Duration: kit.ToastForever, CloseButton: true})
	i18n.SetLocale("tlh")
	handle.Show("Hecho")
	app.ExpectRole("region", "Avisos")
	app.ExpectCount(telatest.ByRole("button", "Cerrar aviso"), 1)
	i18n.SetLocale("tlx")
	app.ExpectRole("region", "Notices")
	app.ExpectCount(telatest.ByRole("button", "Dismiss notice"), 1)
	i18n.SetLocale("fr")
	app.ExpectRole("region", "Notifications")
	app.ExpectCount(telatest.ByRole("button", "Close toast"), 1)
}
