package desktop_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

type settingNote struct {
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}

func settingDefault() []settingNote {
	return []settingNote{
		{Title: "Welcome", Body: "hello", Pinned: true},
		{Title: "Second", Body: "world", Pinned: true},
	}
}

func openSettingStore(t *testing.T) *desktop.Store {
	t.Helper()
	telatest.TempDesktopDirs(t)
	s, err := desktop.OpenStore(filepath.Join(t.TempDir(), "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// settingProbe mounts a component that reads key through UseStoreSetting
// with def and returns a pointer to the value it last saw.
func settingProbe[T any](t *testing.T, s *desktop.Store, key string, def T) (*T, *telatest.App) {
	t.Helper()
	got := new(T)
	app := telatest.MountFunc(t, func() tela.Node {
		return tela.C(func() tela.Node {
			v, _ := desktop.UseStoreSetting(s, key, def)
			*got = v
			return Div(Text("x"))
		})
	})
	return got, app
}

// A stored element that omits a field must not inherit it from the
// default element at the same index.
func TestUseStoreSettingElementsDoNotInheritDefault(t *testing.T) {
	s := openSettingStore(t)
	if err := s.Set("notes", []settingNote{{Title: "Mine"}, {Title: "Other", Body: "b"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := settingProbe(t, s, "notes", settingDefault())
	want := []settingNote{{Title: "Mine"}, {Title: "Other", Body: "b"}}
	if len(*got) != 2 || (*got)[0] != want[0] || (*got)[1] != want[1] {
		t.Errorf("got %+v, want %+v", *got, want)
	}
}

// Decoding must not write into the default's backing array.
func TestUseStoreSettingDoesNotMutateDefault(t *testing.T) {
	s := openSettingStore(t)
	if err := s.Set("notes", []settingNote{{Title: "Mine"}}); err != nil {
		t.Fatal(err)
	}
	def := settingDefault()
	settingProbe(t, s, "notes", def)
	full := def[:cap(def)]
	if full[0] != settingDefault()[0] || full[1] != settingDefault()[1] {
		t.Errorf("the default was modified: %+v", full)
	}
}

// After a store change, a field the user cleared stays cleared.
func TestUseStoreSettingStoreChangeKeepsClearedField(t *testing.T) {
	s := openSettingStore(t)
	if err := s.Set("notes", settingDefault()); err != nil {
		t.Fatal(err)
	}
	var got []settingNote
	app := telatest.MountFunc(t, func() tela.Node {
		return tela.C(func() tela.Node {
			v, _ := desktop.UseStoreSetting(s, "notes", settingDefault())
			got = v
			return Div(Textf("n=%d pinned=%v", len(v), v[0].Pinned))
		})
	})
	app.ExpectText("n=2 pinned=true")
	_ = s.Set("notes", []settingNote{{Title: "Welcome", Body: "hello"}, {Title: "Second"}})
	app.ExpectText("n=2 pinned=false")
	if got[0].Pinned || got[1].Pinned || got[1].Body != "" {
		t.Errorf("a cleared field came back from the default: %+v", got)
	}
}

// UseSetting shows what another program writes to the settings file
// (ADR-253): the component builds again with the new value.
func TestUseSettingSeesOtherPrograms(t *testing.T) {
	s := openSettingStore(t)
	if err := s.Set("theme", "light"); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	app := telatest.MountFunc(t, func() tela.Node {
		v, _ := desktop.UseStoreSetting(s, "theme", "none")
		return Div(Textf("theme %s", v))
	})
	app.ExpectText("theme light")
	if err := os.WriteFile(s.Path(), []byte(`{"theme": "dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app.ExpectText("theme dark")
}
