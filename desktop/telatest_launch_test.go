package desktop_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// telatest.StartLaunch gives the app its own start, per app (no process
// state: tests of other apps run in parallel), with the start at login
// Launch.AtLogin reports; without it there is no start launch.
func TestTelatestStartLaunch(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name    string
		opts    []telatest.Option
		want    []string
		atLogin bool
	}{
		{"none", nil, nil, false},
		{"files", []telatest.Option{telatest.StartLaunch(desktop.Launch{Dir: dir, Args: []string{"list.note"}})},
			[]string{filepath.Join(dir, "list.note") + ":start@start"}, false},
		{"at login", []telatest.Option{telatest.StartLaunch(desktop.Launch{AtLogin: true})}, []string{":start@start@login"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			app := telatest.MountFunc(t, func() tela.Node {
				desktop.UseLaunch(func(l desktop.Launch) {
					s := strings.Join(l.Files(), ",") + ":" + l.Source.String()
					if l.AtStart {
						s += "@start"
					}
					if l.AtLogin {
						s += "@login"
					}
					got = append(got, s)
				})
				return Div(Text("hello"))
			}, c.opts...)
			if !slices.Equal(got, c.want) {
				t.Errorf("launches %q, want %q", got, c.want)
			}
			// Later launches are the app's own business: InjectLaunch.
			got = nil
			app.InjectLaunch(desktop.Launch{Args: []string{"x"}})
			if len(got) != 1 || !strings.HasSuffix(got[0], ":forwarded") {
				t.Errorf("forwarded launch %q", got)
			}
		})
	}
}

// AskForFileDefaults plays Windows: MakeDefault of a file type opens the
// Default apps settings and returns DefaultAsked, leaving the default to
// the user (SetDefaultHandler); a scheme is still set at once.
func TestTelatestDefaultAsked(t *testing.T) {
	doc := desktop.FileType(".note", "application/x-notes-note", "Notes document")
	notes := desktop.URLScheme("notes")
	app := telatest.MountApp(t, tela.App{AppID: "com.example.Notes", Root: tela.C(func() tela.Node { return Text("notes") })})
	forApp := desktop.ForApp(desktop.HandlerApp{Exec: "/opt/notes/notes"})
	app.AskForFileDefaults(true)
	app.SetDefaultHandler(doc, "org.other.Editor") // the user's choice, which only the user changes
	if r, err := desktop.MakeDefault(doc, forApp); r != desktop.DefaultAsked || err != nil {
		t.Fatalf("MakeDefault of a file type = %v, %v; want DefaultAsked", r, err)
	}
	if s, _ := desktop.RegistrationStatus(doc); !s.Registered || s.Default {
		t.Errorf("status after being asked %+v: registered, but the user has not chosen", s)
	}
	if got := app.DefaultsAsked(); !slices.Equal(got, []string{".note"}) {
		t.Errorf("DefaultsAsked = %q, want [.note]", got)
	}
	if r, err := desktop.MakeDefault(notes, forApp); r != desktop.DefaultSet || err != nil {
		t.Errorf("MakeDefault of a scheme = %v, %v; want DefaultSet", r, err)
	}
	app.SetDefaultHandler(doc, "com.example.Notes")
	if s, _ := desktop.RegistrationStatus(doc); !s.Default {
		t.Errorf("status after the user chose it %+v", s)
	}
	app.AskForFileDefaults(false)
	if r, err := desktop.MakeDefault(doc, forApp); r != desktop.DefaultSet || err != nil {
		t.Errorf("MakeDefault after AskForFileDefaults(false) = %v, %v; want DefaultSet", r, err)
	}
}
