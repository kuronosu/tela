package i18n

import (
	"strings"
	"testing"
	"testing/fstest"
)

// Replace makes the files the only catalogs: a message deleted from a file
// and a locale that is gone are gone, the locale set with SetLocale stays,
// and a file with an error changes nothing.
func TestReplace(t *testing.T) {
	isolate(t)
	mustLoad(t, map[string]string{"en.json": `{"a": "A1", "b": "B1"}`, "es.json": `{"a": "A-es"}`})
	SetLocale("es")
	if err := SetFallback("es"); err != nil {
		t.Fatal(err)
	}

	if err := Replace(catalogs(map[string]string{"en.json": `{"a": "A2"}`, "fr.json": `{"a": "A-fr"}`}), "locales/*.json"); err != nil {
		t.Fatal(err)
	}
	if got := Locales(); len(got) != 2 || got[0] != "en" || got[1] != "fr" {
		t.Errorf("Locales = %v, want the files' only", got)
	}
	if got := Locale(); got != "es" {
		t.Errorf("the locale is %q, want the explicit es kept", got)
	}
	// es has no catalog now, and the fallback set for it is forgotten: en.
	if got := T("a"); got != "A2" {
		t.Errorf("a = %q, want the replaced en", got)
	}
	if got := T("b"); got != "b" {
		t.Errorf("b = %q: a message deleted from the file is still there", got)
	}

	bad := catalogs(map[string]string{"en.json": `{"a": "{"}`})
	if err := Replace(bad, "locales/*.json"); err == nil {
		t.Error("Replace accepted a bad message")
	}
	if got := T("a"); got != "A2" {
		t.Errorf("a failed Replace changed the catalogs: a = %q", got)
	}
	if err := Replace(fstest.MapFS{}, "locales/*.json"); err == nil || !strings.Contains(err.Error(), "Replace") {
		t.Errorf("no files: %v, want an error that names Replace", err)
	}
}

// Unload removes the catalogs of some locales, or all of them.
func TestUnload(t *testing.T) {
	isolate(t)
	mustLoad(t, map[string]string{"en.json": `{"a": "A"}`, "es.json": `{"a": "A-es"}`, "fr.json": `{"a": "A-fr"}`})
	SetLocale("fr")

	if err := Unload("fr"); err != nil {
		t.Fatal(err)
	}
	if got := Locales(); len(got) != 2 || got[0] != "en" || got[1] != "es" {
		t.Errorf("Locales = %v after Unload(fr)", got)
	}
	if got := T("a"); got != "A" {
		t.Errorf("a = %q: the messages of fr should come from the fallback now", got)
	}
	if got := Locale(); got != "fr" {
		t.Errorf("the locale is %q, want the explicit fr kept for formatting", got)
	}

	// Nothing is removed when one tag is wrong.
	if err := Unload("es", "de"); err == nil || !strings.Contains(err.Error(), `"de"`) {
		t.Errorf("Unload of a locale that is not loaded: %v", err)
	}
	if err := Unload("not a tag"); err == nil {
		t.Error("Unload accepted a bad tag")
	}
	if got := Locales(); len(got) != 2 {
		t.Errorf("a failed Unload removed catalogs: %v", got)
	}

	if err := Unload(); err != nil {
		t.Fatal(err)
	}
	if got := Locales(); len(got) != 0 {
		t.Errorf("Locales = %v after Unload()", got)
	}
	// And it loads again.
	mustLoad(t, map[string]string{"en.json": `{"a": "A3"}`})
	SetLocale("en")
	if got := T("a"); got != "A3" {
		t.Errorf("after loading again a = %q", got)
	}
}

// The catalogs are the source of Tela's own labels, so Replace and Unload
// tell the components that use them.
func TestUnloadChangesTheLabels(t *testing.T) {
	isolate(t)
	mustLoad(t, map[string]string{"en.json": `{"tela": {"toast": {"close": "Cerrar"}}}`})
	if got := For("en").Label("tela.toast.close"); got != "Cerrar" {
		t.Fatalf("label = %q", got)
	}
	if err := Unload(); err != nil {
		t.Fatal(err)
	}
	if got := For("en").Label("tela.toast.close"); got == "Cerrar" {
		t.Errorf("label = %q after Unload, want Tela's English default", got)
	}
}
