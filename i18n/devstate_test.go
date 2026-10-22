//go:build teladev

package i18n

import (
	"testing"

	"github.com/kuronosu/tela/internal/core"
)

// The locale chosen at run time is kept across tela dev reloads (ADR-114):
// it is in the snapshot of a tree, and restored before the next tree mounts.
func TestDevStateKeepsTheLocale(t *testing.T) {
	t.Cleanup(func() { SetLocale("") })
	SetLocale("es-MX")
	data, rep, err := core.NewTree(core.Config{Dev: true}).CaptureDevState()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range rep.Extra {
		found = found || name == "tela/i18n.locale"
	}
	if !found {
		t.Fatalf("the snapshot has no locale: %+v", rep)
	}

	SetLocale("fr") // the new process starts with another locale
	core.NewTree(core.Config{Dev: true}).SeedDevState(data)
	if got := Locale(); got != "es-MX" {
		t.Errorf("locale %q after the restore, want es-MX", got)
	}

	// A locale that follows the system is not saved.
	SetLocale("")
	data, rep, err = core.NewTree(core.Config{Dev: true}).CaptureDevState()
	if err != nil {
		t.Fatal(err)
	}
	SetLocale("de")
	core.NewTree(core.Config{Dev: true}).SeedDevState(data)
	if got := Locale(); got != "de" {
		t.Errorf("locale %q: a saved system locale must not override the app's choice", got)
	}
}
